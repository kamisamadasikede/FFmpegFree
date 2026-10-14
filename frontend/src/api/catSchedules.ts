/**
 * Cat 定时任务接口层（契约 v0.35.1~v0.40 §6.19.13，设计《Cat-设置与定时任务-设计说明-v0.3.1》§4）。
 * CAT_SCHEDULE_BACKEND_READY=true 且在 Wails 里：调生成的 CatService 绑定（后端 #214，类型 cat.Schedule / cat.ScheduleRun），订阅 cat:schedule。
 * 否则：只有 vite dev 下走内存模拟（变化经模拟事件总线发 cat:schedule；走查开关 ?cat_sim=schedempty / schedfail），正式包里没有模拟数据。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { AppError, call } from '@/api/call'
import { CAT_SCHEDULE_BACKEND_READY } from '@/api/flags'
import { emitSimEvent, hasWailsBackend, onTaskEvent } from '@/services/wails'
import { CAT_AGENT_BUILD, normalizeProviderKind, type CatProviderKind } from '@/api/catAgents'
import { catSim } from '@/views/cat/catDevSim'

export { CAT_SCHEDULE_BACKEND_READY }

export type CatScheduleRunStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'missed' | 'skipped' | 'stopped'
export type CatScheduleFrequency = 'once' | 'daily' | 'weekdays' | 'weekly'

export interface CatScheduleRun {
  id: string
  scheduleId: string
  conversationId?: string
  status: CatScheduleRunStatus
  error?: { code: string; message: string; detail?: string } | null
  scheduledFor: number
  startedAt?: number
  finishedAt?: number
  manual: boolean
  missedReason?: 'closed' | 'asleep' | ''
  /** 契约 v0.35.3 / v0.35.4：只在 queued 时有。0 →「下一个就轮到它」，>0 →「前面还有 N 个任务」，没有 → 不显示 */
  queuePosition?: number
}

export interface CatScheduleInput {
  name: string
  prompt: string
  projectId: string
  frequency: CatScheduleFrequency
  time: string
  weekdays: number[]
  runAt: number
  model: string
  effort: string
  approvalMode: string
  /** v0.40：cat_build | claude_cli | codex_cli；缺省 / 迁移前视为 cat_build */
  agentKind?: CatProviderKind | string
  /** 新建时用；编辑时由暂停 / 恢复控制 */
  enabled?: boolean
}

export interface CatSchedule extends Omit<CatScheduleInput, 'agentKind'> {
  id: string
  agentKind: CatProviderKind | string
  timezone: string
  enabled: boolean
  projectDeleted: boolean
  nextRunAt: number
  lastRun?: CatScheduleRun | null
  createdAt: number
  updatedAt: number
}

export interface CatScheduleEvent {
  scheduleId: string
  schedule?: CatSchedule
  run?: CatScheduleRun
  deleted?: boolean
  /** v0.35.5：用户点了系统通知，打开这条对话（只带它时不改列表行） */
  openConvId?: string
}

export const CAT_SCHEDULE_EVENT = 'cat:schedule'
export const CAT_SCHEDULE_NAME_MAX = 40
export const CAT_SCHEDULE_PROMPT_MAX = 4000

