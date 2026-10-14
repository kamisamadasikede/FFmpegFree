/**
 * Cat 设置接口层（契约 v0.35.1–v0.39 §6.19.12；v0.36 defaultsByAgent；v0.38 providersByAgent；v0.39 技能/MCP 可管理）。
 * CAT_SETTINGS_BACKEND_READY=true 且在 Wails 里：调生成的 CatService 绑定（含 v0.39 技能管理）。
 * 否则走内存模拟；走查开关见 views/cat/catDevSim.ts（savefail / inspect* / clearfail / keyfail / skillsmissing / setenablefail）。
 *
 * Save 走生成的 cat.Settings（v0.36 defaultsByAgent + v0.38 providersByAgent；Key 走 Save/ClearCatProviderKey）。
 * v0.39：InspectCatConfig + SetCatInspectItemEnabled / RevealCatSkillsDir / EnsureCreateCatSkillsDir。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { AppError, call } from '@/api/call'
import { readModeByAgent, type CatModeByAgent } from '@/api/catV0445'
import { CAT_AGENT_BUILD, CAT_PROVIDER_ORDER, isProviderKind, type CatProviderKind } from '@/api/catAgents'
import { CAT_SETTINGS_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'
import { catSim } from '@/views/cat/catDevSim'
import { t } from '@/i18n'

export { CAT_SETTINGS_BACKEND_READY }

export interface CatAgentDefaults {
  model: string
  effort: string
}

export type CatProviderProtocol = 'openai' | 'anthropic'

/** 一家自定义服务商视图（契约 CatProviderSettings；永不含明文 Key） */
export interface CatProviderSettings {
  enabledCustom: boolean
  protocol: CatProviderProtocol | ''
  apiUrl: string
  apiKeySet: boolean
}

export interface CatProviderCapability {
  agentKind: CatProviderKind
  defaultProtocol: CatProviderProtocol
  allowProtocolPick: boolean
  allowedProtocols: CatProviderProtocol[]
}

export interface CatSettings {
  /** 欢迎页预选；默认 cat_build */
  defaultAgentKind: CatProviderKind
  defaultsByAgent: Record<CatProviderKind, CatAgentDefaults>
  /** v0.38：每家自定义服务商；缺 key 视为全关 */
  providersByAgent: Record<CatProviderKind, CatProviderSettings>
  /** v0.44.4 §6.19.18.9：每家新对话默认模式（该家 modes[].id；"" = 该家默认） */
  defaultModeByAgent: CatModeByAgent
  customInstruction: string
  notifyOnFinish: boolean
}

export function emptyDefaults(): CatAgentDefaults {
  return { model: '', effort: '' }
}

export function emptyDefaultsByAgent(): Record<CatProviderKind, CatAgentDefaults> {
  return { cat_build: emptyDefaults(), claude_cli: emptyDefaults(), codex_cli: emptyDefaults() }
}

export function emptyProvider(): CatProviderSettings {
  return { enabledCustom: false, protocol: '', apiUrl: '', apiKeySet: false }
}

export function emptyProvidersByAgent(): Record<CatProviderKind, CatProviderSettings> {
  return { cat_build: emptyProvider(), claude_cli: emptyProvider(), codex_cli: emptyProvider() }
}

/** 产品默认协议能力（与后端 defaultProviderCapabilities 对齐；allowProtocolPick 仍 GUESS=false） */
export const DEFAULT_PROVIDER_CAPABILITIES: readonly CatProviderCapability[] = [
  { agentKind: 'cat_build', defaultProtocol: 'openai', allowProtocolPick: false, allowedProtocols: ['openai'] },
  { agentKind: 'claude_cli', defaultProtocol: 'anthropic', allowProtocolPick: false, allowedProtocols: ['anthropic'] },
  { agentKind: 'codex_cli', defaultProtocol: 'openai', allowProtocolPick: false, allowedProtocols: ['openai'] },
]

export function protocolLabel(p: CatProviderProtocol | ''): string {
  return p === 'anthropic' ? 'Anthropic' : 'OpenAI 兼容'
}

export function protocolUrlPlaceholder(p: CatProviderProtocol | ''): string {
  return p === 'anthropic' ? 'https://api.example.com' : 'https://api.example.com/v1'
}

