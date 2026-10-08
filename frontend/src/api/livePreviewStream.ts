// 直播预览数据层（契约 v0.25 §6.10.3）。页面只调这里。
// GetPreviewStream(sessionId) → { url, mime, hasVideo, hasAudio }；拉流播放用 StartPullPreview 的 previewUrl（同一个地址），状态走 live:pull 事件。
// 开关 LIVE_PREVIEW_V25_BACKEND_READY：打开且在 Wails 里调真实绑定；否则（纯浏览器）按契约 6.10.3.1 模拟。
// 推流的「开启预览」只决定前端连不连这条地址，不重启推流（6.10.3.2a）。
import * as LiveBinding from '../../wailsjs/go/app/LiveService'
import { AppError, call, toAppError } from './call'
import { LIVE_PREVIEW_V25_BACKEND_READY } from './flags'
import { startPullPreview, stopPullPreview, type PullSession } from './live'
import { hasWailsBackend, onEvent } from '@/services/wails'
import { LP_END_PULL, LP_END_PULL_REMOTE, LP_PULL_FAILED, LP_UNAVAILABLE } from '@/errors/livePreviewMessages'

export interface PreviewStream {
  url: string
  /** 契约：恒为 video/x-flv */
  mime: string
  hasVideo: boolean
  hasAudio: boolean
}

/** 契约 6.10.3.1：开关打开并且运行在 Wails 里才调真实绑定 */
export const previewV25IsReal = (): boolean => LIVE_PREVIEW_V25_BACKEND_READY && hasWailsBackend()

/** 推流预览或拉流预览会话的播放地址 */
export async function getPreviewStream(sessionId: string): Promise<PreviewStream> {
  if (!previewV25IsReal()) {
    // 浏览器模拟层没有本机预览服务（契约 6.10.3.1）
    throw new AppError('UNSUPPORTED', LP_UNAVAILABLE, 'reason=preview_unavailable')
  }
  const r = (await call(LiveBinding.GetPreviewStream(sessionId))) as Partial<PreviewStream> | null
  if (!r?.url) throw new AppError('INTERNAL', '没有拿到预览地址')
  return { url: r.url, mime: r.mime || 'video/x-flv', hasVideo: r.hasVideo !== false, hasAudio: !!r.hasAudio }
}

export interface PullPlayback {
  /** 要 Stop 的后端会话；直接播用户地址（ws / wss，或纯浏览器）时为 null */
  session: PullSession | null
  /** 交给播放器的地址 */
  stream: PreviewStream | null
  /**
   * stream.hasVideo / hasAudio 是不是后端给的真实值（包 24 N2）。false = 还不知道，先按有画面有声音，
   * 再看 live:pull playing 带的字段（v0.25.3）、GetPreviewStream 和播放器读到的媒体信息。
   */
  mediaKnown: boolean
}

/**
 * 拉流要播的地址。
 * - ws / wss：后端不收（LIVE_URL_INVALID），前端照旧用 mpegts.js 直接拉（契约 6.10.3.3）。
 * - v0.25 真实后端：StartPullPreview，播 previewUrl（后台还在探测，previewUrl 立即给出）；preview=false 或地址为空按 preview_unavailable。
 * - 纯浏览器：http(s) / ws(s) 直接播。
 */
export async function startPullPlayback(url: string): Promise<PullPlayback> {
  const direct = { session: null, stream: { url, mime: 'video/x-flv', hasVideo: true, hasAudio: true }, mediaKnown: false }
  if (/^wss?:\/\//i.test(url)) return direct
  if (previewV25IsReal()) {
    const s = await startPullPreview({ url, preview: true })
    if (!s.preview || !s.previewUrl) {
      await stopPullPreview(s.id).catch(() => undefined)
      throw new AppError('UNSUPPORTED', LP_UNAVAILABLE, 'reason=preview_unavailable')
    }
    // 包 24 N2：不再写死有画面有声音；后端带了就用（v0.25.3），没带先按都有，之后再纠正
    const known = typeof s.hasVideo === 'boolean'
    return { session: s, stream: { url: s.previewUrl, mime: 'video/x-flv', hasVideo: s.hasVideo ?? true, hasAudio: s.hasAudio ?? true }, mediaKnown: known }
  }
  if (/^https?:\/\//i.test(url)) return direct
  return { session: null, stream: null, mediaKnown: false }
}

/** 停止拉流的后端会话。没有会话、或 Stop 自己出错，都不抛给界面 */
export async function stopPullPlayback(p: PullPlayback | null): Promise<void> {
  if (p?.session) await stopPullPreview(p.session.id).catch(() => undefined)
}

/** live:pull 事件（契约 6.10.3.7）。playing 只发一次；error 是 AppError 形状，地址已脱敏 */
export type PullState = 'playing' | 'ended' | 'interrupted' | 'failed' | 'unsupported'
export interface PullEvent {
  id: string
  state: PullState
  error?: { code?: string; message?: string; detail?: string }
  /** v0.25.3（包 24 N2，后端待合）：playing 事件带上这路流有没有画面 / 声音，和 GetPreviewStream 同名；旧后端没有 */
  hasVideo?: boolean
  hasAudio?: boolean
}

/** 订阅某个拉流预览会话的 live:pull；纯浏览器下什么也不做 */
export function watchPull(sessionId: string, cb: (e: PullEvent) => void): () => void {
  return onEvent<PullEvent>('live:pull', (e) => {
    if (e && e.id === sessionId) cb(e)
  })
}

/**
 * 拉流结束时播放器上显示什么（产品经理 10-08 定稿）：
 * 用户自己点「停止播放」→ 只有标题；不是用户停的（live:pull ended / 播放器读到流结尾）→ 标题 + 第二行 + 「重新拉流」（同一地址直接重开）。
 */
export function pullEndedView(userStopped: boolean): { title: string; note: string; retry: boolean } {
  return userStopped ? { title: LP_END_PULL, note: '', retry: false } : { title: LP_END_PULL, note: LP_END_PULL_REMOTE, retry: true }
}

/**
 * 拉流开始前就失败（live:pull failed、StartPullPreview 出错）时画面上的正文（G3）：用后端分类好的 message；
 * 没有 message、或 message 写的是推流（后端复用了推流的分类）时用 LP_PULL_FAILED，和后端拉流失败的文案一致。
 */
export function pullBreakText(message: string | undefined | null): string {
  const m = (message ?? '').trim()
  return !m || m.includes('推流') ? LP_PULL_FAILED : m
}

export type PreviewFailure = 'unsupported' | 'unavailable' | 'ended' | 'failed'

/** 后端错误 → 播放器。codec 与 preview_unavailable 分开：后者一律用推流那句文案（架构师定） */
export function classifyPreviewError(e: unknown): PreviewFailure {
  const a = toAppError(e)
  if (a.code === 'UNSUPPORTED' && a.reason === 'codec') return 'unsupported'
  if (a.code === 'UNSUPPORTED' && a.reason === 'preview_unavailable') return 'unavailable'
  if (a.code === 'NOT_FOUND' || a.code === 'TASK_CONFLICT') return 'ended'
  return 'failed'
}
