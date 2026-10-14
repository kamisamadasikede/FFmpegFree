import { refreshScratch } from './catScratchFiles'
import { listCatFiles } from '@/api/catFiles'
import { catBackendLive } from '@/api/cat'
import { mergeCatStatus, markRowChecking } from '@/api/catDetect'
import { applyCatOutput, applyCatTiming } from '@/components/cat/catTimingAdapter'
/**
 * Cat 页本地状态（单例）。契约 v0.30 / 设计 v0.4：
 * - CAT_BACKEND_READY=true 且在 Wails 里：会话列表 / 消息 / 发送 / 停止走真实 CatService；cat:message 流式拼进助手气泡
 * - 纯浏览器（走查）：会话列表用 catMock，发送返回未就绪
 * - 新建对话创建时写入所选 agentKind（v0.36：cat_build|claude_cli|codex_cli）；已有对话永不改（发送只带会话 id）
 * - 批准方式按对话记住：普通模式 / Plan / 自动审查 / 全部批准。没选过是全部批准
 * - 项目（契约 v0.31 §6.19.10）：对话归属在创建时决定（项目行「+」带 projectId），之后不可改，没有「移到项目」；
 *   文件夹不见了 → 项目标灰、对话可看不可发；窗口获得焦点时重新 ListCatProjects；cat:project 更新 missing。
 *   接口见 api/catProjects.ts（CAT_PROJECTS_BACKEND_READY=true 走真绑定；浏览器走内存模拟）
 */
import { CAT_LAST_CONV_KEY } from '@/api/catStorageKeys'
import { computed, reactive, watch } from 'vue'
import { Environment } from '../../../wailsjs/runtime/runtime'
import { hasWailsBackend } from '@/services/wails'
import {
  CAT_PROJECT_COPY,
  CAT_PROJECT_NAME_MAX,
  catProjectsOn,
  catProjectsSim,
  createCatProject,
  deleteCatProject,
  duplicateProjectId,
  folderName,
  guessPlatform,
  listCatProjects,
  onCatProject,
  pickProjectFolder,
  projectErrorText,
  queueSimPick,
  relocateCatProject,
  renameCatProject,
  resetCatProjectSim,
  revealCatProject,
  revealLabel,
  validProjectName,
  type CatPlatform,
  type CatProject as ApiCatProject,
  type CatProjectEvent,
} from '@/api/catProjects'
import {
  CAT_AGENT_BUILD,
  CAT_AGENT_CODEX,
  CAT_CONV_COPY,
  CAT_COPY,
  cancelCatTurn,
  createCatConversation,
  deleteCatConversation,
  getCatConversation,
  getCatStatus,
  recheckCat,
  mapCatStatus,
  isCatLive,
  listCatConversations,
  listLiveCatTurns,
  listCatModels,
  listCatThinkLevels,
  onCatMessage,
  onCatStatus,
  onCatTurn,
  sendCatMessage,
  retryCatTurn,
  type CatAgentKind,
  type CatConversation,
  type CatMessage,
  type CatModel,
  type CatStatus,
  type CatThinkLevel,
} from '@/api/cat'
import {
  CAT_ACCESS,
  CAT_MODES,
  mockMessages,
  mockPlainConvs,
  mockCliOutdatedConvs,
  mockProjects,
  titleFrom,
  type CatBlock,
  type CatConv,
  type CatProject,
} from '@/api/catMock'
import type { CatOutgoingFile } from '@/components/cat/catAttach'
import { previewKind, type CatShownFile } from '@/components/cat/catMedia'
import { isRetryNotAllowed } from '@/api/catV0445'
import { applyStreamEvent, newStreamMsg, type CatStreamEvent, type CatStreamMsg, type CatTurnEvent } from '@/api/catStream'
import { liveTurnPlan, transcriptChanged } from './catLive'
import { AppError, toAppError } from '@/api/call'
import { CAT_CLI_OUTDATED_COPY, parseCliOutdated } from '@/components/cat/catCliOutdated'
import { isModelListStale, listCatProviderModels, providerFailureText, type CatProviderModelList } from '@/api/catProviderModels'
import { isProviderKind, type CatProviderKind } from '@/api/catAgents'
import { catPanels, showTurnFinishToast, defaultConvPref, dismissFailToast } from './catPanels'
import { rememberNewConvProject, resolveNewConvProject } from './catLocalPrefs'
import { catSim, catSimOs, catSimPicks, simAgentRaw, simStream, simDetectPayloads } from './catDevSim'
import { DEFAULT_MODEL_LABEL, followsCliModel, FALLBACK_MODES, resolveModeId, mapAgentModes, mapBackendCaps, mapModelEfforts, getCatAgentCapabilities, modeMeta, pickSupported, setBackendCaps, useCatAgentCapabilities } from './catCapabilities'
import {
  CAT_CONV_PREFS_KEY,
  carryPref,
  dropPref,
  prefOf,
  putPref,
  readConvPrefs,
  resolveAccess,
  resolveConvPref,
  writeConvPrefs,
  type CatConvPrefMap,
} from './catConvPrefs'

export const NEW_CONV = 'new'

/** 上次看的对话。重新进入 Cat、下次启动都回到这一条；「新对话」不记 */
const LAST_CONV_KEY = CAT_LAST_CONV_KEY

function readLastConv(): string {
  try {
    const v = globalThis.localStorage?.getItem(LAST_CONV_KEY)?.trim() ?? ''
    return v && v !== NEW_CONV ? v : ''
  } catch {
    return ''
  }
}

function writeLastConv(id: string) {
  try {
    if (!id || id === NEW_CONV) globalThis.localStorage?.removeItem(LAST_CONV_KEY)
    else globalThis.localStorage?.setItem(LAST_CONV_KEY, id)
  } catch {
    /* 无 localStorage 时只在本次运行里记住 */
  }
}

/** 浮提示语气：info（信息图标）/ ok（成功绿勾）/ warn（警示色图标与描边，设计 v0.2 §10b） */
export type CatNoticeTone = 'info' | 'ok' | 'warn'

/** 一轮回复的前端状态：running = 生成中（显示停止）；stopping = 已点停止（按钮立即置灰） */
export interface CatTurnState {
  token: number
  turnId: string
  status: 'running' | 'stopping'
  /** 收到第一段助手文字后记下（之前显示「正在思考…」） */
  assistantId: string
  /** 这一轮已经看到的失败原文 */
  failMsg?: string
  /** 失败 detail（v0.43 cli_outdated） */
  failDetail?: unknown
  /** 刷新之后接回来的：记录以会话文件为准，不跟当场的增量抢 */
  adopted?: boolean
}

const live = isCatLive()

/** 本机按对话记下的模型和思考强度。没记过的对话不继承别的对话。localStorage 不可用时只在这次运行里记。 */
let memoryPrefs: CatConvPrefMap = {}
/** 能力列表已经拉过（哪怕是空的）。没拉过时先显示记下的 id，不把上一条对话的选择带过来。 */
let capsLoaded = false
let capsTicket = 0

function loadPrefMap(): CatConvPrefMap {
  try {
    if (globalThis.localStorage) return readConvPrefs(globalThis.localStorage.getItem(CAT_CONV_PREFS_KEY))
  } catch {
    /* 读不到就当没记过 */
  }
  return memoryPrefs
}

function savePrefMap(map: CatConvPrefMap) {
  try {
    if (globalThis.localStorage) {
      globalThis.localStorage.setItem(CAT_CONV_PREFS_KEY, writeConvPrefs(map))
      return
    }
  } catch {
    /* 隐私模式写不进去时，这次运行里仍按对话分开记 */
  }
  memoryPrefs = map
}

function bootPref() {
  const id = live ? readLastConv() || NEW_CONV : 'c31'
  const pref = prefOf(loadPrefMap(), id)
  return { model: pref?.model ?? '', think: pref?.think ?? '', access: resolveAccess(pref?.access) }
}

const boot = bootPref()