/** 文案：设计 v0.3.1 §4 + 契约 v0.35.2 / 6.19.13.8 / 6.19.13.10 */
export const CAT_SCHED_COPY = {
  title: '定时任务',
  subtitle: '到点后在所属项目里开一个新对话，按设定的指令自动运行',
  /** 列表常驻提示（产品 10-13 定稿；只改这一处） */
  hint: '只在 FFmpegFree 打开时运行，最小化也会运行，关掉就不会。错过的时间点不补跑，列表里会标「已错过」。',
  notReady: '未检测到 Cat 助手，到点的任务会运行失败。',
  recheck: '重新检测',
  create: '新建任务',
  colTask: '任务',
  colFreq: '频率 · 下次',
  colLast: '上次运行',
  colOn: '启用',
  emptyTitle: '还没有定时任务',
  emptyBody: '让 Cat 助手每天、每周或在某个时间点自动做一件事。',
  templates: [
    { name: '每天早上巡检', prompt: '检查项目里昨天新增和改动的文件，列出可能有问题的地方。', frequency: 'daily', time: '09:00', weekdays: [] },
    { name: '每周周报', prompt: '整理这一周项目里的改动，写成一份简短的周报。', frequency: 'weekly', time: '18:00', weekdays: [5] },
    { name: '工作日整理', prompt: '把项目里散落的临时文件列出来，给出整理建议。', frequency: 'weekdays', time: '12:30', weekdays: [] },
  ] as { name: string; prompt: string; frequency: CatScheduleFrequency; time: string; weekdays: number[] }[],
  menuEdit: '编辑',
  menuRun: '立即运行',
  menuCopy: '复制',
  menuDelete: '删除',
  more: '任务操作',
  stop: '停止这次运行',
  copySuffix: ' 副本',
  // 状态
  st: {
    succeeded: '成功',
    failed: '失败',
    missed: '已错过',
    stopped: '已手动停止',
    skipped: '已跳过',
    queued: '排队中',
    running: '运行中',
    never: '还没运行过',
    paused: '已暂停',
  },
  skippedSub: '上一次还没结束',
  missedClosed: '当时 FFmpegFree 没有打开',
  missedAsleep: '当时电脑在睡眠',
  queueAhead: (n: number) => `前面还有 ${n} 个任务`,
  queueNext: '下一个就轮到它',
  timeout: '超过 30 分钟没跑完。',
  replyFailed: '回复没生成出来，请重试。',
  projectDeleted: '项目已删除，请编辑后换一个项目。',
  manualRun: '手动运行',
  ended: '已结束',
  dash: '—',
  freq: { once: '一次', daily: '每天', weekdays: '工作日', weekly: '每周', custom: '自定义' },
  freqList: { once: '仅一次', daily: '每天', weekdays: '工作日', weekly: '每周' },
  // 弹窗
  newTitle: '新建定时任务',
  editTitle: '编辑定时任务',
  name: '名称',
  namePh: '比如：每天早上巡检',
  prompt: '指令',
  promptTip: '每次运行都会作为新对话的第一条消息',
  promptHelp: '全局自定义指令会自动带上。',
  project: '所属项目',
  projectPh: '选择项目',
  approval: '批准方式',
  agentKind: '服务商',
  approveWarn: '无人看着时，Cat 助手会直接改文件、运行命令，不再询问。建议用 Plan。',
  normalHint: '无人看着时没人能确认，任务会停住，30 分钟后算失败。',
  more2: '更多选项',
  model: '模型',
  effort: '思考强度',
  defaultOpt: '默认',
  frequency: '频率',
  date: '日期',
  time: '时间',
  weekdays: ['一', '二', '三', '四', '五', '六', '日'],
  timezone: '时区',
  enabled: '启用',
  cancel: '取消',
  createBtn: '创建',
  saveBtn: '保存',
  later: '下一期开放。',
  errName: '请填写名称。',
  errNameLen: '最多 40 个字。',
  errPrompt: '请填写指令。',
  errPromptLen: '最多 4000 字。',
  errProject: '请选择项目。',
  errDays: '请至少选一天。',
  errPast: '这个时间已经过了。',
  missingProject: (n: string) => `「${n}」的文件夹不见了`,
  saveFailed: '保存失败，请重试。',
  confirmTitle: '确认用「全部批准」？',
  confirmOk: '确认保存',
  delTitle: '删除这个定时任务？',
  delBody: '运行记录会一起删除，已经产生的对话会保留。',
  delOk: '删除',
  delFailed: '删除失败，请重试。',
  opFailed: '操作失败，请重试。',
  // 抽屉
  runs: '运行记录',
  runsEmpty: '还没有运行记录。',
  openConv: '打开对话',
  convGone: '这条对话已删除。',
  close: '关闭',
  // 通知（v0.35.2 + 产品：手动停止不通知）
  notifyDone: (n: string) => `定时任务已完成：${n}`,
  notifyFail: (n: string) => `定时任务运行失败：${n}`,
  notifyBody: (p: string, dur: string) => `${p} · 用时 ${dur}。点这里打开对话。`,
} as const

const live = () => CAT_SCHEDULE_BACKEND_READY && hasWailsBackend()

/** 定时任务入口是否出现：后端就绪；或开发模式下的纯浏览器走查。打包后 DEV=false，开关没开就完全隐藏。 */
export function catSchedulesVisible(): boolean {
  if (CAT_SCHEDULE_BACKEND_READY) return true
  return import.meta.env.DEV && typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go
}
const sim = (f: Parameters<typeof catSim.has>[0]) => import.meta.env.DEV && catSim.has(f)
/** 模拟只在开发模式里有 */
const mockOff = () => !import.meta.env.DEV

