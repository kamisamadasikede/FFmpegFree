// 文档页的纯函数（设计说明 5.4 / 2.4 / 2.1）：中间省略拆分、最近列表时间、扩展名色块、支持的扩展名。放这里方便 api.check.ts 断言。

/** 主文件名不超过这么多个字符就不拆（设计说明：≤ 14 个字符不拆） */
export const MIDDLE_KEEP_MAX = 14
/** 尾部保留的主文件名末尾字符数（设计说明 8 节问题 13：固定 6 个字符 + 扩展名） */
export const MIDDLE_TAIL_CHARS = 6

/**
 * 长文件名中间省略：拆成“头部 + 尾部”，头部可收缩并以 … 结尾（CSS 负责），尾部固定不收缩。
 * 尾部 = 主文件名最后 6 个字符 + 扩展名；主文件名 ≤ 14 个字符不拆（tail 为空）。按 Unicode 字符计数，不会切开 emoji。
 */
export function splitMiddle(name: string): { head: string; tail: string } {
  const dot = name.lastIndexOf('.')
  const stem = dot > 0 ? name.slice(0, dot) : name
  const ext = dot > 0 ? name.slice(dot) : ''
  const chars = Array.from(stem)
  if (chars.length <= MIDDLE_KEEP_MAX) return { head: name, tail: '' }
  return { head: chars.slice(0, chars.length - MIDDLE_TAIL_CHARS).join(''), tail: chars.slice(-MIDDLE_TAIL_CHARS).join('') + ext }
}

/** 最近列表的时间：今天 22:41 / 昨天 18:05 / 09-25 16:40 */
export function formatRecentTime(ms: number, now: number = Date.now()): string {
  if (!ms) return ''
  const d = new Date(ms)
  const n = new Date(now)
  const p = (v: number) => String(v).padStart(2, '0')
  const hm = `${p(d.getHours())}:${p(d.getMinutes())}`
  if (d.toDateString() === n.toDateString()) return `今天 ${hm}`
  const y = new Date(n.getFullYear(), n.getMonth(), n.getDate() - 1)
  if (d.toDateString() === y.toDateString()) return `昨天 ${hm}`
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${hm}`
}

/** 扩展名色块的文字：DOCX / XLSX / PPTX / XLS…；没有扩展名给空串 */
export function extBadge(path: string): string {
  const name = path.split(/[\\/]/).pop() ?? ''
  const i = name.lastIndexOf('.')
  return i > 0 && i < name.length - 1 ? name.slice(i + 1).toUpperCase().slice(0, 5) : ''
}

/** 是否是支持的 Office 扩展名（大小写不敏感）。只用于拖入 / 选择时的提示，最终以后端整体校验为准 */
export function isOfficePath(path: string): boolean {
  return /\.(docx|xlsx|pptx)$/i.test(path)
}

/** 缩放档位：50%~200%，步进 25%，不做“适合宽度”，不记忆 */
export const ZOOM_STEPS = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2] as const
export function nextZoom(cur: number, dir: 1 | -1): number {
  const i = ZOOM_STEPS.findIndex((z) => Math.abs(z - cur) < 1e-6)
  const at = i < 0 ? ZOOM_STEPS.indexOf(1) : i
  return ZOOM_STEPS[Math.min(ZOOM_STEPS.length - 1, Math.max(0, at + dir))]
}

/** 缩略图栏的虚拟窗口：可视范围 ± 2 屏才渲染。返回要渲染的页号范围 [from, to]（从 1 开始，含两端） */
export function thumbWindow(scrollTop: number, viewH: number, slotH: number, total: number, screens = 2): { from: number; to: number } {
  if (total <= 0 || slotH <= 0) return { from: 1, to: 0 }
  const first = Math.floor(scrollTop / slotH)
  const visible = Math.max(1, Math.ceil(viewH / slotH))
  const from = Math.max(1, first + 1 - screens * visible)
  const to = Math.min(total, first + visible + screens * visible)
  return { from, to }
}
