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

const RETRY = (): ErrorButton => ({ label: t('errors.retry'), action: 'retry', icon: 'refresh' })
const VIEW_LOG = (): ErrorButton => ({ label: t('errors.viewLog'), action: 'viewLog' })

function overlay(title: string, description: string, extra: Partial<ErrorMessage> = {}): ErrorMessage {
  return { title, description, style: 'overlay', primary: RETRY(), secondary: VIEW_LOG(), ...extra }
}

export function getErrorMessage(code: ErrorCode): ErrorMessage {
  const table: Record<ErrorCode, ErrorMessage> = {
  LIVE_URL_INVALID: {
    title: t('errors.LIVE_URL_INVALID.title'),
    description: t('errors.LIVE_URL_INVALID.description'),
    style: 'inline',
    primary: null,
    secondary: null,
  },
  LIVE_CONNECT_FAILED: overlay(t('errors.LIVE_CONNECT_FAILED.title'), t('errors.LIVE_CONNECT_FAILED.description'), { taskRow: true }),
  LIVE_PUSH_REJECTED: overlay(t('errors.LIVE_PUSH_REJECTED.title'), t('errors.LIVE_PUSH_REJECTED.description')),
  LIVE_PLAY_FAILED: overlay(t('errors.LIVE_PLAY_FAILED.title'), t('errors.LIVE_PLAY_FAILED.description')),
  LIVE_CORS_BLOCKED: overlay(t('errors.LIVE_CORS_BLOCKED.title'), t('errors.LIVE_CORS_BLOCKED.description')),
  LIVE_PUSH_INTERRUPTED: overlay(t('errors.LIVE_PUSH_INTERRUPTED.title'), t('errors.LIVE_PUSH_INTERRUPTED.description'), { taskRow: true, primary: null }),
  FFMPEG_NOT_FOUND: overlay(t('errors.FFMPEG_NOT_FOUND.title'), t('errors.FFMPEG_NOT_FOUND.description'), {
    primary: { label: t('errors.goSettings'), action: 'route', to: '/settings' },
  }),
  SCREEN_PERMISSION_DENIED: overlay(t('errors.SCREEN_PERMISSION_DENIED.title'), t('errors.SCREEN_PERMISSION_DENIED.description')),
  UNSUPPORTED_PLATFORM: overlay(t('errors.UNSUPPORTED_PLATFORM.title'), t('errors.UNSUPPORTED_PLATFORM.description'), { primary: null, secondary: null }),
  }
  return table[code]
}

