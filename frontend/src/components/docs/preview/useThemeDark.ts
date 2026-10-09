import { onBeforeUnmount, ref } from 'vue'
/** 跟随 html.dark（主题切换时 iframe 里的配色一起变） */
export function useThemeDark() {
  const el = document.documentElement
  const dark = ref(el.classList.contains('dark'))
  const mo = new MutationObserver(() => (dark.value = el.classList.contains('dark')))
  mo.observe(el, { attributes: true, attributeFilter: ['class'] })
  onBeforeUnmount(() => mo.disconnect())
  return dark
}
