// 直播预览数据层（契约 v0.25 §6.10.3）。页面只调这里。
// GetPreviewStream(sessionId) → { url, mime, hasVideo, hasAudio }；拉流播放用 StartPullPreview 的 previewUrl（同一个地址）。
// 开关 LIVE_PREVIEW_V25_BACKEND_READY：false 时浏览器模拟层按契约返回 UNSUPPORTED reason=preview_unavailable、previewUrl ""。
// 推流的「开启预览」只决定前端连不连这条地址，不重启推流（6.10.3.2a）。旧的 GetPreview / 每秒 2 帧轮询已删除。
import { AppError, callService, toAppError } from './call'
import { LIVE_PREVIEW_V25_BACKEND_READY } from './flags'
import { startPullPreview, stopPullPreview, type PullSession } from './live'
import { hasWailsBackend } from '@/services/wails'

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
    throw new AppError('UNSUPPORTED', '这路视频无法在应用内预览，推流不受影响。', 'reason=preview_unavailable')
  }
  const r = await callService<Partial<PreviewStream> | null>('LiveService', 'GetPreviewStream', sessionId)
  if (!r?.url) throw new AppError('INTERNAL', '没有拿到预览地址')
  return { url: r.url, mime: r.mime || 'video/x-flv', hasVideo: r.hasVideo !== false, hasAudio: !!r.hasAudio }
}

export interface PullPlayback {
  /** 要 Stop 的后端会话；v0.25 没打开、或直接播用户地址时为 null */
  session: PullSession | null
  /** 交给播放器的地址。v0.25 是 previewUrl；否则是用户填的 http(s) / ws(s) 地址（rtmp 等要等预览服务） */
  stream: PreviewStream | null
}

/**
 * 拉流要播的地址。
 * v0.25：StartPullPreview，播 previewUrl；空则按 preview_unavailable。
 * 开关关闭：不建后端会话，http(s) / ws(s) 直接播（契约 6.10.3.3：ws / wss 本来就由前端播）。
 */
export async function startPullPlayback(url: string): Promise<PullPlayback> {
  if (previewV25IsReal()) {
    const s = await startPullPreview({ url, preview: true })
    if (!s.previewUrl) {
      await stopPullPreview(s.id).catch(() => undefined)
      throw new AppError('UNSUPPORTED', '这路视频无法在应用内预览，推流不受影响。', 'reason=preview_unavailable')
    }
    return { session: s, stream: { url: s.previewUrl, mime: 'video/x-flv', hasVideo: true, hasAudio: true } }
  }
  if (/^(https?|wss?):\/\//i.test(url)) return { session: null, stream: { url, mime: 'video/x-flv', hasVideo: true, hasAudio: true } }
  return { session: null, stream: null }
}

/** 停止拉流的后端会话。没有会话、或 Stop 自己出错，都不抛给界面 */
export async function stopPullPlayback(p: PullPlayback | null): Promise<void> {
  if (p?.session) await stopPullPreview(p.session.id).catch(() => undefined)
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
