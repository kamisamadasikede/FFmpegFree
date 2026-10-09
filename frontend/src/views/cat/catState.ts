/**
 * Cat 页本地状态（单例）。契约 v0.30 / 设计 v0.4：
 * - CAT_BACKEND_READY=true 且在 Wails 里：会话列表 / 消息 / 发送 / 停止走真实 CatService；cat:message 流式拼进助手气泡
 * - 纯浏览器（走查）：会话列表用 catMock，发送返回未就绪
 * - 新建对话创建时写入 agentKind=cat_build；已有对话永不改 agentKind（发送只带会话 id）
 * - 访问默认「请求批准」；「完全访问」不可选
 * - 项目（契约 v0.31 §6.19.10）：对话归属在创建时决定（项目行「+」带 projectId），之后不可改，没有「移到项目」；
 *   文件夹不见了 → 项目标灰、对话可看不可发；窗口获得焦点时重新 ListCatProjects；cat:project 更新 missing。
 *   接口见 api/catProjects.ts（CAT_PROJECTS_BACKEND_READY=true 走真绑定；浏览器走内存模拟）
 */
import { computed, reactive } from 'vue'
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
  CAT_COPY,
  cancelCatTurn,
  createCatConversation,
  getCatConversation,
  getCatStatus,
  isCatLive,
  listCatConversations,
  listCatModels,
  listCatThinkLevels,
  onCatMessage,
  onCatStatus,
  onCatTurn,
  sendCatMessage,
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
  mockProjects,
  titleFrom,
  type CatBlock,
  type CatConv,
  type CatProject,
} from '@/api/catMock'
import { applyStreamEvent, newStreamMsg, type CatStreamEvent, type CatStreamMsg, type CatTurnEvent } from '@/api/catStream'
import { AppError, toAppError } from '@/api/call'
import { catSim, catSimOs, catSimPicks, simStream } from './catDevSim'

export const NEW_CONV = 'new'

/** 浮提示语气：info（信息图标）/ ok（成功绿勾）/ warn（警示色图标与描边，设计 v0.2 §10b） */
export type CatNoticeTone = 'info' | 'ok' | 'warn'

/** 一轮回复的前端状态：running = 生成中（显示停止）；stopping = 已点停止（按钮立即置灰） */
export interface CatTurnState {
  token: number
  turnId: string
  status: 'running' | 'stopping'
  /** 收到第一段助手文字后记下（之前显示「正在思考…」） */
  assistantId: string
}

const live = isCatLive()

export const catState = reactive({
  /** 当前选中的模型 id（来自 ListCatModels；空列表时为空） */
  model: '' as string,
  /** 当前选中的思考强度 id（无列表时为空，胶囊不显示「· 强度」） */
  think: '' as string,
  access: 'ask' as 'ask' | 'full',
  /** 欢迎页模式；一期只能选 build，只影响新对话 */
  mode: 'cat_build',
  workInProject: true,
  sel: live ? NEW_CONV : 'c31',
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
  /** 删除确认框对应的项目 id */
  deleting: '' as string,
  plain: (live ? [] : mockPlainConvs()) as CatConv[],
  messages: {} as Record<string, CatBlock[]>,
  /** 新对话正在创建（欢迎页输入框忙） */
  creating: false,
  /** 进行中的回复，按会话 id */
  turns: {} as Record<string, CatTurnState>,
  /** 组件状态；非 ready 时显示未就绪横条（无下载按钮）；浏览器走查直接按 missing，避免短暂可点发送 */
  status: (live || (import.meta.env.DEV && catSim.has('checking'))
    ? { state: 'checking', version: '', canDownload: false, error: null }
    : (import.meta.env.DEV && catSim.has('stream'))
      ? { state: 'ready', version: '', canDownload: false, error: null }
    : { state: 'missing', version: '', canDownload: false, error: { code: 'CAT_NOT_READY', message: CAT_COPY.notReady } }) as CatStatus,
  /** 当前会话 agentKind 对应的能力列表（不造假） */
  models: [] as CatModel[],
  thinks: [] as CatThinkLevel[],
})

export const modelName = computed(() => {
  const m = catState.models.find((x) => x.id === catState.model)
  return m?.displayName ?? ''
})

export const thinkName = computed(() => {
  if (!catState.thinks.length) return ''
  return catState.thinks.find((t) => t.id === catState.think)?.displayName ?? ''
})

