/** 推流地址 + 推流码 → 完整地址（推流码为空时原样返回） */
export function joinPushUrl(base: string, key: string): string {
  const b = base.trim()
  const k = key.trim()
  if (!k) return b
  return `${b.replace(/\/+$/, '')}/${k.replace(/^\/+/, '')}`
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

export interface PushUrlInfo {
  scheme: 'rtmp' | 'rtmps' | 'srt'
  host: string
  port: number
  /** 脱敏后的地址，可以直接显示 */
  redacted: string
}

export type PushUrlCheck =
  | { ok: true; info: PushUrlInfo; normalized: string }
  | { ok: false; kind: 'format' | 'protocol' | 'port' | 'mode' | 'app'; message: string }

const PUSH_SCHEMES = ['rtmp', 'rtmps', 'srt']
const DEFAULT_PORT: Record<string, number> = { rtmp: 1935, rtmps: 443 }
const BAD_CHARS = /[\s\u0000-\u001f\u007f|\\"']/

/** 不支持的协议（rtsp、http-flv 等）在地址框下方的提示 */
export const PUSH_PROTOCOL_UNSUPPORTED_TEXT = '暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt'

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
  if (!raw) return { ok: false, kind: 'format', message: '请输入推流地址' }
  if (new TextEncoder().encode(raw).length > 2048 || BAD_CHARS.test(raw)) {
    return { ok: false, kind: 'format', message: '推流地址里有不能使用的字符，或者太长' }
  }
  const p = splitUrl(raw)
  if (!p) return { ok: false, kind: 'format', message: '推流地址格式不正确' }
  if (!PUSH_SCHEMES.includes(p.scheme)) return { ok: false, kind: 'protocol', message: PUSH_PROTOCOL_UNSUPPORTED_TEXT }
  if (!p.host || p.host === '[]') return { ok: false, kind: 'format', message: '推流地址缺少主机名' }
  let port = DEFAULT_PORT[p.scheme] ?? 0
  if (p.port) {
    const n = /^\d+$/.test(p.port) ? Number(p.port) : NaN
    if (!(n >= 1 && n <= 65535)) return { ok: false, kind: 'port', message: '端口需要在 1~65535 之间' }
    port = n
  } else if (p.scheme === 'srt') {
    return { ok: false, kind: 'port', message: 'srt 地址必须写端口' }
  }
  if ((p.scheme === 'rtmp' || p.scheme === 'rtmps') && !p.path.replace(/\//g, '')) {
    return { ok: false, kind: 'app', message: '推流地址至少要有应用名，例如 rtmp://host/live' }
  }
  if (p.scheme === 'srt') {
    const mode = new URLSearchParams(p.query).get('mode')
    if (mode === 'listener' || mode === 'rendezvous') return { ok: false, kind: 'mode', message: 'srt 只支持 caller 模式' }
  }
  const host = p.host.toLowerCase()
  const defaultPort = DEFAULT_PORT[p.scheme]
  const normalized = `${p.scheme}://${p.userinfo ? p.userinfo + '@' : ''}${host}${port && port !== defaultPort ? ':' + port : ''}${p.path}${p.query ? '?' + p.query : ''}`
  return { ok: true, normalized, info: { scheme: p.scheme as PushUrlInfo['scheme'], host, port, redacted: redactPushUrl(raw) } }
}
