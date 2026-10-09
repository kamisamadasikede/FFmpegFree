/**
 * Cat 页本地状态（单例）。契约 v0.30 / 设计 v0.4：
 * - CAT_BACKEND_READY=true 且在 Wails 里：会话列表 / 消息 / 发送 / 停止走真实 CatService；cat:message 流式拼进助手气泡
 * - 纯浏览器（走查）：会话列表用 catMock，发送返回未就绪
 * - 新建对话创建时写入 agentKind=cat_build；已有对话永不改 agentKind（发送只带会话 id）
 * - 访问默认「请求批准」；「完全访问」不可选
 */
import { computed, reactive } from 'vue'
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
import { catSim, simStream } from './catDevSim'

export const NEW_CONV = 'new'

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
  return { id: c.id, title: c.title, agentKind: c.agentKind }
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
  ]
  // 开发走查（?cat_sim=checking|stream）：保持模拟的组件状态，不被浏览器 mock 的 missing 覆盖
  const simStatus = import.meta.env.DEV && (catSim.has('checking') || catSim.has('stream'))
  if (!simStatus) {
    void getCatStatus()
      .then((s) => (catState.status = s))
      .catch(() => (catState.status = { state: 'missing', version: '', canDownload: false, error: { code: 'CAT_NOT_READY', message: CAT_COPY.notReady } }))
  }
  if (live) void reloadConversations()
  return disposeCat
}

export function disposeCat() {
  offs.forEach((f) => f())
  offs = []
}

export async function reloadConversations() {
  try {
    const list = await listCatConversations()
    catState.plain = list.map(toConv)
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
 */
export async function sendMessage(text: string) {
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

  if (id === NEW_CONV) {
    const mode = CAT_MODES.find((m) => m.id === catState.mode)
    if (!mode?.enabled) catState.mode = 'cat_build'
    // 一期前端只能传 cat_build
    const agentKind: CatAgentKind = CAT_AGENT_BUILD
    catState.creating = true
    try {
      const created = await createCatConversation({ agentKind, title: titleFrom(t) })
      id = created.id
      catState.plain.unshift(toConv(created))
    } catch {
      id = `local-${Date.now().toString(36)}-${++seq}`
      catState.plain.unshift({ id, title: titleFrom(t), agentKind })
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
