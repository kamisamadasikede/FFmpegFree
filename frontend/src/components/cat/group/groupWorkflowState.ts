import type { CatGroupWorkflow } from '@/api/catGroupChat'

export function workflowActive(w: CatGroupWorkflow): boolean {
  if (w.dismissed || w.status === 'completed' || w.status === 'stopped') return false
  return w.status === 'running' || w.tasks.some((t) => t.status === 'running' || t.status === 'queued')
}

export function partitionWorkflows(workflows: CatGroupWorkflow[]) {
  const active: CatGroupWorkflow[] = [], history: CatGroupWorkflow[] = []
  for (const w of workflows) {
    if (!w.dismissed) (workflowActive(w) ? active : history).push(w)
  }
  return { active, history }
}
