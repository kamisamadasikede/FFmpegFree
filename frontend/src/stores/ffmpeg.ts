import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as SystemBinding from '../../wailsjs/go/app/SystemService'
import { system } from '../../wailsjs/go/models'
import { call } from '@/api/call'
import { hasWailsBackend, onEvent, previewParams } from '@/services/wails'
import { formatEta } from '@/utils/format'

/** 与契约 9.4 FFmpegStatus 对齐。生成的类型里 state/source 是 string、error 可能为 null，这里收窄后使用 */
export interface FFmpegStatus {
  state: 'checking' | 'ready' | 'missing' | 'outdated' | 'installing' | 'failed'
  path?: string
  version?: string
  source?: 'custom' | 'bundled' | 'system' | 'legacy'
  taskId?: string
  ffprobeMissing?: boolean
  error?: { code: string; message: string; detail?: string } | null
}

export interface InstallProgress {
  progress: number // 0~1
  stage: 'download' | 'verify' | 'extract' | 'validate'
  speedText?: string
  remainText?: string
}

/**
 * ffmpeg_install 任务的进度 → 安装对话框 / 提示条用的形态。
 * 契约 9.3：progress 0~1 覆盖整个流程，下载占 0~0.9，解压 0.9~0.94，校验 0.94~0.98，安装完成 1。
 * task.speed 后端已经格式化好（如 "3.2 MB/s"）；etaSec 为 0 表示未知。
 */
