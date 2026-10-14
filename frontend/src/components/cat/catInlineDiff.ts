
import { t } from '@/i18n'/** 把上一版的区别嵌进当前文件：新增绿色，删掉的行留在原处。没有区别时返回空，继续用编辑器。 */
export interface InlineDiffLine {
  kind: 'add' | 'del' | 'same'
  text: string
}

const omitted = t('cat.misc.s_61324cd5f4')

function splitLines(text: string): string[] {
  if (!text) return []
  const lines = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  if (lines[lines.length - 1] === '') lines.pop()
  return lines
}

function paintLine(line: string): InlineDiffLine {
  if (line === omitted) return { kind: 'same', text: line }
  if (line.startsWith('+')) return { kind: 'add', text: line.slice(1) }
  if (line.startsWith('-')) return { kind: 'del', text: line.slice(1) }
  if (line.startsWith(' ')) return { kind: 'same', text: line.slice(1) }
  return { kind: 'same', text: line }
}

function hunkStart(line: string): number | null {
  const m = /^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line)
  return m ? Number(m[1]) : null
}

function keep(rows: InlineDiffLine[]): InlineDiffLine[] {
  return rows.some((row) => row.kind === 'add' || row.kind === 'del') ? rows : []
}

export interface EditorDiffMarks {
  added: number[]
  /** 当前正文里整行标红（文件已经删掉、编辑器里放的是旧内容） */
  removed: number[]
  deleted: { after: number; lines: string[] }[]
}

interface DiffOp {
  type: 'eq' | 'ins' | 'del'
  text: string
}

function fileLines(text: string): string[] {
  if (!text) return []
  const lines = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  if (lines[lines.length - 1] === '') lines.pop()
  return lines
}

function lcsDiff(a: string[], b: string[]): DiffOp[] {
  const n = a.length
  const m = b.length
  const dp: number[][] = Array.from({ length: n + 1 }, () => Array(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    const row = dp[i]
    const next = dp[i + 1]
    for (let j = m - 1; j >= 0; j--) {
      row[j] = a[i] === b[j] ? next[j + 1] + 1 : Math.max(next[j], row[j + 1])
    }
  }
  const ops: DiffOp[] = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      ops.push({ type: 'eq', text: a[i] })
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      ops.push({ type: 'del', text: a[i] })
      i++
    } else {
      ops.push({ type: 'ins', text: b[j] })
      j++
    }
  }
  while (i < n) ops.push({ type: 'del', text: a[i++] })
  while (j < m) ops.push({ type: 'ins', text: b[j++] })
  return ops
}

function diffOps(a: string[], b: string[]): DiffOp[] {
  let start = 0
  while (start < a.length && start < b.length && a[start] === b[start]) start++
  let aEnd = a.length
  let bEnd = b.length
  while (aEnd > start && bEnd > start && a[aEnd - 1] === b[bEnd - 1]) {
    aEnd--
    bEnd--
  }
  const ops: DiffOp[] = []
  for (let i = 0; i < start; i++) ops.push({ type: 'eq', text: a[i] })
  const aMid = a.slice(start, aEnd)
  const bMid = b.slice(start, bEnd)
  if (aMid.length <= 400 && bMid.length <= 400) ops.push(...lcsDiff(aMid, bMid))
  else {
    for (const text of aMid) ops.push({ type: 'del', text })
    for (const text of bMid) ops.push({ type: 'ins', text })
  }
  for (let i = aEnd; i < a.length; i++) ops.push({ type: 'eq', text: a[i] })
  return ops
}

/** 相对上一版，当前编辑器里哪些行是新增，删掉的行插在哪一行后面。 */
export function editorDiffMarks(original: string, current: string): EditorDiffMarks {
  const added: number[] = []
  const deleted: { after: number; lines: string[] }[] = []
  let line = 0
  let group: string[] | null = null
  let groupAfter = 0
  const flush = () => {
    if (group && group.length) deleted.push({ after: groupAfter, lines: group })
    group = null
  }
  for (const op of diffOps(fileLines(original), fileLines(current))) {
    if (op.type === 'del') {
      if (!group) {
        group = []
        groupAfter = line
      }
      group.push(op.text)
      continue
    }
    flush()
    line++
    if (op.type === 'ins') added.push(line)
  }
  flush()
  return { added, removed: [], deleted }
}

/** 整份旧文件都标成删掉，行还留在编辑器里。 */
export function removedFileMarks(text: string): EditorDiffMarks {
  return { added: [], removed: fileLines(text).map((_, i) => i + 1), deleted: [] }
}

/** 给编辑器的上一版和当前正文。截断过的对比对不齐，交给普通编辑器。 */
export function diffSides(fileText: string, diffText: string): { original: string; modified: string } | null {
  const rows = inlineFileDiff(fileText, diffText)
  if (!rows.length || rows.some((row) => row.text === omitted)) return null
  const nl = fileText.includes('\r\n') ? '\r\n' : '\n'
  const trailing = fileText.endsWith('\n')
  const join = (lines: string[]) => {
    if (!lines.length) return ''
    const body = lines.join(nl)
    return trailing ? body + nl : body
  }
  const original = join(rows.filter((row) => row.kind !== 'add').map((row) => row.text))
  const modified = fileText !== '' ? fileText : join(rows.filter((row) => row.kind !== 'del').map((row) => row.text))
  return { original, modified }
}

export function inlineFileDiff(fileText: string, diffText: string): InlineDiffLine[] {
  const raw = splitLines(diffText)
  if (!raw.length) return []
  if (!raw.some((line) => line.startsWith('@@'))) return keep(raw.map(paintLine))

  const fileLines = splitLines(fileText)
  const out: InlineDiffLine[] = []
  let cursor = 0
  let i = 0
  let stop = false
  while (i < raw.length && !stop) {
    const startNo = hunkStart(raw[i])
    if (startNo == null) {
      if (raw[i] === omitted) {
        out.push({ kind: 'same', text: omitted })
        break
      }
      i++
      continue
    }
    const start = startNo <= 0 ? cursor : startNo - 1
    while (cursor < start && cursor < fileLines.length) out.push({ kind: 'same', text: fileLines[cursor++] })
    i++
    while (i < raw.length && hunkStart(raw[i]) == null) {
      const line = raw[i++]
      if (line === omitted) {
        out.push({ kind: 'same', text: line })
        stop = true
        break
      }
      const painted = paintLine(line)
      if (painted.kind === 'same' && !line.startsWith(' ')) continue
      out.push(painted)
      if (painted.kind !== 'del') cursor++
    }
  }
  while (cursor < fileLines.length) out.push({ kind: 'same', text: fileLines[cursor++] })
  return keep(out)
}
