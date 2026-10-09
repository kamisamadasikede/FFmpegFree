import { defineStore } from 'pinia'
import { reactive, ref, watch } from 'vue'
import { useLiveSessionsStore } from './liveSessions'

/**
 * 直播页标签行的会话入口，和拉流面板上的 4 个数字。
 * previewOn = 设置栏底部的“开启预览”：有进行中的当前会话时和这一路的预览双向同步（会话面板里那一行的开关是同一个值）；
 * 没有进行中的会话时是下一路的初始值，并复位为开（产品经理已定：不记住上次选择）。不写入本机。
 */
/** 会话入口的角标文字：0 路时不显示角标（设计 / 架构 10-08 定，推流、录屏推流、拉流三页一样），只留图标 */
export const sessionBadge = (n: number): string => (n > 0 ? String(n) : '')

export const useLiveDockStore = defineStore('liveDock', () => {
  const sessions = useLiveSessionsStore()
  const open = ref(false)
  const previewOn = ref(true)
  const pull = reactive({ active: false, bitrate: '—', fps: '—', dropped: '—', bytes: '—', unit: '' })
  function resetPull() {
    pull.active = false
    pull.bitrate = '—'
    pull.fps = '—'
    pull.dropped = '—'
    pull.bytes = '—'
    pull.unit = ''
  }

  const liveCur = () => {
    const c = sessions.current
    return c && (c.status === 'run' || c.status === 'stp') ? c : undefined
  }
  // 当前会话（或它的开关）变了 → 右栏开关跟上
  watch(() => { const c = liveCur(); return c ? `${c.id}:${c.preview !== false}` : '' }, () => {
    const c = liveCur()
    if (c) previewOn.value = c.preview !== false
  }, { immediate: true })
  // 右栏开关拨动 → 只改当前这一路
  watch(previewOn, (on) => {
    const c = liveCur()
    if (c && (c.preview !== false) !== on) sessions.setPreview(c.id, on)
  })
  // 没有进行中的会话了 → 复位为开
  watch(() => sessions.busyCount, (n) => { if (n === 0) previewOn.value = true })

  return { open, previewOn, pull, resetPull }
})
