// 直播预览（设计稿 直播页-来源选择与预览-设计说明 v0.1 + 父代理 2026-09-30 01:26 调整）文案。
// 文案（设计稿自拟；契约只给了“开启预览会多占用少量 CPU”和“会话已结束”的意思）。除注明“已定”的以外，均待产品经理确认。改字只改这里。
export const PREVIEW_PANEL_TITLE = '预览'
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
/** 设计说明 §2.5 的说明去掉“只能在开始前选择”：契约 v0.25 ⑤ 推流中途开关预览只连接 / 断开播放器，不重启推流 */
export const PREVIEW_SWITCH_NOTE = '开启预览会多占用少量 CPU'
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

// ---- 包 21 实时播放器（设计说明 v0.1 + 契约 v0.25 §6.10.3.6 / §6.10.3.7，产品经理 10-08 已定）----
export const LP_CONNECTING = '正在连接…'
export const LP_MUTED_HINT = '已静音，点击开启声音'
export const LP_UNSUP_PUSH = '这路视频无法在应用内预览，推流不受影响。'
export const LP_UNSUP_PULL = '这路视频无法在应用内播放。'
/** 契约 reason=preview_unavailable：和推流编码不支持用同一句（架构师定） */
export const LP_UNAVAILABLE = LP_UNSUP_PUSH
export const LP_END_PUSH = '推流已结束'
export const LP_END_PULL = '拉流已结束'
/** 拉流不是用户点停止而结束（live:pull ended：远端停止发布或连接正常关闭，契约 6.10.3.7 ⑩）时的第二行，旁边是「重新拉流」。产品经理 10-08 定稿；用户自己点停止时没有这一行 */
export const LP_END_PULL_REMOTE = '直播已停止，或连接已断开。'
/** 契约 6.10.3.7，产品经理已定（按钮仍是「重新推流」，不跳页面） */
export const LP_BREAK_PUSH = '推流被中断，请回到直播页重新推流。'
/** 产品经理已定：和按钮同一个动词，不用「请重新开始播放」 */
export const LP_BREAK_PULL = '拉流被中断，请重新拉流。'
export const LP_RETRY_PUSH = '重新推流'
export const LP_RETRY_PULL = '重新拉流'
export const LP_PULL_HINT = '支持 http://、https://、ws://、wss:// 开头的直播地址'
export const LP_EMPTY = '还没有进行中的预览'
export const LP_EMPTY_SESS = '还没有推流会话'
export const LP_EMPTY_SESS_HINT = '在右侧设置好后点“开始推流”'
export const LP_EMPTY_PULL = '还没有正在播放的流'
export const LP_KEY_HINT = '推流码会保留。切换页面或重新打开后仍在，默认显示成圆点。'
export const LP_LIMIT = '同时最多 4 路，同一地址只允许 1 路'
export const LP_LAG = (n: number) => `落后约 ${n} 秒`
export const LP_CATCHUP = '回到最新'
export const LP_ESC = '按 Esc 退出全屏'
