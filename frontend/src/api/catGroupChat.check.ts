/** 群聊短复 vs 汇报卡判定（契约 v0.56 §6.21.19）。 */
import { isMemberShortReply, memberShortText } from './catGroupReply'
import { undecidedRouteReasonOf } from './catGroupChat'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catGroupChatChecks(eq: Eq): void {
  eq('replyKind=short', isMemberShortReply({ kind: 'member', replyKind: 'short', text: '在的' }), true)
  eq('replyKind=report keeps card', isMemberShortReply({
    kind: 'member', replyKind: 'report', text: 'x', report: { did: '完成了' },
  }), false)
  eq('no report + text → short', isMemberShortReply({ kind: 'member', text: '招呼一下' }), true)
  eq('report present → card (legacy)', isMemberShortReply({
    kind: 'member', text: '', report: { did: '整段原文' },
  }), false)
  eq('shortText preferred', memberShortText({
    kind: 'member', replyKind: 'short', shortText: '短文案', text: 'fallback',
  }), '短文案')
  eq('text fallback', memberShortText({ kind: 'member', replyKind: 'short', text: '  你好  ' }), '你好')
  eq('user never short', isMemberShortReply({ kind: 'user', text: 'hi' }), false)
  eq('empty member no report not short', isMemberShortReply({ kind: 'member', text: '' }), false)

  // v0.56.1 undecided 灰字 · 原因（route.reason）
  eq('undecided timeout code', undecidedRouteReasonOf({ mode: 'undecided', reason: 'timeout' }), '判断超时')
  eq('undecided empty code', undecidedRouteReasonOf({ mode: 'undecided', reason: 'empty' }), '输出为空')
  eq('undecided parse_error', undecidedRouteReasonOf({ mode: 'undecided', reason: 'parse_error' }), '解析失败')
  eq('undecided run_error', undecidedRouteReasonOf({ mode: 'undecided', reason: 'run_error' }), '判断执行失败')
  eq('undecided unknown_id', undecidedRouteReasonOf({ mode: 'undecided', reason: 'unknown_id' }), '判断结果对不上成员')
  eq('undecided BE Chinese as-is', undecidedRouteReasonOf({ mode: 'undecided', reason: '判断超时' }), '判断超时')
  eq('undecided strip leading dot', undecidedRouteReasonOf({ mode: 'undecided', reason: '·输出为空' }), '输出为空')
  eq('undecided failReason fallback', undecidedRouteReasonOf({ mode: 'undecided', failReason: 'timeout' }), '判断超时')
  eq('ai mode ignores reason', undecidedRouteReasonOf({ mode: 'ai', reason: 'timeout' }), '')
  eq('undecided skips full main copy', undecidedRouteReasonOf({
    mode: 'undecided', reason: '没判断出该谁接，请 @ 对应成员，或再说具体要做什么。',
  }), '')
  eq('undecided empty reason', undecidedRouteReasonOf({ mode: 'undecided' }), '')
}

