/**
 * Cat 右侧源代码管理。状态和改动来自本机 git，路径都相对当前对话的文件夹。
 */
import { callService, toAppError } from '@/api/call'
import { catCallError } from '@/api/catFiles'

export const CAT_GIT_COPY = {
  pr: '创建 PR',
  message: '信息',
  stageAll: '+ 暂存全部',
  commit: '提交',
  commitPush: '提交并推送',
  commitSync: '提交并同步',
  push: '推送',
  force: '强制推送',
  newBranch: '新建分支',
  pushPr: '推送并创建 PR',
  pull: '拉取',
  ff: '仅快进',
  sync: '同步',
  rebase: '变基到',
  fetch: '获取',
  publish: '发布分支',
  noUpstream: '还没有上游分支。',
  needMessage: '请填写提交说明。',
  hasUpstream: '已经有上游分支。',
  forceAsk: '强制推送会用本地提交盖过远程上的同名分支。',
  branchAsk: '新分支名称',
  changes: '更改',
  staged: '已暂存的更改',
  untracked: '未跟踪文件',
  notRepo: '这里还不是 git 仓库。',
  empty: '没有改动。',
  stage: '暂存',
  unstage: '取消暂存',
  openRemote: '在浏览器打开',
  refresh: '刷新',
  files: '文件',
  git: '源代码管理',
  notGitHub: '这个远程地址不是 GitHub，打不开创建页面。',
  preview: '预览里不能改仓库。',
  filter: '查找文件',
  diffBinary: '这个文件看不出文字区别。',
  diffLoading: '正在对比…',
} as const

export interface GitChange {
  path: string
  name: string
  dir: string
  letter: string
  added: number
  deleted: number
  staged: boolean
  unstaged: boolean
}

export type GitMergeState = 'none' | 'merging' | 'rebasing' | 'cherry'

export interface GitConflictEntry {
  path: string
  ours: boolean
  theirs: boolean
  letter: string
}

export interface GitStatus {
  repo: boolean
  branch: string
  upstream: string
  ahead: number
  behind: number
  remote: string
  web: string
  changes: GitChange[]
  /** v0.41：none | merging | rebasing | cherry */
  mergeState: GitMergeState
  /** v0.41：未合并路径；mergeState=none 时为空 */
  conflicts: GitConflictEntry[]
  /** v0.53：签出提交后 HEAD 游离；headSha / headShortSha 为当前提交（后端 PR 字段名） */
  detached: boolean
  headSha: string
  headShortSha: string
}

const RANK: Record<string, number> = { D: 5, M: 4, R: 3, A: 2, U: 1 }

function norm(p: string): string {
  return p.replace(/\\/g, '/').replace(/^\.\//, '').replace(/\/+$/, '')
}

/** 文件用自己的字母。文件夹用子孙里更“重”的那个：改过的盖过未跟踪。 */
export function gitLetterFor(rel: string, isDir: boolean, changes: { path: string; letter: string }[]): string {
  const want = norm(rel)
  if (!want) return ''
  let best = ''
  let score = 0
  for (const c of changes) {
    const p = norm(c.path)
    const hit = isDir ? p === want || p.startsWith(want + '/') : p === want
    if (!hit) continue
    const s = RANK[c.letter] ?? 0
    if (s > score) {
      score = s
      best = c.letter
    }
  }
  return best
}

/** 更改 = 还没暂存的跟踪文件；已暂存单独一组；未跟踪（U）再单独一组。同一文件两边都有改动时两边都出现。 */
export function gitChangeGroups(changes: GitChange[]): { work: GitChange[]; staged: GitChange[]; untracked: GitChange[] } {
  const work: GitChange[] = []
  const staged: GitChange[] = []
  const untracked: GitChange[] = []
  for (const c of changes) {
    if (c.letter === 'U') {
      untracked.push(c)
      continue
    }
    if (c.staged) staged.push(c)
    if (c.unstaged || !c.staged) work.push(c)
  }
  return { work, staged, untracked }
}

export function gitRootLabel(path: string): string {
  const s = path.replace(/[\\/]+$/, '')
  const i = Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\'))
  return i >= 0 ? s.slice(i + 1) : s
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

function num(v: unknown): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : 0
}

export function normalizeMergeState(v: unknown): GitMergeState {
  return v === 'merging' || v === 'rebasing' || v === 'cherry' ? v : 'none'
}

function mapConflict(raw: unknown): GitConflictEntry {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    path: str(r.path),
    ours: r.ours === true,
    theirs: r.theirs === true,
    letter: str(r.letter) || 'UU',
  }
}

