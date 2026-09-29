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
/** 屏幕推流“同时保存本地存档”后端暂未实现（archiveDir 非空 → UNSUPPORTED） */
export const LIVE_ARCHIVE_UNSUPPORTED_TEXT = '暂不支持同时保存本地存档，请关闭“同时保存本地存档”后重试'
/** 选中屏幕推流时来源下方常驻的说明（12px、--ff-text-2、前置信息图标，不弹窗） */
export const LIVE_SCREEN_NO_AUDIO_TEXT = '屏幕推流暂不包含声音'

/**
 * 直播 Start* 同步返回的错误 → 页面上展示的一句话（走 ErrorLine，点“开始”之后才出现，不提前置灰按钮）。
 * 返回 null 表示不属于这里处理的情形（调用方走原来的遮罩 / 行内错误）。
 */
export function liveStartErrorLine(e: { code: string; message?: string; reason?: string; scheme?: string; detail?: string }, opts: { scheme?: string; archive?: boolean } = {}): { title: string; description: string } | null {
  switch (e.code) {
    case 'TASK_CONFLICT':
      return { title: '无法开始推流', description: taskConflictText(e.reason) }
    case 'UNSUPPORTED':
      // 屏幕推流带存档时后端暂返回 UNSUPPORTED（本地存档暂未实现，契约 §6.10：这种 UNSUPPORTED 没有 missing= 行）：提示“暂不支持存档”，不能说成缺协议
      // 有 missing= 行（含 missing=tee）的是缺 ffmpeg 组件，走 liveFfmpegProtocolMissingText（tee 用通用句）；后端存档 PR 合入前不放开存档提示
      if (opts.archive && !hasMissingLine(e.detail)) return { title: '无法开始推流', description: LIVE_ARCHIVE_UNSUPPORTED_TEXT }
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

// ───────────── 文档页错误判断 ─────────────
// 后端（internal/service/doc）的真实取值，逐条对照代码，不靠正则猜 message：
//  - 提交时整体校验失败（ConvertToPDF）：detail 第一行是出错文件的绝对路径（withPath），后面才是原因；
//    任务运行时失败（task.error）：detail 没有路径行，第一行就是原因。所以先剥掉路径行再看“原因首行”。
//  - UNSUPPORTED · 超过 5000 页：detail 首行是 `已排到第 N 页仍未结束`（render.go）或 `文档文字量超过上限`（extract.go）。
//    （契约 6.12.3 写 detail「超过 5000 页」，实际那是 message；detail 用的是上面两句。）
//  - UNSUPPORTED · 不支持的格式：message `暂不支持这种格式`，detail 首行是 `.<扩展名>：<原因>` / `.<扩展名>` / `文件没有扩展名` / `加密文档不支持…`。
//  - UNSUPPORTED · 缺 Unicode 字体：detail 首行 `文档含有 Latin-1 以外的字符，但没有可用的字体`，沿用后端 message。
//  - INVALID_ARGUMENT · 文件损坏：message 固定 `不是有效的 OOXML 文件`，但 detail 首行是自由文本（zip 错误、`缺少 word/document.xml`、
//    `压缩包条目数超过 100000`、`<条目> 解压后超过 256 MiB`……），没有稳定枚举可用，所以这一项只能对 message 做精确相等比较。
//    已提请架构师在契约里给一个稳定的 detail 首行（如 `reason=invalid_ooxml`），有了以后这里改成看 detail。
//  其余任何取值都保守回落到后端 message（后端 message 本身是给用户看的中文）或通用文案。
const PAGES_MESSAGE = '超过 5000 页'
const FORMAT_MESSAGE = '暂不支持这种格式'
const OOXML_INVALID_MESSAGE = '不是有效的 OOXML 文件'
const PAGES_DETAIL_HEADS: RegExp[] = [/^已排到第 \d+ 页仍未结束$/, /^文档文字量超过上限$/]
const NO_FONT_DETAIL_HEAD = '文档含有 Latin-1 以外的字符，但没有可用的字体'
const FORMAT_DETAIL_HEADS: RegExp[] = [/^\.[A-Za-z0-9]+(：.*)?$/, /^文件没有扩展名$/, /^加密文档不支持/]

const ABS_PATH_RE = /^([A-Za-z]:[\\/]|\/|\\\\)/

/** 出错文件的完整路径：ConvertToPDF 整体校验失败时 detail 第一行；不是路径返回空串 */
export function docErrorPath(detail?: string): string {
  const first = (detail ?? '').split(/\r?\n/, 1)[0].trim()
  return first && ABS_PATH_RE.test(first) ? first : ''
}

/** 出错文件名（路径的最后一段） */
export function docErrorFile(_code: string, detail?: string): string {
  const p = docErrorPath(detail)
  return p ? p.split(/[\\/]/).pop() || '' : ''
}

/** “原因首行”：detail 去掉开头的路径行（如果有）后的第一行 */
export function docDetailHead(detail?: string): string {
  const lines = (detail ?? '').split(/\r?\n/).map((l) => l.trim())
  if (lines.length && lines[0] && ABS_PATH_RE.test(lines[0])) lines.shift()
  return lines.find((l) => l !== '') ?? ''
}

/** 待产品经理确认：UNSUPPORTED 且是超过页数上限。maxPages 取 GetDocCapabilities().limits.maxPages（默认 5000） */
export function docTooManyPagesText(maxPages = 5000): string {
  return `文档太长，超过 ${maxPages} 页，无法转换`
}

const isTooManyPages = (message?: string, head = ''): boolean => (message ?? '').trim() === PAGES_MESSAGE || PAGES_DETAIL_HEADS.some((re) => re.test(head))

/** 文档 UNSUPPORTED 的用户文案（任务中心也用）：见上面的取值表；认不出的沿用后端 message，不误导用户去“另存为” */
export function docUnsupportedText(backendMessage?: string, detail?: string, maxPages = 5000): string {
  const head = docDetailHead(detail)
  if (isTooManyPages(backendMessage, head)) return (backendMessage ?? '').trim() || docTooManyPagesText(maxPages) // 任务中心沿用后端 message，行为不变；文档页走 docErrorText 用待确认文案
  if (head === NO_FONT_DETAIL_HEAD) return (backendMessage ?? '').trim() || FALLBACK_DESCRIPTION
  if ((backendMessage ?? '').trim() === FORMAT_MESSAGE || FORMAT_DETAIL_HEADS.some((re) => re.test(head))) return DOC_FORMAT_UNSUPPORTED_TEXT
  return (backendMessage ?? '').trim() || FALLBACK_DESCRIPTION
}

// 待产品经理确认：下面两句是自拟文案（超过页数上限、文件损坏）。设计师倾向保持这两句，产品经理还没最终确认，定稿后只改这里。
/** 待产品经理确认：UNSUPPORTED 且是超过 5000 页（常量形式，页数上限取默认 5000；页面里用 docTooManyPagesText(limits.maxPages)） */
export const DOC_TOO_MANY_PAGES_TEXT = docTooManyPagesText(5000)
/** 待产品经理确认：INVALID_ARGUMENT 且 message 是“不是有效的 OOXML 文件”（打不开、缺部件、条目过多、解压过大） */
export const DOC_FILE_BROKEN_TEXT = '这个文件已损坏，或不是有效的 Word、Excel、PowerPoint 文档'
export const DOC_NOT_FOUND_TEXT = '找不到这个文件，可能已被移动或删除'
/** 整批校验未通过时列表底部的提示（设计说明 5.2，待确认） */
export const DOC_BATCH_INVALID_HINT = '有文件不能转换，本次没有开始转换任何文件。请移除标红的文件后重试。'

/**
 * 文档页（Office 转 PDF、PDF 预览）里 DocService / 转换任务的错误 → 用户可读的话。页面里不要散写文案，统一走这里。
 * 只处理契约 6.12.6 里 DocService 会返回的码：INVALID_ARGUMENT / NOT_FOUND / UNSUPPORTED / IO_ERROR / CONVERT_DISK_FULL / CANCELED / INTERNAL。
 */
export function docErrorText(code: string, backendMessage?: string, detail?: string, maxPages = 5000): string {
  const msg = (backendMessage ?? '').trim()
  switch (code) {
    case 'UNSUPPORTED':
      return isTooManyPages(msg, docDetailHead(detail)) ? docTooManyPagesText(maxPages) : docUnsupportedText(msg, detail, maxPages)
    case 'INVALID_ARGUMENT':
      return msg === OOXML_INVALID_MESSAGE ? DOC_FILE_BROKEN_TEXT : msg || FALLBACK_DESCRIPTION
    case 'NOT_FOUND':
      return DOC_NOT_FOUND_TEXT
    case 'CONVERT_DISK_FULL':
      return taskErrorMessages.CONVERT_DISK_FULL.description
    case 'IO_ERROR':
      return DOC_PDF_NO_PERMISSION_TEXT // 读源文件失败（IO_ERROR）
    case 'CANCELED':
      return '操作已取消。'
    default:
      return msg || FALLBACK_DESCRIPTION
  }
}

// ---- PDF 预览（设计说明 5.3，全部待确认）----
export const DOC_PDF_ERROR_TITLE = '无法预览这个 PDF'
export const DOC_PDF_LOADING_TITLE = '正在加载 PDF…'
export const DOC_PDF_PARSE_FAILED_TEXT = 'PDF 内容无法解析，文件可能已损坏。'
/** pdf.js 抛出的解析失败没有后端错误码，页面上显示这个前端自定义的显示码（不发给后端）；pdfErrorView 的 code 传它表示解析失败 */
export const DOC_PDF_PARSE_FAILED_CODE = 'PDF_PARSE_FAILED'
export const DOC_PDF_INVALID_TEXT = '这不是有效的 PDF 文件。'
export const DOC_PDF_NOT_FOUND_TEXT = '找不到这个文件，可能已被移动或删除。'
export const DOC_PDF_NO_PERMISSION_TEXT = '没有读取这个文件的权限。'
export const DOC_PDF_CHANGED_TEXT = '读取时文件被修改了，请重试。'
export const DOC_PDF_PASSWORD_TITLE = '这个 PDF 需要密码'
export const DOC_PDF_PASSWORD_PROMPT = '请输入密码后查看，密码只用于本次预览。'
export const DOC_PDF_PASSWORD_PLACEHOLDER = '输入密码'
export const DOC_PDF_PASSWORD_WRONG = '密码不正确，请重新输入'
export const DOC_PDF_EMPTY_TITLE = '预览 PDF'
export const DOC_PDF_EMPTY_HINT = '选择或拖入 PDF 文件，也可以从右侧列表打开最近的文件'
export const DOC_PDF_NOT_OPENED = '未打开 PDF'

// 后端 OpenPDF / ReadPDFChunk 里固定的 message（pdf.go）。detail 是路径或字节数，不能用来区分，所以对 message 精确比较。
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
export function pdfErrorView(code: string, backendMessage?: string, maxPdfBytes = 512 * 1024 * 1024): PdfErrorView {
  const msg = (backendMessage ?? '').trim()
  switch (code) {
    case DOC_PDF_PARSE_FAILED_CODE:
      return { text: DOC_PDF_PARSE_FAILED_TEXT, code: DOC_PDF_PARSE_FAILED_CODE, retry: true }
    case 'INVALID_ARGUMENT':
      if (msg === PDF_TOO_LARGE_MESSAGE) return { text: `文件超过 ${Math.round(maxPdfBytes / (1024 * 1024))} MiB，暂不支持预览。`, code, retry: false }
      if (PDF_NOT_PDF_MESSAGES.includes(msg)) return { text: DOC_PDF_INVALID_TEXT, code, retry: false }
      return { text: msg || FALLBACK_DESCRIPTION, code, retry: false }
    case 'NOT_FOUND':
      return { text: DOC_PDF_NOT_FOUND_TEXT, code, retry: false }
    case 'IO_ERROR':
      return { text: PDF_CHANGED_MESSAGES.includes(msg) ? DOC_PDF_CHANGED_TEXT : DOC_PDF_NO_PERMISSION_TEXT, code, retry: true }
    default:
      return { text: msg || FALLBACK_DESCRIPTION, code, retry: true }
  }
}
/** 浏览器里的模拟环境（没有 window.go）才显示的说明 */
export const DOC_DEMO_NOTE = '当前是演示数据：不会真的转换或读取文件'

// ---- 文档页其他文案（设计说明第 5 节；标题「最近生成的 PDF」与现状不符，见设计说明 8 节问题 1，待架构师 / 设计师确认）----
/** 待确认：现状列表只含「打开过预览」的 PDF（OpenPDF 是唯一写入点），转换产物不自动进列表 */
export const DOC_RECENT_TITLE = '最近生成的 PDF'
export const DOC_RECENT_EMPTY_TITLE = '还没有生成过 PDF'
export const DOC_RECENT_EMPTY_HINT = '转换完成后，PDF 会显示在这里'
export const DOC_RECENT_REMOVE_TIP = '从列表移除（不删除文件）'
export const DOC_RECENT_MISSING = '文件已被移动或删除'
export const DOC_RECENT_LIMIT_NOTE = '仅显示最近 200 条'
export const DOC_OUTPUT_SAME_AS_SOURCE = '与源文件相同的文件夹'
export const DOC_DROP_TITLE = '拖入 Word、Excel、PPT 文件'
export const DOC_DROP_HOVER = '松开鼠标添加文件'
export const DOC_PDF_DROP_HOVER = '松开鼠标打开 PDF'
