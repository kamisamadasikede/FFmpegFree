// 助手回复逐字出现：字少时一次一个，积压多了加快，但不会一帧把剩下的全画出来。
import { advanceText, typewriterTake } from './catTypewriter'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catTypewriterChecks(eq: Eq): void {
  eq('打字：没有积压就不打', typewriterTake(0), 0)
  eq('打字：十几个字以内一次一个', [typewriterTake(1), typewriterTake(16)], [1, 1])
  eq('打字：积压越多越快', [typewriterTake(64), typewriterTake(180), typewriterTake(600), typewriterTake(2000)], [2, 3, 6, 8])
  eq('打字：再长也不一次跳完', typewriterTake(5000) < 5000 && typewriterTake(5000) <= 8, true)

  const chars = '你好世界'
  const steps: number[] = []
  let i = 0
  while (i < chars.length) {
    const n = typewriterTake(chars.length - i)
    const next = advanceText(chars, i, n)
    steps.push([...chars.slice(i, next)].length)
    i = next
  }
  eq('打字：四个字一步一个', steps, [1, 1, 1, 1])

  eq('打字：表情算一个字', [advanceText('🙂', 0, 1), advanceText('你🙂好', 1, 1)], [2, 3])

  let at = 0
  const long = '字'.repeat(80)
  let frames = 0
  let biggest = 0
  while (at < long.length && frames < 1000) {
    const next = advanceText(long, at, typewriterTake(long.length - at))
    biggest = Math.max(biggest, next - at)
    at = next
    frames++
  }
  eq('打字：八十个字要分多帧，单帧不超过八个字', [at === long.length, frames > 1, biggest <= 8], [true, true, true])
}
