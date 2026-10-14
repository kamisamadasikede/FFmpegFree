/**
 * Cat 项目接口层（契约 v0.31 §6.19.10 + v0.31.1 RelocateCatProject / RevealCatProject；设计「Cat 项目状态」v0.2）。
 *
 * - CAT_PROJECTS_BACKEND_READY=true 且在 Wails 里：调生成的 CatService 绑定（后端 #181，v2 f588fad；wailsjs/go/app/CatService）
 * - 纯浏览器（没有 window.go，走查 / 截图）：内存模拟，规则与契约一致（路径去重 existed、根目录拒绝、名字 1~60、missing 不能新建对话）
 * - Wails 里但开关关：没有项目；新建 / 改名 / 删除 / 显示 / 重新选择都不调后端（UNSUPPORTED → 「下一期开放。」）
 *
 * 用户可见文案统一放在 CAT_PROJECT_COPY（产品可改，只改这里）。
 */
import * as CatBinding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { AppError, call, toAppError } from '@/api/call'
import { CAT_PROJECTS_BACKEND_READY } from '@/api/flags'
import { emitSimEvent, hasWailsBackend, onEvent } from '@/services/wails'
import { pickDirectory } from '@/api/system'
import { t } from '@/i18n'

export { CAT_PROJECTS_BACKEND_READY }

/** 项目相关文案（契约 6.19.10.8 + 设计「Cat 项目状态 v0.1」§1 + PM 10-09；产品已定） */
export const CAT_PROJECT_COPY = {
  get empty() { return t('cat.project.empty') },
  get emptyConvs() { return t('cat.project.emptyConvs') },
  get pickTitle() { return t('cat.project.pickTitle') },
  get relocateTitle() { return t('cat.project.relocateTitle') },
  get relocated() { return t('cat.project.relocated') },
  get turnRunning() { return t('cat.project.turnRunning') },
  get newProject() { return t('cat.project.newProject') },
  get newConvInProject() { return t('cat.project.newConvInProject') },
  get menu() { return t('cat.project.menu') },
  get rename() { return t('cat.project.rename') },
  get renameHint() { return t('cat.project.renameHint') },
  get renameLabel() { return t('cat.project.renameLabel') },
  get revealWindows() { return t('cat.project.revealWindows') },
  get revealMac() { return t('cat.project.revealMac') },
  get revealLinux() { return t('cat.project.revealLinux') },
  get relocate() { return t('cat.project.relocate') },
  get del() { return t('cat.project.del') },
  delTitle: (name: string) => t('cat.project.delTitle', { name: name }),
  get delBody() { return t('cat.project.delBody') },
  get cancel() { return t('cat.project.cancel') },
  get existed() { return t('cat.project.existed') },
  get missing() { return t('cat.project.missing') },
  get badPath() { return t('cat.project.badPath') },
  get badRoot() { return t('cat.project.badRoot') },
  get badName() { return t('cat.project.badName') },
  get notFound() { return t('cat.project.notFound') },
  get deleteFailed() { return t('cat.project.deleteFailed') },
  get failed() { return t('cat.project.failed') },
  get later() { return t('cat.project.later') },
  workIn: (name: string) => t('cat.project.workIn', { name: name }),
  added: (name: string) => t('cat.project.added', { name: name }),
  deleted: (name: string) => t('cat.project.deleted', { name: name }),
  get addTitle() { return t('cat.project.addTitle') },
  get addLead() { return t('cat.project.addLead') },
  get entryLocal() { return t('cat.project.entryLocal') },
  get entryLocalHint() { return t('cat.project.entryLocalHint') },
  get entryGit() { return t('cat.project.entryGit') },
  get entryGitHint() { return t('cat.project.entryGitHint') },
  get continue() { return t('cat.project.continue') },
  get cloneTitle() { return t('cat.project.cloneTitle') },
  get cloneUrl() { return t('cat.project.cloneUrl') },
  get cloneUrlPh() { return t('cat.project.cloneUrlPh') },
  get cloneParent() { return t('cat.project.cloneParent') },
  get cloneParentOpt() { return t('cat.project.cloneParentOpt') },
  get clonePickParent() { return t('cat.project.clonePickParent') },
  get clonePickParentTitle() { return t('cat.project.clonePickParentTitle') },
  get cloneName() { return t('cat.project.cloneName') },
  get cloneNameHint() { return t('cat.project.cloneNameHint') },
  get cloneSubmit() { return t('cat.project.cloneSubmit') },
  get cloningTitle() { return t('cat.project.cloningTitle') },
  get cloneBusy() { return t('cat.project.cloneBusy') },
  get cloneExists() { return t('cat.project.cloneExists') },
  get badGitUrl() { return t('cat.project.badGitUrl') },
  get noGit() { return t('cat.project.noGit') },
  get cloneAuth() { return t('cat.project.cloneAuth') },
  get cloneUnwritable() { return t('cat.project.cloneUnwritable') },
  get cloneFailPrefix() { return t('cat.project.cloneFailPrefix') },
  get cloneCanceled() { return t('cat.project.cloneCanceled') },
  get noGitHint() { return t('cat.project.noGitHint') },
}

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

