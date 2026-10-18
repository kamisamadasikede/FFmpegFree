/**
 * 项目群聊（契约 §6.21 ~ §6.21.19，v0.55 → v0.57）。
 * 有 Wails 绑定（CatService.ListCatGroups）走真后端，否则走 catGroupChatMock（浏览器预览 / 走查）。
 * 文案全部经 vue-i18n（cat.groupChat.*）。
 */
import { callService, hasServiceBinding } from '@/api/call'
import { onEvent } from '@/services/wails'
import { t } from '@/i18n'
import { groupMock } from './catGroupChatMock'
export { isMemberShortReply, memberShortText } from './catGroupReply'

export type CatAgentKind = 'cat_build' | 'claude_cli' | 'codex_cli' | string
export type CharterStatus = 'ok' | 'generating' | 'failed' | 'none'

export interface CatCharter {
  status: CharterStatus
  owns: string
  notTouch: string
  handoff: string
  dirs: string[]
  edited: string[]
  updatedAt: number
  /** 生成失败原因（BE 可能给 failReason / error / message；normalize 里合并） */
  failReason?: string
}
export interface CatGroup {
  id: string
  projectId: string
  name: string
  autoRoute: boolean
  createdAt: number
  updatedAt: number
  /** v0.55.6 6.21.17 ②：成员摘要（按 order 升序，侧栏头像用） */
  members?: CatGroupMemberBrief[]
}
export interface CatGroupMemberBrief {
  id: string
  name: string
  title: string
  agentKind: string
  order: number
  /** (order % 5) + 1 → --cat-member-{avatarIndex} */
  avatarIndex: number
}
/** v0.55.6 6.21.17 ①：ListCatGroupFiles 一行 */
export interface CatGroupFile {
  path: string
  lastMemberId: string
  lastSeq: number
  lastTurnId: string
  changeCount: number
  overreach: boolean
  tag?: string
  exists: boolean
  updatedAt: number
}
export interface CatGroupMember {
  id: string
  groupId: string
  name: string
  title: string
  agentKind: CatAgentKind
  modelId: string
  thinkLevelId?: string
  order: number
  charter: CatCharter
  /** v0.55.6：运行时只读，不落库 */
  status?: string
  /** 排队总数（不是 n；n 取 queue[].queuePosition） */
  queueLength?: number
  queue?: CatGroupQueuedTask[]
}
export interface CatGroupFileTag {
  path: string
  tag?: string
  added?: number
  deleted?: number
}
export interface CatMessageFooterLite {
  totalMs?: number
  outputTokens?: number
  tokenTotal?: number
  tokensPerSec?: number
  finishedAt?: number
}
export interface CatGroupReport {
  did: string
  attn?: string
  give?: string
  files: CatGroupFileTag[]
  parsed: boolean
  footer?: CatMessageFooterLite
}
export interface CatRouteInfo {
  mode: string
  /**
   * v0.57：ai 成功时主键（1～N）；与协调气泡 mentions[] 一致。
   * 旧单字段 memberId 仅兼容读入（见 routeMemberIdsOf）。
   */
  memberIds?: string[]
  /** @deprecated 写入作废；仅兼容旧读 → 视作 memberIds=[memberId] */
  memberId?: string
  /** v0.57：判为 @全体 时为 true；后端展开全员 id 写入 memberIds / mentions[] */
  everyone?: boolean
  /**
   * ai 成功：判断理由短句（≤40 字，进协调气泡）。
   * undecided（v0.56.1 / 6.21.18）：失败码 timeout|empty|parse_error|run_error|unknown_id，
   * 或后端已映射的中文短句；灰字 · 后缀读此字段。
   */
  reason?: string
  routeSeq?: number
  /** v0.55.6 6.21.17 ④-5：成员正在干活，这条进了他的队列 */
  queued?: boolean
  /** v0.55.6 / v0.55.9：ai / fallback / undecided 可有 */
  durationMs?: number
  /** 防御字段：若 BE 另下发 failReason/detail，undecidedRouteReasonOf 会读 */
  failReason?: string
  detail?: string
}
export interface CatGroupMessage {
  seq: number
  groupId: string
  kind: string
  memberId?: string
  text: string
  mentions?: string[]
  chainId: string
  hop: number
  turnId?: string
  route?: CatRouteInfo
  systemKind?: string
  fromMemberId?: string
  toMemberId?: string
  failReason?: string
  report?: CatGroupReport
  /**
   * v0.56 §6.21.19：短复 vs 汇报卡。
   * - `short` → 群里出短气泡（招呼/澄清/无交付）
   * - `report` → 四段汇报卡
   * 契约未写死字段名；后端若暂未发，见 isMemberShortReply 的兜底形状。
   */
  replyKind?: 'short' | 'report' | string
  /** 可选：短复正文；缺省用 text（或 report.did） */
  shortText?: string
  createdAt: number
}
export interface CatGroupRunning {
  turnId: string
  startedAt: number
  toolCount: number
  elapsedMs: number
}
/** v0.55.6 6.21.17 ④-3：成员内存 FIFO 里的一条排队任务 */
export interface CatGroupQueuedTask {
  chainId: string
  hop: number
  fromSeq: number
  /** 排在它前面的排队任务数，从 0 起（不含正在跑的那一轮） */
  queuePosition: number
}
export interface CatGroupMemberEvent {
  groupId: string
  member: CatGroupMember
  running?: CatGroupRunning
  /** v0.55.6 6.21.17 ④：= member.queueLength / member.queue */
  queueLength?: number
  queue?: CatGroupQueuedTask[]
  removed?: boolean
}
export interface CatGroupRouteEvent {
  groupId: string
  chainId: string
  state: 'routing' | 'routed'
  route: CatRouteInfo
}
export interface CatRouteDetail {
  input: string
  output: string
  mode: string
  memberId: string
  /** v0.57 */
  memberIds?: string[]
  everyone?: boolean
  reason: string
  /** v0.55.6 6.21.17 ③：unix ms；任一为 0 时没有 durationMs，不显示耗时 */
  startedAt?: number
  finishedAt?: number
  durationMs?: number
}
export interface RevertResult {
  reverted: string[]
  skipped: { path: string; reason: string }[]
}

