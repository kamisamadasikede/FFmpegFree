import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import * as TaskBinding from '../../wailsjs/go/app/TaskService'
import { store as goStore } from '../../wailsjs/go/models'
import { call, toAppError } from '@/api/call'
import { hasWailsBackend, onEvent, onSimEvent, previewParams } from '@/services/wails'
import { cancelSimTask, getSimTask, isSimTask, listSimFinished, removeSimTasks, retrySimTask } from '@/api/sim'
import { toInstallProgress, useFFmpegStore } from '@/stores/ffmpeg'
import { buildPreviewActive, buildPreviewHistory, PREVIEW_LOG, unmappedErrorPreview } from '@/stores/tasks.preview'
import { mergeEncoderFields, pickEncoderFields } from '@/api/encoderTask'
import { convertV2IsReal, hideFinishedInTaskCenter, listTasks, unhideInTaskCenter } from '@/api/convertRecords'
import { toReconvertError, toTaskResult, type ApiReconvertError, type ApiTaskResult } from '@/api/taskTypes'

/** 契约第 3 节。注意 canceled 只有一个 l */
export type TaskStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
export type TaskType =
  | 'convert'
  | 'edit_export'
  | 'office_pdf'
  | 'doc_convert' // v0.26 文档多格式转换
  | 'live_file_push'
  | 'live_screen_push'
  | 'ffmpeg_install'

/**
 * 任务中心认识的类型（契约 v0.10~v0.12）。库里若还有旧类型记录（后端保留但不再产生）、或将来出现新类型，
 * 一律忽略（不展示、不报错、不影响其他任务）。旧类型 id 只允许出现在 isLegacyTaskType 里。
 */
export const KNOWN_TASK_TYPES: readonly string[] = ['convert', 'edit_export', 'office_pdf', 'doc_convert', 'live_file_push', 'live_screen_push', 'ffmpeg_install']
const LEGACY_TASK_TYPES: readonly string[] = ['live_relay', 'live_record_push', 'edit_render']
export const isLegacyTaskType = (t: unknown): boolean => typeof t === 'string' && LEGACY_TASK_TYPES.includes(t)
export const isKnownTaskType = (t: unknown): boolean => typeof t === 'string' && !isLegacyTaskType(t) && KNOWN_TASK_TYPES.includes(t)

/** “用时”毫秒数：两端都有值且结束不早于开始才返回，否则 null（不显示，绝不出现 NaN / 负数） */
export function elapsedMs(startedAt?: number, finishedAt?: number): number | null {
  if (typeof startedAt !== 'number' || typeof finishedAt !== 'number') return null
  if (!Number.isFinite(startedAt) || !Number.isFinite(finishedAt) || startedAt <= 0 || finishedAt <= 0 || finishedAt < startedAt) return null
  return finishedAt - startedAt
}

export interface TaskError {
  code: string
  message: string
  detail?: string
}

/** 前端使用的任务对象：生成类型里 status/type 是 string、error 可能为 null，这里收窄 */
export interface TaskItem {
  id: string
  type: TaskType | (string & {})
  status: TaskStatus
  title: string
  inputPaths: string[]
  outputPath: string
  /** 0~1；直播类恒为 -1 */
  progress: number
  speed: string
  etaSec: number
  outTimeSec: number
  params: string
  version: number
  error?: TaskError | null
  createdAt: number
  startedAt: number
  finishedAt: number
  /** 直播任务运行中才有（契约 v0.10）：当前输出帧率 / 近 5 秒码率 kbit/s / 累计丢帧 */
  fps?: number
  bitrateKbps?: number
  droppedFrames?: number
  /** v0.18（契约 9.7）：实际使用的视频编码器 / 设备 / 是否回退 CPU / 原因；没有视频编码的任务缺省 */
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
  /** v0.23（契约 6.14）：convert 任务所属源文件行 / 任务中心已隐藏 / 成功时的输出信息 */
  sourceId?: string
  hiddenInTaskCenter?: boolean
  result?: ApiTaskResult
  /** v0.24（契约 6.17.2）：正在原地重转（status 是 queued / running，旧的 result / outputPath / finishedAt 不变） */
  reconverting?: boolean
  /** v0.24：最近一次重转失败的信息 */
  lastReconvertError?: ApiReconvertError
}

/** 终态任务的快照（见 useTaskStore 的 finalById） */
export interface FinalState {
  id: string
  status: TaskStatus
  error?: TaskError | null
  outputPath: string
  /** 0~1；不知道最后进度时（终态事件先于任何进度事件到达）为 0，成功恒为 1 */
  progress: number
  speed: string
  etaSec: number
  /** 任务开始时间（ms，前端算“用时”用；不知道时缺省 / 0） */
  startedAt?: number
  finishedAt: number
  params: string
  /** 编码器字段（契约 9.7）：终态后转换页 / 剪辑导出结果处还要显示“用了哪个设备 / 是否回退”，所以快照里也带上 */
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
  /** v0.23：成功的 convert 任务的输出信息（终态事件带） */
  result?: ApiTaskResult
}

/** 历史分组 → 状态过滤。失败只查 failed：interrupted 是「已中断」，不算失败（v0.25.3） */
export type HistoryGroup = 'all' | 'succeeded' | 'failed' | 'canceled'
const TERMINAL: TaskStatus[] = ['succeeded', 'failed', 'canceled', 'interrupted']
const GROUP_STATUSES: Record<HistoryGroup, TaskStatus[]> = {
  all: TERMINAL,
  succeeded: ['succeeded'],
  failed: ['failed'],
  canceled: ['canceled'],
}

