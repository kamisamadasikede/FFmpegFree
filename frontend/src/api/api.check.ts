// 接口层自检（不引入测试框架）：node scripts/check-api.mjs 用 esbuild 打包后运行，失败退出码 1。
// 覆盖：AppError 的 reason / clipId 解析、TASK_CONFLICT 文案表、推流地址校验与脱敏、
// 模拟层（Live 两种 TASK_CONFLICT、停止语义、Doc 的 UNSUPPORTED）。
import { AppError, parseDetailHead, toAppError, BACKEND_ERROR_CODES, callService, docErrorReason, DOC_ERROR_REASONS } from './call'
import { toApiTask, type TaskProgressPayload, type TaskStatusPayload, type ApiTask } from './taskTypes'
import {
  taskConflictText, TASK_CONFLICT_GENERIC, actionErrorText, liveStartErrorLine, LIVE_STOP_TEXT, docUnsupportedText, errorMessages, taskErrorMessages,
  liveUrlInvalidText, LIVE_URL_INVALID_GENERIC, LIVE_URL_INVALID_REASON_TEXT, liveFailureMessage, liveConnectFailedText, LIVE_SRT_CONNECT_FAILED_TEXT, LIVE_RTMP_CONNECT_FAILED_TEXT, schemeFromParams,
  liveFfmpegProtocolMissingText, hasMissingLine, LIVE_CANCELED_ARCHIVE_KEPT_TEXT, LIVE_STOPPING_TEXT, LIVE_PUSH_REJECTED_TEXT, LIVE_SRT_PASSPHRASE_TEXT, resolveError, resolveTaskError,
} from '@/errors/errorMessages'
import { parsePushUrl, redactPushUrl } from '@/utils/liveUrl'
import * as live from './live'
import * as doc from './doc'
import { docErrorText, docErrorFile, docErrorPath, docReasonOf, pdfErrorView, DOC_TOO_MANY_PAGES_TEXT, DOC_FILE_BROKEN_TEXT, DOC_FORMAT_UNSUPPORTED_TEXT, DOC_ENCRYPTED_TEXT, DOC_NO_FONT_TEXT, DOC_TOO_LARGE_TEXT } from '@/errors/errorMessages'
import { ffmpegStatusView } from '@/components/ffmpeg/statusView'
import { splitMiddle, nextZoom, thumbWindow, formatRecentTime, extBadge } from '@/utils/docLogic'
import { onSimEvent } from '@/services/wails'
import { retrySimTask, SIM_TITLE_PREFIX, simEncoderScenario } from './sim'
import { PullPreviewController, type PullPreviewApi } from './pullPreviewSession'
import * as pvMsg from '@/errors/livePreviewMessages'
import * as encApi from './encoder'
import { deriveEncoderView, createSeq } from './encoderView'
import * as encMsg from '@/errors/encoderMessages'
import { pushErrorToForm } from '@/views/live/pushErrors'
import { canRetryTask, elapsedMs, isKnownTaskType, isLegacyTaskType, isRetiredType, useTaskStore } from '@/stores/tasks'
import { mainNav } from '@/layout/navigation'
import { createPinia, setActivePinia } from 'pinia'
import { emitSimEvent } from '@/services/wails'
import * as encTask from './encoderTask'
import { convertV2Checks } from './convertV2.check'
import * as d26 from '@/utils/docV26Text'
import * as d27 from '@/utils/docV27Text'
import { PREVIEW_CSP, PREVIEW_SANDBOX, buildSrcdoc, extractHeadStyles } from '@/utils/docSafeHtml'
import { bytesToBase64, chunkRanges, isSameFormat, saveFilters } from '@/api/docV27'
import { convertV24Checks } from './convertV24.check'
import { liveFormsChecks } from '@/stores/liveForms.check'
import { readyRelistChecks } from '@/stores/readyRelist.check'
import { catalogLoadChecks } from '@/stores/catalogLoad.check'
import { pkg22Checks } from '@/stores/eventOrder.check'

const fails: string[] = []
const eq = (name: string, got: unknown, want: unknown) => {
  const a = JSON.stringify(got)
  const b = JSON.stringify(want)
  if (a !== b) fails.push(`✗ ${name}: 得到 ${a}，期望 ${b}`)
}
async function rejects(p: Promise<unknown>): Promise<AppError | null> {
  try {
    await p
    return null
  } catch (e) {
    return toAppError(e)
  }
}
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

/** node 里的模拟 window：只用到 location.search，由 scripts/check-api.mjs 挂到 globalThis */
const win = (globalThis as unknown as { window: { location: { search: string } } }).window

function simEncoderScenarioFor(v: string) {
  const prev = win.location.search
  win.location.search = `?enc=${v}`
  const r = simEncoderScenario()
  win.location.search = prev
  return r
}

