/**
 * 项目群聊（契约 §6.21 ~ §6.21.16，v0.55 → v0.55.5）。
 * 有 Wails 绑定（CatService.ListCatGroups）走真后端，否则走 catGroupChatMock（浏览器预览 / 走查）。
 * 文案全部经 vue-i18n（cat.groupChat.*）。
 */
import { callService, hasServiceBinding } from '@/api/call'
import { onEvent } from '@/services/wails'
import { t } from '@/i18n'
import { groupMock } from './catGroupChatMock'

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
  memberId?: string
  reason?: string
  routeSeq?: number
  /** v0.55.6 6.21.17 ④-5：成员正在干活，这条进了他的队列 */
  queued?: boolean
  /** v0.55.6 6.21.17 ③：只有 ai / fallback 有 */
  durationMs?: number
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

export function normalizeCharter(c: Partial<CatCharter> | undefined | null): CatCharter {
  return {
    status: (c?.status as CharterStatus) || 'none',
    owns: c?.owns ?? '',
    notTouch: c?.notTouch ?? '',
    handoff: c?.handoff ?? '',
    dirs: Array.isArray(c?.dirs) ? [...c!.dirs] : [],
    edited: Array.isArray(c?.edited) ? [...c!.edited] : [],
    updatedAt: c?.updatedAt ?? 0,
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
    route: m.route ?? undefined,
    systemKind: m.systemKind || undefined,
    fromMemberId: m.fromMemberId || undefined,
    toMemberId: m.toMemberId || undefined,
    failReason: m.failReason || undefined,
    report: r ? { did: r.did ?? '', attn: r.attn || undefined, give: r.give || undefined, files: r.files ?? [], parsed: r.parsed !== false, footer: r.footer } : undefined,
    createdAt: m.createdAt ?? 0,
  }
}

const k = (key: string, p?: Record<string, unknown>) => (p ? t(`cat.groupChat.${key}`, p) : t(`cat.groupChat.${key}`))

export function formatElapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  if (s < 60) return k('elapsedSec', { s })
  return k('elapsedMin', { m: Math.floor(s / 60), s: String(s % 60).padStart(2, '0') })
}

/**
 * 灰字一行（契约 6.21.6 / 6.21.16 原文，经 i18n）。
 * 「已转交给 {成员}（第 n/6 次）」：route + hop>0 且 mode 为 handoff/mention、无理由；
 * 「交给 {新}：你指定的」：换人后的 route，mode=reassign|named（后端 mode 取值未在契约写死，两种都认）。
 */
export function systemLineText(m: CatGroupMessage, nameOf: (id?: string) => string): string {
  const to = nameOf(m.toMemberId || m.route?.memberId || m.memberId)
  switch (m.systemKind) {
    case 'route': {
      if (m.route?.queued) return k('routeQueued', { name: to })
      const r = m.route
      if (r && m.hop > 0 && (r.mode === 'handoff' || r.mode === 'mention') && !r.reason) return k('handedOff', { name: to, n: m.hop })
      if (r?.mode === 'named' || r?.mode === 'reassign') return k('routeTo', { name: to, reason: k('routeNamed') })
      if (r?.mode === 'followup') return k('routeTo', { name: to, reason: k('routeFollowup') })
      if (r?.mode === 'fallback') return k('routeTo', { name: to, reason: k('routeFallback') })
      if (r?.mode === 'off') return k('routeTo', { name: to, reason: k('routeOff') })
      if (r?.reason) return k('routeTo', { name: to, reason: r.reason })
      // 契约外的 mode（如 #333 的 retry）：只说交给谁，不猜原因
      return m.text || k('routePlain', { name: to })
    }
    case 'hop_limit': return k('hopLimit')
    case 'no_handoff': return k('noHandoff', { name: nameOf(m.memberId) })
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
