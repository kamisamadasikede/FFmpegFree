// Cat 助手回复的 Markdown 渲染（只给助手消息用；用户消息保持纯文字）。
// 和文档预览同一套库：markdown-it + DOMPurify，但规则更严，且是同步渲染（流式时每帧都要出结果）：
// 1) markdown-it html:false —— 原始 HTML 一律转义成文字；
// 2) 链接只认 http / https / mailto，其余（javascript: / data: / file: / 相对路径…）不生成链接，按原文显示；
// 3) 图片不加载：![alt](https://…) 渲染成「图片：alt」外链（点了用系统浏览器打开），不生成 <img>；
// 4) 输出再过一遍 DOMPurify（白名单标签 / 属性 + 链接协议复查），双保险；
// 5) 链接带 rel="noopener noreferrer" 和 data-cat-ext，组件里拦截点击、交给系统浏览器，绝不在窗口内跳转。
// 流式：未闭合的代码块按代码块显示（自动补上结尾）；按顶层块切分，已完成的块结果缓存，不重复过滤。
import MarkdownIt from 'markdown-it'
import createDOMPurify from 'dompurify'
import type { DOMPurify as DOMPurifyT, Config } from 'dompurify'

/** 允许生成链接的协议 */
export const CAT_LINK_RE = /^(?:https?:\/\/|mailto:)/i
export const isCatLinkAllowed = (url: string) => CAT_LINK_RE.test(url.trim())

export const CAT_MD_COPY = '复制'
export const CAT_MD_COPIED = '已复制'
const IMG_PREFIX = '图片：'

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

function createMd(): MarkdownIt {
  const md = new MarkdownIt({ html: false, linkify: true, typographer: false, breaks: true })
  md.linkify.set({ fuzzyLink: false, fuzzyEmail: false, fuzzyIP: false })
  // markdown-it 先规范化再校验：不通过的 [x](url) / ![x](url) 原样当文字
  md.validateLink = (url: string) => isCatLinkAllowed(url)
  const r = md.renderer.rules

  r.link_open = (tokens, idx, options, _env, self) => {
    const t = tokens[idx]
    t.attrSet('rel', 'noopener noreferrer')
    t.attrSet('data-cat-ext', '1')
    return self.renderToken(tokens, idx, options)
  }
  // 图片：不出 <img>，只出一个外链（alt 文字）
  r.image = (tokens, idx) => {
    const t = tokens[idx]
    const src = t.attrGet('src') ?? ''
    const alt = t.content || src
    if (!isCatLinkAllowed(src)) return esc(alt)
    return `<a class="cat-md-img" href="${esc(src)}" rel="noopener noreferrer" data-cat-ext="1">${esc(IMG_PREFIX + alt)}</a>`
  }
  const code = (body: string, lang: string) =>
    `<div class="cat-md-code"><div class="cat-md-bar"><span class="cat-md-lang">${esc(lang)}</span>` +
    `<button type="button" class="cat-md-copy" data-cat-copy="1" aria-label="复制代码">${CAT_MD_COPY}</button></div>` +
    `<pre><code>${esc(body)}</code></pre></div>`
  r.fence = (tokens, idx) => {
    const t = tokens[idx]
    const lang = (t.info ?? '').trim().split(/\s+/)[0] ?? ''
    return code(t.content, lang.slice(0, 24))
  }
  r.code_block = (tokens, idx) => code(tokens[idx].content, '')
  r.table_open = () => '<div class="cat-md-table"><table>\n'
  r.table_close = () => '</table></div>\n'
  return md
}

let mdInst: MarkdownIt | null = null
const getMd = () => (mdInst ??= createMd())

// ---------------- 流式预处理（纯字符串，可在 node 里测） ----------------