const RUN_STATUSES: CatScheduleRunStatus[] = ['queued', 'running', 'succeeded', 'failed', 'missed', 'skipped', 'stopped']

export function normalizeRun(raw: unknown): CatScheduleRun | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Record<string, unknown>
  const status = RUN_STATUSES.includes(r.status as CatScheduleRunStatus) ? (r.status as CatScheduleRunStatus) : 'failed'
  const e = r.error as { code?: unknown; message?: unknown; detail?: unknown } | null | undefined
  const qp = Number(r.queuePosition)
  return {
    id: String(r.id ?? ''),
    scheduleId: String(r.scheduleId ?? ''),
    conversationId: typeof r.conversationId === 'string' && r.conversationId ? r.conversationId : undefined,
    status,
    error: e && typeof e.code === 'string' ? { code: e.code, message: typeof e.message === 'string' ? e.message : '', detail: typeof e.detail === 'string' ? e.detail : undefined } : null,
    scheduledFor: Number(r.scheduledFor) || 0,
    startedAt: Number(r.startedAt) || 0,
    finishedAt: Number(r.finishedAt) || 0,
    manual: r.manual === true,
    missedReason: r.missedReason === 'asleep' ? 'asleep' : r.missedReason === 'closed' ? 'closed' : '',
    queuePosition: r.queuePosition != null && Number.isFinite(qp) && qp >= 0 ? Math.floor(qp) : undefined,
  }
}

export function normalizeSchedule(raw: unknown): CatSchedule {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const s = (k: string) => (typeof r[k] === 'string' ? (r[k] as string) : '')
  const freq = (['once', 'daily', 'weekdays', 'weekly'] as const).find((f) => f === r.frequency) ?? 'daily'
  return {
    id: s('id'),
    name: s('name'),
    prompt: s('prompt'),
    projectId: s('projectId'),
    frequency: freq,
    time: s('time'),
    weekdays: Array.isArray(r.weekdays) ? (r.weekdays as unknown[]).map(Number).filter((n) => n >= 1 && n <= 7) : [],
    runAt: Number(r.runAt) || 0,
    timezone: s('timezone'),
    model: s('model'),
    effort: s('effort'),
    approvalMode: s('approvalMode') || 'plan',
    agentKind: normalizeProviderKind(s('agentKind')),
    enabled: r.enabled !== false,
    projectDeleted: r.projectDeleted === true,
    nextRunAt: Number(r.nextRunAt) || 0,
    lastRun: normalizeRun(r.lastRun),
    createdAt: Number(r.createdAt) || 0,
    updatedAt: Number(r.updatedAt) || 0,
  }
}

/** 本机时区下按频率算下一次（只给模拟和弹窗预览用；真实后端以 nextRunAt 为准） */
export function computeNextRun(s: Pick<CatScheduleInput, 'frequency' | 'time' | 'weekdays' | 'runAt'>, from = Date.now()): number {
  if (s.frequency === 'once') return s.runAt > from ? s.runAt : 0
  const [hh, mm] = (s.time || '').split(':').map(Number)
  if (!Number.isFinite(hh) || !Number.isFinite(mm)) return 0
  for (let i = 0; i < 8; i++) {
    const d = new Date(from)
    d.setDate(d.getDate() + i)
    d.setHours(hh, mm, 0, 0)
    if (d.getTime() <= from) continue
    const wd = ((d.getDay() + 6) % 7) + 1
    if (s.frequency === 'weekdays' && wd > 5) continue
    if (s.frequency === 'weekly' && !s.weekdays.includes(wd)) continue
    return d.getTime()
  }
  return 0
}

