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
export function toApiTask(raw: any): ApiTask {
  const e = raw?.error
  return {
    id: raw.id,
    type: raw.type,
    status: raw.status,
    title: raw.title ?? '',
    inputPaths: raw.inputPaths ?? [],
    outputPath: raw.outputPath ?? '',
    progress: typeof raw.progress === 'number' ? raw.progress : 0,
    speed: raw.speed ?? '',
    etaSec: raw.etaSec ?? 0,
    params: raw.params ?? '',
    version: raw.version ?? 0,
    error: e && e.code ? { code: e.code, message: e.message ?? '', detail: e.detail || undefined } : null,
    createdAt: raw.createdAt ?? 0,
    startedAt: raw.startedAt ?? 0,
    finishedAt: raw.finishedAt ?? 0,
    ...(raw.fps !== undefined ? { fps: raw.fps } : {}),
    ...(raw.bitrateKbps !== undefined ? { bitrateKbps: raw.bitrateKbps } : {}),
    ...(raw.droppedFrames !== undefined ? { droppedFrames: raw.droppedFrames } : {}),
  }
}
