// 把 mpegts.js 的 error 事件、或浏览器 <video> 的 error 映射成错误码。纯函数，无副作用。
// 判断顺序：地址格式 → 连接超时 → 疑似跨域 → 其他网络/媒体错误。
import type { ErrorCode } from './errorMessages'

// mpegts.js 的 ErrorTypes / ErrorDetails 字符串值（src/player/player-errors.js、io/loader.js）
const TYPE_NETWORK = 'NetworkError'
const DETAIL_EXCEPTION = 'Exception'
const DETAIL_TIMEOUT = 'ConnectingTimeout'

/** mpegts.js `player.on(Mpegts.Events.ERROR, (type, detail, info) => ...)` 的三个参数 */
export interface MpegtsErrorInput {
  kind: 'mpegts'
  type: string
  detail: string
  /** info.code 是 HTTP 状态码；fetch 直接抛错时 mpegts 给 -1 */
  info?: { code?: number; msg?: string } | null
  url?: string
  /** 页面 origin，默认取 location.origin，测试时可传 */
  pageOrigin?: string
}

/** <video> 的 error 事件：video.error 的 code（1 中止 2 网络 3 解码 4 不支持）和 message */
export interface VideoErrorInput {
  kind: 'video'
  code?: number
  message?: string
  url?: string
  /** 拿得到状态码时再传（例如先 HEAD 探测过） */
  status?: number
  pageOrigin?: string
}

export type PlayerErrorInput = MpegtsErrorInput | VideoErrorInput

// 播放器/推流能接受的地址协议，必须带 :// 和主机名
const URL_RE = /^(rtmps?|rtsp|https?|srt|wss?):\/\/[^\s/?#]+/i

/** 地址格式是否可用。注意 new URL('http:/x') 会被浏览器宽松纠正，所以用正则 */
export function isValidStreamUrl(url: string | undefined | null): boolean {
  return typeof url === 'string' && URL_RE.test(url.trim())
}

function currentOrigin(): string | undefined {
  return typeof location !== 'undefined' ? location.origin : undefined
}

/** 只有 http(s)/ws(s) 地址会受浏览器跨域限制；协议、主机或端口不同即跨域 */
export function isCrossOrigin(url: string | undefined, pageOrigin: string | undefined = currentOrigin()): boolean {
  if (!url || !pageOrigin) return false
  try {
    const u = new URL(url.trim())
    if (!/^(https?|wss?):$/.test(u.protocol)) return false
    const page = new URL(pageOrigin)
    const norm = (p: string) => p.replace(/^ws/, 'http')
    return norm(u.protocol) !== norm(page.protocol) || u.host !== page.host
  } catch {
    return false
  }
}

// fetch 因跨域/网络不通失败时，浏览器抛 TypeError，各家消息不同
const FETCH_FAIL_RE = /failed to fetch|networkerror|load failed|typeerror|network request failed/i

/**
 * 映射规则（CORS 只能猜，不能确定，所以只在“跨域 且 状态码为 0”时才判 LIVE_CORS_BLOCKED）：
 * - 地址格式不对 → LIVE_URL_INVALID
 * - 连接超时 → LIVE_CONNECT_FAILED
 * - 网络错误，跨域，且状态码为 0（或 fetch 抛 TypeError 且没有 HTTP 状态）→ LIVE_CORS_BLOCKED
 * - 其余网络/媒体/其他错误 → LIVE_PLAY_FAILED
 */
export function mapPlayerError(input: PlayerErrorInput): ErrorCode {
  if (input.url !== undefined && !isValidStreamUrl(input.url)) return 'LIVE_URL_INVALID'

  if (input.kind === 'mpegts') {
    if (input.type === TYPE_NETWORK) {
      if (input.detail === DETAIL_TIMEOUT) return 'LIVE_CONNECT_FAILED'
      const code = input.info?.code
      const msg = input.info?.msg ?? ''
      const noStatus =
        code === 0 || (input.detail === DETAIL_EXCEPTION && (code === undefined || code === -1) && FETCH_FAIL_RE.test(msg))
      if (noStatus && isCrossOrigin(input.url, input.pageOrigin)) return 'LIVE_CORS_BLOCKED'
    }
    return 'LIVE_PLAY_FAILED'
  }

  // 浏览器 video 元素拿不到 HTTP 状态，只有调用方额外探测出 status === 0 才可能判成跨域
  if (input.status === 0 && input.code === 2 && isCrossOrigin(input.url, input.pageOrigin)) return 'LIVE_CORS_BLOCKED'
  return 'LIVE_PLAY_FAILED'
}
