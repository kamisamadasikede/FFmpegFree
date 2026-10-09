/**
 * Cat 页开发走查开关（仅 `vite dev` + 纯浏览器 + 地址带 ?cat_sim=…）。
 *
 * - 正式包：import.meta.env.DEV 为 false，整段被裁掉，永远返回空集合
 * - Wails 里（有 window.go）：无效
 * - 默认（不带参数）：无效
 *
 * 取值（逗号分隔，可组合）：
 *   checking —— 组件状态停在「检查中」（发送置灰）
 *   stream   —— 组件状态 ready；发送后按定稿事件形状（cat:message append/done + cat:turn）推一段走查用文字，可点「停止」
 *   noproj   —— 没有项目（项目栏显示「还没有项目。」）
 *   missing  —— 「字幕项目」的文件夹不见了（项目标灰、输入框禁用）
 *   slowdel  —— 删除对话要 1.5 秒（走查「删除中」小转圈）
 *   delfail  —— 删除对话失败（走查「删除失败，请重试。」浮提示）
 *   caps     —— 模型 / 思考强度给一份走查用列表（含「超高」等任意档位），走查胶囊「默认」和选档
 *   md       —— 配合 stream：推一段 Markdown 回复（标题、列表、表格、代码块、引用、链接、外链图片、原始 HTML）
 *   mdhold   —— 配合 stream,md：推到代码块中间就停住（围栏未闭合、仍在生成），走查流式中的代码块
 *
 * 另：?cat_os=windows|darwin|linux —— 走查时「在…中显示」按指定平台取文案（默认按浏览器 userAgent 猜）。
 * 另：?cat_pick=<路径>[|<路径>…] —— 走查时「选择项目文件夹」依次返回这些路径（空 = 取消）；用完后返回递增的示例路径。
 */
import type { CatStreamEvent, CatTurnEvent } from '@/api/catStream'
import type { CatModel, CatThinkLevel } from '@/api/cat'

/** 走查：?cat_sim=caps 时的模型列表（第一项 = 默认模型） */
export function simModels(): CatModel[] {
  return [
    { id: 'sim-model-a', displayName: 'Cat 助手 1.0' },
    { id: 'sim-model-b', displayName: 'Cat 助手 1.0 快速' },
  ]
}

/** 走查：?cat_sim=caps 时的强度列表（按后端返回顺序原样显示，不假设只有低/中/高） */
export function simThinks(): CatThinkLevel[] {
  return [
    { id: 'low', displayName: '低' },
    { id: 'medium', displayName: '中' },
    { id: 'high', displayName: '高' },
    { id: 'xhigh', displayName: '超高' },
  ]
}

export type CatSimFlag = 'checking' | 'stream' | 'noproj' | 'missing' | 'slowdel' | 'delfail' | 'caps' | 'md' | 'mdhold'
const FLAGS: readonly string[] = ['checking', 'stream', 'noproj', 'missing', 'slowdel', 'delfail', 'caps', 'md', 'mdhold']

export const catSim: ReadonlySet<CatSimFlag> = (() => {
  if (!import.meta.env.DEV || typeof window === 'undefined') return new Set<CatSimFlag>()
  if ((window as unknown as { go?: unknown }).go) return new Set<CatSimFlag>()
  const v = new URLSearchParams(window.location.search).get('cat_sim') ?? ''
  return new Set(v.split(',').map((x) => x.trim()).filter((x): x is CatSimFlag => FLAGS.includes(x)))
})()

/** 走查：?cat_pick=… 指定的选文件夹结果（仅 vite dev + 纯浏览器） */
export const catSimPicks: string[] = (() => {
  if (!import.meta.env.DEV || typeof window === 'undefined') return []
  if ((window as unknown as { go?: unknown }).go) return []
  const v = new URLSearchParams(window.location.search).get('cat_pick')
  return v === null ? [] : v.split('|')
})()

