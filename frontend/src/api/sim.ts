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
 *   ?sim_kill=1              直播：停止时模拟优雅停止超时被强杀（canceled；带存档的屏幕推流强杀后 outputPath 保留）
 *   ?sim_end=<秒>            直播：推满 N 秒后自然结束（succeeded）
 *   ?sim_missing=rtmp|rtmps|srt   直播：地址 scheme 与之相同时 UNSUPPORTED，detail 是契约 §6.10 的单独一行 `missing=<协议名>`（没有第二行）
 *   ?sim_err=LIVE_URL_INVALID&sim_reason=<值>  直播：detail 首行 reason=<值>（scheme_unsupported|malformed|missing_host|param_not_allowed；unknown=未知值；不带 sim_reason=没有 reason 行）。不注入时，地址本身有问题会按实际原因给 reason
 *   ?sim_scheme=missing      直播：LIVE_CONNECT_FAILED 的 detail 不带 scheme= 首行（测兜底）
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

/** 由注入构造 detail：TASK_CONFLICT / LIVE_URL_INVALID 的首行是 reason=<值>（契约 6.10），其余直接用 sim_detail */
export function injectionDetail(inj: SimInjection): string | undefined {
  const lines: string[] = []
  if ((inj.code === 'TASK_CONFLICT' || inj.code === 'LIVE_URL_INVALID') && inj.reason) lines.push(`reason=${inj.reason === 'unknown' ? 'future_reason' : inj.reason}`)
  if (inj.code === 'LIVE_SOURCE_GONE') lines.push(`kind=${inj.reason === 'screen' ? 'screen' : 'window'}`) // sim_reason=window|screen
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
  /** 优雅停止失败（超时没退出被强杀；有存档最多等 16 秒）→ canceled */
  forceKill?: boolean
  /** 屏幕推流带本地存档：task:progress 不带 bitrateKbps；outputPath 为存档路径，强杀后保留；没等到第一条 progress 就结束（空壳）→ outputPath 清空 */
  archive?: boolean
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
  /** 模拟的实际编码器（契约 9.7）；缺省：convert / edit_export / 直播 = libx264 + cpu，其余不带 */
  encoder?: { encoder: string; encoderDevice: string; hwFallback?: boolean; hwFallbackReason?: string }
  /** 非直播：进度到这个值时模拟“硬件编码启动失败 → 自动改用 CPU”（补发一条 running 的 task:status，进度从头开始）；用 encoder 描述回退后的样子 */
  fallbackAt?: number
  fallbackTo?: { encoder: string; encoderDevice: string; hwFallback?: boolean; hwFallbackReason?: string }
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
  const p: TaskStatusPayload = { id: t.id, version: t.version, status: t.status, ...encoderFields(t), ...extra }
  emitSimEvent('task:status', p)
}

/** 事件里带的编码器字段（有编码器信息的任务才有；与后端 omitempty 一致） */
function encoderFields(t: ApiTask): Partial<TaskStatusPayload> {
  if (!t.encoder) return {}
  return { encoder: t.encoder, ...(t.encoderDevice ? { encoderDevice: t.encoderDevice } : {}), ...(t.hwFallback ? { hwFallback: true } : {}), ...(t.hwFallbackReason ? { hwFallbackReason: t.hwFallbackReason } : {}) }
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
  // 存档空壳：还没推出任何内容就结束 → 后端删掉文件并清空 outputPath（先于终态事件）
  if (e.spec.live?.archive && !e.firstProgressAt) t.outputPath = ''
  t.fps = t.bitrateKbps = t.droppedFrames = undefined
  bump(e)
  // 契约：优雅停止的 succeeded 和强杀的 canceled 都不带 error
  emitStatus(e, { outputPath: t.outputPath || undefined, ...(t.startedAt ? { startedAt: t.startedAt } : {}), finishedAt: t.finishedAt, ...(error ? { error } : {}) })
}

function progress(e: Entry, over: Partial<TaskProgressPayload>) {
  const t = e.task
  bump(e)
  const p: TaskProgressPayload = { id: t.id, version: t.version, progress: t.progress, speed: t.speed, etaSec: t.etaSec, outTimeSec: 0, ...encoderFields(t), ...over }
  t.progress = p.progress
  t.speed = p.speed
  t.etaSec = p.etaSec
  if (p.fps !== undefined) t.fps = p.fps
  if (p.bitrateKbps !== undefined) t.bitrateKbps = p.bitrateKbps
  if (p.droppedFrames !== undefined) t.droppedFrames = p.droppedFrames
  emitSimEvent('task:progress', p)
}

/**
 * 开发用：?enc=<场景> 里和“任务用了哪个编码设备”有关的场景（仅纯浏览器；与设置页的设备列表场景共用同一个参数）：
 *   gpu-task 用显卡不回退；fb-nvenc / fb-unavail 运行中回退到 CPU；fb-unknown 回退且原因是未知枚举；gpu-gone 用的显卡已不在设备列表里；
 *   copy-task -c copy（encoder=copy，没有设备）；cpu-task 用 CPU 不回退（超 4096 / 两遍编码 / 偏好 CPU 都是这个样子）。
 * 其它场景（found / multi / …）和没有 ?enc= 时返回 undefined = 沿用原来的默认（libx264 + cpu）。
 */
