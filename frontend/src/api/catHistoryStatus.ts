/**
 * 契约 v0.45.2：cat.ConversationDetail.historyStatus（生成类型为 string；这里收窄成两种取值，其它 / 缺省 → undefined）。
 */
import { CAT_HISTORY_EMPTY, CAT_HISTORY_MISSING } from '@/views/cat/catCopy'
import type { cat } from '../../wailsjs/go/models'

export type CatHistoryStatus = 'missing' | 'read_failed'

export function readCatHistoryStatus(raw: Pick<cat.ConversationDetail, 'historyStatus'> | null | undefined): CatHistoryStatus | undefined {
  const v: unknown = raw?.historyStatus
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
