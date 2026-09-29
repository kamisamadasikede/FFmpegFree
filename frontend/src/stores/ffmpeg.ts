import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as SystemBinding from '../../wailsjs/go/app/SystemService'
import { system } from '../../wailsjs/go/models'
import { call } from '@/api/call'
import { hasWailsBackend, onEvent, previewParams } from '@/services/wails'

/** 与契约 9.4 FFmpegStatus 对齐。生成的类型里 state/source 是 string、error 可能为 null，这里收窄后使用 */
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

/** 后端事件 / 调用返回的原始状态 → 前端状态（空串字段归一为 undefined） */
function normalize(raw: system.FFmpegStatus | FFmpegStatus): FFmpegStatus {
  const r = raw as any
  return {
    state: r.state,
    path: r.path || undefined,
    version: r.version || undefined,
    source: r.source || undefined,
    taskId: r.taskId || undefined,
    error: r.error ? { code: r.error.code, message: r.error.message, detail: r.error.detail } : null,
  }
}

// 预览模式（仅浏览器里没有 window.go 时）：纯界面模拟，不代表真实状态
const PREVIEW: Record<string, { status: FFmpegStatus; install?: InstallProgress }> = {
  ready: { status: { state: 'ready', version: '7.1', source: 'bundled' } },
  missing: { status: { state: 'missing' } },
  installing: {
    status: { state: 'installing' },
    install: { progress: 0.42, stage: 'download', speedText: '6.1 MB/s', remainText: '剩余 48 MB' },
  },
  failed: { status: { state: 'failed', error: { code: 'IO_ERROR', message: '下载超时，请检查网络' } } },
}

// 生成的绑定里可能还没有的方法（InstallFFmpeg / PickDirectory）：用命名空间对象探测，
// 后端补上并重新生成绑定后自动生效，不需要改这里。
type OptionalFn = ((...args: any[]) => Promise<any>) | undefined
const optional = SystemBinding as unknown as Record<string, OptionalFn>
const previewMode = !hasWailsBackend() && previewParams.has('ff') && !!PREVIEW[previewParams.get('ff')!]

