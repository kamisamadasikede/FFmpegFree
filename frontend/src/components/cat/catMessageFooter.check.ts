import { formatCatFooterCount, formatCatFooterTime, formatCatMessageFooter, pickCatMessageFooter } from './catMessageFooter'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catMessageFooterChecks(eq: Eq): void {
  // 固定本地时刻：用 Date 构造，避免依赖跑检查时的「现在」
  const at = new Date(2026, 9, 13, 22, 15, 0).getTime() // 2026-10-13 22:15 本地
  eq('HH:mm', formatCatFooterTime(at), '22:15')
  eq('无效时间空', formatCatFooterTime(0), '')
  eq('千分位', formatCatFooterCount(12480), '12,480')
  eq('速率取整', formatCatFooterCount(186.4), '186')

  eq('三段', formatCatMessageFooter({ finishedAt: at, tokenTotal: 12480, tokensPerSec: 186 }), '完成于 22:15 · 共 12,480 token · 约 186 token/s')
  eq('仅时间', formatCatMessageFooter({ finishedAt: at }), '完成于 22:15')
  eq('有 token 无速率', formatCatMessageFooter({ finishedAt: at, tokenTotal: 100 }), '完成于 22:15 · 共 100 token')
  eq('无 finishedAt 整段空', formatCatMessageFooter({ tokenTotal: 100, tokensPerSec: 10 }), '')
  eq('速率 0 省略', formatCatMessageFooter({ finishedAt: at, tokenTotal: 100, tokensPerSec: 0 }), '完成于 22:15 · 共 100 token')
  eq('tokenTotal 0 仍算有 usage', formatCatMessageFooter({ finishedAt: at, tokenTotal: 0 }), '完成于 22:15 · 共 0 token')

  eq('pick 终态字段', pickCatMessageFooter({ finishedAt: at, tokenTotal: 12, tokensPerSec: 3.2 }), { finishedAt: at, tokenTotal: 12, tokensPerSec: 3.2 })
  eq('pick 缺省', pickCatMessageFooter({}), {})
  eq('pick 忽略非法', pickCatMessageFooter({ finishedAt: 'x', tokenTotal: null, tokensPerSec: -1 }), {})
}
