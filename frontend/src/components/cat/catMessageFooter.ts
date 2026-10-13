/**
 * Cat 消息尾（契约 v0.37.1 / 设计说明 v0.1.1）：
 * `完成于 HH:mm · 共 N token · 约 M token/s`
 * 仅格式化；有无字段由调用方决定（无 finishedAt 整段不显示）。
 */

export type CatMessageFooterFields = {
  finishedAt?: number
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

/**
 * 由字段拼一行文案。无 `finishedAt` → 空串（不显示尾巴）。
 * 有 usage（有限 `tokenTotal`）才拼 token 段；有限且 >0 的 `tokensPerSec` 才拼速率段。
 */
export function formatCatMessageFooter(f: CatMessageFooterFields): string {
  const finishedAt = typeof f.finishedAt === 'number' && Number.isFinite(f.finishedAt) && f.finishedAt > 0 ? f.finishedAt : 0
  if (!finishedAt) return ''
  const parts = [`完成于 ${formatCatFooterTime(finishedAt)}`]
  const hasUsage = typeof f.tokenTotal === 'number' && Number.isFinite(f.tokenTotal)
  if (hasUsage) {
    parts.push(`共 ${formatCatFooterCount(f.tokenTotal!)} token`)
    const rate = typeof f.tokensPerSec === 'number' && Number.isFinite(f.tokensPerSec) && f.tokensPerSec > 0 ? f.tokensPerSec : 0
    if (rate) parts.push(`约 ${formatCatFooterCount(rate)} token/s`)
  }
  return parts.join(' · ')
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
