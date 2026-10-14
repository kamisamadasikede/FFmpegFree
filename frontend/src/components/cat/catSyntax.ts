
import { t } from '@/i18n'// 代码编辑器的第二层着色：在语言自带的词法之上，把函数名和类型名分开。
// 覆盖 Python、C、C++、Java、C#、JavaScript、TypeScript、Go、PHP、Rust。
// C 和 C++ 在编辑器里都是 cpp。不跑语言服务器。

export const SYNTAX_LANGUAGES = ['python', 'cpp', 'java', 'csharp', 'javascript', 'typescript', 'go', 'php', 'rust'] as const

export const SYNTAX_KINDS = ['keyword', 'function', 'type', 'string', 'number', 'comment'] as const

export type SyntaxKind = (typeof SYNTAX_KINDS)[number]

export type SyntaxSpan = {
  line: number
  start: number
  length: number
  kind: SyntaxKind
}

const words = (text: string) => new Set(text.split(/\s+/).filter(Boolean))

type QuoteStyle = 'go' | 'py' | 'c' | 'js' | 'rust' | 'php' | 'cs'

type Profile = {
  slash: boolean
  hash: boolean
  block: boolean
  nested: boolean
  quotes: QuoteStyle
  triple: 'both' | 'double' | 'no'
  template: boolean
  cppRaw: boolean
  pascal: boolean
  dollar: boolean
  macro: boolean
  keywords: Set<string>
  types: Set<string>
  typeIntro: Set<string>
  funcIntro: Set<string>
}

const GO_KW = 'break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var true false nil'
const GO_TY = 'bool byte rune string error any comparable int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 uintptr float32 float64 complex64 complex128'

const PY_KW = 'False None True and as assert async await break case class continue def del elif else except finally for from global if import in is lambda match nonlocal not or pass raise return try while with yield'
const PY_TY = 'int float complex str bool list dict set tuple bytes bytearray object type'

const JS_KW = 'break case catch class const continue debugger default delete do else export extends finally for from function if import in instanceof let new of return static super switch this throw try typeof var void while with yield async await get set'
const TS_KW = JS_KW + ' abstract as asserts declare enum implements infer interface is keyof module namespace private protected public readonly type override satisfies'
const JS_TY = 'string number boolean any void never unknown object symbol bigint'

const JAVA_KW = 'abstract assert break case catch class const continue default do else enum extends final finally for goto if implements import instanceof interface native new package private protected public return static strictfp super switch synchronized this throw throws transient try volatile while true false null'
const JAVA_TY = 'int long short byte char boolean float double void'

const CPP_KW = 'alignas alignof and and_eq asm auto bitand bitor break case catch class co_await co_return co_yield compl concept const consteval constexpr constinit const_cast continue decltype default delete do dynamic_cast else enum explicit export extern false for friend goto if inline mutable namespace new noexcept not not_eq nullptr operator or or_eq private protected public register reinterpret_cast return sizeof static static_assert static_cast struct switch template this thread_local throw true try typedef typeid typename union using virtual volatile while xor xor_eq'
const CPP_TY = 'void bool char int short long float double signed unsigned size_t wchar_t char8_t char16_t char32_t int8_t uint8_t int16_t uint16_t int32_t uint32_t int64_t uint64_t intptr_t uintptr_t string'

const CS_KW = 'abstract as base break case catch checked class const continue default delegate do else enum event explicit extern finally fixed for foreach goto if implicit in interface internal is lock namespace new null operator out override params private protected public readonly ref return sealed sizeof stackalloc static struct switch this throw true false try typeof unchecked unsafe using virtual volatile while async await var dynamic get set record'
const CS_TY = 'bool byte char decimal double float int long object sbyte short string uint ulong ushort void'

const PHP_KW = 'abstract and as break case catch class clone const continue declare default die do echo else elseif empty enddeclare endfor endforeach endif endswitch endwhile eval exit extends final finally fn for foreach function global goto if implements include include_once instanceof insteadof interface isset list match namespace new or print private protected public require require_once return static switch throw trait try unset use var while xor yield true false null'
const PHP_TY = 'int float string bool iterable object mixed void never array callable'

const RS_KW = 'as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self static struct super trait true type unsafe use where while'
const RS_TY = 'i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f32 f64 bool char str'

