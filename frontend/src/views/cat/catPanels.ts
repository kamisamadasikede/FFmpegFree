/**
 * Cat 设置页 / 定时任务页的状态（契约 v0.35.1~v0.36，设计 v0.4.1）。
 * 与 catState 分开放，避免和美化 PR 冲突；catState 只在新建对话时读这里的默认值。
 */
import { reactive } from 'vue'
import { CAT_AGENT_BUILD, normalizeProviderKind, type CatProviderKind } from '@/api/catAgents'
import { type CatSettings, CAT_SETTINGS_COPY, emptyDefaults, getCatSettings, normalizeSettings, saveCatSettings } from '@/api/catSettings'
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
import type { CatConvPref } from './catConvPrefs'

export type CatView = 'chat' | 'settings' | 'schedules'
export type CatSettingsTab = 'general' | 'instruction' | 'ext' | 'about'

const preview = typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go
const q = typeof location !== 'undefined' ? new URLSearchParams(location.search) : new URLSearchParams()
const qView = preview ? q.get('cat_view') : null
const qTab = preview ? q.get('cat_tab') : null

export const catPanels = reactive({
  view: (qView === 'settings' || (qView === 'schedules' && catSchedulesVisible()) ? qView : 'chat') as CatView,
  tab: (['general', 'instruction', 'ext', 'about'].includes(qTab ?? '') ? qTab : 'general') as CatSettingsTab,
  settings: normalizeSettings({}),
  settingsLoaded: false,
  schedules: [] as CatSchedule[],
  schedulesLoaded: false,
  /** 应用内浮提示（定时任务完成 / 失败），点了打开对话 */
  toast: null as { key: number; title: string; body: string; tone: 'ok' | 'bad'; convId?: string } | null,
})

export function openView(v: CatView) {
  if (v === 'schedules' && !catSchedulesVisible()) return
  catPanels.view = v
  if (v === 'schedules') void refreshSchedules()
  if (v === 'settings') void loadSettings()
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
    return true
  } catch {
    catPanels.settings[key] = prev
    return false
  }
}

/** 新对话用的默认偏好（契约 6.19.12.1-4）；按所选/预选 agentKind 取 defaultsByAgent；批准方式空 = 全部批准 */
export function defaultConvPref(agentKind?: string): CatConvPref {
  const s = catPanels.settings
  const kind = normalizeProviderKind(agentKind ?? s.defaultAgentKind)
  const d = s.defaultsByAgent[kind] ?? emptyDefaults()
  const pref: CatConvPref = { model: d.model, think: d.effort }
  if (s.defaultApprovalMode) pref.access = s.defaultApprovalMode
  return pref
}

/** 定时任务本版仍锁 cat_build（契约 v0.36） */
export function scheduleDefaultPref(): { model: string; effort: string } {
  const d = catPanels.settings.defaultsByAgent[CAT_AGENT_BUILD] ?? emptyDefaults()
  return { model: d.model, effort: d.effort }
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

/** CatPage 挂载时调用：读设置、订阅 cat:schedule；返回卸载函数 */
let openConv: ((id: string) => void) | undefined
export function initCatPanels(projectName: (id: string) => string, open?: (id: string) => void): () => void {
  openConv = open
  void loadSettings()
  if (!catSchedulesVisible()) return () => {}
  void refreshSchedules()
  return onCatSchedule((e) => handleScheduleEvent(e, projectName))
}

export { CAT_SETTINGS_COPY }
