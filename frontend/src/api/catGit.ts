/**
 * Cat 右侧源代码管理。状态和改动来自本机 git，路径都相对当前对话的文件夹。
 */
import { callService } from '@/api/call'
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

export interface GitStatus {
  repo: boolean
  branch: string
  upstream: string
  ahead: number
  behind: number
  remote: string
  web: string
  changes: GitChange[]
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

export function mapGitStatus(raw: unknown): GitStatus {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const changes = Array.isArray(r.changes) ? r.changes.map(mapChange).filter((c) => c.path) : []
  return {
    repo: r.repo === true,
    branch: str(r.branch),
    upstream: str(r.upstream),
    ahead: num(r.ahead),
    behind: num(r.behind),
    remote: str(r.remote),
    web: str(r.web),
    changes,
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
