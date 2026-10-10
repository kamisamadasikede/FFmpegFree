// 每条对话的模型和思考强度：读坏记录、互不影响、欢迎页抄到新对话、列表里没有了就退回。
import {
  carryPref,
  dropPref,
  prefOf,
  putPref,
  readConvPrefs,
  resolveAccess,
  resolveConvPref,
  writeConvPrefs,
  type CatConvPrefMap,
} from './catConvPrefs'

type Eq = (name: string, got: unknown, want: unknown) => void

const models = [{ id: 'm-default' }, { id: 'm-2' }]
const thinks = [{ id: 'low' }, { id: 'high' }, { id: 'xhigh' }]

export function catConvPrefsChecks(eq: Eq): void {
  eq('对话偏好：空的和坏的都当成没记过', [readConvPrefs(null), readConvPrefs(''), readConvPrefs('nope'), readConvPrefs('[]'), readConvPrefs('null')], [{}, {}, {}, {}, {}])
  const raw = JSON.stringify({
    a: { model: 'm-2', think: 'xhigh' },
    b: { model: '', think: '' },
    c: { model: 1, think: 'high' },
    '': { model: 'm-2', think: 'low' },
    d: 'nope',
  })
  eq('对话偏好：只留下有内容的记录', readConvPrefs(raw), { a: { model: 'm-2', think: 'xhigh' }, c: { model: '', think: 'high' } })

  const base: CatConvPrefMap = { a: { model: 'm-2', think: 'xhigh' } }
  const next = putPref(base, 'b', { model: 'm-default', think: 'low' })
  eq('对话偏好：改一条不影响另一条', [base, next], [{ a: { model: 'm-2', think: 'xhigh' } }, { a: { model: 'm-2', think: 'xhigh' }, b: { model: 'm-default', think: 'low' } }])
  eq('对话偏好：能按对话读出来', [prefOf(next, 'a'), prefOf(next, 'gone'), prefOf(next, '')], [{ model: 'm-2', think: 'xhigh' }, null, null])

  const carried = carryPref(next, 'a', 'born')
  eq('对话偏好：新对话带走欢迎页的选择，欢迎页还在', [carried.a, carried.born, next.born ?? null], [{ model: 'm-2', think: 'xhigh' }, { model: 'm-2', think: 'xhigh' }, null])
  eq('对话偏好：没有来源就不抄', carryPref(next, 'missing', 'born'), next)
  eq('对话偏好：删掉一条只少这一条', dropPref(carried, 'born').born ?? null, null)
  eq('对话偏好：写回去还能读出来', readConvPrefs(writeConvPrefs(next)), next)

  eq('对话偏好：没记过用第一个模型，强度留空', resolveConvPref(null, models, thinks), { model: 'm-default', think: '' })
  eq('对话偏好：都还在列表里就原样用', resolveConvPref({ model: 'm-2', think: 'xhigh' }, models, thinks), { model: 'm-2', think: 'xhigh' })
  eq('对话偏好：模型不在了用第一个，强度还在就留着', resolveConvPref({ model: 'gone', think: 'high' }, models, thinks), { model: 'm-default', think: 'high' })
  eq('对话偏好：强度不在了就留空', resolveConvPref({ model: 'm-2', think: 'gone' }, models, thinks), { model: 'm-2', think: '' })
  eq('对话偏好：列表是空的就不沿用旧 id', resolveConvPref({ model: 'm-2', think: 'xhigh' }, [], []), { model: '', think: '' })

  eq('对话偏好：只记了批准方式也留下', readConvPrefs(JSON.stringify({ p: { access: 'plan' } })), { p: { model: '', think: '', access: 'plan' } })
  const withMode = putPref({ a: { model: 'm-2', think: 'xhigh' } }, 'a', { model: 'm-2', think: 'xhigh', access: 'plan' })
  eq('对话偏好：改批准方式不丢掉模型和强度', withMode.a, { model: 'm-2', think: 'xhigh', access: 'plan' })
  eq('对话偏好：全部批准不写进记录', putPref(withMode, 'a', { model: 'm-2', think: 'xhigh', access: 'approve' }).a, { model: 'm-2', think: 'xhigh' })
  eq('对话偏好：批准方式跟着新对话走', carryPref(withMode, 'a', 'born').born, { model: 'm-2', think: 'xhigh', access: 'plan' })
  eq('对话偏好：批准方式能认出来', [resolveAccess('plan'), resolveAccess('auto-review'), resolveAccess('ask'), resolveAccess(''), resolveAccess('nope')], ['plan', 'auto', 'normal', 'approve', 'approve'])
}