/** v0.40 6.19.10.11 */
export type CatCloneStatus = 'starting' | 'cloning' | 'creating' | 'succeeded' | 'failed' | 'canceled'
export interface CloneCatProjectRequest { url: string; parentDir?: string; name?: string }
export interface CloneCatProjectResult {
  cloneId: string
  status: CatCloneStatus
  project?: CatProject
}
export interface CancelCloneCatProjectRequest { cloneId: string }
export interface CatCloneEvent {
  cloneId: string
  status: CatCloneStatus
  url?: string
  parentDir?: string
  name?: string
  progress?: number
  project?: CatProject
  error?: { code?: string; message?: string; detail?: string } | null
  canceled?: boolean
}

/** 事件 cat:project（6.19.10.3） */
export interface CatProjectEvent { id: string; missing: boolean }

export const CAT_PROJECT_EVENT = 'cat:project'
export const CAT_CLONE_EVENT = 'cat:clone'

/** 项目接口走真后端：开关打开 + 在 Wails 里 */
export const catProjectsLive = (): boolean => CAT_PROJECTS_BACKEND_READY && hasWailsBackend()
/** 走内存模拟：纯浏览器（走查） */
export const catProjectsSim = (): boolean => !hasWailsBackend()
/** 项目功能是否可用（真后端或浏览器模拟）；Wails 里开关关时为 false */
export const catProjectsOn = (): boolean => catProjectsLive() || catProjectsSim()

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
    if (err.reason === 'git_url') return CAT_PROJECT_COPY.badGitUrl
    if (err.reason === 'clone_exists') return CAT_PROJECT_COPY.cloneExists
  }
  if (err.code === 'TASK_CONFLICT') {
    if (err.reason === 'turn_running') return CAT_PROJECT_COPY.turnRunning
    if (err.reason === 'clone_busy') return CAT_PROJECT_COPY.cloneBusy
  }
  if (err.code === 'NOT_FOUND') {
    // 无 git 与「找不到项目」共用 NOT_FOUND；优先看 message
    const m = (err.message || '').trim()
    if (m === CAT_PROJECT_COPY.noGit || m.includes(t('cat.misc.s_ca17691921'))) return CAT_PROJECT_COPY.noGit
    return CAT_PROJECT_COPY.notFound
  }
  if (err.code === 'UNSUPPORTED') return CAT_PROJECT_COPY.later
  return CAT_PROJECT_COPY.failed
}

