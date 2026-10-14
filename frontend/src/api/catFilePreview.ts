/**
 * Cat 侧边文件预览。树只提交路径；这里按种类打开查看，文本改完自动写回。
 * 浏览器走查不调用后端，用 mockCatFileBody。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { call, toAppError } from '@/api/call'
import { t } from '@/i18n'

export const CAT_PREVIEW_COPY = {
  get loading() { return t('cat.preview.loading') },
  get failed() { return t('cat.preview.failed') },
  get saveFailed() { return t('cat.preview.saveFailed') },
  get saved() { return t('cat.preview.saved') },
  get save() { return t('cat.preview.save') },
  get saving() { return t('cat.preview.saving') },
  get close() { return t('cat.preview.close') },
  get binary() { return t('cat.preview.binary') },
  get tooLarge() { return t('cat.preview.tooLarge') },
  get doc() { return t('cat.preview.doc') },
  get preview() { return t('cat.preview.preview') },
  get source() { return t('cat.preview.source') },
  get unsaved() { return t('cat.preview.unsaved') },
  get discard() { return t('cat.preview.discard') },
  get htmlPreview() { return t('cat.preview.htmlPreview') },
  get pick() { return t('cat.preview.pick') },
  get pickTitle() { return t('cat.preview.pickTitle') },
  get pickHint() { return t('cat.preview.pickHint') },
  get pickBlocked() { return t('cat.preview.pickBlocked') },
  get pickPhEdit() { return t('cat.preview.pickPhEdit') },
  get pickPhAsk() { return t('cat.preview.pickPhAsk') },
  get pickIntent() { return t('cat.preview.pickIntent') },
  get pickEdit() { return t('cat.preview.pickEdit') },
  get pickAsk() { return t('cat.preview.pickAsk') },
  get pickCancel() { return t('cat.preview.pickCancel') },
  get pickAdd() { return t('cat.preview.pickAdd') },
}

/** 停下输入这么久就写入，不再放保存按钮 */
export const CAT_AUTOSAVE_MS = 600

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

/** markdown 和 svg 先给预览。html 页默认看源码，点预览图标再在里面渲染。其余文本直接进编辑器。 */
export function previewModeFor(name: string): 'markdown' | 'svg' | 'html' | 'code' {
  const n = name.toLowerCase()
  if (n.endsWith('.md') || n.endsWith('.markdown') || n.endsWith('.mdown') || n.endsWith('.mkd') || n.endsWith('.mdx')) return 'markdown'
  if (n.endsWith('.svg')) return 'svg'
  if (n.endsWith('.html') || n.endsWith('.htm')) return 'html'
  return 'code'
}

export function mockCatFileBody(name: string): CatFileBody {
  const mode = previewModeFor(name)
  const content = mode === 'markdown' ? t('cat.misc.s_acfd18f338', { name: name }) : mode === 'html' ? `<!doctype html><title>${name}</title>\n` : `${name}\n`
  return {
    relPath: name,
    name,
    kind: 'text',
    size: content.length,
    content,
    dataBase64: '',
    mime: mode === 'svg' ? 'image/svg+xml' : mode === 'html' ? 'text/html' : 'text/plain',
    language: mode === 'markdown' ? 'markdown' : mode === 'svg' ? 'xml' : mode === 'html' ? 'html' : 'plaintext',
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
  t('cat.misc.s_3f94464bd7'),
  t('cat.misc.s_4b43160326'),
  t('cat.misc.s_b9cd0adb03'),
  t('cat.misc.s_872b1d2875'),
  t('cat.misc.s_d0a9f9d5cb'),
  t('cat.misc.s_06472dfd62'),
])

export function catPreviewErrorText(e: unknown, fallback: string): string {
  const err = toAppError(e)
  if (err.code === 'CAT_PROJECT_MISSING') return t('cat.misc.s_ed72c09744')
  if (KNOWN_TEXT.has(err.message)) return err.message
  return fallback
}
