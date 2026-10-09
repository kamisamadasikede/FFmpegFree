/**
 * Cat 助手接口层（契约 v0.30 §6.19，一期仅 cat_build）。
 *
 * CAT_BACKEND_READY=false（本 PR 默认）：不调 Wails，走 typed mock；模型 / 思考强度列表为空（组件未就绪，不造假）。
 * 后端骨架合入并重新生成 wailsjs/go/app/CatService 后：把 CAT_BACKEND_READY 改为 true，并取消下方动态绑定注释。
 *
 * 用户可见文案只出现「Cat 助手」；未就绪 / 回复失败文案见 CAT_COPY。
 */
import { AppError, call } from '@/api/call'
import { CAT_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'

export { CAT_BACKEND_READY }

/** 契约 6.19.7 冻结文案 */
export const CAT_COPY = {
  notReady: 'Cat 助手还没准备好，发布后即可使用。',
  replyFailed: '回复没生成出来，请重试。',
  writeRefused: '当前只能查看项目文件，改文件和运行命令下一期开放。',
  later: '下一期开放。',
} as const

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
}

export interface SendCatMessageRequest {
  conversationId: string
  content: string
  modelId?: string
  thinkLevelId?: string
  projectPath?: string
}

export interface SendCatMessageResult {
  conversationId: string
  userMessageId: string
  /** 流式时可能先空，靠 cat:message / cat:turn 推进 */
  assistantMessageId?: string
}

const live = () => CAT_BACKEND_READY && hasWailsBackend()

/** 是否走真绑定（本 PR 恒为 false，直到 BE 骨架合入后翻 flags） */
export const isCatLive = (): boolean => live()

function missingStatus(): CatStatus {
  return {
    state: 'missing',
    version: '',
    canDownload: false,
    error: { code: 'CAT_NOT_READY', message: CAT_COPY.notReady },
  }
}

/**
 * 可选动态绑定：wailsjs 里还没有 CatService 时不能静态 import（会让 vue-tsc / vite 失败）。
 * BE 合入并生成绑定后，这里解析到 window.go.app.CatService。
 */
function catBinding(): Record<string, (...args: unknown[]) => Promise<unknown>> | null {
  if (!live()) return null
  try {
    const go = (window as unknown as { go?: { app?: { CatService?: Record<string, (...args: unknown[]) => Promise<unknown>> } } }).go
    return go?.app?.CatService ?? null
  } catch {
    return null
  }
}

export async function getCatStatus(): Promise<CatStatus> {
  const b = catBinding()
  if (!b?.GetCatStatus) return missingStatus()
  const raw = (await call(b.GetCatStatus())) as CatStatus
  return {
    state: (raw?.state as CatComponentState) || 'missing',
    version: raw?.version ?? '',
    canDownload: raw?.canDownload === true,
    error: raw?.error ?? null,
  }
}

export async function recheckCat(): Promise<CatStatus> {
  const b = catBinding()
  if (!b?.RecheckCat) return missingStatus()
  const raw = (await call(b.RecheckCat())) as CatStatus
  return {
    state: (raw?.state as CatComponentState) || 'missing',
    version: raw?.version ?? '',
    canDownload: raw?.canDownload === true,
    error: raw?.error ?? null,
  }
}

/**
 * 模型列表：一期只接受 cat_build。未就绪 / mock 返回空数组（不造展示名）。
 */
export async function listCatModels(agentKind: CatAgentKind): Promise<CatModel[]> {
  if (agentKind !== CAT_AGENT_BUILD) return []
  const b = catBinding()
  if (!b?.ListCatModels) return []
  const raw = (await call(b.ListCatModels(agentKind))) as CatModel[] | null
  return Array.isArray(raw)
    ? raw.map((m) => ({ id: String(m.id), displayName: String(m.displayName || m.id) }))
    : []
}

/** 思考强度：没有就不显示强度半截（契约 / 产品） */
export async function listCatThinkLevels(agentKind: CatAgentKind): Promise<CatThinkLevel[]> {
  if (agentKind !== CAT_AGENT_BUILD) return []
  const b = catBinding()
  if (!b?.ListCatThinkLevels) return []
  const raw = (await call(b.ListCatThinkLevels(agentKind))) as CatThinkLevel[] | null
  return Array.isArray(raw)
    ? raw.map((t) => ({ id: String(t.id), displayName: String(t.displayName || t.id) }))
    : []
}

export async function listCatConversations(): Promise<CatConversation[]> {
  const b = catBinding()
  if (!b?.ListCatConversations) return []
  const raw = (await call(b.ListCatConversations())) as CatConversation[] | null
  return Array.isArray(raw) ? raw : []
}

export async function getCatConversation(id: string): Promise<CatConversationDetail | null> {
  const b = catBinding()
  if (!b?.GetCatConversation) return null
  return (await call(b.GetCatConversation(id))) as CatConversationDetail
}

export async function createCatConversation(req: CreateCatConversationRequest): Promise<CatConversation> {
  if (req.agentKind !== CAT_AGENT_BUILD) {
    throw new AppError('INVALID_ARGUMENT', '不支持的模式')
  }
  const b = catBinding()
  if (!b?.CreateCatConversation) {
    const now = Date.now()
    return {
      id: `local-${now.toString(36)}`,
      title: req.title?.trim() || '新对话',
      agentKind: CAT_AGENT_BUILD,
      createdAt: now,
      updatedAt: now,
    }
  }
  return (await call(b.CreateCatConversation(req))) as CatConversation
}

export async function deleteCatConversation(id: string): Promise<void> {
  const b = catBinding()
  if (!b?.DeleteCatConversation) return
  await call(b.DeleteCatConversation(id))
}

export async function sendCatMessage(req: SendCatMessageRequest): Promise<SendCatMessageResult> {
  const b = catBinding()
  if (!b?.SendCatMessage) {
    // mock：组件未就绪 → 按契约抛 CAT_NOT_READY（页面用 CAT_COPY.notReady 展示）
    throw new AppError('CAT_NOT_READY', CAT_COPY.notReady)
  }
  return (await call(b.SendCatMessage(req))) as SendCatMessageResult
}

export async function cancelCatTurn(conversationId: string): Promise<void> {
  const b = catBinding()
  if (!b?.CancelCatTurn) return
  await call(b.CancelCatTurn(conversationId))
}
