// 错误文案表：产品经理定稿、老板冻结，改文案要先走评审。
// 展示形态见设计原型 proto/errors.html：overlay 用于播放器区域，inline 用于表单，任务中心失败行用 ErrorLine。
import type { IconName } from '../components/icon/icons'
import { docErrorReason, parseDetailHead, type DocErrorReason } from '../api/call'
import { t } from '@/i18n'

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

const RETRY: ErrorButton = { label: t('errors.retry'), action: 'retry', icon: 'refresh' }
const VIEW_LOG: ErrorButton = { label: t('errors.viewLog'), action: 'viewLog' }

function overlay(title: string, description: string, extra: Partial<ErrorMessage> = {}): ErrorMessage {
  return { title, description, style: 'overlay', primary: RETRY, secondary: VIEW_LOG, ...extra }
}

export const errorMessages: Record<ErrorCode, ErrorMessage> = {
  LIVE_URL_INVALID: {
    title: t('errors.extra.s_5c2441e3'),
    description: t('errors.extra.s_c550b819'), // 同 LIVE_URL_INVALID_GENERIC；具体 reason 见 LIVE_URL_INVALID_REASON_TEXT
    style: 'inline',
    primary: null,
    secondary: null,
  },
  LIVE_CONNECT_FAILED: overlay(t('errors.extra.s_39f67974'), t('errors.extra.s_0e589242'), { taskRow: true }),
  LIVE_PUSH_REJECTED: overlay(t('errors.extra.s_adcce971'), t('errors.extra.s_a1b54435')),
  LIVE_PLAY_FAILED: overlay(t('errors.taskFallbackPull'), t('errors.extra.s_ca161311')),
  LIVE_CORS_BLOCKED: overlay(t('errors.extra.s_707dd74e'), t('errors.extra.s_5c9ed724')),
  // 直播会话不能重试（后端 Retry 返回 UNSUPPORTED）：不给“重试”，叫法与直播页一致“推流中断”；“自动重连”已作废（设计说明 v0.2）
  LIVE_PUSH_INTERRUPTED: overlay(t('errors.extra.s_5311ced2'), t('errors.extra.s_168013b7'), { taskRow: true, primary: null }),
  FFMPEG_NOT_FOUND: overlay(t('errors.extra.s_9123b972'), t('errors.extra.s_ad772e44'), {
    primary: { label: t('errors.goSettings'), action: 'route', to: '/settings' },
  }),
  // 原型里是弯引号“”，这里跟原型一致
  // 产品经理定稿：两个码文案不混用（权限被拒 ≠ 平台不支持）
  SCREEN_PERMISSION_DENIED: overlay(t('errors.extra.s_be73c561'), t('errors.extra.s_f3274e53')),
  UNSUPPORTED_PLATFORM: overlay(t('errors.extra.s_0c9cd3c4'), t('errors.extra.s_b2412e2f'), { primary: null, secondary: null }),
}

/** 未收录的错误码（INTERNAL 等）的兜底文案 */
export const FALLBACK_TITLE = t('errors.fallbackTitle')
/** 契约 v0.25.3：映射不到的错误一律这一句。错误码和 reason= 不出现在界面上。 */
export const UNMAPPED_ERROR_TEXT = t('errors.unmapped')
export const FALLBACK_DESCRIPTION = UNMAPPED_ERROR_TEXT

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
  const message = userVisibleMessage(fallbackMessage)
  return {
    code: code || 'INTERNAL',
    known: false,
    title: FALLBACK_TITLE,
    description: message || UNMAPPED_ERROR_TEXT,
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
    title: t('errors.extra.s_fc937a95'),
    description: t('errors.extra.s_47a1d9eb'),
    actions: ['retry', 'changeOutput', 'viewLog'],
  },
  PROBE_FAILED: {
    title: t('errors.extra.s_aad661a5'),
    description: t('errors.extra.s_fefb979b'),
    actions: ['retry', 'viewLog'],
  },
  // 后端因应用退出（根 ctx 取消）取消调用时返回：不是失败，不提供重试，展示为中性样式
  CANCELED: {
    title: t('errors.extra.s_2111ccbb'),
    description: t('errors.action.CANCELED'),
    actions: [],
  },
}

/**
 * 后端旧文案“ffmpeg 异常退出（退出码 -1）”“ffmpeg 退出码 1”用户看不懂：PROCESS_FAILED 且 message 是这类时，前端改写成说人话的一句（设计师走查 G4 建议措辞，待产品经理确认）；
 * 退出码等技术信息在“查看日志”里（detail / 日志），不放主提示。后端新文案（不含“退出码”）原样使用，不改写。
 */
