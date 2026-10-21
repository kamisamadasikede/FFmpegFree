/**
 * 预览类型判定（v0.58）：把「扩展名 / MIME / 探测结果」统一成一个可播放类型。
 *
 * 为什么要单独一份：转换结果、源文件、直播存档、对话附件都各自判断过一次，
 * 之前只有 video / audio / gif 三类，图片和文本一律落到「无法在应用内播放」，
 * 明明浏览器能直接显示。这里集中判定，新增格式只改这一处。
 */
export type PreviewKind = 'video' | 'audio' | 'image' | 'gif' | 'pdf' | 'text' | 'other'

/** 浏览器 <img> 能直接显示的图片格式 */
const IMAGE_EXT = new Set(['jpg', 'jpeg', 'png', 'webp', 'bmp', 'avif', 'ico', 'svg'])
/** 走 <video> 的容器（能否真解码由 WebView 决定，解不了再回退提示） */
const VIDEO_EXT = new Set([
  'mp4', 'm4v', 'mov', 'webm', 'mkv', 'avi', 'flv', 'wmv', 'ts', 'm2ts', 'mpg', 'mpeg', '3gp', 'ogv', 'mxf',
])
/** 走 <audio> 的容器 */
const AUDIO_EXT = new Set(['mp3', 'wav', 'aac', 'm4a', 'flac', 'ogg', 'oga', 'opus', 'wma', 'aiff', 'aif', 'amr', 'ac3'])
const TEXT_EXT = new Set(['txt', 'log', 'md', 'json', 'xml', 'csv', 'srt', 'vtt', 'ass', 'ini', 'yaml', 'yml'])

export function extOf(nameOrPath: string): string {
  const base = (nameOrPath || '').split(/[\\/]/).pop() ?? ''
  const i = base.lastIndexOf('.')
  return i > 0 ? base.slice(i + 1).toLowerCase() : ''
}

export interface KindHint {
  /** 扩展名，优先用结果容器的，其次文件名 */
  ext?: string
  /** 后端给的 mime（/local/<token> 会带） */
  mime?: string
  /** 探测结果：只有音频 */
  audioOnly?: boolean
  /** 探测结果：有视频轨 */
  hasVideo?: boolean
}

/**
 * 判定预览类型。顺序：MIME 最可信 → 扩展名 → 探测结果 → 兜底 video。
 * 注意 gif 单独一类：它要能暂停（画到 canvas），不能用 <video>。
 */
export function previewKindOf(hint: KindHint): PreviewKind {
  const ext = (hint.ext ?? '').toLowerCase()
  const mime = (hint.mime ?? '').toLowerCase()

  if (mime === 'image/gif' || ext === 'gif') return 'gif'
  if (mime.startsWith('image/')) return 'image'
  if (mime === 'application/pdf' || ext === 'pdf') return 'pdf'
  if (mime.startsWith('text/')) return 'text'
  // MIME 说了音 / 视频就以它为准：音频可以装在 mp4 / m4a / mkv 这类容器里，
  // 只看扩展名会把「mp4 里只有音轨」判成视频，播放器只剩黑屏。
  if (mime.startsWith('audio/')) return 'audio'
  if (mime.startsWith('video/')) return 'video'

  if (IMAGE_EXT.has(ext)) return 'image'
  if (AUDIO_EXT.has(ext)) return 'audio'
  if (VIDEO_EXT.has(ext)) return 'video'
  if (TEXT_EXT.has(ext)) return 'text'

  if (hint.audioOnly) return 'audio'
  if (hint.hasVideo) return 'video'
  return 'video'
}

/** 该类型是否需要 <video>/<audio> 这种时间轴媒体（决定控制条怎么显示） */
export function hasTimeline(kind: PreviewKind): boolean {
  return kind === 'video' || kind === 'audio'
}

/** 该类型是否由 <img> 承载（可放大、无时间轴） */
export function isStillImage(kind: PreviewKind): boolean {
  return kind === 'image' || kind === 'gif'
}

/** 供提示文案用：人类可读的类型名 */
export function kindLabel(kind: PreviewKind): string {
  switch (kind) {
    case 'video':
      return '视频'
    case 'audio':
      return '音频'
    case 'image':
      return '图片'
    case 'gif':
      return '动图'
    case 'pdf':
      return 'PDF'
    case 'text':
      return '文本'
    default:
      return '文件'
  }
}
