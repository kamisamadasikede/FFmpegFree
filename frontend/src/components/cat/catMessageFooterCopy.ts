/**
 * Cat 消息尾文案（契约 v0.45 §6.19.19 / v0.47 §6.19.19.5，产品 / 设计 10-14 定稿）。
 * 产品改字只改这里。
 */
export const CAT_FOOTER_COPY = {
  sep: ' · ',
  finishedAt: (hhmm: string) => `完成于 ${hhmm}`,
  firstToken: (d: string) => `首字 ${d}`,
  total: (d: string) => `总耗时 ${d}`,
  tokens: (n: string) => `共 ${n} token`,
  /** v0.47 §6.19.19.5 */
  output: (n: string) => `输出 ${n} token`,
  rate: (n: string) => `约 ${n} token/s`,
  /** 时长：<10s 一位小数；10–59s 整秒；≥60s「1分05秒」 */
  secDecimal: (s: string) => `${s}s`,
  secInt: (s: number) => `${s}s`,
  minSec: (m: number, ss: string) => `${m}分${ss}秒`,
  /** 悬停提示（窄宽度时连同被隐藏项一起，一行一项） */
  hintFirstToken: '首字：从发送到看到第一个字',
  hintTotal: '总耗时：从发送到回复结束',
  /** v0.47：有 token/s 项才带 */
  hintRate: (n: string) => `约 ${n} token/s：按输出 token 计算`,
} as const
