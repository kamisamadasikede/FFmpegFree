/**
 * 契约 v0.47.1 §6.19.21.5：CLI 检测状态按 agents[] 逐家合并 + 每行展示文案。
 * 纯函数，不依赖 Vue / 绑定，自检见 catDetect.check.ts。
 */
import type { CatAgentStatus, CatStatus } from '@/api/cat'
import { CAT_PROVIDER_ORDER } from '@/api/catAgents'

/** 定稿文案（产品 + 设计 10-14）；全部用 --cat-muted 灰字 */
export const CAT_DETECT_COPY = {
  installed: '已安装',
  notDetected: '未检测到',
  checking: '正在检测…',
  lastCheck: (t: string) => `上次检测 ${t}`,
  stale: '结果可能已过期',
  recheck: '重新检测',
  pathTip: (p: string) => `路径：${p}`,
  sep: ' · ',
} as const

/** 载荷是否真带了 agents[]（mapCatStatus 在 raw 上做标记前先用这个判断） */
export function payloadHasAgents(raw: unknown): boolean {
  return !!raw && typeof raw === 'object' && Array.isArray((raw as { agents?: unknown }).agents)
}

/**
 * 合并：next 带 agents[] 时按 agentKind 覆盖对应行，其它行保留；
 * next 没有 agents[]（旧后端 / 异常）时只更新顶层兼容字段，不动任何行。
 */
export function mergeCatStatus(prev: CatStatus, next: CatStatus, nextHasAgents: boolean): CatStatus {
  if (!nextHasAgents) return { ...next, agents: prev.agents ?? [] }
  const by = new Map<string, CatAgentStatus>()
  for (const a of prev.agents ?? []) by.set(a.agentKind, a)
  for (const a of next.agents) by.set(a.agentKind, a)
  // 按注册顺序（cat_build → claude_cli → codex_cli）；以后新增的家排在后面
  const rank = (k: string) => { const i = (CAT_PROVIDER_ORDER as readonly string[]).indexOf(k); return i < 0 ? 99 : i }
  const agents = [...by.values()].sort((x, y) => rank(x.agentKind) - rank(y.agentKind))
  return { ...next, agents }
}

/** 只把某一家标成检测中（单家重新检测用），其它行原样 */
export function markRowChecking(s: CatStatus, kind: string): CatStatus {
  return { ...s, agents: (s.agents ?? []).map((a) => (a.agentKind === kind ? { ...a, checking: true } : a)) }
}

const pad = (n: number) => String(n).padStart(2, '0')
/** 今天：HH:mm；不是今天：M月D日 HH:mm（本地时间） */
export function formatCheckedAt(ms: number, now: number = Date.now()): string {
  const d = new Date(ms)
  const n = new Date(now)
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const today = d.getFullYear() === n.getFullYear() && d.getMonth() === n.getMonth() && d.getDate() === n.getDate()
  return today ? hm : `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`
}

export interface CatDetectRowView {
  /** 还没有任何结果，且正在等 / 检测中：整行只显示「正在检测…」 */
  pending: boolean
  checking: boolean
  ready: boolean
  /** 灰字一行，如「已安装 · 上次检测 14:32」 */
  meta: string
  /** stale 且不在检测中：显示「重新检测」文字按钮 */
  showRecheck: boolean
  /** 悬停 tooltip（无 exePath 时空） */
  pathTip: string
  /** 检测已完成且确实没检测到（可算真失败） */
  failed: boolean
}

/** 有过上次结果：checkedAt / ready 已知（true）/ stale。没结果的首次检测才整行只显示「正在检测…」。 */
export function hasDetectPreviousResult(row: CatAgentStatus | undefined): boolean {
  if (!row) return false
  if (typeof row.checkedAt === 'number') return true
  if (row.stale === true) return true
  if (row.ready === true) return true
  return false
}

/** 展示用的几家是否都在检测中（含结果未到 / forceChecking）；底部「重新检测」据此置灰。 */
export function allDetectRowsChecking(
  kinds: readonly string[],
  agents: CatAgentStatus[] | undefined,
  opts: { forceChecking?: boolean } = {},
): boolean {
  return kinds.every((k) => detectRowView(agents?.find((a) => a.agentKind === k), opts).checking)
}

/**
 * row 缺失 = 该家结果还没到（等同检测中）。forceChecking：全量「重新检测」进行中。
 * 字段 checking / checkedAt / stale / exePath 在旧绑定里可能缺省，全部容错。
 * 有上次结果时保留「已安装/未检测到」等，只把「上次检测…」换成「正在检测…」；stale 文案和按钮在检测中都不出。
 */
export function detectRowView(row: CatAgentStatus | undefined, opts: { forceChecking?: boolean; now?: number } = {}): CatDetectRowView {
  const C = CAT_DETECT_COPY
  const checking = !row || row.checking === true || opts.forceChecking === true
  const hasResult = !!row && (typeof row.checkedAt === 'number' || row.checking !== true)
  const pending = checking && !hasDetectPreviousResult(row)
  const ready = row?.ready === true
  const parts: string[] = []
  if (pending) parts.push(C.checking)
  else {
    parts.push(ready ? C.installed : C.notDetected)
    if (checking) parts.push(C.checking)
    else if (typeof row!.checkedAt === 'number') parts.push(C.lastCheck(formatCheckedAt(row!.checkedAt, opts.now)))
    if (!checking && row!.stale) parts.push(C.stale)
  }
  return {
    pending,
    checking,
    ready: ready && !pending,
    meta: parts.join(C.sep),
    showRecheck: !checking && row?.stale === true,
    pathTip: row?.exePath ? C.pathTip(row.exePath) : '',
    failed: hasResult && !checking && !ready,
  }
}

export type CatLoginTone = 'ok' | 'bad' | 'wait' | 'checking'
/**
 * 登录子行（产品 + 设计 10-14）：登录态不进 cat.detect。
 * - 检测中 / 结果未到 / 来自存档（stale）且 loggedIn 还没被实时检测确认 → 「正在检测…」灰字、无状态点
 * - 实时 true/false → 已登录 / 未登录（带点）
 * - 检测已完成（checking=false、stale=false）且 loggedIn 仍为 null/undefined → 隐藏整行（不显示「不适用」）
 */
export function detectLoginView(
  row: CatAgentStatus | undefined,
  v: Pick<CatDetectRowView, 'checking' | 'pending' | 'ready'>,
  copy: { loggedIn: string; loginBad: string; loginNa: string },
): { text: string; tone: CatLoginTone; bad: boolean } {
  const li = row?.loggedIn
  if (!row || v.pending || v.checking || (li == null && row.stale === true)) {
    return { text: CAT_DETECT_COPY.checking, tone: 'checking', bad: false }
  }
  if (li === true) return { text: copy.loggedIn, tone: 'ok', bad: false }
  if (li === false) return { text: copy.loginBad, tone: 'bad', bad: true }
  // 实时检测已结束且后端未给出 loggedIn：隐藏登录子行（架构师 #285 口径）
  return { text: '', tone: 'ok', bad: false }
}