/** 走查：?cat_os=… 指定平台（仅 vite dev + 纯浏览器） */
export const catSimOs: 'windows' | 'darwin' | 'linux' | '' = (() => {
  if (!import.meta.env.DEV || typeof window === 'undefined') return ''
  if ((window as unknown as { go?: unknown }).go) return ''
  const v = new URLSearchParams(window.location.search).get('cat_os')
  return v === 'windows' || v === 'darwin' || v === 'linux' ? v : ''
})()

const SIM_TEXT =
  '好的，我先看一下第 4 集的字幕时间轴。整体节奏是对的，但第 12 到 18 条字幕整体比画面晚了大约 0.4 秒，' +
  '建议统一往前挪；第 31 条和第 32 条有重叠，可以把第 31 条的结束时间改到第 32 条开始之前。其余部分没有发现明显问题。'

/** 走查 ?cat_sim=stream,md：Markdown 回复（mdhold 时停在 MD_HOLD 处，代码块未闭合） */
const SIM_MD =
  '## 字幕时间轴检查结果\n\n' +
  '整体节奏是对的，有 **两处** 需要调整，详见下表：\n\n' +
  '| 字幕 | 问题 | 建议 |\n| --- | --- | --- |\n| 第 12–18 条 | 比画面晚约 0.4 秒 | 统一提前 `400ms` |\n| 第 31、32 条 | 时间重叠 | 31 条结束改到 32 条开始前 |\n\n' +
  '处理步骤：\n\n1. 备份原字幕文件\n2. 批量平移时间轴\n   - 只选第 12–18 条\n   - 偏移量填 *-0.4s*\n3. 手动修正第 31 条\n\n' +
  '> 提示：平移后建议再整体播放一遍，确认口型对齐。\n\n' +
  '参考 [字幕格式说明](https://example.com/srt) 或发邮件到 [support@example.com](mailto:support@example.com)；' +
  '示意图 ![时间轴截图](https://example.com/timeline.png) 不会自动加载，<b>原始标签</b> 按文字显示。\n\n---\n\n' +
  '可以用下面的脚本批量平移：\n\n```python\nimport pysrt\n\nsubs = pysrt.open("ep04.srt")\n' +
  'for s in subs[11:18]:\n    s.shift(milliseconds=-400)\nsubs.save("ep04.fixed.srt", encoding="utf-8")\n```\n\n改完后告诉我，我再帮你复查。'
const MD_HOLD = 'for s in subs[11:18]:\n    s.shi'

/**
 * 走查用：按定稿事件形状逐段推送（每 120ms 一段 6 字）。isStopped 为真后不再推（与真实后端「迟到文字丢弃」一致）。
 * 只在 catSim.has('stream') 时由 catState.sendMessage 调用。
 */
export function simStream(
  convId: string,
  onMessage: (e: CatStreamEvent) => void,
  onTurn: (e: CatTurnEvent) => void,
  isStopped: () => boolean,
) {
  const turnId = `sim-turn-${Date.now().toString(36)}`
  const messageId = `sim-msg-${Date.now().toString(36)}`
  onTurn({ convId, turnId, status: 'running' })
  let i = 0
  let seq = 0
  const text = catSim.has('md') ? SIM_MD : SIM_TEXT
  const holdAt = catSim.has('md') && catSim.has('mdhold') ? SIM_MD.indexOf(MD_HOLD) + MD_HOLD.length : -1
  const step = catSim.has('md') ? 14 : 6
  const tick = () => {
    if (isStopped()) return
    if (holdAt >= 0 && i >= holdAt) return // 停在未闭合代码块中间，保持「生成中」
    if (i >= text.length) {
      onMessage({ convId, turnId, messageId, seq: ++seq, op: 'done', role: 'assistant', text: '' })
      onTurn({ convId, turnId, status: 'completed', contextUsed: 180432, contextWindow: 256000 })
      return
    }
    const part = text.slice(i, holdAt >= 0 ? Math.min(i + step, holdAt) : i + step)
    i += part.length
    onMessage({ convId, turnId, messageId, seq: ++seq, op: 'append', role: 'assistant', text: part })
    setTimeout(tick, 120)
  }
  setTimeout(tick, 400)
}