/** 名称 / 指令 / 项目 / 频率的校验；返回字段 → 错误文案 */
export function validateSchedule(s: CatScheduleInput, now = Date.now()): Partial<Record<'name' | 'prompt' | 'projectId' | 'weekdays' | 'runAt', string>> {
  const out: Partial<Record<'name' | 'prompt' | 'projectId' | 'weekdays' | 'runAt', string>> = {}
  const name = s.name.trim()
  if (!name) out.name = CAT_SCHED_COPY.errName
  else if ([...name].length > CAT_SCHEDULE_NAME_MAX) out.name = CAT_SCHED_COPY.errNameLen
  const prompt = s.prompt.trim()
  if (!prompt) out.prompt = CAT_SCHED_COPY.errPrompt
  else if (prompt.length > CAT_SCHEDULE_PROMPT_MAX) out.prompt = CAT_SCHED_COPY.errPromptLen
  if (!s.projectId) out.projectId = CAT_SCHED_COPY.errProject
  if (s.frequency === 'weekly' && !s.weekdays.length) out.weekdays = CAT_SCHED_COPY.errDays
  if (s.frequency === 'once' && !(s.runAt > now)) out.runAt = CAT_SCHED_COPY.errPast
  return out
}

/** 只有保存结果是全部批准且之前不是时才确认（契约 6.19.13.10-1） */
export function needsApproveConfirm(next: string, prev: string | null): boolean {
  return next === 'approve' && prev !== 'approve'
}

/** 哪些结果要通知（产品 10-13：只有已完成 / 运行失败；手动停止、已错过、已跳过、排队中都不通知） */
export function notifyTitle(run: CatScheduleRun, name: string): string {
  if (run.status === 'succeeded') return CAT_SCHED_COPY.notifyDone(name)
  if (run.status === 'failed') return CAT_SCHED_COPY.notifyFail(name)
  return ''
}

/** 排队副行：没有位置字段时返回空串（不显示） */
export function queueLine(run: Pick<CatScheduleRun, 'status' | 'queuePosition'>): string {
  if (run.status !== 'queued' || run.queuePosition == null) return ''
  return run.queuePosition === 0 ? CAT_SCHED_COPY.queueNext : CAT_SCHED_COPY.queueAhead(run.queuePosition)
}

export function formatDuration(ms: number): string {
  const sec = Math.max(0, Math.round(ms / 1000))
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return m ? `${m} 分 ${s} 秒` : `${s} 秒`
}

// ---------- 模拟 ----------

const MIN = 60_000
const HOUR = 60 * MIN
let seq = 100
let mockList: CatSchedule[] | null = null
const mockRuns = new Map<string, CatScheduleRun[]>()

function mkRun(scheduleId: string, status: CatScheduleRunStatus, ago: number, extra: Partial<CatScheduleRun> = {}): CatScheduleRun {
  const t = Date.now() - ago
  const done = status === 'succeeded' || status === 'failed' || status === 'stopped'
  return {
    id: `r${++seq}`,
    scheduleId,
    status,
    scheduledFor: t,
    startedAt: status === 'missed' || status === 'skipped' || status === 'queued' ? 0 : t + 2000,
    finishedAt: done ? t + 2000 + 3 * MIN + 12_000 : 0,
    manual: false,
    conversationId: done || status === 'running' ? 'c22' : undefined,
    ...extra,
  }
}

function mkSched(id: string, p: Partial<CatSchedule>, runs: CatScheduleRun[]): CatSchedule {
  const base: CatSchedule = {
    id, name: '', prompt: '检查项目里的改动并给出建议。', projectId: 'p2', frequency: 'daily', time: '09:00', weekdays: [], runAt: 0,
    timezone: 'Asia/Shanghai', model: '', effort: '', approvalMode: 'plan', agentKind: CAT_AGENT_BUILD, enabled: true, projectDeleted: false, nextRunAt: 0,
    createdAt: Date.now() - Number(id.slice(1)) * HOUR, updatedAt: Date.now(), ...p,
  }
  if (!base.nextRunAt && base.enabled && !base.projectDeleted) base.nextRunAt = computeNextRun(base)
  mockRuns.set(id, runs)
  base.lastRun = runs[0] ?? null
  return base
}

