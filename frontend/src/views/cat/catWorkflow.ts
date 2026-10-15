import { reactive } from 'vue'
import { getQueue, getChanges, type MessageQueue, type TurnChanges } from '@/api/catWorkflow'
import { onTaskEvent } from '@/services/wails'
export const catWorkflow = reactive({ queues: {} as Record<string, MessageQueue>, changes: {} as Record<string, TurnChanges[]>, opens: {} as Record<string, { relPath: string; name: string; serial: number }> })
export function acceptQueue(q: MessageQueue) {
  if ((q.revision || 0) < (catWorkflow.queues[q.convId]?.revision || 0)) return
  catWorkflow.queues[q.convId] = q
}
let serial = 0
const revisions = new Map<string, number>()
export function requestWorkspaceFile(convId: string, relPath: string, name = relPath.split('/').pop() || relPath) {
  catWorkflow.opens[convId] = { relPath, name, serial: ++serial }
}
export async function loadWorkflow(convId: string) {
  if (!convId || convId === 'new') return
  const revision = revisions.get(convId) || 0
  const results = await Promise.allSettled([getQueue(convId), getChanges(convId)])
  if ((revisions.get(convId) || 0) !== revision) return
  if (results[0].status === 'fulfilled') acceptQueue(results[0].value)
  if (results[1].status === 'fulfilled') catWorkflow.changes[convId] = results[1].value
}
let workflowLeases = 0
let workflowOff: (() => void) | undefined
export function watchWorkflow(): () => void {
  workflowLeases++
  let active = true
  const release = () => { if (!active) return; active = false; workflowLeases--; if (!workflowLeases) { workflowOff?.(); workflowOff = undefined } }
  if (workflowOff) return release
  const bump = (id: string) => revisions.set(id, (revisions.get(id) || 0) + 1)
  const offs = [
    onTaskEvent<MessageQueue>('cat:queue', q => { bump(q.convId); acceptQueue(q) }),
    onTaskEvent<TurnChanges>('cat:turn-changes', change => {
      bump(change.convId)
      const list = catWorkflow.changes[change.convId] || []
      catWorkflow.changes[change.convId] = [...list.filter(v => v.turnId !== change.turnId).map(v => v.finished ? { ...v, files: [] } : v), change]
    }),
    onTaskEvent<{ convId: string; relPath: string; name: string }>('cat:open-file', e => requestWorkspaceFile(e.convId, e.relPath, e.name)),
  ]
  workflowOff = () => offs.forEach(off => off())
  return release
}

export const previewDrafts = new Map<string, { text: string; revision: string }>()
