// 重新进入 Cat：上次打开的对话写在 localStorage，列表回来后还在就留在那条并展开项目；删了才回新对话。
import { catState, NEW_CONV, reloadConversations, selectConv } from './catState'

type Eq = (name: string, got: unknown, want: unknown) => void

const KEY = 'ffmpegfree.cat.lastConv'

export async function catLastConvChecks(eq: Eq): Promise<void> {
  const win = (globalThis as unknown as { window: Record<string, unknown> }).window
  const prevGo = win.go
  const prevRt = win.runtime
  const store = new Map<string, string>()
  const prevStorage = (globalThis as unknown as { localStorage?: Storage }).localStorage
  const saved = {
    sel: catState.sel,
    model: catState.model,
    think: catState.think,
    projects: catState.projects.map((p) => ({ ...p, convs: p.convs.slice() })),
    plain: catState.plain.slice(),
    convListReady: catState.convListReady,
    newProjectId: catState.newProjectId,
  }
  const projects = [{ id: 'p-last', name: '演示', path: 'D:\\work\\demo', createdAt: 1, updatedAt: 1, missing: false }]
  const convs = [
    { id: 'conv-keep', title: '留下', agentKind: 'cat_build', projectId: 'p-last', createdAt: 1, updatedAt: 2 },
    { id: 'conv-plain', title: '普通', agentKind: 'cat_build', projectId: '', createdAt: 1, updatedAt: 1 },
  ]
  ;(globalThis as unknown as { localStorage: Storage }).localStorage = {
    getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
    setItem: (k: string, v: string) => { store.set(k, v) },
    removeItem: (k: string) => { store.delete(k) },
    clear: () => { store.clear() },
    key: (i: number) => [...store.keys()][i] ?? null,
    get length() { return store.size },
  } as Storage
  win.runtime = {}
  win.go = {
    app: {
      CatService: {
        ListCatProjects: async () => projects,
        ListCatConversations: async () => convs,
      },
    },
  }
  try {
    selectConv('conv-keep')
    eq('上次对话：点进一条就记下', store.get(KEY) ?? null, 'conv-keep')
    await reloadConversations()
    const proj = catState.projects.find((p) => p.id === 'p-last')
    eq('上次对话：列表回来后留在该条并展开项目', [catState.sel, proj?.open, proj?.convs.map((c) => c.id), catState.convListReady], ['conv-keep', true, ['conv-keep'], true])

    selectConv('conv-plain')
    await reloadConversations()
    eq('上次对话：普通对话同样留在该条', [catState.sel, store.get(KEY) ?? null], ['conv-plain', 'conv-plain'])

    selectConv(NEW_CONV)
    eq('上次对话：停在新对话就不记 id', store.get(KEY) ?? null, null)

    selectConv('gone')
    await reloadConversations()
    eq('上次对话：已经删掉则回到新对话并清掉记录', [catState.sel, store.get(KEY) ?? null], [NEW_CONV, null])
  } finally {
    if (prevGo === undefined) delete win.go
    else win.go = prevGo
    if (prevRt === undefined) delete win.runtime
    else win.runtime = prevRt
    if (prevStorage === undefined) delete (globalThis as unknown as { localStorage?: Storage }).localStorage
    else (globalThis as unknown as { localStorage: Storage }).localStorage = prevStorage
    catState.projects = saved.projects
    catState.plain = saved.plain
    catState.sel = saved.sel
    catState.model = saved.model
    catState.think = saved.think
    catState.convListReady = saved.convListReady
    catState.newProjectId = saved.newProjectId
  }
}
