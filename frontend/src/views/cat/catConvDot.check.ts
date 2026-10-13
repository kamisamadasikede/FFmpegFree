// 侧栏标题左边的点：回复进行中转黄圈，成功绿点，失败橙色感叹号。点停止回到灰点。
import { CAT_AGENT_BUILD } from '@/api/cat'
import { convDot, convDotLabel } from './catConvDot'
import { catState, handleTurnEvent } from './catState'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catConvDotChecks(eq: Eq): void {
  eq('状态点：进行中盖过上次结果', convDot(true, 'ok', 'idle'), 'run')
  eq('状态点：成功是绿点', convDot(false, 'ok', 'idle'), 'ok')
  eq('状态点：失败是感叹号', convDot(false, 'bad', 'ok'), 'bad')
  eq('状态点：停下后没有结果就回到走查数据', convDot(false, '', 'idle'), 'idle')
  eq('状态点：没记过就是灰点', convDot(false, '', 'nope'), '')
  eq('状态点：读屏', [convDotLabel('run'), convDotLabel('ok'), convDotLabel('bad'), convDotLabel('')], ['正在回复', '已完成', '回复失败', ''])

  const id = 'dot-check'
  const prevPlain = catState.plain.slice()
  const prevSel = catState.sel
  const prevTurn = catState.turns[id]
  const prevMsg = catState.messages[id]
  const prevOut = catState.outcome[id]
  const prevUnseen = catState.unseen[id]
  try {
    catState.plain = [...prevPlain, { id, title: '状态', agentKind: CAT_AGENT_BUILD }]
    catState.sel = id
    catState.turns[id] = { token: 1, turnId: 'dot-1', status: 'running', assistantId: '' }
    eq('状态点：回合还在就是转圈', convDot(id in catState.turns, catState.outcome[id] ?? '', ''), 'run')
    handleTurnEvent({ convId: id, turnId: 'dot-1', status: 'completed' })
    eq('状态点：完成后转圈停、记成功', [id in catState.turns, catState.outcome[id] ?? ''], [false, 'ok'])

    catState.turns[id] = { token: 2, turnId: 'dot-2', status: 'running', assistantId: '' }
    handleTurnEvent({ convId: id, turnId: 'dot-2', status: 'failed', errorMessage: '出错了' })
    eq('状态点：失败记感叹号', catState.outcome[id] ?? '', 'bad')

    catState.outcome[id] = 'ok'
    catState.turns[id] = { token: 3, turnId: 'dot-3', status: 'stopping', assistantId: '' }
    handleTurnEvent({ convId: id, turnId: 'dot-3', status: 'completed' })
    eq('状态点：点停止不算成功也不算失败', [id in catState.turns, catState.outcome[id] ?? ''], [false, ''])

    catState.turns[id] = { token: 4, turnId: 'dot-4', status: 'running', assistantId: '' }
    handleTurnEvent({ convId: id, turnId: 'dot-4', status: 'failed', errorCode: 'CAT_NOT_READY' })
    eq('状态点：没准备好也算失败', catState.outcome[id] ?? '', 'bad')
  } finally {
    catState.plain = prevPlain
    catState.sel = prevSel
    if (prevTurn) catState.turns[id] = prevTurn
    else delete catState.turns[id]
    if (prevMsg) catState.messages[id] = prevMsg
    else delete catState.messages[id]
    if (prevOut) catState.outcome[id] = prevOut
    else delete catState.outcome[id]
    if (prevUnseen) catState.unseen[id] = prevUnseen
    else delete catState.unseen[id]
  }
}
