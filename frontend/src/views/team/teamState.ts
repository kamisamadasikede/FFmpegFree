import { reactive } from 'vue'
import { getTeamBootstrap, getTeamRoom, getTeamEvents, type TeamBootstrap, type TeamSnapshot, type TeamEvent, type TeamMessage, type TeamTask, type TeamInbox } from '@/api/team'

export const teamState = reactive({ bootstrap: null as TeamBootstrap | null, current: null as TeamSnapshot | null, selected: '', loading: false, error: '', highlight: '', unreadUpdates: 0, openArtifact: null as { roomId: string; path: string } | null })
export const teamError = (e: unknown) => typeof e === 'object' && e && 'message' in e ? String(e.message) : String(e)
let selection = 0
let refreshing: Promise<void> | null = null
export async function loadTeams() { teamState.bootstrap = await getTeamBootstrap() }
export async function selectTeam(id: string, messageId = '') {
  const ticket = ++selection
  teamState.selected = id; teamState.loading = true; teamState.error = ''; teamState.highlight = messageId
  try {
    const snapshot = await getTeamRoom(id)
    if (ticket !== selection) return
    teamState.current = snapshot
    // Notification targets may be older than the most recent page.
    while (messageId && !snapshot.messages.some(m => m.id === messageId) && snapshot.hasEarlier) {
      const earlier = await getTeamRoom(id, snapshot.messages[0]?.roomSeq ?? 0)
      if (ticket !== selection) return
      snapshot.messages.unshift(...earlier.messages); snapshot.hasEarlier = earlier.hasEarlier
      teamState.current = { ...snapshot }
    }
  } catch (e) { if (ticket === selection) { teamState.current = null; teamState.error = teamError(e) } }
  finally { if (ticket === selection) teamState.loading = false }
}
function upsert<T extends { id: string }>(items: T[], value: T) { const i = items.findIndex(x => x.id === value.id); if (i < 0) items.push(value); else items[i] = value }
export function applyTeamEvent(snapshot: TeamSnapshot, event: TeamEvent): boolean {
  if (event.roomId !== snapshot.room.id || event.eventSeq <= snapshot.eventSeq) return true
  if (event.eventSeq !== snapshot.eventSeq + 1) return false
  if (event.kind === 'message') {
    const next = event.data as TeamMessage, old = snapshot.messages.find(m => m.id === next.id)
    if (!old || next.revision > old.revision) upsert(snapshot.messages, next)
    snapshot.messages.sort((a, b) => a.roomSeq - b.roomSeq)
  } else if (event.kind === 'message_delta') {
    const delta = event.data as { id: string; revision: number; contentDelta: string }, old = snapshot.messages.find(m => m.id === delta.id)
	if (!old) return false
    if (old && delta.revision > old.revision) { old.content += delta.contentDelta; old.revision = delta.revision }
  } else if (event.kind === 'task') upsert(snapshot.tasks, event.data as TeamTask)
  else if (event.kind === 'artifact') upsert(snapshot.artifacts, event.data as TeamSnapshot['artifacts'][number])
  else if (event.kind === 'open_file') teamState.openArtifact = event.data as { roomId: string; path: string }
  else if (event.kind === 'inbox') { upsert(snapshot.inbox, event.data as TeamInbox); if (teamState.bootstrap) upsert(teamState.bootstrap.inbox, event.data as TeamInbox) }
  else if (event.kind === 'room') snapshot.room = event.data as TeamSnapshot['room']
  else if (event.kind === 'roster') return false
  snapshot.eventSeq = event.eventSeq
  return true
}
export function refreshTeamEvents(): Promise<void> {
  if (refreshing) return refreshing
  refreshing = (async () => {
    const snapshot = teamState.current, ticket = selection
    if (!snapshot) return
    for (let page = 0; page < 8; page++) {
      const events = await getTeamEvents(snapshot.room.id, snapshot.eventSeq)
      if (ticket !== selection || snapshot !== teamState.current) return
      for (const event of events) {
        if (!applyTeamEvent(snapshot, event)) { await selectTeam(snapshot.room.id); return }
      }
      if (events.length < 250) return
    }
  })().catch(e => { teamState.error = teamError(e) }).finally(() => { refreshing = null })
  return refreshing
}
export interface TeamDraft { content: string; mentions: string[]; replyToId: string; steerTaskId: string; requestId?: string; requestKey?: string }
const draftKey = (id: string) => `ffc.team.draft.v1.${id}`
export function readTeamDraft(id: string): TeamDraft {
  try { const raw = JSON.parse(localStorage.getItem(draftKey(id)) || '{}'); return { content: typeof raw.content === 'string' ? raw.content : '', mentions: Array.isArray(raw.mentions) ? raw.mentions.filter((x: unknown) => typeof x === 'string') : [], replyToId: typeof raw.replyToId === 'string' ? raw.replyToId : '', steerTaskId: typeof raw.steerTaskId === 'string' ? raw.steerTaskId : '', requestId: typeof raw.requestId === 'string' ? raw.requestId : '', requestKey: typeof raw.requestKey === 'string' ? raw.requestKey : '' } } catch { return { content: '', mentions: [], replyToId: '', steerTaskId: '' } }
}
export function writeTeamDraft(id: string, draft: TeamDraft) { if (!id) return; try { localStorage.setItem(draftKey(id), JSON.stringify(draft)) } catch { /* Keep the in-memory draft when local storage is unavailable. */ } }
export const taskStatusLabel = (status: string) => ({ queued: '排队中', starting: '等待执行资源', running: '工作中', stopping: '正在停止', waiting_user: '等你答复', waiting_agent: '等待同事', completed: '已完成', needs_review: '待检查', failed: '失败', cancelled: '已停止' }[status] || status)
