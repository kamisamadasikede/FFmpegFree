import { AppError } from './call'
import { CAT_MERGE_COPY, catMergeError, defaultMergeOption, mapCatMergeResult, mergeEntryOption, mergeOptions, branchOptions, catBranchError, catBranchErrorText, pathEllipsis, mapCatBranches, middleEllipsis, parseBranchOption, CAT_BRANCH_COPY } from './catBranches'

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

  // ---- v0.50 合并到当前分支（预估形状，契约落地后对齐） ----
  eq('合并结果映射（6.19.25）', [
    mapCatMergeResult({ status: 'merged', head: 'abc' }),
    mapCatMergeResult({ status: 'up_to_date', head: 'abc' }),
    mapCatMergeResult({ status: 'conflict', files: ['a', 'b'] }),
  ], [
    { status: 'merged', conflicts: 0, files: [], head: 'abc' },
    { status: 'up_to_date', conflicts: 0, files: [], head: 'abc' },
    { status: 'conflict', conflicts: 2, files: ['a', 'b'], head: '' },
  ])
  eq('合并错误文案（6.19.25）', [
    catMergeError(new AppError('TASK_CONFLICT', '', 'reason=git_dirty')),
    catMergeError(new AppError('TASK_CONFLICT', '', 'reason=turn_running')),
    catMergeError(new AppError('TASK_CONFLICT', '', 'reason=merge_in_progress')),
    catMergeError(new AppError('INVALID_ARGUMENT', 'x', 'reason=git_failed\nerror: Your local changes would be overwritten by merge')),
  ].map((x) => [x.text, x.raw]), [[CAT_MERGE_COPY.dirty, ''], [CAT_MERGE_COPY.running, ''], [CAT_MERGE_COPY.merging, ''], [CAT_MERGE_COPY.dirty, '']])
  eq('合并通用失败带详情（git_failed / 未知 reason）', [
    catMergeError(new AppError('INVALID_ARGUMENT', 'x', 'reason=weird\nboom')),
    catMergeError(new AppError('INVALID_ARGUMENT', 'git merge 失败', 'reason=git_failed\nfatal: x')),
    catMergeError(new AppError('UNSUPPORTED', '后端还没有提供 CatService.GitCatMerge')),
  ].map((x) => [x.kind, x.text, x.raw]), [['failed', '合并失败，请重试。', 'x\nboom'], ['failed', '合并失败，请重试。', 'git merge 失败\nfatal: x'], ['failed', '合并失败，请重试。', '后端还没有提供 CatService.GitCatMerge']])
  eq('合并拦截：非仓库 / 分支不存在 / 同一分支（产品文案，无详情）', [
    catMergeError(new AppError('INVALID_ARGUMENT', '这里还不是 git 仓库。', 'reason=not_git_repo')),
    catMergeError(new AppError('NOT_FOUND', '找不到这个分支。', 'reason=branch_not_found')),
    catMergeError(new AppError('INVALID_ARGUMENT', 'x', 'reason=same_branch')),
  ].map((x) => [x.text, x.raw]), [['这个文件夹不是 Git 仓库。', ''], ['找不到这个分支，可能已被删除。', ''], ['不能把分支合并到它自己。', '']])
  eq('合并文案', [CAT_MERGE_COPY.entry, CAT_MERGE_COPY.title('main'), CAT_MERGE_COPY.merged('dev', 'main'), CAT_MERGE_COPY.upToDate, CAT_MERGE_COPY.conflict(3)], ['合并到当前分支…', '把哪个分支合并到 main？', '已把 dev 合并到 main', '已经是最新，没有需要合并的内容。', '有 3 个文件冲突，请逐个处理。'])
  const mb = mapCatBranches({ git: true, repo: true, branch: 'main', locals: ['main', 'dev'], remotes: ['origin/feat'] })
  const mo = mergeOptions(mb)
  eq('合并选项：分组 + 排除当前', mo.map((o) => [o.id, !!o.header]), [['H:local', true], ['L:dev', false], ['H:remote', true], ['R:origin/feat', false]])
  eq('合并默认选第一个本地', defaultMergeOption(mo), 'L:dev')
  eq('合并入口：没有可合并分支置灰', [mergeEntryOption(mb).disabled, mergeEntryOption(mapCatBranches({ git: true, repo: true, branch: 'main', locals: ['main'] })).disabled, mergeEntryOption(mb).divided], [false, true, true])
}
