import { catDraftFor, catDraftKey, restoreCatDraft } from './catDrafts'
import type { PickNote } from '@/components/cat/catPick'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catDraftsChecks(eq: Eq): void {
  const first = catDraftKey('draft-check-a')
  const second = catDraftKey('draft-check-b')
  const a = catDraftFor(first)
  const b = catDraftFor(second)
  a.text = 'first draft'
  a.files = [{ name: 'a.ts', path: 'D:\\src\\a.ts' }]
  b.text = 'second draft'
  eq('草稿：切换会话后正文和附件各自保留', [catDraftFor(first).text, catDraftFor(first).files[0]?.name, catDraftFor(second).text], ['first draft', 'a.ts', 'second draft'])
  eq('草稿：欢迎页按项目隔离，已有空会话使用自己的编号', new Set([catDraftKey('new'), catDraftKey('new', 'p1'), catDraftKey('new', 'p2'), catDraftKey('p1')]).size, 4)
  a.text = ''
  a.files = []
  restoreCatDraft(first, { text: 'rejected send', files: [{ name: 'b.ts', path: 'D:\\src\\b.ts' }], notes: [] })
  eq('草稿：迟到的发送失败恢复原会话，保留当前会话输入', [a.text, a.files[0]?.name, b.text], ['rejected send', 'b.ts', 'second draft'])
  a.text = 'new typing'
  const note: PickNote = { file: 'x.html', tag: 'div', id: '', classes: '', selector: 'div', text: 'hello', styles: '', snippet: '<div>hello</div>', intent: 'edit', note: 'change it' }
  a.notes = [{ ...note }]
  restoreCatDraft(first, { text: 'original send', files: [{ name: 'b.ts', path: 'd:/src/b.ts' }, { name: 'c.ts', path: 'D:\\src\\c.ts' }], notes: [{ ...note }] })
  eq('草稿：被拒绝的正文与新输入都保留，附件和注释去重', [a.text, a.files.map((f) => f.name), a.notes.length], ['original send\n\nnew typing', ['c.ts', 'b.ts'], 1])
  const restored = catDraftFor(first)
  eq('草稿：组件重新挂载拿回同一份草稿', restored === a, true)
  for (const slot of [a, b]) { slot.text = ''; slot.files = []; slot.notes = [] }
}