export function mapGitStatus(raw: unknown): GitStatus {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const changes = Array.isArray(r.changes) ? r.changes.map(mapChange).filter((c) => c.path) : []
  const conflicts = Array.isArray(r.conflicts) ? r.conflicts.map(mapConflict).filter((c) => c.path) : []
  return {
    repo: r.repo === true,
    branch: str(r.branch),
    upstream: str(r.upstream),
    ahead: num(r.ahead),
    behind: num(r.behind),
    remote: str(r.remote),
    web: str(r.web),
    changes,
    mergeState: normalizeMergeState(r.mergeState),
    conflicts,
    detached: r.detached === true,
    headSha: str(r.headSha),
    headShortSha: str(r.headShortSha) || str(r.headSha).slice(0, 7),
  }
}

function mapChange(raw: unknown): GitChange {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return {
    path: str(r.path),
    name: str(r.name),
    dir: str(r.dir),
    letter: str(r.letter),
    added: num(r.added),
    deleted: num(r.deleted),
    staged: r.staged === true,
    unstaged: r.unstaged === true,
  }
}

export interface GitDiff {
  path: string
  text: string
  binary: boolean
}

export function mapGitDiff(raw: unknown): GitDiff {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  return { path: str(r.path), text: str(r.text), binary: r.binary === true }
}

export function catGitError(e: unknown): string {
  return catCallError(e, 'git 没有完成。')
}

export async function gitCatStatus(convId: string): Promise<GitStatus> {
  return mapGitStatus(await callService<unknown>('CatService', 'GitCatStatus', { convId }))
}

export async function gitCatStage(convId: string, paths: string[]): Promise<void> {
  await callService('CatService', 'GitCatStage', { convId, paths })
}

export async function gitCatUnstage(convId: string, paths: string[]): Promise<void> {
  await callService('CatService', 'GitCatUnstage', { convId, paths })
}

export async function gitCatCommit(convId: string, message: string): Promise<void> {
  await callService('CatService', 'GitCatCommit', { convId, message })
}

export async function gitCatRemote(convId: string, op: string): Promise<void> {
  await callService('CatService', 'GitCatRemote', { convId, op })
}

export async function gitCatBranch(convId: string, name: string): Promise<void> {
  await callService('CatService', 'GitCatBranch', { convId, name })
}

export type GitMenuEntry =
  | { kind: 'sep' }
  | { kind: 'item'; id: string; label: string; disabled: boolean; title: string; danger?: boolean }

/** 提交按钮下拉。没有上游时推送类不可用，发布分支可用。 */
export function gitMenuEntries(input: { message: string; changeCount: number; upstream: string; web: string }): GitMenuEntry[] {
  const msg = input.message.trim()
  const up = input.upstream.trim()
  const canCommit = !!msg && input.changeCount > 0
  const commitWhy = !msg ? CAT_GIT_COPY.needMessage : input.changeCount > 0 ? '' : CAT_GIT_COPY.empty
  const upWhy = up ? '' : CAT_GIT_COPY.noUpstream
  const webWhy = input.web ? '' : CAT_GIT_COPY.notGitHub
  const item = (id: string, label: string, disabled: boolean, title: string, danger?: boolean): GitMenuEntry =>
    ({ kind: 'item', id, label, disabled, title, danger })
  return [
    item('commit', CAT_GIT_COPY.commit, !canCommit, commitWhy),
    item('commit-push', CAT_GIT_COPY.commitPush, !canCommit || !up, !canCommit ? commitWhy : upWhy),
    item('commit-sync', CAT_GIT_COPY.commitSync, !canCommit || !up, !canCommit ? commitWhy : upWhy),
    { kind: 'sep' },
    item('push', CAT_GIT_COPY.push, !up, upWhy),
    item('force', CAT_GIT_COPY.force, !up, upWhy, true),
    item('pr', CAT_GIT_COPY.pr, !input.web, webWhy),
    item('branch', CAT_GIT_COPY.newBranch, false, ''),
    item('push-pr', CAT_GIT_COPY.pushPr, !input.web || !up, !input.web ? webWhy : upWhy),
    { kind: 'sep' },
    item('pull', CAT_GIT_COPY.pull, !up, upWhy),
    item('ff', CAT_GIT_COPY.ff, !up, upWhy),
    item('sync', CAT_GIT_COPY.sync, !up, upWhy),
    item('rebase', up ? `${CAT_GIT_COPY.rebase} ${up}` : `${CAT_GIT_COPY.rebase}上游`, !up, upWhy),
    item('fetch', CAT_GIT_COPY.fetch, false, ''),
    item('publish', CAT_GIT_COPY.publish, !!up, up ? CAT_GIT_COPY.hasUpstream : ''),
  ]
}

