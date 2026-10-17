/**
 * Cat 设置页 / 定时任务页的状态（契约 v0.35.1~v0.40，设计 v0.4.1 + 克隆/通知 v0.1.1）。
 * 与 catState 分开放，避免和美化 PR 冲突；catState 只在新建对话时读这里的默认值。
 */
import { reactive } from 'vue'
import { normalizeProviderKind, type CatProviderKind } from '@/api/catAgents'
import { type CatSettings, CAT_SETTINGS_COPY, saveErrorMessage, emptyDefaults, getCatSettings, normalizeSettings, saveCatSettings } from '@/api/catSettings'
import {
  type CatSchedule,
  type CatScheduleEvent,
  CAT_SCHED_COPY,
  formatDuration,
  listCatSchedules,
  notifyTitle,
  onCatSchedule,
  catSchedulesVisible,
} from '@/api/catSchedules'
import { onEvent } from '@/services/wails'
import { setCatWindowFocused } from '@/api/cat'
import { toAppError } from '@/api/call'
import type { CatConvPref } from './catConvPrefs'
import { FALLBACK_MODES, getCatAgentCapabilities, resolveModeId } from './catCapabilities'
import { t } from '@/i18n'

export type CatView = 'chat' | 'settings' | 'schedules' | 'group'
export type CatSettingsTab = 'general' | 'instruction' | 'ext' | 'about'

const preview = typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go
const q = typeof location !== 'undefined' ? new URLSearchParams(location.search) : new URLSearchParams()
const qView = preview ? q.get('cat_view') : null
const qTab = preview ? q.get('cat_tab') : null
/** 群聊走查：?cat_group_scene=… 直接打开示例群 g1（只在浏览器预览） */
const qGroupScene = preview ? q.get('cat_group_scene') : null

export const catPanels = reactive({
  view: (qGroupScene ? 'group' : qView === 'settings' || (qView === 'schedules' && catSchedulesVisible()) ? qView : 'chat') as CatView,
  tab: (['general', 'instruction', 'ext', 'about'].includes(qTab ?? '') ? qTab : 'general') as CatSettingsTab,
  settings: normalizeSettings({}),
  settingsLoaded: false,
  schedules: [] as CatSchedule[],
  schedulesLoaded: false,
  /** 应用内浮提示（定时任务完成 / 失败），点了打开对话 */
  /** v0.55 群聊：当前打开的群 / 正在建群的项目（空 = 没开建群弹窗） */
  activeGroupId: (qGroupScene ? 'g1' : '') as string,
  createGroupProjectId: (qGroupScene === 'create' ? 'p3' : '') as string,
  toast: null as { key: number; title: string; body: string; tone: 'ok' | 'bad'; convId?: string } | null,
})

export function openView(v: CatView) {
  if (v === 'schedules' && !catSchedulesVisible()) return
  catPanels.view = v
  if (v === 'schedules') void refreshSchedules()
  if (v === 'settings') void loadSettings()
}

export function openGroup(groupId: string) {
  catPanels.activeGroupId = groupId
  catPanels.view = 'group'
}

export function openCreateGroup(projectId: string) {
  catPanels.createGroupProjectId = projectId
}

export async function loadSettings() {
  try {
    catPanels.settings = await getCatSettings()
    catPanels.settingsLoaded = true
  } catch {
    /* 读不到就按全部空值（契约：没有记录 = 全部空值） */
  }
}

/**
 * 改一项：先显示新值，整体提交；失败回滚这一项并返回 false（调用方弹「保存失败，请重试。」）。
 */
export async function saveSetting<K extends keyof CatSettings>(key: K, value: CatSettings[K]): Promise<boolean> {
  const prev = catPanels.settings[key]
  catPanels.settings[key] = value
  try {
    const saved = await saveCatSettings({ ...catPanels.settings })
    // 其它项保存期间可能又改了，只拿回这一项
    catPanels.settings[key] = saved[key]
    // 打开自定义服务商时后端会清掉该家旧的思考强度：用返回值刷新 per-CLI 默认，界面显示「默认」
    if (key === 'providersByAgent') catPanels.settings.defaultsByAgent = saved.defaultsByAgent
    return true
  } catch (e) {
    catPanels.settings[key] = prev
    lastSaveError = saveErrorText(e)
    return false
  }
}

