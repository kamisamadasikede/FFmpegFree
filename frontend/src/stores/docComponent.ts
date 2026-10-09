/**
 * 文档组件状态（契约 6.12.12~6.12.15）。仿 stores/ffmpeg.ts：先订阅事件再拉状态；
 * state 变为 ready（或离开 ready）时通知格式表重新拉取。
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  cancelDocComponentInstall,
  getDocComponentStatus,
  installDocComponent,
  recheckDocComponent,
  watchDocComponent,
  watchDocProgress,
  type DocComponentProgress,
  type DocComponentStatus,
} from '@/api/docV26'
import { toAppError } from '@/api/call'
import { simParam } from '@/api/sim'
import {
  DOC_LINUX_MISSING,
  DOC_LINUX_OUTDATED,
  DOC_OUTDATED_DOWNLOAD,
  docDiskFullText,
  docErrorText,
  docSizeGuideText,
} from '@/utils/docV26Text'
import { formatBytes } from '@/utils/format'

/** 引导卡的视图：guide 首次引导 / slim 收起细条 / downloading / preparing / failed / outdated / linux / hidden(ready/checking) */
export type DocGuideView = 'hidden' | 'guide' | 'slim' | 'downloading' | 'preparing' | 'failed' | 'outdated' | 'linux'

const DISMISS_KEY = 'ff.doc.guideDismissed'

