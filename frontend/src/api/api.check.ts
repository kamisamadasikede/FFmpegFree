// 接口层自检（不引入测试框架）：node scripts/check-api.mjs 用 esbuild 打包后运行，失败退出码 1。
// 覆盖：AppError 的 reason / clipId 解析、TASK_CONFLICT 文案表、推流地址校验与脱敏、Edit 同轨道重叠 / 输出名净化 / 结构校验、
// 模拟层（Live 两种 TASK_CONFLICT、停止语义、Doc 的 UNSUPPORTED、Edit 的 clip 错误）。
import { AppError, parseDetailHead, toAppError, BACKEND_ERROR_CODES, callService } from './call'
import { toApiTask, type TaskProgressPayload, type TaskStatusPayload, type ApiTask } from './taskTypes'
import {
  taskConflictText, TASK_CONFLICT_GENERIC, actionErrorText, liveStartErrorLine, LIVE_STOP_TEXT, docUnsupportedText, errorMessages, taskErrorMessages,
  liveUrlInvalidText, LIVE_URL_INVALID_GENERIC, LIVE_URL_INVALID_REASON_TEXT, liveFailureMessage, liveConnectFailedText, LIVE_SRT_CONNECT_FAILED_TEXT, LIVE_RTMP_CONNECT_FAILED_TEXT, schemeFromParams,
  liveFfmpegProtocolMissingText, hasMissingLine, LIVE_CANCELED_ARCHIVE_KEPT_TEXT, LIVE_STOPPING_TEXT, LIVE_PUSH_REJECTED_TEXT, LIVE_SRT_PASSPHRASE_TEXT, resolveError, resolveTaskError,
} from '@/errors/errorMessages'
import { parsePushUrl, redactPushUrl } from '@/utils/liveUrl'
import * as live from './live'
import * as edit from './edit'
import * as doc from './doc'
import { docErrorText, docErrorFile, docErrorPath, docDetailHead, pdfErrorView, DOC_TOO_MANY_PAGES_TEXT, DOC_FILE_BROKEN_TEXT, DOC_FORMAT_UNSUPPORTED_TEXT } from '@/errors/errorMessages'
import { splitMiddle, nextZoom, thumbWindow, formatRecentTime, extBadge } from '@/utils/docLogic'
import { onSimEvent } from '@/services/wails'
import { retrySimTask, SIM_TITLE_PREFIX } from './sim'
import * as encApi from './encoder'
import { deriveEncoderView } from './encoderView'
import * as encMsg from '@/errors/encoderMessages'
import { pushErrorToForm } from '@/views/live/pushErrors'
import { elapsedMs, isKnownTaskType, isLegacyTaskType } from '@/stores/tasks'

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