/** BE 可能用 failReason / error / message 任一字段带回失败原因 */
export function charterFailReasonOf(c: Partial<CatCharter> | Record<string, unknown> | undefined | null): string {
  if (!c || typeof c !== 'object') return ''
  const r = c as Record<string, unknown>
  for (const k of ['failReason', 'error', 'message', 'reason', 'detail'] as const) {
    const v = r[k]
    if (typeof v === 'string' && v.trim()) return v.trim()
  }
  return ''
}

export function normalizeCharter(c: Partial<CatCharter> | undefined | null): CatCharter {
  const failReason = charterFailReasonOf(c)
  return {
    status: (c?.status as CharterStatus) || 'none',
    owns: c?.owns ?? '',
    notTouch: c?.notTouch ?? '',
    handoff: c?.handoff ?? '',
    dirs: Array.isArray(c?.dirs) ? [...c!.dirs] : [],
    edited: Array.isArray(c?.edited) ? [...c!.edited] : [],
    updatedAt: c?.updatedAt ?? 0,
    ...(failReason ? { failReason } : {}),
  }
}
export function normalizeMember(m: Partial<CatGroupMember> & { id: string }): CatGroupMember {
  return {
    id: m.id,
    groupId: m.groupId ?? '',
    name: m.name ?? '',
    title: m.title ?? '',
    agentKind: m.agentKind ?? 'cat_build',
    modelId: m.modelId ?? '',
    thinkLevelId: m.thinkLevelId || undefined,
    order: m.order ?? 0,
    charter: normalizeCharter(m.charter),
    status: m.status || undefined,
    queueLength: m.queueLength ?? m.queue?.length ?? 0,
    queue: Array.isArray(m.queue) ? m.queue.map((q) => ({ ...q })) : [],
  }
}

/** v0.57：从 route 取 memberIds[]（兼容旧 memberId） */
export function routeMemberIdsOf(route: Partial<CatRouteInfo> | Record<string, unknown> | undefined | null): string[] {
  if (!route || typeof route !== 'object') return []
  const r = route as Record<string, unknown>
  const ids = r.memberIds
  if (Array.isArray(ids) && ids.length) {
    return ids.filter((x): x is string => typeof x === 'string' && !!x)
  }
  if (typeof r.memberId === 'string' && r.memberId) return [r.memberId]
  return []
}

