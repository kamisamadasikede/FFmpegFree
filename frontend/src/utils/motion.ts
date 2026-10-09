/** 用户要求减少动效（系统 prefers-reduced-motion 或设置里给 <html> 加的 reduce-motion）时为 true；滚动用 behavior:'auto'（瞬间）而不是 'smooth' */
export function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined') return true
  return !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches || document.documentElement.classList.contains('reduce-motion')
}
export const scrollBehavior = (): ScrollBehavior => (prefersReducedMotion() ? 'auto' : 'smooth')

/** 动效时长（与 tokens.css 的 --ff-dur-* 一致；JS 动画用） */
export const DUR = { quick: 120, fast: 150, base: 200, slow: 320 } as const
export const EASE_OUT = 'cubic-bezier(0.16, 1, 0.3, 1)'
export const EASE_IN = 'cubic-bezier(0.4, 0, 1, 1)'

/**
 * 正在离开（播放退场动画）的元素：设 inert，焦点在里面就移出。
 * 退场那 150ms 里元素还在 DOM 上，不能让回车 / 点击再触发一次里面的按钮。
 */
export function guardLeaving(el: Element) {
  if (!(el instanceof HTMLElement)) return
  el.setAttribute('inert', '')
  const a = document.activeElement
  if (a instanceof HTMLElement && el.contains(a)) a.blur()
}

type AnimEl = HTMLElement & { _ffAnim?: Animation }
const BOX = ['paddingTop', 'paddingBottom', 'marginTop', 'marginBottom', 'borderTopWidth', 'borderBottomWidth'] as const

function boxFrames(el: HTMLElement): [Keyframe, Keyframe] {
  const cs = getComputedStyle(el)
  const open: Keyframe = { height: `${el.offsetHeight}px`, opacity: 1 }
  const shut: Keyframe = { height: '0px', opacity: 0 }
  for (const k of BOX) {
    open[k] = cs[k]
    shut[k] = '0px'
  }
  return [shut, open]
}

export function stopAnim(el: Element) {
  const a = (el as AnimEl)._ffAnim
  if (a) {
    a.cancel()
    ;(el as AnimEl)._ffAnim = undefined
  }
}

/** 高度 + 透明度展开（横幅、列表新行）。只在进入那一下读一次尺寸，动画交给 WAAPI；减少动效时直接结束 */
export function collapseIn(el: Element, done: () => void) {
  const h = el as AnimEl
  stopAnim(h)
  if (prefersReducedMotion() || typeof h.animate !== 'function') return done()
  const [shut, open] = boxFrames(h)
  const ov = h.style.overflow
  h.style.overflow = 'hidden'
  const a = h.animate([shut, open], { duration: DUR.base, easing: EASE_OUT })
  h._ffAnim = a
  const end = () => {
    h.style.overflow = ov
    if (h._ffAnim === a) h._ffAnim = undefined
    done()
  }
  a.onfinish = end
  a.oncancel = () => (h.style.overflow = ov)
}

/** 高度 + 透明度收起（关横幅、删行）；下面的内容跟着平滑上移，不跳 */
export function collapseOut(el: Element, done: () => void) {
  const h = el as AnimEl
  stopAnim(h)
  guardLeaving(h)
  if (prefersReducedMotion() || typeof h.animate !== 'function') return done()
  const [shut, open] = boxFrames(h)
  h.style.overflow = 'hidden'
  const a = h.animate([open, shut], { duration: DUR.fast, easing: EASE_IN, fill: 'forwards' })
  h._ffAnim = a
  a.onfinish = () => done()
}
