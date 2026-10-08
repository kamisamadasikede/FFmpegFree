/**
 * 转换页 v2（转换记录）的数据层。页面 / store 只用这里导出的函数和类型。
 *
 * 名字和形状按契约 v0.23–v0.23.3（docs/architecture/contract.md §6.14、§5、§6.6），后端 #83 / #84 / #85 已合入。
 * 真实调用在 api/convertRecordsBinding.ts（生成的 Wails 绑定，末尾有和生成类型的编译期对照）；CONVERT_V2_BACKEND_READY（api/flags.ts）为 false、
 * 或纯浏览器（没有 window.go）时，全部走 api/convertRecordsMock.ts 的模拟。
 */
import { CONVERT_V2_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'
import { probeFiles, type ProbeResult } from '@/api/media'
import { revealInFolder } from '@/api/system'
import type { TaskError, TaskStatus } from '@/stores/tasks'
import type { store as goStore } from '../../wailsjs/go/models'
import * as real from '@/api/convertRecordsBinding'
import * as mock from '@/api/convertRecordsMock'

// ---------------- 契约类型（§6.14.2；与生成的 models.ts 逐字段对照见 convertRecordsBinding.ts 末尾） ----------------

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
  /** v0.24（6.17.2）：原地重转中；后端总是返回，前端类型先放可选（最小改动） */
  reconverting?: boolean
  /** v0.24：最近一次重转失败的信息（成功 / 取消时没有） */
  lastReconvertError?: { code: string; message: string; detail?: string; at: number } | null
}