export async function runApiChecks(): Promise<string[]> {
  // ---- AppError：reason / clipId ----
  eq('reason=max_sessions', new AppError('TASK_CONFLICT', 'x', 'reason=max_sessions\n最多同时进行 4 个').reason, 'max_sessions')
  eq('reason=duplicate_url', new AppError('TASK_CONFLICT', 'x', 'reason=duplicate_url').reason, 'duplicate_url')
  eq('没有 reason', new AppError('TASK_CONFLICT', 'x', '任务已经结束').reason, undefined)
  eq('reason 不在首行不算', new AppError('TASK_CONFLICT', 'x', '说明\nreason=max_sessions').reason, undefined)
  eq('clip 首行', parseDetailHead('clip=c_1-a path=/a b/中文.mp4\n原因'), { clipId: 'c_1-a', path: '/a b/中文.mp4' })
  eq('project 首行没有 clip', parseDetailHead('project\n视频轨不能为空'), {})
  eq('toAppError 解析 JSON 的 detail', toAppError('{"code":"TASK_CONFLICT","message":"m","detail":"reason=duplicate_url"}').reason, 'duplicate_url')
  eq('BACKEND_ERROR_CODES 18 个（v0.14 含 LIVE_SOURCE_GONE）', BACKEND_ERROR_CODES.length, 18)
  eq('LIVE_SOURCE_GONE 的 kind 首行', [new AppError('LIVE_SOURCE_GONE', 'x', 'kind=window').kind, new AppError('LIVE_SOURCE_GONE', 'x', 'kind=screen').kind, new AppError('LIVE_SOURCE_GONE', 'x', 'kind=other').kind, new AppError('LIVE_SOURCE_GONE', 'x').kind], ['window', 'screen', undefined, undefined])

  // ---- TASK_CONFLICT 文案（reason → 文案 一张表）----
  eq('max_sessions 文案', taskConflictText('max_sessions'), '最多同时推 4 路')
  eq('duplicate_url 文案', taskConflictText('duplicate_url'), '这个地址已经在推流')
  eq('未知 reason → 通用', taskConflictText('single_screen_only'), TASK_CONFLICT_GENERIC)
  eq('缺失 reason → 通用', taskConflictText(undefined), '操作冲突，请稍后再试')
  eq('Cancel / Remove 的冲突 → 通用', actionErrorText('TASK_CONFLICT', '任务已结束'), '操作冲突，请稍后再试')
  eq('原型链上的键不算 reason', taskConflictText('toString'), TASK_CONFLICT_GENERIC)
  eq('直播停止文案', LIVE_STOP_TEXT, { succeeded: '已结束推流', canceled: '已强制停止' })
  eq('UNSUPPORTED 起始错误行（无协议名）', liveStartErrorLine({ code: 'UNSUPPORTED' })?.description, '当前 ffmpeg 不支持这种推流协议，请在设置的 ffmpeg 页面重新安装或更新')

  eq('doc/xls/ppt/加密 UNSUPPORTED 文案', docUnsupportedText('暂不支持这种格式', '/d/a.doc\n.doc：旧版'), '暂不支持这种格式，请先另存为 docx、xlsx 或 pptx')
  eq('超 5000 页任务中心沿用后端 message', docUnsupportedText('超过 5000 页', '已排到第 5000 页仍未结束'), '超过 5000 页')
  eq('没有可用字体沿用后端 message', docUnsupportedText('没有可用的 Unicode 字体', '文档含有 Latin-1 以外的字符，但没有可用的字体'), '没有可用的 Unicode 字体')
  eq('认不出的 UNSUPPORTED 保守回落后端 message', docUnsupportedText('别的原因', '某行'), '别的原因')
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

  // ---- Edit：同轨道重叠 / 输出名 / 结构 ----
  const vc = (id: string, trackId: string, startSec: number, inSec: number, outSec: number, speed = 1): edit.VideoClip => ({
    id, path: '/m/a.mp4', trackId, startSec, inSec, outSec, speed, effectPreset: 'none', transitionToNext: 'none', transitionDurationSec: 0, blur: 0,
  })
  eq('同轨重叠', edit.findTrackOverlap([vc('a', 'V1', 0, 0, 10), vc('b', 'V1', 5, 0, 10)]), { trackId: 'V1', a: 'a', b: 'b' })
  eq('首尾相接不算重叠', edit.findTrackOverlap([vc('a', 'V1', 0, 0, 10), vc('b', 'V1', 10, 0, 10)]), null)
  eq('不同轨道（画中画）不算重叠', edit.findTrackOverlap([vc('a', 'V1', 0, 0, 10), vc('b', 'V2', 5, 0, 10)]), null)
  eq('速度 2 时占用一半', edit.findTrackOverlap([vc('a', 'V1', 0, 0, 10, 2), vc('b', 'V1', 5, 0, 10)]), null)
  eq('放置检测 wouldOverlap', edit.wouldOverlap([vc('a', 'V1', 0, 0, 10)], vc('n', 'V1', 8, 0, 4)), 'a')
  eq('拖动自己不冲突', edit.wouldOverlap([vc('a', 'V1', 0, 0, 10)], vc('a', 'V1', 2, 0, 10)), null)
  eq('输出名允许中日韩', edit.sanitizeOutputName('周报：剪辑/第1版?'), '周报：剪辑第1版')
  eq('输出名保留设备名', edit.sanitizeOutputName('con'), '_con')
  eq('输出名保留设备名带扩展名', edit.sanitizeOutputName('CON.txt'), '_CON.txt')
  eq('输出名空 → 工程名', edit.sanitizeOutputName('', '我的工程'), '我的工程')
  eq('输出名全非法 → edit', edit.sanitizeOutputName('***'), 'edit')
  eq('输出名尾部点与空格', edit.sanitizeOutputName('abc. . '), 'abc')
  eq('输出名 100 字截断', [...edit.sanitizeOutputName('字'.repeat(150))].length, 100)
  const proj = (): edit.EditProject => ({ ...edit.newEditProject('工程A'), sources: ['/m/a.mp4'], videoTrack: [vc('c1', 'V1', 0, 0, 10)] })
  const okPlan = await edit.validateProject(proj())
  eq('Validate 时长', okPlan.durationSec, 10)
  const overlapProj = proj()
  overlapProj.videoTrack.push(vc('c2', 'V1', 5, 0, 10))
  let err = await rejects(edit.validateProject(overlapProj))
  eq('模拟：同轨重叠 INVALID_ARGUMENT', err?.code, 'INVALID_ARGUMENT')
  eq('模拟：clip 错误的 clipId / path', [err?.clipId, err?.path], ['c2', '/m/a.mp4'])
  const noVideo = proj()
  noVideo.videoTrack = []
  err = await rejects(edit.validateProject(noVideo))
  eq('模拟：视频轨为空 → project 首行', [err?.code, err?.clipId, err?.detail?.split('\n')[0]], ['INVALID_ARGUMENT', undefined, 'project'])
  // ---- 架构师决定 5：SaveProject 不查同轨重叠；outSec=0 一律 INVALID_ARGUMENT ----
  const saved = await edit.saveProject(overlapProj)
  eq('SaveProject 允许同轨重叠（草稿）', saved.id.startsWith('sim-proj-'), true)
  eq('Validate 仍然报同轨重叠', (await rejects(edit.validateProject(overlapProj)))?.code, 'INVALID_ARGUMENT')
  eq('Export 也报同轨重叠', (await rejects(edit.exportProject(overlapProj, { outputName: '', outputDir: '' })))?.code, 'INVALID_ARGUMENT')
  await edit.deleteProject(saved.id)
  const zeroOut = proj()
  zeroOut.videoTrack[0].outSec = 0
  err = await rejects(edit.validateProject(zeroOut))
  eq('Validate：outSec=0 → INVALID_ARGUMENT 且定位 clip', [err?.code, err?.clipId], ['INVALID_ARGUMENT', 'c1'])
  eq('Export：outSec=0 → INVALID_ARGUMENT', (await rejects(edit.exportProject(zeroOut, { outputName: '', outputDir: '' })))?.code, 'INVALID_ARGUMENT')
  eq('Save：outSec=0 草稿可保存', (await rejects(edit.saveProject(zeroOut))), null)
  const eqIn = proj()
  eqIn.videoTrack[0].inSec = 5
  eqIn.videoTrack[0].outSec = 5
  eq('outSec == inSec → INVALID_ARGUMENT', (await rejects(edit.validateProject(eqIn)))?.code, 'INVALID_ARGUMENT')
  const filled = edit.newVideoClip({ path: '/m/a.mp4', durationSec: 42.5 })
  eq('素材加入 clip 填探测到的时长', [filled.inSec, filled.outSec, edit.CLIP_ID_RE.test(filled.id)], [0, 42.5, true])
  eq('素材时长未知不能加入', [(() => { try { edit.newVideoClip({ path: '/m/a.mp4', durationSec: 0 }); return 'ok' } catch (e) { return toAppError(e).code } })()], ['INVALID_ARGUMENT'])
  eq('音频 clip 填时长且 volume=1', ((c) => [c.outSec, c.volume])(edit.newAudioClip({ path: '/m/a.mp3', durationSec: 10 })), [10, 1])
  eq('fillOutSec 补 outSec=0', edit.fillOutSec(vc('z', 'V1', 0, 0, 0), 30).outSec, 30)
  eq('fillOutSec 不动合法值', edit.fillOutSec(vc('z', 'V1', 0, 0, 8), 30).outSec, 8)
  eq('outSec=0 的 clip 不再占时间线', edit.clipTimelineLength(vc('z', 'V1', 0, 0, 0)), 0)
  const badSpeed = proj()
  badSpeed.videoTrack[0].speed = 9
  err = await rejects(edit.validateProject(badSpeed))
  eq('模拟：speed 越界报错而不是截断', [err?.code, err?.clipId], ['INVALID_ARGUMENT', 'c1'])
  eq('Save 不查 speed 范围（只查数量上限）', await rejects(edit.saveProject(badSpeed)), null)
  const badId = proj()
  badId.videoTrack[0].id = 'a b'
  err = await rejects(edit.validateProject(badId))
  eq('模拟：clip id 字符集', err?.code, 'INVALID_ARGUMENT')
  const tooMany = proj()
  tooMany.videoTrack = Array.from({ length: 101 }, (_, i) => vc(`k${i}`, 'V1', i * 10, 0, 5))
  eq('Save 数量上限 101 个 clip → INVALID_ARGUMENT', (await rejects(edit.saveProject(tooMany)))?.code, 'INVALID_ARGUMENT')
  const tooManySrc = proj()
  tooManySrc.sources = Array.from({ length: 101 }, (_, i) => `/m/s${i}.mp4`)
  eq('Save 素材库上限 101 → INVALID_ARGUMENT', (await rejects(edit.saveProject(tooManySrc)))?.code, 'INVALID_ARGUMENT')
  eq('默认导出分辨率 1920×1080', [edit.newEditProject().output.width, edit.newEditProject().output.height], [1920, 1080])
  const meta = await edit.saveProject(proj())
  eq('SaveProject 新建返回 id', meta.id.startsWith('sim-proj-'), true)
  eq('LoadProject 缺失素材', (await edit.loadProject(meta.id)).missingPaths, [])
  eq('ListProjects', (await edit.listProjects()).length, 3) // 前面 Save 的草稿（outSec=0、speed 越界）也在列表里
  for (const m of await edit.listProjects()) if (m.id !== meta.id) await edit.deleteProject(m.id)
  eq('清理草稿后只剩一个', (await edit.listProjects()).length, 1)
  await edit.deleteProject(meta.id)
  eq('DeleteProject 不存在 NOT_FOUND', (await rejects(edit.deleteProject(meta.id)))?.code, 'NOT_FOUND')
  // 预览 404 → 重新取
  const ps = edit.createPreviewSource('/m/a.mp4')
  const u1 = await ps.load()
  eq('预览地址形态', u1.url.startsWith('/local/'), true)
  eq('预览 token 有效时 onMediaError 不重取', await ps.onMediaError(), null)
  win.location.search = '?sim_preview_404=1'
  const ps2 = edit.createPreviewSource('/m/a.mp4')
  const stale = await ps2.load()
  eq('模拟：第一次的 token 已失效(404)', await edit.isPreviewGone(stale.url), true)
  const fresh = await ps2.onMediaError()
  eq('404 后重新调用 GetPreviewURL 拿到新地址', [!!fresh, fresh?.url !== stale.url], [true, true])
  win.location.search = ''

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
  // 表单错误映射：LIVE_SOURCE_GONE 显示在来源选择器下方（where=source），窗口 / 屏幕文案，INVALID_ARGUMENT 沿用通用文案
  eq('表单错误：窗口消失', pushErrorToForm(new AppError('LIVE_SOURCE_GONE', 'm', 'kind=window')), { where: 'source', text: '所选窗口已不可用，请重新选择' })
  eq('表单错误：屏幕消失', pushErrorToForm(new AppError('LIVE_SOURCE_GONE', 'm', 'kind=screen')), { where: 'source', text: '所选屏幕已不可用，请重新选择' })
  eq('表单错误：缺 kind → 窗口版', pushErrorToForm(new AppError('LIVE_SOURCE_GONE', 'm')).text, '所选窗口已不可用，请重新选择')
  eq('表单错误：INVALID_ARGUMENT 沿用通用（form 级，不指向来源）', pushErrorToForm(new AppError('INVALID_ARGUMENT', 'captureSourceId 格式不对')).where, 'form')
  const gt = await live.startScreenPush({ ...screenReq('rtmp://g2.example/live/g2'), captureSourceId: 'window:131426' })
  eq('窗口来源的任务标题与 params', [gt.title.includes('记事本'), JSON.parse(gt.params).captureSourceId], [true, 'window:131426'])
  await live.stopPush(gt.id)
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
    eq(`UNSUPPORTED ${f}`, [err?.code, err?.detail?.split('\n')[0]], ['UNSUPPORTED', f])
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
  eq('doc 格式文案', docErrorText('UNSUPPORTED', '暂不支持这种格式', '.doc：旧版'), DOC_FORMAT_UNSUPPORTED_TEXT)
  eq('超 5000 页文案', docErrorText('UNSUPPORTED', '超过 5000 页', '已排到第 5000 页仍未结束'), DOC_TOO_MANY_PAGES_TEXT)
  eq('文件损坏文案', docErrorText('INVALID_ARGUMENT', '不是有效的 OOXML 文件', 'bad zip'), DOC_FILE_BROKEN_TEXT)
  eq('出错文件取 detail 首行', [docErrorFile('UNSUPPORTED', '/d/a.doc\n旧版'), docErrorFile('UNSUPPORTED', '旧版')], ['a.doc', ''])
  eq('出错文件完整路径（整体校验定位用）', [docErrorPath('/d/a.doc\n.doc：旧版'), docErrorPath('C:\\d\\a.doc\n原因'), docErrorPath('旧版')], ['/d/a.doc', 'C:\\d\\a.doc', ''])
  eq('原因首行剥掉路径行', [docDetailHead('/d/a.doc\n.doc：旧版'), docDetailHead('已排到第 9 页仍未结束'), docDetailHead('')], ['.doc：旧版', '已排到第 9 页仍未结束', ''])
  eq('超页数：按 message 精确匹配或 detail 首行（含带路径行）', [
    docErrorText('UNSUPPORTED', '超过 5000 页', '/d/a.docx\n已排到第 5000 页仍未结束'),
    docErrorText('UNSUPPORTED', '别的', '文档文字量超过上限'),
    docErrorText('UNSUPPORTED', '别的', '文本里写着 5000 页但不是这两句'),
  ], [DOC_TOO_MANY_PAGES_TEXT, DOC_TOO_MANY_PAGES_TEXT, '别的'])
  eq('页数上限取 limits 拼', docErrorText('UNSUPPORTED', '超过 5000 页', '', 8000), '文档太长，超过 8000 页，无法转换')
  eq('损坏：只认 INVALID_ARGUMENT + message 精确相等，不做包含匹配', [
    docErrorText('INVALID_ARGUMENT', '不是有效的 OOXML 文件', '/d/a.docx\nzip: not a valid zip file'),
    docErrorText('INVALID_ARGUMENT', '文件超过 100 MiB', 'OOXML 100 字节'),
    docErrorText('INVALID_ARGUMENT', '路径不合法'),
  ], [DOC_FILE_BROKEN_TEXT, '文件超过 100 MiB', '路径不合法'])
  eq('IO_ERROR 读源文件', docErrorText('IO_ERROR', '读取文件失败'), '没有读取这个文件的权限。')
  const pv = (c: string, m?: string) => pdfErrorView(c, m)
  eq('PDF 预览失败卡片：文案 / 错误码 / 是否可重试', [
    pv('PDF_PARSE_FAILED'), pv('INVALID_ARGUMENT', '不是 PDF 文件'), pv('INVALID_ARGUMENT', '文件超过 512 MiB'), pv('NOT_FOUND', '文件不存在'),
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
  eq('512 MiB 取 limits 拼', pdfErrorView('INVALID_ARGUMENT', '文件超过 512 MiB', 256 * 1024 * 1024).text, '文件超过 256 MiB，暂不支持预览。')
  eq('中间省略拆分：≤14 字符不拆；否则尾部 = 末 6 字符 + 扩展名', [
    splitMiddle('用户调研报告.docx'),
    splitMiddle('2026年第三季度华东区域渠道商务拓展与用户增长复盘汇报材料（终稿-已审阅-v12）.pptx'),
    splitMiddle('ab'),
  ], [{ head: '用户调研报告.docx', tail: '' }, { head: '2026年第三季度华东区域渠道商务拓展与用户增长复盘汇报材料（终稿-已审', tail: '阅-v12）.pptx' }, { head: 'ab', tail: '' }])
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
  const PROTO_GENERIC = '当前 ffmpeg 不支持这种推流协议，请在设置的 ffmpeg 页面重新安装或更新'
  const NAME = (n: string) => `当前的 ffmpeg 不支持 ${n}，请在设置的 ffmpeg 页面重新安装或更新`
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
  eq('UNSUPPORTED + missing=srt → 带协议名', liveStartErrorLine({ code: 'UNSUPPORTED', detail: 'missing=srt' })?.description, '当前的 ffmpeg 不支持 SRT，请在设置的 ffmpeg 页面重新安装或更新')
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
    eq('normalizeList：结果不含编码器名字段', Object.keys(n.devices[1]).sort(), ['available', 'id', 'kind', 'name', 'vendor'])

    // 显示规则：ENCODER_BACKEND_READY=false（默认）+ 纯浏览器 → 没有 ?enc= 完全不显示；有 ?enc= 才显示模拟层
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
    const allText = JSON.stringify(Object.values(encMsg).map((x) => (typeof x === 'function' ? (x as (...a: unknown[]) => string)(2, 'GPU') : x)))
    eq('编码设备文案不含编码器名（NVENC / QSV / AMF / VideoToolbox）', /nvenc|qsv|amf|videotoolbox|h264_|hevc_/i.test(allText), false)
    eq('回退文案锁定（设计稿，待产品经理确认）', [encMsg.ENCODER_FALLBACK_SETTINGS_LINK, encMsg.ENCODER_FALLBACK_LOG_LINK], ['编码设置', '查看日志'])
  }
  return fails
}
