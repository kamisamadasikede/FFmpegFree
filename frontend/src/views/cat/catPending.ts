/** 这一轮发出去之后，正文、思考、工具都还没出现。系统行不算已经开始回复。 */

export function awaitingTurnOutput(blocks: readonly { kind: string; text?: string }[]): boolean {
  let start = -1
  for (let i = blocks.length - 1; i >= 0; i--) {
    if (blocks[i].kind === 'user') {
      start = i
      break
    }
  }
  for (let i = start + 1; i < blocks.length; i++) {
    const b = blocks[i]
    if (b.kind === 'think' || b.kind === 'run') return false
    if (b.kind === 'a' && b.text) return false
  }
  return true
}
