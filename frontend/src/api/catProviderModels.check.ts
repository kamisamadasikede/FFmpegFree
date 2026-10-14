// v0.46 §6.19.20：服务商模型列表的纯函数（提示文案 / 颜色 / 聚焦 / 手填校验 / 10 分钟过期 / 返回值归一化）
import {
  CAT_PROVIDER_MODELS_COPY as PM,
  customToggleHint,
  isModelListStale,
  normalizeManualModelId,
  normalizeProviderModelList,
  providerModelHint,
  providerModelHintTone,
  providerModelNeedsManual,
  shouldFocusManual,
  simProviderModelList,
  type CatProviderModelState,
} from './catProviderModels'

type Eq = (name: string, got: unknown, want: unknown) => void
const r = (state: CatProviderModelState, n = 0) => ({ state, models: Array.from({ length: n }, (_, i) => ({ id: `m${i}`, displayName: `m${i}` })) })

export function catProviderModelsChecks(eq: Eq): void {
  // 文案（产品 10-14 定稿）
  eq('pm 开关说明带 CLI 名 claude', customToggleHint('claude_cli'), '打开后，Claude 只用下面的地址和 Key，模型列表也从这里获取。')
  eq('pm 开关说明 codex', customToggleHint('codex_cli'), '打开后，Codex 只用下面的地址和 Key，模型列表也从这里获取。')
  eq('pm 开关说明 grok', customToggleHint('cat_build'), '打开后，Grok 只用下面的地址和 Key，模型列表也从这里获取。')
  eq('pm ok 非空无提示', providerModelHint(r('ok', 2)), '')
  eq('pm ok 空', providerModelHint(r('ok')), '这个服务商没有返回可用模型，请在下方手动填写模型 ID。')
  eq('pm unauthorized', providerModelHint(r('unauthorized')), 'API Key 无效或没有权限，请检查后点刷新。')
  eq('pm unreachable', providerModelHint(r('unreachable')), '连不上这个服务商，请检查 API 地址和网络，或稍后再试。')
  eq('pm not_supported', providerModelHint(r('not_supported')), '这个服务商不提供模型列表，请在下方手动填写模型 ID。')
  eq('pm bad_response', providerModelHint(r('bad_response')), '服务商返回的模型列表无法识别，请在下方手动填写模型 ID。')
  eq('pm not_configured', providerModelHint(r('not_configured')), '先填写 API 地址和 Key，再获取模型列表。')
  eq('pm 占位无示例', PM.manualPlaceholder, '输入模型 ID')
  eq('pm 首项', [PM.followDefault, PM.followDefaultDesc], ['默认模型', '由 CLI 和服务商决定'])
  eq('pm codex responses', PM.codexNoResponses, '这个服务商不支持 Codex 需要的 Responses 接口，请换一个服务商，或关闭自定义服务商。')
  // 颜色：只有三种是错误
  eq('pm tone 空列表灰', providerModelHintTone(r('ok')), 'muted')
  eq('pm tone not_supported 灰', providerModelHintTone(r('not_supported')), 'muted')
  eq('pm tone not_configured 灰', providerModelHintTone(r('not_configured')), 'muted')
  eq('pm tone unauthorized 红', providerModelHintTone(r('unauthorized')), 'danger')
  eq('pm tone unreachable 红', providerModelHintTone(r('unreachable')), 'danger')
  eq('pm tone bad_response 红', providerModelHintTone(r('bad_response')), 'danger')
  eq('pm tone ok 非空无', providerModelHintTone(r('ok', 1)), '')
  // 手动输入框：哪些状态展开
  eq('pm needsManual', (['ok', 'not_configured', 'unauthorized', 'unreachable', 'not_supported', 'bad_response'] as const).map((s) => providerModelNeedsManual(r(s))), [true, false, true, true, true, true])
  eq('pm ok 非空不展开', providerModelNeedsManual(r('ok', 3)), false)
  // 聚焦：进设置页的自动拉取不抢焦点；用户触发的才抢
  eq('pm 进入不聚焦', shouldFocusManual('enter', r('unreachable')), false)
  eq('pm 刷新聚焦', shouldFocusManual('refresh', r('unreachable')), true)
  eq('pm 开关聚焦', shouldFocusManual('toggle', r('ok')), true)
  eq('pm 改地址聚焦', shouldFocusManual('edit', r('not_supported')), true)
  eq('pm 有列表不聚焦', shouldFocusManual('refresh', r('ok', 2)), false)
  eq('pm not_configured 不聚焦', shouldFocusManual('toggle', r('not_configured')), false)
  // 手填校验（6.19.20.2 ⑧）
  eq('pm 手填去空格', normalizeManualModelId('  gpt-5.1 '), 'gpt-5.1')
  eq('pm 手填空', normalizeManualModelId('   '), null)
  eq('pm 手填含空格', normalizeManualModelId('a b'), null)
  eq('pm 手填横杠开头', normalizeManualModelId('--model'), null)
  eq('pm 手填控制字符', normalizeManualModelId('a\u0007b'), null)
  eq('pm 手填 200', normalizeManualModelId('x'.repeat(200)), 'x'.repeat(200))
  eq('pm 手填 201', normalizeManualModelId('x'.repeat(201)), null)
  eq('pm 手填斜杠冒号可', normalizeManualModelId('openrouter/anthropic:claude'), 'openrouter/anthropic:claude')
  // 10 分钟过期
  eq('pm 从没拉过', isModelListStale(0, 1000), true)
  eq('pm 9 分钟不过期', isModelListStale(1, 1 + 9 * 60_000), false)
  eq('pm 10 分钟整不过期', isModelListStale(1, 1 + 10 * 60_000), false)
  eq('pm 10 分 1 秒过期', isModelListStale(1, 1 + 10 * 60_000 + 1000), true)
  // 归一化
  const n = normalizeProviderModelList({ state: 'ok', fetchedAt: '2026-10-14T04:00:00Z', models: [{ id: 'a', displayName: '' }, { id: 'a' }, { id: 'b', displayName: 'B' }, { id: '' }, null] })
  eq('pm 归一化去重 + displayName 回退 id', n.models, [{ id: 'a', displayName: 'a' }, { id: 'b', displayName: 'B' }])
  eq('pm 归一化 fetchedAt', n.fetchedAt, '2026-10-14T04:00:00Z')
  eq('pm 失败状态清空列表', normalizeProviderModelList({ state: 'unauthorized', models: [{ id: 'x' }], fetchedAt: 'x' }), { state: 'unauthorized', models: [], fetchedAt: '' })
  eq('pm 未知状态 → bad_response', normalizeProviderModelList({ state: 'weird' }).state, 'bad_response')
  eq('pm 非对象', normalizeProviderModelList(null).state, 'bad_response')
  // 走查模拟开关
  eq('pm sim 默认 ok', simProviderModelList('codex_cli', new Set()).models.length > 0, true)
  eq('pm sim unauth', simProviderModelList('codex_cli', new Set(['pm_unauth'])).state, 'unauthorized')
  const e = simProviderModelList('codex_cli', new Set(['pm_empty']))
  eq('pm sim empty', [e.state, e.models], ['ok', []])
}
