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
  // 只在前端由播放器产生，后端不会返回
  | 'LIVE_PLAY_FAILED'
  | 'LIVE_CORS_BLOCKED'

/** 契约里后端会返回的码（不含前端播放器自产的两个） */
export const BACKEND_ERROR_CODES: readonly AppErrorCode[] = [
  'INVALID_ARGUMENT', 'NOT_FOUND', 'FFMPEG_NOT_FOUND', 'TASK_CONFLICT', 'IO_ERROR', 'PROBE_FAILED', 'CANCELED', 'UNSUPPORTED',
  'CONVERT_DISK_FULL', 'PROCESS_FAILED', 'UNSUPPORTED_PLATFORM', 'LIVE_URL_INVALID', 'LIVE_CONNECT_FAILED', 'LIVE_PUSH_REJECTED',
  'LIVE_PUSH_INTERRUPTED', 'SCREEN_PERMISSION_DENIED', 'INTERNAL',
]

export interface DetailHead {
  /** detail 首行 `reason=<值>`（整行只有这一个键值对）；TASK_CONFLICT 用，未知值 / 没有时为 undefined */
  reason?: string
  /** detail 首行 `clip=<id> path=<path>`（Edit 的 clip 级错误）；结构性错误首行是 `project`，两者都为 undefined */
  clipId?: string
  path?: string
}

const REASON_RE = /^reason=([A-Za-z0-9_-]+)$/
const CLIP_RE = /^clip=(\S+) path=(.*)$/

/** 解析 AppError.detail 的第一行（契约：TASK_CONFLICT 的 reason、Edit 的 clip 定位）。只看第一行，解析不了就什么都不返回。 */
export function parseDetailHead(detail?: string): DetailHead {
  if (!detail) return {}
  const first = detail.split(/\r?\n/, 1)[0].trim()
  const r = REASON_RE.exec(first)
  if (r) return { reason: r[1] }
  const c = CLIP_RE.exec(first)
  if (c) return { clipId: c[1], path: c[2] }
  return {}
}

export class AppError extends Error {
  code: AppErrorCode
  detail?: string
  /** 见 parseDetailHead */
  reason?: string
  clipId?: string
  path?: string
  constructor(code: AppErrorCode, message: string, detail?: string) {
    super(message)
    this.name = 'AppError'
    this.code = code
    this.detail = detail
    const head = parseDetailHead(detail)
    this.reason = head.reason
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
  const fn = (globalThis as any).window?.go?.app?.[service]?.[method]
  if (typeof fn !== 'function') throw new AppError('UNSUPPORTED', `后端还没有提供 ${service}.${method}`)
  return await call<T>(fn(...args))
}
