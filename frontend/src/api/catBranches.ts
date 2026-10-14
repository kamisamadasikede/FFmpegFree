/**
 * Cat 输入框底栏：对话文件夹 + 远程分支 + 本地分支切换（契约 v0.49 §6.19.24）。
 * 打开文件夹复用 RevealCatConversationFolder（项目对话打开项目文件夹，普通对话打开对话自己的文件夹）。
 */
import { AppError, callService, toAppError } from '@/api/call'
import { catSim } from '@/views/cat/catDevSim'
import type { CatSelectOption } from '@/components/cat/panels/CatModeSelect.vue'

export const CAT_BRANCH_COPY = {
  noUpstream: '没有远程分支',
  upstreamTitle: '远程分支（上游）',
  localTitle: '本地分支，点击切换',
  detached: '分离 HEAD',
  remoteSection: '远程分支',
  openFolder: '在文件管理器中打开',
  dirty: '有未提交的改动，请先提交或暂存，再切换分支。',
  running: '正在回复，等这一轮结束再切换分支。',
  notFound: '找不到这个分支，可能已被删除。',
  switched: (b: string) => `已切换到 ${b}`,
  switchFailed: '切换分支失败，请重试。',
  detail: '查看详情',
  openFailed: '打不开这个文件夹，可能已被移动或删除。',
} as const

export interface CatBranches {
  root: string
  git: boolean
  repo: boolean
  branch: string
  detached: boolean
  upstream: string
  locals: string[]
  remotes: string[]
}

const strs = (v: unknown): string[] => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string' && x !== '') : [])
const str = (v: unknown): string => (typeof v === 'string' ? v : '')

export function mapCatBranches(raw: unknown): CatBranches {
  const r = (typeof raw === 'object' && raw !== null ? raw : {}) as Record<string, unknown>
  const git = r.git === true
  const repo = git && r.repo === true
  return {
    root: str(r.root),
    git,
    repo,
    branch: repo ? str(r.branch) : '',
    detached: repo && r.detached === true,
    upstream: repo ? str(r.upstream) : '',
    locals: repo ? strs(r.locals) : [],
    remotes: repo ? strs(r.remotes) : [],
  }
}

/** 仅 vite dev + ?cat_sim=branchbar|branchlong|branchfail 的纯浏览器走查；正式包 DEV=false 整段不走 */
const devSim = () => import.meta.env.DEV && typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go && (catSim.has('branchbar') || catSim.has('branchlong') || catSim.has('branchfail') || catSim.has('mergeok') || catSim.has('mergeuptodate') || catSim.has('mergeconflict') || catSim.has('mergedirty') || catSim.has('mergefail'))
let simBranch = 'feat/cat-footer-branch-v049-with-a-really-long-name'
function simList(): CatBranches {
  return mapCatBranches({
    root: catSim.has('branchlong') ? 'D:\\Work\\2026\\客户交付\\婚礼剪辑\\第二季\\字幕项目' : 'D:\\Work\\字幕项目',
    git: true,
    repo: true,
    branch: simBranch,
    upstream: 'origin/feat/cat-footer-branch-v049-with-a-really-long-name',
    locals: ['main', 'v2', 'dev', 'feat/cat-footer-branch-v049-with-a-really-long-name', 'fix/reply-marked-failed', 'fe/settings-dropdown'],
    remotes: ['origin/feat/system-notify', 'origin/feat/context-left', 'origin/old/archive'],
  })
}

export async function listCatBranches(convId: string): Promise<CatBranches> {
  if (devSim()) return simList()
  return mapCatBranches(await callService<unknown>('CatService', 'ListCatBranches', { convId }))
}