export const PROCESS_EXIT_TEXT = t('errors.extra.s_644e104c')
const OLD_PROCESS_EXIT = /^(ffmpeg|转换组件)\s*(异常退出|退出码)|退出码\s*-?\d+/
export function rewriteProcessExitText(code: string | null | undefined, message: string): string {
  return code === 'PROCESS_FAILED' && OLD_PROCESS_EXIT.test(message) ? PROCESS_EXIT_TEXT : message
}

/** 任务中心里没有专属文案的错误码：标题按任务类型（taskFallbackTitle），描述取后端 message。转换类任务用这句 */
export const TASK_FALLBACK_TITLE = t('errors.taskFallbackConvert')
/** 包 24 N4：直播任务没有专属文案时的标题，不能是「转换失败」 */
export const LIVE_PUSH_FALLBACK_TITLE = t('errors.taskFallbackPush')
export const LIVE_PULL_FALLBACK_TITLE = t('errors.taskFallbackPull')
/** 直播任务是拉流还是推流（任务类型以 live_ 开头；现在只有推流两种，名字带 pull 的按拉流） */
export const liveTaskVerb = (type?: string | null): string => (type && /pull/.test(type) ? t('errors.verbPull') : t('errors.verbPush'))
/** 没有专属文案时的标题：直播推流 / 直播拉流 / 旧版导出 / 其他（转换） */
export function taskFallbackTitle(type?: string | null): string {
  if (type && type.startsWith('live_')) return /pull/.test(type || '') ? LIVE_PULL_FALLBACK_TITLE : LIVE_PUSH_FALLBACK_TITLE
  if (type === 'edit_export') return t('errors.taskFallbackExport') // 旧版导出记录（包 24 N3）
  return TASK_FALLBACK_TITLE
}
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
 * 3. 其余一律 标题按任务类型（转换任务「转换失败」，直播「推流失败」/「拉流失败」）+ 后端 message 作描述，绝不出现「出错了」。
 */
export function resolveTaskError(code?: string | null, fallbackMessage?: string | null, taskType?: string | null): ResolvedTaskError {
  if (isTaskErrorCode(code)) {
    return { code, ...taskErrorMessages[code], known: true }
  }
  if (isKnownErrorCode(code)) {
    const m = errorMessages[code]
    const byScheme = code === 'LIVE_CONNECT_FAILED' && isLiveConnectFailedText(fallbackMessage)
    return { code, title: m.title, description: byScheme ? (fallbackMessage as string) : m.description, actions: DEFAULT_TASK_ACTIONS, known: true }
  }
  const message = userVisibleMessage(fallbackMessage)
  return {
    code: code || 'INTERNAL',
    title: taskFallbackTitle(taskType),
    description: (message ? rewriteProcessExitText(code, message) : '') || UNMAPPED_ERROR_TEXT,
    actions: DEFAULT_TASK_ACTIONS,
    known: false,
  }
}

/**
 * 任务卡上实际画出来的文字。未知码、空 message、detail 只有 reason= 时，说明是「出了点问题，请重试。」。
 * text 是标题和说明拼在一起，给自检用：里面不能有错误码，也不能有 reason=。
 */
export function renderTaskError(code?: string | null, message?: string | null, detail?: string | null, taskType?: string | null): ResolvedTaskError & { text: string } {
  const fromDetail = detailWithoutPaths(detail).split(/\r?\n/).map((l) => l.trim()).filter(Boolean).pop() ?? ''
  const resolved = resolveTaskError(code, userVisibleMessage(message) || userVisibleMessage(fromDetail), taskType)
  const text = `${resolved.title}${resolved.description}`
  return { ...resolved, text }
}

// ---- 操作失败提示（toast / 行内）：按钮触发的 Bind 调用返回的 AppError → 用户可读的话 ----
// 与失败行的 ErrorLine 文案分开：这里是「点了某个按钮但没成功」，不是任务本身失败。
const ACTION_ERROR_TEXT: Record<string, string> = {
  CANCELED: t('errors.action.CANCELED'),
  UNSUPPORTED: t('errors.extra.s_7c2c8f78'),
  TASK_CONFLICT: t('errors.extra.s_85f591dc'), // Cancel / Remove 等不带 reason 的冲突；直播 Start* 的冲突见 taskConflictText
  NOT_FOUND: t('errors.extra.s_02128a49'),
}

/** 取 toast 文案：有专门说明的码用说明，其余用后端 message */
export function actionErrorText(code: string, backendMessage: string, reason?: string): string {
  if (code === 'TASK_CONFLICT') return taskConflictText(reason)
  // 旧版导出的重试 / 重转：用后端这句，不要收成「该任务暂不支持重试」或「导出失败」
  if (code === 'UNSUPPORTED' && /旧版导出/.test(backendMessage)) return backendMessage
  if (ACTION_ERROR_TEXT[code]) return ACTION_ERROR_TEXT[code]
  return publicErrorText(backendMessage)
}