export interface CatInspectItem {
  name: string
  description?: string
  source?: string
  enabled: boolean
}

export type CatInspectCategory = 'skill' | 'mcp' | 'plugin' | 'hook'

export interface CatInspectResult {
  skills: CatInspectItem[]
  plugins: CatInspectItem[]
  hooks: CatInspectItem[]
  mcpServers: CatInspectItem[]
  agents: CatInspectItem[]
  version?: string
  /** 空 = 成功；not_ready | timeout | parse | unsupported | no_project */
  reason?: '' | 'not_ready' | 'timeout' | 'parse' | 'unsupported' | 'no_project' | string
  /** v0.39：绝对路径；无项目时省略 */
  skillsRoot?: string
  mcpConfigPath?: string
  skillsRootExists?: boolean
}

export interface CatSkillsDirResult {
  skillsRoot: string
  mcpConfigPath: string
  skillsRootExists: boolean
  opened: boolean
}

export const CAT_INSTRUCTION_MAX = 4000
export const CAT_INSPECT_TIMEOUT_MS = 15000

/** 文案：设计《Cat-设置与定时任务-设计说明-v0.3》§3 + 契约 v0.35.1 §6.19.12.7 / 6.19.13.10-5 */
export const CAT_SETTINGS_COPY = {
  get title() { return t('cat.settings.title') },
  get subtitle() { return t('cat.settings.subtitle') },
  get tabs() {
    return {
      get general() { return t('cat.settings.tabs.general') },
      get instruction() { return t('cat.settings.tabs.instruction') },
      get ext() { return t('cat.settings.tabs.ext') },
      get about() { return t('cat.settings.tabs.about') },
    }
  },
  get newConvCard() { return t('cat.settings.newConvCard') },
  get newConvHint() { return t('cat.settings.newConvHint') },
  get installed() { return t('cat.settings.installed') },
  get notDetected() { return t('cat.settings.notDetected') },
  get model() { return t('cat.settings.model') },
  get modelDesc() { return t('cat.settings.modelDesc') },
  get effort() { return t('cat.settings.effort') },
  get effortDesc() { return t('cat.settings.effortDesc') },
  get approval() { return t('cat.settings.approval') },
  get approvalDesc() { return t('cat.settings.approvalDesc') },
  get defaultOpt() { return t('cat.settings.defaultOpt') },
  get place() { return t('cat.settings.place') },
  get placeDesc() { return t('cat.settings.placeDesc') },
  get placeLast() { return t('cat.settings.placeLast') },
  get placeNone() { return t('cat.settings.placeNone') },
  get looksCard() { return t('cat.settings.looksCard') },
  get theme() { return t('cat.settings.theme') },
  get themeDesc() { return t('cat.settings.themeDesc') },
  get themeLight() { return t('cat.settings.themeLight') },
  get themeDark() { return t('cat.settings.themeDark') },
  get typewriter() { return t('cat.settings.typewriter') },
  get typewriterDesc() { return t('cat.settings.typewriterDesc') },
  get convCard() { return t('cat.settings.convCard') },
  get notify() { return t('cat.settings.notify') },
  get notifyDesc() { return t('cat.settings.notifyDesc') },
  get history() { return t('cat.settings.history') },
  get later() { return t('cat.settings.later') },
  get clear() { return t('cat.settings.clear') },
  get clearDesc() { return t('cat.settings.clearDesc') },
  get clearBtn() { return t('cat.settings.clearBtn') },
  get clearTitle() { return t('cat.settings.clearTitle') },
  get clearBody() { return t('cat.settings.clearBody') },
  get clearOk() { return t('cat.settings.clearOk') },
  get clearFailed() { return t('cat.settings.clearFailed') },
  get cleared() { return t('cat.settings.cleared') },
  get cancel() { return t('cat.settings.cancel') },
  get keysCard() { return t('cat.settings.keysCard') },
  get keys() { return [
    [t('cat.settings.keyNewConv'), 'N'],
    [t('cat.settings.keySearchConv'), 'K'],
    [t('cat.settings.keyToggleSide'), 'B'],
    [t('cat.settings.keyTogglePanel'), 'J'],
  ] as [string, string][] },
  get stopKey() { return t('cat.settings.stopKey') },
  get saved() { return t('cat.settings.saved') },
  get saveFailed() { return t('cat.settings.saveFailed') },
  get instrHint() { return t('cat.settings.instrHint') },
  get instrPlaceholder() { return t('cat.settings.instrPlaceholder') },
  get instrOver() { return t('cat.settings.instrOver') },
  get instrSave() { return t('cat.settings.instrSave') },
  get instrRevert() { return t('cat.settings.instrRevert') },
  get extTipBefore() { return t('cat.settings.extTipBefore') },
  get extTipProject() { return t('cat.settings.extTipProject') },
  get extTipMid() { return t('cat.settings.extTipMid') },
  get extTipPathFallback() { return t('cat.settings.extTipPathFallback') },
  get extTipAfter() { return t('cat.settings.extTipAfter') },
  get openDir() { return t('cat.settings.openDir') },
  get createAndOpen() { return t('cat.settings.createAndOpen') },
  get noSkillsDir() { return t('cat.settings.noSkillsDir') },
  get noSkillsDirBody() { return t('cat.settings.noSkillsDirBody') },
  get noProjectExt() { return t('cat.settings.noProjectExt') },
  get pickProject() { return t('cat.settings.pickProject') },
  get skills() { return t('cat.settings.skills') },
  get exts() { return t('cat.settings.exts') },
  get extsFull() { return t('cat.settings.extsFull') },
  get hooks() { return t('cat.settings.hooks') },
  get extEmpty() { return t('cat.settings.extEmpty') },
  get extFailed() { return t('cat.settings.extFailed') },
  get retry() { return t('cat.settings.retry') },
  get on() { return t('cat.settings.on') },
  get off() { return t('cat.settings.off') },
  get statusCard() { return t('cat.settings.statusCard') },
  get detect() { return t('cat.settings.detect') },
  get ready() { return t('cat.settings.ready') },
  get missing() { return t('cat.settings.missing') },
  get checking() { return t('cat.settings.checking') },
  get version() { return t('cat.settings.version') },
  get noneReady() { return t('cat.settings.noneReady') },
  get lastCheck() { return t('cat.settings.lastCheck') },
  get recheck() { return t('cat.settings.recheck') },
  get rechecking() { return t('cat.settings.rechecking') },
  get aboutCard() { return t('cat.settings.aboutCard') },
  get aboutLine() { return t('cat.settings.aboutLine') },
  get openData() { return t('cat.settings.openData') },
  get customFold() { return t('cat.settings.customFold') },
  get customFoldHintOff() { return t('cat.settings.customFoldHintOff') },
  get customFoldHintOn() { return t('cat.settings.customFoldHintOn') },
  get enableCustom() { return t('cat.settings.enableCustom') },
  get apiUrlRequired() { return t('cat.settings.apiUrlRequired') },
  get enableCustomDesc() { return t('cat.settings.enableCustomDesc') },
  get protocol() { return t('cat.settings.protocol') },
  get protocolLocked() { return t('cat.settings.protocolLocked') },
  get apiUrl() { return t('cat.settings.apiUrl') },
  get apiUrlDesc() { return t('cat.settings.apiUrlDesc') },
  get apiKey() { return t('cat.settings.apiKey') },
  get apiKeyDesc() { return t('cat.settings.apiKeyDesc') },
  get apiKeyPlaceholder() { return t('cat.settings.apiKeyPlaceholder') },
  get apiKeySaved() { return t('cat.settings.apiKeySaved') },
  get apiKeyClear() { return t('cat.settings.apiKeyClear') },
  get clearKeyTitle() { return t('cat.settings.clearKeyTitle') },
  get clearKeyBody() { return t('cat.settings.clearKeyBody') },
  get clearKeyOk() { return t('cat.settings.clearKeyOk') },
}

