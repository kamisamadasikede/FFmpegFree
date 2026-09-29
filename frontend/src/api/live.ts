/**
 * LiveService 接口层（契约 v0.10，#19 最新提交为准，尚未冻结）。直播页的全部后端调用都在这个文件里。
 *
 * LIVE_BACKEND_READY（api/flags.ts）= false：走本地模拟（api/sim.ts，定时器推进任务、发与后端同形的 task:* 事件，不发任何 HTTP / SSE / WebSocket）。
 * = true：调用 Wails 绑定 window.go.app.LiveService.*（api/call.ts 的 callService，绑定不存在时抛 UNSUPPORTED，不会悄悄走模拟）；停止走 TaskService.Cancel。
 *
 * 契约要点：
 * - 会话 = live 池里的一个任务，会话 id = 任务 id；Start* 立即返回入队快照（status=queued），不等连接成功；
 *   “已经在推”= running 且收到该任务的第一条 task:progress；running 但还没有 progress = “连接中”。
 * - 停止 = TaskService.Cancel(taskID)。优雅停止成功 → succeeded（error 为空）“已结束推流”；5 秒强杀 → canceled（无错误码）“已强制停止”。只看 status。
 * - 指标走 task:progress 的 fps / bitrateKbps / droppedFrames（没有 live:stats、没有 uptimeSec；已推时长 = 现在 − Task.startedAt）。
 * - 推流地址只允许 rtmp / rtmps / srt；完整地址（含推流码）只在调用参数里，不写 localStorage / 日志 / console，展示一律用脱敏形式。
 * - Retry 直播任务返回 UNSUPPORTED：“重新开始”= 用表单里的值再调一次 Start*。
 *
 * 对照表：
 *   本文件函数              契约
 *   startFilePush           LiveService.StartFilePush(FilePushRequest) → Task
 *   startScreenPush         LiveService.StartScreenPush(ScreenPushRequest) → Task
 *   getCaptureCapabilities  LiveService.GetCaptureCapabilities()
 *   listScreens             LiveService.ListScreens()
 *   checkPushURL            LiveService.CheckPushURL(url) → PushURLInfo（只校验，不联网）
 *   stopPush                TaskService.Cancel(taskID)
 *   listRunning             TaskService.ListActive()（type = live_*）
 *   watchLiveTask           事件 task:progress / task:status（按任务 id 过滤）
 *   pickMaterial            SystemService.PickFiles + MediaService.Probe（素材不再上传拷贝）
 */
import * as TaskBinding from '../../wailsjs/go/app/TaskService'
import { AppError, call, callService, toAppError } from '@/api/call'
import { LIVE_BACKEND_READY } from '@/api/flags'
import { probeFiles } from '@/api/media'
import { activeSimEntries, cancelSimTask, createSimTask, getSimTask, injectionDetail, listSimActive, simDelay, simError, simInjection, simParam, type SimLiveSpec } from '@/api/sim'
import { pickFiles, type PickFilter } from '@/api/system'
import { toApiTask, type ApiTask, type ApiTaskError, type TaskProgressPayload, type TaskStatusPayload } from '@/api/taskTypes'
import { onTaskEvent } from '@/services/wails'
import { parsePushUrl, redactPushUrl } from '@/utils/liveUrl'

export { LIVE_BACKEND_READY }
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

export interface PushURLInfo {
  scheme: 'rtmp' | 'rtmps' | 'srt'
  host: string
  port: number
  /** 脱敏后的地址，可以直接显示 */
  redacted: string
}

/** 全零 = 全部使用默认 */
export const defaultPushOptions = (): PushOptions => ({ width: 0, height: 0, fps: 0, videoBitrateKbps: 0, audioBitrateKbps: 0 })

// ───────────── Start* / 能力 / 校验 ─────────────

