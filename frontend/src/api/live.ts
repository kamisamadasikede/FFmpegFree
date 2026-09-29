/**
 * 直播页的全部后端调用都在这个文件里（过渡期）。
 *
 * 现状：调用 v1 的 gin 接口（axios → http://localhost:19200，加 SSE 和 /ws WebSocket）。
 * 迁移：改成 Wails LiveService 时只改本文件，页面和 store 不用动。对照表（契约 v0.4，第 4 节 LiveService / 第 5 节事件）：
 *
 *   本文件函数                    v1 接口                               v2 LiveService
 *   listMaterials/uploadMaterial  GET /api/getSteamFiles、POST /api/uploadSteamup   SystemService.PickFiles（直接选本地绝对路径，不再上传拷贝）
 *   deleteMaterial                POST /api/deletesteamVideo             删除（v2 不拷贝素材，无此操作）
 *   startFilePush                 POST /api/live/stream/start            StartFilePush(req) → Task
 *   listRunning                   GET  /api/live/stream/list             TaskService.ListActive()（type = live_file_push / live_record_push / live_relay）
 *   stopStream                    POST /api/live/stream/stop             Stop(taskID)
 *   startRecordPush               WS   /ws（首条文本 + 二进制分片）      StartRecordPush(req) → { wsURL, token }，前端 MediaRecorder 往 wsURL 写
 *   subscribeStats                GET  /api/live/health 每秒轮询          事件 live:stats { id, bitrateKbps, fps, droppedFrames, uptimeSec }
 *   onLiveEvent                   SSE  /api/sse                          事件 task:status { id, status, error }
 *   getLiveHealth/listArchives/relay*  运维面板用，同上映射到 GetHealth / ListArchives / StartRelay / Stop
 *
 * 错误：统一转成 LiveError（code 对齐契约第 2 节和 errors/errorMessages.ts），页面只认 code。
 * v1 后端区分不了“服务器拒绝推流”和“进程中途退出”，SSE 的 failed 一律按 LIVE_PUSH_INTERRUPTED 处理。
 */
import api from '@/api'

export class LiveError extends Error {
  code: string
  detail?: string
  constructor(code: string, message: string, detail?: string) {
    super(message)
    this.code = code
    this.detail = detail
  }
}

/** 统一错误出口：网络不通、业务 code != 200、HTTP 4xx/5xx 都转成 LiveError */
function toLiveError(e: unknown): LiveError {
  if (e instanceof LiveError) return e
  const err = e as any
  if (err?.response) {
    return new LiveError('INTERNAL', err.response.data?.message || err.response.data?.error || `请求失败（${err.response.status}）`)
  }
  if (err?.request || err?.code === 'ERR_NETWORK') {
    return new LiveError('INTERNAL', '无法连接应用的本地服务，请确认应用后端在运行。')
  }
  return new LiveError('INTERNAL', err?.message || '未知错误')
}

function unwrap<T>(res: { data: any }): T {
  const body = res.data
  if (body && typeof body === 'object' && 'code' in body && body.code !== 200) {
    throw new LiveError('INTERNAL', body.message || '操作失败')
  }
  if (body && typeof body === 'object' && 'error' in body && !('code' in body)) throw new LiveError('INTERNAL', String(body.error))
  return (body && 'data' in body ? body.data : body) as T
}

async function call<T>(fn: () => Promise<{ data: any }>): Promise<T> {
  try {
    return unwrap<T>(await fn())
  } catch (e) {
    throw toLiveError(e)
  }
}

// ───────────── 文件推流 ─────────────

export interface LiveMaterial {
  name: string
  /** 预览用的 http 地址 */
  url: string
  duration: string
  date: string
  cover?: string
}

export interface FilePushRequest {
  material: LiveMaterial
  /** 完整推流地址（已拼好推流码） */
  url: string
  archiveEnabled: boolean
  segmentSeconds: number
  relayTargets: string[]
}

export interface RunningStream {
  streamId: string
  name: string
  url: string
  /** 已推流时长，“HH:MM:SS” */
  duration: string
}

