// 预览取帧轮询核心（不依赖 Vue，check:api 里用假时钟做自检）。规则（设计稿 v1.1 §6.1 + 任务书）：
//   · 约 500ms 调一次 GetPreview；上一次没返回不发下一次（请求不重叠）；
//   · 用事件序号丢弃过期返回（停止 / 重新开始 / 页面隐藏之后回来的旧结果不生效）；
//   · 返回空帧不是错误，只算“还没画面”（加载中）；有过画面之后的空帧保持上一帧；
//   · 首帧 10 秒仍没有画面 → 失败；连续 5 次取帧出错（约 2.5 秒）→ 失败；失败后不再自动轮询，点重试才重新开始；
//   · active=false（会话结束）→ 保留最后一帧，停止轮询；页面不可见 → 停止，重新可见立即取一次再继续；
//   · 时间戳只用来丢弃过期帧（ts 不比已显示的新就不更新），不给用户看。
import type { LivePreview } from '@/api/live'

export type PreviewPhase = 'idle' | 'loading' | 'ok' | 'failed' | 'ended'
export interface PreviewSnapshot {
  phase: PreviewPhase
  /** 最后一帧 JPEG 的 base64（不带 data: 前缀）；没有为 '' */
  data: string
  ts: number
}
export interface PollerClock {
  now(): number
  setTimeout(fn: () => void, ms: number): unknown
  clearTimeout(h: unknown): void
}
export interface PreviewPollerOptions {
  fetch: (sessionId: string) => Promise<LivePreview>
  onChange?: (s: PreviewSnapshot) => void
  intervalMs?: number
  firstFrameTimeoutMs?: number
  maxErrors?: number
  clock?: PollerClock
}

export const PREVIEW_INTERVAL_MS = 500
export const PREVIEW_FIRST_FRAME_TIMEOUT_MS = 10_000
export const PREVIEW_MAX_ERRORS = 5
export const PREVIEW_DATA_URL_PREFIX = 'data:image/jpeg;base64,'
/** 直接拼 data URL，不建 blob（不累积对象 URL，旧字符串随下一帧被 GC） */
export const previewDataUrl = (b64: string): string => PREVIEW_DATA_URL_PREFIX + b64

const realClock: PollerClock = { now: () => Date.now(), setTimeout: (fn, ms) => setTimeout(fn, ms), clearTimeout: (h) => clearTimeout(h as ReturnType<typeof setTimeout>) }

export class PreviewPoller {
  private opts: Required<Omit<PreviewPollerOptions, 'onChange'>> & Pick<PreviewPollerOptions, 'onChange'>
  private id = ''
  private seq = 0
  private inflight = false
  private running = false
  private visible = true
  private disposed = false
  private errors = 0
  private pollTimer: unknown = null
  private frameTimer: unknown = null
  /** 已发出的 GetPreview 次数（自检用） */
  requests = 0
  snap: PreviewSnapshot = { phase: 'idle', data: '', ts: 0 }

  constructor(o: PreviewPollerOptions) {
    this.opts = { intervalMs: PREVIEW_INTERVAL_MS, firstFrameTimeoutMs: PREVIEW_FIRST_FRAME_TIMEOUT_MS, maxErrors: PREVIEW_MAX_ERRORS, clock: realClock, ...o }
  }

  get sessionId(): string {
    return this.id
  }

  get isRunning(): boolean {
    return this.running
  }

  private set(next: Partial<PreviewSnapshot>) {
    this.snap = { ...this.snap, ...next }
    if (!this.disposed) this.opts.onChange?.(this.snap)
  }
  private clearTimers() {
    if (this.pollTimer !== null) this.opts.clock.clearTimeout(this.pollTimer)
    if (this.frameTimer !== null) this.opts.clock.clearTimeout(this.frameTimer)
    this.pollTimer = this.frameTimer = null
  }

