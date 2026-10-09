import { contextRingDash, contextUsagePercent, formatContextUsageSummary } from './catContextUsage'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catContextUsageChecks(eq: Eq): void {
  eq('空容量不算占用', contextUsagePercent(10, 0), 0)
  eq('占用不超过一整圈', contextUsagePercent(300, 100), 1)
  const empty = contextRingDash(0, 256000)
  const full = 2 * Math.PI * 10
  eq('没有占用时圆环停在起点', Math.abs(empty.offset - full) < 1e-9, true)
  const some = contextRingDash(128000, 256000)
  eq('用到一半时弧长减半', Math.abs(some.offset - full / 2) < 1e-6, true)
  eq('小数字摘要', formatContextUsageSummary(12, 100), '12/100 (12%)')
  const big = formatContextUsageSummary(155377, 256000)
  eq('大数字摘要带百分比', big.includes('%') && big.includes('/'), true)
  eq('大数字走紧凑记法', big.includes('万'), true)
}