let lastSaveError = ''
/** 上一次 saveSetting 失败的显示文案（见 saveErrorText）；没有失败时为空串。 */
export function takeSaveError(): string {
  const t = lastSaveError
  lastSaveError = ''
  return t
}

/** 保存失败文案：后端中文 message 原样显示；非中文加「保存失败：」；没有 message 用通用文案。 */
export function saveErrorText(e: unknown): string {
  const msg = (toAppError(e).message || '').trim()
  return saveErrorMessage(msg === 'sim' ? '' : msg)
}

/** 新对话用的默认偏好（契约 6.19.12.1-4）；按所选/预选 agentKind 取 defaultsByAgent；批准方式空 = 全部批准 */
export function defaultConvPref(agentKind?: string): CatConvPref {
  const s = catPanels.settings
  const kind = normalizeProviderKind(agentKind ?? s.defaultAgentKind)
  const d = s.defaultsByAgent[kind] ?? emptyDefaults()
  const pref: CatConvPref = { model: d.model, think: d.effort }
  // v0.44.4 §6.19.18.9 第 4 条：defaultModeByAgent[kind]；空 / 已不在该家 modes[] → 该家最保守默认（旧值按「旧值」列映射）
  const want = s.defaultModeByAgent?.[kind] ?? ''
  const caps = getCatAgentCapabilities(kind)
  if (caps.modes !== FALLBACK_MODES) pref.access = resolveModeId(kind, want)
  else if (want) pref.access = want
  return pref
}

/** 定时任务模型/强度默认：按所选 agentKind 取 defaultsByAgent（v0.40；缺省 cat_build） */
export function scheduleDefaultPref(agentKind?: string): { model: string; effort: string } {
  const kind = normalizeProviderKind(agentKind ?? catPanels.settings.defaultAgentKind)
  const d = catPanels.settings.defaultsByAgent[kind] ?? emptyDefaults()
  return { model: d.model, effort: d.effort }
}

/** 新建定时任务默认服务商 = CatSettings.defaultAgentKind */
export function scheduleDefaultAgentKind(): CatProviderKind {
  return normalizeProviderKind(catPanels.settings.defaultAgentKind)
}


export async function refreshSchedules() {
  if (!catSchedulesVisible()) return
  try {
    catPanels.schedules = await listCatSchedules()
  } catch {
    /* 保持上次的列表 */
  }
  catPanels.schedulesLoaded = true
}

export { catSchedulesVisible }
export const enabledScheduleCount = () => catPanels.schedules.filter((s) => s.enabled && !s.projectDeleted).length

let toastSeq = 0
let toastTimer: ReturnType<typeof setTimeout> | undefined

function windowInFront(): boolean {
  if (typeof document === 'undefined') return true
  // v0.35.5 ⑤：可见且没最小化就算前台，不要求获得焦点
  return document.visibilityState === 'visible'
}

/**
 * 处理 cat:schedule：更新列表行；运行结束且结果是已完成 / 运行失败、窗口在前台、通知开着 → 应用内浮提示。
 * 窗口不在前台时系统通知由后端发，前端不重复。手动停止 / 已错过 / 已跳过 / 排队中都不提示（契约 v0.35.3）。
 */
