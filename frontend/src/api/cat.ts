/**
 * Cat 助手接口层（契约 v0.30 §6.19，一期仅 cat_build）。
 *
 * CAT_BACKEND_READY=true 且在 Wails 里：调真实 CatService 绑定（wailsjs/go/app/CatService，后端 #172）。
 * 纯浏览器（没有 window.go，走查 / 截图）：仍走 typed mock（模型 / 思考强度为空，发送返回未就绪）。
 * 事件：cat:status / cat:message / cat:turn，归一化见 api/catStream.ts（兼容定稿流式形状和骨架整段形状）。
 *
 * 用户可见文案只出现「Cat 助手」；未就绪 / 回复失败 / 已停止文案见 CAT_COPY。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { AppError, call } from '@/api/call'
import { CAT_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend, onTaskEvent } from '@/services/wails'
import { normalizeMessageEvent, normalizeTurnEvent, type CatStreamEvent, type CatTurnEvent } from '@/api/catStream'

export { CAT_BACKEND_READY }

/** 契约 6.19.7 冻结文案（stopped：产品定，停止生成后单独一行浅灰系统提示） */
export const CAT_COPY = {
  notReady: 'Cat 助手还没准备好，发布后即可使用。',
  replyFailed: '回复没生成出来，请重试。',
  writeRefused: '当前只能查看项目文件，改文件和运行命令下一期开放。',
  later: '下一期开放。',
  stopped: '已停止生成。',
} as const

/** 事件名（契约 6.19.5） */
export const CAT_EVENTS = { status: 'cat:status', message: 'cat:message', turn: 'cat:turn' } as const

/** 一期唯一可创建 / 可发送的 agentKind（契约 6.19.2） */
export const CAT_AGENT_BUILD = 'cat_build' as const
export type CatAgentKind = typeof CAT_AGENT_BUILD | 'cat_cli' | 'cat_code' | 'cat_agent'

export type CatComponentState = 'checking' | 'ready' | 'missing' | 'failed'

export interface CatStatus {
  state: CatComponentState
  version: string
  canDownload: boolean
  error?: { code: string; message: string; detail?: string } | null
}

export interface CatModel {
  id: string
  displayName: string
}

export interface CatThinkLevel {
  id: string
  displayName: string
}

export interface CatConversation {
  id: string
  title: string
  /** 创建后不可变；一期仅 cat_build */
  agentKind: CatAgentKind
  projectPath?: string
  createdAt: number
  updatedAt: number
}

export interface CatMessage {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  createdAt: number
}

export interface CatConversationDetail extends CatConversation {
  messages: CatMessage[]
}

export interface CreateCatConversationRequest {
  agentKind: CatAgentKind
  title?: string
  projectPath?: string
}

export interface SendCatMessageRequest {
  conversationId: string
  content: string
  modelId?: string
  thinkLevelId?: string
  projectPath?: string
}

/**
 * 后端 SendCatMessage 返回（契约 v0.30.1：必须带 turnId，前端记下用于 CancelCatTurn）。
 * 流式时文字靠 cat:message / cat:turn 推进；assistantMessage 可能为空。
 */
export interface SendCatMessageResult {
  turnId: string
  userMessage: CatMessage
  assistantMessage?: CatMessage | null
}

export interface CancelCatTurnRequest {
  convId: string
  turnId?: string
}

const live = () => CAT_BACKEND_READY && hasWailsBackend()

/** 是否走真绑定（CAT_BACKEND_READY=true 且在 Wails 里） */
export const isCatLive = (): boolean => live()

function missingStatus(): CatStatus {
  return {
    state: 'missing',
    version: '',
    canDownload: false,
    error: { code: 'CAT_NOT_READY', message: CAT_COPY.notReady },
  }
}

const STATES: CatComponentState[] = ['checking', 'ready', 'missing', 'failed']

export function mapCatStatus(raw: unknown): CatStatus {
  if (!raw || typeof raw !== 'object') return missingStatus()
  const r = raw as { state?: string; version?: string; error?: { code?: string; message?: string; detail?: string } | null }
  const state = STATES.includes(r.state as CatComponentState) ? (r.state as CatComponentState) : 'missing'
  const e = r.error
  return {
    state,
    version: r.version ?? '',
    // 一期组件未发布：无论后端返回什么都不出现下载入口
    canDownload: false,
    error: e && typeof e.code === 'string' ? { code: e.code, message: e.message ?? '', detail: e.detail } : null,
  }
}

const KINDS: CatAgentKind[] = [CAT_AGENT_BUILD, 'cat_cli', 'cat_code', 'cat_agent']
function mapConv(raw: { id?: string; title?: string; agentKind?: string; projectPath?: string; createdAt?: number; updatedAt?: number }): CatConversation {
  return {
    id: String(raw.id ?? ''),
    title: raw.title?.trim() || '新对话',
    // 原样保留后端存的 agentKind（不改写）；未知值只用于展示，发送仍走会话自己的 id
    agentKind: (KINDS.includes(raw.agentKind as CatAgentKind) ? raw.agentKind : CAT_AGENT_BUILD) as CatAgentKind,
    projectPath: raw.projectPath || undefined,
    createdAt: Number(raw.createdAt) || 0,
    updatedAt: Number(raw.updatedAt) || 0,
  }
}

function mapMsg(raw: { id?: string; role?: string; content?: string; createdAt?: number } | null | undefined): CatMessage {
  const role = raw?.role === 'user' || raw?.role === 'system' ? raw.role : 'assistant'
  return { id: String(raw?.id ?? ''), role, content: String(raw?.content ?? ''), createdAt: Number(raw?.createdAt) || 0 }
}

