import { readCatHistoryStatus, type CatHistoryStatus } from './catHistoryStatus'
import { pickCatOutput, pickCatTiming } from '@/components/cat/catTimingAdapter'
import { t } from '@/i18n'
/**
 * Cat 助手接口层（契约 v0.30 §6.19；v0.36 多服务商 cat_build|claude_cli|codex_cli）。
 *
 * CAT_BACKEND_READY=true 且在 Wails 里：调真实 CatService 绑定（wailsjs/go/app/CatService）。
 * 纯浏览器（没有 window.go，走查 / 截图）：仍走 typed mock（模型 / 思考强度为空，发送返回未就绪）。
 * 事件：cat:status / cat:message / cat:turn，归一化见 api/catStream.ts。
 *
 * 气泡 / 侧栏总称「Cat 助手」；服务商选择处写「Grok Build」「Claude」「Codex」。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat, catagent } from '../../wailsjs/go/models'
import { AppError, call, callService } from '@/api/call'
import { CAT_BACKEND_READY } from '@/api/flags'
import { readRetryableTurn } from '@/api/catV0445'
import { hasWailsBackend, onTaskEvent } from '@/services/wails'
import { normalizeMessageEvent, normalizeTurnEvent, type CatStreamEvent, type CatTurnEvent } from '@/api/catStream'
import { asPreviewKind, type CatShownFile } from '@/components/cat/catMedia'

export { CAT_BACKEND_READY }

/** 契约 6.19.7 冻结文案（stopped：产品定，停止生成后单独一行浅灰系统提示） */
export const CAT_COPY = {
  get notReady() { return t('cat.copy.notReady') },
  get replyFailed() { return t('cat.copy.replyFailed') },
  /** v0.53.1：CAT_REPLY_FAILED reason=idle_timeout（CLI 连续 10 分钟没有任何输出） */
  get idleTimeout() { return t('cat.copy.idleTimeout') },
  /** v0.53.1：CAT_REPLY_FAILED reason=timeout（总时长上限，仅定时任务） */
  get totalTimeout() { return t('cat.copy.totalTimeout') },
  get writeRefused() { return t('cat.copy.writeRefused') },
  get later() { return t('cat.copy.later') },
  get stopped() { return t('cat.copy.stopped') },
}

/**
 * 删除对话（设计「Cat 项目状态 v0.3」§11–15）。
 * deleteFailed：产品 10-09 已定（浮提示；失败时对话行保持不动）。
 */
export const CAT_CONV_COPY = {
  get menu() { return t('cat.conv.menu') },
  get del() { return t('cat.conv.del') },
  get delTitle() { return t('cat.conv.delTitle') },
  get delBody() { return t('cat.conv.delBody') },
  get cancel() { return t('cat.conv.cancel') },
  get deleting() { return t('cat.conv.deleting') },
  get deleted() { return t('cat.conv.deleted') },
  get deleteFailed() { return t('cat.conv.deleteFailed') },
}

/** 事件名（契约 6.19.5） */
export const CAT_EVENTS = { status: 'cat:status', message: 'cat:message', turn: 'cat:turn' } as const

export {
  CAT_AGENT_BUILD,
  CAT_AGENT_CLAUDE,
  CAT_AGENT_CODEX,
  CAT_PROVIDER_ORDER,
  CAT_PROVIDER_LABEL,
  isProviderKind,
  normalizeProviderKind,
  providerLabel,
  type CatProviderKind,
  type CatAgentKind,
} from '@/api/catAgents'
import {
  CAT_AGENT_BUILD,
  CAT_PROVIDER_ORDER,
  isProviderKind,
  type CatAgentKind,
  type CatProviderKind,
} from '@/api/catAgents'

export type CatComponentState = 'checking' | 'ready' | 'missing' | 'failed'