export async function runApiChecks(): Promise<string[]> {
  // ---- AppError：reason / clipId ----
  eq('reason=max_sessions', new AppError('TASK_CONFLICT', 'x', 'reason=max_sessions\n最多同时进行 4 个').reason, 'max_sessions')
  eq('reason=duplicate_url', new AppError('TASK_CONFLICT', 'x', 'reason=duplicate_url').reason, 'duplicate_url')
  eq('没有 reason', new AppError('TASK_CONFLICT', 'x', '任务已经结束').reason, undefined)
  eq('reason 不在首行不算', new AppError('TASK_CONFLICT', 'x', '说明\nreason=max_sessions').reason, undefined)
  // Doc 错误：detail 首行 reason=<枚举>，其后可以有路径行 / 说明行
  eq('Doc reason 枚举', [...DOC_ERROR_REASONS], ['too_many_pages', 'format', 'encrypted', 'no_font', 'invalid_ooxml', 'too_large'])
  eq('Doc reason 首行', new AppError('UNSUPPORTED', '超过 5000 页', 'reason=too_many_pages\n已排到第 5001 页仍未结束').reason, 'too_many_pages')
  eq('Doc reason 后跟路径行', docErrorReason(new AppError('INVALID_ARGUMENT', '不是有效的 OOXML 文件', 'reason=invalid_ooxml\n/d/a.docx\nzip: not a valid zip file').reason), 'invalid_ooxml')
  eq('docErrorReason 不认识的值', [docErrorReason('screen_busy'), docErrorReason(undefined), docErrorReason('')], [undefined, undefined, undefined])
  eq('Doc reason 不在首行不算', docErrorReason(new AppError('UNSUPPORTED', 'x', '/d/a.doc\nreason=format').reason), undefined)
  eq('clip 首行', parseDetailHead('clip=c_1-a path=/a b/中文.mp4\n原因'), { clipId: 'c_1-a', path: '/a b/中文.mp4' })
  eq('project 首行没有 clip', parseDetailHead('project\n视频轨不能为空'), {})
  eq('toAppError 解析 JSON 的 detail', toAppError('{"code":"TASK_CONFLICT","message":"m","detail":"reason=duplicate_url"}').reason, 'duplicate_url')
  eq('BACKEND_ERROR_CODES 38 个（v0.26~v0.28 DOC_* + v0.29 LANG_* + v0.30 CAT_*）', BACKEND_ERROR_CODES.length, 38)
  eq('LIVE_SOURCE_GONE 的 kind 首行', [new AppError('LIVE_SOURCE_GONE', 'x', 'kind=window').kind, new AppError('LIVE_SOURCE_GONE', 'x', 'kind=screen').kind, new AppError('LIVE_SOURCE_GONE', 'x', 'kind=other').kind, new AppError('LIVE_SOURCE_GONE', 'x').kind], ['window', 'screen', undefined, undefined])

  // ---- TASK_CONFLICT 文案（reason → 文案 一张表）----
  eq('max_sessions 文案', taskConflictText('max_sessions'), '最多同时推 4 路')
  eq('duplicate_url 文案', taskConflictText('duplicate_url'), '这个地址已经在推流')
  eq('未知 reason → 通用', taskConflictText('single_screen_only'), TASK_CONFLICT_GENERIC)
  eq('缺失 reason → 通用', taskConflictText(undefined), '操作冲突，请稍后再试')
  eq('Cancel / Remove 的冲突 → 通用', actionErrorText('TASK_CONFLICT', '任务已结束'), '操作冲突，请稍后再试')
  eq('原型链上的键不算 reason', taskConflictText('toString'), TASK_CONFLICT_GENERIC)
  eq('直播停止文案', LIVE_STOP_TEXT, { succeeded: '已结束推流', canceled: '已强制停止' })
  eq('UNSUPPORTED 起始错误行（无协议名）', liveStartErrorLine({ code: 'UNSUPPORTED' })?.description, '当前转换组件不支持这种推流协议。请到设置的“转换组件”里重新安装或更新。')

  eq('任务中心：reason=format / encrypted 的 UNSUPPORTED', [docUnsupportedText('暂不支持这种格式', 'reason=format\n/d/a.doc\n.doc：旧版'), docUnsupportedText('暂不支持这种格式', 'reason=encrypted')], [DOC_FORMAT_UNSUPPORTED_TEXT, DOC_ENCRYPTED_TEXT])
  eq('任务中心：超 5000 页沿用后端 message（reason=too_many_pages）', docUnsupportedText('超过 5000 页', 'reason=too_many_pages\n已排到第 5000 页仍未结束'), '超过 5000 页')
  eq('任务中心：缺字体沿用后端 message（reason=no_font）', docUnsupportedText('没有可用的 Unicode 字体', 'reason=no_font'), '没有可用的 Unicode 字体')
  eq('任务中心：reason 缺失按 message 精确相等兜底；认不出的回落后端 message', [docUnsupportedText('暂不支持这种格式', ''), docUnsupportedText('别的原因', '某行'), docUnsupportedText('', undefined)], [DOC_FORMAT_UNSUPPORTED_TEXT, '别的原因', docUnsupportedText('', undefined)])
  // ---- 旧任务类型忽略 ----
  eq('live_relay 忽略', isKnownTaskType('live_relay'), false)
  eq('live_record_push 忽略', isKnownTaskType('live_record_push'), false)
  eq('edit_render 忽略', isKnownTaskType('edit_render'), false)
  eq('未知类型忽略', isKnownTaskType('whatever'), false)
  eq('旧类型识别', ['live_relay', 'live_record_push', 'edit_render'].every(isLegacyTaskType), true)
  eq('用时：正常', elapsedMs(1000, 4000), 3000)
  eq('用时：缺 startedAt', elapsedMs(undefined, 4000), null)
  eq('用时：startedAt=0', elapsedMs(0, 4000), null)
  eq('用时：结束早于开始', elapsedMs(5000, 4000), null)
  eq('用时：NaN', elapsedMs(NaN, 4000), null)
  eq('live_screen_push 认识', isKnownTaskType('live_screen_push'), true)
  eq('edit_export 认识', isKnownTaskType('edit_export'), true)

  // ---- 推流地址 ----
  const failKind = (u: string) => { const r = parsePushUrl(u); return r.ok ? 'ok' : r.kind }
  const failReason = (u: string) => { const r = parsePushUrl(u); return r.ok ? 'ok' : r.reason }
  eq('rtsp 协议不支持', failKind('rtsp://h/live'), 'protocol')
  eq('http-flv 协议不支持', failKind('http://h/live.flv'), 'protocol')
  eq('srt 缺端口', failKind('srt://h?streamid=a'), 'port')
  eq('srt listener 拒绝', failKind('srt://h:9000?mode=listener'), 'mode')
  eq('rtmp 缺应用名', failKind('rtmp://h/'), 'app')
  const okUrl = parsePushUrl('rtmp://H.example/live/k')
  eq('rtmp 默认端口', okUrl.ok ? okUrl.info.port : -1, 1935)
  // 本地校验按后端 reason 枚举归类
  eq('reason：协议不支持', failReason('rtsp://h/live'), 'scheme_unsupported')
  eq('reason：格式错', failReason('not a url'), 'malformed')
  eq('reason：缺主机', failReason('rtmp:///live/k'), 'missing_host')
  eq('reason：srt listener', failReason('srt://h:9000?mode=listener'), 'param_not_allowed')
  eq('含空白拒绝', parsePushUrl('rtmp://h/live/a b').ok, false)
  eq('脱敏 rtmp', redactPushUrl('rtmp://u:p@h:1935/live/abc123?token=xyz'), 'rtmp://***@h:1935/live/***?token=***')
  eq('脱敏 srt', redactPushUrl('srt://h:9000?streamid=a&passphrase=b'), 'srt://h:9000?streamid=***&passphrase=***')
  eq('脱敏 多段流名', redactPushUrl('rtmp://h/live/a/b/c'), 'rtmp://h/live/***')
  eq('脱敏 非法串不回显', redactPushUrl('not a url secret'), '<invalid-url>')

  let err: AppError | null

  // ---- Live 预览（契约 v0.25）：不再有 GetPreview 截图 ----
  eq('v0.25：模拟层不再提供 GetPreview', typeof (live as { getPreview?: unknown }).getPreview, 'undefined')
  eq('模拟 StartPullPreview：不出画面且 previewUrl 为空', [((await live.startPullPreview({ url: 'http://h/live/abc' })).preview), (await live.startPullPreview({ url: 'http://h/live/abc' })).previewUrl], [false, ''])
  eq('模拟 StopPullPreview 无操作', await live.stopPullPreview('x'), undefined)

  // ---- Live：两种 TASK_CONFLICT 都能由模拟层复现 ----
  win.location.search = ''
  const req = (url: string): live.FilePushRequest => ({ inputPath: '/m/a.mp4', url, loop: true, options: live.defaultPushOptions() })
  win.location.search = '?sim_missing=srt'
  err = await rejects(live.startFilePush(req('srt://h9.example:9000?streamid=a')))
  eq('缺 srt 协议 → UNSUPPORTED + detail', [err?.code, err?.detail], ['UNSUPPORTED', 'missing=srt'])
  win.location.search = ''
  const first = await live.startFilePush(req('rtmp://h1.example/live/secretkey1'))
  eq('Start 返回入队快照', [first.status, first.version, first.progress], ['queued', 1, -1])
  eq('标题脱敏', first.title.includes('secretkey1'), false)
  eq('模拟任务标题带“演示”前缀', first.title.startsWith(SIM_TITLE_PREFIX), true)
  eq('params 脱敏', first.params.includes('secretkey1'), false)
  err = await rejects(live.startFilePush(req('rtmp://h1.example/live/secretkey1')))
  eq('duplicate_url', [err?.code, err?.reason], ['TASK_CONFLICT', 'duplicate_url'])
  eq('duplicate_url 的 detail 不带地址', (err?.detail ?? '').includes('h1.example') || (err?.detail ?? '').includes('secretkey1'), false)
  eq('duplicate_url 文案', taskConflictText(err?.reason), '这个地址已经在推流')
  // 屏幕推流同一时间最多 1 路；判断顺序 duplicate_url → screen_busy → max_sessions
  const screenReq = (url: string): live.ScreenPushRequest => ({ screenId: 'avf:0', url, hideCursor: false, audio: 'none', archiveDir: '', options: live.defaultPushOptions() })
  // v0.14 采集来源：列表 = 屏幕 + 窗口；来源失效 → LIVE_SOURCE_GONE（detail 首行 kind=）；格式不对 INVALID_ARGUMENT；失败不占会话
  const sources = await live.listCaptureSources()
  eq('采集来源：屏幕在前，窗口在后', sources.map((s) => s.kind), ['screen', 'screen', 'window', 'window'])
  eq('采集来源 id', sources.map((s) => s.id), ['screen:0', 'screen:1', 'window:65890', 'window:131426'])
  const runningBefore = (await live.listRunning()).length
  err = await rejects(live.startScreenPush({ ...screenReq('rtmp://g1.example/live/g1'), captureSourceId: 'window:999' }))
  eq('LIVE_SOURCE_GONE（窗口）', [err?.code, err?.kind, err?.detail], ['LIVE_SOURCE_GONE', 'window', 'kind=window'])
  err = await rejects(live.startScreenPush({ ...screenReq('rtmp://g1.example/live/g1'), captureSourceId: 'screen:9' }))
  eq('LIVE_SOURCE_GONE（屏幕）', [err?.code, err?.kind], ['LIVE_SOURCE_GONE', 'screen'])
  err = await rejects(live.startScreenPush({ ...screenReq('rtmp://g1.example/live/g1'), captureSourceId: 'window:0x10' }))
  eq('captureSourceId 格式不对', err?.code, 'INVALID_ARGUMENT')
  eq('来源失败不占会话', (await live.listRunning()).length, runningBefore)

  // 来源选择器（RecordPush 用）：多屏 / 含窗口 / 仅屏幕 / 空 / 列表失败 / 来源消失后刷新
  eq('选择器：默认选第一个屏幕（屏幕在前）', (sources.find((x) => x.kind === 'screen') ?? sources[0]).id, 'screen:0')
  eq('选择器：多屏 + 含窗口 → 两个分组都有', ['screen', 'window'].map((k) => sources.filter((x) => x.kind === k).length), [2, 2])
  win.location.search = '?sim_sources=screens'
  const onlyScreens = await live.listCaptureSources()
  eq('仅屏幕（平台不返回 window）→ 没有 window 项，分组标题不出现', [onlyScreens.length, onlyScreens.some((x) => x.kind === 'window')], [2, false])
  win.location.search = '?sim_sources=empty'
  eq('列表为空', (await live.listCaptureSources()).length, 0)
  win.location.search = '?sim_sources=fail'
  eq('列表失败 → 抛错', (await rejects(live.listCaptureSources()))?.code, 'INTERNAL')
  win.location.search = '?sim_source_gone=1'
  live.resetSimSources()
  const before = await live.listCaptureSources()
  eq('来源消失：第一次拉取时窗口还在', before.some((x) => x.id === 'window:65890'), true)
  const goneErr = await rejects(live.startScreenPush({ ...screenReq('rtmp://gone.example/live/k'), captureSourceId: 'window:65890' }))
  eq('来源消失：开始推流 → LIVE_SOURCE_GONE(kind=window)', [goneErr?.code, goneErr?.kind, goneErr?.detail], ['LIVE_SOURCE_GONE', 'window', 'kind=window'])
  eq('LIVE_SOURCE_GONE 的错误 detail / message 不含窗口标题', /演示文稿|PowerPoint/.test(`${goneErr?.detail}${goneErr?.message}`), false)
  const after = await live.listCaptureSources()
  eq('来源消失：刷新后该窗口不在列表里，其他窗口和屏幕还在', [after.some((x) => x.id === 'window:65890'), after.length], [false, 3])
  win.location.search = ''
  live.resetSimSources()
  // Windows 分组下拉的纯函数：标题中间省略（尾部固定 10 个字符）、选择器形态判断（不靠“列表为空”）
  {
    const { splitSourceTitle, sourcePickerMode } = await import('@/utils/liveSource')
    eq('中间省略：短标题不拆', splitSourceTitle('会议纪要.txt - 记事本'), { head: '会议纪要.txt - 记事本', tail: '' })
    const long = '2026年第三季度经营分析汇报（终稿·第12版）- 演示文稿'
    const sp = splitSourceTitle(long)
    eq('中间省略：长标题尾部保留最后 10 个字符，头 + 尾 = 全名', [Array.from(sp.tail).length, sp.head + sp.tail === long, sp.tail], [10, true, '（终稿·第12版）- 演示文稿'.slice(-10)])
    eq('中间省略：按码点，不劈开 emoji', splitSourceTitle('😀'.repeat(30)).tail, '😀'.repeat(10))
    eq('选择器形态：Windows → 下拉；列表里有窗口 → 下拉；macOS / Linux 仅屏幕 → 单选列表；空列表不改变判断', [sourcePickerMode('windows', []), sourcePickerMode('', [{ kind: 'window' }]), sourcePickerMode('darwin', [{ kind: 'screen' }]), sourcePickerMode('linux', []), sourcePickerMode('', [])], ['dropdown', 'dropdown', 'list', 'list', 'list'])
    win.location.search = '?sim_os=linux&sim_sources=screens'
    eq('模拟 Linux：平台 linux、三块屏、主屏名“屏幕 1（主显示器）”、没有窗口', [(await live.getCaptureCapabilities()).platform, (await live.listCaptureSources()).map((x) => x.title)], ['linux', ['屏幕 1（主显示器）', '屏幕 2', '屏幕 3']])
    win.location.search = ''
    eq('模拟 Windows（默认）：平台 windows；主屏名“屏幕 1（主显示器）”，不再用“（主）”', [(await live.getCaptureCapabilities()).platform, (await live.listCaptureSources())[0].title, JSON.stringify(await live.listCaptureSources()).includes('（主）')], ['windows', '屏幕 1（主显示器）', false])
    win.location.search = '?sim_sources=stale'
    live.resetSimSources()
    await live.listCaptureSources()
    eq('刷新失败保留旧列表：第一次成功，之后刷新失败', (await rejects(live.listCaptureSources()))?.code, 'INTERNAL')
    win.location.search = '?sim_sources=nowin'
    eq('Windows 没有窗口：只有屏幕', (await live.listCaptureSources()).map((x) => x.kind), ['screen', 'screen'])
    win.location.search = ''
    live.resetSimSources()
    // 小修订包 12：开始推流可用性矩阵、没选来源时请求不带 captureSourceId、previewOn 复位、提示条位置
    {
      const { recordStartEnabled } = await import('@/utils/liveSource')
      const base: Parameters<typeof recordStartEnabled>[0] = { blocked: false, starting: false, hasUrl: true, sourceId: 'window:1', state: 'ready', gone: false }
      const en = (o: Partial<typeof base>) => recordStartEnabled({ ...base, ...o })
      eq('开始推流可用：列表正常且已选来源', en({}), true)
      eq('开始推流可用：刷新中（保留旧列表 + 已选项）不置灰', en({ state: 'loading' }), true)
      eq('开始推流可用：刷新失败但旧列表里还有已选项不置灰', en({ state: 'failed' }), true)
      eq('开始推流可用：刷新失败有旧列表（有已选项）→ 按钮可用；另一路：ffmpeg 就绪、地址有、刷新失败（LIVE_SOURCE_STALE）', [en({ state: 'failed', sourceId: 'screen:0' }), en({ state: 'failed', sourceId: 'window:1', gone: false })], [true, true])
      eq('开始推流可用：首次加载中且没有已选项 → 置灰', en({ sourceId: '', state: 'loading' }), false)
      eq('开始推流可用：首次失败 / 空列表且没有已选项 → 可用（不传来源，后端默认推主屏）', [en({ sourceId: '', state: 'failed' }), en({ sourceId: '', state: 'empty' })], [true, true])
      eq('开始推流可用：来源失效（GONE）等待重选 → 置灰', en({ sourceId: '', state: 'ready', gone: true }), false)
      eq('开始推流可用：失效后刷新又失败仍置灰（不悄悄改推主屏）', en({ sourceId: '', state: 'failed', gone: true }), false)
      eq('开始推流可用：ffmpeg 未就绪 / 正在开始 / 地址为空 → 置灰（优先于一切）', [en({ blocked: true }), en({ starting: true }), en({ hasUrl: false }), en({ blocked: true, sourceId: '', state: 'failed' })], [false, false, false, false])
      const noSrc = live.buildScreenPushRequest({ url: 'rtmp://main.example/live/m', sourceId: '', archiveDir: '', preview: true })
      eq('没选来源：请求里不带 captureSourceId（连键都没有），screenId 为空 = 主显示器', ['captureSourceId' in noSrc, JSON.stringify(noSrc).includes('captureSourceId'), noSrc.screenId], [false, false, ''])
      const withSrc = live.buildScreenPushRequest({ url: 'rtmp://main.example/live/m2', sourceId: 'window:131426', archiveDir: '', preview: false })
      eq('选了来源：原样带 captureSourceId，preview 原样带', [withSrc.captureSourceId, withSrc.preview], ['window:131426', false])
      // previewOn 复位（组件不能在 node 里挂载，按源码断言）：开始成功后复位为开；拉流在播放结束（busy 变 false）后复位
      const fsx = await import('node:fs')
      const rd = (f: string) => fsx.readFileSync(`${process.cwd()}/${f}`, 'utf8')
      eq('没选来源：触发器显示“屏幕 1（主显示器）”，表单里不再有“未选择来源，将推送主屏”', [(await import('@/errors/errorMessages')).LIVE_SOURCE_DEFAULT_MAIN_NAME, 'defaultMainScreenHint' in (await import('@/utils/liveSource')), /LIVE_SOURCE_DEFAULT_MAIN_NAME/.test(rd('src/components/live/CaptureSourcePicker.vue')), /未选择来源|推送主屏|DEFAULT_MAIN_HINT/.test(rd('src/views/live/RecordPush.vue') + rd('src/errors/errorMessages.ts'))], ['屏幕 1（主显示器）', false, true, false])
      eq('触发器主屏名的条件：没选来源、非失效、非首次加载中才显示（源码）', /const showDefaultMain = computed\(\(\) => !current\.value && !props\.gone && props\.state !== 'loading'\)/.test(rd('src/components/live/CaptureSourcePicker.vue')), true)
      // 包 20：开始成功后推流码不再清空（表单原样保留 / 重启恢复），预览开关照旧复位为开
      // 包 21（契约 v0.25 ⑤）：开关在推流中途也能拨，只连接 / 断开播放器；和当前会话双向同步，没有进行中的会话时复位为开
      const dockSrc = rd('src/stores/liveDock.ts')
      eq('包 21 预览开关：推流中不置灰；同步当前会话；无会话复位为开', [
        /<PreviewSwitch v-model="previewOn" \/>/.test(rd('src/views/live/FilePush.vue')), /<PreviewSwitch v-model="previewOn" \/>/.test(rd('src/views/live/RecordPush.vue')),
        /sessions\.setPreview\(c\.id, on\)/.test(dockSrc), /watch\(\(\) => sessions\.busyCount, \(n\) => \{ if \(n === 0\) previewOn\.value = true \}\)/.test(dockSrc),
      ], [true, true, true, true])
      const panelSrc = rd('src/components/live/LiveSessionPanel.vue')
      eq('包 21 会话面板：只列进行中的；每一行的预览开关可以拨（不再 aria-disabled）', [/v-for="r in store\.activeRows"/.test(panelSrc), /@click="store\.setPreview\(r\.id, r\.preview === false\)"/.test(panelSrc), /aria-disabled/.test(panelSrc)], [true, true, false])
      const entrySrc = rd('src/components/live/LiveSessionEntry.vue')
      eq('包 21 角标只数进行中的', /store\.busyCount/.test(entrySrc), true)
      eq('包 21 角标：0 路时整个不显示（推流 / 录屏 / 拉流共用一个入口）', [/<i v-if="badge" class="badge"/.test(entrySrc), /sessionBadge\(count\.value\)/.test(entrySrc)], [true, true])
      eq('包 20：开始成功后不再清空推流码', [/key\.value = ''/.test(rd('src/views/live/FilePush.vue')), /key\.value = ''/.test(rd('src/views/live/RecordPush.vue'))], [false, false])
      const pullSrc = rd('src/views/live/PullPlay.vue')
      eq('包 21：拉流页用实时播放器，不再轮询预览图、不再写 HTTP-FLV', [/usePreviewPoller|PreviewStage|getPreview\b|HTTP-FLV/.test(pullSrc), /<LivePlayer/.test(pullSrc), /LP_PULL_HINT/.test(pullSrc)], [false, true, true])
      // 回退提示条：Tab 条下方通栏（LiveLayout），不再在推流页左列里
      const lay = rd('src/views/live/LiveLayout.vue')
      eq('回退提示条在 LiveLayout 的 Tab 条（nav）之后、RouterView 之前；两个推流页里不再有', [lay.indexOf('</nav>') < lay.indexOf('<LiveFallbackNotice') && lay.indexOf('<LiveFallbackNotice') < lay.indexOf('<RouterView'), rd('src/views/live/FilePush.vue').includes('LiveFallbackNotice'), rd('src/views/live/RecordPush.vue').includes('LiveFallbackNotice')], [true, false, false])
      // 预览舞台网格轨道必须 minmax(0,1fr)，否则图片撑开轨道被裁（走查 S1）
      eq('播放器舞台：grid-template 用 minmax(0,1fr)', /grid-template:\s*minmax\(0,\s*1fr\)\s*\/\s*minmax\(0,\s*1fr\)/.test(rd('src/components/live/LivePlayer.vue')), true)
    }
  }
  // 表单错误映射：LIVE_SOURCE_GONE 显示在来源选择器下方（where=source），窗口 / 屏幕文案，INVALID_ARGUMENT 沿用通用文案
  eq('表单错误：窗口消失', pushErrorToForm(new AppError('LIVE_SOURCE_GONE', 'm', 'kind=window')), { where: 'source', text: '所选窗口已不可用，请重新选择' })
  eq('表单错误：屏幕消失', pushErrorToForm(new AppError('LIVE_SOURCE_GONE', 'm', 'kind=screen')), { where: 'source', text: '所选屏幕已不可用，请重新选择' })
  eq('表单错误：缺 kind → 窗口版', pushErrorToForm(new AppError('LIVE_SOURCE_GONE', 'm')).text, '所选窗口已不可用，请重新选择')
  eq('表单错误：INVALID_ARGUMENT 沿用通用（form 级，不指向来源）', pushErrorToForm(new AppError('INVALID_ARGUMENT', 'captureSourceId 格式不对')).where, 'form')
  const gt = await live.startScreenPush({ ...screenReq('rtmp://g2.example/live/g2'), captureSourceId: 'window:131426' })
  eq('窗口来源的任务标题与 params', [gt.title.includes('记事本'), JSON.parse(gt.params).captureSourceId], [true, 'window:131426'])
  await live.stopPush(gt.id)
  await new Promise((r) => setTimeout(r, 1700)) // 模拟层停止需要一小会儿，之后才能再开屏幕推流
  {
    const noSrc = live.buildScreenPushRequest({ url: 'rtmp://main.example/live/m', sourceId: '', archiveDir: '', preview: true })
    const mt = await live.startScreenPush(noSrc)
    eq('没选来源也能开始（模拟层默认推主屏），params 里没有 captureSourceId', [mt.title.includes('屏幕 1'), JSON.parse(mt.params).captureSourceId], [true, undefined])
    await live.stopPush(mt.id)
  }
  await new Promise((r) => setTimeout(r, 1700)) // 模拟层停止需要一小会儿，之后才能再开屏幕推流
  await live.startScreenPush(screenReq('rtmp://s1.example/live/sk1'))
  err = await rejects(live.startScreenPush(screenReq('rtmp://s2.example/live/sk2')))
  eq('screen_busy', [err?.code, err?.reason], ['TASK_CONFLICT', 'screen_busy'])
  eq('screen_busy 文案', taskConflictText(err?.reason), '屏幕推流同一时间只能有 1 路，请先停止当前的屏幕推流')
  eq('screen_busy 的 detail 不带地址', (err?.detail ?? '').includes('s2.example') || (err?.detail ?? '').includes('sk2'), false)
  err = await rejects(live.startScreenPush(screenReq('rtmp://s1.example/live/sk1')))
  eq('同地址优先 duplicate_url', err?.reason, 'duplicate_url')
  for (let i = 2; i <= 3; i++) await live.startFilePush(req(`rtmp://h${i}.example/live/k${i}`))
  err = await rejects(live.startScreenPush(screenReq('rtmp://s3.example/live/sk3')))
  eq('已有屏幕推流又满 4 路：screen_busy 优先于 max_sessions', err?.reason, 'screen_busy')
  err = await rejects(live.startFilePush(req('rtmp://h5.example/live/k5')))
  eq('max_sessions', [err?.code, err?.reason], ['TASK_CONFLICT', 'max_sessions'])
  eq('max_sessions 文案', taskConflictText(err?.reason), '最多同时推 4 路')
  err = await rejects(live.startFilePush(req('rtsp://h/live')))
  eq('协议不支持 → LIVE_URL_INVALID', err?.code, 'LIVE_URL_INVALID')
  eq('LIVE_URL_INVALID 的 detail 首行 reason，第二行脱敏地址', [err?.reason, err?.detail], ['scheme_unsupported', 'reason=scheme_unsupported\nrtsp://h/live'])
  // 预览参数触发：未知 reason / 缺 reason
  win.location.search = '?sim_err=TASK_CONFLICT&sim_reason=unknown'
  eq('预览参数：未知 reason → 通用文案', taskConflictText((await rejects(live.startFilePush(req('rtmp://h9.example/live/k'))))?.reason), TASK_CONFLICT_GENERIC)
  win.location.search = '?sim_err=TASK_CONFLICT'
  eq('预览参数：缺 reason', (await rejects(live.startFilePush(req('rtmp://h9.example/live/k'))))?.reason, undefined)
  win.location.search = '?sim_err=TASK_CONFLICT&sim_reason=max_sessions'
  eq('预览参数：max_sessions', (await rejects(live.startFilePush(req('rtmp://h9.example/live/k'))))?.reason, 'max_sessions')
  win.location.search = '?sim_err=TASK_CONFLICT&sim_reason=duplicate_url'
  eq('预览参数：duplicate_url', (await rejects(live.startFilePush(req('rtmp://h9.example/live/k'))))?.reason, 'duplicate_url')
  win.location.search = ''

  // 停止语义：优雅停止 succeeded 无 error；强杀 canceled 无 error
  const ended: Record<string, TaskStatusPayload> = {}
  const off = onSimEvent<TaskStatusPayload>('task:status', (p) => {
    if (['succeeded', 'failed', 'canceled'].includes(p.status)) ended[p.id] = p
  })
  const connected = new Set<string>()
  const offP = onSimEvent<TaskProgressPayload>('task:progress', (p) => connected.add(p.id))
  await sleep(1700)
  eq('第一条 progress 之后才算已连接', connected.has(first.id), true)
  await live.stopPush(first.id)
  await live.stopPush(first.id) // 正在停止中重复点击：无操作
  await sleep(900)
  eq('优雅停止 → succeeded 且没有 error', [ended[first.id]?.status, ended[first.id]?.error], ['succeeded', undefined])
  eq('已结束再 Cancel → TASK_CONFLICT 且不带 reason', [(await rejects(live.stopPush(first.id)))?.code, (await rejects(live.stopPush(first.id)))?.reason], ['TASK_CONFLICT', undefined])
  win.location.search = '?sim_kill=1'
  const killed = await live.startFilePush(req('rtmp://hk.example/live/kk'))
  win.location.search = ''
  await sleep(1700)
  await live.stopPush(killed.id)
  await sleep(1800)
  eq('强杀 → canceled 且没有 error', [ended[killed.id]?.status, ended[killed.id]?.error], ['canceled', undefined])
  eq('Retry 直播任务 UNSUPPORTED', (await rejects(Promise.resolve().then(() => retrySimTask(first.id))))?.code, 'UNSUPPORTED')
  for (const r of await live.listRunning()) await live.stopPush(r.streamId).catch(() => undefined)
  off()
  offP()

  // ---- 架构师决定 3：LIVE_CONNECT_FAILED 的 scheme= / LIVE_URL_INVALID 的 reason= ----
  eq('scheme 首行解析', [new AppError('LIVE_CONNECT_FAILED', 'x', 'scheme=srt\nConnection failed').scheme, new AppError('LIVE_CONNECT_FAILED', 'x', 'scheme=rtmps').scheme], ['srt', 'rtmps'])
  eq('scheme 不在首行不算', new AppError('LIVE_CONNECT_FAILED', 'x', 'Connection failed\nscheme=srt').scheme, undefined)
  eq('SRT 文案', liveConnectFailedText('srt'), '连接失败，请检查地址和口令是否正确')
  const RTMP_TEXT = '连接失败，请检查推流地址和推流码是否正确，以及网络是否通畅'
  eq('RTMP / RTMPS 文案', [liveConnectFailedText('rtmp'), liveConnectFailedText('rtmps')], [RTMP_TEXT, RTMP_TEXT])
  eq('未知 / 缺 scheme 用 RTMP 那句', [liveConnectFailedText('quic'), liveConnectFailedText(undefined), liveConnectFailedText(null), liveConnectFailedText(''), liveConnectFailedText('toString')], [RTMP_TEXT, RTMP_TEXT, RTMP_TEXT, RTMP_TEXT, RTMP_TEXT])
  eq('detail 的 scheme 优先于兜底', liveFailureMessage({ code: 'LIVE_CONNECT_FAILED', message: 'm', detail: 'scheme=rtmp\nx' }, 'srt'), LIVE_RTMP_CONNECT_FAILED_TEXT)
  eq('detail 没有 scheme 时用脱敏 params 的 scheme 兜底', liveFailureMessage({ code: 'LIVE_CONNECT_FAILED', message: 'm', detail: 'boom' }, schemeFromParams('{"url":"srt://h:9000?streamid=***"}')), LIVE_SRT_CONNECT_FAILED_TEXT)
  eq('scheme、兜底都没有 → RTMP 那句', liveFailureMessage({ code: 'LIVE_CONNECT_FAILED', message: '无法连接推流目标' }), RTMP_TEXT)
  eq('LIVE_PUSH_REJECTED 文案', liveFailureMessage({ code: 'LIVE_PUSH_REJECTED', message: '被拒绝', detail: 'scheme=srt' }), '服务器拒绝了推流，请检查推流码是否有效，或是否已被其他推流占用')
  eq('LIVE_PUSH_REJECTED 表内文案', [errorMessages.LIVE_PUSH_REJECTED.description, LIVE_PUSH_REJECTED_TEXT], [LIVE_PUSH_REJECTED_TEXT, LIVE_PUSH_REJECTED_TEXT])
  eq('LIVE_CONNECT_FAILED 表内文案与遮罩', [errorMessages.LIVE_CONNECT_FAILED.description, resolveError('LIVE_CONNECT_FAILED', LIVE_SRT_CONNECT_FAILED_TEXT).description, resolveTaskError('LIVE_CONNECT_FAILED', LIVE_SRT_CONNECT_FAILED_TEXT).description], [RTMP_TEXT, LIVE_SRT_CONNECT_FAILED_TEXT, LIVE_SRT_CONNECT_FAILED_TEXT])
  eq('liveStartErrorLine 用 e.scheme', liveStartErrorLine({ code: 'LIVE_CONNECT_FAILED', scheme: 'rtmp' }, { scheme: 'srt' })?.description, LIVE_RTMP_CONNECT_FAILED_TEXT)
  eq('scheme_unsupported 文案', liveUrlInvalidText('scheme_unsupported'), '暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt')
  eq('malformed 文案', liveUrlInvalidText('malformed'), '推流地址格式不正确，请检查后重新输入')
  eq('missing_host 文案', liveUrlInvalidText('missing_host'), '推流地址里缺少服务器地址，请检查后重新输入')
  eq('param_not_allowed 文案', liveUrlInvalidText('param_not_allowed'), '推流地址里有不支持的参数，请去掉后重试')
  eq('LIVE_URL_INVALID 行内表文案 = 通用文案', errorMessages.LIVE_URL_INVALID.description, '推流地址不可用，请检查后重新输入')
  eq('LIVE_URL_INVALID_GENERIC 文案', LIVE_URL_INVALID_GENERIC, '推流地址不可用，请检查后重新输入')
  eq('未知 reason → 推流地址不可用', [liveUrlInvalidText('future_reason'), liveUrlInvalidText(undefined), liveUrlInvalidText('toString')], [LIVE_URL_INVALID_GENERIC, LIVE_URL_INVALID_GENERIC, LIVE_URL_INVALID_GENERIC])
  eq('reason 表只有四个取值', Object.keys(LIVE_URL_INVALID_REASON_TEXT).sort(), ['malformed', 'missing_host', 'param_not_allowed', 'scheme_unsupported'])
  // 模拟层：LIVE_URL_INVALID 各取值、注入的未知 / 缺失
  const urlReason = async (url: string) => (await rejects(live.startFilePush(req(url))))?.reason
  eq('模拟：scheme_unsupported', await urlReason('rtsp://h/live'), 'scheme_unsupported')
  eq('模拟：malformed', await urlReason('not a url'), 'malformed')
  eq('模拟：missing_host', await urlReason('rtmp:///live/k'), 'missing_host')
  eq('模拟：param_not_allowed', await urlReason('srt://h:9000?mode=listener'), 'param_not_allowed')
  eq('CheckPushURL 同样带 reason', (await rejects(live.checkPushURL('rtsp://h/x')))?.reason, 'scheme_unsupported')
  for (const [want, sim] of [['scheme_unsupported', 'scheme_unsupported'], ['malformed', 'malformed'], ['missing_host', 'missing_host'], ['param_not_allowed', 'param_not_allowed']] as const) {
    win.location.search = `?sim_err=LIVE_URL_INVALID&sim_reason=${sim}`
    eq(`预览参数：LIVE_URL_INVALID reason=${want}`, (await rejects(live.startFilePush(req('rtmp://h9.example/live/k'))))?.reason, want)
  }
  win.location.search = '?sim_err=LIVE_URL_INVALID&sim_reason=unknown'
  eq('预览参数：未知 reason → 通用文案', liveUrlInvalidText((await rejects(live.startFilePush(req('rtmp://h9.example/live/k'))))?.reason), LIVE_URL_INVALID_GENERIC)
  win.location.search = '?sim_err=LIVE_URL_INVALID'
  eq('预览参数：缺 reason → 通用文案', liveUrlInvalidText((await rejects(live.startFilePush(req('rtmp://h9.example/live/k'))))?.reason), LIVE_URL_INVALID_GENERIC)
  win.location.search = ''
  // 模拟层：连接失败任务的 detail 首行 scheme=（rtmp / srt），?sim_scheme=missing 缺首行
  const connFail = async (url: string, extra = ''): Promise<AppError | null> => {
    win.location.search = `?sim_err=LIVE_CONNECT_FAILED${extra}`
    const t = await live.startFilePush(req(url))
    win.location.search = ''
    const done = new Promise<TaskStatusPayload>((resolve) => {
      const stop = onSimEvent<TaskStatusPayload>('task:status', (p) => {
        if (p.id === t.id && p.status === 'failed') { stop(); resolve(p) }
      })
    })
    const p = await done
    return p.error ? new AppError(p.error.code as AppError['code'], p.error.message, p.error.detail) : null
  }
  const cSrt = await connFail('srt://hc1.example:9000?streamid=a')
  eq('模拟：SRT 连接失败 scheme=srt', [cSrt?.code, cSrt?.scheme], ['LIVE_CONNECT_FAILED', 'srt'])
  const cRtmp = await connFail('rtmp://hc2.example/live/k')
  eq('模拟：RTMP 连接失败 scheme=rtmp', [cRtmp?.code, cRtmp?.scheme], ['LIVE_CONNECT_FAILED', 'rtmp'])
  const cRtmps = await connFail('rtmps://hc3.example/live/k')
  eq('模拟：RTMPS 连接失败 scheme=rtmps', cRtmps?.scheme, 'rtmps')
  const cNone = await connFail('rtmp://hc4.example/live/k', '&sim_scheme=missing')
  eq('模拟：缺 scheme 首行', cNone?.scheme, undefined)
  eq('连接失败 detail 不带完整地址', [cSrt, cRtmp].every((e) => !(e?.detail ?? '').includes('streamid=a') && !(e?.detail ?? '').includes('/live/k')), true)

  // ---- 类型守卫 / 绑定查找 ----
  eq('toApiTask 容忍脏数据', ((t: ApiTask) => [t.id, t.progress, t.inputPaths, t.error])(toApiTask({ id: 't', inputPaths: ['a', 1], error: { code: '' } })), ['t', 0, ['a'], null])
  eq('toApiTask 保留编码器字段（v0.18）', ((t: ApiTask) => [t.encoder, t.encoderDevice, t.hwFallback, t.hwFallbackReason])(toApiTask({ id: 't', encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, hwFallbackReason: 'nvenc_init_failed' })), ['libx264', 'cpu', true, 'nvenc_init_failed'])
  eq('toApiTask 没有编码器字段时缺省', ((t: ApiTask) => [t.encoder, t.encoderDevice, t.hwFallback, t.hwFallbackReason])(toApiTask({ id: 't' })), [undefined, undefined, undefined, undefined])
  eq('toApiTask 保留直播字段', ((t: ApiTask) => [t.fps, t.bitrateKbps, t.droppedFrames])(toApiTask({ id: 't', fps: 30, bitrateKbps: 2500, droppedFrames: 0 })), [30, 2500, 0])
  eq('toApiTask null → 空任务不抛错', toApiTask(null).id, '')
  eq('绑定不存在 → UNSUPPORTED', (await rejects(callService('LiveService', 'Nope')))?.code, 'UNSUPPORTED')

  // ---- Doc ----
  const caps = await doc.getDocCapabilities()
  eq('模拟层 experimental 默认 true', doc.isExperimental(caps), true)
  eq('后端给 false 优先', doc.isExperimental({ ...caps, experimental: false }), false)
  eq('老后端没有字段 → true', doc.isExperimental({ ...caps, experimental: undefined }), true)
  for (const f of ['/d/a.doc', '/d/b.xls', '/d/c.ppt', '/d/加密.docx', '/d/x.csv', '/d/y.txt']) {
    err = await rejects(doc.convertToPDF([f], ''))
    eq(`UNSUPPORTED ${f}`, [err?.code, err?.reason && docErrorPath(err.detail)], ['UNSUPPORTED', f]) // 契约 v0.16：首行 reason=…，第二行才是出错文件路径
  }
  const good = await doc.convertToPDF(['/d/a.docx', '/d/b.pptx'], '')
  eq('ConvertToPDF 返回与 inputs 一一对应', good.map((t) => t.type), ['office_pdf', 'office_pdf'])
  err = await rejects(doc.convertToPDF(['/d/a.docx', '/d/b.doc'], ''))
  eq('整体校验：一个不通过整体失败', err?.code, 'UNSUPPORTED')
  const src = await doc.openPDF('/d/a.pdf')
  const whole = await doc.readWholePDF(src)
  eq('分块读取拼出整份', [whole.length, new TextDecoder().decode(whole.subarray(0, 5))], [src.size, '%PDF-'])
  eq('length 越界 INVALID_ARGUMENT', (await rejects(doc.readPDFChunk(src.id, 0, 0)))?.code, 'INVALID_ARGUMENT')
  eq('句柄不存在 NOT_FOUND', (await rejects(doc.readPDFChunk('nope', 0, 10)))?.code, 'NOT_FOUND')
  eq('最近列表', (await doc.listRecentPDFs()).length, 1)
  eq('limit > 200 按 200 处理，不报错', (await doc.listRecentPDFs(100000)).length, 1)
  // base64：标准字母表、含 = 填充；length 是原始字节数，不是 data 的字符数
  const enc = (bytes: number[]) => btoa(String.fromCharCode(...bytes))
  eq('atob 解码含 = 填充', Array.from(doc.decodeChunk({ offset: 0, length: 4, eof: true, size: 4, data: enc([37, 80, 68, 70]) })), [37, 80, 68, 70])
  eq('解码长度以原始字节为准（data 字符数更多）', enc([1, 2, 3, 4]).length > 4 && doc.decodeChunk({ offset: 0, length: 4, eof: true, size: 4, data: enc([1, 2, 3, 4]) }).length === 4, true)
  eq('分块长度不一致 INTERNAL', (() => { try { doc.decodeChunk({ offset: 0, length: 5, eof: true, size: 5, data: enc([1, 2, 3, 4]) }); return '' } catch (e) { return toAppError(e).code } })(), 'INTERNAL')
  eq('模拟大文件不能预览 → UNSUPPORTED', (await rejects(doc.loadPDF('/d/大文件.pdf')))?.code, 'UNSUPPORTED')
  eq('小文件 loadPDF 返回字节', (await doc.loadPDF('/d/b.pdf')).data?.length, 2 * 1024 * 1024)
  // 文档错误文案统一走 errorMessages
  // reason 优先（契约 v0.16）：六个枚举各一条，message 故意写成别的，证明是按 reason 判断的
  const R = (code: string, reason: string, msg = '随便', rest = '') => docErrorText(code, msg, `reason=${reason}${rest ? '\n' + rest : ''}`)
  eq('reason → 文案（六种）', [
    R('UNSUPPORTED', 'too_many_pages'), R('UNSUPPORTED', 'format'), R('UNSUPPORTED', 'encrypted'), R('UNSUPPORTED', 'no_font'),
    R('INVALID_ARGUMENT', 'invalid_ooxml'), R('INVALID_ARGUMENT', 'too_large'),
  ], [DOC_TOO_MANY_PAGES_TEXT, DOC_FORMAT_UNSUPPORTED_TEXT, DOC_ENCRYPTED_TEXT, DOC_NO_FONT_TEXT, DOC_FILE_BROKEN_TEXT, DOC_TOO_LARGE_TEXT])
  eq('reason=format 也可出现在 INVALID_ARGUMENT（OpenPDF 之外的场景不误判）', R('INVALID_ARGUMENT', 'format'), DOC_FORMAT_UNSUPPORTED_TEXT)
  eq('reason 优先于 message（message 与 reason 不一致时听 reason）', docErrorText('INVALID_ARGUMENT', '不是有效的 OOXML 文件', 'reason=too_large\n/d/a.docx\n压缩包条目数超过 100000'), DOC_TOO_LARGE_TEXT)
  eq('带路径行（第二行）的整体校验错误', docErrorText('UNSUPPORTED', '暂不支持这种格式', 'reason=format\n/d/a.doc\n.doc：旧版'), DOC_FORMAT_UNSUPPORTED_TEXT)
  eq('未知 reason（不在枚举里）当作没有 reason，走 message 兜底 / 后端 message', [docErrorText('UNSUPPORTED', '暂不支持这种格式', 'reason=future'), docErrorText('UNSUPPORTED', '别的', 'reason=future')], [DOC_FORMAT_UNSUPPORTED_TEXT, '别的'])
  eq('兜底：reason 缺失时对 message 精确相等（老后端）', [
    docErrorText('UNSUPPORTED', '暂不支持这种格式', '.doc：旧版'), docErrorText('UNSUPPORTED', '超过 5000 页', '已排到第 5000 页仍未结束'), docErrorText('UNSUPPORTED', '没有可用的 Unicode 字体'),
    docErrorText('INVALID_ARGUMENT', '不是有效的 OOXML 文件', 'bad zip'), docErrorText('INVALID_ARGUMENT', '文件超过 100 MiB', '104857601 字节'),
  ], [DOC_FORMAT_UNSUPPORTED_TEXT, DOC_TOO_MANY_PAGES_TEXT, DOC_NO_FONT_TEXT, DOC_FILE_BROKEN_TEXT, DOC_TOO_LARGE_TEXT])
  eq('兜底不做包含匹配：detail 里写着 5000 页 / OOXML 也不算', [
    docErrorText('UNSUPPORTED', '别的', '文本里写着 5000 页'), docErrorText('INVALID_ARGUMENT', '路径不合法', 'OOXML'),
  ], ['别的', '路径不合法'])
  eq('没有 reason 的错误保持原有处理', [
    docErrorText('INVALID_ARGUMENT', '路径不合法', '/d/a.docx'), docErrorText('INVALID_ARGUMENT', '一次最多提交 50 个文件'), docErrorText('NOT_FOUND', '文件不存在', '/d/a.docx\n文件不存在'),
    docErrorText('CONVERT_DISK_FULL', '磁盘空间不足，无法写入输出文件'), docErrorText('IO_ERROR', '读取文件失败'), docErrorText('CANCELED', '已取消'), docErrorText('INTERNAL', '内部错误', 'fpdf: x'),
  ], ['路径不合法', '一次最多提交 50 个文件', '找不到这个文件，可能已被移动或删除', taskErrorMessages.CONVERT_DISK_FULL.description, '没有读取这个文件的权限。', '操作已取消。', '内部错误'])
  eq('页数上限取 limits 拼', docErrorText('UNSUPPORTED', '超过 5000 页', 'reason=too_many_pages', 8000), '文档太长，超过 8000 页，无法转换')
  // 出错文件路径：reason 行在首行，路径在第二行；不把 reason= 行当路径；兼容旧形态（路径在第一行）
  eq('出错文件路径：前两行里找，reason= 行不算路径', [
    docErrorPath('reason=format\n/d/a.doc\n.doc：旧版'), docErrorPath('reason=invalid_ooxml\nC:\\d\\a.xlsx\n缺少 xl/workbook.xml'), docErrorPath('/d/a.doc\n旧版'),
    docErrorPath('reason=too_many_pages\n已排到第 5000 页仍未结束'), docErrorPath('reason=format'), docErrorPath('旧版'), docErrorPath(undefined),
    docErrorPath('reason=format\n说明\n/d/第三行不算.doc'),
  ], ['/d/a.doc', 'C:\\d\\a.xlsx', '/d/a.doc', '', '', '', '', ''])
  eq('出错文件名', [docErrorFile('UNSUPPORTED', 'reason=format\n/d/a.doc\n旧版'), docErrorFile('UNSUPPORTED', 'reason=format\n旧版'), docErrorFile('UNSUPPORTED', '/d/a.doc\n旧版')], ['a.doc', '', 'a.doc'])
  eq('detail 首行 reason 解析 / 枚举过滤', [docReasonOf('reason=format\n/d/a.doc'), docReasonOf('reason=future'), docReasonOf('/d/a.doc'), docReasonOf(undefined)], ['format', undefined, undefined, undefined])
  eq('IO_ERROR 读源文件', docErrorText('IO_ERROR', '读取文件失败'), '没有读取这个文件的权限。')
  const pv = (c: string, m?: string, d?: string) => pdfErrorView(c, m, undefined, d)
  eq('PDF 预览失败卡片：文案 / 错误码 / 是否可重试', [
    pv('PDF_PARSE_FAILED'), pv('INVALID_ARGUMENT', '不是 PDF 文件', 'reason=format\n/d/a.pdf'), pv('INVALID_ARGUMENT', '文件超过 512 MiB', 'reason=too_large\n600000000 字节'), pv('NOT_FOUND', '文件不存在'),
    pv('IO_ERROR', '读取文件失败'), pv('IO_ERROR', '文件在读取时被替换，请重试'), pv('INVALID_ARGUMENT', '别的原因'),
  ].map((v) => [v.text, v.code, v.retry]), [
    ['PDF 内容无法解析，文件可能已损坏。', 'PDF_PARSE_FAILED', true],
    ['这不是有效的 PDF 文件。', 'INVALID_ARGUMENT', false],
    ['文件超过 512 MiB，暂不支持预览。', 'INVALID_ARGUMENT', false],
    ['找不到这个文件，可能已被移动或删除。', 'NOT_FOUND', false],
    ['没有读取这个文件的权限。', 'IO_ERROR', true],
    ['读取时文件被修改了，请重试。', 'IO_ERROR', true],
    ['别的原因', 'INVALID_ARGUMENT', false],
  ])
  eq('512 MiB 取 limits 拼', pdfErrorView('INVALID_ARGUMENT', '文件超过 512 MiB', 256 * 1024 * 1024, 'reason=too_large').text, '文件超过 256 MiB，暂不支持预览。')
  eq('PDF：reason 优先于 message；扩展名不对（reason=format）也算不是 PDF；reason 缺失按 message 兜底', [
    pdfErrorView('INVALID_ARGUMENT', '只支持 .pdf 文件', undefined, 'reason=format').text, pdfErrorView('INVALID_ARGUMENT', '随便', undefined, 'reason=too_large').text,
    pdfErrorView('INVALID_ARGUMENT', '不是 PDF 文件', undefined, '/d/a.pdf').text, pdfErrorView('INVALID_ARGUMENT', 'PDF 路径必须是绝对路径').text,
  ], ['这不是有效的 PDF 文件。', '文件超过 512 MiB，暂不支持预览。', '这不是有效的 PDF 文件。', 'PDF 路径必须是绝对路径'])
  eq('模拟层的 Doc 错误形态与契约 v0.16 一致（reason 首行、路径第二行）', await (async () => {
    const one = async (name: string) => { const e = await rejects(doc.convertToPDF([`/d/${name}`], '')); return [e?.code, e?.reason, docErrorPath(e?.detail)] }
    return [await one('旧版.doc'), await one('加密.docx'), await one('损坏.docx'), await one('超大.docx')]
  })(), [['UNSUPPORTED', 'format', '/d/旧版.doc'], ['UNSUPPORTED', 'encrypted', '/d/加密.docx'], ['INVALID_ARGUMENT', 'invalid_ooxml', '/d/损坏.docx'], ['INVALID_ARGUMENT', 'too_large', '/d/超大.docx']])
  eq('中间省略拆分：≤14 字符不拆；否则尾部 = 末 6 字符 + 扩展名', [
    splitMiddle('用户调研报告.docx'),
    splitMiddle('2026年第三季度华东区域渠道商务拓展与用户增长复盘汇报材料（终稿-已审阅-v12）.pptx'),
    splitMiddle('ab'),
  ], [{ head: '用户调研报告.docx', tail: '' }, { head: '2026年第三季度华东区域渠道商务拓展与用户增长复盘汇报材料（终稿-已审', tail: '阅-v12）.pptx' }, { head: 'ab', tail: '' }])
  eq('侧栏 ffmpeg 状态：文案 / aria-label / 可点性', (['ready', 'checking', 'missing', 'outdated', 'failed', 'installing'] as const).map((k) => { const v = ffmpegStatusView(k); return [v.tone, v.text, v.label, v.actionLabel, v.clickable] }), [
    ['ok', '转换组件已就绪', '转换组件已就绪', '转换组件已就绪', false],
    ['q', '转换组件检测中…', '转换组件检测中…', '转换组件检测中…', false],
    ['warn', '转换组件未就绪', '转换组件未就绪', '转换组件未就绪，点击打开安装对话框', true],
    ['warn', '转换组件未就绪', '转换组件未就绪', '转换组件未就绪，点击打开安装对话框', true],
    ['warn', '转换组件未就绪', '转换组件未就绪', '转换组件未就绪，点击打开安装对话框', true],
    ['run', '转换组件安装中…', '转换组件安装中，点击查看进度', '转换组件安装中，点击查看进度', true],
  ])
  eq('缩放档位 50%~200% 步进 25%，两端夹住', [nextZoom(1, -1), nextZoom(0.5, -1), nextZoom(2, 1), nextZoom(1.75, 1), nextZoom(1.3, 1)], [0.75, 0.5, 2, 2, 1.25])
  eq('缩略图窗口：可视范围 ± 2 屏', [thumbWindow(0, 600, 100, 5000), thumbWindow(50000, 600, 100, 5000), thumbWindow(0, 600, 100, 0)], [{ from: 1, to: 18 }, { from: 489, to: 518 }, { from: 1, to: 0 }])
  const NOW = new Date(2026, 8, 30, 12, 0).getTime()
  const at = (d: number, h: number, m: number) => new Date(2026, 8, 30 + d, h, m).getTime()
  eq('最近列表时间：今天 / 昨天 / 更早', [formatRecentTime(at(0, 22, 41), NOW), formatRecentTime(at(-1, 18, 5), NOW), formatRecentTime(at(-5, 16, 40), NOW)], ['今天 22:41', '昨天 18:05', '09-25 16:40'])
  eq('扩展名色块', [extBadge('/a/b.docx'), extBadge('C:\\a\\b.XLSX'), extBadge('/a/noext')], ['DOCX', 'XLSX', ''])
  // ---- 产品定稿：SRT 口令长度 / 缺协议 / 不泄露地址口令推流码 ----
  eq('LIVE_SRT_PASSPHRASE_TEXT', LIVE_SRT_PASSPHRASE_TEXT, 'SRT 口令需要 10 到 79 个字符')
  eq('口令长度边界', ['', 'a'.repeat(9), 'a'.repeat(10), 'a'.repeat(79), 'a'.repeat(80)].map(live.isValidSrtPassphrase), [true, false, true, true, false])
  eq('口令按字符数（中文 10 个）', live.isValidSrtPassphrase('口令口令口令口令口令'), true)
  eq('取地址里的 passphrase', [live.srtPassphraseFromUrl('srt://h:9000?streamid=a&passphrase=abc%20def'), live.srtPassphraseFromUrl('srt://h:9000?streamid=a'), live.srtPassphraseFromUrl('rtmp://h/live/k?passphrase=x')], ['abc def', undefined, undefined])
  const shortPass = 'short9xxx'
  const shortUrl = `srt://sp.example:9000?streamid=sidsecret&passphrase=${shortPass}`
  err = await rejects(live.startFilePush(req(shortUrl)))
  eq('口令太短：前端先拦（INVALID_ARGUMENT，产品文案）', [err?.code, err?.message], ['INVALID_ARGUMENT', 'SRT 口令需要 10 到 79 个字符'])
  eq('口令太短：没有创建任务', (await live.listRunning()).length, 0)
  err = await rejects(live.startScreenPush(screenReq(shortUrl)))
  eq('屏幕推流口令太短同样先拦', [err?.code, err?.message], ['INVALID_ARGUMENT', 'SRT 口令需要 10 到 79 个字符'])
  eq('口令太长先拦', (await rejects(live.startFilePush(req(`srt://sp.example:9000?passphrase=${'a'.repeat(80)}`))))?.code, 'INVALID_ARGUMENT')
  const okPass = await live.startFilePush(req('srt://sp2.example:9000?streamid=s&passphrase=abcdefghij'))
  eq('口令 10 位放行', okPass.status, 'queued')
  await live.stopPush(okPass.id)
  const PROTO_GENERIC = '当前转换组件不支持这种推流协议。请到设置的“转换组件”里重新安装或更新。'
  const NAME = (n: string) => `当前转换组件不支持 ${n}。请到设置的“转换组件”里重新安装或更新。`
  eq('缺协议（契约 §6.10）：missing=rtmp|rtmps|srt 带协议名', ['missing=rtmp', 'missing=rtmps', 'missing=srt'].map((d) => liveFfmpegProtocolMissingText(d)), [NAME('RTMP'), NAME('RTMPS'), NAME('SRT')])
  eq('缺协议：某一行严格等于即可（CRLF / 多行）', [liveFfmpegProtocolMissingText('missing=srt\r\n'), liveFfmpegProtocolMissingText('x\nmissing=rtmps')], [NAME('SRT'), NAME('RTMPS')])
  eq('缺协议：missing=tee → 通用句（不显示 tee）', liveFfmpegProtocolMissingText('missing=tee'), PROTO_GENERIC)
  eq('缺协议：严格匹配，别的写法一律通用句', [
    'MISSING=SRT', 'missing=SRT', 'missing=Srt', ' missing=srt', 'missing=srt ', 'missing = srt', 'missing=srtx', 'missing=quic', 'missing=', 'missing=srt,rtmp', 'xmissing=srt',
    'missing protocol: SRT', 'protocol=srt', 'ffmpeg 缺少协议：srt', 'Protocol not found: srt', 'reason=missing=srt', 'a missing=srt b',
  ].map((d) => liveFfmpegProtocolMissingText(d)).filter((t) => t !== PROTO_GENERIC), [])
  eq('缺协议：没有 detail → 通用句', [liveFfmpegProtocolMissingText(undefined), liveFfmpegProtocolMissingText(null), liveFfmpegProtocolMissingText('')], [PROTO_GENERIC, PROTO_GENERIC, PROTO_GENERIC])
  eq('缺协议：起始错误行带协议名', liveStartErrorLine({ code: 'UNSUPPORTED', detail: 'missing=srt' })?.description, NAME('SRT'))
  eq('缺协议：起始错误行 missing=tee → 通用句', liveStartErrorLine({ code: 'UNSUPPORTED', detail: 'missing=tee' })?.description, PROTO_GENERIC)
  eq('缺协议：起始错误行写法不严格 → 通用句', liveStartErrorLine({ code: 'UNSUPPORTED', detail: 'ffmpeg 缺少协议：srt' })?.description, PROTO_GENERIC)
  eq('missing= 行判断', [hasMissingLine('missing=tee'), hasMissingLine('missing=srt'), hasMissingLine(undefined), hasMissingLine(''), hasMissingLine('other'), hasMissingLine(' missing=srt')], [true, true, false, false, false, false])
  // 模拟层 ?sim_missing 产出的 detail 就是契约格式：单独一行 missing=<协议名>，没有第二行
  eq('模拟 sim_missing 的 detail 走同一条路径', liveStartErrorLine({ code: 'UNSUPPORTED', detail: 'missing=rtmps' })?.description, NAME('RTMPS'))
  // 所有文案都不含传入的地址 / 口令 / 推流码
  {
    const secretUrl = 'srt://user:pw123@leak.example:9000/live/streamKEY999?streamid=sidLEAK&passphrase=passLEAK1234'
    const rtmpUrl = 'rtmp://leak.example/live/rtmpKEY888'
    const secrets = ['user:pw123', 'pw123', 'leak.example', 'streamKEY999', 'sidLEAK', 'passLEAK1234', 'rtmpKEY888', secretUrl, rtmpUrl, shortPass]
    const outputs: string[] = []
    const detailWith = (head: string, u: string) => `${head}\nConnection to ${u} failed\nmissing protocol: srt at ${u}`
    for (const u of [secretUrl, rtmpUrl, 'rtsp://leak.example/live/rtmpKEY888']) {
      for (const scheme of ['srt', 'rtmp', 'rtmps', 'quic', undefined]) {
        outputs.push(liveConnectFailedText(scheme))
        for (const code of ['LIVE_CONNECT_FAILED', 'LIVE_PUSH_REJECTED']) outputs.push(liveFailureMessage({ code, message: '后端 message', detail: `scheme=${scheme}\n${u}` }, scheme))
        outputs.push(liveStartErrorLine({ code: 'LIVE_CONNECT_FAILED', scheme }, { scheme })?.description ?? '')
      }
      for (const reason of ['scheme_unsupported', 'malformed', 'missing_host', 'param_not_allowed', 'future', undefined]) outputs.push(liveUrlInvalidText(reason))
      for (const reason of ['duplicate_url', 'screen_busy', 'max_sessions', 'future', undefined]) outputs.push(taskConflictText(reason), actionErrorText('TASK_CONFLICT', u, reason))
      outputs.push(liveFfmpegProtocolMissingText(detailWith('missing=srt', u)), liveFfmpegProtocolMissingText(detailWith('missing=tee', u)), liveStartErrorLine({ code: 'UNSUPPORTED', detail: detailWith('missing=srt', u) })?.description ?? '',
        liveFfmpegProtocolMissingText(detailWith('reason=x', u)), liveStartErrorLine({ code: 'UNSUPPORTED', detail: detailWith('x', u) })?.description ?? '')
      outputs.push(LIVE_PUSH_REJECTED_TEXT, LIVE_SRT_PASSPHRASE_TEXT)
      // 模拟层真实产出的错误：URL 非法 / 冲突 / 口令 / 检查地址
      const e1 = await rejects(live.checkPushURL(u.replace('://', ':/')))
      outputs.push(e1?.message ?? '', ...(e1?.detail ? [e1.detail] : []))
    }
    for (const c of Object.keys(errorMessages)) {
      const m = errorMessages[c as keyof typeof errorMessages]
      outputs.push(m.title, m.description)
    }
    for (const w of [shortUrl, 'srt://sp.example:9000?passphrase=short9xxx']) {
      const e2 = await rejects(live.startFilePush(req(w)))
      outputs.push(e2?.message ?? '', e2?.detail ?? '')
    }
    const leaks = outputs.filter((t) => secrets.some((x) => t.includes(x)))
    eq('任何文案输出都不含传入的地址 / 口令 / 推流码', leaks, [])
    eq('自检的输出集合非空', outputs.length > 100, true)
  }
  // ---- 联调：开关 true 时纯浏览器环境（无 window.go）仍走模拟；后端 #47 已放开带存档的屏幕推流 ----
  eq('LIVE_BACKEND_READY 已打开', live.LIVE_BACKEND_READY, true)
  eq('无 window.go → liveIsReal() 为 false（走模拟）', live.liveIsReal(), false)
  for (const t of await live.listRunning()) await live.stopPush(t.streamId).catch(() => undefined)
  await sleep(1500)
  // 带存档不再 UNSUPPORTED：任务 outputPath = 存档路径；UNSUPPORTED 仍只按缺组件（missing=）处理
  const arcTask = await live.startScreenPush({ ...screenReq('rtmp://arc.example/live/arckey'), archiveDir: '/m/arc' })
  eq('带存档屏幕推流不再 UNSUPPORTED，outputPath 在存档目录下', /^\/m\/arc\/screen-\d{8}-\d{6}\.mp4$/.test(arcTask.outputPath), true)
  eq('存档路径不含地址 / 推流码', arcTask.outputPath.includes('arc.example') || arcTask.outputPath.includes('arckey'), false)
  eq('UNSUPPORTED + missing=tee → 仍是缺组件通用句（不带 tee，也不再是存档提示）', liveStartErrorLine({ code: 'UNSUPPORTED', detail: 'missing=tee' })?.description, PROTO_GENERIC)
  eq('UNSUPPORTED + missing=srt → 带协议名', liveStartErrorLine({ code: 'UNSUPPORTED', detail: 'missing=srt' })?.description, '当前转换组件不支持 SRT。请到设置的“转换组件”里重新安装或更新。')
  eq('UNSUPPORTED 无 missing= → 通用句', liveStartErrorLine({ code: 'UNSUPPORTED' })?.description, PROTO_GENERIC)
  const arcProgress: TaskProgressPayload[] = []
  const offA = onSimEvent<TaskProgressPayload>('task:progress', (p) => { if (p.id === arcTask.id) arcProgress.push(p) })
  const arcEnd = new Promise<live.LiveTaskEnd>((resolve) => live.watchLiveTask(arcTask.id, { onEnd: resolve }))
  await sleep(2600)
  eq('带存档的会话 task:progress 不带 bitrateKbps', [arcProgress.length > 0, arcProgress.every((p) => p.bitrateKbps === undefined)], [true, true])
  await live.stopPush(arcTask.id)
  const arcDone = await arcEnd
  offA()
  eq('带存档优雅停止 → succeeded，outputPath 保留、无 error', [arcDone.status, arcDone.outputPath, arcDone.error], ['succeeded', arcTask.outputPath, null])
  // 强杀且存档保留：canceled + outputPath 非空；没等到第一条 progress 就强杀（空壳）→ outputPath 为空
  win.location.search = '?sim_kill=1'
  const arcKill = await live.startScreenPush({ ...screenReq('rtmp://arc2.example/live/k2'), archiveDir: '/m/arc' })
  win.location.search = ''
  const killEnd = new Promise<live.LiveTaskEnd>((resolve) => live.watchLiveTask(arcKill.id, { onEnd: resolve }))
  await sleep(3400) // 300ms 启动 + 连接中 1200ms（1 秒一跳 → 约 2.3 秒后第一条 progress）
  await live.stopPush(arcKill.id)
  const killed2 = await killEnd
  eq('强杀且存档保留 → canceled + outputPath 非空（页面显示“已强制停止，存档已保留，文件可能不完整”）', [killed2.status, killed2.outputPath === arcKill.outputPath, killed2.error], ['canceled', true, null])
  win.location.search = '?sim_kill=1'
  const noArcKill = await live.startScreenPush(screenReq('rtmp://arc3.example/live/k3'))
  win.location.search = ''
  const noArcEnd = new Promise<live.LiveTaskEnd>((resolve) => live.watchLiveTask(noArcKill.id, { onEnd: resolve }))
  await sleep(1700)
  await live.stopPush(noArcKill.id)
  eq('无存档强杀 → canceled + outputPath 为空', (await noArcEnd).outputPath, '')
  win.location.search = '?sim_kill=1'
  const emptyShell = await live.startScreenPush({ ...screenReq('rtmp://arc4.example/live/k4'), archiveDir: '/m/arc' })
  win.location.search = ''
  const shellEnd = new Promise<live.LiveTaskEnd>((resolve) => live.watchLiveTask(emptyShell.id, { onEnd: resolve }))
  await sleep(400)
  await live.stopPush(emptyShell.id)
  await sleep(200)
  await live.forceStopPush(emptyShell.id)
  const shell = await shellEnd
  eq('空壳存档（还没推出内容就强制停止）→ canceled + outputPath 清空 → 只显示“已强制停止”', [shell.status, shell.outputPath], ['canceled', ''])
  for (const t of await live.listRunning()) await live.stopPush(t.streamId).catch(() => undefined)
  // 已结束推流且有存档（succeeded + outputPath）不出“文件可能不完整”：文案只在 canceled + outputPath 非空时出现
  eq('存档已保留文案', LIVE_CANCELED_ARCHIVE_KEPT_TEXT, '已强制停止，存档已保留，文件可能不完整')
  eq('正在停止文案', LIVE_STOPPING_TEXT, '正在停止…')
  // scheme= 首行解析（设计稿 v0.2 §7-16）：只看 detail 首行整行 scheme=rtmp|rtmps|srt；缺失 / 不在首行 / 写法不严格 → RTMP 版；与 missing= 不混用
  for (const [d, want] of [['scheme=rtmp\nx', 'rtmp'], ['scheme=rtmps', 'rtmps'], ['scheme=srt\nConnection failed', 'srt'], ['scheme=SRT', 'srt']] as const) {
    eq(`scheme 首行 ${JSON.stringify(d)}`, new AppError('LIVE_CONNECT_FAILED', 'x', d).scheme, want)
  }
  for (const d of [undefined, '', 'Connection failed\nscheme=srt', 'scheme=srt extra', ' x\nscheme=srt', 'missing=srt', 'reason=malformed']) {
    eq(`scheme 无效首行 ${JSON.stringify(d)} → undefined`, new AppError('LIVE_CONNECT_FAILED', 'x', d).scheme, undefined)
  }
  eq('连接失败：首行 scheme=srt → SRT 版', liveFailureMessage({ code: 'LIVE_CONNECT_FAILED', detail: 'scheme=srt\nx' }, 'rtmp'), LIVE_SRT_CONNECT_FAILED_TEXT)
  eq('连接失败：首行 scheme=rtmp / rtmps → RTMP 版', [liveFailureMessage({ code: 'LIVE_CONNECT_FAILED', detail: 'scheme=rtmp' }), liveFailureMessage({ code: 'LIVE_CONNECT_FAILED', detail: 'scheme=rtmps' })], [LIVE_RTMP_CONNECT_FAILED_TEXT, LIVE_RTMP_CONNECT_FAILED_TEXT])
  eq('连接失败：无首行 → RTMP 版（不看后面行的 scheme=）', liveFailureMessage({ code: 'LIVE_CONNECT_FAILED', detail: 'Connection failed\nscheme=srt' }), LIVE_RTMP_CONNECT_FAILED_TEXT)
  eq('scheme= 与 missing= 不混用：missing=srt 不影响连接失败文案，scheme=srt 不产生缺协议名', [liveFailureMessage({ code: 'LIVE_CONNECT_FAILED', detail: 'missing=srt' }), liveFfmpegProtocolMissingText('scheme=srt')], [LIVE_RTMP_CONNECT_FAILED_TEXT, PROTO_GENERIC])
  // 屏幕推流两个码、TASK_CONFLICT 四条文案都在映射里
  eq('TASK_CONFLICT 映射四条', [taskConflictText('max_sessions'), taskConflictText('duplicate_url'), taskConflictText('screen_busy'), taskConflictText('other')], ['最多同时推 4 路', '这个地址已经在推流', '屏幕推流同一时间只能有 1 路，请先停止当前的屏幕推流', '操作冲突，请稍后再试'])
  eq('SCREEN_PERMISSION_DENIED 文案', errorMessages.SCREEN_PERMISSION_DENIED.description, '没有获得屏幕录制权限，请在系统设置中允许 FFmpegFree 录制屏幕后重试')
  eq('UNSUPPORTED_PLATFORM 文案', errorMessages.UNSUPPORTED_PLATFORM.description, '当前系统暂不支持屏幕推流')
  eq('errorMessages 已知码不含 LIVE_PLAY 以外遗漏', Object.keys(errorMessages).length >= 8 && Object.keys(taskErrorMessages).length >= 3, true)

  // ---- 编码设备（后端字段：ffmpegReady + devices[id,name,vendor,kind,available,reason?]；cpu 永远第一项；偏好 auto|cpu|设备 id）----
  {
    const nv = { id: 'nvidia-0', name: 'NVIDIA GeForce RTX 4060', vendor: 'nvidia', kind: 'gpu', available: true }
    const n = encApi.normalizeList({ ffmpegReady: true, devices: [{ id: 'cpu', name: 'CPU', vendor: 'unknown', kind: 'cpu', available: true }, nv, { id: 'x', name: 'X', vendor: 'matrox', kind: 'gpu', available: false, reason: '驱动异常' }, { name: '没有 id' }, null] })
    eq('normalizeList：未知 vendor → unknown，缺 id 的丢弃，reason 保留', n.devices.map((d) => [d.id, d.vendor, d.available, d.reason]), [['cpu', 'unknown', true, undefined], ['nvidia-0', 'nvidia', true, undefined], ['x', 'unknown', false, '驱动异常']])
    eq('normalizeList：缺 cpu 时补在第一项', encApi.normalizeList({ ffmpegReady: true, devices: [nv] }).devices.map((d) => d.id), ['cpu', 'nvidia-0'])
    eq('normalizeList：空 / 错误形状 → 只有 cpu；ffmpegReady 只有明确 false 才算未就绪', [encApi.normalizeList(null).devices.map((d) => d.id), encApi.normalizeList(undefined).ffmpegReady, encApi.normalizeList({ ffmpegReady: false }).ffmpegReady], [['cpu'], true, false])
    eq('normalizeList：结果不含编码器名字段（encoders）', Object.keys(n.devices[1]).sort(), ['available', 'discrete', 'id', 'kind', 'name', 'vendor'])
    eq('normalizeList：后端 encoders{h264,hevc} 被丢弃', JSON.stringify(encApi.normalizeList({ ffmpegReady: true, devices: [{ ...nv, discrete: true, encoders: { h264: 'h264_nvenc', hevc: 'hevc_nvenc' } }] })).includes('nvenc'), false)

    // 显示规则：纯浏览器（无 Wails）→ 没有 ?enc= 完全不显示；有 ?enc= 才显示模拟层（标志为 true 时 Wails 里另见下面）
    win.location.search = ''
    eq('面板显示：默认（无 ?enc=）→ 不显示', encApi.encoderPanelVisible(), false)
    win.location.search = '?ff=ready'
    eq('面板显示：只有别的参数 → 不显示', encApi.encoderPanelVisible(), false)
    win.location.search = '?enc=found'
    eq('面板显示：?enc=found → 显示模拟层', encApi.encoderPanelVisible(), true)

    encApi.resetEncoderSim()
    win.location.search = '?enc=found'
    const l1 = await encApi.listEncoderDevices()
    eq('模拟 found：第一项 cpu、两张显卡、ffmpegReady', [l1.ffmpegReady, l1.devices.map((d) => d.id), l1.devices.map((d) => d.kind)], [true, ['cpu', 'nvidia-0', 'intel-0'], ['cpu', 'gpu', 'gpu']])
    eq('偏好默认 auto', await encApi.getEncoderPreference(), 'auto')
    await encApi.setEncoderPreference('nvidia-0')
    eq('Set 后 Get 返回新值', await encApi.getEncoderPreference(), 'nvidia-0')
    await encApi.setEncoderPreference('cpu')
    eq('偏好可设为 cpu', await encApi.getEncoderPreference(), 'cpu')
    // GetEncoderPreferenceInfo：自动 / CPU / 具体显卡名；设备不可用时仍带名字
    encApi.resetEncoderSim()
    eq('偏好信息：默认 → 自动', await encApi.getEncoderPreferenceInfo(), { id: 'auto', name: '自动', available: true })
    await encApi.setEncoderPreference('nvidia-0')
    eq('偏好信息：具体显卡名', (await encApi.getEncoderPreferenceInfo()).name, 'NVIDIA GeForce RTX 4060')
    await encApi.setEncoderPreference('gone-0')
    eq('偏好信息：设备不存在 → available=false 且有原因', [(await encApi.getEncoderPreferenceInfo()).available, !!(await encApi.getEncoderPreferenceInfo()).reason], [false, true])
    eq('refreshEncoderDevices 模拟层可用', (await encApi.refreshEncoderDevices()).devices[0].id, 'cpu')
    encApi.resetEncoderSim()
    win.location.search = '?enc=none'
    const l2 = await encApi.listEncoderDevices()
    eq('模拟 none：只有 cpu', l2.devices.map((d) => d.id), ['cpu'])
    win.location.search = '?enc=unavail'
    encApi.resetEncoderSim()
    const l3 = await encApi.listEncoderDevices()
    eq('模拟 unavail：偏好保持原显卡，列表里 available=false 并带原因', [await encApi.getEncoderPreference(), l3.devices.find((d) => d.id === 'nvidia-0')?.available, !!l3.devices.find((d) => d.id === 'nvidia-0')?.reason], ['nvidia-0', false, true])
    win.location.search = '?enc=noff'
    eq('模拟 noff：ffmpegReady=false', (await encApi.listEncoderDevices()).ffmpegReady, false)
    win.location.search = '?enc=fail'
    eq('模拟 fail：抛错', (await rejects(encApi.listEncoderDevices()))?.code, 'INTERNAL')
    win.location.search = ''
    encApi.resetEncoderSim()

    // ---- 对接后端 #60：多显卡排序 / 偏好信息 / 假 Wails 绑定 / 过期返回 / 标志为 false ----
    const dev = (id: string, kind: string, discrete: boolean, available = true) => ({ id, name: id, vendor: 'unknown', kind, discrete, available })
    eq('排序：cpu 第一，独显在前、集显在后，同级保持后端顺序', encApi.normalizeList({ ffmpegReady: true, devices: [dev('cpu', 'cpu', false), dev('intel-0', 'gpu', false), dev('amd-0', 'gpu', true), dev('nvidia-0', 'gpu', true), dev('intel-1', 'gpu', false)] }).devices.map((d) => d.id), ['cpu', 'amd-0', 'nvidia-0', 'intel-0', 'intel-1'])
    eq('排序：后端 cpu 不在第一位也会被纠正到第一', encApi.normalizeList({ ffmpegReady: true, devices: [dev('nvidia-0', 'gpu', true), dev('cpu', 'cpu', false)] }).devices.map((d) => d.id), ['cpu', 'nvidia-0'])
    eq('排序：缺 discrete 视为集显', encApi.normalizeList({ ffmpegReady: true, devices: [{ id: 'a', name: 'a', kind: 'gpu' }, dev('b', 'gpu', true)] }).devices.map((d) => d.id), ['cpu', 'b', 'a'])
    win.location.search = '?enc=multi'
    encApi.resetEncoderSim()
    const lm = await encApi.listEncoderDevices()
    eq('模拟 multi：独显（AMD、NVIDIA）在集显（Intel）前，下拉第一张 = 自动会选的那张', lm.devices.map((d) => d.id), ['cpu', 'amd-0', 'nvidia-0', 'intel-0'])
    eq('视图：多显卡 auto → 提示的是排序后的第一张', deriveEncoderView({ loading: false, failed: false, pref: 'auto', ffmpegReady: true, list: lm }).note.text, encMsg.encoderAutoNote(3, 'AMD Radeon RX 7600'))
    // 偏好信息：所选不可用（含设备已不存在）时显示保存的名字 + 警告 + “改回自动”
    const cur = { loading: false, failed: false, ffmpegReady: true }
    const ghostList = encApi.normalizeList({ ffmpegReady: true, devices: [dev('cpu', 'cpu', false), { ...dev('nvidia-0', 'gpu', true, false), name: '保存的显卡名', reason: '驱动异常' }] })
    const g = deriveEncoderView({ ...cur, pref: 'nvidia-0', list: ghostList, info: { id: 'nvidia-0', name: '保存的显卡名', available: false, reason: '驱动异常' } })
    eq('视图：所选不可用 → 名字取偏好信息，警告 + 改回自动', [g.selectText, g.selectWarn, g.note.action], ['保存的显卡名', true, 'resetAuto'])
    const gone = deriveEncoderView({ ...cur, pref: 'nvidia-0', list: encApi.normalizeList({ ffmpegReady: true, devices: [] }), info: { id: 'nvidia-0', name: 'NVIDIA GeForce RTX 4060', available: false } })
    eq('视图：所选设备不在列表 → 偏好信息里记下的名字仍显示', [gone.selectText, gone.selectWarn], ['NVIDIA GeForce RTX 4060', true])
    const noName = deriveEncoderView({ ...cur, pref: 'nvidia-0', list: encApi.normalizeList({ ffmpegReady: true, devices: [] }), info: { id: 'nvidia-0', name: '', available: false } })
    eq('视图：连名字都没记过 → 兜底名', noName.selectText, encMsg.ENCODER_UNKNOWN_SELECTED)
    const stale = deriveEncoderView({ ...cur, pref: 'cpu', list: lm, info: { id: 'nvidia-0', name: '别的显卡', available: false } })
    eq('视图：偏好信息与当前偏好 id 不一致（过期）不采用', [stale.selectText, stale.selectWarn], [encMsg.ENCODER_OPTION_CPU, false])
    eq('视图：偏好信息说可用 → 正常', deriveEncoderView({ ...cur, pref: 'nvidia-0', list: lm, info: { id: 'nvidia-0', name: 'NVIDIA GeForce RTX 4060', available: true } }).selectWarn, false)
    eq('视图：偏好信息说不可用而列表说可用 → 走警告（保守）', deriveEncoderView({ ...cur, pref: 'nvidia-0', list: lm, info: { id: 'nvidia-0', name: 'x', available: false } }).selectWarn, true)
    eq('视图：ffmpegReady=false（后端）→ noff 状态，显示定稿文案', deriveEncoderView({ ...cur, pref: 'auto', list: encApi.normalizeList({ ffmpegReady: false, devices: [dev('cpu', 'cpu', false)] }) }).note.text, encMsg.ENCODER_NO_FFMPEG_NOTE)
    // 模拟层：偏好写入后重读
    encApi.resetEncoderSim()
    win.location.search = '?enc=found'
    await encApi.setEncoderPreference('amd-0')
    const pi = await encApi.getEncoderPreferenceInfo()
    eq('偏好写入后重读：信息里是显卡名（设备当前不在列表里 → available=false 且名字仍在）', [pi.id, pi.name, pi.available], ['amd-0', 'AMD Radeon RX 7600', false])
    await encApi.setEncoderPreference('nvidia-0')
    eq('偏好写入后重读：选可用显卡 → available=true', [(await encApi.getEncoderPreferenceInfo()).name, (await encApi.getEncoderPreferenceInfo()).available], ['NVIDIA GeForce RTX 4060', true])
    win.location.search = '?enc=unavail'
    encApi.resetEncoderSim()
    const pu = await encApi.getEncoderPreferenceInfo()
    eq('模拟 unavail：偏好信息 = 保存的显卡名 + 不可用 + 原因', [pu.name, pu.available, !!pu.reason], ['NVIDIA GeForce RTX 4060', false, true])
    // 事件序号：后发起的让先发起的作废
    const sq = createSeq()
    const t1 = sq.next()
    const t2 = sq.next()
    eq('事件序号：旧的作废、新的有效', [sq.isCurrent(t1), sq.isCurrent(t2)], [false, true])
    win.location.search = ''
    encApi.resetEncoderSim()
    // 假 Wails 绑定（window.go.app.SystemService）：标志为 false 时不调用；字段原样对齐；encoders 丢弃
    const calls: string[] = []
    const svc = {
      ListEncoderDevices: async () => (calls.push('List'), { ffmpegReady: true, devices: [{ ...dev('cpu', 'cpu', false), encoders: { h264: 'libx264', hevc: 'libx265' } }, { ...dev('intel-0', 'gpu', false), encoders: { h264: 'h264_qsv', hevc: '' } }, { ...dev('nvidia-0', 'gpu', true), encoders: { h264: 'h264_nvenc', hevc: '' } }] }),
      RefreshEncoderDevices: async () => (calls.push('Refresh'), { ffmpegReady: false, devices: [dev('cpu', 'cpu', false)] }),
      GetEncoderPreference: async () => (calls.push('GetPref'), 'nvidia-0'),
      GetEncoderPreferenceInfo: async () => (calls.push('Info'), { id: 'nvidia-0', name: 'NVIDIA GeForce RTX 4060', available: false, reason: '驱动异常' }),
      SetEncoderPreference: async (id: string) => (calls.push('Set:' + id), undefined),
    }
    // 标志为 true：Wails 里显示并走真实绑定；?enc= 只在纯浏览器生效
    delete (win as unknown as Record<string, unknown>).go // 先当纯浏览器
    delete (win as unknown as Record<string, unknown>).runtime
    eq('标志值：ENCODER_BACKEND_READY 为 true（后端第二个 PR #67 / #68 已合入）', encApi.ENCODER_BACKEND_READY, true)
    eq('纯浏览器（无 Wails）+ 标志 true：没有 ?enc= 仍不显示（encoderIsReal=false）', [encApi.encoderPanelVisible(), encApi.encoderIsReal()], [false, false])
    win.location.search = '?enc=found'
    eq('纯浏览器 + ?enc=：显示模拟层（仅开发用），不调用绑定', [encApi.encoderPanelVisible(), encApi.encoderIsReal(), (await encApi.listEncoderDevices()).devices.length, calls.length], [true, false, 3, 0])
    win.location.search = ''
    ;(win as unknown as Record<string, unknown>).go = { app: { SystemService: svc } }
    ;(win as unknown as Record<string, unknown>).runtime = {}
    eq('标志为 true + 有 Wails：面板显示、走真实绑定', [encApi.encoderPanelVisible(), encApi.encoderIsReal()], [true, true])
    win.location.search = '?enc=none'
    eq('标志为 true + 有 Wails：?enc= 无效（仍走真实绑定，不用模拟场景）', [encApi.encoderPanelVisible(), encApi.encoderIsReal(), (await encApi.listEncoderDevices()).devices.map((d) => d.id), calls.includes('List')], [true, true, ['cpu', 'nvidia-0', 'intel-0'], true])
    eq('标志为 true + 有 Wails：偏好走真实绑定', [await encApi.getEncoderPreference(), (await encApi.getEncoderPreferenceInfo()).name], ['nvidia-0', 'NVIDIA GeForce RTX 4060'])
    await encApi.setEncoderPreference('cpu')
    eq('标志为 true + 有 Wails：Set 调用真实绑定', calls.includes('Set:cpu'), true)
    eq('标志为 true + 有 Wails：刷新走真实绑定', [(await encApi.refreshEncoderDevices()).ffmpegReady, calls.includes('Refresh')], [false, true])
    win.location.search = ''
    delete (win as unknown as Record<string, unknown>).go
    delete (win as unknown as Record<string, unknown>).runtime
    // 真实绑定形状：直接对 normalizeList / normalizeInfo 喂后端返回（标志为 false 时 API 函数不走绑定，这里验字段对齐）
    const realList = encApi.normalizeList(await svc.ListEncoderDevices())
    eq('真实绑定形状：排序后 cpu / nvidia（独显）/ intel（集显），无编码器名', [realList.devices.map((d) => d.id), JSON.stringify(realList).includes('nvenc') || JSON.stringify(realList).includes('qsv') || JSON.stringify(realList).includes('libx264')], [['cpu', 'nvidia-0', 'intel-0'], false])
    eq('真实绑定形状：ffmpegReady=false + 仅 cpu', encApi.normalizeList(await svc.RefreshEncoderDevices()), { ffmpegReady: false, devices: [{ id: 'cpu', name: 'cpu', vendor: 'unknown', kind: 'cpu', discrete: false, available: true }] })
    eq('真实绑定形状：偏好信息', encApi.normalizeInfo(await svc.GetEncoderPreferenceInfo()), { id: 'nvidia-0', name: 'NVIDIA GeForce RTX 4060', available: false, reason: '驱动异常' })
    eq('normalizeInfo：缺字段 → auto / 空名 / 可用', encApi.normalizeInfo({}), { id: 'auto', name: '', available: true })

    // 视图推导（设置页的状态行）
    const base = { loading: false, failed: false, pref: 'auto', ffmpegReady: true }
    const two = encApi.normalizeList({ ffmpegReady: true, devices: [nv, { id: 'intel-0', name: 'Intel UHD Graphics 770', vendor: 'intel', kind: 'gpu', available: true }] })
    const only = encApi.normalizeList({ ffmpegReady: true, devices: [] })
    const bad = encApi.normalizeList({ ffmpegReady: true, devices: [{ ...nv, available: false, reason: '驱动异常' }] })
    const v = (o: Partial<Parameters<typeof deriveEncoderView>[0]>) => deriveEncoderView({ ...base, list: two, ...o })
    eq('视图：检测中 → 下拉与重新检测置灰', [v({ loading: true }).state, v({ loading: true }).selectDisabled, v({ loading: true }).redetectDisabled], ['detecting', true, true])
    eq('视图：有显卡 + auto → 提示第一张可用显卡', v({}).note.text, encMsg.encoderAutoNote(2, 'NVIDIA GeForce RTX 4060'))
    eq('视图：没检测到显卡 → 无显卡提示，下拉不列显卡', [v({ list: only }).note.text, v({ list: only }).gpus.length], [encMsg.ENCODER_NONE_NOTE, 0])
    eq('视图：选了具体显卡 → 显示显卡名，ok 色', [v({ pref: 'nvidia-0' }).selectText, v({ pref: 'nvidia-0' }).note.tone], ['NVIDIA GeForce RTX 4060', 'ok'])
    eq('视图：选 CPU', [v({ pref: 'cpu' }).selectText, v({ pref: 'cpu' }).note.text], [encMsg.ENCODER_OPTION_CPU, encMsg.ENCODER_CPU_NOTE])
    const un = v({ list: bad, pref: 'nvidia-0' })
    eq('视图：所选不可用 → 保持原显卡名、警告描边、“改回自动”，菜单里没有对应勾选', [un.selectText, un.selectWarn, un.note.action, un.selectedKey, un.note.tone], ['NVIDIA GeForce RTX 4060', true, 'resetAuto', '', 'warn'])
    eq('视图：所选显卡已不在列表 → 用兜底名', v({ list: only, pref: 'nvidia-0' }).selectText, encMsg.ENCODER_UNKNOWN_SELECTED)
    eq('视图：检测失败 → 红色 alert 文案', [v({ failed: true, list: null }).note.text, v({ failed: true, list: null }).note.tone], [encMsg.ENCODER_FAILED_NOTE, 'err'])
    eq('视图：ffmpeg 未就绪（store 或后端 ffmpegReady=false）→ 提示去安装，下拉置灰', [v({ ffmpegReady: false }).state, v({ list: { ffmpegReady: false, devices: [] } }).state, v({ ffmpegReady: false }).selectDisabled], ['noff', 'noff', true])
    // 界面文案不得出现编码器名
    // 原因枚举表的键是后端固定枚举（nvenc_init_failed 等，不会显示给用户），所以扫的是句子（值），不扫键
    const allText = JSON.stringify(Object.values(encMsg).map((x) => (typeof x === 'function' ? (x as (...a: unknown[]) => string)(2, 'GPU') : typeof x === 'object' ? Object.values(x as Record<string, string>) : x)))
    eq('编码设备文案不含编码器名（NVENC / QSV / AMF / VideoToolbox）', /nvenc|qsv|amf|videotoolbox|h264_|hevc_/i.test(allText), false)
    eq('回退文案锁定（设计稿，待产品经理确认）', [encMsg.ENCODER_FALLBACK_SETTINGS_LINK, encMsg.ENCODER_FALLBACK_LOG_LINK], ['编码设置', '查看日志'])

    // ───────── 任务的编码设备信息（契约 v0.18 §9.7 / v0.19）─────────
    {
      const NV = { id: 'nvidia-0', name: 'NVIDIA GeForce RTX 4060', vendor: 'nvidia', kind: 'gpu', discrete: true, available: true } as const
      const devs = [encApi.normalizeList({ ffmpegReady: true, devices: [{ id: 'cpu', name: 'CPU（软件编码）', vendor: 'unknown', kind: 'cpu', available: true }, NV] }).devices][0]
      // 四字段合并：缺省不覆盖
      const cur: encTask.EncoderFields = { encoder: 'h264_nvenc', encoderDevice: 'nvidia-0' }
      encTask.mergeEncoderFields(cur, {})
      eq('合并：事件四字段全缺省 → 已有值不变', cur, { encoder: 'h264_nvenc', encoderDevice: 'nvidia-0' })
      encTask.mergeEncoderFields(cur, { hwFallback: false })
      eq('合并：hwFallback=false / 缺省不写', cur.hwFallback, undefined)
      encTask.mergeEncoderFields(cur, { encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, hwFallbackReason: 'nvenc_init_failed' })
      eq('合并：回退补发的 running status → 改成 CPU 并置 hwFallback', cur, { encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, hwFallbackReason: 'nvenc_init_failed' })
      encTask.mergeEncoderFields(cur, { encoder: undefined, encoderDevice: '', hwFallback: undefined, hwFallbackReason: '' })
      eq('合并：后续 progress 缺省 / 空串 → 不被覆盖成空、hwFallback 不被清掉', cur, { encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, hwFallbackReason: 'nvenc_init_failed' })
      encTask.mergeEncoderFields(cur, { hwFallbackReason: 'brand_new_reason_x' })
      eq('合并：只带一个字段也能单独更新', [cur.encoder, cur.hwFallbackReason], ['libx264', 'brand_new_reason_x'])

      // 走 task store（事件通道）：created(显卡) → progress(只带进度) → status running 补发(CPU + hwFallback) → progress(缺省) → succeeded(不带编码器字段) → 终态快照
      setActivePinia(createPinia())
      const ts = useTaskStore()
      await ts.init()
      const t0 = Date.now()
      emitSimEvent('task:created', { id: 'e1', type: 'convert', status: 'queued', title: 'a.mov', inputPaths: [], outputPath: '/o/a.mp4', progress: 0, speed: '', etaSec: 0, params: '', version: 1, createdAt: t0, startedAt: 0, finishedAt: 0, encoder: 'h264_nvenc', encoderDevice: 'nvidia-0' })
      emitSimEvent('task:status', { id: 'e1', version: 2, status: 'running', startedAt: t0, encoder: 'h264_nvenc', encoderDevice: 'nvidia-0' })
      emitSimEvent('task:progress', { id: 'e1', version: 3, progress: 0.2, speed: '2x', etaSec: 10, outTimeSec: 1 })
      const g = () => ts.taskById('e1') as encTask.EncoderFields | undefined
      eq('store：progress 不带编码器字段 → 显卡信息还在', [g()?.encoder, g()?.encoderDevice, g()?.hwFallback], ['h264_nvenc', 'nvidia-0', undefined])
      emitSimEvent('task:status', { id: 'e1', version: 4, status: 'running', encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, hwFallbackReason: 'nvenc_init_failed' })
      emitSimEvent('task:progress', { id: 'e1', version: 5, progress: 0.1, speed: '1x', etaSec: 20, outTimeSec: 1 })
      eq('store：补发 running status 后更新 hwFallback / 设备，之后缺省的 progress 不覆盖', [g()?.encoder, g()?.encoderDevice, g()?.hwFallback, g()?.hwFallbackReason], ['libx264', 'cpu', true, 'nvenc_init_failed'])
      emitSimEvent('task:status', { id: 'e1', version: 6, status: 'succeeded', finishedAt: t0 + 5000 })
      eq('store：终态事件不带编码器字段 → 终态快照仍保留', [g()?.encoder, g()?.encoderDevice, g()?.hwFallback, g()?.hwFallbackReason], ['libx264', 'cpu', true, 'nvenc_init_failed'])
      // 终态事件带字段、任务不在活动列表里（先于 created 到达）
      emitSimEvent('task:status', { id: 'e2', version: 3, status: 'failed', startedAt: t0, finishedAt: t0 + 1, error: { code: 'PROCESS_FAILED', message: 'x' }, encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, hwFallbackReason: 'encoder_start_failed' })
      eq('store：不在活动列表里的终态事件也把四字段记进快照', [(ts.taskById('e2') as encTask.EncoderFields | undefined)?.hwFallback, (ts.taskById('e2') as encTask.EncoderFields | undefined)?.hwFallbackReason], [true, 'encoder_start_failed'])

      // 显示规则
      const on = true
      const fb = { encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, hwFallbackReason: 'nvenc_init_failed', startedAt: 1000 }
      eq('提示：hwFallback=true 且运行过且功能启用 → 显示', encTask.showFallbackNotice(fb, on), true)
      eq('提示：hwFallback=false（auto 落到 CPU / 偏好 CPU）→ 不显示', encTask.showFallbackNotice({ ...fb, hwFallback: false }, on), false)
      eq('提示：-c copy（encoder=copy、无设备）→ 不显示，也不显示设备', [encTask.showFallbackNotice({ encoder: 'copy', startedAt: 1000 }, on), encTask.usedDeviceText({ encoder: 'copy', startedAt: 1000 }, devs, on)], [false, ''])
      eq('提示：两遍编码 / 宽或高超 4096 / VP9 走 CPU（hwFallback 缺省）→ 不显示，设备显示 CPU', [encTask.showFallbackNotice({ encoder: 'libx264', encoderDevice: 'cpu', startedAt: 1000 }, on), encTask.usedDeviceText({ encoder: 'libx264', encoderDevice: 'cpu', startedAt: 1000 }, devs, on)], [false, 'CPU'])
      eq('提示：startedAt=0（从未运行：排队中取消 / 退出时还在排队，字段仍是提交时的值）→ 不显示提示、不显示设备', [encTask.showFallbackNotice({ ...fb, startedAt: 0 }, on), encTask.usedDeviceText({ ...fb, startedAt: 0 }, devs, on)], [false, ''])
      eq('提示：startedAt 缺省同样不显示', [encTask.showFallbackNotice({ ...fb, startedAt: undefined }, on), encTask.usedDeviceText({ ...fb, startedAt: undefined }, devs, on)], [false, ''])
      eq('提示：没有任务 / 没有编码器字段 → 不显示', [encTask.showFallbackNotice(undefined, on), encTask.showFallbackNotice({ startedAt: 1000 }, on), encTask.usedDeviceText({ startedAt: 1000 }, devs, on)], [false, false, ''])
      // 直播页提示条（推流页）：live_* 任务里有 startedAt>0 且 hwFallback 才显示；startedAt 为 0 / 缺失、非直播任务、功能未启用都不显示
      const liveFb = { ...fb, type: 'live_file_push' }
      eq('直播提示条：运行过的直播任务回退 → 显示（文件 / 屏幕推流都算）', [encTask.liveFallbackShown([liveFb], on), encTask.liveFallbackShown([{ ...liveFb, type: 'live_screen_push' }], on)], [true, true])
      eq('直播提示条：startedAt=0 / 缺失 → 不显示', [encTask.liveFallbackShown([{ ...liveFb, startedAt: 0 }], on), encTask.liveFallbackShown([{ ...liveFb, startedAt: undefined }], on)], [false, false])
      eq('直播提示条：没回退 / 非直播任务回退 / 功能未启用 / 空列表 → 不显示', [encTask.liveFallbackShown([{ ...liveFb, hwFallback: false }], on), encTask.liveFallbackShown([{ ...fb, type: 'convert' }], on), encTask.liveFallbackShown([liveFb], false), encTask.liveFallbackShown([], on)], [false, false, false, false])
      eq('直播提示条：列表里只要有一个满足就显示', encTask.liveFallbackShown([{ ...liveFb, hwFallback: false }, { ...liveFb, startedAt: 0 }, liveFb], on), true)
      // 标志：ENCODER_BACKEND_READY=true → Wails 里整体启用；纯浏览器只有 ?enc= 启用（仅开发）
      eq('标志：ENCODER_BACKEND_READY 为 true', encApi.ENCODER_BACKEND_READY, true)
      win.location.search = ''
      eq('纯浏览器且无 ?enc= → 编码设备界面关闭：不显示提示、不显示设备', [encTask.encoderTaskUiEnabled(), encTask.showFallbackNotice(fb), encTask.usedDeviceText(fb, devs)], [false, false, ''])
      win.location.search = '?enc=fb-nvenc'
      eq('纯浏览器 ?enc= 预览 → 界面启用（仅开发用）', [encTask.encoderTaskUiEnabled(), encTask.showFallbackNotice(fb), encTask.usedDeviceText(fb, devs)], [true, true, 'CPU（已回退）'])
      win.location.search = ''
      ;(win as unknown as Record<string, unknown>).go = { app: {} }
      ;(win as unknown as Record<string, unknown>).runtime = {}
      eq('Wails 里（标志 true）→ 界面启用：hwFallback 显示提示、设备显示名', [encTask.encoderTaskUiEnabled(), encTask.showFallbackNotice(fb), encTask.usedDeviceText(fb, devs)], [true, true, 'CPU（已回退）'])
      win.location.search = '?enc=none'
      eq('Wails 里 ?enc= 无效但界面照常启用', encTask.encoderTaskUiEnabled(), true)
      eq('Wails 里：startedAt=0 仍不显示提示和设备', [encTask.showFallbackNotice({ ...fb, startedAt: 0 }), encTask.usedDeviceText({ ...fb, startedAt: 0 }, devs)], [false, ''])
      win.location.search = ''
      delete (win as unknown as Record<string, unknown>).go
      delete (win as unknown as Record<string, unknown>).runtime
      // 设备名：只取 name；cpu → CPU；查不到 → 显卡；永不显示 id
      eq('设备名：显卡取列表里的 name', encTask.deviceDisplayName('nvidia-0', devs), 'NVIDIA GeForce RTX 4060')
      eq('设备栏：回退到 CPU → “CPU（已回退）”；没回退的 CPU 任务仍是“CPU”', [encTask.usedDeviceText({ encoder: 'libx264', encoderDevice: 'cpu', hwFallback: true, startedAt: 1 }, devs, true), encTask.usedDeviceText({ encoder: 'libx264', encoderDevice: 'cpu', startedAt: 1 }, devs, true)], ['CPU（已回退）', 'CPU'])
      eq('设备名：cpu → CPU（不取后端的“CPU（软件编码）”）', encTask.deviceDisplayName('cpu', devs), 'CPU')
      eq('设备名：查不到 / 列表没读到 → 显卡，不显示 id', [encTask.deviceDisplayName('nvidia-9', devs), encTask.deviceDisplayName('nvidia-9', null), encTask.deviceDisplayName('nvidia-9', [])], ['显卡', '显卡', '显卡'])
      eq('设备文字：显卡任务 → 设备名，encoder（h264_nvenc）不出现', encTask.usedDeviceText({ encoder: 'h264_nvenc', encoderDevice: 'nvidia-0', startedAt: 1 }, devs, true), 'NVIDIA GeForce RTX 4060')

      // hwFallbackReason 文案：与契约 §9.7 枚举一一对应；未知走兜底；不含编码器名
      const ENUM = ['device_unavailable', 'nvenc_init_failed', 'qsv_init_failed', 'amf_init_failed', 'videotoolbox_failed', 'encoder_unavailable', 'encoder_start_failed']
      eq('原因文案的枚举 = 契约 9.7 的七个（一一对应）', Object.keys(encMsg.ENCODER_FALLBACK_REASONS).sort(), [...ENUM].sort())
      eq('原因文案：每个枚举都有非空句子', ENUM.every((r) => encMsg.encoderFallbackReasonText(r).length > 0 && encMsg.encoderFallbackReasonText(r) !== encMsg.ENCODER_FALLBACK_REASON_GENERIC || r === ''), true)
      eq('原因文案：未知 / 空 / undefined / 原型属性名 → 通用兜底句，不报错', [encMsg.encoderFallbackReasonText('brand_new_reason_x'), encMsg.encoderFallbackReasonText(''), encMsg.encoderFallbackReasonText(undefined), encMsg.encoderFallbackReasonText('toString')], Array(4).fill(encMsg.ENCODER_FALLBACK_REASON_GENERIC))
      // 界面文案不得出现编码器名：扫 encoderMessages 的全部导出 + 新增 / 改动的模板与脚本
      const banned = /nvenc|qsv|amf|videotoolbox|libx26|h264_|hevc_|x264|x265|encoderDevice|nvidia-0/i
      const texts = Object.values(encMsg).flatMap((x) => (typeof x === 'function' ? [(x as (...a: unknown[]) => string)(2, 'GPU')] : typeof x === 'string' ? [x] : Object.values(x as Record<string, string>)))
      eq('encoderMessages 全部文案（含原因句）不含编码器名', texts.filter((t) => banned.test(t)), [])
      // 产品经理定稿（小修订包 12）：用词统一，界面文案不含“硬件编码”“转码”和编码器名
      eq('encoderMessages 全部文案不含“硬件编码”“转码”', texts.filter((t) => /硬件编码|转码/.test(t)), [])
      const enumTexts = Object.fromEntries(ENUM.map((r) => [r, encMsg.encoderFallbackReasonText(r)]))
      const START_FAILED = '显卡编码器启动失败，已改用 CPU。'
      eq('七个原因码 → 定稿文案', enumTexts, { device_unavailable: '所选显卡当时不可用，已改用 CPU。', nvenc_init_failed: START_FAILED, qsv_init_failed: START_FAILED, amf_init_failed: START_FAILED, videotoolbox_failed: START_FAILED, encoder_unavailable: '没有可用的显卡编码器，已改用 CPU。', encoder_start_failed: START_FAILED })
      eq('未知原因兜底句', encMsg.ENCODER_FALLBACK_REASON_GENERIC, '未能确定具体原因，详情见下方日志。')
      eq('定稿文案：回退提示（转换 / 已完成历史任务 / 直播）+ 设备栏 + 无设备句号', [encMsg.ENCODER_FALLBACK_CONVERT, encMsg.ENCODER_FALLBACK_TASK_ROW_DONE, encMsg.ENCODER_FALLBACK_LIVE, encMsg.ENCODER_DEVICE_CPU_FALLBACK_NAME, encMsg.ENCODER_NONE_NOTE], ['显卡编码失败，已自动改用 CPU 转换。', '已自动改用 CPU 完成转换。', '显卡编码启动失败，已自动改用 CPU 推流。', 'CPU（已回退）', '未检测到可用的显卡，将使用 CPU。'])
      eq('回退文案都不写“继续转换”', [encMsg.ENCODER_FALLBACK_CONVERT, encMsg.ENCODER_FALLBACK_TASK_ROW, encMsg.ENCODER_FALLBACK_TASK_ROW_DONE, encMsg.ENCODER_FALLBACK_LIVE].filter((t) => /继续/.test(t)), [])
      {
        // 源码里用户可见字符串（去掉注释后的 .vue 模板 / .ts 字符串字面量）不含旧用词和编码器名
        const fsu = await import('node:fs')
        const rootu = `${process.cwd()}/src/`
        const walk = (d: string): string[] => fsu.readdirSync(rootu + d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(`${d}${e.name}/`) : [`${d}${e.name}`]))
        const files = walk('').filter((f) => /\.(vue|ts)$/.test(f) && !/\.check\.ts$/.test(f))
        const stripComments = (code: string) => code.replace(/<!--[\s\S]*?-->/g, '').replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:'"`\\])\/\/[^\n]*/g, '$1')
        const strLits = (code: string): string[] => [...code.matchAll(/'((?:[^'\\\n]|\\.)*)'|"((?:[^"\\\n]|\\.)*)"|`((?:[^`\\]|\\.)*)`/g)].map((m) => m[1] ?? m[2] ?? m[3] ?? '')
        const tplText = (src: string): string[] => {
          const t = src.includes('<template>') ? src.slice(src.indexOf('<template>'), src.lastIndexOf('</template>') + 11) : ''
          return [...t.matchAll(/>([^<]+)</g)].map((m) => m[1])
        }
        const bad: string[] = []
        for (const f of files) {
          const raw = fsu.readFileSync(rootu + f, 'utf8')
          const noCmt = stripComments(f.endsWith('.vue') ? raw.replace(/<style[\s\S]*?<\/style>/g, '') : raw)
          const visible = [...strLits(noCmt), ...(f.endsWith('.vue') ? tplText(noCmt) : [])]
          for (const v of visible) {
            if (/硬件编码|转码/.test(v)) bad.push(`${f}: ${v.slice(0, 40)}`)
            // 编码器名：只查带中文的字符串（内部枚举 / encoder 字段值如 h264_nvenc、nvenc_init_failed 是协议值，不是界面文字）
            if (/[\u4e00-\u9fff]/.test(v) && /NVENC|QSV|AMF|VideoToolbox/i.test(v)) bad.push(`${f}: ${v.slice(0, 40)}`)
          }
        }
        eq('源码里用户可见字符串（不含注释）不含“硬件编码”“转码”和 NVENC / QSV / AMF / VideoToolbox', bad, [])
      }
      const fs = await import('node:fs')
      const root = `${process.cwd()}/` // npm run check:api 在 frontend/ 下运行
      const tplFiles = ['src/views/ConvertPage.vue', 'src/components/convert/ConvertKid.vue', 'src/components/convert/ConvertSourceRow.vue', 'src/components/convert/ConvertPreviewDialog.vue', 'src/components/convert/ConvertSettingsPanel.vue', 'src/components/convert/ConvertDeleteDialog.vue', 'src/views/TaskCenter.vue', 'src/components/encoder/EncoderFallbackNotice.vue', 'src/components/live/LiveFallbackNotice.vue']
      const tplHits: string[] = []
      for (const f of tplFiles) {
        const src = fs.readFileSync(root + f, 'utf8')
        const tpl = src.slice(src.indexOf('<template>'), src.lastIndexOf('</template>') + 11) // 新页面是 script 在前，取第一个 <template> 到最后一个 </template>
        for (const m of tpl.matchAll(/(?:\{\{([^}]*)\}\})|(?:>([^<{]+)<)/g)) {
          const seg = (m[1] ?? m[2] ?? '').replace(/ENCODER_[A-Z_]+/g, '')
          if (/nvenc|qsv|amf|videotoolbox|libx26|x264|x265/i.test(seg) || /\.encoder\b|\.encoderDevice\b/.test(seg)) tplHits.push(`${f}: ${seg.trim()}`)
        }
      }
      eq('模板文字插值里没有编码器名，也不直接输出 encoder / encoderDevice 字段', tplHits, [])
      // ───────── 走查修订（设计师 PR69/70 走查 G1 / G6 / G11 / D2）─────────
      const readSrc = (f: string) => fs.readFileSync(root + f, 'utf8')
      // 减少动效：滚动 behavior
      const gw = globalThis as unknown as { window: Record<string, unknown>; document?: unknown }
      const oldMM = gw.window.matchMedia
      const oldDoc = gw.document
      let rmHook = false
      let rmMedia = false
      gw.document = { documentElement: { classList: { contains: (c: string) => c === 'reduce-motion' && rmHook } } }
      gw.window.matchMedia = (q: string) => ({ matches: q.includes('reduce') && rmMedia })
      const motion = await import('@/utils/motion')
      eq('滚动：默认 smooth', motion.scrollBehavior(), 'smooth')
      rmMedia = true
      eq('滚动：prefers-reduced-motion → auto（不做平滑动画）', motion.scrollBehavior(), 'auto')
      rmMedia = false; rmHook = true
      eq('滚动：html.reduce-motion → auto', motion.scrollBehavior(), 'auto')
      gw.window.matchMedia = oldMM as never
      gw.document = oldDoc
      // G1 / D1：设备名过长只截自己，并有 title 显示全名
      const kidSrc = readSrc('src/components/convert/ConvertKid.vue')
      eq('转换记录设备名：有 title 全名；回退短标带说明 title', [/<span v-else class="dev" :title="device">/.test(kidSrc), /class="cv-fb" :title="ENCODER_DEVICE_CPU_FALLBACK_TITLE"/.test(kidSrc)], [true, true])
      eq('转换记录设备名段 min-width:0 + 省略号；回退短标警告色', [/\.cv-km \.l3 \.dev\{min-width:0;overflow:hidden;text-overflow:ellipsis\}/.test(readSrc('src/components/convert/convert-v2.css')), /\.cv-fb\{[^}]*--ff-warning-text/.test(readSrc('src/components/convert/convert-v2.css'))], [true, true])
      eq('设备一栏回退短标：转换记录用 cv-fb，日志头用 dev-fb（警告色），样式在 base.css', [/class="cv-fb"/.test(kidSrc), /class="dev-fb"/.test(readSrc('src/views/TaskCenter.vue')), /\.dev-fb \{[^}]*--ff-warning-text[^}]*\}/.test(readSrc('src/styles/base.css'))], [true, true, true])
      eq('任务中心日志头设备名：有 title + 省略号', [/class="dv" :title=/.test(readSrc('src/views/TaskCenter.vue')), /\.logdev \.dv \{[^}]*text-overflow: ellipsis/.test(readSrc('src/views/TaskCenter.vue'))], [true, true])
      // G6：查看日志后滚动到日志面板（尊重减少动效）、焦点到面板
      const tcSrc = readSrc('src/views/TaskCenter.vue')
      eq('任务中心：打开日志会 revealLog（scrollIntoView + scrollBehavior + focus）', [/loadLog\(\)\s*revealLog\(\)/.test(tcSrc), /scrollIntoView\(\{ block: 'nearest', behavior: scrollBehavior\(\) \}\)/.test(tcSrc), /el\.focus\(\{ preventScroll: true \}\)/.test(tcSrc), /ref="logWrapEl" class="logwrap" tabindex="-1" role="region"/.test(tcSrc)], [true, true, true, true])
      // G11：“编码设置”跳转定位 + 子导航
      eq('编码设置跳转目标：设置页 + ?section=encoder', encTask.encoderSettingsLocation(), { path: '/settings/general', query: { section: 'encoder' } })
      const rawPush = ['src/views/ConvertPage.vue', 'src/components/live/LiveFallbackNotice.vue'].filter((f) => /router\.push\('\/settings\/general'\)/.test(readSrc(f)))
      eq('两处“编码设置”链接都用 encoderSettingsLocation（不再直接 push 设置页顶部）', rawPush, [])
      const layoutSrc = readSrc('src/views/settings/SettingsLayout.vue')
      eq('设置子导航：“编码设备”只在 encoderPanelVisible() 时加入，锚点 sec-encoder', [/\.\.\.\(encoderPanelVisible\(\) \? \[\{ key: 'encoder', label: ENCODER_PANEL_TITLE, to: '\/settings\/general', section: ENCODER_SECTION_ID \}\] : \[\]\)/.test(layoutSrc), encTask.ENCODER_SECTION_ID], [true, 'sec-encoder'])
      const setSrc = readSrc('src/views/Settings.vue')
      eq('设置页：读 ?section=encoder → 滚到面板并把焦点放到小节标题；面板不显示时不做', [/route\.query\.section !== ENCODER_SECTION_QUERY \|\| !encoderVisible/.test(setSrc), /scrollIntoView\(\{ behavior: scrollBehavior\(\)/.test(setSrc), /h2'\)\?\.focus\(\{ preventScroll: true \}\)/.test(setSrc), /<h2 :id="headingId" tabindex="-1">/.test(readSrc('src/components/encoder/EncoderDevicePanel.vue'))], [true, true, true, true])
      win.location.search = ''
      eq('子导航显示条件：纯浏览器无 ?enc= → 不显示；有 ?enc= → 显示', [encApi.encoderPanelVisible()], [false])
      win.location.search = '?enc=found'
      eq('子导航显示条件：?enc= → 显示', encApi.encoderPanelVisible(), true)
      win.location.search = ''
      // 模拟层（?enc=）：回退场景
      const s1 = simEncoderScenarioFor('fb-nvenc'); const s2 = simEncoderScenarioFor('gpu-task'); const s3 = simEncoderScenarioFor('copy-task'); const s4 = simEncoderScenarioFor('found')
      eq('?enc= 任务场景：fb-nvenc 回退 / gpu-task 不回退 / copy-task 无设备 / 设备列表场景不改任务', [s1?.hwFallback, s2?.hwFallback, s3?.encoder, s3?.encoderDevice, s4], [true, undefined, 'copy', '', undefined])
      // ───────── 预览包 15 ─────────
      {
        // S1：探测成功的媒体缺 hasVideo / hasAudio（后端 omitempty）→ 兜底 false；已有值保留
        const { normalizeMediaInfo } = await import('@/api/media')
        eq('S1 归一化：缺字段 → false / false；已有 true 保留；已有 false 保留', [normalizeMediaInfo({} as { hasVideo?: boolean; hasAudio?: boolean }), normalizeMediaInfo({ hasVideo: true }), normalizeMediaInfo({ hasVideo: false, hasAudio: true })], [{ hasVideo: false, hasAudio: false }, { hasVideo: true, hasAudio: false }, { hasVideo: false, hasAudio: true }])
        await convertV2Checks(eq, readSrc)
        await convertV24Checks(eq, readSrc)
        await liveFormsChecks(eq, readSrc) // 包 20：直播表单持久化 + 推流码遮挡
        await readyRelistChecks(eq, readSrc) // 包 20：就绪后补取列表（#100 配合）+ 走查 D2 / D3 / D4 / D7
        await catalogLoadChecks(eq, readSrc) // 格式目录：检测中保持骨架，8 秒才超时，就绪后自动再取
        await pkg22Checks(eq, readSrc) // 包 22：契约 v0.25.1（暂存 / 对齐 / 只往终态走）+ 走查 af6a508
        // G11 版本号
        const { cleanFfmpegVersion } = await import('@/utils/ffmpegVersion')
        eq('G11 版本号：旧（带 URL 尾巴）/ 新（干净）/ 其他尾巴 / 空', ['9.0.2-https://www.martin-riedl.de', '9.0.2', '7.1.1-essentials_build-www.gyan.dev', '6.0', ' 4.4.2-0ubuntu0.22.04.1 ', '', undefined].map((v) => cleanFfmpegVersion(v)), ['9.0.2', '9.0.2', '7.1.1', '6.0', '4.4.2', '', ''])
        eq('G11 版本号：非数字开头（git 构建）只去 URL 尾巴，其余原样', [cleanFfmpegVersion('N-117000-gabcdef-https://example.com'), cleanFfmpegVersion('N-117000-gabcdef')], ['N-117000-gabcdef', 'N-117000-gabcdef'])
        eq('G11 版本号：进 store 时清理（源码）', /version: cleanFfmpegVersion\(raw\.version\) \|\| undefined/.test(readSrc('src/stores/ffmpeg.ts')), true)
        // G4：旧文案改写；新文案（后端）原样；退出码不出现在主提示
        const em = await import('@/errors/errorMessages')
        const G4 = ['ffmpeg 异常退出（退出码 -1）', 'ffmpeg 退出码 1', '转换组件异常退出（退出码 -1）', '转换组件退出码 1']
        eq('G4 旧文案（PROCESS_FAILED）→ 说人话，不含退出码 / ffmpeg', G4.map((m) => em.resolveTaskError('PROCESS_FAILED', m).description), [em.PROCESS_EXIT_TEXT, em.PROCESS_EXIT_TEXT, em.PROCESS_EXIT_TEXT, em.PROCESS_EXIT_TEXT])
        eq('G4 改写句：全角标点、不含“硬件编码”“转码”、退出码、编码器名', [em.PROCESS_EXIT_TEXT, /硬件编码|转码|退出码|ffmpeg|nvenc|qsv|amf/i.test(em.PROCESS_EXIT_TEXT)], ['转换被意外中断，可以重试；如果反复出现，请查看日志。', false])
        eq('G4 后端新文案 / 其他码：原样', [em.resolveTaskError('PROCESS_FAILED', '转换没有成功，请查看日志').description, em.resolveTaskError('INTERNAL', 'ffmpeg 异常退出（退出码 -1）').description], ['转换没有成功，请查看日志', 'ffmpeg 异常退出（退出码 -1）'])
        // G8：直播行不给重试，叫“推流中断”，没有“自动重连”
        const lm = em.errorMessages.LIVE_PUSH_INTERRUPTED
        eq('N3 推流中断：描述“请回到直播页重新推流。”（不再与标题同义重复）；应用退出后中断那句保持原样', [lm.description, /'应用退出时推流被中断，请回到直播页重新推流。'/.test(readSrc('src/views/TaskCenter.vue'))], ['请回到直播页重新推流。', true])
        eq('G8 推流中断：标题“推流中断”、没有“重试”主按钮、文案不含“自动重连”“点击重试”', [lm.title, lm.primary, /自动重连|点击重试/.test(lm.description)], ['推流中断', null, false])
        const tcs = readSrc('src/views/TaskCenter.vue')
        eq('G8 任务中心：行尾“重试”走 canRetryTask；失败行重试排除直播和已下线类型', [/const canRetry = \(t: TaskItem\) => canRetryTask\(t\)/.test(tcs), /:hide-retry="t\.status === 'interrupted' \|\| isLiveType\(t\.type\) \|\| isRetiredType\(t\.type\)"/.test(tcs)], [true, true])
        eq('canRetryTask：失败 / 中断 / 已取消的转换可重试；直播、剪辑导出（已下线）、成功不可', [
          canRetryTask({ type: 'convert', status: 'failed' }), canRetryTask({ type: 'convert', status: 'interrupted' }), canRetryTask({ type: 'convert', status: 'canceled' }), canRetryTask({ type: 'office_pdf', status: 'failed' }),
          canRetryTask({ type: 'live_file_push', status: 'failed' }), canRetryTask({ type: 'edit_export', status: 'failed' }), canRetryTask({ type: 'edit_export', status: 'interrupted' }), canRetryTask({ type: 'convert', status: 'succeeded' }),
        ], [true, true, true, true, false, false, false, false])
        eq('剪辑导出：仍是已知类型（旧记录照常显示），但标为已下线', [isKnownTaskType('edit_export'), isRetiredType('edit_export'), isRetiredType('convert')], [true, true, false])
        eq('剪辑已移除：侧栏没有剪辑入口，#/edit 重定向到转换页', [mainNav.some((i) => i.path === '/edit' || i.label === '剪辑'), /\{ path: '\/edit\/:pathMatch\(\.\*\)\*', redirect: '\/' \}/.test(readSrc('src/router/index.ts')), /VideoEditor/.test(readSrc('src/router/index.ts'))], [false, true, false])
        // G7：全站 Element Plus 中文 locale
        const mainSrc = readSrc('src/main.ts')
        eq('G7 Element Plus 全局 zh-cn locale', [/import zhCn from 'element-plus\/es\/locale\/lang\/zh-cn'/.test(mainSrc), /app\.use\(ElementPlus, \{[^}]*locale: zhCn/.test(mainSrc)], [true, true])
        const zh = (await import('element-plus/es/locale/lang/zh-cn')).default as { el: { pagination: { total: string } } }
        eq('G7 分页“共 {total} 条”', zh.el.pagination.total, '共 {total} 条')
      }
    }
  }

    // ---- 拉流预览会话：Stop 一定被调用 ----
    const mkPull = (opt: { startMs?: number; preview?: boolean; failStart?: boolean; failStop?: boolean } = {}) => {
      const log: string[] = []
      let n = 0
      const gate: { release?: () => void } = {}
      const api: PullPreviewApi = {
        async start(req) {
          log.push('start:' + req.url)
          if (opt.startMs) await new Promise<void>((r) => (gate.release = r))
          if (opt.failStart) throw new Error('x')
          return { id: 'ps' + ++n, redacted: 'r', preview: opt.preview !== false, previewUrl: '' }
        },
        async stop(id) {
          log.push('stop:' + id)
          if (opt.failStop) throw new Error('stop failed')
        },
      }
      return { log, gate, c: new PullPreviewController(api) }
    }
    {
      const { log, c } = mkPull()
      await c.begin('http://a/x.flv', true)
      await c.end()
      eq('拉流：点停止 → Stop 被调用', log, ['start:http://a/x.flv', 'stop:ps1'])
      await c.end()
      eq('拉流：重复停止不重复 Stop', log.length, 2)
    }
    {
      const { log, c } = mkPull()
      const a = c.begin('u', true)
      const b = c.begin('u', true)
      await Promise.all([a, b])
      eq('拉流：重复点击不重复 Start', log.filter((x) => x.startsWith('start')).length, 1)
      await c.end() // 卸载
      eq('拉流：卸载（离开页面）→ Stop', log.filter((x) => x.startsWith('stop')), ['stop:ps1'])
    }
    {
      const { log, gate, c } = mkPull({ startMs: 1 })
      const a = c.begin('u', true)
      await c.end() // Start 还没返回就点停止 / 离开
      gate.release?.()
      await a
      eq('拉流：Start 在途时被取消 → 返回后补 Stop，且不进入 active', [log, c.state], [['start:u', 'stop:ps1'], 'idle'])
    }
    {
      const { log, c } = mkPull({ failStop: true })
      await c.begin('u', true)
      const err = await rejects(c.end())
      eq('拉流：Stop 自己出错不抛给界面，也算调用过', [err, log], [null, ['start:u', 'stop:ps1']])
    }
    {
      const { log, c } = mkPull({ preview: false })
      await c.begin('u', true)
      eq('拉流：后端说不出预览（如纯音频）→ 该会话仍 Stop，状态 off', [log, c.state], [['start:u', 'stop:ps1'], 'off'])
    }
    {
      const { log, c } = mkPull()
      await c.begin('u', false)
      eq('拉流：开关关 → 不调后端', [log, c.state], [[], 'off'])
    }
    {
      const { log, c } = mkPull({ failStart: true })
      await c.begin('u', true)
      eq('拉流：Start 出错 → failed，没有会话可 Stop', [c.state, log], ['failed', ['start:u']])
      await c.end()
      eq('拉流：failed 后 end 不调 Stop', log.length, 1)
    }

    // ---- 包 21：预览开关和会话的同步（真实 store，假行） ----
    {
      setActivePinia(createPinia())
      const { useLiveSessionsStore } = await import('@/stores/liveSessions')
      const { useLiveDockStore } = await import('@/stores/liveDock')
      const { nextTick } = await import('vue')
      const ss = useLiveSessionsStore()
      const dk = useLiveDockStore()
      const row = (id: string, status: 'run' | 'int' | 'ok', preview = true) => ({ id, kind: 'file' as const, url: 'rtmp://h/live/****', status, archive: false, outputPath: '', startedAt: 1, endedAt: 0, bitrateKbps: null, preview })
      ss.rows.push(row('a', 'run'), row('b', 'run'), row('c', 'int'), row('d', 'ok'))
      await nextTick()
      eq('包 21：面板 / 角标只数进行中的', [ss.activeRows.map((r) => r.id), ss.busyCount], [['a', 'b'], 2])
      eq('包 21：当前会话默认是第一个进行中的', ss.current?.id, 'a')
      dk.previewOn = false
      await nextTick()
      eq('包 21：右栏开关关 → 只关当前这一路', [ss.rows[0].preview, ss.rows[1].preview], [false, true])
      ss.setPreview('a', true)
      await nextTick()
      eq('包 21：面板里拨当前这一路 → 右栏开关跟上', dk.previewOn, true)
      ss.setPreview('b', false)
      await nextTick()
      eq('包 21：拨别的那一路 → 右栏开关不动，当前这一路不动', [dk.previewOn, ss.rows[0].preview], [true, true])
      ss.selectPreview('b')
      await nextTick()
      eq('包 21：换当前会话 → 右栏开关显示那一路的值', dk.previewOn, false)
      ss.rows[0].status = 'int'
      ss.rows[1].status = 'ok'
      await nextTick()
      eq('包 21：没有进行中的会话 → 开关复位为开，角标 0', [dk.previewOn, ss.busyCount], [true, 0])
      const { sessionBadge } = await import('@/stores/liveDock')
      eq('包 21 拉流页：只有进行中显示「停止播放」，被中断 / 结束 / 不支持都是「开始播放」（场景 17，10-08 改）', /<LiveButton v-if="busy" icon="x" @click="stop">停止播放<\/LiveButton>\s*<LiveButton v-else variant="pri" icon="play" @click="start">开始播放<\/LiveButton>/.test((await import("node:fs")).readFileSync(`${process.cwd()}/src/views/live/PullPlay.vue`, 'utf8')), true)
      eq('包 21：角标文字 0 路为空（不显示），其余是路数', [sessionBadge(ss.busyCount), sessionBadge(0), sessionBadge(1), sessionBadge(4)], ['', '', '1', '4'])
      dk.pull.active = true
      eq('包 21：拉流播放中角标 1，结束后不显示', [sessionBadge(dk.pull.active ? 1 : 0), (dk.resetPull(), sessionBadge(dk.pull.active ? 1 : 0))], ['1', ''])
    }

    // ---- v0.25 模拟层：不出 JPEG 帧 ----
    const stream = await import('./livePreviewStream')
    const denied = await rejects(stream.getPreviewStream('any'))
    eq('v0.25 模拟：GetPreviewStream 尚未实现 → preview_unavailable', [denied?.code, denied?.reason], ['UNSUPPORTED', 'preview_unavailable'])
    eq('v0.25 模拟：拉流回放用户自己的地址', (await stream.startPullPlayback('https://pull.example/a.flv')).stream?.url, 'https://pull.example/a.flv')
    await stream.stopPullPlayback(null)
    eq('v0.25 模拟：ws / wss 地址前端直接拉（后端不收）', (await stream.startPullPlayback('wss://pull.example/a.flv')).session, null)
    eq('v0.25：开关已打开（联调），纯浏览器里仍走模拟', [(await import('./flags')).LIVE_PREVIEW_V25_BACKEND_READY, stream.previewV25IsReal()], [true, false])
    // 产品经理 10-08 定稿：拉流结束分两种
    eq('拉流结束：不是用户点停止（live:pull ended / 流读完）→ 标题 + 第二行 + 「重新拉流」', stream.pullEndedView(false), { title: '拉流已结束', note: '直播已停止，或连接已断开。', retry: true })
    eq('拉流结束：用户自己点「停止播放」→ 只有「拉流已结束」，没有第二行和按钮', stream.pullEndedView(true), { title: '拉流已结束', note: '', retry: false })
    {
      const fsx = (await import('node:fs')).readFileSync
      const pull = fsx(`${process.cwd()}/src/views/live/PullPlay.vue`, 'utf8')
      const player = fsx(`${process.cwd()}/src/components/live/LivePlayer.vue`, 'utf8')
      eq('拉流页（包 22 先到先定）：用户停止 → gate.user()；终态统一由 applyOutcome 按 byUser 走 pullEndedView', [/function stop\(\) \{\s*if \(!busy\.value\) return\s*gate\.user\(\)/.test(pull), /endedNote\.value = pullEndedView\(o\.byUser\)\.note/.test(pull), /gate\.event\(e\.state/.test(pull)], [true, true, true])
      eq('播放器：结束且有第二行时显示第二行和「重新拉流」（restart 用同一地址）', [/<small v-if="phase === 'ended' && endedNote" class="lp-sub">\{\{ endedNote \}\}<\/small>/.test(player), /v-if="phase === 'interrupted' \|\| \(phase === 'ended' && endedNote\)"/.test(player), /@restart="start"/.test(pull)], [true, true, true])
    }
    // 文案里没有编码器名，时间戳不进文案
    // ---- 包 24 ----
    {
      setActivePinia(createPinia())
      const { useLiveSessionsStore } = await import('@/stores/liveSessions')
      const { nextTick } = await import('vue')
      const ss = useLiveSessionsStore()
      const row = (id: string, status: 'run' | 'int' | 'ok' | 'cnl') => ({ id, kind: 'file' as const, url: 'rtmp://h/live/****', status, archive: false, outputPath: '', startedAt: 1, endedAt: 0, bitrateKbps: null })
      ss.rows.push(row('old', 'int'))
      await nextTick()
      eq('N1：最近一行被中断、没有进行中的 → 「重新推流」针对这一行', ss.retryRow?.id, 'old')
      ss.rows.unshift(row('new', 'run'))
      await nextTick()
      eq('N1：重新推流进行中 → 不显示「重新推流」', ss.retryRow, undefined)
      ss.rows[0].status = 'ok'
      await nextTick()
      eq('N1：重新推流后正常停止 → 回到「开始推流」（旧的中断行不再算）', ss.retryRow, undefined)
      ss.rows[0].status = 'int'
      await nextTick()
      eq('N1：新的一路又被中断 → 「重新推流」针对新的这一行', ss.retryRow?.id, 'new')
      const fsx = (await import('node:fs')).readFileSync
      for (const f of ['FilePush', 'RecordPush']) {
        const src = fsx(`${process.cwd()}/src/views/live/${f}.vue`, 'utf8')
        eq(`N1：${f} 只看 store.retryRow`, [/const showRetry = computed\(\(\) => !!store\.retryRow\)/.test(src), /const row = store\.retryRow/.test(src), /rows\.some\(\(r\) => r\.status === 'int'\)/.test(src)], [true, true, false])
      }
      const em = await import('@/errors/errorMessages')
      eq('Q1：未知码 + detail 只有 reason=whatever → 出了点问题，请重试。，界面文字不含错误码和 reason=', (() => {
        const view = em.renderTaskError('NO_SUCH_CODE', '', 'reason=whatever', 'convert')
        return [view.description, view.text.includes('NO_SUCH_CODE'), view.text.includes('reason='), em.publicErrorText('reason=whatever'), em.actionErrorText('NO_SUCH_CODE', 'reason=whatever')]
      })(), ['出了点问题，请重试。', false, false, '出了点问题，请重试。', '出了点问题，请重试。'])
      eq('Q1：已知码仍用映射文案，不带错误码', (() => {
        const view = em.renderTaskError('CONVERT_DISK_FULL', '', 'reason=no_space', 'convert')
        return [view.description, view.text.includes('CONVERT_DISK_FULL'), view.text.includes('reason=')]
      })(), ['输出位置的可用空间不够，请清理空间或换一个输出文件夹。', false, false])
      eq('N4：没有专属文案时标题按任务类型', [em.resolveTaskError('INTERNAL', '推流异常退出', 'live_file_push').title, em.resolveTaskError('INTERNAL', 'x', 'live_screen_push').title, em.resolveTaskError('INTERNAL', 'x', 'live_pull').title, em.resolveTaskError('INTERNAL', 'x', 'convert').title, em.resolveTaskError('INTERNAL', 'x').title, em.resolveTaskError('PROCESS_FAILED', 'x', 'edit_export').title], ['推流失败', '推流失败', '拉流失败', '转换失败', '转换失败', '导出失败'])
      const tc = fsx(`${process.cwd()}/src/views/TaskCenter.vue`, 'utf8')
      eq('N4：直播任务意外退出（failed + INTERNAL 等）按「已中断 / 推流被中断」显示，不显示错误码', [/const LIVE_EXIT_CODES = new Set\(\['', 'INTERNAL', 'PROCESS_FAILED', 'LIVE_PUSH_INTERRUPTED'\]\)/.test(tc), /v-else-if="t\.error && liveBroken\(t\)"[\s\S]{0,240}:title="interruptOf\(t\)\.title"[\s\S]{0,160}hide-code/.test(tc), /:task-type="t\.type"/.test(tc)], [true, true, true])
      eq('N3：旧记录类型叫「旧版导出」，中断说明不提已下线的功能名', [/edit_export: '旧版导出'/.test(tc), /应用退出时这个任务被中断。这类任务已不再支持，不能重试，可以移除这条记录。/.test(tc)], [true, true])
      const lp = await import('@/errors/livePreviewMessages')
      eq('N6：直播页上推流被中断的正文；拉流不变', [lp.LP_BREAK_PUSH, lp.LP_BREAK_PULL], ['推流被中断，请重新推流。', '拉流被中断，请重新拉流。'])
      eq('N2：只有声音时的说明（不带句号）', lp.LP_AUDIO_ONLY, '这路直播只有声音')
      const player = fsx(`${process.cwd()}/src/components/live/LivePlayer.vue`, 'utf8')
      const pullSrc = fsx(`${process.cwd()}/src/views/live/PullPlay.vue`, 'utf8')
      const lps = fsx(`${process.cwd()}/src/api/livePreviewStream.ts`, 'utf8')
      eq('N2：播放器不再写死 hasVideo: true；纯音频不给全屏', [/hasVideo: props\.hasVideo \? undefined : false \}/.test(player), /hasVideo: true \}/.test(player), /<button v-if="!audioOnly" type="button" class="lp-btn" :aria-label="fullOn/.test(player)], [true, false, true])
      eq('N2：拉流读后端的 hasVideo / hasAudio（PullSession、live:pull playing），没有时用 GetPreviewStream 兜底', [/hasVideo: s\.hasVideo \?\? true, hasAudio: s\.hasAudio \?\? true/.test(lps), /if \(e\.state === 'playing'\) return void applyMedia\(e\)/.test(pullSrc), /scheduleMediaProbe\(pb\.session\.id\)/.test(pullSrc)], [true, true, true])
      const set = fsx(`${process.cwd()}/src/views/Settings.vue`, 'utf8')
      const fp = fsx(`${process.cwd()}/src/components/settings/FFmpegPanel.vue`, 'utf8')
      eq('N5：回退时「恢复默认」在「打开组件所在文件夹」左边；坏了且没有能用的组件时只有「手动指定」「恢复默认」', [/v-if="ffmpeg\.customFellBack"[^\n]*恢复默认<\/button>\s*<button[^\n]*打开组件所在文件夹/.test(set), /#custom-actions>\s*<div class="facts">\s*<button[^\n]*手动指定<\/button>\s*<button[^\n]*恢复默认<\/button>/.test(set), /status\.source === 'custom'/.test(set)], [true, true, false])
      eq('N5：两句文案', [fp.includes('手动指定的转换组件不可用，已改用默认组件。'), fp.includes('转换组件未就绪</span>'), fp.includes('<small>手动指定的转换组件不可用。</small>')], [true, true, true])
    }
    eq('预览文案锁定（待产品经理确认的自拟部分除外）', [pvMsg.PREVIEW_LOADING_TITLE, pvMsg.PREVIEW_SWITCH_LABEL, pvMsg.PREVIEW_SWITCH_NOTE, pvMsg.PREVIEW_ROW_ON, pvMsg.PREVIEW_ROW_OFF, pvMsg.PREVIEW_OFF_TITLE], ['正在获取画面，通常需要几秒', '开启预览', '开启预览会多占用少量 CPU', '预览：开', '预览：关', '该会话未开启预览'])
  // ---- v0.26 文档转换文案（设计 v0.2 §六；数字只从后端字段来）----
  eq('v0.26 大小句：installBytes>0 带安装后占用', d26.docSizeGuideText(373_252_096, 1_610_612_736), '需要下载约 356 MB，安装后约占用 1.5 GB 磁盘空间。')
  eq('v0.26 大小句：installBytes=0 不写安装后占用', d26.docSizeGuideText(373_252_096, 0).includes('安装后'), false)
  eq('v0.26 大小句：downloadBytes=0 不出句子', d26.docSizeGuideText(0, 0), '')
  eq('v0.26 磁盘不够：needBytes 来自 detail，不显示 reason=', d26.docDiskFullText('reason=no_space\nneedBytes=2520000000\nfreeBytes=100', 'x'), '磁盘空间不够，至少需要 2.4 GB 可用空间。')
  eq('v0.26 密码 / 坏文件不可重试，超时 / 崩溃可重试', ['DOC_ENCRYPTED', 'DOC_CORRUPT', 'DOC_TIMEOUT', 'DOC_COMPONENT_CRASHED'].map((c) => d26.docErrorRetryable(c)), [false, false, true, true])
  eq('v0.26 未映射码 → 兜底，不出现错误码', d26.docErrorText('NO_SUCH', ''), '出了点问题，请重试。')
  eq('v0.26 Linux 组件未就绪', d26.docErrorText('DOC_COMPONENT_NOT_READY', '', true), '请先在系统里安装 LibreOffice，然后重启应用。')
  eq('v0.26 排队文案', [d26.DOC_QUEUE_AHEAD(0), d26.DOC_QUEUE_AHEAD(2), d26.DOC_QUEUE_LINE(0), d26.DOC_QUEUE_LINE(3)], ['排队中 · 下一个', '排队中 · 前面还有 2 项', '下一个', '前面还有 3 项'])
  eq('v0.27 被占用两码可重试，文案为转换版', [d26.docErrorText('DOC_PRESENTATION_BUSY', ''), d26.docErrorText('DOC_ENGINE_BUSY', ''), d26.docErrorRetryable('DOC_PRESENTATION_BUSY'), d26.docErrorRetryable('DOC_ENGINE_BUSY')], ['请先关闭正在打开的演示文稿，再转换。', '请先关闭正在打开的文档，再转换。', true, true])
  eq('v0.27 结果警告 simple_fallback；未知码不显示', d26.docResultWarnings(['simple_fallback', 'whatever']), ['这次是简易转换，只保留了文字。可以稍后重转。'])
  eq('CSV 说明用产品最终定稿句', d26.DOC_CSV_HINT, '转成 CSV 只会保留第一个工作表。')
  eq('Win/mac 组件太旧文案 + 按钮', [d26.DOC_OUTDATED_DOWNLOAD, d26.DOC_OUTDATED_BUTTON], ['文档组件版本太旧，请重新下载。', '更新文档组件'])
  // ---- v0.27 预览 / 编辑 / 引擎文案（6.12.28~6.12.52；设计 v0.3）----
  eq('v0.27 引擎使用行', [d27.engineInUseText('office'), d27.engineInUseText('wps'), d27.engineInUseText('downloaded'), d27.engineInUseText('system'), d27.engineInUseText('')], ['正在使用本机 Microsoft Office', '正在使用本机 WPS', '正在使用文档组件', '正在使用文档组件', ''])
  eq('v0.27 记录引擎行（go/simple 不写）', [d27.engineRecordText('office'), d27.engineRecordText('wps'), d27.engineRecordText('component'), d27.engineRecordText('go'), d27.engineRecordText('simple'), d27.engineRecordText(undefined)], ['由本机 Microsoft Office 转换', '由本机 WPS 转换', '由文档组件转换', '', '', ''])
  eq('v0.27 预览失败只认三码', [d27.previewFailedNotice('DOC_ENCRYPTED').text, d27.previewFailedNotice('DOC_PRESENTATION_BUSY'), d27.previewFailedNotice('DOC_ENGINE_BUSY'), d27.previewFailedNotice('DOC_CORRUPT').text, d27.previewFailedNotice('x').text], [d27.PV_ENCRYPTED, { text: d27.PV_PRESENTATION_BUSY, retry: true }, { text: d27.PV_ENGINE_BUSY, retry: true }, d27.PV_FAILED, d27.PV_FAILED])
  eq('v0.27 unavailable：Win needs / too_large / outdated；Linux 无按钮', [
    d27.previewUnavailableNotice('needs_component', { linux: false, outdated: false }),
    d27.previewUnavailableNotice('too_large_for_simple', { linux: false, outdated: false }),
    d27.previewUnavailableNotice('needs_component', { linux: false, outdated: true }),
    d27.previewUnavailableNotice('needs_component', { linux: true, outdated: false }),
    d27.previewUnavailableNotice('too_large_for_simple', { linux: true, outdated: false }),
  ], [
    { text: d27.PV_NEEDS_COMPONENT, download: d27.PV_DOWNLOAD_BUTTON },
    { text: d27.PV_TOO_LARGE_SIMPLE, download: d27.PV_DOWNLOAD_BUTTON },
    { text: d26.DOC_OUTDATED_DOWNLOAD, download: d26.DOC_OUTDATED_BUTTON },
    { text: d26.DOC_LINUX_MISSING },
    { text: '文件太大，没法简易预览。' },
  ])
  eq('v0.27 unavailable：已有可用引擎时不叫用户下载，只说暂时无法预览（Win / Linux）', [
    d27.previewUnavailableNotice('needs_component', { linux: false, outdated: false, engineReady: true }),
    d27.previewUnavailableNotice('too_large_for_simple', { linux: false, outdated: true, engineReady: true }),
    d27.previewUnavailableNotice('needs_component', { linux: true, outdated: false, engineReady: true }),
  ], [{ text: d27.PV_FAILED }, { text: d27.PV_FAILED }, { text: d27.PV_FAILED }])
  eq('v0.27 无 reason 的 INVALID_ARGUMENT：只有应用目录那句按 message 认，其它都是「出了点问题」', [
    d27.saveErrorView('INVALID_ARGUMENT', undefined, 'textAs', '不能保存到应用自己的文件夹里，请换一个位置。').text,
    d27.saveErrorView('INVALID_ARGUMENT', undefined, 'docxAs', '出了点问题，请重试。').text,
    d27.saveErrorView('INVALID_ARGUMENT', undefined, 'textAs', '不支持的编码').text,
    d27.saveErrorView('INVALID_ARGUMENT', undefined, 'text', '缺少版本信息').text,
    d27.saveErrorView('INVALID_ARGUMENT', 'chunk_order', 'docx').text,
    d27.saveErrorView('IO_ERROR', 'in_use', 'textAs'),
    d27.saveErrorView('INVALID_ARGUMENT', 'encoding', 'text'),
    d27.saveErrorView('IO_ERROR', 'in_use', 'textAs', d27.SAVE_AS_IN_USE_BY_RECORD_TEXT),
    d27.saveErrorView('TASK_CONFLICT', 'converting', 'docxAs'),
  ], [d27.SAVE_APP_DIR_TEXT, d27.UNMAPPED, d27.UNMAPPED, d27.UNMAPPED, d27.UNMAPPED, { text: '文件正被其他程序占用，请关闭后再保存。', actions: ['retry'] }, { text: '有些字符没法按原编码保存，请另存为 UTF-8。', actions: ['saveAsUtf8'] }, { text: d27.SAVE_AS_IN_USE_BY_RECORD_TEXT, actions: [] }, { text: d27.EDIT_CONVERTING_TIP, actions: [] }])
  eq('v0.27 保存出错：file_changed 另存为优先；in_use；backup；converting 用「转完」', [
    d27.saveErrorView('TASK_CONFLICT', 'file_changed', 'text'),
    d27.saveErrorView('IO_ERROR', 'in_use', 'text'),
    d27.saveErrorView('IO_ERROR', 'backup', 'docx'),
    d27.saveErrorView('TASK_CONFLICT', 'converting', 'text').text,
    d27.saveErrorView('INVALID_ARGUMENT', 'checksum', 'docx').text,
    d27.saveErrorView('NOT_FOUND', 'save_session', 'docx').text,
  ], [
    { text: '文件在别处被改过了，请重新打开，或另存为。', actions: ['reopen', 'saveAs'] },
    { text: '文件正被其他程序占用，请关闭后再保存。', actions: ['retry', 'saveAs'] },
    { text: '没法在这个文件夹留备份，文件没有保存。请另存为。', actions: ['saveAs'] },
    d27.EDIT_CONVERTING_TIP,
    d27.UNMAPPED,
    d27.UNMAPPED,
  ])
  eq('v0.27 重新打开确认（场景 35b，不走未保存确认）', d27.REOPEN_DISCARD_CONFIRM, '重新打开会丢掉你改的内容，确定吗？')
  eq('v0.27 editBlock 悬停', [d27.editBlockTip('format', 'xlsx'), d27.editBlockTip('too_large', 'md'), d27.editBlockTip('macro', 'docx'), d27.editBlockTip('malformed', 'csv')], [d27.EDIT_FORMAT_TIP, d27.EDIT_TOO_LARGE_TIP, d27.EDIT_MACRO_TIP, d27.EDIT_MALFORMED_CSV_TIP])
  // CSP / srcdoc（6.12.32.4 / 6.12.44）：sandbox 必须为空；CSP 是 srcdoc 第一个 meta
  const srcdocHtml = buildSrcdoc('<p>hi</p><script>x</script>', { title: 't' })
  eq('srcdoc：CSP 是第一个 meta，sandbox 属性值为空', [srcdocHtml.indexOf('<meta http-equiv="Content-Security-Policy"'), srcdocHtml.includes(PREVIEW_CSP), PREVIEW_SANDBOX, /sandbox="[^"]+"/.test('<iframe sandbox="">') === false ? PREVIEW_SANDBOX === '' : false], [srcdocHtml.indexOf('<head>') + 6, true, '', true])
  eq('srcdoc：不带网络资源允许项', [/script-src|connect-src|frame-src/.test(PREVIEW_CSP), PREVIEW_CSP.includes("default-src 'none'"), PREVIEW_CSP.includes('img-src data:')], [false, true, true])
  eq('extractHeadStyles：保留 style，去掉 head 其它', extractHeadStyles('<html><head><meta charset=utf-8><style>.a{color:red}</style><script>1</script></head><body><p>x</p></body></html>'), '<style>.a{color:red}</style><p>x</p>')
  // Range 分段 / base64 / 同格式
  eq('chunkRanges：每段 ≤ max，覆盖整段', chunkRanges(10_000_000, 4 * 1024 * 1024), [[0, 4194304], [4194304, 8388608], [8388608, 10000000]])
  eq('bytesToBase64', bytesToBase64(new Uint8Array([72, 105])), btoa('Hi'))
  // ---- v0.28 PDF 输入（6.12.58~6.12.65；产品 10-09 定稿）----
  eq('v0.28 定稿文案', [d26.DOC_PDF_LAYOUT_HINT, d26.DOC_PDF_TEXT_ONLY_HINT, d26.DOC_PDF_NO_TEXT_TEXT, d26.DOC_PDF_TOO_LARGE_TEXT, d26.DOC_PDF_TOO_MANY_PAGES_TEXT], ['PDF 转 Word 会尽量保留排版，复杂版式和扫描件可能会走样。', '只提取文字，不保留排版和图片。', '这个 PDF 里没有能提取的文字，可能是扫描件。', 'PDF 太大了，最多支持 200 MB。', 'PDF 页数太多，最多支持 500 页。'])
  eq('v0.28 DOC_PDF_NO_TEXT 不可重试', [d26.docErrorText('DOC_PDF_NO_TEXT', '后端原句'), d26.docErrorRetryable('DOC_PDF_NO_TEXT')], [d26.DOC_PDF_NO_TEXT_TEXT, false])
  eq('v0.28 旧句「PDF 暂时不能…」已删', d26.docErrorText('DOC_PDF_INPUT_UNSUPPORTED', 'PDF 暂时不能转成其他格式。'), '不支持这种文件。')
  eq('v0.28 添加 PDF：too_large / too_many_pages 用定稿句，不出现 reason=；非 PDF 的 too_large 用后端 message；密码沿用', [
    d26.docAddErrorText({ code: 'INVALID_ARGUMENT', message: 'PDF 太大了，最大 200 MB。', detail: 'reason=too_large' }, 'D:/a/手册.PDF'),
    d26.docAddErrorText({ code: 'INVALID_ARGUMENT', message: 'x', detail: 'reason=too_many_pages\n512' }, 'D:/a/b.pdf'),
    d26.docAddErrorText({ code: 'INVALID_ARGUMENT', message: '文件超过 100 MB', detail: 'reason=too_large' }, 'D:/a/b.docx'),
    d26.docAddErrorText({ code: 'DOC_ENCRYPTED', message: '' }, 'D:/a/b.pdf'),
  ], [d26.DOC_PDF_TOO_LARGE_TEXT, d26.DOC_PDF_TOO_MANY_PAGES_TEXT, '文件超过 100 MB', '这个文件有密码保护，不能转换。请先去掉密码再添加。'])
  eq('v0.28 PDF 说明行', [
    d26.docPdfNote(['pdf'], { ext: 'docx', available: true, hintKey: 'pdf_layout' })?.text,
    d26.docPdfNote(['pdf'], { ext: 'txt', available: true, simple: true })?.text,
    d26.docPdfNote(['pdf'], { ext: 'md', available: true, simple: true, hintKey: 'md_lossy' })?.text, // 旧后端兜底：PDF 源上永不显示 md_lossy 那句
    d26.docPdfNote(['pdf'], { ext: 'md', available: true, simple: true, hintKey: 'pdf_text' })?.text,
    d26.docPdfNote(['pdf'], { ext: 'html', available: true, simple: true })?.download,
    d26.docPdfNote(['pdf'], { ext: 'html', available: true, simple: true }, true)?.download,
    d26.docPdfNote(['pdf'], { ext: 'html', available: true, simple: false }),
    d26.docPdfNote(['text'], { ext: 'docx', available: true }),
  ], [d26.DOC_PDF_LAYOUT_HINT, d26.DOC_PDF_TEXT_ONLY_HINT, d26.DOC_PDF_TEXT_ONLY_HINT, d26.DOC_PDF_TEXT_ONLY_HINT, true, false, null, null])
  eq('v0.28 hintKey 映射：pdf_text → 只提取文字；md_lossy 仍是 Markdown 那句（非 PDF）', [
    d26.docHintText('pdf_text'),
    d26.docHintText('md_lossy'),
    d26.docHintText('pdf_layout'),
  ], [d26.DOC_PDF_TEXT_ONLY_HINT, d26.DOC_MD_HINT, d26.DOC_PDF_LAYOUT_HINT])
  eq('v0.28 组件未就绪横条：开关关时原句，开时加 PDF 半句', [d26.docHintSimpleBar(false), d26.docHintSimpleBar(true) + '。'], ['Word、ODT、TXT 可以简易转 PDF，md 和网页可以互转', 'Word、ODT、TXT 可以简易转 PDF，md 和网页可以互转，PDF 可以提取文字转成 TXT、md 和简易网页。'])
  eq('v0.28 DOC_ENCRYPTED owner_only 单独一句、不可重试；其他密码沿用；PDF 太大 / 页数太多不可重试', [
    d26.docAddErrorText({ code: 'DOC_ENCRYPTED', message: 'x', detail: 'reason=owner_only' }, 'D:/a/b.pdf'),
    d26.docErrorText('DOC_ENCRYPTED', 'x', false, 'reason=owner_only'),
    d26.docErrorText('DOC_ENCRYPTED', 'x', false, 'reason=user_password'),
    d26.docErrorRetryable('DOC_ENCRYPTED', 'reason=owner_only'),
    d26.docErrorRetryable('INVALID_ARGUMENT', 'reason=too_large'),
    d26.docErrorRetryable('INVALID_ARGUMENT', 'reason=too_many_pages\npages=600'),
  ], ['这个 PDF 设置了权限保护，暂时不能转换。', '这个 PDF 设置了权限保护，暂时不能转换。', '这个文件有密码保护，不能转换。请先去掉密码再添加。', false, false, false])
  eq('v0.28 交集说明含 PDF', d26.intersectionWhy(['pdf', 'text']), '选中的文件有PDF和文档两类，只显示它们都能转的格式。')
  {
    const m = await (await import('@/api/docV26')).getFormatMatrix()
    const pdf = m.sources.find((x) => x.ext === 'pdf')
    eq('v0.28 模拟格式表：pdf 家族、7 个目标、没有表格 / 演示 / 图片 / pdf', [m.inputs.includes('pdf'), pdf?.family, pdf?.targets.map((t) => t.ext)], [true, 'pdf', ['doc', 'docx', 'odt', 'rtf', 'txt', 'html', 'md']])
    eq('v0.28 模拟（没有组件、没有 Word）：Word 类置灰，txt / md / html 可用', pdf?.targets.map((t) => t.available), [false, false, false, false, true, true, true])
    eq('v0.28 状态 engines 里没有 go', (await (await import('@/api/docV26')).getDocComponentStatus()).engines.some((e) => e.id === 'go'), false)
    eq('v0.28 pdf → txt / md / html（没有组件）：不需要组件、简易', pdf?.targets.filter((t) => ['txt', 'md', 'html'].includes(t.ext)).map((t) => [t.needsComponent, t.simple]), [[false, true], [false, true], [false, true]])
    eq('v0.28 模拟：pdf → txt / md 用 pdf_text；html 简易仍 simple_mode；md_lossy 不出现在 PDF 源', [
      pdf?.targets.find((x) => x.ext === 'txt')?.hintKey,
      pdf?.targets.find((x) => x.ext === 'md')?.hintKey,
      pdf?.targets.find((x) => x.ext === 'html')?.hintKey,
      pdf?.targets.some((x) => x.hintKey === 'md_lossy'),
    ], ['pdf_text', 'pdf_text', 'simple_mode', false])
    const added = await (await import('@/api/docV26')).addDocSources(['D:/a/说明书.pdf', 'D:/a/超大.pdf', 'D:/a/页数多.pdf'])
    eq('v0.28 模拟添加 PDF', [added[0].source?.family, added[1].error?.detail, added[2].error?.detail], ['pdf', 'reason=too_large', 'reason=too_many_pages'])
  }
  eq('另存为同格式', [isSameFormat('md', 'markdown'), isSameFormat('html', 'htm'), isSameFormat('docx', 'doc'), saveFilters('csv')[0].patterns], [true, true, false, ['*.csv']])

  return fails
}