function profile(partial: Omit<Profile, 'keywords' | 'types' | 'typeIntro' | 'funcIntro'> & {
  keywords: string
  types: string
  typeIntro: string
  funcIntro: string
}): Profile {
  return {
    ...partial,
    keywords: words(partial.keywords),
    types: words(partial.types),
    typeIntro: words(partial.typeIntro),
    funcIntro: words(partial.funcIntro),
  }
}

const baseC = {
  slash: true,
  hash: false,
  block: true,
  nested: false,
  quotes: 'c' as const,
  triple: 'no' as const,
  template: false,
  cppRaw: false,
  pascal: true,
  dollar: false,
  macro: false,
}

const PROFILES: Record<string, Profile> = {
  go: profile({
    ...baseC,
    quotes: 'go',
    keywords: GO_KW,
    types: GO_TY,
    typeIntro: 'type',
    funcIntro: 'func',
  }),
  python: profile({
    slash: false,
    hash: true,
    block: false,
    nested: false,
    quotes: 'py',
    triple: 'both',
    template: false,
    cppRaw: false,
    pascal: true,
    dollar: false,
    macro: false,
    keywords: PY_KW,
    types: PY_TY,
    typeIntro: 'class',
    funcIntro: 'def',
  }),
  javascript: profile({
    ...baseC,
    quotes: 'js',
    template: true,
    keywords: JS_KW,
    types: JS_TY,
    typeIntro: 'class extends',
    funcIntro: 'function',
  }),
  typescript: profile({
    ...baseC,
    quotes: 'js',
    template: true,
    keywords: TS_KW,
    types: JS_TY,
    typeIntro: 'class interface type enum extends implements',
    funcIntro: 'function',
  }),
  java: profile({
    ...baseC,
    triple: 'double',
    keywords: JAVA_KW,
    types: JAVA_TY,
    typeIntro: 'class interface enum extends implements',
    funcIntro: '',
  }),
  cpp: profile({
    ...baseC,
    cppRaw: true,
    keywords: CPP_KW,
    types: CPP_TY,
    typeIntro: 'class struct enum union',
    funcIntro: '',
  }),
  csharp: profile({
    ...baseC,
    quotes: 'cs',
    triple: 'double',
    keywords: CS_KW,
    types: CS_TY,
    typeIntro: 'class struct interface enum record extends',
    funcIntro: '',
  }),
  php: profile({
    ...baseC,
    hash: true,
    quotes: 'php',
    dollar: true,
    keywords: PHP_KW,
    types: PHP_TY,
    typeIntro: 'class interface trait enum extends implements',
    funcIntro: 'function fn',
  }),
  rust: profile({
    ...baseC,
    nested: true,
    quotes: 'rust',
    macro: true,
    keywords: RS_KW,
    types: RS_TY,
    typeIntro: 'struct enum trait type impl',
    funcIntro: 'fn',
  }),
}

type Pos = { i: number; line: number; col: number }

class Scan {
  i = 0
  line = 0
  col = 0
  prev = ''

  constructor(
    readonly text: string,
    readonly spans: SyntaxSpan[],
  ) {}

  get n(): number {
    return this.text.length
  }

  at(): string {
    return this.text.charAt(this.i)
  }

  snapshot(): Pos {
    return { i: this.i, line: this.line, col: this.col }
  }

  advanceOne(): void {
    const c = this.text[this.i]
    if (c === '\r') {
      this.i++
      if (this.text[this.i] === '\n') this.i++
      this.line++
      this.col = 0
      return
    }
    if (c === '\n') {
      this.i++
      this.line++
      this.col = 0
      return
    }
    this.i++
    this.col++
  }

  emitTo(from: Pos, kind: SyntaxKind): void {
    let i = from.i
    let line = from.line
    let col = from.col
    let segCol = col
    let segLine = line
    const text = this.text
    const end = this.i
    while (i < end) {
      const c = text[i]
      if (c === '\r' || c === '\n') {
        if (col > segCol) this.spans.push({ line: segLine, start: segCol, length: col - segCol, kind })
        if (c === '\r' && text[i + 1] === '\n') i++
        i++
        line++
        col = 0
        segCol = 0
        segLine = line
      } else {
        i++
        col++
      }
    }
    if (col > segCol) this.spans.push({ line: segLine, start: segCol, length: col - segCol, kind })
  }
}

function isIdentStart(c: string): boolean {
  return c !== '' && /[A-Za-z_\u00C0-\uFFFF]/.test(c)
}

function isIdentPart(c: string): boolean {
  return c !== '' && /[A-Za-z0-9_\u00C0-\uFFFF]/.test(c)
}