export const useDocComponentStore = defineStore('docComponent', () => {
  const status = ref<DocComponentStatus>({
    state: 'checking',
    version: '',
    source: '',
    canDownload: true,
    downloadBytes: 0,
    installBytes: 0,
  })
  const progress = ref<DocComponentProgress | null>(null)
  /** 下载速度（字节/秒），按进度事件估算 */
  const speedBps = ref(0)
  const busy = ref(false)
  const actionError = ref<{ code: string; message: string; detail?: string } | null>(null)
  const dismissed = ref(readDismissed())
  let inited = false
  let readyListeners: Array<() => void> = []
  let lastSample: { t: number; bytes: number } | null = null

  function readDismissed(): boolean {
    if (simParam('doc') === 'slim') return true
    try {
      return globalThis.localStorage?.getItem(DISMISS_KEY) === '1'
    } catch {
      return false
    }
  }

  const ready = computed(() => status.value.state === 'ready')
  const isLinux = computed(() => !status.value.canDownload)
  const notReady = computed(() => !['ready', 'checking'].includes(status.value.state))
  const inFlight = computed(() => status.value.state === 'downloading' || status.value.state === 'preparing')

  const pct = computed(() => {
    const p = progress.value
    if (p?.phase === 'downloading' && typeof p.progress === 'number') return Math.round(p.progress * 100)
    const total = status.value.downloadBytes
    const rec = status.value.receivedBytes ?? 0
    return total > 0 ? Math.round((rec / total) * 100) : 0
  })
  const receivedBytes = computed(() => progress.value?.receivedBytes ?? status.value.receivedBytes ?? 0)
  const totalBytes = computed(() => progress.value?.totalBytes || status.value.downloadBytes)

  const sizeText = computed(() => docSizeGuideText(status.value.downloadBytes, status.value.installBytes))
  const downloadSizeShort = computed(() => (status.value.downloadBytes > 0 ? `约 ${formatBytes(status.value.downloadBytes)}` : ''))

  /** 状态行 / 失败卡的文字（不出错误码、不出 reason=） */
  const errorText = computed(() => {
    const e = actionError.value ?? status.value.error
    if (!e) return ''
    if (e.code === 'CONVERT_DISK_FULL') return docDiskFullText(e.detail, '')
    return docErrorText(e.code, e.message, isLinux.value)
  })

  const outdatedText = computed(() => (isLinux.value ? DOC_LINUX_OUTDATED : DOC_OUTDATED_DOWNLOAD))
  const linuxMissingText = DOC_LINUX_MISSING

  const guideView = computed<DocGuideView>(() => {
    const s = status.value.state
    if (s === 'ready' || s === 'checking') return 'hidden'
    if (s === 'downloading') return 'downloading'
    if (s === 'preparing') return 'preparing'
    if (actionError.value || s === 'failed') return dismissed.value && s !== 'failed' ? 'slim' : 'failed'
    if (s === 'outdated') return dismissed.value ? 'slim' : 'outdated'
    if (isLinux.value) return dismissed.value ? 'slim' : 'linux'
    return dismissed.value ? 'slim' : 'guide'
  })

  function onReady(cb: () => void): () => void {
    readyListeners.push(cb)
    return () => (readyListeners = readyListeners.filter((x) => x !== cb))
  }

  function setStatus(next: DocComponentStatus) {
    const wasReady = status.value.state === 'ready'
    status.value = { ...next, installBytes: next.installBytes ?? 0 }
    if (next.state !== 'downloading') {
      speedBps.value = 0
      lastSample = null
    }
    if (next.state === 'downloading' && next.receivedBytes != null) {
      progress.value = {
        phase: 'downloading',
        receivedBytes: next.receivedBytes,
        totalBytes: next.downloadBytes,
        progress: next.downloadBytes ? next.receivedBytes / next.downloadBytes : 0,
      }
    }
    if (next.state === 'preparing') progress.value = { phase: 'preparing', receivedBytes: next.downloadBytes, totalBytes: next.downloadBytes }
    if (next.state === 'ready' || next.state === 'missing') progress.value = null
    if (next.state !== 'failed') actionError.value = null
    const isReadyNow = next.state === 'ready'
    if (isReadyNow !== wasReady) readyListeners.forEach((f) => f())
  }

  function onProgress(p: DocComponentProgress) {
    progress.value = p
    if (p.phase === 'downloading') {
      const now = Date.now()
      if (lastSample && now > lastSample.t && p.receivedBytes >= lastSample.bytes) {
        const inst = ((p.receivedBytes - lastSample.bytes) * 1000) / (now - lastSample.t)
        speedBps.value = speedBps.value ? speedBps.value * 0.7 + inst * 0.3 : inst
      }
      if (p.receivedBytes === 0) speedBps.value = 0 // 校验不过从头下
      lastSample = { t: now, bytes: p.receivedBytes }
    }
  }

  async function init() {
    if (inited) return
    inited = true
    // 先订阅再拉（契约第 5 节订阅顺序）
    watchDocComponent((s) => setStatus(s))
    watchDocProgress((p) => onProgress(p))
    try {
      setStatus(await getDocComponentStatus())
    } catch (e) {
      console.error('GetDocComponentStatus failed', e)
    }
  }

  async function run(fn: () => Promise<unknown>) {
    if (busy.value) return
    busy.value = true
    actionError.value = null
    try {
      const r = await fn()
      if (r && typeof r === 'object' && 'state' in (r as object)) setStatus(r as DocComponentStatus)
    } catch (e) {
      const ae = toAppError(e)
      actionError.value = { code: ae.code, message: ae.message, detail: ae.detail }
    } finally {
      busy.value = false
    }
  }

  /** 下载 / 失败后重试（后端从断点续传） */
  const install = (mirror = '') => run(() => installDocComponent(mirror))
  const cancel = () => run(() => cancelDocComponentInstall())
  const recheck = () => run(() => recheckDocComponent())

  function dismissGuide() {
    dismissed.value = true
    try {
      globalThis.localStorage?.setItem(DISMISS_KEY, '1')
    } catch {
      /* ignore */
    }
  }
  function reopenGuide() {
    dismissed.value = false
  }

  return {
    status,
    progress,
    speedBps,
    busy,
    actionError,
    dismissed,
    ready,
    isLinux,
    notReady,
    inFlight,
    pct,
    receivedBytes,
    totalBytes,
    sizeText,
    downloadSizeShort,
    errorText,
    outdatedText,
    linuxMissingText,
    guideView,
    init,
    install,
    cancel,
    recheck,
    dismissGuide,
    reopenGuide,
    onReady,
    setStatus,
  }
})
