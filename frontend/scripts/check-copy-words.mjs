// node scripts/check-copy-words.mjs —— 界面文字里不能出现“ffmpeg”（不分大小写），统一叫“转换组件”（老板要求，2026-10-08）。
// 包 24 N3：已下线的功能名“剪辑”也不能出现在界面文字里（旧记录类型叫“旧版导出”）。
// 只查用户能看到 / 听到的文字：.vue 模板里的文字节点、给人看的静态属性（title、placeholder、aria-label…）、
// 以及 .vue / .ts 里像文案的字符串字面量（含中文或全角标点、带空格的句子、或者挂在 label / title / text… 这类键上）。
// 不查：注释、import、标识符、正则字面量、事件名 / 类型 id（如 'ffmpeg:status'、'ffmpeg_install'）、*.check.ts 测试文件。
// 产品名 FFmpegFree 不算。确实需要保留的行，在同一行写注释 copy-check-ignore。失败时退出码 1。
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const BANNED_BASE = /ffmpeg|剪辑/i
// 契约 v0.26 / 设计 v0.2 §四：文档组件在界面上只叫「文档组件」，LibreOffice 只允许出现在 Linux 的这两句（整句放行，别的写法都算违规）
const LIBRE = /libreoffice|soffice/i
export const LIBRE_ALLOWED = new Set([
  '请先在系统里安装 LibreOffice，然后重启应用。',
  '系统里的 LibreOffice 版本太旧，请升级到 7.2 或更高版本，然后重启应用。',
])
const BANNED = { test: (s) => BANNED_BASE.test(s) || (LIBRE.test(s) && !LIBRE_ALLOWED.has(s.trim())) }
const bannedAt = (s) => { const a = s.search(BANNED_BASE); return a >= 0 ? a : Math.max(0, s.search(LIBRE)) }
// 契约 v0.25.3：已知错误码和 reason= 不能出现在界面文案里。注释、比较用的码、detail 载荷不查。
const KNOWN_CODES = ['INVALID_ARGUMENT','NOT_FOUND','TASK_CONFLICT','IO_ERROR','CANCELED','UNSUPPORTED','INTERNAL','FFMPEG_NOT_FOUND','PROBE_FAILED','PROCESS_FAILED','CONVERT_DISK_FULL','UNSUPPORTED_PLATFORM','LIVE_URL_INVALID','LIVE_CONNECT_FAILED','LIVE_PUSH_REJECTED','LIVE_PUSH_INTERRUPTED','SCREEN_PERMISSION_DENIED','LIVE_SOURCE_GONE','LIVE_PLAY_FAILED','LIVE_CORS_BLOCKED','PDF_PARSE_FAILED','DOC_COMPONENT_NOT_READY','DOC_DOWNLOAD_FAILED','DOC_CHECKSUM_FAILED','DOC_COMPONENT_INSTALL_FAILED','DOC_FORMAT_UNSUPPORTED','DOC_PDF_INPUT_UNSUPPORTED','DOC_ENCRYPTED','DOC_CORRUPT','DOC_TIMEOUT','DOC_COMPONENT_CRASHED','DOC_PRESENTATION_BUSY','DOC_ENGINE_BUSY','DOC_PDF_NO_TEXT','LANG_ASR_NOT_READY','LANG_ASR_EMPTY','LANG_ASR_FAILED','LANG_DOWNLOAD_FAILED','LANG_CHECKSUM_FAILED']
const CODE_LEAK = new RegExp('\\b(?:' + KNOWN_CODES.join('|') + ')\\b|reason=')
const RENDER_KEYS = new Set(['label','title','subtitle','text','description','placeholder','hint','note','tooltip','tip','empty','emptyText','confirmText','cancelText','ariaLabel','heading','caption','content','actionLabel','tag'])
const HAN = /[\u3400-\u9fff\u3000-\u303f\uff01-\uff5e\u2026\u201c\u201d]/
const USER_KEYS = new Set(['label', 'title', 'subtitle', 'text', 'description', 'desc', 'message', 'msg', 'tip', 'tooltip', 'hint', 'note', 'detail', 'placeholder', 'content', 'actionLabel', 'ariaLabel', 'tag', 'primary', 'secondary', 'heading', 'caption', 'empty', 'emptyText', 'confirmText', 'cancelText'])
const USER_ATTRS = /^(title|placeholder|aria-label|aria-description|aria-valuetext|alt|content|label|text|description|message|subtitle|heading|empty-text|confirm-button-text|cancel-button-text|tip|tip-[\w-]+|[\w-]*-text|[\w-]*-label)$/