export async function getCatStatus(): Promise<CatStatus> {
  if (!live()) return missingStatus()
  return mapCatStatus(await call(CatBinding.GetCatStatus()))
}

export async function recheckCat(): Promise<CatStatus> {
  if (!live()) return missingStatus()
  return mapCatStatus(await call(CatBinding.RecheckCat()))
}

function mapList<T extends { id: string; displayName: string }>(raw: unknown): T[] {
  return Array.isArray(raw)
    ? raw
        .filter((m) => m && typeof m.id === 'string' && m.id)
        .map((m) => ({ id: String(m.id), displayName: String(m.displayName || m.id) }) as T)
    : []
}

/**
 * 模型列表：一期只接受 cat_build。未就绪 / 浏览器 / 出错都返回空数组（不造展示名）。
 */
export async function listCatModels(agentKind: CatAgentKind): Promise<CatModel[]> {
  if (agentKind !== CAT_AGENT_BUILD || !live()) return []
  try {
    return mapList<CatModel>(await call(CatBinding.ListCatModels(agentKind)))
  } catch {
    return []
  }
}

/** 思考强度：没有就不显示强度半截（契约 / 产品） */
export async function listCatThinkLevels(agentKind: CatAgentKind): Promise<CatThinkLevel[]> {
  if (agentKind !== CAT_AGENT_BUILD || !live()) return []
  try {
    return mapList<CatThinkLevel>(await call(CatBinding.ListCatThinkLevels(agentKind)))
  } catch {
    return []
  }
}

export async function listCatConversations(): Promise<CatConversation[]> {
  if (!live()) return []
  const raw = await call(CatBinding.ListCatConversations())
  return Array.isArray(raw) ? raw.map(mapConv) : []
}

export async function getCatConversation(id: string): Promise<CatConversationDetail | null> {
  if (!live()) return null
  const raw = await call(CatBinding.GetCatConversation(id))
  if (!raw) return null
  return { ...mapConv(raw), messages: Array.isArray(raw.messages) ? raw.messages.map(mapMsg) : [] }
}

/** 创建会话：agentKind 在这里写入，之后任何接口都不再改它（契约 6.19.2） */
export async function createCatConversation(req: CreateCatConversationRequest): Promise<CatConversation> {
  if (req.agentKind !== CAT_AGENT_BUILD) {
    throw new AppError('INVALID_ARGUMENT', '不支持的模式')
  }
  if (!live()) {
    const now = Date.now()
    return {
      id: `local-${now.toString(36)}`,
      title: req.title?.trim() || '新对话',
      agentKind: CAT_AGENT_BUILD,
      createdAt: now,
      updatedAt: now,
    }
  }
  const raw = await call(
    CatBinding.CreateCatConversation(
      cat.CreateConversationRequest.createFrom({
        agentKind: req.agentKind,
        title: req.title?.trim() ?? '',
        projectPath: req.projectPath ?? '',
        accessMode: 'ask',
      }),
    ),
  )
  return mapConv(raw)
}

export async function deleteCatConversation(id: string): Promise<void> {
  if (!live()) return
  await call(CatBinding.DeleteCatConversation(id))
}

/**
 * 发送：只带会话 id，后端按会话已存的 agentKind 路由（前端不传 agentKind，也就改不了）。
 * 浏览器 mock：组件未就绪 → CAT_NOT_READY。
 */
export async function sendCatMessage(req: SendCatMessageRequest): Promise<SendCatMessageResult> {
  if (!live()) {
    throw new AppError('CAT_NOT_READY', CAT_COPY.notReady)
  }
  const raw = await call(
    CatBinding.SendCatMessage(
      cat.SendMessageRequest.createFrom({
        conversationId: req.conversationId,
        content: req.content,
        modelId: req.modelId ?? '',
        thinkLevelId: req.thinkLevelId ?? '',
        projectPath: req.projectPath ?? '',
      }),
    ),
  )
  return {
    // 绑定模型还没生成 turnId 字段时按空串处理（turnId 也会从 cat:turn running / cat:message 里拿到）
    turnId: String((raw as { turnId?: unknown } | null)?.turnId ?? ''),
    userMessage: mapMsg(raw?.userMessage),
    assistantMessage: raw?.assistantMessage ? mapMsg(raw.assistantMessage) : null,
  }
}

/**
 * 停止生成（契约 v0.30.1 §6.19.9：CancelCatTurn({ convId, turnId })，立即停止、幂等）。
 * 过渡：v2 上生成的绑定仍是 #172 的 CancelCatTurn(conversationId string)（同一会话同时只有一轮，按会话取消等价）。
 * 后端改成 CancelCatTurnRequest 并重新生成绑定后，下面这行会类型报错，届时改成整个 req 下传。
 */
export async function cancelCatTurn(req: CancelCatTurnRequest): Promise<void> {
  if (!req.convId || !live()) return
  await call(CatBinding.CancelCatTurn(req.convId))
}

// ---------- 事件 ----------

export function onCatStatus(cb: (s: CatStatus) => void): () => void {
  return onTaskEvent<unknown>(CAT_EVENTS.status, (raw) => cb(mapCatStatus(raw)))
}

export function onCatMessage(cb: (e: CatStreamEvent) => void): () => void {
  return onTaskEvent<unknown>(CAT_EVENTS.message, (raw) => {
    const e = normalizeMessageEvent(raw)
    if (e) cb(e)
  })
}

export function onCatTurn(cb: (e: CatTurnEvent) => void): () => void {
  return onTaskEvent<unknown>(CAT_EVENTS.turn, (raw) => {
    const e = normalizeTurnEvent(raw)
    if (e) cb(e)
  })
}
