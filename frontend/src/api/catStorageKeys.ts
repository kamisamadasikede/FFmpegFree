
import { t } from '@/i18n'/** Cat 页写在本机 localStorage 里的键。不是界面文字，键名沿用旧值，改了会丢掉已记住的设置。 */
export const CAT_LAST_CONV_KEY = 'ffmpegfree.cat.lastConv'
export const CAT_LAST_GROUP_KEY = 'ffmpegfree.cat.lastGroup'
export const CAT_GROUP_POSITIONS_KEY = 'ffmpegfree.cat.groupPositions'
export const CAT_CONV_PREFS_KEY = 'ffmpegfree.cat.convPrefs'
export const CAT_PREVIEW_WIDTH_KEY = 'ffmpegfree.cat.previewWidth'
/** 侧栏 / 右栏收起状态（C3 / E3），'1' = 收起 */
export const CAT_RAIL_COLLAPSED_KEY = 'ffmpegfree.cat.railCollapsed'
export const CAT_RIGHT_COLLAPSED_KEY = 'ffmpegfree.cat.rightCollapsed'
/** 设置 → 通用 本机偏好（设计 v0.3.1 §3，不进 CatSettings）：新对话放在 / 回复逐字显示 / 上次新对话用的项目 */
export const CAT_NEW_CONV_PLACE_KEY = 'ffmpegfree.cat.newConvPlace'
export const CAT_TYPEWRITER_KEY = 'ffmpegfree.cat.typewriter'
export const CAT_LAST_NEW_PROJECT_KEY = 'ffmpegfree.cat.lastNewProject'
/** 每个项目是否已确认过「全部放开」（v0.44，键 = 项目路径） */
export const CAT_DANGER_ACK_KEY = 'ffmpegfree.cat.dangerAck'
