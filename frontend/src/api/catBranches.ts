/**
 * Cat 输入框底栏：对话文件夹 + 远程分支 + 本地分支切换（契约 v0.49 §6.19.24）。
 * 打开文件夹复用 RevealCatConversationFolder（项目对话打开项目文件夹，普通对话打开对话自己的文件夹）。
 */
import { callService, toAppError } from '@/api/call'
import type { CatSelectOption } from '@/components/cat/panels/CatModeSelect.vue'

export const CAT_BRANCH_COPY = {
  noUpstream: '无远程分支',
  upstreamTitle: '远程分支（上游）',
  localTitle: '本地分支，点击切换',
  detached: '分离 HEAD',
  remoteSection: '远程分支',
  openFolder: '在文件管理器中打开',
  dirty: '有未提交的改动，先提交或暂存再切换分支。',
  running: '正在回复，等这一轮结束再切换分支。',
  notFound: '找不到这个分支。',
  switched: (b: string) => `已切换到 ${b}`,
  switchFailed: '切换分支失败，请重试。',
  openFailed: '打不开文件夹，请重试。',
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

export async function listCatBranches(convId: string): Promise<CatBranches> {
  return mapCatBranches(await callService<unknown>('CatService', 'ListCatBranches', { convId }))
}

export async function switchCatBranch(convId: string, branch: string, remote: boolean): Promise<void> {
  await callService('CatService', 'SwitchCatBranch', { convId, branch, remote })
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
    out.push({ id: 'H:remote', label: CAT_BRANCH_COPY.remoteSection, disabled: true, divided: out.length > 0 })
    for (const n of b.remotes) out.push({ id: 'R:' + n, label: n })
  }
  return out
}

export function parseBranchOption(id: string): { branch: string; remote: boolean } | null {
  if (id.startsWith('L:')) return { branch: id.slice(2), remote: false }
  if (id.startsWith('R:')) return { branch: id.slice(2), remote: true }
  return null
}

export function catBranchErrorText(e: unknown): string {
  const err = toAppError(e)
  if (err.code === 'TASK_CONFLICT' && err.reason === 'git_dirty') return CAT_BRANCH_COPY.dirty
  if (err.code === 'TASK_CONFLICT' && err.reason === 'turn_running') return CAT_BRANCH_COPY.running
  if (err.code === 'NOT_FOUND' && err.reason === 'branch_not_found') return CAT_BRANCH_COPY.notFound
  if (err.code === 'CAT_PROJECT_MISSING') return '项目文件夹不见了。'
  return err.message || CAT_BRANCH_COPY.switchFailed
}
