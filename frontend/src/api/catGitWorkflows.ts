import { callService } from '@/api/call'
import { mapGitDiff, type GitDiff, type GitStatus } from '@/api/catGit'
import { t } from '@/i18n'
const workbenchKey = 'cat.gitWorkbench.'

export type GitHistoryScope = 'all' | 'current' | 'outgoing' | 'incoming'
export type GitWorkbenchTab = 'changes' | 'history' | 'branches' | 'stashes'
export interface GitStash { sha: string; ref: string; message: string; time: number }

export function gitSyncState(st: GitStatus | null) {
  if (!st?.repo) return 'none'
  if (st.detached) return 'detached'
  if (!st.upstream) return 'unpublished'
  if (st.ahead > 0 && st.behind > 0) return 'diverged'
  if (st.ahead > 0) return 'outgoing'
  if (st.behind > 0) return 'incoming'
  return 'synced'
}

export function gitPrimaryRemote(st: GitStatus | null): { op: string; label: string; disabled: boolean; reason: string } {
  const state = gitSyncState(st)
  const reason = !st?.repo ? t('cat.git.notRepo') : st.detached ? t('cat.gitWorkbench.detachedHint') :
    st.mergeState !== 'none' || st.conflicts.length > 0 ? t('cat.gitWorkbench.resolveFirst') : !st.remote ? t('cat.gitWorkbench.noRemote') : !st.headSha ? t('cat.gitWorkbench.firstCommit') : ''
  const op = state === 'unpublished' ? 'publish' : state === 'outgoing' ? 'push' : state === 'incoming' ? 'ff' : state === 'diverged' ? 'sync' : 'fetch'
  const label = t(workbenchKey + (op === 'publish' ? 'publish' : op === 'push' ? 'push' : op === 'ff' ? 'pull' : op === 'sync' ? 'sync' : 'fetch'))
  return { op, label, disabled: !!reason, reason }
}

export function mapGitStashes(raw: unknown): GitStash[] {
  if (!Array.isArray(raw)) return []
  return raw.filter(v => v && typeof v === 'object').map(v => ({
    sha: typeof v.sha === 'string' ? v.sha : '', ref: typeof v.ref === 'string' ? v.ref : '',
    message: typeof v.message === 'string' ? v.message : '', time: typeof v.time === 'number' ? v.time : 0,
  })).filter(v => v.sha && v.ref)
}

export async function listCatStashes(convId: string): Promise<GitStash[]> {
  return mapGitStashes(await callService('CatService', 'ListCatStashes', { convId }))
}
export async function gitCatStash(convId: string, op: 'save' | 'apply' | 'pop' | 'drop', opts: { sha?: string; message?: string; includeUntracked?: boolean } = {}) {
  await callService('CatService', 'GitCatStash', { convId, op, ...opts })
}
export async function getCatStashDiff(convId: string, sha: string): Promise<GitDiff> {
  return mapGitDiff(await callService('CatService', 'GetCatStashDiff', { convId, sha }))
}
export async function gitCatUndoCommit(convId: string, sha: string): Promise<string> {
  const r = await callService<{ message?: string }>('CatService', 'GitCatUndoCommit', { convId, sha })
  return typeof r?.message === 'string' ? r.message : ''
}
export async function gitCatDiscard(convId: string, paths: string[]) {
  await callService('CatService', 'GitCatDiscard', { convId, paths })
}
export async function gitCatAddRemote(convId: string, name: string, url: string) {
  await callService('CatService', 'GitCatAddRemote', { convId, name, url })
}
export async function gitCatInit(convId: string) {
  await callService('CatService', 'GitCatInit', { convId })
}
export async function gitCatConflictSkip(convId: string) {
  await callService('CatService', 'GitCatConflictSkip', { convId })
}
export async function gitCatCommitAction(convId: string, sha: string, headSha: string, op: 'cherry-pick' | 'revert') {
  await callService('CatService', 'GitCatCommitAction', { convId, sha, headSha, op })
}