export const catState = reactive({
  /** 当前对话的模型 id。每条对话分开记；没记过时列表回来后用第一个。 */
  model: boot.model,
  /**
   * 当前对话的思考强度 id。没点过就为空，发送时不带强度（后端不传 --reasoning-effort）；
   * 胶囊显示「默认」。换对话只换这一条的记录。
   */
  think: boot.think,
  /** 当前对话的批准方式。没记过是全部批准。 */
  /** 模式 id：v0.44 起存该 agentKind 的 mode id（旧四档 id 仍可读） */
  access: boot.access as string,
  /** 欢迎页模式；v0.36 三家可选，只影响新对话 */
  mode: 'cat_build',
  workInProject: true,
  sel: live ? readLastConv() || NEW_CONV : 'c31',
  /** 会话列表已从后端读完。没读完时不要因为「暂时找不到」把选中打回新对话 */
  convListReady: !live,
  projects: (live || (import.meta.env.DEV && catSim.has('noproj')) ? [] : mockProjects()) as CatProject[],
  /** 欢迎页（sel=new）属于哪个项目：从项目行「+」进来时为该项目 id；「新对话」/ 欢迎页为空 */
  newProjectId: '' as string,
  /** 新建 / 重复定位的项目行高亮（2.4s 后清除） */
  highlight: '' as string,
  /** 中间顶部浮提示（2.4s 自动消失）；key 变化用于重新播放动画 */
  notice: null as { text: string; key: number; tone: CatNoticeTone } | null,
  /** 读屏播报（不可见） */
  announce: '' as string,
  /** 系统平台（「在 … 中显示」文案） */
  platform: ((import.meta.env.DEV && catSimOs) || guessPlatform()) as CatPlatform,
  /** 正在行内改名的项目 id */
  renaming: '' as string,
  /** v0.40：添加项目双入口对话框 */
  addingProject: false,
  /** 删除确认框对应的项目 id */
  deleting: '' as string,
  /** 删除确认框对应的对话 id（设计 v0.3 §13；与 deleting 同一个确认框组件） */
  deletingConv: '' as string,
  plain: (live ? [] : [...(catSim.has('codexconv') ? [{ id: 'x1', title: 'Codex 自定义服务商', agentKind: CAT_AGENT_CODEX }] : []), ...(catSim.has('clioutdated') ? mockCliOutdatedConvs() : []), ...mockPlainConvs()]) as CatConv[],
  messages: {} as Record<string, CatBlock[]>,
  /** 新对话正在创建（欢迎页输入框忙） */
  creating: false,
  /** 进行中的回复，按会话 id。切换对话不取消这些回合 */
  turns: {} as Record<string, CatTurnState>,
  /** 回复结束时用户不在该对话上：侧栏蓝点。点进该对话后清掉 */
  unseen: {} as Record<string, boolean>,
  /** 这次打开应用期间，每条对话上次回复的结果。不写入数据库。进行中看 turns，不看这里。 */
  outcome: {} as Record<string, 'ok' | 'bad'>,
  /** 各会话最近一次上下文占用（已用 / 容量）。没有真实数字就不放，圆环不画 0%。 */
  context: (live ? {} : {
    c11: { used: 48200, window: 256000 },
    c12: { used: 91000, window: 256000 },
    c21: { used: 210000, window: 500000 },
    c22: { used: 64000, window: 256000 },
    c31: { used: 155377, window: 256000 },
  }) as Record<string, { used: number; window: number }>,
  /** 文件面板刷新记号：窗口重新聚焦、一轮结束时加一（不做文件监视） */
  filesRev: 0,
  /** 组件状态；非 ready 时显示未就绪横条（无下载按钮）；浏览器走查直接按 missing，避免短暂可点发送 */
  status: (live || (import.meta.env.DEV && catSim.has('checking'))
    ? { state: 'checking', version: '', canDownload: false, error: null, agents: [] }
    : (import.meta.env.DEV && catSim.has('stream'))
      ? {
          state: 'ready',
          version: '',
          canDownload: false,
          error: null,
          agents: catSim.has('allagents')
            ? ([
                { agentKind: 'cat_build', ready: true, version: '1.0' },
                { agentKind: 'claude_cli', ready: true, version: '1.0' },
                { agentKind: 'codex_cli', ready: true, version: '1.0' },
              ] as CatStatus['agents'])
            : [],
        }
    : { state: 'missing', version: '', canDownload: false, error: { code: 'CAT_NOT_READY', message: CAT_COPY.notReady }, agents: [] }) as CatStatus,
  /** 当前会话 agentKind 对应的能力列表（不造假） */
  models: [] as CatModel[],
  thinks: [] as CatThinkLevel[],
  /** v0.46：模型列表拉取时间（ms，输入区打开下拉时超过 10 分钟重拉）和拉不到时的状态（自定义开时才有） */
  modelsAt: 0,
  modelsFor: '' as string,
  providerModels: null as Pick<CatProviderModelList, 'state' | 'models'> | null,
  /** B1：正在读这条对话的记录（第一次打开、本地还没有内容时） */
  histLoading: {} as Record<string, boolean>,
  /** 记录读过了（读到空 / 没找到也算），用来区分「还在读」和「真的没有记录」 */
  histLoaded: {} as Record<string, boolean>,
  /** v0.45.1 historyStatus（空状态文案用） */
  histStatus: {} as Record<string, import('@/api/catHistoryStatus').CatHistoryStatus | undefined>,
  /** v0.44.5「重试」：can = 最后一轮失败且 retryable=true；gone = 点了但后端报 retry_not_allowed（原地灰字）。不写入数据库 */
  retry: {} as Record<string, 'can' | 'gone'>,
})

export const modelName = computed(() => {
  const m = catState.models.find((x) => x.id === catState.model)
  // 契约 6.19.3：显示后端给的模型名原样；列表里没有时退回 id；都没有时胶囊显示「Cat 助手」
  if (!catState.model && followsCliModel(currentAgentKind())) return DEFAULT_MODEL_LABEL
  return m?.displayName || catState.model || ''
})

/** 没选强度时胶囊 / 菜单显示的中性词（不显示任何一档的名字） */
export const THINK_DEFAULT_LABEL = '默认'

/** 强度显示名：无列表 → 空（不显示强度半截）；有列表没选 → 「默认」；选了 → 该档 displayName（任意 id 都按 displayName 显示） */
export const thinkName = computed(() => {
  const eff = currentCaps.value.efforts
  if (!eff.options.length) return ''
  return eff.options.find((t) => t.id === (catState.think || eff.defaultId))?.label ?? THINK_DEFAULT_LABEL
})

/** 合一胶囊文案：有强度才拼「·」；都没有时显示「Cat 助手」 */
export const capsuleLabel = computed(() => {
  const m = modelName.value
  const t = thinkName.value
  if (m && t) return `${m} · ${t}`
  if (m) return m
  return 'Cat 助手'
})

/** 当前会话 agentKind 的模式 / 强度列表（统一走 useCatAgentCapabilities；契约 v0.44 前模式用写死兜底） */
export const currentCaps = useCatAgentCapabilities(() => currentAgentKind(), () => catState.model)

/** 胶囊上显示的模式：当前选的不在这家列表里时显示该家默认 */
export const accessMeta = computed(() => {
  const list = currentCaps.value.modes
  const opt =
    list.options.find((o) => o.id === catState.access) ??
    list.options.find((o) => o.id === list.defaultId) ??
    list.options[0]
  return opt ? modeMeta(opt) : modeMeta({ id: CAT_ACCESS[CAT_ACCESS.length - 1].id, label: CAT_ACCESS[CAT_ACCESS.length - 1].name })
})

/** 组件没准备好（检测中不算）：显示未就绪横条 */
export const catNotReady = computed(() => catState.status.state === 'missing' || catState.status.state === 'failed')

/** 组件检查中：不显示横条，但发送置灰（产品：别让用户点了才被拒） */
export const catChecking = computed(() => catState.status.state === 'checking')

/** 当前视图所属项目：对话的项目，或从项目「+」进来的欢迎页的项目 */
export const currentProject = computed<CatProject | undefined>(() => {
  if (catState.sel === NEW_CONV) return catState.projects.find((p) => p.id === catState.newProjectId)
  return findConv(catState.sel)?.project
})

/** 当前项目文件夹不见了：输入框禁用 + 提示「项目文件夹不见了。」 */
export const projectMissing = computed(() => !!currentProject.value?.missing)

export const revealText = computed(() => revealLabel(catState.platform))

export function findConv(id: string): { project?: CatProject; conv: CatConv } | null {
  for (const p of catState.projects) {
    const c = p.convs.find((x) => x.id === id)
    if (c) return { project: p, conv: c }
  }
  const d = catState.plain.find((x) => x.id === id)
  return d ? { conv: d } : null
}

const loading = new Set<string>()

export function messagesOf(id: string): CatBlock[] {
  if (!catState.messages[id]) {
    if (live) {
      catState.messages[id] = []
      void loadMessages(id)
    } else {
      const f = findConv(id)
      if (f && import.meta.env.DEV && catSim.has('slowconv')) {
        // 走查：模拟读记录要 900ms，看切换对话的消息骨架
        catState.messages[id] = []
        catState.histLoading[id] = true
        setTimeout(() => {
          catState.messages[id] = mockMessages(id, f.conv.title)
          catState.histLoading[id] = false
        }, 900)
      } else catState.messages[id] = f ? mockMessages(id, f.conv.title) : []
    }
  }
  return catState.messages[id]
}

function blocksFrom(list: CatMessage[]): CatBlock[] {
  return list.map((m): CatBlock => {
    if (m.role === 'user') return { kind: 'user', text: m.content, files: m.media }
    if (m.role === 'status') return { kind: 'status', text: m.content, streaming: false }
    if (m.role === 'system') return { kind: 'sys', text: m.content, tone: 'err' }
    if (m.role === 'thought') return { kind: 'think', id: m.id, text: m.content }
    if (m.role === 'tool') return { kind: 'run', id: m.id, text: m.content }
    const a: Extract<CatBlock, { kind: 'a' }> = { kind: 'a', id: m.id, text: m.content, streaming: false }
    // v0.37.1：重开对话带回已持久化的消息尾；缺字段不补
    if (m.finishedAt && m.finishedAt > 0) a.finishedAt = m.finishedAt
    if (typeof m.tokenTotal === 'number' && Number.isFinite(m.tokenTotal)) a.tokenTotal = m.tokenTotal
    applyCatOutput(a, m)
    applyCatTiming(a, m)
    return a
  })
}

function blockSnap(list: CatBlock[]): { kind: string; text: string }[] {
  return list.map((b) => ({ kind: b.kind, text: 'text' in b && typeof b.text === 'string' ? b.text : '' }))
}

