
import { t } from '@/i18n'/**
 * 网页预览里点选元素，把说明放进输入框。
 * 预览框不能读页面内部，所以把一段脚本放进页面，由它把元素说明传出来。
 * 这段脚本不改磁盘上的文件。
 */

export const PICK_SOURCE = 'ffmpegfree-cat-pick'
export const PICK_START = t('cat.misc.s_d2d0e41a2c')
export const PICK_END = t('cat.misc.s_82fd7a76a9')

export interface PickBox {
  left: number
  top: number
  width: number
  height: number
}

export interface PickSelect {
  kind: 'select'
  tag: string
  id: string
  classes: string
  selector: string
  text: string
  styles: string
  snippet: string
  box: PickBox
}

/** 加进输入框的一条注释。输入框里只显示气泡，发送时再写进正文。 */
export interface PickNote {
  file: string
  tag: string
  id: string
  classes: string
  selector: string
  text: string
  styles: string
  snippet: string
  intent: 'edit' | 'ask'
  note: string
}

export type PickEvent = { kind: 'ready' } | { kind: 'esc' } | { kind: 'move'; box: PickBox } | PickSelect

export function newPickToken(): string {
  let s = ''
  for (let i = 0; i < 16; i++) s += Math.floor(Math.random() * 36).toString(36)
  return s + Date.now().toString(36)
}

function clip(v: unknown, n: number): string {
  if (typeof v !== 'string') return ''
  const s = v.replace(/\u0000/g, '').trim()
  return s.length > n ? s.slice(0, n) + '…' : s
}

function num(v: unknown): number {
  if (typeof v !== 'number' || !Number.isFinite(v)) return 0
  if (v > 100000) return 100000
  if (v < -100000) return -100000
  return Math.round(v)
}

function boxOf(d: Record<string, unknown>): PickBox {
  return { left: num(d.left), top: num(d.top), width: num(d.width), height: num(d.height) }
}

/** 只认这次预览发出的说明。过长的字段截断，避免把整页塞进输入框。 */
export function readPickEvent(data: unknown, token: string): PickEvent | null {
  if (!data || typeof data !== 'object') return null
  const d = data as Record<string, unknown>
  if (d.source !== PICK_SOURCE || d.token !== token || typeof token !== 'string' || !token) return null
  if (d.kind === 'ready') return { kind: 'ready' }
  if (d.kind === 'esc') return { kind: 'esc' }
  if (d.kind === 'move') return { kind: 'move', box: boxOf(d) }
  if (d.kind !== 'select') return null
  const tag = clip(d.tag, 40).toLowerCase().replace(/[^a-z0-9-]/g, '')
  if (!tag) return null
  return {
    kind: 'select',
    tag,
    id: clip(d.id, 80),
    classes: clip(d.classes, 120),
    selector: clip(d.selector, 300),
    text: clip(d.text, 120),
    styles: clip(d.styles, 400),
    snippet: clip(d.snippet, 600),
    box: boxOf(d),
  }
}

/** 弹窗和气泡上的短标题：优先用元素里的文字。 */
export function pickLabel(p: { tag: string; id: string; classes: string; text: string }): string {
  const text = p.text.replace(/\s+/g, ' ').trim()
  if (text) return text.length > 28 ? text.slice(0, 28) + '…' : text
  const classes = p.classes.split(/\s+/).filter(Boolean).slice(0, 2)
  const name = p.tag + (p.id ? '#' + p.id : '') + (classes.length ? '.' + classes.join('.') : '')
  return name || t('cat.misc.s_eeb2c63ecd')
}

/** 弹窗放在元素右边；右边不够就放到左边，底下不够就上移。 */
export function placePickPop(anchor: PickBox, pop: { width: number; height: number }, view: { width: number; height: number }, gap = 8): { left: number; top: number } {
  const m = 8
  let left = anchor.left + anchor.width + gap
  let top = anchor.top
  if (left + pop.width > view.width - m) left = anchor.left - gap - pop.width
  if (left < m) left = m
  if (left + pop.width > view.width - m) left = Math.max(m, view.width - pop.width - m)
  if (top + pop.height > view.height - m) top = view.height - pop.height - m
  if (top < m) top = m
  return { left: Math.round(left), top: Math.round(top) }
}

