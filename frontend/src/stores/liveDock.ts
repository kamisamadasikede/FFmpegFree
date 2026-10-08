import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'

/** 直播页标签行的会话入口，和拉流面板上的 4 个数字。预览开关放这里，文件 / 录屏两页共用，不写入本机（不记住上次选择）。 */
export const useLiveDockStore = defineStore('liveDock', () => {
  const open = ref(false)
  /** 下一次开始推流后要不要立刻连预览。会话进行中拨动它只连接 / 断开播放器，不重启推流。 */
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
  return { open, previewOn, pull, resetPull }
})
