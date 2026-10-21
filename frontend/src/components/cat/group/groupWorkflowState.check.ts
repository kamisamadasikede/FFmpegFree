import type { CatGroupWorkflow as Workflow } from '@/api/catGroupChat'
import { partitionWorkflows, workflowActive } from './groupWorkflowState'

type Eq = (name: string, got: unknown, want: unknown) => void
export function groupWorkflowChecks(eq: Eq) {
  const w = (status: Workflow['status'], tasks: Workflow['tasks'][number]['status'][]): Workflow => ({
    chainId: status, groupId: 'g', originSeq: 1, revision: 1, updatedAt: 1, request: 'request', status,
    tasks: tasks.map((status, i) => ({ id: String(i), memberId: 'm', title: 'task', kind: 'develop', instruction: '', dependsOn: [], attempts: 1, status })),
  })
  const active = w('running', ['running', 'pending'])
  const retrying = w('running', ['queued', 'pending'])
  const failed = w('blocked', ['failed', 'pending'])
  const stopped = w('stopped', ['stopped', 'pending'])
  const completed = w('completed', ['done'])
  const partlyFailed = w('blocked', ['failed', 'running', 'pending'])
  const dismissed = { ...failed, dismissed: true }
  eq('executing request belongs in progress', workflowActive(active), true)
  eq('retry backoff stays in progress', workflowActive(retrying), true)
  eq('failure awaiting input goes to history', workflowActive(failed), false)
  eq('stopped request leaves progress', workflowActive(stopped), false)
  eq('completed request leaves progress', workflowActive(completed), false)
  eq('independent work remains visible despite sibling failure', workflowActive(partlyFailed), true)
  eq('dismissed cards remain hidden', workflowActive(dismissed), false)
  const grouped = partitionWorkflows([active, failed, retrying, stopped, completed, partlyFailed, dismissed])
  eq('active partition contains only processing work', grouped.active, [active, retrying, partlyFailed])
  eq('history contains all settled outcomes', grouped.history, [failed, stopped, completed])
  const resumed = { ...failed, status: 'running' as const, tasks: retrying.tasks }
  eq('continuing a record returns it to progress', partitionWorkflows([resumed]).history.length, 0)
  eq('stopped history never blocks a new request', partitionWorkflows([stopped, active]).active, [active])
}
