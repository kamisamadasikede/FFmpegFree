
import { t } from '@/i18n'/**
 * 编码设备（GPU 加速）相关文案，集中放在这里。
 * 状态：设计说明《编码设备-设计说明-v0.1》§2.3 的设计师自拟文案，**全部待产品经理确认**。
 * 产品经理在群里 00:23 定的回退提示几句没能在这里读到，先用设计稿文案；定稿后只改这个文件（api.check.ts 里有一条自检锁着这些字，改文案时同步改）。
 * 界面不显示 NVENC / QSV 之类编码器名（后端返回里也没有）。
 * 产品经理定稿（小修订包 12）：界面叫“显卡编码”，不用“硬件编码”；转换任务说“转换”，直播任务说“推流”，都不用“转码”；全角标点。
 */
export const ENCODER_PANEL_TITLE = t('errors.extra.s_e252a5ef')
export const ENCODER_ROW_DESC = t('errors.extra.s_86da1321')
export const ENCODER_OPTION_AUTO = t('errors.extra.s_082372a0')
export const ENCODER_OPTION_CPU = 'CPU'
export const ENCODER_GROUP_GPU = t('errors.extra.s_d405d97b')
export const ENCODER_DETECTING_SELECT = t('errors.extra.s_b2779045')
export const ENCODER_DETECTING_NOTE = t('errors.extra.s_bc5490b7')
export const ENCODER_REDETECT = t('errors.extra.s_f515f8fb')
export const ENCODER_NONE_NOTE = t('errors.extra.s_efc6cc5e')
export const ENCODER_UNAVAILABLE_NOTE = t('errors.extra.s_7b83da67')
export const ENCODER_UNAVAILABLE_ACTION = t('errors.extra.s_0a2e5d7a')
/** 所选显卡本次检测里找不到（连名字都拿不到）时下拉里显示的占位名 */
export const ENCODER_UNKNOWN_SELECTED = t('errors.extra.s_ecc5d7c5')
export const ENCODER_FAILED_NOTE = t('errors.extra.s_32ad6042')
export const ENCODER_NO_FFMPEG_NOTE = t('errors.extra.s_915db52c')
/** 设计稿没有这一条：选了 CPU 时状态行的说明（新增，待确认） */
export const ENCODER_CPU_NOTE = t('errors.extra.s_93118152')
export const encoderAutoNote = (n: number, firstName: string) => t('errors.encoder.autoNote', { n, firstName })
export const encoderGpuNote = (name: string) => t('errors.encoder.gpuNote', { name })
export const encoderComboLabel = (current: string) => t('errors.encoder.comboLabel', { current })

// ---- 显卡编码失败、已自动回退 CPU 的提示（契约 v0.18 §9.7；按功能区分文案，转换 / 直播 / 任务行沿用设计稿；全部待产品经理确认）----
export const ENCODER_FALLBACK_CONVERT = t('errors.extra.s_a4d1bd58')
/** 转换页 v2 完成的记录下方（设计 §3.2 / §五 “回退（完成）”） */
export const ENCODER_FALLBACK_CONVERT_DONE = t('errors.extra.s_372e63b9')
export const ENCODER_FALLBACK_LIVE = t('errors.extra.s_67fb61fc')
/** 任务中心行：已结束的历史任务（产品经理定稿；都不写“继续转换”） */
export const ENCODER_FALLBACK_TASK_ROW_DONE = t('errors.extra.s_1e915b17')
/** 任务中心行：还在运行 / 排队中的转换任务（产品经理没单独定，先用不带“完成”“继续”的说法，待确认） */
export const ENCODER_FALLBACK_TASK_ROW = t('errors.extra.s_a190a4b7')
export const ENCODER_FALLBACK_SETTINGS_LINK = t('errors.extra.s_d7bb1fec')
export const ENCODER_FALLBACK_LOG_LINK = t('errors.viewLog')
export const ENCODER_FALLBACK_CLOSE = t('errors.extra.s_c335e973')

// ---- 任务里“使用的设备”（只显示设备名，不显示编码器名、不显示 id）----
export const ENCODER_DEVICE_LABEL = t('errors.extra.s_e252a5ef')
export const ENCODER_DEVICE_CPU_NAME = 'CPU'
/** 任务详情设备一栏：显卡编码失败、已回退时（产品经理定稿；全文交给提示条，这里只放短标） */
export const ENCODER_DEVICE_CPU_FALLBACK_NAME = t('errors.extra.s_fc987a3f')
/** 短标的悬停说明（设计师稿 v0.1.2 建议；按定稿用词写“显卡编码”） */
export const ENCODER_DEVICE_CPU_FALLBACK_TITLE = t('errors.extra.s_0eb5c1a2')
/** 设备已不存在 / 名字读不到时的退化文字 */
export const ENCODER_DEVICE_GPU_FALLBACK_NAME = t('errors.extra.s_d405d97b')
export const encoderUsedDevice = (name: string) => t('errors.encoder.usedDevice', { name })

/**
 * hwFallbackReason（契约 §9.7 固定枚举）→ 一句用户文案（产品经理定稿），**只放在任务详情的“查看日志”面板里**，任务行 / 任务中心行不显示；不出现编码器名。
 * 枚举与后端 internal/ffmpeg/hwenc.go、契约 9.7、前端 api/taskTypes.ts 一一对应（api.check.ts 锁着）。
 */
const ENCODER_FALLBACK_START_FAILED = t('errors.extra.s_7b5f128d')
export const ENCODER_FALLBACK_REASONS: Readonly<Record<string, string>> = {
  device_unavailable: t('errors.extra.s_73c3f977'),
  encoder_unavailable: t('errors.extra.s_8b7f6d80'),
  nvenc_init_failed: ENCODER_FALLBACK_START_FAILED,
  qsv_init_failed: ENCODER_FALLBACK_START_FAILED,
  amf_init_failed: ENCODER_FALLBACK_START_FAILED,
  videotoolbox_failed: ENCODER_FALLBACK_START_FAILED,
  encoder_start_failed: ENCODER_FALLBACK_START_FAILED,
}
/** 未知枚举（后端新增而前端没跟上）/ 没给原因的兜底句 */
export const ENCODER_FALLBACK_REASON_GENERIC = t('errors.extra.s_f0ce2221')
export const encoderFallbackReasonText = (reason: string | undefined | null): string =>
  (reason && Object.prototype.hasOwnProperty.call(ENCODER_FALLBACK_REASONS, reason) ? ENCODER_FALLBACK_REASONS[reason] : ENCODER_FALLBACK_REASON_GENERIC)
