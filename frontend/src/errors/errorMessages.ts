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
  | 'UNSUPPORTED_PLATFORM'

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
    description: '推流地址不可用，请检查后重新输入', // 同 LIVE_URL_INVALID_GENERIC；具体 reason 见 LIVE_URL_INVALID_REASON_TEXT
    style: 'inline',
    primary: null,
    secondary: null,
  },
  LIVE_CONNECT_FAILED: overlay('无法连接流服务器', '连接失败，请检查推流地址和推流码是否正确，以及网络是否通畅', { taskRow: true }),
  LIVE_PUSH_REJECTED: overlay('服务器拒绝了推流', '服务器拒绝了推流，请检查推流码是否有效，或是否已被其他推流占用'),
  LIVE_PLAY_FAILED: overlay('拉流失败', '请检查地址和流服务器状态。'),
  LIVE_CORS_BLOCKED: overlay('播放被跨域限制拦截', '需要流服务器允许跨域访问。'),
  LIVE_PUSH_INTERRUPTED: overlay('推流已中断', '可以点击重试；开启自动重连后会自动重试。', { taskRow: true }),
  FFMPEG_NOT_FOUND: overlay('未找到 ffmpeg', '请到设置中安装，或手动指定 ffmpeg 路径。', {
    primary: { label: '去设置', action: 'route', to: '/settings' },
  }),
  // 原型里是弯引号“”，这里跟原型一致
  // 产品经理定稿：两个码文案不混用（权限被拒 ≠ 平台不支持）
  SCREEN_PERMISSION_DENIED: overlay('没有录屏权限', '没有获得屏幕录制权限，请在系统设置中允许 FFmpegFree 录制屏幕后重试'),
  UNSUPPORTED_PLATFORM: overlay('无法进行屏幕推流', '当前系统暂不支持屏幕推流', { primary: null, secondary: null }),
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
  screen_busy: '屏幕推流同一时间只能有 1 路，请先停止当前的屏幕推流',
}

/** TASK_CONFLICT 的用户文案；reason 取自 AppError.reason（api/call.ts 解析） */
export function taskConflictText(reason?: string | null): string {
  return (reason && Object.prototype.hasOwnProperty.call(TASK_CONFLICT_REASON_TEXT, reason) && TASK_CONFLICT_REASON_TEXT[reason]) || TASK_CONFLICT_GENERIC
}

// ---- 直播页专用文案（产品 / 设计定稿）----
/** 直播任务停止后的状态文案：只看 Task.status（后端保证 succeeded 时 error 为空、canceled 时不带错误码），不看 error */
export const LIVE_STOP_TEXT = { succeeded: '已结束推流', canceled: '已强制停止' } as const
/** SRT 连接失败（产品经理定稿）：后端统一判 LIVE_CONNECT_FAILED（无法区分服务器未开与口令错误），文案由前端负责 */
export const LIVE_SRT_CONNECT_FAILED_TEXT = '连接失败，请检查地址和口令是否正确'
/** RTMP / RTMPS，以及 detail 缺 scheme 首行 / scheme 不认识时的连接失败文案（产品经理定稿） */
export const LIVE_RTMP_CONNECT_FAILED_TEXT = '连接失败，请检查推流地址和推流码是否正确，以及网络是否通畅'
/** LIVE_PUSH_REJECTED（产品经理定稿） */
export const LIVE_PUSH_REJECTED_TEXT = '服务器拒绝了推流，请检查推流码是否有效，或是否已被其他推流占用'
/** SRT 口令长度不在 10~79 时前端先拦（api/live.ts 的 validateSrtPassphrase），不发给后端 */
export const LIVE_SRT_PASSPHRASE_TEXT = 'SRT 口令需要 10 到 79 个字符'

/** LIVE_CONNECT_FAILED 的文案：scheme 来自 detail 首行 `scheme=rtmp|rtmps|srt`（AppError.scheme）。只有 srt 用 SRT 文案；rtmp / rtmps / 缺失 / 不认识一律用 RTMP 那句（产品定稿） */
export function liveConnectFailedText(scheme?: string | null): string {
  return typeof scheme === 'string' && scheme.toLowerCase() === 'srt' ? LIVE_SRT_CONNECT_FAILED_TEXT : LIVE_RTMP_CONNECT_FAILED_TEXT
}
function isLiveConnectFailedText(text?: string | null): boolean {
  return text === LIVE_SRT_CONNECT_FAILED_TEXT || text === LIVE_RTMP_CONNECT_FAILED_TEXT
}

/**
 * 直播任务 / 调用失败 → 遮罩和任务行上显示的说明（作为 message 传给 ErrorOverlay / ErrorLine）。
 * LIVE_CONNECT_FAILED：按 detail 首行 scheme= 选 SRT / RTMP 文案；detail 没有 scheme 时用 fallbackScheme（脱敏 params.url 或页面地址的 scheme，只是兜底）；都没有 → RTMP 那句。
 * LIVE_PUSH_REJECTED：固定文案。其余码原样返回后端 message。
 */
