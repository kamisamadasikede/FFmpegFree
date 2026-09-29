import { liveUrlInvalidText } from '@/errors/errorMessages'

/** 推流地址 + 推流码 → 完整地址（推流码为空时原样返回） */
export function joinPushUrl(base: string, key: string): string {
  const b = base.trim()
  const k = key.trim()
  if (!k) return b
  return `${b.replace(/\/+$/, '')}/${k.replace(/^\/+/, '')}`
}

/**
 * 推流地址 + 推流码 / 口令 → 完整地址（“推流码 / 口令”是同一个输入框）。
 * rtmp / rtmps：接在地址后面作为流名（joinPushUrl）；srt：作为 passphrase 查询参数（已有查询串时用 &）。口令为空原样返回。
 * 结果只允许作为调用参数存在，不显示、不存储。
 */
export function composePushUrl(base: string, key: string): string {
  const b = base.trim()
  const k = key.trim()
  if (!k) return b
  if (/^srt:\/\//i.test(b)) return `${b}${b.includes('?') ? '&' : '?'}passphrase=${encodeURIComponent(k)}`
  return joinPushUrl(b, k)
}

/** 会话行 / 标题里显示的地址：后端脱敏结果里的 *** 一律显示成 ****（设计稿 v0.2），如 rtmp://host/app/****、srt://host:9000?passphrase=**** */
export function displayPushUrl(redacted: string): string {
  return (redacted ?? '').replace(/\*{3,}/g, '****')
}

/** 每行一个地址，去空行和首尾空格 */
export function parseTargets(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
}

// ───────────── 推流地址校验 / 脱敏（契约 v0.10 §4 LiveService「推流地址校验规则」「脱敏」，前端镜像）─────────────
// 后端才是权威：这里只用来在失焦时给出提示、给模拟层用，通过不代表后端一定接受。
// 完整地址（含推流码）只允许存在于调用参数和内存里：不写 localStorage / 日志 / console，列表和标题一律显示 redactPushUrl 的结果。

/** 后端 LIVE_URL_INVALID 的 detail 首行 `reason=` 取值（稳定枚举，只追加）；前端本地校验按同一套归类，模拟层用它 */
export type LiveUrlInvalidReason = 'scheme_unsupported' | 'malformed' | 'missing_host' | 'param_not_allowed'

export interface PushUrlInfo {
  scheme: 'rtmp' | 'rtmps' | 'srt'
  host: string
  port: number
  /** 脱敏后的地址，可以直接显示 */
  redacted: string
}

export type PushUrlCheck =
  | { ok: true; info: PushUrlInfo; normalized: string }
  | { ok: false; kind: 'format' | 'protocol' | 'port' | 'mode' | 'app'; message: string; reason: LiveUrlInvalidReason }

const PUSH_SCHEMES = ['rtmp', 'rtmps', 'srt']
const DEFAULT_PORT: Record<string, number> = { rtmp: 1935, rtmps: 443 }
const BAD_CHARS = /[\s\u0000-\u001f\u007f|\\"']/


interface Parts {
  scheme: string
  userinfo: string
  host: string
  port: string
  path: string
  query: string
}

function splitUrl(raw: string): Parts | null {
  const m = /^([A-Za-z][A-Za-z0-9+.-]*):\/\/([^/?#]*)([^?#]*)(?:\?([^#]*))?(?:#.*)?$/.exec(raw)
  if (!m) return null
  const auth = m[2]
  const at = auth.lastIndexOf('@')
  const userinfo = at >= 0 ? auth.slice(0, at) : ''
  const hostport = at >= 0 ? auth.slice(at + 1) : auth
  let host = hostport
  let port = ''
  if (hostport.startsWith('[')) {
    const end = hostport.indexOf(']')
    if (end < 0) return null
    host = hostport.slice(0, end + 1)
    port = hostport.slice(end + 1).replace(/^:/, '')
  } else {
    const i = hostport.lastIndexOf(':')
    if (i >= 0) {
      host = hostport.slice(0, i)
      port = hostport.slice(i + 1)
    }
  }
  return { scheme: m[1].toLowerCase(), userinfo, host, port, path: m[3] ?? '', query: m[4] ?? '' }
}

/** 契约的 RedactURL：用户信息 → ***@；rtmp/rtmps 只留应用名，流名 → ***；所有查询参数值 → ***；解析失败返回 <invalid-url>，绝不回显原文 */
export function redactPushUrl(raw: string): string {
  const p = splitUrl((raw ?? '').trim())
  if (!p || !p.host) return '<invalid-url>'
  let path = p.path
  if (p.scheme === 'rtmp' || p.scheme === 'rtmps') {
    const segs = path.split('/').filter(Boolean)
    path = segs.length ? '/' + segs[0] + (segs.length > 1 ? '/***' : '') : ''
  }
  const query = p.query
    ? '?' + p.query.split('&').filter(Boolean).map((kv) => kv.split('=')[0] + '=***').join('&')
    : ''
  return `${p.scheme}://${p.userinfo ? '***@' : ''}${p.host}${p.port ? ':' + p.port : ''}${path}${query}`
}

/** 校验推流地址（镜像契约规则 1~5）。kind: format=缺 scheme / 写法不对；protocol=协议不支持（rtsp、http 等）；port / mode 见规则 3、5 */
export function parsePushUrl(input: string): PushUrlCheck {
  const raw = (input ?? '').trim()
  if (!raw) return { ok: false, kind: 'format', reason: 'malformed', message: '请输入推流地址' }
  if (new TextEncoder().encode(raw).length > 2048 || BAD_CHARS.test(raw)) {
    return { ok: false, kind: 'format', reason: 'malformed', message: liveUrlInvalidText('malformed') }
  }
  const p = splitUrl(raw)
  if (!p) return { ok: false, kind: 'format', reason: 'malformed', message: liveUrlInvalidText('malformed') }
  if (!PUSH_SCHEMES.includes(p.scheme)) return { ok: false, kind: 'protocol', reason: 'scheme_unsupported', message: liveUrlInvalidText('scheme_unsupported') }
  if (!p.host || p.host === '[]') return { ok: false, kind: 'format', reason: 'missing_host', message: liveUrlInvalidText('missing_host') }
  let port = DEFAULT_PORT[p.scheme] ?? 0
  if (p.port) {
    const n = /^\d+$/.test(p.port) ? Number(p.port) : NaN
    if (!(n >= 1 && n <= 65535)) return { ok: false, kind: 'port', reason: 'malformed', message: liveUrlInvalidText('malformed') }
    port = n
  } else if (p.scheme === 'srt') {
    return { ok: false, kind: 'port', reason: 'malformed', message: liveUrlInvalidText('malformed') }
  }
  if ((p.scheme === 'rtmp' || p.scheme === 'rtmps') && !p.path.replace(/\//g, '')) {
    return { ok: false, kind: 'app', reason: 'malformed', message: liveUrlInvalidText('malformed') }
  }
  if (p.scheme === 'srt') {
    const mode = new URLSearchParams(p.query).get('mode')
    if (mode === 'listener' || mode === 'rendezvous') return { ok: false, kind: 'mode', reason: 'param_not_allowed', message: liveUrlInvalidText('param_not_allowed') }
  }
  const host = p.host.toLowerCase()
  const defaultPort = DEFAULT_PORT[p.scheme]
  const normalized = `${p.scheme}://${p.userinfo ? p.userinfo + '@' : ''}${host}${port && port !== defaultPort ? ':' + port : ''}${p.path}${p.query ? '?' + p.query : ''}`
  return { ok: true, normalized, info: { scheme: p.scheme as PushUrlInfo['scheme'], host, port, redacted: redactPushUrl(raw) } }
}
