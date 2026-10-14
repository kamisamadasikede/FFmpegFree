/**
 * 契约 v0.45.1：CatConversationDetail.historyStatus（可选，'missing' | 'read_failed'；有消息时不带）。
 * 生成的绑定暂无此字段：临时本地类型 + 读取器，重新生成绑定后仍从这里读。
 */
import { CAT_HISTORY_EMPTY, CAT_HISTORY_MISSING } from '@/views/cat/catCopy'

export type CatHistoryStatus = 'missing' | 'read_failed'

export function readCatHistoryStatus(raw: unknown): CatHistoryStatus | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const v = (raw as { historyStatus?: unknown }).historyStatus
  return v === 'missing' || v === 'read_failed' ? v : undefined
}

/** 空状态只出一行：missing → 本地找不到；read_failed → 暂时无法显示；缺省 → 空串（v0.45.2：显示欢迎页，不出灰字） */
export function catHistoryEmptyText(s: CatHistoryStatus | undefined): string {
  if (s === 'missing') return CAT_HISTORY_MISSING
  if (s === 'read_failed') return CAT_HISTORY_EMPTY
  return ''
}

/** v0.45.2：已读完、没有消息时显示什么 */
export function catEmptyConvView(s: CatHistoryStatus | undefined): 'welcome' | 'line' {
  return s ? 'line' : 'welcome'
}
