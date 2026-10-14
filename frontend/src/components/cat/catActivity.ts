/**
 * 中间消息里，思考 / 工具调用 / 文件查找怎么收起。
 * 挨在一起的工具调用和搜索收成一组；挨在一起的新增和编辑收成一组。
 * 助手正文原样留下。文件改动默认展开，工具调用默认收起。
 * 转圈只留给思考。搜索、读取、编辑直接显示，不转圈。
 */
import type { IconName } from '@/components/icon/icons'
import type { CatBlock } from '@/api/catMock'
import { userSendFailed } from '@/views/cat/catPending'
import { t } from '@/i18n'

export interface CatFold {
  key: string
  tone: 'think' | 'tool' | 'change'
  title: string
  /** 点开后才显示；空字符串表示这一行没有可展开的内容 */
  body: string
  streaming: boolean
  defaultOpen: boolean
}

export type CatRow =
  | { kind: 'block'; key: string; block: CatBlock }
  | { kind: 'fold'; key: string; fold: CatFold }

interface ToolItem {
  summary: string
  detail: string
  fileChange: boolean
}

const FILE_CHANGE_NAME = /^(?:edit(?:ed|ing)?|writ(?:e(?:_file)?|ing|ten)|wrote|search_replace|apply(?:_|\s+)patch|str_?replace|hashline(?:_edit)?|patch(?:ed|ing)?|diff|modif(?:y|ied|ying))\b/i
const FILE_CHANGE_ZH = /改动|修改|写入|改写/
const LOOKUP_NAME = /^(?:read(?:_file)?|list(?:_dir|dir)?|search|grep|find|glob)\b/i
const LOOKUP_ZH = /读取|搜索|查找|列出/

/** 这一行是不是「改了文件」（要展开），而不是查 / 读 / 列 */
export function isFileChangeLabel(summary: string): boolean {
  const s = summary.trim()
  return FILE_CHANGE_NAME.test(s) || FILE_CHANGE_ZH.test(s)
}

export function isLookupLabel(summary: string): boolean {
  const s = summary.trim()
  if (isFileChangeLabel(s)) return false
  return LOOKUP_NAME.test(s) || LOOKUP_ZH.test(s)
}

/** `- Read: a.go` / `### Search` 才是新的一条工具；diff 里的 `- old` 不是 */
function isToolLine(trimmed: string): boolean {
  if (/^#{3,4}\s+\S/.test(trimmed)) return true
  const m = trimmed.match(/^(?:[-*•]|\d+[.)])\s+(.+)$/)
  if (!m) return false
  const rest = m[1]
  if (/^\S{1,40}\s*[：:]/.test(rest)) return true
  if (/^[A-Za-z][\w.-]{0,32}$/.test(rest)) return true
  if (/^(?:读取|搜索|查找|列出|修改|写入)/.test(rest)) return true
  return false
}

function stripMarker(trimmed: string): string {
  return trimmed.replace(/^(?:[-*•]|\d+[.)])\s+/, '').replace(/^#{3,4}\s+/, '').trim()
}

/** `Read: a.go` 这种单行，把路径拆到下拉里；`Search: foo` 这种查询留在标题上 */
function peelToolPath(summary: string, detail: string): { summary: string; detail: string } {
  if (detail) return { summary, detail }
  const m = summary.match(/^(\S.{0,40}?)\s*[：:]\s*(\S.+)$/)
  if (!m) return { summary, detail }
  const head = m[1].trim()
  const rest = m[2].trim()
  if (!/[/\\]/.test(rest) && !/\.[A-Za-z0-9]{1,8}$/.test(rest)) return { summary, detail }
  return { summary: head, detail: rest }
}

/** 把导出的一整段工具，或实时的一行标签，拆成一条一条 */
export function splitToolItems(text: string): ToolItem[] {
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  const groups: string[][] = []
  for (const raw of lines) {
    const trimmed = raw.trim()
    if (!trimmed) {
      if (groups.length) groups[groups.length - 1].push('')
      continue
    }
    if (isToolLine(trimmed) || !groups.length) groups.push([stripMarker(trimmed)])
    else groups[groups.length - 1].push(trimmed)
  }
  const items: ToolItem[] = []
  for (const group of groups) {
    const summary0 = (group[0] ?? '').trim()
    const detail0 = group.slice(1).join('\n').trim()
    if (!summary0 && !detail0) continue
    const peeled = peelToolPath(summary0 || detail0.split('\n')[0], summary0 ? detail0 : '')
    items.push({
      summary: peeled.summary,
      detail: peeled.detail,
      fileChange: isFileChangeLabel(peeled.summary || summary0),
    })
  }
  return items
}

function blockKey(b: CatBlock, i: number): string {
  if (b.kind === 'a' && b.id) return `a:${b.id}`
  if (b.kind === 'user') return `user:${i}`
  return `${b.kind}:${i}`
}