export async function listMaterials(): Promise<LiveMaterial[]> {
  const list = await call<any[]>(() => api.get('/api/getSteamFiles'))
  return (list || []).map((v) => ({ name: v.name, url: v.url, duration: v.duration, date: v.date, cover: v.cover }))
}

export async function uploadMaterial(file: File, onProgress?: (percent: number) => void): Promise<void> {
  const form = new FormData()
  form.append('file', file)
  await call(() =>
    api.post('/api/uploadSteamup', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: (e) => {
        if (e.total) onProgress?.(Math.round((e.loaded * 100) / e.total))
      },
    }),
  )
}

export async function deleteMaterial(name: string): Promise<void> {
  await call(() => api.post('/api/deletesteamVideo', { name }))
}

/** 返回 streamId */
export async function startFilePush(req: FilePushRequest): Promise<string> {
  const data = await call<{ streamId: string }>(() =>
    api.post('/api/live/stream/start', {
      ...req.material,
      steamurl: req.url,
      archiveEnabled: req.archiveEnabled,
      segmentSeconds: req.segmentSeconds,
      relayTargets: req.relayTargets,
    }),
  )
  return data.streamId
}

export async function listRunning(): Promise<RunningStream[]> {
  try {
    const res = await api.get('/api/live/stream/list')
    return ((res.data?.streams as any[]) || []).map((s) => ({ streamId: s.streamId, name: s.name, url: s.steamurl, duration: s.duration }))
  } catch (e) {
    throw toLiveError(e)
  }
}

export async function stopStream(s: { streamId?: string; name?: string; url?: string }): Promise<void> {
  await call(() => api.post('/api/live/stream/stop', { streamId: s.streamId, name: s.name, steamurl: s.url }))
}

// ───────────── 录屏推流 ─────────────

export interface RecordPushOptions {
  /** 完整推流地址（已拼好推流码） */
  url: string
  archiveEnabled: boolean
  segmentSeconds: number
  relayTargets: string[]
  videoBitsPerSecond?: number
}

export interface RecordHandle {
  stop: () => void
}

function wsBase(): string {
  return String(api.defaults.baseURL || '').replace(/^http/, 'ws')
}

/** 把 MediaStream 经 WebSocket 分片写给后端 ffmpeg；后端一断开（或推流进程退出），onClosed 就会触发 */
export function startRecordPush(
  stream: MediaStream,
  opts: RecordPushOptions,
  hooks: { onStarted?: () => void; onClosed: (error?: LiveError) => void },
): RecordHandle {
  const socket = new WebSocket(`${wsBase()}/ws`)
  let recorder: MediaRecorder | null = null
  let closed = false
  let chain: Promise<void> = Promise.resolve()

  const finish = (error?: LiveError) => {
    if (closed) return
    closed = true
    if (recorder && recorder.state !== 'inactive') recorder.stop()
    if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) socket.close()
    hooks.onClosed(error)
  }

  socket.onopen = () => {
    socket.send(
      JSON.stringify({
        type: 'stream_url',
        url: opts.url,
        archiveEnabled: opts.archiveEnabled,
        segmentSeconds: opts.segmentSeconds,
        relayTargets: opts.relayTargets,
      }),
    )
    recorder = new MediaRecorder(stream, { mimeType: 'video/webm;codecs=vp8', videoBitsPerSecond: opts.videoBitsPerSecond })
    recorder.ondataavailable = (ev) => {
      if (ev.data.size === 0) return
      // 保证分片按顺序发送
      chain = chain.then(async () => {
        const buf = await ev.data.arrayBuffer()
        if (socket.readyState === WebSocket.OPEN) socket.send(buf)
      })
    }
    recorder.start(80)
    hooks.onStarted?.()
  }
  socket.onclose = () => finish()
  socket.onerror = () => finish(new LiveError('INTERNAL', '与应用本地服务的连接异常，请确认应用后端在运行。'))

  return { stop: () => finish() }
}

// ───────────── 状态与事件 ─────────────

export interface LiveStatsSample {
  streamId: string
  bitrateKbps: number
  fps: number
  droppedFrames: number
  uptimeSec: number
  status: string
  lastError?: string
}

