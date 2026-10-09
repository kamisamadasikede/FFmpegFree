/** 语音工具（转字幕）界面定稿文案。禁止 Whisper / 模型名 / Python / CLI / 「语言组件」。 */

export const VOICE_PAGE_LABEL = '语音工具'
export const ASR_COMPONENT_NAME = '语音识别组件'
export const TASK_TYPE_SPEECH_TO_SUBTITLE_LABEL = '转字幕'

/** 组件未发布横幅标题 / 正文（落地稿 01） */
export const ASR_NOT_PUBLISHED_TITLE = '语音识别组件还没准备好'
export const ASR_NOT_PUBLISHED =
  '语音识别组件还没准备好，发布后即可下载。'

export const ASR_EMPTY = '这段音频里没有识别到有效内容。'
export const ASR_EMPTY_TITLE = '没有识别到内容'
/** 与契约 v0.29.1 LANG_ASR_FAILED 一致 */
export const ASR_FAIL = '识别没完成，请稍后重试。'
export const ASR_FAIL_TITLE = '识别没完成'

export const EXPORT_BLOCK_TITLE = '还不能导出'
export const EXPORT_BLOCK_BODY =
  '时间不能重叠；结束时间必须晚于开始时间；单条最多 80 字。改完后再导出。'

export const CUE_MAX_CHARS = 80

export const TIER_STANDARD_LABEL = '标准'
export const TIER_HD_LABEL = '高清'
export const TIER_STANDARD_SIZE = '约 400 MB'
export const TIER_HD_SIZE = '约 1–1.5 GB'

export function tierSizeText(tier: 'standard' | 'hd'): string {
  return tier === 'hd' ? TIER_HD_SIZE : TIER_STANDARD_SIZE
}

export function tierGuideHint(tier: 'standard' | 'hd'): string {
  if (tier === 'hd') {
    return `当前是「高清」档（${TIER_HD_SIZE}）；设置里可换「标准」。`
  }
  return `当前是「标准」档（${TIER_STANDARD_SIZE}）；设置里可换「高清」。`
}
