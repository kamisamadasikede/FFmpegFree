// 转换页 v2（契约 v0.23 §6.14 + v0.23.1）自检：由 api.check.ts 调用。只测数据层 / 模拟层 / 纯函数和几处源码约定，不渲染组件。
import { toAppError, type AppError } from './call'
import { recordOf, parseParams, thumbStateOf, isThumbUrl } from './convertRecords'
import * as mock from './convertRecordsMock'
import { cancelSimTask, getSimTask, hideSimFinished, listSimFinished, retrySimTask, unhideSimTasks } from './sim'
import { onSimEvent } from '@/services/wails'
import { createPinia, setActivePinia } from 'pinia'
import { useTaskStore } from '@/stores/tasks'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useConvertRecordsStore } from '@/stores/convertRecords'
import { nextTick } from 'vue'
import { FFPROBE_MISSING_TEXT, liveFfmpegProtocolMissingText, LIVE_FFMPEG_PROTOCOL_MISSING_TEXT } from '@/errors/errorMessages'
import { midEllipsis, midTailMin } from '@/utils/midEllipsis'
import { sourceMetaText, dupPresetTitles, presetShortTitle, setPresetCatalog, recordParamsText, recordLine, deleteToast, toastText, revealDeleteFailureText, sourceRemovedParts, conflictReason, CONFLICT_NO_AUDIO, CONFLICT_NO_VIDEO, CONFLICT_TITLE, formatRecordTime, coverKindOf, extOf, isHevcCodec, unplayableHint, REVEAL_LABEL } from '@/utils/convertText'
import { codecName, rowInfoText, videoCodecText, audioCodecText } from '@/utils/mediaText'
import { metaInfoOf } from '@/stores/convertRecords'
import { submitResultOf, skippedNotice, submitCopyErrorText, SUBMIT_COPYING_TEXT, SUBMIT_COPY_FAILED_TEXT } from '@/utils/convertSubmit'
import { store as goStore } from '../../wailsjs/go/models'

type Eq = (name: string, got: unknown, want: unknown) => void
async function rejects(p: Promise<unknown>): Promise<AppError | null> {
  try {
    await p
    return null
  } catch (e) {
    return toAppError(e)
  }
}