/** 单家状态（契约 v0.36 CatAgentStatus） */
/** v0.47.1 检测字段直接用生成的 catagent.AgentStatus（checking/checkedAt/stale/exePath） */
export interface CatAgentStatus extends Partial<Pick<catagent.AgentStatus, 'checking' | 'checkedAt' | 'stale' | 'exePath'>> {
  agentKind: CatProviderKind
  ready: boolean
  version: string
  detail?: string
  /** v0.44 agents[].modes 原样（由 views/cat/catCapabilities 的 mapAgentModes 解析） */
  rawModes?: unknown
}

export interface CatStatus {
  state: CatComponentState
  version: string
  canDownload: boolean
  error?: { code: string; message: string; detail?: string } | null
  /** v0.36：固定顺序 cat_build → claude_cli → codex_cli；缺省时由 mapCatStatus 补空行 */
  agents: CatAgentStatus[]
  /** v0.47.1：原始载荷是否带 agents[]；false 时合并只动顶层字段 */
  hasAgents?: boolean
}

export interface CatModel {
  id: string
  displayName: string
  /** v0.44：该模型支持的档位 id（未知 = []）；旧后端没有该字段 */
  efforts?: string[]
  defaultEffort?: string
}

export interface CatThinkLevel {
  id: string
  displayName: string
}

export interface CatConversation {
  id: string
  title: string
  /** 创建后不可变；v0.36：cat_build|claude_cli|codex_cli */
  agentKind: CatAgentKind
  /** v0.31：所属项目，创建时写入、之后不可变；空 = 不属于任何项目（显示在「对话」下） */
  projectId?: string
  createdAt: number
  updatedAt: number
}

export interface CatMessage {
  id: string
  role: 'user' | 'assistant' | 'system' | 'thought' | 'tool' | 'status'
  content: string
  createdAt: number
  /** 刷新后从会话记录里读出的图片、视频和可预览文档 */
  media?: CatShownFile[]
  /** v0.37.1 消息尾（仅 assistant；缺省不显示） */
  finishedAt?: number
  tokenTotal?: number
  /** v0.47 §6.19.19.5 输出 token；缺省则不带 tokensPerSec */
  outputTokens?: number
  tokensPerSec?: number
  /** v0.45 首字 / 总耗时（毫秒；缺省不显示） */
  firstTokenMs?: number
  totalMs?: number
}

export interface CatConversationDetail extends CatConversation {
  messages: CatMessage[]
  /** v0.45.1：没有正文的原因（有消息时不带） */
  historyStatus?: CatHistoryStatus
  /** 打开会话时带上的上下文占用；没有真实数字则为 0 */
  contextUsed: number
  contextWindow: number
  /** v0.44.6：由 ConversationDetail.lastTurn 推导；failed && retryable → turnId，否则 '' */
  retryTurnId: string
}

export interface CreateCatConversationRequest {
  agentKind: CatAgentKind
  title?: string
  /** v0.31：只能在创建时给（项目行「+」）；之后没有任何接口能改 */
  projectId?: string
}

export interface SendCatMessageRequest {
  conversationId: string
  content: string
  modelId?: string
  thinkLevelId?: string
  /** normal / plan / auto / approve。空等于全部批准 */
  permissionMode?: string
  /** 这一轮附上的本机文件。图片按内容块发送，其它文件带路径 */
  attachments?: { name: string; path: string }[]
}

/**
 * 后端 SendCatMessage 返回（契约 v0.30.1：用户消息落库、本轮开始后立即返回，带 turnId，前端记下用于 CancelCatTurn）。
 * 助手回复走 cat:message / cat:turn；assistantMessage 只为兼容保留（流式路径下为空）。
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
/** 是否走真后端（供其它 Cat 模块判断） */
export const catBackendLive = (): boolean => live()

/** 是否走真绑定（CAT_BACKEND_READY=true 且在 Wails 里） */
export const isCatLive = (): boolean => live()

function missingStatus(): CatStatus {
  return {
    state: 'missing',
    version: '',
    canDownload: false,
    error: { code: 'CAT_NOT_READY', message: CAT_COPY.notReady },
    agents: emptyAgents(),
  }
}