function isPascal(word: string): boolean {
  if (!/^[A-Z]/.test(word)) return false
  if (word.length === 1) return true
  return !/^[A-Z][A-Z0-9_]*$/.test(word)
}

function peekNext(scan: Scan, profile: Profile): string {
  const text = scan.text
  let i = scan.i
  while (i < text.length) {
    const c = text[i]
    if (c === ' ' || c === '\t' || c === '\r' || c === '\n') {
      i++
      continue
    }
    if (profile.slash && text.startsWith('//', i)) {
      while (i < text.length && text[i] !== '\n') i++
      continue
    }
    if (profile.hash && c === '#') {
      while (i < text.length && text[i] !== '\n') i++
      continue
    }
    if (profile.block && text.startsWith('/*', i)) {
      const end = text.indexOf('*/', i + 2)
      i = end < 0 ? text.length : end + 2
      continue
    }
    return text.slice(i, i + 2)
  }
  return ''
}

function tryComment(scan: Scan, profile: Profile): boolean {
  const text = scan.text
  if (profile.slash && text.startsWith('//', scan.i)) {
    const from = scan.snapshot()
    while (scan.i < scan.n && scan.at() !== '\n' && scan.at() !== '\r') scan.advanceOne()
    scan.emitTo(from, 'comment')
    return true
  }
  if (profile.hash && scan.at() === '#') {
    const from = scan.snapshot()
    while (scan.i < scan.n && scan.at() !== '\n' && scan.at() !== '\r') scan.advanceOne()
    scan.emitTo(from, 'comment')
    return true
  }
  if (profile.block && text.startsWith('/*', scan.i)) {
    const from = scan.snapshot()
    let depth = 1
    scan.advanceOne()
    scan.advanceOne()
    while (scan.i < scan.n && depth > 0) {
      if (profile.nested && text.startsWith('/*', scan.i)) {
        depth++
        scan.advanceOne()
        scan.advanceOne()
        continue
      }
      if (text.startsWith('*/', scan.i)) {
        depth--
        scan.advanceOne()
        scan.advanceOne()
        continue
      }
      scan.advanceOne()
    }
    scan.emitTo(from, 'comment')
    return true
  }
  return false
}

function scanQuoted(scan: Scan, quote: string, multiline: boolean): void {
  const from = scan.snapshot()
  scan.advanceOne()
  while (scan.i < scan.n) {
    const c = scan.at()
    if (!multiline && (c === '\n' || c === '\r')) break
    if (c === '\\') {
      scan.advanceOne()
      if (scan.i < scan.n) scan.advanceOne()
      continue
    }
    scan.advanceOne()
    if (c === quote) break
  }
  scan.emitTo(from, 'string')
  scan.prev = ''
}

function scanTriple(scan: Scan, quote: string): void {
  const from = scan.snapshot()
  scan.advanceOne()
  scan.advanceOne()
  scan.advanceOne()
  const text = scan.text
  while (scan.i < scan.n && !text.startsWith(quote, scan.i)) scan.advanceOne()
  if (text.startsWith(quote, scan.i)) {
    scan.advanceOne()
    scan.advanceOne()
    scan.advanceOne()
  }
  scan.emitTo(from, 'string')
  scan.prev = ''
}

function scanRawBacktick(scan: Scan): void {
  const from = scan.snapshot()
  scan.advanceOne()
  while (scan.i < scan.n && scan.at() !== '`') scan.advanceOne()
  if (scan.at() === '`') scan.advanceOne()
  scan.emitTo(from, 'string')
  scan.prev = ''
}

function scanTemplate(scan: Scan, profile: Profile): void {
  let from = scan.snapshot()
  scan.advanceOne()
  while (scan.i < scan.n) {
    const c = scan.at()
    if (c === '\\') {
      scan.advanceOne()
      if (scan.i < scan.n) scan.advanceOne()
      continue
    }
    if (c === '`') {
      scan.advanceOne()
      scan.emitTo(from, 'string')
      scan.prev = ''
      return
    }
    if (c === '$' && scan.text[scan.i + 1] === '{') {
      scan.emitTo(from, 'string')
      scan.advanceOne()
      scan.advanceOne()
      scanCode(scan, profile, true)
      if (scan.at() === '}') scan.advanceOne()
      from = scan.snapshot()
      continue
    }
    scan.advanceOne()
  }
  scan.emitTo(from, 'string')
  scan.prev = ''
}