export function handleScheduleEvent(e: CatScheduleEvent, projectName: (id: string) => string = () => '') {
  if (e.openConvId && !e.schedule && !e.run && !e.deleted) {
    openConv?.(e.openConvId)
    return
  }
  if (e.openConvId) openConv?.(e.openConvId)
  const i = catPanels.schedules.findIndex((s) => s.id === e.scheduleId)
  if (e.deleted) {
    if (i >= 0) catPanels.schedules.splice(i, 1)
    return
  }
  const prevRun = i >= 0 ? catPanels.schedules[i].lastRun : null
  if (e.schedule) {
    const next = { ...e.schedule, lastRun: e.run ?? e.schedule.lastRun }
    if (i >= 0) catPanels.schedules[i] = next
    else catPanels.schedules.unshift(next)
  } else if (e.run && i >= 0) {
    catPanels.schedules[i] = { ...catPanels.schedules[i], lastRun: e.run }
  }
  const run = e.run
  if (!run) return
  const sched = catPanels.schedules.find((s) => s.id === e.scheduleId)
  const title = sched ? notifyTitle(run, sched.name) : ''
  // 只有这次打开期间亲眼看着跑完的才提示；启动时补标的「应用关闭，这次运行中断了。」等不提示（v0.35.5 ③）
  const justFinished = !!prevRun && prevRun.id === run.id && (prevRun.status === 'running' || prevRun.status === 'queued')
  if (!title || !justFinished || !catPanels.settings.notifyOnFinish || !windowInFront()) return
  const dur = run.finishedAt && run.startedAt ? formatDuration(run.finishedAt - run.startedAt) : formatDuration(0)
  clearTimeout(toastTimer)
  catPanels.toast = {
    key: ++toastSeq,
    title,
    body: CAT_SCHED_COPY.notifyBody(projectName(sched!.projectId), dur),
    tone: run.status === 'succeeded' ? 'ok' : 'bad',
    convId: run.conversationId,
  }
  toastTimer = setTimeout(() => (catPanels.toast = null), 6000)
}

export function dismissToast() {
  clearTimeout(toastTimer)
  catPanels.toast = null
}

const CAT_NOTIFY_EVENT = 'cat:notify'
/**
 * v0.48 6.19.12.11：普通 turn 终态只由后端发 OS 系统通知（窗口无键盘焦点时），前端不再弹应用内 toast。
 * 这里把 focus / blur / visibilitychange 上报给后端（防抖 200ms；启动时按 document.hasFocus() 报一次）。
 */
function startFocusReporter(): () => void {
  if (typeof window === 'undefined' || typeof document === 'undefined') return () => {}
  let timer: ReturnType<typeof setTimeout> | undefined
  let last: boolean | undefined
  const report = () => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      const f = document.visibilityState !== 'hidden' && document.hasFocus()
      if (f === last) return
      last = f
      void setCatWindowFocused(f)
    }, 200)
  }
  window.addEventListener('focus', report)
  window.addEventListener('blur', report)
  document.addEventListener('visibilitychange', report)
  last = document.visibilityState !== 'hidden' && document.hasFocus()
  void setCatWindowFocused(last)
  return () => {
    clearTimeout(timer)
    window.removeEventListener('focus', report)
    window.removeEventListener('blur', report)
    document.removeEventListener('visibilitychange', report)
  }
}

function handleNotifyEvent(raw: unknown) {
  const r = raw as { openConvId?: unknown } | null
  const id = r && typeof r.openConvId === 'string' ? r.openConvId : ''
  if (id) openConv?.(id)
}

/** CatPage 挂载时调用：读设置、订阅 cat:schedule / cat:notify；返回卸载函数 */
let openConv: ((id: string) => void) | undefined
export function initCatPanels(projectName: (id: string) => string, open?: (id: string) => void): () => void {
  openConv = open
  void loadSettings()
  const offs: Array<() => void> = []
  offs.push(onEvent(CAT_NOTIFY_EVENT, handleNotifyEvent))
  offs.push(startFocusReporter())
  if (catSchedulesVisible()) {
    void refreshSchedules()
    offs.push(onCatSchedule((e) => handleScheduleEvent(e, projectName)))
  }
  return () => { for (const off of offs) off() }
}

export { CAT_SETTINGS_COPY }
