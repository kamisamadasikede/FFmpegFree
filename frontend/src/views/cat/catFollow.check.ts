// 回复变高时贴底跟随；用户往上翻之后停住，回到底部再跟上。
import { keepFollowing, nearBottom } from './catFollow'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catFollowChecks(eq: Eq): void {
  eq('跟随：内容还没高出视口', nearBottom(0, 100, 200), true)
  eq('跟随：正好在底部', nearBottom(600, 1000, 400), true)
  eq('跟随：差一截仍算在底部', nearBottom(560, 1000, 400), true)
  eq('跟随：往上翻开了就不再算底部', nearBottom(400, 1000, 400), false)
  eq('跟随：坏数字不把跟随关掉', nearBottom(Number.NaN, 1000, 400), true)

  eq('跟随：内容自己变高时保持原来的跟随', keepFollowing(false, false, true), true)
  eq('跟随：用户翻上去就停', keepFollowing(true, false, true), false)
  eq('跟随：用户回到底部再跟上', keepFollowing(true, true, false), true)
}