async function loadMessages(id: string, quiet = false, force = false) {
  if (id.startsWith('local-') || loading.has(id)) return
  const turn = catState.turns[id]
  // 本页发出的这一轮还在往外流：屏幕上就是最新的，不去用还没写完的记录覆盖。
  // 人点标题栏刷新时 force 为真，按文件重读这一段，也不停掉正在进行的回复。
  // 刷新接回来的那一轮相反，记录在文件里继续长，要反复读。
  if (!force && turn && !turn.adopted) return
  loading.add(id)
  if (!quiet && !catState.messages[id]?.length) catState.histLoading[id] = true
  try {
    if (import.meta.env.DEV && (catSim.has('histempty') || catSim.has('histmissing') || catSim.has('histreadfail'))) {
      // 走查：记录没找到（零轮）
      catState.messages[id] = []
      catState.histStatus[id] = catSim.has('histmissing') ? 'missing' : catSim.has('histreadfail') ? 'read_failed' : undefined
      catState.histLoaded[id] = true
      return
    }
    const d = await getCatConversation(id)
    if (!d) {
      if (findConv(id) && !catState.messages[id]?.length) catState.histLoaded[id] = true
      return
    }
    if (!findConv(id)) return
    catState.histLoaded[id] = true
    catState.histStatus[id] = d.historyStatus
    // 等待期间本页又自己发出了一轮：不要用文件里的旧记录盖住正在流的内容。
    const now = catState.turns[id]
    if (!force && now && !now.adopted) return
    if (d.contextUsed > 0 && d.contextWindow > 0) catState.context[id] = { used: d.contextUsed, window: d.contextWindow }
    const found = findConv(id)
    if (found && d.title) found.conv.title = d.title
    const next = blocksFrom(d.messages)
    if (catState.turns[id]?.status === 'running') {
      for (let i = next.length - 1; i >= 0; i--) {
        const row = next[i]
        if (row.kind === 'status') {
          row.streaming = true
          break
        }
      }
    }
    const cur = catState.messages[id]
    if (!cur || transcriptChanged(blockSnap(cur), blockSnap(next))) catState.messages[id] = next
    if (!catState.turns[id]) applyRetryable(id, d.retryTurnId)
  } catch {
    if (!quiet && catState.sel === id) notify('没能从助手里读出这段对话。', 'warn')
  } finally {
    loading.delete(id)
    catState.histLoading[id] = false
  }
}

/** GetCatConversation 的 retryable（v0.44.5）：最后一轮失败且可重试 → 显示「重试」；否则不显示（已显示的灰字保留） */
function applyRetryable(id: string, turnId: string) {
  if (!turnId) {
    lastFailedTurn.delete(id)
    if (catState.retry[id] === 'can') delete catState.retry[id]
    return
  }
  lastFailedTurn.set(id, turnId)
  catState.retry[id] = 'can'
  const list = catState.messages[id] ?? (catState.messages[id] = [])
  const tail = list[list.length - 1]
  if (!(tail?.kind === 'sys' && tail.tone === 'err')) list.push({ kind: 'sys', text: CAT_COPY.replyFailed, tone: 'err' })
}

/** 标题栏刷新：重读当前对话的记录。不取消正在进行的回复。 */
export async function reloadConversation(id: string): Promise<void> {
  if (!id || id === NEW_CONV) return
  await loadMessages(id, false, true)
}

let livePoll: ReturnType<typeof setInterval> | undefined
let seq = 0

function ensureLivePoll() {
  if (livePoll) return
  livePoll = setInterval(() => { void pollLiveTurns() }, 1000)
}

function stopLivePoll() {
  if (!livePoll) return
  clearInterval(livePoll)
  livePoll = undefined
}

function heldTurns() {
  return Object.entries(catState.turns).map(([convId, t]) => ({ convId, adopted: !!t.adopted }))
}

/** 打开页面或发送被拒绝时：把还在跑的对话接回来，侧栏继续转，记录接着长。 */
async function adoptLiveTurns() {
  const liveTurns = await listLiveCatTurns()
  if (!liveTurns) return
  const plan = liveTurnPlan(heldTurns(), liveTurns)
  for (const t of plan.adopt) {
    if (!findConv(t.convId)) continue
    catState.turns[t.convId] = { token: ++seq, turnId: t.turnId, status: 'running', assistantId: '', adopted: true }
    void loadMessages(t.convId, true)
  }
  if (Object.values(catState.turns).some((t) => t.adopted)) ensureLivePoll()
}

async function pollLiveTurns() {
  const held = heldTurns()
  if (!held.some((h) => h.adopted)) {
    stopLivePoll()
    return
  }
  const liveTurns = await listLiveCatTurns()
  // 这一次没问到：保持转圈，不要当成进程已经结束。
  if (!liveTurns) return
  const plan = liveTurnPlan(held, liveTurns)
  for (const id of plan.refresh) void loadMessages(id, true)
  for (const id of plan.release) {
    const token = catState.turns[id]?.token
    void finishAdopted(id, token)
  }
}

async function finishAdopted(id: string, token?: number) {
  if (loading.has(id)) await new Promise((r) => setTimeout(r, 40))
  await loadMessages(id, true)
  const turn = catState.turns[id]
  // 本进程拉起的这一轮还会发结束事件，成功或失败写在那条事件里。先让它落地。
  if (turn?.adopted && turn.turnId && turn.token === token && turn.status !== 'stopping') {
    await new Promise((r) => setTimeout(r, 400))
  }
  releaseAdopted(id, token)
}

function releaseAdopted(id: string, token?: number) {
  const turn = catState.turns[id]
  if (!turn || !turn.adopted || (token !== undefined && turn.token !== token)) return
  // 人已经点了停止：等原来的停止收尾，这里不另报成功或失败。
  if (turn.status === 'stopping') return
  delete catState.turns[id]
  quietStreaming(id)
}

function toConv(c: CatConversation): CatConv {
  return { id: c.id, title: c.title, agentKind: c.agentKind, projectId: c.projectId }
}

/** 当前选中对话的 agentKind；新对话用欢迎页选中模式（v0.36 三家可点） */
export function currentAgentKind(): CatAgentKind {
  if (catState.sel === NEW_CONV) {
    const m = CAT_MODES.find((x) => x.id === catState.mode)
    return m?.enabled ? m.agentKind : CAT_AGENT_BUILD
  }
  return findConv(catState.sel)?.conv.agentKind ?? CAT_AGENT_BUILD
}

/** 把当前对话记下的模型和强度摆到界面上。列表还没到时先用记下的 id。 */
function showConvPref(convId: string) {
  // v0.35：欢迎页没选过时用 Cat 设置里的新对话默认值；已有对话不回填
  const pref = prefOf(loadPrefMap(), convId) ?? (convId === NEW_CONV ? defaultConvPref(currentAgentKind()) : null)
  catState.access = accessFor(pref?.access)
  if (!capsLoaded) {
    catState.model = pref?.model ?? ''
    catState.think = pref?.think ?? ''
    return
  }
  // v0.46：自定义开时手填的 / 不在当前列表的模型照常保留（显示 id + 「不在当前列表」），不自动改
  if (customProviderOn(currentAgentKind()) && pref?.model && !catState.models.some((m) => m.id === pref.model)) {
    catState.model = pref.model
    catState.think = ''
    return
  }
  const resolved = resolveConvPref(pref, catState.models, catState.thinks)
  // Claude：没选过 / 选的不在列表里 → 「默认模型」（空，跟随 CLI），不自动选第一个
  if (followsCliModel(currentAgentKind()) && !(pref?.model && catState.models.some((m) => m.id === pref.model))) resolved.model = ''
  catState.model = resolved.model
  catState.think = resolved.think
}

/** v0.46：这家开了自定义服务商（输入区模型下拉加「手动填写…」） */
export function customProviderOn(kind: CatAgentKind = currentAgentKind()): boolean {
  return isProviderKind(kind) && !!catPanels.settings.providersByAgent?.[kind]?.enabledCustom
}

/** 输入区打开模型下拉时调用：距上次拉取超过 10 分钟（或换了家）就重拉 */
export function refreshModelsIfStale(now = Date.now()): void {
  const kind = currentAgentKind()
  if (catState.modelsFor !== kind || isModelListStale(catState.modelsAt, now)) void refreshCapabilities(kind)
}

/** 用户点了模型、强度或批准方式：只写当前这条对话。 */
function rememberSelection() {
  const id = catState.sel
  if (!id) return
  savePrefMap(putPref(loadPrefMap(), id, { model: catState.model, think: catState.think, access: catState.access }))
}

export function chooseModel(id: string) {
  if (!id && !followsCliModel(currentAgentKind())) return
  catState.model = id
  // 强度按模型：新模型不支持当前强度 → 退回该模型默认
  catState.think = pickSupported(getCatAgentCapabilities(currentAgentKind(), id).efforts, catState.think)
  rememberSelection()
}

/** id 为空 = 回到「默认」（不传强度） */
export function chooseThink(id: string) {
  catState.think = id
  rememberSelection()
}

/** 当前这家的 mode id（v0.44）；后端还没下发列表时沿用旧四档规则 */
function accessFor(id: string | undefined | null): string {
  const kind = currentAgentKind()
  const caps = getCatAgentCapabilities(kind)
  return caps.modes === FALLBACK_MODES ? resolveAccess(id) : resolveModeId(kind, id)
}

export function chooseAccess(id: string) {
  catState.access = accessFor(id)
  rememberSelection()
}

/** 欢迎页上选的模型和强度跟着新对话走，不写到别的对话上。 */
function carryDraftPref(toId: string) {
  const map = loadPrefMap()
  // 欢迎页没点过：把设置里的默认值写给新对话（契约 6.19.12.1-3）
  if (!prefOf(map, NEW_CONV)) {
    const d = defaultConvPref(currentAgentKind())
    if (d.model || d.think || d.access) savePrefMap(putPref(map, toId, d))
    return
  }
  savePrefMap(carryPref(map, NEW_CONV, toId))
}

/**
 * 按会话 agentKind 拉模型/强度，不造假。
 * 拉完后只套当前对话记下的选择：模型不在列表里用第一个，强度不在列表里留空。
 * 退回的值不写回记录，所以别的对话、以及这档以后又出现时，都还是原来选的。
 */
