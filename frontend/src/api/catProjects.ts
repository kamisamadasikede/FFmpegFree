/**
 * Cat 项目接口层（契约 v0.31 §6.19.10 + v0.31.1 RelocateCatProject / RevealCatProject；设计「Cat 项目状态」v0.2）。
 *
 * - CAT_PROJECTS_BACKEND_READY=true 且在 Wails 里且绑定存在：按名字调 CatService（callService，后端合入前不 import 生成文件）
 * - 纯浏览器（没有 window.go，走查 / 截图）：内存模拟，规则与契约一致（路径去重 existed、根目录拒绝、名字 1~60、missing 不能新建对话）
 * - Wails 里但开关关：没有项目；新建 / 改名 / 删除 / 显示 / 重新选择都不调后端（UNSUPPORTED → 「下一期开放。」）
 *
 * 用户可见文案统一放在 CAT_PROJECT_COPY（产品可改，只改这里）。
 */
import { AppError, callService, toAppError } from '@/api/call'
import { CAT_PROJECTS_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend, onEvent } from '@/services/wails'
import { pickDirectory } from '@/api/system'

export { CAT_PROJECTS_BACKEND_READY }

/** 项目相关文案（契约 6.19.10.8 + 设计「Cat 项目状态 v0.1」§1 + PM 10-09；产品已定） */
export const CAT_PROJECT_COPY = {
  empty: '还没有项目。',
  emptyConvs: '还没有对话。',
  pickTitle: '选择项目文件夹',
  /** 设计 v0.2 §09b：重新选择时系统对话框标题 */
  relocateTitle: '重新选择项目文件夹',
  relocated: '已重新选择文件夹。',
  /** 契约 v0.31.1 6.19.10.8（架构师建议，产品可改） */
  turnRunning: '有对话正在回复，请先停止再换文件夹。',
  newProject: '新建项目文件夹',
  newConvInProject: '在这个项目里新建对话',
  menu: '项目操作',
  rename: '重命名',
  renameHint: '回车保存，Esc 取消',
  renameLabel: '项目名称',
  revealWindows: '在资源管理器中显示',
  revealMac: '在访达中显示',
  revealLinux: '在文件管理器中显示',
  relocate: '重新选择文件夹',
  del: '删除项目',
  delTitle: (name: string) => `删除项目「${name}」？`,
  delBody: '删除项目会同时删除它下面的对话，文件夹里的文件不受影响。',
  cancel: '取消',
  existed: '这个文件夹已经建过项目了。',
  missing: '项目文件夹不见了。',
  badPath: '请选择一个文件夹。',
  badRoot: '不能把整个磁盘作为项目，请选择里面的文件夹。',
  badName: '名字需要 1~60 个字。',
  notFound: '找不到这个项目。',
  deleteFailed: '删除项目失败，请重试。',
  failed: '操作没有完成，请重试。',
  later: '下一期开放。',
  workIn: (name: string) => `在项目中工作 · ${name}`,
  added: (name: string) => `已添加项目「${name}」。`,
  deleted: (name: string) => `已删除项目「${name}」。`,
} as const

/** 名字最长字符数（按 Unicode 字符，契约 6.19.10.2） */
export const CAT_PROJECT_NAME_MAX = 60

/** 契约 6.19.10.1 */
export interface CatProject {
  id: string
  name: string
  path: string
  createdAt: number
  updatedAt: number
  missing: boolean
}
export interface CreateCatProjectRequest { path: string; name?: string }
export interface CreateCatProjectResult { project: CatProject; existed: boolean }
export interface RenameCatProjectRequest { id: string; name: string }
export interface DeleteCatProjectRequest { id: string }
/** 契约 v0.31.1 6.19.10.2 第 9 条：在系统文件管理器里打开项目文件夹；文件夹不在 → CAT_PROJECT_MISSING */
export interface RevealCatProjectRequest { id: string }
/**
 * 契约 v0.31.1 6.19.10.2 第 8 条：换项目文件夹，对话保留、名字不变；missing 的项目也可以。
 * 新路径属于另一个项目 → INVALID_ARGUMENT reason=project_duplicate，detail 第二行 projectId=<已有项目 id>（本项目不变）；
 * 有进行中的一轮 → TASK_CONFLICT reason=turn_running。返回更新后的 CatProject。
 */
