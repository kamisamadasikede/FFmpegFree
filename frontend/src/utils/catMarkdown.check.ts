// Cat 助手回复 Markdown 自检（api.check.ts 调用）：markdown-it 渲染 + DOMPurify（jsdom 窗口，由 scripts/check-api.mjs 提供）。
import { getCatPurifier, prepareStreamingSource, renderCatMarkdown, renderCatMarkdownRaw } from './catMarkdown'

type Eq = (name: string, got: unknown, want: unknown) => void

export async function catMarkdownChecks(eq: Eq): Promise<void> {
  const mk = (globalThis as unknown as { __makeDomWindow?: () => Promise<Window> }).__makeDomWindow
  if (!mk) {
    eq('Cat md：需要 jsdom 窗口', 'missing', 'ok')
    return
  }
  const win = await mk()
  getCatPurifier(win)
  const html = (s: string, streaming = false) => renderCatMarkdown(s, streaming).map((b) => b.html).join('')
  const doc = (s: string, streaming = false) => {
    const d = win.document.createElement('div')
    d.innerHTML = html(s, streaming)
    return d
  }

  // 基本元素
  const rich = doc('# 标题\n\n## 二级\n\n- a\n- b\n\n1. x\n2. y\n\n**粗** *斜* `行内`\n\n> 引用\n\n---\n\n| 列1 | 列2 |\n| --- | :-: |\n| 1 | 2 |\n\n```ts\nconst a = 1 < 2\n```\n')
  eq('Cat md：标题 / 列表 / 强调 / 引用 / 分隔线', ['h1', 'h2', 'ul li', 'ol li', 'strong', 'em', 'p > code', 'blockquote', 'hr'].map((q) => rich.querySelectorAll(q).length),
    [1, 1, 2, 2, 1, 1, 1, 1, 1])
  eq('Cat md：表格包在可横向滚动的容器里，对齐只留 text-align', [rich.querySelectorAll('.cat-md-table > table tr').length, rich.querySelector('td:last-child')?.getAttribute('style')], [2, 'text-align:center'])
  const code = rich.querySelector('.cat-md-code')
  eq('Cat md：代码块（语言、复制按钮、内容转义）', [code?.querySelector('.cat-md-lang')?.textContent, code?.querySelector('button[data-cat-copy]')?.textContent, code?.querySelector('pre code')?.textContent],
    ['ts', '复制', 'const a = 1 < 2\n'])
  eq('Cat md：按顶层块切分', renderCatMarkdownRaw('a\n\nb\n\n- c').length, 3)

  // 原始 HTML / 脚本
  const evilSrc = '<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n<b onclick="x()">hi</b>\n\n<iframe src="https://e.com"></iframe>'
  const evil = doc(evilSrc)
  const attrs = Array.from(evil.querySelectorAll('*')).flatMap((el) => Array.from(el.attributes).map((a) => a.name))
  eq('Cat md：原始 HTML 转义成文字，不出标签、不出事件属性',
    [evil.querySelectorAll('script,img,iframe,b').length, attrs.some((a) => a.startsWith('on')), /<script>alert\(1\)<\/script>/.test(evil.textContent ?? ''), /<script/i.test(html(evilSrc))], [0, false, true, false])

  // 链接
  const bad = doc('[点我](javascript:alert(1)) [d](data:text/html;base64,AAAA) [f](file:///etc/passwd) [r](./a.md) <javascript:alert(1)>')
  eq('Cat md：javascript: / data: / file: / 相对链接不生成链接', [bad.querySelectorAll('a[href]').length, /javascript:alert/.test(bad.textContent ?? '')], [0, true])
  const good = doc('[官网](https://example.com/a?b=1) 和 https://example.org 以及 [信](mailto:a@example.com)')
  const as = Array.from(good.querySelectorAll('a'))
  eq('Cat md：https / 裸链接 / mailto 生成外链（noopener + data-cat-ext，无 target）',
    as.map((a) => [a.getAttribute('href'), a.getAttribute('rel'), a.getAttribute('data-cat-ext'), a.hasAttribute('target')]),
    [['https://example.com/a?b=1', 'noopener noreferrer', '1', false], ['https://example.org', 'noopener noreferrer', '1', false], ['mailto:a@example.com', 'noopener noreferrer', '1', false]])

  // 图片：不加载
  const img = doc('![示意图](https://example.com/x.png) ![本地](file:///c.png) ![d](data:image/png;base64,AAAA)')
  eq('Cat md：图片不出 <img>，外链图片变成「图片：alt」链接，其余只留文字',
    [img.querySelectorAll('img').length, Array.from(img.querySelectorAll('a')).map((a) => [a.textContent, a.getAttribute('href')]), /本地/.test(img.textContent ?? '')],
    [0, [['图片：示意图', 'https://example.com/x.png']], true])

  // DOMPurify 第二道：直接喂危险 HTML（假设 markdown-it 漏过）
  const p = getCatPurifier(win)
  const leaked = String(p.sanitize('<a href="javascript:alert(1)" data-cat-ext="1" target="_blank">x</a><img src="https://e.com/a.png"><p style="background:url(https://e.com)" onclick="x()">y</p><div data-foo="1">z</div>',
    { ALLOWED_TAGS: ['a', 'p', 'div', 'img'], ALLOWED_ATTR: ['href', 'style', 'data-cat-ext', 'target'], ALLOWED_URI_REGEXP: /^(?:https?:\/\/|mailto:)/i }))
  eq('Cat md：过滤器钩子去掉坏链接 / target / 非表格样式', [/javascript:|target=|data-cat-ext|style=|onclick/.test(leaked)], [false])

  // 流式：未闭合代码块
  let thrown = ''
  let partial = ''
  try {
    partial = html('先看代码：\n\n```python\nprint(1)\nfor i in ra', true)
  } catch (e) {
    thrown = String(e)
  }
  eq('Cat md：流式未闭合围栏按代码块渲染、不抛错', [thrown, /<pre><code>print\(1\)\nfor i in ra\n?<\/code><\/pre>/.test(partial)], ['', true])
  eq('Cat md：流式补上结尾围栏', prepareStreamingSource('```js\nx', true), '```js\nx\n```\n')
  eq('Cat md：结尾围栏写了一半先不显示', prepareStreamingSource('```js\nx\n``', true), '```js\nx\n```\n')
  eq('Cat md：开始围栏写了一半先不显示', prepareStreamingSource('段落\n`', true), '段落\n')
  eq('Cat md：开始围栏行未换行也算代码块', prepareStreamingSource('段落\n```py', true), '段落\n```py\n```\n')
  eq('Cat md：表格行写了一半先不显示', prepareStreamingSource('| a | b |\n| --', true), '| a | b |\n')
  eq('Cat md：已闭合 / 非流式原样', [prepareStreamingSource('```\nx\n```\n后', true), prepareStreamingSource('```\nx', false)], ['```\nx\n```\n后', '```\nx'])
  eq('Cat md：流式时已完成的块 HTML 不变（增量不重建）', renderCatMarkdown('# 一\n\n段落二', true)[0].html === renderCatMarkdown('# 一\n\n段落二继续', true)[0].html, true)
  const tbl = doc('| a | b |\n| --- | --- |\n| 1 |', true)
  eq('Cat md：流式半截表格行不抛错，降级显示', tbl.querySelectorAll('table').length, 1)

  // 恢复浏览器默认（node 下没有 window，后续检查不再用）
}
