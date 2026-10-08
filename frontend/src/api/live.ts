/**
 * LiveService 接口层（契约 v0.10，#19 最新提交为准，尚未冻结）。直播页的全部后端调用都在这个文件里。
 *
 * LIVE_BACKEND_READY（api/flags.ts）= true 且运行在 Wails 里（liveIsReal()）：直接调用生成的绑定 wailsjs/go/app/LiveService（类型来自 wailsjs/go/models.ts 的 live 命名空间）；停止走 TaskService.Cancel，刷新后接回走 TaskService.ListActive。
 * 开关为 false，或纯浏览器环境（没有 window.go，npm run dev / check:api）：走本地模拟（api/sim.ts，定时器推进任务、发与后端同形的 task:* 事件，不发任何 HTTP / SSE / WebSocket）。
 *
 * 契约要点：
 * - 会话 = live 池里的一个任务，会话 id = 任务 id；Start* 立即返回入队快照（status=queued），不等连接成功；
 *   “已经在推”= running 且收到该任务的第一条 task:progress；running 但还没有 progress = “连接中”。
 * - 停止 = TaskService.Cancel(taskID)。优雅停止成功 → succeeded（error 为空）“已结束推流”；5 秒强杀 → canceled（无错误码）“已强制停止”。只看 status。
 * - 指标走 task:progress 的 fps / bitrateKbps / droppedFrames（没有 live:stats、没有 uptimeSec；已推时长 = 现在 − Task.startedAt）。
 * - 推流地址只允许 rtmp / rtmps / srt；完整地址（含推流码）只在调用参数里，不写日志 / console，展示一律用脱敏形式；表单输入只存本机 localStorage（stores/liveForms.ts，包 20）。
 * - Retry 直播任务返回 UNSUPPORTED：“重新开始”= 用表单里的值再调一次 Start*。
 *
 * 对照表：
 *   本文件函数              契约
 *   startFilePush           LiveService.StartFilePush(FilePushRequest) → Task
 *   startScreenPush         LiveService.StartScreenPush(ScreenPushRequest) → Task
 *   getCaptureCapabilities  LiveService.GetCaptureCapabilities()
 *   listScreens             LiveService.ListScreens()
 *   listCaptureSources      LiveService.ListCaptureSources()（v0.14）
 *   checkPushURL            LiveService.CheckPushURL(url) → PushURLInfo（只校验，不联网）
 *   （预览画面）            LiveService.GetPreviewStream(sessionId) → {url, mime}（契约 v0.25，见 api/livePreviewStream.ts；旧的 GetPreview 截图轮询已删除）
 *   startPullPreview        LiveService.StartPullPreview(PullPreviewRequest) → PullSession（拉流预览会话）
 *   stopPullPreview         LiveService.StopPullPreview(sessionId)
 *   stopPush                TaskService.Cancel(taskID)
 *   listRunning             TaskService.ListActive()（type = live_*）
 *   watchLiveTask           事件 task:progress / task:status（按任务 id 过滤）
 *   pickMaterial            SystemService.PickFiles + MediaService.Probe（素材不再上传拷贝）
 */
import * as TaskBinding from '../../wailsjs/go/app/TaskService'
import * as LiveBinding from '../../wailsjs/go/app/LiveService'
import { live as goLive } from '../../wailsjs/go/models'
import { AppError, call, toAppError } from '@/api/call'
import { LIVE_BACKEND_READY } from '@/api/flags'
import { probeFiles } from '@/api/media'
import { activeSimEntries, cancelSimTask, createSimTask, forceKillSimTask, getSimTask, injectionDetail, listSimActive, simDelay, simError, simInjection, simParam, type SimLiveSpec } from '@/api/sim'
import { pickFiles, type PickFilter } from '@/api/system'
import { toApiTask, type ApiTask, type ApiTaskError, type TaskProgressPayload, type TaskStatusPayload } from '@/api/taskTypes'
import { hasWailsBackend, onTaskEvent } from '@/services/wails'
import { parsePushUrl, redactPushUrl } from '@/utils/liveUrl'
import { LIVE_FFMPEG_PROTOCOL_MISSING_TEXT, LIVE_PUSH_REJECTED_TEXT, LIVE_RTMP_CONNECT_FAILED_TEXT, LIVE_SRT_PASSPHRASE_TEXT, LIVE_URL_INVALID_GENERIC } from '@/errors/errorMessages'

export { LIVE_BACKEND_READY }
/**
 * 是否走真实后端：开关打开 **并且** 运行在 Wails 里（有 window.go）。纯浏览器环境（npm run dev / vite preview / check:api）始终走本地模拟，
 * 不会因为开关为 true 而调用不存在的绑定。页面用它判断“演示数据 / 演示提示”，只在模拟环境出现。
 */
