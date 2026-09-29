import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import * as TaskBinding from '../../wailsjs/go/app/TaskService'
import { store as goStore } from '../../wailsjs/go/models'
import { call, toAppError } from '@/api/call'
import { hasWailsBackend, onEvent, onSimEvent, previewParams } from '@/services/wails'
import { cancelSimTask, isSimTask, listSimFinished, removeSimTasks, retrySimTask } from '@/api/sim'
import { toInstallProgress, useFFmpegStore } from '@/stores/ffmpeg'
import { buildPreviewActive, buildPreviewHistory, PREVIEW_LOG } from '@/stores/tasks.preview'

/** 契约第 3 节。注意 canceled 只有一个 l */
export type TaskStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
export type TaskType =
  | 'convert'
  | 'edit_export'
  | 'office_pdf'
  | 'live_file_push'
  | 'live_screen_push'
  | 'ffmpeg_install'

/**
 * 任务中心认识的类型（契约 v0.10~v0.12）。live_relay / live_record_push 是保留但不再产生的旧类型，edit_render 是 Edit 改名前的旧名，
 * 库里若还有这些记录、或将来出现新类型，一律忽略（不展示、不报错、不影响其他任务）。
 */
export const KNOWN_TASK_TYPES: readonly string[] = ['convert', 'edit_export', 'office_pdf', 'live_file_push', 'live_screen_push', 'ffmpeg_install']
export const isKnownTaskType = (t: unknown): boolean => typeof t === 'string' && KNOWN_TASK_TYPES.includes(t)

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
  finishedAt: number
  params: string
}

/** 历史分组 → 状态过滤。failed 组把 interrupted 也算进去（都需要用户手动重试） */
export type HistoryGroup = 'all' | 'succeeded' | 'failed' | 'canceled'
const TERMINAL: TaskStatus[] = ['succeeded', 'failed', 'canceled', 'interrupted']
const GROUP_STATUSES: Record<HistoryGroup, TaskStatus[]> = {
  all: TERMINAL,
  succeeded: ['succeeded'],
  failed: ['failed', 'interrupted'],
  canceled: ['canceled'],
}

