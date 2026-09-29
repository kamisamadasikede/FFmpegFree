import * as ConvertBinding from '../../wailsjs/go/app/ConvertService'
import { ffmpeg, store as goStore } from '../../wailsjs/go/models'
import { call } from '@/api/call'
import { hasWailsBackend } from '@/services/wails'

/** 一次 Submit 最多 50 个文件（契约 6.9） */
export const MAX_SUBMIT = 50

export interface PresetItem {
  id: string
  name: string
  builtIn: boolean
  options: ffmpeg.ConvertOptions
}

export async function listPresets(): Promise<PresetItem[]> {
  if (!hasWailsBackend()) return PREVIEW_PRESETS
  const list = await call(ConvertBinding.ListPresets())
  return (list ?? []).map((p) => ({ id: p.id, name: p.name, builtIn: p.builtIn, options: p.options }))
}

/** 提交：一个文件一个任务，返回顺序与 inputs 一致。整体校验，失败则一个任务都没提交（detail 第一行是出错文件路径）。 */
export async function submitConvert(inputs: string[], options: ffmpeg.ConvertOptions, outputDir: string): Promise<goStore.Task[]> {
  // targetSizeMb 暂缓，必须为 0；这里统一清零，防止预设 / 旧参数里带进来
  const opts = ffmpeg.ConvertOptions.createFrom({ ...options, targetSizeMb: 0 })
  return await call(ConvertBinding.Submit(inputs, opts, outputDir))
}

/** convert 任务的 params：{input, options, outputDir}（契约 6.9） */
export interface ConvertParams {
  input: string
  options: ffmpeg.ConvertOptions
  outputDir: string
}

/**
 * 解析 convert 任务的 params。只有形态完全符合时才返回，否则 null（调用方保留原提示，不猜）：
 * input 非空字符串、options 是带 container 的对象、outputDir 是字符串（可空）。
 */
export function parseConvertParams(params: string | undefined | null): ConvertParams | null {
  if (!params) return null
  try {
    const p = JSON.parse(params)
    if (!p || typeof p !== 'object') return null
    if (typeof p.input !== 'string' || !p.input.trim()) return null
    const o = p.options
    if (!o || typeof o !== 'object' || typeof o.container !== 'string' || !o.container) return null
    return { input: p.input, options: ffmpeg.ConvertOptions.createFrom(o), outputDir: typeof p.outputDir === 'string' ? p.outputDir : '' }
  } catch {
    return null
  }
}

/** 用原来的输入和参数、新的输出文件夹重新提交（仅这一次，不改默认输出位置）；返回新任务 */
export async function resubmitToDir(params: ConvertParams, dir: string): Promise<goStore.Task> {
  const list = await submitConvert([params.input], params.options, dir)
  return list[0]
}

// ---- 预览用预设（与后端 12 个内置预设一致）----
const mk = (id: string, name: string, o: Partial<ffmpeg.ConvertOptions>): PresetItem => ({
  id, name, builtIn: true, options: ffmpeg.ConvertOptions.createFrom({ container: '', videoCodec: '', audioCodec: '', width: 0, height: 0, fps: 0, videoBitrate: 0, audioBitrate: 0, crf: 0, targetSizeMb: 0, trimStart: 0, trimEnd: 0, ...o }),
})
const PREVIEW_PRESETS: PresetItem[] = [
  mk('builtin-mp4-h264', 'MP4（H.264 + AAC，通用）', { container: 'mp4', videoCodec: 'h264', audioCodec: 'aac' }),
  mk('builtin-mp4-h264-1080p', 'MP4 1080p（H.264 + AAC）', { container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', width: 1920 }),
  mk('builtin-mp4-h264-720p', 'MP4 720p（H.264 + AAC）', { container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', width: 1280 }),
  mk('builtin-mp4-h265', 'MP4（H.265 + AAC，体积更小）', { container: 'mp4', videoCodec: 'h265', audioCodec: 'aac' }),
  mk('builtin-mkv-copy', 'MKV（不重新编码，只换封装）', { container: 'mkv', videoCodec: 'copy', audioCodec: 'copy' }),
  mk('builtin-mov-h264', 'MOV（H.264 + AAC）', { container: 'mov', videoCodec: 'h264', audioCodec: 'aac' }),
  mk('builtin-webm-vp9', 'WebM（VP9 + Opus）', { container: 'webm', videoCodec: 'vp9', audioCodec: 'opus' }),
  mk('builtin-gif', 'GIF 动图（宽 480，12 帧/秒）', { container: 'gif', width: 480, fps: 12 }),
  mk('builtin-mp3', 'MP3（192 kbps）', { container: 'mp3', audioCodec: 'mp3', audioBitrate: 192000 }),
  mk('builtin-m4a', 'M4A（AAC 192 kbps）', { container: 'm4a', audioCodec: 'aac', audioBitrate: 192000 }),
  mk('builtin-wav', 'WAV（无损 PCM）', { container: 'wav', audioCodec: 'pcm' }),
  mk('builtin-flac', 'FLAC（无损压缩）', { container: 'flac', audioCodec: 'flac' }),
]
