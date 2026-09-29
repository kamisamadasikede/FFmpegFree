// 直播预览（设计稿 直播页-来源选择与预览-设计说明 v0.1 + 父代理 2026-09-30 01:26 调整）文案。
// 文案（设计稿自拟；契约只给了“开启预览会多占用少量 CPU”和“会话已结束”的意思）。除注明“已定”的以外，均待产品经理确认。改字只改这里。
export const PREVIEW_PANEL_TITLE = '预览'
export const PREVIEW_RATE_NOTE = '预览约每秒 2 帧，仅供确认画面'
export const PREVIEW_EMPTY_TITLE = '还没有进行中的会话'
export const PREVIEW_EMPTY_HINT = '开始推流后，这里会显示预览画面'
/** 父代理调整：加载中文案 */
export const PREVIEW_LOADING_TITLE = '正在获取画面，通常需要几秒' // 正文架构师 / 前端定稿
export const PREVIEW_LOADING_HINT_PUSH = '不影响推流，可以先做别的' // 副文案待产品经理确认
export const PREVIEW_LOADING_HINT_PULL = '不影响播放，可以先做别的'
export const PREVIEW_FAILED_TITLE = '预览暂时不可用'
export const PREVIEW_FAILED_HINT_PUSH = '不影响推流，可以稍后重试'
export const PREVIEW_FAILED_HINT_PULL = '不影响播放，可以稍后重试'
export const PREVIEW_RETRY = '重试'
/** 父代理调整：会话没开预览（开始时把开关关了）的空态 */
export const PREVIEW_OFF_TITLE = '该会话未开启预览' // 标题架构师 / 前端定稿；副文案待产品经理确认
export const PREVIEW_OFF_HINT_PUSH = '预览只能在开始推流前选择，下次开始时可开启'
export const PREVIEW_OFF_HINT_PULL = '预览只能在开始播放前选择，下次播放时可开启'
export const PREVIEW_ENDED_TITLE = '会话已结束'
export const PREVIEW_ENDED_NOFRAME_HINT = '没有可显示的画面'

// 表单里的开关（预览是会话启动参数：只在开始前可选；父代理 01:55 按定稿设计说明落地）
export const PREVIEW_SWITCH_LABEL = '开启预览' // 架构师 / 前端定稿
export const PREVIEW_SWITCH_NOTE = '开启预览会多占用少量 CPU，只能在开始前选择' // 架构师 / 前端定稿
export const PREVIEW_SWITCH_NOTE_STARTING = '正在开始推流，暂不能更改' // 产品经理已定：开始中开关和“开始推流”一起置灰（文案自拟）
export const PREVIEW_SWITCH_NOTE_PLAYING = '播放中不能更改，下次播放生效' // 待产品经理确认
// 会话行 / 播放器控制条里的只读文字
export const PREVIEW_ROW_ON = '预览：开' // 架构师 / 前端定稿
export const PREVIEW_ROW_OFF = '预览：关'
export const PREVIEW_ROW_TITLE = '预览是开始推流前选好的，运行中不能更改' // 待产品经理确认
export const PREVIEW_ROW_CURRENT = '正在预览' // 待产品经理确认
export const PREVIEW_ROW_SELECTED = '当前选中' // 预览为关的会话被选中时；待产品经理确认
export const PREVIEW_ROW_VIEW = '查看预览' // 待产品经理确认

/** 读屏播报（只在状态变化时播，不随换帧播） */
export const PREVIEW_LIVE_LOADING = '正在获取画面，通常需要几秒'
export const PREVIEW_LIVE_OK = '预览已开启'
export const PREVIEW_LIVE_FAILED_PUSH = '预览暂时不可用，不影响推流'
export const PREVIEW_LIVE_FAILED_PULL = '预览暂时不可用，不影响播放'
export const PREVIEW_LIVE_OFF = '该会话未开启预览'
export const PREVIEW_LIVE_ENDED = '会话已结束'
/** 预览图 alt：随会话变化，不随帧变化；address 用脱敏形式 */
export const previewAlt = (kind: 'push' | 'pull', address: string): string => (kind === 'pull' ? `${address} 的拉流画面预览` : `${address} 的推流画面预览`)
/** 地址协议后端不出预览（ws / wss）时拉流舞台的说明；设计稿没有，自拟 */
export const PREVIEW_UNSUPPORTED_TITLE = '这种地址暂不支持预览'
export const PREVIEW_UNSUPPORTED_HINT = '不影响播放'
