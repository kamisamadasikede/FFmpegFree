/**
 * 群聊「查看过程」纯逻辑（无 Vue / Pinia）：走查假数据解析、convId 拆解。
 * 契约 6.21.12：订 cat:message / cat:turn，convId=group:<groupId>:<memberId>。
 */

export type ProcessBlock = {
  id: string
  kind: 'text' | 'tool' | 'think'
  text: string
  streaming?: boolean
}

/** 走查假数据（[tool] arg 行）→ 过程块 */
export function demoProcessBlocks(raw: string): ProcessBlock[] {
  const out: ProcessBlock[] = []
  let n = 0
  for (const ln of raw.split('\n')) {
    const hit = /^\[(\w+)\] (.*)$/.exec(ln)
    const last = out[out.length - 1]
    if (hit) {
      out.push({ id: `demo-${++n}`, kind: 'tool', text: `${hit[1]} ${hit[2]}` })
    } else if (!ln.trim()) {
      // 空行结束当前工具/正文段，下一段另起
      if (last) out.push({ id: `demo-${++n}`, kind: 'text', text: '' })
    } else if (last?.kind === 'tool') {
      last.text = last.text ? `${last.text}\n${ln}` : ln
    } else if (last?.kind === 'text') {
      last.text = last.text ? `${last.text}\n${ln}` : ln
    } else {
      out.push({ id: `demo-${++n}`, kind: 'text', text: ln })
    }
  }
  return out.filter((b) => b.kind === 'tool' || b.text.trim())
}

export function memberIdFromGroupConv(convId: string): string | null {
  if (!convId.startsWith('group:')) return null
  const parts = convId.split(':')
  return parts.length >= 3 ? parts[parts.length - 1]! : null
}

export function groupIdFromGroupConv(convId: string): string | null {
  if (!convId.startsWith('group:')) return null
  const parts = convId.split(':')
  return parts.length >= 3 ? parts[1]! : null
}
