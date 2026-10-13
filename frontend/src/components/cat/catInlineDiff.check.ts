import { diffSides, editorDiffMarks, inlineFileDiff, removedFileMarks } from './catInlineDiff'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catInlineDiffChecks(eq: Eq): void {
  eq(
    '文件里标出新增，改动前面的原文还在',
    inlineFileDiff('a\nb\nc\n', '@@ -2 +2,2 @@\n b\n+x\n c'),
    [
      { kind: 'same', text: 'a' },
      { kind: 'same', text: 'b' },
      { kind: 'add', text: 'x' },
      { kind: 'same', text: 'c' },
    ],
  )
  eq(
    '删掉的行插在原来的位置',
    inlineFileDiff('a\nd\n', '@@ -1,3 +1,2 @@\n a\n-b\n-c\n d'),
    [
      { kind: 'same', text: 'a' },
      { kind: 'del', text: 'b' },
      { kind: 'del', text: 'c' },
      { kind: 'same', text: 'd' },
    ],
  )
  eq('新文件整份都是新增', inlineFileDiff('hi\n', '+hi'), [{ kind: 'add', text: 'hi' }])
  eq('没有区别就交给原来的编辑器', inlineFileDiff('a\n', ''), [])
  eq(
    '删掉的文件只剩红行',
    inlineFileDiff('', '@@ -1,2 +0,0 @@\n-old\n-line'),
    [
      { kind: 'del', text: 'old' },
      { kind: 'del', text: 'line' },
    ],
  )
  eq(
    '编辑器拿到上一版和当前文件',
    diffSides('a\nb\nx\nc\n', '@@ -2 +2,2 @@\n b\n+x\n c'),
    { original: 'a\nb\nc\n', modified: 'a\nb\nx\nc\n' },
  )
  eq('新文件相对空文件', diffSides('hi\n', '+hi'), { original: '', modified: 'hi\n' })
  eq('删掉的文件当前是空的', diffSides('', '@@ -1,2 +0,0 @@\n-old\n-line'), { original: 'old\nline', modified: '' })
  eq(
    '编辑器行号：新增在当前文件里',
    editorDiffMarks('a\nb\nc\n', 'a\nb\nx\nc\n'),
    { added: [3], removed: [], deleted: [] },
  )
  eq(
    '编辑器行号：删掉的行留在原处',
    editorDiffMarks('a\nb\nc\nd\n', 'a\nd\n'),
    { added: [], removed: [], deleted: [{ after: 1, lines: ['b', 'c'] }] },
  )
  eq('新文件每一行都是新增', editorDiffMarks('', 'hi\n'), { added: [1], removed: [], deleted: [] })
  eq('没改动就不着色', editorDiffMarks('a\n', 'a\n'), { added: [], removed: [], deleted: [] })
  eq('删掉的整份文件按行标红', removedFileMarks('old\nline\n'), { added: [], removed: [1, 2], deleted: [] })
}
