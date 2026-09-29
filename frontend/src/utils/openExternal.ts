import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { hasWailsBackend } from '@/services/wails'

/**
 * 用系统浏览器打开链接。Wails 里走 BrowserOpenURL；纯浏览器开发环境（没有 window.runtime）用 window.open 兜底。
 * 只放行 http(s)，不要用 <a target="_blank">（会在 webview 内打开）。
 */
export function openExternal(url: string): void {
  if (!/^https?:\/\//i.test(url)) return
  if (hasWailsBackend()) BrowserOpenURL(url)
  else window.open(url, '_blank', 'noopener,noreferrer')
}