function seed(): CatSchedule[] {
  if (sim('schedempty')) return []
  return [
    mkSched('s1', { name: '每日直播日志巡检', frequency: 'daily', time: '09:00' }, [
      mkRun('s1', 'running', 72_000, { manual: true }),
      mkRun('s1', 'succeeded', 10 * HOUR),
      mkRun('s1', 'failed', 34 * HOUR, { error: { code: 'CAT_REPLY_FAILED', message: CAT_SCHED_COPY.timeout } }),
      mkRun('s1', 'stopped', 58 * HOUR),
      mkRun('s1', 'missed', 82 * HOUR, { missedReason: 'asleep' }),
      mkRun('s1', 'missed', 106 * HOUR, { missedReason: 'closed' }),
      mkRun('s1', 'skipped', 130 * HOUR),
    ]),
    mkSched('s2', { name: '周报：字幕进度', projectId: 'p3', frequency: 'weekly', time: '18:00', weekdays: [1, 5] }, [
      mkRun('s2', 'failed', 26 * HOUR, { error: null }),
    ]),
    mkSched('s3', { name: '素材文件夹整理', projectId: 'p1', frequency: 'weekdays', time: '12:30', enabled: false, nextRunAt: 0 }, [
      mkRun('s3', 'succeeded', 50 * HOUR),
    ]),
    mkSched('s4', { name: '直播日志汇总', frequency: 'daily', time: '14:00' }, [mkRun('s4', 'skipped', 5 * HOUR)]),
    mkSched('s5', { name: '发布前检查', projectId: 'p1', frequency: 'once', runAt: Date.now() + 7 * 24 * HOUR, approvalMode: 'approve' }, []),
    mkSched('s6', { name: '素材格式抽检', projectId: 'p1', frequency: 'daily', time: '19:00' }, [mkRun('s6', 'queued', 20_000, { queuePosition: 2 })]),
    mkSched('s9', { name: '字幕错别字检查', projectId: 'p3', frequency: 'daily', time: '19:00' }, [mkRun('s9', 'queued', 15_000, { queuePosition: 0 })]),
    mkSched('s10', { name: '推流地址可用性检查', frequency: 'daily', time: '19:00' }, [mkRun('s10', 'queued', 10_000)]),
    mkSched('s7', { name: '睡眠期间的晨检', projectId: 'p3', frequency: 'daily', time: '07:30' }, [mkRun('s7', 'missed', 3 * HOUR, { missedReason: 'asleep' })]),
    mkSched('s8', { name: '旧项目周检', projectId: 'p9', frequency: 'weekly', time: '10:00', weekdays: [3], enabled: false, projectDeleted: true, nextRunAt: 0 }, []),
  ]
}

function list(): CatSchedule[] {
  if (!mockList) mockList = seed()
  return mockList
}

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))
function emit(e: CatScheduleEvent) {
  emitSimEvent(CAT_SCHEDULE_EVENT, e)
}
function failMaybe() {
  if (sim('schedfail')) throw new AppError('IO_ERROR', 'sim')
}
function find(id: string): CatSchedule {
  const s = list().find((x) => x.id === id)
  if (!s) throw new AppError('NOT_FOUND', '')
  return s
}

/** 模拟：一次运行（排队 → 运行 → 成功），按事件推送 */
function simulateRun(s: CatSchedule, manual: boolean): CatScheduleRun {
  const runs = mockRuns.get(s.id) ?? []
  const busy = runs.find((r) => r.status === 'running' || r.status === 'queued')
  if (busy) {
    const skipped = mkRun(s.id, 'skipped', 0, { manual })
    push(s, skipped)
    return skipped
  }
  const run = mkRun(s.id, 'queued', 0, { manual, queuePosition: 0 })
  push(s, run)
  setTimeout(() => {
    const up = { ...run, status: 'running' as const, startedAt: Date.now(), queuePosition: undefined, conversationId: 'c22' }
    push(s, up)
    setTimeout(() => push(s, { ...up, status: 'succeeded', finishedAt: Date.now() }), 2500)
  }, 1200)
  return run
}

function push(s: CatSchedule, run: CatScheduleRun) {
  const runs = mockRuns.get(s.id) ?? []
  const i = runs.findIndex((r) => r.id === run.id)
  if (i >= 0) runs[i] = run
  else runs.unshift(run)
  mockRuns.set(s.id, runs.slice(0, 100))
  s.lastRun = run
  emit({ scheduleId: s.id, schedule: { ...s }, run })
}

// ---------- 接口 ----------

function toInput(req: CatScheduleInput) {
  return {
    name: req.name, prompt: req.prompt, projectId: req.projectId, frequency: req.frequency, time: req.time,
    weekdays: req.weekdays, runAt: req.runAt, model: req.model, effort: req.effort, approvalMode: req.approvalMode,
    agentKind: normalizeProviderKind(req.agentKind),
  }
}