/** 克隆失败 → 行内字段 / 块级文案（设计 v0.1.1：无效地址与不可写走行内） */
export type CloneErrorPlace = 'url' | 'parent' | 'banner'
export function cloneErrorPlacement(e: unknown): { place: CloneErrorPlace; text: string } {
  const err = toAppError(e)
  const msg = (err.message || '').trim()
  if (err.reason === 'git_url' || msg === CAT_PROJECT_COPY.badGitUrl) {
    return { place: 'url', text: CAT_PROJECT_COPY.badGitUrl }
  }
  if (msg === CAT_PROJECT_COPY.cloneUnwritable || /无法写入目标文件夹/.test(msg)) {
    return { place: 'parent', text: CAT_PROJECT_COPY.cloneUnwritable }
  }
  if (err.reason === 'clone_exists' || msg === CAT_PROJECT_COPY.cloneExists) {
    return { place: 'parent', text: CAT_PROJECT_COPY.cloneExists }
  }
  if (msg === CAT_PROJECT_COPY.noGit || (err.code === 'NOT_FOUND' && /没有找到 git/.test(msg))) {
    return { place: 'banner', text: CAT_PROJECT_COPY.noGit }
  }
  if (msg === CAT_PROJECT_COPY.cloneAuth || /仓库需要登录/.test(msg)) {
    return { place: 'banner', text: CAT_PROJECT_COPY.cloneAuth }
  }
  if (err.reason === 'clone_busy' || msg === CAT_PROJECT_COPY.cloneBusy) {
    return { place: 'banner', text: CAT_PROJECT_COPY.cloneBusy }
  }
  if (msg.startsWith(CAT_PROJECT_COPY.cloneFailPrefix)) {
    return { place: 'banner', text: msg.slice(0, 120 + CAT_PROJECT_COPY.cloneFailPrefix.length) }
  }
  if (msg) return { place: 'banner', text: msg.startsWith(t('cat.misc.s_6dc5b37546')) ? msg : CAT_PROJECT_COPY.cloneFailPrefix + msg.slice(0, 120) }
  return { place: 'banner', text: projectErrorText(e) }
}

/** 从 URL 取默认仓库名（去 .git） */
export function repoNameFromUrl(url: string): string {
  let u = url.trim().replace(/[\\/]+$/, '')
  // git@host:org/repo.git
  const ssh = u.match(/^[^@]+@[^:]+:(.+)$/)
  if (ssh) u = ssh[1]
  else {
    try {
      const path = u.includes('://') ? new URL(u).pathname : u
      u = path
    } catch { /* keep */ }
  }
  const base = u.split('/').filter(Boolean).pop() || ''
  return base.replace(/\.git$/i, '') || 'repo'
}