export function normalizeRoute(r: Partial<CatRouteInfo> | undefined | null): CatRouteInfo | undefined {
  if (!r) return undefined
  const memberIds = routeMemberIdsOf(r)
  return {
    mode: r.mode ?? '',
    ...(memberIds.length ? { memberIds, memberId: memberIds[0] } : {}),
    ...(r.everyone ? { everyone: true } : {}),
    ...(r.reason ? { reason: r.reason } : {}),
    ...(r.routeSeq != null ? { routeSeq: r.routeSeq } : {}),
    ...(r.queued ? { queued: true } : {}),
    ...(r.durationMs != null ? { durationMs: r.durationMs } : {}),
    ...(r.failReason ? { failReason: r.failReason } : {}),
    ...(r.detail ? { detail: r.detail } : {}),
  }
}

/** v0.57：系统身份「协调」短气泡（非成员、非灰字） */
export function isCoordMessage(m: Pick<CatGroupMessage, 'kind' | 'systemKind'>): boolean {
  return m.kind === 'system' && m.systemKind === 'coord'
}

/**
 * 协调气泡正文：优先 BE 已拼好的 text（v0.57 = 理由 + @成员…/@全体）；
 * 缺 text 时本地按同序拼：reason + @…。everyoneLabel 由调用方传入 i18n「全体」。
 */
export function coordBodyText(
  m: CatGroupMessage,
  nameOf: (id?: string) => string,
  everyoneLabel: string,
): string {
  if (m.text?.trim()) return m.text.trim()
  const reason = (m.route?.reason ?? '').trim() || ''
  let atPart = ''
  if (m.route?.everyone) {
    atPart = `@${everyoneLabel}`
  } else {
    const ids = (m.mentions?.length ? m.mentions : routeMemberIdsOf(m.route)).filter(Boolean)
    atPart = ids.map((id) => `@${nameOf(id)}`).filter((s) => s.length > 1).join(' ')
  }
  if (reason && atPart) return `${reason} ${atPart}`
  return reason || atPart
}

export function normalizeMessage(m: Partial<CatGroupMessage> & { seq: number }): CatGroupMessage {
  const r = m.report
  return {
    seq: m.seq,
    groupId: m.groupId ?? '',
    kind: m.kind ?? 'system',
    memberId: m.memberId || undefined,
    text: m.text ?? '',
    mentions: m.mentions ?? [],
    chainId: m.chainId ?? '',
    hop: m.hop ?? 0,
    turnId: m.turnId || undefined,
    route: normalizeRoute(m.route),
    systemKind: m.systemKind || undefined,
    fromMemberId: m.fromMemberId || undefined,
    toMemberId: m.toMemberId || undefined,
    failReason: m.failReason || undefined,
    report: r ? { did: r.did ?? '', attn: r.attn || undefined, give: r.give || undefined, files: r.files ?? [], parsed: r.parsed !== false, footer: r.footer } : undefined,
    replyKind: m.replyKind || undefined,
    shortText: m.shortText || undefined,
    createdAt: m.createdAt ?? 0,
  }
}


const k = (key: string, p?: Record<string, unknown>) => (p ? t(`cat.groupChat.${key}`, p) : t(`cat.groupChat.${key}`))

export function formatElapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  if (s < 60) return k('elapsedSec', { s })
  return k('elapsedMin', { m: Math.floor(s / 60), s: String(s % 60).padStart(2, '0') })
}


/** v0.56.1 / 6.21.18：undecided 机器码 → i18n 短句（与 BE undecidedReasonSuffix 对齐）。 */
const UNDECIDED_REASON_I18N: Record<string, string> = {
  timeout: 'routeUndecidedReasonTimeout',
  empty: 'routeUndecidedReasonEmpty',
  parse_error: 'routeUndecidedReasonParse',
  run_error: 'routeUndecidedReasonRun',
  unknown_id: 'routeUndecidedReasonUnknownId',
}

/**
 * undecided 灰字 · 后原因展示。
 * 优先 BE 原文（已是中文短句则原样）；机器码才映射。前端不另翻笼统句。
 * 防御读取 reason / failReason / detail / error / message。
 */
export function undecidedRouteReasonOf(
  route: Partial<CatRouteInfo> | Record<string, unknown> | undefined | null,
): string {
  if (!route || typeof route !== 'object') return ''
  const r = route as Record<string, unknown>
  if (typeof r.mode === 'string' && r.mode && r.mode !== 'undecided') return ''
  let raw = ''
  for (const key of ['reason', 'failReason', 'detail', 'error', 'message'] as const) {
    const v = r[key]
    if (typeof v === 'string' && v.trim()) {
      raw = v.trim()
      break
    }
  }
  if (!raw) return ''
  raw = raw.replace(/^[·•.\s]+/u, '').trim()
  if (!raw) return ''
  // 旧包曾把整句主文塞进 reason，不当作后缀
  if (raw.includes('\u6ca1\u5224\u65ad\u51fa\u8be5\u8c01\u63a5') || /couldn.?t tell who/i.test(raw)) return ''
  const i18nKey = UNDECIDED_REASON_I18N[raw]
  if (i18nKey) return k(i18nKey)
  return raw
}

