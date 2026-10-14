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

/**
 * v0.48.1：这一轮最后成功了（有最终回复）。回复过程中先冒出来的错误行（中转重连、标准错误告警）
 * 不算失败：从最后一句用户消息之后删掉，用户消息不会再标红。原文仍在日志里。
 */
export function dropTurnErrors<T extends { kind: string; tone?: string }>(blocks: T[]): void {
  let start = -1
  for (let i = blocks.length - 1; i >= 0; i--) {
    if (blocks[i].kind === 'user') {
      start = i
      break
    }
  }
  for (let i = blocks.length - 1; i > start; i--) {
    const b = blocks[i]
    if (b.kind === 'sys' && b.tone === 'err') blocks.splice(i, 1)
  }
}
