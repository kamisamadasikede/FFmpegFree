/** 侧栏对话标题左边的状态点。进行中优先于上一次结果。 */
export type ConvDot = '' | 'idle' | 'run' | 'ok' | 'bad'

/**
 * running：这一轮还在（含正在停止）。
 * outcome：这次打开应用里记下的上次结果，ok 成功，bad 失败。
 * stored：走查数据里原来的点；真实会话一般没有。
 */
export function convDot(running: boolean, outcome: string, stored?: string): ConvDot {
  if (running) return 'run'
  if (outcome === 'ok' || outcome === 'bad') return outcome
  if (stored === 'ok' || stored === 'idle' || stored === 'run' || stored === 'bad') return stored
  return ''
}

/** 读屏用。灰点不报。 */
export function convDotLabel(dot: string): string {
  if (dot === 'run') return '正在回复'
  if (dot === 'ok') return '已完成'
  if (dot === 'bad') return '回复失败'
  return ''
}
