// Cat 模型和思考强度按对话记住（本机 localStorage）。
// 没选过的对话：模型用列表第一个，强度留空（不传 --reasoning-effort，胶囊显示「默认」）。
// 选过的对话刷新后还在；换对话只动当前这一条。
import { CAT_CONV_PREFS_KEY } from './catConvPrefs'
import { catState, capsuleLabel, chooseModel, chooseThink, NEW_CONV, refreshCapabilities, sendMessage, thinkName, THINK_DEFAULT_LABEL } from './catState'

type Eq = (name: string, got: unknown, want: unknown) => void
type Lvl = { id: string; displayName: string }

export async function catThinkChecks(eq: Eq): Promise<void> {
  const win = (globalThis as unknown as { window: Record<string, unknown> }).window
  const prevGo = win.go
  const prevRt = win.runtime
  const prevStorage = (globalThis as unknown as { localStorage?: Storage }).localStorage
  const store = new Map<string, string>()
  const saved = {
    model: catState.model,
    think: catState.think,
    access: catState.access,
    models: catState.models,
    thinks: catState.thinks,
    sel: catState.sel,
    status: catState.status,
    turns: catState.turns,
    plain: catState.plain.slice(),
    creating: catState.creating,
    newProjectId: catState.newProjectId,
  }
  const messageIds = new Set(Object.keys(catState.messages))

  let thinks: Lvl[] = [
    { id: 'low', displayName: '低' },
    { id: 'medium', displayName: '中' },
    { id: 'high', displayName: '高' },
    { id: 'xhigh', displayName: '超高' },
  ]
  const sent: Array<Record<string, unknown>> = []
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
        ListCatModels: async () => [{ id: 'm-default', displayName: 'Cat 助手 1.0' }, { id: 'm-2', displayName: 'Cat 助手 1.0 快速' }],
        ListCatThinkLevels: async () => thinks,
        CreateCatConversation: async () => ({ id: 'conv-born', title: '', agentKind: 'cat_build', createdAt: 1, updatedAt: 1 }),
        SendCatMessage: async (req: Record<string, unknown>) => {
          sent.push({ ...req })
          return { turnId: `t${sent.length}` }
        },
      },
    },
  }
  const prefs = () => JSON.parse(store.get(CAT_CONV_PREFS_KEY) || '{}') as Record<string, { model: string; think: string }>
  try {
    catState.sel = 'c21'
    catState.model = ''
    catState.think = ''
    await refreshCapabilities('cat_build')
    eq('强度：没记过的对话用第一个模型，强度不预选', [catState.model, catState.think], ['m-default', ''])
    eq('强度：没选时显示「默认」', [thinkName.value, capsuleLabel.value], [THINK_DEFAULT_LABEL, 'Cat 助手 1.0 · 默认'])

    catState.status = { state: 'ready', version: '', canDownload: false, error: null, agents: [] }
    catState.turns = {}
    await sendMessage('看一下')
    eq('强度：没选时发送不带强度', [sent[0]?.thinkLevelId, sent[0]?.modelId], ['', 'm-default'])

    chooseModel('m-2')
    chooseThink('xhigh')
    eq('强度：xhigh 按 v0.44 显示「很高」，值还是 xhigh', [capsuleLabel.value, catState.think], ['Cat 助手 1.0 快速 · 很高', 'xhigh'])
    catState.turns = {}
    await sendMessage('再看一下')
    eq('强度：选了就带上', [sent[1]?.thinkLevelId, sent[1]?.modelId], ['xhigh', 'm-2'])

    // 刷新：内存里的选择丢掉，只靠这条对话的记录
    catState.model = ''
    catState.think = ''
    await refreshCapabilities('cat_build')
    eq('强度：刷新后还是这条对话的模型和强度', [catState.model, catState.think], ['m-2', 'xhigh'])

    catState.sel = 'c22'
    eq('强度：换一条没记过的对话回到默认', [catState.model, catState.think], ['m-default', ''])
    chooseThink('low')
    eq('强度：改当前对话不改另一条的记录', [prefs().c21, prefs().c22], [{ model: 'm-2', think: 'xhigh' }, { model: 'm-default', think: 'low' }])
    catState.sel = 'c21'
    eq('强度：切回去还是原来的', [catState.model, catState.think], ['m-2', 'xhigh'])
    catState.turns = {}
    await sendMessage('还是这条')
    eq('强度：切回去发送仍带原来的', [sent[2]?.thinkLevelId, sent[2]?.modelId], ['xhigh', 'm-2'])

    thinks = [{ id: 'low', displayName: '低' }, { id: 'high', displayName: '高' }]
    await refreshCapabilities('cat_build')
    eq('强度：选过的档位不在新列表 → 界面清空，记录还在', [catState.think, thinkName.value, prefs().c21?.think], ['', THINK_DEFAULT_LABEL, 'xhigh'])
    catState.turns = {}
    await sendMessage('列表里没有')
    eq('强度：档位不在列表里就不带', sent[3]?.thinkLevelId, '')

    thinks = [
      { id: 'low', displayName: '低' },
      { id: 'medium', displayName: '中' },
      { id: 'high', displayName: '高' },
      { id: 'xhigh', displayName: '超高' },
    ]
    await refreshCapabilities('cat_build')
    eq('强度：档位又出现后仍是这条对话选过的', catState.think, 'xhigh')

    thinks = []
    await refreshCapabilities('cat_build')
    eq('强度：列表为空不显示强度', [catState.think, thinkName.value, capsuleLabel.value], ['', '', 'Cat 助手 1.0 快速'])

    thinks = [
      { id: 'low', displayName: '低' },
      { id: 'medium', displayName: '中' },
      { id: 'high', displayName: '高' },
      { id: 'xhigh', displayName: '超高' },
    ]
    await refreshCapabilities('cat_build')
    catState.newProjectId = ''
    catState.sel = NEW_CONV
    chooseModel('m-default')
    chooseThink('high')
    catState.turns = {}
    catState.creating = false
    await sendMessage('新的一条')
    eq('强度：新对话带走欢迎页的选择', [catState.sel, sent[4]?.modelId, sent[4]?.thinkLevelId], ['conv-born', 'm-default', 'high'])
    catState.sel = 'c21'
    eq('强度：新建对话不改已经有的对话', [catState.model, catState.think], ['m-2', 'xhigh'])
    catState.sel = 'conv-born'
    eq('强度：新建的对话自己留着欢迎页的选择', [catState.model, catState.think], ['m-default', 'high'])
    catState.sel = NEW_CONV
    eq('强度：欢迎页下次还是自己的选择', [catState.model, catState.think], ['m-default', 'high'])

    // Hold creation while the user navigates and changes another chat's choices.
    const backend = (win.go as { app: { CatService: {
      CreateCatConversation: () => Promise<unknown>
      SendCatMessage: (req: Record<string, unknown>) => Promise<unknown>
    } } }).app.CatService
    let finishCreate!: (value: unknown) => void
    backend.CreateCatConversation = () => new Promise((resolve) => { finishCreate = resolve })
    catState.turns = {}
    const creating = sendMessage('后台创建的一条')
    catState.sel = 'c21'
    chooseModel('m-2')
    chooseThink('low')
    finishCreate({ id: 'conv-background', title: '', agentKind: 'cat_build', createdAt: 1, updatedAt: 1 })
    await creating
    eq('发送：创建期间切换页面，完成不抢回当前会话', catState.sel, 'c21')
    eq('发送：请求仍使用点击发送时的模型和强度', [sent[5]?.conversationId, sent[5]?.modelId, sent[5]?.thinkLevelId], ['conv-background', 'm-default', 'high'])
    eq('发送：后台创建的会话保存提交时的选择', prefs()['conv-background'], { model: 'm-default', think: 'high' })

    let rejectSend!: (reason: unknown) => void
    backend.SendCatMessage = () => new Promise((_resolve, reject) => { rejectSend = reject })
    catState.sel = 'conv-background'
    catState.turns = {}
    const rejected = sendMessage('未被收下的草稿', [{ name: 'draft.ts', path: 'D:/draft.ts' }])
    catState.sel = 'c21'
    rejectSend(JSON.stringify({ code: 'CAT_PROJECT_MISSING', message: '项目文件夹不见了。' }))
    eq('发送：离开后收到拒绝，仍返回正文和附件供原会话恢复', await rejected, { text: '未被收下的草稿', files: [{ name: 'draft.ts', path: 'D:/draft.ts' }], conversationId: 'conv-background' })
    eq('发送：拒绝不修改当前选中的会话', catState.sel, 'c21')

  } finally {
    if (prevGo === undefined) delete win.go
    else win.go = prevGo
    if (prevRt === undefined) delete win.runtime
    else win.runtime = prevRt
    if (prevStorage === undefined) delete (globalThis as unknown as { localStorage?: Storage }).localStorage
    else (globalThis as unknown as { localStorage: Storage }).localStorage = prevStorage
    for (const id of Object.keys(catState.messages)) if (!messageIds.has(id)) delete catState.messages[id]
    catState.plain = saved.plain
    catState.creating = saved.creating
    catState.newProjectId = saved.newProjectId
    Object.assign(catState, {
      models: saved.models,
      thinks: saved.thinks,
      sel: saved.sel,
      status: saved.status,
      turns: saved.turns,
    })
    // 换对话会按记录重摆模型和强度，还原放在选中之后
    catState.model = saved.model
    catState.think = saved.think
    catState.access = saved.access
  }
}
