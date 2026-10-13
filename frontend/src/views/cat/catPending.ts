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
    if (b.kind === 'think' || b.kind === 'run' || b.kind === 'status') return false
    if (b.kind === 'a' && b.text) return false
  }
  return true
}

/** 这句用户消息后面、下一句之前出现了失败行。 */
export function userSendFailed(blocks: readonly { kind: string; tone?: string }[], index: number): boolean {
  if (blocks[index]?.kind !== 'user') return false
  for (let i = index + 1; i < blocks.length; i++) {
    const n = blocks[i]
    if (n.kind === 'user') return false
    if (n.kind === 'sys' && n.tone === 'err') return true
  }
  return false
}
