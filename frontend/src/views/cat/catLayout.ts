import { ref, watch } from 'vue'
import { CAT_RAIL_COLLAPSED_KEY, CAT_RIGHT_COLLAPSED_KEY } from '@/api/catStorageKeys'
import { t } from '@/i18n'

function load(key: string): boolean {
  try {
    return typeof localStorage !== 'undefined' && localStorage.getItem(key) === '1'
  } catch {
    return false
  }
}
function keep(key: string, on: boolean) {
  try {
    localStorage.setItem(key, on ? '1' : '0')
  } catch {
    /* 存不了就只在本次有效 */
  }
}

/** C3 侧栏收起（Ctrl B）/ E3 右栏收起（Ctrl J），记在本机 */
export const railCollapsed = ref(load(CAT_RAIL_COLLAPSED_KEY))
export const rightCollapsed = ref(load(CAT_RIGHT_COLLAPSED_KEY))
watch(railCollapsed, (v) => keep(CAT_RAIL_COLLAPSED_KEY, v))
watch(rightCollapsed, (v) => keep(CAT_RIGHT_COLLAPSED_KEY, v))

/** Ctrl/Cmd+B、Ctrl/Cmd+J；返回是否处理了 */
export function onLayoutKey(e: KeyboardEvent): boolean {
  if (!(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey) return false
  const k = e.key.toLowerCase()
  if (k === 'b') railCollapsed.value = !railCollapsed.value
  else if (k === 'j') rightCollapsed.value = !rightCollapsed.value
  else return false
  e.preventDefault()
  return true
}
