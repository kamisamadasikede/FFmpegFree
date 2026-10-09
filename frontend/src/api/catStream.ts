/**
 * Cat 助手流式回复的纯逻辑（无 Vue / 无 Wails，便于 api.check.ts 自检）。
 *
 * 事件形状（架构 / 产品定稿，下一包优先）：
 *   cat:message { convId, turnId, messageId, seq, op: 'append' | 'replace' | 'done', block?: { type, text? }, textDelta? }
 *   cat:turn    { convId, turnId, status: 'running' | 'cancelled' | 'failed' | 'completed', error? }
 * 兼容后端 #172 当前骨架形状（整段，不带 seq）：
 *   cat:message { conversationId, messageId, role, content, delta?, createdAt }
 *   cat:turn    { conversationId, status: 'succeeded' | 'failed' | 'canceled', error? }
 * 两种都归一成 CatStreamEvent / CatTurnEvent，页面只认归一后的形状。
 */

export type CatStreamOp = 'append' | 'replace' | 'done'

export interface CatStreamEvent {
  convId: string
  turnId: string
  messageId: string
  /** 无 seq（旧形状）时为 undefined，按到达顺序应用 */
  seq?: number
  op: CatStreamOp
  /** v0.30.1 事件不带 role（只推助手消息），缺省 assistant；旧形状的 user 由前端本地先放，忽略 */
  role: 'assistant' | 'user' | 'system'
  blockType?: string
  /** append：增量；replace：整段 */
  text: string
}

export type CatTurnStatus = 'running' | 'cancelled' | 'failed' | 'completed'

export interface CatTurnEvent {
  convId: string
  turnId: string
  status: CatTurnStatus
  errorCode?: string
}

type Raw = Record<string, unknown>
const str = (v: unknown): string => (typeof v === 'string' ? v : v == null ? '' : String(v))

export function normalizeMessageEvent(raw: unknown): CatStreamEvent | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Raw
  const convId = str(r.convId ?? r.conversationId)
  const messageId = str(r.messageId)
  if (!convId || !messageId) return null
  const role = (str(r.role) || 'assistant') as CatStreamEvent['role']
  const seq = typeof r.seq === 'number' && Number.isFinite(r.seq) ? r.seq : undefined
  const op0 = str(r.op)
  if (op0 === 'append' || op0 === 'replace' || op0 === 'done') {
    const block = (r.block && typeof r.block === 'object' ? r.block : null) as Raw | null
    const text = op0 === 'append' ? str(r.textDelta) : op0 === 'replace' ? str(block?.text ?? r.textDelta) : ''
    return { convId, turnId: str(r.turnId), messageId, seq, op: op0, role, blockType: block ? str(block.type) : undefined, text }
  }
  // 旧形状：delta=true 追加，否则整段替换并视为已完成
  const content = str(r.content)
  if (r.delta === true) return { convId, turnId: str(r.turnId), messageId, seq, op: 'append', role, text: content }
  return { convId, turnId: str(r.turnId), messageId, seq, op: 'replace', role, text: content }
}

export function normalizeTurnEvent(raw: unknown): CatTurnEvent | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Raw
  const convId = str(r.convId ?? r.conversationId)
  if (!convId) return null
  const s = str(r.status)
  const status: CatTurnStatus | null =
    s === 'running' || s === 'started' ? 'running'
      : s === 'cancelled' || s === 'canceled' ? 'cancelled'
        : s === 'failed' ? 'failed'
          : s === 'completed' || s === 'succeeded' ? 'completed'
            : null
  if (!status) return null
  const err = r.error && typeof r.error === 'object' ? (r.error as Raw) : null
  return { convId, turnId: str(r.turnId), status, errorCode: err ? str(err.code) || undefined : undefined }
}

/** 一条助手消息的流式拼装状态 */
export interface CatStreamMsg {
  messageId: string
  text: string
  done: boolean
  /** 下一条期望的 seq；undefined = 还没确定起点 */
  next?: number
  /** 乱序先到的事件（seq → 事件） */
  buf: Map<number, CatStreamEvent>
  /** 已应用过的 seq（防重复） */
  seen: Set<number>
}

/** 乱序缓冲上限：超过后丢弃最远的，避免无限增长 */
export const CAT_STREAM_BUF_MAX = 512

export function newStreamMsg(messageId: string): CatStreamMsg {
  return { messageId, text: '', done: false, buf: new Map(), seen: new Set() }
}

function applyOne(m: CatStreamMsg, ev: CatStreamEvent) {
  if (ev.op === 'append') m.text += ev.text
  else if (ev.op === 'replace') m.text = ev.text
  else m.done = true
  if (ev.seq !== undefined) {
    m.seen.add(ev.seq)
    m.next = ev.seq + 1
  }
}

function drain(m: CatStreamMsg) {
  while (m.next !== undefined && m.buf.has(m.next)) {
    const e = m.buf.get(m.next)!
    m.buf.delete(m.next)
    applyOne(m, e)
  }
}

/** 按序号顺序应用剩下的缓冲（done 到了但中间缺号：不再等，按顺序补上） */
function flushGaps(m: CatStreamMsg) {
  const keys = [...m.buf.keys()].sort((a, b) => a - b)
  for (const k of keys) {
    const e = m.buf.get(k)!
    m.buf.delete(k)
    if (!m.seen.has(k)) applyOne(m, e)
  }
}

/**
 * 把一条事件合进消息；返回 true 表示文本或完成状态有变化。
 * - 重复 seq：忽略
 * - 比期望小（已过去）：忽略
 * - 比期望大（有缺口）：先缓冲，等缺的到了再一起应用；done 到达时不再等缺口
 * - replace：权威整段，直接替换并把起点挪到它之后，更早的缓冲丢弃
 * - 已 done 后的事件：忽略
 */
export function applyStreamEvent(m: CatStreamMsg, ev: CatStreamEvent): boolean {
  if (m.done) return false
  const before = m.text
  if (ev.seq === undefined) {
    applyOne(m, ev)
    return m.text !== before || m.done
  }
  const s = ev.seq
  if (m.seen.has(s) || m.buf.has(s)) return false
  if (m.next !== undefined && s < m.next) return false

  if (ev.op === 'replace') {
    for (const k of [...m.buf.keys()]) if (k < s) m.buf.delete(k)
    applyOne(m, ev)
    drain(m)
  } else if (m.next === undefined) {
    // 起点：0 或 1 直接开始；更大的号先缓冲（前面的可能还在路上）
    if (s <= 1) {
      applyOne(m, ev)
      drain(m)
    } else {
      bufPut(m, ev)
    }
  } else if (s === m.next) {
    applyOne(m, ev)
    drain(m)
  } else {
    bufPut(m, ev)
  }

  if (ev.op === 'done' && !m.done) {
    // done 先到但前面缺号：按序补上能补的，然后结束
    m.buf.delete(s)
    flushGaps(m)
    m.done = true
  }
  return m.text !== before || m.done
}

function bufPut(m: CatStreamMsg, ev: CatStreamEvent) {
  m.buf.set(ev.seq!, ev)
  if (m.buf.size > CAT_STREAM_BUF_MAX) {
    const far = Math.max(...m.buf.keys())
    m.buf.delete(far)
  }
}

/** 用户是否要求减少动效（不做打字机效果，直接追加） */
export function prefersReducedMotion(): boolean {
  try {
    return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  } catch {
    return false
  }
}
