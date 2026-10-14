import { AppError } from './call'
import { branchOptions, catBranchError, catBranchErrorText, pathEllipsis, mapCatBranches, middleEllipsis, parseBranchOption, CAT_BRANCH_COPY } from './catBranches'

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
  eq('文案（产品 #300 走查）', [CAT_BRANCH_COPY.noUpstream, CAT_BRANCH_COPY.dirty, CAT_BRANCH_COPY.notFound, CAT_BRANCH_COPY.openFailed], ['没有远程分支', '有未提交的改动，请先提交或暂存，再切换分支。', '找不到这个分支，可能已被删除。', '打不开这个文件夹，可能已被移动或删除。'])
  eq('切换失败：原始报错进查看详情', catBranchError(new AppError('PROCESS_FAILED', 'git checkout 失败', 'error: pathspec x')), { text: '切换分支失败，请重试。', raw: 'git checkout 失败\nerror: pathspec x' })
  eq('切换失败：would be overwritten 兜底归未提交改动', [
    catBranchError(new AppError('PROCESS_FAILED', 'git checkout 失败', 'error: Your local changes to the following files WOULD BE OVERWRITTEN by checkout:\n\ta.ts')),
    catBranchError(new AppError('PROCESS_FAILED', 'error: untracked working tree files would be overwritten by checkout')),
    catBranchErrorText(new AppError('PROCESS_FAILED', 'x', 'would be overwritten')),
  ], [{ text: CAT_BRANCH_COPY.dirty, raw: '' }, { text: CAT_BRANCH_COPY.dirty, raw: '' }, CAT_BRANCH_COPY.dirty])
  eq('切换失败：已知原因不带详情', catBranchError(new AppError('NOT_FOUND', '', 'reason=branch_not_found')), { text: CAT_BRANCH_COPY.notFound, raw: '' })
  eq('分组标题不可点', branchOptions(mapCatBranches({ git: true, repo: true, locals: ['a'], remotes: ['origin/b'] }))[1].header, true)
  eq('路径省略', [
    pathEllipsis('D:\\Work\\2026\\客户\\字幕项目'),
    pathEllipsis('D:\\a\\字幕项目'),
    pathEllipsis('/home/boss/work/projects/字幕项目/'),
    pathEllipsis('/home/字幕项目'),
  ], ['D:\\…\\字幕项目', 'D:\\a\\字幕项目', '/home/…/字幕项目', '/home/字幕项目'])
}