const SIM_MESSAGES: Record<string, string> = {
  INVALID_ARGUMENT: '参数不合法',
  NOT_FOUND: '输入文件不存在',
  PROBE_FAILED: '无法解析输入文件',
  FFMPEG_NOT_FOUND: '未找到 ffmpeg 可执行文件',
  TASK_CONFLICT: '任务冲突',
  UNSUPPORTED: '当前 ffmpeg 不支持这种推流协议',
  UNSUPPORTED_PLATFORM: '当前系统或会话不支持屏幕采集',
  LIVE_URL_INVALID: '推流地址不合法',
  LIVE_CONNECT_FAILED: '无法连接推流目标',
  LIVE_PUSH_REJECTED: '目标服务器拒绝了推流',
  LIVE_PUSH_INTERRUPTED: '推流被中断',
  SCREEN_PERMISSION_DENIED: '没有屏幕录制权限',
  CANCELED: '调用已取消',
  INTERNAL: 'ffmpeg 异常退出',
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

/** 模拟：Start* 的同步校验（契约“Start* 同步返回的错误”），返回标准化地址 */
function simValidateStart(url: string, options: PushOptions): { normalized: string; redacted: string; scheme: string } {
  const inj = simInjection()
  if (inj && inj.when === 'call') simError(inj.code, simMsg(inj.code), injectionDetail(inj))
  checkOptions(options)
  const u = parsePushUrl(url)
  // detail 只带脱敏后的地址，绝不回显原文
  if (!u.ok) return simError('LIVE_URL_INVALID', u.message, redactPushUrl(url))
  const missing = simParam('sim_missing')
  if (missing && missing === u.info.scheme) simError('UNSUPPORTED', simMsg('UNSUPPORTED'), `ffmpeg 缺少协议：${missing}`)
  const live = activeSimEntries().filter((e) => e.task.type === 'live_file_push' || e.task.type === 'live_screen_push')
  // 后端两种冲突用 detail 第一行 reason=<值> 区分，detail 里不带任何地址片段
  if (live.length >= MAX_SESSIONS) simError('TASK_CONFLICT', '进行中的直播会话已达上限', 'reason=max_sessions\n最多同时进行 4 个直播会话')
  if (live.some((e) => e.meta?.normalized === u.normalized)) simError('TASK_CONFLICT', '该推流地址已有进行中的会话', 'reason=duplicate_url\n已有会话使用同一推流地址')
  return { normalized: u.normalized, redacted: u.info.redacted, scheme: u.info.scheme }
}

function simLiveSpec(scheme: string): SimLiveSpec {
  const inj = simInjection()
  const live: SimLiveSpec = { forceKill: simParam('sim_kill') === '1' }
  const end = Number(simParam('sim_end'))
  if (end > 0) live.endAfterSec = end
  if (inj && inj.when === 'task') {
    live.fail = { code: inj.code, message: simMsg(inj.code), detail: injectionDetail(inj) ?? (scheme === 'srt' && inj.code === 'LIVE_CONNECT_FAILED' ? 'Connection to srt://***@host:9000?streamid=*** failed: Input/output error' : undefined), afterSec: 4 }
  }
  return live
}

/** 文件推流。返回入队快照（queued）；连接 / 鉴权失败以任务 failed + error 体现，不是这里的返回错误 */
export async function startFilePush(req: FilePushRequest): Promise<ApiTask> {
  if (LIVE_BACKEND_READY) return toApiTask(await callService('LiveService', 'StartFilePush', req))
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
    meta: { normalized: v.normalized },
  })
}

/** 屏幕推流（可同时本地存档） */
export async function startScreenPush(req: ScreenPushRequest): Promise<ApiTask> {
  if (LIVE_BACKEND_READY) return toApiTask(await callService('LiveService', 'StartScreenPush', req))
  await simDelay(150)
  const caps = await getCaptureCapabilities()
  if (!caps.supported) simError('UNSUPPORTED_PLATFORM', caps.reason || simMsg('UNSUPPORTED_PLATFORM'))
  if (caps.permission === 'denied') simError('SCREEN_PERMISSION_DENIED', simMsg('SCREEN_PERMISSION_DENIED'))
  const screens = await listScreens()
  if (req.screenId && !screens.some((s) => s.id === req.screenId)) simError('INVALID_ARGUMENT', 'screenId 不存在')
  if (req.archiveDir && !isAbs(req.archiveDir)) simError('INVALID_ARGUMENT', 'archiveDir 必须是绝对路径')
  if (req.audio !== 'none' && req.audio !== 'silent') simError('INVALID_ARGUMENT', 'audio 只能是 none 或 silent')
  const v = simValidateStart(req.url, req.options)
  const screen = screens.find((s) => s.id === req.screenId) ?? screens.find((s) => s.primary)!
  return createSimTask({
    type: 'live_screen_push',
    title: `屏幕推流：${screen.name} → ${v.redacted}`,
    inputPaths: [],
    outputPath: '',
    params: JSON.stringify({ kind: 'screen', screenId: req.screenId, url: v.redacted, hideCursor: req.hideCursor, audio: req.audio, archiveDir: req.archiveDir, options: req.options }),
    live: simLiveSpec(v.scheme),
    meta: { normalized: v.normalized },
  })
}

