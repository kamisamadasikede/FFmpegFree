// 格式目录加载（包 24）：由 api.check.ts 调用。用可控时钟，不真等 8 秒。
import { createCatalogController, catalogNotReady, CATALOG_LOAD_TIMEOUT_MS, CATALOG_NOT_READY_CODE } from './catalogLoad'
import { liveInterruptView, LP_BREAK_PULL, LP_BREAK_PUSH, LP_BREAK_PUSH_AWAY } from '@/errors/livePreviewMessages'
import { actionErrorText, detailWithoutPaths } from '@/errors/errorMessages'
import { parseDetailHead } from '@/api/call'
import { guardCall, isMseStateError } from '@/components/live/mseGuard'
import { previewRetryDelay } from '@/components/live/previewTiming'

type Eq = (name: string, got: unknown, want: unknown) => void

export async function catalogLoadChecks(eq: Eq, readSrc: (f: string) => string): Promise<void> {
  eq('格式目录超时是 8 秒', CATALOG_LOAD_TIMEOUT_MS, 8000)
  eq('整表 converter_not_ready 算旧结果；缺编码器不算', catalogNotReady([{ reasonCode: 'missing_encoder' }, { reasonCode: CATALOG_NOT_READY_CODE }]), true)
  eq('没有 converter_not_ready 就不算旧结果', catalogNotReady([{ reasonCode: 'missing_muxer' }, {}]), false)

  const clock = () => {
    let q: { fn: () => void; ms: number }[] = []
    const schedule = (fn: () => void, ms: number) => {
      const item = { fn, ms }
      q.push(item)
      return () => {
        q = q.filter((x) => x !== item)
      }
    }
    const fire = (ms: number) => {
      const due = q.filter((x) => x.ms <= ms)
      q = q.filter((x) => x.ms > ms)
      due.forEach((x) => x.fn())
    }
    const phases: string[] = []
    const ctl = createCatalogController({ timeoutMs: 8000, schedule, onPhase: (p) => phases.push(p) })
    return { ctl, fire, phases }
  }

  {
    // 冷启动：检测要 7 秒，后端最多等 6 秒就返回整表 converter_not_ready。骨架保持，不闪超时。
    const { ctl, fire, phases } = clock()
    const gen = ctl.start('checking')
    fire(6000)
    eq('6 秒时检测还没结束：计时没到，仍是加载', ctl.phase, 'loading')
    eq('检测中收到旧目录：保持骨架，不展示', ctl.resolve(gen, 'checking', true), 'hold')
    eq('收到旧目录后仍是加载，不是错误', ctl.phase, 'loading')
    fire(8000)
    eq('已经收到响应，8 秒到了也不报超时', ctl.phase, 'loading')
    eq('就绪事件到来要再取一次', ctl.onStatus('ready'), true)
    const gen2 = ctl.start('ready')
    eq('再取到正常目录：展示', ctl.resolve(gen2, 'ready', false), 'apply')
    eq('展示后阶段是 shown', ctl.phase, 'shown')
    eq('过期的第一次响应丢掉', ctl.resolve(gen, 'ready', false), 'ignore')
    eq('这一路没有进入错误', phases.includes('error'), false)
  }
  {
    // 请求一直没回来：满 8 秒才报错；之后的成功响应仍然用。
    const { ctl, fire } = clock()
    const gen = ctl.start('ready')
    fire(7999)
    eq('8 秒前还在加载', ctl.phase, 'loading')
    fire(8000)
    eq('8 秒没有响应：错误', ctl.phase, 'error')
    eq('迟到的成功响应仍然展示', ctl.resolve(gen, 'ready', false), 'apply')
    eq('迟到成功后是 shown', ctl.phase, 'shown')
  }
  {
    // 请求本身失败：马上是错误，不用等 8 秒。
    const { ctl, fire } = clock()
    const gen = ctl.start('checking')
    ctl.reject(gen)
    eq('请求失败马上是错误', ctl.phase, 'error')
    fire(8000)
    eq('失败之后到点不再改状态', ctl.phase, 'error')
    eq('就绪后自动再取一次', ctl.onStatus('ready'), true)
  }
  {
    // store 明确没就绪：不转圈，也不把目录里的旧原因当就绪。
    const { ctl } = clock()
    ctl.start('missing')
    eq('缺失时直接是未就绪态', ctl.phase, 'unready')
    eq('缺失时不展示旧目录', ctl.resolve(1, 'missing', true), 'ignore')
    eq('之后就绪会再取', ctl.onStatus('ready'), true)
  }
  {
    // 已经就绪但拿到的仍是旧结果：只自动再取一次，避免死循环。
    const { ctl } = clock()
    const gen = ctl.start('ready')
    eq('就绪但整表未就绪码：再取', ctl.resolve(gen, 'ready', true), 'reload')
    const gen2 = ctl.start('ready')
    eq('再取还是旧结果：停在错误，不再自动取', ctl.resolve(gen2, 'ready', true), 'ignore')
    eq('停在错误', ctl.phase, 'error')
    eq('不会第三次自动取', ctl.onStatus('ready'), false)
    const gen3 = ctl.start('ready', true)
    eq('用户重试后正常目录可以展示', ctl.resolve(gen3, 'ready', false), 'apply')
  }
  {
    const panel = readSrc('src/components/convert/ConvertFormatPanel.vue')
    const foot = readSrc('src/components/convert/ConvertSettingsPanel.vue')
    const store = readSrc('src/stores/convertRecords.ts')
    eq('格式列：加载用骨架，超时文案和重试，没就绪带去设置', [/cv-fxsk/.test(panel), /格式没能加载出来。/.test(panel), /重试/.test(panel), /转换组件还没有就绪。/.test(panel), /去设置/.test(panel)], [true, true, true, true, true])
    eq('格式列不出现“未就绪 / 尚未就绪”（只有“还没有就绪”这一句）', /未就绪|尚未就绪/.test(panel + foot), false)
    eq('“正在加载格式”在按钮下面，不在格式块上转圈文案', /正在加载格式/.test(foot), true)
    eq('目录加载看 ffmpeg store 的状态，事件名是 ffmpeg:status', [/createCatalogController/.test(store), /ffmpeg\.status\.state/.test(store), /'ffmpeg:status'/.test(readSrc('src/stores/ffmpeg.ts'))], [true, true, true])
    const ff = readSrc('src/stores/ffmpeg.ts')
    const norm = ff.slice(ff.indexOf('function normalize'), ff.indexOf('const PREVIEW'))
    eq('组件状态不读 path，normalize 不用 as any', [/path: r\.path/.test(ff), /as any/.test(norm), /customPathInvalid/.test(ff)], [false, false, true])
    eq('失败页只查 failed，不含 interrupted', /failed: \['failed'\]/.test(readSrc('src/stores/tasks.ts')), true)
    eq('设备列表在 state 变成 ready 时强制重测', /state === 'ready' && prev !== 'ready'/.test(readSrc('src/components/encoder/EncoderDevicePanel.vue')), true)
  }
  eq('reason 认前缀，stderr 跟在后面不影响', [parseDetailHead('reason=push\n[flv] error').reason, parseDetailHead('reason=pull leftover').reason, parseDetailHead('kind=window').kind], ['push', 'pull', 'window'])
  eq('reason=push 直播页 / 别处两句；reason=pull 始终拉流那句；不看 message', [
    liveInterruptView({ reason: 'push', onLivePage: true })?.sentence,
    liveInterruptView({ reason: 'push' })?.sentence,
    liveInterruptView({ reason: 'pull', onLivePage: true })?.sentence,
    liveInterruptView({ reason: 'pull', taskType: 'live_file_push' })?.sentence,
  ], [LP_BREAK_PUSH, LP_BREAK_PUSH_AWAY, LP_BREAK_PULL, LP_BREAK_PULL])
  eq('没有 reason 时按任务类型；LIVE_SOURCE_GONE 不改成中断句', [
    liveInterruptView({ taskType: 'live_screen_push' })?.sentence,
    liveInterruptView({ taskType: 'live_pull' })?.sentence,
    liveInterruptView({ code: 'LIVE_SOURCE_GONE', reason: 'push', taskType: 'live_screen_push' }),
    liveInterruptView({ reason: 'other', taskType: 'live_file_push', onLivePage: true }),
  ], [LP_BREAK_PUSH_AWAY, LP_BREAK_PULL, null, null])
  eq('旧版导出的重试文案原样显示', actionErrorText('UNSUPPORTED', '旧版导出记录只能查看和删除，不能重试'), '旧版导出记录只能查看和删除，不能重试')
  eq('detail 里的路径行和 reason= 不给用户看', detailWithoutPaths('reason=push\nC:\\Tools\\ffmpeg.exe\n请重试'), '请重试')
  eq('预览 503 前 4 次隔 200ms，之后 1 秒', [previewRetryDelay(0), previewRetryDelay(3), previewRetryDelay(4)], [200, 200, 1000])
  eq('InvalidStateError 被接住，别的错误照旧抛', [
    guardCall(() => { const e = new Error('mse'); e.name = 'InvalidStateError'; throw e }),
    isMseStateError(Object.assign(new Error('x'), { name: 'InvalidAccessError' })),
    (() => { try { guardCall(() => { throw new Error('nope') }); return 'swallowed' } catch { return 'threw' } })(),
  ], [undefined, true, 'threw'])
  {
    const player = readSrc('src/components/live/LivePlayer.vue')
    const leave = player.slice(player.indexOf('onDeactivated(() =>'))
    eq('离开页面不换 video，等销毁后再换', [/installMseGuard\(\)/.test(player), /afterMseSettles/.test(player), /videoKey\.value\+\+/.test(leave)], [true, true, false])
  }
}
