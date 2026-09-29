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

// ---- 任务中心失败行（ErrorLine）专用文案 ----
// 冻结的八个直播 / 录屏错误码（上面的 errorMessages）原样沿用；这里只补任务中心自己的错误码。
// 后端目前把磁盘写满报成 IO_ERROR，还不会发 CONVERT_DISK_FULL；码名已由产品定稿，后端跟进后即生效。

/** 任务中心失败行上的操作。changeOutput 目前只发事件（见 TaskCenter.vue），还没有真正的换目录能力 */
export type TaskErrorAction = 'retry' | 'changeOutput' | 'viewLog'

export interface TaskErrorMessage {
  title: string
  description: string
  /** 按显示顺序 */
  actions: TaskErrorAction[]
}

export type TaskErrorCode = 'CONVERT_DISK_FULL' | 'PROBE_FAILED'

export const taskErrorMessages: Record<TaskErrorCode, TaskErrorMessage> = {
  CONVERT_DISK_FULL: {
    title: '磁盘空间不足',
    description: '输出位置的可用空间不够，请清理空间或换一个输出文件夹。',
    actions: ['retry', 'changeOutput', 'viewLog'],
  },
  PROBE_FAILED: {
    title: '无法读取输入文件',
    description: '文件可能已损坏，或不是音视频文件。',
    actions: ['retry', 'viewLog'],
  },
}

/** 任务中心里没有专属文案的错误码：标题固定，描述取后端 message */
export const TASK_FALLBACK_TITLE = '转换失败'
const DEFAULT_TASK_ACTIONS: TaskErrorAction[] = ['retry', 'viewLog']

export interface ResolvedTaskError {
  code: string
  title: string
  description: string
  actions: TaskErrorAction[]
  /** false 表示没有专属文案（走了 转换失败 兜底） */
  known: boolean
}

export function isTaskErrorCode(code: unknown): code is TaskErrorCode {
  return typeof code === 'string' && Object.prototype.hasOwnProperty.call(taskErrorMessages, code)
}

/**
 * 任务中心失败行的文案：
 * 1. 任务中心专属码（CONVERT_DISK_FULL）用表里的文案；
 * 2. 冻结的直播码（LIVE_PUSH_INTERRUPTED 等）用 errorMessages 里的文案；
 * 3. 其余一律 标题「转换失败」+ 后端 message 作描述，绝不出现「出错了」。
 */
export function resolveTaskError(code?: string | null, fallbackMessage?: string | null): ResolvedTaskError {
  if (isTaskErrorCode(code)) {
    return { code, ...taskErrorMessages[code], known: true }
  }
  if (isKnownErrorCode(code)) {
    const m = errorMessages[code]
    return { code, title: m.title, description: m.description, actions: DEFAULT_TASK_ACTIONS, known: true }
  }
  const message = (fallbackMessage ?? '').trim()
  return {
    code: code || 'INTERNAL',
    title: TASK_FALLBACK_TITLE,
    description: message || FALLBACK_DESCRIPTION,
    actions: DEFAULT_TASK_ACTIONS,
    known: false,
  }
}

// ---- 操作失败提示（toast / 行内）：按钮触发的 Bind 调用返回的 AppError → 用户可读的话 ----
// 与失败行的 ErrorLine 文案分开：这里是「点了某个按钮但没成功」，不是任务本身失败。
const ACTION_ERROR_TEXT: Record<string, string> = {
  UNSUPPORTED: '该任务暂不支持重试',
  TASK_CONFLICT: '任务状态已变化，请刷新后再试',
  NOT_FOUND: '找不到这个任务或文件，可能已被删除',
}

/** 取 toast 文案：有专门说明的码用说明，其余用后端 message */
export function actionErrorText(code: string, backendMessage: string): string {
  return ACTION_ERROR_TEXT[code] ?? (backendMessage || FALLBACK_DESCRIPTION)
}