export interface LiveHealthItem {
  streamId: string
  displayName: string
  input: string
  targets: string[]
  source: 'file' | 'screen' | 'relay'
  status: string
  archiveEnabled: boolean
  segmentSeconds: number
  archiveDir: string
  fps: number
  bitrateKbps: number
  ingressBitrateKbps: number
  speed: number
  outTimeMs: number
  estimatedLatencyMs: number
  dropFrames: number
  dupFrames: number
  health: 'healthy' | 'warning' | 'critical'
  diagnosis: string
  lastError?: string
  startedAt: string
  updatedAt: string
}

export interface LiveArchiveItem {
  streamId: string
  fileName: string
  fileUrl: string
  sizeBytes: number
  updatedAt: string
}

export interface RelayTaskItem {
  streamId: string
  displayName: string
  sourceUrl: string
  targets: string[]
  status: string
  health: 'healthy' | 'warning' | 'critical'
  latencyMs: number
  dropFrames: number
}

export interface RelayStartPayload {
  displayName: string
  sourceUrl: string
  targets: string[]
  archiveEnabled: boolean
  segmentSeconds: number
}

export interface LiveHealth {
  summary: { total: number; active: number; warning: number; critical: number }
  items: LiveHealthItem[]
}

export const getLiveHealth = () => call<LiveHealth>(() => api.get('/api/live/health'))
export const listArchives = () => call<{ count: number; items: LiveArchiveItem[] }>(() => api.get('/api/live/archives'))
export const startRelay = (p: RelayStartPayload) => call(() => api.post('/api/live/relay/start', p))
export const stopRelay = (streamId: string) => call(() => api.post('/api/live/relay/stop', { streamId }))
export const listRelay = () => call<{ items: RelayTaskItem[] }>(() => api.get('/api/live/relay/list'))

/**
 * 订阅实时指标，返回取消订阅函数。
 * match.streamId 精确匹配（文件推流）；只给 source 时取该来源里最新的进行中会话（录屏推流，v1 的 ws 不回传 streamId）。
 * v1 用每秒轮询 /api/live/health；v2 换成 live:stats 事件。
 */
export function subscribeStats(match: { streamId?: string; source?: 'file' | 'screen' }, cb: (s: LiveStatsSample | null) => void): () => void {
  let stopped = false
  const since = Date.now() - 2000 // 只认订阅之后才启动的会话，避免匹配到上一次已结束的
  const tick = async () => {
    try {
      const { items } = await getLiveHealth()
      const candidates = (items || []).filter((i) =>
        match.streamId ? i.streamId === match.streamId : i.source === match.source && Date.parse(i.startedAt) >= since,
      )
      candidates.sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt))
      const it = candidates[0]
      if (stopped) return
      cb(
        it
          ? {
              streamId: it.streamId,
              bitrateKbps: it.bitrateKbps || it.ingressBitrateKbps || 0,
              fps: it.fps || 0,
              droppedFrames: it.dropFrames || 0,
              uptimeSec: Math.max(0, Math.round((Date.now() - Date.parse(it.startedAt)) / 1000)),
              status: it.status,
              lastError: it.lastError,
            }
          : null,
      )
    } catch {
      /* 轮询失败忽略，等下一次 */
    }
  }
  const timer = setInterval(tick, 1000)
  tick()
  return () => {
    stopped = true
    clearInterval(timer)
  }
}

export interface LiveEvent {
  streamId?: string
  filename?: string
  streamUrl?: string
  status: 'completed' | 'stopped' | 'failed'
  error?: string
}

let es: EventSource | null = null
const listeners = new Set<(e: LiveEvent) => void>()

/** 推流进程结束事件（v1 SSE，共用一条连接，最后一个订阅者取消时关闭） */
export function onLiveEvent(cb: (e: LiveEvent) => void): () => void {
  listeners.add(cb)
  if (!es) {
    es = new EventSource(`${api.defaults.baseURL}/api/sse`)
    es.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data) as LiveEvent
        listeners.forEach((l) => l(data))
      } catch (e) {
        console.error('SSE 数据解析失败:', e)
      }
    }
    es.onerror = (e) => console.warn('SSE 连接异常:', e)
  }
  return () => {
    listeners.delete(cb)
    if (listeners.size === 0) {
      es?.close()
      es = null
    }
  }
}