/**
 * 灰字一行（契约 6.21.6 / 6.21.16 / 6.21.18 原文，经 i18n；产品 v0.56 交接文案）。
 * 「已请 {成员} 接着」：route + hop>0 且 mode 为 handoff/mention、无理由（消息流不写第 n/6；hop 仅过程抽屉）；
 * 「交给 {新}：你指定的」：换人后的 route，mode=reassign|named（后端 mode 取值未在契约写死，两种都认）。
 * TODO(v0.56+): 短复 vs 完整汇报卡等契约字段落地后再接 UI，本期只改灰字/空交给谁文案。
 */
export function systemLineText(m: CatGroupMessage, nameOf: (id?: string) => string): string {
  const toId = m.toMemberId || routeMemberIdsOf(m.route)[0] || m.route?.memberId || m.memberId
  const to = nameOf(toId)
  switch (m.systemKind) {
    case 'coord':
      // 协调气泡走专用短气泡 UI；灰字路径不应落到这里
      return m.text || m.route?.reason || ''
    case 'route': {
      if (m.route?.queued) return k('routeQueued', { name: to })
      const r = m.route
      if (r && m.hop > 0 && (r.mode === 'handoff' || r.mode === 'mention') && !r.reason) return k('handedOff', { name: to })
      if (r?.mode === 'named' || r?.mode === 'reassign') return k('routeTo', { name: to, reason: k('routeNamed') })
      if (r?.mode === 'followup') return k('routeTo', { name: to, reason: k('routeFollowup') })
      if (r?.mode === 'undecided') return k('routeUndecided')
      if (r?.mode === 'fallback') return k('routeTo', { name: to, reason: k('routeFallback') })
      if (r?.mode === 'off') return k('routeTo', { name: to, reason: k('routeOff') })
      if (r?.reason) return k('routeTo', { name: to, reason: r.reason })
      // 契约外的 mode（如 #333 的 retry）：只说交给谁，不猜原因
      return m.text || k('routePlain', { name: to })
    }
    case 'hop_limit': return k('hopLimit')
    case 'no_handoff': return k('noHandoff', { name: nameOf(m.memberId) })  // 空「交给谁」：弱灰字「{成员} 已完成」，不写「没有交给下一位」
    case 'reassign': return k('reassigned', { from: nameOf(m.fromMemberId), to: nameOf(m.toMemberId) })
    case 'failed': return k('failed', { name: nameOf(m.memberId), reason: m.failReason || m.text || k('unknownReason') })
    case 'stopped': return k('stopped', { name: nameOf(m.memberId) })
    case 'report_format': return k('reportFormat')
    default: return m.text
  }
}

/** 兜底解析（降级路径）：## 完成了什么 / 需要注意 / 交给谁，附别名 */
const HEADS: Record<'did' | 'attn' | 'give', RegExp> = {
  did: /^#{1,4}\s*(\u5b8c\u6210\u4e86\u4ec0\u4e48|\u505a\u4e86\u4ec0\u4e48|\u7ed3\u8bba|done|did|what i did)\s*$/i,
  attn: /^#{1,4}\s*(\u9700\u8981\u6ce8\u610f|\u6ce8\u610f|notes?|attention|heads up)\s*$/i,
  give: /^#{1,4}\s*(\u4ea4\u7ed9\u8c01|\u4e0b\u4e00\u6b65|handoff|next)\s*$/i,
}
export function parseReport(text: string): Pick<CatGroupReport, 'did' | 'attn' | 'give' | 'parsed'> {
  const out: Record<string, string[]> = {}
  let cur: string | null = null
  for (const line of text.split(/\r?\n/)) {
    const hit = (Object.keys(HEADS) as (keyof typeof HEADS)[]).find((h) => HEADS[h].test(line.trim()))
    if (hit) { cur = hit; out[hit] = []; continue }
    if (cur) out[cur].push(line)
  }
  if (!out.did) return { did: text.trim(), parsed: false }
  const j = (a?: string[]) => (a ? a.join('\n').trim() || undefined : undefined)
  return { did: j(out.did) ?? '', attn: j(out.attn), give: j(out.give), parsed: true }
}