/** 粗校验：空 / 明显非 git URL（与契约 6.19.10.11 对齐的前端预检） */
export function looksLikeGitUrl(url: string): boolean {
  const u = url.trim()
  if (!u) return false
  if (/^https?:\/\//i.test(u)) return true
  if (/^git@/i.test(u)) return true
  if (/^ssh:\/\//i.test(u)) return true
  if (/\.git$/i.test(u)) return true
  return false
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
  if (catProjectsSim()) return sim.picks.length ? (sim.picks.shift() as string) : t('cat.misc.s_8ada02e15b', { v0: ++sim.pickN })
  if (!catProjectsLive()) off()
  return await pickDirectory(relocate ? CAT_PROJECT_COPY.relocateTitle : CAT_PROJECT_COPY.pickTitle)
}

export async function listCatProjects(): Promise<CatProject[]> {
  if (catProjectsLive()) {
    const raw: unknown = await call(CatBinding.ListCatProjects())
    return Array.isArray(raw) ? raw.map(mapProject) : []
  }
  if (catProjectsSim()) return sim.list.map(simCopy)
  return []
}

export async function createCatProject(req: CreateCatProjectRequest): Promise<CreateCatProjectResult> {
  if (catProjectsLive()) {
    const body = cat.CreateProjectRequest.createFrom({ path: req.path })
    // name 省略 = 用文件夹名（契约 6.19.10.2）；只有用户给了名字才带
    if (req.name?.trim()) body.name = req.name
    return mapResult(await call(CatBinding.CreateCatProject(body)))
  }
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
  if (catProjectsLive()) return mapProject(await call(CatBinding.RenameCatProject(cat.RenameProjectRequest.createFrom({ id: req.id, name: req.name }))))
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
    await call(CatBinding.DeleteCatProject(cat.DeleteProjectRequest.createFrom({ id: req.id })))
    return
  }
  if (!catProjectsSim()) off()
  sim.list = sim.list.filter((p) => p.id !== req.id)
}

/** 在系统文件管理器里显示项目文件夹（文件夹不见了时菜单项禁用，不调用） */
export async function revealCatProject(req: RevealCatProjectRequest): Promise<void> {
  if (catProjectsLive()) {
    await call(CatBinding.RevealCatProject(cat.RevealProjectRequest.createFrom({ id: req.id })))
    return
  }
  if (!catProjectsSim()) off()
  const p = sim.list.find((x) => x.id === req.id)
  if (!p) throw new AppError('NOT_FOUND', CAT_PROJECT_COPY.notFound)
  if (simMissing(p)) throw new AppError('CAT_PROJECT_MISSING', CAT_PROJECT_COPY.missing)
  console.info(t('cat.misc.s_6f884def1c'), p.path)
}

/** 重新选择文件夹（6.19.10.2 第 8 条）：对话保留、名字不变；重复 → INVALID_ARGUMENT reason=project_duplicate（第二行 projectId=） */
export async function relocateCatProject(req: RelocateCatProjectRequest): Promise<CatProject> {
  if (catProjectsLive()) return mapProject(await call(CatBinding.RelocateCatProject(cat.RelocateProjectRequest.createFrom({ id: req.id, path: req.path }))))
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

// ---------- v0.40 克隆 ----------

const cloneSim = {
  active: '' as string,
  timers: [] as ReturnType<typeof setTimeout>[],
}

function clearCloneTimers() {
  for (const t of cloneSim.timers) clearTimeout(t)
  cloneSim.timers = []
}

function emitClone(e: CatCloneEvent) {
  emitSimEvent(CAT_CLONE_EVENT, e)
}

function mapCloneResult(raw: unknown): CloneCatProjectResult {
  const r = (raw ?? {}) as { cloneId?: unknown; status?: unknown; project?: unknown }
  const status = (['starting', 'cloning', 'creating', 'succeeded', 'failed', 'canceled'] as const).find((s) => s === r.status) ?? 'starting'
  return {
    cloneId: String(r.cloneId ?? ''),
    status,
    project: r.project ? mapProject(r.project) : undefined,
  }
}

function mapCloneEvent(raw: unknown): CatCloneEvent | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Record<string, unknown>
  const status = (['starting', 'cloning', 'creating', 'succeeded', 'failed', 'canceled'] as const).find((s) => s === r.status)
  if (!status || typeof r.cloneId !== 'string' || !r.cloneId) return null
  const err = r.error as { code?: unknown; message?: unknown; detail?: unknown } | null | undefined
  return {
    cloneId: r.cloneId,
    status,
    url: typeof r.url === 'string' ? r.url : undefined,
    parentDir: typeof r.parentDir === 'string' ? r.parentDir : undefined,
    name: typeof r.name === 'string' ? r.name : undefined,
    progress: typeof r.progress === 'number' && Number.isFinite(r.progress) ? r.progress : undefined,
    project: r.project ? mapProject(r.project) : undefined,
    error: err && typeof err === 'object'
      ? { code: typeof err.code === 'string' ? err.code : undefined, message: typeof err.message === 'string' ? err.message : undefined, detail: typeof err.detail === 'string' ? err.detail : undefined }
      : null,
    canceled: r.canceled === true || status === 'canceled',
  }
}

/** 选克隆保存位置；取消返回 "" */
export async function pickCloneParentDir(): Promise<string> {
  if (catProjectsSim()) return sim.picks.length ? (sim.picks.shift() as string) : `D:\\Projects`
  if (!catProjectsLive()) off()
  return await pickDirectory(CAT_PROJECT_COPY.clonePickParentTitle)
}

export async function cloneCatProject(req: CloneCatProjectRequest): Promise<CloneCatProjectResult> {
  if (catProjectsLive()) {
    const body = cat.CloneCatProjectRequest.createFrom({ url: req.url })
    if (req.parentDir?.trim()) body.parentDir = req.parentDir.trim()
    if (req.name?.trim()) body.name = req.name.trim()
    return mapCloneResult(await call(CatBinding.CloneCatProject(body)))
  }
  if (!catProjectsSim()) off()
  const url = req.url.trim()
  if (!looksLikeGitUrl(url)) throw new AppError('INVALID_ARGUMENT', CAT_PROJECT_COPY.badGitUrl, 'reason=git_url')
  if (cloneSim.active) throw new AppError('TASK_CONFLICT', CAT_PROJECT_COPY.cloneBusy, 'reason=clone_busy')
  const cloneId = `sim-clone-${Date.now().toString(36)}`
  cloneSim.active = cloneId
  clearCloneTimers()
  const name = (req.name?.trim() || repoNameFromUrl(url))
  const parent = (req.parentDir?.trim() || 'D:\\Projects')
  const path = `${parent.replace(/[\\/]+$/, '')}\\${name}`
  emitClone({ cloneId, status: 'starting', url, parentDir: parent, name })
  cloneSim.timers.push(setTimeout(() => {
    if (cloneSim.active !== cloneId) return
    emitClone({ cloneId, status: 'cloning', url, parentDir: parent, name, progress: 0.2 })
  }, 200))
  cloneSim.timers.push(setTimeout(() => {
    if (cloneSim.active !== cloneId) return
    emitClone({ cloneId, status: 'cloning', url, parentDir: parent, name, progress: 0.6 })
  }, 700))
  cloneSim.timers.push(setTimeout(() => {
    if (cloneSim.active !== cloneId) return
    emitClone({ cloneId, status: 'creating', url, parentDir: parent, name, progress: 0.95 })
  }, 1100))
  cloneSim.timers.push(setTimeout(() => {
    if (cloneSim.active !== cloneId) return
    void (async () => {
      try {
        const r = await createCatProject({ path, name })
        cloneSim.active = ''
        emitClone({ cloneId, status: 'succeeded', url, parentDir: parent, name, progress: 1, project: r.project })
      } catch (e) {
        cloneSim.active = ''
        const err = toAppError(e)
        emitClone({ cloneId, status: 'failed', url, parentDir: parent, name, error: { code: err.code, message: projectErrorText(e), detail: err.detail } })
      }
    })()
  }, 1400))
  return { cloneId, status: 'starting' }
}

export async function cancelCloneCatProject(req: CancelCloneCatProjectRequest): Promise<void> {
  if (catProjectsLive()) {
    await call(CatBinding.CancelCloneCatProject(cat.CancelCloneCatProjectRequest.createFrom({ cloneId: req.cloneId })))
    return
  }
  if (!catProjectsSim()) off()
  if (cloneSim.active && cloneSim.active === req.cloneId) {
    clearCloneTimers()
    cloneSim.active = ''
    emitClone({ cloneId: req.cloneId, status: 'canceled', canceled: true })
  }
}

export function onCatClone(cb: (e: CatCloneEvent) => void): () => void {
  if (catProjectsLive()) {
    return onEvent<unknown>(CAT_CLONE_EVENT, (raw) => {
      const e = mapCloneEvent(raw)
      if (e) cb(e)
    })
  }
  if (catProjectsSim()) {
    return onEvent<unknown>(CAT_CLONE_EVENT, (raw) => {
      const e = mapCloneEvent(raw)
      if (e) cb(e)
    })
  }
  return () => {}
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