const live = () => CAT_SETTINGS_BACKEND_READY && hasWailsBackend()
const sim = (f: Parameters<typeof catSim.has>[0]) => import.meta.env.DEV && catSim.has(f)

const EFFORTS = ['low', 'medium', 'high', 'xhigh', '']

function oneDefaults(raw: unknown): CatAgentDefaults {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const model = typeof r.model === 'string' ? r.model : ''
  const effort = typeof r.effort === 'string' && EFFORTS.includes(r.effort) ? r.effort : ''
  return { model, effort }
}

const PROTOCOLS: CatProviderProtocol[] = ['openai', 'anthropic']

function oneProvider(raw: unknown): CatProviderSettings {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const protocolRaw = typeof r.protocol === 'string' ? r.protocol : ''
  const protocol = PROTOCOLS.includes(protocolRaw as CatProviderProtocol) ? (protocolRaw as CatProviderProtocol) : ''
  const apiUrl = typeof r.apiUrl === 'string' ? r.apiUrl : ''
  return {
    enabledCustom: r.enabledCustom === true,
    protocol,
    apiUrl,
    // 只信服务端/模拟的 apiKeySet；忽略请求伪造
    apiKeySet: r.apiKeySet === true,
  }
}

/**
 * 读路径：v0.36 defaultsByAgent + defaultAgentKind；v0.38 providersByAgent。
 * 旧顶层 defaultModel/defaultEffort 迁进 cat_build（该 key 已有非空值则不覆盖）。
 * 永不接受明文 Key。
 */
