import { catEmptyConvView, catHistoryEmptyText, readCatHistoryStatus } from './catHistoryStatus'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catHistoryStatusChecks(eq: Eq): void {
  eq('historyStatus missing', readCatHistoryStatus({ historyStatus: 'missing' }), 'missing')
  eq('historyStatus read_failed', readCatHistoryStatus({ historyStatus: 'read_failed' }), 'read_failed')
  eq('historyStatus 缺省', readCatHistoryStatus({}), undefined)
  eq('historyStatus 未知值', readCatHistoryStatus({ historyStatus: 'gone' }), undefined)
  eq('historyStatus null', readCatHistoryStatus(null), undefined)
  eq('空状态 missing', catHistoryEmptyText('missing'), '这段对话的会话记录在本地找不到了。')
  eq('空状态 read_failed', catHistoryEmptyText('read_failed'), '这段对话的历史记录暂时无法显示。')
  eq('空状态 缺省无灰字', catHistoryEmptyText(undefined), '')
  eq('无字段 + 空 → 欢迎页', catEmptyConvView(readCatHistoryStatus({})), 'welcome')
  eq('missing → 灰字一行', catEmptyConvView('missing'), 'line')
  eq('read_failed → 灰字一行', catEmptyConvView('read_failed'), 'line')
}