export async function refreshCapabilities(agentKind: CatAgentKind = currentAgentKind()) {
  const ticket = ++capsTicket
  const sim = import.meta.env.DEV && (catSim.has('caps') || catSim.has('codexcaps'))
  // eslint-disable-next-line prefer-const
  let [models, thinks] = await Promise.all(
    sim
      ? [Promise.resolve(simAgentRaw(agentKind).models as CatModel[]), Promise.resolve([] as CatThinkLevel[])]
      : [listCatModels(agentKind), listCatThinkLevels(agentKind)],
  )
  if (ticket !== capsTicket) return
  // v0.46：自定义开时 ListCatModels 已经是服务商列表；拉不到（空）时再问一次状态，给一行提示（后端有 10 分钟缓存，不重拉）
  let pm: Pick<CatProviderModelList, 'state' | 'models'> | null = null
  if (!sim && customProviderOn(agentKind)) {
    pm = models.length ? { state: 'ok', models } : await listCatProviderModels(agentKind as CatProviderKind, false)
    if (ticket !== capsTicket) return
    if (!models.length && pm.state === 'ok' && pm.models.length) models = pm.models // 纯浏览器走查：ListCatModels 不模拟服务商
  }
  catState.providerModels = pm
  catState.modelsAt = Date.now()
  catState.modelsFor = agentKind
  catState.models = models
  catState.thinks = thinks
  if (sim) setBackendCaps(agentKind, { thinks, ...mapBackendCaps(simAgentRaw(agentKind)) })
  else {
    const st = catState.status.agents?.find((a) => a.agentKind === agentKind)
    setBackendCaps(agentKind, { thinks, modes: mapAgentModes({ modes: st?.rawModes }), effortsByModel: mapModelEfforts(models) })
  }
  capsLoaded = true
  showConvPref(catState.sel)
}

// ---------- 进入页面：状态、会话列表、事件 ----------

let offs: Array<() => void> = []

/** 未就绪横幅里的「重新检测」：先显示检查中，再用结果覆盖 */
export async function recheckCatStatus(): Promise<void> {
  if (catState.status.state === 'checking') return
  await recheckCatAgents()
}

/** v0.47.1：cat:status / Get / Recheck 的结果一律按 agents[] 合并进 store，不整份替换 */
export function applyCatStatus(s: CatStatus): void {
  const was = catState.status.state
  catState.status = mergeCatStatus(catState.status, s, s.hasAgents === true)
  if (catState.status.state === 'ready' && was !== 'ready') void refreshCapabilities()
}

/**
 * 重新检测的唯一入口。agentKind 空 = 全部（全部行进检测中）；传了只把该行置检测中，其它行不动。
 * 旧绑定不支持参数时 recheckCat 自动退化为全量，但界面上仍只动该行。
 */
export async function recheckCatAgents(agentKind?: CatProviderKind): Promise<void> {
  const prev = catState.status
  catState.status = agentKind
    ? markRowChecking(prev, agentKind)
    : { ...prev, state: 'checking', agents: (prev.agents ?? []).map((a) => ({ ...a, checking: true })) }
  try {
    if (import.meta.env.DEV && catSim.size && !live) {
      await new Promise((r) => setTimeout(r, 900))
      const done = { ...catState.status, state: prev.state, agents: (catState.status.agents ?? []).map((a) => (!agentKind || a.agentKind === agentKind ? { ...a, checking: false, stale: false, checkedAt: Date.now() } : a)) }
      applyCatStatus({ ...done, hasAgents: true })
      return
    }
    applyCatStatus(await recheckCat(agentKind))
  } catch {
    // 失败：撤回检测中，保留上次结果
    catState.status = { ...catState.status, state: prev.state === 'checking' ? 'missing' : prev.state, agents: (catState.status.agents ?? []).map((a) => ({ ...a, checking: prev.agents?.find((p) => p.agentKind === a.agentKind)?.checking ?? false })) }
  }
}

/** CatPage 挂载时调用；返回卸载函数 */
export function initCat(): () => void {
  disposeCat()
  offs = [
    onCatStatus(applyCatStatus),
    onCatMessage(handleMessageEvent),
    onCatTurn(handleTurnEvent),
    onCatProject(handleProjectEvent),
  ]
  if (typeof window !== 'undefined') {
    window.addEventListener('focus', onWindowFocus)
    offs.push(() => window.removeEventListener('focus', onWindowFocus))
  }
  if (hasWailsBackend()) {
    void Environment()
      .then((e) => {
        if (e?.platform === 'windows' || e?.platform === 'darwin' || e?.platform === 'linux') catState.platform = e.platform
      })
      .catch(() => undefined)
  }
  if (catProjectsSim()) seedProjectSim()
  // 开发走查（?cat_sim=checking|stream）：保持模拟的组件状态，不被浏览器 mock 的 missing 覆盖
  const detectSim = import.meta.env.DEV ? simDetectPayloads() : []
  for (const p of detectSim) {
    const t = setTimeout(() => applyCatStatus(mapCatStatus(p.raw)), p.ms)
    offs.push(() => clearTimeout(t))
  }
  const simStatus = import.meta.env.DEV && (catSim.has('checking') || catSim.has('stream') || detectSim.length > 0)
  if (!simStatus) {
    void getCatStatus()
      .then(applyCatStatus)
      .catch(() => (catState.status = { state: 'missing', version: '', canDownload: false, error: { code: 'CAT_NOT_READY', message: CAT_COPY.notReady }, agents: [] }))
  }
  if (live) void reloadConversations().finally(() => { void adoptLiveTurns() })
  else void refreshProjects()
  return disposeCat
}

// 同步写入：点完对话马上关掉窗口也不会丢掉。换对话立刻换成那一条自己的模型和强度。
watch(() => catState.sel, (id) => {
  if (isCatLive()) writeLastConv(id)
  showConvPref(id)
}, { flush: 'sync' })

export function disposeCat() {
  offs.forEach((f) => f())
  offs = []
  stopLivePoll()
}

export async function reloadConversations() {
  catState.convListReady = false
  try {
    const [list, projects] = await Promise.all([listCatConversations(), listCatProjects().catch(() => null)])
    if (projects) mergeProjects(projects, true)
    const byId = new Map(catState.projects.map((p) => [p.id, p]))
    for (const p of catState.projects) p.convs = []
    const plain: CatConv[] = []
    // 后端已按 6.19.10.6 排好，分组时保持顺序；项目不认识（开关关 / 已删）的放「对话」
    for (const c of list) {
      // 刚删掉的对话：删除前发出的列表请求晚回来时不让它复活
      if (removedConvs.has(c.id)) continue
      const p = c.projectId ? byId.get(c.projectId) : undefined
      if (p) p.convs.push(toConv(c))
      else plain.push(toConv(c))
    }
    catState.plain = plain
    keepLastConv()
    // 没有进行中回复的会话下次打开时重新读（离开页面期间可能错过事件）
    for (const id of Object.keys(catState.messages)) if (!catState.turns[id]) delete catState.messages[id]
  } catch {
    /* 保持现有列表 */
  } finally {
    catState.convListReady = true
  }
}

/** 列表到位后：上次的对话还在就留在上面，并展开它所在的项目；已经删了才回新对话 */
function keepLastConv() {
  const id = catState.sel
  if (!id || id === NEW_CONV) return
  const found = findConv(id)
  if (!found) {
    catState.sel = NEW_CONV
    return
  }
  if (found.project) found.project.open = true
}

// ---------- 流式 ----------

const streams = new Map<string, CatStreamMsg>()

function assistantBlock(convId: string, messageId: string): Extract<CatBlock, { kind: 'a' }> | undefined {
  const list = catState.messages[convId]
  if (!list) return undefined
  for (let i = list.length - 1; i >= 0; i--) {
    const b = list[i]
    if (b.kind === 'a' && b.id === messageId) return b
  }
  return undefined
}

function activityBlock(convId: string, messageId: string): Extract<CatBlock, { kind: 'think' | 'run' }> | undefined {
  const list = catState.messages[convId]
  if (!list) return undefined
  for (let i = list.length - 1; i >= 0; i--) {
    const b = list[i]
    if ((b.kind === 'think' || b.kind === 'run') && b.id === messageId) return b
  }
  return undefined
}

/** 已经收尾的轮次。迟到事件不能再开转圈，也不能写进下一轮。 */
const sealedTurns = new Set<string>()

function sealTurn(turnId: string) {
  const id = turnId.trim()
  if (!id) return
  sealedTurns.delete(id)
  sealedTurns.add(id)
  while (sealedTurns.size > 100) {
    const oldest = sealedTurns.values().next().value
    if (!oldest) break
    sealedTurns.delete(oldest)
  }
}

/** 重试状态只在还没继续回复时留着。 */
function dropStatus(convId: string) {
  const list = catState.messages[convId]
  if (!list) return
  for (let i = list.length - 1; i >= 0; i--) {
    if (list[i].kind === 'status') list.splice(i, 1)
  }
}

/** 思考、工具和正文停掉转圈。点停止和回合结束都走这里。 */
function quietStreaming(convId: string) {
  const list = catState.messages[convId]
  if (!list) return
  for (const b of list) {
    if ((b.kind === 'a' || b.kind === 'think' || b.kind === 'run') && b.streaming) b.streaming = false
    if ((b.kind === 'think' || b.kind === 'run') && b.id) streams.delete(b.id)
  }
}

