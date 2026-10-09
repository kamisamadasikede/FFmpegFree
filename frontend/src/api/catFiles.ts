/**
 * Cat 右侧文件面板（契约 v0.32 §6.19.11）。
 * 真后端：ListCatFiles 只列一层，展开再要下一层；模拟数据只在没有 window.go 时由面板自己用。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { call, toAppError } from '@/api/call'

export const CAT_FILES_COPY = {
  empty: '这里还没有文件。',
  truncated: '只显示前 500 项。',
  failed: '文件列表加载失败，请重试。',
  loading: '正在加载…',
  search: '按文件名搜索',
  open: '打开文件夹',
  refresh: '刷新',
  noMatch: '没有匹配的文件。',
  retry: '重试',
} as const

export interface CatFileEntry {
  name: string
  relPath: string
  isDir: boolean
  size: number
  modTime: number
}

export interface CatFileList {
  root: string
  entries: CatFileEntry[]
  truncated: boolean
}

export interface CatFileNode {
  name: string
  relPath: string
  isDir: boolean
  depth: number
  expanded: boolean
  loading: boolean
  truncated: boolean
  children: CatFileNode[] | null
}

export interface CatFileRow {
  key: string
  depth: number
  name: string
  isDir: boolean
  expanded: boolean
  kind: 'entry' | 'note' | 'loading'
}

export function mapFileList(raw: unknown): CatFileList {
  const r = (raw && typeof raw === 'object' ? raw : {}) as { root?: unknown; entries?: unknown; truncated?: unknown }
  const entries = Array.isArray(r.entries) ? r.entries.map(mapFileEntry).filter((e) => e.name) : []
  return { root: typeof r.root === 'string' ? r.root : '', entries, truncated: r.truncated === true }
}

function mapFileEntry(raw: unknown): CatFileEntry {
  const r = (raw && typeof raw === 'object' ? raw : {}) as { name?: unknown; relPath?: unknown; isDir?: unknown; size?: unknown; modTime?: unknown }
  return {
    name: typeof r.name === 'string' ? r.name : '',
    relPath: typeof r.relPath === 'string' ? r.relPath : '',
    isDir: r.isDir === true,
    size: typeof r.size === 'number' ? r.size : 0,
    modTime: typeof r.modTime === 'number' ? r.modTime : 0,
  }
}

export function entriesToNodes(entries: CatFileEntry[], depth: number): CatFileNode[] {
  return entries.map((e) => ({
    name: e.name,
    relPath: e.relPath,
    isDir: e.isDir,
    depth,
    expanded: false,
    loading: false,
    truncated: false,
    children: null,
  }))
}

/** 把已加载的树摊成行。搜索只看已加载的名字，不递归去要未加载的层；匹配的文件会带上已展开的上级。 */
export function flattenCatFiles(nodes: CatFileNode[] | null, query: string): CatFileRow[] {
  const out: CatFileRow[] = []
  walk(nodes, query.trim().toLowerCase(), out)
  return out
}

function walk(nodes: CatFileNode[] | null, q: string, out: CatFileRow[]) {
  if (!nodes) return
  for (const n of nodes) {
    const self = !q || n.name.toLowerCase().includes(q)
    const kids: CatFileRow[] = []
    if (n.isDir && n.expanded) walk(n.children, q, kids)
    if (!self && kids.length === 0) continue
    out.push({ key: n.relPath, depth: n.depth, name: n.name, isDir: n.isDir, expanded: n.expanded, kind: 'entry' })
    if (n.isDir && n.expanded && n.loading && n.children == null) {
      out.push({ key: n.relPath + '\nloading', depth: n.depth + 1, name: CAT_FILES_COPY.loading, isDir: false, expanded: false, kind: 'loading' })
    }
    out.push(...kids)
    if (n.isDir && n.expanded && n.truncated) {
      out.push({ key: n.relPath + '\ncut', depth: n.depth + 1, name: CAT_FILES_COPY.truncated, isDir: false, expanded: false, kind: 'note' })
    }
  }
}

export function collectExpanded(nodes: CatFileNode[] | null, out: string[] = []): string[] {
  if (!nodes) return out
  for (const n of nodes) {
    if (n.isDir && n.expanded) {
      out.push(n.relPath)
      collectExpanded(n.children, out)
    }
  }
  return out
}

export async function listCatFiles(convId: string, relPath = ''): Promise<CatFileList> {
  const raw = await call(CatBinding.ListCatFiles(cat.ListFilesRequest.createFrom({ convId, relPath })))
  return mapFileList(raw)
}

export async function revealCatConversationFolder(convId: string): Promise<void> {
  await call(CatBinding.RevealCatConversationFolder(cat.RevealConversationFolderRequest.createFrom({ convId })))
}

export function catFilesErrorText(e: unknown): string {
  return toAppError(e).code === 'CAT_PROJECT_MISSING' ? '项目文件夹不见了。' : CAT_FILES_COPY.failed
}
