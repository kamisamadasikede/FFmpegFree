/**
 * Cat 消息尾（契约 v0.37.1 / 设计说明 v0.1.1）：
 * `完成于 HH:mm · 首字 · 总耗时 · 共 N token · 输出 N token · 约 M token/s`（v0.47）
 * token/s 只显示后端给的值，前端不算；无 outputTokens 则「输出」「token/s」都不显示。
 * 仅格式化；有无字段由调用方决定（无 finishedAt 整段不显示）。
 */

import { CAT_FOOTER_COPY as C } from './catMessageFooterCopy'
import { pickCatOutput, pickCatTiming } from './catTimingAdapter'

export type CatMessageFooterFields = {
  finishedAt?: number
  /** v0.45 */
  firstTokenMs?: number
  totalMs?: number
  tokenTotal?: number
  /** v0.47：本轮输出 token；缺省则不显示「输出」与 token/s */
  outputTokens?: number
  /** 后端算好的（v0.47 按输出 token）；前端只格式化 */
  tokensPerSec?: number
}

/** 本地时区 24h `HH:mm`（与 Cat 其它时间展示一致：用本机 Date） */
export function formatCatFooterTime(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return ''
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 整数千分位（例 12480 → `12,480`）；非有限数返回空 */
export function formatCatFooterCount(n: number): string {
  if (!Number.isFinite(n)) return ''
  return Math.round(n).toLocaleString('en-US')
}

/** v0.45 时长：<10s 一位小数；10–59s 整秒；≥60s「1分05秒」 */
export function formatCatDuration(msIn: number): string {
  if (!Number.isFinite(msIn) || msIn < 0) return ''
  const s = msIn / 1000
  const d1 = Math.round(s * 10) / 10
  if (d1 < 10) return C.secDecimal(d1.toFixed(1))
  const whole = Math.round(s)
  if (whole < 60) return C.secInt(whole)
  return C.minSec(Math.floor(whole / 60), String(whole % 60).padStart(2, '0'))
}

/** 消息尾一段；key 也是窄宽度隐藏顺序的依据 */
export type CatFooterPart = { key: 'finishedAt' | 'firstToken' | 'total' | 'tokens' | 'output' | 'rate'; text: string }

/** 窄宽度时依次隐藏（v0.47）：速率 → 输出 → 共 → 首字 → 完成于 → 总耗时（最后） */
export const CAT_FOOTER_HIDE_ORDER: readonly CatFooterPart['key'][] = ['rate', 'output', 'tokens', 'firstToken', 'finishedAt', 'total']

export function catMessageFooterParts(f: CatMessageFooterFields): CatFooterPart[] {
  const finishedAt = typeof f.finishedAt === 'number' && Number.isFinite(f.finishedAt) && f.finishedAt > 0 ? f.finishedAt : 0
  if (!finishedAt) return []
  const parts: CatFooterPart[] = [{ key: 'finishedAt', text: C.finishedAt(formatCatFooterTime(finishedAt)) }]
  const ft = typeof f.firstTokenMs === 'number' ? formatCatDuration(f.firstTokenMs) : ''
  if (ft) parts.push({ key: 'firstToken', text: C.firstToken(ft) })
  const tt = typeof f.totalMs === 'number' ? formatCatDuration(f.totalMs) : ''
  if (tt) parts.push({ key: 'total', text: C.total(tt) })
  if (typeof f.tokenTotal === 'number' && Number.isFinite(f.tokenTotal)) {
    parts.push({ key: 'tokens', text: C.tokens(formatCatFooterCount(f.tokenTotal)) })
  }
  // v0.47：「输出」与 token/s 只看 outputTokens；绝不拿 tokenTotal 顶替
  const out = typeof f.outputTokens === 'number' && Number.isFinite(f.outputTokens) && f.outputTokens > 0 ? f.outputTokens : 0
  if (out) {
    parts.push({ key: 'output', text: C.output(formatCatFooterCount(out)) })
    const rate = typeof f.tokensPerSec === 'number' && Number.isFinite(f.tokensPerSec) && f.tokensPerSec > 0 ? f.tokensPerSec : 0
    if (rate) parts.push({ key: 'rate', text: C.rate(formatCatFooterCount(rate)) })
  }
  return parts
}

/** 去掉 n 段（按隐藏顺序），返回 [显示, 隐藏(按原顺序)] */
export function catFooterSplit(parts: CatFooterPart[], hideCount: number): [CatFooterPart[], CatFooterPart[]] {
  const hide = new Set<string>()
  for (const k of CAT_FOOTER_HIDE_ORDER) {
    if (hide.size >= hideCount) break
    if (parts.some((p) => p.key === k)) hide.add(k)
  }
  return [parts.filter((p) => !hide.has(p.key)), parts.filter((p) => hide.has(p.key))]
}

/** 悬停提示：被隐藏的项 + 说明（首字 / 总耗时 / v0.47 token/s）（有对应项才带），一行一项。
 * 速率被窄窗隐藏时不重复列「约 N token/s」，只留说明行「约 N token/s：按输出 token 计算」（产品 10-14）。 */
export function catFooterTooltip(parts: CatFooterPart[], hidden: CatFooterPart[]): string {
  const lines = hidden.filter((p) => p.key !== 'rate').map((p) => p.text)
  if (parts.some((p) => p.key === 'firstToken')) lines.push(C.hintFirstToken)
  if (parts.some((p) => p.key === 'total')) lines.push(C.hintTotal)
  const rate = parts.find((p) => p.key === 'rate')
  if (rate) lines.push(C.hintRate(rate.text.replace(/^约 (.+) token\/s$/, '$1')))
  return lines.join('\n')
}

/**
 * 由字段拼一行文案。无 `finishedAt` → 空串（不显示尾巴）。
 * 有 `tokenTotal` 才拼「共」；有 `outputTokens`(>0) 才拼「输出」，再有后端 `tokensPerSec`(>0) 才拼速率。
 */
export function formatCatMessageFooter(f: CatMessageFooterFields): string {
  return catMessageFooterParts(f).map((p) => p.text).join(C.sep)
}

/** 从原始 JSON 抽出消息尾字段（缺省 / 非法 → 不带） */
export function pickCatMessageFooter(raw: Record<string, unknown> | null | undefined): CatMessageFooterFields {
  if (!raw) return {}
  const out: CatMessageFooterFields = {}
  const finishedAt = numOrUndef(raw.finishedAt)
  if (finishedAt !== undefined && finishedAt > 0) out.finishedAt = finishedAt
  if ('tokenTotal' in raw) {
    const t = numOrUndef(raw.tokenTotal)
    if (t !== undefined) out.tokenTotal = t
  }
  Object.assign(out, pickCatOutput(raw), pickCatTiming(raw))
  return out
}

function numOrUndef(v: unknown): number | undefined {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof v === 'string' && v.trim()) {
    const n = Number(v)
    if (Number.isFinite(n)) return n
  }
  return undefined
}
