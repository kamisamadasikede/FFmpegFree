// md / html 预览的安全显示（契约 6.12.32.4 硬规则 + 6.12.44）：
// 1) DOMPurify 过滤；2) 放进 sandbox 为空的 iframe；3) srcdoc 开头注入严格 CSP，不加载任何网络或本地资源。
// md 渲染时不允许原始 HTML 通过（markdown-it html:false，原始 HTML 被转义成文字）。
// 过滤只作用于显示，绝不写回文件。dompurify / markdown-it 按需加载。
import type { DOMPurify as DOMPurifyT, Config } from 'dompurify'

export const PREVIEW_CSP = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:; form-action 'none'; base-uri 'none'"
/** iframe 的 sandbox 属性值：必须为空（不加 allow-scripts / allow-same-origin / allow-top-navigation / allow-popups / allow-forms） */
export const PREVIEW_SANDBOX = ''

const FORBID_TAGS = ['script', 'iframe', 'frame', 'frameset', 'object', 'embed', 'applet', 'form', 'input', 'button', 'textarea', 'select', 'link', 'meta', 'base', 'noscript', 'template', 'svg', 'math']
const BAD_CSS = /url\s*\([^)]*\)?|@import[^;]*;?|expression\s*\([^)]*\)?|-moz-binding|behavior\s*:/gi

type Purifier = DOMPurifyT

let cached: Purifier | null = null
/** 测试时传入 jsdom 的 window；浏览器里用当前 window */
export async function getPurifier(win?: Window): Promise<Purifier> {
  if (cached && !win) return cached
  const mod = await import('dompurify')
  const factory = (mod.default ?? mod) as unknown as (w?: Window) => Purifier
  const p = factory(win ?? window)
  installHooks(p)
  if (!win) cached = p
  return p
}

function installHooks(p: Purifier) {
  p.addHook('uponSanitizeElement', (node, data) => {
    const el = node as Element
    // 图片只留 data:image/*，其余换成 alt 文字
    if (data.tagName === 'img') {
      const src = (el.getAttribute('src') ?? '').trim()
      if (!/^data:image\//i.test(src)) {
        const alt = el.getAttribute('alt') ?? ''
        const doc = el.ownerDocument
        if (doc && el.parentNode) el.parentNode.replaceChild(doc.createTextNode(alt), el)
        else el.remove()
      }
    }
  })
  p.addHook('afterSanitizeAttributes', (node) => {
    const el = node as Element
    if (!el.getAttribute) return
    // 链接只留文字
    if (el.tagName === 'A') {
      el.removeAttribute('href')
      el.removeAttribute('target')
      el.removeAttribute('ping')
    }
    for (const a of ['src', 'srcset', 'background', 'poster', 'xlink:href', 'action', 'formaction', 'cite', 'longdesc']) {
      const v = el.getAttribute(a)
      if (v == null) continue
      if (el.tagName === 'IMG' && a === 'src' && /^data:image\//i.test(v.trim())) continue
      el.removeAttribute(a)
    }
    const st = el.getAttribute('style')
    if (st && BAD_CSS.test(st)) el.setAttribute('style', st.replace(BAD_CSS, ''))
    BAD_CSS.lastIndex = 0
  })
  p.addHook('uponSanitizeElement', (node, data) => {
    // <style> 元素里的 url( / @import / expression( 去掉
    if (data.tagName === 'style') {
      const el = node as Element
      el.textContent = (el.textContent ?? '').replace(BAD_CSS, '')
      BAD_CSS.lastIndex = 0
    }
  })
}

const CONFIG: Config = {
  WHOLE_DOCUMENT: false,
  FORBID_TAGS,
  FORBID_ATTR: ['srcdoc', 'http-equiv'],
  ALLOW_DATA_ATTR: false,
  ALLOW_UNKNOWN_PROTOCOLS: false,
  // data: 只给图片（hook 里再收紧）；其余协议一律不要
  ALLOWED_URI_REGEXP: /^data:image\//i,
  ADD_TAGS: ['style'],
  // 让开头的 <style> 留在 body 里（否则解析时被挪进 head 丢掉）
  FORCE_BODY: true,
}

/** 整页 html：把 head 里的 <style> 挪到正文前面再过滤（title、meta、script 等都不要） */
export function extractHeadStyles(html: string): string {
  const head = /<head[^>]*>([\s\S]*?)<\/head>/i.exec(html)?.[1] ?? ''
  const styles = head.match(/<style[^>]*>[\s\S]*?<\/style>/gi) ?? []
  const body = /<body[^>]*>([\s\S]*)<\/body>/i.exec(html)?.[1] ?? html.replace(/<head[^>]*>[\s\S]*?<\/head>/i, '')
  return styles.join('') + body
}

export async function sanitizeHtml(html: string, win?: Window): Promise<string> {
  const p = await getPurifier(win)
  return String(p.sanitize(extractHeadStyles(html), CONFIG))
}

let md: { render(src: string): string } | null = null
/** md → 安全 HTML：markdown-it 关掉原始 HTML 和自动链接，再过 DOMPurify */
export async function renderMarkdown(src: string, win?: Window): Promise<string> {
  if (!md) {
    const mod = await import('markdown-it')
    const MarkdownIt = (mod.default ?? mod) as unknown as new (o: Record<string, unknown>) => { render(src: string): string }
    md = new MarkdownIt({ html: false, linkify: false, typographer: false, breaks: false })
  }
  return sanitizeHtml(md.render(src), win)
}

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

/** 给 iframe srcdoc 的整页：CSP meta 必须是第一个子元素；主题色、缩放在这里写成内联样式 */
export function buildSrcdoc(safeBody: string, opts: { dark?: boolean; zoom?: number; title?: string } = {}): string {
  const z = opts.zoom && opts.zoom > 0 ? opts.zoom : 1
  const fg = opts.dark ? '#E6E8EB' : '#1F2329'
  const bg = opts.dark ? '#1E1F22' : '#FFFFFF'
  const muted = opts.dark ? '#2B2D31' : '#F2F3F5'
  const css = `html,body{margin:0;background:${bg};color:${fg}}body{font:14px/1.7 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;padding:24px 32px;zoom:${z}}main{max-width:760px;margin:0 auto;word-wrap:break-word}img{max-width:100%}pre,code{font-family:ui-monospace,Consolas,monospace;background:${muted};border-radius:4px}pre{padding:12px;overflow:auto}code{padding:1px 4px}table{border-collapse:collapse}td,th{border:1px solid ${muted};padding:4px 8px}blockquote{margin:0;padding-left:12px;border-left:3px solid ${muted};opacity:.85}a{color:inherit;text-decoration:underline}`
  return `<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="${PREVIEW_CSP}"><meta charset="utf-8"><title>${esc(opts.title ?? '')}</title><style>${css}</style></head><body><main>${safeBody}</main></body></html>`
}
