/**
 * 群聊短复 vs 汇报卡（契约 v0.56 §6.21.19）。
 * 纯函数，无 i18n / Wails 依赖，供 UI 与自检共用。
 */
export interface CatGroupReplyShape {
  kind: string
  text?: string
  replyKind?: string
  shortText?: string
  report?: { did?: string } | null
}

/**
 * 识别顺序：
 * 1. 显式 `replyKind=short|report`
 * 2. 无 report、有正文 → 短复（后端短复路径常省略 report）
 * 3. 其余带 report 的 member → 汇报卡（含旧版 parsed=false 未格式化兜底）
 */
export function isMemberShortReply(m: CatGroupReplyShape): boolean {
  if (m.kind !== 'member') return false
  const kind = (m.replyKind || '').toLowerCase()
  if (kind === 'short') return true
  if (kind === 'report') return false
  if (!m.report) return !!memberShortText(m)
  return false
}

/** 短复气泡正文：shortText → text →（短复时）report.did */
export function memberShortText(m: CatGroupReplyShape): string {
  const st = (m.shortText || '').trim()
  if (st) return st
  const tx = (m.text || '').trim()
  if (tx) return tx
  if ((m.replyKind || '').toLowerCase() === 'short' && m.report?.did) return String(m.report.did).trim()
  return ''
}
