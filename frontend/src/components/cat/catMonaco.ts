// 侧边文本编辑器：一套编辑器，语言包按扩展名注册。主题跟页面一起变。
import { applyMonacoTheme, monaco, monoFontFamily } from '@/utils/monacoJson'
import 'monaco-editor/esm/vs/basic-languages/monaco.contribution'
import { SYNTAX_KINDS, SYNTAX_LANGUAGES, encodeSyntaxSpans, syntaxSpans } from './catSyntax'

export { applyMonacoTheme, monaco, monoFontFamily }

const legend = { tokenTypes: [...SYNTAX_KINDS], tokenModifiers: [] as string[] }
const marked = globalThis as { __ffCatSyntax?: boolean }
if (!marked.__ffCatSyntax) {
  marked.__ffCatSyntax = true
  monaco.languages.registerDocumentSemanticTokensProvider(SYNTAX_LANGUAGES, {
    getLegend: () => legend,
    provideDocumentSemanticTokens(model) {
      return { data: encodeSyntaxSpans(syntaxSpans(model.getLanguageId(), model.getValue())) }
    },
    releaseDocumentSemanticTokens() {},
  })
}
