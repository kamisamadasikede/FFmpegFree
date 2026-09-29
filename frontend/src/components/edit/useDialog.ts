// 弹框焦点管理：打开时焦点进入弹框（优先 [data-autofocus]），Tab 在弹框内循环，Esc 关闭，关闭后焦点回到打开前的元素。
import { nextTick, onBeforeUnmount, onMounted, type Ref } from 'vue'

const FOCUSABLE = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

export function useDialog(el: Ref<HTMLElement | null>, onClose: () => void, restoreTo?: () => HTMLElement | null) {
  const opener = document.activeElement as HTMLElement | null
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      e.preventDefault()
      onClose()
      return
    }
    if (e.key !== 'Tab' || !el.value) return
    const items = Array.from(el.value.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((n) => n.offsetParent !== null || n === document.activeElement)
    if (!items.length) return
    const first = items[0]
    const last = items[items.length - 1]
    if (e.shiftKey && (document.activeElement === first || !el.value.contains(document.activeElement))) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && (document.activeElement === last || !el.value.contains(document.activeElement))) {
      e.preventDefault()
      first.focus()
    }
  }
  onMounted(async () => {
    document.addEventListener('keydown', onKey, true)
    await nextTick()
    const target = el.value?.querySelector<HTMLElement>('[data-autofocus]') ?? el.value?.querySelector<HTMLElement>(FOCUSABLE)
    target?.focus()
  })
  onBeforeUnmount(() => {
    document.removeEventListener('keydown', onKey, true)
    const back = restoreTo?.() ?? opener
    if (back && document.contains(back)) back.focus()
  })
}