export const isTerminal = (s: string): boolean => TERMINAL.includes(s as TaskStatus)
export const isLiveType = (t: string): boolean => t.startsWith('live_')
/**
 * 已下线功能的任务类型（剪辑功能已移除，老板决定 2026-10-08）：旧记录照常显示、可以移除，但不能重试。
 * 后端删掉 EditService 后不会再产生这类任务；仍保留在 KNOWN_TASK_TYPES 里，旧记录才不会被当成未知类型丢掉。
 */
export const RETIRED_TASK_TYPES: readonly string[] = ['edit_export']
export const isRetiredType = (t: string): boolean => RETIRED_TASK_TYPES.includes(t)
/** 能否重试：失败 / 已中断 / 已取消，且不是直播（直播回直播页重新推流）、不是已下线功能的任务 */
export const canRetryTask = (t: { status: string; type: string }): boolean =>
  (t.status === 'failed' || t.status === 'interrupted' || t.status === 'canceled') && !isLiveType(t.type) && !isRetiredType(t.type)

// ---- 事件 payload（契约第 5 节） ----
interface ProgressPayload {
  id: string
  version: number
  progress: number
  speed: string
  etaSec: number
  outTimeSec: number
  fps?: number
  bitrateKbps?: number
  droppedFrames?: number
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
}
interface StatusPayload {
  id: string
  version: number
  status: TaskStatus
  error?: TaskError | null
  outputPath?: string
  /** 后端 PR #32：running 事件与四种终态事件都带；排队中被取消则缺省 */
  startedAt?: number
  finishedAt?: number
  encoder?: string
  encoderDevice?: string
  hwFallback?: boolean
  hwFallbackReason?: string
  /** v0.23：终态事件一定带；原地重试的 queued 事件带 0 */
  progress?: number
  /** v0.23：成功的 convert 终态事件带 */
  result?: unknown
  /** v0.23：原地重试（同一个 id 回到 queued；没有 task:created） */
  retried?: boolean
  /** v0.23：UnhideInTaskCenter 的事件（false；status 是当前状态不变，只改这一项和 version）和 retried 事件带 */
  hiddenInTaskCenter?: boolean
  /** v0.24（6.17.2）：convert 任务的 task:status 都带 */
  reconverting?: boolean
  /** v0.24：重转结束那一次的终态事件才有 */
  reconvertOutcome?: 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  lastReconvertError?: unknown
}
/** v0.24：重转结束的事件（转换页据此给提示、重新取参数） */
export interface ReconvertEnd {
  id: string
  outcome: 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  error?: ApiReconvertError
}
interface RemovedPayload { ids: string[] }
type BufferedEvent =
  | { kind: 'created'; version: number; payload: goStore.Task }
  | { kind: 'progress'; version: number; payload: ProgressPayload }
  | { kind: 'status'; version: number; payload: StatusPayload }
  | { kind: 'removed'; version: number; payload: RemovedPayload }

function normalizeError(e: unknown): TaskError | null {
  const r = e as any
  return r && r.code ? { code: r.code, message: r.message ?? '', detail: r.detail || undefined } : null
}

export function normalizeTask(raw: goStore.Task | TaskItem): TaskItem {
  const r = raw as any
  return {
    id: r.id,
    type: r.type,
    status: r.status,
    title: r.title ?? '',
    inputPaths: r.inputPaths ?? [],
    outputPath: r.outputPath ?? '',
    progress: typeof r.progress === 'number' ? r.progress : 0,
    speed: r.speed ?? '',
    etaSec: r.etaSec ?? 0,
    outTimeSec: r.outTimeSec ?? 0,
    params: r.params ?? '',
    version: r.version ?? 0,
    error: normalizeError(r.error),
    createdAt: r.createdAt ?? 0,
    startedAt: r.startedAt ?? 0,
    finishedAt: r.finishedAt ?? 0,
    ...(r.fps !== undefined ? { fps: r.fps } : {}),
    ...(r.bitrateKbps !== undefined ? { bitrateKbps: r.bitrateKbps } : {}),
    ...(r.droppedFrames !== undefined ? { droppedFrames: r.droppedFrames } : {}),
    ...(r.encoder ? { encoder: r.encoder } : {}),
    ...(r.encoderDevice ? { encoderDevice: r.encoderDevice } : {}),
    ...(r.hwFallback ? { hwFallback: true } : {}),
    ...(r.hwFallbackReason ? { hwFallbackReason: r.hwFallbackReason } : {}),
    ...(typeof r.sourceId === 'string' && r.sourceId ? { sourceId: r.sourceId } : {}),
    ...(r.hiddenInTaskCenter === true ? { hiddenInTaskCenter: true } : {}),
    ...(toTaskResult(r.result) ? { result: toTaskResult(r.result) } : {}),
    ...(r.reconverting === true ? { reconverting: true } : {}),
    ...(toReconvertError(r.lastReconvertError) ? { lastReconvertError: toReconvertError(r.lastReconvertError) } : {}),
  }
}

const mergeEncoder = mergeEncoderFields // 逐字段合并，缺省不覆盖（api/encoderTask.ts，自检见 api.check.ts）

const PREVIEW_ACTIVE = previewParams.has('tasks')
const previewMode = !hasWailsBackend() && PREVIEW_ACTIVE