export const liveIsReal = (): boolean => LIVE_BACKEND_READY && hasWailsBackend()
/** 页面用的错误类型：就是 AppError（code / message / detail / reason） */
export type LiveError = AppError

// ───────────── 契约类型（与 §4 LiveService 字段一一对应）─────────────

/** 两个 Start* 共用的编码选项。全部可省略（零值 = 默认）；越界 INVALID_ARGUMENT */
export interface PushOptions {
  /** 0 = 保持（文件）/ 采集分辨率（屏幕）；上限 8192，输出保证偶数 */
  width: number
  height: number
  /** 0 = 保持源帧率（文件）/ 30（屏幕）；范围 1~60 */
  fps: number
  /** 0 = 2500；范围 100~50000（kbit/s） */
  videoBitrateKbps: number
  /** 0 = 128；范围 32~512 */
  audioBitrateKbps: number
}

export interface FilePushRequest {
  /** 绝对路径的普通文件，必须有视频画面（否则 INVALID_ARGUMENT） */
  inputPath: string
  /** 完整推流地址（含推流码）。仅调用参数，不存储、不打印 */
  url: string
  /** true = 循环直到用户停止；false = 播完自然结束（succeeded） */
  loop: boolean
  options: PushOptions
  /** 预览画面开关（契约 v0.14）：省略 / true = 带预览（GetPreviewStream 可用）；false = 不加预览输出。只在开始时决定 */
  preview?: boolean
}

export interface ScreenPushRequest {
  url: string
  /** ListScreens 返回的 id（不透明字符串，原样传回）；"" = 主显示器 */
  screenId: string
  /** 零值 = 画面里带鼠标指针 */
  hideCursor: boolean
  /** none（默认，视频流里没有音轨）| silent（补一路静音音轨）；采集声音 v1 不做 */
  audio: 'none' | 'silent'
  /** 非空 = 同时在本地存一份 mp4（绝对路径）；"" = 不存档 */
  archiveDir: string
  options: PushOptions
  /** 同 FilePushRequest.preview */
  preview?: boolean
  /** v0.14 可选：ListCaptureSources 返回的 id（screen:<序号> | window:<hwnd 十进制>），原样传回；不传 = 按 screenId（原行为）；传了以它为准。来源已不可用 → LIVE_SOURCE_GONE（detail 首行 kind=window|screen） */
  captureSourceId?: string
}

export interface CaptureCapabilities {
  supported: boolean
  platform: 'windows' | 'darwin' | 'linux' | (string & {})
  backend: 'gdigrab' | 'avfoundation' | 'x11grab' | ''
  /** 只对 linux 有意义 */
  sessionType: 'x11' | 'wayland' | 'unknown' | ''
  permission: 'granted' | 'denied' | 'unknown' | 'notRequired'
  /** v1 恒为 false */
  audioCapture: boolean
  /** 不支持时给用户看的中文原因 */
  reason: string
}

export interface ScreenInfo {
  id: string
  name: string
  primary: boolean
  x: number
  y: number
  width: number
  height: number
  scale: number
}

/** v0.14：一个可选的采集来源（ListCaptureSources）。macOS / Linux 只有 kind=screen */
export interface CaptureSource {
  /** 不透明字符串，原样传给 captureSourceId：screen:<序号> | window:<hwnd 十进制> */
  id: string
  kind: 'screen' | 'window'
  /** screen：屏幕名（如“屏幕 1（主显示器）”）；window：窗口标题原文 */
  title: string
  /** 物理像素；查不到为 0 */
  width: number
  height: number
}

export interface PushURLInfo {
  scheme: 'rtmp' | 'rtmps' | 'srt'
  host: string
  port: number
  /** 脱敏后的地址，可以直接显示 */
  redacted: string
}

/** 拉流预览会话请求：后端 ffmpeg 读远端流只出预览，播放仍由前端播放器直接拉地址 */
export interface PullPreviewRequest {
  /** rtmp / rtmps / srt / http / https 地址（ws / wss 后端返回 LIVE_URL_INVALID） */
  url: string
  /** 省略 / true = 出预览；false = 不启动 ffmpeg */
  preview?: boolean
}

export interface PullSession {
  /** 会话 id，传给 stopPullPreview */
  id: string
  /** 脱敏后的地址，可以直接显示 */
  redacted: string
  /** v0.25：有没有预览视频流（转换组件缺 tee / tcp 时 false）。浏览器模拟层恒为 false */
  preview: boolean
  /** v0.25：同 GetPreviewStream 的 url；没有预览视频流时为空串。浏览器模拟层恒为 ""（契约 6.10.3.1） */
  previewUrl: string
}

/** 全零 = 全部使用默认 */
export const defaultPushOptions = (): PushOptions => ({ width: 0, height: 0, fps: 0, videoBitrateKbps: 0, audioBitrateKbps: 0 })

// ───────────── Start* / 能力 / 校验 ─────────────

