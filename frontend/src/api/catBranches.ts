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
  detached: '未在分支上',
  detachedTip: '当前签出的是一个提交，不在任何分支上',
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
const devSim = () => import.meta.env.DEV && typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go && (catSim.has('branchbar') || catSim.has('branchlong') || catSim.has('branchfail') || catSim.has('detached'))
let simBranch = 'feat/cat-footer-branch-v049-with-a-really-long-name'
function simList(): CatBranches {
  return mapCatBranches({
    root: catSim.has('branchlong') ? 'D:\\Work\\2026\\客户交付\\婚礼剪辑\\第二季\\字幕项目' : 'D:\\Work\\字幕项目',
    git: true,
    repo: true,
    branch: catSim.has('detached') ? '' : simBranch,
    detached: catSim.has('detached'),
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