export const useTaskStore = defineStore('tasks', () => {
  /** queued + running，按 id 索引；对象原地修改（task:progress 只改进度字段，不替换对象） */
  const byId = reactive<Record<string, TaskItem>>({})
  const ready = ref(false) // ListActive 已完成并回放过缓冲
  const loadError = ref<TaskError | null>(null)

  /** 已经进入终态的任务的最后版本，用来丢弃它们之后才到的过期事件 */
  const finishedVersions = new Map<string, number>()
  function rememberFinished(id: string, version: number) {
    finishedVersions.set(id, version)
    if (finishedVersions.size > 500) finishedVersions.delete(finishedVersions.keys().next().value as string)
  }

  /**
   * 已收到 task:removed（或本地 remove() 已删除）的任务 id。
   * 任务 id 不复用，task:removed 又没有 version，所以之后到达的该 id 的 created/status/progress 一律丢弃，
   * 包括缓冲回放和 Get 补齐。Set 按插入顺序保存，超过上限时淘汰最旧的。
   */
  const REMOVED_CAP = 2000
  const removedIds = new Set<string>()
  function markRemoved(ids: Iterable<string>) {
    for (const id of ids) {
      removedIds.delete(id) // 重新插入，刷新它在淘汰顺序里的位置
      removedIds.add(id)
      finishedVersions.delete(id)
    }
    while (removedIds.size > REMOVED_CAP) removedIds.delete(removedIds.values().next().value as string)
  }

  /**
   * 刚结束的任务的终态快照（id → 状态 / 错误 / 输出路径 / 最后进度）。活动列表里终态任务会被移除，
   * 而转换页的文件行需要在任务结束后继续显示结果，所以在这里留一份（最多 500 条，先进先出）。
   * task:removed 会清掉对应条目。
   */
  const finalById = reactive<Record<string, FinalState>>({})
  function recordFinal(f: FinalState) {
    delete finalById[f.id]
    finalById[f.id] = f
    const keys = Object.keys(finalById)
    if (keys.length > 500) for (const k of keys.slice(0, keys.length - 500)) delete finalById[k]
  }
  /** 仅浏览器预览（?convert=…）用：直接放一个终态快照 */
  function seedFinal(f: FinalState) {
    recordFinal(f)
  }
  /** 该任务是否已被删除（收到 task:removed 或本地删除过） */
  function wasRemoved(id: string): boolean {
    return removedIds.has(id)
  }
  /** 转换页等按 id 取任务：活动的优先，其次是刚结束的终态快照 */
  function taskById(id: string): TaskItem | FinalState | undefined {
    return byId[id] ?? finalById[id]
  }

  /**
   * 活动列表相关的派生数据，一次遍历算完：排序结果、各种计数、排队序号。
   * 只读取 status / startedAt / createdAt / type，不碰 progress / speed / etaSec，
   * 所以高频的 task:progress 不会让它重新计算（只有任务增删或状态变化才会）。
   * 排序：运行中在前（新开始的在前，与原型一致），排队的在后（先提交的在前 = 队列顺序）。
   */
  const derived = computed(() => {
    const running: TaskItem[] = []
    const queued: TaskItem[] = []
    let liveActive = 0
    for (const t of Object.values(byId)) {
      if (isLiveType(t.type)) liveActive++
      if (t.status === 'running') running.push(t)
      else queued.push(t)
    }
    running.sort((a, b) => b.startedAt - a.startedAt || b.createdAt - a.createdAt)
    queued.sort((a, b) => a.createdAt - b.createdAt)
    // 排队序号只对 batch 池任务有意义（直播不排队）
    const positions = new Map<string, number>()
    let n = 0
    for (const t of queued) if (!isLiveType(t.type)) positions.set(t.id, ++n)
    return { list: [...running, ...queued], running: running.length, queued: queued.length, liveActive, positions }
  })
  const active = computed<TaskItem[]>(() => derived.value.list)
  /** 侧边栏"任务中心"角标 = 进行中的任务数（运行中 + 排队中，含直播推流） */
  const runningCount = computed(() => derived.value.list.length)
  const hasRunning = computed(() => runningCount.value > 0)
  const runningOnly = computed(() => derived.value.running)
  const queuedCount = computed(() => derived.value.queued)
  /** 进行中的直播任务数（统计条上的"直播推流单独计数"提示用） */
  const liveActiveCount = computed(() => derived.value.liveActive)
  /** 排队序号（从 1 开始），直播任务或不在队列里返回 0 */
  function queuePosition(id: string): number {
    return derived.value.positions.get(id) ?? 0
  }

  // ---- 历史（分页，按需加载） ----
  const history = ref<TaskItem[]>([])
  const historyTotal = ref(0)
  const historyLoading = ref(false)
  const historyLoaded = ref(false)
  const historyError = ref<TaskError | null>(null)
  /** includeHidden：任务中心“显示已隐藏”开关（v0.23 TaskFilter.includeHidden） */
  const historyFilter = reactive<{ group: HistoryGroup; types: string[]; page: number; pageSize: number; includeHidden: boolean }>({
    group: 'all',
    types: [],
    page: 1,
    pageSize: 20,
    includeHidden: false,
  })
  const previewHistory = ref<TaskItem[]>([])

  let historySeq = 0
  function historyMatches(t: TaskItem): boolean {
    if (!isKnownTaskType(t.type)) return false
    if (!GROUP_STATUSES[historyFilter.group].includes(t.status)) return false
    return !historyFilter.types.length || historyFilter.types.includes(t.type)
  }
  async function loadHistory() {
    historyLoaded.value = true
    if (previewMode) {
      const all = previewHistory.value.filter((t) => GROUP_STATUSES[historyFilter.group].includes(t.status) && (historyFilter.includeHidden || !t.hiddenInTaskCenter))
      historyTotal.value = all.length
      history.value = all.slice((historyFilter.page - 1) * historyFilter.pageSize, historyFilter.page * historyFilter.pageSize)
      return
    }
    if (!hasWailsBackend()) {
      // 浏览器里没有后端：显示接口层模拟任务（api/sim.ts）产生的历史
      const all = listSimFinished(historyFilter.includeHidden).map((t) => normalizeTask(t as unknown as goStore.Task)).filter((t) => historyMatches(t))
      historyTotal.value = all.length
      history.value = all.slice((historyFilter.page - 1) * historyFilter.pageSize, historyFilter.page * historyFilter.pageSize)
      return
    }
    const seq = ++historySeq
    historyLoading.value = true
    try {
      const q = {
        types: [...historyFilter.types],
        statuses: [...GROUP_STATUSES[historyFilter.group]],
        limit: historyFilter.pageSize,
        offset: (historyFilter.page - 1) * historyFilter.pageSize,
      }
      // v0.23 的 includeHidden：走转换记录数据层（真实绑定 / 模拟同一套）；开关关掉时仍用原来的 List（旧后端没有隐藏的概念）
      const page = convertV2IsReal()
        ? await listTasks({ ...q, includeHidden: historyFilter.includeHidden })
        : await call(TaskBinding.List(goStore.TaskFilter.createFrom(q)))
      if (seq !== historySeq) return // 更晚的请求已发出
      const real = (page.items ?? []).map((t) => normalizeTask(t as unknown as goStore.Task)).filter((t) => isKnownTaskType(t.type)) // 旧 / 未知类型忽略
      // 合并接口层模拟的历史（新的在前）：第一页放在最前面；总数加上模拟条数，翻页时后端 offset 不变，只是第一页多几行
      const sim = listSimFinished(historyFilter.includeHidden).map((t) => normalizeTask(t as unknown as goStore.Task)).filter((t) => historyMatches(t))
      history.value = historyFilter.page === 1 ? [...sim, ...real] : real
      historyTotal.value = (page.total ?? 0) + sim.length
      historyError.value = null
      // 删除后当前页可能已经没数据了，退回最后一页
      if (!history.value.length && historyTotal.value > 0 && historyFilter.page > 1) {
        historyFilter.page = Math.max(1, Math.ceil(historyTotal.value / historyFilter.pageSize))
        await loadHistory()
      }
    } catch (e) {
      if (seq === historySeq) historyError.value = normalizeError(toAppError(e))
      console.error('TaskService.List failed', e)
    } finally {
      if (seq === historySeq) historyLoading.value = false
    }
  }
  function setHistoryGroup(group: HistoryGroup) {
    historyFilter.group = group
    historyFilter.page = 1
    return loadHistory()
  }
  function setHistoryTypes(types: string[]) {
    historyFilter.types = types
    historyFilter.page = 1
    return loadHistory()
  }
  /** 任务中心“显示已隐藏”开关 */
  function setShowHidden(on: boolean) {
    historyFilter.includeHidden = on
    historyFilter.page = 1
    void loadStats() // 页签计数跟着开关
    return loadHistory()
  }
  function setHistoryPage(page: number) {
    historyFilter.page = page
    return loadHistory()
  }

  // ---- 统计条（原型 4 格）：运行中 / 排队中 来自活动列表；今日完成 / 失败 来自 List ----
  // 页签计数（历史 / 失败）跟着当前列表走：“显示已隐藏”打开时含已隐藏的（设计 §7.3：数字用带 includeHidden 的 List 的 total）。
  // “失败”统计卡片和“失败”页签用同一个数（走查 X9：卡片不含已隐藏、页签含，两处对不上），都跟着开关。
  // “今日完成”不受开关影响：总是含已隐藏的（今天完成后又被隐藏的仍算今天完成）。
  const todayDone = ref(0)
  const todayDoneCapped = ref(false) // List 单页最多 200，今日完成超过时显示 200+
  const failedTotal = ref(0) // “失败”页签计数和统计卡片（跟开关）
  const finishedTotal = ref(0) // 全部已结束任务数（历史页签角标，跟开关）
  let statsLoaded = false
  /** 接口层模拟任务（浏览器预览 / 模拟转换记录）的计数 */
  function simCounts(includeHidden: boolean, midnight: number) {
    const all = listSimFinished(includeHidden).map((t) => normalizeTask(t as unknown as goStore.Task)).filter((t) => isKnownTaskType(t.type))
    return { finished: all.length, failed: all.filter((t) => t.status === 'failed').length, today: all.filter((t) => t.status === 'succeeded' && t.finishedAt >= midnight).length }
  }
  /** v0.23 后端用带 includeHidden 的 List；旧后端没有隐藏的概念，用原来的 List */
  function listFor(statuses: TaskStatus[], limit: number, includeHidden: boolean) {
    return convertV2IsReal()
      ? listTasks({ types: [], statuses, limit, offset: 0, includeHidden })
      : call(TaskBinding.List(goStore.TaskFilter.createFrom({ types: [], statuses, limit, offset: 0 })))
  }
  async function loadStats() {
    statsLoaded = true
    const midnight = new Date().setHours(0, 0, 0, 0)
    const withHidden = historyFilter.includeHidden
    if (previewMode) {
      const shown = previewHistory.value.filter((t) => withHidden || !t.hiddenInTaskCenter)
      todayDone.value = previewHistory.value.filter((t) => t.status === 'succeeded' && t.finishedAt >= midnight).length
      failedTotal.value = shown.filter((t) => t.status === 'failed').length
      finishedTotal.value = shown.length
      return
    }
    const simTab = simCounts(withHidden, midnight)
    const simAll = simCounts(true, midnight)
    if (!hasWailsBackend()) {
      finishedTotal.value = simTab.finished
      failedTotal.value = simTab.failed
      todayDone.value = simAll.today
      todayDoneCapped.value = false
      return
    }
    try {
      const [done, failedTab, all] = await Promise.all([
        listFor(['succeeded'], 200, true),
        listFor(['failed'], 1, withHidden),
        listFor(TERMINAL, 1, withHidden),
      ])
      finishedTotal.value = (all.total ?? 0) + simTab.finished
      const items = done.items ?? []
      todayDone.value = items.filter((t) => t.finishedAt >= midnight).length + simAll.today
      todayDoneCapped.value = items.length >= 200 && items.every((t) => t.finishedAt >= midnight)
      failedTotal.value = (failedTab.total ?? 0) + simTab.failed
    } catch (e) {
      console.error('load task stats failed', e)
    }
  }

  let refreshTimer: ReturnType<typeof setTimeout> | undefined
  /** 有任务结束 / 被删除后，合并多次触发，刷新已经加载过的历史和统计 */
  function scheduleRefresh() {
    if (!historyLoaded.value && !statsLoaded) return
    clearTimeout(refreshTimer)
    refreshTimer = setTimeout(() => {
      if (historyLoaded.value) loadHistory()
      if (statsLoaded) loadStats()
    }, 300)
  }

  // ---- ffmpeg 安装进度 → ffmpeg store ----
  function syncInstall(t: TaskItem) {
    if (t.status !== 'running' && t.status !== 'queued') return
    const ff = useFFmpegStore()
    // 安装任务：类型是 ffmpeg_install，或就是 FFmpegStatus.taskId / InstallFFmpeg 返回的那个任务
    if (t.type !== 'ffmpeg_install' && ff.status.taskId !== t.id) return
    ff.updateInstall(toInstallProgress(t.progress, t.speed, t.etaSec))
  }

  // ---- 事件应用（带版本判断） ----
  const recovering = new Set<string>()
  /** 收到未知任务的非终态事件时，用 Get 补齐（正常订阅顺序下极少发生） */
  async function recover(id: string) {
    if (recovering.has(id) || removedIds.has(id) || !hasWailsBackend()) return
    recovering.add(id)
    try {
      const t = normalizeTask(await call(TaskBinding.Get(id)))
      // Get 期间可能收到了 task:removed，此时结果作废
      if (!removedIds.has(id) && !isTerminal(t.status) && !byId[id] && isKnownTaskType(t.type)) {
        byId[id] = t
        syncInstall(t)
      }
    } catch (e) {
      console.warn('recover task failed', id, e)
    } finally {
      recovering.delete(id)
    }
  }

  /**
   * 不在活动列表里的任务收到非终态事件时，要不要去补齐：从没结束过的要；已经结束过的，只有版本比结束时更大才要——
   * 转换任务原地重试（同一个 id 重新排队，产品决定 v1 / 契约 v0.23 待定）之后的事件就是这种，旧版本的迟到事件仍然丢弃。
   */
  function reopenedAfterFinish(id: string, version: number): boolean {
    if (byId[id]) return false
    const fv = finishedVersions.get(id)
    return fv === undefined || version > fv
  }

  /** 原地重试返回的任务（id 不变）：清掉终态快照和结束版本，放回活动列表；旧的 error / hwFallback 不带过来（normalizeTask 只取返回值里有的字段） */
  function reopen(raw: goStore.Task | TaskItem) {
    const t = normalizeTask(raw)
    delete t.hiddenInTaskCenter
    delete t.result
    finishedVersions.delete(t.id)
    delete finalById[t.id]
    const cur = byId[t.id]
    if (!cur || cur.version <= t.version) byId[t.id] = t
    history.value = history.value.filter((h) => h.id !== t.id)
  }

  function applyCreated(raw: goStore.Task) {
    const t = normalizeTask(raw)
    if (removedIds.has(t.id)) return
    if (!isKnownTaskType(t.type)) return // 旧 / 未知类型：忽略，不报错
    const cur = byId[t.id]
    if (cur && cur.version >= t.version) return
    if ((finishedVersions.get(t.id) ?? -1) >= t.version) return
    if (isTerminal(t.status)) return
    byId[t.id] = t
    syncInstall(t)
  }

  function applyProgress(p: ProgressPayload) {
    if (removedIds.has(p.id)) return
    const cur = byId[p.id]
    if (!cur) {
      if (reopenedAfterFinish(p.id, p.version)) recover(p.id)
      return
    }
    if (p.version <= cur.version) return
    cur.version = p.version
    cur.progress = p.progress
    cur.speed = p.speed ?? ''
    cur.etaSec = p.etaSec ?? 0
    cur.outTimeSec = p.outTimeSec ?? 0
    if (p.fps !== undefined) cur.fps = p.fps
    if (p.bitrateKbps !== undefined) cur.bitrateKbps = p.bitrateKbps
    if (p.droppedFrames !== undefined) cur.droppedFrames = p.droppedFrames
    mergeEncoder(cur, p)
    syncInstall(cur)
  }

  /**
   * 原地重试的那条 queued 事件（retried:true，契约 v0.23 §5 / §6.6）：清掉上一轮的进度、速度、剩余时间、开始 / 结束时间、
   * 编码器四个字段、错误和 result，hiddenInTaskCenter 清回 false；本地没有这条任务时按 Get 拉一次（模拟任务直接取模拟快照）。
   */
  function applyRetried(p: StatusPayload) {
    const cur = byId[p.id]
    if (cur) {
      if (p.version <= cur.version) return
      Object.assign(cur, { version: p.version, status: p.status, progress: p.progress ?? 0, speed: '', etaSec: 0, outTimeSec: 0, startedAt: 0, finishedAt: 0, error: null })
      for (const k of ['encoder', 'encoderDevice', 'hwFallback', 'hwFallbackReason', 'result', 'hiddenInTaskCenter'] as const) delete cur[k]
      if (p.outputPath) cur.outputPath = p.outputPath
      mergeEncoder(cur, p)
      return
    }
    if ((finishedVersions.get(p.id) ?? -1) >= p.version) return
    finishedVersions.delete(p.id)
    delete finalById[p.id]
    history.value = history.value.filter((h) => h.id !== p.id)
    const sim = isSimTask(p.id) ? getSimTask(p.id) : undefined
    if (sim) {
      const t = normalizeTask(sim as unknown as goStore.Task)
      if (!isTerminal(t.status) && isKnownTaskType(t.type)) byId[t.id] = t
    } else recover(p.id)
    scheduleRefresh()
  }

  /** UnhideInTaskCenter 的事件：只改 hiddenInTaskCenter 和 version（§5）；不当作终态事件（不改快照、不记结束版本以外的东西） */
  function applyUnhidden(p: StatusPayload) {
    const cur = byId[p.id]
    if (cur) {
      if (p.version > cur.version) {
        cur.version = p.version
        cur.hiddenInTaskCenter = p.hiddenInTaskCenter
      }
      return
    }
    if ((finishedVersions.get(p.id) ?? -1) < p.version && finishedVersions.has(p.id)) rememberFinished(p.id, p.version)
    const h = history.value.find((t) => t.id === p.id)
    if (h && h.version < p.version) {
      h.version = p.version
      h.hiddenInTaskCenter = p.hiddenInTaskCenter || undefined
    } else if (!h && historyLoaded.value) scheduleRefresh() // “显示已隐藏”关闭时：这一行可以回到列表
  }

  /** v0.24：重转结束（reconvertOutcome）的监听者；不管本地有没有这条任务都通知 */
  const reconvertListeners = new Set<(e: ReconvertEnd) => void>()
  function onReconvertEnd(cb: (e: ReconvertEnd) => void): () => void {
    reconvertListeners.add(cb)
    return () => reconvertListeners.delete(cb)
  }
  function applyStatus(p: StatusPayload) {
    if (removedIds.has(p.id)) return
    if (p.reconvertOutcome && (byId[p.id] ? p.version > byId[p.id].version : (finishedVersions.get(p.id) ?? -1) < p.version)) {
      const ev: ReconvertEnd = { id: p.id, outcome: p.reconvertOutcome, ...(toReconvertError(p.lastReconvertError) ? { error: toReconvertError(p.lastReconvertError) } : {}) }
      queueMicrotask(() => reconvertListeners.forEach((cb) => cb(ev)))
    }
    if (p.retried) return applyRetried(p)
    if (p.hiddenInTaskCenter !== undefined) return applyUnhidden(p)
    const cur = byId[p.id]
    if (!cur) {
      if (isTerminal(p.status)) {
        // 没在活动列表里的终态事件：历史需要刷新
        if ((finishedVersions.get(p.id) ?? -1) < p.version) {
          rememberFinished(p.id, p.version)
          recordFinal({
            id: p.id, status: p.status, error: normalizeError(p.error), outputPath: p.outputPath ?? '',
            progress: p.status === 'succeeded' ? 1 : typeof p.progress === 'number' ? p.progress : 0, speed: '', etaSec: 0, startedAt: p.startedAt || undefined, finishedAt: p.finishedAt ?? Date.now(), params: '', ...pickEncoderFields(p),
            ...(toTaskResult(p.result) ? { result: toTaskResult(p.result) } : {}),
          })
          scheduleRefresh()
        }
      } else if (reopenedAfterFinish(p.id, p.version)) {
        // v0.24 原地重转（6.17.3）：成功的记录重新排队，只发一条 queued 的 task:status（reconverting:true），不发 task:created
        if (p.reconverting) {
          finishedVersions.delete(p.id)
          delete finalById[p.id]
          const sim = isSimTask(p.id) ? getSimTask(p.id) : undefined
          if (sim) {
            const t = normalizeTask(sim as unknown as goStore.Task)
            if (!isTerminal(t.status) && isKnownTaskType(t.type)) byId[t.id] = t
            return
          }
        }
        recover(p.id)
      }
      return
    }
    if (p.version <= cur.version) return
    cur.version = p.version
    cur.status = p.status
    if (p.reconverting !== undefined) {
      if (p.reconverting) cur.reconverting = true
      else delete cur.reconverting
    }
    if (p.error) cur.error = normalizeError(p.error)
    if (p.outputPath) cur.outputPath = p.outputPath
    if (p.finishedAt) cur.finishedAt = p.finishedAt
    mergeEncoder(cur, p)
    // 事件带的 startedAt 优先；没有就沿用本地已记录值（running 时本地第一次见到才记当前时间）
    if (p.startedAt) cur.startedAt = p.startedAt
    else if (p.status === 'running' && !cur.startedAt) cur.startedAt = Date.now()
    if (isTerminal(p.status)) {
      // v0.23：终态事件带 progress（canceled 保留取消那一刻的值）；成功恒为 1；成功的转换带 result
      if (typeof p.progress === 'number' && p.progress >= 0) cur.progress = p.progress
      if (p.status === 'succeeded') cur.progress = 1
      const result = toTaskResult(p.result)
      if (result) cur.result = result
      recordFinal({
        id: cur.id, status: p.status, error: cur.error, outputPath: cur.outputPath, progress: cur.progress,
        speed: '', etaSec: 0, startedAt: cur.startedAt, finishedAt: cur.finishedAt || Date.now(), params: cur.params, ...pickEncoderFields(cur),
        ...(cur.result ? { result: cur.result } : {}),
      })
      delete byId[p.id]
      rememberFinished(p.id, p.version)
      scheduleRefresh()
    } else {
      syncInstall(cur)
    }
  }

  function applyRemoved(p: RemovedPayload) {
    const ids = new Set(p.ids ?? [])
    markRemoved(ids)
    for (const id of ids) {
      delete byId[id]
      delete finalById[id]
    }
    if (history.value.some((t) => ids.has(t.id))) scheduleRefresh()
    else if (statsLoaded) scheduleRefresh()
  }

  function apply(ev: BufferedEvent) {
    switch (ev.kind) {
      case 'created': return applyCreated(ev.payload)
      case 'progress': return applyProgress(ev.payload)
      case 'status': return applyStatus(ev.payload)
      case 'removed': return applyRemoved(ev.payload)
    }
  }

  // ---- 初始化：先订阅并缓冲事件 → ListActive → 按 version 回放（契约第 5 节 store 规则） ----
  let buffer: BufferedEvent[] | null = null
  let started = false
  function handle(ev: BufferedEvent) {
    // removed 没有 version，缓冲期间也立刻登记，保证回放时该 id 的 created/status/progress 被丢弃
    if (ev.kind === 'removed') markRemoved(ev.payload.ids ?? [])
    if (buffer) buffer.push(ev)
    else apply(ev)
  }

  /**
   * 拉取 queued + running 并回放缓冲。init 时缓冲已经在收集事件；之后（如手动刷新）自己开一个缓冲。
   * 缓冲期间到达的事件不直接应用，等 ListActive 返回后按 version 升序回放，版本不大于本地的丢弃。
   */
  async function refreshActive() {
    const own = buffer === null
    if (own) buffer = []
    try {
      const list = await call(TaskBinding.ListActive())
      loadError.value = null
      const seen = new Set<string>()
      for (const raw of list ?? []) {
        const t = normalizeTask(raw)
        if (removedIds.has(t.id) || !isKnownTaskType(t.type)) continue
        seen.add(t.id)
        const cur = byId[t.id]
        if (!cur || cur.version < t.version) byId[t.id] = t
        syncInstall(byId[t.id])
      }
      // 本地有、服务端已经没有的（期间结束了）：终态由缓冲里的 task:status 或历史刷新处理
      // 接口层模拟任务（开关为 false 时的直播 / 剪辑 / 文档）不在后端的 ListActive 里，不能清掉
      for (const id of Object.keys(byId)) if (!seen.has(id) && !isSimTask(id)) delete byId[id]
    } catch (e) {
      loadError.value = normalizeError(toAppError(e))
      console.error('TaskService.ListActive failed', e)
    } finally {
      const pending = buffer ?? []
      buffer = null
      // 有 version 的事件按 version 升序回放（稳定排序）；removed 没有 version，放最后（删除是终局）
      const ordered: BufferedEvent[] = [
        ...pending.filter((e) => e.kind !== 'removed').sort((a, b) => a.version - b.version),
        ...pending.filter((e) => e.kind === 'removed'),
      ]
      for (const ev of ordered) apply(ev)
      ready.value = true
    }
  }

  async function init() {
    if (started) return
    started = true
    // 接口层模拟（api/sim.ts）发的事件，形状与后端事件一致；真实后端就绪后总线上不会再有事件
    onSimEvent<goStore.Task>('task:created', (p) => handle({ kind: 'created', version: p.version, payload: p }))
    onSimEvent<ProgressPayload>('task:progress', (p) => handle({ kind: 'progress', version: p.version, payload: p }))
    onSimEvent<StatusPayload>('task:status', (p) => handle({ kind: 'status', version: p.version, payload: p }))
    onSimEvent<RemovedPayload>('task:removed', (p) => handle({ kind: 'removed', version: Infinity, payload: p }))
    if (!hasWailsBackend()) {
      // 浏览器预览：?tasks=N 生成 N 个假的进行中任务，同时生成假历史（?hist=M 调数量）
      if (previewMode) {
        const n = Math.max(0, Number(previewParams.get('tasks')) || 0)
        for (const t of buildPreviewActive(n)) byId[t.id] = t
        previewHistory.value = buildPreviewHistory(
          previewParams.has('hist') ? Math.max(0, Number(previewParams.get('hist')) || 0) : 8,
        )
        if (previewParams.get('q1') === '1') previewHistory.value.unshift(unmappedErrorPreview())
      }
      ready.value = true
      return
    }
    // 1. 先订阅并缓冲；2. ListActive；3. 回放（都在 refreshActive 里）
    buffer = []
    onEvent<goStore.Task>('task:created', (p) => handle({ kind: 'created', version: p.version, payload: p }))
    onEvent<ProgressPayload>('task:progress', (p) => handle({ kind: 'progress', version: p.version, payload: p }))
    onEvent<StatusPayload>('task:status', (p) => handle({ kind: 'status', version: p.version, payload: p }))
    onEvent<RemovedPayload>('task:removed', (p) => handle({ kind: 'removed', version: Infinity, payload: p }))
    await refreshActive()
  }

  // ---- 动作 ----
  async function cancel(id: string) {
    if (previewMode) {
      const t = byId[id]
      if (!t) return
      delete byId[id]
      previewHistory.value.unshift({ ...t, status: 'canceled', finishedAt: Date.now() })
      scheduleRefresh()
      return
    }
    if (isSimTask(id)) return cancelSimTask(id)
    await call(TaskBinding.Cancel(id))
  }

  /**
   * 正在「重试 / 换输出位置重新提交」的任务 id（在途集合）。按原任务 id 记：同一个失败任务的两种再提交互斥，
   * 快速连点、或两个组件（任务中心 / 转换页）同时点，都只会发出一次调用（后端 M4：重复 Retry 会产生重复任务）。
   */
  const busyIds = reactive(new Set<string>())
  function isBusy(id: string | undefined | null): boolean {
    return !!id && busyIds.has(id)
  }
  /** 在 id 的在途保护下运行 fn；已有在途调用时忽略本次（返回 undefined）。成功或失败都会释放。 */
  async function exclusive<T>(id: string, fn: () => Promise<T>): Promise<T | undefined> {
    if (!id || busyIds.has(id)) return undefined
    busyIds.add(id)
    try {
      return await fn()
    } finally {
      busyIds.delete(id)
    }
  }

  /**
   * 重试。契约 v0.23：failed / interrupted / canceled 一律原地重试（同一个 id；旧后端仍可能返回新 id，两种都兼容）：
   * 返回的 id 与原 id 相同 → reopen（清掉旧的终态快照、错误和回退提示）；不同 → 按新任务放进活动列表。
   * 同一任务已有重试在途时忽略（返回 undefined）。
   */
  function retry(id: string): Promise<TaskItem | undefined> {
    return exclusive(id, () => doRetry(id)).then((t) => t ?? undefined)
  }
  async function doRetry(id: string): Promise<TaskItem | undefined> {
    if (previewMode) {
      const old = previewHistory.value.find((t) => t.id === id)
      if (!old) return
      // v0.23：所有可重试类型都原地重试（同一个 id）
      const t: TaskItem = { ...old, status: 'queued', error: null, progress: 0, startedAt: 0, finishedAt: 0, version: old.version + 1 }
      for (const k of ['hwFallback', 'hwFallbackReason', 'result', 'hiddenInTaskCenter'] as const) delete t[k]
      previewHistory.value = previewHistory.value.filter((h) => h.id !== id)
      byId[t.id] = t
      await loadHistory()
      return t
    }
    const raw = isSimTask(id) ? (retrySimTask(id) as unknown as goStore.Task) : await call(TaskBinding.Retry(id))
    const t = normalizeTask(raw)
    if (t.id === id) reopen(t)
    // 新 id：task:created 事件会带来同一个对象；先放进去让界面立刻有反馈，版本判断保证不重复
    else applyCreated(t as unknown as goStore.Task)
    scheduleRefresh()
    return t
  }

  async function remove(ids: string[], deleteOutput = false) {
    if (!ids.length) return
    if (previewMode) {
      previewHistory.value = previewHistory.value.filter((t) => !ids.includes(t.id))
      await loadHistory()
      await loadStats()
      return
    }
    if (ids.every(isSimTask)) removeSimTasks(ids)
    else await call(TaskBinding.Remove(ids, deleteOutput))
    markRemoved(ids)
    // task:removed 事件也会到；本地先更新，避免界面等一个来回
    applyRemoved({ ids })
    history.value = history.value.filter((t) => !ids.includes(t.id))
  }

  /**
   * 任务中心“隐藏已结束”（契约 v0.23 HideFinishedInTaskCenter，替代 ClearFinished / “清除已结束”）：所有类型的已结束任务只从任务中心隐藏
   * （hiddenInTaskCenter），不删除记录和文件；转换页的转换记录照常显示。真正的删除只在转换页做。开关为 false 时走模拟（只影响模拟任务）。
   */
  async function hideFinished() {
    if (previewMode) {
      for (const t of previewHistory.value) if (isTerminal(t.status)) t.hiddenInTaskCenter = true
    } else await hideFinishedInTaskCenter()
    history.value = []
    historyFilter.page = 1
    await loadHistory()
    await loadStats()
  }

  /** “显示已隐藏”里的“取消隐藏”（UnhideInTaskCenter(ids)，§6.14.11）：事件会逐条到，这里再重新 List 一次 */
  async function unhide(ids: string[]) {
    if (!ids.length) return
    if (previewMode) {
      for (const t of previewHistory.value) if (ids.includes(t.id)) t.hiddenInTaskCenter = false
    } else await unhideInTaskCenter(ids)
    await loadHistory()
    await loadStats()
  }

  /**
   * 提交（ConvertService.Submit 等）返回的新任务：先放进活动列表让界面立刻有反馈；
   * 之后同一任务的 task:created 事件靠版本判断不会重复。
   */
  function track(list: goStore.Task[] | TaskItem[]) {
    for (const t of list ?? []) applyCreated(t as unknown as goStore.Task)
  }

  /** 按 id 主动取一次任务（事件可能在订阅前就发完了）：终态记快照，进行中放进活动列表 */
  async function fetchFinal(id: string) {
    if (removedIds.has(id) || byId[id] || finalById[id] || !hasWailsBackend()) return
    try {
      const t = normalizeTask(await call(TaskBinding.Get(id)))
      if (removedIds.has(id) || byId[id] || finalById[id]) return
      if (isTerminal(t.status)) {
        recordFinal({
          id: t.id, status: t.status, error: t.error, outputPath: t.outputPath, progress: t.status === 'succeeded' ? 1 : t.progress,
          speed: '', etaSec: 0, startedAt: t.startedAt, finishedAt: t.finishedAt, params: t.params, ...pickEncoderFields(t),
          ...(t.result ? { result: t.result } : {}),
        })
      } else byId[id] = t
    } catch (e) {
      console.warn('fetch task failed', id, e)
    }
  }

  async function getLog(id: string, tailLines = 200): Promise<string> {
    if (previewMode || !hasWailsBackend()) return PREVIEW_LOG
    return await call(TaskBinding.GetLog(id, tailLines))
  }

  return {
    // 状态
    ready, loadError, active, runningCount, hasRunning, runningOnly, queuedCount, liveActiveCount, queuePosition,
    history, historyTotal, historyLoading, historyLoaded, historyError, historyFilter,
    todayDone, todayDoneCapped, failedTotal, finishedTotal,
    // 方法
    init, refreshActive, loadHistory, setHistoryGroup, setHistoryTypes, setHistoryPage, setShowHidden, loadStats,
    cancel, retry, isBusy, exclusive, remove, hideFinished, unhide, getLog, track, fetchFinal, taskById, seedFinal, wasRemoved, onReconvertEnd,
  }
})