// 模拟层的后端 message（真实后端的 message 页面基本不直接显示，文案见 errors/errorMessages.ts；这里与产品定稿保持一致，方便预览）
const SIM_MESSAGES: Record<string, string> = {
  INVALID_ARGUMENT: '参数不合法',
  NOT_FOUND: '输入文件不存在',
  PROBE_FAILED: '无法解析输入文件',
  FFMPEG_NOT_FOUND: '未找到转换组件',
  TASK_CONFLICT: '操作冲突，请稍后再试',
  UNSUPPORTED: LIVE_FFMPEG_PROTOCOL_MISSING_TEXT,
  UNSUPPORTED_PLATFORM: '当前系统暂不支持屏幕推流',
  LIVE_URL_INVALID: LIVE_URL_INVALID_GENERIC,
  LIVE_CONNECT_FAILED: LIVE_RTMP_CONNECT_FAILED_TEXT,
  LIVE_PUSH_REJECTED: LIVE_PUSH_REJECTED_TEXT,
  LIVE_PUSH_INTERRUPTED: '推流被中断',
  SCREEN_PERMISSION_DENIED: '没有获得屏幕录制权限，请在系统设置中允许 FFmpegFree 录制屏幕后重试',
  LIVE_SOURCE_GONE: '所选窗口已不可用，请重新选择',
  CANCELED: '调用已取消',
  INTERNAL: '推流异常退出',
}
const simMsg = (code: string) => SIM_MESSAGES[code] ?? '模拟错误'

const baseName = (p: string) => p.split(/[\\/]/).pop() || p
const isAbs = (p: string) => /^([A-Za-z]:[\\/]|\/|\\\\)/.test(p)
const MAX_SESSIONS = 4

function checkOptions(o: PushOptions) {
  const bad = (what: string): never => simError('INVALID_ARGUMENT', `选项越界：${what}`)
  if (o.width < 0 || o.width > 8192) bad('width（0~8192）')
  if (o.height < 0 || o.height > 8192) bad('height（0~8192）')
  if (o.fps !== 0 && (o.fps < 1 || o.fps > 60)) bad('fps（1~60）')
  if (o.videoBitrateKbps !== 0 && (o.videoBitrateKbps < 100 || o.videoBitrateKbps > 50000)) bad('videoBitrateKbps（100~50000）')
  if (o.audioBitrateKbps !== 0 && (o.audioBitrateKbps < 32 || o.audioBitrateKbps > 512)) bad('audioBitrateKbps（32~512）')
}

/** LIVE_URL_INVALID 的 detail：首行 `reason=<值>`（契约稳定枚举），第二行是脱敏后的地址，绝不回显原文 */
function urlInvalidDetail(reason: string, url: string): string {
  return `reason=${reason}\n${redactPushUrl(url)}`
}

// ───────────── SRT 口令校验（前端先拦，不发给后端）─────────────
export const SRT_PASSPHRASE_MIN = 10
export const SRT_PASSPHRASE_MAX = 79

/** SRT 口令长度是否合法：空口令（不加密）合法；否则必须 10~79 个字符（按 Unicode 码点数；后端按字节时更严，这里取前端能确定的下限 / 上限） */
export function isValidSrtPassphrase(passphrase: string): boolean {
  if (!passphrase) return true
  const n = Array.from(passphrase).length
  return n >= SRT_PASSPHRASE_MIN && n <= SRT_PASSPHRASE_MAX
}