const clean = (s) => s.replace(/FFmpegFree/g, '')
const isCopyLike = (s, key) => {
  const t = clean(s)
  if (!BANNED.test(t)) return false
  if (LIBRE.test(t) && !BANNED_BASE.test(t)) return HAN.test(t) || /\s/.test(t.trim()) || (key != null && USER_KEYS.has(key))
  return /剪辑/.test(t) || HAN.test(t) || /ffmpeg\s|\sffmpeg/i.test(t) || (key != null && USER_KEYS.has(key))
}

/** 极简 JS 词法：跳过注释和正则，收集字符串字面量（模板字符串只取 ${} 外的文字，${} 里递归）。返回 [{ value, start, key }] */
export function jsLiterals(src, base = 0) {
  const out = []
  let i = 0
  let prev = '' // 上一个有意义的字符，用来区分除号和正则
  let prevWord = ''
  const keyBefore = (pos) => {
    const m = /([A-Za-z_$][\w$]*|'[^']*'|"[^"]*")\s*:\s*$/.exec(src.slice(Math.max(0, pos - 60), pos))
    return m ? m[1].replace(/^['"]|['"]$/g, '') : null
  }
  while (i < src.length) {
    const c = src[i]
    const n = src[i + 1]
    if (c === '/' && n === '/') { while (i < src.length && src[i] !== '\n') i++; continue }
    if (c === '/' && n === '*') { const e = src.indexOf('*/', i + 2); i = e < 0 ? src.length : e + 2; continue }
    if (c === '"' || c === "'") {
      let j = i + 1, v = ''
      while (j < src.length && src[j] !== c && src[j] !== '\n') { if (src[j] === '\\') { v += src[j + 1] ?? ''; j += 2 } else v += src[j++] }
      out.push({ value: v, start: base + i, key: keyBefore(i) })
      i = j + 1; prev = c; prevWord = ''; continue
    }
    if (c === '`') {
      const key = keyBefore(i)
      let j = i + 1, v = ''
      const start = i
      while (j < src.length && src[j] !== '`') {
        if (src[j] === '\\') { v += src[j + 1] ?? ''; j += 2; continue }
        if (src[j] === '$' && src[j + 1] === '{') {
          // 找配对的 }，期间跳过嵌套字符串
          let depth = 1, k = j + 2
          while (k < src.length && depth > 0) {
            const ch = src[k]
            if (ch === '{') depth++
            else if (ch === '}') depth--
            else if (ch === '`' || ch === '"' || ch === "'") {
              const q = ch; k++
              while (k < src.length && src[k] !== q) { if (src[k] === '\\') k++; else if (q === '`' && src[k] === '$' && src[k + 1] === '{') { let d = 1; k += 2; while (k < src.length && d) { if (src[k] === '{') d++; else if (src[k] === '}') d--; k++ } continue } k++ }
            }
            k++
          }
          out.push(...jsLiterals(src.slice(j + 2, k - 1), base + j + 2))
          v += ' '
          j = k
          continue
        }
        v += src[j++]
      }
      out.push({ value: v, start: base + start, key })
      i = j + 1; prev = '`'; prevWord = ''; continue
    }
    if (c === '/') {
      const regexOk = prev === '' || '(,=:[!&|?{};+-*%<>~^'.includes(prev) || ['return', 'typeof', 'case', 'in', 'of', 'void'].includes(prevWord)
      if (regexOk) {
        let j = i + 1, cls = false
        while (j < src.length && src[j] !== '\n') {
          if (src[j] === '\\') { j += 2; continue }
          if (src[j] === '[') cls = true
          else if (src[j] === ']') cls = false
          else if (src[j] === '/' && !cls) break
          j++
        }
        j++
        while (/[a-z]/i.test(src[j] ?? '')) j++
        i = j; prev = ')'; prevWord = ''; continue
      }
    }
    if (/[A-Za-z_$0-9]/.test(c)) {
      let j = i
      while (j < src.length && /[\w$]/.test(src[j])) j++
      prevWord = src.slice(i, j); prev = 'a'; i = j; continue
    }
    if (!/\s/.test(c)) { prev = c; prevWord = '' }
    i++
  }
  return out
}

/** 在 .vue 模板里收集：文字节点、静态属性、绑定表达式 / 插值里的字符串 */
function templateHits(tpl, base, push) {
  let i = 0
  const text = (s, off) => {
    let k = 0
    while (k < s.length) {
      const a = s.indexOf('{{', k)
      const plain = a < 0 ? s.slice(k) : s.slice(k, a)
      if (BANNED.test(clean(plain))) push(off + k + bannedAt(clean(plain)), plain.trim())
      else if (CODE_LEAK.test(plain)) { CODE_LEAK.lastIndex = 0; push(off + k, plain.trim()) }
      else CODE_LEAK.lastIndex = 0
      if (a < 0) break
      const b = s.indexOf('}}', a + 2)
      const expr = s.slice(a + 2, b < 0 ? s.length : b)
      for (const l of jsLiterals(expr, off + a + 2)) if (isCopyLike(l.value, l.key) || leakHit(l.value, l.key, true)) push(l.start, l.value)
      k = b < 0 ? s.length : b + 2
    }
  }
  while (i < tpl.length) {
    if (tpl.startsWith('<!--', i)) { const e = tpl.indexOf('-->', i); i = e < 0 ? tpl.length : e + 3; continue }
    if (tpl[i] === '<') {
      let j = i + 1, q = ''
      while (j < tpl.length) { const ch = tpl[j]; if (q) { if (ch === q) q = '' } else if (ch === '"' || ch === "'") q = ch; else if (ch === '>') break; j++ }
      const tag = tpl.slice(i + 1, j)
      const re = /([:@#]?[\w\-.:[\]]+)\s*=\s*("([^"]*)"|'([^']*)')/g
      let m
      while ((m = re.exec(tag))) {
        const name = m[1]
        const val = m[3] ?? m[4] ?? ''
        const off = base + i + 1 + m.index + m[0].indexOf(val)
        const bare = name.replace(/^[:@#]/, '').replace(/^v-bind:/, '')
        const shownAttr = USER_ATTRS.test(bare)
        if (/^[:@#]|^v-/.test(name)) { for (const l of jsLiterals(val, off)) if (isCopyLike(l.value, l.key) || leakHit(l.value, l.key, shownAttr)) push(l.start, l.value) }
        else if (BANNED.test(clean(val)) && (USER_ATTRS.test(name) || HAN.test(val))) push(off, `${name}="${val}"`)
        else if (CODE_LEAK.test(val) && USER_ATTRS.test(name)) { CODE_LEAK.lastIndex = 0; push(off, `${name}="${val}"`) }
        else CODE_LEAK.lastIndex = 0
      }
      i = j + 1; continue
    }
    let j = tpl.indexOf('<', i)
    if (j < 0) j = tpl.length
    text(tpl.slice(i, j), base + i)
    i = j
  }
}

function leakHit(value, key, rendered) {
  if (!CODE_LEAK.test(value)) return false
  CODE_LEAK.lastIndex = 0
  return rendered || (key != null && RENDER_KEYS.has(key))
}

export function scanFile(path, src) {
  const hits = []
  const lines = src.split('\n')
  const lineOf = (off) => src.slice(0, off).split('\n').length
  const push = (off, s) => {
    const ln = lineOf(off)
    if (/copy-check-ignore/.test(lines[ln - 1] ?? '')) return
    hits.push({ line: ln, text: s })
  }
  if (path.endsWith('.vue')) {
    const t0 = src.indexOf('<template')
    const t1 = src.lastIndexOf('</template>')
    if (t0 >= 0 && t1 > t0) { const s = src.indexOf('>', t0) + 1; templateHits(src.slice(s, t1), s, push) }
    const re = /<script[^>]*>([\s\S]*?)<\/script>/g
    let m
    while ((m = re.exec(src))) { const off = m.index + m[0].indexOf(m[1]); for (const l of jsLiterals(m[1], off)) if (isCopyLike(l.value, l.key) || leakHit(l.value, l.key, false)) push(l.start, l.value) }
  } else {
    for (const l of jsLiterals(src)) if (isCopyLike(l.value, l.key) || leakHit(l.value, l.key, false)) push(l.start, l.value)
  }
  return hits
}

function walk(dir, acc = []) {
  for (const f of readdirSync(dir)) {
    const p = join(dir, f)
    if (statSync(p).isDirectory()) walk(p, acc)
    else if (/\.(vue|ts)$/.test(f) && !/\.check\.ts$/.test(f) && !/\.d\.ts$/.test(f)) acc.push(p)
  }
  return acc
}

export function runCopyWordCheck() {
  const root = fileURLToPath(new URL('..', import.meta.url))
  const fails = []
  // 自测：确保检查本身能抓到 / 不误报
  const selfFails = []
  const expectHit = (name, file, src, n) => { const h = scanFile(file, src).length; if (h !== n) selfFails.push(`自测失败：${name}（期望 ${n} 处，实际 ${h} 处）`) }
  expectHit('模板文字', 'a.vue', '<template><h2>FFmpeg</h2></template>', 1)
  expectHit('静态 title', 'a.vue', '<template><i title="ffmpeg 已就绪" class="ffmpeg-dot"></i></template>', 1)
  expectHit('绑定里的中文串', 'a.vue', `<template><b :tip="x ? '需要先安装 ffmpeg' : ''" @click="ffmpeg.open()">{{ ffmpeg.status }}</b></template>`, 1)
  expectHit('label 键', 'a.ts', "const a = { key: 'ffmpeg', label: 'FFmpeg' }", 1)
  expectHit('模板字符串只看文字', 'a.ts', 'const a = `下载中 ${Math.round(ffmpeg.install.progress * 100)}%`', 0)
  expectHit('模板字符串里的 ffmpeg 文字', 'a.ts', 'const a = `ffmpeg ${v}`', 1)
  expectHit('注释 / import / 正则 / 事件名 / 产品名', 'a.ts', "import { useFFmpegStore } from '@/stores/ffmpeg'\n// ffmpeg 已就绪\nconst r = /^ffmpeg\\s*退出码/\non('ffmpeg:status'); t = 'ffmpeg_install'; p = 'D:\\\\FFmpegFree\\\\输出'", 0)
  expectHit('已下线功能名', 'a.vue', `<template><span>{{ x }}</span><i title="剪辑导出"></i></template><script setup lang="ts">const L = { edit_export: '剪辑' }</script>`, 2)
  expectHit('忽略标记', 'a.ts', "const a = '需要 ffmpeg' // copy-check-ignore", 0)
  expectHit('模板里的错误码', 'a.vue', '<template><p>INTERNAL</p></template>', 1)
  expectHit('模板里的 reason=', 'a.vue', '<template><p>reason=whatever</p></template>', 1)
  expectHit('说明键里的 reason=', 'a.ts', "const a = { description: '出错了 reason=push' }", 1)
  expectHit('兜底文案本身', 'a.ts', "export const UNMAPPED_ERROR_TEXT = '出了点问题，请重试。'", 0)
  expectHit('比较和载荷不算文案', 'a.ts', "if (code === 'INTERNAL') throw new AppError('NOT_FOUND', '文件已被移动或删除', 'reason=file')\n// reason=push LIVE_SOURCE_GONE", 0)
  expectHit('映射表的键不是文案', 'a.ts', "const errorMessages = { INTERNAL: { title: '出错了', description: '请重试。' } }", 0)
  expectHit('LibreOffice 在界面文字里', 'a.vue', '<template><p>需要安装 LibreOffice</p></template>', 1)
  expectHit('LibreOffice 在文案常量里', 'a.ts', "const a = { text: '正在启动 LibreOffice…' }", 1)
  expectHit('LibreOffice 放行的 Linux 两句', 'a.ts', "const a = '请先在系统里安装 LibreOffice，然后重启应用。'\nconst b = '系统里的 LibreOffice 版本太旧，请升级到 7.2 或更高版本，然后重启应用。'", 0)
  expectHit('soffice 进程名在文案里', 'a.vue', '<template><i title="soffice 已退出"></i></template>', 1)
  expectHit('文档错误码不能当文案', 'a.vue', '<template><p>DOC_TIMEOUT</p></template>', 1)
  fails.push(...selfFails)
  for (const p of walk(join(root, 'src'))) {
    for (const h of scanFile(p, readFileSync(p, 'utf8'))) fails.push(`${relative(root, p)}:${h.line}  界面文字含禁用词（ffmpeg / 剪辑 / LibreOffice / 错误码 / reason=）：${h.text}`)
  }
  return fails
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  const fails = runCopyWordCheck()
  if (fails.length) { console.error(fails.join('\n')); process.exit(1) }
  console.log('界面文字检查通过：没有“ffmpeg”、“剪辑”、错误码和 reason=')
}
