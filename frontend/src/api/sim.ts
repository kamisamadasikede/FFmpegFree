/**
 * 接口层模拟引擎（三个服务共用）：在内存里维护模拟任务，用定时器推进进度，
 * 并通过 services/wails.ts 的模拟事件总线发出与真实后端形状一致的 task:created / task:progress / task:status / task:removed，
 * 任务 store 订阅同一条总线，所以任务中心、侧边栏角标等都能看到模拟任务。
 *
 * 只在对应服务的 *_BACKEND_READY = false 时使用。完整推流地址 / 口令不会进入这里：
 * 调用方只传脱敏后的 title / params，标准化地址仅放在 meta（内存，不发事件、不打印）。
 */
import { AppError, type AppErrorCode } from '@/api/call'
import type { ApiTask, ApiTaskError, TaskProgressPayload, TaskStatusPayload } from '@/api/taskTypes'
import { emitSimEvent } from '@/services/wails'

/** 读预览参数（?live_err=… 等）；没有返回 null */
export const simParam = (name: string): string | null => new URLSearchParams(globalThis.window?.location?.search ?? '').get(name)

/**
 * 预览参数触发错误（只在模拟层生效）：
 *   ?sim_err=<错误码>        触发该错误码。同步类的码（参数 / 冲突 / 缺 ffmpeg 等）由 Start* / Export / ConvertToPDF 直接抛出；
 *                            任务类的码（LIVE_CONNECT_FAILED、PROCESS_FAILED、CONVERT_DISK_FULL 等）让任务在模拟过程中失败
 *   ?sim_when=task|call      强制某个码走任务失败 / 同步抛出（默认按下面的 TASK_ONLY 表判断）
 *   ?sim_reason=<值>         TASK_CONFLICT 的 detail 首行 reason=<值>（缺省不带 reason；填 unknown 可复现未知 reason）
 *   ?sim_detail=<文本>       附加到 detail 第二行
 *   ?sim_kill=1              直播：停止时模拟 5 秒内没退出被强杀（canceled）
 *   ?sim_end=<秒>            直播：推满 N 秒后自然结束（succeeded）
 *   ?sim_missing=srt|rtmps   直播：UNSUPPORTED，detail 写缺哪个协议
 */
export interface SimInjection {
  code: AppErrorCode
  when: 'call' | 'task'
  reason?: string
  detail?: string
}

/** 这些码只会作为任务失败出现（在任务的 error 上），不会由 Start* 同步返回 */
const TASK_ONLY: readonly string[] = ['LIVE_CONNECT_FAILED', 'LIVE_PUSH_REJECTED', 'LIVE_PUSH_INTERRUPTED', 'CONVERT_DISK_FULL', 'PROCESS_FAILED', 'PROBE_FAILED_TASK']

export function simInjection(): SimInjection | null {
  const code = simParam('sim_err') as AppErrorCode | null
  if (!code) return null
  const when = (simParam('sim_when') as 'call' | 'task' | null) ?? (TASK_ONLY.includes(code) ? 'task' : 'call')
  return { code, when, reason: simParam('sim_reason') ?? undefined, detail: simParam('sim_detail') ?? undefined }
}

/** 由注入构造 detail：TASK_CONFLICT 的首行是 reason=<值>（契约 6.10），其余直接用 sim_detail */
export function injectionDetail(inj: SimInjection): string | undefined {
  const lines: string[] = []
  if (inj.code === 'TASK_CONFLICT' && inj.reason) lines.push(`reason=${inj.reason === 'unknown' ? 'future_reason' : inj.reason}`)
  if (inj.detail) lines.push(inj.detail)
  return lines.length ? lines.join('\n') : undefined
}

export const simDelay = (ms: number) => new Promise<void>((r) => setTimeout(r, ms))

/** 抛出一个契约错误码（模拟层用） */
export function simError(code: AppErrorCode, message: string, detail?: string): never {
  throw new AppError(code, message, detail)
}

export interface SimLiveSpec {
  /** 第一条 task:progress 之前的“连接中”时长 */
  connectMs?: number
  /** 任务失败：连接阶段失败（LIVE_CONNECT_FAILED / LIVE_PUSH_REJECTED）在第一条 progress 之前，其余在 afterSec 秒后 */
  fail?: { code: AppErrorCode; message: string; detail?: string; afterSec?: number }
  /** 非循环文件推流播完自然结束（succeeded） */
  endAfterSec?: number
  /** 优雅停止失败（5 秒内没退出被强杀）→ canceled */
  forceKill?: boolean
}