  /** 开始（或重新开始）轮询一个会话：回到“加载中”，清空上一会话的画面 */
  start(sessionId: string): void {
    if (this.disposed) return
    this.clearTimers()
    this.seq++
    this.id = sessionId
    this.errors = 0
    this.running = true
    this.snap = { phase: 'loading', data: '', ts: 0 }
    this.opts.onChange?.(this.snap)
    if (this.visible) {
      this.armFrameTimer()
      this.poll()
    }
  }

  /** 失败后点“重试” */
  retry(): void {
    if (this.id && this.snap.phase === 'failed') this.start(this.id)
  }

  /** 停止轮询，保持当前显示（预览开关关闭、切走当前预览行等） */
  stop(): void {
    this.running = false
    this.seq++
    this.clearTimers()
  }

  /** 会话结束：停止轮询，保留最后一帧 */
  end(): void {
    if (this.disposed) return
    this.stop()
    if (this.snap.phase !== 'ended') this.set({ phase: 'ended' })
  }

  /** 回到空闲（没有会话） */
  reset(): void {
    this.stop()
    this.id = ''
    this.snap = { phase: 'idle', data: '', ts: 0 }
    if (!this.disposed) this.opts.onChange?.(this.snap)
  }

  setVisible(v: boolean): void {
    if (v === this.visible) return
    this.visible = v
    if (!v) {
      // 隐藏：停止排队，丢弃在途请求的结果；首帧计时清掉（隐藏的时间不算）
      this.seq++
      this.clearTimers()
      return
    }
    if (!this.running) return
    if (!this.snap.data) this.armFrameTimer()
    this.schedule(0) // 重新可见：立即取一次再继续（在途请求没回来时等它回来后再取，不叠加）
  }

  dispose(): void {
    this.disposed = true
    this.running = false
    this.seq++
    this.clearTimers()
  }

  private armFrameTimer() {
    if (this.frameTimer !== null) this.opts.clock.clearTimeout(this.frameTimer)
    this.frameTimer = this.opts.clock.setTimeout(() => {
      this.frameTimer = null
      if (this.running && this.visible && !this.snap.data) this.fail()
    }, this.opts.firstFrameTimeoutMs)
  }

  private schedule(ms: number) {
    if (!this.running || !this.visible || this.inflight) return
    if (this.pollTimer !== null) this.opts.clock.clearTimeout(this.pollTimer)
    this.pollTimer = this.opts.clock.setTimeout(() => {
      this.pollTimer = null
      this.poll()
    }, ms)
  }

  private fail() {
    this.running = false
    this.seq++
    this.clearTimers()
    this.set({ phase: 'failed' })
  }

  private poll() {
    if (!this.running || !this.visible || this.inflight || this.disposed) return
    this.inflight = true
    this.requests++
    const my = ++this.seq
    const began = this.opts.clock.now()
    let p: Promise<LivePreview>
    try {
      p = this.opts.fetch(this.id)
    } catch (e) {
      p = Promise.reject(e)
    }
    p.then(
      (r) => {
        this.inflight = false
        if (my === this.seq) this.onResult(r)
        this.after(began)
      },
      () => {
        this.inflight = false
        if (my === this.seq) this.onError()
        this.after(began)
      },
    )
  }

  private after(began: number) {
    const spent = this.opts.clock.now() - began
    this.schedule(Math.max(0, this.opts.intervalMs - spent))
  }

  private onError() {
    if (++this.errors >= this.opts.maxErrors) this.fail()
  }

  private onResult(r: LivePreview) {
    this.errors = 0
    if (r.data && (!this.snap.data || r.ts > this.snap.ts)) {
      this.set({ phase: this.snap.phase === 'ended' ? 'ended' : 'ok', data: r.data, ts: r.ts })
      if (this.frameTimer !== null) {
        this.opts.clock.clearTimeout(this.frameTimer)
        this.frameTimer = null
      }
    }
    if (!r.active) return this.end()
    // 空帧：不是错误。还没有过画面 = 继续“加载中”；有过画面 = 保持上一帧
  }
}
