/**
 * 直播页的全部后端调用都在这个文件里。
 *
 * 现状：LiveService 还没有落地，LIVE_BACKEND_READY = false，所有推流 / 控制调用都走下面的本地模拟（固定的演示指标），
 * 不发任何 HTTP / SSE / WebSocket 请求。v1 的本地 gin 服务已移除，不再兼容。
 *
 * 迁移：后端 LiveService 落地后，把各函数里 `if (!LIVE_BACKEND_READY) return sim…` 之外的分支换成 Wails 绑定调用，
 * 再把 LIVE_BACKEND_READY 改成 true，并删掉页面顶部的演示提示条（LiveLayout.vue）。页面和 composable 不用动。对照表（契约 v0.4）：
 *
 *   本文件函数                    v2 LiveService / 事件
 *   listMaterials/uploadMaterial  SystemService.PickFiles（直接选本地绝对路径，不再上传拷贝）
 *   deleteMaterial                v2 不拷贝素材，无此操作
 *   startFilePush                 StartFilePush(req) → Task
 *   listRunning                   TaskService.ListActive()（type = live_file_push / live_record_push / live_relay）
 *   stopStream                    Stop(taskID)
 *   startRecordPush               StartRecordPush(req) → { wsURL, token }
 *   subscribeStats                事件 live:stats { id, bitrateKbps, fps, droppedFrames, uptimeSec }
 *   onLiveEvent                   事件 task:status { id, status, error }
 *
 * 错误：统一转成 LiveError（code 对齐契约第 2 节和 errors/errorMessages.ts），页面只认 code。
 */

/** 后端 LiveService 是否已接入。false 时所有推流 / 控制调用走本地模拟，不发任何网络请求。 */
export const LIVE_BACKEND_READY = false

export class LiveError extends Error {
  code: string
  detail?: string
  constructor(code: string, message: string, detail?: string) {
    super(message)
    this.code = code
    this.detail = detail
  }
}

/** LIVE_BACKEND_READY 变 true 之后、真实调用还没接上之前的占位：明确报错，而不是悄悄走模拟 */
function notWired(): never {
  throw new LiveError('INTERNAL', '直播后端调用尚未接入')
}

const delay = (ms: number) => new Promise<void>((r) => setTimeout(r, ms))

// ───────────── 文件推流 ─────────────

export interface LiveMaterial {
  name: string
  /** 预览用的地址（演示数据没有真实文件，为空） */
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

// 演示素材（内存里，刷新即恢复）
const demoMaterials: LiveMaterial[] = [
  { name: '产品发布会.mp4', url: '', duration: '00:42:18', date: '2026-09-28 20:11:03' },
  { name: '直播预热片.mp4', url: '', duration: '00:03:12', date: '2026-09-27 09:30:45' },
]

export async function listMaterials(): Promise<LiveMaterial[]> {
  if (!LIVE_BACKEND_READY) return demoMaterials.map((m) => ({ ...m }))
  return notWired()
}

export async function uploadMaterial(file: File, onProgress?: (percent: number) => void): Promise<void> {
  if (!LIVE_BACKEND_READY) {
    for (const p of [30, 70, 100]) {
      await delay(120)
      onProgress?.(p)
    }
    if (!demoMaterials.some((m) => m.name === file.name)) {
      demoMaterials.push({ name: file.name, url: '', duration: '00:00:00', date: new Date().toLocaleString('sv-SE') })
    }
    return
  }
  return notWired()
}

export async function deleteMaterial(name: string): Promise<void> {
  if (!LIVE_BACKEND_READY) {
    const i = demoMaterials.findIndex((m) => m.name === name)
    if (i >= 0) demoMaterials.splice(i, 1)
    return
  }
  return notWired()
}

/** 返回 streamId */
export async function startFilePush(_req: FilePushRequest): Promise<string> {
  if (!LIVE_BACKEND_READY) {
    await delay(300)
    return 'demo-file-push'
  }
  return notWired()
}

export async function listRunning(): Promise<RunningStream[]> {
  if (!LIVE_BACKEND_READY) return []
  return notWired()
}

export async function stopStream(_s: { streamId?: string; name?: string; url?: string }): Promise<void> {
  if (!LIVE_BACKEND_READY) return
  return notWired()
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

/** 开始录屏推流。演示模式下 stream 为 null（不采集屏幕），只模拟“已连接 → 运行 → 停止” */
export function startRecordPush(
  _stream: MediaStream | null,
  _opts: RecordPushOptions,
  hooks: { onStarted?: () => void; onClosed: (error?: LiveError) => void },
): RecordHandle {
  if (!LIVE_BACKEND_READY) {
    let closed = false
    const t = setTimeout(() => !closed && hooks.onStarted?.(), 300)
    return {
      stop: () => {
        if (closed) return
        closed = true
        clearTimeout(t)
        hooks.onClosed()
      },
    }
  }
  return notWired()
}

// ───────────── 状态与事件 ─────────────

export interface LiveStatsSample {
  streamId: string
  bitrateKbps: number
  fps: number
  droppedFrames: number
  uptimeSec: number
  status: string
  /** 演示模式带固定分辨率；真实模式由播放器 / 采集轨道取 */
  width?: number
  height?: number
  lastError?: string
}

// 与原型 pages.html?page=live 的示例值一致的固定演示指标
const DEMO_BITRATE = [6020, 5990, 6005, 5960, 6000, 5955, 5990, 5940, 5970, 5965, 5985, 5950]

/**
 * 订阅实时指标，返回取消订阅函数。
 * 演示模式：每秒一个固定的采样（6 Mbps 上下小幅波动、30 fps、0 丢帧、1920×1080），不发任何请求。
 */
export function subscribeStats(match: { streamId?: string; source?: 'file' | 'screen' }, cb: (s: LiveStatsSample | null) => void): () => void {
  if (!LIVE_BACKEND_READY) {
    let n = 0
    const id = match.streamId ?? 'demo-record-push'
    const tick = () => {
      cb({
        streamId: id,
        bitrateKbps: DEMO_BITRATE[n % DEMO_BITRATE.length],
        fps: 30,
        droppedFrames: 0,
        uptimeSec: n,
        status: 'running',
        width: 1920,
        height: 1080,
      })
      n++
    }
    const timer = setInterval(tick, 1000)
    tick()
    return () => clearInterval(timer)
  }
  return notWired()
}

export interface LiveEvent {
  streamId?: string
  filename?: string
  streamUrl?: string
  status: 'completed' | 'stopped' | 'failed'
  error?: string
}

/** 推流结束事件订阅，返回取消订阅函数。演示模式不会有事件 */
export function onLiveEvent(_cb: (e: LiveEvent) => void): () => void {
  if (!LIVE_BACKEND_READY) return () => undefined
  return notWired()
}
