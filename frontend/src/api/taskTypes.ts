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
  // v0.18（契约 9.7）：任务实际使用的视频编码器；没有视频编码的任务缺省
  /** 如 h264_nvenc / libx264 / libx265 / libvpx-vp9 / gif / copy */
  encoder?: string
  /** 设备 id（nvidia / intel / amd / apple …）；CPU 编码为 'cpu'；copy 缺省 */
  encoderDevice?: string
  /** 想用硬件但实际用了 CPU（设备不可用，或硬件编码启动失败后自动用 CPU 重试） */
  hwFallback?: boolean
  /** 回退原因（固定枚举，一行，不含路径）：device_unavailable / nvenc_init_failed / qsv_init_failed / amf_init_failed / videotoolbox_failed / encoder_unavailable / encoder_start_failed */
  hwFallbackReason?: string
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
  // v0.18：与 Task 的同名字段一致（缺省 = 没有视频编码器信息）
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
}

/** task:status 载荷 */
export interface TaskStatusPayload {
  id: string
  version: number
  status: TaskStatus
  error?: ApiTaskError | null
  outputPath?: string
  /** running 与四种终态事件都带；排队中被取消则缺省 */
  startedAt?: number
  finishedAt?: number
  /** v0.18：running / 终态事件带；运行中硬件编码回退 CPU 时补发一条 running 事件更新这些字段 */
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
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
    ...(str(r.encoder) ? { encoder: str(r.encoder) } : {}),
    ...(str(r.encoderDevice) ? { encoderDevice: str(r.encoderDevice) } : {}),
    ...(r.hwFallback === true ? { hwFallback: true } : {}),
    ...(str(r.hwFallbackReason) ? { hwFallbackReason: str(r.hwFallbackReason) } : {}),
  }
}

function asRecord(v: unknown): Record<string, unknown> {
  return typeof v === 'object' && v !== null ? (v as Record<string, unknown>) : {}
}
