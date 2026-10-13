/**
 * 对话附件的预览种类。图片和视频由页面直接加载缩略图，文档点开后再读。
 */
import { callService } from '@/api/call'

export type CatPreviewKind =
  | 'image'
  | 'video'
  | 'audio'
  | 'pdf'
  | 'docx'
  | 'xlsx'
  | 'pptx'
  | 'markdown'
  | 'html'
  | 'svg'
  | 'file'

export interface CatShownFile {
  name: string
  path?: string
  kind: CatPreviewKind
  mime?: string
  /** 刷新后的图片数据，已经是缩小过的 */
  data?: string
}

const KINDS = new Set<CatPreviewKind>(['image', 'video', 'audio', 'pdf', 'docx', 'xlsx', 'pptx', 'markdown', 'html', 'svg', 'file'])

const EXT: Record<string, CatPreviewKind> = {
  png: 'image',
  jpg: 'image',
  jpeg: 'image',
  gif: 'image',
  webp: 'image',
  bmp: 'image',
  ico: 'image',
  avif: 'image',
  mp4: 'video',
  m4v: 'video',
  webm: 'video',
  mov: 'video',
  mkv: 'video',
  avi: 'video',
  mp3: 'audio',
  wav: 'audio',
  m4a: 'audio',
  ogg: 'audio',
  opus: 'audio',
  flac: 'audio',
  weba: 'audio',
  pdf: 'pdf',
  docx: 'docx',
  xlsx: 'xlsx',
  xlsm: 'xlsx',
  xls: 'xlsx',
  pptx: 'pptx',
  md: 'markdown',
  markdown: 'markdown',
  html: 'html',
  htm: 'html',
  svg: 'svg',
}

export function previewKind(name: string): CatPreviewKind {
  const i = name.lastIndexOf('.')
  if (i < 0) return 'file'
  return EXT[name.slice(i + 1).toLowerCase()] ?? 'file'
}

export function asPreviewKind(kind: string): CatPreviewKind {
  return KINDS.has(kind as CatPreviewKind) ? (kind as CatPreviewKind) : 'file'
}

export function canPreview(kind: CatPreviewKind): boolean {
  return kind !== 'file'
}

export function showsThumb(kind: CatPreviewKind): boolean {
  return kind === 'image' || kind === 'video'
}

function normPath(p: string): string {
  return p.replace(/\//g, '\\').replace(/\\+$/g, '').toLowerCase()
}

/** 缩略图已经画出的路径和「图片」占位，不再在气泡里重复一行。 */
export function userDisplayText(text: string, files: readonly Pick<CatShownFile, 'path' | 'kind' | 'data'>[] | undefined): string {
  if (!text) return ''
  const hide = new Set<string>()
  for (const f of files ?? []) {
    if (f.path) hide.add(normPath(f.path))
    if (f.kind === 'image' && (f.data || f.path)) hide.add('图片')
  }
  if (!hide.size) return text
  const lines = text.split('\n').filter((line) => {
    const t = line.trim()
    if (!t) return true
    return !hide.has(t) && !hide.has(normPath(t))
  })
  return lines.join('\n').trim()
}

export function imageDataUrl(file: Pick<CatShownFile, 'mime' | 'data'>): string {
  const data = (file.data ?? '').replace(/\s/g, '')
  if (!data || data.length > 300_000 || !/^[A-Za-z0-9+/=]+$/.test(data)) return ''
  return `data:${file.mime || 'image/png'};base64,${data}`
}

export interface CatMediaURL {
  url: string
  mime: string
  kind: CatPreviewKind
  name: string
}

const urlCache = new Map<string, CatMediaURL>()

/** 向本机要一个页面能加载的地址。同一次运行里同一路径复用。 */
export async function catMediaURL(path: string): Promise<CatMediaURL | null> {
  const key = normPath(path)
  const hit = urlCache.get(key)
  if (hit) return hit
  try {
    const raw = await callService<unknown>('CatService', 'PreviewCatMedia', path)
    const r = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {}
    const url = typeof r.url === 'string' ? r.url : ''
    if (!url.startsWith('/local/')) return null
    const got: CatMediaURL = {
      url,
      mime: typeof r.mime === 'string' ? r.mime : '',
      kind: asPreviewKind(typeof r.kind === 'string' ? r.kind : ''),
      name: typeof r.name === 'string' ? r.name : '',
    }
    urlCache.set(key, got)
    return got
  } catch {
    return null
  }
}

const TEXT_CAP = 1024 * 1024
const OFFICE_CAP = 8 * 1024 * 1024

export async function catMediaText(path: string): Promise<string | 'large' | null> {
  const hit = await catMediaURL(path)
  if (!hit) return null
  try {
    const res = await fetch(hit.url)
    if (res.status === 413) return 'large'
    if (!res.ok) return null
    const len = Number(res.headers.get('content-length') || 0)
    if (len > TEXT_CAP) return 'large'
    const text = await res.text()
    return text.length > TEXT_CAP ? 'large' : text
  } catch {
    return null
  }
}

export async function catMediaBase64(path: string): Promise<{ b64: string; mime: string } | 'large' | null> {
  const hit = await catMediaURL(path)
  if (!hit) return null
  try {
    const res = await fetch(hit.url)
    if (res.status === 413) return 'large'
    if (!res.ok) return null
    const len = Number(res.headers.get('content-length') || 0)
    if (len > OFFICE_CAP) return 'large'
    const buf = new Uint8Array(await res.arrayBuffer())
    if (buf.byteLength > OFFICE_CAP) return 'large'
    return { b64: toBase64(buf), mime: hit.mime }
  } catch {
    return null
  }
}

function toBase64(bytes: Uint8Array): string {
  let s = ''
  const step = 0x8000
  for (let i = 0; i < bytes.length; i += step) {
    s += String.fromCharCode(...bytes.subarray(i, i + step))
  }
  return btoa(s)
}
