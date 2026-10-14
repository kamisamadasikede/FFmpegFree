#!/usr/bin/env node
/**
 * check-i18n（契约 §6.20.2）
 * 1) A 期范围内的 .vue/.ts 不得残留 CJK 字面量（注释除外；白名单见 i18n-allow.json）
 * 2) zh-CN / en / ja key 集合与插值参数名一致
 * 3) 代码里 t('literal') 的 key 必须存在于 zh-CN（errors. 前缀动态 key 放行）
 */
import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('..', import.meta.url))
const scope = JSON.parse(readFileSync(join(root, 'scripts/i18n-scope.json'), 'utf8'))
const allow = JSON.parse(readFileSync(join(root, 'scripts/i18n-allow.json'), 'utf8'))
const allowExact = new Set(allow.exactStrings || [])
const nativeNames = new Set(allow.nativeNames || ['简体中文', '日本語'])

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
  // very small glob: ** and *
  const esc = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*\*/g, '::DS::').replace(/\*/g, '[^/]*').replace(/::DS::/g, '.*')
  return new RegExp('^' + esc + '$').test(rel.replace(/\\/g, '/'))
}

function inScope(rel) {
  const r = rel.replace(/\\/g, '/')
  for (const g of scope.excludeGlobs || []) {
    if (matchGlob(r, g) || matchGlob(r, g.replace(/^\*\*\//, ''))) return false
    // also match basename patterns
    if (g.includes('**/') && matchGlob(r, g)) return false
  }
  if (r.endsWith('.check.ts') || /Mock/.test(r) || /catDevSim\.ts$/.test(r) || /\/sim\.ts$/.test(r) || /\/views\/dev\//.test(r)) return false
  for (const g of scope.include || []) {
    const pat = g.endsWith('/**') ? g.slice(0, -3) : g
    if (g.endsWith('/**')) {
      if (r === pat || r.startsWith(pat + '/')) return true
    } else if (g.includes('*')) {
      if (matchGlob(r, g) || matchGlob(r.split('/').pop(), g.split('/').pop())) return true
      // src/api/cat*.ts
      const base = g.split('/').pop()
      if (base.includes('*') && r.startsWith(g.slice(0, g.length - base.length)) && matchGlob(r.split('/').pop(), base)) return true
    } else if (r === g) return true
  }
  return false
}

function stripComments(src, isVue) {
  // For vue: split template/script; strip HTML comments and JS comments
  if (!isVue) {
    return src
      .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(HAN, ' '))
      .replace(/(^|[^:])\/\/.*$/gm, (m) => m.replace(HAN, ' '))
  }
  return src
    .replace(/<!--([\s\S]*?)-->/g, (m) => m.replace(HAN, ' '))
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(HAN, ' '))
    .replace(/(^|[^:])\/\/.*$/gm, (m) => m.replace(HAN, ' '))
}

function collectFiles() {
  const all = walk(join(root, 'src'))
  return all.filter((p) => inScope(relative(root, p).replace(/\\/g, '/')))
}

// ---- rule 1: CJK literals ----
const strRe = /(['"`])((?:\\.|(?!\1)[^\\])*?)\1/g
for (const file of collectFiles()) {
  const rel = relative(root, file).replace(/\\/g, '/')
  let src = readFileSync(file, 'utf8')
  src = stripComments(src, file.endsWith('.vue'))
  // skip locales import paths etc.
  let m
  const re = /(['"])((?:\\.|(?!\1)[^\\])*?)\1|`((?:\\.|[^`\\$]|\$(?!\{)|\$\{[^}]*\})*)`/gs
  while ((m = re.exec(src))) {
    const raw = m[2] != null ? m[2] : m[3]
    if (raw == null) continue
    const s = raw.replace(/\\n/g, '\n').replace(/\\'/g, "'").replace(/\\"/g, '"')
    if (!HAN.test(s)) continue
    if (allowExact.has(s) || nativeNames.has(s)) continue
    // template with only allowlisted? still fail
    // skip empty
    if (!s.trim()) continue
    const line = src.slice(0, m.index).split('\n').length
    fails.push(`${rel}:${line}: CJK literal ${JSON.stringify(s.slice(0, 60))}`)
  }
}

// ---- rule 2: key parity + interpolation params ----
function flatKeys(obj, prefix = '', out = {}) {
  if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
    for (const [k, v] of Object.entries(obj)) flatKeys(v, prefix ? prefix + '.' + k : k, out)
  } else out[prefix] = obj
  return out
}
function paramsOf(s) {
  if (typeof s !== 'string') return []
  const set = new Set()
  for (const m of s.matchAll(/\{(\w+)\}/g)) set.add(m[1])
  return [...set].sort()
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

// ---- rule 3: t('literal') exists ----
const tRe = /\bt\(\s*['"]([^'"]+)['"]/g
for (const file of collectFiles()) {
  const rel = relative(root, file).replace(/\\/g, '/')
  const src = readFileSync(file, 'utf8')
  let m
  while ((m = tRe.exec(src))) {
    const key = m[1]
    if (key.startsWith('errors.') && key.includes('${')) continue
    // dynamic errors.CODE allowed only as prefix with no further restriction if contains + 
    if (!(key in zh)) fails.push(`${rel}: t('${key}') not in zh-CN`)
  }
}

if (fails.length) {
  console.error(fails.slice(0, 80).join('\n'))
  if (fails.length > 80) console.error(`… and ${fails.length - 80} more`)
  process.exitCode = 1
} else {
  console.log(`i18n check passed (${zk.size} keys, A-scope)`)
}