/** 思考：短状态（「思考完成 · 6 秒」）就一行；真正的思考正文收在「思考」下面 */
function thinkFold(b: Extract<CatBlock, { kind: 'think' }>, i: number): CatRow {
  const text = b.text.trim()
  const streaming = !!b.streaming
  const status = !streaming && !!text && !text.includes('\n') && [...text].length <= 28 && text.startsWith(t('cat.misc.s_21d68b2de0'))
  const fold: CatFold = {
    key: `think:${b.id ?? i}`,
    tone: 'think',
    title: status ? text : t('cat.misc.s_21d68b2de0'),
    body: status ? '' : text,
    streaming,
    defaultOpen: false,
  }
  return { kind: 'fold', key: fold.key, fold }
}

interface HeldTool {
  key: string
  item: ToolItem
}

function toolTitle(item: ToolItem): string {
  const lines = item.detail.split('\n').map((s) => s.trim()).filter(Boolean)
  const countFiles = lines.length > 1 && /^(?:read(?:_file)?|读取)\b/i.test(item.summary)
  return countFiles ? `${item.summary} · ${lines.length}` : item.summary || t('cat.misc.s_20dce2c6fa')
}

function itemText(item: ToolItem): string {
  const summary = item.summary.trim()
  const detail = item.detail.trim()
  if (summary && detail) return `${summary}\n${detail}`
  return summary || detail
}

function pushToolGroup(rows: CatRow[], held: HeldTool[]) {
  if (!held.length) return
  const key = held.length === 1 ? held[0].key : `tools:${held[0].key}`
  const fold: CatFold = {
    key,
    tone: 'tool',
    title: held.length === 1 ? toolTitle(held[0].item) : t('cat.misc.s_944f6814a5', { n: held.length }),
    body: held.length === 1 ? held[0].item.detail : held.map((h) => itemText(h.item)).join('\n\n'),
    streaming: false,
    defaultOpen: false,
  }
  rows.push({ kind: 'fold', key, fold })
  held.length = 0
}

function pushChangeGroup(rows: CatRow[], held: HeldTool[]) {
  if (!held.length) return
  const key = held.length === 1 ? held[0].key : `changes:${held[0].key}`
  const fold: CatFold = {
    key,
    tone: 'change',
    title: held.length === 1 ? held[0].item.summary || t('cat.misc.s_5b8ad40944') : t('cat.misc.s_9cc7373d85', { n: held.length }),
    body: held.length === 1 ? held[0].item.detail : held.map((h) => itemText(h.item)).join('\n\n'),
    streaming: false,
    defaultOpen: true,
  }
  rows.push({ kind: 'fold', key, fold })
  held.length = 0
}

/**
 * 连续的工具调用和搜索合成一组，连续的新增和编辑合成一组。
 * 中间插了正文或另一种操作，就另起一组。查找默认收起，文件改动默认展开。
 */
export function presentCatBlocks(blocks: CatBlock[]): CatRow[] {
  const rows: CatRow[] = []
  const tools: HeldTool[] = []
  const changes: HeldTool[] = []
  const flush = () => {
    pushToolGroup(rows, tools)
    pushChangeGroup(rows, changes)
  }
  blocks.forEach((b, i) => {
    if (b.kind === 'run') {
      splitToolItems(b.text).forEach((item, j) => {
        const key = `${b.id ?? `i${i}`}:${j}`
        if (item.fileChange) {
          pushToolGroup(rows, tools)
          changes.push({ key, item })
        } else {
          pushChangeGroup(rows, changes)
          tools.push({ key, item })
        }
      })
      return
    }
    flush()
    if (b.kind === 'think') rows.push(thinkFold(b, i))
    else if (b.kind === 'user') {
      const block = userSendFailed(blocks, i) ? { ...b, failed: true } : b
      rows.push({ kind: 'block', key: blockKey(b, i), block })
    } else rows.push({ kind: 'block', key: blockKey(b, i), block: b })
  })
  flush()
  return rows
}

export function foldIcon(fold: Pick<CatFold, 'tone' | 'title'>): IconName {
  if (fold.tone === 'think') return 'think'
  if (fold.tone === 'change') return 'edit'
  if (/^(?:read(?:_file)?|读取)\b/i.test(fold.title)) return 'file'
  if (/^(?:list(?:_dir|dir)?|列出)\b/i.test(fold.title)) return 'folder'
  if (fold.title.startsWith(t('cat.misc.s_d59385523a')) || isLookupLabel(fold.title)) return 'search'
  return 'tool'
}

/** 文件改动的多行内容用等宽，普通一句话不用 */
export function foldBodyMono(fold: Pick<CatFold, 'tone' | 'body'>): boolean {
  return fold.tone === 'change' && (fold.body.includes('\n') || /^(?:\+\+\+|---|@@)/.test(fold.body))
}

/** 对比行：+ 绿色新增，- 红色删掉，其余是原文或文件名 */
export function diffLineKind(line: string): 'add' | 'del' | 'same' {
  if (line.startsWith('+') && !line.startsWith('+++')) return 'add'
  if (line.startsWith('-') && !line.startsWith('---')) return 'del'
  return 'same'
}