export async function listCatSchedules(): Promise<CatSchedule[]> {
  if (live()) {
    const raw: unknown = await call(CatBinding.ListCatSchedules())
    return Array.isArray(raw) ? raw.map(normalizeSchedule) : []
  }
  if (mockOff()) return []
  await wait(200)
  return list().map((s) => ({ ...s }))
}

export async function createCatSchedule(req: CatScheduleInput): Promise<CatSchedule> {
  if (live()) {
    const created = normalizeSchedule(await call(CatBinding.CreateCatSchedule(cat.ScheduleInput.createFrom(toInput(req)))))
    // 后端新建默认启用；弹窗里关了启用就紧跟着暂停
    return req.enabled === false ? normalizeSchedule(await call(CatBinding.PauseCatSchedule(cat.ScheduleIDRequest.createFrom({ id: created.id })))) : created
  }
  if (mockOff()) throw new AppError('UNSUPPORTED', '')
  await wait(400)
  failMaybe()
  const s = mkSched(`s${++seq}`, { ...req, enabled: req.enabled !== false, createdAt: Date.now(), nextRunAt: 0 }, [])
  if (!s.enabled) s.nextRunAt = 0
  list().unshift(s)
  emit({ scheduleId: s.id, schedule: { ...s } })
  return { ...s }
}

export async function updateCatSchedule(id: string, req: CatScheduleInput): Promise<CatSchedule> {
  if (live()) {
    const up = normalizeSchedule(await call(CatBinding.UpdateCatSchedule(cat.UpdateScheduleRequest.createFrom({ id, ...toInput(req) }))))
    if (req.enabled === undefined || req.enabled === up.enabled) return up
    const req2 = cat.ScheduleIDRequest.createFrom({ id })
    return normalizeSchedule(await call(req.enabled ? CatBinding.ResumeCatSchedule(req2) : CatBinding.PauseCatSchedule(req2)))
  }
  if (mockOff()) throw new AppError('UNSUPPORTED', '')
  await wait(400)
  failMaybe()
  const s = find(id)
  Object.assign(s, req, { updatedAt: Date.now(), projectDeleted: false })
  if (req.enabled !== undefined) s.enabled = req.enabled
  s.nextRunAt = s.enabled ? computeNextRun(s) : 0
  emit({ scheduleId: id, schedule: { ...s } })
  return { ...s }
}

export async function deleteCatSchedule(id: string): Promise<void> {
  if (live()) {
    await call(CatBinding.DeleteCatSchedule(cat.ScheduleIDRequest.createFrom({ id })))
    return
  }
  if (mockOff()) throw new AppError('UNSUPPORTED', '')
  await wait(300)
  failMaybe()
  mockList = list().filter((x) => x.id !== id)
  mockRuns.delete(id)
  emit({ scheduleId: id, deleted: true })
}

export async function pauseCatSchedule(id: string): Promise<CatSchedule> {
  if (live()) return normalizeSchedule(await call(CatBinding.PauseCatSchedule(cat.ScheduleIDRequest.createFrom({ id }))))
  if (mockOff()) throw new AppError('UNSUPPORTED', '')
  await wait(200)
  failMaybe()
  const s = find(id)
  s.enabled = false
  s.nextRunAt = 0
  emit({ scheduleId: id, schedule: { ...s } })
  return { ...s }
}

export async function resumeCatSchedule(id: string): Promise<CatSchedule> {
  if (live()) return normalizeSchedule(await call(CatBinding.ResumeCatSchedule(cat.ScheduleIDRequest.createFrom({ id }))))
  if (mockOff()) throw new AppError('UNSUPPORTED', '')
  await wait(200)
  failMaybe()
  const s = find(id)
  if (s.projectDeleted) throw new AppError('INVALID_ARGUMENT', CAT_SCHED_COPY.projectDeleted)
  s.enabled = true
  s.nextRunAt = computeNextRun(s)
  emit({ scheduleId: id, schedule: { ...s } })
  return { ...s }
}

export async function runNowCatSchedule(id: string): Promise<CatScheduleRun> {
  if (live()) {
    const r = normalizeRun(await call(CatBinding.RunNowCatSchedule(cat.ScheduleIDRequest.createFrom({ id }))))
    if (!r) throw new AppError('INTERNAL', '')
    return r
  }
  if (mockOff()) throw new AppError('UNSUPPORTED', '')
  await wait(200)
  failMaybe()
  return simulateRun(find(id), true)
}