export interface SimTaskSpec {
  type: ApiTask['type']
  title: string
  inputPaths: string[]
  outputPath: string
  /** 直播任务必须是已脱敏的 params */
  params: string
  /** 非直播：模拟总时长（秒），默认 6 */
  simSeconds?: number
  /** 非直播：媒体时长（秒），给 outTimeSec 用 */
  mediaSec?: number
  /** 非直播任务失败：进度到 atProgress 时失败 */
  fail?: { code: AppErrorCode; message: string; detail?: string; atProgress?: number }
  live?: SimLiveSpec
  /** 只在内存里用的附加信息（如标准化推流地址），不会进入任何事件 */
  meta?: Record<string, unknown>
}

interface Entry {
  task: ApiTask
  spec: SimTaskSpec
  timer?: ReturnType<typeof setInterval>
  startTimer?: ReturnType<typeof setTimeout>
  stopTimer?: ReturnType<typeof setTimeout>
  stopping: boolean
  firstProgressAt: number
}

const entries = new Map<string, Entry>()
let seq = 0
const DEMO_BITRATE = [6020, 5990, 6005, 5960, 6000, 5955, 5990, 5940, 5970, 5965, 5985, 5950]

const snapshot = (t: ApiTask): ApiTask => ({ ...t, inputPaths: [...t.inputPaths], error: t.error ? { ...t.error } : null })
const isActiveStatus = (s: string) => s === 'queued' || s === 'running'

export const isSimTask = (id: string): boolean => entries.has(id)
export const getSimMeta = (id: string): Record<string, unknown> | undefined => entries.get(id)?.spec.meta
export const getSimTask = (id: string): ApiTask | undefined => {
  const e = entries.get(id)
  return e ? snapshot(e.task) : undefined
}
export const listSimActive = (): ApiTask[] => [...entries.values()].filter((e) => isActiveStatus(e.task.status)).map((e) => snapshot(e.task))
/** 已结束的模拟任务，新的在前 */
export const listSimFinished = (): ApiTask[] =>
  [...entries.values()]
    .filter((e) => !isActiveStatus(e.task.status))
    .map((e) => snapshot(e.task))
    .sort((a, b) => b.createdAt - a.createdAt)
/** 进行中的模拟任务的 meta（重复地址 / 会话上限检查用） */
export const activeSimEntries = (type?: ApiTask['type']): { task: ApiTask; meta?: Record<string, unknown> }[] =>
  [...entries.values()].filter((e) => isActiveStatus(e.task.status) && (!type || e.task.type === type)).map((e) => ({ task: snapshot(e.task), meta: e.spec.meta }))

function emitStatus(e: Entry, extra: Partial<TaskStatusPayload> = {}) {
  const t = e.task
  const p: TaskStatusPayload = { id: t.id, version: t.version, status: t.status, ...extra }
  emitSimEvent('task:status', p)
}

function bump(e: Entry): number {
  return ++e.task.version
}

function finish(e: Entry, status: 'succeeded' | 'failed' | 'canceled', error?: ApiTaskError) {
  clearInterval(e.timer)
  clearTimeout(e.startTimer)
  clearTimeout(e.stopTimer)
  const t = e.task
  t.status = status
  t.finishedAt = Date.now()
  if (status === 'succeeded' && !e.spec.live) t.progress = 1
  t.error = error ?? null
  t.fps = t.bitrateKbps = t.droppedFrames = undefined
  bump(e)
  // 契约：优雅停止的 succeeded 和强杀的 canceled 都不带 error
  emitStatus(e, { outputPath: t.outputPath || undefined, finishedAt: t.finishedAt, ...(error ? { error } : {}) })
}

function progress(e: Entry, over: Partial<TaskProgressPayload>) {
  const t = e.task
  bump(e)
  const p: TaskProgressPayload = { id: t.id, version: t.version, progress: t.progress, speed: t.speed, etaSec: t.etaSec, outTimeSec: 0, ...over }
  t.progress = p.progress
  t.speed = p.speed
  t.etaSec = p.etaSec
  if (p.fps !== undefined) t.fps = p.fps
  if (p.bitrateKbps !== undefined) t.bitrateKbps = p.bitrateKbps
  if (p.droppedFrames !== undefined) t.droppedFrames = p.droppedFrames
  emitSimEvent('task:progress', p)
}