const STATES: CatComponentState[] = ['checking', 'ready', 'missing', 'failed']

function emptyAgents(): CatAgentStatus[] {
  return CAT_PROVIDER_ORDER.map((agentKind) => ({ agentKind, ready: false, version: '' }))
}

/** v0.47.1：只返回载荷里真有的行（按注册顺序），不再补「顶层映射为 Grok、其余 ready:false」的假行 */
function mapAgents(raw: unknown): CatAgentStatus[] {
  const by = new Map<string, CatAgentStatus>()
  if (Array.isArray(raw)) {
    for (const item of raw) {
      if (!item || typeof item !== 'object') continue
      const r = item as Record<string, unknown>
      if (!isProviderKind(String(r.agentKind ?? ''))) continue
      const kind = r.agentKind as CatProviderKind
      by.set(kind, {
        agentKind: kind,
        ready: r.ready === true,
        version: typeof r.version === 'string' ? r.version : '',
        detail: typeof r.detail === 'string' ? r.detail : '',
        rawModes: r.modes,
        checking: r.checking === true,
        checkedAt: typeof r.checkedAt === 'number' && r.checkedAt > 0 ? r.checkedAt : undefined,
        stale: r.stale === true,
        exePath: typeof r.exePath === 'string' && r.exePath ? r.exePath : undefined,
      })
    }
  }
  return CAT_PROVIDER_ORDER.filter((k) => by.has(k)).map((k) => by.get(k)!)
}

export function mapCatStatus(raw: unknown): CatStatus {
  if (!raw || typeof raw !== 'object') return missingStatus()
  const r = raw as {
    state?: string
    version?: string
    agents?: unknown
    error?: { code?: string; message?: string; detail?: string } | null
  }
  const state = STATES.includes(r.state as CatComponentState) ? (r.state as CatComponentState) : 'missing'
  const e = r.error
  const version = r.version ?? ''
  return {
    state,
    version,
    // 一期组件未发布：无论后端返回什么都不出现下载入口
    canDownload: false,
    error: e && typeof e.code === 'string' ? { code: e.code, message: e.message ?? '', detail: e.detail } : null,
    agents: mapAgents(r.agents),
    hasAgents: Array.isArray(r.agents),
  }
}

const KINDS: CatAgentKind[] = [CAT_AGENT_BUILD, 'claude_cli', 'codex_cli', 'cat_cli', 'cat_code', 'cat_agent']
function mapConv(raw: { id?: string; title?: string; agentKind?: string; projectId?: string; createdAt?: number; updatedAt?: number }): CatConversation {
  return {
    id: String(raw.id ?? ''),
    title: raw.title?.trim() || t('cat.misc.s_1ac07a4bb2'),
    // 原样保留后端存的 agentKind（不改写）；未知值只用于展示，发送仍走会话自己的 id
    agentKind: (KINDS.includes(raw.agentKind as CatAgentKind) ? raw.agentKind : CAT_AGENT_BUILD) as CatAgentKind,
    projectId: raw.projectId || undefined,
    createdAt: Number(raw.createdAt) || 0,
    updatedAt: Number(raw.updatedAt) || 0,
  }
}

function mapMedia(raw: unknown): CatShownFile[] | undefined {
  if (!Array.isArray(raw)) return undefined
  const out: CatShownFile[] = []
  for (const item of raw) {
    if (!item || typeof item !== 'object') continue
    const r = item as Record<string, unknown>
    const name = typeof r.name === 'string' ? r.name.trim() : ''
    const path = typeof r.path === 'string' ? r.path : ''
    const data = typeof r.data === 'string' ? r.data : ''
    const kind = asPreviewKind(typeof r.kind === 'string' ? r.kind : '')
    if (!name && !path && !data) continue
    out.push({
      name: name || t('cat.misc.s_2a0c4740f1'),
      path: path || undefined,
      kind,
      mime: typeof r.mime === 'string' ? r.mime : undefined,
      data: data || undefined,
    })
  }
  return out.length ? out : undefined
}

