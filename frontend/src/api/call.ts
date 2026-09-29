// Wails Bind 调用的统一封装（契约第 2 节）。
// Go 端返回的 error 会以 AppError 的 JSON 字符串作为 Promise 的 reject 值，这里统一解析成 AppError。

export type AppErrorCode =
  | 'INVALID_ARGUMENT'
  | 'NOT_FOUND'
  | 'FFMPEG_NOT_FOUND'
  | 'TASK_CONFLICT'
  | 'IO_ERROR'
  | 'PROCESS_FAILED'
  | 'PROBE_FAILED'
  | 'CONVERT_DISK_FULL'
  | 'UNSUPPORTED_PLATFORM'
  | 'UNSUPPORTED'
  | 'CANCELED'
  | 'INTERNAL'

export class AppError extends Error {
  code: AppErrorCode
  detail?: string
  constructor(code: AppErrorCode, message: string, detail?: string) {
    super(message)
    this.name = 'AppError'
    this.code = code
    this.detail = detail
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