function oneLine(s: string): string {
  return s.replace(/\s+/g, ' ').trim().split(PICK_END).join(t('cat.misc.s_e039e24138')).split(PICK_START).join(t('cat.misc.s_130f3b6c71'))
}

function safeBlock(s: string): string {
  return s.split(PICK_END).join(t('cat.misc.s_e039e24138')).split(PICK_START).join(t('cat.misc.s_130f3b6c71'))
}

/** 发送时写进正文的一段说明。输入框里不直接显示这段文字。 */
export function formatPick(p: PickNote): string {
  const classes = p.classes.split(/\s+/).filter(Boolean).slice(0, 3)
  const name = p.tag + (p.id ? '#' + p.id : '') + (classes.length ? '.' + classes.join('.') : '')
  const lines = [PICK_START, t('cat.misc.s_29ba21b97d', { n: oneLine(p.file || t('cat.misc.unnamed')) }), t('cat.misc.s_bbc823ebf1', { name: oneLine(name) }), t('cat.misc.s_b7b6ee6cae', { n: p.intent === 'ask' ? t('cat.misc.question') : t('cat.misc.change') })]
  if (p.note.trim()) lines.push(t('cat.misc.s_c0e5dd524e', { n: oneLine(p.note) }))
  if (p.selector) lines.push(t('cat.misc.s_bffc0e4b1b', { n: oneLine(p.selector) }))
  if (p.text) lines.push(t('cat.misc.s_f71c314747', { n: oneLine(p.text) }))
  if (p.styles) lines.push(t('cat.misc.s_64842ab296', { n: oneLine(p.styles) }))
  if (p.snippet) {
    lines.push(t('cat.misc.s_96b4131ea5'))
    lines.push(safeBlock(p.snippet))
  }
  lines.push(PICK_END)
  return lines.join('\n')
}

/** 发送时把注释接在用户自己写的文字前面。没有注释时原文不动。 */
export function composePickSend(draft: string, notes: PickNote[]): string {
  const blocks = notes.map((n) => formatPick(n)).filter((b) => b.trim())
  if (!blocks.length) return draft
  const base = draft.replace(/^\s+/, '').replace(/\s+$/, '')
  if (!base) return blocks.join('\n\n') + '\n'
  return blocks.join('\n\n') + '\n\n' + base + '\n'
}

