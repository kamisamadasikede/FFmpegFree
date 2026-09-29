// 错误文案表：产品经理定稿、老板冻结，改文案要先走评审。
// 展示形态见设计原型 proto/errors.html：overlay 用于播放器区域，inline 用于表单，任务中心失败行用 ErrorLine。
import type { IconName } from '../components/icon/icons'

export type ErrorStyle = 'overlay' | 'inline'

export type ErrorCode =
  | 'LIVE_URL_INVALID'
  | 'LIVE_CONNECT_FAILED'
  | 'LIVE_PUSH_REJECTED'
  | 'LIVE_PLAY_FAILED'
  | 'LIVE_CORS_BLOCKED'
  | 'LIVE_PUSH_INTERRUPTED'
  | 'FFMPEG_NOT_FOUND'
  | 'SCREEN_PERMISSION_DENIED'

export type ErrorButtonAction =
  | 'retry' // 触发 ErrorOverlay 的 retry 事件
  | 'viewLog' // 触发 viewLog 事件
  | 'route' // router.push(to)，同时触发 primary 事件

export interface ErrorButton {
  label: string
  action: ErrorButtonAction
  /** action 为 route 时的目标路径 */
  to?: string
  /** 原型的遮罩按钮没有图标；需要时在这里加 */
  icon?: IconName
}

export interface ErrorMessage {
  title: string
  /** 最多两行，遮罩里用 line-clamp 兜底 */
  description: string
  style: ErrorStyle
  /** 主按钮（右）。inline 没有按钮 */
  primary: ErrorButton | null
  /** 次按钮（左） */
  secondary: ErrorButton | null
  /** 是否同时出现在任务中心失败行（推流中断、连接失败） */
  taskRow?: boolean
}

const RETRY: ErrorButton = { label: '重试', action: 'retry' }
const VIEW_LOG: ErrorButton = { label: '查看日志', action: 'viewLog' }

function overlay(title: string, description: string, extra: Partial<ErrorMessage> = {}): ErrorMessage {
  return { title, description, style: 'overlay', primary: RETRY, secondary: VIEW_LOG, ...extra }
}

export const errorMessages: Record<ErrorCode, ErrorMessage> = {
  LIVE_URL_INVALID: {
    title: '流地址不正确',
    description: '请输入正确的流地址，例如 rtmp://、http://、srt:// 开头。',
    style: 'inline',
    primary: null,
    secondary: null,
  },
  LIVE_CONNECT_FAILED: overlay('无法连接流服务器', '请检查地址和网络是否正常。', { taskRow: true }),
  LIVE_PUSH_REJECTED: overlay('服务器拒绝了推流', '请检查串流密钥和推流地址是否正确。'),
  LIVE_PLAY_FAILED: overlay('拉流失败', '请检查地址和流服务器状态。'),
  LIVE_CORS_BLOCKED: overlay('播放被跨域限制拦截', '需要流服务器允许跨域访问。'),
  LIVE_PUSH_INTERRUPTED: overlay('推流已中断', '可以点击重试；开启自动重连后会自动重试。', { taskRow: true }),
  FFMPEG_NOT_FOUND: overlay('未找到 ffmpeg', '请到设置中安装，或手动指定 ffmpeg 路径。', {
    primary: { label: '去设置', action: 'route', to: '/settings' },
  }),
  // 原型里是弯引号“”，这里跟原型一致
  SCREEN_PERMISSION_DENIED: overlay('没有录屏权限', '请在系统设置的“隐私与安全性”里允许本应用录制屏幕，然后重试。'),
}

/** 未收录的错误码（INTERNAL 等）的兜底文案 */
export const FALLBACK_TITLE = '出错了'
export const FALLBACK_DESCRIPTION = '发生了未知错误，请查看日志了解详情。'

export interface ResolvedError extends ErrorMessage {
  /** 展示用的错误码；未传时为 INTERNAL */
  code: string
  /** false 表示走了兜底 */
  known: boolean
}

export function isKnownErrorCode(code: unknown): code is ErrorCode {
  return typeof code === 'string' && Object.prototype.hasOwnProperty.call(errorMessages, code)
}

/**
 * 按错误码取文案。已知码用表里的冻结文案（忽略 fallbackMessage）；
 * 未知码标题固定为“出错了”，描述取 fallbackMessage（后端 message）。
 */
export function resolveError(code?: string | null, fallbackMessage?: string | null): ResolvedError {
  if (isKnownErrorCode(code)) {
    return { ...errorMessages[code], code, known: true }
  }
  const message = (fallbackMessage ?? '').trim()
  return {
    code: code || 'INTERNAL',
    known: false,
    title: FALLBACK_TITLE,
    description: message || FALLBACK_DESCRIPTION,
    style: 'overlay',
    primary: RETRY,
    secondary: VIEW_LOG,
  }
}