/** detail 里不给用户看的行：reason= / scheme= / kind= / missing=，以及像路径的行。 */
const MACHINE_LINE = /^(?:reason|scheme|kind|missing)=/
const KNOWN_CODE_RE = /\b(?:INVALID_ARGUMENT|NOT_FOUND|TASK_CONFLICT|IO_ERROR|CANCELED|UNSUPPORTED|INTERNAL|FFMPEG_NOT_FOUND|PROBE_FAILED|PROCESS_FAILED|CONVERT_DISK_FULL|UNSUPPORTED_PLATFORM|LIVE_URL_INVALID|LIVE_CONNECT_FAILED|LIVE_PUSH_REJECTED|LIVE_PUSH_INTERRUPTED|SCREEN_PERMISSION_DENIED|LIVE_SOURCE_GONE|LIVE_PLAY_FAILED|LIVE_CORS_BLOCKED|PDF_PARSE_FAILED|DOC_COMPONENT_NOT_READY|DOC_DOWNLOAD_FAILED|DOC_CHECKSUM_FAILED|DOC_COMPONENT_INSTALL_FAILED|DOC_FORMAT_UNSUPPORTED|DOC_PDF_INPUT_UNSUPPORTED|DOC_ENCRYPTED|DOC_CORRUPT|DOC_TIMEOUT|DOC_COMPONENT_CRASHED|DOC_PRESENTATION_BUSY|DOC_ENGINE_BUSY|DOC_PDF_NO_TEXT|LANG_ASR_NOT_READY|LANG_ASR_EMPTY|LANG_ASR_FAILED|LANG_DOWNLOAD_FAILED|LANG_CHECKSUM_FAILED|CAT_NOT_READY|CAT_REPLY_FAILED|CAT_PROJECT_MISSING)\b/g
const UNKNOWN_CODE_RE = /\b[A-Z][A-Z0-9]*_[A-Z0-9_]+\b/g

export function detailWithoutPaths(detail?: string | null): string {
  const keep = (detail ?? '').split(/\r?\n/).map((l) => l.trim()).filter((l) => !!l && !MACHINE_LINE.test(l) && !looksLikePath(l))
  return keep.join('\n')
}

/** 能给用户看的说明。空的、只有 reason= 或错误码的，返回空串。 */
export function userVisibleMessage(text?: string | null): string {
  const lines = (text ?? '').split(/\r?\n/).map((l) => l.trim()).filter((l) => l && !MACHINE_LINE.test(l) && !looksLikePath(l))
  UNKNOWN_CODE_RE.lastIndex = 0
  KNOWN_CODE_RE.lastIndex = 0
  const s = lines.join('\n').replace(/\breason=\S*/g, ' ').replace(UNKNOWN_CODE_RE, ' ').replace(KNOWN_CODE_RE, ' ').replace(/[ \t]{2,}/g, ' ').trim()
  if (!s || MACHINE_LINE.test(s) || s.includes('reason=')) return ''
  return s
}

/** 界面上的一句错误说明：能看的用原文，其余用「出了点问题，请重试。」 */
export function publicErrorText(message?: string | null): string {
  return userVisibleMessage(message) || UNMAPPED_ERROR_TEXT
}
function looksLikePath(line: string): boolean {
  if (/^[A-Za-z]:[\\/]/.test(line) || /^\\\\/.test(line)) return true
  if (/^\/(usr|home|opt|var|tmp|Users|Applications|private)\b/.test(line)) return true
  if (/[A-Za-z]:\\[^\s]{3,}/.test(line)) return true
  return false
}

// ---- TASK_CONFLICT 的 reason → 文案（契约 6.10：detail 首行 `reason=<值>`，稳定枚举，只追加不改名）----
// 追加新 reason 只需要在这张表里加一行。未知 reason、缺失 reason，以及 Cancel / Remove 等本来就不带 reason 的冲突，一律走 TASK_CONFLICT_GENERIC。
export const TASK_CONFLICT_GENERIC = t('errors.extra.s_85f591dc')
export const TASK_CONFLICT_REASON_TEXT: Record<string, string> = {
  max_sessions: t('errors.extra.s_7dda9fd6'),
  duplicate_url: t('errors.extra.s_e27b4bfe'),
  screen_busy: t('errors.extra.s_ca177745'),
}

/** TASK_CONFLICT 的用户文案；reason 取自 AppError.reason（api/call.ts 解析） */
export function taskConflictText(reason?: string | null): string {
  return (reason && Object.prototype.hasOwnProperty.call(TASK_CONFLICT_REASON_TEXT, reason) && TASK_CONFLICT_REASON_TEXT[reason]) || TASK_CONFLICT_GENERIC
}

