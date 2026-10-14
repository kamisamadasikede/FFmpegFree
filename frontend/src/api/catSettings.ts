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
  title: '设置',
  subtitle: '只影响 Cat；FFmpegFree 其它设置在应用的「设置」里',
  tabs: { general: '通用', instruction: '自定义指令', ext: '技能与扩展', about: '关于与状态' },
  newConvCard: '新对话默认',
  newConvHint: '新建对话时按所选服务商带上这里的默认值；每个对话创建后锁定该服务商，之后只显示该服务商返回的模型和强度。',
  installed: '已安装',
  notDetected: '未检测到',
  model: '默认模型',
  modelDesc: '新建对话时预选；每个对话之后各自记住',
  effort: '思考强度',
  effortDesc: '选「默认」时由 Cat 助手自己决定',
  approval: '批准方式',
  approvalDesc: '写文件、跑命令前怎么处理',
  defaultOpt: '默认',
  place: '新对话放在',
  placeDesc: '从侧栏「新对话」进入时的默认项目',
  placeLast: '上次用的项目',
  placeNone: '不放在项目里',
  looksCard: '外观',
  theme: '主题',
  themeDesc: '跟随 FFmpegFree 的「设置 → 外观」',
  themeLight: '跟随应用 · 浅色',
  themeDark: '跟随应用 · 暗色',
  typewriter: '回复逐字显示',
  typewriterDesc: '关闭后一段一段出现',
  convCard: '对话',
  notify: '完成后通知',
  notifyDesc: '窗口不在前台时发系统通知，在前台时显示应用内提示',
  history: '历史保留',
  later: '下一期开放。',
  clear: '清空全部对话',
  clearDesc: '也会删除应用自己的普通对话文件夹；项目文件夹绝不删除',
  clearBtn: '清空…',
  clearTitle: '清空全部对话？',
  clearBody: '所有对话记录会被删除，包括定时任务产生的对话，无法恢复。定时任务、项目文件夹和里面的文件不受影响。',
  clearOk: '清空',
  clearFailed: '清空失败，请重试。',
  cleared: '已清空全部对话。',
  cancel: '取消',
  keysCard: '快捷键',
  keys: [
    ['新对话', 'N'],
    ['搜索对话', 'K'],
    ['收起侧栏', 'B'],
    ['切换右侧面板', 'J'],
  ] as [string, string][],
  stopKey: '停止回复',
  saved: '已保存',
  saveFailed: '保存失败，请重试。',
  instrHint: '每次对话都会自动带上这段指令。改动对新发送的消息生效，进行中的回复不受影响。',
  instrPlaceholder: '比如：回答用中文；改代码前先说明要改哪些文件。',
  instrOver: '最多 4000 字。',
  instrSave: '保存',
  instrRevert: '恢复上次保存',
  /** v0.39：页顶路径说明（无 skillsRoot 时用模板路径） */
  extTipBefore: '技能与 MCP 按',
  extTipProject: '当前项目',
  extTipMid: '存放，三家共用。路径：',
  extTipPathFallback: '<项目>/.ffmpegfree/cat/skills',
  extTipAfter: '（MCP：同级 mcp.json）。在此只能启用或停用；安装与卸载请在对应 CLI 或系统终端完成。',
  openDir: '打开目录',
  createAndOpen: '创建并打开',
  noSkillsDir: '项目里还没有技能目录。',
  noSkillsDirBody: '将在当前项目下创建 `.ffmpegfree/cat/skills`，并打开该文件夹。',
  noProjectExt: '先打开或选择一个项目，再管理技能与扩展。',
  pickProject: '选择项目…',
  skills: '技能',
  exts: '扩展',
  extsFull: '扩展（MCP 与插件）',
  hooks: '钩子',
  extEmpty: '还没有安装。',
  extFailed: '列表加载失败，请重试。',
  retry: '重试',
  on: '已启用',
  off: '未启用',
  statusCard: '状态',
  detect: '检测',
  ready: '已安装 · 已就绪',
  missing: '未检测到',
  checking: '检查中…',
  version: '版本',
  login: '登录',
  loggedIn: '已登录',
  loginBad: '登录失效',
  loginNa: '不适用',
  noneReady: '未检测到可用的助手，请先安装并确保可在终端直接运行。',
  lastCheck: '上次检测',
  recheck: '重新检测',
  rechecking: '检测中…',
  aboutCard: '关于',
  aboutLine: 'Cat · FFmpegFree 内置的项目助手',
  openData: '打开数据文件夹',
  // v0.38 自定义服务商（设计 v0.1.1 §3 / 契约 6.19.12.8）
  customFold: '自定义服务商',
  customFoldHintOff: '默认用本机登录；需要时可填 URL / Key',
  customFoldHintOn: '启用后只用下面的 URL / Key，不写回系统环境变量',
  enableCustom: '启用自定义',
  enableCustomDesc: '关闭后回落本机登录 / 环境变量',
  protocol: '协议',
  protocolLocked: '已锁定，按该助手默认',
  apiUrl: 'API URL',
  apiUrlDesc: 'Base URL',
  apiKey: 'API Key',
  apiKeyDesc: '保存后不明文回显',
  apiKeyPlaceholder: '粘贴 API Key',
  apiKeySaved: '已保存',
  apiKeyClear: '清除',
  clearKeyTitle: '清除 API Key？',
  clearKeyBody: '清除后这台电脑上不再保存该 Key，对话将改用 CLI 默认登录。',
  clearKeyOk: '清除',
} as const

export function loginBanner(name: string) {
  return `${name}登录失效，请重新登录后再试。`
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

export async function saveCatSettings(req: CatSettings): Promise<CatSettings> {
  if (req.customInstruction.length > CAT_INSTRUCTION_MAX) throw new AppError('INVALID_ARGUMENT', CAT_SETTINGS_COPY.instrOver)
  const payload = settingsPayload(req)
  if (live()) {
    return normalizeSettings(await call(CatBinding.SaveCatSettings(cat.Settings.createFrom(payload))))
  }
  await wait(250)
  if (sim('savefail')) throw new AppError('IO_ERROR', 'sim')
  // Save 不因提交而清除已存 Key；apiKeySet 仍由 mockSecrets 决定
  mockSettings = withMockKeyFlags(normalizeSettings(payload))
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
  if (!key) throw new AppError('INVALID_ARGUMENT', '请填写 API Key。')
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
      { name: 'release-notes', description: '按提交生成更新说明', source: 'project', enabled: true },
      { name: 'pdf-reader', description: '读取 PDF 正文', source: 'user', enabled: false },
    ],
    plugins: [{ name: 'srt-tools', description: '字幕格式转换', source: 'plugin', enabled: false }],
    hooks: [{ name: 'pre-commit-lint', description: '提交前检查代码格式', source: 'project', enabled: true }],
    mcpServers: [
      { name: 'filesystem', description: '读写项目文件', source: 'user', enabled: true },
      { name: 'web-search', description: '网页搜索', source: 'user', enabled: true },
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