/** 屏幕采集能不能用、为什么不能用。模拟：?sim_err=UNSUPPORTED_PLATFORM（Wayland）/ SCREEN_PERMISSION_DENIED（macOS 未授权） */
export async function getCaptureCapabilities(): Promise<CaptureCapabilities> {
  if (LIVE_BACKEND_READY) return await callService<CaptureCapabilities>('LiveService', 'GetCaptureCapabilities')
  const code = simParam('sim_err')
  if (code === 'UNSUPPORTED_PLATFORM') {
    return { supported: false, platform: 'linux', backend: '', sessionType: 'wayland', permission: 'notRequired', audioCapture: false, reason: '当前是 Wayland 会话，暂不支持屏幕采集，请切换到 X11 会话' }
  }
  return {
    supported: true, platform: 'darwin', backend: 'avfoundation', sessionType: '', audioCapture: false, reason: '',
    permission: code === 'SCREEN_PERMISSION_DENIED' ? 'denied' : 'granted',
  }
}

export async function listScreens(): Promise<ScreenInfo[]> {
  if (LIVE_BACKEND_READY) return (await callService<ScreenInfo[] | null>('LiveService', 'ListScreens')) ?? []
  return [
    { id: 'avf:0', name: '显示器 1（主）', primary: true, x: 0, y: 0, width: 2880, height: 1800, scale: 2 },
    { id: 'avf:1', name: '显示器 2', primary: false, x: 2880, y: 0, width: 1920, height: 1080, scale: 1 },
  ]
}

/** 只校验地址并返回脱敏后的显示文本，不联网。不通过 LIVE_URL_INVALID */
export async function checkPushURL(url: string): Promise<PushURLInfo> {
  if (LIVE_BACKEND_READY) return await callService<PushURLInfo>('LiveService', 'CheckPushURL', url)
  const u = parsePushUrl(url)
  if (!u.ok) return simError('LIVE_URL_INVALID', u.message, redactPushUrl(url))
  return u.info
}

// ───────────── 停止 / 查询 ─────────────

/** 停止 = TaskService.Cancel(taskID)。立即返回，不等 ffmpeg 退出；结果以 task:status 为准。已结束 → TASK_CONFLICT（无 reason），正在停止中重复点击 → 无操作 */
export async function stopPush(taskId: string): Promise<void> {
  if (LIVE_BACKEND_READY) {
    await call(TaskBinding.Cancel(taskId))
    return
  }
  cancelSimTask(taskId)
}

export interface RunningStream {
  /** = 任务 id */
  streamId: string
  type: 'live_file_push' | 'live_screen_push'
  /** 任务标题（已脱敏，形如 “文件推流：a.mp4 → rtmp://host/app/***”） */
  title: string
  inputPaths: string[]
  startedAt: number
}

/** 页面刷新后接回还在推的会话：TaskService.ListActive 里的 live_* 任务。params 已脱敏，拿不到完整地址 */
export async function listRunning(): Promise<RunningStream[]> {
  const tasks = LIVE_BACKEND_READY ? ((await call(TaskBinding.ListActive())) ?? []).map(toApiTask) : listSimActive()
  return tasks
    .filter((t) => t.type === 'live_file_push' || t.type === 'live_screen_push')
    .map((t) => ({ streamId: t.id, type: t.type as RunningStream['type'], title: t.title, inputPaths: t.inputPaths, startedAt: t.startedAt }))
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
  const end = (status: string, error?: ApiTaskError | null) => {
    if (ended) return
    ended = true
    off()
    h.onEnd({ taskId, status: status as LiveTaskEnd['status'], error: error ?? null })
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
    if (TERMINAL.includes(p.status)) end(p.status, p.error)
  })
  const off = () => {
    offProgress()
    offStatus()
  }
  // 订阅前任务可能已经结束（事件发完了）：补取一次
  void (async () => {
    try {
      const t = LIVE_BACKEND_READY ? toApiTask(await call(TaskBinding.Get(taskId))) : getSimTask(taskId)
      if (t && TERMINAL.includes(t.status) && t.version > version) end(t.status, t.error)
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