export async function gitCatDiff(convId: string, relPath: string): Promise<GitDiff> {
  return mapGitDiff(await callService<unknown>('CatService', 'GitCatDiff', { convId, relPath }))
}

export async function gitCatPullRequest(convId: string): Promise<string> {
  const raw = await callService<unknown>('CatService', 'GitCatPullRequest', { convId })
  const url = raw && typeof raw === 'object' ? (raw as { url?: unknown }).url : ''
  return typeof url === 'string' ? url : ''
}

export type GitConflictMode = 'ours' | 'theirs' | 'both' | 'content'

export interface GitBranchItem {
  name: string
  kind: 'local' | 'remote'
  current: boolean
}

export const CAT_GIT_CONFLICT_COPY = {
  group: '冲突',
  merging: '正在合并',
  rebasing: '正在变基',
  cherry: '正在拣选',
  mergeMeta: 'pull 遇冲突',
  rebaseMeta: '还有冲突待解决',
  cherryMeta: '还有冲突待解决',
  ours: '采用当前',
  theirs: '采用传入',
  both: '保留双方',
  resolved: '已解决',
  needResolve: (n: number) => `还有 ${n} 个文件未解决`,
  allClear: '全部冲突已解决，可以完成合并',
  allClearRebase: '全部冲突已解决，可以完成变基',
  allClearCherry: '全部冲突已解决，可以完成拣选',
  completeMerge: '完成合并',
  completeRebase: '完成变基',
  completeCherry: '完成拣选',
  abortMerge: '中止合并',
  abortRebase: '中止变基',
  abortCherry: '中止拣选',
  abortMergeTitle: '中止合并？',
  abortRebaseTitle: '中止变基？',
  abortCherryTitle: '中止拣选？',
  abortMergeBody: '未解决的冲突将丢弃，仓库回到合并前。',
  abortRebaseBody: '未解决的冲突将丢弃，仓库回到变基前。',
  abortCherryBody: '未解决的冲突将丢弃，仓库回到拣选前。',
  abortOk: '中止',
  abortCancel: '取消',
  messageLabel: '提交说明',
  defaultMergeMsg: (branch: string) => `Merge branch '${branch || '…'}'`,
  conflictNeed: '有冲突需要解决。',
  phaseNote: '一期简易：单文件采用当前 / 传入 / 双方；点开可手改。真三栏 Merge Editor 标二期。',
  dirtyTitle: '切换分支',
  dirtyBody: (branch: string) => `工作区有未提交更改。仍要切换到 ${branch}？`,
  dirtyOk: '仍要切换',
  dirtyCancel: '取消',
  branchFilter: '筛选分支…',
  localSec: '本地分支',
  remoteSec: '远端（只读示意）',
  notGit: '非 Git',
  checkout: '切换分支',
} as const

export function unresolvedConflicts(status: GitStatus | null | undefined): GitConflictEntry[] {
  if (!status || status.mergeState === 'none') return []
  return status.conflicts ?? []
}