export function liveFailureMessage(err: { code?: string | null; message?: string | null; detail?: string | null }, fallbackScheme?: string | null): string {
  if (err.code === 'LIVE_CONNECT_FAILED') return liveConnectFailedText(parseDetailHead(err.detail ?? undefined).scheme ?? fallbackScheme)
  if (err.code === 'LIVE_PUSH_REJECTED') return LIVE_PUSH_REJECTED_TEXT
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
export const LIVE_URL_MALFORMED_TEXT = '推流地址格式不正确，请检查后重新输入'
export const LIVE_URL_MISSING_HOST_TEXT = '推流地址里缺少服务器地址，请检查后重新输入'
export const LIVE_URL_PARAM_NOT_ALLOWED_TEXT = '推流地址里有不支持的参数，请去掉后重试'
export const LIVE_URL_INVALID_GENERIC = '推流地址不可用，请检查后重新输入'
export const LIVE_URL_INVALID_REASON_TEXT: Record<string, string> = {
  scheme_unsupported: LIVE_PROTOCOL_UNSUPPORTED_TEXT,
  malformed: LIVE_URL_MALFORMED_TEXT,
  missing_host: LIVE_URL_MISSING_HOST_TEXT,
  param_not_allowed: LIVE_URL_PARAM_NOT_ALLOWED_TEXT,
}
/** LIVE_URL_INVALID 的用户文案；reason 取自 AppError.reason（api/call.ts 解析）；地址框失焦校验也用它 */
export function liveUrlInvalidText(reason?: string | null): string {
  return (reason && Object.prototype.hasOwnProperty.call(LIVE_URL_INVALID_REASON_TEXT, reason) && LIVE_URL_INVALID_REASON_TEXT[reason]) || LIVE_URL_INVALID_GENERIC
}
/** ffmpeg 缺推流协议、detail 里没有认得出的协议名（产品经理定稿） */
export const LIVE_FFMPEG_PROTOCOL_MISSING_TEXT = '当前 ffmpeg 不支持这种推流协议，请在设置的 ffmpeg 页面重新安装或更新'
/**
 * 开始前 ffmpeg 缺协议（Start* 同步返回 UNSUPPORTED）。格式以契约 §6.10（2.2 错误码 detail 约定表）为准：
 * detail 是单独一行 `missing=<协议名>`，协议名只取 `rtmp` / `rtmps` / `srt`；带本地存档的会话另需 tee，缺时是 `missing=tee`；CheckPushURL 不返回它。
 * 识别规则（严格）：detail 按行拆开，某一行**严格等于** `missing=rtmp` / `missing=rtmps` / `missing=srt` 才带协议名（协议名大写显示）；
 * 其余一律用不带协议名的通用句，包括 `missing=tee`、大小写不同、带空格、别的写法、ffmpeg 原文；detail 原文永远不进文案。
 */
export const LIVE_PROTOCOL_NAMES: Record<string, string> = { rtmp: 'RTMP', rtmps: 'RTMPS', srt: 'SRT' }
/** detail 里是否有 `missing=` 开头的行（契约里这是缺 ffmpeg 组件的标记；没有 = 别的原因的 UNSUPPORTED，如屏幕推流存档未实现） */
export function hasMissingLine(detail?: string | null): boolean {
  return (detail ?? '').split(/\r?\n/).some((l) => l.startsWith('missing='))
}
export function liveMissingProtocolName(detail?: string | null): string | undefined {
  for (const line of (detail ?? '').split(/\r?\n/)) {
    const m = /^missing=(rtmps|rtmp|srt)$/.exec(line)
    if (m) return LIVE_PROTOCOL_NAMES[m[1]]
  }
  return undefined
}
export function liveFfmpegProtocolMissingText(detail?: string | null): string {
  const name = liveMissingProtocolName(detail)
  return name ? `当前的 ffmpeg 不支持 ${name}，请在设置的 ffmpeg 页面重新安装或更新` : LIVE_FFMPEG_PROTOCOL_MISSING_TEXT
}
/** 带存档的会话被强杀且存档保留（status=canceled 且 outputPath 非空）时状态行的文案（设计稿 v0.2 §6.8，产品定稿） */
export const LIVE_CANCELED_ARCHIVE_KEPT_TEXT = '已强制停止，存档已保留，文件可能不完整'
/** 正在停止（已发停止、等后端事件；有存档最多等 16 秒，界面不做超时处理） */
export const LIVE_STOPPING_TEXT = '正在停止…'
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
      // 后端 #47 已实现存档，页面不再对存档显示“暂不支持”。UNSUPPORTED 一律按缺 ffmpeg 组件处理：
      // detail 某一行严格等于 missing=rtmp|rtmps|srt 才带协议名；missing=tee（带存档时缺 tee muxer）和其余情形用不带协议名的通用句
      return { title: '无法开始推流', description: liveFfmpegProtocolMissingText(e.detail) }
    case 'LIVE_CONNECT_FAILED': {
      // scheme 优先取 detail 首行（AppError.scheme），拿不到才用页面上地址的 scheme 兜底；都没有 → RTMP 那句
      return { title: '无法连接流服务器', description: liveConnectFailedText(e.scheme ?? opts.scheme) }
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