function mapMsg(raw: { id?: string; role?: string; content?: string; createdAt?: number; media?: unknown; finishedAt?: unknown; tokenTotal?: unknown; outputTokens?: unknown; tokensPerSec?: unknown } | null | undefined): CatMessage {
  const r = raw?.role
  const role = r === 'user' || r === 'system' || r === 'thought' || r === 'tool' ? r : 'assistant'
  const msg: CatMessage = {
    id: String(raw?.id ?? ''),
    role,
    content: String(raw?.content ?? ''),
    createdAt: Number(raw?.createdAt) || 0,
    media: role === 'user' ? mapMedia(raw?.media) : undefined,
  }
  // v0.37.1：仅助手带回消息尾；旧消息缺字段不补假数据
  if (role === 'assistant' && raw) {
    const finishedAt = Number(raw.finishedAt)
    if (Number.isFinite(finishedAt) && finishedAt > 0) msg.finishedAt = finishedAt
    if (raw.tokenTotal !== undefined && raw.tokenTotal !== null && raw.tokenTotal !== '') {
      const tot = Number(raw.tokenTotal)
      if (Number.isFinite(tot)) msg.tokenTotal = tot
    }
    // v0.47：outputTokens + 后端 tokensPerSec 同进退
    Object.assign(msg, pickCatOutput(raw), pickCatTiming(raw))
  }
  return msg
}

export async function getCatStatus(): Promise<CatStatus> {
  if (!live()) return missingStatus()
  return mapCatStatus(await call(CatBinding.GetCatStatus()))
}

/** v0.48 6.19.12.11-1：上报主窗口键盘焦点（后端据此决定普通 turn 是否发系统通知）。失败静默。 */
export async function setCatWindowFocused(focused: boolean): Promise<void> {
  if (!live()) return
  try { await CatBinding.SetCatWindowFocused(focused) } catch { /* 服务未就绪：忽略 */ }
}

export interface CatNotifyStatus { available: boolean; authorized: boolean }

/** v0.48 6.19.12.11-5：系统通知是否可用 / 已授权；浏览器 / 出错 → null（不显示提示）。 */
export async function getCatNotifyStatus(): Promise<CatNotifyStatus | null> {
  if (!live()) return null
  try {
    const r = await CatBinding.GetCatNotifyStatus()
    return { available: r?.available === true, authorized: r?.authorized === true }
  } catch {
    return null
  }
}

/** 6.19.12.11-5 灰字：开关开且不可用 / 未授权时显示；其余返回空串。 */
export function catNotifyHint(on: boolean, st: CatNotifyStatus | null, isMac: boolean): string {
  if (!on || !st) return ''
  if (st.available && st.authorized) return ''
  if (isMac && st.available && !st.authorized) return CAT_NOTIFY_HINT.macDenied
  return CAT_NOTIFY_HINT.unavailable
}

export const CAT_NOTIFY_HINT = {
  get macDenied() { return t('cat.notify.macDenied') },
  get unavailable() { return t('cat.notify.unavailable') },
}

/** v0.47.1：RecheckCat(agentKind)。空 = 全部；传了只检该家（返回仍是全量 agents[]）。 */
export async function recheckCat(agentKind: CatProviderKind | '' = ''): Promise<CatStatus> {
  if (!live()) return missingStatus()
  return mapCatStatus(await call(CatBinding.RecheckCat(agentKind)))
}

function mapList<T extends { id: string; displayName: string }>(raw: unknown): T[] {
  return Array.isArray(raw)
    ? raw
        .filter((m) => m && typeof m.id === 'string' && m.id)
        .map((m) => ({ id: String(m.id), displayName: String(m.displayName || m.id) }) as T)
    : []
}