export interface RelocateCatProjectRequest { id: string; path: string }
/** 事件 cat:project（6.19.10.3） */
export interface CatProjectEvent { id: string; missing: boolean }

export const CAT_PROJECT_EVENT = 'cat:project'

const METHODS = ['ListCatProjects', 'CreateCatProject', 'RenameCatProject', 'DeleteCatProject', 'RevealCatProject', 'RelocateCatProject'] as const
type Method = (typeof METHODS)[number]

function hasBinding(m: Method): boolean {
  const svc = (globalThis as { window?: { go?: { app?: { CatService?: Record<string, unknown> } } } }).window?.go?.app?.CatService
  return typeof svc?.[m] === 'function'
}

/** 项目接口走真后端：开关打开 + 在 Wails 里 */
export const catProjectsLive = (): boolean => CAT_PROJECTS_BACKEND_READY && hasWailsBackend()
/** 走内存模拟：纯浏览器（走查） */
export const catProjectsSim = (): boolean => !hasWailsBackend()
/** 项目功能是否可用（真后端或浏览器模拟）；Wails 里开关关时为 false */
export const catProjectsOn = (): boolean => catProjectsLive() || catProjectsSim()

async function real<T>(m: Method, ...args: unknown[]): Promise<T> {
  if (!hasBinding(m)) throw new AppError('UNSUPPORTED', CAT_PROJECT_COPY.later)
  return await callService<T>('CatService', m, ...args)
}

function off(): never {
  throw new AppError('UNSUPPORTED', CAT_PROJECT_COPY.later)
}

export function mapProject(raw: unknown): CatProject {
  const r = (raw ?? {}) as Partial<Record<keyof CatProject, unknown>>
  return {
    id: String(r.id ?? ''),
    name: String(r.name ?? ''),
    path: String(r.path ?? ''),
    createdAt: Number(r.createdAt) || 0,
    updatedAt: Number(r.updatedAt) || 0,
    missing: r.missing === true,
  }
}
function mapResult(raw: unknown): CreateCatProjectResult {
  const r = (raw ?? {}) as { project?: unknown; existed?: unknown }
  return { project: mapProject(r.project), existed: r.existed === true }
}

// ---------- 名字 / 路径工具（前端预检，规则同契约；后端仍以自己的校验为准） ----------

/** 文件夹名（filepath.Base 的前端版；兼容 \ 和 /） */
export function folderName(path: string): string {
  const p = path.trim().replace(/[\\/]+$/, '')
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i >= 0 ? p.slice(i + 1) : p
}

/** 名字是否合法：去首尾空白后 1~60 个 Unicode 字符，不含换行 / 控制字符 */
export function validProjectName(name: string): boolean {
  const t = name.trim()
  const n = [...t].length
  // eslint-disable-next-line no-control-regex
  return n >= 1 && n <= CAT_PROJECT_NAME_MAX && !/[\u0000-\u001f\u007f]/.test(t)
}

