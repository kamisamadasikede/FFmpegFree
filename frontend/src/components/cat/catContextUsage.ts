/** 上下文圆环的纯计算（无 Vue）。数字口径与输入框悬停摘要一致：已用 / 容量（百分比）。 */

export function contextUsagePercent(used: number, size: number): number {
  if (!Number.isFinite(used) || !Number.isFinite(size) || size <= 0) return 0
  return Math.min(Math.max(used / size, 0), 1)
}

/** 中文紧凑记法：千以上用万 / 亿，和输入框悬停摘要同一套。 */
export function formatContextTokenCount(value: number, maximumFractionDigits = 1): string {
  if (!Number.isFinite(value)) return ''
  return new Intl.NumberFormat('zh-CN', {
    notation: Math.abs(value) >= 1000 ? 'compact' : 'standard',
    maximumFractionDigits,
    minimumFractionDigits: 0,
  }).format(value)
}

export function formatContextUsageSummary(used: number, size: number): string {
  const pct = new Intl.NumberFormat('zh-CN', {
    maximumFractionDigits: 1,
    style: 'percent',
  }).format(contextUsagePercent(used, size))
  return `${formatContextTokenCount(used)}/${formatContextTokenCount(size, 0)} (${pct})`
}

export const CONTEXT_RING = { radius: 10, viewBox: 24, center: 12, stroke: 4 } as const

export function contextRingDash(used: number, size: number): { array: string; offset: number } {
  const circumference = 2 * Math.PI * CONTEXT_RING.radius
  const offset = circumference * (1 - contextUsagePercent(used, size))
  return { array: `${circumference} ${circumference}`, offset }
}
