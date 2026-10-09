// Cat 流式拼装自检（api.check.ts 调用）：去重、乱序缓冲、缺口、replace、done、旧整段形状、cat:turn 归一。
import { applyStreamEvent, newStreamMsg, normalizeMessageEvent, normalizeTurnEvent, type CatStreamEvent } from './catStream'
import { CAT_COPY, mapCatStatus } from './cat'

type Eq = (name: string, got: unknown, want: unknown) => void

const ev = (seq: number | undefined, op: 'append' | 'replace' | 'done', text = ''): CatStreamEvent => ({
  convId: 'c1', turnId: 't1', messageId: 'm1', seq, op, role: 'assistant', text,
})

export function catStreamChecks(eq: Eq): void {
  // 定稿形状归一
  eq('append 归一', normalizeMessageEvent({ convId: 'c', turnId: 't', messageId: 'm', seq: 3, op: 'append', textDelta: '你好' }),
    { convId: 'c', turnId: 't', messageId: 'm', seq: 3, op: 'append', role: 'assistant', blockType: undefined, text: '你好' })
  eq('replace 取 block.text', normalizeMessageEvent({ convId: 'c', turnId: 't', messageId: 'm', seq: 4, op: 'replace', block: { type: 'text', text: '整段' } })?.text, '整段')
  eq('done 无文字', normalizeMessageEvent({ convId: 'c', turnId: 't', messageId: 'm', seq: 5, op: 'done' })?.op, 'done')
  eq('缺会话 / 消息 id 丢弃', normalizeMessageEvent({ messageId: 'm', op: 'append' }), null)
  // 骨架 #172 形状
  eq('旧形状整段 → replace', normalizeMessageEvent({ conversationId: 'c', messageId: 'm', role: 'assistant', content: '答', createdAt: 1 })?.op, 'replace')
  eq('旧形状 delta → append', normalizeMessageEvent({ conversationId: 'c', messageId: 'm', role: 'assistant', content: '答', delta: true })?.op, 'append')
  eq('旧形状 user 角色保留', normalizeMessageEvent({ conversationId: 'c', messageId: 'u', role: 'user', content: '问' })?.role, 'user')

  eq('turn running', normalizeTurnEvent({ convId: 'c', turnId: 't', status: 'running' })?.status, 'running')
  eq('turn succeeded → completed', normalizeTurnEvent({ conversationId: 'c', status: 'succeeded' })?.status, 'completed')
  eq('turn canceled → cancelled', normalizeTurnEvent({ conversationId: 'c', status: 'canceled' })?.status, 'cancelled')
  eq('turn failed 带错误码', normalizeTurnEvent({ conversationId: 'c', status: 'failed', error: { code: 'CAT_REPLY_FAILED' } })?.errorCode, 'CAT_REPLY_FAILED')
  eq('未知 turn 状态丢弃', normalizeTurnEvent({ convId: 'c', status: 'weird' }), null)
  eq('turn 带上下文占用', [normalizeTurnEvent({ convId: 'c', status: 'completed', contextUsed: 1200, contextWindow: 256000 })?.contextUsed, normalizeTurnEvent({ convId: 'c', status: 'completed', contextUsed: 1200, contextWindow: 256000 })?.contextWindow], [1200, 256000])
  eq('turn 占用为 0 不带', normalizeTurnEvent({ convId: 'c', status: 'completed', contextUsed: 0, contextWindow: 256000 })?.contextUsed, undefined)

  // 顺序追加
  let m = newStreamMsg('m1')
  applyStreamEvent(m, ev(0, 'append', 'A'))
  applyStreamEvent(m, ev(1, 'append', 'B'))
  eq('顺序追加', m.text, 'AB')
  eq('重复 seq 忽略', applyStreamEvent(m, ev(1, 'append', 'B')), false)
  eq('重复后文字不变', m.text, 'AB')
  eq('过期 seq 忽略', applyStreamEvent(m, ev(0, 'append', 'X')), false)

  // 乱序：先到 3、2，再到 1 → 缓冲后按序补上
  m = newStreamMsg('m1')
  applyStreamEvent(m, ev(1, 'append', 'a'))
  applyStreamEvent(m, ev(3, 'append', 'c'))
  eq('缺 2 时先缓冲 3', m.text, 'a')
  applyStreamEvent(m, ev(3, 'append', 'c'))
  eq('缓冲里的重复也忽略', m.buf.size, 1)
  applyStreamEvent(m, ev(2, 'append', 'b'))
  eq('缺口补上后按序应用', m.text, 'abc')
  eq('缓冲清空', m.buf.size, 0)

  // 起点不是 0/1：先缓冲，等前面的
  m = newStreamMsg('m1')
  applyStreamEvent(m, ev(2, 'append', 'y'))
  eq('首个是 2 先缓冲', m.text, '')
  applyStreamEvent(m, ev(1, 'append', 'x'))
  eq('1 到了连同 2 一起', m.text, 'xy')

  // replace 权威整段
  m = newStreamMsg('m1')
  applyStreamEvent(m, ev(0, 'append', '草稿'))
  applyStreamEvent(m, ev(1, 'replace', '定稿'))
  applyStreamEvent(m, ev(2, 'append', '！'))
  eq('replace 后继续追加', m.text, '定稿！')

  // done 先到但中间缺号：不再等，按序补上已缓冲的再结束
  m = newStreamMsg('m1')
  applyStreamEvent(m, ev(0, 'append', '1'))
  applyStreamEvent(m, ev(2, 'append', '3'))
  applyStreamEvent(m, ev(4, 'done'))
  eq('done 时补上缓冲', m.text, '13')
  eq('done 后标记完成', m.done, true)
  eq('done 后再来的忽略', applyStreamEvent(m, ev(1, 'append', '2')), false)

  // 无 seq（旧形状）按到达顺序
  m = newStreamMsg('m1')
  applyStreamEvent(m, ev(undefined, 'append', '甲'))
  applyStreamEvent(m, ev(undefined, 'append', '乙'))
  eq('无 seq 追加', m.text, '甲乙')
  applyStreamEvent(m, ev(undefined, 'replace', '丙'))
  eq('无 seq 整段替换', m.text, '丙')

  // 后端 #174：seq 从 1 起，若干 append 后 done，无 role（缺省 assistant）
  m = newStreamMsg('m9')
  const reply = '这是一段比较长的回复，用来模拟后端把文字按二十四个字切成几段追加，然后发结束。'
  const parts = reply.match(/.{1,24}/gu) ?? []
  const evs = parts.map((p, i) => normalizeMessageEvent({ convId: 'c', turnId: 't', messageId: 'm9', seq: i + 1, op: 'append', textDelta: p })!)
  evs.push(normalizeMessageEvent({ convId: 'c', turnId: 't', messageId: 'm9', seq: parts.length + 1, op: 'done' })!)
  eq('#174 事件无 role 时按助手', evs[0].role, 'assistant')
  // 打乱顺序并混入重复，结果应与顺序到达一致
  const shuffled = [evs[1], evs[0], evs[0], ...evs.slice(2, -1).reverse(), evs[evs.length - 1], evs[1]]
  for (const e of shuffled) applyStreamEvent(m, e)
  eq('#174 乱序 + 重复后拼出完整回复', m.text, reply)
  eq('#174 done 后完成', m.done, true)

  // 组件状态：一期永远没有下载入口
  eq('canDownload 恒 false', mapCatStatus({ state: 'missing', version: '', canDownload: true }).canDownload, false)
  eq('未知状态按 missing', mapCatStatus({ state: 'downloading' }).state, 'missing')
  eq('停止文案', CAT_COPY.stopped, '已停止生成。')
  eq('失败文案', CAT_COPY.replyFailed, '回复没生成出来，请重试。')
  eq('未就绪文案', CAT_COPY.notReady, '未检测到 Cat 助手，请先安装并确保可在终端直接运行。')
  eq('登录失效文案', CAT_COPY.authInvalid, '登录失效，请重新登录后再试。')
}