export function conflictLabels(state: GitMergeState) {
  if (state === 'rebasing') {
    return {
      bar: CAT_GIT_CONFLICT_COPY.rebasing,
      meta: CAT_GIT_CONFLICT_COPY.rebaseMeta,
      complete: CAT_GIT_CONFLICT_COPY.completeRebase,
      abort: CAT_GIT_CONFLICT_COPY.abortRebase,
      abortTitle: CAT_GIT_CONFLICT_COPY.abortRebaseTitle,
      abortBody: CAT_GIT_CONFLICT_COPY.abortRebaseBody,
      allClear: CAT_GIT_CONFLICT_COPY.allClearRebase,
    }
  }
  if (state === 'cherry') {
    return {
      bar: CAT_GIT_CONFLICT_COPY.cherry,
      meta: CAT_GIT_CONFLICT_COPY.cherryMeta,
      complete: CAT_GIT_CONFLICT_COPY.completeCherry,
      abort: CAT_GIT_CONFLICT_COPY.abortCherry,
      abortTitle: CAT_GIT_CONFLICT_COPY.abortCherryTitle,
      abortBody: CAT_GIT_CONFLICT_COPY.abortCherryBody,
      allClear: CAT_GIT_CONFLICT_COPY.allClearCherry,
    }
  }
  return {
    bar: CAT_GIT_CONFLICT_COPY.merging,
    meta: CAT_GIT_CONFLICT_COPY.mergeMeta,
    complete: CAT_GIT_CONFLICT_COPY.completeMerge,
    abort: CAT_GIT_CONFLICT_COPY.abortMerge,
    abortTitle: CAT_GIT_CONFLICT_COPY.abortMergeTitle,
    abortBody: CAT_GIT_CONFLICT_COPY.abortMergeBody,
    allClear: CAT_GIT_CONFLICT_COPY.allClear,
  }
}

export function isGitConflictError(e: unknown): boolean {
  const err = toAppError(e)
  return err.code === 'INVALID_ARGUMENT' && err.reason === 'git_conflict'
}

export function isGitDirtyError(e: unknown): boolean {
  const err = toAppError(e)
  return err.code === 'TASK_CONFLICT' && err.reason === 'git_dirty'
}

export async function gitCatConflictResolve(
  convId: string,
  path: string,
  mode: GitConflictMode,
  content?: string,
): Promise<void> {
  await callService('CatService', 'GitCatConflictResolve', {
    convId,
    path,
    mode,
    ...(mode === 'content' ? { content: content ?? '' } : {}),
  })
}

export async function gitCatConflictComplete(convId: string, message?: string): Promise<void> {
  await callService('CatService', 'GitCatConflictComplete', { convId, message: message ?? '' })
}

export async function gitCatConflictAbort(convId: string): Promise<void> {
  await callService('CatService', 'GitCatConflictAbort', { convId })
}

export async function gitCatCheckout(convId: string, name: string, force = false): Promise<void> {
  await callService('CatService', 'GitCatCheckout', { convId, name, force })
}

/** 分支菜单：本地当前 ✓ + 可选额外本地；远端只读示意（upstream / remotes） */
export function gitBranchMenuItems(input: {
  branch: string
  upstream: string
  locals?: string[]
  remotes?: string[]
}): GitBranchItem[] {
  const cur = input.branch.trim()
  const seen = new Set<string>()
  const out: GitBranchItem[] = []
  const addLocal = (name: string, current: boolean) => {
    const n = name.trim()
    if (!n || seen.has('L:' + n)) return
    seen.add('L:' + n)
    out.push({ name: n, kind: 'local', current })
  }
  const addRemote = (name: string) => {
    const n = name.trim()
    if (!n || seen.has('R:' + n)) return
    seen.add('R:' + n)
    out.push({ name: n, kind: 'remote', current: false })
  }
  if (cur) addLocal(cur, true)
  for (const n of input.locals ?? []) addLocal(n, n === cur)
  const up = input.upstream.trim()
  if (up) {
    // upstream 常是 origin/main 形态 → 远端示意；纯本地名也放远端区旁注
    if (up.includes('/')) addRemote(up)
    else addRemote('origin/' + up)
  }
  for (const n of input.remotes ?? []) addRemote(n)
  return out
}

export function filterBranchItems(items: GitBranchItem[], q: string): GitBranchItem[] {
  const s = q.trim().toLowerCase()
  if (!s) return items
  return items.filter((i) => i.name.toLowerCase().includes(s))
}

/** 浏览器走查用分支列表（细视觉线框同款） */
export const SIM_GIT_BRANCHES = {
  locals: ['main', 'feat/srt-fix', 'fix/timeline-0.4'],
  remotes: ['origin/main'],
} as const
