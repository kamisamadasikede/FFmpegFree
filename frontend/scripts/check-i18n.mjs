#!/usr/bin/env node
/**
 * check-i18n（契约 §6.20.2）
 * A 期：仅 scripts/i18n-scope.json 列出的路径查 CJK 字面量；三语 key/参数一致；t('…') 字面量 key 必须在 zh-CN。
 */
import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('..', import.meta.url))
const scope = JSON.parse(readFileSync(join(root, 'scripts/i18n-scope.json'), 'utf8'))
const allow = JSON.parse(readFileSync(join(root, 'scripts/i18n-allow.json'), 'utf8'))
const allowExact = new Set(allow.exactStrings || [])
const nativeNames = new Set(allow.nativeNames || ['简体中文', '日本語'])
const substrAllow = allow.substringAllow || []
const HAN = /[\u3400-\u9fff]/
const fails = []

function walk(dir, out = []) {
  if (!existsSync(dir)) return out
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    const st = statSync(p)
    if (st.isDirectory()) walk(p, out)
    else if (/\.(vue|ts)$/.test(name)) out.push(p)
  }
  return out
}

function matchGlob(rel, pattern) {
  const esc = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*\*/g, '::DS::').replace(/\*/g, '[^/]*').replace(/::DS::/g, '.*')
  return new RegExp('^' + esc + '$').test(rel.replace(/\\/g, '/'))
}

function inScope(rel) {
  const r = rel.replace(/\\/g, '/')
  if (r.endsWith('.check.ts') || /Mock/.test(r) || /catDevSim\.ts$/.test(r) || /\/sim\.ts$/.test(r) || /\/views\/dev\//.test(r)) return false
  for (const g of scope.excludeGlobs || []) {
    if (matchGlob(r, g)) return false
  }
  for (const g of scope.include || []) {
    if (g.endsWith('/**')) {
      const pat = g.slice(0, -3)
      if (r === pat || r.startsWith(pat + '/')) return true
    } else if (g.includes('*')) {
      if (matchGlob(r, g)) return true
    } else if (r === g) return true
  }
  return false
}

/** Collect string/template literals, skipping comments and regex literals. */
function literals(src) {
  const out = []
  let i = 0
  const n = src.length
  let prev = ''
  const isRegexContext = () => /[=(:,!&|?{};/\n]$/.test(prev) || prev === ''
  while (i < n) {
    if (src.startsWith('<!--', i)) {
      const j = src.indexOf('-->', i + 4)
      i = j < 0 ? n : j + 3
      continue
    }
    if (src[i] === '/' && src[i + 1] === '/') {
      while (i < n && src[i] !== '\n') i++
      continue
    }
    if (src[i] === '/' && src[i + 1] === '*') {
      const j = src.indexOf('*/', i + 2)
      i = j < 0 ? n : j + 2
      continue
    }
    // regex literal
    if (src[i] === '/' && isRegexContext()) {
      let j = i + 1, esc = false, cls = false
      while (j < n) {
        const c = src[j]
        if (esc) { esc = false; j++; continue }
        if (c === '\\') { esc = true; j++; continue }
        if (c === '[') { cls = true; j++; continue }
        if (c === ']' && cls) { cls = false; j++; continue }
        if (c === '/' && !cls) { j++; break }
        if (c === '\n') break
        j++
      }
      while (j < n && /[a-z]/i.test(src[j])) j++ // flags
      prev = '/'
      i = j
      continue
    }
    const c = src[i]
    if (c === '"' || c === "'") {
      let j = i + 1, v = ''
      while (j < n && src[j] !== c && src[j] !== '\n') {
        if (src[j] === '\\') { v += src[j + 1] ?? ''; j += 2 } else v += src[j++]
      }
      out.push({ value: v, index: i })
      prev = c
      i = j + 1
      continue
    }
    if (c === '`') {
      let j = i + 1, v = '', esc = false
      while (j < n) {
        if (esc) { esc = false; j++; continue }
        if (src[j] === '\\') { esc = true; j++; continue }
        if (src[j] === '`') break
        if (src[j] === '$' && src[j + 1] === '{') {
          let d = 0, q = j + 1
          while (q < n) {
            if (src[q] === '{') d++
            else if (src[q] === '}') { d--; if (d === 0) break }
            q++
          }
          v += '{…}'
          j = q + 1
          continue
        }
        v += src[j++]
      }
      out.push({ value: v, index: i })
      prev = '`'
      i = j + 1
      continue
    }
    if (!/\s/.test(c)) prev = c
    i++
  }
  return out
}

function collectFiles() {
  return walk(join(root, 'src')).filter((p) => inScope(relative(root, p).replace(/\\/g, '/')))
}

for (const file of collectFiles()) {
  const rel = relative(root, file).replace(/\\/g, '/')
  const src = readFileSync(file, 'utf8')
  for (const { value: s, index } of literals(src)) {
    if (!HAN.test(s)) continue
    if (allowExact.has(s) || nativeNames.has(s)) continue
    if (substrAllow.some((x) => s.includes(x))) continue
    const line = src.slice(0, index).split('\n').length
    fails.push(`${rel}:${line}: CJK literal ${JSON.stringify(s.slice(0, 80))}`)
  }
}

function flatKeys(obj, prefix = '', out = {}) {
  if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
    for (const [k, v] of Object.entries(obj)) flatKeys(v, prefix ? prefix + '.' + k : k, out)
  } else out[prefix] = obj
  return out
}
function paramsOf(s) {
  if (typeof s !== 'string') return []
  return [...new Set([...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort()
}
const zh = flatKeys(JSON.parse(readFileSync(join(root, 'src/locales/zh-CN.json'), 'utf8')))
const en = flatKeys(JSON.parse(readFileSync(join(root, 'src/locales/en.json'), 'utf8')))
const ja = flatKeys(JSON.parse(readFileSync(join(root, 'src/locales/ja.json'), 'utf8')))
const zk = new Set(Object.keys(zh)), ek = new Set(Object.keys(en)), jk = new Set(Object.keys(ja))
for (const k of zk) {
  if (!ek.has(k)) fails.push(`en missing key ${k}`)
  if (!jk.has(k)) fails.push(`ja missing key ${k}`)
  else {
    const pz = paramsOf(zh[k]).join(','), pe = paramsOf(en[k]).join(','), pj = paramsOf(ja[k]).join(',')
    if (pz !== pe) fails.push(`en params differ for ${k}: zh[${pz}] en[${pe}]`)
    if (pz !== pj) fails.push(`ja params differ for ${k}: zh[${pz}] ja[${pj}]`)
  }
}
for (const k of ek) if (!zk.has(k)) fails.push(`zh-CN missing key present in en: ${k}`)
for (const k of jk) if (!zk.has(k)) fails.push(`zh-CN missing key present in ja: ${k}`)

const tRe = /\bt\(\s*['"]([^'"]+)['"]/g
for (const file of collectFiles()) {
  const rel = relative(root, file).replace(/\\/g, '/')
  const src = readFileSync(file, 'utf8')
  let m
  while ((m = tRe.exec(src))) {
    const key = m[1]
    if (!(key in zh)) fails.push(`${rel}: t('${key}') not in zh-CN`)
  }
}

if (fails.length) {
  console.error(fails.slice(0, 100).join('\n'))
  if (fails.length > 100) console.error(`… and ${fails.length - 100} more`)
  process.exitCode = 1
} else {
  console.log(`i18n check passed (${zk.size} keys, A-scope)`)
}
