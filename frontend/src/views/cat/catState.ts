/**
 * Cat 页本地状态（单例）。契约 v0.30 / 设计 v0.4：
 * - CAT_BACKEND_READY=false：会话列表用 catMock；模型/强度走 api/cat（未就绪为空，不造假）
 * - 新建对话锁定 agentKind=cat_build；已有对话不改 agentKind
 * - 访问默认「请求批准」；「完全访问」不可选
 */
import { computed, reactive } from 'vue'
import {
  CAT_AGENT_BUILD,
  CAT_COPY,
  createCatConversation,
  listCatModels,
  listCatThinkLevels,
  type CatAgentKind,
  type CatModel,
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
import { AppError } from '@/api/call'

export const NEW_CONV = 'new'

export const catState = reactive({
  /** 当前选中的模型 id（来自 ListCatModels；空列表时为空） */
  model: '' as string,
  /** 当前选中的思考强度 id（无列表时为空，胶囊不显示「· 强度」） */
  think: '' as string,
  access: 'ask' as 'ask' | 'full',
  /** 欢迎页模式；一期只能选 build，只影响新对话 */
  mode: 'cat_build',
  workInProject: true,
  sel: 'c31',
  projects: mockProjects() as CatProject[],
  plain: mockPlainConvs() as CatConv[],
  messages: {} as Record<string, CatBlock[]>,
  pending: '' as string,
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

export function findConv(id: string): { project?: CatProject; conv: CatConv } | null {
  for (const p of catState.projects) {
    const c = p.convs.find((x) => x.id === id)
    if (c) return { project: p, conv: c }
  }
  const d = catState.plain.find((x) => x.id === id)
  return d ? { conv: d } : null
}

export function messagesOf(id: string): CatBlock[] {
  if (!catState.messages[id]) {
    const f = findConv(id)
    catState.messages[id] = f ? mockMessages(id, f.conv.title) : []
  }
  return catState.messages[id]
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

let seq = 0
let replyTimer: ReturnType<typeof setTimeout> | undefined

/**
 * 发送：新对话创建时写入 agentKind（不可再改）；
 * CAT_BACKEND_READY=false 时 Send 会 CAT_NOT_READY，气泡展示未就绪文案（不假装成功流式）。
 */
export async function sendMessage(text: string) {
  const t = text.trim()
  if (!t || catState.pending) return
  let id = catState.sel
  let agentKind = currentAgentKind()

  if (id === NEW_CONV) {
    const mode = CAT_MODES.find((m) => m.id === catState.mode)
    if (!mode?.enabled) {
      agentKind = CAT_AGENT_BUILD
      catState.mode = 'cat_build'
    } else {
      agentKind = mode.agentKind
    }
    // 一期前端只能传 cat_build
    if (agentKind !== CAT_AGENT_BUILD) agentKind = CAT_AGENT_BUILD

    try {
      const created = await createCatConversation({ agentKind, title: titleFrom(t) })
      id = created.id
      catState.plain.unshift({
        id: created.id,
        title: created.title,
        agentKind: created.agentKind,
      })
      catState.messages[id] = []
      catState.sel = id
    } catch {
      id = `local-${Date.now().toString(36)}-${++seq}`
      catState.plain.unshift({ id, title: titleFrom(t), agentKind: CAT_AGENT_BUILD })
      catState.messages[id] = []
      catState.sel = id
    }
  } else {
    // 已有对话：永不改 agentKind
    agentKind = findConv(id)?.conv.agentKind ?? CAT_AGENT_BUILD
  }

  const list = messagesOf(id)
  for (let i = list.length - 1; i >= 0; i--) if (list[i].kind === 'run') list.splice(i, 1)
  list.push({ kind: 'user', text: t })
  catState.pending = id

  clearTimeout(replyTimer)
  replyTimer = setTimeout(() => {
    // mock / 未就绪：不假装成功；展示定稿未就绪文案
    messagesOf(id).push({ kind: 'p', text: CAT_COPY.notReady })
    catState.pending = ''
  }, 400)

  void agentKind // 真绑定 Send 时按会话 agentKind 路由（本 PR CAT_BACKEND_READY=false）
}

export function tipLater() {
  return CAT_COPY.later
}

export function asAppError(e: unknown): AppError {
  return e instanceof AppError ? e : new AppError('CAT_REPLY_FAILED', CAT_COPY.replyFailed)
}
