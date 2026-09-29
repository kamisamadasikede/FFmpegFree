import { computed, onScopeDispose, reactive, ref } from 'vue'
import { previewParams } from '@/services/wails'

export type LiveKind = 'file' | 'record' | 'pull'
export type LivePhase = 'idle' | 'starting' | 'running' | 'error'

export interface LiveStats {
  bitrateKbps: number
  fps: number
  dropped: number
  /** 已推送 / 已接收字节数 */
  bytes: number
  width: number
  height: number
}

export interface LiveSeries {
  bitrate: number[]
  fps: number[]
  dropped: number[]
  bytes: number[]
}

const SERIES_LEN = 12

/** 地址里带 ?live=running|error|idle 时，直播页用本地模拟数据展示对应状态（只影响界面，不碰后端）。
 *  ?code=LIVE_CORS_BLOCKED 指定错误码，?detail=xxx 指定错误码后面的附加信息。 */
export const livePreview = previewParams.get('live') as 'running' | 'error' | 'idle' | 'invalid' | null

// 与原型 pages.html?page=live 的示例值一致
const SIM = {
  uptimeSec: 42 * 60 + 18,
  bytes: 1.86 * 1024 ** 3,
  bitrate: [6020, 5990, 6005, 5960, 6000, 5955, 5990, 5940, 5970, 5965, 5985, 5950, 6012],
}

export function formatClock(sec: number): string {
  const s = Math.max(0, Math.floor(sec))
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(Math.floor(s / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`
}

/** 字节数 → { value, unit }：GB 两位小数，其余一位 */
export function formatBytes(bytes: number): { value: string; unit: string } {
  if (bytes >= 1024 ** 3) return { value: (bytes / 1024 ** 3).toFixed(2), unit: 'GB' }
  if (bytes >= 1024 ** 2) return { value: (bytes / 1024 ** 2).toFixed(1), unit: 'MB' }
  return { value: (bytes / 1024).toFixed(1), unit: 'KB' }
}

function emptyStats(): LiveStats {
  return { bitrateKbps: 0, fps: 0, dropped: 0, bytes: 0, width: 0, height: 0 }
}

/**
 * 直播页三个页签共用的会话状态：阶段、错误、实时指标、迷你曲线、本地日志，以及预览模拟。
 * 真实数据由各页签通过 api/live.ts（过渡期 v1，之后 Wails LiveService）喂进来。
 */
export function useLiveSession(kind: LiveKind) {
  const phase = ref<LivePhase>('idle')
  const errorCode = ref('')
  const errorDetail = ref('')
  const errorMessage = ref('')
  const uptimeSec = ref(0)
  const stats = reactive<LiveStats>(emptyStats())
  const series = reactive<LiveSeries>({ bitrate: [], fps: [], dropped: [], bytes: [] })
  const logs = ref<string[]>([])
  const isPreview = !!livePreview

  let clock: ReturnType<typeof setInterval> | null = null
  let sim: ReturnType<typeof setInterval> | null = null
  let simTick = 0

  const running = computed(() => phase.value === 'running')
  const busy = computed(() => phase.value === 'starting' || phase.value === 'running')

  function log(text: string) {
    const d = new Date()
    const p = (n: number) => String(n).padStart(2, '0')
    logs.value.push(`${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}  ${text}`)
    if (logs.value.length > 500) logs.value.shift()
  }

  function stopClock() {
    if (clock) clearInterval(clock)
    if (sim) clearInterval(sim)
    clock = sim = null
  }

  function startClock(from = 0) {
    stopClock()
    uptimeSec.value = from
    clock = setInterval(() => uptimeSec.value++, 1000)
  }

  function clearData() {
    Object.assign(stats, emptyStats())
    series.bitrate = []
    series.fps = []
    series.dropped = []
    series.bytes = []
    uptimeSec.value = 0
  }

  function pushSeries(arr: number[], v: number) {
    arr.push(v)
    if (arr.length > SERIES_LEN) arr.shift()
  }

  /** 收到一个指标采样（每秒一次）。bytes 不传时按码率累计估算 */
  function addSample(s: Partial<LiveStats>) {
    Object.assign(stats, s)
    if (s.bytes === undefined) stats.bytes += (stats.bitrateKbps * 1000) / 8
    pushSeries(series.bitrate, stats.bitrateKbps)
    pushSeries(series.fps, stats.fps)
    pushSeries(series.dropped, stats.dropped)
    pushSeries(series.bytes, stats.bytes)
  }

  function setStarting() {
    stopClock()
    clearData()
    phase.value = 'starting'
    errorCode.value = errorDetail.value = errorMessage.value = ''
  }

  function setRunning() {
    if (phase.value === 'running') return
    phase.value = 'running'
    startClock(0)
  }

  function setIdle() {
    stopClock()
    phase.value = 'idle'
  }

  function fail(code: string, detail = '', message = '') {
    stopClock()
    phase.value = 'error'
    errorCode.value = code
    errorDetail.value = detail
    errorMessage.value = message
    log(`失败 ${code}${detail ? ' · ' + detail : ''}${message ? ' · ' + message : ''}`)
  }

  // ───── 预览模拟 ─────
  function simRunning(resolution: { width: number; height: number } = { width: 1920, height: 1080 }) {
    stopClock()
    clearData()
    errorCode.value = errorDetail.value = errorMessage.value = ''
    phase.value = 'running'
    stats.width = resolution.width
    stats.height = resolution.height
    stats.fps = 30
    stats.dropped = 0
    stats.bytes = SIM.bytes
    for (const b of SIM.bitrate) {
      stats.bitrateKbps = b
      pushSeries(series.bitrate, b)
      pushSeries(series.fps, 30)
      pushSeries(series.dropped, 0)
      pushSeries(series.bytes, series.bytes.length) // 累计量单调上升
    }
    series.bytes[series.bytes.length - 1] = SIM.bitrate.length
    startClock(SIM.uptimeSec)
    simTick = 0
    sim = setInterval(() => {
      simTick++
      const b = Math.round(6000 + 40 * Math.sin(simTick * 1.7) + 30 * Math.cos(simTick * 0.6))
      addSample({ bitrateKbps: b, fps: 30, dropped: 0 })
    }, 1000)
    log('（预览模拟）推流中')
  }

  function simError() {
    simRunning()
    stopClock()
    const defCode = kind === 'pull' ? 'LIVE_PLAY_FAILED' : 'LIVE_PUSH_REJECTED'
    const code = previewParams.get('code') || defCode
    const detail = previewParams.get('detail') ?? (code === 'LIVE_PUSH_REJECTED' ? '服务器返回 403' : '')
    fail(code, detail)
  }

  function initPreview() {
    if (livePreview === 'running') simRunning()
    else if (livePreview === 'error') simError()
  }

  onScopeDispose(stopClock)

  return {
    kind, phase, running, busy, isPreview,
    errorCode, errorDetail, errorMessage,
    uptimeSec, stats, series, logs,
    log, addSample, setStarting, setRunning, setIdle, fail, clearData, startClock, stopClock,
    simRunning, initPreview,
  }
}

export type LiveSession = ReturnType<typeof useLiveSession>
