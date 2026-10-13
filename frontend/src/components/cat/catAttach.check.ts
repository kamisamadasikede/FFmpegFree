import { splitAttach } from './catAttach'
import { previewKind, tableDocumentHtml, userDisplayText } from './catMedia'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catAttachChecks(eq: Eq): void {
  const five = splitAttach([1, 2, 3, 4, 5, 6])
  eq('附件：前四个留在输入框上', five.shown, [1, 2, 3, 4])
  eq('附件：超出的收进角标', five.extra, [5, 6])
  eq('附件：刚好四个不收起', splitAttach([1, 2, 3, 4]).extra, [])
  eq('附件：没有文件就两边都空', splitAttach([]), { shown: [], extra: [] })
  eq('图片和视频能出缩略图', [previewKind('a.PNG'), previewKind('clip.mp4'), previewKind('note.docx'), previewKind('main.go')], ['image', 'video', 'docx', 'file'])
  eq('常见文档能预览', [previewKind('a.md'), previewKind('a.mdown'), previewKind('a.txt'), previewKind('a.csv'), previewKind('a.doc'), previewKind('a.rtf'), previewKind('a.odt')], ['markdown', 'markdown', 'text', 'csv', 'file', 'file', 'file'])
  eq('表格预览转义', tableDocumentHtml('a.csv', '名,值\n<x>,1', ',', false).includes('&lt;x&gt;') && tableDocumentHtml('a.csv', '名,值\n甲,1', ',', false).includes('<table>'), true)
  eq('发出去的图片不重复占位', userDisplayText('看这张\n图片', [{ kind: 'image', data: 'QQ' }]), '看这张')
  eq('只有图片时不留占位文字', userDisplayText('图片', [{ kind: 'image', path: 'D:\\a.png' }]), '')
  eq('视频路径交给缩略图', userDisplayText('看这个\nD:\\clip\\a.mp4', [{ kind: 'video', path: 'D:\\clip\\a.mp4' }]), '看这个')
}