/**
 * 模型列表：v0.36 接受三家。未知 / 未注册 / 浏览器 / 出错 → 空数组（不造展示名）。
 */
export async function listCatModels(agentKind: CatAgentKind): Promise<CatModel[]> {
  if (!isProviderKind(agentKind) || !live()) return []
  try {
    const raw: unknown = await call(CatBinding.ListCatModels(agentKind))
    const byId = new Map<string, Record<string, unknown>>()
    for (const m of Array.isArray(raw) ? raw : []) if (m && typeof m.id === 'string') byId.set(m.id, m)
    return mapList<CatModel>(raw).map((m) => {
      const r = byId.get(m.id) ?? {}
      return Array.isArray(r.efforts)
        ? { ...m, efforts: r.efforts.filter((e: unknown): e is string => typeof e === 'string'), defaultEffort: typeof r.defaultEffort === 'string' ? r.defaultEffort : '' }
        : m
    })
  } catch {
    return []
  }
}

/** 契约 v0.35：xhigh 显示「极高」（值不变）；后端老版本还给「超高」时也在这里改掉 */
export const CAT_THINK_XHIGH_LABEL = t('cat.misc.s_cfebcb06bc')
export function fixThinkName(level: CatThinkLevel): CatThinkLevel {
  if (level.id === 'xhigh' || level.displayName === t('cat.misc.s_1a42e25f14')) return { ...level, displayName: CAT_THINK_XHIGH_LABEL }
  return level
}

/** 思考强度：列表空 → 界面整行隐藏（契约 v0.36）；不写死某家永远没有 */
export async function listCatThinkLevels(agentKind: CatAgentKind): Promise<CatThinkLevel[]> {
  if (!isProviderKind(agentKind) || !live()) return []
  try {
    return mapList<CatThinkLevel>(await call(CatBinding.ListCatThinkLevels(agentKind))).map(fixThinkName)
  } catch {
    return []
  }
}

export async function listCatConversations(): Promise<CatConversation[]> {
  if (!live()) return []
  const raw = await call(CatBinding.ListCatConversations())
  return Array.isArray(raw) ? raw.map(mapConv) : []
}

export interface LiveCatTurn {
  convId: string
  turnId: string
}

/**
 * 刷新后仍在回复的对话。
 * 绑定还没装上、或这一次没问到，返回 null：调用方保持现状，不要当成「都已经结束」。
 * 问到了但没有，返回空数组。
 */
export async function listLiveCatTurns(): Promise<LiveCatTurn[] | null> {
  if (!live()) return []
  try {
    const raw = await callService<unknown>('CatService', 'ListLiveCatTurns')
    if (!Array.isArray(raw)) return []
    return raw.flatMap((item) => {
      const r = item && typeof item === 'object' ? (item as Record<string, unknown>) : {}
      const convId = typeof r.convId === 'string' ? r.convId : ''
      if (!convId) return []
      return [{ convId, turnId: typeof r.turnId === 'string' ? r.turnId : '' }]
    })
  } catch {
    return null
  }
}

export async function getCatConversation(id: string): Promise<CatConversationDetail | null> {
  if (!live()) return null
  const raw = await call(CatBinding.GetCatConversation(id))
  if (!raw) return null
  return {
    ...mapConv(raw),
    messages: Array.isArray(raw.messages) ? raw.messages.map(mapMsg) : [],
    contextUsed: positiveNum(raw.contextUsed),
    contextWindow: positiveNum(raw.contextWindow),
    historyStatus: readCatHistoryStatus(raw),
    retryTurnId: readRetryableTurn(raw),
  }
}

function positiveNum(v: unknown): number {
  const n = typeof v === 'number' ? v : typeof v === 'string' && v.trim() ? Number(v) : NaN
  return Number.isFinite(n) && n > 0 ? n : 0
}

