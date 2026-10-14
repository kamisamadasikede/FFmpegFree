import { t } from '@/i18n'
/**
 * Cat 消息尾文案（契约 v0.45 §6.19.19 / v0.47 §6.19.19.5，产品 / 设计 10-14 定稿）。
 * 产品改字只改这里。
 */
export const CAT_FOOTER_COPY = {
  get sep() { return t('cat.footer.sep') },
  finishedAt: (hhmm: string) => t('cat.footer.finishedAt', { v0: hhmm }),
  firstToken: (d: string) => t('cat.footer.firstToken', { v0: d }),
  total: (d: string) => t('cat.footer.total', { v0: d }),
  tokens: (n: string) => t('cat.footer.tokens', { n: n }),
  output: (n: string) => t('cat.footer.output', { n: n }),
  rate: (n: string) => t('cat.footer.rate', { n: n }),
  secDecimal: (s: string) => t('cat.footer.secDecimal', { s: s }),
  secInt: (s: number) => t('cat.footer.secInt', { s: s }),
  minSec: (m: number, ss: string) => t('cat.footer.minSec', { m: m, v1: ss }),
  get hintFirstToken() { return t('cat.footer.hintFirstToken') },
  get hintTotal() { return t('cat.footer.hintTotal') },
  hintRate: (n: string) => t('cat.footer.hintRate', { n: n }),
}