/** 合一胶囊文案：有强度才拼「·」；都没有时显示「Cat 助手」 */
export const capsuleLabel = computed(() => {
  const m = modelName.value
  const t = thinkName.value
  if (m && t) return `${m} · ${t}`
  if (m) return m
  return 'Cat 助手'
})

export const accessShort = computed(() => CAT_ACCESS.find((a) => a.id === catState.access)?.short ?? '请求批准')

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
      catState.messages[id] = f ? mockMessages(id, f.conv.title) : []
    }
  }
  return catState.messages[id]
}

function blocksFrom(list: CatMessage[]): CatBlock[] {
  return list.map((m): CatBlock =>
    m.role === 'user' ? { kind: 'user', text: m.content }
      : m.role === 'system' ? { kind: 'sys', text: m.content }
        : { kind: 'a', id: m.id, text: m.content, streaming: false },
  )
}

async function loadMessages(id: string) {
  if (loading.has(id) || id.startsWith('local-')) return
  loading.add(id)
  try {
    const d = await getCatConversation(id)
    // 加载期间已经开始一轮（本地已放了用户消息 / 流式文字）就不覆盖
    if (d && !catState.turns[id] && !(catState.messages[id]?.length)) catState.messages[id] = blocksFrom(d.messages)
  } catch {
    /* 读不到就保持空，不弹错 */
  } finally {
    loading.delete(id)
  }
}

function toConv(c: CatConversation): CatConv {
  return { id: c.id, title: c.title, agentKind: c.agentKind, projectId: c.projectId }
}

/** 当前选中对话的 agentKind；新对话用欢迎页选中模式（一期强制 cat_build） */
export function currentAgentKind(): CatAgentKind {
  if (catState.sel === NEW_CONV) {
    const m = CAT_MODES.find((x) => x.id === catState.mode)
    return m?.enabled ? m.agentKind : CAT_AGENT_BUILD
  }
  return findConv(catState.sel)?.conv.agentKind ?? CAT_AGENT_BUILD
}

/** 按会话 agentKind 拉模型/强度；无强度则清空 think，不造假 */
export async function refreshCapabilities(agentKind: CatAgentKind = currentAgentKind()) {
  const [models, thinks] = await Promise.all([listCatModels(agentKind), listCatThinkLevels(agentKind)])
  catState.models = models
  catState.thinks = thinks
  if (!models.find((m) => m.id === catState.model)) catState.model = models[0]?.id ?? ''
  if (!thinks.length) catState.think = ''
  else if (!thinks.find((t) => t.id === catState.think)) catState.think = thinks[0]?.id ?? ''
}

// ---------- 进入页面：状态、会话列表、事件 ----------

let offs: Array<() => void> = []

