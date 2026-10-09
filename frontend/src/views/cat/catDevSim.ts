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
 */
import type { CatStreamEvent, CatTurnEvent } from '@/api/catStream'

export type CatSimFlag = 'checking' | 'stream' | 'noproj'

export const catSim: ReadonlySet<CatSimFlag> = (() => {
  if (!import.meta.env.DEV || typeof window === 'undefined') return new Set<CatSimFlag>()
  if ((window as unknown as { go?: unknown }).go) return new Set<CatSimFlag>()
  const v = new URLSearchParams(window.location.search).get('cat_sim') ?? ''
  return new Set(v.split(',').map((x) => x.trim()).filter((x): x is CatSimFlag => x === 'checking' || x === 'stream' || x === 'noproj'))
})()

const SIM_TEXT =
  '好的，我先看一下第 4 集的字幕时间轴。整体节奏是对的，但第 12 到 18 条字幕整体比画面晚了大约 0.4 秒，' +
  '建议统一往前挪；第 31 条和第 32 条有重叠，可以把第 31 条的结束时间改到第 32 条开始之前。其余部分没有发现明显问题。'

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
  const tick = () => {
    if (isStopped()) return
    if (i >= SIM_TEXT.length) {
      onMessage({ convId, turnId, messageId, seq: ++seq, op: 'done', role: 'assistant', text: '' })
      onTurn({ convId, turnId, status: 'completed' })
      return
    }
    const part = SIM_TEXT.slice(i, i + 6)
    i += 6
    onMessage({ convId, turnId, messageId, seq: ++seq, op: 'append', role: 'assistant', text: part })
    setTimeout(tick, 120)
  }
  setTimeout(tick, 400)
}