export function normalizeSettings(raw: unknown): CatSettings {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const s = (k: string) => (typeof r[k] === 'string' ? (r[k] as string) : '')
  const by = emptyDefaultsByAgent()
  const src = r.defaultsByAgent
  if (src && typeof src === 'object') {
    for (const kind of CAT_PROVIDER_ORDER) {
      const cell = (src as Record<string, unknown>)[kind]
      if (cell && typeof cell === 'object') by[kind] = oneDefaults(cell)
    }
  }
  const providers = emptyProvidersByAgent()
  const psrc = r.providersByAgent
  if (psrc && typeof psrc === 'object') {
    for (const kind of CAT_PROVIDER_ORDER) {
      const cell = (psrc as Record<string, unknown>)[kind]
      if (cell && typeof cell === 'object') providers[kind] = oneProvider(cell)
    }
  }
  const oldModel = s('defaultModel')
  const oldEffortRaw = s('defaultEffort')
  const oldEffort = EFFORTS.includes(oldEffortRaw) ? oldEffortRaw : ''
  if ((oldModel || oldEffort) && !by.cat_build.model && !by.cat_build.effort) {
    by.cat_build = { model: oldModel, effort: oldEffort }
  }
  const kindRaw = s('defaultAgentKind')
  return {
    defaultAgentKind: isProviderKind(kindRaw) ? kindRaw : CAT_AGENT_BUILD,
    defaultsByAgent: by,
    providersByAgent: providers,
    defaultModeByAgent: readModeByAgent(r),
    customInstruction: s('customInstruction'),
    // 契约：nil 视为 true
    notifyOnFinish: r.notifyOnFinish !== false,
  }
}

/** 只提交 v0.36+v0.38 形状（不含旧 defaultModel/defaultEffort / 明文 Key / 伪造 apiKeySet） */
export function settingsPayload(req: CatSettings): Record<string, unknown> {
  const defaultsByAgent: Record<string, CatAgentDefaults> = {}
  const providersByAgent: Record<string, Omit<CatProviderSettings, 'apiKeySet'> & { apiKeySet?: never }> = {}
  for (const kind of CAT_PROVIDER_ORDER) {
    const d = req.defaultsByAgent[kind] ?? emptyDefaults()
    defaultsByAgent[kind] = { model: d.model || '', effort: d.effort || '' }
    const p = req.providersByAgent[kind] ?? emptyProvider()
    const protocol = PROTOCOLS.includes(p.protocol as CatProviderProtocol) ? p.protocol : ''
    providersByAgent[kind] = {
      enabledCustom: !!p.enabledCustom,
      protocol,
      apiUrl: (p.apiUrl || '').slice(0, 2048),
      // 不提交 apiKeySet；服务端以密钥存储为准
    }
  }
  return {
    defaultAgentKind: isProviderKind(req.defaultAgentKind) ? req.defaultAgentKind : CAT_AGENT_BUILD,
    defaultsByAgent,
    providersByAgent,
    defaultModeByAgent: Object.fromEntries(CAT_PROVIDER_ORDER.map((k) => [k, (req.defaultModeByAgent?.[k] || '').trim()])),
    customInstruction: req.customInstruction,
    notifyOnFinish: req.notifyOnFinish,
  }
}

