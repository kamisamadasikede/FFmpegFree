import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import { store as goStore } from '../../wailsjs/go/models'
import { call, toAppError } from '@/api/call'
import { listPresets, MAX_SUBMIT, parseConvertParams, resubmitToDir, submitConvert, type PresetItem } from '@/api/convert'
import { probeFiles, thumbnailOf } from '@/api/media'
import { canPickFiles, pickDirectory, pickFiles, revealInFolder, getDefaultOutputDir } from '@/api/system'
import { hasWailsBackend, previewParams } from '@/services/wails'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { normalizeTask, useTaskStore, type TaskError, type TaskItem, type TaskStatus } from '@/stores/tasks'
import { dirName, fileBaseName } from '@/utils/format'
import { codecName } from '@/utils/mediaText'
import { actionErrorText } from '@/errors/errorMessages'

/** 文件行在页面上的状态（探测 → 待转换 → 排队 / 转换中 → 结果） */
export type RowState = 'waiting' | 'probing' | 'invalid' | 'conflict' | 'ready' | 'queued' | 'running' | 'succeeded' | 'failed' | 'interrupted' | 'canceled'
export type PageMode = 'idle' | 'ready' | 'running' | 'done' | 'failed'

export interface RowError { code: string; message: string; detail?: string }

export interface ConvertRow {
  key: string
  path: string
  name: string
  /** 探测阶段：waiting = 还没探测（ffmpeg 未就绪，就绪后自动探测） */
  probe: 'waiting' | 'probing' | 'ok' | 'error'
  info?: goStore.MediaInfo
  probeError?: RowError
  thumb: string
  thumbState: 'idle' | 'loading' | 'done'
  /** 信息卡用的大封面（宽 640）；选中 / 单文件时才取 */
  cover: string
  coverState: 'idle' | 'loading' | 'done'
  /** 提交后对应的任务 id（按 Submit 返回顺序绑定） */
  taskId: string
  /** 提交时的预设短名快照，用来显示「转为 MP4」；提交前跟随当前所选预设 */
  label: string
  /** 整体校验失败时后端 detail 第一行指向这个文件 */
  submitError?: RowError
}

/** 文件行需要的任务字段（活动任务和终态快照的公共部分） */
export interface RowTask {
  status: TaskStatus
  progress: number
  speed: string
  etaSec: number
  error?: TaskError | null
  outputPath: string
  /** 开始 / 结束时间（ms）：完成态“用时”由前端用它们相减；0 或缺省 = 未知 */
  startedAt?: number
  finishedAt?: number
}

const VIDEO_CONTAINERS = ['mp4', 'mkv', 'mov', 'webm', 'avi', 'flv', 'gif']
const AUDIO_CONTAINERS = ['mp3', 'aac', 'm4a', 'wav', 'flac', 'ogg', 'opus']

/** 预设名「MP4（H.264 + AAC，通用）」→ 标题 MP4 + 说明 H.264 + AAC，通用 */
export function splitPresetName(name: string): { title: string; sub: string } {
  const m = name.match(/^(.*?)[（(](.*)[）)]\s*$/)
  return m ? { title: m[1].trim(), sub: m[2].trim() } : { title: name, sub: '' }
}

/**
 * 每一轮 probePending 最多取多少个还没读取的行去探测。列表本身最多 MAX_SUBMIT（50）个，所以这里只是兜底上限，
 * 与列表上限保持一致；分成小批（PROBE_BATCH）回调由 probeFiles 负责。
 */
const PROBE_ROUND_MAX = MAX_SUBMIT

let seq = 0
const newKey = () => `cf${Date.now().toString(36)}${(seq++).toString(36)}`

