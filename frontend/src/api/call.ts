// Wails Bind 调用的统一封装（契约第 2 节）。
// Go 端返回的 error 会以 AppError 的 JSON 字符串作为 Promise 的 reject 值，这里统一解析成 AppError。

/**
 * 契约第 2 节错误码表（以 #19 / #22 / #23 最新提交为准）+ 只在前端由播放器产生的两个码。
 * 逐项对照见 PR 正文 / src/api/README.md。
 */
export type AppErrorCode =
  // 通用
  | 'INVALID_ARGUMENT'
  | 'NOT_FOUND'
  | 'TASK_CONFLICT'
  | 'IO_ERROR'
  | 'CANCELED'
  | 'UNSUPPORTED'
  | 'INTERNAL'
  // ffmpeg / 转换
  | 'FFMPEG_NOT_FOUND'
  | 'PROBE_FAILED'
  | 'PROCESS_FAILED'
  | 'CONVERT_DISK_FULL'
  // 平台 / 直播 / 录屏
  | 'UNSUPPORTED_PLATFORM'
  | 'LIVE_URL_INVALID'
  | 'LIVE_CONNECT_FAILED'
  | 'LIVE_PUSH_REJECTED'
  | 'LIVE_PUSH_INTERRUPTED'
  | 'SCREEN_PERMISSION_DENIED'
  // v0.26 文档转换（DocService，契约 6.12.20）
  | 'DOC_COMPONENT_NOT_READY'
  | 'DOC_DOWNLOAD_FAILED'
  | 'DOC_CHECKSUM_FAILED'
  | 'DOC_COMPONENT_INSTALL_FAILED'
  | 'DOC_FORMAT_UNSUPPORTED'
  | 'DOC_PDF_INPUT_UNSUPPORTED'
  | 'DOC_ENCRYPTED'
  | 'DOC_CORRUPT'
  | 'DOC_TIMEOUT'
  | 'DOC_COMPONENT_CRASHED'
  // v0.27 本机 Office / WPS 被占用（6.12.33）
  | 'DOC_PRESENTATION_BUSY'
  | 'DOC_ENGINE_BUSY'
  | 'LIVE_SOURCE_GONE' // v0.14：屏幕推流所选的窗口 / 屏幕已不可用，detail 首行 kind=window|screen
  // 只在前端由播放器产生，后端不会返回
  | 'LIVE_PLAY_FAILED'
  | 'LIVE_CORS_BLOCKED'

/** 契约里后端会返回的码（不含前端播放器自产的两个） */
export const BACKEND_ERROR_CODES: readonly AppErrorCode[] = [
  'INVALID_ARGUMENT', 'NOT_FOUND', 'FFMPEG_NOT_FOUND', 'TASK_CONFLICT', 'IO_ERROR', 'PROBE_FAILED', 'CANCELED', 'UNSUPPORTED',
  'CONVERT_DISK_FULL', 'PROCESS_FAILED', 'UNSUPPORTED_PLATFORM', 'LIVE_URL_INVALID', 'LIVE_CONNECT_FAILED', 'LIVE_PUSH_REJECTED',
  'LIVE_PUSH_INTERRUPTED', 'SCREEN_PERMISSION_DENIED', 'LIVE_SOURCE_GONE', 'INTERNAL',
  'DOC_COMPONENT_NOT_READY', 'DOC_DOWNLOAD_FAILED', 'DOC_CHECKSUM_FAILED', 'DOC_COMPONENT_INSTALL_FAILED', 'DOC_FORMAT_UNSUPPORTED', 'DOC_PDF_INPUT_UNSUPPORTED', 'DOC_ENCRYPTED', 'DOC_CORRUPT', 'DOC_TIMEOUT', 'DOC_COMPONENT_CRASHED',
  'DOC_PRESENTATION_BUSY', 'DOC_ENGINE_BUSY',
]

export interface DetailHead {
  /** detail 首行 `reason=<值>`（整行只有这一个键值对）；TASK_CONFLICT、LIVE_URL_INVALID、DocService 错误（DocErrorReason）用，没有时为 undefined */
  reason?: string
  /** detail 首行 `scheme=rtmp|rtmps|srt`（整行只有这一个键值对）；LIVE_CONNECT_FAILED 用，没有时为 undefined */
  scheme?: string
  /** detail 首行 `kind=window|screen`（整行只有这一个键值对）；LIVE_SOURCE_GONE 用，没有 / 未知值时为 undefined */
  kind?: 'window' | 'screen'
  /** detail 首行 `clip=<id> path=<path>`（Edit 的 clip 级错误）；结构性错误首行是 `project`，两者都为 undefined */
  clipId?: string
  path?: string
}

