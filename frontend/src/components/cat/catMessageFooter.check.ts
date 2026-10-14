import { catFooterSplit, catFooterTooltip, catMessageFooterParts, formatCatDuration, formatCatFooterCount, formatCatFooterTime, formatCatMessageFooter, pickCatMessageFooter } from './catMessageFooter'

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

  // v0.45 时长格式
  eq('时长 1.2s', formatCatDuration(1200), '1.2s')
  eq('时长 0', formatCatDuration(0), '0.0s')
  eq('时长 9.94s', formatCatDuration(9940), '9.9s')
  eq('时长 9.96s 进位到整秒', formatCatDuration(9960), '10s')
  eq('时长 10s', formatCatDuration(10000), '10s')
  eq('时长 59.4s', formatCatDuration(59400), '59s')
  eq('时长 59.6s 进位到分', formatCatDuration(59600), '1分00秒')
  eq('时长 65s', formatCatDuration(65000), '1分05秒')
  eq('时长 125.4s', formatCatDuration(125400), '2分05秒')
  eq('时长负数空', formatCatDuration(-1), '')
  eq('时长 NaN 空', formatCatDuration(NaN), '')

  const full = { finishedAt: at, firstTokenMs: 1200, totalMs: 8400, tokenTotal: 12345, tokensPerSec: 85 }
  eq('v0.45 全量', formatCatMessageFooter(full), '完成于 22:15 · 首字 1.2s · 总耗时 8.4s · 共 12,345 token · 约 85 token/s')
  eq('无首字（停止 / 失败）', formatCatMessageFooter({ finishedAt: at, totalMs: 3600 }), '完成于 22:15 · 总耗时 3.6s')
  eq('只有首字', formatCatMessageFooter({ finishedAt: at, firstTokenMs: 800 }), '完成于 22:15 · 首字 0.8s')
  eq('长耗时', formatCatMessageFooter({ finishedAt: at, firstTokenMs: 12300, totalMs: 65000 }), '完成于 22:15 · 首字 12s · 总耗时 1分05秒')
  eq('pick 计时', pickCatMessageFooter({ finishedAt: at, firstTokenMs: 1200, totalMs: '8400' }), { finishedAt: at, firstTokenMs: 1200, totalMs: 8400 })
  eq('pick 计时非法丢弃', pickCatMessageFooter({ finishedAt: at, firstTokenMs: -5, totalMs: 'x' }), { finishedAt: at })

  // 窄宽度隐藏顺序：速率 → token → 首字 → 完成于 → 总耗时
  const parts = catMessageFooterParts(full)
  const keys = (n: number) => catFooterSplit(parts, n)[0].map((p) => p.key)
  eq('隐藏 1', keys(1), ['finishedAt', 'firstToken', 'total', 'tokens'])
  eq('隐藏 2', keys(2), ['finishedAt', 'firstToken', 'total'])
  eq('隐藏 3', keys(3), ['finishedAt', 'total'])
  eq('隐藏 4', keys(4), ['total'])
  eq('无速率时隐藏 1 = 去 token', catFooterSplit(catMessageFooterParts({ finishedAt: at, totalMs: 8400, tokenTotal: 5 }), 1)[0].map((p) => p.key), ['finishedAt', 'total'])
  eq('提示：不隐藏只有说明', catFooterTooltip(parts, []), '首字：从发送到看到第一个字\n总耗时：从发送到回复结束')
  eq('提示：隐藏项在前，一行一项', catFooterTooltip(parts, catFooterSplit(parts, 2)[1]), '共 12,345 token\n约 85 token/s\n首字：从发送到看到第一个字\n总耗时：从发送到回复结束')
  eq('提示：无计时无说明', catFooterTooltip(catMessageFooterParts({ finishedAt: at }), []), '')
}