/** @deprecated 兼容旧引用；取值时已按当前语言生成 */
export const errorMessages: Record<ErrorCode, ErrorMessage> = new Proxy({} as Record<ErrorCode, ErrorMessage>, {
  get(_t, prop: string) {
    if (prop in { LIVE_URL_INVALID:1,LIVE_CONNECT_FAILED:1,LIVE_PUSH_REJECTED:1,LIVE_PLAY_FAILED:1,LIVE_CORS_BLOCKED:1,LIVE_PUSH_INTERRUPTED:1,FFMPEG_NOT_FOUND:1,SCREEN_PERMISSION_DENIED:1,UNSUPPORTED_PLATFORM:1 }) {
      return getErrorMessage(prop as ErrorCode)
    }
    return undefined
  },
  has(_t, prop: string) {
    return prop in { LIVE_URL_INVALID:1,LIVE_CONNECT_FAILED:1,LIVE_PUSH_REJECTED:1,LIVE_PLAY_FAILED:1,LIVE_CORS_BLOCKED:1,LIVE_PUSH_INTERRUPTED:1,FFMPEG_NOT_FOUND:1,SCREEN_PERMISSION_DENIED:1,UNSUPPORTED_PLATFORM:1 }
  },
  ownKeys() { return ['LIVE_URL_INVALID','LIVE_CONNECT_FAILED','LIVE_PUSH_REJECTED','LIVE_PLAY_FAILED','LIVE_CORS_BLOCKED','LIVE_PUSH_INTERRUPTED','FFMPEG_NOT_FOUND','SCREEN_PERMISSION_DENIED','UNSUPPORTED_PLATFORM'] },
  getOwnPropertyDescriptor(t, prop) {
    if (this.has!(t, prop)) return { configurable: true, enumerable: true, value: (this.get as any)(t, prop) }
    return undefined
  },
})

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
      return { ...getErrorMessage(code), description: fallbackMessage as string, code, known: true }
    }
    return { ...getErrorMessage(code), code, known: true }
  }
  const message = userVisibleMessage(fallbackMessage)
  return {
    code: code || 'INTERNAL',
    known: false,
    title: t('errors.fallbackTitle'),
    description: message || t('errors.unmapped'),
    style: 'overlay',
    primary: RETRY(),
    secondary: VIEW_LOG(),
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

export function getTaskErrorMessage(code: TaskErrorCode): TaskErrorMessage {
  const table: Record<TaskErrorCode, TaskErrorMessage> = {
  CONVERT_DISK_FULL: {
    title: t('errors.CONVERT_DISK_FULL.title'),
    description: t('errors.CONVERT_DISK_FULL.description'),
    actions: ['retry', 'changeOutput', 'viewLog'],
  },
  PROBE_FAILED: {
    title: t('errors.PROBE_FAILED.title'),
    description: t('errors.PROBE_FAILED.description'),
    actions: ['retry', 'viewLog'],
  },
  CANCELED: {
    title: t('errors.CANCELED.title'),
    description: t('errors.CANCELED.description'),
    actions: [],
  },
  }
  return table[code]
}
export const taskErrorMessages: Record<TaskErrorCode, TaskErrorMessage> = new Proxy({} as Record<TaskErrorCode, TaskErrorMessage>, {
  get(_t, prop: string) {
    if (prop === 'CONVERT_DISK_FULL' || prop === 'PROBE_FAILED' || prop === 'CANCELED') return getTaskErrorMessage(prop)
    return undefined
  },
  has(_t, prop: string) { return prop === 'CONVERT_DISK_FULL' || prop === 'PROBE_FAILED' || prop === 'CANCELED' },
  ownKeys() { return ['CONVERT_DISK_FULL','PROBE_FAILED','CANCELED'] },
  getOwnPropertyDescriptor(t, prop) {
    if (this.has!(t, prop)) return { configurable: true, enumerable: true, value: (this.get as any)(t, prop) }
    return undefined
  },
})

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
export const TASK_FALLBACK_TITLE = t('errors.extra.s_fcce57cf')
/** 包 24 N4：直播任务没有专属文案时的标题，不能是「转换失败」 */
export const LIVE_PUSH_FALLBACK_TITLE = t('errors.extra.s_b245a1c9')
export const LIVE_PULL_FALLBACK_TITLE = t('errors.extra.s_51083325')
/** 直播任务是拉流还是推流（任务类型以 live_ 开头；现在只有推流两种，名字带 pull 的按拉流） */
export const liveTaskVerb = (type?: string | null): string => (type && /pull/.test(type) ? t('errors.verbPull') : t('errors.verbPush'))
/** 没有专属文案时的标题：直播推流 / 直播拉流 / 旧版导出 / 其他（转换） */
export function taskFallbackTitle(type?: string | null): string {
  if (type && type.startsWith('live_')) return /pull/.test(type) ? t('errors.taskFallbackPull') : t('errors.taskFallbackPush')
  if (type === 'edit_export') return t('errors.taskFallbackExport') || t('errors.extra.s_dd51ab50') // 旧版导出记录（包 24 N3）
  return t('errors.taskFallbackConvert')
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
    return { code, ...getTaskErrorMessage(code), known: true }
  }
  if (isKnownErrorCode(code)) {
    const m = getErrorMessage(code)
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
export const LIVE_STOP_TEXT = { get succeeded(){ return t('errors.live.stopSucceeded') }, get canceled(){ return t('errors.live.stopCanceled') } }
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
  return text === t('errors.live.srtConnectFailed') || text === t('errors.live.rtmpConnectFailed') || text === LIVE_SRT_CONNECT_FAILED_TEXT || text === LIVE_RTMP_CONNECT_FAILED_TEXT
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
  const m = /"url"\s*:\s*t('errors.extra.s_ffd66747')