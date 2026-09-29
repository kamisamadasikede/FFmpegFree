import * as MediaBinding from '../../wailsjs/go/app/MediaService'
import { store as goStore } from '../../wailsjs/go/models'
import { AppError, call, toAppError } from '@/api/call'
import { hasWailsBackend } from '@/services/wails'

/** Probe 一次最多 500 个（契约）；转换页每 8 个一批回调一次，大列表读取时能看到 12/50 这样的进度 */
export const PROBE_BATCH = 8

/** 一个文件的探测结果：要么有 info，要么有 error（单个文件失败不影响整批） */
export interface ProbeResult {
  path: string
  info?: goStore.MediaInfo
  error?: { code: string; message: string; detail?: string }
}

/**
 * 批量探测：返回值与入参一一对应。
 * 整批调用失败（FFMPEG_NOT_FOUND / INTERNAL 等）时，这一批每个文件都带上同一个 error，后面的批次照常尝试。
 */
export async function probeFiles(paths: string[], onBatch?: (results: ProbeResult[]) => void): Promise<ProbeResult[]> {
  const all: ProbeResult[] = []
  for (let i = 0; i < paths.length; i += PROBE_BATCH) {
    const chunk = paths.slice(i, i + PROBE_BATCH)
    let results: ProbeResult[]
    try {
      const infos = hasWailsBackend() ? await call(MediaBinding.Probe(chunk)) : await previewProbe(chunk)
      results = chunk.map((path, k) => {
        const info = infos?.[k]
        if (!info) return { path, error: { code: 'INTERNAL', message: '没有拿到这个文件的信息' } }
        if (info.error) return { path, error: { code: info.error.code, message: info.error.message, detail: info.error.detail } }
        return { path, info }
      })
    } catch (e) {
      const err = toAppError(e)
      results = chunk.map((path) => ({ path, error: { code: err.code, message: err.message, detail: err.detail } }))
    }
    all.push(...results)
    onBatch?.(results)
  }
  return all
}

/** 缩略图（data URL，宽度约 width）。没有画面 / 失败返回 ''，调用方显示占位图标 */
export async function thumbnailOf(path: string, atSec: number, width = 96): Promise<string> {
  if (!hasWailsBackend()) return previewThumb(path)
  try {
    const t = await call(MediaBinding.Thumbnail(path, atSec, width))
    return t?.dataUrl ?? ''
  } catch (e) {
    if (e instanceof AppError && e.code === 'FFMPEG_NOT_FOUND') return ''
    console.warn('Thumbnail failed', path, e)
    return ''
  }
}

// ---- 浏览器预览用的假数据（没有 window.go） ----
const KNOWN: Record<string, Partial<goStore.MediaInfo>> = {
  '产品发布会_完整版.mov': { size: 862 * 1024 ** 2, duration: 724, width: 1920, height: 1080, videoCodec: 'h264', audioCodec: 'aac', container: 'mov' },
  'vlog_杭州西湖.mkv': { size: 512 * 1024 ** 2, duration: 201, width: 3840, height: 2160, videoCodec: 'hevc', audioCodec: 'aac', container: 'matroska' },
  '访谈录音_第三期.wav': { size: 54 * 1024 ** 2, duration: 285, videoCodec: '', audioCodec: 'pcm_s16le', sampleRate: 48000, channels: 2, container: 'wav' },
  'screen_record_0928.flv': { size: 41 * 1024 ** 2, duration: 58, width: 1280, height: 720, videoCodec: 'h264', audioCodec: 'aac', container: 'flv' },
}
function baseName(p: string) {
  return p.split(/[\\/]/).pop() ?? p
}
async function previewProbe(paths: string[]): Promise<goStore.MediaInfo[]> {
  // ?slowprobe=毫秒：预览里模拟每批的探测耗时，用来看“正在读取文件信息（12/50）”
  const delay = Number(new URLSearchParams(window.location.search).get('slowprobe')) || 0
  if (delay) await new Promise((r) => setTimeout(r, delay))
  return paths.map((p) => {
    const name = baseName(p)
    if (name.startsWith('损坏')) {
      return goStore.MediaInfo.createFrom({ path: p, name, error: { code: 'PROBE_FAILED', message: '无法解析这个文件', detail: 'Invalid data found when processing input' } })
    }
    const k = KNOWN[name] ?? { size: 128 * 1024 ** 2, duration: 95, width: 1920, height: 1080, videoCodec: 'h264', audioCodec: 'aac', container: 'mp4' }
    const audioOnly = !k.width
    return goStore.MediaInfo.createFrom({ path: p, name, hasVideo: !audioOnly, hasAudio: true, channels: 2, ...k })
  })
}
const GRADIENTS: [string, string][] = [
  ['#F59E0B', '#EF4444'], ['#10B981', '#3B82F6'], ['#8B5CF6', '#EC4899'], ['#64748B', '#334155'], ['#06B6D4', '#6366F1'],
]
/** 预览缩略图：用渐变 SVG 代替真实画面（仅预览，真实运行走 MediaService.Thumbnail） */
function previewThumb(path: string): string {
  let h = 0
  for (const ch of baseName(path)) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  const [a, b] = GRADIENTS[h % GRADIENTS.length]
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="96" height="60"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${a}"/><stop offset="1" stop-color="${b}"/></linearGradient></defs><rect width="96" height="60" fill="url(#g)"/></svg>`
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
}