/** 创建并启动一个模拟任务：立即发 task:created（queued, version 1），300ms 后 running，然后定时推进 */
export function createSimTask(spec: SimTaskSpec): ApiTask {
  const id = `sim${Date.now().toString(36)}${(++seq).toString(36)}`
  const live = !!spec.live
  const task: ApiTask = {
    id, type: spec.type, status: 'queued', title: spec.title, inputPaths: [...spec.inputPaths], outputPath: spec.outputPath,
    progress: live ? -1 : 0, speed: '', etaSec: 0, params: spec.params, version: 1, error: null, createdAt: Date.now(), startedAt: 0, finishedAt: 0,
  }
  const e: Entry = { task, spec, stopping: false, firstProgressAt: 0 }
  entries.set(id, e)
  emitSimEvent('task:created', snapshot(task))
  e.startTimer = setTimeout(() => {
    task.status = 'running'
    task.startedAt = Date.now()
    bump(e)
    emitStatus(e)
    if (live) runLive(e)
    else runBatch(e)
  }, 300)
  return snapshot(task)
}

function runBatch(e: Entry) {
  const { spec, task } = e
  const total = (spec.simSeconds ?? 6) * 1000
  const media = spec.mediaSec ?? 60
  const t0 = Date.now()
  e.timer = setInterval(() => {
    const p = Math.min(1, (Date.now() - t0) / total)
    if (spec.fail && p >= (spec.fail.atProgress ?? 0.6)) {
      finish(e, 'failed', { code: spec.fail.code, message: spec.fail.message, detail: spec.fail.detail })
      return
    }
    if (p >= 1) {
      progress(e, { progress: 1, speed: '2.4x', etaSec: 0, outTimeSec: media })
      finish(e, 'succeeded')
      return
    }
    progress(e, { progress: p, speed: '2.4x', etaSec: Math.round((total * (1 - p)) / 1000), outTimeSec: p * media })
  }, 250)
  void task
}

function runLive(e: Entry) {
  const live = e.spec.live!
  const connectMs = live.connectMs ?? 1200
  const failNow = live.fail && (live.fail.code === 'LIVE_CONNECT_FAILED' || live.fail.code === 'LIVE_PUSH_REJECTED')
  const t0 = Date.now()
  let n = 0
  e.timer = setInterval(() => {
    const now = Date.now()
    if (!e.firstProgressAt) {
      if (now - t0 < connectMs) return
      if (failNow) {
        finish(e, 'failed', { code: live.fail!.code, message: live.fail!.message, detail: live.fail!.detail })
        return
      }
      e.firstProgressAt = now
    }
    const sec = (now - e.firstProgressAt) / 1000
    if (live.fail && !failNow && sec >= (live.fail.afterSec ?? 4)) {
      finish(e, 'failed', { code: live.fail.code, message: live.fail.message, detail: live.fail.detail })
      return
    }
    if (live.endAfterSec && sec >= live.endAfterSec && !e.stopping) {
      finish(e, 'succeeded')
      return
    }
    // task:progress：progress 恒为 -1，etaSec 恒为 0，带 fps / bitrateKbps / droppedFrames
    progress(e, { progress: -1, speed: '1.00x', etaSec: 0, outTimeSec: sec, fps: 30, bitrateKbps: DEMO_BITRATE[n % DEMO_BITRATE.length], droppedFrames: 0 })
    n++
  }, 1000)
}

/** TaskService.Cancel 的模拟。直播：优雅停止 → succeeded（无 error）；forceKill → 1.5 秒后 canceled（无 error）。 */
export function cancelSimTask(id: string): void {
  const e = entries.get(id)
  if (!e) simError('NOT_FOUND', '任务不存在')
  if (!isActiveStatus(e.task.status)) simError('TASK_CONFLICT', '任务已经结束，不能取消')
  if (e.stopping) return // 正在停止中，重复点击返回 nil
  if (!e.spec.live) {
    finish(e, 'canceled')
    return
  }
  e.stopping = true
  clearInterval(e.timer)
  e.stopTimer = setTimeout(() => finish(e, e.spec.live!.forceKill ? 'canceled' : 'succeeded'), e.spec.live.forceKill ? 1500 : 600)
}

/** TaskService.Retry 的模拟：直播会话 UNSUPPORTED；进行中 TASK_CONFLICT；其余按原参数重新创建 */
export function retrySimTask(id: string): ApiTask {
  const e = entries.get(id)
  if (!e) simError('NOT_FOUND', '任务不存在')
  if (isActiveStatus(e.task.status)) simError('TASK_CONFLICT', '任务还在进行中，不能重试')
  if (e.spec.live) simError('UNSUPPORTED', '直播会话不能重试，请重新开始推流')
  return createSimTask(e.spec)
}

export function removeSimTasks(ids: string[]): void {
  for (const id of ids) if (entries.has(id) && isActiveStatus(entries.get(id)!.task.status)) simError('TASK_CONFLICT', '任务还在进行中，不能删除')
  const gone = ids.filter((id) => entries.delete(id))
  if (gone.length) emitSimEvent('task:removed', { ids: gone })
}