export function toInstallProgress(progress: number, speed: string, etaSec: number): InstallProgress {
  const p = Math.min(1, Math.max(0, progress))
  return {
    progress: p,
    stage: p < 0.9 ? 'download' : p < 0.94 ? 'extract' : 'validate',
    speedText: speed || undefined,
    remainText: etaSec > 0 ? `剩余 ${formatEta(etaSec)}` : undefined,
  }
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
    ffprobeMissing: !!r.ffprobeMissing,
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

/** 与契约 9.4 InstallOptions 对齐。mirrors 不含默认源（契约：「可用镜像（不含默认源）」）；defaultMirror 是后端将来可能补的字段，没有时默认源就是 "" */
export interface InstallOptions {
  platform: string
  supported: boolean
  mirrors: string[]
  defaultMirror?: string
}

// ?convert=…（转换页预览）没带 ff 时默认 ready
const previewFf = previewParams.get('ff') ?? (previewParams.has('convert') ? 'ready' : null)
const previewMode = !hasWailsBackend() && !!previewFf && !!PREVIEW[previewFf]

export const useFFmpegStore = defineStore('ffmpeg', () => {
  const status = ref<FFmpegStatus>({ state: 'checking' })
  const install = ref<InstallProgress | null>(null)
  const promptDismissed = ref(false)
  const bannerClosed = ref(false) // 只隐藏本次会话
  const dialogOpen = ref(false)
  const justBecameReady = ref(false)
  const manualInputOpen = ref(false) // 没有目录选择器时，对话框里显示路径输入框

  /** 安装功能可用（绑定已有 InstallFFmpeg）；只有浏览器预览里 ?noinstall 会关闭，用来看"即将上线"样式 */
  const installOptions = ref<InstallOptions | null>(null)
  /** 默认下载源（GetInstallOptions 返回；契约里默认源是 ""）。不写死镜像名 */
  const defaultMirror = computed(() => installOptions.value?.defaultMirror ?? '')
  /**
   * 可选下载源：默认源在前，后面是 GetInstallOptions().mirrors（去重）。
   * 契约的 mirrors 不含默认源，所以 Windows（有 cn 镜像）是 2 个来源、macOS / Linux 只有 1 个。
   */
  const sources = computed(() => {
    const o = installOptions.value
    return o ? [...new Set([defaultMirror.value, ...(o.mirrors ?? [])])] : [defaultMirror.value]
  })
  /** 有可切换的下载源（sources.length > 1）：失败行才显示「换下载源重试」 */
  const canSwitchMirror = computed(() => sources.value.length > 1)

  async function loadInstallOptions() {
    if (previewMode) {
      // ?mirrors=2 → 有一个镜像（两个来源，Windows 形态）；不带则只有默认源
      installOptions.value = { platform: 'preview', supported: true, mirrors: previewParams.get('mirrors') === '2' ? ['cn'] : [] }
      return
    }
    if (!hasWailsBackend()) return
    try {
      const o = await call(SystemBinding.GetInstallOptions())
      installOptions.value = { platform: o?.platform ?? '', supported: !!o?.supported, mirrors: o?.mirrors ?? [], defaultMirror: (o as any)?.defaultMirror || undefined }
    } catch (e) {
      console.error('GetInstallOptions failed', e) // 拿不到就按只有默认源处理
    }
  }

  const installAvailable = !(previewMode && previewParams.has('noinstall'))
  /** 有系统目录选择器（PickDirectory 已在绑定里）；只有浏览器预览里 ?nopicker 会关闭，用来看"手动输入路径"的样式 */
  const canPickDirectory = !(previewMode && previewParams.has('nopicker'))

  const ready = computed(() => status.value.state === 'ready')
  const needsAttention = computed(() => !['ready', 'checking'].includes(status.value.state))
  /**
   * 依赖 ffmpeg 的入口（转换/剪辑/直播）是否置灰：只看 ffmpeg 当前状态（missing/outdated/installing/failed）。
   * 刻意不看 promptDismissed / bannerClosed——用户点「稍后」只是不再打扰，不代表 ffmpeg 可用。
   * checking（启动检测中）不置灰，避免每次启动闪一下灰。
   */
  const featuresBlocked = computed(() => needsAttention.value)
  /**
   * 安装对话框是否显示：用户/守卫要求打开（dialogOpen）且 ffmpeg 确实需要处理。
   * 已经 ready（含安装刚完成）时无论 dialogOpen 是什么都不显示，不会再冒出"需要安装 ffmpeg"。
   */
  const dialogVisible = computed(() => dialogOpen.value && needsAttention.value)

  // 路由守卫用：首次导航时状态可能还在 checking，守卫要等它出结果（或超时）再判断
  let settleWaiters: Array<() => void> = []
  /** 是否会有真实（或预览）状态到来；浏览器里没有后端也没有 ?ff= 时状态永远是 checking，不该等 */
  const statusExpected = hasWailsBackend() || previewMode
  /** 等状态离开 checking，最多等 timeoutMs；超时也 resolve（守卫按当时的状态放行，不卡住导航） */
  function whenSettled(timeoutMs = 1500): Promise<void> {
    if (status.value.state !== 'checking' || !statusExpected) return Promise.resolve()
    return new Promise((resolve) => {
      const done = () => {
        clearTimeout(timer)
        resolve()
      }
      const timer = setTimeout(done, timeoutMs)
      settleWaiters.push(done)
    })
  }

  function setStatus(next: FFmpegStatus) {
    const was = status.value.state
    status.value = next
    if (next.state !== 'checking' && settleWaiters.length) {
      const ws = settleWaiters
      settleWaiters = []
      ws.forEach((w) => w())
    }
    if (next.state === 'ready') {
      // 契约 9.5：ready 后后端重置 ffmpegPromptDismissed，本地保持一致
      promptDismissed.value = false
      promptShown = false
      // 可用了就不该再有"需要安装"对话框：安装对话框在 installing 时也是开着的（进度视图），
      // ready 后 dialogOpen 若还是 true，installing 视图一消失就会退回"需要安装 ffmpeg / 下载"视图，等于让用户重复下载。
      // 手动指定路径成功（pickPath）走的也是这里。
      dialogOpen.value = false
      manualInputOpen.value = false
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
      const preview = previewFf
      if (preview && PREVIEW[preview]) {
        setStatus(PREVIEW[preview].status)
        install.value = PREVIEW[preview].install ?? null
        dialogOpen.value = previewParams.has('dlg')
        await loadInstallOptions()
      }
      return // 浏览器里没有后端，保持 checking
    }

    // 先订阅再查询：查询期间到达的事件一定比查询结果新
    onEvent<system.FFmpegStatus>('ffmpeg:status', applyEvent)
    loadInstallOptions() // 不阻塞状态查询
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

  let lastMirror: string | null = null // 上次实际使用的下载源；null = 还没装过，用默认源
  /**
   * 安装：调用 InstallFFmpeg(mirror)。mirror 省略 = 上次用的源（第一次是 GetInstallOptions 的默认源）；
   * 只能传 '' 或 GetInstallOptions().mirrors 里的名字（后端对其他值返回 INVALID_ARGUMENT）。
   * 幂等：已有进行中的安装时后端直接返回该任务。进度由任务 store 通过 updateInstall() 推进。
   * 预览模式（浏览器里没有 window.go）只切到 installing 的静态样子。
   */
  async function startInstall(mirror?: string) {
    if (!installAvailable) return
    if (previewMode) {
      setStatus(PREVIEW.installing.status)
      install.value = PREVIEW.installing.install!
      return
    }
    if (!hasWailsBackend()) return
    if (!installOptions.value) await loadInstallOptions()
    if (mirror !== undefined) lastMirror = mirror
    const use = lastMirror ?? defaultMirror.value
    lastMirror = use
    const seqAtStart = eventSeq
    const task = await call(SystemBinding.InstallFFmpeg(use))
    // 后端随后会推 ffmpeg:status(installing)；这里先本地切换，避免按钮空档。
    // 调用返回前已经收到过状态事件（例如安装已经完成推了 ready），说明事件比这个返回值新，不能再把状态改回 installing。
    if (eventSeq !== seqAtStart) return
    setStatus({ ...status.value, state: 'installing', taskId: task?.id })
    install.value = toInstallProgress(task?.progress ?? 0, task?.speed ?? '', task?.etaSec ?? 0)
  }

  /** 换另一个下载源重试（仅 canSwitchMirror 时有意义）：在 sources 里取上次用的源之后的下一个 */
  async function retryWithOtherMirror() {
    const list = sources.value
    const cur = lastMirror ?? defaultMirror.value
    const next = list[(Math.max(0, list.indexOf(cur)) + 1) % list.length]
    await startInstall(next)
  }

  /** 取消进行中的安装（保留已下载部分）；没有安装在进行时后端无操作。取消后后端会重新检测并推 ffmpeg:status */
  async function cancelInstall() {
    if (previewMode) {
      setStatus(PREVIEW.missing.status)
      return
    }
    if (!hasWailsBackend()) return
    await call(SystemBinding.CancelFFmpegInstall())
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
      dir = await call<string>(SystemBinding.PickDirectory('选择 ffmpeg 所在文件夹'))
      if (!dir) return // 用户取消
    }
    if (previewMode) return
    const seqAtStart = eventSeq
    const st = normalize(await call(SystemBinding.SetFFmpegPath(dir)))
    // SetFFmpegPath 成功时后端同时推 ffmpeg:status(ready)，事件已应用过就不用再用返回值覆盖
    if (eventSeq === seqAtStart) setStatus(st)
  }

  /** 清除手动指定并重新检测（SetFFmpegPath('')） */
  async function clearCustomPath() {
    if (previewMode) return
    const seqAtStart = eventSeq
    const st = normalize(await call(SystemBinding.SetFFmpegPath('')))
    if (eventSeq === seqAtStart) setStatus(st)
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
    installAvailable, canPickDirectory, manualInputOpen, installOptions, sources, canSwitchMirror,
    ready, needsAttention, featuresBlocked, dialogVisible, whenSettled, init, startInstall, retryWithOtherMirror, cancelInstall, pickPath, clearCustomPath, recheck, dismissPrompt, updateInstall,
  }
})
