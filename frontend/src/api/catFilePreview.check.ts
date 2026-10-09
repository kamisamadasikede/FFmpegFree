// Cat 侧边预览的种类映射：文本留下内容，二进制只留字节，未知种类不当成可编辑文本。
import { mapFileBody, mockCatFileBody, previewModeFor } from './catFilePreview'

type Eq = (name: string, got: unknown, want: unknown) => void

const empty = {
  relPath: '',
  name: '',
  kind: 'binary',
  size: 0,
  content: '',
  dataBase64: '',
  mime: '',
  language: '',
  editable: false,
  modTime: 0,
}

export function catFilePreviewChecks(eq: Eq): void {
  eq('预览映射：空和缺字段', mapFileBody(null), empty)
  eq(
    '预览映射：文本可编辑',
    mapFileBody({ relPath: 'a.go', name: 'a.go', kind: 'text', size: 2, content: 'hi', language: 'go', editable: true, modTime: 3 }),
    { relPath: 'a.go', name: 'a.go', kind: 'text', size: 2, content: 'hi', dataBase64: '', mime: '', language: 'go', editable: true, modTime: 3 },
  )
  eq(
    '预览映射：图片只留字节',
    mapFileBody({ kind: 'image', name: 'a.png', relPath: 'a.png', dataBase64: 'aaa', content: 'nope', editable: true, mime: 'image/png', size: 4 }),
    { relPath: 'a.png', name: 'a.png', kind: 'image', size: 4, content: '', dataBase64: 'aaa', mime: 'image/png', language: '', editable: false, modTime: 0 },
  )
  eq(
    '预览映射：未知种类不当文本',
    mapFileBody({ kind: 'exe', name: 'a.bin', relPath: 'a.bin', content: 'x', editable: true, dataBase64: 'aaa' }),
    { ...empty, name: 'a.bin', relPath: 'a.bin' },
  )
  eq('预览切换：markdown / svg / 其他', [previewModeFor('A.MD'), previewModeFor('readme.markdown'), previewModeFor('icon.SVG'), previewModeFor('main.go')], ['markdown', 'markdown', 'svg', 'code'])
  eq('模拟预览是可编辑文本', [mockCatFileBody('main.rs').kind, mockCatFileBody('main.rs').editable, mockCatFileBody('说明.md').language], ['text', true, 'markdown'])
}
