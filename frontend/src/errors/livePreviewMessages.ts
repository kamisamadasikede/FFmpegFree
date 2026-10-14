import { t } from '@/i18n'
// 直播预览（设计稿 直播页-来源选择与预览-设计说明 v0.1 + 父代理 2026-09-30 01:26 调整）文案。
// 文案（设计稿自拟；契约只给了“开启预览会多占用少量 CPU”和“会话已结束”的意思）。除注明“已定”的以外，均待产品经理确认。改字只改这里。
export const PREVIEW_PANEL_TITLE = t('errors.extra.s_645dbc55')
export const PREVIEW_EMPTY_TITLE = t('errors.extra.s_8ea54129')
export const PREVIEW_EMPTY_HINT = t('errors.extra.s_a859b17a')
/** 父代理调整：加载中文案 */
export const PREVIEW_LOADING_TITLE = t('errors.extra.s_59990f29') // 正文架构师 / 前端定稿
export const PREVIEW_LOADING_HINT_PUSH = t('errors.extra.s_f9a4ac3f') // 副文案待产品经理确认
export const PREVIEW_LOADING_HINT_PULL = t('errors.extra.s_2d2ec7a5')
export const PREVIEW_FAILED_TITLE = t('errors.extra.s_cdd39780')
export const PREVIEW_FAILED_HINT_PUSH = t('errors.extra.s_423f6e2a')
export const PREVIEW_FAILED_HINT_PULL = t('errors.extra.s_16881e5f')
export const PREVIEW_RETRY = t('errors.retry')
/** 父代理调整：会话没开预览（开始时把开关关了）的空态 */
export const PREVIEW_OFF_TITLE = t('errors.extra.s_81140f36') // 标题架构师 / 前端定稿；副文案待产品经理确认
export const PREVIEW_OFF_HINT_PUSH = t('errors.extra.s_dc13dba2')
export const PREVIEW_OFF_HINT_PULL = t('errors.extra.s_848c7d70')
export const PREVIEW_ENDED_TITLE = t('errors.extra.s_f6c6c3fe')
export const PREVIEW_ENDED_NOFRAME_HINT = t('errors.extra.s_998b2f89')

// 表单里的开关（预览是会话启动参数：只在开始前可选；父代理 01:55 按定稿设计说明落地）
export const PREVIEW_SWITCH_LABEL = t('errors.extra.s_9b2fabf7') // 架构师 / 前端定稿
/** 设计说明 §2.5 的说明去掉“只能在开始前选择”：契约 v0.25 ⑤ 推流中途开关预览只连接 / 断开播放器，不重启推流 */
export const PREVIEW_SWITCH_NOTE = t('errors.extra.s_579fe606')
// 会话行 / 播放器控制条里的只读文字
export const PREVIEW_ROW_ON = t('errors.extra.s_509a3b7d') // 架构师 / 前端定稿
export const PREVIEW_ROW_OFF = t('errors.extra.s_02e8f17b')
export const PREVIEW_ROW_TITLE = t('errors.extra.s_2af111b7') // 待产品经理确认
export const PREVIEW_ROW_CURRENT = t('errors.extra.s_f5278865') // 待产品经理确认
export const PREVIEW_ROW_SELECTED = t('errors.extra.s_bf94700b') // 预览为关的会话被选中时；待产品经理确认
export const PREVIEW_ROW_VIEW = t('errors.extra.s_624bfa0f') // 待产品经理确认

/** 读屏播报（只在状态变化时播，不随换帧播） */
export const PREVIEW_LIVE_LOADING = t('errors.extra.s_59990f29')
export const PREVIEW_LIVE_OK = t('errors.extra.s_ec78d76d')
export const PREVIEW_LIVE_FAILED_PUSH = t('errors.extra.s_9e1dd852')
export const PREVIEW_LIVE_FAILED_PULL = t('errors.extra.s_6d39fbc2')
export const PREVIEW_LIVE_OFF = t('errors.extra.s_81140f36')
export const PREVIEW_LIVE_ENDED = t('errors.extra.s_f6c6c3fe')
/** 预览图 alt：随会话变化，不随帧变化；address 用脱敏形式 */
export const previewAlt = (kind: 'push' | 'pull', address: string): string => (kind === 'pull' ? t('errors.extra.s_a2c0341d', { v0: address }) : t('errors.extra.s_3e4ccc1a', { v0: address }))
/** 地址协议后端不出预览（ws / wss）时拉流舞台的说明；设计稿没有，自拟 */
export const PREVIEW_UNSUPPORTED_TITLE = t('errors.extra.s_2eb87402')
export const PREVIEW_UNSUPPORTED_HINT = t('errors.extra.s_5d986092')

