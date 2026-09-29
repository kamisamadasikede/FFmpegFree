// JSON 文本的纯函数工具：错误位置换算、错误文案、缩进检测、转义。
// 服务端（JsonService）是格式化和校验的主路径；这里的函数用于：
//   1. 把服务端英文错误信息翻成设计稿里的中文提示；
//   2. 没有 Wails 绑定时（浏览器里跑 vite dev）的本地兜底；
//   3. 转义 / 去转义（纯字符串操作，不需要走后端）。

export interface SyntaxErr {
  line: number
  column: number
  message: string
}

/** 把服务端（Go）/ JSON.parse（V8）的英文错误翻成简短中文，文案取自设计稿「缺少逗号或括号」。 */
export function describeSyntax(raw: string): string {
  const m = raw.replace(/^JSON 语法错误[:：]\s*/, '')
  if (/[\u4e00-\u9fa5]/.test(m)) return m // 已经是中文（服务端的 内容为空 / 内容不完整 / 结束后还有多余内容，或本地校验已翻译）
  const rules: [RegExp, string][] = [
    [/after object key:value pair|after array element|Expected ',' or|after property value/i, '缺少逗号或括号'],
    [/beginning of object key string|double-quoted property name|Expected property name/i, '键名需要用双引号括起来'],
    [/after object key\b|Expected ':'|after property name/i, '键名后面缺少冒号'],
    [/beginning of value|Unexpected token|Expected value/i, '这里应该是一个值'],
    [/string literal|string escape|control character|Unterminated string|Bad escaped/i, '字符串里有非法字符或未闭合'],
    [/in literal|numeric literal|decimal point|exponent|No number after/i, '字面量或数字格式不正确'],
    [/end of JSON input|unexpected end/i, '内容不完整，可能缺少右括号或引号'],
    [/after top-level value|non-whitespace character after JSON/i, 'JSON 结束后还有多余内容'],
  ]
  for (const [re, zh] of rules) if (re.test(m)) return zh
  return '语法错误'
}

export function offsetToLineCol(text: string, offset: number): { line: number; column: number } {
  const off = Math.max(0, Math.min(offset, text.length))
  let line = 1
  let last = -1
  let i = text.indexOf('\n')
  while (i !== -1 && i < off) {
    line++
    last = i
    i = text.indexOf('\n', i + 1)
  }
  return { line, column: off - last }
}

/** 本地 JSON.parse 校验，位置取自引擎的错误信息。 */
export function localValidate(text: string): SyntaxErr | null {
  try {
    JSON.parse(text)
    return null
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    const lc = msg.match(/line (\d+) column (\d+)/i)
    let pos: { line: number; column: number }
    if (lc) pos = { line: +lc[1], column: +lc[2] }
    else {
      const p = msg.match(/position (\d+)/i)
      pos = p ? offsetToLineCol(text, +p[1]) : /end of JSON input/i.test(msg) ? offsetToLineCol(text, text.length) : { line: 1, column: 1 }
    }
    return { ...pos, message: describeSyntax(msg) }
  }
}

/** 在已通过校验的文本上重排缩进（不经过 JSON.parse，数字和键顺序原样保留）。indent 为 null 表示压缩成单行。 */
export function reindent(text: string, indent: number | null): string {
  const out: string[] = []
  const unit = indent ? ' '.repeat(indent) : ''
  let depth = 0
  let i = 0
  const n = text.length
  const nextNonWs = (from: number) => {
    let j = from
    while (j < n && /\s/.test(text[j])) j++
    return j
  }
  while (i < n) {
    const c = text[i]
    if (c === '"') {
      let j = i + 1
      while (j < n && text[j] !== '"') j += text[j] === '\\' ? 2 : 1
      out.push(text.slice(i, j + 1))
      i = j + 1
    } else if (c === '{' || c === '[') {
      const j = nextNonWs(i + 1)
      if (text[j] === (c === '{' ? '}' : ']')) {
        out.push(c + text[j])
        i = j + 1
      } else {
        depth++
        out.push(c)
        if (indent) out.push('\n' + unit.repeat(depth))
        i++
      }
    } else if (c === '}' || c === ']') {
      depth--
      if (indent) out.push('\n' + unit.repeat(depth))
      out.push(c)
      i++
    } else if (c === ',') {
      out.push(indent ? ',\n' + unit.repeat(depth) : ',')
      i++
    } else if (c === ':') {
      out.push(indent ? ': ' : ':')
      i++
    } else if (/\s/.test(c)) {
      i++
    } else {
      let j = i
      while (j < n && !/[\s,:\]}]/.test(text[j])) j++
      out.push(text.slice(i, j))
      i = j
    }
  }
  return out.join('')
}

export function countLines(text: string): number {
  let n = 1
  let i = text.indexOf('\n')
  while (i !== -1) {
    n++
    i = text.indexOf('\n', i + 1)
  }
  return n
}

/** 状态栏右侧的缩进描述：无缩进 / 缩进 N 空格 / 缩进 Tab。 */
export function describeIndent(text: string): string {
  if (!text.includes('\n')) return '无缩进'
  const m = text.match(/\n([ \t]+)\S/)
  if (!m) return '无缩进'
  return m[1][0] === '\t' ? '缩进 Tab' : `缩进 ${m[1].length} 空格`
}

/** 转义：得到可以放进 JSON 字符串里的内容（不带外层引号）。 */
export function escapeText(text: string): string {
  return JSON.stringify(text).slice(1, -1)
}

/** 去转义：接受带引号的 JSON 字符串字面量，或不带外层引号的转义内容。 */
export function unescapeText(text: string): string {
  const t = text.trim()
  if (t.length >= 2 && t.startsWith('"') && t.endsWith('"')) {
    try {
      const v = JSON.parse(t)
      if (typeof v === 'string') return v
    } catch {
      /* 落到下面按裸内容处理 */
    }
  }
  try {
    const v = JSON.parse('"' + t.replace(/\r/g, '\\r').replace(/\n/g, '\\n') + '"')
    return v as string
  } catch {
    throw new Error('不是有效的转义字符串')
  }
}

/** 没有 Wails 绑定时（浏览器里跑 vite dev / 预览）的本地格式化，返回结构和 JsonService.Format 一致。 */
export function localFormat(text: string, compact: boolean, indent = 2) {
  const bad = localValidate(text)
  if (bad) return { formatted: '', error: bad.message, errorPos: { line: bad.line, column: bad.column } }
  return { formatted: reindent(text.trim(), compact ? null : indent), error: '', errorPos: { line: 0, column: 0 } }
}