export interface ConvertSource {
  sourceId: string
  path: string
  name: string
  addedAt: number
  lastActivityAt: number
  /** 持久化的媒体信息，可能没有；G3 起 hasVideo / hasAudio 是真实值（可用于显示），冲突预检仍以当次 Probe 为准 */
  media?: goStore.MediaInfo
  /** v0.24（6.15.3）：副本字段；后端总是返回（copyError 除外），前端类型先放可选（最小改动）。copyState: none | copying | ready | failed | canceled */
  originalPath?: string
  storedPath?: string
  copyState?: string
  copiedBytes?: number
  totalBytes?: number
  copyError?: TaskError | null
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

/**
 * v0.23.1 ListSources 的 status：'' 全部；active = 有任一排队中 / 进行中记录的行；failed = 有任一失败 / 中断记录的行（已取消不算）。
 * 其它值 INVALID_ARGUMENT。分页、排序不变；每行内嵌的最新记录和 recordCount 不按状态过滤（要找失败的那条，用户展开这一行）。
 */
export type ConvertSourceStatus = '' | 'active' | 'failed'
export const CONVERT_SOURCE_STATUSES: readonly ConvertSourceStatus[] = ['', 'active', 'failed']
export interface ConvertSourceFilter {
  /** 源文件行数，默认 50，最大 200 */
  limit: number
  offset: number
  /** 每行内嵌的记录数，默认 20，最大 100 */
  recordLimit: number
  /** v0.23.1，可选，缺省 = ''（全部） */
  status?: ConvertSourceStatus
}
/** SearchSources 也带同样的 status（v0.23.2，语义与 ListSources 相同） */
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
  /** v0.24.1（6.17.1）："replace" | "regenerate" | ""（不能重转，原因见 reconvertBlock） */
  reconvertMode?: string
  /** v0.24.1：reconvertMode="" 时 invalid_state | copy_not_ready | source_missing | output_moved */
  reconvertBlock?: string
}
export interface SourcePathCheck {
  sourceId: string
  found: boolean
  exists: boolean
  /** v0.24：原文件 / 副本各自是否还在 */
  originalExists?: boolean
  storedExists?: boolean
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
/** v0.23.1：完成记录“打开所在文件夹”按 id（转换页不再调 RevealInFolder(path)） */
export const revealRecord = (taskId: string): Promise<void> => api().RevealRecord(taskId)
/**
 * 删除后有文件没删成（DeleteResult.failures）时 toast 里的“打开所在文件夹”：记录已经删了，没有 id 可用，
 * 用 failures[0].path 调旧的 SystemService.RevealInFolder（v0.23.2 架构师批准的唯一例外）。模拟时不调用系统。
 */
/**
 * 删除失败后“打开所在文件夹”：旧的 SystemService.RevealInFolder(path)（契约 v0.23.2 例外；v0.23.3：删除后 10 分钟内任何位置都放行）。
 * 超过 10 分钟 / 重启后 INVALID_ARGUMENT，文件被移走 NOT_FOUND——页面用 revealDeleteFailureText 出提示。
 */
export const revealDeleteFailure = (path: string): Promise<void> => (hasWailsBackend() ? revealInFolder(path) : mock.revealDeleteFailureMock(path))
/** v0.23.1：取单个源文件行 */
export const getSource = (sourceId: string): Promise<ConvertSourceEntry> => api().GetSource(sourceId)

// ---------------- 探测（MediaService.Probe） ----------------
/**
 * 冲突预检要用当次探测的 hasVideo / hasAudio（ConvertSource.media 只用来显示，文件可能已经变了）。
 * 真实后端直接 MediaService.Probe；模拟时场景里的源文件给场景的探测结果，其它路径照常探测。
 */
export const probeSources = (paths: string[], onBatch?: (r: ProbeResult[]) => void): Promise<ProbeResult[]> =>
  convertV2IsReal() ? probeFiles(paths, onBatch) : mock.probeMock(paths, onBatch)

// ---------------- 缩略图 ----------------
/**
 * 缩略图状态（契约 §6.14.10 + 设计）：
 *   img     有画面：dataUrl（data:image/jpeg;base64,...）直接放进 <img>
 *   missing 文件不在（NOT_FOUND reason=file，含记录不是 succeeded）：虚线占位
 *   type    做不出缩略图：按类型封面显示（视频胶片 / 图片 / 音符）。
 *           UNSUPPORTED reason=format（如纯音频）是正常情况；其它错误（转换组件缺失 / 截图失败 / 超时 / 返回空）
 *           带 failed=true 并 console.warn 错误码，源文件变了或转换组件就绪后会重取
 *   gone    记录 / 源文件行已不存在（NOT_FOUND reason=record）：该行会被删掉，先按类型封面显示
 */
export type ThumbState = { kind: 'img'; url: string } | { kind: 'missing' } | { kind: 'type'; failed?: boolean } | { kind: 'gone' }
export function thumbStateOf(e: unknown, what = ''): ThumbState {
  const err = e as { code?: string; message?: string; detail?: string }
  const reason = /^reason=(\w+)/.exec(err?.detail ?? '')?.[1]
  if (err?.code === 'NOT_FOUND' && reason === 'file') return { kind: 'missing' }
  if (err?.code === 'NOT_FOUND' && reason === 'record') return { kind: 'gone' }
  if (err?.code === 'UNSUPPORTED' && reason === 'format') return { kind: 'type' }
  console.warn(`[缩略图] 取${what || '缩略图'}失败，改用类型封面`, err?.code ?? 'UNKNOWN', err?.detail ?? '', err?.message ?? e)
  return { kind: 'type', failed: true }
}
/** 后端返回的必须是能直接放进 <img> 的地址（data: / http(s): / blob:）；空或别的格式按失败处理 */
export const isThumbUrl = (url: unknown): url is string => typeof url === 'string' && /^(data:image\/|https?:|blob:)/.test(url)
const toThumb = (p: Promise<string>, what: string): Promise<ThumbState> =>
  p.then(
    (url) => (isThumbUrl(url) ? ({ kind: 'img', url } as ThumbState) : thumbStateOf({ code: 'EMPTY_THUMB', detail: `url=${String(url).slice(0, 40)}` }, what)),
    (e) => thumbStateOf(e, what),
  )
/** 源文件行的缩略图（ConvertService.GetSourceThumbnail，行进入可视区域时逐个调用） */
export const getSourceThumbnail = (sourceId: string): Promise<ThumbState> => toThumb(api().GetSourceThumbnail(sourceId), `源文件缩略图 sourceId=${sourceId}`)
/** 记录输出文件的缩略图（ConvertService.GetRecordThumbnail） */
export const getRecordThumbnail = (taskId: string): Promise<ThumbState> => toThumb(api().GetRecordThumbnail(taskId), `记录缩略图 taskId=${taskId}`)