// ---- 直播页专用文案（产品 / 设计定稿）----
/** 直播任务停止后的状态文案：只看 Task.status（后端保证 succeeded 时 error 为空、canceled 时不带错误码），不看 error */
export const LIVE_STOP_TEXT = { succeeded: t('errors.extra.s_f6845c6e'), canceled: t('errors.extra.s_136ab2bd') } as const
/** SRT 连接失败（产品经理定稿）：后端统一判 LIVE_CONNECT_FAILED（无法区分服务器未开与口令错误），文案由前端负责 */
export const LIVE_SRT_CONNECT_FAILED_TEXT = t('errors.extra.s_382f2ea2')
/** RTMP / RTMPS，以及 detail 缺 scheme 首行 / scheme 不认识时的连接失败文案（产品经理定稿） */
export const LIVE_RTMP_CONNECT_FAILED_TEXT = t('errors.extra.s_0e589242')
/** LIVE_PUSH_REJECTED（产品经理定稿） */
export const LIVE_PUSH_REJECTED_TEXT = t('errors.extra.s_a1b54435')
/** SRT 口令长度不在 10~79 时前端先拦（api/live.ts 的 validateSrtPassphrase），不发给后端 */
export const LIVE_SRT_PASSPHRASE_TEXT = t('errors.extra.s_2dc3e9af')

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
export const LIVE_PROTOCOL_UNSUPPORTED_TEXT = t('errors.extra.s_991b4381')
export const LIVE_URL_MALFORMED_TEXT = t('errors.extra.s_775dcc2f')
export const LIVE_URL_MISSING_HOST_TEXT = t('errors.extra.s_439e6eab')
export const LIVE_URL_PARAM_NOT_ALLOWED_TEXT = t('errors.extra.s_7ccd8b50')
export const LIVE_URL_INVALID_GENERIC = t('errors.extra.s_c550b819')
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
export const LIVE_FFMPEG_PROTOCOL_MISSING_TEXT = t('errors.extra.s_5f8b6f9c')
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
  return name ? t('errors.extra.s_3a7b6a7c', { n: name }) : LIVE_FFMPEG_PROTOCOL_MISSING_TEXT
}
/** 转换组件缺少读取文件信息的部分（ffprobe）：设置 / 关于里的转换组件状态（产品经理 10-08 定稿） */
export const FFPROBE_MISSING_TEXT = t('errors.extra.s_19650fda')
/** 带存档的会话被强杀且存档保留（status=canceled 且 outputPath 非空）时状态行的文案（设计稿 v0.2 §6.8，产品定稿） */
export const LIVE_CANCELED_ARCHIVE_KEPT_TEXT = t('errors.extra.s_4271358e')
/** 正在停止（已发停止、等后端事件；有存档最多等 16 秒，界面不做超时处理） */
export const LIVE_STOPPING_TEXT = t('errors.extra.s_a7ad5d41')
/** 选中屏幕推流时来源下方常驻的说明（12px、--ff-text-2、前置信息图标，不弹窗） */
export const LIVE_SCREEN_NO_AUDIO_TEXT = t('errors.extra.s_0ec77d1c')

