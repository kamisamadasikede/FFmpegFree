/**
 * v0.46 §6.19.20：拉自定义服务商自己的模型列表（ListCatProviderModels）。
 *
 * 走生成的绑定 CatBinding.ListCatProviderModels（后端 #277，契约 6.19.20.2 / 6.19.20.7）。
 * 纯浏览器 / 走查：走模拟，开关见 catDevSim（pm_*）。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { call } from '@/api/call'
import { CAT_SETTINGS_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'
import { catSim } from '@/views/cat/catDevSim'
import type { CatModel } from '@/api/cat'
import { parseFailDetailFields } from '@/components/cat/catCliOutdated'
import type { CatProviderKind } from '@/api/catAgents'
import { t } from '@/i18n'

export type CatProviderModelState = 'ok' | 'not_configured' | 'unauthorized' | 'unreachable' | 'not_supported' | 'bad_response'
export const CAT_PROVIDER_MODEL_STATES: readonly CatProviderModelState[] = ['ok', 'not_configured', 'unauthorized', 'unreachable', 'not_supported', 'bad_response']

/** 前端视图：生成的 cat.CatProviderModelList 归一化后（state 收窄、models 去重） */
export interface CatProviderModelList {
  state: CatProviderModelState
  models: CatModel[]
  /** RFC3339；state=ok 时给 */
  fetchedAt: string
}

/** §6.19.20.5 文案（产品 10-14 定稿） */
export const CAT_PROVIDER_MODELS_COPY = {
  get loading() { return t('cat.providerModels.loading') },
  get refreshTip() { return t('cat.providerModels.refreshTip') },
  get empty() { return t('cat.providerModels.empty') },
  get unauthorized() { return t('cat.providerModels.unauthorized') },
  get unreachable() { return t('cat.providerModels.unreachable') },
  get not_supported() { return t('cat.providerModels.not_supported') },
  get bad_response() { return t('cat.providerModels.bad_response') },
  get not_configured() { return t('cat.providerModels.not_configured') },
  get manualEntry() { return t('cat.providerModels.manualEntry') },
  get manualPlaceholder() { return t('cat.providerModels.manualPlaceholder') },
  get notInList() { return t('cat.providerModels.notInList') },
  get followDefault() { return t('cat.providerModels.followDefault') },
  get followDefaultDesc() { return t('cat.providerModels.followDefaultDesc') },
  get cancel() { return t('cat.providerModels.cancel') },
  get codexNoResponses() { return t('cat.providerModels.codexNoResponses') },
  get manualInvalid() { return t('cat.providerModels.manualInvalid') },
}

/** 一行提示：ok 且非空 → ''（不显示） */
export function providerModelHint(r: Pick<CatProviderModelList, 'state' | 'models'>): string {
  if (r.state === 'ok') return r.models.length ? '' : CAT_PROVIDER_MODELS_COPY.empty
  return CAT_PROVIDER_MODELS_COPY[r.state] ?? CAT_PROVIDER_MODELS_COPY.bad_response
}

/** 提示颜色：只有 unauthorized / unreachable / bad_response 是错误（--ff-danger）；空列表、not_supported、not_configured 灰字（--cat-muted） */
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
  // 与后端 ValidCustomModelID 一致：按码点计 1–200、不以 - 开头、不含 unicode.IsSpace / IsControl（含 C1 控制符）
  const id = raw.trim()
  const n = Array.from(id).length
  if (n < 1 || n > 200 || id.startsWith('-')) return null
  // eslint-disable-next-line no-control-regex
  if (/[\s\u0000-\u001f\u007f-\u009f]/.test(id)) return null
  return id
}

const CLI_SHORT: Record<CatProviderKind, string> = { cat_build: 'Grok', claude_cli: 'Claude', codex_cli: 'Codex' }
/** 开关说明（产品定稿：带实际 CLI 名） */
export function customToggleHint(kind: CatProviderKind): string {
  return t('cat.misc.s_d396b4f6c9', { cli: CLI_SHORT[kind] })
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
    try {
      return normalizeProviderModelList(await call(CatBinding.ListCatProviderModels(cat.ListCatProviderModelsRequest.createFrom({ agentKind, refresh }))))
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
export const GROK_CUSTOM_CHAT_KNOWN_ISSUE = t('cat.misc.s_09bc7fe65e')
/** 开关下面的已知问题灰字：只有 Grok 且自定义开着时有 */
export function customKnownIssue(kind: CatProviderKind, enabledCustom: boolean): string {
  return kind === 'cat_build' && enabledCustom ? GROK_CUSTOM_CHAT_KNOWN_ISSUE : ''
}
