// 点停止后：思考不再转，迟到的思考和工具不再写进对话；下一轮照常接收。
import { catState, handleMessageEvent, handleTurnEvent } from './catState'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catStopChecks(eq: Eq): void {
  const prevSel = catState.sel
  const prevMsgs = catState.messages.d1
  const prevTurn = catState.turns.d1
  const thought = (text: string, seq: number, turnId: string, messageId: string) =>
    handleMessageEvent({
      convId: 'd1', turnId, messageId, seq, op: 'replace', role: 'assistant', blockType: 'thought', text,
    })
  const thinks = () => (catState.messages.d1 ?? []).filter((b) => b.kind === 'think')
  try {
    catState.sel = 'd1'
    catState.messages.d1 = [{ kind: 'user', text: '写登录页' }]
    catState.turns.d1 = { token: 7, turnId: 'turn-7', status: 'running', assistantId: '' }
    thought('正在写入', 1, 'turn-7', 'think-1')
    eq('停止：思考行在转', thinks(), [{ kind: 'think', id: 'think-1', text: '正在写入', streaming: true }])

    catState.turns.d1.status = 'stopping'
    thought('还在读', 2, 'turn-7', 'think-1')
    eq('停止：点了停止后思考停转，文字不改', thinks(), [{ kind: 'think', id: 'think-1', text: '正在写入', streaming: false }])

    handleTurnEvent({ convId: 'd1', turnId: 'turn-7', status: 'cancelled' })
    const list = catState.messages.d1 ?? []
    eq('停止：回合结束', 'd1' in catState.turns, false)
    eq('停止：末行是已停止生成', list[list.length - 1], { kind: 'sys', text: '已停止生成。' })

    thought('还在读文件', 3, 'turn-7', 'think-1')
    handleMessageEvent({
      convId: 'd1', turnId: 'turn-7', messageId: 'tool-1', seq: 1, op: 'replace', role: 'assistant', blockType: 'tool', text: 'read_file',
    })
    eq('停止：迟到的思考不改字、不再转', thinks(), [{ kind: 'think', id: 'think-1', text: '正在写入', streaming: false }])
    eq('停止：迟到的工具行不加', list.some((b) => b.kind === 'run'), false)
    eq('停止：迟到事件不写到停止行后面', list[list.length - 1], { kind: 'sys', text: '已停止生成。' })

    catState.turns.d1 = { token: 8, turnId: 'turn-8', status: 'running', assistantId: '' }
    thought('新的一轮', 1, 'turn-8', 'think-2')
    handleMessageEvent({
      convId: 'd1', turnId: 'turn-7', messageId: 'tool-2', seq: 1, op: 'replace', role: 'assistant', blockType: 'tool', text: 'read_file',
    })
    eq('停止：新一轮的思考照常显示，旧一轮加不进来', thinks(), [
      { kind: 'think', id: 'think-1', text: '正在写入', streaming: false },
      { kind: 'think', id: 'think-2', text: '新的一轮', streaming: true },
    ])
    eq('停止：新一轮进行中，旧工具不加', list.some((b) => b.kind === 'run'), false)
    handleTurnEvent({ convId: 'd1', turnId: 'turn-8', status: 'cancelled' })
  } finally {
    catState.sel = prevSel
    if (prevMsgs) catState.messages.d1 = prevMsgs
    else delete catState.messages.d1
    if (prevTurn) catState.turns.d1 = prevTurn
    else delete catState.turns.d1
  }
}