/** 取地址查询参数里的 passphrase（srt 才有意义；解析不了返回 undefined）。只读、不打印、不存储 */
export function srtPassphraseFromUrl(url: string): string | undefined {
  const m = /^srt:\/\/[^?#]*\?([^#]*)/i.exec((url ?? '').trim())
  if (!m) return undefined
  for (const kv of m[1].split('&')) {
    const i = kv.indexOf('=')
    if (i > 0 && kv.slice(0, i) === 'passphrase') {
      try {
        return decodeURIComponent(kv.slice(i + 1).replace(/\+/g, ' '))
      } catch {
        return kv.slice(i + 1)
      }
    }
  }
  return undefined
}

/** 口令长度不对就抛 INVALID_ARGUMENT（message = 产品文案，不含口令 / 地址，detail 首行 reason=srt_passphrase_length）。Start* 在调后端之前先调它 */
export function assertSrtPassphrase(url: string): void {
  const pass = srtPassphraseFromUrl(url)
  if (pass !== undefined && !isValidSrtPassphrase(pass)) {
    throw new AppError('INVALID_ARGUMENT', LIVE_SRT_PASSPHRASE_TEXT, 'reason=srt_passphrase_length')
  }
}

/** 模拟：Start* 的同步校验（契约“Start* 同步返回的错误”），返回标准化地址 */
function simValidateStart(url: string, options: PushOptions, screen = false): { normalized: string; redacted: string; scheme: string } {
  const inj = simInjection()
  if (inj && inj.when === 'call') simError(inj.code, simMsg(inj.code), injectionDetail(inj))
  checkOptions(options)
  const u = parsePushUrl(url)
  // detail 只带脱敏后的地址，绝不回显原文
  if (!u.ok) return simError('LIVE_URL_INVALID', u.message, urlInvalidDetail(u.reason, url))
  const missing = simParam('sim_missing')
  // 契约 §6.10：缺协议时 detail 是单独一行 missing=<协议名>（rtmp / rtmps / srt），没有第二行
  if (missing && missing === u.info.scheme) simError('UNSUPPORTED', simMsg('UNSUPPORTED'), `missing=${missing}`)
  const live = activeSimEntries().filter((e) => e.task.type === 'live_file_push' || e.task.type === 'live_screen_push')
  // 后端两种冲突用 detail 第一行 reason=<值> 区分，detail 里不带任何地址片段
  // 判断顺序（后端统一）：duplicate_url → screen_busy → max_sessions
  if (live.some((e) => e.meta?.normalized === u.normalized)) simError('TASK_CONFLICT', '该推流地址已有进行中的会话', 'reason=duplicate_url\n已有会话使用同一推流地址')
  // 屏幕推流同一时间最多 1 路（含排队、正在停止：activeSimEntries 包含这些状态）
  if (screen && live.some((e) => e.task.type === 'live_screen_push')) simError('TASK_CONFLICT', '已有进行中的屏幕推流', 'reason=screen_busy\n屏幕推流同一时间只能有 1 路')
  if (live.length >= MAX_SESSIONS) simError('TASK_CONFLICT', '进行中的直播会话已达上限', 'reason=max_sessions\n最多同时进行 4 个直播会话')
  return { normalized: u.normalized, redacted: u.info.redacted, scheme: u.info.scheme }
}

/** LIVE_CONNECT_FAILED 的 detail 首行固定 `scheme=rtmp|rtmps|srt`（架构师决定 3）；?sim_detail 附加为后面的行。?sim_scheme=missing 让首行缺失（测兜底） */
function connectFailedDetail(inj: NonNullable<ReturnType<typeof simInjection>>, scheme: string): string | undefined {
  if (inj.code !== 'LIVE_CONNECT_FAILED') return injectionDetail(inj)
  const head = simParam('sim_scheme') === 'missing' ? [] : [`scheme=${scheme}`]
  const rest = inj.detail ?? (scheme === 'srt' ? 'Connection to srt://***@host:9000?streamid=*** failed: Input/output error' : 'Connection to rtmp://host:1935 failed: Connection refused')
  return [...head, rest].join('\n')
}

function simLiveSpec(scheme: string): SimLiveSpec {
  const inj = simInjection()
  const live: SimLiveSpec = { forceKill: simParam('sim_kill') === '1' }
  const end = Number(simParam('sim_end'))
  if (end > 0) live.endAfterSec = end
  if (inj && inj.when === 'task') {
    live.fail = { code: inj.code, message: simMsg(inj.code), detail: connectFailedDetail(inj, scheme), afterSec: 4 }
  }
  return live
}

/** 文件推流。返回入队快照（queued）；连接 / 鉴权失败以任务 failed + error 体现，不是这里的返回错误 */
export async function startFilePush(req: FilePushRequest): Promise<ApiTask> {
  assertSrtPassphrase(req.url)
  if (liveIsReal()) return toApiTask(await call(LiveBinding.StartFilePush(goLive.FilePushRequest.createFrom(req))))
  await simDelay(150)
  if (!isAbs(req.inputPath)) simError('INVALID_ARGUMENT', '输入文件必须是绝对路径')
  const v = simValidateStart(req.url, req.options)
  return createSimTask({
    type: 'live_file_push',
    title: `文件推流：${baseName(req.inputPath)} → ${v.redacted}`,
    inputPaths: [req.inputPath],
    outputPath: '',
    params: JSON.stringify({ kind: 'file', input: req.inputPath, url: v.redacted, loop: req.loop, options: req.options }),
    live: { ...simLiveSpec(v.scheme), ...(req.loop ? {} : { endAfterSec: Number(simParam('sim_end')) || 90 }) },
    meta: { normalized: v.normalized, preview: req.preview !== false },
  })
}

/** 屏幕推流请求：没选来源（sourceId 为空）时**不带 captureSourceId**（也不带 screenId），后端默认推主显示器；选了就原样传回，开始时后端重新校验 */
export function buildScreenPushRequest(p: { url: string; sourceId: string; archiveDir: string; preview: boolean }): ScreenPushRequest {
  return {
    url: p.url, screenId: '', hideCursor: false, audio: 'none', archiveDir: p.archiveDir, options: defaultPushOptions(), preview: p.preview,
    ...(p.sourceId ? { captureSourceId: p.sourceId } : {}),
  }
}

/** 屏幕推流（可同时本地存档；后端 #47 已实现，archiveDir 非空时任务的 outputPath = 存档路径） */
export async function startScreenPush(req: ScreenPushRequest): Promise<ApiTask> {
  assertSrtPassphrase(req.url)
  if (liveIsReal()) return toApiTask(await call(LiveBinding.StartScreenPush(goLive.ScreenPushRequest.createFrom(req))))
  await simDelay(150)
  const caps = await getCaptureCapabilities()
  if (!caps.supported) simError('UNSUPPORTED_PLATFORM', caps.reason || simMsg('UNSUPPORTED_PLATFORM'))
  if (caps.permission === 'denied') simError('SCREEN_PERMISSION_DENIED', simMsg('SCREEN_PERMISSION_DENIED'))
  const screens = await listScreens()
  if (req.screenId && !screens.some((s) => s.id === req.screenId)) simError('INVALID_ARGUMENT', 'screenId 不存在')
  if (req.archiveDir && !isAbs(req.archiveDir)) simError('INVALID_ARGUMENT', 'archiveDir 必须是绝对路径')
  if (req.audio !== 'none' && req.audio !== 'silent') simError('INVALID_ARGUMENT', 'audio 只能是 none 或 silent')
  const v = simValidateStart(req.url, req.options, true)
  let screenName = (screens.find((s) => s.id === req.screenId) ?? screens.find((s) => s.primary)!).name
  if (req.captureSourceId) {
    // 与真实后端一致：格式不对 INVALID_ARGUMENT；来源不在当前列表里（窗口已关闭 / 最小化，屏幕序号不存在）→ LIVE_SOURCE_GONE，detail 只有 kind=<值>
    if (!/^(screen|window):(0|[1-9][0-9]*)$/.test(req.captureSourceId)) simError('INVALID_ARGUMENT', 'captureSourceId 格式不对')
    const src = simSources().find((s) => s.id === req.captureSourceId)
    if (!src) {
      const kind = req.captureSourceId.startsWith('window:') ? 'window' : 'screen'
      simError('LIVE_SOURCE_GONE', kind === 'window' ? '所选窗口已不可用，请重新选择' : '所选屏幕已不可用，请重新选择', `kind=${kind}`)
    }
    screenName = src.title
  }
  // 后端 #47：archiveDir 非空 → 任务的 outputPath = 存档路径（分片 mp4，直接写最终文件名）；有存档的会话没有 bitrateKbps
  const archivePath = req.archiveDir ? simArchivePath(req.archiveDir) : ''
  return createSimTask({
    type: 'live_screen_push',
    title: `屏幕推流：${screenName} → ${v.redacted}`,
    inputPaths: [],
    outputPath: archivePath,
    params: JSON.stringify({ kind: 'screen', screenId: req.screenId, url: v.redacted, hideCursor: req.hideCursor, audio: req.audio, archiveDir: req.archiveDir, options: req.options, ...(req.captureSourceId ? { captureSourceId: req.captureSourceId } : {}) }),
    live: { ...simLiveSpec(v.scheme), ...(archivePath ? { archive: true } : {}) },
    meta: { normalized: v.normalized, preview: req.preview !== false },
  })
}

/** 模拟：存档文件名与后端一致 screen-YYYYMMDD-HHMMSS.mp4 */
function simArchivePath(dir: string): string {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  const sep = dir.includes('\\') && !dir.includes('/') ? '\\' : '/'
  return `${dir.replace(/[\\/]+$/, '')}${sep}screen-${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}.mp4`
}

/** 屏幕采集能不能用、为什么不能用。模拟：?sim_err=UNSUPPORTED_PLATFORM（Wayland）/ SCREEN_PERMISSION_DENIED（macOS 未授权） */
export async function getCaptureCapabilities(): Promise<CaptureCapabilities> {
  if (liveIsReal()) return (await call(LiveBinding.GetCaptureCapabilities())) as CaptureCapabilities
  const code = simParam('sim_err')
  if (code === 'UNSUPPORTED_PLATFORM') {
    return { supported: false, platform: 'linux', backend: '', sessionType: 'wayland', permission: 'notRequired', audioCapture: false, reason: '当前是 Wayland 会话，暂不支持屏幕采集，请切换到 X11 会话' }
  }
  // 模拟平台：默认 Windows（分组下拉，可选窗口）；?sim_sources=screens 或 ?sim_os=mac|linux → macOS / Linux（屏幕单选列表）
  const os = simParam('sim_os') ?? (simParam('sim_sources') === 'screens' ? 'mac' : 'win')
  if (os === 'linux') return { supported: true, platform: 'linux', backend: 'x11grab', sessionType: 'x11', permission: 'notRequired', audioCapture: false, reason: '' }
  if (os === 'win') return { supported: true, platform: 'windows', backend: 'gdigrab', sessionType: '', permission: 'notRequired', audioCapture: false, reason: '' }
  return {
    supported: true, platform: 'darwin', backend: 'avfoundation', sessionType: '', audioCapture: false, reason: '',
    permission: code === 'SCREEN_PERMISSION_DENIED' ? 'denied' : 'granted',
  }
}

export async function listScreens(): Promise<ScreenInfo[]> {
  if (liveIsReal()) return (await call(LiveBinding.ListScreens())) ?? []
  return simScreenList()
}

/**
 * v0.14：屏幕推流可选的采集来源（屏幕 + Windows 上的应用窗口）。真实后端 macOS / Linux 只有 screen；不能采集屏幕时 UNSUPPORTED_PLATFORM。
 * 模拟层按 Windows 的样子返回 2 个屏幕 + 2 个窗口（window:<hwnd> 里的 hwnd 是假的；开始推流前从列表里去掉窗口 = 已关闭，可用 `?sim_source_gone=1` 让窗口消失）。
 */
export async function listCaptureSources(): Promise<CaptureSource[]> {
  if (liveIsReal()) return ((await call(LiveBinding.ListCaptureSources())) ?? []) as CaptureSource[]
  const mode = simParam('sim_sources') // loading（一直加载）| fail（列表失败）| empty（空）| screens（只有屏幕，像 macOS / Linux）
  if (mode === 'loading') return new Promise(() => undefined)
  await simDelay(120)
  if (mode === 'fail') simError('INTERNAL', '获取采集来源失败')
  if (mode === 'empty') return []
  // stale：第一次成功，之后刷新都失败（有旧列表时的“刷新失败”）；refreshing：第一次成功，之后刷新一直在途（“刷新中”）
  if (mode === 'stale' && simListCalls >= 1) simError('INTERNAL', '获取采集来源失败')
  if (mode === 'refreshing' && simListCalls >= 1) return new Promise(() => undefined)
  const list = simSources()
  simListCalls++
  // screens：只有屏幕（macOS / Linux）；nowin：Windows 但没有可选窗口
  return mode === 'screens' || mode === 'nowin' ? list.filter((s) => s.kind === 'screen') : list
}

let simListCalls = 0
/** 只给自检用：重置“窗口消失”的计数 */
export function resetSimSources(): void {
  simListCalls = 0
}
/** 2 屏 + 2 窗。`?sim_source_gone=1`：列表被拉过一次之后「演示文稿」窗口消失（页面先看到它、选中、点开始 → LIVE_SOURCE_GONE → 刷新后它不在了） */
function simSources(): CaptureSource[] {
  const screens = simScreenList().map((s, i): CaptureSource => ({ id: `screen:${i}`, kind: 'screen', title: s.name, width: s.width, height: s.height }))
  const gone = simParam('sim_source_gone') === '1' && simListCalls >= 1
  const windows: CaptureSource[] = [
    ...(gone ? [] : [{ id: 'window:65890', kind: 'window' as const, title: '演示文稿.pptx - PowerPoint', width: 1600, height: 900 }]),
    { id: 'window:131426', kind: 'window', title: '记事本', width: 800, height: 600 },
    // ?sim_win=many：设计稿里的 5 个窗口（含长标题，看中间省略）
    ...(simParam('sim_win') === 'many'
      ? [
          { id: 'window:200001', kind: 'window' as const, title: '会议纪要.txt - 记事本', width: 1280, height: 720 },
          { id: 'window:200002', kind: 'window' as const, title: '产品原型 - 设计工具', width: 1600, height: 900 },
          { id: 'window:200003', kind: 'window' as const, title: '直播中控台 - 浏览器', width: 1440, height: 900 },
          { id: 'window:200004', kind: 'window' as const, title: '2026年第三季度经营分析汇报（终稿·第12版）- 演示文稿', width: 1920, height: 1080 },
          { id: 'window:200005', kind: 'window' as const, title: '概念片_终版.mp4 - 视频播放器', width: 1280, height: 720 },
        ]
      : []),
  ]
  return [...screens, ...windows]
}
function simScreenList(): ScreenInfo[] {
  if (simParam('sim_os') === 'linux') {
    return [
      { id: 'x11:0', name: '屏幕 1（主显示器）', primary: true, x: 0, y: 0, width: 1920, height: 1080, scale: 1 },
      { id: 'x11:1', name: '屏幕 2', primary: false, x: 1920, y: 0, width: 2560, height: 1440, scale: 1 },
      { id: 'x11:2', name: '屏幕 3', primary: false, x: -1280, y: 0, width: 1280, height: 720, scale: 1 },
    ]
  }
  return [
    { id: 'avf:0', name: '屏幕 1（主显示器）', primary: true, x: 0, y: 0, width: 1920, height: 1080, scale: 2 },
    { id: 'avf:1', name: '屏幕 2', primary: false, x: 1920, y: 0, width: 2560, height: 1440, scale: 1 },
  ]
}

/** 只校验地址并返回脱敏后的显示文本，不联网。不通过 LIVE_URL_INVALID */
export async function checkPushURL(url: string): Promise<PushURLInfo> {
  if (liveIsReal()) return (await call(LiveBinding.CheckPushURL(url))) as PushURLInfo
  const u = parsePushUrl(url)
  if (!u.ok) return simError('LIVE_URL_INVALID', u.message, urlInvalidDetail(u.reason, url))
  return u.info
}

// ───────────── 拉流预览会话 ─────────────

/** 模拟的拉流预览会话（id → 地址 / 是否出预览） */
const simPulls = new Map<string, { url: string; preview: boolean }>()
let simPullSeq = 0
/** 自检用：当前还没 Stop 的模拟拉流预览会话数 */
export const simPullPreviewCount = (): number => simPulls.size

/**
 * 开始拉流预览会话（契约 v0.25：PullSession.previewUrl）。同一地址幂等。
 * 浏览器模拟层（契约 6.10.3.1）：preview 恒为 false，previewUrl 恒为 ""。真实绑定只在
 * LIVE_PREVIEW_V25_BACKEND_READY 打开时由 api/livePreviewStream.ts 调用。
 */
export async function startPullPreview(req: PullPreviewRequest): Promise<PullSession> {
  if (liveIsReal()) {
    const s = (await call(LiveBinding.StartPullPreview(goLive.PullPreviewRequest.createFrom(req)))) as Partial<PullSession>
    return { id: s.id ?? '', redacted: s.redacted ?? '', preview: !!s.preview, previewUrl: s.previewUrl ?? '' }
  }
  await simDelay(60)
  const redacted = redactPushUrl(req.url)
  for (const [id, v] of simPulls) if (v.url === req.url) return { id, redacted, preview: false, previewUrl: '' }
  const id = `sim-pull-${++simPullSeq}`
  simPulls.set(id, { url: req.url, preview: false })
  return { id, redacted, preview: false, previewUrl: '' }
}

/** 停止拉流预览会话；会话已结束时无操作 */
export async function stopPullPreview(sessionId: string): Promise<void> {
  if (liveIsReal()) return void (await call(LiveBinding.StopPullPreview(sessionId)))
  simPulls.delete(sessionId)
}

// ───────────── 停止 / 查询 ─────────────

/** 停止 = TaskService.Cancel(taskID)。立即返回，不等 ffmpeg 退出；结果以 task:status 为准。已结束 → TASK_CONFLICT（无 reason），正在停止中重复点击 → 无操作 */
export async function stopPush(taskId: string): Promise<void> {
  if (liveIsReal()) {
    await call(TaskBinding.Cancel(taskId))
    return
  }
  cancelSimTask(taskId)
}

/**
 * 强制停止（“正在停止…”行上的 [强制停止]）。后端目前没有单独的强杀入口：重复 Cancel 是无操作（契约 6.6），
 * 所以真实环境下这里只是再发一次 Cancel，不会缩短等待（最多 16 秒后端自己强杀）；模拟层立即强杀 → canceled。
 * 需要架构师给强杀入口后再接（见 PR 说明）。
 */
export async function forceStopPush(taskId: string): Promise<void> {
  if (liveIsReal()) {
    await call(TaskBinding.Cancel(taskId))
    return
  }
  forceKillSimTask(taskId)
}

export interface RunningStream {
  /** = 任务 id */
  streamId: string
  type: 'live_file_push' | 'live_screen_push'
  /** 任务标题（已脱敏，形如 “文件推流：a.mp4 → rtmp://host/app/***”） */
  title: string
  inputPaths: string[]
  startedAt: number
  /** 存档路径；非空 = 有本地存档（分片 mp4）。文件推流恒为 "" */
  outputPath: string
  /** 脱敏后的推流地址（params.url，形如 rtmp://host/app/***）；解析不了为 "" */
  url: string
}

function redactedUrlFromParams(params: string): string {
  try {
    const u = (JSON.parse(params) as { url?: unknown }).url
    return typeof u === 'string' ? u : ''
  } catch {
    return ''
  }
}

/** 页面刷新后接回还在推的会话：TaskService.ListActive 里的 live_* 任务。params 已脱敏，拿不到完整地址 */
export async function listRunning(): Promise<RunningStream[]> {
  const tasks = liveIsReal() ? ((await call(TaskBinding.ListActive())) ?? []).map(toApiTask) : listSimActive()
  return tasks
    .filter((t) => t.type === 'live_file_push' || t.type === 'live_screen_push')
    .map((t) => ({
      streamId: t.id, type: t.type as RunningStream['type'], title: t.title, inputPaths: t.inputPaths, startedAt: t.startedAt,
      outputPath: t.outputPath, url: redactedUrlFromParams(t.params),
    }))
}

// ───────────── 任务事件 ─────────────

export interface LiveProgress {
  taskId: string
  bitrateKbps: number
  fps: number
  droppedFrames: number
  /** 已输出的媒体时长（秒） */
  outTimeSec: number
  speed: string
}

export interface LiveTaskEnd {
  taskId: string
  /** succeeded=优雅停止 / 自然结束（已结束推流）；canceled=强杀（已强制停止）；failed=看 error；interrupted=应用退出 */
  status: 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  error?: ApiTaskError | null
  /** 终态 task:status 里的 outputPath：有本地存档且保留时非空（succeeded / canceled / failed / interrupted 都可能有）；空 = 没有存档，或空壳存档已被后端删掉。`canceled` 且非空 = 强杀且存档已保留 */
  outputPath: string
}

export interface LiveWatchHandlers {
  /** 收到该任务的第一条 task:progress（“连接中” → “正在推流”） */
  onConnected?: () => void
  onProgress?: (p: LiveProgress) => void
  onEnd: (e: LiveTaskEnd) => void
}

const TERMINAL = ['succeeded', 'failed', 'canceled', 'interrupted']

/** 订阅一个直播任务的 task:progress / task:status，返回取消订阅函数。按 version 丢弃旧事件 */
export function watchLiveTask(taskId: string, h: LiveWatchHandlers): () => void {
  let version = 0
  let connected = false
  let ended = false
  const end = (status: string, error?: ApiTaskError | null, outputPath?: string) => {
    if (ended) return
    ended = true
    off()
    h.onEnd({ taskId, status: status as LiveTaskEnd['status'], error: error ?? null, outputPath: outputPath ?? '' })
  }
  const offProgress = onTaskEvent<TaskProgressPayload>('task:progress', (p) => {
    if (p.id !== taskId || ended || p.version <= version) return
    version = p.version
    if (!connected) {
      connected = true
      h.onConnected?.()
    }
    h.onProgress?.({ taskId, bitrateKbps: p.bitrateKbps ?? 0, fps: p.fps ?? 0, droppedFrames: p.droppedFrames ?? 0, outTimeSec: p.outTimeSec ?? 0, speed: p.speed ?? '' })
  })
  const offStatus = onTaskEvent<TaskStatusPayload>('task:status', (p) => {
    if (p.id !== taskId || p.version <= version) return
    version = p.version
    if (TERMINAL.includes(p.status)) end(p.status, p.error, p.outputPath)
  })
  const off = () => {
    offProgress()
    offStatus()
  }
  // 订阅前任务可能已经结束（事件发完了）：补取一次
  void (async () => {
    try {
      const t = liveIsReal() ? toApiTask(await call(TaskBinding.Get(taskId))) : getSimTask(taskId)
      if (t && TERMINAL.includes(t.status) && t.version > version) end(t.status, t.error, t.outputPath)
    } catch (e) {
      console.warn('live task get failed', taskId, toAppError(e).code)
    }
  })()
  return () => {
    ended = true
    off()
  }
}

// ───────────── 素材（文件推流的输入）─────────────

export interface LiveMaterial {
  name: string
  /** 绝对路径（FilePushRequest.inputPath） */
  path: string
  /** “HH:MM:SS” */
  duration: string
}

const VIDEO_FILTER: PickFilter = { name: '视频文件', patterns: ['*.mp4', '*.m4v', '*.mov', '*.mkv', '*.flv', '*.avi', '*.webm', '*.ts'] }
const clock = (sec: number) => {
  const s = Math.max(0, Math.floor(sec))
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(Math.floor(s / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`
}

/** 演示素材（内存里，刷新即恢复），模拟层用 */
export function demoMaterials(): LiveMaterial[] {
  return [
    { name: '产品发布会.mp4', path: '/Users/me/Movies/产品发布会.mp4', duration: '00:42:18' },
    { name: '直播预热片.mp4', path: '/Users/me/Movies/直播预热片.mp4', duration: '00:03:12' },
  ]
}

/** 系统选文件对话框（SystemService.PickFiles）+ 探测时长（MediaService.Probe）。取消返回 []；没有 PickFiles 绑定抛 UNSUPPORTED；没有视频画面的文件抛 INVALID_ARGUMENT */
export async function pickMaterial(): Promise<LiveMaterial[]> {
  const paths = await pickFiles(VIDEO_FILTER, false)
  if (!paths.length) return []
  const [r] = await probeFiles(paths)
  if (r.error) throw new AppError(r.error.code as never, r.error.message, r.error.detail)
  if (r.info && r.info.hasVideo === false) throw new AppError('INVALID_ARGUMENT', '这个文件没有视频画面，不能用来推流')
  return [{ name: baseName(r.path), path: r.path, duration: clock(r.info?.duration ?? 0) }]
}

export type { ApiTask, ApiTaskError }