// ---- 直播 v1.1 采集来源选择器文案（设计稿未出，先按 v0.2 风格；**全部待产品经理确认**，改字只改这里）----
export const LIVE_SOURCE_FIELD_LABEL = t('errors.extra.s_c9713ce0')
export const LIVE_SOURCE_FIELD_LABEL_SCREEN = t('errors.extra.s_cc96e566') // macOS / Linux 屏幕单选列表的标签（设计说明 §4.7）
export const LIVE_SOURCE_GROUP_SCREEN = t('errors.extra.s_21b8ea59')
export const LIVE_SOURCE_GROUP_WINDOW = t('errors.extra.s_5a38c7fc')
export const LIVE_SOURCE_REFRESH = t('errors.extra.s_93bc1f09')
export const LIVE_SOURCE_LOADING = t('errors.extra.s_1a8be31b')
export const LIVE_SOURCE_EMPTY = t('errors.extra.s_9c04a123')
export const LIVE_SOURCE_FAILED = t('errors.extra.s_f4f6c982')
export const LIVE_SOURCE_RETRY = t('errors.retry')
// Windows 分组下拉（设计说明 §4，文案表 §8；未注明“定稿”的均待产品经理确认）
export const LIVE_SOURCE_PLACEHOLDER = t('errors.extra.s_94a3c65b')
export const LIVE_SOURCE_PLACEHOLDER_LOADING = t('errors.extra.s_5b9d6c8d')
export const LIVE_SOURCE_REFRESH_SHORT = t('errors.extra.s_694fc5ef')
export const LIVE_SOURCE_REFRESH_ARIA = t('errors.extra.s_1d32c1e3')
export const LIVE_SOURCE_REFRESHING = t('errors.extra.s_57176ef2')
export const LIVE_SOURCE_GONE_TAG = t('errors.extra.s_0ea55b4c')
export const LIVE_SOURCE_NO_WINDOW_TITLE = t('errors.extra.s_d3eeffb0')
export const LIVE_SOURCE_NO_WINDOW_HINT = t('errors.extra.s_b116a502')
/** 首次加载失败（没有旧列表）：一句话，指向下方“刷新”按钮（产品经理定稿） */
export const LIVE_SOURCE_FAIL_TEXT = t('errors.extra.s_cee99bcf')
/** 没选来源时触发器直接显示的名字（后端默认推主显示器；表单里不再另加提示，产品经理定稿） */
export const LIVE_SOURCE_DEFAULT_MAIN_NAME = t('errors.extra.s_f6fdb209')
export const LIVE_SOURCE_STALE = t('errors.extra.s_2ddb6429')
export const liveSourceWindowCount = (n: number): string => t('errors.extra.s_54c88101', { n: n })
/** 会话列表空状态：Windows 加“或应用窗口”，macOS / Linux 保持 v0.2 原文 */
export const LIVE_RECORD_EMPTY_HINT_WIN = t('errors.extra.s_18c48c31')
export const LIVE_RECORD_EMPTY_HINT = t('errors.extra.s_26ca0c39')
/** LIVE_SOURCE_GONE：窗口版由产品经理给出（待确认）；屏幕版（kind=screen）用通用说法，待确认 */
export const LIVE_SOURCE_GONE_WINDOW_TEXT = t('errors.extra.s_11555195')
export const LIVE_SOURCE_GONE_SCREEN_TEXT = t('errors.extra.s_3ac6edf8')
/** detail 首行 kind=window|screen；没有 / 未知 → 按契约用窗口版（码本身就表示“所选窗口已不可用”）。文案里不带窗口标题 */
export function liveSourceGoneText(kind?: string): string {
  return kind === 'screen' ? LIVE_SOURCE_GONE_SCREEN_TEXT : LIVE_SOURCE_GONE_WINDOW_TEXT
}

/**
 * 直播 Start* 同步返回的错误 → 页面上展示的一句话（走 ErrorLine，点“开始”之后才出现，不提前置灰按钮）。
 * 返回 null 表示不属于这里处理的情形（调用方走原来的遮罩 / 行内错误）。
 */
export function liveStartErrorLine(e: { code: string; message?: string; reason?: string; scheme?: string; detail?: string }, opts: { scheme?: string } = {}): { title: string; description: string } | null {
  switch (e.code) {
    case 'TASK_CONFLICT':
      return { title: t('errors.extra.s_bf32dd27'), description: taskConflictText(e.reason) }
    case 'UNSUPPORTED':
      // 后端 #47 已实现存档，页面不再对存档显示“暂不支持”。UNSUPPORTED 一律按缺 ffmpeg 组件处理：
      // detail 某一行严格等于 missing=rtmp|rtmps|srt 才带协议名；missing=tee（带存档时缺 tee muxer）和其余情形用不带协议名的通用句
      return { title: t('errors.extra.s_bf32dd27'), description: liveFfmpegProtocolMissingText(e.detail) }
    case 'LIVE_CONNECT_FAILED': {
      // scheme 优先取 detail 首行（AppError.scheme），拿不到才用页面上地址的 scheme 兜底；都没有 → RTMP 那句
      return { title: t('errors.extra.s_39f67974'), description: liveConnectFailedText(e.scheme ?? opts.scheme) }
    }
    default:
      return null
  }
}

// ---- 文档页（DocService，契约 6.12）----
export const DOC_EXPERIMENTAL_LABEL = t('errors.extra.s_51ebde99')
export const DOC_EXPERIMENTAL_NOTE = t('errors.extra.s_31bacb7b')
/** doc / xls / ppt、加密文档等 UNSUPPORTED */
export const DOC_FORMAT_UNSUPPORTED_TEXT = t('errors.extra.s_3c22b204')

// ---- 添加文件时的探测失败（转换页文件行，MediaService.Probe 单项 error）----
// 与任务失败文案分开：这是「这个文件加不进来」，不是转换失败。
const PROBE_ERROR_TEXT: Record<string, string> = {
  CANCELED: t('errors.action.CANCELED'),
  PROBE_FAILED: t('errors.extra.s_fefb979b'),
  NOT_FOUND: t('errors.extra.s_8fca2e8c'),
  INVALID_ARGUMENT: t('errors.extra.s_1dcac24e'),
  IO_ERROR: t('errors.extra.s_e84eeaaa'),
  FFMPEG_NOT_FOUND: t('errors.extra.s_55b65284'),
}
export const PROBE_ERROR_TITLE = t('errors.extra.s_8000ec96')