export async function switchCatBranch(convId: string, branch: string, remote: boolean): Promise<void> {
  if (devSim()) {
    await new Promise((r) => setTimeout(r, 300))
    if (catSim.has('branchfail')) throw new AppError('PROCESS_FAILED', 'git checkout 失败', "fatal: unable to write new index file\nerror: unable to create file src/main.ts: Permission denied")
    simBranch = remote ? branch.replace(/^[^/]+\//, '') : branch
    return
  }
  await callService('CatService', 'SwitchCatBranch', { convId, branch, remote })
}

/**
 * 底栏长路径（设计规格）：中间省略，只留开头盘符 / 第一级目录和最后一级文件夹名，
 * 如 `D:\…\字幕项目`、`/home/…/字幕项目`；够短（≤3 段）原样返回。完整路径放 tooltip。
 */
export function pathEllipsis(path: string): string {
  const s = (path ?? '').replace(/[\\/]+$/, '')
  const sep = s.includes('\\') ? '\\' : '/'
  const parts = s.split(/[\\/]+/)
  // 绝对 unix 路径 parts[0] 为空：留第一级目录
  const head = parts[0] === '' ? sep + (parts[1] ?? '') : parts[0]
  const rest = parts[0] === '' ? parts.slice(2) : parts.slice(1)
  if (rest.length <= 2) return s
  return head + sep + '…' + sep + rest[rest.length - 1]
}

/** 长路径中间省略：保留开头盘符/前几段和结尾，完整路径放 tooltip */
export function middleEllipsis(path: string, max = 36): string {
  const s = path ?? ''
  if (s.length <= max) return s
  const keep = max - 1
  const tail = Math.ceil(keep * 0.6)
  const head = keep - tail
  return s.slice(0, head) + '…' + s.slice(s.length - tail)
}

/** 分支下拉选项：本地在上；只在远程的放在「远程分支」小标题下。id 用 L:/R: 前缀区分 */
export function branchOptions(b: CatBranches): CatSelectOption[] {
  const out: CatSelectOption[] = b.locals.map((n) => ({ id: 'L:' + n, label: n }))
  if (b.remotes.length) {
    out.push({ id: 'H:remote', label: CAT_BRANCH_COPY.remoteSection, disabled: true, header: true, divided: out.length > 0 })
    for (const n of b.remotes) out.push({ id: 'R:' + n, label: n })
  }
  return out
}

export function parseBranchOption(id: string): { branch: string; remote: boolean } | null {
  if (id.startsWith('L:')) return { branch: id.slice(2), remote: false }
  if (id.startsWith('R:')) return { branch: id.slice(2), remote: true }
  return null
}

/** 切换失败：已知原因给对应文案（raw 空）；其余统一「切换分支失败，请重试。」，原始报错放「查看详情」 */
export function catBranchError(e: unknown): { text: string; raw: string } {
  const err = toAppError(e)
  if (err.code === 'TASK_CONFLICT' && err.reason === 'git_dirty') return { text: CAT_BRANCH_COPY.dirty, raw: '' }
  if (err.code === 'TASK_CONFLICT' && err.reason === 'turn_running') return { text: CAT_BRANCH_COPY.running, raw: '' }
  if (err.code === 'NOT_FOUND' && err.reason === 'branch_not_found') return { text: CAT_BRANCH_COPY.notFound, raw: '' }
  if (err.code === 'CAT_PROJECT_MISSING') return { text: '项目文件夹不见了。', raw: '' }
  // 兜底：git 报 would be overwritten 也算未提交改动（后端也在归类）
  if (/would be overwritten/i.test(`${err.message ?? ''}\n${err.detail ?? ''}`)) return { text: CAT_BRANCH_COPY.dirty, raw: '' }
  const raw = [err.message, err.detail].filter((x) => !!x && x.trim()).join('\n')
  return { text: CAT_BRANCH_COPY.switchFailed, raw }
}

export function catBranchErrorText(e: unknown): string {
  const err = toAppError(e)
  if (err.code === 'TASK_CONFLICT' && err.reason === 'git_dirty') return CAT_BRANCH_COPY.dirty
  if (err.code === 'TASK_CONFLICT' && err.reason === 'turn_running') return CAT_BRANCH_COPY.running
  if (err.code === 'NOT_FOUND' && err.reason === 'branch_not_found') return CAT_BRANCH_COPY.notFound
  return catBranchError(e).text
}

// ---------------------------------------------------------------------------
// v0.50 合并到当前分支（GitCatMerge(convId, branch)）。
// 契约 6.19.25；后端绑定未合入前 callService 按名字找不到会抛 UNSUPPORTED（走「合并失败」）。
// ---------------------------------------------------------------------------

export const CAT_MERGE_COPY = {
  entry: '合并到当前分支…',
  title: (cur: string) => `把哪个分支合并到 ${cur}？`,
  localGroup: '本地分支',
  remoteGroup: '远程分支',
  ok: '合并',
  cancel: '取消',
  merged: (b: string, cur: string) => `已把 ${b} 合并到 ${cur}`,
  upToDate: '已经是最新，没有需要合并的内容。',
  conflict: (n: number) => `有 ${n} 个文件冲突，请逐个处理。`,
  dirty: '有未提交的改动，请先提交或暂存，再合并。',
  running: '正在回复，等这一轮结束再合并。',
  merging: '上一次合并还没完成，请先处理完冲突，或放弃这次合并。',
  notRepo: '这个文件夹不是 Git 仓库。',
  notFound: '找不到这个分支，可能已被删除。',
  sameBranch: '不能把分支合并到它自己。',
  failed: '合并失败，请重试。',
  detail: '查看详情',
} as const

export type CatMergeStatus = 'merged' | 'up_to_date' | 'conflict'
export interface CatMergeResult {
  status: CatMergeStatus
  /** 冲突文件数 = files.length（status=conflict） */
  conflicts: number
  /** conflict：未合并路径（相对对话根，排序） */
  files: string[]
  /** merged / up_to_date：合并后 HEAD sha */
  head: string
}

/** 契约 6.19.25 GitCatMergeResult：{ status: 'merged'|'up_to_date'|'conflict', head?, files?: string[] }；n = files.length */
export function mapCatMergeResult(raw: unknown): CatMergeResult {
  const r = (typeof raw === 'object' && raw !== null ? raw : {}) as Record<string, unknown>
  const files = strs(r.files)
  const s = str(r.status)
  if (s === 'conflict') return { status: 'conflict', conflicts: files.length, files, head: '' }
  if (s === 'up_to_date') return { status: 'up_to_date', conflicts: 0, files: [], head: str(r.head) }
  return { status: 'merged', conflicts: 0, files: [], head: str(r.head) }
}

export type CatMergeBlock = 'dirty' | 'running' | 'merging' | 'notRepo' | 'notFound' | 'sameBranch'
export interface CatMergeError {
  kind: CatMergeBlock | 'failed'
  text: string
  /** 只有 failed 带原始报错，进「查看详情」 */
  raw: string
}

const MERGE_BLOCK_TEXT: Record<CatMergeBlock, string> = { dirty: CAT_MERGE_COPY.dirty, running: CAT_MERGE_COPY.running, merging: CAT_MERGE_COPY.merging, notRepo: CAT_MERGE_COPY.notRepo, notFound: CAT_MERGE_COPY.notFound, sameBranch: CAT_MERGE_COPY.sameBranch }
export function catMergeBlock(kind: CatMergeBlock): CatMergeError {
  return { kind, text: MERGE_BLOCK_TEXT[kind], raw: '' }
}

/**
 * 契约 6.19.25 + 6.19.25.1 错误（2.2）：TASK_CONFLICT reason=git_dirty / turn_running / merge_in_progress 给对应文案、不带详情；
 * NOT_FOUND branch_not_found / 非仓库 / 同一分支给产品文案（不带详情）；其余（INVALID_ARGUMENT git_failed 等）「合并失败，请重试。」+ 详情。
 * 前端兜底：原文含 would be overwritten 也归 git_dirty（后端同样归类）。
 */
export function catMergeError(e: unknown): CatMergeError {
  const err = toAppError(e)
  if (err.code === 'TASK_CONFLICT' && err.reason === 'git_dirty') return catMergeBlock('dirty')
  if (err.code === 'TASK_CONFLICT' && err.reason === 'turn_running') return catMergeBlock('running')
  if (err.code === 'TASK_CONFLICT' && err.reason === 'merge_in_progress') return catMergeBlock('merging')
  if (err.code === 'NOT_FOUND' && err.reason === 'branch_not_found') return catMergeBlock('notFound')
  // 6.19.25.1：not_git_repo / same_branch（INVALID_ARGUMENT），不带详情；前端下拉也已排除当前分支
  if (err.code === 'INVALID_ARGUMENT' && err.reason === 'not_git_repo') return catMergeBlock('notRepo')
  if (err.code === 'INVALID_ARGUMENT' && err.reason === 'same_branch') return catMergeBlock('sameBranch')
  if (/would be overwritten/i.test(`${err.message ?? ''}\n${err.detail ?? ''}`)) return catMergeBlock('dirty')
  // detail 首行是 reason=…，第二行起是 git 原始 stderr
  const detail = (err.detail ?? '').replace(/^reason=[A-Za-z0-9_-]+[^\n]*\n?/, '')
  const raw = [err.message, detail].filter((x) => !!x && x.trim()).join('\n')
  return { kind: 'failed', text: CAT_MERGE_COPY.failed, raw }
}

/** 弹窗里可选的分支：本地 / 远程两组，当前分支不出现。id 用 L:/R: 前缀 */
export function mergeOptions(b: CatBranches): CatSelectOption[] {
  const locals = b.locals.filter((n) => n !== b.branch)
  const remotes = b.remotes
  const out: CatSelectOption[] = []
  if (locals.length) {
    out.push({ id: 'H:local', label: CAT_MERGE_COPY.localGroup, disabled: true, header: true })
    for (const n of locals) out.push({ id: 'L:' + n, label: n })
  }
  if (remotes.length) {
    out.push({ id: 'H:remote', label: CAT_MERGE_COPY.remoteGroup, disabled: true, header: true, divided: out.length > 0 })
    for (const n of remotes) out.push({ id: 'R:' + n, label: n })
  }
  return out
}

/** 默认选中第一个本地分支；没有本地就第一个远程 */
export function defaultMergeOption(opts: CatSelectOption[]): string {
  return (opts.find((o) => o.id.startsWith('L:')) ?? opts.find((o) => o.id.startsWith('R:')))?.id ?? ''
}

/** 底栏下拉最底下的入口项 id */
export const MERGE_ENTRY_ID = 'M:merge'
export function mergeEntryOption(b: CatBranches): CatSelectOption {
  return { id: MERGE_ENTRY_ID, label: CAT_MERGE_COPY.entry, icon: 'merge', divided: true, disabled: mergeOptions(b).length === 0 }
}

const mergeSim = () =>
  import.meta.env.DEV && typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go &&
  (catSim.has('mergeok') || catSim.has('mergeuptodate') || catSim.has('mergeconflict') || catSim.has('mergedirty') || catSim.has('mergefail'))
/** 走查：?cat_sim=mergeok/... 同时给底栏一份分支（等同 branchbar） */
export const catMergeSimOn = (): boolean => !!mergeSim()

export async function mergeCatBranch(convId: string, branch: string, remote = false): Promise<CatMergeResult> {
  if (mergeSim()) {
    await new Promise((r) => setTimeout(r, catSim.has('mergeok') ? 1200 : 600))
    if (catSim.has('mergedirty')) throw new AppError('TASK_CONFLICT', 'git merge 被拦下', 'reason=git_dirty')
    if (catSim.has('mergefail')) throw new AppError('INVALID_ARGUMENT', 'git merge 失败', `reason=git_failed\nfatal: refusing to merge unrelated histories\nerror: could not merge ${branch}`)
    if (catSim.has('mergeconflict')) return mapCatMergeResult({ status: 'conflict', files: ['第1集/第1集.srt', '第2集/片头.ass', '第3集/第3集.srt'] })
    if (catSim.has('mergeuptodate')) return mapCatMergeResult({ status: 'up_to_date' })
    return mapCatMergeResult({ status: 'merged' })
  }
  return mapCatMergeResult(await callService<unknown>('CatService', 'GitCatMerge', { convId, branch, remote }))
}