/** 点选脚本。调用前不执行；里面不能出现会提前结束脚本标签的字符序列。 */
export function pickRuntime(token: string): string {
  const tok = JSON.stringify(token)
  const src = JSON.stringify(PICK_SOURCE)
  return `(() => {
  var TOKEN = ${tok};
  var SRC = ${src};
  var armed = false;
  var hoverEl = null;
  var selectedEl = null;
  var hoverBox = null;
  var selBox = null;
  var label = null;
  function cssEscape(s) {
    if (window.CSS && CSS.escape) return CSS.escape(s);
    return s.replace(/[^a-zA-Z0-9_-]/g, '\\\\$&');
  }
  function send(kind, extra) {
    var msg = { source: SRC, token: TOKEN, kind: kind };
    if (extra) {
      msg.tag = extra.tag;
      msg.id = extra.id;
      msg.classes = extra.classes;
      msg.selector = extra.selector;
      msg.text = extra.text;
      msg.styles = extra.styles;
      msg.snippet = extra.snippet;
      msg.left = extra.left || 0;
      msg.top = extra.top || 0;
      msg.width = extra.width || 0;
      msg.height = extra.height || 0;
    }
    parent.postMessage(msg, '*');
  }
  function makeBox() {
    var n = document.createElement('div');
    n.setAttribute('data-cat-pick', '1');
    n.style.cssText = 'all:initial;position:fixed;z-index:2147483646;pointer-events:none;box-sizing:border-box;border:2px solid #3b82f6;background:rgba(59,130,246,.12);display:none;margin:0;padding:0;';
    return n;
  }
  function ensure() {
    if (hoverBox) return;
    hoverBox = makeBox();
    selBox = makeBox();
    label = document.createElement('div');
    label.setAttribute('data-cat-pick', '1');
    label.style.cssText = 'all:initial;position:fixed;z-index:2147483647;pointer-events:none;background:#1d4ed8;color:#fff;font:12px/1.2 sans-serif;padding:2px 6px;border-radius:4px;white-space:nowrap;display:none;';
    document.documentElement.appendChild(hoverBox);
    document.documentElement.appendChild(selBox);
    document.documentElement.appendChild(label);
  }
  function place(node, el, strong) {
    var r = el.getBoundingClientRect();
    node.style.display = 'block';
    node.style.left = r.left + 'px';
    node.style.top = r.top + 'px';
    node.style.width = Math.max(r.width, 2) + 'px';
    node.style.height = Math.max(r.height, 2) + 'px';
    node.style.borderColor = strong ? '#1d4ed8' : '#3b82f6';
    node.style.background = strong ? 'rgba(29,78,216,.16)' : 'rgba(59,130,246,.12)';
  }
  function hide() {
    hoverEl = null;
    selectedEl = null;
    if (hoverBox) hoverBox.style.display = 'none';
    if (selBox) selBox.style.display = 'none';
    if (label) label.style.display = 'none';
    if (document.documentElement) document.documentElement.style.cursor = '';
  }
  function clearSelection() {
    hoverEl = null;
    selectedEl = null;
    if (hoverBox) hoverBox.style.display = 'none';
    if (selBox) selBox.style.display = 'none';
    if (label) label.style.display = 'none';
  }
  function targetOf(e) {
    var n = e.target;
    if (!n) return null;
    if (n.nodeType === 3) n = n.parentElement;
    if (!n || n.nodeType !== 1) return null;
    if (n.hasAttribute && n.hasAttribute('data-cat-pick')) return null;
    return n;
  }
  function textOf(el) {
    var t = '';
    var tag = el.tagName;
    if (tag === 'IMG') t = el.getAttribute('alt') || '';
    else if (tag === 'INPUT' || tag === 'TEXTAREA') t = el.getAttribute('placeholder') || el.value || '';
    else t = el.innerText || el.textContent || '';
    t = String(t).replace(/\\s+/g, ' ').trim();
    if (t.length > 80) t = t.slice(0, 80) + '…';
    return t;
  }
  function stylesOf(el) {
    var keys = ['color', 'background-color', 'font-size', 'font-weight', 'display', 'padding', 'margin', 'border-radius', 'width', 'height'];
    var cs = getComputedStyle(el);
    var parts = [];
    for (var i = 0; i < keys.length; i++) {
      var v = cs.getPropertyValue(keys[i]).trim();
      if (!v || v === 'none' || v === 'auto' || v === 'normal' || v === '0px' || v === 'rgba(0, 0, 0, 0)' || v === 'transparent') continue;
      parts.push(keys[i] + ': ' + v);
    }
    var s = parts.join('; ');
    if (s.length > 360) s = s.slice(0, 360) + '…';
    return s;
  }
  function selectorOf(el) {
    var parts = [];
    var cur = el;
    var depth = 0;
    while (cur && cur.nodeType === 1 && depth < 8) {
      var tag = cur.tagName.toLowerCase();
      if (tag === 'html') break;
      var id = cur.getAttribute('id') || '';
      if (id && document.querySelectorAll('#' + cssEscape(id)).length === 1) {
        parts.unshift(tag + '#' + id);
        break;
      }
      var part = tag;
      var cls = [];
      if (cur.classList) {
        for (var i = 0; i < cur.classList.length && cls.length < 2; i++) {
          var c = cur.classList[i];
          if (/^[A-Za-z_][-A-Za-z0-9_]*$/.test(c)) cls.push(c);
        }
      }
      if (cls.length) part += '.' + cls.join('.');
      var parentEl = cur.parentElement;
      if (parentEl) {
        var same = 0;
        var index = 0;
        for (var k = 0; k < parentEl.children.length; k++) {
          if (parentEl.children[k].tagName === cur.tagName) {
            same++;
            if (parentEl.children[k] === cur) index = same;
          }
        }
        if (same > 1) part += ':nth-of-type(' + index + ')';
      }
      parts.unshift(part);
      if (!parentEl) break;
      cur = parentEl;
      depth++;
    }
    return parts.join(' > ');
  }
  function snippetOf(el) {
    var clone = el.cloneNode(true);
    if (clone.querySelectorAll) {
      var junk = clone.querySelectorAll('[data-cat-pick]');
      for (var i = junk.length - 1; i >= 0; i--) {
        if (junk[i].parentNode) junk[i].parentNode.removeChild(junk[i]);
      }
    }
    var html = clone.outerHTML || '';
    if (html.length > 500) html = html.slice(0, 500) + '…';
    return html;
  }
  function describe(el) {
    var id = el.getAttribute('id') || '';
    var classes = [];
    if (el.classList) {
      for (var i = 0; i < el.classList.length && classes.length < 3; i++) classes.push(el.classList[i]);
    }
    var r = el.getBoundingClientRect();
    return {
      tag: el.tagName.toLowerCase(),
      id: id.slice(0, 80),
      classes: classes.join(' ').slice(0, 120),
      selector: selectorOf(el).slice(0, 300),
      text: textOf(el),
      styles: stylesOf(el),
      snippet: snippetOf(el),
      left: Math.round(r.left),
      top: Math.round(r.top),
      width: Math.round(r.width),
      height: Math.round(r.height)
    };
  }
  function showLabel(el) {
    if (!label) return;
    var r = el.getBoundingClientRect();
    var name = el.tagName.toLowerCase();
    var t = textOf(el);
    if (t) {
      var short = t.length > 16 ? t.slice(0, 16) + '…' : t;
      name += ' "' + short + '"';
    }
    name += ' ' + Math.round(Math.max(r.width, 0)) + 'x' + Math.round(Math.max(r.height, 0));
    label.textContent = name;
    label.style.display = 'block';
    label.style.left = Math.max(4, r.left) + 'px';
    label.style.top = Math.max(4, r.top - 8) + 'px';
  }
  function onMove(e) {
    if (!armed) return;
    var el = targetOf(e);
    if (!el) return;
    ensure();
    hoverEl = el;
    place(hoverBox, el, false);
    if (!selectedEl) showLabel(el);
  }
  function onClick(e) {
    if (!armed) return;
    var el = targetOf(e);
    if (!el) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.stopImmediatePropagation) e.stopImmediatePropagation();
    if (e.type !== 'pointerdown') return;
    if (e.button != null && e.button !== 0) return;
    ensure();
    selectedEl = el;
    place(selBox, el, true);
    showLabel(el);
    send('select', describe(el));
  }
  function onKey(e) {
    if (!armed || e.key !== 'Escape') return;
    e.preventDefault();
    send('esc');
  }
  function onScroll() {
    if (hoverEl && hoverBox) place(hoverBox, hoverEl, false);
    if (selectedEl && selBox) place(selBox, selectedEl, true);
    if (selectedEl) showLabel(selectedEl);
    else if (hoverEl) showLabel(hoverEl);
    if (!selectedEl) return;
    var r = selectedEl.getBoundingClientRect();
    send('move', { left: Math.round(r.left), top: Math.round(r.top), width: Math.round(r.width), height: Math.round(r.height) });
  }
  window.addEventListener('pointermove', onMove, true);
  window.addEventListener('pointerdown', onClick, true);
  window.addEventListener('mousedown', onClick, true);
  window.addEventListener('click', onClick, true);
  window.addEventListener('auxclick', onClick, true);
  window.addEventListener('contextmenu', onClick, true);
  window.addEventListener('keydown', onKey, true);
  window.addEventListener('scroll', onScroll, true);
  window.addEventListener('message', function (e) {
    var d = e.data;
    if (!d || d.source !== SRC || d.token !== TOKEN) return;
    if (e.source !== parent) return;
    if (d.kind === 'clear') {
      clearSelection();
      return;
    }
    if (d.kind !== 'arm') return;
    armed = !!d.on;
    if (document.documentElement) document.documentElement.style.cursor = armed ? 'crosshair' : '';
    if (!armed) hide();
  });
  send('ready');
})();`
}

/** 把点选脚本接到页面末尾。调用方只把它用于预览，不写回文件。 */
export function withPickScript(html: string, token: string): string {
  if (!/^[a-z0-9]+$/i.test(token)) return html
  const tag = '<script data-cat-pick="1">' + pickRuntime(token) + '</scr' + 'ipt>'
  const i = html.toLowerCase().lastIndexOf('</body>')
  if (i >= 0) return html.slice(0, i) + tag + html.slice(i)
  return html + tag
}
