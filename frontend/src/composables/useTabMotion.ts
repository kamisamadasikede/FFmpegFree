// 页签 / 路由切换动效（动画 P1）。
// - useFadeOnChange：source 变化后，新内容只做 150ms 淡入（WAAPI，只动 opacity）。不做 out-in、不推迟渲染：
//   nextTick 在新内容渲染之后、绘制之前，所以不会先闪一下实的再变透明。首次进入、减少动效时不播。
// - useTabInk：下划线式页签的指示条，用 translate + scale 从旧页签滑到新页签（只在切换 / 尺寸变化时读一次布局）。
import { nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { DUR, EASE_OUT, prefersReducedMotion } from '@/utils/motion'

export function useFadeOnChange(source: () => unknown, el: () => Element | null | undefined) {
  watch(source, (v, old) => {
    if (old === undefined || v === old || prefersReducedMotion()) return
    void nextTick(() => {
      const e = el()
      if (e && typeof (e as HTMLElement).animate === 'function') (e as HTMLElement).animate([{ opacity: 0 }, { opacity: 1 }], { duration: DUR.fast, easing: EASE_OUT })
    })
  })
}

/** 指示条基准宽 100px，用 scale 拉到页签宽度（2px 高的条，缩放看不出变形） */
export const INK_BASE = 100

export function useTabInk(list: Ref<HTMLElement | null>, active: () => unknown, selector: string) {
  const style = ref<Record<string, string>>({ opacity: '0' })
  /** 第一次放好位置之后才开过渡，打开页面时不从左边滑进来 */
  const ready = ref(false)
  function measure() {
    const a = list.value?.querySelector<HTMLElement>(selector)
    if (!a) {
      style.value = { opacity: '0' }
      return
    }
    style.value = { translate: `${a.offsetLeft}px 0`, scale: `${a.offsetWidth / INK_BASE} 1` }
  }
  watch(active, () => void nextTick(measure))
  let ro: ResizeObserver | null = null
  onMounted(() => {
    measure()
    requestAnimationFrame(() => requestAnimationFrame(() => (ready.value = true)))
    if (typeof ResizeObserver !== 'undefined' && list.value) {
      ro = new ResizeObserver(() => measure())
      ro.observe(list.value)
      for (const c of Array.from(list.value.children)) ro.observe(c)
    }
  })
  onBeforeUnmount(() => ro?.disconnect())
  return { style, ready }
}