let mockSettings: CatSettings = normalizeSettings({})
/** 模拟密钥：只记是否已设，永不回显明文 */
const mockSecrets = new Set<CatProviderKind>()
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))
// 走查 ?cat_sim=pmcustom：三家自定义都开、地址和 Key 已填（v0.46 模型下拉）
if (sim('pmcustom')) {
  for (const kind of CAT_PROVIDER_ORDER) {
    mockSettings.providersByAgent[kind] = { enabledCustom: true, protocol: '', apiUrl: kind === 'claude_cli' ? 'https://gw.example.com' : 'https://gw.example.com/v1', apiKeySet: true }
    mockSecrets.add(kind)
  }
}

function withMockKeyFlags(s: CatSettings): CatSettings {
  const providersByAgent = emptyProvidersByAgent()
  for (const kind of CAT_PROVIDER_ORDER) {
    const p = s.providersByAgent[kind] ?? emptyProvider()
    providersByAgent[kind] = { ...p, apiKeySet: mockSecrets.has(kind) }
  }
  return { ...s, providersByAgent }
}

export async function getCatSettings(): Promise<CatSettings> {
  if (live()) return normalizeSettings(await call(CatBinding.GetCatSettings()))
  return withMockKeyFlags(normalizeSettings(mockSettings))
}

/** 保存失败显示文案（产品口径）：中文原样；非中文 →「保存失败：」+ 原文；空 →「保存失败，请重试。」 */
export function saveErrorMessage(msg: string): string {
  const m = (msg || '').trim()
  if (!m) return CAT_SETTINGS_COPY.saveFailed
  return /[\u4e00-\u9fff]/.test(m) ? m : t('cat.misc.s_78d235cc9c') + m
}

export async function saveCatSettings(req: CatSettings): Promise<CatSettings> {
  if (req.customInstruction.length > CAT_INSTRUCTION_MAX) throw new AppError('INVALID_ARGUMENT', CAT_SETTINGS_COPY.instrOver)
  const payload = settingsPayload(req)
  if (live()) {
    return normalizeSettings(await call(CatBinding.SaveCatSettings(cat.Settings.createFrom(payload))))
  }
  await wait(250)
  if (sim('savefail')) throw new AppError('IO_ERROR', 'sim')
  // Save 不因提交而清除已存 Key；apiKeySet 仍由 mockSecrets 决定
  const next = normalizeSettings(payload)
  // 同真机（#292）：Codex 由关到开自定义服务商时，后端清掉它旧的思考强度并在返回值里带回
  const ck: CatProviderKind = 'codex_cli'
  if (next.providersByAgent[ck]?.enabledCustom && !mockSettings.providersByAgent[ck]?.enabledCustom && next.defaultsByAgent[ck]) {
    next.defaultsByAgent = { ...next.defaultsByAgent, [ck]: { ...next.defaultsByAgent[ck], effort: '' } }
  }
  mockSettings = withMockKeyFlags(next)
  return withMockKeyFlags(mockSettings)
}

export function normalizeProvider(raw: unknown): CatProviderSettings {
  return oneProvider(raw)
}

export function normalizeCapabilities(raw: unknown): CatProviderCapability[] {
  if (!Array.isArray(raw) || !raw.length) return [...DEFAULT_PROVIDER_CAPABILITIES]
  const by = new Map<string, CatProviderCapability>()
  for (const x of raw) {
    if (!x || typeof x !== 'object') continue
    const r = x as Record<string, unknown>
    const kind = typeof r.agentKind === 'string' ? r.agentKind : ''
    if (!isProviderKind(kind)) continue
    const dp = typeof r.defaultProtocol === 'string' && PROTOCOLS.includes(r.defaultProtocol as CatProviderProtocol)
      ? (r.defaultProtocol as CatProviderProtocol)
      : 'openai'
    const allowed = Array.isArray(r.allowedProtocols)
      ? (r.allowedProtocols.filter((p): p is CatProviderProtocol => typeof p === 'string' && PROTOCOLS.includes(p as CatProviderProtocol)) as CatProviderProtocol[])
      : [dp]
    by.set(kind, {
      agentKind: kind,
      defaultProtocol: dp,
      allowProtocolPick: r.allowProtocolPick === true,
      allowedProtocols: allowed.length ? allowed : [dp],
    })
  }
  return CAT_PROVIDER_ORDER.map((k) => by.get(k) ?? DEFAULT_PROVIDER_CAPABILITIES.find((c) => c.agentKind === k)!)
}

