/**
 * 转换页 v2（转换记录）的数据层。页面 / store 只用这里导出的函数和类型。
 *
 * 名字和形状按契约 v0.23（已合入 v2 da6c6eb，docs/architecture/contract.md §6.14、§5、§6.6）。后端实现还在并行开发、绑定还没生成，
 * 所以真实调用集中在 api/convertRecordsBinding.ts（V023_METHODS）。CONVERT_V2_BACKEND_READY（api/flags.ts）为 false、
 * 或纯浏览器（没有 window.go）时，全部走 api/convertRecordsMock.ts 的模拟。后端合入、绑定生成并联调后核对 binding 文件、把开关改成 true。
 */
import { CONVERT_V2_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'
import { probeFiles, type ProbeResult } from '@/api/media'
import type { TaskError, TaskStatus } from '@/stores/tasks'
import type { store as goStore } from '../../wailsjs/go/models'
import * as real from '@/api/convertRecordsBinding'
import * as mock from '@/api/convertRecordsMock'

// ---------------- 契约类型（§6.14.2；生成 models.ts 后可换成生成的类型） ----------------

/** Task.result：只有成功的 convert 任务有（探测失败时可能只有 sizeBytes，连 stat 都失败则没有） */
export interface TaskResult {
  sizeBytes: number
  durationSec?: number
  width?: number
  height?: number
  audioBitrateKbps?: number
}

/** 契约第 3 节的 Task，含 v0.23 字段（这里只列转换页用到的） */
export interface V023Task {
  id: string
  type: string
  status: TaskStatus
  title: string
  inputPaths: string[]
  outputPath: string
  progress: number
  speed: string
  etaSec: number
  params: string
  version: number
  error?: TaskError | null
  createdAt: number
  startedAt: number
  finishedAt: number
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
  sourceId?: string
  hiddenInTaskCenter: boolean
  result?: TaskResult
}

export interface ConvertSource {
  sourceId: string
  path: string
  name: string
  addedAt: number
  lastActivityAt: number
  /** 按 path_key 关联的 media 表缓存，可能没有；**hasVideo / hasAudio 恒为 false，不能拿来做冲突预检**（要以当次 Probe 为准） */
  media?: goStore.MediaInfo
}

export interface ConvertSourceEntry {
  source: ConvertSource
  /** 最新的 recordLimit 条，createdAt 倒序 */
  records: V023Task[]
  recordCount: number
  /** 只有 SearchSources：源文件名命中 */
  nameMatched?: boolean
  /** 只有 SearchSources：输出文件名命中的记录 id */
  matchedTaskIds?: string[]
}

export interface ConvertSourceFilter {
  /** 源文件行数，默认 50，最大 200 */
  limit: number
  offset: number
  /** 每行内嵌的记录数，默认 20，最大 100 */
  recordLimit: number
}
export interface ConvertSearchFilter extends ConvertSourceFilter {
  keyword: string
}
export interface ConvertSourcePage {
  items: ConvertSourceEntry[]
  total: number
}
export interface TaskPage {
  items: V023Task[]
  total: number
}

export interface AddSourceResult {
  path: string
  source?: ConvertSource
  existed: boolean
  error?: TaskError
}

/** 转换参数（ffmpeg.ConvertOptions） */
export interface RecordOptions {
  container: string
  videoCodec: string
  audioCodec: string
  width: number
  height: number
  fps: number
  videoBitrate: number
  audioBitrate: number
  crf: number
  targetSizeMb: number
  trimStart: number
  trimEnd: number
}

export interface ConvertSubmitRequest {
  sourceIds: string[]
  options: RecordOptions
  outputDir: string
  presetId: string
}

export interface TaskPathCheck {
  taskId: string
  found: boolean
  inputExists: boolean
  outputExists: boolean
}
export interface SourcePathCheck {
  sourceId: string
  found: boolean
  exists: boolean
}

export interface DeleteFailure {
  taskId: string
  path?: string
  /** in_use | permission | not_task_output | io | still_running（只追加） */
  reason: string
  /** 固定中文文案（§6.14.4） */
  message: string
}
export interface DeleteResult {
  deletedTaskIds: string[]
  deletedSourceIds: string[]
  deletedFiles: number
  failures: DeleteFailure[]
}

export interface PreviewURL {
  url: string
  mime: string
  size: number
}

/** 默认分页（§6.14.3）：首屏 50 行、每行内嵌 20 条；“加载更早的记录”再取 50 行；“展开更多”每次 50 条 */
export const SOURCE_PAGE = 50
export const RECORD_LIMIT = 20
export const MORE_RECORDS = 50

// ---------------- 转换页用的记录形状（由 Task 解析） ----------------

/** convert 任务的 params（§6.9 + §6.14.2 的三个快照） */
export interface ConvertParamsSnap {
  input: string
  options: Partial<RecordOptions>
  outputDir: string
  /** 三个快照键：v0.23 之前的旧任务没有（undefined），自定义参数为 '' */
  presetId?: string
  presetName?: string
  paramsSummary?: string
}
export function parseParams(params: string | undefined): ConvertParamsSnap {
  try {
    const p = params ? JSON.parse(params) : {}
    return {
      input: typeof p.input === 'string' ? p.input : '',
      options: p.options && typeof p.options === 'object' ? p.options : {},
      outputDir: typeof p.outputDir === 'string' ? p.outputDir : '',
      ...(typeof p.presetId === 'string' ? { presetId: p.presetId } : {}),
      ...(typeof p.presetName === 'string' ? { presetName: p.presetName } : {}),
      ...(typeof p.paramsSummary === 'string' ? { paramsSummary: p.paramsSummary } : {}),
    }
  } catch {
    return { input: '', options: {}, outputDir: '' }
  }
}

/** 转换页的一条记录（由 convert 任务解析；实时状态再由任务 store 覆盖） */
export interface ConvertRecord {
  id: string
  sourceId: string
  inputPath: string
  outputPath: string
  title: string
  status: TaskStatus
  progress: number
  speed: string
  etaSec: number
  error: TaskError | null
  version: number
  createdAt: number
  startedAt: number
  finishedAt: number
  options: Partial<RecordOptions>
  /** 三个快照；v0.23 之前的旧任务没有（undefined） */
  presetId?: string
  presetName?: string
  paramsSummary?: string
  result?: TaskResult
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
}
export function recordOf(t: V023Task): ConvertRecord {
  const p = parseParams(t.params)
  return {
    id: t.id, sourceId: t.sourceId ?? '', inputPath: t.inputPaths?.[0] ?? p.input, outputPath: t.outputPath ?? '', title: t.title ?? '',
    status: t.status, progress: typeof t.progress === 'number' ? t.progress : 0, speed: t.speed ?? '', etaSec: t.etaSec ?? 0,
    error: t.error && t.error.code ? { code: t.error.code, message: t.error.message ?? '', ...(t.error.detail ? { detail: t.error.detail } : {}) } : null,
    version: t.version ?? 0, createdAt: t.createdAt ?? 0, startedAt: t.startedAt ?? 0, finishedAt: t.finishedAt ?? 0, options: p.options,
    ...(p.presetId !== undefined ? { presetId: p.presetId } : {}), ...(p.presetName !== undefined ? { presetName: p.presetName } : {}),
    ...(p.paramsSummary !== undefined ? { paramsSummary: p.paramsSummary } : {}),
    ...(t.status === 'succeeded' && t.result ? { result: { ...t.result } } : {}),
    ...(t.encoder ? { encoder: t.encoder } : {}), ...(t.encoderDevice ? { encoderDevice: t.encoderDevice } : {}),
    ...(t.hwFallback ? { hwFallback: true } : {}), ...(t.hwFallbackReason ? { hwFallbackReason: t.hwFallbackReason } : {}),
  }
}

// ---------------- 调用（名字与契约 v0.23 一致） ----------------

/** 是否走真实后端（开关打开且在 Wails 里） */
export const convertV2IsReal = (): boolean => CONVERT_V2_BACKEND_READY && hasWailsBackend()
const api = () => (convertV2IsReal() ? real : mock)

// ConvertService
export const addSources = (paths: string[]): Promise<AddSourceResult[]> => api().AddSources(paths)
export const listSources = (f: ConvertSourceFilter): Promise<ConvertSourcePage> => api().ListSources(f)
export const listSourceRecords = (sourceId: string, limit: number, offset: number): Promise<TaskPage> => api().ListSourceRecords(sourceId, limit, offset)
export const searchSources = (f: ConvertSearchFilter): Promise<ConvertSourcePage> => api().SearchSources(f)
export const checkSources = (ids: string[]): Promise<SourcePathCheck[]> => api().CheckSources(ids)
/** “将保存为”：返回完整输出路径（不占位，提交时可能不同） */
export const previewOutputName = (sourceId: string, opts: RecordOptions, outputDir: string): Promise<string> => api().PreviewOutputName(sourceId, opts, outputDir)
export const submitSources = (req: ConvertSubmitRequest): Promise<V023Task[]> => api().SubmitSources(req)
/** 只用于已成功的记录：又转一次，新增一条（其余状态 TASK_CONFLICT；失败 / 取消 / 中断用 TaskService.Retry 原地重试） */
export const reconvert = (taskId: string): Promise<V023Task> => api().Reconvert(taskId)
export const deleteRecords = (taskIds: string[], deleteOutputs: boolean): Promise<DeleteResult> => api().DeleteRecords(taskIds, deleteOutputs)
export const deleteSource = (sourceId: string, deleteOutputs: boolean): Promise<DeleteResult> => api().DeleteSource(sourceId, deleteOutputs)
export const getSourcePreviewURL = (sourceId: string): Promise<PreviewURL> => api().GetSourcePreviewURL(sourceId)
export const openSourceWithSystem = (sourceId: string): Promise<void> => api().OpenSourceWithSystem(sourceId)
export const revealSource = (sourceId: string): Promise<void> => api().RevealSource(sourceId)

// TaskService
/** 任务中心“隐藏已结束”：所有类型、所有已结束且未隐藏的任务只隐藏，不删除；返回本次隐藏的条数 */
export const hideFinishedInTaskCenter = (): Promise<number> => api().HideFinishedInTaskCenter()
/** 任务中心“显示已隐藏”里的“取消隐藏”（§6.14.11）：幂等；每个真正取消隐藏的任务发一条 task:status（hiddenInTaskCenter:false） */
export const unhideInTaskCenter = (ids: string[]): Promise<void> => api().UnhideInTaskCenter(ids)
/** 任务中心历史（带 includeHidden）。只有“显示已隐藏”打开时用；平时仍走任务 store 原来的 List */
export const listTasks = (f: { types: string[]; statuses: TaskStatus[]; limit: number; offset: number; includeHidden: boolean }): Promise<TaskPage> => api().List(f)
export const checkPaths = (taskIds: string[]): Promise<TaskPathCheck[]> => api().CheckPaths(taskIds)
export const getPreviewURL = (taskId: string, which: 'input' | 'output'): Promise<PreviewURL> => api().GetPreviewURL(taskId, which)
/** 用系统默认程序打开；没有关联程序 NOT_FOUND（reason=no_app，界面提示“没有找到能打开这个文件的程序”） */
export const openWithSystem = (taskId: string, which: 'input' | 'output'): Promise<void> => api().OpenWithSystem(taskId, which)
/** 在文件夹中显示某条记录的输出：#82 没有按任务 id 的接口，沿用 SystemService.RevealInFolder(outputPath)（6.8）；模拟时不调用系统 */
/** v0.23.1：完成记录“打开所在文件夹”按 id（转换页不再调 RevealInFolder(path)） */
export const revealRecord = (taskId: string): Promise<void> => api().RevealRecord(taskId)
/** v0.23.1：取单个源文件行 */
export const getSource = (sourceId: string): Promise<ConvertSourceEntry> => api().GetSource(sourceId)

// ---------------- 探测（MediaService.Probe） ----------------
/**
 * 冲突预检要用当次探测的 hasVideo / hasAudio（ConvertSource.media 的这两个字段恒为 false，不能用）。
 * 真实后端直接 MediaService.Probe；模拟时场景里的源文件给场景的探测结果，其它路径照常探测。
 */
export const probeSources = (paths: string[], onBatch?: (r: ProbeResult[]) => void): Promise<ProbeResult[]> =>
  convertV2IsReal() ? probeFiles(paths, onBatch) : mock.probeMock(paths, onBatch)

// ---------------- 缩略图 ----------------
/**
 * 缩略图三种状态（契约 §6.14.10 + 设计）：
 *   img     有画面：dataUrl（data:image/jpeg;base64,...）直接放进 <img>
 *   missing 文件不在（NOT_FOUND reason=file，含记录不是 succeeded）：虚线占位
 *   type    做不出缩略图（UNSUPPORTED reason=format，如纯音频；以及 ffmpeg 缺失 / 截图失败 / 超时）：按类型显示图标
 *   gone    记录 / 源文件行已不存在（NOT_FOUND reason=record）：该行会被删掉，先按类型图标显示
 */
export type ThumbState = { kind: 'img'; url: string } | { kind: 'missing' } | { kind: 'type' } | { kind: 'gone' }
export function thumbStateOf(e: unknown): ThumbState {
  const err = e as { code?: string; detail?: string }
  const reason = /^reason=(\w+)/.exec(err?.detail ?? '')?.[1]
  if (err?.code === 'NOT_FOUND' && reason === 'file') return { kind: 'missing' }
  if (err?.code === 'NOT_FOUND' && reason === 'record') return { kind: 'gone' }
  return { kind: 'type' }
}
const toThumb = (p: Promise<string>): Promise<ThumbState> =>
  p.then((url) => (url ? ({ kind: 'img', url } as ThumbState) : ({ kind: 'type' } as ThumbState)), thumbStateOf)
/** 源文件行的缩略图（ConvertService.GetSourceThumbnail，行进入可视区域时逐个调用） */
export const getSourceThumbnail = (sourceId: string): Promise<ThumbState> => toThumb(api().GetSourceThumbnail(sourceId))
/** 记录输出文件的缩略图（ConvertService.GetRecordThumbnail） */
export const getRecordThumbnail = (taskId: string): Promise<ThumbState> => toThumb(api().GetRecordThumbnail(taskId))
