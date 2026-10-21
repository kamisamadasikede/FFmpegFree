import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { useCatGroupChat } from './catGroupChat'
import type { CatGroupWorkflow } from '@/api/catGroupChat'

type Eq = (name: string, got: unknown, want: unknown) => void

export async function catGroupStoreChecks(eq: Eq) {
  const host = window as unknown as { go?: unknown }
  const previousGo = host.go
  const pinia = createPinia()
  setActivePinia(pinia)
  const calls: string[] = []
  const bindings: Record<string, (...args: any[]) => Promise<unknown>> = {
    ListCatGroups: async () => [],
    ClearCatGroupWorkflowHistory: async (groupId: string) => { calls.push(groupId); return ['failed', 'stopped'] },
    DismissCatGroupWorkflow: async () => {},
  }
  host.go = { app: { CatService: bindings } }
  const makeWorkflow = (chainId: string): CatGroupWorkflow => ({
    chainId, groupId: 'g', request: chainId, originSeq: 1, updatedAt: 1, revision: 1,
    status: chainId === 'active' ? 'running' : 'stopped', tasks: [],
  })
  try {
    const store = useCatGroupChat(pinia)
    store.activeId = 'g'
    store.workflows = ['active', 'failed', 'stopped'].map(makeWorkflow)
    store.messages = [{ seq: 1, groupId: 'g', kind: 'user', text: 'keep chat', chainId: 'failed', hop: 0, createdAt: 1 }]
    eq('group store exports clear history action', typeof store.clearWorkflowHistory, 'function')
    eq('group store exports single-card removal action', typeof store.dismissWorkflow, 'function')
    await store.clearWorkflowHistory()
    eq('clear history calls the real bridge once for current group', calls, ['g'])
    eq('clear hides only backend-confirmed records', store.workflows.map(w => [w.chainId, !!w.dismissed]), [['active', false], ['failed', true], ['stopped', true]])
    eq('clear keeps conversation messages', store.messages.map(m => m.text), ['keep chat'])

    store.workflows = ['active', 'failed'].map(makeWorkflow)
    bindings.ClearCatGroupWorkflowHistory = async () => { throw new Error('bridge failed') }
    let error = ''
    try { await store.clearWorkflowHistory() } catch (e) { error = (e as Error).message }
    eq('clear surfaces bridge errors instead of pretending success', error.includes('bridge failed'), true)
    eq('failed bridge leaves cards unchanged', store.workflows.some(w => w.dismissed), false)

    let resolve!: (ids: string[]) => void
    bindings.ClearCatGroupWorkflowHistory = async () => new Promise<string[]>(done => { resolve = done })
    const pending = store.clearWorkflowHistory()
    store.activeId = 'another-group'
    store.workflows = [makeWorkflow('failed')]
    resolve(['failed'])
    await pending
    eq('late clear response cannot hide another group records', !!store.workflows[0].dismissed, false)

    store.activeId = 'g'
    delete bindings.ClearCatGroupWorkflowHistory
    error = ''
    try { await store.clearWorkflowHistory() } catch (e) { error = (e as Error).message }
    eq('old backend reports a missing binding rather than JS TypeError', error.includes('后端还没有提供 CatService.ClearCatGroupWorkflowHistory'), true)
    eq('missing backend never locally deletes cards', !!store.workflows[0].dismissed, false)
    await store.dismissWorkflow('failed')
    eq('single-card removal remains available', !!store.workflows[0].dismissed, true)

    const sendIds: string[] = []
    let loseReply = true
    bindings.SendCatGroupMessage = async (req: { groupId: string; text: string; clientMessageId: string }) => {
      sendIds.push(req.clientMessageId)
      if (loseReply) { loseReply = false; throw new Error('reply lost after acceptance') }
      return { chainId: 'request', userMessage: { seq: 2, groupId: req.groupId, kind: 'user', text: req.text, chainId: 'request', hop: 0, createdAt: 1 } }
    }
    try { await store.send('请分析', []) } catch { /* accepted but response uncertain */ }
    await store.send('请分析', [])
    eq('retry of uncertain send reuses its client id', sendIds[0] === sendIds[1] && !!sendIds[0], true)
    await store.send('请分析', [])
    eq('intentional repeat after confirmed send gets a new client id', sendIds[1] !== sendIds[2], true)

  } finally {
    host.go = previousGo
    disposePinia(pinia)
    setActivePinia(undefined)
  }
}