export async function convertV2Checks(eq: Eq, readSrc: (f: string) => string): Promise<void> {
  // ---- 包 20：SubmitSources 的 skipped（v0.24 §6.15.4 第 6 条），文案是产品经理 10-08 定稿 ----
  {
    eq('submitResultOf：新形状 / 旧形状 Task[] / null', [submitResultOf({ tasks: [1], skipped: [{ sourceId: 'a', reason: 'copying' }] }), submitResultOf([1, 2]), submitResultOf(null), submitResultOf({ tasks: null, skipped: null })],
      [{ tasks: [1], skipped: [{ sourceId: 'a', reason: 'copying' }] }, { tasks: [1, 2], skipped: [] }, { tasks: [], skipped: [] }, { tasks: [], skipped: [] }])
    const sk = (r: string) => ({ sourceId: 'x', reason: r })
    eq('跳过提示：还在准备中 / 没能准备好 / 两类都有 / 没有', [skippedNotice([sk('copying'), sk('copying')]), skippedNotice([sk('copy_failed'), sk('copy_canceled')]), skippedNotice([sk('copying'), sk('copy_canceled')]), skippedNotice([sk('weird')]), skippedNotice([])],
      ['有 2 个文件还在准备中，已转换其余文件。准备好后再点转换。', '有 2 个文件没能准备好，已转换其余文件。请在列表里点“重试”后再转换。', '有 1 个文件还在准备中，已转换其余文件。准备好后再点转换。另有 1 个文件没能准备好，请在列表里点“重试”。', '有 1 个文件没能准备好，已转换其余文件。请在列表里点“重试”后再转换。', ''])
    eq('整体失败：TASK_CONFLICT reason=copying / copy_failed 给短提示；其余照旧', [submitCopyErrorText({ code: 'TASK_CONFLICT', detail: 'reason=copying\nsourceId=AB12' }), submitCopyErrorText({ code: 'TASK_CONFLICT', detail: 'reason=copy_failed\nsourceId=AB12' }), submitCopyErrorText({ code: 'TASK_CONFLICT', detail: 'reason=duplicate_url' }), submitCopyErrorText({ code: 'NOT_FOUND', detail: 'reason=copying' }), submitCopyErrorText({ code: 'TASK_CONFLICT' })],
      ['文件还在准备中，准备好后再点转换。', '文件没能准备好，请在列表里点“重试”后再转换。', '', '', ''])
    eq('文案定稿', [SUBMIT_COPYING_TEXT, SUBMIT_COPY_FAILED_TEXT], ['文件还在准备中，准备好后再点转换。', '文件没能准备好，请在列表里点“重试”后再转换。'])
    const b = readSrc('src/api/convertRecordsBinding.ts')
    eq('binding：SubmitSources 返回 {tasks, skipped}（submitResultOf），有生成类型对照', [/submitResultOf<V023Task>\(await call\(CS\.SubmitSources/.test(b), /\?\.tasks\)\)/.test(b), /SameKeys<ConvertSubmitResult, Data<convert\.ConvertSubmitResult>>/.test(b), /SameKeys<SkippedSource, Data<convert\.SkippedSource>>/.test(b)], [true, false, true, true])
    const sys = readSrc('src/api/system.ts')
    eq('输出位置显示：v0.24.1 留空 = 应用的输出文件夹，不再说源文件所在文件夹', [/GetStorageDirs/.test(sys), /各自源文件所在的文件夹/.test(readSrc('src/components/convert/ConvertSettingsPanel.vue')), /与源文件相同的文件夹/.test(readSrc('src/components/settings/OutputDirRow.vue') + readSrc('src/errors/errorMessages.ts'))], [true, false, false])

    // 模拟后端 + store：部分跳过 → 提交其余、toast、跳过的保持勾选；全部没就绪 → 短提示、不算失败、勾选不变
    mock.resetConvertMock('added')
    setActivePinia(createPinia())
    const cv = useConvertRecordsStore()
    await cv.reload()
    await cv.loadPresets()
    setPresetCatalog([]) // loadPresets 会写入预设目录，别影响后面的标题自检
    useFFmpegStore().status = { state: 'ready' }
    const ids = Object.keys(cv.sources).filter((id) => cv.sources[id].exists !== false).slice(0, 3)
    const toastText = (): string | undefined => (cv.toast as { text: string } | null)?.text
    if (ids.length >= 2 && cv.selectedPreset) {
      for (const id of ids) {
        mock.mockSetCopyState(id, 'ready')
        cv.sources[id].probe = 'ok' // 不走探测，直接当作读过
      }
      mock.mockSetCopyState(ids[0], 'copying')
      cv.clearSelection()
      for (const id of ids) cv.toggle(id)
      const before = cv.submittableRows.length
      await cv.submit()
      eq('部分跳过：提交其余、toast 用定稿文案、跳过的那行还勾着、没有错误', [before, toastText(), [...cv.selected], cv.submitError],
        [ids.length, '有 1 个文件还在准备中，已转换其余文件。准备好后再点转换。', [ids[0]], null])
      cv.toast = null
      await cv.submit()
      eq('全部还在准备：短提示（不是“无法开始转换”），勾选不变', [toastText(), cv.submitError, [...cv.selected]], ['文件还在准备中，准备好后再点转换。', null, [ids[0]]])
      mock.mockSetCopyState(ids[0], 'failed')
      await cv.submit()
      eq('全部复制失败：短提示', toastText(), '文件没能准备好，请在列表里点“重试”后再转换。')
      mock.mockSetCopyState(ids[0], 'ready')
    } else eq('部分跳过自检：模拟场景至少要有 2 个能转换的文件', [ids.length >= 2, !!cv.selectedPreset], [true, true])
  }
  // ---- 包 20 封面：类型封面兜底（产品经理 + 设计 10-08）----
  {
    const warn = console.warn
    const warned: unknown[][] = []
    console.warn = (...a: unknown[]) => void warned.push(a)
    try {
      eq('缩略图：NOT_FOUND file → missing；record → gone；UNSUPPORTED format → 类型封面（不算失败、不打日志）', [thumbStateOf({ code: 'NOT_FOUND', detail: 'reason=file' }), thumbStateOf({ code: 'NOT_FOUND', detail: 'reason=record' }), thumbStateOf({ code: 'UNSUPPORTED', detail: 'reason=format' }), warned.length], [{ kind: 'missing' }, { kind: 'gone' }, { kind: 'type' }, 0])
      const t = thumbStateOf({ code: 'INTERNAL', message: '生成缩略图失败', detail: 'reason=thumbnail' }, '源文件缩略图 sourceId=s1')
      eq('缩略图：其它错误 → 类型封面 failed=true，console.warn 带错误码（不静默吞掉）', [t, warned.length, warned[0]?.includes('INTERNAL'), String(warned[0]?.[0]).includes('sourceId=s1')], [{ kind: 'type', failed: true }, 1, true, true])
      eq('缩略图地址：data:image / http(s) / blob 可用；空串、文件路径不可用', [isThumbUrl('data:image/jpeg;base64,xx'), isThumbUrl('blob:x'), isThumbUrl(''), isThumbUrl('C:\\a.jpg'), isThumbUrl(undefined)], [true, true, false, false, false])
    } finally {
      console.warn = warn
    }
    eq('封面类型：PNG / JPG = 图片，音频容器 = 音符，GIF 和其余 = 视频（设计场景 35）', ['png', 'JPG', 'gif', 'flac', 'm4a', 'mp4', 'mov', ''].map((e) => coverKindOf(e)), ['image', 'image', 'video', 'audio', 'audio', 'video', 'video', 'video'])
    eq('封面类型：有探测结果按有没有画面（MP4 纯音频 → 音符）；图片格式优先', [coverKindOf('mp4', { width: 0, hasVideo: false }), coverKindOf('mp4', { width: 1920, hasVideo: true }), coverKindOf('png', { width: 320, hasVideo: true })], ['audio', 'video', 'image'])
    eq('扩展名：路径 / 大小写 / 没有扩展名', [extOf('D:\\a\\SOURCE.MP4'), extOf('/x/y.tar.gz'), extOf('README'), extOf('.env')], ['mp4', 'gz', '', ''])
    const thumbSrc = readSrc('src/components/convert/ConvertThumb.vue')
    eq('ConvertThumb：类型封面用胶片 / 图片 / 音符（cv-cov c-v / c-i / c-a），生成中加扫光，不再用灰色文件图标（doc）', [/video: 'film', image: 'image', audio: 'music'/.test(thumbSrc), /video: 'c-v', image: 'c-i', audio: 'c-a'/.test(thumbSrc), /gen/.test(thumbSrc), /name="doc"|'doc'/.test(thumbSrc)], [true, true, true, false])
    const kidSrc = readSrc('src/components/convert/ConvertKid.vue')
    const rowSrc = readSrc('src/components/convert/ConvertSourceRow.vue')
    eq('子记录 / 父行：格式角标进封面，名字旁不再有格式标签；大小用 cv-sz 不截断', [/class="cv-fmt"/.test(kidSrc), /:fmt="fmt"/.test(kidSrc), /:fmt="fmt"/.test(rowSrc), /class="cv-sz"/.test(kidSrc), /class="cv-sz"/.test(rowSrc)], [false, true, true, true, true])
    const css = readSrc('src/components/convert/convert-v2.css')
    const last = (re: RegExp) => [...css.matchAll(re)].pop()?.[0] ?? ''
    eq('样式（设计包 20 段）：子记录无竖线、无卡片边框，封面 64×36 / 56×32，1024 不再缩小父行封面', [/border-left:2px/.test(last(/(^|\n)\.cv-kids\{[^}]*\}/g)), /border:0/.test(last(/(^|\n)\.cv-kid\{[^}]*\}/g)), /\.cv-th,\.cv2\.w1024 \.cv-prow>\.cv-th\{width:64px;height:36px;border-radius:6px\}/.test(css), /\.cv-th\.sm\{width:56px;height:32px;border-radius:5px\}/.test(css), /\.cv-sz\{flex:none/.test(css)], [false, true, true, true, true])
    // 缩略图取数流程（包 20 验收）：请求中 = 生成中（null）→ 返回后自动换成画面；失败 → 类型封面，不永久缓存，之后再挂载成功就换成画面
    {
      const win = (globalThis as unknown as { window: { location: { search: string } } }).window
      const prev = win.location.search
      const realNow = Date.now
      const warn = console.warn
      console.warn = () => undefined
      const flush = async () => { for (let i = 0; i < 8; i++) await new Promise((r) => setTimeout(r, 0)) }
      try {
        setActivePinia(createPinia())
        const cv = useConvertRecordsStore()
        mock.resetConvertMock('mixed')
        const row = (id: string, name: string) => {
          cv.sources[id] = { sourceId: id, path: `D:\\Footage\\${name}`, name, addedAt: 0, lastActivityAt: 0, probe: 'pending', thumb: null, thumbAsked: false, flashAt: 0, recordCount: 0, loadedIds: [], loadingMore: false, copySeq: 0 }
          return cv.sources[id]
        }
        win.location.search = '?cv_thumb=hold'
        const s1 = row('mock-src-launch', 'launch-4k.mov')
        cv.ensureThumb(s1)
        await flush()
        const pending = s1.thumb
        cv.ensureThumb(s1) // 请求中再调用不重复发
        mock.releaseMockThumbs()
        await flush()
        eq('源文件缩略图：请求中 thumb = null（生成中封面）→ 返回后自动换成画面', [pending, s1.thumb?.kind, (s1.thumb as { url?: string })?.url?.startsWith('data:image/')], [null, 'img', true])
        win.location.search = '?cv_thumb=fail'
        const s2 = row('mock-src-iv', '采访-机位A.mkv')
        cv.ensureThumb(s2)
        await flush()
        const failed = s2.thumb
        win.location.search = ''
        cv.ensureThumb(s2) // 刚失败：5 秒内不重取
        await flush()
        const soon = s2.thumb
        Date.now = () => realNow() + 6000
        cv.ensureThumb(s2) // 再次挂载：重取成功
        await flush()
        eq('源文件缩略图：失败 → 类型封面（failed）；不永久缓存，之后再挂载成功换成画面', [failed, soon, s2.thumb?.kind], [{ kind: 'type', failed: true }, { kind: 'type', failed: true }, 'img'])
        Date.now = realNow
        // 子记录：完成后才取；失败后下次重取成功
        const kid = { id: 'simcv-launchMp4', status: 'succeeded', version: 1, outputPath: 'D:\\Footage\\launch-4k.mp4', outputGone: false, options: { container: 'mp4' } } as unknown as Parameters<typeof cv.ensureRecThumb>[0]
        cv.ensureRecThumb({ ...kid, status: 'running' } as typeof kid)
        const notYet = cv.recThumbs.has(kid.id)
        win.location.search = '?cv_thumb=fail'
        cv.ensureRecThumb(kid)
        await flush()
        const kFail = cv.recThumbs.get(kid.id)
        win.location.search = ''
        Date.now = () => realNow() + 6000
        cv.ensureRecThumb(kid)
        const kPending = cv.recThumbs.get(kid.id)
        await flush()
        eq('记录缩略图：未完成不取；完成后取，失败 → 类型封面；重取时显示生成中，成功换成画面', [notYet, kFail, kPending, cv.recThumbs.get(kid.id)?.kind], [false, { kind: 'type', failed: true }, undefined, 'img'])
      } finally {
        Date.now = realNow
        console.warn = warn
        win.location.search = prev
      }
    }
    // 复验 N1 / N2
    eq('无法播放说明：H.265 建议 MP4 · H.264，其余照旧', [isHevcCodec('hevc'), isHevcCodec('libx265'), isHevcCodec('h265'), isHevcCodec('hevc_nvenc'), isHevcCodec('h264'), unplayableHint(true).includes('或转成 MP4 · H.264 后再预览。'), unplayableHint(false).includes('或转成 MP4 后再预览。')], [true, true, true, true, false, true, true])
    const pv = readSrc('src/components/convert/ConvertPreviewDialog.vue')
    eq('无法播放面板按钮叫“打开所在文件夹”，不再有“在文件夹中显示”', [REVEAL_LABEL, pv.includes('在文件夹中显示'), pv.includes('{{ REVEAL_LABEL }}')], ['打开所在文件夹', false, true])
  }
  // ---- 第 2 行（设计 §7.3 第 14 条）：一个格式化函数，三处共用 ----
  const P = { presetId: 'builtin-mp4-h264-1080p', presetName: 'MP4 1080p（H.264 + AAC）', paramsSummary: 'H.264 · 1080p', title: 'a.mov → MP4' }
  eq('第 2 行：presetId 非空 → 预设名（卡片标题），悬停 = paramsSummary', recordParamsText(P), { text: 'MP4 1080p', tip: 'H.264 · 1080p', preset: true })
  eq('第 2 行：presetId / presetName 为空、有摘要 → 自定义 · 摘要', recordParamsText({ presetId: '', presetName: '', paramsSummary: 'H.264 · 1080p · 8.0 Mbps', title: 't' }).text, '自定义 · H.264 · 1080p · 8.0 Mbps')
  eq('第 2 行：三个快照都为空（旧任务）→ title', [recordParamsText({ title: 'a.mov → MP4' }).text, recordParamsText({ presetId: '', presetName: '', paramsSummary: '', title: 'b' }).text], ['a.mov → MP4', 'b'])
  eq('recordLine：时间 · 预设名，title = paramsSummary', recordLine(P, ['今天 11:48']), { text: '今天 11:48 · MP4 1080p', title: 'H.264 · 1080p' })
  eq('recordLine：自定义 → title = 整行', recordLine({ paramsSummary: 'H.264 · 1080p', title: 't' }, ['今天 10:05']), { text: '今天 10:05 · 自定义 · H.264 · 1080p', title: '今天 10:05 · 自定义 · H.264 · 1080p' })
  eq('recordLine：任务中心不带时间（head 为空）', recordLine({ title: 'a.mov → MP4' }, []).text, 'a.mov → MP4')
  eq('recordLine：预览底栏带“… 转换”前缀和设备后缀', recordLine(P, ['今天 11:48 转换'], ['显卡']).text, '今天 11:48 转换 · MP4 1080p · 显卡')
  // ---- 参数摘要（v0.23.1：不带容器名；只给宽的写 p 值）----
  const S = mock.mockParamsSummary
  eq('摘要：只给宽 3840/2560/1920/1280/854 → 2160p/1440p/1080p/720p/480p', [3840, 2560, 1920, 1280, 854].map((w) => S({ container: 'mp4', videoCodec: 'h264', width: w })), ['H.264 · 2160p', 'H.264 · 1440p', 'H.264 · 1080p', 'H.264 · 720p', 'H.264 · 480p'])
  eq('摘要：其它宽度“宽 N”；宽高都有“宽×高”；只给高“高p”', [S({ container: 'mp4', videoCodec: 'h264', width: 1000 }), S({ container: 'mp4', videoCodec: 'h264', width: 1920, height: 800 }), S({ container: 'mp4', videoCodec: 'h264', height: 1080, videoBitrate: 8_000_000 })], ['H.264 · 宽 1000', 'H.264 · 1920×800', 'H.264 · 1080p · 8.0 Mbps'])
  eq('摘要：不含容器名；音频写码率', [S({ container: 'mp3', audioCodec: 'mp3', audioBitrate: 192000 }), /MP4|WEBM|MP3/.test(S({ container: 'webm', videoCodec: 'vp9' }))], ['192 kbps', false])
  // ---- 删除结果 / 冲突 ----
  {
    // 删除 / 移除 toast（产品经理 10-08 定稿）：主句 + still_running 句 + 文件句
    const F = (path: string) => ({ reason: path ? 'in_use' : 'still_running', path })
    const ids = (n: number) => Array.from({ length: n }, (_, i) => `t${i}`)
    const R = (n: number, srcGone: boolean, m: number, ...fs: ReturnType<typeof F>[]) => ({ deletedTaskIds: ids(n), deletedSourceIds: srcGone ? ['s1'] : [], deletedFiles: m, failures: fs })
    const SRC = { kind: 'source' as const, id: 's1', name: 'launch-4k.mov' }
    const REC = { kind: 'record' as const, id: 't9', name: 'a.mp4' }
    const T = (t: ReturnType<typeof deleteToast>) => [toastText(t.parts), t.warn, t.path]
    const FILE = '有 1 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。'
    // 源文件行：移除成功
    eq('移除 toast：有记录 / 删了输出 / 没有记录（不出现“0 条记录”），普通样式', [T(deleteToast(SRC, R(3, true, 0))), T(deleteToast(SRC, R(3, true, 1))), T(deleteToast({ ...SRC, name: '新文件.mov' }, R(0, true, 0)))], [['已从列表移除“launch-4k.mov”和 3 条记录。', false, ''], ['已从列表移除“launch-4k.mov”和 3 条记录，并删除了 1 个文件。', false, ''], ['已从列表移除“新文件.mov”。', false, '']])
    eq('移除 toast：文件名单独一段（页面放进可省略、带 title 的 span）', [sourceRemovedParts('a.mov', 2, 0), deleteToast(SRC, R(1, false, 0, F(''))).parts.filter((x) => typeof x !== 'string')], [['已从列表移除“', { name: 'a.mov' }, '”和 2 条记录。'], [{ name: 'launch-4k.mov' }]])
    eq('移除成功 + 文件没删成：文件句在后，警告，打开第一个非空路径', T(deleteToast(SRC, R(3, true, 1, F('D:\\a.mp4'), F('D:\\b.mp4')))), ['已从列表移除“launch-4k.mov”和 3 条记录，并删除了 1 个文件。有 2 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。', true, 'D:\\a.mp4'])
    // 源文件行：因 still_running 保留
    eq('行保留 n>0：已删除 n 条记录。+ 保留句；不以“已从列表移除”开头；没有文件夹操作', T(deleteToast(SRC, R(2, false, 1, F('')))), ['已删除 2 条记录。有 1 条转换没能及时停止，“launch-4k.mov”仍保留在列表里，请稍后再移除。', true, ''])
    eq('行保留 n=0：去掉第一句', T(deleteToast(SRC, R(0, false, 0, F(''), F('')))), ['有 2 条转换没能及时停止，“launch-4k.mov”仍保留在列表里，请稍后再移除。', true, ''])
    eq('行保留 + 文件没删成：still_running 句在前、文件句在后，各自计数；有非空 path 才有文件夹操作', T(deleteToast(SRC, R(1, false, 0, F('D:\\c.mp4'), F('')))), ['已删除 1 条记录。有 1 条转换没能及时停止，“launch-4k.mov”仍保留在列表里，请稍后再移除。' + FILE, true, 'D:\\c.mp4'])
    // 删除记录（单条 / 批量）
    eq('删除记录：没有失败 → 已删除 n 条记录。普通样式', T(deleteToast(REC, R(1, false, 0))), ['已删除 1 条记录。', false, ''])
    eq('删除记录 still_running n=0 / n>0', [T(deleteToast(REC, R(0, false, 0, F('')))), T(deleteToast(REC, R(2, false, 0, F(''))))], [['有 1 条转换没能及时停止，请稍后再删除。', true, ''], ['已删除 2 条记录。有 1 条转换没能及时停止，请稍后再删除。', true, '']])
    // 按 reason 分类（复核 N2 / N3；产品经理 10-08 确认）：被占用 → 没有权限 → 其他，每类一句，都在主句和 still_running 句之后
    const X = (reason: string, path = 'D:\\x.mp4') => ({ reason, path })
    eq('文件失败按 reason：permission / not_task_output / io 各自的句子', [T(deleteToast(REC, R(1, false, 0, X('permission')))), T(deleteToast(REC, R(1, false, 0, X('not_task_output'), X('io'))))], [['已删除 1 条记录。有 1 个文件没有权限删除，请手动删除。', true, 'D:\\x.mp4'], ['已删除 1 条记录。有 2 个文件没能删除，请手动删除。', true, 'D:\\x.mp4']])
    eq('混合顺序：主句 → still_running → 被占用 → 没有权限 → 其他，各自计数（DeleteRecords）', toastText(deleteToast(REC, R(2, false, 0, X('io', 'D:\\io.mp4'), X('permission'), F(''), X('in_use'), X('not_task_output'), X('in_use'))).parts), '已删除 2 条记录。有 1 条转换没能及时停止，请稍后再删除。有 2 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。有 1 个文件没有权限删除，请手动删除。有 2 个文件没能删除，请手动删除。')
    eq('混合顺序（DeleteSource 行保留）：同样的顺序；文件夹取第一条非空 path（不管原因）', T(deleteToast(SRC, R(1, false, 0, F(''), X('permission', 'D:\\p.mp4'), X('in_use', 'D:\\u.mp4')))), ['已删除 1 条记录。有 1 条转换没能及时停止，“launch-4k.mov”仍保留在列表里，请稍后再移除。有 1 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。有 1 个文件没有权限删除，请手动删除。', true, 'D:\\p.mp4'])
    eq('按 reason 分类而不是按 path：path 为空的文件类失败不算“没能及时停止”；未知 reason 算其他', T(deleteToast(REC, R(1, false, 0, X('io', ''), X('weird')))), ['已删除 1 条记录。有 2 个文件没能删除，请手动删除。', true, 'D:\\x.mp4'])
    eq('删除记录：文件没删成 / 混合（still_running 句在前，文件句在后）', [T(deleteToast(REC, R(1, false, 0, F('D:\\a.mp4')))), T(deleteToast(REC, R(1, false, 0, F('D:\\a.mp4'), F(''))))], [['已删除 1 条记录。' + FILE, true, 'D:\\a.mp4'], ['已删除 1 条记录。有 1 条转换没能及时停止，请稍后再删除。' + FILE, true, 'D:\\a.mp4']])
    const pg = readSrc('src/views/ConvertPage.vue')
    const css = readSrc('src/components/convert/convert-v2.css')
    eq('toast 排版（复核 D2）：普通行内排版、行高 1.5、按钮不缩进；不再用 inline-flex', [/\.cv-toast-box \.cv-toast\{display:block;[^}]*line-height:1\.5/.test(css), /\.cv-toast-box \.el-message__content\{line-height:1\.5\}/.test(css), /\.cv-toast-act\{display:block;margin:2px 0 0;/.test(css), /cv-toast\{display:inline-flex/.test(css)], [true, true, true, false])
    eq('页面：toast 全走 deleteToast；警告 8 秒；有路径才给“打开所在文件夹”；整段普通文字（名称先中间省略，悬停看全文）', [/showDeleteToast\(deleteToast\(a, r\)\)/.test(pg), /type: t\.warn \? 'warning' : 'success', duration: t\.warn \? WARN_TOAST_MS : TOAST_MS/.test(pg), /path \? h\('button'/.test(pg), /h\('span', \{ class: 'cv-toast-tx', title: short === full \? undefined : full \}, short\)/.test(pg) && /midEllipsisPx\(p\.name, TOAST_NAME_PX, font\)/.test(pg), /已从列表移除|没能及时停止/.test(pg)], [true, true, true, true, false])
  }
  {
    // 中间省略（复核 N1）：保留扩展名和它前面 1 个字；任务名保留“.mov → MP4”；放得下不动；检查里按字数量宽
    const L = (s: string) => Array.from(s).length
    eq('midEllipsis：文件名 / 任务名 / 放得下 / 没有扩展名 / 太窄退回末尾省略', [midEllipsis('launch-4k.mp4', 10, L), midEllipsis('launch-4k.mov → MP4', 14, L), midEllipsis('a.mp4', 10, L), midEllipsis('abcdefghijklmnop', 8, L), midEllipsis('launch-4k.mp4', 4, L)], ['laun…k.mp4', 'la…k.mov → MP4', 'a.mp4', 'abc…mnop', 'lau…'])
    eq('midEllipsis：结果不超过可用宽度，中文按字切', [L(midEllipsis('采访-机位A-第二天上午.mp4', 12, L)) <= 12, midEllipsis('采访-机位A-第二天上午.mp4', 12, L).endsWith('午.mp4'), midTailMin('x.webm'), midTailMin('noext')], [true, true, 6, 4])
    const used = ['src/views/TaskCenter.vue', 'src/components/convert/ConvertSourceRow.vue', 'src/components/convert/ConvertKid.vue', 'src/components/convert/ConvertDeleteDialog.vue', 'src/components/convert/ConvertPreviewDialog.vue', 'src/views/ConvertPage.vue'].map((f) => /MidEllipsis|midEllipsisPx/.test(readSrc(f)))
    eq('MidEllipsis 用在任务中心任务名、源文件名、记录名、删除弹窗、预览标题、移除提示', used, [true, true, true, true, true, true])
    eq('1024“更多”按钮读屏：更多：打开所在文件夹、从列表移除；菜单项 aria-label 从列表移除', [/aria-label="`更多：打开所在文件夹、\$\{SOURCE_REMOVE_LABEL\}`"/.test(readSrc('src/components/convert/ConvertSourceRow.vue')), /role="menuitem" :aria-label="SOURCE_REMOVE_LABEL"/.test(readSrc('src/components/convert/ConvertSourceRow.vue'))], [true, true])
  }
  eq('编码显示名：H.265 / ProRes，不出现 HEVC / PRORES；表里有的按表', ['hevc', 'h265', 'prores', 'h264', 'av1', 'pcm_s16le', 'cinepak', 'copy'].map((c) => codecName(c)), ['H.265', 'H.265', 'ProRes', 'H.264', 'AV1', 'PCM', 'Cinepak', '原编码'])
  eq('走查 X3：FFV1 / DNxHD / MJPEG 按表；表里没有的原样大写，不做首字母大写（不出现“Ffv1”）', [codecName('ffv1'), codecName('dnxhd'), codecName('mjpeg'), codecName('foo2'), mock.mockParamsSummary({ container: 'mkv', videoCodec: 'ffv1' })], ['FFV1', 'DNxHD', 'MJPEG', 'FOO2', 'FFV1'])
  eq('走查 X3：前端只有 mediaText 一张编码名表，没有别处自己首字母大写', [/charAt\(0\)\.toUpperCase\(\)/.test(readSrc('src/utils/mediaText.ts')), /charAt\(0\)\.toUpperCase\(\)/.test(readSrc('src/api/convertRecordsMock.ts')), /codecName\(base\)/.test(readSrc('src/api/convertRecordsMock.ts'))], [false, false, true])
  {
    // 契约 v0.23.4 / PR #92：MediaInfo.videoCodecName / audioCodecName 优先，原样显示；缺了才退回前端的表
    const V = (o: Partial<goStore.MediaInfo>) => goStore.MediaInfo.createFrom({ id: '', path: '', name: '', size: 0, duration: 60, width: 1920, height: 1080, videoCodec: '', audioCodec: '', bitrate: 0, thumbUrl: '', hasVideo: true, hasAudio: true, probedAt: 0, ...o })
    eq('编码显示名：后端字段优先，不改大小写；缺了退回 codecName', [videoCodecText({ videoCodec: 'ffv1', videoCodecName: 'FFV1' }), videoCodecText({ videoCodec: 'foo', videoCodecName: 'Foo' }), videoCodecText({ videoCodec: 'ffv1' }), audioCodecText({ audioCodec: 'pcm_s24le', audioCodecName: 'PCM' }), audioCodecText({ audioCodec: 'ac3' }), videoCodecText({})], ['FFV1', 'Foo', 'FFV1', 'PCM', 'AC-3', ''])
    eq('父行 / 文件行 / 预览底栏都用后端显示名', [sourceMetaText(V({ videoCodec: 'dnxhd', videoCodecName: 'DNxHD后端' })).includes('DNxHD后端'), rowInfoText(V({ videoCodec: 'dnxhd', videoCodecName: 'DNxHD后端' })).includes('DNxHD后端'), /videoCodecText\(i\)/.test(readSrc('src/components/convert/ConvertPreviewDialog.vue')), /codecName\(i\.videoCodec\)|codecName\(i\.audioCodec\)/.test(readSrc('src/components/convert/ConvertPreviewDialog.vue') + readSrc('src/utils/convertText.ts') + readSrc('src/utils/mediaText.ts'))], [true, true, true, false])
    // G3：media 整份入库后保留 hasVideo / hasAudio
    const pod = V({ width: 0, height: 0, audioCodec: 'pcm_s16le', audioCodecName: 'PCM', hasVideo: false, sampleRate: 48000, channels: 2, size: 0 })
    const mute = V({ videoCodec: 'h264', videoCodecName: 'H.264', hasAudio: false, size: 0 })
    eq('G3：只有 media（重启后）也保留 hasVideo / hasAudio：音频显示采样率 · 声道，无声视频显示“没有声音”', [sourceMetaText(metaInfoOf({ media: pod })!), sourceMetaText(metaInfoOf({ media: mute })!), metaInfoOf({ media: mute })?.hasAudio], ['48 kHz · 立体声 · 01:00', '1920×1080 · H.264 · 没有声音 · 01:00', false])
    const legacy = metaInfoOf({ media: V({ videoCodec: 'h264', videoCodecName: 'H.264', hasVideo: false, hasAudio: false, size: 0 }) })!
    eq('G3 之前的旧缓存（两个都是 false）按不知道处理：按宽高当视频，不显示“没有声音”', [legacy.hasVideo, legacy.hasAudio, sourceMetaText(legacy)], [undefined, undefined, '1920×1080 · H.264 · 01:00'])
    eq('当次探测（info）优先于 media', metaInfoOf({ info: mute, media: pod })?.hasAudio, false)
  }
  {
    // 走查 G2：第 2 行 / 任务中心 / 预览底栏和预设卡片共用 presetShortTitle：同名预设补编码
    const names = ['MP4（H.264 + AAC，通用）', 'MP4 1080p（H.264 + AAC）', 'MP4（H.265 + AAC，体积更小）', 'WebM（VP9 + Opus）']
    const dup = dupPresetTitles(names)
    eq('dupPresetTitles：只有 MP4 重名', [...dup], ['MP4'])
    eq('presetShortTitle：MP4 · H.264 / MP4 · H.265；不重名的不补（MP4 1080p、WebM）', [presetShortTitle(names[0], 'h264', dup), presetShortTitle(names[2], 'hevc', dup), presetShortTitle(names[1], 'h264', dup), presetShortTitle(names[3], 'vp9', dup)], ['MP4 · H.264', 'MP4 · H.265', 'MP4 1080p', 'WebM'])
    const R = (presetName: string, videoCodec: string) => ({ presetId: 'x', presetName, paramsSummary: 's', title: 't', options: { videoCodec } })
    eq('recordParamsText：H.264 / H.265 记录不再都写“MP4”', [recordParamsText(R(names[0], 'h264'), dup).text, recordParamsText(R(names[2], 'h265'), dup).text, recordLine(R(names[2], 'h265'), ['今天 14:48']).text], ['MP4 · H.264', 'MP4 · H.265', '今天 14:48 · MP4'])
    setPresetCatalog(names)
    eq('setPresetCatalog 之后默认用当前预设列表（第 2 行 / 任务中心 / 预览底栏）', [recordLine(R(names[2], 'h265'), ['今天 14:48']).text, recordLine(R(names[0], 'h264'), ['今天 14:48 转换'], ['CPU']).text], ['今天 14:48 · MP4 · H.265', '今天 14:48 转换 · MP4 · H.264 · CPU'])
    setPresetCatalog([])
    const st = readSrc('src/stores/convertRecords.ts')
    eq('G2：卡片标题走 presetShortTitle，取到预设后写入 setPresetCatalog；任务中心进入时也写入', [/presetShortTitle\(p\.name, p\.options\.videoCodec \|\| p\.options\.audioCodec, dup\)/.test(st), /setPresetCatalog\(presets\.value\.map/.test(st), /listPresets\(\)\.then\(\(ps\) => setPresetCatalog/.test(readSrc('src/views/TaskCenter.vue'))], [true, true, true])
  }
  eq('PM 10-08 文案：缺推流协议（带 / 不带协议名）、转换组件不完整', [liveFfmpegProtocolMissingText('missing=srt'), LIVE_FFMPEG_PROTOCOL_MISSING_TEXT, FFPROBE_MISSING_TEXT, /FFPROBE_MISSING_TEXT/.test(readSrc('src/components/settings/FFmpegPanel.vue')), /缺少读取文件信息的部分/.test(readSrc('src/components/settings/FFmpegPanel.vue'))], ['当前转换组件不支持 SRT。请到设置的“转换组件”里重新安装或更新。', '当前转换组件不支持这种推流协议。请到设置的“转换组件”里重新安装或更新。', '转换组件不完整，无法读取文件信息。请到设置的“转换组件”里重新安装。', true, false])
  eq('冲突：无画面配视频 / 无声配音频 / 读取中不判断 / 兼容', [conflictReason({ hasAudio: true }, true, 'mp4'), conflictReason({ hasVideo: true }, true, 'mp3'), conflictReason({}, false, 'mp4'), conflictReason({ hasVideo: true, hasAudio: true }, true, 'mp4')], [CONFLICT_NO_VIDEO, CONFLICT_NO_AUDIO, null, null])
  eq('冲突文案定稿（不显示错误码）', [CONFLICT_TITLE, CONFLICT_NO_VIDEO, CONFLICT_NO_AUDIO], ['这个文件不能用当前预设', '没有画面，不能转成视频格式。请换一个音频预设，或取消勾选。', '没有声音，不能转成音频格式。请换一个视频预设，或取消勾选。'])
  {
    const now = new Date(2026, 9, 8, 13, 0).getTime()
    eq('记录时间：今天 / 昨天', [formatRecordTime(new Date(2026, 9, 8, 11, 48).getTime(), now), formatRecordTime(new Date(2026, 9, 7, 21, 14).getTime(), now)], ['今天 11:48', '昨天 21:14'])
  }
  // ---- 模拟层：GetSource / RevealRecord（v0.23.1）----
  mock.resetConvertMock('missing')
  const e = await mock.GetSource('mock-src-wed')
  eq('GetSource：返回一个 ConvertSourceEntry（同 ListSources 的一项）', [e.source.sourceId, e.recordCount, e.records.length, e.records.map((r) => r.id)], ['mock-src-wed', 2, 2, ['simcv-wedGif', 'simcv-wedMp4']])
  eq('GetSource 不存在：NOT_FOUND reason=record', [(await rejects(mock.GetSource('mock-src-nope')))?.code, (await rejects(mock.GetSource('mock-src-nope')))?.reason], ['NOT_FOUND', 'record'])
  eq('RevealRecord：输出已不存在 → NOT_FOUND reason=file；记录不存在 → reason=record；正常 → 成功', [(await rejects(mock.RevealRecord('simcv-wedGif')))?.reason, (await rejects(mock.RevealRecord('simcv-nope')))?.reason, await rejects(mock.RevealRecord('simcv-wedMp4'))], ['file', 'record', null])
  mock.resetConvertMock('dup')
  {
    const recs = (await mock.GetSource('mock-src-iv')).records.map(recordOf)
    const [ivNew, ivOld] = recs
    eq('模拟记录：预设记录 presetId 非空，摘要按宽写 720p；自定义记录 presetId 为空', [ivNew.presetId, ivNew.paramsSummary, ivOld.presetId, ivOld.presetName, ivOld.paramsSummary], ['builtin-mp4-h264-720p', 'H.264 · 720p', '', '', 'H.264 · 1080p · 8.0 Mbps'])
    eq('模拟记录第 2 行：预设名 / 自定义 · 摘要', [recordParamsText(ivNew).text, recordParamsText(ivOld).text], ['MP4 720p', '自定义 · H.264 · 1080p · 8.0 Mbps'])
    eq('parseParams：旧任务 params 为空 → 三个快照 undefined', [parseParams('').presetId, parseParams('{"options":{"container":"mp4"}}').paramsSummary], [undefined, undefined])
    eq('Reconvert：记录不存在 NOT_FOUND', (await rejects(mock.Reconvert({ taskId: 'simcv-nope' })))?.code, 'NOT_FOUND')
  }
  // ---- ListSources status（v0.23.1）：EXISTS 语义，内嵌记录 / recordCount 不过滤，分页排序不变 ----
  {
    const ids = async (status: '' | 'active' | 'failed', limit = 50, offset = 0) => {
      const p = await mock.ListSources({ limit, offset, recordLimit: 20, status })
      return { ids: p.items.map((e) => e.source.sourceId.replace('mock-src-', '')), total: p.total, items: p.items }
    }
    mock.resetConvertMock('mixed')
    const all = await ids('')
    const act = await ids('active')
    const fail = await ids('failed')
    eq('status 筛选（mixed）：全部 5 行；进行中 / 失败都只有 launch', [all.ids, act.ids, fail.ids, act.total, fail.total], [['launch', 'iv', 'rec', 'pod', 'wed'], ['launch'], ['launch'], 1, 1])
    eq('status 筛选：行内嵌记录和 recordCount 不按状态过滤（含已完成那条）', [fail.items[0].recordCount, fail.items[0].records.map((r) => r.status)], [3, ['failed', 'running', 'succeeded']])
    eq('status 缺省 = 全部', (await mock.ListSources({ limit: 50, offset: 0, recordLimit: 20 })).total, all.total)
    mock.resetConvertMock('canceled')
    eq('status 筛选（canceled）：已取消不算失败，也不算进行中', [(await ids('failed')).ids, (await ids('active')).ids], [[], []])
    mock.resetConvertMock('running')
    const r1 = await ids('active')
    const r2 = await ids('active', 1, 1)
    eq('status 筛选（running）：排队中 / 进行中都算；排序按最近活动；分页不变', [r1.ids, r1.total, r2.ids, r2.total], [['rec', 'launch'], 2, ['launch'], 2])
    eq('status 筛选（running）：没有失败行', (await ids('failed')).ids, [])
    const bad = await rejects(mock.ListSources({ limit: 50, offset: 0, recordLimit: 20, status: 'canceled' as never }))
    eq('status 其它值 → INVALID_ARGUMENT', bad?.code, 'INVALID_ARGUMENT')
    eq('status 失败 = failed / interrupted（模拟层定义）', /failed: \['failed', 'interrupted'\]/.test(readSrc('src/api/convertRecordsMock.ts')), true)
    eq('绑定层：ListSources / SearchSources 都总是带 status（缺省 \'\'）', [/CS\.ListSources\(convert\.ConvertSourceFilter\.createFrom\(\{ \.\.\.f, status: f\.status \?\? '' \}\)\)/.test(readSrc('src/api/convertRecordsBinding.ts')), /CS\.SearchSources\(convert\.ConvertSearchFilter\.createFrom\(\{ \.\.\.f, status: f\.status \?\? '' \}\)\)/.test(readSrc('src/api/convertRecordsBinding.ts')), /interface ConvertSearchFilter extends ConvertSourceFilter \{/.test(readSrc('src/api/convertRecords.ts'))], [true, true, true])
    {
      const b = readSrc('src/api/convertRecordsBinding.ts')
      eq('联调：CONVERT_V2_BACKEND_READY = true；binding 直接用生成的 ConvertService / TaskService，不再按名字 callService；有生成类型对照', [/CONVERT_V2_BACKEND_READY: boolean = true/.test(readSrc('src/api/flags.ts')), /from '\.\.\/\.\.\/wailsjs\/go\/app\/ConvertService'/.test(b), /from '\.\.\/\.\.\/wailsjs\/go\/app\/TaskService'/.test(b), /callService|V023_METHODS/.test(b), /BINDING_SHAPES_OK/.test(b)], [true, true, true, false, true])
      eq('纯浏览器仍走模拟（convertV2IsReal 要求 window.go）', /convertV2IsReal = \(\): boolean => CONVERT_V2_BACKEND_READY && hasWailsBackend\(\)/.test(readSrc('src/api/convertRecords.ts')), true)
    }
    const st = readSrc('src/stores/convertRecords.ts')
    eq('store：筛选走 ListSources(status)；搜索带同样的 status（v0.23.2），有关键字时切筛选重新搜索', [/listSources\(\{[^}]*status \}\)/.test(st), /searchSources\(\{[^}]*, status \}\)/.test(st), /if \(kw\) return search\(keyword\.value\)/.test(st)], [true, true, true])
    eq('store：筛选“失败”时行默认展开（ListSources / SearchSources 两处）', (st.match(/status === 'failed'\) for \(const id of order\) if \(!known\.has\(id\)\) foldSession\[id\] = true|sh\.status === 'failed'\) foldSession/g) ?? []).length, 2)
    const pg = readSrc('src/views/ConvertPage.vue')
    eq('页面：没有“已加载的记录里…”提示，筛选为空时是普通空状态（设计 §四 11 文案）；搜索时筛选不置灰', [/已加载的记录里/.test(pg), pg.includes('没有进行中的记录'), pg.includes('没有失败的记录'), /filterOff|搜索时显示全部状态/.test(pg)], [false, true, true, false])
    // 契约 6.14.4：10 秒内没停下来的记录不删（still_running，path 为空）；DeleteSource 有一条没删就保留这一行
    {
      const win = (globalThis as unknown as { window: { location: { search: string } } }).window
      const prev = win.location.search
      const T2 = (t: ReturnType<typeof deleteToast>) => [toastText(t.parts), t.warn, t.path]
      try {
        mock.resetConvertMock('mixed')
        win.location.search = '?cv_delfail=still_running'
        const r = await mock.DeleteSource('mock-src-launch', true)
        const left = await mock.GetSource('mock-src-launch')
        eq('still_running：进行中那条不删、path 为空；其余照删；行保留（deletedSourceIds 为空）', [r.failures.map((f) => [f.reason, f.path]), r.deletedTaskIds.length, r.deletedSourceIds, left.recordCount, left.records.map((x) => x.status)], [[['still_running', '']], 2, [], 1, ['running']])
        eq('still_running 的提示：行保留句，没有文件夹操作', T2(deleteToast({ kind: 'source', id: 'mock-src-launch', name: 'launch-4k.mov' }, r)), ['已删除 2 条记录。有 1 条转换没能及时停止，“launch-4k.mov”仍保留在列表里，请稍后再移除。', true, ''])
        eq('行保留后：GetSource / ListSources 仍能看到这一行和留下的进行中记录', (await mock.ListSources({ limit: 50, offset: 0, recordLimit: 20 })).items.find((e) => e.source.sourceId === 'mock-src-launch')?.records.map((x) => x.id), ['simcv-launchRun1080'])
        // 再移除一次：还停不下来 → n=0
        const again = await mock.DeleteSource('mock-src-launch', true)
        eq('再移除仍停不下来：n=0，去掉第一句，行仍保留', [again.deletedTaskIds.length, again.deletedSourceIds, T2(deleteToast({ kind: 'source', id: 'mock-src-launch', name: 'launch-4k.mov' }, again))[0]], [0, [], '有 1 条转换没能及时停止，“launch-4k.mov”仍保留在列表里，请稍后再移除。'])
        mock.resetConvertMock('mixed')
        win.location.search = '?cv_delfail=in_use,still_running'
        const r3 = await mock.DeleteSource('mock-src-launch', true)
        const n3 = deleteToast({ kind: 'source', id: 'mock-src-launch', name: 'launch-4k.mov' }, r3)
        eq('两种失败都有：still_running 句在前、文件句在后，各自计数；文件夹取第一条非空 path', [toastText(n3.parts), n3.warn, !!n3.path, n3.path === r3.failures.find((f) => f.path)?.path], ['已删除 2 条记录。有 1 条转换没能及时停止，“launch-4k.mov”仍保留在列表里，请稍后再移除。有 1 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。', true, true, true])
        // 模拟：多个文件类原因按文件轮流（in_use,permission）→ 行删掉，两句按顺序
        mock.resetConvertMock('mixed')
        win.location.search = '?cv_delfail=permission,in_use'
        const rp = await mock.DeleteSource('mock-src-iv', true)
        eq('模拟 DeleteSource 多种文件失败：行删掉；被占用句在没有权限句前面', [rp.deletedSourceIds, rp.failures.map((f) => f.reason).sort(), toastText(deleteToast({ kind: 'source', id: 'mock-src-iv', name: '采访-机位A.mkv' }, rp).parts)], [['mock-src-iv'], ['in_use', 'permission'], '已从列表移除“采访-机位A.mkv”和 2 条记录。有 1 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。有 1 个文件没有权限删除，请手动删除。'])
        // DeleteRecords：进行中那条停不下来 → 不删、仍可见
        mock.resetConvertMock('mixed')
        win.location.search = '?cv_delfail=still_running'
        const rr = await mock.DeleteRecords(['simcv-launchRun1080', 'simcv-launchFail'], false)
        eq('DeleteRecords still_running：停不下来的那条保留，其余照删', [rr.deletedTaskIds, rr.failures.map((f) => [f.taskId, f.reason, f.path]), (await mock.GetSource('mock-src-launch')).records.map((x) => x.id).includes('simcv-launchRun1080')], [['simcv-launchFail'], [['simcv-launchRun1080', 'still_running', '']], true])
        eq('DeleteRecords still_running 的提示（n>0 / n=0）', [T2(deleteToast({ kind: 'record', id: 'x', name: '' }, rr))[0], T2(deleteToast({ kind: 'record', id: 'x', name: '' }, await mock.DeleteRecords(['simcv-launchRun1080'], false)))[0]], ['已删除 1 条记录。有 1 条转换没能及时停止，请稍后再删除。', '有 1 条转换没能及时停止，请稍后再删除。'])
        // v0.23.3：打开所在文件夹失败——移走了 NOT_FOUND →“找不到这个文件。”；超过 10 分钟 / 重启 INVALID_ARGUMENT → 普通错误提示
        win.location.search = '?cv_revealfail=NOT_FOUND'
        const nf = await rejects(mock.revealDeleteFailureMock('D:\\x.mp4'))
        win.location.search = '?cv_revealfail=INVALID_ARGUMENT'
        const ia = await rejects(mock.revealDeleteFailureMock('D:\\x.mp4'))
        win.location.search = prev
        eq('打开所在文件夹失败的提示：NOT_FOUND / INVALID_ARGUMENT（超时）都写“找不到这个文件”（无句号），其他用错误本身的文案；正常不报错', [nf && revealDeleteFailureText(nf), ia && revealDeleteFailureText(ia), revealDeleteFailureText({ code: 'IO_ERROR', message: '打不开' }), await rejects(mock.revealDeleteFailureMock('D:\\x.mp4'))], ['找不到这个文件', '找不到这个文件', '打不开', null])
        eq('页面：打开所在文件夹失败走 revealDeleteFailureText，不再静默忽略 INVALID_ARGUMENT', [/revealDeleteFailure\(path\)\.catch\(\(e\) => ElMessage\.error\(revealDeleteFailureText\(toAppError\(e\)\)\)\)/.test(readSrc('src/views/ConvertPage.vue')), /INVALID_ARGUMENT/.test(readSrc('src/views/ConvertPage.vue'))], [true, false])
        mock.resetConvertMock('mixed')
        win.location.search = prev
        const r2 = await mock.DeleteSource('mock-src-launch', false)
        eq('不注入：进行中的先取消再删，行一起删掉', [r2.failures.length, r2.deletedSourceIds], [0, ['mock-src-launch']])
      } finally {
        win.location.search = prev
      }
    }
    // v0.23.2：SearchSources 的 status 与 ListSources 同语义
    mock.resetConvertMock('mixed')
    const sr = async (keyword: string, status: '' | 'active' | 'failed') => (await mock.SearchSources({ keyword, limit: 50, offset: 0, recordLimit: 20, status })).items.map((e) => e.source.sourceId.replace('mock-src-', ''))
    eq('SearchSources status：关键字 + 失败 / 进行中 / 全部', [await sr('mov', ''), await sr('mov', 'failed'), await sr('mov', 'active'), await sr('采访', 'failed')], [['launch', 'wed'], ['launch'], ['launch'], []])
    eq('SearchSources status 其它值 → INVALID_ARGUMENT', (await rejects(mock.SearchSources({ keyword: 'a', limit: 50, offset: 0, recordLimit: 20, status: 'x' as never })))?.code, 'INVALID_ARGUMENT')
  }
  // ---- 原地重试（含已取消）、隐藏 / 取消隐藏 ----
  mock.resetConvertMock('canceled')
  {
    const seen: { id: string; retried?: boolean; status: string }[] = []
    const off = onSimEvent<{ id: string; retried?: boolean; status: string }>('task:status', (p) => { if (p.id === 'simcv-launchCx') seen.push(p) })
    const t = retrySimTask('simcv-launchCx')
    eq('已取消的转换原地重试：同一个 id、回到排队、进度清零、发 task:status retried:true', [t.id, t.status, t.progress, seen[0]?.retried, seen[0]?.status], ['simcv-launchCx', 'queued', 0, true, 'queued'])
    off()
    if (getSimTask('simcv-launchCx')) cancelSimTask('simcv-launchCx')
    const visibleBefore = listSimFinished(false).map((x) => x.id)
    const n = hideSimFinished()
    eq('隐藏已结束：已结束的都隐藏，记录仍在（GetSource 照常返回）', [n > 0, listSimFinished(false).some((x) => x.id === 'simcv-launchWebmOk'), (await mock.GetSource('mock-src-launch')).records.some((r) => r.id === 'simcv-launchWebmOk')], [true, false, true])
    {
      setActivePinia(createPinia())
      const ts = useTaskStore()
      await ts.loadStats()
      const off3 = [ts.finishedTotal, ts.failedTotal, ts.todayDone]
      ts.historyFilter.includeHidden = true
      await ts.loadStats()
      const on3 = [ts.finishedTotal, ts.failedTotal, ts.todayDone]
      ts.historyFilter.includeHidden = false
      eq('任务中心计数：隐藏后页签计数不含已隐藏；“显示已隐藏”打开时含（用 total）；今日完成不受开关影响', [off3[0] < on3[0], on3[0] >= listSimFinished(true).filter((x) => x.type === 'convert').length, off3[2] === on3[2], on3[2] > 0], [true, true, true, true])
    }
    const un: { id: string; hiddenInTaskCenter?: boolean }[] = []
    const off2 = onSimEvent<{ id: string; hiddenInTaskCenter?: boolean }>('task:status', (p) => un.push(p))
    unhideSimTasks(['simcv-launchWebmOk'])
    off2()
    eq('取消隐藏：发 task:status hiddenInTaskCenter:false，回到默认列表', [un.length, un[0]?.hiddenInTaskCenter, listSimFinished(false).some((x) => x.id === 'simcv-launchWebmOk')], [1, false, true])
    unhideSimTasks(visibleBefore) // 其它自检的模拟任务恢复原样
    eq('UnhideInTaskCenter：未知 id → NOT_FOUND reason=record', (await rejects(mock.UnhideInTaskCenter(['nope'])))?.reason, 'record')
  }
  mock.resetConvertMock('mixed')
  // ---- 源码约定 ----
  const tc = readSrc('src/views/TaskCenter.vue')
  const page = readSrc('src/views/ConvertPage.vue')
  const panel = readSrc('src/components/convert/ConvertSettingsPanel.vue')
  const del = readSrc('src/components/convert/ConvertDeleteDialog.vue')
  const row = readSrc('src/components/convert/ConvertSourceRow.vue')
  const kid = readSrc('src/components/convert/ConvertKid.vue')
  const pv = readSrc('src/components/convert/ConvertPreviewDialog.vue')
  eq('任务中心：按钮“隐藏已结束”+ 说明 tooltip；“显示已隐藏”每次进入默认关', [/HIDE_FINISHED_LABEL = '隐藏已结束'/.test(tc), tc.includes('只从任务中心隐藏，转换记录仍保留在转换页'), /tasks\.historyFilter\.includeHidden = false/.test(tc), /localStorage[^\n]*includeHidden|includeHidden[^\n]*localStorage/.test(tc)], [true, true, true, false])
  eq('任务中心：转换行不给“移除”，给“在转换页查看”（先 GetSource，已删时提示）', [tc.includes('转换记录请在转换页删除'), /getSource\(t\.sourceId/.test(tc), tc.includes('这条转换记录已被删除')], [true, true, true])
  eq('任务中心转换行第二行用共用格式化函数', /recordParamsText\(/.test(tc), true)
  eq('转换页：没有“再转一个”/ startOver，没有 Reconvert 入口', [/再转一个|startOver/.test(page + panel + row + kid + pv), /\breconvert\(/.test(page + panel + row + kid + pv)], [false, false])
  eq('转换页：“转换 N 个文件”按钮', /转换 \$\{[^}]+\} 个文件/.test(panel), true)
  eq('删除确认：只删记录；勾选“同时删除输出文件”才提示无法恢复', [/只删除记录，(<b>)?不删除磁盘上的文件/.test(del), del.includes('同时删除输出文件'), del.includes('删除后无法恢复。')], [true, true, true])
  {
    const st = readSrc('src/stores/convertRecords.ts')
    eq('源文件行删除定稿：标题 / 按钮提示 / 菜单项“从列表移除”；没有旧说法', [/SOURCE_REMOVE_TITLE = '从列表移除这个文件和它的全部记录'/.test(st), /SOURCE_REMOVE_LABEL = '从列表移除'/.test(st), /:aria-label="`\$\{SOURCE_REMOVE_LABEL\} \$\{src\.name\}`" :title="SOURCE_REMOVE_LABEL"/.test(row), /\{\{ SOURCE_REMOVE_LABEL \}\}<\/button>/.test(row), /删除源文件和全部记录/.test(row + del + st + page)], [true, true, true, true, false])
    eq('源文件行确认按钮：普通“移除”，勾选后红色“移除并删除文件”', [/withOutput\.value \? '移除并删除文件' : '移除'/.test(del), /a\.value\?\.kind === 'record' \|\| withOutput\.value \? 'danger' : 'pri'/.test(del)], [true, true])
    eq('移除弹窗：标题下文件行（名称 · n 条转换记录）；“移除时会先取消它”；未勾选图标中性灰', [/class="cv-delfile"/.test(del) && /\{\{ a\.count \}\} 条转换记录/.test(del), /'移除' : '删除'\}时会先取消它/.test(del), /:class="\{ neutral: !danger \}"/.test(del)], [true, true, true])
    eq('toast：普通 4 秒、警告 8 秒；有失败时带“打开所在文件夹”（RevealInFolder(failures[0].path)）', [/TOAST_MS = 4000/.test(page), /WARN_TOAST_MS = 8000/.test(page), page.includes('打开所在文件夹'), /revealInFolder\(path\)/.test(readSrc('src/api/convertRecords.ts'))], [true, true, true, true])
  }
  eq('三处第 2 行都走 recordLine / recordParamsText', [/recordLine\(/.test(kid), /recordLine\(/.test(pv)], [true, true])
  eq('预览：快捷键空格 / ←→ 5 秒 / F / Esc；token 404 重新取一次地址', [/' '|'Space'/.test(pv), /ArrowLeft/.test(pv) && /ArrowRight/.test(pv), /'f'|'F'/.test(pv), /Escape/.test(pv), /404/.test(pv)], [true, true, true, true, true])
  eq('记录用 RevealRecord（不再用 RevealInFolder(path)）', [/revealRecord\(/.test(readSrc('src/stores/convertRecords.ts')), /RevealInFolder|revealInFolder/.test(readSrc('src/api/convertRecordsBinding.ts'))], [true, false])
  eq('第 2 行 / 预览页脚只用快照 presetName，不读当前预设卡片', [/presetLabel|presetTitle/.test(kid + row + pv)], [false])
  eq('失败卡片：ErrorLine 卡片版（操作左、错误码右，同一行）', [(kid.match(/actions-row/g) ?? []).length, /class="arow-line"/.test(readSrc('src/components/common/ErrorLine.vue'))], [2, true])
  eq('任务中心：固定列宽，操作列按内容估算，任务名优先；窄窗口“演示”只在 title', [/\.tbl \{ table-layout: fixed; \}/.test(tc), /'--ops-w': opsWidth \+ 'px'/.test(tc), /\.simtag \{ display: none; \}/.test(tc)], [true, true, true])
  eq('任务中心（走查 X9）：“失败”卡片和页签同一个数 failedTotal（都跟“显示已隐藏”）；没有 failedCard', [/<small>失败<\/small><b[^>]*>\{\{ tasks\.failedTotal \}\}/.test(tc), /failedCard/.test(tc + readSrc('src/stores/tasks.ts')), /void loadStats\(\) \/\/ 页签计数跟着开关/.test(readSrc('src/stores/tasks.ts'))], [true, false, true])
  eq('任务中心（走查 X10）：失败 / 已中断的进度条上方只放百分比，原因在错误行（N3 后 1024 另有短格式）', [/<div class="pline"[^>]*>\s*<span class="wide">\{\{ barText\(t\) \}\}<\/span>/.test(tc), /const barText = \(t: TaskItem\): string => \(t\.status === 'failed' \|\| t\.status === 'interrupted' \? '' : progressText\(t\)\)/.test(tc)], [true, true])
  eq('预览（走查 G4）：有画面的文件 loadedmetadata 后 videoWidth = 0 → 无法在应用内播放', [/expectsVideo\.value && \(el as HTMLVideoElement\)\.videoWidth === 0\) \{\s*el\.pause\(\)\s*playing\.value = false\s*stage\.value = 'unplayable'/.test(pv), /const expectsVideo = computed\(\(\) => \(rec\.value \? !isAudioContainer\(container\.value\) : !!srcInfo\.value && !isAudioOnly\(srcInfo\.value\)\)\)/.test(pv)], [true, true])
  eq('预览（走查 X5）：放不了 / 文件不在 / 出错时控制条收起，音量条不显示', [/const failed = computed\(\(\) => stage\.value === 'unplayable' \|\| stage\.value === 'gone' \|\| stage\.value === 'error'\)/.test(pv), /class="cv-pvbody" :class="\{ nobar: failed \}"/.test(pv), /\.cv-pvbody\.nobar \.ff-player \.ctrl\{display:none\}/.test(readSrc('src/components/convert/convert-v2.css')), /v-if="kind !== 'gif' && !failed" type="button" class="cv-vol"/.test(pv)], [true, true, true, true])
  eq('转换页提示（走查 X4）：放在左栏栏头下面，两处都带 offset', (page.match(/offset: toastOffset\(\)/g) ?? []).length, 2)
  eq('1280 垃圾桶（走查 X7）：aria-label 带文件名，title 不变', /:aria-label="`\$\{SOURCE_REMOVE_LABEL\} \$\{src\.name\}`" :title="SOURCE_REMOVE_LABEL"/.test(row), true)
  let oldGone = true
  try { readSrc('src/stores/convert.ts'); oldGone = false } catch { /* 已删除 */ }
  try { readSrc('src/components/convert/ConvertFileRow.vue'); oldGone = false } catch { /* 已删除 */ }
  eq('旧转换页 store / 行组件已删除', oldGone, true)
  // ---- 走查 S1：记录数只在一处计数（同一个任务 id 只加一次），栏头 / 行 / 移除确认框一致 ----
  {
    mock.resetConvertMock('added')
    setActivePinia(createPinia())
    const cv = useConvertRecordsStore()
    const ts = useTaskStore()
    await cv.reload()
    const sid = 'mock-src-demo'
    const c0 = cv.sources[sid]?.recordCount ?? -1
    const h0 = cv.recordCount
    const o = { container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', width: 0, height: 0, fps: 0, videoBitrate: 0, audioBitrate: 0, crf: 0, targetSizeMb: 0, trimStart: 0, trimEnd: 0 }
    const sub = async () => (await mock.SubmitSources({ sourceIds: [sid], options: o, outputDir: '', presetId: 'builtin-mp4-h264' })).tasks[0]
    // 真实后端里 task:status 可能先于 SubmitSources 返回：先进任务 store（监听里加），再 afterSubmit
    const t1 = await sub()
    ts.track([t1] as never)
    await nextTick()
    await nextTick()
    const afterEvent = cv.sources[sid].recordCount
    cv.afterSubmit([t1])
    const afterSubmit1 = cv.sources[sid].recordCount
    // 反过来：先 afterSubmit（里面 track），再收到 task:status
    const t2 = await sub()
    cv.afterSubmit([t2])
    const afterSubmit2 = cv.sources[sid].recordCount
    ts.track([t2] as never)
    await nextTick()
    await nextTick()
    eq('S1：提交 1 条 +1，收到首个 task:status 不再变（两种先后顺序）', [c0 >= 0, afterEvent - c0, afterSubmit1 - c0, afterSubmit2 - c0, cv.sources[sid].recordCount - c0], [true, 1, 1, 2, 2])
    eq('S1：栏头总数、行记录数、移除确认框的条数一致', [cv.recordCount - h0, cv.deleteAsk('source', sid)?.count, cv.parents.find((p) => p.src.sourceId === sid)?.kids.length], [2, c0 + 2, c0 + 2])
    const r = await cv.confirmDelete(cv.deleteAsk('source', sid)!, false)
    eq('S1：移除后 toast 用的条数 = 确认框里的条数', r.deletedTaskIds.length, c0 + 2)
    const st = readSrc('src/stores/convertRecords.ts')
    eq('S1：recordCount++ 只出现在 addRecord 里一处', (st.match(/recordCount\+\+/g) ?? []).length, 1)
  }
  // ---- 走查 G1：添加文件后 listOffset 跟着推进，不误出“加载更早的记录” ----
  {
    mock.resetConvertMock('added')
    setActivePinia(createPinia())
    const cv = useConvertRecordsStore()
    await cv.reload()
    const n0 = cv.sourceCount
    const before = cv.hasMore
    await cv.addPaths(['D:\\Videos\\走查G1-新文件.mp4'])
    const afterNew = [cv.hasMore, cv.sourceCount - n0]
    await cv.addPaths(['D:\\Videos\\走查G1-新文件.mp4'])
    eq('G1：加载完没有更多 → 添加新文件、重复添加后仍然没有“加载更早的记录”，文件数只加 1', [before, ...afterNew, cv.hasMore, cv.sourceCount - n0], [false, false, 1, false, 1])
  }
  mock.resetConvertMock('mixed')
}
