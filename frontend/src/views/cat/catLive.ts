
import { t } from '@/i18n'/** 刷新后接回还在回复的对话：新出现的接上，还在的继续读记录，已经结束的放开。 */
export interface LiveRef {
  convId: string
  turnId: string
}

export interface HeldTurn {
  convId: string
  adopted: boolean
}

export function liveTurnPlan(held: HeldTurn[], live: LiveRef[]): { adopt: LiveRef[]; refresh: string[]; release: string[] } {
  const liveIds = new Set(live.map((t) => t.convId).filter(Boolean))
  const heldIds = new Set(held.map((t) => t.convId))
  const adopt = live.filter((t) => t.convId && !heldIds.has(t.convId))
  const refresh: string[] = []
  const release: string[] = []
  for (const h of held) {
    if (!h.adopted) continue
    if (liveIds.has(h.convId)) refresh.push(h.convId)
    else release.push(h.convId)
  }
  return { adopt, refresh, release }
}

/** 记录没变就不要整段换掉，避免把正在看的位置打乱。 */
export function transcriptChanged(prev: { kind: string; text: string }[], next: { kind: string; text: string }[]): boolean {
  if (prev.length !== next.length) return true
  for (let i = 0; i < prev.length; i++) {
    if (prev[i].kind !== next[i].kind || prev[i].text !== next[i].text) return true
  }
  return false
}
