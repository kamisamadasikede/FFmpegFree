// 错误文案表：产品经理定稿、老板冻结，改文案要先走评审。
// 展示形态见设计原型 proto/errors.html：overlay 用于播放器区域，inline 用于表单，任务中心失败行用 ErrorLine。
import type { IconName } from '../components/icon/icons'
import { parseDetailHead } from '../api/call'

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
  /** 按钮前的图标；原型里只有“重试”带 refresh 图标，其余按钮不带 */
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

const RETRY: ErrorButton = { label: '重试', action: 'retry', icon: 'refresh' }
const VIEW_LOG: ErrorButton = { label: '查看日志', action: 'viewLog' }

function overlay(title: string, description: string, extra: Partial<ErrorMessage> = {}): ErrorMessage {
  return { title, description, style: 'overlay', primary: RETRY, secondary: VIEW_LOG, ...extra }
}

export const errorMessages: Record<ErrorCode, ErrorMessage> = {
  LIVE_URL_INVALID: {
    title: '流地址不正确',
    description: '请输入正确的推流地址，例如 rtmp://、rtmps:// 或 srt:// 开头。',
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
    // SRT 连接失败：后端统一判 LIVE_CONNECT_FAILED，页面把产品文案作为 message 传进来时替换说明
    // 直播连接失败：页面按 detail 首行 scheme= 选好文案（liveConnectFailedText）后作为 message 传进来，替换说明
    if (code === 'LIVE_CONNECT_FAILED' && isLiveConnectFailedText(fallbackMessage)) {
      return { ...errorMessages[code], description: fallbackMessage as string, code, known: true }
    }
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
// 后端 v0.9（ConvertService）起，磁盘写满发 CONVERT_DISK_FULL（原来归在 IO_ERROR）。

/** 任务中心 / 转换页失败行上的操作。changeOutput = 选新文件夹后用原参数重新提交（仅 convert 任务，见 api/convert.ts） */
export type TaskErrorAction = 'retry' | 'changeOutput' | 'viewLog'

export interface TaskErrorMessage {
  title: string
  description: string
  /** 按显示顺序 */
  actions: TaskErrorAction[]
}

export type TaskErrorCode = 'CONVERT_DISK_FULL' | 'PROBE_FAILED' | 'CANCELED'

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
  // 后端因应用退出（根 ctx 取消）取消调用时返回：不是失败，不提供重试，展示为中性样式
  CANCELED: {
    title: '已取消',
    description: '操作已取消。',
    actions: [],
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
    const byScheme = code === 'LIVE_CONNECT_FAILED' && isLiveConnectFailedText(fallbackMessage)
    return { code, title: m.title, description: byScheme ? (fallbackMessage as string) : m.description, actions: DEFAULT_TASK_ACTIONS, known: true }
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
  CANCELED: '操作已取消。',
  UNSUPPORTED: '该任务暂不支持重试',
  TASK_CONFLICT: '操作冲突，请稍后再试', // Cancel / Remove 等不带 reason 的冲突；直播 Start* 的冲突见 taskConflictText
  NOT_FOUND: '找不到这个任务或文件，可能已被删除',
}

/** 取 toast 文案：有专门说明的码用说明，其余用后端 message */
export function actionErrorText(code: string, backendMessage: string, reason?: string): string {
  if (code === 'TASK_CONFLICT') return taskConflictText(reason)
  return ACTION_ERROR_TEXT[code] ?? (backendMessage || FALLBACK_DESCRIPTION)
}

// ---- TASK_CONFLICT 的 reason → 文案（契约 6.10：detail 首行 `reason=<值>`，稳定枚举，只追加不改名）----
// 追加新 reason 只需要在这张表里加一行。未知 reason、缺失 reason，以及 Cancel / Remove 等本来就不带 reason 的冲突，一律走 TASK_CONFLICT_GENERIC。
export const TASK_CONFLICT_GENERIC = '操作冲突，请稍后再试'
export const TASK_CONFLICT_REASON_TEXT: Record<string, string> = {
  max_sessions: '最多同时推 4 路',
  duplicate_url: '这个地址已经在推流',
  // 待产品定：屏幕推流可能新增“同时最多 1 路”，reason 值待定，定后在此追加一行
}

/** TASK_CONFLICT 的用户文案；reason 取自 AppError.reason（api/call.ts 解析） */
export function taskConflictText(reason?: string | null): string {
  return (reason && Object.prototype.hasOwnProperty.call(TASK_CONFLICT_REASON_TEXT, reason) && TASK_CONFLICT_REASON_TEXT[reason]) || TASK_CONFLICT_GENERIC
}

// ---- 直播页专用文案（产品 / 设计定稿）----
/** 直播任务停止后的状态文案：只看 Task.status（后端保证 succeeded 时 error 为空、canceled 时不带错误码），不看 error */
export const LIVE_STOP_TEXT = { succeeded: '已结束推流', canceled: '已强制停止' } as const
/** SRT 连接失败：后端统一判 LIVE_CONNECT_FAILED（无法区分服务器未开与口令错误），文案由前端负责 */
export const LIVE_SRT_CONNECT_FAILED_TEXT = '连接失败，请检查地址和口令是否正确'
/** RTMP / RTMPS 连接失败。⚠ 待产品定稿（架构师决定 3：先用这句） */
export const LIVE_RTMP_CONNECT_FAILED_TEXT = '连接失败，请检查推流地址是否正确、服务器是否在线'

/** LIVE_CONNECT_FAILED 的文案：scheme 来自 detail 首行 `scheme=rtmp|rtmps|srt`（AppError.scheme）。scheme 缺失 / 不认识返回 undefined，调用方用后端 message / 表里的通用说明 */
export const LIVE_CONNECT_FAILED_TEXT_BY_SCHEME: Record<string, string> = {
  srt: LIVE_SRT_CONNECT_FAILED_TEXT,
  rtmp: LIVE_RTMP_CONNECT_FAILED_TEXT,
  rtmps: LIVE_RTMP_CONNECT_FAILED_TEXT,
}
export function liveConnectFailedText(scheme?: string | null): string | undefined {
  return scheme && Object.prototype.hasOwnProperty.call(LIVE_CONNECT_FAILED_TEXT_BY_SCHEME, scheme) ? LIVE_CONNECT_FAILED_TEXT_BY_SCHEME[scheme] : undefined
}
function isLiveConnectFailedText(text?: string | null): boolean {
  return !!text && Object.values(LIVE_CONNECT_FAILED_TEXT_BY_SCHEME).includes(text)
}

/**
 * 直播任务 / 调用失败 → 遮罩和任务行上显示的说明（作为 message 传给 ErrorOverlay / ErrorLine）。
 * LIVE_CONNECT_FAILED：按 detail 首行 scheme= 选 SRT / RTMP 文案；detail 没有 scheme 时用 fallbackScheme（脱敏 params.url 或页面地址的 scheme，只是兜底）。
 * 其余码原样返回后端 message。
 */
export function liveFailureMessage(err: { code?: string | null; message?: string | null; detail?: string | null }, fallbackScheme?: string | null): string {
  if (err.code === 'LIVE_CONNECT_FAILED') {
    const text = liveConnectFailedText(parseDetailHead(err.detail ?? undefined).scheme ?? fallbackScheme)
    if (text) return text
  }
  return err.message ?? ''
}

/** 从任务 params 里取脱敏地址的 scheme（兜底用；架构师决定 3：不再作为主要依据） */
export function schemeFromParams(params?: string | null): string | undefined {
  const m = /"url"\s*:\s*"(rtmps?|srt):\/\//i.exec(params ?? '')
  return m ? m[1].toLowerCase() : undefined
}

// ---- LIVE_URL_INVALID 的 reason → 文案（契约 6.10：detail 首行 `reason=<值>`，稳定枚举，只追加不改名）----
// 追加新 reason 只需要在这张表里加一行。未知值、缺失 reason 一律走 LIVE_URL_INVALID_GENERIC。
export const LIVE_PROTOCOL_UNSUPPORTED_TEXT = '暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt'
/** ⚠ malformed / missing_host / param_not_allowed 的文案待产品定稿，先共用这一句 */
export const LIVE_URL_MALFORMED_TEXT = '推流地址格式不正确'
export const LIVE_URL_INVALID_GENERIC = '推流地址不正确'
export const LIVE_URL_INVALID_REASON_TEXT: Record<string, string> = {
  scheme_unsupported: LIVE_PROTOCOL_UNSUPPORTED_TEXT,
  malformed: LIVE_URL_MALFORMED_TEXT,
  missing_host: LIVE_URL_MALFORMED_TEXT,
  param_not_allowed: LIVE_URL_MALFORMED_TEXT,
}
/** LIVE_URL_INVALID 的用户文案；reason 取自 AppError.reason（api/call.ts 解析）；地址框失焦校验也用它 */
export function liveUrlInvalidText(reason?: string | null): string {
  return (reason && Object.prototype.hasOwnProperty.call(LIVE_URL_INVALID_REASON_TEXT, reason) && LIVE_URL_INVALID_REASON_TEXT[reason]) || LIVE_URL_INVALID_GENERIC
}
/** 开始前 ffmpeg 缺 srt / rtmps 协议：后端 UNSUPPORTED，detail 写缺哪个。文案待产品定稿 */
export const LIVE_FFMPEG_PROTOCOL_MISSING_TEXT = '当前 ffmpeg 不支持这种推流协议'
/** 选中屏幕推流时来源下方常驻的说明（12px、--ff-text-2、前置信息图标，不弹窗） */
export const LIVE_SCREEN_NO_AUDIO_TEXT = '屏幕推流暂不包含声音'

/**
 * 直播 Start* 同步返回的错误 → 页面上展示的一句话（走 ErrorLine，点“开始”之后才出现，不提前置灰按钮）。
 * 返回 null 表示不属于这里处理的情形（调用方走原来的遮罩 / 行内错误）。
 */
export function liveStartErrorLine(e: { code: string; message?: string; reason?: string; scheme?: string; detail?: string }, opts: { scheme?: string } = {}): { title: string; description: string } | null {
  switch (e.code) {
    case 'TASK_CONFLICT':
      return { title: '无法开始推流', description: taskConflictText(e.reason) }
    case 'UNSUPPORTED':
      return { title: '无法开始推流', description: LIVE_FFMPEG_PROTOCOL_MISSING_TEXT }
    case 'LIVE_CONNECT_FAILED': {
      // scheme 优先取 detail 首行（AppError.scheme），拿不到才用页面上地址的 scheme 兜底
      const text = liveConnectFailedText(e.scheme ?? opts.scheme)
      return text ? { title: '无法连接流服务器', description: text } : null
    }
    default:
      return null
  }
}

// ---- 文档页（DocService，契约 6.12）----
export const DOC_EXPERIMENTAL_LABEL = '实验性'
export const DOC_EXPERIMENTAL_NOTE = '仅提取文字，不保留图片和样式'
/** doc / xls / ppt、加密文档等 UNSUPPORTED */
export const DOC_FORMAT_UNSUPPORTED_TEXT = '暂不支持这种格式，请先另存为 docx、xlsx 或 pptx'

// ---- 添加文件时的探测失败（转换页文件行，MediaService.Probe 单项 error）----
// 与任务失败文案分开：这是「这个文件加不进来」，不是转换失败。
const PROBE_ERROR_TEXT: Record<string, string> = {
  CANCELED: '操作已取消。',
  PROBE_FAILED: '文件可能已损坏，或不是音视频文件。',
  NOT_FOUND: '找不到这个文件，可能已被移动或删除。',
  INVALID_ARGUMENT: '这是文件夹或不支持的路径，请选择音视频文件。',
  IO_ERROR: '没有读取这个文件的权限。',
  FFMPEG_NOT_FOUND: '需要先安装 ffmpeg 才能读取文件信息。',
}
export const PROBE_ERROR_TITLE = '无法读取这个文件'

/** 探测失败行的说明：已知码用上面的文案，其余用后端 message */
export function probeErrorText(code: string, backendMessage?: string): string {
  return PROBE_ERROR_TEXT[code] ?? ((backendMessage ?? '').trim() || FALLBACK_DESCRIPTION)
}

/** 提交失败（Submit 整体校验不通过）时没有对应到具体文件的错误标题 */
export const SUBMIT_ERROR_TITLE = '无法开始转换'

/**
 * 文档 UNSUPPORTED 的用户文案：doc / xls / ppt、加密文档、其他不支持的格式（含 csv / txt、odt、rtf…）统一用格式说明；
 * 超过 5000 页、没有可用 Unicode 字体这两种是别的原因，沿用后端 message，不误导用户去“另存为”。
 */
export function docUnsupportedText(backendMessage?: string, detail?: string): string {
  const other = /5000|页数|字体/.test(`${backendMessage ?? ''}\n${detail ?? ''}`)
  return other ? (backendMessage || FALLBACK_DESCRIPTION) : DOC_FORMAT_UNSUPPORTED_TEXT
}
