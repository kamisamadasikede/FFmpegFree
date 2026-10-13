// 左右栏和文件预览宽度：默认、下限、给对话留空、窗口很窄时再让步。
import { CAT_CHAT_MIN, CAT_PREVIEW_DEFAULT, CAT_PANE_FLOOR, clampPaneWidth, clampPreviewWidth, readPreviewWidth, settleWidths } from './catPreviewWidth'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catPreviewWidthChecks(eq: Eq): void {
  eq('没记过宽度时用默认', readPreviewWidth(null), CAT_PREVIEW_DEFAULT)
  eq('坏记录也回默认', readPreviewWidth('nope'), CAT_PREVIEW_DEFAULT)
  eq('记住的宽度取整', readPreviewWidth('520.4'), 520)

  eq('正常窗口按用户拖的宽度', clampPreviewWidth(480, 1400, 248), 480)
  eq('拖太宽时给对话留 360', clampPreviewWidth(2000, 1400, 248), 1400 - 248 - 236 - 360)
  eq('拖太窄时不低于 280', clampPreviewWidth(100, 1400, 248), 280)
  eq('窗口不够时预览让到剩余空间', clampPreviewWidth(480, 900, 248), 160)
  eq('文件栏跟着树的实际宽度留空', clampPreviewWidth(2000, 1400, 248, 300), 1400 - 248 - 300 - CAT_CHAT_MIN)

  eq('会话栏按拖的宽度', clampPaneWidth(320, 1400, 236 + CAT_CHAT_MIN, 200, 248), 320)
  eq('会话栏拖太宽时给对话留空', clampPaneWidth(900, 1400, 236 + CAT_CHAT_MIN, 200, 248), 1400 - 236 - CAT_CHAT_MIN)
  eq('会话栏拖太窄时不低于 200', clampPaneWidth(80, 1400, 236 + CAT_CHAT_MIN, 200, 248), 200)
  eq('窗口很窄时会话栏收到底', clampPaneWidth(248, 700, 236 + CAT_CHAT_MIN, 200, 248), CAT_PANE_FLOOR)

  eq('空间够时三栏都不动', settleWidths(1400, 248, 236, 480, true), { side: 248, tree: 236, preview: 480 })
  eq('窗口变窄先收预览', settleWidths(1100, 248, 236, 480, true), { side: 248, tree: 236, preview: 1100 - 248 - 236 - CAT_CHAT_MIN })
  eq('预览收到底后再收文件栏', settleWidths(1000, 248, 400, 160, true), { side: 248, tree: 1000 - 248 - 160 - CAT_CHAT_MIN, preview: 160 })
  eq('没有预览时先收文件栏，文件栏到底再收会话栏', settleWidths(700, 248, 300, 480, false), { side: 180, tree: CAT_PANE_FLOOR, preview: 0 })
}
