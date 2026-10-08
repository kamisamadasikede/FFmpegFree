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
  // v0.23（契约 6.14）
  /** 只有 convert 任务有：所属源文件行 */
  sourceId?: string
  /** 任务中心已隐藏（“隐藏已结束”）；转换页照常显示；原地重试时清回 false */
  hiddenInTaskCenter?: boolean
  /** 只有成功的 convert 任务有：完成时探测输出得到的信息 */
  result?: ApiTaskResult
  // v0.24（契约 6.17.2）：只有 convert 任务会用到
  /** true = 正在原地重转（status 是 queued / running）；旧的 result / outputPath / finishedAt / params 不变 */
  reconverting?: boolean
  /** 最近一次重转失败的信息；重转成功、被取消或又开始一次重转时清空 */
  lastReconvertError?: ApiReconvertError
}

/** v0.24 Task.lastReconvertError（6.17.2） */
export interface ApiReconvertError {
  code: string
  message: string
  detail?: string
  at: number
}

/** v0.23 Task.result（6.14.2） */
export interface ApiTaskResult {
  sizeBytes: number
  durationSec?: number
  width?: number
  height?: number
  audioBitrateKbps?: number
  /** v0.24：结果的警告（目前只有 "short_output" = 输出时长不到预期的 90% 且短了 2 秒以上，6.14.6）；没有时缺省 */
  warnings?: string[]
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
  /** v0.23：终态事件一定带（canceled / failed / interrupted 保留结束那一刻的值）；原地重试的 queued 事件带 0 */
  progress?: number
  /** v0.23：成功的 convert 任务的终态事件带 */
  result?: ApiTaskResult
  /** v0.23：原地重试发出的那一条 queued 事件为 true（没有 task:created）；前端据此清掉上一轮的进度、编码器、错误、result，并把 hiddenInTaskCenter 清回 false */
  retried?: boolean
  /** v0.23：只出现在 UnhideInTaskCenter 的事件（false，status 不变，只改这一项和 version）和 retried 事件上 */
  hiddenInTaskCenter?: boolean
  /** v0.24（6.17.2）：凡是 convert 任务的 task:status 都带 */
  reconverting?: boolean
  /** v0.24：只在重转结束那一次的终态事件里出现 */
  reconvertOutcome?: 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  /** v0.24：重转失败的终态事件带 */
  lastReconvertError?: ApiReconvertError
}

/** 事件 / 接口里的 result → ApiTaskResult（sizeBytes 必须是数；其余只取有限数值） */
export function toTaskResult(raw: unknown): ApiTaskResult | undefined {
  const r = asRecord(raw)
  if (typeof r.sizeBytes !== 'number' || !Number.isFinite(r.sizeBytes)) return undefined
  const out: ApiTaskResult = { sizeBytes: r.sizeBytes }
  for (const k of ['durationSec', 'width', 'height', 'audioBitrateKbps'] as const) if (typeof r[k] === 'number' && Number.isFinite(r[k])) out[k] = r[k] as number
  if (Array.isArray(r.warnings)) {
    const w = r.warnings.filter((x): x is string => typeof x === 'string' && !!x)
    if (w.length) out.warnings = w
  }
  return out
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
    ...(str(r.sourceId) ? { sourceId: str(r.sourceId) } : {}),
    ...(r.hiddenInTaskCenter === true ? { hiddenInTaskCenter: true } : {}),
    ...(toTaskResult(r.result) ? { result: toTaskResult(r.result) } : {}),
    ...(r.reconverting === true ? { reconverting: true } : {}),
    ...(toReconvertError(r.lastReconvertError) ? { lastReconvertError: toReconvertError(r.lastReconvertError) } : {}),
  }
}

/** v0.24 lastReconvertError → ApiReconvertError（code 必须是非空字符串） */
export function toReconvertError(raw: unknown): ApiReconvertError | undefined {
  const r = asRecord(raw)
  if (typeof r.code !== 'string' || !r.code) return undefined
  return { code: r.code, message: typeof r.message === 'string' ? r.message : '', ...(typeof r.detail === 'string' && r.detail ? { detail: r.detail } : {}), at: typeof r.at === 'number' ? r.at : 0 }
}

function asRecord(v: unknown): Record<string, unknown> {
  return typeof v === 'object' && v !== null ? (v as Record<string, unknown>) : {}
}
