// 包 22 自检（由 api.check.ts 调用）：契约 v0.25.1 的三条规则（暂存 / 对齐 / 只往终态走）+ 走查 af6a508 的前端项。
import { createPinia, setActivePinia } from 'pinia'
import { canApplySnapshot, createEarlyEvents, PullOutcomeGate, type PullOutcome } from './eventOrder'
import { liveAnnouncement, nextLagShown, stageAspectOf, type AnnounceInput } from '@/components/live/livePlayerLogic'
import * as lp from '@/errors/livePreviewMessages'
import { pullBreakText, pullEndedView } from '@/api/livePreviewStream'
import { componentFolderErrorText, COMPONENT_NOT_READY_TEXT, openComponentFolder } from '@/api/system'
import { AppError } from '@/api/call'

type Eq = (name: string, got: unknown, want: unknown) => void
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
const win = (globalThis as unknown as { window: { location: { search: string } } }).window

export async function pkg22Checks(eq: Eq, readSrc: (f: string) => string): Promise<void> {
  // ---- ① 暂存 ----
  {
    const b = createEarlyEvents<{ seq: number; v: string }>(2)
    b.hold('a', { seq: 2, v: 'copying' })
    b.hold('a', { seq: 1, v: 'old' }) // 更小的 seq 不覆盖
    b.hold('a', { seq: 3, v: 'ready' })
    eq('v0.25.1 ① 暂存：同一 id 只留 seq 最大的一条', b.take('a'), { seq: 3, v: 'ready' })
    eq('v0.25.1 ① 暂存：取出后删除', [b.take('a'), b.size], [undefined, 0])
    b.hold('x', { seq: 1, v: '' }); b.hold('y', { seq: 2, v: '' }); b.hold('z', { seq: 3, v: '' })
    eq('v0.25.1 ① 暂存：超过上限丢最早暂存的', [b.has('x'), b.has('y'), b.has('z'), b.size], [false, true, true, 2])
  }
  // ---- ② 对齐 ----
  eq('v0.25.1 ② 对齐：查询在途时这一行又收到事件 → 不用快照覆盖', [canApplySnapshot(0, 0), canApplySnapshot(4, 4), canApplySnapshot(4, 5)], [true, true, false])

  // ---- S1：真正走一遍转换页的 store（模拟层 ?cv_copyrace=） ----
  {
    const mock = await import('@/api/convertRecordsMock')
    const { useConvertRecordsStore } = await import('./convertRecords')
    const prev = win.location.search
    try {
      setActivePinia(createPinia())
      const cv = useConvertRecordsStore()
      mock.resetConvertMock('empty')
      await cv.init()
      win.location.search = '?cv_copyrace=1'
      await cv.addPaths(['D:\\Videos\\race-small.mp4'])
      await sleep(0)
      const row = Object.values(cv.sources).find((r) => r.name === 'race-small.mp4')
      eq('S1：copying / ready 事件都早于 AddSources 返回（快照还是复制中）→ 暂存后补上，直接就绪，不用刷新', [row?.copyState, row?.copySeq !== 0, row ? cv.isCheckable(row) : null], ['ready', true, true])
      win.location.search = '?cv_copyrace=lost'
      await cv.addPaths(['D:\\Videos\\race-lost.mp4'])
      await sleep(0)
      const lost = Object.values(cv.sources).find((r) => r.name === 'race-lost.mp4')
      eq('S1：ready 事件丢了 → AddSources 返回后用这一批 id 再查一次（GetSource）自己恢复成就绪', lost?.copyState, 'ready')
    } finally {
      win.location.search = prev
    }
    eq('S1 源码：applyCopyEvent 收到没入列的行不再直接 return，而是 earlyCopy.hold；upsertSource 入列时 earlyCopy.take', [/if \(!row\) \{\s*earlyCopy\.hold/.test(readSrc('src/stores/convertRecords.ts')), /earlyCopy\.take\(s\.sourceId\)/.test(readSrc('src/stores/convertRecords.ts')), /reconcileBatch\(/.test(readSrc('src/stores/convertRecords.ts'))], [true, true, true])
  }

  // ---- ③ 拉流只往终态走（先到先定） ----
  {
    const run = async (hasSession: boolean, steps: (g: PullOutcomeGate) => void | Promise<void>) => {
      const got: PullOutcome[] = []
      const g = new PullOutcomeGate((o) => got.push(o), () => hasSession, 40)
      await steps(g)
      g.dispose()
      return got.map((o) => `${o.phase}/${o.source}${o.byUser ? '/user' : ''}${o.message ? '/' + o.message : ''}`)
    }
    eq('v0.25.1 ③ 先到 ended、后到 interrupted：保持已结束', await run(true, (g) => { g.event('ended'); g.event('interrupted') }), ['ended/event'])
    eq('v0.25.1 ③ 先到 interrupted、后到 ended：保持被中断', await run(true, (g) => { g.event('interrupted'); g.event('ended') }), ['interrupted/event'])
    eq('v0.25.1 ③ 用户点停止后到的事件都不改', await run(true, (g) => { g.user(); g.event('interrupted'); g.event('failed', 'x') }), ['ended/user/user'])
    eq('G2：有后端会话时播放器先读到结尾 → 等后端事件；后端说 interrupted 就是被中断', await run(true, async (g) => { g.player('ended'); await sleep(5); g.event('interrupted'); await sleep(60) }), ['interrupted/event'])
    eq('G2：等不到后端事件 → 按播放器的结果（结尾 = 已结束）', await run(true, async (g) => { g.player('ended'); await sleep(70); g.event('interrupted') }), ['ended/player'])
    eq('G2：没有后端会话（ws 直连）→ 播放器结果马上定', await run(false, (g) => { g.player('ended'); g.event('interrupted') }), ['ended/player'])
    eq('G2：不支持的编码不用等', await run(true, (g) => { g.player('unsupported') }), ['unsupported/player'])
    eq('failed 事件 = 被中断，带后端 message', await run(true, (g) => { g.event('failed', '拉流失败，请检查直播地址和网络。') }), ['interrupted/event/拉流失败，请检查直播地址和网络。'])
    eq('reset 之后（重新拉流）才能再定一次', await run(true, (g) => { g.event('ended'); g.reset(); g.event('interrupted') }), ['ended/event', 'interrupted/event'])
    eq('close（离开页面）之后什么都不回调', await run(true, (g) => { g.close(); g.event('interrupted'); g.player('ended') }), [])
    const pp = readSrc('src/views/live/PullPlay.vue')
    eq('PullPlay：live:pull 和播放器结果都只经过 PullOutcomeGate；不再有直接改 phase 的 onEnded / onBroken 分支', [/new PullOutcomeGate\(/.test(pp), /gate\.event\(e\.state/.test(pp), /gate\.player\('ended'\)/.test(pp), /gate\.player\('interrupted'\)/.test(pp), /function finish\(/.test(pp)], [true, true, true, true, false])
  }
  // ---- 文案 ----
  eq('G2 文案：没点停止先到 ended → 标题 + 第二行 + 重新拉流；用户停止只有标题', [pullEndedView(false), pullEndedView(true)], [{ title: '拉流已结束', note: '直播已停止，或连接已断开。', retry: true }, { title: '拉流已结束', note: '', retry: false }])
  eq('G3 兜底：没有 message / message 写的是推流 → 拉流失败那句；拉流自己的 message 原样', [pullBreakText(''), pullBreakText('推流启动失败'), pullBreakText('拉流失败，请检查直播地址和网络。'), pullBreakText('对方拒绝了连接。')], ['拉流失败，请检查直播地址和网络。', '拉流失败，请检查直播地址和网络。', '拉流失败，请检查直播地址和网络。', '对方拒绝了连接。'])
  eq('拉流 preview_unavailable 与 codec 两句分开，推流页仍用推流那句', [lp.LP_UNAVAILABLE_PULL, lp.LP_UNSUP_PULL, lp.LP_UNAVAILABLE], ['这路视频暂时无法在应用内播放。', '这路视频无法在应用内播放。', '这路视频无法在应用内预览，推流不受影响。'])
  eq('LivePlayer：unavailable 按页面分', /reason === 'unavailable'\) return props\.kind === 'pull' \? LP_UNAVAILABLE_PULL : LP_UNAVAILABLE/.test(readSrc('src/components/live/LivePlayer.vue')), true)
  eq('X3 报错列全四种前缀；X2 占位符不含 flv', [lp.LP_PULL_URL_INVALID, /flv/i.test(lp.LP_PULL_PLACEHOLDER), /room\.flv|请输入 http:\/\/ 或 ws:\/\//.test(readSrc('src/views/live/PullPlay.vue'))], ['直播地址需要以 http://、https://、ws:// 或 wss:// 开头。', false, false])
  eq('G6：地址改了就收起报错，开始校验通过时也清掉', [/watch\(url, \(\) => \{ if \(!livePreview\) urlInvalid\.value = false \}\)/.test(readSrc('src/views/live/PullPlay.vue')), /urlInvalid\.value = false\n\s*unwatch/.test(readSrc('src/views/live/PullPlay.vue'))], [true, true])

  // ---- G4 / G5 / 追帧胶囊 ----
  eq('G4：比例 = 指定 > 视频自己的宽高比 > 16:9', [stageAspectOf(undefined, 9 / 16), stageAspectOf(16 / 9, 9 / 16), stageAspectOf(undefined, null), stageAspectOf(undefined, NaN)], [9 / 16, 16 / 9, 16 / 9, 16 / 9])
  eq('G4：PullPlay / 推流预览不再写死 16:9', [/:aspect="vis\?\.aspect \?\? 16 \/ 9"/.test(readSrc('src/views/live/PullPlay.vue')), /:aspect="16 \/ 9"/.test(readSrc('src/components/live/LivePushPreview.vue'))], [false, false])
  {
    const T = { connecting: lp.LP_CONNECTING, startedPush: lp.LP_LIVE_STARTED_PUSH, startedPull: lp.LP_LIVE_STARTED_PULL, mutedSuffix: lp.LP_LIVE_MUTED_SUFFIX, buffering: lp.LP_LIVE_BUFFERING }
    const a = (o: Partial<AnnounceInput>) => liveAnnouncement({ phase: 'playing', kind: 'pull', startMuted: false, hasAudio: true, bufferingLong: false, overlayText: '', endedNote: '', text: T, ...o })
    eq('G5 播报：连接中 / 有声开始 / 静音开始 / 没声音不说已静音 / 缓冲超 2 秒 / 结束带第二行 / 被中断 / 未开始不变', [
      a({ phase: 'connecting' }), a({}), a({ kind: 'push', startMuted: true }), a({ kind: 'push', startMuted: true, hasAudio: false }), a({ phase: 'buffering', bufferingLong: true }),
      a({ phase: 'ended', overlayText: lp.LP_END_PULL, endedNote: lp.LP_END_PULL_REMOTE }), a({ phase: 'interrupted', overlayText: lp.LP_BREAK_PULL }), a({ phase: 'empty' }),
    ], ['正在连接…', '拉流已开始', '推流预览已开始，已静音', '推流预览已开始', '正在缓冲', '拉流已结束。直播已停止，或连接已断开。', '拉流被中断，请重新拉流。', null])
  }
  eq('「回到最新」：超过 3 秒出现、1.5 秒以下收起、中间保持', [nextLagShown(null, 2.9), nextLagShown(null, 3.6), nextLagShown(4, 2), nextLagShown(4, 1.4)], [null, 4, 2, null])
  {
    const pl = readSrc('src/components/live/LivePlayer.vue')
    eq('追帧：开着时加速（liveSync）+ 落后 6 秒跳到最新；胶囊只在追帧关闭时出现；缓冲不出转圈（只播报）', [/liveSync: true/.test(pl), /liveBufferLatencyMaxLatency: 6/.test(pl), /props\.lowLatency \? null : lagOwn\.value/.test(pl), /:class="\{ dimbg: dimmed, buf: phase === 'buffering' \}"/.test(pl) && !/phase === 'buffering' \|\| buffering\.value/.test(pl)], [true, true, true, true])
    eq('G1：结束 / 中断前把最后一帧画到画布上再销毁播放器', [/if \(dimmed\.value\) freeze\(\)/.test(pl), /class="lp-shot"/.test(pl)], [true, true])
    eq('X1：没有声音时不出「已静音」提示', /hintOn\.value && props\.hasAudio/.test(pl), true)
  }
  eq('X4：面板外层不叫 .pop（否则 scoped 样式叠两次，离入口 16px）', [/class="drop"/.test(readSrc('src/components/live/LiveSessionEntry.vue')), /\.pop \{ position: absolute/.test(readSrc('src/components/live/LiveSessionEntry.vue'))], [true, false])

  // ---- X5 ----
  {
    const fp = readSrc('src/components/settings/FFmpegPanel.vue')
    eq('X5：转换组件面板没有路径行（不显示 status.path、没有 pathbox / title 路径）', [/status\.path/.test(fp), /pathbox/.test(fp), /转换组件已就绪/.test(fp)], [false, false, true])
    eq('X5：设置页「打开组件所在文件夹」走 OpenStorageFolder("component")，NOT_FOUND →「转换组件还没有就绪。」', [/打开组件所在文件夹/.test(readSrc('src/views/Settings.vue')), /OpenStorageFolder\('component'\)/.test(readSrc('src/api/system.ts')), componentFolderErrorText(new AppError('NOT_FOUND', 'x')), componentFolderErrorText(new AppError('INVALID_ARGUMENT', '/usr/bin/x'))], [true, true, COMPONENT_NOT_READY_TEXT, '没能打开组件所在文件夹，请稍后再试。'])
    let err = ''
    try { await openComponentFolder(false) } catch (e) { err = componentFolderErrorText(e) }
    eq('X5 模拟层：组件没就绪 → NOT_FOUND', err, COMPONENT_NOT_READY_TEXT)
  }
}
