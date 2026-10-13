/**
 * 助手回复的打字节奏。
 * 积压少时一次一个字；积压多了加快。单帧有上限，不会把剩下的文字一次画出来。
 */

/** 这一帧打几个字。behind 是还没画出来的长度（UTF-16）。 */
export function typewriterTake(behind: number): number {
  if (behind <= 0) return 0
  if (behind <= 16) return 1
  if (behind <= 64) return 2
  if (behind <= 180) return 3
  if (behind <= 600) return 6
  return 8
}

/** 从 index 再往前数 chars 个字（一个码点算一个字），返回新的 UTF-16 下标。 */
export function advanceText(text: string, index: number, chars: number): number {
  let i = index < 0 ? 0 : index
  if (i > text.length) i = text.length
  let n = 0
  while (i < text.length && n < chars) {
    const cp = text.codePointAt(i) ?? 0
    i += cp > 0xffff ? 2 : 1
    n++
  }
  return i
}

const painted = new Map<string, number>()

/** 这条回复已经画到的下标。换对话再回来时接着打，不从头重来，也不整段跳完。 */
export function paintedIndex(id: string): number | undefined {
  return painted.get(id)
}

export function rememberPainted(id: string, index: number) {
  if (!id) return
  if (painted.has(id)) painted.delete(id)
  painted.set(id, index)
  while (painted.size > 200) {
    const oldest = painted.keys().next().value
    if (!oldest) break
    painted.delete(oldest)
  }
}