export const isTerminal = (s: string): boolean => TERMINAL.includes(s as TaskStatus)
export const isLiveType = (t: string): boolean => t.startsWith('live_')

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
}
interface StatusPayload {
  id: string
  version: number
  status: TaskStatus
  error?: TaskError | null
  outputPath?: string
  finishedAt?: number
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
  }
}

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
   * 已收到 task:removed（或本地 remove()/clearFinished 已删除）的任务 id。
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
  const historyFilter = reactive<{ group: HistoryGroup; types: string[]; page: number; pageSize: number }>({
    group: 'all',
    types: [],
    page: 1,
    pageSize: 20,
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
      const all = previewHistory.value.filter((t) => GROUP_STATUSES[historyFilter.group].includes(t.status))
      historyTotal.value = all.length
      history.value = all.slice((historyFilter.page - 1) * historyFilter.pageSize, historyFilter.page * historyFilter.pageSize)
      return
    }
    if (!hasWailsBackend()) {
      // 浏览器里没有后端：显示接口层模拟任务（api/sim.ts）产生的历史
      const all = listSimFinished().map((t) => normalizeTask(t as unknown as goStore.Task)).filter((t) => historyMatches(t))
      historyTotal.value = all.length
      history.value = all.slice((historyFilter.page - 1) * historyFilter.pageSize, historyFilter.page * historyFilter.pageSize)
      return
    }
    const seq = ++historySeq
    historyLoading.value = true
    try {
      const filter = goStore.TaskFilter.createFrom({
        types: [...historyFilter.types],
        statuses: [...GROUP_STATUSES[historyFilter.group]],
        limit: historyFilter.pageSize,
        offset: (historyFilter.page - 1) * historyFilter.pageSize,
      })
      const page = await call(TaskBinding.List(filter))
      if (seq !== historySeq) return // 更晚的请求已发出
      const real = (page.items ?? []).map(normalizeTask).filter((t) => isKnownTaskType(t.type)) // 旧 / 未知类型忽略
      // 合并接口层模拟的历史（新的在前）：第一页放在最前面；总数加上模拟条数，翻页时后端 offset 不变，只是第一页多几行
      const sim = listSimFinished().map((t) => normalizeTask(t as unknown as goStore.Task)).filter((t) => historyMatches(t))
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
  function setHistoryPage(page: number) {
    historyFilter.page = page
    return loadHistory()
  }

  // ---- 统计条（原型 4 格）：运行中 / 排队中 来自活动列表；今日完成 / 失败 来自 List ----
  const todayDone = ref(0)
  const todayDoneCapped = ref(false) // List 单页最多 200，今日完成超过时显示 200+
  const failedTotal = ref(0)
  const finishedTotal = ref(0) // 全部已结束任务数（历史页签角标）
  let statsLoaded = false
  async function loadStats() {
    statsLoaded = true
    if (previewMode) {
      const midnight = new Date().setHours(0, 0, 0, 0)
      todayDone.value = previewHistory.value.filter((t) => t.status === 'succeeded' && t.finishedAt >= midnight).length
      failedTotal.value = previewHistory.value.filter((t) => t.status === 'failed' || t.status === 'interrupted').length
      finishedTotal.value = previewHistory.value.length
      return
    }
    if (!hasWailsBackend()) return
    try {
      const midnight = new Date().setHours(0, 0, 0, 0)
      const [done, failed, all] = await Promise.all([
        call(TaskBinding.List(goStore.TaskFilter.createFrom({ types: [], statuses: ['succeeded'], limit: 200, offset: 0 }))),
        call(TaskBinding.List(goStore.TaskFilter.createFrom({ types: [], statuses: ['failed', 'interrupted'], limit: 1, offset: 0 }))),
        call(TaskBinding.List(goStore.TaskFilter.createFrom({ types: [], statuses: TERMINAL, limit: 1, offset: 0 }))),
      ])
      finishedTotal.value = all.total ?? 0
      const items = done.items ?? []
      todayDone.value = items.filter((t) => t.finishedAt >= midnight).length
      todayDoneCapped.value = items.length >= 200 && items.every((t) => t.finishedAt >= midnight)
      failedTotal.value = failed.total ?? 0
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
      if (!finishedVersions.has(p.id)) recover(p.id)
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
    syncInstall(cur)
  }

  function applyStatus(p: StatusPayload) {
    if (removedIds.has(p.id)) return
    const cur = byId[p.id]
    if (!cur) {
      if (isTerminal(p.status)) {
        // 没在活动列表里的终态事件：历史需要刷新
        if ((finishedVersions.get(p.id) ?? -1) < p.version) {
          rememberFinished(p.id, p.version)
          recordFinal({
            id: p.id, status: p.status, error: normalizeError(p.error), outputPath: p.outputPath ?? '',
            progress: p.status === 'succeeded' ? 1 : 0, speed: '', etaSec: 0, finishedAt: p.finishedAt ?? Date.now(), params: '',
          })
          scheduleRefresh()
        }
      } else if (!finishedVersions.has(p.id)) {
        recover(p.id)
      }
      return
    }
    if (p.version <= cur.version) return
    cur.version = p.version
    cur.status = p.status
    if (p.error) cur.error = normalizeError(p.error)
    if (p.outputPath) cur.outputPath = p.outputPath
    if (p.finishedAt) cur.finishedAt = p.finishedAt
    if (p.status === 'running' && !cur.startedAt) cur.startedAt = Date.now()
    if (isTerminal(p.status)) {
      if (p.status === 'succeeded') cur.progress = 1
      recordFinal({
        id: cur.id, status: p.status, error: cur.error, outputPath: cur.outputPath, progress: cur.progress,
        speed: '', etaSec: 0, finishedAt: cur.finishedAt || Date.now(), params: cur.params,
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

  /** 重试：用原参数生成新任务，原任务保留在历史里。同一任务已有重试在途时忽略（返回 undefined） */
  function retry(id: string): Promise<TaskItem | undefined> {
    return exclusive(id, () => doRetry(id)).then((t) => t ?? undefined)
  }
  async function doRetry(id: string): Promise<TaskItem | undefined> {
    if (previewMode) {
      const old = previewHistory.value.find((t) => t.id === id)
      if (!old) return
      const t: TaskItem = { ...old, id: 'new_' + id, status: 'queued', error: null, progress: 0, startedAt: 0, finishedAt: 0, createdAt: Date.now() }
      byId[t.id] = t
      return t
    }
    if (isSimTask(id)) {
      const nt = normalizeTask(retrySimTask(id) as unknown as goStore.Task)
      scheduleRefresh()
      return nt
    }
    const t = normalizeTask(await call(TaskBinding.Retry(id)))
    // task:created 事件会带来同一个对象；先放进去让界面立刻有反馈，版本判断保证不重复
    applyCreated(t as unknown as goStore.Task)
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

  async function clearFinished() {
    if (previewMode) {
      previewHistory.value = previewHistory.value.filter((t) => !isTerminal(t.status))
      await loadHistory()
      await loadStats()
      return
    }
    await call(TaskBinding.ClearFinished())
    // 已加载的历史都是终态任务，全部被清除；没加载的靠后端发来的 task:removed 登记
    markRemoved(history.value.map((t) => t.id))
    history.value = []
    historyTotal.value = 0
    historyFilter.page = 1
    scheduleRefresh()
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
          speed: '', etaSec: 0, finishedAt: t.finishedAt, params: t.params,
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
    init, refreshActive, loadHistory, setHistoryGroup, setHistoryTypes, setHistoryPage, loadStats,
    cancel, retry, isBusy, exclusive, remove, clearFinished, getLog, track, fetchFinal, taskById, seedFinal, wasRemoved,
  }
})
