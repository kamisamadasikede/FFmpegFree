import { applyTeamEvent, readTeamDraft, writeTeamDraft, teamState } from './teamState'
import type { TeamSnapshot, TeamMessage, TeamInbox } from '@/api/team'
type Equal = (name: string, got: unknown, want: unknown) => void
export function teamStateChecks(eq: Equal) {
 const s: TeamSnapshot = { room: { id: 'room', name: '群', projectId: '', workspace: '', coordinatorId: '', paused: false, archived: false, rosterVersion: 1, updatedAt: 1 }, members: [], messages: [], tasks: [], inbox: [], artifacts: [], eventSeq: 10, hasEarlier: false }
 const m: TeamMessage = { id: 'm', roomId: 'room', roomSeq: 1, revision: 1, fromMemberId: 'a', kind: 'assistant', content: '你', mentions: [], status: 'streaming', createdAt: 1 }
 eq('team ordered message accepted', applyTeamEvent(s, { roomId: 'room', eventSeq: 11, kind: 'message', data: m }), true)
 eq('team duplicate event ignored', applyTeamEvent(s, { roomId: 'room', eventSeq: 11, kind: 'message', data: m }), true)
 eq('team no duplicate message', s.messages.length, 1)
 eq('team delta accepted', applyTeamEvent(s, { roomId: 'room', eventSeq: 12, kind: 'message_delta', data: { id: 'm', revision: 2, contentDelta: '好' } }), true)
 eq('team delta appended once', s.messages[0].content, '你好')
 eq('team gap needs snapshot', applyTeamEvent(s, { roomId: 'room', eventSeq: 14, kind: 'message', data: m }), false)
 eq('team gap preserves cursor', s.eventSeq, 12)
 eq('team other room ignored', applyTeamEvent(s, { roomId: 'other', eventSeq: 13, kind: 'message', data: m }), true)
 eq('team other room preserves cursor', s.eventSeq, 12)
 eq('team delta missing base recovers', applyTeamEvent(s, { roomId: 'room', eventSeq: 13, kind: 'message_delta', data: { id: 'missing', revision: 2, contentDelta: 'x' } }), false)
 const inItem: TeamInbox = { id: 'in', roomId: 'room', messageId: 'm', taskId: 'task', recipientId: 'human', needsReply: true, readAt: 0, answeredAt: 0, notificationStatus: 'pending', createdAt: 1 }
 const old = teamState.bootstrap
 teamState.bootstrap = { human: { id: 'h', name: '本人', role: '负责人', avatar: '', notifyMentions: true }, rooms: [], employees: [], inbox: [], modes: {} }
 applyTeamEvent(s, { roomId: 'room', eventSeq: 13, kind: 'inbox', data: inItem })
 eq('team inbox updates navigation unread', teamState.bootstrap.inbox.length, 1)
 eq('team reading does not answer', s.inbox[0].answeredAt, 0)
 teamState.bootstrap = old
 const storage = new Map<string, string>(), original = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')
 Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: { getItem: (k: string) => storage.get(k) ?? null, setItem: (k: string, v: string) => storage.set(k, v) } })
 try {
  writeTeamDraft('a', { content: '保留原文', mentions: ['member-id'], replyToId: 'question', steerTaskId: '', requestId: 'stable', requestKey: 'fingerprint' })
  writeTeamDraft('b', { content: '另一个群', mentions: [], replyToId: '', steerTaskId: '' })
  eq('team drafts isolated by room', readTeamDraft('a').content, '保留原文')
  eq('team draft retains structured mention', readTeamDraft('a').mentions, ['member-id'])
  eq('team draft retains retry identity', readTeamDraft('a').requestId, 'stable')
  eq('team another draft intact', readTeamDraft('b').content, '另一个群')
  storage.set('ffc.team.draft.v1.bad', '{invalid')
  eq('team malformed draft safe', readTeamDraft('bad').content, '')
  storage.set('ffc.team.draft.v1.types', JSON.stringify({ content: 23, mentions: [12, 'valid'], replyToId: 3, steerTaskId: false }))
  eq('team draft validates member ids', readTeamDraft('types').mentions, ['valid'])
  eq('team draft validates reply id', readTeamDraft('types').replyToId, '')
 } finally { if (original) Object.defineProperty(globalThis, 'localStorage', original); else Reflect.deleteProperty(globalThis, 'localStorage') }
}