const FENCE_OPEN = /^\s*(`{3,}|~{3,})(.*)$/

/**
 * 流式中的文字整理（streaming=false 原样返回）：
 * - 未闭合的代码块：末尾补上结束围栏，按代码块渲染（不会先当成段落再跳成代码块）；
 * - 最后一行还没写完、而且只是半截围栏（` 或 ``）：先不显示，避免闪一下；
 * - 最后一行还没写完、像表格行（以 | 开头）：先不显示，等这一行写完（表格不会先变段落再跳成表格）。
 */
export function prepareStreamingSource(src: string, streaming: boolean): string {
  if (!streaming || !src) return src
  const lines = src.split('\n')
  const lastComplete = src.endsWith('\n')
  let open: { ch: string; len: number } | null = null
  const n = lastComplete ? lines.length : lines.length - 1
  for (let i = 0; i < n; i++) {
    const ln = lines[i]
    if (!open) {
      const m = FENCE_OPEN.exec(ln)
      if (m && !(m[1][0] === '`' && m[2].includes('`'))) open = { ch: m[1][0], len: m[1].length }
    } else {
      const m = /^\s*(`{3,}|~{3,})\s*$/.exec(ln)
      if (m && m[1][0] === open.ch && m[1].length >= open.len) open = null
    }
  }
  let body = src
  if (!lastComplete) {
    const tail = lines[lines.length - 1]
    const head = lines.slice(0, -1).join('\n')
    let drop = false
    if (open) {
      // 结束围栏写了一半（或刚好写完但还没换行）
      const m = /^\s*(`+|~+)\s*$/.exec(tail)
      if (m && m[1][0] === open.ch) drop = true
    } else {
      if (/^\s*(`{1,2}|~{1,2})$/.test(tail)) drop = true // 开始围栏写了一半
      else if (/^\s*\|/.test(tail)) drop = true // 表格行写了一半
      else {
        const m = FENCE_OPEN.exec(tail) // 开始围栏所在行还没换行：已经算进代码块
        if (m && !(m[1][0] === '`' && m[2].includes('`'))) open = { ch: m[1][0], len: m[1].length }
      }
    }
    if (drop) body = head.length ? head + '\n' : ''
  }
  if (open) body = (body.endsWith('\n') || !body ? body : body + '\n') + open.ch.repeat(open.len) + '\n'
  return body
}

// ---------------- markdown → 分块 HTML ----------------

export interface CatMdBlock {
  key: number
  html: string
}

/** 只做 markdown-it 渲染（未过滤），按顶层块切开 */
export function renderCatMarkdownRaw(text: string, streaming = false): string[] {
  const md = getMd()
  const src = prepareStreamingSource(text ?? '', streaming)
  const env = {}
  const tokens = md.parse(src, env)
  const out: string[] = []
  let start = 0
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i]
    if (t.level === 0 && t.nesting !== 1) {
      out.push(md.renderer.render(tokens.slice(start, i + 1), md.options, env))
      start = i + 1
    }
  }
  if (start < tokens.length) out.push(md.renderer.render(tokens.slice(start), md.options, env))
  return out
}

// ---------------- DOMPurify（第二道） ----------------

const ALLOWED_TAGS = ['p', 'br', 'hr', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'ul', 'ol', 'li', 'strong', 'em', 's', 'del', 'code', 'pre', 'blockquote', 'a', 'table', 'thead', 'tbody', 'tr', 'th', 'td', 'div', 'span', 'button']
const ALLOWED_ATTR = ['href', 'rel', 'class', 'start', 'style', 'type', 'aria-label', 'data-cat-ext', 'data-cat-copy']
const PURIFY_CONFIG: Config = {
  ALLOWED_TAGS,
  ALLOWED_ATTR,
  ALLOW_DATA_ATTR: false,
  ALLOW_UNKNOWN_PROTOCOLS: false,
  // 链接协议白名单在 afterSanitizeAttributes 钩子里按 CAT_LINK_RE 收紧（ALLOWED_URI_REGEXP 会连带校验 type 等普通属性，不能用它）
  WHOLE_DOCUMENT: false,
}

function installHooks(p: DOMPurifyT) {
  p.addHook('afterSanitizeAttributes', (node) => {
    const el = node as Element
    if (!el.getAttribute) return
    if (el.tagName === 'A') {
      const href = el.getAttribute('href') ?? ''
      if (href && isCatLinkAllowed(href)) {
        el.setAttribute('rel', 'noopener noreferrer')
        el.setAttribute('data-cat-ext', '1')
      } else {
        el.removeAttribute('href')
        el.removeAttribute('data-cat-ext')
      }
      el.removeAttribute('target')
    } else el.removeAttribute('data-cat-ext')
    if (el.tagName !== 'BUTTON') el.removeAttribute('data-cat-copy')
    // markdown-it 只会给表格单元格写 text-align，其余样式一律不要
    const st = el.getAttribute('style')
    if (st != null) {
      const m = /^\s*text-align\s*:\s*(left|right|center)\s*;?\s*$/i.exec(st)
      if (m && (el.tagName === 'TH' || el.tagName === 'TD')) el.setAttribute('style', `text-align:${m[1].toLowerCase()}`)
      else el.removeAttribute('style')
    }
  })
}

let purifier: DOMPurifyT | null = null
/** 测试时传入 jsdom 的 window（会替换缓存）；浏览器里用当前 window */
export function getCatPurifier(win?: Window): DOMPurifyT {
  if (purifier && !win) return purifier
  const factory = createDOMPurify as unknown as (w?: Window) => DOMPurifyT
  const p = factory(win ?? window)
  installHooks(p)
  purifier = p
  sanitizeCache.clear()
  return p
}

const sanitizeCache = new Map<string, string>()
const CACHE_MAX = 400

/** 单块过滤（结果按输入缓存：流式时已完成的块不会重复过滤） */
export function sanitizeCatHtml(html: string): string {
  const hit = sanitizeCache.get(html)
  if (hit !== undefined) return hit
  const out = String(getCatPurifier().sanitize(html, PURIFY_CONFIG))
  if (sanitizeCache.size >= CACHE_MAX) sanitizeCache.delete(sanitizeCache.keys().next().value as string)
  sanitizeCache.set(html, out)
  return out
}

/** 助手消息 → 安全的分块 HTML（key 为块序号，用来按块增量更新） */
export function renderCatMarkdown(text: string, streaming = false): CatMdBlock[] {
  return renderCatMarkdownRaw(text, streaming).map((h, key) => ({ key, html: sanitizeCatHtml(h) }))
}