/** 探测失败行的说明：已知码用上面的文案，其余用后端 message */
export function probeErrorText(code: string, backendMessage?: string): string {
  return PROBE_ERROR_TEXT[code] ?? publicErrorText(backendMessage)
}

/** 提交失败（Submit 整体校验不通过）时没有对应到具体文件的错误标题 */
export const SUBMIT_ERROR_TITLE = t('errors.extra.s_12551860')

// ───────────── 文档页错误判断 ─────────────
// 契约 v0.16（2.2 / 6.12.6）：Doc 面向前端的“文件本身有问题”类错误，detail 首行严格是 `reason=<枚举>`
// （too_many_pages | format | encrypted | no_font | invalid_ooxml | too_large，只追加），code 和 message 不变。
// 取消、磁盘满、读写失败、NOT_FOUND、路径 / 参数 / 输出目录 / 句柄类 INVALID_ARGUMENT、INTERNAL 没有 reason，走该码的原有处理。
// ConvertToPDF 整体校验失败时，出错文件的绝对路径在 reason 行之后（第二行）；任务 error 没有路径行。
// 判断顺序：先看 reason；reason 缺失（老后端 / 没有 reason 的场景）才用下面对 message 的精确相等兜底。不做正则或包含匹配。
const PAGES_MESSAGE = '超过 5000 页'
const FORMAT_MESSAGE = '暂不支持这种格式'
const NO_FONT_MESSAGE = '没有可用的 Unicode 字体'
const OOXML_INVALID_MESSAGE = '不是有效的 OOXML 文件'
const OFFICE_TOO_LARGE_MESSAGE = '文件超过 100 MiB'

const ABS_PATH_RE = /^([A-Za-z]:[\\/]|\/|\\\\)/

/** detail 首行 reason=…（只认契约枚举里的值，其余返回 undefined） */
export function docReasonOf(detail?: string): DocErrorReason | undefined {
  return docErrorReason(parseDetailHead(detail).reason)
}

/** 出错文件的完整路径：ConvertToPDF 整体校验失败时在 reason 行之后（第二行）；也兼容没有 reason 行、路径在第一行的旧形态。只看前两行，reason= 行不当路径；找不到返回空串 */
export function docErrorPath(detail?: string): string {
  const lines = (detail ?? '').split(/\r?\n/, 2).map((l) => l.trim())
  for (const l of lines) {
    if (l && !/^reason=/.test(l) && ABS_PATH_RE.test(l)) return l
  }
  return ''
}

/** 出错文件名（路径的最后一段） */
export function docErrorFile(_code: string, detail?: string): string {
  const p = docErrorPath(detail)
  return p ? p.split(/[\\/]/).pop() || '' : ''
}

/** 待产品经理确认：超过页数上限。maxPages 取 GetDocCapabilities().limits.maxPages（默认 5000） */
export function docTooManyPagesText(maxPages = 5000): string {
  return t('errors.extra.s_84fc39a8', { n: maxPages })
}

// 待产品经理确认：下面几句是自拟文案（超过页数上限、文件损坏、加密、缺字体、文件太大），设计师倾向保持前两句，产品经理还没最终确认，定稿后只改这里。
/** 待产品经理确认：reason=too_many_pages（常量形式，页数上限取默认 5000；页面里用 docTooManyPagesText(limits.maxPages)） */
export const DOC_TOO_MANY_PAGES_TEXT = docTooManyPagesText(5000)
/** 待产品经理确认：reason=invalid_ooxml（不是 zip、缺必需部件、XML 损坏） */
export const DOC_FILE_BROKEN_TEXT = t('errors.extra.s_fad92bce')
/** 待产品经理确认：reason=encrypted（加密的 Office 文档；也可能是旧版格式改了扩展名） */
export const DOC_ENCRYPTED_TEXT = t('errors.extra.s_f5bf1288')
/** 待产品经理确认：reason=no_font（需要 Unicode 字体而没有；正常构建内嵌字体总是可用，基本不会出现） */
export const DOC_NO_FONT_TEXT = t('errors.extra.s_408087b4')
/** 待产品经理确认：reason=too_large（Office 文件 > 100 MiB，或 zip 条目 / 解压大小超限） */
export const DOC_TOO_LARGE_TEXT = t('errors.extra.s_6b01917d')
export const DOC_NOT_FOUND_TEXT = t('errors.extra.s_f2140939')
/** 整批校验未通过时列表底部的提示（设计说明 5.2，待确认） */
export const DOC_BATCH_INVALID_HINT = t('errors.extra.s_7acf55d1')

/**
 * 按 reason（缺失时按 message 精确相等兜底）得出 Office 转换错误的文案；不是这几类返回 undefined。
 * taskCenter 为 true 时保持任务中心原有行为：超页数沿用后端 message，缺字体沿用后端 message。
 */
