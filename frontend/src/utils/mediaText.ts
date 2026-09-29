/** 媒体信息的显示文字（文件行和信息卡共用） */
import { formatBytes } from '@/utils/format'
import type { store as goStore } from '../../wailsjs/go/models'

const CODEC: Record<string, string> = { h264: 'H.264', hevc: 'HEVC', h265: 'H.265', vp9: 'VP9', vp8: 'VP8', av1: 'AV1', mpeg4: 'MPEG-4', aac: 'AAC', mp3: 'MP3', opus: 'Opus', vorbis: 'Vorbis', flac: 'FLAC', ac3: 'AC-3', wmav2: 'WMA' }

export function codecName(c?: string): string {
  if (!c) return ''
  if (c.toLowerCase() === 'copy') return '原编码'
  return CODEC[c.toLowerCase()] ?? (c.startsWith('pcm') ? 'PCM' : c.toUpperCase())
}

export function channelText(n?: number): string {
  if (!n) return ''
  return n === 1 ? '单声道' : n === 2 ? '立体声' : `${n} 声道`
}

export function sampleRateText(hz?: number): string {
  return hz ? `${+(hz / 1000).toFixed(1)} kHz` : ''
}

export function bitrateText(bps?: number): string {
  return bps && bps > 0 ? `${Math.round(bps / 1000)} kbps` : ''
}

/** 信息卡 / 文件行用：有画面的文件按视频显示，其余按音频 */
export function isAudioInfo(i?: goStore.MediaInfo): boolean {
  return !!i && (i.hasVideo === false || !i.width)
}

/** 行内一行信息：分辨率 · 编码 · 大小（音频：采样率 · 声道 · 大小） */
export function rowInfoText(i: goStore.MediaInfo): string {
  const parts: string[] = []
  if (!isAudioInfo(i)) parts.push(`${i.width}×${i.height}`, codecName(i.videoCodec))
  else {
    parts.push(sampleRateText(i.sampleRate), channelText(i.channels))
    if (!i.sampleRate && !i.channels && i.audioCodec) parts.push(codecName(i.audioCodec))
  }
  parts.push(formatBytes(i.size))
  return parts.filter(Boolean).join(' · ')
}