export function handleMessageEvent(e: CatStreamEvent) {
  if (e.role === 'user') return // 用户消息本地已先放
  // 已不在列表里的对话（刚删掉的）：直接忽略，不报错也不重建状态
  if (!findConv(e.convId)) return
  // 刷新接回来的回合以文件为准，当场增量会和整段重读叠成两份。
  if (catState.turns[e.convId]?.adopted) return
  const turn = catState.turns[e.convId]
  // 这一轮已经停止或收尾：不再追加，也不要把思考重新转起来。
  // 下一轮已经开始时，只丢掉旧编号，不能把新一轮的转圈清掉。
  if (e.turnId && sealedTurns.has(e.turnId)) {
    if (!turn || turn.turnId === e.turnId || turn.status === 'stopping') quietStreaming(e.convId)
    return
  }
  if (!turn || turn.status === 'stopping') {
    quietStreaming(e.convId)
    return
  }
  if (e.turnId && turn.turnId && e.turnId !== turn.turnId) return
  if (e.turnId && !turn.turnId) turn.turnId = e.turnId
  const list = catState.messages[e.convId]
  if (!list) return // 这条会话还没打开过，打开时会从后端整段读

  let m = streams.get(e.messageId)
  if (!m) {
    if (assistantBlock(e.convId, e.messageId) && !turn) return // 已经是落盘的完整消息
    m = newStreamMsg(e.messageId)
    streams.set(e.messageId, m)
  }
  const changed = applyStreamEvent(m, e)
  if (!changed) return
  if (e.blockType === 'retry') {
    const text = m.text.trim()
    const idx = list.findIndex((b) => b.kind === 'status')
    if (!text) {
      if (idx >= 0) list.splice(idx, 1)
      return
    }
    if (idx >= 0 && list[idx].kind === 'status') {
      list[idx].text = text
      list[idx].streaming = true
    } else {
      list.push({ kind: 'status', id: e.messageId, text, streaming: true })
    }
    return
  }
  if (e.blockType === 'error') {
    dropStatus(e.convId)
    const text = m.text.trim()
    if (text) {
      const dup = list.some((b) => b.kind === 'sys' && b.tone === 'err' && b.text === text)
      if (!dup) list.push({ kind: 'sys', text, tone: 'err' })
      if (turn && !turn.failMsg) turn.failMsg = text
    }
    streams.delete(e.messageId)
    return
  }
  if (e.blockType === 'thought' || e.blockType === 'tool') {
    dropStatus(e.convId)
    const kind = e.blockType === 'thought' ? 'think' : 'run'
    let row = activityBlock(e.convId, e.messageId)
    if (!row) {
      if (!m.text && m.done) return
      list.push({ kind, id: e.messageId, text: m.text, streaming: !m.done })
    } else {
      row.text = m.text
      row.streaming = !m.done
    }
    if (m.done) streams.delete(e.messageId)
    return
  }
  dropStatus(e.convId)
  let b = assistantBlock(e.convId, e.messageId)
  if (!b) {
    if (!m.text && m.done) return
    list.push({ kind: 'a', id: e.messageId, text: m.text, streaming: !m.done, reveal: true })
    b = list[list.length - 1] as Extract<CatBlock, { kind: 'a' }>
  } else {
    b.text = m.text
    b.streaming = !m.done
    if (!m.done) b.reveal = true
  }
  if (turn && !turn.assistantId) turn.assistantId = e.messageId
  if (m.done) streams.delete(e.messageId)
}

export function handleTurnEvent(e: CatTurnEvent) {
  // 删除时后端可能还会发一条 cat:turn cancelled：对话已不在列表里就忽略
  if (!findConv(e.convId)) return
  if (e.contextUsed && e.contextWindow) catState.context[e.convId] = { used: e.contextUsed, window: e.contextWindow }
  const turn = catState.turns[e.convId]
  // 旧轮次的终态迟到：这一轮还在本地就继续收尾；已经收掉、或已经是下一轮，就只停转圈
  if (e.turnId && sealedTurns.has(e.turnId) && turn?.turnId !== e.turnId) {
    if ((!turn || turn.status === 'stopping') && e.status !== 'running') quietStreaming(e.convId)
    return
  }
  if (!turn) {
    if (e.status !== 'running') {
      if (e.turnId) sealTurn(e.turnId)
      quietStreaming(e.convId)
    }
    return
  }
  // 接回来的回合先把文件里的记录读完，再收尾。先收尾的话，后到的整段记录会把「已停止生成。」盖掉。
  if (turn.adopted && e.status !== 'running') {
    const token = turn.token
    const convId = e.convId
    const status = e.status
    const errorCode = e.errorCode
    const errorMessage = e.errorMessage
    const errorDetail = e.errorDetail
    const turnId = e.turnId
    void loadMessages(convId, true).finally(() => {
      const cur = catState.turns[convId]
      if (!cur || cur.token !== token) return
      if (turnId && cur.turnId && turnId !== cur.turnId) return
      const incoming = (errorMessage || '').trim()
      if (incoming && incoming !== CAT_COPY.replyFailed) cur.failMsg = incoming
      else if (!cur.failMsg && incoming) cur.failMsg = incoming
      if (errorDetail != null) cur.failDetail = errorDetail
      if (cur.status === 'stopping' || status === 'cancelled') endTurn(convId, 'cancelled', token, e)
      else if (status === 'completed') endTurn(convId, 'completed', token, e)
      else endTurn(convId, errorCode === 'CAT_NOT_READY' ? 'not_ready' : 'failed', token, e)
    })
    return
  }
  if (e.turnId && turn.turnId && e.turnId !== turn.turnId) return
  if (e.status === 'running') {
    if (e.turnId && !turn.turnId) turn.turnId = e.turnId
    return
  }
  const incoming = (e.errorMessage || '').trim()
  if (incoming && incoming !== CAT_COPY.replyFailed) turn.failMsg = incoming
  else if (!turn.failMsg && incoming) turn.failMsg = incoming
  if (e.errorDetail != null) turn.failDetail = e.errorDetail
  if (e.status === 'completed') endTurn(e.convId, 'completed', undefined, e)
  else if (e.status === 'cancelled') endTurn(e.convId, 'cancelled', undefined, e)
  else endTurn(e.convId, e.errorCode === 'CAT_NOT_READY' ? 'not_ready' : 'failed', undefined, e)
}

type TurnEnd = 'completed' | 'cancelled' | 'failed' | 'not_ready'

const stopTimers = new Map<string, ReturnType<typeof setTimeout>>()

/** 把终态 cat:turn 上的消息尾挂到助手气泡（无 finishedAt 不挂；不造假） */
function applyTurnFooter(convId: string, turn: CatTurnState, ev?: CatTurnEvent) {
  if (!ev) return
  const finishedAt = typeof ev.finishedAt === 'number' && ev.finishedAt > 0 ? ev.finishedAt : 0
  if (!finishedAt) return
  const list = catState.messages[convId]
  if (!list) return
  const mid = (ev.messageId || turn.assistantId || '').trim()
  let b = mid ? assistantBlock(convId, mid) : undefined
  if (!b) {
    for (let i = list.length - 1; i >= 0; i--) {
      const row = list[i]
      if (row.kind === 'a') {
        b = row
        break
      }
    }
  }
  if (!b) return
  b.finishedAt = finishedAt
  if (typeof ev.tokenTotal === 'number' && Number.isFinite(ev.tokenTotal)) b.tokenTotal = ev.tokenTotal
  else delete b.tokenTotal
  applyCatOutput(b, ev) // v0.47：输出 token + 后端 token/s
  // v0.45：每轮按事件重写；缺的删掉（重试不残留旧值）
  applyCatTiming(b, ev)
}

/** 结束一轮（事件和 SendCatMessage 的返回谁先到谁算，只处理一次） */
const simFailed = new Set<string>()
/** 每条对话最后一轮失败的 turnId（RetryCatTurn 用） */
const lastFailedTurn = new Map<string, string>()

/** 未绑定项目的对话：查一次对话文件夹是否有文件（面板显示规则，#271） */
export function refreshConvScratch(convId: string): void {
  if (!convId || convId === NEW_CONV) return
  const bound = !!findConv(convId)?.project
  void refreshScratch(convId, bound, async (id) => {
    if (catBackendLive()) return (await listCatFiles(id)).entries.length > 0
    return import.meta.env.DEV && catSim.has('scratchfiles')
  })
}

function endTurn(convId: string, how: TurnEnd, token?: number, ev?: CatTurnEvent) {
  const turn = catState.turns[convId]
  if (!turn || (token !== undefined && turn.token !== token)) return
  refreshConvScratch(convId)
  if (turn.status === 'stopping') how = 'cancelled'
  if (turn.turnId) sealTurn(turn.turnId)
  clearTimeout(stopTimers.get(convId))
  stopTimers.delete(convId)
  delete catState.turns[convId]
  if (how === 'completed') catState.outcome[convId] = 'ok'
  else if (how === 'failed' || how === 'not_ready') catState.outcome[convId] = 'bad'
  else delete catState.outcome[convId]
  // v0.44.5：只信后端的 retryable（失败事件），前端不再自行推断
  if (how === 'failed' && turn.turnId && ev?.retryable === true) {
    lastFailedTurn.set(convId, turn.turnId)
    catState.retry[convId] = 'can'
  } else {
    lastFailedTurn.delete(convId)
    delete catState.retry[convId]
  }
  const list = catState.messages[convId] ?? (catState.messages[convId] = [])
  // 收起流式光标，已流出来的文字原样保留（不加后缀）
  quietStreaming(convId)
  dropStatus(convId)
  // v0.37.1：终态后挂消息尾（进行中不会走到这里）
  applyTurnFooter(convId, turn, ev)
  // 人已经离开这个对话：结束后在侧栏打蓝点。人还在看就不打
  if (how === 'completed' && catState.sel !== convId) catState.unseen[convId] = true
  else delete catState.unseen[convId]
  if (turn.assistantId) streams.delete(turn.assistantId)
  catState.filesRev++
  if (how === 'cancelled') list.push({ kind: 'sys', text: CAT_COPY.stopped })
  else if (how === 'failed') {
    const outdated = parseCliOutdated(turn.failDetail, findConv(convId)?.conv.agentKind ?? currentAgentKind())
    if (outdated) {
      // v0.43：CLI 版本过旧单独一行；原始报错收进「查看详情」（raw 为空则不出，v0.43.1）
      list.push({ kind: 'sys', text: CAT_CLI_OUTDATED_COPY.body(outdated.cli), tone: 'err', outdated })
    } else {
    // v0.46：Codex 服务商没有 Responses 接口（retryable=false → 不出重试）；错误行样式同 cli_outdated
    const provText = providerFailureText(turn.failDetail)
    if (provText) list.push({ kind: 'sys', text: provText, tone: 'err', providerErr: true })
    else {
    const text = (turn.failMsg || '').trim() || CAT_COPY.replyFailed
    if (!list.some((b) => b.kind === 'sys' && b.tone === 'err' && b.text === text)) {
      list.push({ kind: 'sys', text, tone: 'err' })
    }
    }
    }
  }
  else if (how === 'not_ready') list.push({ kind: 'sys', text: catState.status.error?.message || CAT_COPY.notReady })
  // v0.40：完成后通知 — 前台 toast；cancelled 不发；开关关则不发（系统通知由后端在后台发）
  if (how === 'completed' || how === 'failed' || how === 'not_ready') {
    const title = findConv(convId)?.conv.title || ''
    const errText = how === 'completed' ? '' : ((how === 'failed' && providerFailureText(turn.failDetail)) || (turn.failMsg || '').trim() || (how === 'not_ready' ? (catState.status.error?.message || CAT_COPY.notReady) : CAT_COPY.replyFailed))
    showTurnFinishToast(convId, how === 'completed' ? 'completed' : 'failed', title, errText)
  }
}