/** 创建会话：agentKind 在这里写入，之后任何接口都不再改它（契约 6.19.2 / v0.36） */
export async function createCatConversation(req: CreateCatConversationRequest): Promise<CatConversation> {
  if (!isProviderKind(req.agentKind)) {
    throw new AppError('INVALID_ARGUMENT', t('cat.misc.s_501dc143fb'))
  }
  if (!live()) {
    const now = Date.now()
    return {
      id: `local-${now.toString(36)}`,
      title: req.title?.trim() || t('cat.misc.s_1ac07a4bb2'),
      agentKind: req.agentKind,
      projectId: req.projectId || undefined,
      createdAt: now,
      updatedAt: now,
    }
  }
  // v0.31：projectPath 已停用（后端恒为空，只为绑定兼容传 ""）；归属只看 projectId，没有项目时省略
  const body = cat.CreateConversationRequest.createFrom({
    agentKind: req.agentKind,
    title: req.title?.trim() ?? '',
    projectPath: '',
    accessMode: 'ask',
  })
  if (req.projectId) body.projectId = req.projectId
  return mapConv(await call(CatBinding.CreateCatConversation(body)))
}

/**
 * 删除对话（契约 v0.31.2）：进行中的一轮由后端先停（最多等 5 秒），前端不先调 CancelCatTurn；
 * 不存在的 id 返回 nil（可重复删），所以没有 NOT_FOUND 分支。
 * 浏览器 mock：会话列表就在页面内存里（catState），这里直接成功，由调用方移除。
 */
export async function deleteCatConversation(id: string): Promise<void> {
  if (!id) throw new AppError('INVALID_ARGUMENT', t('cat.misc.s_fcadf0ee8c'))
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
  const body = cat.SendMessageRequest.createFrom({
    conversationId: req.conversationId,
    content: req.content,
    modelId: req.modelId ?? '',
    thinkLevelId: req.thinkLevelId ?? '',
    permissionMode: req.permissionMode ?? '',
    projectPath: '',
  })
  // 生成的构造器只拷它认识的字段。附件在构造之后挂上，绑定按整个对象送过去。
  if (req.attachments?.length) {
    ;(body as unknown as { attachments: { name: string; path: string }[] }).attachments = req.attachments
  }
  const raw = await call(CatBinding.SendCatMessage(body))
  return {
    turnId: String(raw?.turnId ?? ''),
    userMessage: mapMsg(raw?.userMessage),
    assistantMessage: raw?.assistantMessage ? mapMsg(raw.assistantMessage) : null,
  }
}

/** 停止生成（契约 v0.30.1 §6.19.9：CancelCatTurn({ convId, turnId })，立即停止、幂等） */
export async function cancelCatTurn(req: CancelCatTurnRequest): Promise<void> {
  if (!req.convId || !live()) return
  await call(CatBinding.CancelCatTurn(catagent.CancelCatTurnRequest.createFrom({ convId: req.convId, turnId: req.turnId ?? '' })))
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

/**
 * 契约 v0.44 6.19.18.7：原地重试最后一轮失败的 turn（不新增用户气泡）。
 * 绑定由 wails generate module 生成（后端 #258）。
 */
export interface RetryCatTurnRequest {
  conversationId: string
  turnId: string
}
export async function retryCatTurn(req: RetryCatTurnRequest): Promise<{ turnId: string }> {
  if (!live()) throw new AppError('CAT_NOT_READY', CAT_COPY.notReady)
  const raw = (await call(CatBinding.RetryCatTurn({ conversationId: req.conversationId, turnId: req.turnId }))) as { turnId?: unknown } | null
  return { turnId: String(raw?.turnId ?? '') }
}

/** v0.53.1：失败 detail 第一行的超时 reason → 专用文案；不是超时返回 '' */
export function catTimeoutText(detail: unknown): string {
  if (typeof detail !== 'string') return ''
  const first = detail.split('\n', 1)[0].trim()
  if (first === 'reason=idle_timeout') return CAT_COPY.idleTimeout
  if (first === 'reason=timeout') return CAT_COPY.totalTimeout
  return ''
}