/** CatPage 挂载时调用；返回卸载函数 */
export function initCat(): () => void {
  disposeCat()
  offs = [
    onCatStatus((s) => {
      const was = catState.status.state
      catState.status = s
      if (s.state === 'ready' && was !== 'ready') void refreshCapabilities()
    }),
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
  const simStatus = import.meta.env.DEV && (catSim.has('checking') || catSim.has('stream'))
  if (!simStatus) {
    void getCatStatus()
      .then((s) => (catState.status = s))
      .catch(() => (catState.status = { state: 'missing', version: '', canDownload: false, error: { code: 'CAT_NOT_READY', message: CAT_COPY.notReady } }))
  }
  if (live) void reloadConversations()
  else void refreshProjects()
  return disposeCat
}

export function disposeCat() {
  offs.forEach((f) => f())
  offs = []
}

export async function reloadConversations() {
  try {
    const [list, projects] = await Promise.all([listCatConversations(), listCatProjects().catch(() => null)])
    if (projects) mergeProjects(projects, true)
    const byId = new Map(catState.projects.map((p) => [p.id, p]))
    for (const p of catState.projects) p.convs = []
    const plain: CatConv[] = []
    // 后端已按 6.19.10.6 排好，分组时保持顺序；项目不认识（开关关 / 已删）的放「对话」
    for (const c of list) {
      const p = c.projectId ? byId.get(c.projectId) : undefined
      if (p) p.convs.push(toConv(c))
      else plain.push(toConv(c))
    }
    catState.plain = plain
    // 没有进行中回复的会话下次打开时重新读（离开页面期间可能错过事件）
    for (const id of Object.keys(catState.messages)) if (!catState.turns[id]) delete catState.messages[id]
  } catch {
    /* 保持现有列表 */
  }
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

export function handleMessageEvent(e: CatStreamEvent) {
  if (e.role === 'user') return // 用户消息本地已先放
  const turn = catState.turns[e.convId]
  // 点了停止之后到的文字不再追加（保留已显示的）
  if (turn?.status === 'stopping') return
  if (turn && e.turnId && turn.turnId && e.turnId !== turn.turnId) return
  if (turn && e.turnId && !turn.turnId) turn.turnId = e.turnId
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
  let b = assistantBlock(e.convId, e.messageId)
  if (!b) {
    if (!m.text && m.done) return
    list.push({ kind: 'a', id: e.messageId, text: m.text, streaming: !m.done })
    b = list[list.length - 1] as Extract<CatBlock, { kind: 'a' }>
  } else {
    b.text = m.text
    b.streaming = !m.done
  }
  if (turn && !turn.assistantId) turn.assistantId = e.messageId
  if (m.done) streams.delete(e.messageId)
}

export function handleTurnEvent(e: CatTurnEvent) {
  const turn = catState.turns[e.convId]
  if (!turn) return
  if (e.turnId && turn.turnId && e.turnId !== turn.turnId) return
  if (e.status === 'running') {
    if (e.turnId && !turn.turnId) turn.turnId = e.turnId
    return
  }
  if (e.status === 'completed') endTurn(e.convId, 'completed')
  else if (e.status === 'cancelled') endTurn(e.convId, 'cancelled')
  else endTurn(e.convId, e.errorCode === 'CAT_NOT_READY' ? 'not_ready' : 'failed')
}

type TurnEnd = 'completed' | 'cancelled' | 'failed' | 'not_ready'

const stopTimers = new Map<string, ReturnType<typeof setTimeout>>()

/** 结束一轮（事件和 SendCatMessage 的返回谁先到谁算，只处理一次） */
function endTurn(convId: string, how: TurnEnd, token?: number) {
  const turn = catState.turns[convId]
  if (!turn || (token !== undefined && turn.token !== token)) return
  if (turn.status === 'stopping') how = 'cancelled'
  clearTimeout(stopTimers.get(convId))
  stopTimers.delete(convId)
  delete catState.turns[convId]
  const list = catState.messages[convId] ?? (catState.messages[convId] = [])
  // 收起流式光标，已流出来的文字原样保留（不加后缀）
  for (const b of list) if (b.kind === 'a' && b.streaming) b.streaming = false
  if (turn.assistantId) streams.delete(turn.assistantId)
  if (how === 'cancelled') list.push({ kind: 'sys', text: CAT_COPY.stopped })
  else if (how === 'failed') list.push({ kind: 'sys', text: CAT_COPY.replyFailed, tone: 'err' })
  else if (how === 'not_ready') list.push({ kind: 'sys', text: CAT_COPY.notReady })
}

let seq = 0

/**
 * 发送：新对话创建时写入 agentKind（不可再改）；已有对话只带会话 id。
 * 返回值：后端没收下这条消息（CAT_PROJECT_MISSING：没建对话 / 没存用户消息）且用户还停在原处时返回原文，
 * 调用方放回输入框（CatComposer.restoreDraft）；其它情况返回 undefined。
 */
export async function sendMessage(text: string): Promise<string | undefined> {
  const t = text.trim()
  if (!t || catState.creating) return
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
    // 一期前端只能传 cat_build
    const agentKind: CatAgentKind = CAT_AGENT_BUILD
    const project = catState.projects.find((p) => p.id === catState.newProjectId)
    catState.creating = true
    try {
      const created = await createCatConversation({ agentKind, title: titleFrom(t), projectId: project?.id })
      id = created.id
      const conv = toConv(created)
      if (project) {
        conv.projectId = project.id
        project.convs.unshift(conv)
        project.open = true
      } else catState.plain.unshift(conv)
      catState.newProjectId = ''
    } catch (e) {
      if (project && toAppError(e).code === 'CAT_PROJECT_MISSING') {
        // 不创建对话；欢迎页随之显示「项目文件夹不见了。」，文字放回输入框
        project.missing = true
        return catState.sel === NEW_CONV && catState.newProjectId === project.id ? text : undefined
      }
      id = `local-${Date.now().toString(36)}-${++seq}`
      if (project) {
        project.convs.unshift({ id, title: titleFrom(t), agentKind, projectId: project.id })
        catState.newProjectId = ''
      } else catState.plain.unshift({ id, title: titleFrom(t), agentKind })
      catState.messages[id] = [{ kind: 'user', text: t }, { kind: 'sys', text: CAT_COPY.replyFailed, tone: 'err' }]
      catState.sel = id
      return
    } finally {
      catState.creating = false
    }
    catState.messages[id] = []
    catState.sel = id
  }

  const list = messagesOf(id)
  for (let i = list.length - 1; i >= 0; i--) if (list[i].kind === 'run') list.splice(i, 1)
  list.push({ kind: 'user', text: t })
  const token = ++seq
  catState.turns[id] = { token, turnId: '', status: 'running', assistantId: '' }

  if (import.meta.env.DEV && catSim.has('stream')) {
    // 仅 vite dev + 纯浏览器 + ?cat_sim=stream（见 catDevSim.ts）；正式包里 catSim 恒为空
    simStream(id, handleMessageEvent, handleTurnEvent, () => catState.turns[id]?.token !== token || catState.turns[id]?.status === 'stopping')
    return
  }
  try {
    const res = await sendCatMessage({
      conversationId: id,
      content: t,
      modelId: catState.model || undefined,
      thinkLevelId: catState.think || undefined,
    })
    const turn = catState.turns[id]
    if (turn?.token === token && res.turnId && !turn.turnId) turn.turnId = res.turnId
    // Send 还没返回时就点了停止：那时后端可能还没登记这一轮，拿到 turnId 后再取消一次（幂等）
    if (turn?.token === token && turn.status === 'stopping' && res.turnId) {
      void cancelCatTurn({ convId: id, turnId: res.turnId }).catch(() => undefined)
    }
    // v0.30.1：Send 在本轮开始后立即返回，结束由 cat:turn 决定；
    // 只有兼容旧的同步返回（直接带了助手回复）时才在这里收尾
    if (turn?.token === token && res.assistantMessage?.content) {
      if (turn.status !== 'stopping') {
        const a = res.assistantMessage
        const b = assistantBlock(id, a.id)
        if (!b) messagesOf(id).push({ kind: 'a', id: a.id, text: a.content, streaming: false })
        else if (!b.text) b.text = a.content
      }
      endTurn(id, 'completed', token)
    }
  } catch (e) {
    const err = toAppError(e)
    if (err.code === 'CAT_PROJECT_MISSING') {
      // 6.19.10.2 第 6 条：后端没存这条用户消息、没启这一轮 → 撤掉本地先放的用户消息，项目标灰
      const turn = catState.turns[id]
      if (turn?.token === token) delete catState.turns[id]
      const l = messagesOf(id)
      for (let i = l.length - 1; i >= 0; i--) if (l[i].kind === 'user' && (l[i] as { text: string }).text === t) { l.splice(i, 1); break }
      const p = findConv(id)?.project
      if (p) p.missing = true
      return catState.sel === id ? text : undefined
    }
    endTurn(id, err.code === 'CANCELED' ? 'cancelled' : err.code === 'CAT_NOT_READY' ? 'not_ready' : 'failed', token)
  }
}

/** 停止生成：按钮立即置灰；已流出的文字保留，结束后单独一行「已停止生成。」 */
export async function stopTurn(convId: string = catState.sel) {
  const turn = catState.turns[convId]
  if (!turn || turn.status === 'stopping') return
  turn.status = 'stopping'
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

/** 新建项目：选文件夹 → CreateCatProject；existed → 提示并定位已有项目；新建 → 插顶部、展开、进入该项目的新对话 */
export async function createProject() {
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
  if (!path) return // 用户取消：什么都不做
  try {
    const r = await createCatProject({ path })
    if (r.existed) {
      if (!catState.projects.some((p) => p.id === r.project.id)) await refreshProjects()
      notify(CAT_PROJECT_COPY.existed)
      focusProject(r.project.id)
      return
    }
    const a = r.project
    catState.projects = catState.projects.filter((p) => p.id !== a.id)
    catState.projects.unshift({ id: a.id, name: a.name, path: a.path, missing: a.missing, createdAt: a.createdAt, open: true, branch: '', convs: [] })
    catState.sel = NEW_CONV
    catState.newProjectId = a.id
    focusProject(a.id)
    announce(CAT_PROJECT_COPY.added(a.name))
  } catch (e) {
    notify(projectErrorText(e), 'warn')
  }
}

/** 项目行「+」：在这个项目里新建对话（文件夹不见了时不可用） */
export function newConvInProject(id: string) {
  const p = catState.projects.find((x) => x.id === id)
  if (!p || p.missing) return
  catState.sel = NEW_CONV
  catState.newProjectId = id
}

/** 选中某条对话 / 「新对话」（不属于任何项目） */
export function selectConv(id: string) {
  catState.sel = id
  if (id === NEW_CONV) catState.newProjectId = ''
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
