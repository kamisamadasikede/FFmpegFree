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

/**
 * Cat 回复里的链接：http(s) 同 openExternal；另外放行 mailto:（交给系统邮件程序）。其余协议一律不打开。
 * 不在 webview 里跳转。
 */
export function openExternalLink(url: string): boolean {
  const u = url.trim()
  if (/^https?:\/\//i.test(u)) {
    openExternal(u)
    return true
  }
  if (/^mailto:/i.test(u)) {
    if (hasWailsBackend()) BrowserOpenURL(u)
    else window.open(u, '_blank', 'noopener,noreferrer')
    return true
  }
  return false
}
