/**
 * 办公画布组件状态 + Cat 演示文稿 artifact（契约 6.19.16）。
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  cancelOfficeCanvasInstall,
  emitOfficeSimArtifact,
  exportOfficeCanvasPptx,
  getOfficeCanvasStatus,
  installOfficeCanvas,
  isOfficeCanvasSim,
  MOCK_OFFICE_DECK,
  OFFICE_CANVAS_COPY as C,
  openOfficeCanvasFolder,
  peekOfficeSimStatus,
  recheckOfficeCanvas,
  setOfficeSimStatus,
  watchOfficeArtifact,
  watchOfficeCanvas,
  watchOfficeCanvasProgress,
  type OfficeArtifact,
  type OfficeCanvasStatus,
  type OfficeDeck,
} from '@/api/officeCanvas'
import { toAppError } from '@/api/call'
import { ElMessage } from 'element-plus'

const LATER_KEY = 'ff.officeCanvas.later'

function readLater(): boolean {
  try {
    return localStorage.getItem(LATER_KEY) === '1'
  } catch {
    return false
  }
}

function writeLater(v: boolean) {
  try {
    if (v) localStorage.setItem(LATER_KEY, '1')
    else localStorage.removeItem(LATER_KEY)
  } catch {
    /* ignore */
  }
}