/** 6.19.10.2 第 2 条的比较键（模拟层用；Windows 风格路径不区分大小写） */
export function pathKey(path: string): string {
  let p = path.trim().replace(/[\\/]+$/, '')
  const win = /^[A-Za-z]:/.test(p) || p.startsWith('\\\\')
  if (win) p = p.replace(/\//g, '\\').toLowerCase()
  return p
}
/** 盘符根 / 文件系统根 / UNC 共享根 */
export function isRootPath(path: string): boolean {
  const p = path.trim()
  return p === '/' || /^[A-Za-z]:[\\/]?$/.test(p) || /^\\\\[^\\]+\\[^\\]+\\?$/.test(p)
}
function isAbs(path: string): boolean {
  return path.startsWith('/') || /^[A-Za-z]:[\\/]/.test(path) || path.startsWith('\\\\')
}

/** project_duplicate 的 detail 第二行 `projectId=<id>`（契约 2.2）；取不到返回 "" */
export function duplicateProjectId(e: unknown): string {
  const err = toAppError(e)
  if (err.code !== 'INVALID_ARGUMENT' || err.reason !== 'project_duplicate') return ''
  const line = (err.detail ?? '').split(/\r?\n/).map((x) => x.trim()).find((x) => /^projectId=(\S+)$/.test(x))
  return line ? line.slice('projectId='.length) : ''
}

/** 错误 → 用户文案（只出现 CAT_PROJECT_COPY 里的句子，不显示错误码 / 路径） */
export function projectErrorText(e: unknown): string {
  const err = toAppError(e)
  if (err.code === 'CAT_PROJECT_MISSING') return CAT_PROJECT_COPY.missing
  if (err.code === 'INVALID_ARGUMENT') {
    if (err.reason === 'project_root') return CAT_PROJECT_COPY.badRoot
    if (err.reason === 'project_name') return CAT_PROJECT_COPY.badName
    if (err.reason === 'project_path') return CAT_PROJECT_COPY.badPath
    if (err.reason === 'project_duplicate') return CAT_PROJECT_COPY.existed
  }
  if (err.code === 'TASK_CONFLICT' && err.reason === 'turn_running') return CAT_PROJECT_COPY.turnRunning
  if (err.code === 'NOT_FOUND') return CAT_PROJECT_COPY.notFound
  if (err.code === 'UNSUPPORTED') return CAT_PROJECT_COPY.later
  return CAT_PROJECT_COPY.failed
}

// ---------- 内存模拟（纯浏览器） ----------

const sim = {
  list: [] as CatProject[],
  seq: 0,
  /** 走查：被「拿走」的文件夹路径键（missing 计算用） */
  gone: new Set<string>(),
  /** 走查：选文件夹对话框依次返回这些路径 */
  picks: [] as string[],
  pickN: 0,
}

/** 走查 / 自检：重置模拟数据 */
export function resetCatProjectSim(list: Array<Omit<CatProject, 'missing'> & { missing?: boolean }> = [], gone: string[] = []) {
  sim.list = list.map((p) => ({ ...p, missing: false }))
  sim.gone = new Set(gone.map(pathKey))
  for (const p of list) if (p.missing) sim.gone.add(pathKey(p.path))
  sim.seq = 0
}
/** 走查：下一次选文件夹返回的路径（空串 = 用户取消） */
export function queueSimPick(...paths: string[]) {
  sim.picks.push(...paths)
}
/** 走查：模拟文件夹被移走 / 回来 */
export function setSimFolderGone(path: string, gone: boolean) {
  if (gone) sim.gone.add(pathKey(path))
  else sim.gone.delete(pathKey(path))
}
const simMissing = (p: CatProject) => sim.gone.has(pathKey(p.path))
const simCopy = (p: CatProject): CatProject => ({ ...p, missing: simMissing(p) })

function simValidatePath(path: string) {
  const p = path.trim()
  if (!p || !isAbs(p)) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.badPath, 'reason=project_path')
  if (isRootPath(p)) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.badRoot, 'reason=project_root')
  if (sim.gone.has(pathKey(p))) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.badPath, 'reason=project_path')
}

// ---------- 接口 ----------

/** 选项目文件夹；用户取消返回 ""（调用方什么都不做）。relocate=true 用「重新选择项目文件夹」标题（设计 v0.2 §09b） */
export async function pickProjectFolder(relocate = false): Promise<string> {
  if (catProjectsSim()) return sim.picks.length ? (sim.picks.shift() as string) : `D:\\工作\\新项目 ${++sim.pickN}`
  if (!catProjectsLive()) off()
  return await pickDirectory(relocate ? CAT_PROJECT_COPY.relocateTitle : CAT_PROJECT_COPY.pickTitle)
}

export async function listCatProjects(): Promise<CatProject[]> {
  if (catProjectsLive()) {
    const raw = await real<unknown>('ListCatProjects')
    return Array.isArray(raw) ? raw.map(mapProject) : []
  }
  if (catProjectsSim()) return sim.list.map(simCopy)
  return []
}

export async function createCatProject(req: CreateCatProjectRequest): Promise<CreateCatProjectResult> {
  if (catProjectsLive()) return mapResult(await real('CreateCatProject', { path: req.path, name: req.name ?? '' }))
  if (!catProjectsSim()) off()
  simValidatePath(req.path)
  const key = pathKey(req.path)
  const old = sim.list.find((p) => pathKey(p.path) === key)
  if (old) return { project: simCopy(old), existed: true }
  let name = (req.name ?? '').trim()
  if (name && !validProjectName(name)) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.badName, 'reason=project_name')
  if (!name) name = [...folderName(req.path)].slice(0, CAT_PROJECT_NAME_MAX).join('')
  const now = Date.now()
  const p: CatProject = { id: `sim-p-${now.toString(36)}-${++sim.seq}`, name, path: req.path.trim(), createdAt: now, updatedAt: now, missing: false }
  sim.list.unshift(p)
  return { project: { ...p }, existed: false }
}

