// 文件预览宽度：默认、下限、给对话留空、窗口很窄时再让步。
import { CAT_PREVIEW_DEFAULT, clampPreviewWidth, readPreviewWidth } from './catPreviewWidth'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catPreviewWidthChecks(eq: Eq): void {
  eq('没记过宽度时用默认', readPreviewWidth(null), CAT_PREVIEW_DEFAULT)
  eq('坏记录也回默认', readPreviewWidth('nope'), CAT_PREVIEW_DEFAULT)
  eq('记住的宽度取整', readPreviewWidth('520.4'), 520)

  eq('正常窗口按用户拖的宽度', clampPreviewWidth(480, 1400, 248), 480)
  eq('拖太宽时给对话留 360', clampPreviewWidth(2000, 1400, 248), 1400 - 248 - 236 - 360)
  eq('拖太窄时不低于 280', clampPreviewWidth(100, 1400, 248), 280)
  eq('窗口不够时预览让到剩余空间', clampPreviewWidth(480, 900, 248), 160)
}
