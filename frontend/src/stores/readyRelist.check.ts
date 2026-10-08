// 包 20 小修自检（由 api.check.ts 调用）：
// ① 转换组件就绪后补取一次源文件列表（后端 PR #100 配合，stores/readyRelist.ts + convertRecords 接线）
// ② 走查 D2 完成横幅“结果已保存到输出文件夹 + 打开文件夹”  ③ D3 音频行不取缩略图、不扫光
// ④ D4 预览 reason=copying → “准备中”  ⑤ D7 图片格式（含 GIF）不说“没有声音”
import { createReadyRelist } from './readyRelist'
import { NO_SOUND_INFO_EXTS, sourceMetaText } from '@/utils/convertText'
import { PREVIEW_COPYING_TEXT } from '@/utils/convertSubmit'

type Eq = (name: string, got: unknown, want: unknown) => void
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

export async function readyRelistChecks(eq: Eq, readSrc: (f: string) => string): Promise<void> {
  // ---------- ① 运行时：用可控的假列表请求 ----------
  const mk = (o: { ready: boolean; needs?: boolean }) => {
    const st = { ready: o.ready, needs: o.needs ?? true, relists: 0, fetches: 0 }
    let release: (() => void) | null = null
    const g = createReadyRelist({
      isReady: () => st.ready,
      needs: () => st.needs,
      relist: async () => {
        st.relists++
        await g.track(async () => void st.fetches++)
      },
    })
    /** 一次列表请求，返回 resolve 函数（模拟在途） */
    const slowFetch = () => {
      const p = g.track(() => new Promise<void>((r) => (release = r)))
      return { p, done: () => release?.() }
    }
    return { st, g, slowFetch }
  }

  {
    // 启动时已经 ready：列表请求时 ready → 之后不会补
    const { st, g } = mk({ ready: true })
    await g.track(async () => {})
    g.onReady()
    await sleep(5)
    eq('RL 启动时已 ready：不补取', st.relists, 0)
  }
  {
    // 未就绪时列过 → 变 ready 补一次；再来一次 ready 事件（同一状态）不再补
    const { st, g } = mk({ ready: false })
    await g.track(async () => {})
    st.ready = true
    g.onReady()
    await sleep(5)
    eq('RL 未就绪时列过 → 就绪后补一次', st.relists, 1)
    g.onReady()
    await sleep(5)
    eq('RL 补过以后再次 onReady（没有新的未就绪列表）：不再补', st.relists, 1)
    // 又掉线又恢复，期间列过 → 这次转入 ready 再补一次
    st.ready = false
    await g.track(async () => {})
    st.ready = true
    g.onReady()
    await sleep(5)
    eq('RL 每次转入 ready 最多补一次（期间未就绪时列过）', st.relists, 2)
  }
  {
    // 行都有 media 了（needs=false）：不补
    const { st, g } = mk({ ready: false, needs: false })
    await g.track(async () => {})
    st.ready = true
    g.onReady()
    await sleep(5)
    eq('RL 行都已有 media：不补', st.relists, 0)
  }
  {
    // 在途：不并发第二个，等它结束后补一次
    const { st, g, slowFetch } = mk({ ready: false })
    const f = slowFetch()
    st.ready = true
    g.onReady()
    g.onReady()
    await sleep(5)
    eq('RL 有列表请求在途：不另发', [st.relists, g.state.inflight, g.state.pending], [0, 1, true])
    f.done()
    await f.p
    await sleep(5)
    eq('RL 在途的结束后补一次（只一次）', [st.relists, st.fetches, g.state.pending, g.state.listedUnready], [1, 1, false, false])
  }
  {
    // 在途的那次结束时行已经补齐（后端等到了检测）：needs=false → 不再补
    const { st, g, slowFetch } = mk({ ready: false })
    const f = slowFetch()
    st.ready = true
    g.onReady()
    st.needs = false
    f.done()
    await f.p
    await sleep(5)
    eq('RL 在途那次已经补齐：结束后不再补', st.relists, 0)
  }
  {
    // 补取失败不抛到外面
    const g = createReadyRelist({ isReady: () => true, needs: () => true, relist: async () => Promise.reject(new Error('x')) })
    const warn = console.warn
    console.warn = () => {}
    let threw = false
    try {
      const notReady = createReadyRelist({ isReady: () => false, needs: () => true, relist: async () => {} })
      await notReady.track(async () => {})
      await g.track(async () => {})
      g.onReady()
      await sleep(5)
    } catch {
      threw = true
    } finally {
      console.warn = warn
    }
    eq('RL 补取失败只 warn', threw, false)
  }

  // ---------- ① store 接线（源码） ----------
  const st = readSrc('src/stores/convertRecords.ts')
  eq('RL store：所有 ListSources 都经过 readyRelist.track（API 改名导入）', [
    /listSources as apiListSources/.test(st),
    /const listSources = \(f: Parameters<typeof apiListSources>\[0\]\) => readyRelist\.track\(\(\) => apiListSources\(f\)\)/.test(st),
    (st.match(/apiListSources\(/g) ?? []).length,
  ], [true, true, 1])
  eq('RL store：只在 ready 变 true 时调用 onReady（非 immediate）；原来的缩略图重取保留', [
    /watch\(\(\) => ffmpeg\.ready, \(ok\) => ok && readyRelist\.onReady\(\)\)/.test(st),
    /watch\(\(\) => ffmpeg\.ready, \(ok\) => ok && retryFailedThumbs\(\)\)/.test(st),
    /watch\(\(\) => ffmpeg\.ready, \(ok\) => ok && void probePending\(\)\)/.test(st),
  ], [true, true, true])
  const relistBody = /async function relistInPlace\(\) \{([\s\S]*?)\n  \}\n/.exec(st)?.[1] ?? ''
  eq('RL store：原地合并（putEntries），不清勾选 / 不动 loading / 不清 pinned', [
    /listOffset\.value \+= putEntries\(p\.items\)/.test(relistBody),
    /selected\.(clear|delete)|loading\.value =|pinned\.value =|\bsources\[[^\]]+\] = /.test(relistBody),
    /needs: \(\) => inited && \(!loaded\.value \|\| Object\.values\(sources\)\.some\(\(s\) => !s\.media && s\.exists !== false\)\)/.test(st),
  ], [true, false, true])

  // ---------- ② D2 完成横幅 ----------
  const page = readSrc('src/views/ConvertPage.vue')
  eq('D2 横幅：有失败只留“本轮完成 a 项，失败 b 项”，不出现保存说明和打开按钮', [
    page.includes('结果已保存到输出文件夹'),
    /v-if="cv\.roundBanner\.ok && !cv\.roundBanner\.fail"/.test(page),
    /源文件下(?!面)|源文件所在/.test(page),
  ], [true, true, false])
  eq('D2 打开：自定义文件夹 RevealInFolder，默认 OpenStorageFolder("output")；提交时记下本轮的输出位置', [
    /if \(dir\) await revealInFolder\(dir\)\s*else await openStorageFolder\('output'\)/.test(st),
    (st.match(/for \(const t of res\.tasks\) roundDirs\.set\(t\.id, dir\)/g) ?? []).length,
    /showBanner\(ok, fail, dirs\.size === 1 \? \[\.\.\.dirs\]\[0\] : ''\)/.test(st),
    /export async function openStorageFolder\(kind: 'output' \| 'uploads'\)[\s\S]*?SystemBinding\.OpenStorageFolder\(kind\)/.test(readSrc('src/api/system.ts')),
  ], [true, 2, true, true])

  // ---------- ③ D3 音频不取缩略图 ----------
  const ens = /function ensureThumb\(s: SourceRow\) \{([\s\S]*?)\n  \}\n/.exec(st)?.[1] ?? ''
  const iAudio = ens.indexOf("coverKindOf(extOf(s.name), metaInfoOf(s)) === 'audio'")
  eq('D3 ensureThumb：音频行直接音符封面，在请求缩略图之前返回', [iAudio > 0, iAudio < ens.indexOf('getSourceThumbnail'), /s\.thumb = \{ kind: 'type' \}/.test(ens)], [true, true, true])
  eq('D3 ConvertThumb：音频封面不扫光', /kind\.value !== 'audio' && !props\.pend && !props\.state/.test(readSrc('src/components/convert/ConvertThumb.vue')), true)

  // ---------- ④ D4 预览 reason=copying ----------
  eq('D4 文案', PREVIEW_COPYING_TEXT, '文件还在准备中，准备好后才能预览。')
  eq('D4 列表行的 title / data-tip 没有“复制完成后才能预览”（没有这种提示就保持为空）',
    /复制完成后才能预览/.test(readSrc('src/components/convert/ConvertSourceRow.vue') + readSrc('src/components/convert/ConvertKid.vue')), false)
  const pv = readSrc('src/components/convert/ConvertPreviewDialog.vue')
  const iCopy = pv.indexOf("if (r.reason === 'copying') {")
  eq('D4 预览：reason=copying 不论错误码都映射成“准备中”（排在 UNSUPPORTED / 其他错误之前）', [iCopy > 0, iCopy < pv.indexOf("r.code === 'UNSUPPORTED') stage.value = 'unplayable'"), /errText\.value = PREVIEW_COPYING_TEXT/.test(pv)], [true, true, true])

  // ---------- ⑤ D7 图片格式不说“没有声音” ----------
  const silentVideo = { width: 1200, height: 1600, videoCodec: 'png', hasVideo: true, hasAudio: false, duration: 0, size: 23245 }
  eq('D7 PNG / GIF / JPG 不显示“没有声音”；无声 MP4 照旧显示', [
    sourceMetaText(silentVideo, 'png').includes('没有声音'),
    sourceMetaText({ ...silentVideo, videoCodec: 'gif', duration: 3 }, 'GIF').includes('没有声音'),
    sourceMetaText(silentVideo, 'jpg').includes('没有声音'),
    sourceMetaText({ ...silentVideo, videoCodec: 'h264', duration: 10 }, 'mp4').includes('没有声音'),
    sourceMetaText({ ...silentVideo, videoCodec: 'h264', duration: 10 }).includes('没有声音'),
  ], [false, false, false, true, true])
  eq('D7 NO_SOUND_INFO_EXTS 含 gif 和静态图片；父行传扩展名', [NO_SOUND_INFO_EXTS.includes('gif'), NO_SOUND_INFO_EXTS.includes('png'), /sourceMetaText\(info\.value, ext\.value\)/.test(readSrc('src/components/convert/ConvertSourceRow.vue'))], [true, true, true])
}