export async function listCatScheduleRuns(scheduleId: string, limit = 100): Promise<CatScheduleRun[]> {
  if (live()) {
    const raw: unknown = await call(CatBinding.ListCatScheduleRuns(cat.ListScheduleRunsRequest.createFrom({ scheduleId, limit: Math.min(limit, 100) })))
    return Array.isArray(raw) ? (raw.map(normalizeRun).filter(Boolean) as CatScheduleRun[]) : []
  }
  if (mockOff()) return []
  await wait(300)
  return [...(mockRuns.get(scheduleId) ?? [])].slice(0, limit)
}

export function onCatSchedule(cb: (e: CatScheduleEvent) => void): () => void {
  return onTaskEvent<unknown>(CAT_SCHEDULE_EVENT, (raw) => {
    if (!raw || typeof raw !== 'object') return
    const r = raw as Record<string, unknown>
    const scheduleId = typeof r.scheduleId === 'string' ? r.scheduleId : ''
    if (!scheduleId) return
    cb({
      scheduleId,
      schedule: r.schedule ? normalizeSchedule(r.schedule) : undefined,
      run: normalizeRun(r.run) ?? undefined,
      deleted: r.deleted === true,
      openConvId: typeof r.openConvId === 'string' && r.openConvId ? r.openConvId : undefined,
    })
  })
}

// ---------- 显示 ----------

const WD = ['日', '一', '二', '三', '四', '五', '六']
const pad = (n: number) => String(n).padStart(2, '0')

/** 今天 / 明天 / 昨天 / 周X（7 天内）/ 10月20日 + HH:mm */
export function fmtWhen(ts: number, now = Date.now()): string {
  if (!ts) return CAT_SCHED_COPY.dash
  const d = new Date(ts)
  const day0 = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const diff = Math.round((day0(d) - day0(new Date(now))) / 86_400_000)
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (diff === 0) return `今天 ${hm}`
  if (diff === 1) return `明天 ${hm}`
  if (diff === -1) return `昨天 ${hm}`
  if (Math.abs(diff) < 7) return `周${WD[d.getDay()]} ${hm}`
  return `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`
}

/** 频率列：每天 09:00 / 工作日 12:30 / 每周一、周五 18:00 / 一次 · 10月20日 20:00 */
export function fmtFreq(s: Pick<CatSchedule, 'frequency' | 'time' | 'weekdays' | 'runAt'>): string {
  const F = CAT_SCHED_COPY.freq
  if (s.frequency === 'once') {
    const d = new Date(s.runAt)
    return `${F.once} · ${d.getMonth() + 1}月${d.getDate()}日 ${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
  if (s.frequency === 'weekly') return `每周${s.weekdays.map((w) => CAT_SCHED_COPY.weekdays[w - 1]).join('、周')} ${s.time}`
  return `${F[s.frequency]} ${s.time}`
}

export type RunTone = 'ok' | 'bad' | 'warn' | 'grey' | 'skip' | 'queue' | 'live'

/** 状态 → 显示名、色调、副行 */
/** 手动停止：列表和运行记录都只写「已手动停止」，没有副行（设计走查 10-13） */
export function runView(run: CatScheduleRun | null | undefined, now = Date.now()): { label: string; tone: RunTone; sub: string } {
  const S = CAT_SCHED_COPY.st
  if (!run) return { label: S.never, tone: 'grey', sub: '' }
  switch (run.status) {
    case 'succeeded':
      return { label: S.succeeded, tone: 'ok', sub: '' }
    case 'failed':
      return { label: S.failed, tone: 'bad', sub: run.error?.message?.trim() || CAT_SCHED_COPY.replyFailed }
    case 'missed':
      return { label: S.missed, tone: 'warn', sub: run.missedReason === 'asleep' ? CAT_SCHED_COPY.missedAsleep : CAT_SCHED_COPY.missedClosed }
    case 'stopped':
      return { label: S.stopped, tone: 'grey', sub: '' }
    case 'skipped':
      return { label: S.skipped, tone: 'skip', sub: CAT_SCHED_COPY.skippedSub }
    case 'queued':
      return { label: S.queued, tone: 'queue', sub: queueLine(run) }
    case 'running':
      return { label: `${S.running} · 已 ${formatDuration(now - (run.startedAt || run.scheduledFor))}`, tone: 'live', sub: '' }
  }
}
