/**
 * 编码设备（GPU 加速）相关文案，集中放在这里。
 * 状态：设计说明《编码设备-设计说明-v0.1》§2.3 的设计师自拟文案，**全部待产品经理确认**。
 * 产品经理在群里 00:23 定的回退提示几句没能在这里读到，先用设计稿文案；定稿后只改这个文件（api.check.ts 里有一条自检锁着这些字，改文案时同步改）。
 * 界面不显示 NVENC / QSV 之类编码器名（后端返回里也没有）。
 * 产品经理定稿（小修订包 12）：界面叫“显卡编码”，不用“硬件编码”；转换任务说“转换”，直播任务说“推流”，都不用“转码”；全角标点。
 */
export const ENCODER_PANEL_TITLE = '编码设备'
export const ENCODER_ROW_DESC = '转换、剪辑导出和直播推流会用所选设备做显卡编码；只复制音视频流、不重新编码的任务不受影响。'
export const ENCODER_OPTION_AUTO = '自动（有显卡优先）'
export const ENCODER_OPTION_CPU = 'CPU'
export const ENCODER_GROUP_GPU = '显卡'
export const ENCODER_DETECTING_SELECT = '正在检测…'
export const ENCODER_DETECTING_NOTE = '正在检测可用的显卡…'
export const ENCODER_REDETECT = '重新检测'
export const ENCODER_NONE_NOTE = '未检测到可用的显卡，将使用 CPU。'
export const ENCODER_UNAVAILABLE_NOTE = '所选显卡当前不可用，可能已被移除或驱动异常。转换和直播会先使用 CPU，建议改回“自动”。'
export const ENCODER_UNAVAILABLE_ACTION = '改回“自动”'
/** 所选显卡本次检测里找不到（连名字都拿不到）时下拉里显示的占位名 */
export const ENCODER_UNKNOWN_SELECTED = '所选显卡'
export const ENCODER_FAILED_NOTE = '显卡检测失败。转换和直播会先使用 CPU，可以点“重新检测”再试一次。'
export const ENCODER_NO_FFMPEG_NOTE = '需要先安装 ffmpeg 才能检测显卡，安装完成后会自动检测。'
/** 设计稿没有这一条：选了 CPU 时状态行的说明（新增，待确认） */
export const ENCODER_CPU_NOTE = '将使用 CPU 进行转换、剪辑导出和直播推流。'
export const encoderAutoNote = (n: number, firstName: string) => `检测到 ${n} 张可用显卡。选“自动”时，将优先使用 ${firstName}。`
export const encoderGpuNote = (name: string) => `将使用 ${name} 进行转换、剪辑导出和直播推流。显卡编码失败时，会自动改用 CPU。`
export const encoderComboLabel = (current: string) => `编码设备：${current}`

// ---- 显卡编码失败、已自动回退 CPU 的提示（契约 v0.18 §9.7；按功能区分文案，转换 / 直播 / 任务行沿用设计稿，剪辑导出一条是新增；全部待产品经理确认）----
export const ENCODER_FALLBACK_CONVERT = '显卡编码失败，已自动改用 CPU 转换。'
/** 转换页 v2 完成的记录下方（设计 §3.2 / §五 “回退（完成）”） */
export const ENCODER_FALLBACK_CONVERT_DONE = '显卡编码失败，已自动改用 CPU 完成转换。'
export const ENCODER_FALLBACK_LIVE = '显卡编码启动失败，已自动改用 CPU 推流。'
/** 任务中心行：已结束的历史任务（产品经理定稿；都不写“继续转换”） */
export const ENCODER_FALLBACK_TASK_ROW_DONE = '已自动改用 CPU 完成转换。'
/** 任务中心行：还在运行 / 排队中的转换任务（产品经理没单独定，先用不带“完成”“继续”的说法，待确认） */
export const ENCODER_FALLBACK_TASK_ROW = '已自动改用 CPU 转换。'
export const ENCODER_FALLBACK_SETTINGS_LINK = '编码设置'
export const ENCODER_FALLBACK_LOG_LINK = '查看日志'
export const ENCODER_FALLBACK_CLOSE = '关闭提示'
/** 剪辑导出的提示条（设计稿没有，新增，待确认） */
export const ENCODER_FALLBACK_EXPORT = '显卡编码失败，已自动改用 CPU 完成导出。'

// ---- 任务里“使用的设备”（只显示设备名，不显示编码器名、不显示 id）----
export const ENCODER_DEVICE_LABEL = '编码设备'
export const ENCODER_DEVICE_CPU_NAME = 'CPU'
/** 任务详情设备一栏：显卡编码失败、已回退时（产品经理定稿；全文交给提示条，这里只放短标） */
export const ENCODER_DEVICE_CPU_FALLBACK_NAME = 'CPU（已回退）'
/** 短标的悬停说明（设计师稿 v0.1.2 建议；按定稿用词写“显卡编码”） */
export const ENCODER_DEVICE_CPU_FALLBACK_TITLE = '显卡编码失败，已自动改用 CPU'
/** 设备已不存在 / 名字读不到时的退化文字 */
export const ENCODER_DEVICE_GPU_FALLBACK_NAME = '显卡'
export const encoderUsedDevice = (name: string) => `使用 ${name}`

/**
 * hwFallbackReason（契约 §9.7 固定枚举）→ 一句用户文案（产品经理定稿），**只放在任务详情的“查看日志”面板里**，任务行 / 任务中心行不显示；不出现编码器名。
 * 枚举与后端 internal/ffmpeg/hwenc.go、契约 9.7、前端 api/taskTypes.ts 一一对应（api.check.ts 锁着）。
 */
const ENCODER_FALLBACK_START_FAILED = '显卡编码器启动失败，已改用 CPU。'
export const ENCODER_FALLBACK_REASONS: Readonly<Record<string, string>> = {
  device_unavailable: '所选显卡当时不可用，已改用 CPU。',
  encoder_unavailable: '没有可用的显卡编码器，已改用 CPU。',
  nvenc_init_failed: ENCODER_FALLBACK_START_FAILED,
  qsv_init_failed: ENCODER_FALLBACK_START_FAILED,
  amf_init_failed: ENCODER_FALLBACK_START_FAILED,
  videotoolbox_failed: ENCODER_FALLBACK_START_FAILED,
  encoder_start_failed: ENCODER_FALLBACK_START_FAILED,
}
/** 未知枚举（后端新增而前端没跟上）/ 没给原因的兜底句 */
export const ENCODER_FALLBACK_REASON_GENERIC = '未能确定具体原因，详情见下方日志。'
export const encoderFallbackReasonText = (reason: string | undefined | null): string =>
  (reason && Object.prototype.hasOwnProperty.call(ENCODER_FALLBACK_REASONS, reason) ? ENCODER_FALLBACK_REASONS[reason] : ENCODER_FALLBACK_REASON_GENERIC)