export const useOfficeCanvasStore = defineStore('officeCanvas', () => {
  const status = ref<OfficeCanvasStatus>({
    state: 'checking',
    version: '',
    source: '',
    canDownload: false,
    downloadBytes: 0,
    installBytes: 0,
    error: null,
  })
  const loaded = ref(false)
  const busy = ref(false)
  const later = ref(readLater())
  /** 当前对话是否出现演示文稿能力（有 artifact 或走查） */
  const slidesNeeded = ref(false)
  const artifact = ref<OfficeArtifact | null>(null)
  const artifactsByConv = new Map<string, OfficeArtifact>()
  const selectedConv = ref('')
  function selectConversation(convId: string) {
    if (artifact.value) artifactsByConv.set(artifact.value.convId, artifact.value)
    selectedConv.value = convId
    artifact.value = artifactsByConv.get(convId) || null
    slidesNeeded.value = !!artifact.value
    exportOk.value = ''
  }
  const exportBusy = ref(false)
  const exportOk = ref('')
  let offStatus: (() => void) | undefined
  let offProg: (() => void) | undefined
  let offArt: (() => void) | undefined

  const ready = computed(() => status.value.state === 'ready')
  const downloading = computed(() => status.value.state === 'downloading' || status.value.state === 'preparing')
  const failed = computed(() => status.value.state === 'failed')
  const pct = computed(() => {
    const total = status.value.downloadBytes || 1
    const got = status.value.receivedBytes ?? 0
    if (status.value.state === 'preparing') return 100
    return Math.min(99, Math.round((got / total) * 100))
  })
  const deck = computed<OfficeDeck | null>(() => artifact.value?.deck ?? null)
  const slideCount = computed(() => deck.value?.slides?.length ?? 0)

  /** 对话内下载卡：有演示文稿能力且未 ready；「稍后」只藏卡，可再开 */
  const showDownloadCard = computed(() => {
    if (!slidesNeeded.value) return false
    if (ready.value) return false
    if (later.value && !downloading.value && !failed.value) return false
    return true
  })

  /** 右侧预览壳：组件 ready 且有 deck（或走查 mock） */
  const showPreviewShell = computed(() => ready.value && slideCount.value > 0)

  function applySimFromQuery() {
    if (!isOfficeCanvasSim() || typeof window === 'undefined') return
    if (!import.meta.env.DEV) return
    const v = new URLSearchParams(window.location.search).get('cat_sim') ?? ''
    const flags = new Set(v.split(',').map((x) => x.trim()).filter(Boolean))
    if (!flags.has('office') && !flags.has('officeneed') && !flags.has('officedl') && !flags.has('officefail') && !flags.has('officepreview') && !flags.has('officeready')) {
      return
    }
    slidesNeeded.value = true
    later.value = false
    if (flags.has('officedl')) {
      setOfficeSimStatus({
        state: 'downloading',
        canDownload: true,
        downloadBytes: 16 * 1024 * 1024,
        receivedBytes: Math.round(16 * 1024 * 1024 * 0.58),
        phase: 'downloading',
        error: null,
      })
    } else if (flags.has('officefail')) {
      setOfficeSimStatus({
        state: 'failed',
        canDownload: true,
        downloadBytes: 16 * 1024 * 1024,
        error: { code: 'LANG_DOWNLOAD_FAILED', message: C.failedUnconfigured, detail: 'reason=not_configured' },
      })
    } else if (flags.has('officepreview') || flags.has('officeready')) {
      setOfficeSimStatus({
        state: 'ready',
        source: 'downloaded',
        version: '0.0.0-sim',
        canDownload: true,
        downloadBytes: 16 * 1024 * 1024,
        error: null,
      })
      // 直接写入 artifact（init 时 watcher 尚未挂上，不能只靠 emit）
      artifact.value = {
        kind: 'slides',
        id: 'art-slides-1',
        convId: 'sim-conv',
        status: flags.has('officepreview') ? 'streaming' : 'ready',
        deck: {
          title: MOCK_OFFICE_DECK.title,
          slides: MOCK_OFFICE_DECK.slides.slice(0, 3),
        },
      }
      emitOfficeSimArtifact({
        status: artifact.value.status,
        deck: artifact.value.deck,
      })
    } else {
      // officeneed / office：需下载
      setOfficeSimStatus({
        state: 'missing',
        canDownload: true,
        downloadBytes: 16 * 1024 * 1024,
        error: null,
      })
    }
    status.value = peekOfficeSimStatus()
  }

  async function init() {
    applySimFromQuery()
    if (!offStatus) {
      offStatus = watchOfficeCanvas((s) => {
        status.value = s
      })
    }
    if (!offProg) {
      offProg = watchOfficeCanvasProgress((p) => {
        if (p.phase === 'downloading' || p.phase === 'preparing') {
          status.value = {
            ...status.value,
            state: p.phase === 'preparing' ? 'preparing' : 'downloading',
            phase: p.phase,
            receivedBytes: p.receivedBytes,
            downloadBytes: p.totalBytes || status.value.downloadBytes,
          }
        }
      })
    }
    if (!offArt) {
      offArt = watchOfficeArtifact((a) => {
        artifactsByConv.set(a.convId, a)
        if (selectedConv.value && selectedConv.value !== a.convId) return
        artifact.value = a
        slidesNeeded.value = true
        if (later.value && status.value.state !== 'ready') {
          // 新 artifact 再出现时允许重新露出卡（用户可点下载）
          later.value = false
          writeLater(false)
        }
      })
    }
    if (!isOfficeCanvasSim() || !slidesNeeded.value) {
      status.value = await getOfficeCanvasStatus()
    } else {
      status.value = peekOfficeSimStatus()
    }
    loaded.value = true
  }

  function markSlidesNeeded() {
    slidesNeeded.value = true
  }

  function dismissLater() {
    later.value = true
    writeLater(true)
  }

  function reopenCard() {
    later.value = false
    writeLater(false)
  }

  async function install() {
    if (busy.value) return
    busy.value = true
    later.value = false
    writeLater(false)
    try {
      await installOfficeCanvas()
      status.value = await getOfficeCanvasStatus()
    } catch (e) {
      const err = toAppError(e)
      status.value = {
        ...status.value,
        state: 'failed',
        error: { code: err.code, message: err.message, detail: err.detail },
      }
    } finally {
      busy.value = false
    }
  }

  async function cancel() {
    busy.value = true
    try {
      await cancelOfficeCanvasInstall()
      status.value = await getOfficeCanvasStatus()
    } catch (e) {
      console.error('CancelOfficeCanvasInstall', toAppError(e).message)
    } finally {
      busy.value = false
    }
  }

  async function recheck() {
    busy.value = true
    try {
      status.value = await recheckOfficeCanvas()
    } finally {
      busy.value = false
    }
  }

  async function exportPptx() {
    if (!artifact.value || exportBusy.value) return
    exportBusy.value = true
    exportOk.value = ''
    try {
      const r = await exportOfficeCanvasPptx({
        convId: artifact.value.convId,
        artifactId: artifact.value.id,
      })
      exportOk.value = r.path.replace(/^.*[/\\]/, '') || '演示文稿.pptx'
      ElMessage.success(`已导出 ${exportOk.value}`)
    } catch (e) {
      const err = toAppError(e)
      const msg = err.message?.startsWith(C.exportFailPrefix) ? err.message : C.exportFailPrefix + (err.message || '未知错误')
      ElMessage.error(msg)
    } finally {
      exportBusy.value = false
    }
  }

  async function openFolder() {
    if (!ready.value) return
    try {
      await openOfficeCanvasFolder()
    } catch (e) {
      ElMessage.error(toAppError(e).message || '没能打开组件所在文件夹')
    }
  }

  /** 测试 / 走查注入 deck */
  function setMockDeck(deckIn: OfficeDeck, convId = 'sim-conv') {
    slidesNeeded.value = true
    artifact.value = {
      kind: 'slides',
      id: 'art-slides-1',
      convId,
      status: 'streaming',
      deck: deckIn,
    }
  }

  return {
    status,
    loaded,
    busy,
    later,
    slidesNeeded,
    artifact,
    exportBusy,
    exportOk,
    ready,
    downloading,
    failed,
    pct,
    deck,
    slideCount,
    showDownloadCard,
    showPreviewShell,
    init,
    markSlidesNeeded,
    dismissLater,
    reopenCard,
    install,
    cancel,
    recheck,
    exportPptx,
    openFolder,
    setMockDeck,
    selectConversation,
    copy: C,
  }
})
