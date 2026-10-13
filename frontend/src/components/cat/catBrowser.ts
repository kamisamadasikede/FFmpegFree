import { withPickScript } from './catPick'

export const BROWSER_NAV = 'ffmpegfree-cat-nav'

/** 把地址收成可以嵌进页面的 http(s) 网址。不对就返回空。 */
export function browserURL(raw: string): string {
  const t = raw.trim()
  if (!t) return ''
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(t) ? t : 'http://' + t
  let u: URL
  try {
    u = new URL(withScheme)
  } catch {
    return ''
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return ''
  if (!u.hostname) return ''
  return u.toString()
}

/** 地址栏：像网址就打开，纯文字去谷歌。脚本、文件这些协议丢掉。 */
export function browserOpen(raw: string): string {
  const t = raw.trim()
  if (!t) return ''
  const search = 'https://www.google.com/search?q=' + encodeURIComponent(t)
  if (/\s/.test(t)) return search
  if (/^(javascript|data|file|vbscript|mailto):/i.test(t)) return ''
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(t)) return browserURL(t)
  const direct = browserURL(t)
  if (direct) {
    const host = new URL(direct).hostname
    const realHost = host.includes('.') || host.includes(':') || host === 'localhost' || /^\d{1,3}(\.\d{1,3}){3}$/.test(host)
    if (realHost) return direct
  }
  return search
}

function escapeAttr(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
}

/** 让页面里的相对地址回到原来的网站。已经有基准地址就不动。 */
export function withBase(html: string, pageURL: string): string {
  if (/<base\b/i.test(html)) return html
  const tag = '<base href="' + escapeAttr(pageURL) + '">'
  const m = /<head\b[^>]*>/i.exec(html)
  if (!m) return tag + html
  const at = m.index + m[0].length
  return html.slice(0, at) + tag + html.slice(at)
}

export function stripCSP(html: string): string {
  return html.replace(/<meta\b[^>]*http-equiv\s*=\s*["']?\s*content-security-policy\b[^>]*>/gi, '')
}

/** 点链接和提交表单时告诉外面换页，这样点选脚本还在。 */
export function navRuntime(token: string): string {
  const tok = JSON.stringify(token)
  const src = JSON.stringify(BROWSER_NAV)
  return `(() => {
  var TOKEN = ${tok};
  var SRC = ${src};
  function abs(h) {
    try { return new URL(h, document.baseURI).href } catch (e) { return '' }
  }
  function send(href) {
    if (!href || !/^https?:/i.test(href)) return;
    parent.postMessage({ source: SRC, token: TOKEN, kind: 'open', href: href }, '*');
  }
  document.addEventListener('click', function (e) {
    if (e.button != null && e.button !== 0) return;
    var n = e.target;
    if (n && n.nodeType === 3) n = n.parentElement;
    while (n && n.tagName !== 'A') n = n.parentElement;
    if (!n) return;
    var href = n.getAttribute('href') || '';
    if (!href || href.charAt(0) === '#') return;
    var next = abs(href);
    if (!next) return;
    e.preventDefault();
    e.stopPropagation();
    send(next);
  }, true);
  document.addEventListener('submit', function (e) {
    var f = e.target;
    if (!f || f.tagName !== 'FORM') return;
    e.preventDefault();
    var action = f.getAttribute('action') || document.baseURI;
    var next = abs(action);
    var method = (f.getAttribute('method') || 'get').toLowerCase();
    if (method === 'get') {
      try {
        var u = new URL(next);
        var els = f.elements || [];
        for (var i = 0; i < els.length; i++) {
          var el = els[i];
          if (!el.name || el.disabled) continue;
          u.searchParams.append(el.name, el.value || '');
        }
        next = u.href;
      } catch (err) {}
    }
    send(next);
  }, true);
})();`
}

export function withNavScript(html: string, token: string): string {
  if (!/^[a-z0-9]+$/i.test(token)) return html
  const tag = '<script data-cat-nav="1">' + navRuntime(token) + '</scr' + 'ipt>'
  const i = html.toLowerCase().lastIndexOf('</body>')
  if (i >= 0) return html.slice(0, i) + tag + html.slice(i)
  return html + tag
}

/** 嵌进浏览器的页面：基准地址、点选、链接改由外面打开。 */
export function browserDocument(html: string, pageURL: string, token: string): string {
  return withNavScript(withPickScript(stripCSP(withBase(html, pageURL)), token), token)
}

export function readBrowserNav(data: unknown, token: string): string {
  if (!data || typeof data !== 'object' || !token) return ''
  const d = data as Record<string, unknown>
  if (d.source !== BROWSER_NAV || d.token !== token || d.kind !== 'open') return ''
  return typeof d.href === 'string' ? browserURL(d.href) : ''
}