function scanRustRaw(scan: Scan): boolean {
  const text = scan.text
  if (scan.at() !== 'r') return false
  let j = scan.i + 1
  let hashes = 0
  while (text[j] === '#') {
    hashes++
    j++
  }
  if (text[j] !== '"') return false
  const end = '"' + '#'.repeat(hashes)
  const at = text.indexOf(end, j + 1)
  const from = scan.snapshot()
  const target = at < 0 ? scan.n : at + end.length
  while (scan.i < target) scan.advanceOne()
  scan.emitTo(from, 'string')
  scan.prev = ''
  return true
}

function scanRustTick(scan: Scan): void {
  const text = scan.text
  const next = text[scan.i + 1] ?? ''
  const next2 = text[scan.i + 2] ?? ''
  if (next === '\\' || (next2 === "'" && next !== '\n' && next !== '\r')) {
    scanQuoted(scan, "'", false)
    return
  }
  scan.advanceOne()
  if (isIdentStart(scan.at())) {
    while (isIdentPart(scan.at())) scan.advanceOne()
  }
  scan.prev = ''
}

function scanCppRaw(scan: Scan): boolean {
  const text = scan.text
  if (scan.at() !== 'R' || text[scan.i + 1] !== '"') return false
  let j = scan.i + 2
  let delim = ''
  while (j < scan.n && text[j] !== '(') {
    const c = text[j]
    if (c === '\n' || c === '\r' || c === '"' || c === ')' || c === ' ') return false
    delim += c
    j++
    if (delim.length > 16) return false
  }
  if (text[j] !== '(') return false
  const end = ')' + delim + '"'
  const at = text.indexOf(end, j + 1)
  const from = scan.snapshot()
  const target = at < 0 ? scan.n : at + end.length
  while (scan.i < target) scan.advanceOne()
  scan.emitTo(from, 'string')
  scan.prev = ''
  return true
}

function scanCSharpString(scan: Scan): boolean {
  const text = scan.text
  const c0 = scan.at()
  const c1 = text[scan.i + 1] ?? ''
  const c2 = text[scan.i + 2] ?? ''
  let verbatim = false
  if ((c0 === '@' && c1 === '$' && c2 === '"') || (c0 === '$' && c1 === '@' && c2 === '"') || (c0 === '@' && c1 === '"')) {
    verbatim = true
  } else if (!(c0 === '$' && c1 === '"')) {
    return false
  }
  const from = scan.snapshot()
  while (scan.at() !== '"') scan.advanceOne()
  scan.advanceOne()
  while (scan.i < scan.n) {
    const c = scan.at()
    if (!verbatim && (c === '\n' || c === '\r')) break
    if (!verbatim && c === '\\') {
      scan.advanceOne()
      if (scan.i < scan.n) scan.advanceOne()
      continue
    }
    scan.advanceOne()
    if (c === '"') {
      if (verbatim && scan.at() === '"') {
        scan.advanceOne()
        continue
      }
      break
    }
  }
  scan.emitTo(from, 'string')
  scan.prev = ''
  return true
}

function tryString(scan: Scan, profile: Profile): boolean {
  const c = scan.at()
  const text = scan.text
  if (profile.quotes === 'rust' && scanRustRaw(scan)) return true
  if (profile.cppRaw && scanCppRaw(scan)) return true
  if (profile.quotes === 'cs' && scanCSharpString(scan)) return true
  if (profile.triple !== 'no' && (c === '"' || c === "'")) {
    const triple = c === '"' ? '"""' : "'''"
    const allow = profile.triple === 'both' || c === '"'
    if (allow && text.startsWith(triple, scan.i)) {
      scanTriple(scan, triple)
      return true
    }
  }
  if (profile.quotes === 'go' && c === '`') {
    scanRawBacktick(scan)
    return true
  }
  if (profile.template && c === '`') {
    scanTemplate(scan, profile)
    return true
  }
  if (profile.quotes === 'rust' && c === "'") {
    scanRustTick(scan)
    return true
  }
  if (c === '"' || c === "'") {
    scanQuoted(scan, c, false)
    return true
  }
  return false
}

