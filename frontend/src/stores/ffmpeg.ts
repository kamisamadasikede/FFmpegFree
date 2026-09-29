import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { onEvent, previewParams, service } from '@/services/wails'

/** 与契约 9.4 FFmpegStatus 对齐；wailsjs 生成 models.ts 后换成生成的类型 */
export interface FFmpegStatus {
  state: 'checking' | 'ready' | 'missing' | 'outdated' | 'installing' | 'failed'
  path?: string
  version?: string
  source?: 'custom' | 'bundled' | 'system' | 'legacy'
  taskId?: string
  error?: { code: string; message: string; detail?: string } | null
}

export interface InstallProgress {
  progress: number // 0~1
  stage: 'download' | 'verify' | 'extract' | 'validate'
  speedText?: string
  remainText?: string
}

const PREVIEW: Record<string, { status: FFmpegStatus; install?: InstallProgress }> = {
  ready: { status: { state: 'ready', version: '7.1', source: 'bundled' } },
  missing: { status: { state: 'missing' } },
  installing: {
    status: { state: 'installing' },
    install: { progress: 0.42, stage: 'download', speedText: '6.1 MB/s', remainText: '剩余 48 MB' },
  },
  failed: { status: { state: 'failed', error: { code: 'IO_ERROR', message: '下载超时，请检查网络' } } },
}

export const useFFmpegStore = defineStore('ffmpeg', () => {
  const status = ref<FFmpegStatus>({ state: 'checking' })
  const install = ref<InstallProgress | null>(null)
  const promptDismissed = ref(false)
  const bannerClosed = ref(false) // 只隐藏本次会话
  const dialogOpen = ref(false)
  const justBecameReady = ref(false)

  const ready = computed(() => status.value.state === 'ready')
  const needsAttention = computed(() => !['ready', 'checking'].includes(status.value.state))

  function setStatus(next: FFmpegStatus) {
    const was = status.value.state
    status.value = next
    if (next.state === 'ready' && was !== 'ready' && was !== 'checking') {
      justBecameReady.value = true
      setTimeout(() => (justBecameReady.value = false), 3000)
    }
    if (next.state !== 'installing') install.value = null
  }

  async function init() {
    const preview = previewParams.get('ff')
    if (preview && PREVIEW[preview]) {
      setStatus(PREVIEW[preview].status)
      install.value = PREVIEW[preview].install ?? null
      dialogOpen.value = previewParams.has('dlg')
      return
    }
    onEvent<FFmpegStatus>('ffmpeg:status', setStatus)
    const svc = service('SystemService')
    if (!svc?.GetFFmpegStatus) return // 后端 SystemService 还没接上，保持 checking
    setStatus(await svc.GetFFmpegStatus())
    const settings = await svc.GetSettings?.()
    promptDismissed.value = !!settings?.ffmpegPromptDismissed
    if (status.value.state === 'missing' && !promptDismissed.value) dialogOpen.value = true
  }

  async function startInstall(mirror = '') {
    const svc = service('SystemService')
    if (!svc?.InstallFFmpeg) throw new Error('SystemService 尚未就绪')
    const task = await svc.InstallFFmpeg(mirror)
    setStatus({ ...status.value, state: 'installing', taskId: task.id })
  }

  async function pickPath() {
    const svc = service('SystemService')
    if (!svc?.PickDirectory || !svc?.SetFFmpegPath) throw new Error('SystemService 尚未就绪')
    const dir = await svc.PickDirectory()
    if (dir) setStatus(await svc.SetFFmpegPath(dir))
  }

  async function dismissPrompt() {
    promptDismissed.value = true
    dialogOpen.value = false
    const svc = service('SystemService')
    const s = await svc?.GetSettings?.()
    if (s) await svc!.UpdateSettings({ ...s, ffmpegPromptDismissed: true })
  }

  /** 任务 store 收到安装任务的 task:progress 时调用 */
  function updateInstall(p: InstallProgress) {
    install.value = p
  }

  return {
    status, install, promptDismissed, bannerClosed, dialogOpen, justBecameReady,
    ready, needsAttention, init, startInstall, pickPath, dismissPrompt, updateInstall,
  }
})