export async function listCatProviderCapabilities(): Promise<CatProviderCapability[]> {
  if (live()) return normalizeCapabilities(await call(CatBinding.ListCatProviderCapabilities()))
  return [...DEFAULT_PROVIDER_CAPABILITIES]
}

export async function saveCatProviderKey(agentKind: CatProviderKind, apiKey: string): Promise<CatProviderSettings> {
  const key = apiKey.trim()
  if (!key) throw new AppError('INVALID_ARGUMENT', t('cat.misc.s_8482b9a077'))
  if (live()) {
    return normalizeProvider(
      await call(CatBinding.SaveCatProviderKey(cat.SaveProviderKeyRequest.createFrom({ agentKind, apiKey: key }))),
    )
  }
  await wait(200)
  if (sim('keyfail')) throw new AppError('IO_ERROR', 'sim')
  mockSecrets.add(agentKind)
  const cur = mockSettings.providersByAgent[agentKind] ?? emptyProvider()
  const next = { ...cur, apiKeySet: true }
  mockSettings = {
    ...mockSettings,
    providersByAgent: { ...mockSettings.providersByAgent, [agentKind]: next },
  }
  return { ...next }
}

export async function clearCatProviderKey(agentKind: CatProviderKind): Promise<CatProviderSettings> {
  if (live()) {
    return normalizeProvider(
      await call(CatBinding.ClearCatProviderKey(cat.ClearProviderKeyRequest.createFrom({ agentKind }))),
    )
  }
  await wait(200)
  if (sim('keyfail')) throw new AppError('IO_ERROR', 'sim')
  mockSecrets.delete(agentKind)
  const cur = mockSettings.providersByAgent[agentKind] ?? emptyProvider()
  // 不改 enabledCustom / apiUrl
  const next = { ...cur, apiKeySet: false }
  mockSettings = {
    ...mockSettings,
    providersByAgent: { ...mockSettings.providersByAgent, [agentKind]: next },
  }
  return { ...next }
}

const emptyInspect = (reason: string): CatInspectResult => ({
  skills: [],
  plugins: [],
  hooks: [],
  mcpServers: [],
  agents: [],
  reason,
  skillsRoot: '',
  mcpConfigPath: '',
  skillsRootExists: false,
})

function items(raw: unknown): CatInspectItem[] {
  if (!Array.isArray(raw)) return []
  return raw
    .filter((x) => x && typeof x === 'object' && typeof (x as { name?: unknown }).name === 'string')
    .map((x) => {
      const r = x as Record<string, unknown>
      return { name: String(r.name), description: typeof r.description === 'string' ? r.description : '', source: typeof r.source === 'string' ? r.source : '', enabled: r.enabled !== false }
    })
}

function oneItem(raw: unknown): CatInspectItem {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    name: typeof r.name === 'string' ? r.name : '',
    description: typeof r.description === 'string' ? r.description : '',
    source: typeof r.source === 'string' ? r.source : '',
    enabled: r.enabled !== false,
  }
}

export function normalizeInspect(raw: unknown): CatInspectResult {
  if (!raw || typeof raw !== 'object') return emptyInspect('parse')
  const r = raw as Record<string, unknown>
  const reason = typeof r.reason === 'string' ? r.reason : ''
  const skillsRoot = typeof r.skillsRoot === 'string' ? r.skillsRoot : ''
  const mcpConfigPath = typeof r.mcpConfigPath === 'string' ? r.mcpConfigPath : ''
  return {
    skills: items(r.skills),
    plugins: items(r.plugins),
    hooks: items(r.hooks),
    mcpServers: items(r.mcpServers),
    agents: items(r.agents),
    version: typeof r.version === 'string' ? r.version : '',
    reason,
    skillsRoot,
    mcpConfigPath,
    skillsRootExists: r.skillsRootExists === true,
  }
}