function tryNumber(scan: Scan): boolean {
  if (!/[0-9]/.test(scan.at())) return false
  const from = scan.snapshot()
  const text = scan.text
  const next = text[scan.i + 1] ?? ''
  if (scan.at() === '0' && (next === 'x' || next === 'X' || next === 'b' || next === 'B' || next === 'o' || next === 'O')) {
    scan.advanceOne()
    scan.advanceOne()
    while (scan.i < scan.n && /[0-9a-fA-F_]/.test(scan.at())) scan.advanceOne()
  } else {
    while (scan.i < scan.n && /[0-9_]/.test(scan.at())) scan.advanceOne()
    if (scan.at() === '.' && /[0-9]/.test(text[scan.i + 1] ?? '')) {
      scan.advanceOne()
      while (scan.i < scan.n && /[0-9_]/.test(scan.at())) scan.advanceOne()
    }
    if (scan.at() === 'e' || scan.at() === 'E') {
      const n1 = text[scan.i + 1] ?? ''
      const n2 = text[scan.i + 2] ?? ''
      if (/[0-9]/.test(n1) || ((n1 === '+' || n1 === '-') && /[0-9]/.test(n2))) {
        scan.advanceOne()
        if (scan.at() === '+' || scan.at() === '-') scan.advanceOne()
        while (scan.i < scan.n && /[0-9_]/.test(scan.at())) scan.advanceOne()
      }
    }
  }
  scan.emitTo(from, 'number')
  scan.prev = ''
  return true
}

function readWord(scan: Scan, profile: Profile): void {
  const from = scan.snapshot()
  let word = ''
  while (isIdentPart(scan.at())) {
    word += scan.at()
    scan.advanceOne()
  }
  const ahead = peekNext(scan, profile)
  const call = ahead.startsWith('(')
  const macro = profile.macro && ahead.startsWith('!') && ahead[1] !== '='
  // get / set 只有在访问器位置才是关键字；get() 这种调用按函数上色。
  const contextualCall = (word === 'get' || word === 'set') && (call || macro)
  let kind: SyntaxKind | null = null
  if (profile.types.has(word)) kind = 'type'
  else if (profile.keywords.has(word) && !contextualCall) kind = 'keyword'
  else if (scan.prev === 'new' || profile.typeIntro.has(scan.prev)) kind = 'type'
  else if (call || macro || profile.funcIntro.has(scan.prev)) kind = 'function'
  else if (profile.pascal && isPascal(word)) kind = 'type'
  if (kind) scan.emitTo(from, kind)
  scan.prev = word
}

function scanCode(scan: Scan, profile: Profile, stopBrace: boolean): void {
  let depth = 0
  while (scan.i < scan.n) {
    if (stopBrace && depth === 0 && scan.at() === '}') return
    if (tryComment(scan, profile)) continue
    if (tryString(scan, profile)) continue
    if (tryNumber(scan)) continue
    if (profile.dollar && scan.at() === '$') {
      scan.advanceOne()
      while (isIdentPart(scan.at())) scan.advanceOne()
      scan.prev = ''
      continue
    }
    if (isIdentStart(scan.at())) {
      readWord(scan, profile)
      continue
    }
    if (scan.at() === ' ' || scan.at() === '\t' || scan.at() === '\r' || scan.at() === '\n') {
      scan.advanceOne()
      continue
    }
    if (scan.at() === '{') {
      depth++
      scan.prev = ''
      scan.advanceOne()
      continue
    }
    if (scan.at() === '}') {
      if (depth > 0) depth--
      scan.prev = ''
      scan.advanceOne()
      continue
    }
    scan.prev = ''
    scan.advanceOne()
  }
}

export function syntaxSpans(language: string, text: string): SyntaxSpan[] {
  const found = PROFILES[language]
  if (!found || !text) return []
  const scan = new Scan(text, [])
  scanCode(scan, found, false)
  return scan.spans
}

export function syntaxLabels(language: string, text: string): string[] {
  const lines = text.split(/\r?\n/)
  return syntaxSpans(language, text).map((span) => {
    const line = lines[span.line] ?? ''
    return `${line.slice(span.start, span.start + span.length)}:${span.kind}`
  })
}

export function encodeSyntaxSpans(spans: SyntaxSpan[]): Uint32Array {
  const data = new Uint32Array(spans.length * 5)
  let prevLine = 0
  let prevStart = 0
  for (let i = 0; i < spans.length; i++) {
    const span = spans[i]
    const deltaLine = span.line - prevLine
    const deltaStart = deltaLine === 0 ? span.start - prevStart : span.start
    const at = i * 5
    data[at] = deltaLine
    data[at + 1] = deltaStart
    data[at + 2] = span.length
    data[at + 3] = SYNTAX_KINDS.indexOf(span.kind)
    data[at + 4] = 0
    prevLine = span.line
    prevStart = span.start
  }
  return data
}
