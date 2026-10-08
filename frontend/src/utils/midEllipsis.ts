/**
 * 中间省略（复核 N1）：放不下时把中间换成“…”，保留开头和结尾——结尾至少保留扩展名和它前面的 1 个字，
 * 例如 “launch-4k.mp4” → “laun…k.mp4”；任务名 “launch-4k.mov → MP4” 只省略源文件名中间，“.mov → MP4” 整段保留（设计说明 §八 第 34 条：“→ 目标格式”完整显示）。
 * measure 返回一段文字的显示宽度（页面用 canvas 量，检查里用字数）。
 */
export const MID_ELLIPSIS = '…'
export function midTailMin(text: string): number {
  const arrow = text.lastIndexOf(' → ')
  if (arrow > 0) {
    // 任务名：“→ 目标格式”整段保留，再加上源文件的扩展名和它前面 1 个字
    const d = text.lastIndexOf('.', arrow)
    const from = d > 0 && arrow - d <= 8 ? d - 1 : arrow - 1
    return Math.min(text.length, text.length - Math.max(0, from))
  }
  const dot = text.lastIndexOf('.')
  if (dot > 0 && text.length - dot <= 8) return Math.min(text.length, text.length - dot + 1)
  return Math.min(text.length, 4)
}
/** 按像素宽度省略（canvas 量字；没有 canvas 时按每字 font-size 粗估），给不方便挂组件的纯文字用（例如 toast） */
export function midEllipsisPx(text: string, maxPx: number, font: string): string {
  const ctx = typeof document !== 'undefined' ? document.createElement('canvas').getContext('2d') : null
  if (ctx) {
    ctx.font = font
    return midEllipsis(text, maxPx, (s) => ctx.measureText(s).width)
  }
  const px = parseFloat(/(\d+(?:\.\d+)?)px/.exec(font)?.[1] ?? '13')
  return midEllipsis(text, maxPx, (s) => Array.from(s).length * px)
}
export function midEllipsis(text: string, maxWidth: number, measure: (s: string) => number): string {
  if (!text || measure(text) <= maxWidth) return text
  const chars = Array.from(text) // 按字符（含代理对）切，不切坏表情 / 生僻字
  const len = chars.length
  const minTail = Math.min(midTailMin(text), len - 1)
  const build = (n: number) => {
    const t = Math.min(n, Math.max(minTail, Math.round(n / 3)))
    return chars.slice(0, n - t).join('') + MID_ELLIPSIS + chars.slice(len - t).join('')
  }
  // n = 保留的字数（不含“…”），找放得下的最大 n
  let lo = 0
  let hi = len - 1
  if (measure(build(Math.min(minTail, hi))) > maxWidth) {
    // 连“…+结尾”都放不下：退回末尾省略，至少显示“…”
    let k = len - 1
    while (k > 0 && measure(chars.slice(0, k).join('') + MID_ELLIPSIS) > maxWidth) k--
    return chars.slice(0, k).join('') + MID_ELLIPSIS
  }
  lo = Math.min(minTail, hi)
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2)
    if (measure(build(mid)) <= maxWidth) lo = mid
    else hi = mid - 1
  }
  return build(lo)
}
