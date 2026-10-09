// Cat 删除对话自检（api.check.ts 调用）：浏览器 mock 路径（node 下没有 window.go），会话列表就在 catState 内存里。
// 契约 v0.31.2：直接删、不存在的 id 也成功（可重复删）；删后迟到的 cat:turn / cat:message 忽略、不复活。
import { AppError } from '@/api/call'
import { deleteCatConversation } from '@/api/cat'
import { catState, deleteConversation, findConv, handleMessageEvent, handleTurnEvent, NEW_CONV } from './catState'

type Eq = (name: string, got: unknown, want: unknown) => void

export async function catDeleteChecks(eq: Eq): Promise<void> {
  let bad: unknown = null
  try {
    await deleteCatConversation('')
  } catch (e) {
    bad = e
  }
  eq('删对话：空 id → INVALID_ARGUMENT', (bad as AppError | null)?.code, 'INVALID_ARGUMENT')
  eq('删对话：mock 下不存在的 id 也成功', await deleteCatConversation('nope').then(() => 'ok'), 'ok')

  // 非当前的普通对话：只删那一行，当前选中不变
  catState.sel = 'c21'
  eq('删对话：普通对话（非当前）', [await deleteConversation('d2'), catState.plain.map((c) => c.id), catState.sel], ['other', ['d1', 'd3'], 'c21'])

  // 回复进行中的当前对话（项目下唯一一条）：直接删 → 回欢迎态、项目里没有对话、进行中状态清掉
  catState.sel = 'c31'
  catState.messages.c31 = [{ kind: 'user', text: 'x' }]
  catState.turns.c31 = { token: 999, turnId: 't1', status: 'running', assistantId: '' }
  const r = await deleteConversation('c31')
  const p3 = catState.projects.find((p) => p.id === 'p3')
  eq('删对话：进行中的当前对话 → 欢迎态', [r, catState.sel, catState.newProjectId, p3?.convs.length, 'c31' in catState.turns, 'c31' in catState.messages],
    ['current', NEW_CONV, '', 0, false, false])

  // 删后迟到的事件：忽略，不报错、不重建状态
  handleTurnEvent({ convId: 'c31', turnId: 't1', status: 'cancelled' })
  handleMessageEvent({ convId: 'c31', turnId: 't1', messageId: 'm1', seq: 1, op: 'append', role: 'assistant', text: '迟到' })
  eq('删对话：迟到的 cat:turn / cat:message 忽略', ['c31' in catState.turns, 'c31' in catState.messages, findConv('c31')], [false, false, null])

  // 重复删：不报错、什么都不变
  eq('删对话：重复删', [await deleteConversation('c31'), await deleteConversation('d2'), catState.plain.length], ['other', 'other', 2])
}
