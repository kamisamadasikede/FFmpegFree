// Monaco 精简装载：只要编辑器内核 + json 语言 + 少量常用交互，不引入其他语言和 worker。
// 主题颜色全部读自 tokens.css 的 --ff-* 变量，浅色 / 暗色跟随 html.dark 实时切换。
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api'
import 'monaco-editor/esm/vs/language/json/monaco.contribution'
import 'monaco-editor/esm/vs/editor/contrib/semanticTokens/browser/documentSemanticTokens.js'
import 'monaco-editor/esm/vs/editor/contrib/clipboard/browser/clipboard'
import 'monaco-editor/esm/vs/editor/contrib/contextmenu/browser/contextmenu'
import 'monaco-editor/esm/vs/editor/contrib/bracketMatching/browser/bracketMatching'
import 'monaco-editor/esm/vs/editor/contrib/linesOperations/browser/linesOperations'
import 'monaco-editor/esm/vs/editor/contrib/wordOperations/browser/wordOperations'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'

;(self as unknown as { MonacoEnvironment: monaco.Environment }).MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
}

// 语法校验、格式化都由 JsonService 负责（错误位置要和服务端一致），关掉 json 语言自带的 worker 功能。
// 只保留语言注册和括号 / 引号自动补全配置。
const jsonDefaults = (monaco.languages as unknown as {
  json: {
    jsonDefaults: {
      setDiagnosticsOptions(o: object): void
      setModeConfiguration(o: object): void
    }
  }
}).json.jsonDefaults
jsonDefaults.setDiagnosticsOptions({ validate: false })
jsonDefaults.setModeConfiguration({
  documentFormattingEdits: false,
  documentRangeFormattingEdits: false,
  completionItems: false,
  hovers: false,
  documentSymbols: false,
  tokens: false, // 自己注册词法，把 null 单独区分出来
  colors: false,
  foldingRanges: false,
  diagnostics: false,
  selectionRanges: false,
})

monaco.languages.setMonarchTokensProvider('json', {
  tokenizer: {
    root: [
      [/"(?:[^"\\]|\\.)*"(?=\s*:)/, 'string.key.json'],
      [/"(?:[^"\\]|\\.)*"?/, 'string.value.json'],
      [/-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/, 'number.json'],
      [/\b(?:true|false)\b/, 'keyword.json'],
      [/\bnull\b/, 'keyword.null.json'],
      [/[{}[\],:]/, 'delimiter.json'],
      [/\s+/, 'white'],
    ],
  },
})

function rgbToHex(color: string): string {
  const match = color.match(/rgba?\(\s*(\d+)[,\s]+(\d+)[,\s]+(\d+)/)
  if (!match) return ''
  const hex = (part: string) => Number(part).toString(16).padStart(2, '0')
  return `#${hex(match[1])}${hex(match[2])}${hex(match[3])}`
}

// getPropertyValue 会留下 var(--ff-*)，Monaco 只接受十六进制。先解析成真正的颜色。
const css = (name: string) => {
  const probe = document.createElement('span')
  probe.style.color = `var(${name})`
  document.documentElement.appendChild(probe)
  const resolved = rgbToHex(getComputedStyle(probe).color)
  probe.remove()
  return resolved || getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}
const bare = (hex: string) => hex.replace('#', '')
/** #rrggbb + 透明度 → #rrggbbaa */
const alpha = (hex: string, a: number) => hex + Math.round(a * 255).toString(16).padStart(2, '0')

export const isDarkNow = () => document.documentElement.classList.contains('dark')

export function monoFontFamily(): string {
  return css('--ff-font-mono') || 'monospace'
}

/** 按当前 CSS 变量重新定义并应用主题。切换主题时重复调用即可。 */
export function applyMonacoTheme(): void {
  const dark = isDarkNow()
  const name = dark ? 'ff-dark' : 'ff-light'
  const text1 = css('--ff-text-1')
  const text2 = css('--ff-text-2')
  const text3 = css('--ff-text-3')
  monaco.editor.defineTheme(name, {
    base: dark ? 'vs-dark' : 'vs',
    inherit: true,
    rules: [
      { token: '', foreground: bare(text1) },
      { token: 'comment', foreground: bare(css('--ff-code-comment')), fontStyle: 'italic' },
      { token: 'keyword', foreground: bare(css('--ff-code-keyword')) },
      { token: 'function', foreground: bare(css('--ff-code-function')) },
      { token: 'type', foreground: bare(css('--ff-code-type')) },
      { token: 'string', foreground: bare(css('--ff-code-string')) },
      { token: 'number', foreground: bare(css('--ff-code-number')) },
      { token: 'regexp', foreground: bare(css('--ff-code-string')) },
      { token: 'tag', foreground: bare(css('--ff-code-keyword')) },
      { token: 'attribute.name', foreground: bare(css('--ff-code-type')) },
      { token: 'delimiter', foreground: bare(text3) },
      { token: 'string.key.json', foreground: bare(css('--ff-syntax-key')) },
      { token: 'string.value.json', foreground: bare(css('--ff-syntax-string')) },
      { token: 'number.json', foreground: bare(css('--ff-syntax-number')) },
      { token: 'keyword.json', foreground: bare(css('--ff-syntax-bool')) },
      { token: 'keyword.null.json', foreground: bare(css('--ff-syntax-null')) },
      { token: 'delimiter.json', foreground: bare(text3) },
    ],
    colors: {
      'editor.background': css('--ff-bg-surface'),
      'editor.foreground': text1,
      'editorGutter.background': css('--ff-bg-surface'),
      'editorLineNumber.foreground': text3,
      'editorLineNumber.activeForeground': text2,
      'editorCursor.foreground': text1,
      'editor.selectionBackground': alpha(css('--ff-primary'), 0.28),
      'editor.inactiveSelectionBackground': alpha(css('--ff-primary'), 0.16),
      'editor.lineHighlightBackground': css('--ff-bg-hover'),
      'editorWidget.background': css('--ff-bg-elevated'),
      'editorWidget.border': css('--ff-border'),
      'editorSuggestWidget.background': css('--ff-bg-elevated'),
      'input.background': css('--ff-bg-surface'),
      'input.border': css('--ff-border'),
      'focusBorder': css('--ff-primary'),
      'editorBracketMatch.background': alpha(css('--ff-primary'), 0.16),
      'editorBracketMatch.border': alpha(css('--ff-primary'), 0.4),
      'diffEditor.insertedTextBackground': alpha(css('--ff-success'), 0.2),
      'diffEditor.removedTextBackground': alpha(css('--ff-danger'), 0.18),
      'diffEditor.insertedLineBackground': alpha(css('--ff-success'), 0.16),
      'diffEditor.removedLineBackground': alpha(css('--ff-danger'), 0.14),
      'diffEditorGutter.insertedLineBackground': alpha(css('--ff-success'), 0.16),
      'diffEditorGutter.removedLineBackground': alpha(css('--ff-danger'), 0.14),
      'scrollbarSlider.background': alpha(text3, 0.4),
      'scrollbarSlider.hoverBackground': alpha(text3, 0.6),
      'scrollbarSlider.activeBackground': alpha(text3, 0.8),
      'menu.background': css('--ff-bg-elevated'),
      'menu.foreground': text1,
      'menu.selectionBackground': css('--ff-bg-hover'),
      'menu.selectionForeground': text1,
      'menu.separatorBackground': css('--ff-border'),
    },
  })
  monaco.editor.setTheme(name)
}

export { monaco }
