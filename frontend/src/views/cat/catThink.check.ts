// Cat 思考强度不预选自检（api.check.ts 调用）：node 下临时挂一个假的 window.go.app.CatService，走真实接口层。
// 后端 #196：没选强度就不传 --reasoning-effort；前端不预选，胶囊显示「默认」。
import { catState, capsuleLabel, refreshCapabilities, sendMessage, thinkName, THINK_DEFAULT_LABEL } from './catState'

type Eq = (name: string, got: unknown, want: unknown) => void
type Lvl = { id: string; displayName: string }

export async function catThinkChecks(eq: Eq): Promise<void> {
  const win = (globalThis as unknown as { window: Record<string, unknown> }).window
  const prevGo = win.go
  const prevRt = win.runtime
  const saved = { model: catState.model, think: catState.think, models: catState.models, thinks: catState.thinks, sel: catState.sel, status: catState.status, turns: catState.turns }

  let thinks: Lvl[] = [
    { id: 'low', displayName: '低' },
    { id: 'medium', displayName: '中' },
    { id: 'high', displayName: '高' },
    { id: 'xhigh', displayName: '超高' },
  ]
  const sent: Array<Record<string, unknown>> = []
  win.runtime = {}
  win.go = {
    app: {
      CatService: {
        ListCatModels: async () => [{ id: 'm-default', displayName: 'Cat 助手 1.0' }, { id: 'm-2', displayName: 'Cat 助手 1.0 快速' }],
        ListCatThinkLevels: async () => thinks,
        SendCatMessage: async (req: Record<string, unknown>) => {
          sent.push({ ...req })
          return { turnId: `t${sent.length}` }
        },
      },
    },
  }
  try {
    catState.model = ''
    catState.think = ''
    await refreshCapabilities('cat_build')
    eq('强度：刷新后不预选，模型预选第一个', [catState.model, catState.think], ['m-default', ''])
    eq('强度：没选时显示「默认」', [thinkName.value, capsuleLabel.value], [THINK_DEFAULT_LABEL, 'Cat 助手 1.0 · 默认'])

    // 没选强度发送：thinkLevelId 为空串（后端不传 --reasoning-effort）
    catState.status = { state: 'ready', version: '', canDownload: false, error: null }
    catState.sel = 'c21'
    catState.turns = {}
    await sendMessage('看一下')
    eq('强度：没选时发送不带强度', [sent[0]?.thinkLevelId, sent[0]?.modelId], ['', 'm-default'])

    // 用户选一档（任意 id，按 displayName 显示）
    catState.think = 'xhigh'
    eq('强度：选了「超高」显示其名字', capsuleLabel.value, 'Cat 助手 1.0 · 超高')
    catState.turns = {}
    await sendMessage('再看一下')
    eq('强度：选了就带上', sent[1]?.thinkLevelId, 'xhigh')

    // 再刷新：选过的还在列表里 → 保留
    await refreshCapabilities('cat_build')
    eq('强度：刷新后保留仍在列表里的选择', catState.think, 'xhigh')

    // 列表里没有了 → 清空（不退回第一档）
    thinks = [{ id: 'low', displayName: '低' }, { id: 'high', displayName: '高' }]
    await refreshCapabilities('cat_build')
    eq('强度：选过的档位不在新列表 → 清空', [catState.think, thinkName.value], ['', THINK_DEFAULT_LABEL])

    // 强度列表为空：不显示强度半截（不显示「默认」）
    thinks = []
    await refreshCapabilities('cat_build')
    eq('强度：列表为空不显示强度', [catState.think, thinkName.value, capsuleLabel.value], ['', '', 'Cat 助手 1.0'])
  } finally {
    if (prevGo === undefined) delete win.go
    else win.go = prevGo
    if (prevRt === undefined) delete win.runtime
    else win.runtime = prevRt
    Object.assign(catState, saved)
  }
}
