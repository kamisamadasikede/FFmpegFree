/** 三栏宽度。左右两栏和文件预览都可以拖，并给中间对话留出位置。 */
import { reactive } from 'vue'
import { CAT_PREVIEW_WIDTH_KEY } from '@/api/catStorageKeys'
import { t } from '@/i18n'

export { CAT_PREVIEW_WIDTH_KEY }
export const CAT_SIDE_WIDTH_KEY = 'ffmpegfree.cat.sideWidth'
export const CAT_TREE_WIDTH_KEY = 'ffmpegfree.cat.treeWidth'
export const CAT_EXTRA_WIDTH_KEY = 'ffmpegfree.cat.extraWidth'
export const CAT_EXTRA_DEFAULT = 760
export const CAT_EXTRA_MIN = 420
export const CAT_TREE_WIDTH = 236
export const CAT_SIDE_DEFAULT = 248
export const CAT_SIDE_MIN = 200
export const CAT_TREE_MIN = 180
export const CAT_PREVIEW_MIN = 280
export const CAT_PREVIEW_DEFAULT = 480
/** 窗口很窄时，左右栏和预览都可以收到这个宽度，但不会更窄。 */
export const CAT_PANE_FLOOR = 160
/** 中间对话至少留这么宽，避免输入框被两边顶出去 */
export const CAT_CHAT_MIN = 360

export function readPaneWidth(raw: string | null, fallback: number): number {
  const n = Number(raw)
  if (!Number.isFinite(n) || n <= 0) return fallback
  return Math.round(n)
}

export function readPreviewWidth(raw: string | null): number {
  return readPaneWidth(raw, CAT_PREVIEW_DEFAULT)
}

function storedWidth(key: string, fallback: number): number {
  try {
    return readPaneWidth(localStorage.getItem(key), fallback)
  } catch {
    return fallback
  }
}

/** 左侧会话栏和右侧文件栏的当前宽度。拖动时改这里，刷新后从本地记录恢复。 */
export const catPane = reactive({
  side: storedWidth(CAT_SIDE_WIDTH_KEY, CAT_SIDE_DEFAULT),
  tree: storedWidth(CAT_TREE_WIDTH_KEY, CAT_TREE_WIDTH),
})

export function rememberPaneWidth(key: string, width: number) {
  try {
    localStorage.setItem(key, String(Math.round(width)))
  } catch {
    /* 隐私模式写不进就只在这次运行里生效 */
  }
}

/** 拖动开始时量一次上下限。拖的过程里只用这两个数，不再读页面宽度。 */
export function paneLimits(total: number, taken: number, min: number): { lo: number; hi: number } {
  const hi = Math.max(CAT_PANE_FLOOR, total - taken)
  const lo = Math.min(min, hi)
  return { lo, hi }
}

export function clampToLimits(want: number, limits: { lo: number; hi: number }, fallback: number): number {
  const n = Number.isFinite(want) ? want : fallback
  return Math.round(Math.min(limits.hi, Math.max(limits.lo, n)))
}

/**
 * 把一栏收进可用范围。taken 是其它栏和对话必须留下的宽度。
 * 窗口够宽时不低于 min；窗口很窄时可以收到 floor。
 */
export function clampPaneWidth(want: number, total: number, taken: number, min: number, fallback: number): number {
  return clampToLimits(want, paneLimits(total, taken, min), fallback)
}

/**
 * 把想要的预览宽度收进可用范围。
 * total 是整页三栏的宽，side 是左侧对话列表的宽，tree 是右侧文件栏的宽。
 */
export function clampPreviewWidth(want: number, total: number, side: number, tree = CAT_TREE_WIDTH): number {
  return clampPaneWidth(want, total, side + tree + CAT_CHAT_MIN, CAT_PREVIEW_MIN, CAT_PREVIEW_DEFAULT)
}

/**
 * 窗口变窄时先收预览，再收文件栏，最后收左侧会话栏。
 * 每一栏都不低于 floor。空间够时不改用户拖过的宽度。
 */
export function settleWidths(total: number, side: number, tree: number, preview: number, previewOpen: boolean): { side: number; tree: number; preview: number } {
  let s = side
  let t = tree
  let p = previewOpen ? preview : 0
  let need = s + t + p + CAT_CHAT_MIN - total
  if (need > 0 && previewOpen) {
    const cut = Math.min(need, Math.max(0, p - CAT_PANE_FLOOR))
    p -= cut
    need -= cut
  }
  if (need > 0) {
    const cut = Math.min(need, Math.max(0, t - CAT_PANE_FLOOR))
    t -= cut
    need -= cut
  }
  if (need > 0) {
    const cut = Math.min(need, Math.max(0, s - CAT_PANE_FLOOR))
    s -= cut
  }
  return { side: Math.round(s), tree: Math.round(t), preview: Math.round(previewOpen ? p : 0) }
}

/**
 * 按住分隔条左右拖。sign 为 1 时向右变宽，为 -1 时向左变宽。
 * 移动时每帧只调用一次 paint，由调用方直接改元素宽度；松手后 done 再写入状态。
 */
export function dragPaneWidth(handle: HTMLElement, e: PointerEvent, sign: 1 | -1, paint: (delta: number) => void, done: () => void) {
  if (e.button !== 0) return
  e.preventDefault()
  handle.setPointerCapture(e.pointerId)
  const startX = e.clientX
  const prevCursor = document.body.style.cursor
  const prevSelect = document.body.style.userSelect
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  let dx = 0
  let frame = 0
  const move = (ev: PointerEvent) => {
    dx = sign * (ev.clientX - startX)
    if (frame) return
    frame = requestAnimationFrame(() => {
      frame = 0
      paint(dx)
    })
  }
  const up = (ev: PointerEvent) => {
    if (handle.hasPointerCapture(ev.pointerId)) handle.releasePointerCapture(ev.pointerId)
    handle.removeEventListener('pointermove', move)
    handle.removeEventListener('pointerup', up)
    handle.removeEventListener('pointercancel', up)
    if (frame) cancelAnimationFrame(frame)
    paint(dx)
    document.body.style.cursor = prevCursor
    document.body.style.userSelect = prevSelect
    done()
  }
  handle.addEventListener('pointermove', move)
  handle.addEventListener('pointerup', up)
  handle.addEventListener('pointercancel', up)
}
