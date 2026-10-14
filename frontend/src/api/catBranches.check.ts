import { AppError } from './call'
import { branchOptions, catBranchErrorText, mapCatBranches, middleEllipsis, parseBranchOption, CAT_BRANCH_COPY } from './catBranches'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catBranchesChecks(eq: Eq): void {
  eq('底栏分支：空返回', mapCatBranches(null), { root: '', git: false, repo: false, branch: '', detached: false, upstream: '', locals: [], remotes: [] })
  eq('底栏分支：没装 git 不给分支', mapCatBranches({ root: 'C:\\x', git: false, repo: true, branch: 'main' }).branch, '')
  const b = mapCatBranches({ root: '/p', git: true, repo: true, branch: 'main', upstream: 'origin/main', locals: ['dev', 'main'], remotes: ['origin/feat'] })
  eq('底栏分支：选项', branchOptions(b).map((o) => [o.id, o.label, !!o.disabled]), [
    ['L:dev', 'dev', false],
    ['L:main', 'main', false],
    ['H:remote', '远程分支', true],
    ['R:origin/feat', 'origin/feat', false],
  ])
  eq('底栏分支：解析', [parseBranchOption('L:a/b'), parseBranchOption('R:origin/x'), parseBranchOption('H:remote')], [{ branch: 'a/b', remote: false }, { branch: 'origin/x', remote: true }, null])
  const long = 'C:\\Users\\boss\\AppData\\Roaming\\FFmpegFree\\cat\\conversations\\0123456789abcdef'
  const m = middleEllipsis(long, 36)
  eq('路径中间省略', [m.length, m.includes('…'), m.startsWith('C:\\'), m.endsWith('0123456789abcdef'), middleEllipsis('/short')], [36, true, true, true, '/short'])
  eq('切分支错误文案', [
    catBranchErrorText(new AppError('TASK_CONFLICT', '', 'reason=git_dirty')),
    catBranchErrorText(new AppError('TASK_CONFLICT', 'x', 'reason=turn_running')),
    catBranchErrorText(new AppError('PROCESS_FAILED', '')),
  ], [CAT_BRANCH_COPY.dirty, CAT_BRANCH_COPY.running, CAT_BRANCH_COPY.switchFailed])
}