export async function renameCatProject(req: RenameCatProjectRequest): Promise<CatProject> {
  if (catProjectsLive()) return mapProject(await real('RenameCatProject', { id: req.id, name: req.name }))
  if (!catProjectsSim()) off()
  if (!validProjectName(req.name)) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.badName, 'reason=project_name')
  const p = sim.list.find((x) => x.id === req.id)
  if (!p) throw new AppError('NOT_FOUND', CAT_PROJECT_COPY.notFound)
  p.name = req.name.trim()
  p.updatedAt = Date.now()
  return simCopy(p)
}

export async function deleteCatProject(req: DeleteCatProjectRequest): Promise<void> {
  if (!req.id) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.notFound)
  if (catProjectsLive()) {
    await real('DeleteCatProject', { id: req.id })
    return
  }
  if (!catProjectsSim()) off()
  sim.list = sim.list.filter((p) => p.id !== req.id)
}

/** 在系统文件管理器里显示项目文件夹（文件夹不见了时菜单项禁用，不调用） */
export async function revealCatProject(req: RevealCatProjectRequest): Promise<void> {
  if (catProjectsLive()) {
    await real('RevealCatProject', { id: req.id })
    return
  }
  if (!catProjectsSim()) off()
  const p = sim.list.find((x) => x.id === req.id)
  if (!p) throw new AppError('NOT_FOUND', CAT_PROJECT_COPY.notFound)
  if (simMissing(p)) throw new AppError('CAT_PROJECT_MISSING', CAT_PROJECT_COPY.missing)
  console.info('[模拟] 显示项目文件夹', p.path)
}

/** 重新选择文件夹（6.19.10.2 第 8 条）：对话保留、名字不变；重复 → INVALID_ARGUMENT reason=project_duplicate（第二行 projectId=） */
export async function relocateCatProject(req: RelocateCatProjectRequest): Promise<CatProject> {
  if (catProjectsLive()) return mapProject(await real('RelocateCatProject', { id: req.id, path: req.path }))
  if (!catProjectsSim()) off()
  if (!req.id) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.notFound)
  const p = sim.list.find((x) => x.id === req.id)
  if (!p) throw new AppError('NOT_FOUND', CAT_PROJECT_COPY.notFound)
  simValidatePath(req.path)
  const key = pathKey(req.path)
  if (key === pathKey(p.path)) return simCopy(p)
  const other = sim.list.find((x) => x.id !== p.id && pathKey(x.path) === key)
  if (other) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.existed, `reason=project_duplicate\nprojectId=${other.id}`)
  p.path = req.path.trim()
  p.updatedAt = Date.now()
  return simCopy(p)
}

export function onCatProject(cb: (e: CatProjectEvent) => void): () => void {
  if (!catProjectsLive()) return () => {}
  return onEvent<unknown>(CAT_PROJECT_EVENT, (raw) => {
    const r = raw as { id?: unknown; missing?: unknown } | null
    if (r && typeof r.id === 'string' && r.id) cb({ id: r.id, missing: r.missing === true })
  })
}

// ---------- 平台文案 ----------

export type CatPlatform = 'windows' | 'darwin' | 'linux'

/** 「在 … 中显示」按平台（PM 10-09） */
export function revealLabel(platform: CatPlatform | string): string {
  if (platform === 'windows') return CAT_PROJECT_COPY.revealWindows
  if (platform === 'darwin') return CAT_PROJECT_COPY.revealMac
  return CAT_PROJECT_COPY.revealLinux
}

/** 纯浏览器 / 拿不到运行时信息时按 userAgent 猜 */
export function guessPlatform(ua: string = typeof navigator !== 'undefined' ? navigator.userAgent : ''): CatPlatform {
  if (/Windows/i.test(ua)) return 'windows'
  if (/Mac OS X|Macintosh/i.test(ua)) return 'darwin'
  return 'linux'
}
