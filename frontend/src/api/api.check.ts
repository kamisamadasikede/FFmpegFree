// 接口层自检（不引入测试框架）：node scripts/check-api.mjs 用 esbuild 打包后运行，失败退出码 1。
// 覆盖：AppError 的 reason / clipId 解析、TASK_CONFLICT 文案表、推流地址校验与脱敏、Edit 同轨道重叠 / 输出名净化 / 结构校验、
// 模拟层（Live 两种 TASK_CONFLICT、停止语义、Doc 的 UNSUPPORTED、Edit 的 clip 错误）。
import { AppError, parseDetailHead, toAppError, BACKEND_ERROR_CODES } from './call'
import { taskConflictText, TASK_CONFLICT_GENERIC, actionErrorText, liveStartErrorLine, LIVE_STOP_TEXT, docUnsupportedText, errorMessages, taskErrorMessages } from '@/errors/errorMessages'
import { parsePushUrl, redactPushUrl } from '@/utils/liveUrl'
import * as live from './live'
import * as edit from './edit'
import * as doc from './doc'
import { onSimEvent } from '@/services/wails'
import { retrySimTask } from './sim'
import { isKnownTaskType } from '@/stores/tasks'

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

export async function runApiChecks(): Promise<string[]> {
  // ---- AppError：reason / clipId ----
  eq('reason=max_sessions', new AppError('TASK_CONFLICT', 'x', 'reason=max_sessions\n最多同时进行 4 个').reason, 'max_sessions')
  eq('reason=duplicate_url', new AppError('TASK_CONFLICT', 'x', 'reason=duplicate_url').reason, 'duplicate_url')
  eq('没有 reason', new AppError('TASK_CONFLICT', 'x', '任务已经结束').reason, undefined)
  eq('reason 不在首行不算', new AppError('TASK_CONFLICT', 'x', '说明\nreason=max_sessions').reason, undefined)
  eq('clip 首行', parseDetailHead('clip=c_1-a path=/a b/中文.mp4\n原因'), { clipId: 'c_1-a', path: '/a b/中文.mp4' })
  eq('project 首行没有 clip', parseDetailHead('project\n视频轨不能为空'), {})
  eq('toAppError 解析 JSON 的 detail', toAppError('{"code":"TASK_CONFLICT","message":"m","detail":"reason=duplicate_url"}').reason, 'duplicate_url')
  eq('BACKEND_ERROR_CODES 17 个', BACKEND_ERROR_CODES.length, 17)

  // ---- TASK_CONFLICT 文案（reason → 文案 一张表）----
  eq('max_sessions 文案', taskConflictText('max_sessions'), '最多同时推 4 路')
  eq('duplicate_url 文案', taskConflictText('duplicate_url'), '这个地址已经在推流')
  eq('未知 reason → 通用', taskConflictText('single_screen_only'), TASK_CONFLICT_GENERIC)
  eq('缺失 reason → 通用', taskConflictText(undefined), '操作冲突，请稍后再试')
  eq('Cancel / Remove 的冲突 → 通用', actionErrorText('TASK_CONFLICT', '任务已结束'), '操作冲突，请稍后再试')
  eq('原型链上的键不算 reason', taskConflictText('toString'), TASK_CONFLICT_GENERIC)
  eq('直播停止文案', LIVE_STOP_TEXT, { succeeded: '已结束推流', canceled: '已强制停止' })
  eq('UNSUPPORTED 起始错误行', liveStartErrorLine({ code: 'UNSUPPORTED' })?.description, '当前 ffmpeg 不支持这种推流协议')

  eq('doc/xls/ppt/加密 UNSUPPORTED 文案', docUnsupportedText('不支持这种格式', '/d/a.doc\n旧版'), '暂不支持这种格式，请先另存为 docx、xlsx 或 pptx')
  eq('超 5000 页沿用后端 message', docUnsupportedText('超过 5000 页'), '超过 5000 页')
  // ---- 旧任务类型忽略 ----
  eq('live_relay 忽略', isKnownTaskType('live_relay'), false)
  eq('live_record_push 忽略', isKnownTaskType('live_record_push'), false)
  eq('edit_render 忽略', isKnownTaskType('edit_render'), false)
  eq('未知类型忽略', isKnownTaskType('whatever'), false)
  eq('live_screen_push 认识', isKnownTaskType('live_screen_push'), true)
  eq('edit_export 认识', isKnownTaskType('edit_export'), true)

  // ---- 推流地址 ----
  eq('rtsp 协议不支持', (parsePushUrl('rtsp://h/live') as any).kind, 'protocol')
  eq('http-flv 协议不支持', (parsePushUrl('http://h/live.flv') as any).kind, 'protocol')
  eq('srt 缺端口', (parsePushUrl('srt://h?streamid=a') as any).kind, 'port')
  eq('srt listener 拒绝', (parsePushUrl('srt://h:9000?mode=listener') as any).kind, 'mode')
  eq('rtmp 缺应用名', (parsePushUrl('rtmp://h/') as any).kind, 'app')
  eq('rtmp 默认端口', (parsePushUrl('rtmp://H.example/live/k') as any).info.port, 1935)
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
  const badSpeed = proj()
  badSpeed.videoTrack[0].speed = 9
  err = await rejects(edit.saveProject(badSpeed))
  eq('模拟：speed 越界报错而不是截断', [err?.code, err?.clipId], ['INVALID_ARGUMENT', 'c1'])
  const badId = proj()
  badId.videoTrack[0].id = 'a b'
  err = await rejects(edit.saveProject(badId))
  eq('模拟：clip id 字符集', err?.code, 'INVALID_ARGUMENT')
  const meta = await edit.saveProject(proj())
  eq('SaveProject 新建返回 id', meta.id.startsWith('sim-proj-'), true)
  eq('LoadProject 缺失素材', (await edit.loadProject(meta.id)).missingPaths, [])
  eq('ListProjects', (await edit.listProjects()).length, 1)
  await edit.deleteProject(meta.id)
  eq('DeleteProject 不存在 NOT_FOUND', (await rejects(edit.deleteProject(meta.id)))?.code, 'NOT_FOUND')
  // 预览 404 → 重新取
  const ps = edit.createPreviewSource('/m/a.mp4')
  const u1 = await ps.load()
  eq('预览地址形态', u1.url.startsWith('/local/'), true)
  eq('预览 token 有效时 onMediaError 不重取', await ps.onMediaError(), null)
  ;(globalThis as any).window.location.search = '?sim_preview_404=1'
  const ps2 = edit.createPreviewSource('/m/a.mp4')
  const stale = await ps2.load()
  eq('模拟：第一次的 token 已失效(404)', await edit.isPreviewGone(stale.url), true)
  const fresh = await ps2.onMediaError()
  eq('404 后重新调用 GetPreviewURL 拿到新地址', [!!fresh, fresh?.url !== stale.url], [true, true])
  ;(globalThis as any).window.location.search = ''

  // ---- Live：两种 TASK_CONFLICT 都能由模拟层复现 ----
  const win = (globalThis as any).window
  win.location.search = ''
  const req = (url: string): live.FilePushRequest => ({ inputPath: '/m/a.mp4', url, loop: true, options: live.defaultPushOptions() })
  win.location.search = '?sim_missing=srt'
  err = await rejects(live.startFilePush(req('srt://h9.example:9000?streamid=a')))
  eq('缺 srt 协议 → UNSUPPORTED + detail', [err?.code, err?.detail], ['UNSUPPORTED', 'ffmpeg 缺少协议：srt'])
  win.location.search = ''
  const first = await live.startFilePush(req('rtmp://h1.example/live/secretkey1'))
  eq('Start 返回入队快照', [first.status, first.version, first.progress], ['queued', 1, -1])
  eq('标题脱敏', first.title.includes('secretkey1'), false)
  eq('params 脱敏', first.params.includes('secretkey1'), false)
  err = await rejects(live.startFilePush(req('rtmp://h1.example/live/secretkey1')))
  eq('duplicate_url', [err?.code, err?.reason], ['TASK_CONFLICT', 'duplicate_url'])
  eq('duplicate_url 的 detail 不带地址', (err?.detail ?? '').includes('h1.example') || (err?.detail ?? '').includes('secretkey1'), false)
  eq('duplicate_url 文案', taskConflictText(err?.reason), '这个地址已经在推流')
  for (let i = 2; i <= 4; i++) await live.startFilePush(req(`rtmp://h${i}.example/live/k${i}`))
  err = await rejects(live.startFilePush(req('rtmp://h5.example/live/k5')))
  eq('max_sessions', [err?.code, err?.reason], ['TASK_CONFLICT', 'max_sessions'])
  eq('max_sessions 文案', taskConflictText(err?.reason), '最多同时推 4 路')
  err = await rejects(live.startFilePush(req('rtsp://h/live')))
  eq('协议不支持 → LIVE_URL_INVALID', err?.code, 'LIVE_URL_INVALID')
  eq('LIVE_URL_INVALID 的 detail 是脱敏地址', err?.detail, 'rtsp://h/live')
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
  const ended: Record<string, any> = {}
  const off = onSimEvent<any>('task:status', (p) => {
    if (['succeeded', 'failed', 'canceled'].includes(p.status)) ended[p.id] = p
  })
  const connected = new Set<string>()
  const offP = onSimEvent<any>('task:progress', (p) => connected.add(p.id))
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
  eq('errorMessages 已知码不含 LIVE_PLAY 以外遗漏', Object.keys(errorMessages).length >= 8 && Object.keys(taskErrorMessages).length >= 3, true)
  return fails
}
