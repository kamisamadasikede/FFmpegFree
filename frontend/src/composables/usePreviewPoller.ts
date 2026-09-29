import { onActivated, onBeforeUnmount, onDeactivated, onMounted, shallowRef } from 'vue'
import { getPreview } from '@/api/live'
import { PreviewPoller, type PreviewSnapshot } from '@/api/livePreviewPoller'

/**
 * 把 PreviewPoller 接进组件：文档可见性（document.visibilityState）、KeepAlive 的激活 / 停用、卸载都会通知它。
 * 页面被 KeepAlive 缓存（切到别的页签）= 不可见 → 停止轮询；回来立即取一次再继续。
 */
export function usePreviewPoller() {
  const snap = shallowRef<PreviewSnapshot>({ phase: 'idle', data: '', ts: 0 })
  const poller = new PreviewPoller({ fetch: getPreview, onChange: (s) => (snap.value = s) })
  let docVisible = typeof document === 'undefined' || document.visibilityState !== 'hidden'
  let active = true
  const apply = () => poller.setVisible(docVisible && active)
  const onVis = () => {
    docVisible = document.visibilityState !== 'hidden'
    apply()
  }
  onMounted(() => document.addEventListener('visibilitychange', onVis))
  onActivated(() => {
    active = true
    apply()
  })
  onDeactivated(() => {
    active = false
    apply()
  })
  onBeforeUnmount(() => {
    document.removeEventListener('visibilitychange', onVis)
    poller.dispose()
  })
  return { snap, poller }
}
