/**
 * Cat 消息尾（契约 v0.37.1 / 设计说明 v0.1.1）：
 * `完成于 HH:mm · 共 N token · 约 M token/s`
 * 仅格式化；有无字段由调用方决定（无 finishedAt 整段不显示）。
 */

import { CAT_FOOTER_COPY as C } from './catMessageFooterCopy'
import { pickCatTiming } from './catTimingAdapter'

export type CatMessageFooterFields = {
  finishedAt?: number
  /** v0.45 */
  firstTokenMs?: number
  totalMs?: number
  tokenTotal?: number
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
export type CatFooterPart = { key: 'finishedAt' | 'firstToken' | 'total' | 'tokens' | 'rate'; text: string }

/** 窄宽度时依次隐藏：速率 → token → 首字 → 完成于 → 总耗时（最后） */
export const CAT_FOOTER_HIDE_ORDER: readonly CatFooterPart['key'][] = ['rate', 'tokens', 'firstToken', 'finishedAt', 'total']

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

/** 悬停提示：被隐藏的项 + 两行说明（有对应项才带），一行一项 */
export function catFooterTooltip(parts: CatFooterPart[], hidden: CatFooterPart[]): string {
  const lines = hidden.map((p) => p.text)
  if (parts.some((p) => p.key === 'firstToken')) lines.push(C.hintFirstToken)
  if (parts.some((p) => p.key === 'total')) lines.push(C.hintTotal)
  return lines.join('\n')
}

/**
 * 由字段拼一行文案。无 `finishedAt` → 空串（不显示尾巴）。
 * 有 usage（有限 `tokenTotal`）才拼 token 段；有限且 >0 的 `tokensPerSec` 才拼速率段。
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
  if ('tokensPerSec' in raw) {
    const r = numOrUndef(raw.tokensPerSec)
    if (r !== undefined && r > 0) out.tokensPerSec = r
  }
  Object.assign(out, pickCatTiming(raw))
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
