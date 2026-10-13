import { awaitingTurnOutput, userSendFailed } from './catPending'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catPendingChecks(eq: Eq): void {
  const prior = [
    { kind: 'user', text: '上一句' },
    { kind: 'think', text: '想过了' },
    { kind: 'a', text: '答过了' },
  ]
  eq('思考占位：上一轮有内容时，新的一句发出去马上显示', awaitingTurnOutput([...prior, { kind: 'user', text: '这句' }]), true)
  eq('思考占位：系统行不挡住', awaitingTurnOutput([...prior, { kind: 'user', text: '这句' }, { kind: 'sys', text: '出错了' }]), true)
  eq('思考占位：这一轮已经有思考就换成思考行', awaitingTurnOutput([...prior, { kind: 'user', text: '这句' }, { kind: 'think', text: '在想' }]), false)
  eq('思考占位：这一轮已经有正文就收起', awaitingTurnOutput([...prior, { kind: 'user', text: '这句' }, { kind: 'a', text: '来了' }]), false)
  eq('思考占位：空的正文气泡还留着占位', awaitingTurnOutput([...prior, { kind: 'user', text: '这句' }, { kind: 'a', text: '' }]), true)
  eq('思考占位：正在重试时不再叠一句正在思考', awaitingTurnOutput([...prior, { kind: 'user', text: '这句' }, { kind: 'status', text: '正在重试（1/15）' }]), false)
  const failed = [
    { kind: 'user', text: '第一句' },
    { kind: 'a', text: '好' },
    { kind: 'user', text: '第二句' },
    { kind: 'sys', text: '回复没生成出来，请重试。', tone: 'err' },
    { kind: 'user', text: '第三句' },
  ]
  eq('失败记号：只标失败那一句', [userSendFailed(failed, 0), userSendFailed(failed, 2), userSendFailed(failed, 4)], [false, true, false])
}
