/**
 * Cat 设置接口层（契约 v0.35.1 §6.19.12；v0.36 defaultsByAgent）。
 * CAT_SETTINGS_BACKEND_READY=true 且在 Wails 里：调生成的 CatService 绑定。
 * 否则走内存模拟；走查开关见 views/cat/catDevSim.ts（savefail / inspectfail / inspectslow / inspectempty / clearfail）。
 *
 * Save 走生成的 cat.Settings（v0.36：defaultAgentKind + defaultsByAgent）。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { AppError, call } from '@/api/call'
import { CAT_AGENT_BUILD, CAT_PROVIDER_ORDER, isProviderKind, type CatProviderKind } from '@/api/catAgents'
import { CAT_SETTINGS_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'
import { catSim } from '@/views/cat/catDevSim'

export { CAT_SETTINGS_BACKEND_READY }

export interface CatAgentDefaults {
  model: string
  effort: string
}

export interface CatSettings {
  /** 欢迎页预选；默认 cat_build */
  defaultAgentKind: CatProviderKind
  defaultsByAgent: Record<CatProviderKind, CatAgentDefaults>
  defaultApprovalMode: string
  customInstruction: string
  notifyOnFinish: boolean
}

export function emptyDefaults(): CatAgentDefaults {
  return { model: '', effort: '' }
}

export function emptyDefaultsByAgent(): Record<CatProviderKind, CatAgentDefaults> {
  return { cat_build: emptyDefaults(), claude_cli: emptyDefaults(), codex_cli: emptyDefaults() }
}

export interface CatInspectItem {
  name: string
  description?: string
  source?: string
  enabled: boolean
}

export interface CatInspectResult {
  skills: CatInspectItem[]
  plugins: CatInspectItem[]
  hooks: CatInspectItem[]
  mcpServers: CatInspectItem[]
  agents: CatInspectItem[]
  version?: string
  reason?: '' | 'not_ready' | 'timeout' | 'parse' | string
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
  extTip: '这里只读显示 Cat 助手在本机已安装的内容。增删改请在 Cat 助手里操作，下一期开放在这里管理。',
  skills: '技能',
  exts: '扩展',
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

/**
 * 读路径：v0.36 defaultsByAgent + defaultAgentKind。
 * 旧顶层 defaultModel/defaultEffort 迁进 cat_build（该 key 已有非空值则不覆盖）。
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
    defaultApprovalMode: ['normal', 'plan', 'auto', 'approve'].includes(s('defaultApprovalMode')) ? s('defaultApprovalMode') : '',
    customInstruction: s('customInstruction'),
    // 契约：nil 视为 true
    notifyOnFinish: r.notifyOnFinish !== false,
  }
}

/** 只提交 v0.36 形状（不含旧 defaultModel/defaultEffort） */
export function settingsPayload(req: CatSettings): Record<string, unknown> {
  const defaultsByAgent: Record<string, CatAgentDefaults> = {}
  for (const kind of CAT_PROVIDER_ORDER) {
    const d = req.defaultsByAgent[kind] ?? emptyDefaults()
    defaultsByAgent[kind] = { model: d.model || '', effort: d.effort || '' }
  }
  return {
    defaultAgentKind: isProviderKind(req.defaultAgentKind) ? req.defaultAgentKind : CAT_AGENT_BUILD,
    defaultsByAgent,
    defaultApprovalMode: req.defaultApprovalMode,
    customInstruction: req.customInstruction,
    notifyOnFinish: req.notifyOnFinish,
  }
}

let mockSettings: CatSettings = normalizeSettings({})
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

export async function getCatSettings(): Promise<CatSettings> {
  if (live()) return normalizeSettings(await call(CatBinding.GetCatSettings()))
  return normalizeSettings(mockSettings)
}

export async function saveCatSettings(req: CatSettings): Promise<CatSettings> {
  if (req.customInstruction.length > CAT_INSTRUCTION_MAX) throw new AppError('INVALID_ARGUMENT', CAT_SETTINGS_COPY.instrOver)
  const payload = settingsPayload(req)
  if (live()) {
    return normalizeSettings(await call(CatBinding.SaveCatSettings(cat.Settings.createFrom(payload))))
  }
  await wait(250)
  if (sim('savefail')) throw new AppError('IO_ERROR', 'sim')
  mockSettings = normalizeSettings(payload)
  return normalizeSettings(mockSettings)
}

const emptyInspect = (reason: string): CatInspectResult => ({ skills: [], plugins: [], hooks: [], mcpServers: [], agents: [], reason })

function items(raw: unknown): CatInspectItem[] {
  if (!Array.isArray(raw)) return []
  return raw
    .filter((x) => x && typeof x === 'object' && typeof (x as { name?: unknown }).name === 'string')
    .map((x) => {
      const r = x as Record<string, unknown>
      return { name: String(r.name), description: typeof r.description === 'string' ? r.description : '', source: typeof r.source === 'string' ? r.source : '', enabled: r.enabled !== false }
    })
}

export function normalizeInspect(raw: unknown): CatInspectResult {
  if (!raw || typeof raw !== 'object') return emptyInspect('parse')
  const r = raw as Record<string, unknown>
  return {
    skills: items(r.skills),
    plugins: items(r.plugins),
    hooks: items(r.hooks),
    mcpServers: items(r.mcpServers),
    agents: items(r.agents),
    version: typeof r.version === 'string' ? r.version : '',
    reason: typeof r.reason === 'string' ? r.reason : '',
  }
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

/** 只读；失败一律空列表 + reason，不抛错。前端再兜一层 15 秒超时。 */
export async function inspectCatConfig(projectId = ''): Promise<CatInspectResult> {
  if (live()) {
    return withTimeout(
      call(CatBinding.InspectCatConfig(cat.InspectRequest.createFrom({ projectId }))).then(normalizeInspect),
      CAT_INSPECT_TIMEOUT_MS + 1000,
      () => emptyInspect('timeout'),
    )
  }
  await wait(sim('inspectslow') ? 4000 : 600)
  if (sim('inspectfail')) return emptyInspect('timeout')
  if (sim('inspectempty')) return emptyInspect('')
  return {
    skills: [
      { name: 'subtitle-check', description: '检查字幕时间轴和重叠', source: 'user', enabled: true },
      { name: 'release-notes', description: '按提交记录整理更新说明', source: 'project', enabled: false },
    ],
    plugins: [{ name: 'image-tools', description: '图片压缩和格式转换', source: 'plugin', enabled: true }],
    hooks: [],
    mcpServers: [{ name: 'files', description: '读取本机文件夹', source: 'user', enabled: true }],
    agents: [{ name: 'reviewer', enabled: true }],
    version: '1.0.0',
    reason: '',
  }
}

export async function clearAllCatConversations(): Promise<void> {
  if (live()) {
    await call(CatBinding.ClearAllCatConversations())
    return
  }
  await wait(500)
  if (sim('clearfail')) throw new AppError('IO_ERROR', 'sim')
}