function fileChips(files: CatOutgoingFile[]): CatShownFile[] | undefined {
  if (!files.length) return undefined
  return files.map((f) => ({ name: f.name || f.path, path: f.path, kind: previewKind(f.name || f.path) }))
}

/**
 * 发送：新对话创建时写入 agentKind（不可再改）；已有对话只带会话 id。
 * 返回值：后端没收下这条消息（CAT_PROJECT_MISSING：没建对话 / 没存用户消息）且用户还停在原处时返回原文和附件，
 * 调用方放回输入框；其它情况返回 undefined。
 */
export async function sendMessage(text: string, files: CatOutgoingFile[] = []): Promise<{ text: string; files: CatOutgoingFile[] } | undefined> {
  const t = text.trim()
  const picked = files.filter((f) => f.path)
  if ((!t && !picked.length) || catState.creating) return
  const titleSeed = t || picked[0]?.name || ''
  let id = catState.sel
  if (id !== NEW_CONV && catState.turns[id]) return
  // 检查中：按钮已置灰；回车等其它入口也直接忽略
  if (catState.status.state === 'checking') return
  // 组件未就绪：只展示定稿文案，不启一轮、不造假流式回复（产品锁定）
  if (catState.status.state === 'missing' || catState.status.state === 'failed') {
    if (id === NEW_CONV) {
      // 欢迎页：不建会话，只在当前页提示；已有会话则追加一条系统行
      return
    }
    const list = messagesOf(id)
    if (!list.some((b) => b.kind === 'sys' && b.text === CAT_COPY.notReady)) {
      list.push({ kind: 'sys', text: CAT_COPY.notReady })
    }
    return
  }

  // 项目文件夹不见了：输入框已禁用；回车等其它入口也直接忽略（对话可看不可发）
  if (projectMissing.value) return

  if (id === NEW_CONV) {
    const mode = CAT_MODES.find((m) => m.id === catState.mode)
    if (!mode?.enabled) catState.mode = 'cat_build'
    // v0.36：按欢迎页选中的胶囊锁定 agentKind
    const agentKind: CatAgentKind = (CAT_MODES.find((m) => m.id === catState.mode)?.agentKind ?? CAT_AGENT_BUILD) as CatAgentKind
    const project = catState.projects.find((p) => p.id === catState.newProjectId)
    catState.creating = true
    try {
      const created = await createCatConversation({ agentKind, projectId: project?.id })
      id = created.id
      const conv = toConv(created)
      if (!conv.title) conv.title = titleFrom(titleSeed)
      if (project) {
        conv.projectId = project.id
        project.convs.unshift(conv)
        project.open = true
        rememberNewConvProject(project.id)
      } else catState.plain.unshift(conv)
      catState.newProjectId = ''
    } catch (e) {
      if (project && toAppError(e).code === 'CAT_PROJECT_MISSING') {
        // 不创建对话；欢迎页随之显示「项目文件夹不见了。」，文字放回输入框
        project.missing = true
        return catState.sel === NEW_CONV && catState.newProjectId === project.id ? { text, files: picked } : undefined
      }
      id = `local-${Date.now().toString(36)}-${++seq}`
      if (project) {
        project.convs.unshift({ id, title: titleFrom(titleSeed), agentKind, projectId: project.id })
        catState.newProjectId = ''
      } else catState.plain.unshift({ id, title: titleFrom(titleSeed), agentKind })
      carryDraftPref(id)
      catState.messages[id] = [{ kind: 'user', text: t, files: fileChips(picked) }, { kind: 'sys', text: CAT_COPY.replyFailed, tone: 'err' }]
      catState.outcome[id] = 'bad'
      catState.sel = id
      return
    } finally {
      catState.creating = false
    }
    carryDraftPref(id)
    catState.messages[id] = []
    catState.sel = id
  }

  const list = messagesOf(id)
  for (let i = list.length - 1; i >= 0; i--) if (list[i].kind === 'run') list.splice(i, 1)
  list.push({ kind: 'user', text: t, files: fileChips(picked) })
  const token = ++seq
  catState.turns[id] = { token, turnId: '', status: 'running', assistantId: '' }
  // 新一轮开始：前一失败轮不再可重试（6.19.18.7 第 7 条）
  lastFailedTurn.delete(id)
  delete catState.retry[id]

  if (import.meta.env.DEV && catSim.has('stream')) {
    // 仅 vite dev + 纯浏览器 + ?cat_sim=stream（见 catDevSim.ts）；正式包里 catSim 恒为空
    if (catSim.has('codexnoresponses')) {
      // 走查 v0.46：Codex 自定义服务商没有 /responses → CAT_REPLY_FAILED + reason，retryable=false
      const ft = catState.turns[id]
      ft.turnId = `sim-noresp-${token}`
      setTimeout(() => handleTurnEvent({ convId: id, turnId: ft.turnId, status: 'failed', retryable: false, errorCode: 'CAT_REPLY_FAILED', errorMessage: CAT_COPY.replyFailed, errorDetail: 'reason=provider_responses_unsupported\nagentKind=codex_cli' }), 300)
      return
    }
    if (catSim.has('failonce') && !simFailed.has(id)) {
      // 走查 6.19.18.7：第一轮直接失败，再点「重试」原地重发
      simFailed.add(id)
      const ft = catState.turns[id]
      ft.turnId = `sim-fail-${token}`
      // 失败事件带 retryable=true（v0.44.5）
      setTimeout(() => endTurn(id, 'failed', token, { convId: id, turnId: ft.turnId, status: 'failed', retryable: true }), 300)
      return
    }
    simStream(id, handleMessageEvent, handleTurnEvent, () => catState.turns[id]?.token !== token || catState.turns[id]?.status === 'stopping')
    return
  }
  try {
    const res = await sendCatMessage({
      conversationId: id,
      content: t,
      modelId: catState.model || undefined,
      // 没选强度就不带（接口层转成空串，后端不传 --reasoning-effort）
      thinkLevelId: catState.think || undefined,
      permissionMode: catState.access,
      attachments: picked.length ? picked.map((f) => ({ name: f.name, path: f.path })) : undefined,
    })
    const turn = catState.turns[id]
    if (turn?.token === token && res.turnId && !turn.turnId) turn.turnId = res.turnId
    // Send 还没返回时就点了停止：那时后端可能还没登记这一轮，拿到 turnId 后再取消一次（幂等）
    if (res.turnId && turn?.token === token && turn.status === 'stopping') {
      sealTurn(res.turnId)
      void cancelCatTurn({ convId: id, turnId: res.turnId }).catch(() => undefined)
    } else if (res.turnId && (!turn || turn.token !== token)) {
      // 这一轮已经收尾之后 Send 才返回：记下编号，下一轮不会吃到它的迟到事件
      sealTurn(res.turnId)
    }
    // v0.30.1：Send 在本轮开始后立即返回，结束由 cat:turn 决定；
    // 只有兼容旧的同步返回（直接带了助手回复）时才在这里收尾
    if (turn?.token === token && res.assistantMessage?.content) {
      if (turn.status !== 'stopping') {
        const a = res.assistantMessage
        const b = assistantBlock(id, a.id)
        if (!b) messagesOf(id).push({ kind: 'a', id: a.id, text: a.content, streaming: false, reveal: true })
        else if (!b.text) b.text = a.content
      }
      endTurn(id, 'completed', token)
    }
  } catch (e) {
    const err = toAppError(e)
    // 发送途中对话被删了：什么都不做（不重建消息）
    if (!findConv(id)) return
    if (err.code === 'CAT_PROJECT_MISSING') {
      // 6.19.10.2 第 6 条：后端没存这条用户消息、没启这一轮 → 撤掉本地先放的用户消息，项目标灰
      const turn = catState.turns[id]
      if (turn?.token === token) delete catState.turns[id]
      const l = messagesOf(id)
      for (let i = l.length - 1; i >= 0; i--) if (l[i].kind === 'user' && (l[i] as { text: string }).text === t) { l.splice(i, 1); break }
      const p = findConv(id)?.project
      if (p) p.missing = true
      return catState.sel === id ? { text, files: picked } : undefined
    }
    if (err.code === 'TASK_CONFLICT' && err.reason === 'turn_running') {
      // 刷新后旧的回复还在。这条没发出去，文字放回输入框，界面改接正在跑的那一轮。
      const turn = catState.turns[id]
      if (turn?.token === token) delete catState.turns[id]
      const l = messagesOf(id)
      for (let i = l.length - 1; i >= 0; i--) if (l[i].kind === 'user' && (l[i] as { text: string }).text === t) { l.splice(i, 1); break }
      notify((err.message || '').trim() || '这条对话还在回复，请先停止再发。', 'warn')
      void adoptLiveTurns()
      return catState.sel === id ? { text, files: picked } : undefined
    }
    if (err.code === 'CAT_NOT_READY') {
      catState.status = {
        state: 'failed',
        version: catState.status.version,
        canDownload: false,
        error: { code: 'CAT_NOT_READY', message: err.message || CAT_COPY.notReady },
        agents: catState.status.agents ?? [],
      }
      endTurn(id, 'not_ready', token)
      return
    }
    const failed = catState.turns[id]
    const msg = (err.message || '').trim()
    if (failed?.token === token && msg && msg !== CAT_COPY.replyFailed) failed.failMsg = msg
    if (failed?.token === token && err.detail) failed.failDetail = err.detail
    endTurn(id, err.code === 'CANCELED' ? 'cancelled' : 'failed', token)
  }
}

