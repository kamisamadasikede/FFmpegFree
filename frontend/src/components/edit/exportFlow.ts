// 导出流程状态（模块级单例）：导出弹框、校验、提交、状态条。任务进度只走 stores/tasks（task store 的进度通道），不自己轮询。
import { computed, reactive, ref } from 'vue'
import { DEFAULT_OUTPUT, exportProject, toAppError, validateProject, withExplicitOutput, type EditFormat, type EditPlan, type EditProject } from '@/api/edit'
import { getDefaultOutputDir, pickDirectory, revealInFolder } from '@/api/system'
import { hasWailsBackend } from '@/services/wails'
import { useTaskStore, type FinalState, type TaskItem } from '@/stores/tasks'
import {
  TEXT, exportErrorView, exportNameError, formatClock, hasTransitionIgnored, isSilentWarning, isWindowsPlatform, normalizeWarnings, sanitizeNameInput, type EditWarning, type ExportErrorView,
} from '@/utils/editLogic'
import { fileBaseName } from '@/utils/format'
import { useEditor, type StripEncoder, type StripView } from './editor'
import { pickEncoderFields } from '@/api/encoderTask'

const PREVIEW_DIR = '/Users/me/Movies/FFmpegFree'

function create() {
  const ed = useEditor()
  const tasks = useTaskStore()

  // ───────── 弹框 ─────────
  const open = ref(false)
  const validating = ref(false)
  const plan = ref<EditPlan | null>(null)
  const warnings = ref<EditWarning[]>([])
  const validateError = ref<ExportErrorView | null>(null)
  const submitting = ref(false)
  const submitError = ref<ExportErrorView | null>(null)
  const form = reactive({ name: '', dir: '', format: 'mp4' as EditFormat, width: DEFAULT_OUTPUT.width, height: DEFAULT_OUTPUT.height, fps: DEFAULT_OUTPUT.fps })
  const dirDefault = ref('')

  const nameErr = computed(() => exportNameError(form.name, form.dir || dirDefault.value, form.format, isWindowsPlatform()))
  const outputOk = computed(() => {
    const w = Number(form.width), h = Number(form.height), f = Number(form.fps)
    return Number.isFinite(w) && w >= 16 && w <= 7680 && Number.isFinite(h) && h >= 16 && h <= 4320 && Number.isFinite(f) && f > 0 && f <= 120
  })
  /** 弹框摘要里“向下取偶数”后的实际尺寸 */
  const actualSize = computed(() => ({ w: Math.floor(Number(form.width) / 2) * 2, h: Math.floor(Number(form.height) / 2) * 2 }))
  const noAudio = computed(() => ed.project.audioTrack.length === 0)
  const canStart = computed(() => !validating.value && !submitting.value && !validateError.value && !!plan.value && !nameErr.value && outputOk.value)

  /** 首个提示条最多显示 3 条，其余“另有 N 个提示” */
  const shownWarnings = computed(() => warnings.value.filter((w) => !isSilentWarning(w)))

  async function openDialog() {
    if (busy.value) return
    open.value = true
    validating.value = true
    plan.value = null
    warnings.value = []
    validateError.value = null
    submitError.value = null
    const o = ed.project.output
    form.name = sanitizeNameInput(ed.project.name)
    form.format = (o.format || DEFAULT_OUTPUT.format) as EditFormat
    form.width = o.width || DEFAULT_OUTPUT.width
    form.height = o.height || DEFAULT_OUTPUT.height
    form.fps = o.fps || DEFAULT_OUTPUT.fps
    if (!dirDefault.value) dirDefault.value = (await getDefaultOutputDir().catch(() => '')) || (hasWailsBackend() ? '' : PREVIEW_DIR)
    if (!form.dir) form.dir = dirDefault.value
    try {
      const p = await validateProject(withExplicitOutput(ed.currentProject()))
      plan.value = p
      warnings.value = normalizeWarnings(p.warnings)
    } catch (e) {
      const err = toAppError(e)
      validateError.value = exportErrorView(err, ed.project)
      highlight(validateError.value)
    } finally {
      validating.value = false
    }
  }
  function closeDialog() {
    open.value = false
  }
  async function pickDir() {
    try {
      const d = await pickDirectory('选择保存位置')
      if (d) form.dir = d
    } catch (e) {
      submitError.value = exportErrorView(toAppError(e), ed.project)
    }
  }
  function applyOutput() {
    ed.project.output = { format: form.format, width: Math.floor(Number(form.width)), height: Math.floor(Number(form.height)), fps: Number(form.fps) }
  }

  // ───────── 提交 ─────────
  const taskId = ref('')
  const outName = ref('')
  const outDir = ref('')
  const snap = ref<EditProject | null>(null)
  const savedWarnings = ref<EditWarning[]>([])
  const dismissed = ref(false)
  const localErr = ref<ExportErrorView | null>(null)
  const forced = ref<StripView | null>(null)
  const meta = ref('')

  async function submit(dir = form.dir, name = form.name): Promise<boolean> {
    applyOutput()
    const project = ed.currentProject()
    submitting.value = true
    submitError.value = null
    try {
      const t = await exportProject(withExplicitOutput(project), { outputName: name, outputDir: dir })
      tasks.track([t as unknown as TaskItem])
      taskId.value = t.id
      snap.value = project
      outName.value = name
      outDir.value = dir
      savedWarnings.value = warnings.value
      meta.value = `${project.output.width}×${project.output.height} · ${formatClock(plan.value?.durationSec ?? ed.total.value)}`
      dismissed.value = false
      localErr.value = null
      forced.value = null
      ed.errorClipId.value = null
      open.value = false
      return true
    } catch (e) {
      const v = exportErrorView(toAppError(e), project)
      submitError.value = v
      highlight(v)
      return false
    } finally {
      submitting.value = false
    }
  }
  async function start() {
    if (!canStart.value) return
    await submit()
  }

  function highlight(v: ExportErrorView | null) {
    ed.errorClipId.value = v?.clipId ?? null
  }

  // ───────── 状态条 ─────────
  const current = computed<TaskItem | FinalState | undefined>(() => (taskId.value ? tasks.taskById(taskId.value) : undefined))
  const busy = computed(() => {
    if (forced.value) return forced.value.kind === 'run'
    const c = current.value
    return !!c && (c.status === 'queued' || c.status === 'running')
  })
  /** 状态条上的编码器信息（原始字段；是否显示、设备名怎么取由 ExportStrip 决定） */
  function encOf(c: TaskItem | FinalState): StripEncoder {
    return { ...pickEncoderFields(c), startedAt: c.startedAt }
  }
  const strip = computed<StripView | null>(() => {
    if (forced.value) return dismissed.value && forced.value.kind !== 'run' ? null : forced.value
    if (localErr.value) return { kind: 'err', view: localErr.value, taskId: taskId.value }
    const c = current.value
    if (!c || dismissed.value && c.status !== 'queued' && c.status !== 'running') return null
    const name = outName.value ? `${outName.value}.${(snap.value?.output.format || 'mp4')}` : fileBaseName(c.outputPath || '')
    if (c.status === 'queued' || c.status === 'running') {
      const pct = Math.max(0, Math.min(100, Math.round((c.progress > 0 ? c.progress : 0) * 100)))
      return { kind: 'run', name, pct, speedText: c.speed || '', etaText: c.etaSec > 0 ? `剩余约 ${formatClock(c.etaSec)}` : '', enc: encOf(c) }
    }
    if (c.status === 'succeeded') {
      return { kind: 'ok', name: fileBaseName(c.outputPath) || name, meta: meta.value, ignoredTransitions: hasTransitionIgnored(savedWarnings.value), outputPath: c.outputPath, enc: encOf(c) }
    }
    if (c.status === 'canceled') return { kind: 'cx' }
    const err = ('error' in c ? c.error : null) ?? { code: 'INTERNAL', message: '', detail: '' }
    return { kind: 'err', view: exportErrorView(err, snap.value ?? ed.project), taskId: c.id }
  })

  async function cancelExport() {
    if (taskId.value) {
      try {
        await tasks.cancel(taskId.value)
      } catch (e) {
        ed.say(toAppError(e).message)
      }
    }
  }
  function dismiss() {
    dismissed.value = true
    localErr.value = null
    ed.errorClipId.value = null
  }
  async function retry() {
    if (!taskId.value) return
    try {
      const t = await tasks.retry(taskId.value)
      if (t) {
        taskId.value = t.id
        dismissed.value = false
        localErr.value = null
        ed.errorClipId.value = null
      }
    } catch (e) {
      // 素材已被删除等：不产生新任务，错误行更新为新的错误
      localErr.value = exportErrorView(toAppError(e), snap.value ?? ed.project)
      highlight(localErr.value)
    }
  }
  async function changeOutput() {
    let d = ''
    try {
      d = await pickDirectory('选择保存位置')
    } catch (e) {
      ed.say(toAppError(e).message)
      return
    }
    if (!d || !snap.value) return
    // 换目录必须重新 Export（Retry 沿用旧目录）
    applyOutputFrom(snap.value)
    try {
      const t = await exportProject(withExplicitOutput(snap.value), { outputName: outName.value, outputDir: d })
      tasks.track([t as unknown as TaskItem])
      taskId.value = t.id
      outDir.value = d
      dismissed.value = false
      localErr.value = null
      ed.errorClipId.value = null
    } catch (e) {
      localErr.value = exportErrorView(toAppError(e), snap.value)
    }
  }
  function applyOutputFrom(_p: EditProject) {
    /* 快照的 output 已是提交时的值 */
  }
  function locate() {
    const v = strip.value
    const id = v && v.kind === 'err' ? v.view.clipId : null
    if (!id) return
    const c = ed.clipById(id)
    if (!c) return
    ed.errorClipId.value = id
    ed.selectClip(id)
    ed.revealTime(c.startSec)
  }
  async function reveal(path: string) {
    try {
      await revealInFolder(path)
    } catch (e) {
      ed.say(toAppError(e).message)
    }
  }
  const logOpen = ref(false)
  const logText = ref('')
  async function viewLog() {
    logText.value = ''
    logOpen.value = true
    try {
      logText.value = (await tasks.getLog(taskId.value || (strip.value && strip.value.kind === 'err' ? strip.value.taskId : ''), 200)) || '暂无日志'
    } catch (e) {
      logText.value = toAppError(e).message
    }
  }

  return {
    open, validating, plan, warnings, shownWarnings, validateError, submitting, submitError, form, dirDefault, nameErr, outputOk, actualSize, noAudio, canStart,
    openDialog, closeDialog, pickDir, start, busy, strip, forced, dismissed, localErr, taskId, savedWarnings, cancelExport, dismiss, retry, changeOutput, locate, reveal, logOpen, logText, viewLog,
    highlight, TEXT,
  }
}

let inst: ReturnType<typeof create> | null = null
export const useExportFlow = () => (inst ??= create())
