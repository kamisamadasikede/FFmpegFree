/**
 * v0.46 §6.19.20：拉自定义服务商自己的模型列表（ListCatProviderModels）。
 *
 * 【临时】后端 PR 还没合，绑定未重新生成：这里先用和契约一致的本地类型，
 * 运行时从 window.go.app.CatService 上按名字取方法；没有该方法（旧后端）当 not_configured 处理。
 * 后端合入、重新生成绑定后改为 import CatBinding.ListCatProviderModels + cat.CatProviderModelList，删掉 TmpBinding。
 * 纯浏览器 / 走查：走模拟，开关见 catDevSim（pm_*）。
 */
import { call } from '@/api/call'
import { CAT_SETTINGS_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'
import { catSim } from '@/views/cat/catDevSim'
import type { CatModel } from '@/api/cat'
import { parseFailDetailFields } from '@/components/cat/catCliOutdated'
import type { CatProviderKind } from '@/api/catAgents'

export type CatProviderModelState = 'ok' | 'not_configured' | 'unauthorized' | 'unreachable' | 'not_supported' | 'bad_response'
export const CAT_PROVIDER_MODEL_STATES: readonly CatProviderModelState[] = ['ok', 'not_configured', 'unauthorized', 'unreachable', 'not_supported', 'bad_response']

/** 【临时类型】= 契约 ListCatProviderModelsRequest */
export interface ListCatProviderModelsRequest {
  agentKind: CatProviderKind
  refresh: boolean
}
/** 【临时类型】= 契约 CatProviderModelList */
export interface CatProviderModelList {
  state: CatProviderModelState
  models: CatModel[]
  /** RFC3339；state=ok 时给 */
  fetchedAt: string
}

/** §6.19.20.5 文案（产品 10-14 定稿） */
export const CAT_PROVIDER_MODELS_COPY = {
  loading: '正在获取模型列表…',
  refreshTip: '重新获取模型列表',
  empty: '这个服务商没有返回可用模型，请在下方手动填写模型 ID。',
  unauthorized: 'API Key 无效或没有权限，请检查后点刷新。',
  unreachable: '连不上这个服务商，请检查 API 地址和网络，或稍后再试。',
  not_supported: '这个服务商不提供模型列表，请在下方手动填写模型 ID。',
  bad_response: '服务商返回的模型列表无法识别，请在下方手动填写模型 ID。',
  not_configured: '填好 API 地址和 Key 后，就能获取模型列表。',
  manualEntry: '手动填写…',
  manualPlaceholder: '输入模型 ID',
  notInList: '（不在当前列表）',
  followDefault: '默认模型',
  followDefaultDesc: '由 CLI 和服务商决定',
  cancel: '取消',
  codexNoResponses: '这个服务商不支持 Codex 需要的 Responses 接口，请换一个服务商，或关闭自定义服务商。',
  manualInvalid: '模型 ID 不能含空格，不能以 - 开头，最长 200 个字符。',
} as const

/** 一行提示：ok 且非空 → ''（不显示） */
export function providerModelHint(r: Pick<CatProviderModelList, 'state' | 'models'>): string {
  if (r.state === 'ok') return r.models.length ? '' : CAT_PROVIDER_MODELS_COPY.empty
  return CAT_PROVIDER_MODELS_COPY[r.state] ?? CAT_PROVIDER_MODELS_COPY.bad_response
}

/** 提示颜色：只有 unauthorized / unreachable / bad_response 是错误（--ff-danger）；空列表、not_supported、not_configured 灰字（--ff-text-2） */
export function providerModelHintTone(r: Pick<CatProviderModelList, 'state' | 'models'>): '' | 'muted' | 'danger' {
  if (!providerModelHint(r)) return ''
  return r.state === 'unauthorized' || r.state === 'unreachable' || r.state === 'bad_response' ? 'danger' : 'muted'
}

/** 手动输入框只在用户触发的拉取（点刷新 / 打开开关 / 改地址或 Key）后抢焦点；进设置页的自动拉取不抢 */
export type ProviderModelFetchCause = 'enter' | 'refresh' | 'toggle' | 'edit'
export function shouldFocusManual(cause: ProviderModelFetchCause, r: Pick<CatProviderModelList, 'state' | 'models'>): boolean {
  return cause !== 'enter' && providerModelNeedsManual(r)
}

/** 只有「服务商给不出可用列表」才自动展开手动输入框：ok 但空 / not_supported / bad_response。
 *  unauthorized / unreachable / not_configured 不展开、不抢焦点：保留下拉，修好后点刷新恢复 */
export function providerModelNeedsManual(r: Pick<CatProviderModelList, 'state' | 'models'>): boolean {
  return (r.state === 'ok' && !r.models.length) || r.state === 'not_supported' || r.state === 'bad_response'
}

/** 契约 6.19.20.2 ⑧：去首尾空格后 1–200 字符、不含空白和控制字符、不以 - 开头。不合法返回 null */
export function normalizeManualModelId(raw: string): string | null {
  const id = raw.trim()
  if (!id || id.length > 200 || id.startsWith('-')) return null
  // eslint-disable-next-line no-control-regex
  if (/[\s\u0000-\u001f\u007f]/.test(id)) return null
  return id
}

const CLI_SHORT: Record<CatProviderKind, string> = { cat_build: 'Grok', claude_cli: 'Claude', codex_cli: 'Codex' }
/** 开关说明（产品定稿：带实际 CLI 名） */
export function customToggleHint(kind: CatProviderKind): string {
  return `打开后，${CLI_SHORT[kind]} 只用下面的地址和 Key，模型列表也从这里获取。`
}

export const PROVIDER_MODELS_TTL_MS = 10 * 60 * 1000
/** 输入区：距上次拉取超过 10 分钟（或从没拉过）→ 打开下拉时重拉 */
export function isModelListStale(fetchedAtMs: number, nowMs: number = Date.now()): boolean {
  return !fetchedAtMs || nowMs - fetchedAtMs > PROVIDER_MODELS_TTL_MS
}

function mapModels(raw: unknown): CatModel[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  const out: CatModel[] = []
  for (const m of raw) {
    if (!m || typeof m.id !== 'string' || !m.id || seen.has(m.id)) continue
    seen.add(m.id)
    out.push({ id: m.id, displayName: typeof m.displayName === 'string' && m.displayName ? m.displayName : m.id })
  }
  return out
}

export function normalizeProviderModelList(raw: unknown): CatProviderModelList {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const state = CAT_PROVIDER_MODEL_STATES.includes(r.state as CatProviderModelState) ? (r.state as CatProviderModelState) : 'bad_response'
  return {
    state,
    models: state === 'ok' ? mapModels(r.models) : [],
    fetchedAt: state === 'ok' && typeof r.fetchedAt === 'string' ? r.fetchedAt : '',
  }
}

type TmpBinding = (req: ListCatProviderModelsRequest) => Promise<unknown>
function tmpBinding(): TmpBinding | null {
  const w = globalThis as unknown as { window?: { go?: { app?: { CatService?: Record<string, unknown> } } } }
  const fn = w.window?.go?.app?.CatService?.ListCatProviderModels
  return typeof fn === 'function' ? (fn as TmpBinding) : null
}

const live = () => CAT_SETTINGS_BACKEND_READY && hasWailsBackend()
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

/** 走查模拟：按开关给状态 */
export function simProviderModelList(kind: CatProviderKind, flags: ReadonlySet<string>, now = new Date()): CatProviderModelList {
  const fail = (state: CatProviderModelState): CatProviderModelList => ({ state, models: [], fetchedAt: '' })
  if (flags.has('pm_notcfg')) return fail('not_configured')
  if (flags.has('pm_unauth')) return fail('unauthorized')
  if (flags.has('pm_unreach')) return fail('unreachable')
  if (flags.has('pm_unsupported')) return fail('not_supported')
  if (flags.has('pm_bad')) return fail('bad_response')
  const at = now.toISOString()
  if (flags.has('pm_empty')) return { state: 'ok', models: [], fetchedAt: at }
  const ids = kind === 'claude_cli' ? ['claude-sonnet-4-5', 'claude-opus-4-1', 'claude-haiku-4-5'] : kind === 'codex_cli' ? ['gpt-5.1', 'gpt-5.1-mini', 'o4-mini'] : ['grok-4', 'grok-4-fast', 'grok-code-fast-1']
  return { state: 'ok', models: ids.map((id) => ({ id, displayName: id })), fetchedAt: at }
}

/** 只有参数错误后端才返回 error；这里出错一律当 unreachable（不回传原文） */
export async function listCatProviderModels(agentKind: CatProviderKind, refresh: boolean): Promise<CatProviderModelList> {
  if (live()) {
    const fn = tmpBinding()
    if (!fn) return { state: 'not_configured', models: [], fetchedAt: '' }
    try {
      return normalizeProviderModelList(await call(fn({ agentKind, refresh })))
    } catch {
      return { state: 'unreachable', models: [], fetchedAt: '' }
    }
  }
  const flags = import.meta.env.DEV ? (catSim as ReadonlySet<string>) : new Set<string>()
  await wait(flags.has('pm_slow') ? 60_000 : 350)
  return simProviderModelList(agentKind, flags)
}

/** v0.46：Codex 走自定义服务商但对方没有 /responses（CAT_REPLY_FAILED，detail.reason，retryable=false；形状同 v0.43 cli_outdated） */
export const PROVIDER_RESPONSES_UNSUPPORTED_REASON = 'provider_responses_unsupported'
/** 是这个原因 → 返回失败行文案；否则 null（界面照旧） */
export function providerFailureText(detail: unknown): string | null {
  return (parseFailDetailFields(detail).reason || '').trim() === PROVIDER_RESPONSES_UNSUPPORTED_REASON ? CAT_PROVIDER_MODELS_COPY.codexNoResponses : null
}

/** 设置页下拉选项（任何状态都有「默认模型」和「手动填写…」，unauthorized / unreachable 时也不会卡住） */
export type ProviderModelOption = { value: string; label: string; disabled?: boolean }
export const PM_SEP = '__sep'
export const PM_MANUAL = '__manual'
export function providerModelOptions(r: Pick<CatProviderModelList, 'state' | 'models'> | null, current: string): ProviderModelOption[] {
  const models = r?.models ?? []
  const out: ProviderModelOption[] = [{ value: '', label: CAT_PROVIDER_MODELS_COPY.followDefault }]
  if (current && !models.some((m) => m.id === current)) out.push({ value: current, label: `${current} ${CAT_PROVIDER_MODELS_COPY.notInList}` })
  for (const m of models) out.push({ value: m.id, label: m.displayName })
  out.push({ value: PM_SEP, label: '──────────', disabled: true })
  out.push({ value: PM_MANUAL, label: CAT_PROVIDER_MODELS_COPY.manualEntry })
  return out
}

/** 已知问题（v0.46 第 5 条）：Grok 走自定义服务商能列模型，但发对话仍要登录 Grok。修好后把这个常量改成 '' 即可去掉提示（产品 10-14 定稿） */
export const GROK_CUSTOM_CHAT_KNOWN_ISSUE = 'Grok Build 暂时只从自定义服务商获取模型列表；发对话需要先登录 Grok，也不保证会走自定义服务商。'
/** 开关下面的已知问题灰字：只有 Grok 且自定义开着时有 */
export function customKnownIssue(kind: CatProviderKind, enabledCustom: boolean): string {
  return kind === 'cat_build' && enabledCustom ? GROK_CUSTOM_CHAT_KNOWN_ISSUE : ''
}
