/**
 * 契约 v0.45：assistant CatMessage / 终态 cat:turn 的 firstTokenMs、totalMs。
 * 字段名 / 类型取自生成的 store.CatMessage；cat:turn 事件载荷无生成类型，同名字段在这里一并读。
 */
import type { store } from '../../../wailsjs/go/models'

export type CatTimingFields = Partial<Pick<store.CatMessage, 'firstTokenMs' | 'totalMs'>>

export const CAT_TIMING_FIELDS = { firstTokenMs: 'firstTokenMs', totalMs: 'totalMs' } as const satisfies Record<keyof CatTimingFields, keyof store.CatMessage>

function ms(v: unknown): number | undefined {
  const n = typeof v === 'number' ? v : typeof v === 'string' && v.trim() ? Number(v) : NaN
  return Number.isFinite(n) && n >= 0 ? n : undefined
}

/** 缺省 / 非法 → 不带（不补假数据） */
export function pickCatTiming(raw: unknown): CatTimingFields {
  if (!raw || typeof raw !== 'object') return {}
  const r = raw as Record<string, unknown>
  const out: CatTimingFields = {}
  const f = ms(r[CAT_TIMING_FIELDS.firstTokenMs])
  const t = ms(r[CAT_TIMING_FIELDS.totalMs])
  if (f !== undefined) out.firstTokenMs = f
  if (t !== undefined) out.totalMs = t
  return out
}

/**
 * v0.47 §6.19.19.5：`outputTokens`（assistant CatMessage + 终态 cat:turn）。
 * 后端 bindings 未生成前先按契约字段名手写；缺省 / 非法 / ≤0 → 不带。
 * 不带 outputTokens 时 tokensPerSec 也丢掉（契约：两者同进退，不得用 tokenTotal 顶替）。
 */
export type CatOutputFields = { outputTokens?: number; tokensPerSec?: number }
export const CAT_OUTPUT_FIELDS = { outputTokens: 'outputTokens', tokensPerSec: 'tokensPerSec' } as const

function pos(v: unknown): number | undefined {
  const n = typeof v === 'number' ? v : typeof v === 'string' && v.trim() ? Number(v) : NaN
  return Number.isFinite(n) && n > 0 ? n : undefined
}

export function pickCatOutput(raw: unknown): CatOutputFields {
  if (!raw || typeof raw !== 'object') return {}
  const r = raw as Record<string, unknown>
  const o = pos(r[CAT_OUTPUT_FIELDS.outputTokens])
  if (o === undefined) return {}
  const out: CatOutputFields = { outputTokens: o }
  const rate = pos(r[CAT_OUTPUT_FIELDS.tokensPerSec])
  if (rate !== undefined) out.tokensPerSec = rate
  return out
}

export function applyCatOutput(target: CatOutputFields, src: CatOutputFields): void {
  if (src.outputTokens !== undefined) target.outputTokens = src.outputTokens
  else delete target.outputTokens
  if (src.outputTokens !== undefined && src.tokensPerSec !== undefined) target.tokensPerSec = src.tokensPerSec
  else delete target.tokensPerSec
}

/** 把计时字段写到目标上；缺的删掉（重试 / 新一轮不残留旧值） */
export function applyCatTiming(target: CatTimingFields, src: CatTimingFields): void {
  if (src.firstTokenMs !== undefined) target.firstTokenMs = src.firstTokenMs
  else delete target.firstTokenMs
  if (src.totalMs !== undefined) target.totalMs = src.totalMs
  else delete target.totalMs
}