export function normalizeSkillsDir(raw: unknown): CatSkillsDirResult {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    skillsRoot: typeof r.skillsRoot === 'string' ? r.skillsRoot : '',
    mcpConfigPath: typeof r.mcpConfigPath === 'string' ? r.mcpConfigPath : '',
    skillsRootExists: r.skillsRootExists === true,
    opened: r.opened === true,
  }
}

/** 页顶路径展示：有 skillsRoot 时尽量显示「项目名/.ffmpegfree/cat/skills」；否则模板。 */
export function skillsPathLabel(_skillsRoot: string | undefined, projectName: string): string {
  if (projectName) return `${projectName}/.ffmpegfree/cat/skills`
  return CAT_SETTINGS_COPY.extTipPathFallback
}

/** 扩展卡：plugins + mcpServers 合并；agents 一期不显示（契约 6.19.12.7-4） */
export function extensionItems(r: CatInspectResult): CatInspectItem[] {
  return [...r.plugins, ...r.mcpServers]
}

function withTimeout<T>(p: Promise<T>, ms: number, fallback: () => T): Promise<T> {
  return new Promise((resolve) => {
    const t = setTimeout(() => resolve(fallback()), ms)
    p.then((v) => resolve(v), () => resolve(fallback())).finally(() => clearTimeout(t))
  })
}

const MOCK_SKILLS_ROOT = '/mock/字幕项目/.ffmpegfree/cat/skills'
const MOCK_MCP_PATH = '/mock/字幕项目/.ffmpegfree/cat/mcp.json'

type MockInspectStore = {
  skills: CatInspectItem[]
  plugins: CatInspectItem[]
  hooks: CatInspectItem[]
  mcpServers: CatInspectItem[]
  agents: CatInspectItem[]
  skillsRootExists: boolean
}

function defaultMockInspect(): MockInspectStore {
  return {
    skills: [
      { name: 'srt-fixer', description: '字幕时间轴校准与合并', source: 'project', enabled: true },
      { name: 'release-notes', description: t('cat.misc.s_6071ab4694'), source: 'project', enabled: true },
      { name: 'pdf-reader', description: t('cat.misc.s_7edbc50534'), source: 'user', enabled: false },
    ],
    plugins: [{ name: 'srt-tools', description: t('cat.misc.s_af2d21fae7'), source: 'plugin', enabled: false }],
    hooks: [{ name: 'pre-commit-lint', description: t('cat.misc.s_d36ed4263f'), source: 'project', enabled: true }],
    mcpServers: [
      { name: 'filesystem', description: t('cat.misc.s_3054404bc5'), source: 'user', enabled: true },
      { name: 'web-search', description: t('cat.misc.s_0128baeda1'), source: 'user', enabled: true },
    ],
    agents: [{ name: 'reviewer', enabled: true }],
    skillsRootExists: true,
  }
}

let mockInspect = defaultMockInspect()

function mockPaths(projectId: string) {
  if (!projectId) return { skillsRoot: '', mcpConfigPath: '' }
  return { skillsRoot: MOCK_SKILLS_ROOT, mcpConfigPath: MOCK_MCP_PATH }
}

function snapshotMockInspect(projectId: string, reason = ''): CatInspectResult {
  const paths = mockPaths(projectId)
  return {
    skills: mockInspect.skills.map((x) => ({ ...x })),
    plugins: mockInspect.plugins.map((x) => ({ ...x })),
    hooks: mockInspect.hooks.map((x) => ({ ...x })),
    mcpServers: mockInspect.mcpServers.map((x) => ({ ...x })),
    agents: mockInspect.agents.map((x) => ({ ...x })),
    version: '1.0.0',
    reason,
    skillsRoot: paths.skillsRoot,
    mcpConfigPath: paths.mcpConfigPath,
    skillsRootExists: !!projectId && mockInspect.skillsRootExists,
  }
}

