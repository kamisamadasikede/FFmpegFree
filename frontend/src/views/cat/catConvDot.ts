
import { t } from '@/i18n'/**
 * 侧栏对话标题左边的状态点，一行最多一个。
 * 优先级：进行中 run > 失败 bad > 有新回复未读 new > 已完成 ok > 走查数据。
 * 以前「未读」是另一个独立绿点，后台完成时 ok 绿点和未读绿点会同时出现。
 */
export type ConvDot = '' | 'idle' | 'run' | 'ok' | 'bad' | 'new'

/**
 * running：这一轮还在（含正在停止）。
 * outcome：这次打开应用里记下的上次结果，ok 成功，bad 失败。
 * stored：走查数据里原来的点；真实会话一般没有。
 */
export function convDot(running: boolean, outcome: string, stored?: string, unseen = false): ConvDot {
  if (running) return 'run'
  if (outcome === 'bad') return 'bad'
  if (unseen) return 'new'
  if (outcome === 'ok') return 'ok'
  if (stored === 'ok' || stored === 'idle' || stored === 'run' || stored === 'bad') return stored
  return ''
}

/** 读屏用。灰点不报。 */
export function convDotLabel(dot: string): string {
  if (dot === 'run') return t('cat.misc.s_1b7f7edf64')
  if (dot === 'ok') return t('cat.misc.s_fad5222ca0')
  if (dot === 'bad') return t('cat.misc.s_d49180ecf8')
  if (dot === 'new') return t('cat.misc.s_a9f18180b0')
  return ''
}
