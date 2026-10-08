/** 媒体信息的显示文字（文件行和信息卡共用） */
import { formatBytes } from '@/utils/format'
import type { store as goStore } from '../../wailsjs/go/models'

/**
 * 编码显示名——前端唯一的一张表（文件行、信息卡、预览弹窗、预设卡片都用 codecName；界面上统一写 H.265、ProRes，不出现 HEVC / PRORES）。
 * 后端也在改成一张共用的编码名表（走查 X3）；这里和它保持同样的写法。
 * 表里没有的：原样转成大写（编码名多是缩写，如 FFV1 / HUFFYUV），不自己做首字母大写（走查 X3：FFV1 被写成“Ffv1”）。
 */
const CODEC: Record<string, string> = {
  h264: 'H.264', avc: 'H.264', hevc: 'H.265', h265: 'H.265', prores: 'ProRes', vp9: 'VP9', vp8: 'VP8', av1: 'AV1', mpeg4: 'MPEG-4', mpeg2video: 'MPEG-2', mpeg1video: 'MPEG-1', mjpeg: 'MJPEG',
  dnxhd: 'DNxHD', ffv1: 'FFV1', theora: 'Theora', gif: 'GIF', png: 'PNG', vc1: 'VC-1', wmv1: 'WMV', wmv2: 'WMV', wmv3: 'WMV', cinepak: 'Cinepak', huffyuv: 'HuffYUV',
  aac: 'AAC', mp3: 'MP3', mp2: 'MP2', opus: 'Opus', vorbis: 'Vorbis', flac: 'FLAC', alac: 'ALAC', ac3: 'AC-3', eac3: 'E-AC-3', dts: 'DTS', truehd: 'TrueHD', wmav1: 'WMA', wmav2: 'WMA', amr_nb: 'AMR', amr_wb: 'AMR-WB', ape: 'APE', wavpack: 'WavPack',
}

export function codecName(c?: string): string {
  if (!c) return ''
  const k = c.toLowerCase()
  if (k === 'copy') return '原编码'
  if (CODEC[k]) return CODEC[k]
  if (k.startsWith('pcm')) return 'PCM'
  return c.toUpperCase()
}

/**
 * 媒体信息里的编码显示名（契约 v0.23.4）：优先用后端按 internal/codecname 唯一一张表生成的 videoCodecName / audioCodecName，
 * 前端原样显示（不做大小写处理）；缺了（旧数据、模拟层）才退回上面的 codecName。
 */
type CodecInfo = { videoCodec?: string; audioCodec?: string; videoCodecName?: string; audioCodecName?: string }
export function videoCodecText(i?: CodecInfo): string {
  return i?.videoCodecName || codecName(i?.videoCodec)
}
export function audioCodecText(i?: CodecInfo): string {
  return i?.audioCodecName || codecName(i?.audioCodec)
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
  if (!isAudioInfo(i)) parts.push(`${i.width}×${i.height}`, videoCodecText(i))
  else {
    parts.push(sampleRateText(i.sampleRate), channelText(i.channels))
    if (!i.sampleRate && !i.channels && i.audioCodec) parts.push(audioCodecText(i))
  }
  parts.push(formatBytes(i.size))
  return parts.filter(Boolean).join(' · ')
}