function docReasonText(code: string, msg: string, detail: string | undefined, maxPages: number, taskCenter = false): string | undefined {
  if (code !== 'UNSUPPORTED' && code !== 'INVALID_ARGUMENT') return undefined
  const reason = docReasonOf(detail)
  if (reason) {
    switch (reason) {
      case 'too_many_pages':
        return taskCenter ? msg || docTooManyPagesText(maxPages) : docTooManyPagesText(maxPages)
      case 'format':
        return DOC_FORMAT_UNSUPPORTED_TEXT
      case 'encrypted':
        return DOC_ENCRYPTED_TEXT
      case 'no_font':
        return taskCenter ? msg || DOC_NO_FONT_TEXT : DOC_NO_FONT_TEXT
      case 'invalid_ooxml':
        return DOC_FILE_BROKEN_TEXT
      case 'too_large':
        return DOC_TOO_LARGE_TEXT
    }
  }
  // 兜底：reason 缺失，对 message 做精确相等
  if (code === 'UNSUPPORTED') {
    if (msg === PAGES_MESSAGE) return taskCenter ? msg : docTooManyPagesText(maxPages)
    if (msg === FORMAT_MESSAGE) return DOC_FORMAT_UNSUPPORTED_TEXT
    if (msg === NO_FONT_MESSAGE) return taskCenter ? msg : DOC_NO_FONT_TEXT
  } else {
    if (msg === OOXML_INVALID_MESSAGE) return DOC_FILE_BROKEN_TEXT
    if (msg === OFFICE_TOO_LARGE_MESSAGE) return DOC_TOO_LARGE_TEXT
  }
  return undefined
}

/** 文档 UNSUPPORTED 的用户文案（任务中心也用）：先看 reason，再兜底 message；认不出的沿用后端 message，不误导用户去“另存为” */
export function docUnsupportedText(backendMessage?: string, detail?: string, maxPages = 5000): string {
  const msg = (backendMessage ?? '').trim()
  return docReasonText('UNSUPPORTED', msg, detail, maxPages, true) ?? publicErrorText(msg)
}

/**
 * 文档页（Office 转 PDF、PDF 预览）里 DocService / 转换任务的错误 → 用户可读的话。页面里不要散写文案，统一走这里。
 * 只处理契约 6.12.6 里 DocService 会返回的码：INVALID_ARGUMENT / NOT_FOUND / UNSUPPORTED / IO_ERROR / CONVERT_DISK_FULL / CANCELED / INTERNAL。
 */
export function docErrorText(code: string, backendMessage?: string, detail?: string, maxPages = 5000): string {
  const msg = (backendMessage ?? '').trim()
  switch (code) {
    case 'UNSUPPORTED':
    case 'INVALID_ARGUMENT':
      // 有 reason 的按 reason；路径 / 参数 / 输出目录类的 INVALID_ARGUMENT 没有 reason，沿用后端 message
      return docReasonText(code, msg, detail, maxPages) ?? publicErrorText(msg)
    case 'NOT_FOUND':
      return DOC_NOT_FOUND_TEXT
    case 'CONVERT_DISK_FULL':
      return taskErrorMessages.CONVERT_DISK_FULL.description
    case 'IO_ERROR':
      return DOC_PDF_NO_PERMISSION_TEXT // 读源文件失败（IO_ERROR）
    case 'CANCELED':
      return t('errors.action.CANCELED')
    default:
      return publicErrorText(msg)
  }
}

// ---- PDF 预览（设计说明 5.3，全部待确认）----
export const DOC_PDF_ERROR_TITLE = t('errors.extra.s_b4035d66')
export const DOC_PDF_LOADING_TITLE = t('errors.extra.s_104212f3')
export const DOC_PDF_PARSE_FAILED_TEXT = t('errors.extra.s_b844449d')
/** pdf.js 抛出的解析失败没有后端错误码，页面上显示这个前端自定义的显示码（不发给后端）；pdfErrorView 的 code 传它表示解析失败 */
export const DOC_PDF_PARSE_FAILED_CODE = 'PDF_PARSE_FAILED'
export const DOC_PDF_INVALID_TEXT = t('errors.extra.s_0c526e13')
export const DOC_PDF_NOT_FOUND_TEXT = t('errors.extra.s_8fca2e8c')
export const DOC_PDF_NO_PERMISSION_TEXT = t('errors.extra.s_e84eeaaa')
export const DOC_PDF_CHANGED_TEXT = t('errors.extra.s_5bedfc88')
export const DOC_PDF_PASSWORD_TITLE = t('errors.extra.s_b801ab1f')
export const DOC_PDF_PASSWORD_PROMPT = t('errors.extra.s_ca6713e6')
export const DOC_PDF_PASSWORD_PLACEHOLDER = t('errors.extra.s_3f017281')
export const DOC_PDF_PASSWORD_WRONG = t('errors.extra.s_56fb8e99')
export const DOC_PDF_EMPTY_TITLE = t('errors.extra.s_338ccc30')
export const DOC_PDF_EMPTY_HINT = t('errors.extra.s_328ca9c3')
export const DOC_PDF_NOT_OPENED = t('errors.extra.s_daf4f141')