/** toolCount 口径（6.21.14 ⑤）：kind=tool 首次出现的块数 */
export function countToolBlocks(seen: Set<string>, block: { id: string; kind: string }): number {
  if (block.kind === 'tool' && !seen.has(block.id)) seen.add(block.id)
  return seen.size
}

export function memberColorVar(order: number): string {
  return `var(--cat-member-${(((order % 5) + 5) % 5) + 1})`
}

export function groupRealApi(): boolean {
  return hasServiceBinding('CatService', 'ListCatGroups')
}
const S = 'CatService'
const real = {
  listGroups: (projectId: string) => callService<CatGroup[]>(S, 'ListCatGroups', projectId),
  getGroup: (groupId: string) => callService<{ group: CatGroup; members: CatGroupMember[] }>(S, 'GetCatGroup', groupId),
  createGroup: (projectId: string, name: string) => callService<CatGroup>(S, 'CreateCatGroup', { projectId, name }),
  renameGroup: (groupId: string, name: string) => callService<CatGroup>(S, 'RenameCatGroup', { groupId, name }),
  deleteGroup: (groupId: string) => callService<void>(S, 'DeleteCatGroup', groupId),
  setAutoRoute: (groupId: string, on: boolean) => callService<CatGroup>(S, 'SetCatGroupAutoRoute', groupId, on),
  addMember: (req: { groupId: string; name: string; title: string; agentKind: string; modelId: string; thinkLevelId?: string }) =>
    callService<CatGroupMember>(S, 'AddCatGroupMember', { thinkLevelId: '', ...req }),
  updateMember: (req: { memberId: string; name?: string; title?: string; agentKind?: string; modelId?: string; thinkLevelId?: string }) =>
    callService<CatGroupMember>(S, 'UpdateCatGroupMember', req),
  removeMember: (memberId: string) => callService<void>(S, 'RemoveCatGroupMember', memberId),
  reorderMembers: (groupId: string, memberIds: string[]) => callService<CatGroupMember[]>(S, 'ReorderCatGroupMembers', { groupId, memberIds }),
  regenerateCharter: (memberId: string) => callService<CatGroupMember>(S, 'RegenerateCatCharter', memberId),
  updateCharter: (req: { memberId: string; owns?: string; notTouch?: string; dirs: string[] }) => callService<CatGroupMember>(S, 'UpdateCatCharter', req),
  send: (groupId: string, text: string, mentions: string[]) =>
    callService<{ userMessage: CatGroupMessage; chainId: string }>(S, 'SendCatGroupMessage', { groupId, text, mentions }),
  reassign: (groupId: string, chainId: string, toMemberId: string) =>
    callService<{ system: CatGroupMessage }>(S, 'ReassignCatGroupRoute', { groupId, chainId, toMemberId }),
  stopGroup: (groupId: string) => callService<void>(S, 'StopCatGroup', groupId),
  stopMember: (memberId: string) => callService<void>(S, 'StopCatGroupMember', memberId),
  listMessages: (groupId: string, beforeSeq = 0, limit = 50) =>
    callService<{ messages: CatGroupMessage[]; hasMore: boolean }>(S, 'ListCatGroupMessages', { groupId, beforeSeq, limit }),
  routeDetail: (groupId: string, seq: number) => callService<CatRouteDetail>(S, 'GetCatRouteDetail', { groupId, seq }),
  listFiles: (groupId: string) => callService<CatGroupFile[]>(S, 'ListCatGroupFiles', groupId),
  revert: (memberId: string, turnId: string, files: string[]) => callService<RevertResult>(S, 'RevertCatOverreach', { memberId, turnId, files }),
  retry: (groupId: string, memberId: string, chainId: string) => callService<void>(S, 'RetryCatGroupMemberTurn', { groupId, memberId, chainId }),
  onMessage: (cb: (m: CatGroupMessage) => void) => onEvent<CatGroupMessage>('cat:group-message', cb),
  onMember: (cb: (e: CatGroupMemberEvent) => void) => onEvent<CatGroupMemberEvent>('cat:group-member', cb),
  onRoute: (cb: (e: CatGroupRouteEvent) => void) => onEvent<CatGroupRouteEvent>('cat:group-route', cb),
}
export type CatGroupApi = typeof real
export function groupApi(): CatGroupApi {
  return groupRealApi() ? real : (groupMock as unknown as CatGroupApi)
}
