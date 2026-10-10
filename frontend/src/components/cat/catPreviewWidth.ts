/** 文件预览宽度。树宽固定，预览宽度由用户拖动，并给中间对话留出位置。 */
export const CAT_PREVIEW_WIDTH_KEY = 'ffmpegfree.cat.previewWidth'
export const CAT_TREE_WIDTH = 236
export const CAT_PREVIEW_MIN = 280
export const CAT_PREVIEW_DEFAULT = 480
/** 中间对话至少留这么宽，避免输入框被预览顶出去 */
export const CAT_CHAT_MIN = 360

export function readPreviewWidth(raw: string | null): number {
  const n = Number(raw)
  if (!Number.isFinite(n) || n <= 0) return CAT_PREVIEW_DEFAULT
  return Math.round(n)
}

/**
 * 把想要的预览宽度收进可用范围。
 * total 是整页三栏的宽，side 是左侧对话列表的宽。
 * 窗口很窄时预览可以低于最小宽度，但不会比 160 更窄。
 */
export function clampPreviewWidth(want: number, total: number, side: number): number {
  const room = total - side - CAT_TREE_WIDTH - CAT_CHAT_MIN
  const max = Math.max(160, room)
  const n = Number.isFinite(want) ? want : CAT_PREVIEW_DEFAULT
  const lo = Math.min(CAT_PREVIEW_MIN, max)
  return Math.round(Math.min(max, Math.max(lo, n)))
}
