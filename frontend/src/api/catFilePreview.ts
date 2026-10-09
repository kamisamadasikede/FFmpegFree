/**
 * Cat 侧边文件预览。树只提交路径；这里按种类打开查看，文本可以改完再写回。
 * 浏览器走查不调用后端，用 mockCatFileBody。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { call, toAppError } from '@/api/call'

export const CAT_PREVIEW_COPY = {
  loading: '正在读取…',
  failed: '文件读取失败，请重试。',
  saveFailed: '文件保存失败，请重试。',
  saved: '已保存。',
  save: '保存',
  saving: '正在保存…',
  close: '关闭',
  binary: '这个文件不能在这里查看。',
  tooLarge: '这个文件太大，这里放不下。',
  doc: '这个旧版文档不能在这里打开。',
  preview: '预览',
  source: '源码',
  unsaved: '还有未保存的修改。',
  discard: '放弃',
} as const

const KINDS = ['text', 'image', 'media', 'pdf', 'docx', 'xlsx', 'pptx', 'doc', 'binary', 'tooLarge'] as const

export type CatFileKind = (typeof KINDS)[number]

export interface CatFileBody {
  relPath: string
  name: string
  kind: CatFileKind
  size: number
  content: string
  dataBase64: string
  mime: string
  language: string
  editable: boolean
  modTime: number
}

const BYTE_KINDS = new Set<CatFileKind>(['image', 'media', 'pdf', 'docx', 'xlsx', 'pptx'])

export function mapFileBody(raw: unknown): CatFileBody {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const kind: CatFileKind = typeof r.kind === 'string' && (KINDS as readonly string[]).includes(r.kind) ? (r.kind as CatFileKind) : 'binary'
  const text = kind === 'text'
  return {
    relPath: typeof r.relPath === 'string' ? r.relPath : '',
    name: typeof r.name === 'string' ? r.name : '',
    kind,
    size: typeof r.size === 'number' ? r.size : 0,
    content: text && typeof r.content === 'string' ? r.content : '',
    dataBase64: BYTE_KINDS.has(kind) && typeof r.dataBase64 === 'string' ? r.dataBase64 : '',
    mime: typeof r.mime === 'string' ? r.mime : '',
    language: text ? (typeof r.language === 'string' && r.language ? r.language : 'plaintext') : '',
    editable: text && r.editable === true,
    modTime: typeof r.modTime === 'number' ? r.modTime : 0,
  }
}

/** markdown 和 svg 先给预览，源码仍是同一份文本。其余文本直接进编辑器。 */
export function previewModeFor(name: string): 'markdown' | 'svg' | 'code' {
  const n = name.toLowerCase()
  if (n.endsWith('.md') || n.endsWith('.markdown')) return 'markdown'
  if (n.endsWith('.svg')) return 'svg'
  return 'code'
}

export function mockCatFileBody(name: string): CatFileBody {
  const mode = previewModeFor(name)
  const content = mode === 'markdown' ? `# ${name}\n\n这是预览。\n` : `${name}\n`
  return {
    relPath: name,
    name,
    kind: 'text',
    size: content.length,
    content,
    dataBase64: '',
    mime: mode === 'svg' ? 'image/svg+xml' : 'text/plain',
    language: mode === 'markdown' ? 'markdown' : mode === 'svg' ? 'xml' : 'plaintext',
    editable: true,
    modTime: 0,
  }
}

export async function readCatFile(convId: string, relPath: string): Promise<CatFileBody> {
  const raw = await call(CatBinding.ReadCatFile(cat.ReadFileRequest.createFrom({ convId, relPath })))
  return mapFileBody(raw)
}

export async function writeCatFile(convId: string, relPath: string, content: string): Promise<{ relPath: string; size: number; modTime: number }> {
  const raw = await call(CatBinding.WriteCatFile(cat.WriteFileRequest.createFrom({ convId, relPath, content })))
  const r = (raw && typeof raw === 'object' ? raw : {}) as { relPath?: unknown; size?: unknown; modTime?: unknown }
  return {
    relPath: typeof r.relPath === 'string' ? r.relPath : relPath,
    size: typeof r.size === 'number' ? r.size : 0,
    modTime: typeof r.modTime === 'number' ? r.modTime : 0,
  }
}

const KNOWN_TEXT = new Set([
  '这个文件太大，不能在这里修改。',
  '这个文件不能在这里修改。',
  '只能保存文本。',
  '找不到这个文件。',
  '这是一个文件夹。',
  '这个路径不能查看。',
])

export function catPreviewErrorText(e: unknown, fallback: string): string {
  const err = toAppError(e)
  if (err.code === 'CAT_PROJECT_MISSING') return '项目文件夹不见了。'
  if (KNOWN_TEXT.has(err.message)) return err.message
  return fallback
}