/** Inspect：须带 projectId + 应传 agentKind；无项目 → reason=no_project。失败空列表 + reason，不抛错。 */
export async function inspectCatConfig(projectId = '', agentKind = ''): Promise<CatInspectResult> {
  if (!projectId) return emptyInspect('no_project')
  if (live()) {
    return withTimeout(
      call(
        CatBinding.InspectCatConfig(
          cat.InspectRequest.createFrom({ projectId, agentKind: agentKind || '' }),
        ),
      ).then(normalizeInspect),
      CAT_INSPECT_TIMEOUT_MS + 1000,
      () => emptyInspect('timeout'),
    )
  }
  await wait(sim('inspectslow') ? 4000 : 400)
  if (sim('inspectfail')) return { ...emptyInspect('timeout'), ...mockPaths(projectId), skillsRootExists: mockInspect.skillsRootExists }
  if (sim('inspectempty')) {
    return {
      ...emptyInspect(''),
      ...mockPaths(projectId),
      skillsRootExists: mockInspect.skillsRootExists,
      version: '1.0.0',
    }
  }
  if (sim('skillsmissing')) mockInspect.skillsRootExists = false
  return snapshotMockInspect(projectId, '')
}

/** v0.39：启停；失败抛错由 UI 回滚 + toast */
export async function setCatInspectItemEnabled(req: {
  projectId: string
  agentKind?: string
  category: CatInspectCategory
  name: string
  enabled: boolean
}): Promise<CatInspectItem> {
  if (!req.projectId) throw new AppError('INVALID_ARGUMENT', 'project')
  if (!req.name) throw new AppError('INVALID_ARGUMENT', 'name')
  if (live()) {
    return oneItem(
      await call(
        CatBinding.SetCatInspectItemEnabled(
          cat.SetInspectItemEnabledRequest.createFrom({
            projectId: req.projectId,
            agentKind: req.agentKind || '',
            category: req.category,
            name: req.name,
            enabled: req.enabled,
          }),
        ),
      ),
    )
  }
  await wait(200)
  if (sim('setenablefail')) throw new AppError('IO_ERROR', 'sim')
  const listKey =
    req.category === 'skill' ? 'skills' : req.category === 'hook' ? 'hooks' : req.category === 'plugin' ? 'plugins' : 'mcpServers'
  const list = mockInspect[listKey]
  const hit = list.find((x) => x.name === req.name)
  if (!hit) throw new AppError('INVALID_ARGUMENT', 'name')
  hit.enabled = req.enabled
  return { ...hit }
}

/** v0.39：打开技能目录；不存在 → exists=false、opened=false、不抛错 */
export async function revealCatSkillsDir(projectId: string): Promise<CatSkillsDirResult> {
  if (!projectId) throw new AppError('INVALID_ARGUMENT', 'project')
  if (live()) {
    return normalizeSkillsDir(
      await call(CatBinding.RevealCatSkillsDir(cat.SkillsDirRequest.createFrom({ projectId }))),
    )
  }
  await wait(200)
  const paths = mockPaths(projectId)
  if (sim('skillsmissing') || !mockInspect.skillsRootExists) {
    return { ...paths, skillsRootExists: false, opened: false }
  }
  return { ...paths, skillsRootExists: true, opened: true }
}

/** v0.39：创建空技能目录并打开；不写 mcp.json / gitignore */
export async function ensureCreateCatSkillsDir(projectId: string): Promise<CatSkillsDirResult> {
  if (!projectId) throw new AppError('INVALID_ARGUMENT', 'project')
  if (live()) {
    return normalizeSkillsDir(
      await call(CatBinding.EnsureCreateCatSkillsDir(cat.SkillsDirRequest.createFrom({ projectId }))),
    )
  }
  await wait(300)
  mockInspect.skillsRootExists = true
  const paths = mockPaths(projectId)
  return { ...paths, skillsRootExists: true, opened: true }
}

export async function clearAllCatConversations(): Promise<void> {
  if (live()) {
    await call(CatBinding.ClearAllCatConversations())
    return
  }
  await wait(500)
  if (sim('clearfail')) throw new AppError('IO_ERROR', 'sim')
}