/**
 * DocService 错误的 detail 首行 `reason=<值>` 稳定枚举（契约 2.2 / 6.12.6，只追加）：
 * too_many_pages（UNSUPPORTED，超过 5000 页）、format（UNSUPPORTED / INVALID_ARGUMENT，格式不受支持）、encrypted（UNSUPPORTED，加密 Office 文档）、
 * no_font（UNSUPPORTED，缺 Unicode 字体）、invalid_ooxml（INVALID_ARGUMENT，不是有效的 OOXML）、too_large（INVALID_ARGUMENT，超大小 / 超 zip 限制）。
 * 只有这些“文件本身有问题”的错误带 reason；取消、磁盘满、读写失败等不带。message 不变，仍是给用户看的短句。
 */
export const DOC_ERROR_REASONS = ['too_many_pages', 'format', 'encrypted', 'no_font', 'invalid_ooxml', 'too_large'] as const
export type DocErrorReason = (typeof DOC_ERROR_REASONS)[number]

/** AppError.reason → Doc 的 reason 枚举；不在枚举里（含 TASK_CONFLICT / LIVE_URL_INVALID 的 reason）返回 undefined */
export function docErrorReason(reason?: string): DocErrorReason | undefined {
  return (DOC_ERROR_REASONS as readonly string[]).includes(reason ?? '') ? (reason as DocErrorReason) : undefined
}

const REASON_RE = /^reason=([A-Za-z0-9_-]+)/
const SCHEME_RE = /^scheme=([A-Za-z0-9+.-]+)$/
const KIND_RE = /^kind=(window|screen)$/
const CLIP_RE = /^clip=(\S+) path=(.*)$/

/** 解析 AppError.detail 的开头（契约：TASK_CONFLICT / LIVE_URL_INVALID 的 reason、LIVE_CONNECT_FAILED 的 scheme、Edit 的 clip 定位）。只看第一行。reason= 认前缀，后面还可以有 stderr，不必整行只有这一个键。解析不了就什么都不返回。 */
export function parseDetailHead(detail?: string): DetailHead {
  if (!detail) return {}
  const first = detail.split(/\r?\n/, 1)[0].trim()
  const r = REASON_RE.exec(first)
  if (r) return { reason: r[1] }
  const sc = SCHEME_RE.exec(first)
  if (sc) return { scheme: sc[1].toLowerCase() }
  const k = KIND_RE.exec(first)
  if (k) return { kind: k[1] as 'window' | 'screen' }
  const c = CLIP_RE.exec(first)
  if (c) return { clipId: c[1], path: c[2] }
  return {}
}

export class AppError extends Error {
  code: AppErrorCode
  detail?: string
  /** 见 parseDetailHead */
  reason?: string
  scheme?: string
  /** LIVE_SOURCE_GONE 的 detail 首行 kind=window|screen */
  kind?: 'window' | 'screen'
  clipId?: string
  path?: string
  constructor(code: AppErrorCode, message: string, detail?: string) {
    super(message)
    this.name = 'AppError'
    this.code = code
    this.detail = detail
    const head = parseDetailHead(detail)
    this.reason = head.reason
    this.scheme = head.scheme
    this.kind = head.kind
    this.clipId = head.clipId
    this.path = head.path
  }
}

export function toAppError(raw: unknown): AppError {
  if (raw instanceof AppError) return raw
  const text = typeof raw === 'string' ? raw : raw instanceof Error ? raw.message : String(raw)
  try {
    const obj = JSON.parse(text)
    if (obj && typeof obj.code === 'string' && typeof obj.message === 'string') {
      return new AppError(obj.code, obj.message, obj.detail)
    }
  } catch {
    // 不是 AppError JSON，按内部错误处理
  }
  return new AppError('INTERNAL', text || '未知错误')
}

/** 调用一个 Bind 方法，失败时抛出 AppError。 */
export async function call<T>(p: Promise<T>): Promise<T> {
  try {
    return await p
  } catch (e) {
    throw toAppError(e)
  }
}

/**
 * 调用 window.go.app.<service>.<method>（Wails 生成的绑定在运行时就是这样调用的）。
 * 后端还没有这些绑定时不能 import 生成文件（vue-tsc / vite build 会找不到模块），所以这里按名字取；
 * 绑定不存在时抛 UNSUPPORTED，而不是悄悄走模拟。绑定落地后可以改成 import 生成文件，调用方不用动。
 */
export async function callService<T>(service: string, method: string, ...args: unknown[]): Promise<T> {
  const fn = lookupBinding(service, method)
  if (!fn) throw new AppError('UNSUPPORTED', `后端还没有提供 ${service}.${method}`)
  return await call<T>(fn(...args) as Promise<T>)
}

/** window.go.app.<service>.<method>；任何一级不是期望的形状都返回 null */
function lookupBinding(service: string, method: string): ((...args: unknown[]) => unknown) | null {
  const root: unknown = (globalThis as { window?: unknown }).window
  const step = (v: unknown, key: string): unknown => (typeof v === 'object' && v !== null ? (v as Record<string, unknown>)[key] : undefined)
  const fn = step(step(step(root, 'go'), 'app'), service)
  const m = step(fn, method)
  return typeof m === 'function' ? (m as (...args: unknown[]) => unknown).bind(fn) : null
}