export function simEncoderScenario(): { encoder: string; encoderDevice: string; hwFallback?: boolean; hwFallbackReason?: string; fallbackAt?: number; gpuStart?: { encoder: string; encoderDevice: string } } | undefined {
  const GPU = { encoder: 'h264_nvenc', encoderDevice: 'nvidia-0' }
  const CPU = { encoder: 'libx264', encoderDevice: 'cpu' }
  switch (simParam('enc')) {
    case 'gpu-task': return GPU
    case 'fb-nvenc': return { ...CPU, hwFallback: true, hwFallbackReason: 'nvenc_init_failed', fallbackAt: 0.25, gpuStart: GPU }
    case 'fb-unavail': return { ...CPU, hwFallback: true, hwFallbackReason: 'device_unavailable' }
    case 'fb-unknown': return { ...CPU, hwFallback: true, hwFallbackReason: 'brand_new_reason_x', fallbackAt: 0.25, gpuStart: GPU }
    case 'gpu-gone': return { encoder: 'h264_nvenc', encoderDevice: 'nvidia-9' }
    case 'copy-task': return { encoder: 'copy', encoderDevice: '' }
    case 'cpu-task': return CPU
    default: return undefined
  }
}

/** 创建并启动一个模拟任务：立即发 task:created（queued, version 1），300ms 后 running，然后定时推进 */
/** 模拟任务的标题前缀：所有出现标题的地方（任务中心、日志面板、通知）都能看出是演示数据；任务中心会把它换成“演示”标签 */
export const SIM_TITLE_PREFIX = '【演示】'

export function createSimTask(spec: SimTaskSpec): ApiTask {
  const id = `sim${Date.now().toString(36)}${(++seq).toString(36)}`
  const live = !!spec.live
  const task: ApiTask = {
    id, type: spec.type, status: 'queued', title: SIM_TITLE_PREFIX + spec.title, inputPaths: [...spec.inputPaths], outputPath: spec.outputPath,
    progress: live ? -1 : 0, speed: '', etaSec: 0, params: spec.params, version: 1, error: null, createdAt: Date.now(), startedAt: 0, finishedAt: 0,
  }
  const sc = spec.type === 'convert' || spec.type === 'edit_export' ? simEncoderScenario() : undefined
  if (sc && !spec.encoder) {
    // 场景里“运行中才回退”的：一开始记显卡，到 fallbackAt 再改成 CPU 并补发 running；其余从头就是最终样子
    spec = { ...spec, encoder: sc.gpuStart ? { ...sc.gpuStart } : { encoder: sc.encoder, encoderDevice: sc.encoderDevice, ...(sc.hwFallback ? { hwFallback: true, hwFallbackReason: sc.hwFallbackReason } : {}) }, ...(sc.fallbackAt ? { fallbackAt: sc.fallbackAt, fallbackTo: sc } : {}) } as SimTaskSpec
  }
  // ?enc=fb-nvenc / fb-unavail / fb-unknown：直播任务也带“硬件回退”字段（一开始就是 CPU + hwFallback；startedAt 仍是 0，running 之后才显示提示条）
  if (live && !spec.encoder) {
    const lsc = simEncoderScenario()
    if (lsc?.hwFallback) spec = { ...spec, encoder: { encoder: lsc.encoder, encoderDevice: lsc.encoderDevice, hwFallback: true, hwFallbackReason: lsc.hwFallbackReason } } as SimTaskSpec
  }
  const enc = spec.encoder ?? (spec.type === 'convert' || spec.type === 'edit_export' || live ? { encoder: 'libx264', encoderDevice: 'cpu' } : undefined)
  if (enc) Object.assign(task, { encoder: enc.encoder, encoderDevice: enc.encoderDevice, ...(enc.hwFallback ? { hwFallback: true } : {}), ...(enc.hwFallbackReason ? { hwFallbackReason: enc.hwFallbackReason } : {}) })
  const e: Entry = { task, spec, stopping: false, firstProgressAt: 0 }
  entries.set(id, e)
  emitSimEvent('task:created', snapshot(task))
  e.startTimer = setTimeout(() => {
    task.status = 'running'
    task.startedAt = Date.now()
    bump(e)
    emitStatus(e, { startedAt: task.startedAt })
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
    if (spec.fallbackAt && spec.fallbackTo && !task.hwFallback && p >= spec.fallbackAt) {
      // 契约 9.7：硬件启动失败 → 日志一行 → 编码器改成 CPU + hwFallback → 补发一条 running 的 task:status → 同一命令用 CPU 重跑（进度从头开始，进度条只增不减）
      const to = spec.fallbackTo
      Object.assign(task, { encoder: to.encoder, encoderDevice: to.encoderDevice, hwFallback: true, ...(to.hwFallbackReason ? { hwFallbackReason: to.hwFallbackReason } : {}) })
      bump(e)
      emitStatus(e)
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
    progress(e, { progress: -1, speed: '1.00x', etaSec: 0, outTimeSec: sec, fps: 30, ...(live.archive ? {} : { bitrateKbps: DEMO_BITRATE[n % DEMO_BITRATE.length] }), droppedFrames: 0 })
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
  // 演示用的等待时长（真实：无存档 5 秒、有存档 16 秒后强杀）；有存档的优雅停止多等一会儿，好看到“正在停止…”
  e.stopTimer = setTimeout(() => finish(e, e.spec.live!.forceKill ? 'canceled' : 'succeeded'), e.spec.live.forceKill ? 1500 : e.spec.live.archive ? 1200 : 600)
}

/** 模拟“强制停止”：正在停止中的直播会话立即强杀 → canceled（存档按 finish 里的规则保留 / 清空） */
export function forceKillSimTask(id: string): void {
  const e = entries.get(id)
  if (!e) simError('NOT_FOUND', '任务不存在')
  if (!isActiveStatus(e.task.status)) simError('TASK_CONFLICT', '任务已经结束，不能取消')
  if (!e.spec.live) return cancelSimTask(id)
  clearInterval(e.timer)
  clearTimeout(e.stopTimer)
  e.stopping = true
  finish(e, 'canceled')
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
