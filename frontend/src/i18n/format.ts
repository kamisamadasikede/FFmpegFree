/** 日期 / 数字 / 相对时间，locale 取当前语言（§6.20.2） */
import { currentLocale } from '@/i18n'

export function formatDateTime(ts: number | Date, opts?: Intl.DateTimeFormatOptions): string {
  const d = typeof ts === 'number' ? new Date(ts) : ts
  return new Intl.DateTimeFormat(currentLocale(), opts ?? { dateStyle: 'medium', timeStyle: 'short' }).format(d)
}

export function formatNumber(n: number, opts?: Intl.NumberFormatOptions): string {
  return new Intl.NumberFormat(currentLocale(), opts).format(n)
}

export function formatRelative(ts: number, now = Date.now()): string {
  const diffSec = Math.round((ts - now) / 1000)
  const rtf = new Intl.RelativeTimeFormat(currentLocale(), { numeric: 'auto' })
  const abs = Math.abs(diffSec)
  if (abs < 60) return rtf.format(diffSec, 'second')
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour')
  return rtf.format(Math.round(diffSec / 86400), 'day')
}

/** 文件大小：单位 KB/MB 不翻译 */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(n < 10 * 1024 * 1024 ? 1 : 0)} MB`
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`
}
