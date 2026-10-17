/**
 * 对话附件的预览种类。图片和视频由页面直接加载缩略图，文档点开后再读。
 */
import { callService } from '@/api/call'
import { buildSrcdoc } from '@/utils/docSafeHtml'
import { t } from '@/i18n'

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
  | 'text'
  | 'csv'
  | 'file'

export interface CatShownFile {
  name: string
  path?: string
  kind: CatPreviewKind
  mime?: string
  /** 刷新后的图片数据，已经是缩小过的 */
  data?: string
}

const KINDS = new Set<CatPreviewKind>(['image', 'video', 'audio', 'pdf', 'docx', 'xlsx', 'pptx', 'markdown', 'html', 'svg', 'text', 'csv', 'file'])

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
  mdown: 'markdown',
  mkd: 'markdown',
  mdx: 'markdown',
  txt: 'text',
  text: 'text',
  log: 'text',
  csv: 'csv',
  tsv: 'csv',
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
  text = stripInternalInstructions(text)
  if (!text) return ''
  const hide = new Set<string>()
  for (const f of files ?? []) {
    if (f.path) hide.add(normPath(f.path))
    if (f.kind === 'image' && (f.data || f.path)) hide.add(t('cat.media.image'))
  }
  if (!hide.size) return text
  const lines = text.split('\n').filter((line) => {
    const t = line.trim()
    if (!t) return true
    return !hide.has(t) && !hide.has(normPath(t))
  })
  return lines.join('\n').trim()
}

/** Compatibility for already loaded CLI history; copy/search use the same text. */
export function stripInternalInstructions(text: string): string {
  return text
    .replace(/<ffc_internal_(instructions|file_preview)>[\s\S]*?<\/ffc_internal_\1>/g, '')
    .replace(/ffc app file preview: When the user asks to open\/show a workspace file or you produce an Office artifact,[\s\S]*?Do not include the capability token in your reply or save it in project files\./g, '')
    .trim()
}

function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

/** 纯文本预览。内容放进沙箱页面，不执行。 */
export function plainDocumentHtml(name: string, body: string, dark: boolean): string {
  return buildSrcdoc(`<pre>${esc(body)}</pre>`, { dark, title: name })
}

function splitRow(line: string, sep: string): string[] {
  const out: string[] = []
  let cur = ''
  let quoted = false
  for (let i = 0; i < line.length; i++) {
    const c = line[i]
    if (quoted) {
      if (c === '"') {
        if (line[i + 1] === '"') {
          cur += '"'
          i++
        } else quoted = false
      } else cur += c
      continue
    }
    if (c === '"') {
      quoted = true
      continue
    }
    if (c === sep) {
      out.push(cur)
      cur = ''
      continue
    }
    cur += c
  }
  out.push(cur)
  return out
}

/** 逗号或制表符表格。最多 200 行、30 列。 */
export function tableDocumentHtml(name: string, body: string, sep: string, dark: boolean): string {
  const lines = body.replace(/^\uFEFF/, '').split(/\r?\n/)
  while (lines.length && lines[lines.length - 1] === '') lines.pop()
  const cut = lines.length > 200 || lines.some((line) => splitRow(line, sep).length > 30)
  const rows = lines.slice(0, 200).map((line) => splitRow(line, sep).slice(0, 30))
  const html = rows
    .map((row) => `<tr>${row.map((cell) => `<td>${esc(cell)}</td>`).join('')}</tr>`)
    .join('')
  const note = cut ? `<p>${t('cat.media.textPreviewNote')}</p>` : ''
  return buildSrcdoc(`<table>${html}</table>${note}`, { dark, title: name })
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

const blobCache = new Map<string, string>()

function mimeOf(path: string): string {
  const name = path.replace(/\\/g, '/').split('/').pop() ?? ''
  const kind = previewKind(name)
  if (kind === 'svg') return 'image/svg+xml'
  if (name.toLowerCase().endsWith('.jpg') || name.toLowerCase().endsWith('.jpeg')) return 'image/jpeg'
  if (name.toLowerCase().endsWith('.gif')) return 'image/gif'
  if (name.toLowerCase().endsWith('.webp')) return 'image/webp'
  if (name.toLowerCase().endsWith('.bmp')) return 'image/bmp'
  return 'image/png'
}

/** 直接读本机图片。开发服务不转发 /local 时，输入框用这个地址显示。 */
export async function catMediaObjectURL(path: string): Promise<string> {
  const key = normPath(path)
  const cached = blobCache.get(key)
  if (cached) return cached
  try {
    const b64 = await callService<string>('CatService', 'ReadCatMedia', path)
    if (!b64) return ''
    const bin = atob(b64)
    const bytes = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
    const url = URL.createObjectURL(new Blob([bytes], { type: mimeOf(path) }))
    blobCache.set(key, url)
    return url
  } catch {
    return ''
  }
}

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
