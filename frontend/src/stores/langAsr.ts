import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  exportSubtitleCues,
  getAsrTier,
  getLangAsrStatus,
  mockAsrOutcome,
  MOCK_CUES,
  setAsrTier,
  validateCues,
  type AsrTier,
  type LangAsrStatus,
  type SubtitleCue,
} from '@/api/lang'
import { pickFiles, MEDIA_FILE_FILTER } from '@/api/system'
import { toAppError } from '@/api/call'
import { ASR_EMPTY, ASR_FAIL, EXPORT_BLOCK_BODY, EXPORT_BLOCK_TITLE } from '@/utils/langText'
import { previewParams } from '@/services/wails'

export type VoicePhase = 'idle' | 'running' | 'done' | 'empty' | 'fail'

export const useLangAsrStore = defineStore('langAsr', () => {
  const status = ref<LangAsrStatus>({
    state: 'missing',
    version: '',
    source: '',
    tier: 'standard',
    canDownload: false,
    downloadBytes: 0,
    installBytes: 0,
    error: null,
  })
  const tier = ref<AsrTier>('standard')
  const loaded = ref(false)
  const filePath = ref('')
  const fileName = ref('')
  const phase = ref<VoicePhase>('idle')
  const progress = ref(0)
  const cues = ref<SubtitleCue[]>([])
  const exportFormat = ref<'srt' | 'vtt'>('srt')
  const exportError = ref('')
  const exportOk = ref('')
  const busy = ref(false)
  let runToken = 0

  /** 未发布：missing 且 canDownload=false（落地稿 01） */
  const unpublished = computed(() => status.value.state === 'missing' && status.value.canDownload === false)
  /** 浏览器走查：?voice=demo 允许用假 cues 演示时间轴（组件仍显示未发布卡） */
  const demoTimeline = computed(() => previewParams.get('voice') === 'demo')
  const componentMissing = computed(() => status.value.state === 'missing' || status.value.state === 'checking')
  const canDownload = computed(() => status.value.canDownload === true)
  const hasFile = computed(() => !!filePath.value)
  const canPick = computed(() => (!unpublished.value || demoTimeline.value) && phase.value !== 'running')
  const canGenerate = computed(
    () => hasFile.value && phase.value !== 'running' && !busy.value && (!unpublished.value || demoTimeline.value),
  )
  const canExport = computed(() => phase.value === 'done' && cues.value.length > 0 && !busy.value)

  async function init() {
    tier.value = await getAsrTier()
    status.value = await getLangAsrStatus()
    loaded.value = true
  }

  async function refreshStatus() {
    status.value = await getLangAsrStatus()
    tier.value = status.value.tier
  }

  /** 切换档位只改引导体积，不触发下载 */
  async function changeTier(next: AsrTier) {
    if (next === tier.value) return
    await setAsrTier(next)
    tier.value = next
    await refreshStatus()
  }

  function baseName(path: string): string {
    const n = path.replace(/\\/g, '/').split('/').pop() ?? path
    return n.replace(/\.[^.]+$/, '') || '字幕'
  }

  function setFile(path: string) {
    runToken++
    filePath.value = path
    fileName.value = path.replace(/\\/g, '/').split('/').pop() ?? path
    phase.value = 'idle'
    progress.value = 0
    cues.value = []
    exportError.value = ''
    exportOk.value = ''
  }

  function clearFile() {
    runToken++
    filePath.value = ''
    fileName.value = ''
    phase.value = 'idle'
    progress.value = 0
    cues.value = []
    exportError.value = ''
    exportOk.value = ''
  }

  async function chooseFile() {
    busy.value = true
    try {
      const paths = await pickFiles(MEDIA_FILE_FILTER, false)
      if (paths[0]) setFile(paths[0])
    } catch (e) {
      console.error('选择文件失败', toAppError(e).message)
    } finally {
      busy.value = false
    }
  }

  function addPaths(paths: string[]) {
    if (paths[0]) setFile(paths[0])
  }

  async function generate() {
    if (!filePath.value || phase.value === 'running') return
    const token = ++runToken
    phase.value = 'running'
    progress.value = 0
    cues.value = []
    exportError.value = ''
    exportOk.value = ''
    busy.value = true
    const outcome = mockAsrOutcome(filePath.value)
    // 假进度：约 1.2s
    const steps = 8
    for (let i = 1; i <= steps; i++) {
      await new Promise((r) => setTimeout(r, 140))
      if (token !== runToken) return
      progress.value = Math.round((i / steps) * 100)
    }
    if (token !== runToken) return
    busy.value = false
    if (outcome === 'empty') {
      phase.value = 'empty'
      return
    }
    if (outcome === 'fail') {
      phase.value = 'fail'
      return
    }
    // 组件未发布：仍用假 cues 演示时间轴
    cues.value = MOCK_CUES.map((c) => ({ ...c }))
    phase.value = 'done'
    progress.value = 100
  }

  function retryGenerate() {
    void generate()
  }

  function updateCue(id: string, patch: Partial<Pick<SubtitleCue, 'text' | 'startMs' | 'endMs'>>) {
    const i = cues.value.findIndex((c) => c.id === id)
    if (i < 0) return
    const next = { ...cues.value[i], ...patch }
    cues.value = cues.value.map((c, idx) => (idx === i ? next : c))
    exportError.value = ''
    exportOk.value = ''
  }

  async function exportCues() {
    exportError.value = ''
    exportOk.value = ''
    const v = validateCues(cues.value)
    if (!v.ok) {
      exportError.value = `${EXPORT_BLOCK_TITLE}。${EXPORT_BLOCK_BODY}`
      return
    }
    busy.value = true
    try {
      const { fileName: out } = await exportSubtitleCues(cues.value, exportFormat.value, baseName(filePath.value))
      exportOk.value = `已导出“${out}”。`
    } catch {
      exportError.value = `${EXPORT_BLOCK_TITLE}。${EXPORT_BLOCK_BODY}`
    } finally {
      busy.value = false
    }
  }

  return {
    status,
    tier,
    loaded,
    filePath,
    fileName,
    phase,
    progress,
    cues,
    exportFormat,
    exportError,
    exportOk,
    busy,
    unpublished,
    demoTimeline,
    componentMissing,
    canDownload,
    hasFile,
    canPick,
    canGenerate,
    canExport,
    init,
    refreshStatus,
    changeTier,
    setFile,
    clearFile,
    chooseFile,
    addPaths,
    generate,
    retryGenerate,
    updateCue,
    exportCues,
    emptyMessage: ASR_EMPTY,
    failMessage: ASR_FAIL,
  }
})
