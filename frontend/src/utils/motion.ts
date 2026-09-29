/** 用户要求减少动效（系统 prefers-reduced-motion 或设置里给 <html> 加的 reduce-motion）时为 true；滚动用 behavior:'auto'（瞬间）而不是 'smooth' */
export function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined') return true
  return !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches || document.documentElement.classList.contains('reduce-motion')
}
export const scrollBehavior = (): ScrollBehavior => (prefersReducedMotion() ? 'auto' : 'smooth')
