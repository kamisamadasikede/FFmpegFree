/**
 * 统一预览弹窗的状态（契约 6.12.32、6.12.37~6.12.48；设计 v0.3 §九 / §十一）。
 * 一次只保留一个预览（切换 / 关闭都 CancelDocPreview），所以不会碰到「最多 8 个」上限。
 * doc:preview 事件可能比 GetDocPreview 先到：按 previewId 暂存，拿到返回值后再套用。
 */
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'
import { cancelDocPreview, getDocPreview, watchDocPreview, type DocPreview, type DocPreviewEvent, type DocPreviewRequest } from '@/api/docV27'
import { toAppError } from '@/api/call'
import { useDocComponentStore } from '@/stores/docComponent'

export interface PreviewItem {
  /** sourceId 或 taskId 二选一 */
  req: DocPreviewRequest
  name: string
  sizeBytes: number
  /** 模拟用：这一行有排队 / 运行中的转换 */
  converting?: boolean
  /** 模拟用：原文件不在了 */
  missing?: boolean
}

export const usePreviewStore = defineStore('docPreview', () => {
  const open = ref(false)
  const items = ref<PreviewItem[]>([])
  const index = ref(0)
  const preview = shallowRef<DocPreview | null>(null)
  /** GetDocPreview 本身失败（NOT_FOUND 等）：一律显示「这个文件暂时无法预览。」 */
  const callFailed = ref(false)
  const loading = ref(false)
  /** 这次运行里在应用里保存过的结果（6.12.41：只在会话内记，不落库） */
  const editedTasks = ref(new Set<string>())
  /** 弹窗外的入口元素，关闭后把焦点还回去 */
  let trigger: HTMLElement | null = null
  const buffered = new Map<string, DocPreviewEvent>()
  let seq = 0
  let refetched404 = false
  let unwatch: (() => void) | null = null

  const current = computed(() => items.value[index.value] ?? null)
  const multi = computed(() => items.value.length > 1)

  function ensureWatch() {
    if (unwatch) return
    unwatch = watchDocPreview((e) => {
      const p = preview.value
      if (p && p.previewId === e.previewId) apply(e)
      else {
        buffered.set(e.previewId, e)
        if (buffered.size > 16) buffered.delete(buffered.keys().next().value as string)
      }
    })
  }
  function apply(e: DocPreviewEvent) {
    const p = preview.value
    if (!p || p.previewId !== e.previewId) return
    preview.value = { ...p, state: e.state, kind: e.kind || p.kind, url: e.url ?? p.url, error: e.error ?? null }
  }

  async function release() {
    const p = preview.value
    preview.value = null
    if (p) await cancelDocPreview(p.previewId).catch(() => {})
  }

  async function load() {
    const it = current.value
    if (!it) return
    const my = ++seq
    await release()
    loading.value = true
    callFailed.value = false
    const comp = useDocComponentStore()
    try {
      const p = await getDocPreview(it.req, { name: it.name, sizeBytes: it.sizeBytes, engineReady: comp.ready, converting: it.converting, missing: it.missing })
      if (my !== seq || !open.value) {
        void cancelDocPreview(p.previewId).catch(() => {})
        return
      }
      preview.value = p
      const b = buffered.get(p.previewId)
      if (b) {
        buffered.delete(p.previewId)
        apply(b)
      }
    } catch (e) {
      if (my !== seq) return
      console.warn('GetDocPreview failed', toAppError(e).code)
      callFailed.value = true
    } finally {
      if (my === seq) loading.value = false
    }
  }

  function show(list: PreviewItem[], i = 0, from?: HTMLElement | null) {
    ensureWatch()
    trigger = from ?? (document.activeElement as HTMLElement | null)
    items.value = list
    index.value = Math.min(Math.max(0, i), Math.max(0, list.length - 1))
    open.value = true
    refetched404 = false
    void load()
  }
  function go(delta: number) {
    const n = index.value + delta
    if (n < 0 || n >= items.value.length) return
    index.value = n
    refetched404 = false
    void load()
  }
  async function close() {
    open.value = false
    seq++
    await release()
    loading.value = false
    const t = trigger
    trigger = null
    if (t && document.contains(t)) setTimeout(() => t.focus(), 0)
  }
  /** 本地地址 404（token 作废）：重新 GetDocPreview 一次（6.12.32.2） */
  function onUrlGone() {
    if (refetched404) {
      callFailed.value = true
      return
    }
    refetched404 = true
    void load()
  }
  /** 保存成功后重新拿一次预览（6.12.49：旧的本地地址会失效），不经过「加载中」，旧的拿到新的之后再撤销 */
  async function refresh(): Promise<DocPreview | null> {
    const it = current.value
    const old = preview.value
    if (!it || !old) return null
    const comp = useDocComponentStore()
    try {
      const p = await getDocPreview(it.req, { name: it.name, sizeBytes: it.sizeBytes, engineReady: comp.ready, converting: it.converting, missing: it.missing })
      if (!open.value || preview.value !== old) {
        void cancelDocPreview(p.previewId).catch(() => {})
        return null
      }
      preview.value = p
      void cancelDocPreview(old.previewId).catch(() => {})
      return p
    } catch {
      /* 保持旧的 */
      return null
    }
  }
  const reload = () => {
    refetched404 = false
    return load()
  }
  function markEdited(taskId?: string) {
    if (!taskId) return
    const s = new Set(editedTasks.value)
    s.add(taskId)
    editedTasks.value = s
  }

  return { open, items, index, preview, callFailed, loading, current, multi, editedTasks, show, go, close, reload, refresh, onUrlGone, markEdited }
})