/**
 * 失败消息的「重试」（契约 v0.44 6.19.18.7）：调 RetryCatTurn 原地重试最后一轮失败的 turn，
 * 不新增用户气泡；失败的助手消息 / 行内错误由新一轮的流式输出替换。
 * 没有失败 turnId（比如同步就报错、后端没登记这一轮）时才退回旧做法：把最后一句用户消息再发一次。
 */
export async function retryLastMessage(): Promise<void> {
  const id = catState.sel
  if (id === NEW_CONV || catState.turns[id]) return
  const list = messagesOf(id)
  let lastUser = -1
  for (let i = list.length - 1; i >= 0; i--) if (list[i].kind === 'user') { lastUser = i; break }
  if (lastUser < 0) return
  const failedTurnId = lastFailedTurn.get(id)
  // 6.19.18.7 第 6-7 条：没有 retryable 的失败轮就不该有按钮；不再退回「再发一条」
  if (!failedTurnId || catState.retry[id] !== 'can') return
  // 原地：去掉用户气泡之后的失败输出，用同一个气泡接新一轮；失败提示一并收起
  dismissFailToast(id)
  const removed = list.splice(lastUser + 1)
  lastFailedTurn.delete(id)
  delete catState.retry[id]
  delete catState.outcome[id]
  const token = ++seq
  catState.turns[id] = { token, turnId: '', status: 'running', assistantId: '' }
  if (import.meta.env.DEV && catSim.has('stream')) {
    if (catSim.has('retrygone')) {
      // 走查：retryable=true 但后端已不让重试（竞态）→ 原地灰字
      setTimeout(() => retryGone(id, token, removed), 300)
      return
    }
    simStream(id, handleMessageEvent, handleTurnEvent, () => catState.turns[id]?.token !== token || catState.turns[id]?.status === 'stopping')
    return
  }
  try {
    const res = await retryCatTurn({ conversationId: id, turnId: failedTurnId })
    const turn = catState.turns[id]
    if (turn?.token === token && res.turnId && !turn.turnId) turn.turnId = res.turnId
    else if (res.turnId) sealTurn(res.turnId)
  } catch (e) {
    const err = toAppError(e)
    if (isRetryNotAllowed(err)) return retryGone(id, token, removed)
    const t = catState.turns[id]
    if (t?.token === token) {
      t.failMsg = (err.message || '').trim() || CAT_COPY.replyFailed
      if (err.detail) t.failDetail = err.detail
    }
    endTurn(id, 'failed', token)
    // 同步失败没有新 turnId：保留原失败 turn，便于再点一次重试
    lastFailedTurn.set(id, failedTurnId)
    catState.retry[id] = 'can'
  }
}

/** retry_not_allowed：放回原来的失败输出，按钮原地换成灰字（不弹 toast、不用红色） */
function retryGone(id: string, token: number, removed: CatBlock[]) {
  if (catState.turns[id]?.token === token) delete catState.turns[id]
  const list = catState.messages[id] ?? (catState.messages[id] = [])
  list.push(...removed)
  catState.outcome[id] = 'bad'
  lastFailedTurn.delete(id)
  catState.retry[id] = 'gone'
}

/** 停止生成：按钮立即置灰；已流出的文字保留，结束后单独一行「已停止生成。」 */
// ---------- 删除对话（契约 v0.31.2 / 设计 v0.3 §11–15） ----------

/** 已删掉的对话 id：晚到的列表 / 事件不让它复活 */
const removedConvs = new Set<string>()

/** 清掉一条对话在前端的全部状态（进行中的一轮、流式缓存、停止计时、消息） */
function dropConvState(id: string) {
  const turn = catState.turns[id]
  if (turn?.assistantId) streams.delete(turn.assistantId)
  clearTimeout(stopTimers.get(id))
  stopTimers.delete(id)
  delete catState.turns[id]
  delete catState.messages[id]
  delete catState.unseen[id]
  delete catState.outcome[id]
  delete catState.retry[id]
  loading.delete(id)
  savePrefMap(dropPref(loadPrefMap(), id))
}

/**
 * 删除对话（确认框确认后调用）。直接调 DeleteCatConversation：回复进行中也一样，后端自己先停（最多等 5 秒），
 * 前端不先调 CancelCatTurn。成功：移除该行；是当前对话则回到「新对话」欢迎态。失败：行不动，浮提示「删除失败，请重试。」。
 * 返回 'current' | 'other'（成功，删的是不是当前对话）或 false（失败）。
 */
export async function deleteConversation(id: string): Promise<'current' | 'other' | false> {
  if (!findConv(id)) return 'other'
  announce(CAT_CONV_COPY.deleting)
  try {
    if (import.meta.env.DEV && catSim.has('slowdel')) await new Promise((r) => setTimeout(r, 1500))
    if (import.meta.env.DEV && catSim.has('delfail')) throw new AppError('INTERNAL', 'sim')
    await deleteCatConversation(id)
  } catch {
    notify(CAT_CONV_COPY.deleteFailed, 'warn')
    return false
  }
  const f = findConv(id)
  if (!f) return 'other'
  removedConvs.add(id)
  if (f.project) f.project.convs = f.project.convs.filter((c) => c.id !== id)
  else catState.plain = catState.plain.filter((c) => c.id !== id)
  dropConvState(id)
  const wasCurrent = catState.sel === id
  if (wasCurrent) {
    catState.sel = NEW_CONV
    catState.newProjectId = ''
  }
  announce(CAT_CONV_COPY.deleted)
  return wasCurrent ? 'current' : 'other'
}

export async function stopTurn(convId: string = catState.sel) {
  const turn = catState.turns[convId]
  if (!turn || turn.status === 'stopping') return
  turn.status = 'stopping'
  if (turn.turnId) sealTurn(turn.turnId)
  quietStreaming(convId)
  const token = turn.token
  clearTimeout(stopTimers.get(convId))
  // 后端没在合理时间内回 cat:turn / 返回，也按已停止收尾
  stopTimers.set(convId, setTimeout(() => endTurn(convId, 'cancelled', token), 5000))
  try {
    await cancelCatTurn({ convId, turnId: turn.turnId || undefined })
  } catch {
    /* 停止失败也按已停止收尾（不再追加文字） */
  }
  if (!isCatLive()) endTurn(convId, 'cancelled', token)
}

export function tipLater() {
  return CAT_COPY.later
}

export function asAppError(e: unknown): AppError {
  return e instanceof AppError ? e : new AppError('CAT_REPLY_FAILED', CAT_COPY.replyFailed)
}

// ---------- 项目（契约 v0.31 §6.19.10） ----------

/** 浏览器走查：把侧栏的示意项目放进内存模拟，规则（去重、missing）由 api/catProjects.ts 统一处理 */
function seedProjectSim() {
  const missing = import.meta.env.DEV && catSim.has('missing')
  resetCatProjectSim(
    catState.projects.map((p) => ({ id: p.id, name: p.name, path: p.path, createdAt: p.createdAt, updatedAt: p.createdAt, missing: missing && p.id === 'p3' })),
  )
  if (catSimPicks.length) queueSimPick(...catSimPicks)
}

/** 把后端列表合进侧栏：保留展开状态和已加载的对话；顺序以后端为准（6.19.10.6） */
function mergeProjects(list: ApiCatProject[], replaceOrder = true) {
  const old = new Map(catState.projects.map((p) => [p.id, p]))
  const next: CatProject[] = list.map((a) => {
    const p = old.get(a.id)
    if (p) {
      p.name = a.name
      p.path = a.path
      p.missing = a.missing
      p.createdAt = a.createdAt
      return p
    }
    return { id: a.id, name: a.name, path: a.path, missing: a.missing, createdAt: a.createdAt, open: false, branch: '', convs: [] }
  })
  if (replaceOrder) {
    // 已不存在的项目（别处删了）：它的对话也不再显示；正在看的话回到欢迎页
    const gone = catState.projects.filter((p) => !list.some((a) => a.id === p.id))
    for (const p of gone) dropProjectConvs(p)
    catState.projects = next
  }
}

function dropProjectConvs(p: CatProject) {
  for (const c of p.convs) {
    delete catState.messages[c.id]
    if (catState.sel === c.id) catState.sel = NEW_CONV
  }
  if (catState.newProjectId === p.id) {
    catState.newProjectId = ''
  }
}