export const useFFmpegStore = defineStore('ffmpeg', () => {
  const status = ref<FFmpegStatus>({ state: 'checking' })
  const install = ref<InstallProgress | null>(null)
  const promptDismissed = ref(false)
  const bannerClosed = ref(false) // 只隐藏本次会话
  const dialogOpen = ref(false)
  const justBecameReady = ref(false)
  const manualInputOpen = ref(false) // 没有目录选择器时，对话框里显示路径输入框

  /** 后端有 InstallFFmpeg（预览模式下用 ?noinstall 关闭）；false 时界面显示"安装功能即将上线" */
  const installAvailable = previewMode
    ? !previewParams.has('noinstall')
    : typeof optional.InstallFFmpeg === 'function'
  /** 后端有 PickDirectory；false 时手动指定路径改为文本框 */
  const canPickDirectory = previewMode
    ? !previewParams.has('nopicker')
    : typeof optional.PickDirectory === 'function'

  const ready = computed(() => status.value.state === 'ready')
  const needsAttention = computed(() => !['ready', 'checking'].includes(status.value.state))

  function setStatus(next: FFmpegStatus) {
    const was = status.value.state
    status.value = next
    if (next.state === 'ready') {
      // 契约 9.5：ready 后后端重置 ffmpegPromptDismissed，本地保持一致
      promptDismissed.value = false
      promptShown = false
      if (was !== 'ready' && was !== 'checking') {
        justBecameReady.value = true
        setTimeout(() => (justBecameReady.value = false), 3000)
      }
    }
    if (next.state !== 'installing') install.value = null
  }

  // 首次 missing 且用户没选过"稍后"时弹一次确认框；需要等设置读完才能判断
  let settingsLoaded = false
  let promptShown = false
  function maybePrompt() {
    if (!settingsLoaded || promptShown || promptDismissed.value) return
    if (status.value.state === 'missing') {
      promptShown = true
      dialogOpen.value = true
    }
  }

  let eventSeq = 0 // 每收到一次 ffmpeg:status 事件加一，用来丢弃过期的主动查询结果
  function applyEvent(raw: system.FFmpegStatus) {
    eventSeq++
    setStatus(normalize(raw))
    maybePrompt()
  }

  async function init() {
    if (!hasWailsBackend()) {
      const preview = previewParams.get('ff')
      if (preview && PREVIEW[preview]) {
        setStatus(PREVIEW[preview].status)
        install.value = PREVIEW[preview].install ?? null
        dialogOpen.value = previewParams.has('dlg')
      }
      return // 浏览器里没有后端，保持 checking
    }

    // 先订阅再查询：查询期间到达的事件一定比查询结果新
    onEvent<system.FFmpegStatus>('ffmpeg:status', applyEvent)
    try {
      const seqAtStart = eventSeq
      const st = await call(SystemBinding.GetFFmpegStatus())
      if (eventSeq === seqAtStart) setStatus(normalize(st))
    } catch (e) {
      console.error('GetFFmpegStatus failed', e)
    }
    try {
      const s = await call(SystemBinding.GetSettings())
      promptDismissed.value = !!s?.ffmpegPromptDismissed
    } catch (e) {
      console.error('GetSettings failed', e)
    }
    settingsLoaded = true
    maybePrompt()
  }

  /** 安装：后端没有 InstallFFmpeg 时不做任何事（界面已禁用），预览模式只切到 installing 的静态样子 */
  async function startInstall(mirror = '') {
    if (!installAvailable) return
    if (previewMode) {
      setStatus(PREVIEW.installing.status)
      install.value = PREVIEW.installing.install!
      return
    }
    const task = await call(optional.InstallFFmpeg!(mirror))
    // 后端随后会推 ffmpeg:status(installing)；这里先本地切换，避免按钮空档
    setStatus({ ...status.value, state: 'installing', taskId: task?.id })
  }

  /** 手动指定目录。dir 省略时用系统目录选择器（PickDirectory）；没有选择器时调用方必须传 dir */
  async function pickPath(dir?: string) {
    if (dir === undefined) {
      if (!canPickDirectory) {
        manualInputOpen.value = true // 没有选择器：打开对话框里的文本框
        dialogOpen.value = true
        return
      }
      if (previewMode) return
      dir = await call<string>(optional.PickDirectory!())
      if (!dir) return // 用户取消
    }
    if (previewMode) return
    setStatus(normalize(await call(SystemBinding.SetFFmpegPath(dir))))
  }

  /** 清除手动指定并重新检测（SetFFmpegPath('')） */
  async function clearCustomPath() {
    if (previewMode) return
    setStatus(normalize(await call(SystemBinding.SetFFmpegPath(''))))
  }

  /** 重新检测：后端先推 checking 再推结果，返回值就是最终结果 */
  async function recheck() {
    if (previewMode) return
    const seqAtStart = eventSeq
    const st = await call(SystemBinding.RecheckFFmpeg())
    if (eventSeq === seqAtStart) setStatus(normalize(st))
  }

  async function dismissPrompt() {
    promptDismissed.value = true
    dialogOpen.value = false
    if (previewMode || !hasWailsBackend()) return
    const s = await call(SystemBinding.GetSettings())
    await call(SystemBinding.UpdateSettings(system.Settings.createFrom({ ...s, ffmpegPromptDismissed: true })))
  }

  /** 任务 store 收到安装任务的 task:progress 时调用 */
  function updateInstall(p: InstallProgress) {
    install.value = p
  }

  return {
    status, install, promptDismissed, bannerClosed, dialogOpen, justBecameReady,
    installAvailable, canPickDirectory, manualInputOpen,
    ready, needsAttention, init, startInstall, pickPath, clearCustomPath, recheck, dismissPrompt, updateInstall,
  }
})
