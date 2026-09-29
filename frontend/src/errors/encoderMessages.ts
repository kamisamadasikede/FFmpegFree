/**
 * 编码设备（GPU 加速）相关文案，集中放在这里。
 * 状态：设计说明《编码设备-设计说明-v0.1》§2.3 的设计师自拟文案，**全部待产品经理确认**。
 * 产品经理在群里 00:23 定的回退提示几句没能在这里读到，先用设计稿文案；定稿后只改这个文件（api.check.ts 里有一条自检锁着这些字，改文案时同步改）。
 * 界面不显示 NVENC / QSV 之类编码器名（后端返回里也没有）。
 */
export const ENCODER_PANEL_TITLE = '编码设备'
export const ENCODER_ROW_DESC = '格式转换和直播转码使用所选设备的硬件编码器；只转封装（复制流）的任务不受影响。'
export const ENCODER_OPTION_AUTO = '自动（有显卡优先）'
export const ENCODER_OPTION_CPU = 'CPU'
export const ENCODER_GROUP_GPU = '显卡'
export const ENCODER_DETECTING_SELECT = '正在检测…'
export const ENCODER_DETECTING_NOTE = '正在检测可用的显卡…'
export const ENCODER_REDETECT = '重新检测'
export const ENCODER_NONE_NOTE = '未检测到可用的显卡，将使用 CPU'
export const ENCODER_UNAVAILABLE_NOTE = '所选的显卡当前不可用（可能已被拔出或驱动异常），转码会改用 CPU。建议改回“自动”。'
export const ENCODER_UNAVAILABLE_ACTION = '改回“自动”'
/** 所选显卡本次检测里找不到（连名字都拿不到）时下拉里显示的占位名 */
export const ENCODER_UNKNOWN_SELECTED = '所选显卡'
export const ENCODER_FAILED_NOTE = '显卡检测失败：无法读取 ffmpeg 的编码器列表。转码会先使用 CPU，可以稍后重新检测。'
export const ENCODER_NO_FFMPEG_NOTE = '需要先安装 ffmpeg，才能检测显卡。'
/** 设计稿没有这一条：选了 CPU 时状态行的说明（新增，待确认） */
export const ENCODER_CPU_NOTE = '将使用 CPU 转换和直播转码。'
export const encoderAutoNote = (n: number, firstName: string) => `检测到 ${n} 张可用显卡。选“自动”时，将优先使用 ${firstName}。`
export const encoderGpuNote = (name: string) => `将使用 ${name} 转换和直播转码。硬件编码失败时，会自动改用 CPU。`
export const encoderComboLabel = (current: string) => `编码设备：${current}`

// ---- 硬件编码失败、已自动回退 CPU 的提示（契约 v0.18 §9.7；按功能区分文案，转换 / 直播 / 任务行沿用设计稿，剪辑导出一条是新增；全部待产品经理确认）----
export const ENCODER_FALLBACK_CONVERT = '硬件编码失败，已自动改用 CPU 完成转换。'
export const ENCODER_FALLBACK_LIVE = '显卡编码启动失败，已自动改用 CPU 推流。'
export const ENCODER_FALLBACK_TASK_ROW = '硬件编码失败，已自动改用 CPU 继续转换。'
export const ENCODER_FALLBACK_SETTINGS_LINK = '编码设置'
export const ENCODER_FALLBACK_LOG_LINK = '查看日志'
export const ENCODER_FALLBACK_CLOSE = '关闭提示'
/** 嵌在剪辑导出条里的回退提示，关闭按钮的读屏名（外层导出条已有一个“关闭提示”，避免读屏读到两个同名按钮；只是无障碍标签，不显示） */
export const ENCODER_FALLBACK_CLOSE_INNER = '关闭回退提示'
/** 剪辑导出的提示条（设计稿没有，新增，待确认） */
export const ENCODER_FALLBACK_EXPORT = '硬件编码失败，已自动改用 CPU 完成导出。'

// ---- 任务里“使用的设备”（只显示设备名，不显示编码器名、不显示 id）----
export const ENCODER_DEVICE_LABEL = '编码设备'
export const ENCODER_DEVICE_CPU_NAME = 'CPU'
/** 设备已不存在 / 名字读不到时的退化文字 */
export const ENCODER_DEVICE_GPU_FALLBACK_NAME = '显卡'
export const encoderUsedDevice = (name: string) => `使用 ${name}`

/**
 * hwFallbackReason（契约 §9.7 固定枚举）→ 一句用户文案，只用在任务详情的次要说明里（主提示条用上面按功能区分的三条）。
 * 枚举与后端 internal/ffmpeg/hwenc.go、契约 9.7、前端 api/taskTypes.ts 一一对应（api.check.ts 锁着）；文案不含编码器名。**待产品经理确认**。
 */
export const ENCODER_FALLBACK_REASONS: Readonly<Record<string, string>> = {
  device_unavailable: '提交时所选显卡不可用。',
  nvenc_init_failed: '显卡初始化失败，可能是驱动过旧或显卡被占用。',
  qsv_init_failed: '显卡初始化失败，可能是驱动过旧或显卡被占用。',
  amf_init_failed: '显卡初始化失败，可能是驱动过旧或显卡被占用。',
  videotoolbox_failed: '系统的硬件编码初始化失败。',
  encoder_unavailable: '当前 ffmpeg 不支持这张显卡的硬件编码。',
  encoder_start_failed: '硬件编码启动时出错。',
}
/** 未知枚举（后端新增而前端没跟上）的兜底句 */
export const ENCODER_FALLBACK_REASON_GENERIC = '硬件编码没有成功。'
export const encoderFallbackReasonText = (reason: string | undefined | null): string =>
  (reason && Object.prototype.hasOwnProperty.call(ENCODER_FALLBACK_REASONS, reason) ? ENCODER_FALLBACK_REASONS[reason] : ENCODER_FALLBACK_REASON_GENERIC)