// 后端 OpenPDF / ReadPDFChunk（契约 v0.16）：不是 PDF / 扩展名不对 = reason=format，超 512 MiB = reason=too_large；其余（NOT_FOUND、IO_ERROR、路径 / 句柄类）没有 reason。
// reason 缺失时才用下面对 message 的精确相等兜底。
const PDF_NOT_PDF_MESSAGES = ['不是 PDF 文件', '只支持 .pdf 文件']
const PDF_TOO_LARGE_MESSAGE = '文件超过 512 MiB'
// 文件在读取时变了：后端 OpenPDF 是“文件在读取时被替换，请重试”，前端读整份时 c.size !== src.size 抛的是“PDF 在读取时被修改”
const PDF_CHANGED_MESSAGES = ['文件在读取时被替换，请重试', 'PDF 在读取时被修改']

export interface PdfErrorView {
  /** 一句原因（13px） */
  text: string
  /** 错误码（12px 等宽）：后端码，或前端显示码 PDF_PARSE_FAILED */
  code: string
  /** 可重试：解析失败、无权限、读取时被修改；不可重试的（不是 PDF、超 512 MiB、文件不存在）只给“选择其他 PDF” */
  retry: boolean
}

/** 预览加载失败卡片（设计说明 5.3）。maxPdfBytes 取 limits.maxPdfBytes（默认 512 MiB） */
export function pdfErrorView(code: string, backendMessage?: string, maxPdfBytes = 512 * 1024 * 1024, detail?: string): PdfErrorView {
  const msg = (backendMessage ?? '').trim()
  const reason = docReasonOf(detail)
  switch (code) {
    case DOC_PDF_PARSE_FAILED_CODE:
      return { text: DOC_PDF_PARSE_FAILED_TEXT, code: DOC_PDF_PARSE_FAILED_CODE, retry: true }
    case 'INVALID_ARGUMENT':
      if (reason === 'too_large' || (!reason && msg === PDF_TOO_LARGE_MESSAGE)) return { text: t('errors.extra.s_b0b975b6', { n: Math.round(maxPdfBytes / (1024 * 1024)) }), code, retry: false }
      if (reason === 'format' || (!reason && PDF_NOT_PDF_MESSAGES.includes(msg))) return { text: DOC_PDF_INVALID_TEXT, code, retry: false }
      return { text: publicErrorText(msg), code, retry: false }
    case 'NOT_FOUND':
      return { text: DOC_PDF_NOT_FOUND_TEXT, code, retry: false }
    case 'IO_ERROR':
      return { text: PDF_CHANGED_MESSAGES.includes(msg) ? DOC_PDF_CHANGED_TEXT : DOC_PDF_NO_PERMISSION_TEXT, code, retry: true }
    default:
      return { text: publicErrorText(msg), code, retry: true }
  }
}
/** 浏览器里的模拟环境（没有 window.go）才显示的说明 */
export const DOC_DEMO_NOTE = t('errors.extra.s_0ef28c8a')

// ---- 文档页其他文案（设计说明第 5 节；最近列表标题已由架构师定为「最近打开的 PDF」，与现状一致）----
/** 架构师已定：列表只含「打开过预览」的 PDF（OpenPDF 是唯一写入点），转换产物不自动进列表 */
export const DOC_RECENT_TITLE = t('errors.extra.s_9463cae3')
export const DOC_RECENT_EMPTY_TITLE = t('errors.extra.s_83a36c50')
export const DOC_RECENT_EMPTY_HINT = t('errors.extra.s_541cd968')
export const DOC_RECENT_REMOVE_TIP = t('errors.extra.s_fba5261b')
export const DOC_RECENT_MISSING = t('errors.extra.s_6ac1f0cf')
export const DOC_RECENT_LIMIT_NOTE = t('errors.extra.s_da1643e0')
/** 没有自定义输出位置、也取不到实际路径时的说法（v0.24.1：留空 = 应用的 output 文件夹，不再是源文件所在文件夹） */
export const OUTPUT_DIR_DEFAULT_TEXT = t('errors.extra.s_3ff2b5c4')
export const DOC_DROP_TITLE = t('errors.extra.s_44ae64ba')
export const DOC_DROP_HOVER = t('errors.extra.s_99516c68')
export const DOC_PDF_DROP_HOVER = t('errors.extra.s_fc5c1c18')