// ---- 包 21 实时播放器（设计说明 v0.1 + 契约 v0.25 §6.10.3.6 / §6.10.3.7，产品经理 10-08 已定）----
export const LP_CONNECTING = t('errors.extra.s_594397c9')
export const LP_MUTED_HINT = t('errors.extra.s_122160f1')
export const LP_UNSUP_PUSH = t('errors.extra.s_e63b9ee5')
export const LP_UNSUP_PULL = t('errors.extra.s_d4e1748b')
/** 契约 reason=preview_unavailable：和推流编码不支持用同一句（架构师定） */
export const LP_UNAVAILABLE = LP_UNSUP_PUSH
/** 拉流页的 preview_unavailable（产品经理 10-08 定，包 22）：和 codec 那句（LP_UNSUP_PULL）分开，两句不混用；推流页仍用 LP_UNAVAILABLE */
export const LP_UNAVAILABLE_PULL = t('errors.extra.s_79328902')
export const LP_END_PUSH = t('errors.extra.s_036f7f59')
export const LP_END_PULL = t('errors.extra.s_c715d1e9')
/** 拉流不是用户点停止而结束（live:pull ended：远端停止发布或连接正常关闭，契约 6.10.3.7 ⑩）时的第二行，旁边是「重新拉流」。产品经理 10-08 定稿；用户自己点停止时没有这一行 */
export const LP_END_PULL_REMOTE = t('errors.extra.s_ec40bf94')
/**
 * 直播页上推流被中断时画面中央那句（包 24 N6，产品经理 10-08 定：用户就在直播页，不再说「回到直播页」，与拉流那句对称）。
 * 按钮仍是「重新推流」，不跳页面。直播页以外（任务中心、错误卡）仍用原句「推流被中断，请回到直播页重新推流。」。
 */
export const LP_BREAK_PUSH = t('errors.extra.s_fee1655b')
/** 直播页以外（任务中心）：同一句，但告诉用户回到直播页 */
export const LP_BREAK_PUSH_AWAY = t('errors.extra.s_2f9a4a3d')
/** 产品经理已定：和按钮同一个动词，不用「请重新开始播放」。拉流不论在哪一页都是这一句 */
export const LP_BREAK_PULL = t('errors.extra.s_e201003d')

/**
 * 直播被中断的那一句。只看 detail 首行 reason（reason=push / reason=pull），不看 message。
 * 后端还没带 reason 时按任务类型兜底（live_* 里带 pull 的算拉流，其余直播算推流）。
 * LIVE_SOURCE_GONE 不在这里：窗口 / 屏幕关掉仍用原来的「所选窗口已不可用」。
 * 返回 null 表示这条不是“被中断”文案。
 */
export function liveInterruptView(o: { reason?: string | null; code?: string | null; taskType?: string | null; onLivePage?: boolean }): { title: string; description: string; sentence: string } | null {
  if (o.code === 'LIVE_SOURCE_GONE') return null
  const pushSentence = o.onLivePage ? LP_BREAK_PUSH : LP_BREAK_PUSH_AWAY
  const which = o.reason === 'push' || o.reason === 'pull'
    ? o.reason
    : o.reason
      ? null
      : o.taskType && /pull/.test(o.taskType)
        ? 'pull'
        : o.taskType && o.taskType.startsWith('live_')
          ? 'push'
          : null
  if (!which) return null
  if (which === 'pull') return { title: t('errors.extra.s_f584c599'), description: t('errors.extra.s_f80b369f'), sentence: LP_BREAK_PULL }
  const description = o.onLivePage ? t('errors.extra.s_baf41d10') : t('errors.extra.s_168013b7')
  return { title: t('errors.extra.s_a91bd08f'), description, sentence: pushSentence }
}
/** 包 24 N2：拉到的流只有声音时舞台中间那行（设计 10-08 定稿，不带句号） */
export const LP_AUDIO_ONLY = t('errors.extra.s_0a8fca8a')
export const LP_RETRY_PUSH = t('errors.extra.s_10fb5b48')
export const LP_RETRY_PULL = t('errors.extra.s_9963a689')
export const LP_PULL_HINT = t('errors.extra.s_be8daae9')
/** 拉流地址校验报错（X3，产品经理 10-08 定）：和说明一样列全四种前缀 */
export const LP_PULL_URL_INVALID = t('errors.extra.s_60ad751c')
/** 拉流地址输入框的占位符（X2：界面不写 flv，照设计稿的示例） */
export const LP_PULL_PLACEHOLDER = 'https://live.example.com/room/8848'
/** 拉流开始前就失败（live:pull failed / StartPullPreview 出错）的兜底正文（G3，和后端拉流失败的文案一致）；后端 message 里写着「推流」时也用这句 */
export const LP_PULL_FAILED = t('errors.extra.s_551128e3')
/** 读屏播报（设计说明 §6.2）：开始、缓冲超过 2 秒 */
export const LP_LIVE_STARTED_PUSH = t('errors.extra.s_aea31660')
export const LP_LIVE_STARTED_PULL = t('errors.extra.s_2d38fcbf')
export const LP_LIVE_MUTED_SUFFIX = t('errors.extra.s_97240878')
export const LP_LIVE_BUFFERING = t('errors.extra.s_01a1c4a0')
export const LP_EMPTY = t('errors.extra.s_eafe1a0c')
export const LP_EMPTY_SESS = t('errors.extra.s_99de1782')
export const LP_EMPTY_SESS_HINT = t('errors.extra.s_9c10fcfa')
export const LP_EMPTY_PULL = t('errors.extra.s_981fc7de')
export const LP_KEY_HINT = t('errors.extra.s_5c6cc6d7')
export const LP_LIMIT = t('errors.extra.s_7af78b33')
export const LP_LAG = (n: number) => t('errors.extra.s_bea6e5bb', { n: n })
export const LP_CATCHUP = t('errors.extra.s_5c3decbe')
export const LP_ESC = t('errors.extra.s_ccd558d9')
