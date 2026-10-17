/** 群聊短复 vs 汇报卡判定（契约 v0.56 §6.21.19）。 */
import { isMemberShortReply, memberShortText } from './catGroupReply'

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
}
