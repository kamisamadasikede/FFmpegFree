// 契约第 3 节 Task（含 v0.10 直播新增字段）。与 store 里的 TaskItem 不同：这是接口层返回的原样形状。
import type { TaskStatus, TaskType } from '@/stores/tasks'
import type { AppErrorCode } from '@/api/call'

export interface ApiTaskError {
  code: AppErrorCode | (string & {})
  message: string
  detail?: string
}

export interface ApiTask {
  id: string
  type: TaskType
  status: TaskStatus
  title: string
  inputPaths: string[]
  outputPath: string
  /** 0~1；直播类恒为 -1 */
  progress: number
  speed: string
  etaSec: number
  /** 原始参数 JSON；直播任务的 params 已脱敏，不能用来重试 */
  params: string
  version: number
  error?: ApiTaskError | null
  createdAt: number
  startedAt: number
  finishedAt: number
  // v0.10：只有直播任务运行中才有值，只在内存
  fps?: number
  bitrateKbps?: number
  droppedFrames?: number
}

/** task:progress 载荷（契约第 5 节；后三项只有直播任务有） */
export interface TaskProgressPayload {
  id: string
  version: number
  progress: number
  speed: string
  etaSec: number
  outTimeSec: number
  fps?: number
  bitrateKbps?: number
  droppedFrames?: number
}

/** task:status 载荷 */
export interface TaskStatusPayload {
  id: string
  version: number
  status: TaskStatus
  error?: ApiTaskError | null
  outputPath?: string
  finishedAt?: number
}

/** Wails 生成的 store.Task（或事件里的对象）→ ApiTask：error 为 null / 缺省统一成 null，数值缺省补 0 */
export function toApiTask(raw: unknown): ApiTask {
  const r = asRecord(raw)
  const e = asRecord(r.error)
  const num = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) ? v : 0)
  const str = (v: unknown): string => (typeof v === 'string' ? v : '')
  const opt = (k: 'fps' | 'bitrateKbps' | 'droppedFrames') => (typeof r[k] === 'number' ? { [k]: r[k] as number } : {})
  return {
    id: str(r.id),
    type: str(r.type) as ApiTask['type'],
    status: str(r.status) as ApiTask['status'],
    title: str(r.title),
    inputPaths: Array.isArray(r.inputPaths) ? r.inputPaths.filter((x): x is string => typeof x === 'string') : [],
    outputPath: str(r.outputPath),
    progress: typeof r.progress === 'number' ? r.progress : 0,
    speed: str(r.speed),
    etaSec: num(r.etaSec),
    params: str(r.params),
    version: num(r.version),
    error: typeof e.code === 'string' && e.code ? { code: e.code, message: str(e.message), detail: str(e.detail) || undefined } : null,
    createdAt: num(r.createdAt),
    startedAt: num(r.startedAt),
    finishedAt: num(r.finishedAt),
    ...opt('fps'),
    ...opt('bitrateKbps'),
    ...opt('droppedFrames'),
  }
}

function asRecord(v: unknown): Record<string, unknown> {
  return typeof v === 'object' && v !== null ? (v as Record<string, unknown>) : {}
}