/** 重新读项目列表（进入页面、窗口获得焦点）：只更新项目本身（名字 / missing / 增删），不重读对话 */
export async function refreshProjects() {
  if (!catProjectsOn()) return
  try {
    mergeProjects(await listCatProjects())
  } catch {
    /* 保持现有列表 */
  }
}

let focusAt = 0
function onWindowFocus() {
  // 焦点抖动时不重复打（0.5s 内只算一次）
  const now = Date.now()
  if (now - focusAt < 500) return
  focusAt = now
  void refreshProjects()
  catState.filesRev++
}

export function handleProjectEvent(e: CatProjectEvent) {
  const p = catState.projects.find((x) => x.id === e.id)
  if (p) p.missing = e.missing
}

let noticeSeq = 0
let noticeTimer: ReturnType<typeof setTimeout> | undefined
/** 中间顶部浮提示，2.4s 后自动消失（设计 §07） */
export function notify(text: string, tone: CatNoticeTone = 'info') {
  clearTimeout(noticeTimer)
  catState.notice = { text, key: ++noticeSeq, tone }
  noticeTimer = setTimeout(() => (catState.notice = null), 2400)
}

function announce(text: string) {
  catState.announce = ''
  setTimeout(() => (catState.announce = text), 30)
}

let hlTimer: ReturnType<typeof setTimeout> | undefined
/** 展开、滚到可见并高亮项目行（侧栏监听 highlight 负责滚动和焦点） */
export function focusProject(id: string) {
  const p = catState.projects.find((x) => x.id === id)
  if (!p) return
  p.open = true
  clearTimeout(hlTimer)
  catState.highlight = ''
  setTimeout(() => {
    catState.highlight = id
    hlTimer = setTimeout(() => (catState.highlight = ''), 2400)
  }, 0)
}

/** v0.40：打开添加项目双入口对话框（本机文件夹 | 从 Git 克隆） */
export function createProject() {
  if (!catProjectsOn()) {
    notify(CAT_PROJECT_COPY.later)
    return
  }
  catState.addingProject = true
}

export function closeAddProject() {
  catState.addingProject = false
}

/** 成功添加 / 克隆后：插顶部、展开、进入可聊（与本机添加一致） */
export function applyAddedProject(a: { id: string; name: string; path: string; missing: boolean; createdAt: number }, existed: boolean) {
  if (existed) {
    if (!catState.projects.some((p) => p.id === a.id)) {
      catState.projects.unshift({ id: a.id, name: a.name, path: a.path, missing: a.missing, createdAt: a.createdAt, open: true, branch: '', convs: [] })
    }
    notify(CAT_PROJECT_COPY.existed)
    focusProject(a.id)
    return
  }
  catState.projects = catState.projects.filter((p) => p.id !== a.id)
  catState.projects.unshift({ id: a.id, name: a.name, path: a.path, missing: a.missing, createdAt: a.createdAt, open: true, branch: '', convs: [] })
  catState.sel = NEW_CONV
  catState.newProjectId = a.id
  focusProject(a.id)
  announce(CAT_PROJECT_COPY.added(a.name))
  notify(CAT_PROJECT_COPY.added(a.name))
}

/** 本机文件夹入口：选目录 → CreateCatProject */
export async function createProjectFromFolder() {
  if (!catProjectsOn()) {
    notify(CAT_PROJECT_COPY.later)
    return
  }
  let path = ''
  try {
    path = await pickProjectFolder()
  } catch (e) {
    notify(projectErrorText(e), 'warn')
    return
  }
  if (!path) return
  try {
    const r = await createCatProject({ path })
    if (r.existed) {
      if (!catState.projects.some((p) => p.id === r.project.id)) await refreshProjects()
      applyAddedProject(r.project, true)
      return
    }
    applyAddedProject(r.project, false)
  } catch (e) {
    notify(projectErrorText(e), 'warn')
  }
}

/** 项目行「+」：在这个项目里新建对话（文件夹不见了时不可用） */
/** 侧栏「新对话」：按设置「新对话放在」预选项目（本机偏好） */
export function startNewConv() {
  selectConv(NEW_CONV)
  catState.newProjectId = resolveNewConvProject(catState.projects)
}

export function newConvInProject(id: string) {
  const p = catState.projects.find((x) => x.id === id)
  if (!p || p.missing) return
  catPanels.view = 'chat'
  catState.sel = NEW_CONV
  catState.newProjectId = id
}

/** 选中某条对话 / 「新对话」。不停止其它对话里还在跑的回复 */
export function selectConv(id: string) {
  catPanels.view = 'chat'
  catState.sel = id
  if (id === NEW_CONV) catState.newProjectId = ''
  delete catState.unseen[id]
  // 点开对话就向后端要这段记录。服务商和对话 id 在侧栏，正文不在本地库。
  if (id !== NEW_CONV && (live || (import.meta.env.DEV && (catSim.has('histempty') || catSim.has('histmissing') || catSim.has('histreadfail'))))) void loadMessages(id)
}

/**
 * 改名：去首尾空白；清空 = 恢复为文件夹名（设计 §03）；1~60 个字（契约），不合法提示「名字需要 1~60 个字。」并保持编辑。
 * 返回 true = 结束编辑。
 */
export async function renameProject(id: string, raw: string): Promise<boolean> {
  const p = catState.projects.find((x) => x.id === id)
  if (!p) return true
  let name = raw.trim()
  if (!name) name = [...folderName(p.path)].slice(0, CAT_PROJECT_NAME_MAX).join('') || p.name
  if (name === p.name) return true
  // 超长在输入框里实时提示（设计 v0.2 §10c），这里只是兜底：不保存、保持编辑
  if (!validProjectName(name)) return false
  try {
    const r = await renameCatProject({ id, name })
    p.name = r.name || name
    p.missing = r.missing
    return true
  } catch (e) {
    notify(projectErrorText(e), 'warn')
    return toAppError(e).code !== 'INVALID_ARGUMENT'
  }
}

/** 删除项目（确认框确认后调用）：只删应用里的记录和它下面的对话，不动文件夹 */
export async function deleteProject(id: string): Promise<boolean> {
  const p = catState.projects.find((x) => x.id === id)
  if (!p) return true
  // 进行中的一轮由后端先取消（6.19.10.2 第 4 条）；前端收尾本地状态
  try {
    await deleteCatProject({ id })
  } catch (e) {
    const err = toAppError(e)
    notify(err.code === 'UNSUPPORTED' ? CAT_PROJECT_COPY.later : CAT_PROJECT_COPY.deleteFailed)
    return false
  }
  for (const c of p.convs) {
    delete catState.turns[c.id]
  }
  dropProjectConvs(p)
  catState.projects = catState.projects.filter((x) => x.id !== id)
  announce(CAT_PROJECT_COPY.deleted(p.name))
  return true
}

/** 在资源管理器 / 访达 / 文件管理器中显示 */
export async function revealProject(id: string) {
  try {
    await revealCatProject({ id })
  } catch (e) {
    const err = toAppError(e)
    if (err.code === 'CAT_PROJECT_MISSING') handleProjectEvent({ id, missing: true })
    notify(projectErrorText(e), 'warn')
  }
}

/**
 * 重新选择文件夹（契约 v0.31.1 第 8 条 / 设计 v0.2 §09）：路径更新、名字不变、对话保留；成功浮提示「已重新选择文件夹。」并高亮该行。
 * 新路径属于别的项目（project_duplicate）→ 提示「这个文件夹已经建过项目了。」并定位那个项目；本项目保持原状态（仍灰就仍灰）。
 */
export async function relocateProject(id: string) {
  let path = ''
  try {
    path = await pickProjectFolder(true)
  } catch (e) {
    notify(projectErrorText(e), 'warn')
    return
  }
  if (!path) return
  try {
    const r = await relocateCatProject({ id, path })
    const p = catState.projects.find((x) => x.id === id)
    if (p) {
      p.path = r.path || path
      p.missing = r.missing
    }
    notify(CAT_PROJECT_COPY.relocated, 'ok')
    focusProject(id)
  } catch (e) {
    const err = toAppError(e)
    if (err.code === 'INVALID_ARGUMENT' && err.reason === 'project_duplicate') {
      notify(CAT_PROJECT_COPY.existed)
      const other = duplicateProjectId(err)
      if (!other) {
        void refreshProjects()
        return
      }
      if (!catState.projects.some((x) => x.id === other)) await refreshProjects()
      focusProject(other)
      return
    }
    notify(projectErrorText(err), 'warn')
  }
}

/** 「清空全部对话」成功后：真后端重新读列表；模拟里直接清空内存里的对话（项目保留） */
export async function afterClearAll() {
  catState.turns = {}
  catState.messages = {}
  catState.unseen = {}
  catState.outcome = {}
  catState.sel = NEW_CONV
  catState.newProjectId = ''
  if (live) {
    await reloadConversations()
    return
  }
  catState.plain = []
  for (const p of catState.projects) p.convs = []
}


/** 欢迎页换服务商：清草稿偏好，按该家 defaultsByAgent 预填并拉能力列表 */
export function changeWelcomeMode(modeId: string) {
  const m = CAT_MODES.find((x) => x.id === modeId)
  if (!m?.enabled) return
  catState.mode = m.id
  savePrefMap(dropPref(loadPrefMap(), NEW_CONV))
  void refreshCapabilities(m.agentKind)
  showConvPref(NEW_CONV)
}

/** 设置里的新对话默认值变了：欢迎页没选过时跟着换 */
export function reapplyNewConvDefaults() {
  if (catState.sel === NEW_CONV && !prefOf(loadPrefMap(), NEW_CONV)) showConvPref(NEW_CONV)
}