export const useConvertStore = defineStore('convert', () => {
  const tasks = useTaskStore()
  const ffmpeg = useFFmpegStore()

  const rows = ref<ConvertRow[]>([])
  const presets = ref<PresetItem[]>([])
  const presetsLoaded = ref(false)
  const presetsError = ref<RowError | null>(null)
  const selectedPresetId = ref('')
  /** 「保存到」：本次提交用的文件夹；'' = 用设置里的默认输出位置（或源文件夹），不改设置 */
  const outputOverride = ref('')
  const defaultOutputDir = ref('')
  const submitting = ref(false)
  /** 没有对应到具体文件的提交错误 */
  const submitError = ref<RowError | null>(null)
  const notice = ref('')
  const pickSoon = ref('')

  // ---------------- 预设 ----------------
  const selectedPreset = computed(() => presets.value.find((p) => p.id === selectedPresetId.value))
  /**
   * 预设标题：名字里括号前的部分。标题重名（「MP4（H.264…）」和「MP4（H.265…）」都是 MP4）时追加编码简称，
   * 如「MP4 · H.265」；行上的「转为 X」、页脚提示都用这个标题。
   */
  const presetTitles = computed(() => {
    const count = new Map<string, number>()
    for (const p of presets.value) {
      const t = splitPresetName(p.name).title
      count.set(t, (count.get(t) ?? 0) + 1)
    }
    const m = new Map<string, string>()
    for (const p of presets.value) {
      const t = splitPresetName(p.name).title
      const o = p.options
      const codec = (count.get(t) ?? 0) > 1 ? codecName(o.videoCodec || o.audioCodec) : ''
      m.set(p.id, codec ? `${t} · ${codec}` : t)
    }
    return m
  })
  const presetTitle = (p: PresetItem) => presetTitles.value.get(p.id) ?? splitPresetName(p.name).title
  const presetShort = computed(() => (selectedPreset.value ? presetTitle(selectedPreset.value) : ''))

  async function loadPresets() {
    try {
      presets.value = await listPresets()
      presetsError.value = null
      if (!selectedPreset.value && presets.value.length) selectedPresetId.value = presets.value[0].id
    } catch (e) {
      const err = toAppError(e)
      presetsError.value = { code: err.code, message: err.message, detail: err.detail }
    } finally {
      presetsLoaded.value = true
    }
  }
  async function loadDefaultDir() {
    try {
      defaultOutputDir.value = await getDefaultOutputDir()
    } catch (e) {
      console.warn('read default output dir failed', e)
    }
  }
  let inited = false
  async function init() {
    if (inited) return
    inited = true
    await Promise.all([loadPresets(), loadDefaultDir()])
  }
  /** 回到页面时刷新默认输出位置（用户可能在设置里改过） */
  function refreshDefaultDir() {
    if (inited) void loadDefaultDir()
  }

  // ---------------- 行状态 ----------------
  function rowTask(r: ConvertRow): RowTask | undefined {
    if (!r.taskId) return undefined
    return tasks.taskById(r.taskId)
  }

  /** 当前预设与文件不兼容的原因（探测成功后才判断），null = 兼容 */
  function conflictOf(r: ConvertRow): string | null {
    if (r.probe !== 'ok' || !r.info || !selectedPreset.value) return null
    const c = selectedPreset.value.options.container
    if (VIDEO_CONTAINERS.includes(c) && r.info.hasVideo === false) return '这个文件没有画面，不能转成视频格式。请换一个音频预设，或移出列表。'
    if (AUDIO_CONTAINERS.includes(c) && r.info.hasAudio === false) return '这个文件没有音轨，不能转成音频格式。请换一个视频预设，或移出列表。'
    return null
  }

  function stateOf(r: ConvertRow): RowState {
    const t = rowTask(r)
    if (t) return t.status
    if (r.probe === 'waiting') return 'waiting'
    if (r.probe === 'probing') return 'probing'
    if (r.probe === 'error') return 'invalid'
    if (conflictOf(r)) return 'conflict'
    return 'ready'
  }

  const ACTIVE: RowState[] = ['queued', 'running']
  const isActive = (s: RowState) => ACTIVE.includes(s)
  const isFailed = (s: RowState) => s === 'failed' || s === 'interrupted'

  const states = computed(() => rows.value.map((r) => ({ r, s: stateOf(r) })))
  const activeRows = computed(() => states.value.filter((x) => isActive(x.s)).map((x) => x.r))
  /** 还没提交的行（含探测失败的） */
  const pendingRows = computed(() => states.value.filter((x) => !x.r.taskId || tasks.wasRemoved(x.r.taskId)).map((x) => x.r))
  const submittableRows = computed(() => states.value.filter((x) => x.s === 'ready').map((x) => x.r))
  const blockedCount = computed(() => states.value.filter((x) => x.s === 'invalid' || x.s === 'conflict').length)
  const canceledCount = computed(() => states.value.filter((x) => x.s === 'canceled').length)
  const busyProbing = computed(() => states.value.some((x) => x.s === 'probing' || x.s === 'waiting'))

  const mode = computed<PageMode>(() => {
    if (!rows.value.length) return 'idle'
    const ss = states.value.map((x) => x.s)
    if (ss.some(isActive)) return 'running'
    if (ss.some((s) => s === 'ready' || s === 'probing' || s === 'waiting' || s === 'invalid' || s === 'conflict')) return 'ready'
    if (ss.some((s) => isFailed(s))) return 'failed'
    if (ss.every((s) => s === 'succeeded' || s === 'canceled')) return ss.some((s) => s === 'succeeded') ? 'done' : 'ready'
    return 'ready'
  })

  const totalBytes = computed(() => rows.value.reduce((n, r) => n + (r.info?.size ?? 0), 0))
  const succeededRows = computed(() => states.value.filter((x) => x.s === 'succeeded').map((x) => x.r))
  const failedRows = computed(() => states.value.filter((x) => isFailed(x.s)).map((x) => x.r))
  /** 整体进度（含已完成的行，取平均）和速度 / 剩余时间（取正在运行的第一个任务） */
  const overall = computed(() => {
    const bound = states.value.filter((x) => x.r.taskId && x.s !== 'canceled')
    if (!bound.length) return { pct: 0, done: 0, total: 0, speed: '', etaSec: 0 }
    let sum = 0
    let done = 0
    for (const { r, s } of bound) {
      const t = rowTask(r)
      sum += s === 'succeeded' ? 1 : Math.min(1, Math.max(0, t?.progress ?? 0))
      if (s === 'succeeded') done++
    }
    const run = states.value.map((x) => (x.s === 'running' ? rowTask(x.r) : undefined)).find(Boolean)
    return { pct: Math.round((sum / bound.length) * 100), done, total: bound.length, speed: run?.speed ?? '', etaSec: run?.etaSec ?? 0 }
  })

  /** 开始转换不可用的原因（界面显示提示）；'' = 可以开始 */
  const startBlockReason = computed(() => {
    if (!ffmpeg.ready) return 'ffmpeg'
    if (!selectedPreset.value) return 'preset'
    if (busyProbing.value) return 'probing'
    // 坏文件（读取失败 / 与预设不兼容）不挡整批：submit() 只提交可转换的行，页面上提示“已跳过 N 个”
    if (!submittableRows.value.length) return 'empty'
    return ''
  })

  // ---------------- 添加文件 ----------------
  function addPaths(paths: string[]) {
    if (mode.value === 'running') return
    notice.value = ''
    submitError.value = null
    const have = new Set(rows.value.map((r) => r.path))
    let dup = 0
    let over = 0
    const added: ConvertRow[] = []
    for (const p of paths) {
      if (!p) continue
      if (have.has(p)) {
        dup++
        continue
      }
      if (rows.value.length + added.length >= MAX_SUBMIT) {
        over++
        continue
      }
      have.add(p)
      added.push(reactive<ConvertRow>({
        key: newKey(), path: p, name: fileBaseName(p), probe: 'waiting', thumb: '', thumbState: 'idle', cover: '', coverState: 'idle', taskId: '', label: '',
      }))
    }
    rows.value.push(...added)
    const msgs: string[] = []
    if (over) msgs.push(`一次最多转换 ${MAX_SUBMIT} 个文件，多出的 ${over} 个没有加入。`)
    if (dup) msgs.push(`${dup} 个文件已经在列表里。`)
    notice.value = msgs.join('')
    void probePending()
  }

  let probing = false
  /** 本轮读取已完成的个数；总数 = 已完成 + 还没读完的行（读取中途又加文件时总数跟着变） */
  const probeDone = ref(0)
  const probeProgress = computed(() => {
    const left = rows.value.filter((r) => r.probe === 'waiting' || r.probe === 'probing').length
    return { done: probeDone.value, total: probeDone.value + left }
  })
  /** 探测所有还没探测的行（分批，每批之后立刻更新界面）；ffmpeg 未就绪时等待，就绪后由 watch 再调用 */
  async function probePending() {
    if (probing || !ffmpeg.ready) return
    probing = true
    probeDone.value = 0
    try {
      for (;;) {
        // probeFiles 内部每 PROBE_BATCH（8）个一批回调一次，界面上能看到“正在读取文件信息（12/50）…”
        const batch = rows.value.filter((r) => r.probe === 'waiting').slice(0, PROBE_ROUND_MAX)
        if (!batch.length) break
        for (const r of batch) r.probe = 'probing'
        await probeFiles(batch.map((r) => r.path), (results) => {
          results.forEach((res, i) => {
            const r = batch.find((b) => b.path === res.path) ?? batch[i]
            if (!r || !rows.value.includes(r)) return
            if (res.info) {
              r.info = res.info
              r.probe = 'ok'
              probeDone.value++
            } else if (res.error?.code === 'FFMPEG_NOT_FOUND') {
              r.probe = 'waiting' // ffmpeg 又不可用了：等它就绪
            } else {
              r.probe = 'error'
              r.probeError = res.error
              probeDone.value++
            }
          })
        })
        if (rows.value.some((r) => r.probe === 'waiting') && !ffmpeg.ready) break
      }
    } finally {
      probing = false
      probeDone.value = 0
    }
  }

  /** 文件选择对话框 → 加入列表。没有 PickFiles 绑定时给出提示（仍可拖入） */
  async function chooseFiles() {
    pickSoon.value = ''
    if (!canPickFiles()) {
      pickSoon.value = '选择文件功能即将上线，请先把文件拖到这里。'
      return
    }
    try {
      const paths = await pickFiles()
      if (paths.length) addPaths(paths)
    } catch (e) {
      const err = toAppError(e)
      if (err.code === 'UNSUPPORTED') pickSoon.value = err.message
      else notice.value = err.message
    }
  }

  // ---------------- 缩略图（行进入视野时才取） ----------------
  let thumbActive = 0
  const thumbQueue: ConvertRow[] = []
  function pumpThumbs() {
    while (thumbActive < 3 && thumbQueue.length) {
      const r = thumbQueue.shift()!
      if (!rows.value.includes(r) || r.thumbState !== 'loading') continue
      thumbActive++
      const at = Math.min(10, (r.info?.duration ?? 0) * 0.1)
      thumbnailOf(r.path, at, 96)
        .then((url) => {
          r.thumb = url
        })
        .finally(() => {
          r.thumbState = 'done'
          thumbActive--
          pumpThumbs()
        })
    }
  }
  function ensureThumb(r: ConvertRow) {
    if (r.thumbState !== 'idle' || r.probe !== 'ok' || !r.info) return
    if (r.info.hasVideo === false || !r.info.width) {
      r.thumbState = 'done' // 纯音频没有画面
      return
    }
    if (r.info.thumbUrl) {
      // Probe 已经附带了默认缩略图（宽 320）
      r.thumb = r.info.thumbUrl
      r.thumbState = 'done'
      return
    }
    r.thumbState = 'loading'
    thumbQueue.push(r)
    pumpThumbs()
  }

  // ---------------- 文件信息卡（单文件 / 点击某行） ----------------
  /** 多个文件时被点选的行；列表里只有 1 行时信息卡直接显示它 */
  const focusKey = ref('')
  const focusRow = computed<ConvertRow | undefined>(() => {
    if (rows.value.length === 1) return rows.value[0]
    return rows.value.find((r) => r.key === focusKey.value)
  })
  function focusOn(r: ConvertRow) {
    focusKey.value = focusKey.value === r.key ? '' : r.key
  }
  /** 信息卡的 16:9 封面：Thumbnail(path, at, 640)；纯音频没有封面 */
  function ensureCover(r: ConvertRow | undefined) {
    if (!r || r.coverState !== 'idle' || r.probe !== 'ok' || !r.info) return
    if (r.info.hasVideo === false || !r.info.width) {
      r.coverState = 'done'
      return
    }
    r.coverState = 'loading'
    const at = Math.min(10, (r.info.duration ?? 0) * 0.1)
    thumbnailOf(r.path, at, 640)
      .then((url) => {
        r.cover = url
      })
      .catch((e) => {
        console.warn('load cover failed', r.path, e) // 封面只是装饰：失败就保持占位图标
      })
      .finally(() => {
        r.coverState = 'done'
      })
  }

  // ---------------- 行操作 ----------------
  function removeRow(key: string) {
    rows.value = rows.value.filter((r) => r.key !== key)
    notice.value = ''
  }
  /** 把读取失败 / 不兼容的行一次全部移出（“已跳过 N 个无法转换的文件 · 移出”） */
  function removeBlocked() {
    const drop = new Set(states.value.filter((x) => x.s === 'invalid' || x.s === 'conflict').map((x) => x.r.key))
    if (!drop.size) return
    rows.value = rows.value.filter((r) => !drop.has(r.key))
    notice.value = ''
  }
  function clear() {
    rows.value = []
    focusKey.value = ''
    notice.value = ''
    submitError.value = null
    pickSoon.value = ''
  }
  /** 把一行放回「待转换」（取消 / 中断后重新加入；任务记录保留在任务中心） */
  function unbind(r: ConvertRow) {
    r.taskId = ''
    r.submitError = undefined
  }

  // ---------------- 输出位置 ----------------
  async function chooseOutputDir() {
    const dir = hasWailsBackend() ? await pickDirectory('选择这次转换的输出文件夹') : '/Users/me/Movies/客户项目/成片'
    if (dir) outputOverride.value = dir
  }
  const effectiveOutputDir = computed(() => outputOverride.value || defaultOutputDir.value)

  // ---------------- 提交 ----------------
  function applySubmitError(e: unknown, batch: ConvertRow[]) {
    const err = toAppError(e)
    const first = (err.detail ?? '').split(/\r?\n/)[0]?.trim() ?? ''
    const hit = first ? batch.find((r) => r.path === first) : undefined
    const re: RowError = { code: err.code, message: err.message, detail: err.detail }
    if (hit) hit.submitError = re
    else submitError.value = re
  }

  async function submit() {
    if (submitting.value || startBlockReason.value || !selectedPreset.value) return
    const batch = submittableRows.value.slice()
    submitting.value = true
    submitError.value = null
    for (const r of batch) r.submitError = undefined
    try {
      const list = await submitConvert(batch.map((r) => r.path), selectedPreset.value.options, outputOverride.value)
      // 返回的任务与 inputs 一一对应（契约 6.9），按顺序绑定
      const label = presetShort.value
      batch.forEach((r, i) => {
        const t = list[i]
        if (!t) return
        r.taskId = t.id
        r.label = label
      })
      tasks.track(list)
    } catch (e) {
      applySubmitError(e, batch)
    } finally {
      submitting.value = false
    }
  }

  async function cancelRow(r: ConvertRow) {
    if (!r.taskId) return
    try {
      await tasks.cancel(r.taskId)
    } catch (e) {
      const err = toAppError(e)
      notice.value = actionErrorText(err.code, err.message)
    }
  }
  async function cancelAll() {
    await Promise.all(activeRows.value.map((r) => cancelRow(r)))
  }

  async function retryRow(r: ConvertRow) {
    if (!r.taskId || tasks.isBusy(r.taskId)) return
    try {
      const t = await tasks.retry(r.taskId) // store 里有在途保护：任务中心同时点也只发一次
      if (t) r.taskId = t.id
    } catch (e) {
      // Retry 会重新探测输入：文件已被删除等情况在这里如实提示，原失败行保留
      const err = toAppError(e)
      notice.value = actionErrorText(err.code, err.message)
    }
  }
  const retryingAll = ref(false)
  async function retryAllFailed() {
    if (retryingAll.value) return
    retryingAll.value = true
    try {
      for (const r of failedRows.value.slice()) await retryRow(r)
    } finally {
      retryingAll.value = false
    }
  }

  /**
   * 「更换输出位置」（失败行，磁盘空间不足）：选新文件夹 → 用原输入和参数重新提交。
   * 参数取自任务 params（{input, options, outputDir}）；解析不出来就不猜，返回 false 由调用方保留原提示。
   */
  async function changeOutputAndResubmit(r: ConvertRow): Promise<'ok' | 'cancelled' | 'no-params' | 'busy'> {
    // 与 Retry 共用按任务 id 的在途保护：选目录对话框开着或提交未返回时忽略再次点击
    if (!r.taskId) return 'no-params'
    const res = await tasks.exclusive(r.taskId, () => changeOutputInner(r))
    return res ?? 'busy'
  }
  async function changeOutputInner(r: ConvertRow): Promise<'ok' | 'cancelled' | 'no-params'> {
    const t = rowTask(r)
    let params = parseConvertParams((t as TaskItem | undefined)?.params)
    if (!params && r.taskId && hasWailsBackend()) {
      try {
        const full = normalizeTask(await call((await import('../../wailsjs/go/app/TaskService')).Get(r.taskId)))
        params = parseConvertParams(full.params)
      } catch (e) {
        console.warn('Get task for resubmit failed', e)
      }
    }
    if (!params) return 'no-params'
    const dir = await pickDirectory('选择这次转换的输出文件夹')
    if (!dir) return 'cancelled'
    const nt = await resubmitToDir(params, dir)
    tasks.track([nt])
    if (nt) r.taskId = nt.id
    return 'ok'
  }

  async function reveal(r: ConvertRow) {
    const t = rowTask(r)
    if (!t?.outputPath) return
    await revealInFolder(t.outputPath)
  }
  /** 打开输出位置：第一个成功文件所在处（文件管理器里选中它；Linux 只打开所在文件夹）。输出在多个文件夹时只打开第一个 */
  async function revealOutput() {
    const r = succeededRows.value[0]
    if (r) await reveal(r)
  }
  /** 完成后输出所在的文件夹（去重）；不唯一时界面写“N 个文件夹” */
  const outputFolders = computed(() => {
    const set = new Set<string>()
    for (const r of succeededRows.value) {
      const t = rowTask(r)
      if (t?.outputPath) set.add(dirName(t.outputPath))
    }
    return [...set]
  })
  const outputFolder = computed(() => (outputFolders.value.length === 1 ? outputFolders.value[0] : ''))

  // ---------------- 浏览器预览（无 window.go）：?convert=idle|files|probefail|running|done|failed ----------------
  function seedPreview(kind: string) {
    const mkTask = (id: string, status: TaskStatus, progress: number, extra: Partial<TaskItem> = {}) => normalizeTask({
      id, type: 'convert', status, title: '', inputPaths: [], outputPath: '', progress, speed: '', etaSec: 0, params: '', version: 5, createdAt: Date.now() - 60000, startedAt: Date.now() - 30000, finishedAt: 0, ...extra,
    } as TaskItem)
    const names = ['产品发布会_完整版.mov', 'vlog_杭州西湖.mkv', '访谈录音_第三期.wav', 'screen_record_0928.flv']
    const dir = '/Users/me/Movies/素材'
    let paths = names.map((n) => `${dir}/${n}`)
    // files：三个视频文件（干净的待转换状态）；probefail：再加一个损坏文件和一个纯音频（不兼容 MP4 预设）
    if (kind === 'files') paths = [paths[0], paths[1], paths[3]]
    if (kind === 'single' || kind === 'singledone') paths = [paths[0]]
    if (kind === 'audio') paths = [paths[2]]
    if (kind === 'many') paths = Array.from({ length: 50 }, (_, i) => `${dir}/素材_${String(i + 1).padStart(2, '0')}.mp4`)
    if (kind === 'probefail') paths = [paths[0], paths[1], `${dir}/损坏_采访素材.mp4`, paths[2]]
    addPaths(paths)
    // 预览里 Probe 是同步假数据，但仍走 probePending；这里等它跑完再绑定任务
    const bind = () => {
      const list = rows.value
      if (list.some((r) => r.probe === 'waiting' || r.probe === 'probing')) return setTimeout(bind, 20)
      const put = (i: number, t: TaskItem, fin = false) => {
        const r = list[i]
        if (!r) return
        r.taskId = t.id
        r.label = presetShort.value
        if (fin) {
          tasks.seedFinal({ id: t.id, status: t.status, error: t.error, outputPath: t.outputPath, progress: t.status === 'succeeded' ? 1 : t.progress, speed: '', etaSec: 0, startedAt: t.startedAt, finishedAt: Date.now(), params: t.params })
        } else tasks.track([t] as unknown as goStore.Task[])
      }
      const ok = (i: number, out: string) => put(i, mkTask(`pv${i}`, 'succeeded', 1, { outputPath: `${dir}/输出/${out}`, ...(kind === 'singledone' ? { startedAt: Date.now() - 302_000 } : {}) }), true)
      if (kind === 'running') {
        put(0, mkTask('pv0', 'running', 0.68, { speed: '2.4x', etaSec: 97 }))
        put(1, mkTask('pv1', 'running', 0.31, { speed: '1.8x', etaSec: 140 }))
        put(2, mkTask('pv2', 'queued', 0, { startedAt: 0 }))
        ok(3, 'screen_record_0928.mp4')
      } else if (kind === 'done' || kind === 'donemulti' || kind === 'singledone') {
        ok(0, '产品发布会_完整版.mp4'); ok(1, 'vlog_杭州西湖.mp4'); ok(2, '访谈录音_第三期.mp4'); ok(3, 'screen_record_0928.mp4')
        // donemulti：输出在两个文件夹里（每个文件保存在各自源文件夹）
        if (kind === 'donemulti') put(3, mkTask('pv3', 'succeeded', 1, { outputPath: '/Users/me/Desktop/录屏/screen_record_0928.mp4' }), true)
      } else if (kind === 'failed') {
        ok(0, '产品发布会_完整版.mp4')
        put(1, mkTask('pv1', 'failed', 0.31, { error: { code: 'CONVERT_DISK_FULL', message: '磁盘空间不足', detail: 'write /Volumes/Backup/输出/vlog_杭州西湖.mp4.part.mp4: no space left on device' }, params: JSON.stringify({ input: `${dir}/${names[1]}`, options: { container: 'mp4', videoCodec: 'h264', audioCodec: 'aac' }, outputDir: '/Volumes/Backup/输出' }) }), true)
        ok(2, '访谈录音_第三期.mp4')
        put(3, mkTask('pv3', 'failed', 0.12, { error: { code: 'PROCESS_FAILED', message: 'ffmpeg 处理失败', detail: 'Unknown encoder' } }), true)
      }
    }
    bind()
  }

  return {
    rows, presets, presetsLoaded, presetsError, selectedPresetId, selectedPreset, presetShort,
    outputOverride, defaultOutputDir, effectiveOutputDir, outputFolder, outputFolders, submitting, retryingAll, submitError, notice, pickSoon,
    mode, overall, totalBytes, startBlockReason, blockedCount, canceledCount, probeProgress, presetTitle, focusRow, focusOn, ensureCover, removeBlocked, activeRows, failedRows, succeededRows, pendingRows, submittableRows,
    init, refreshDefaultDir, loadPresets, addPaths, probePending, chooseFiles, ensureThumb, removeRow, clear, unbind,
    chooseOutputDir, submit, cancelRow, cancelAll, retryRow, retryAllFailed, changeOutputAndResubmit, reveal, revealOutput,
    stateOf, rowTask, conflictOf, seedPreview,
  }
})

/** 浏览器预览参数 ?convert=idle|files|probefail|running|done|failed；真实运行为 null */
export const PREVIEW_CONVERT = !hasWailsBackend() ? previewParams.get('convert') : null
