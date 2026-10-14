import { AppError } from './call'
import {
  aheadBehindText, agoText, branchMenuItems, branchTreeError, buildTreeRows, CAT_BTREE_COPY as C, fetchedAtText, FETCH_THROTTLE_MS,
  localNameError, mapBranchCompare, mapBranchTree, parseUnifiedDiff, refreshTip, shouldAutoFetch, type BTreeRow,
} from './catBranchTree'

type Eq = (name: string, got: unknown, want: unknown) => void

const show = (rows: BTreeRow[]) => rows.map((r) => (r.type === 'group' ? `#${r.label}` : r.type === 'folder' ? `${'  '.repeat(r.depth)}${r.open ? 'v' : '>'}${r.label}/` : `${'  '.repeat(r.depth)}${r.label}`))

export function catBranchTreeChecks(eq: Eq): void {
  // ---- 适配：绑定形状 → 前端类型 ----
  eq('分支树：空返回', mapBranchTree(null), { groups: [], recent: [], detached: false, fetchedAt: 0, fetching: false, fetchError: '', noCommits: false })
  const raw = {
    groups: [
      { kind: 'local', branches: [
        { name: 'main', upstream: 'origin/main', ahead: 0, behind: 4, favorite: true, lastCommit: { sha: 's', subject: 'x', time: 100 } },
        { name: 'feat/a', current: true, ahead: 2, behind: 3 },
        { name: 'feat/b' },
        { name: 'release/v1/rc' },
        { name: '' },
      ] },
      { kind: 'remote', remote: 'origin', branches: [{ name: 'HEAD' }, { name: 'main', remote: 'origin' }, { name: 'feat/x', remote: 'origin', favorite: true }] },
    ],
    recent: ['feat/a', 'gone', 'main', 'feat/a', 3],
    fetchedAt: 1760000000,
    fetchError: 'fatal: x',
    extra: 1,
  }
  const t = mapBranchTree(raw)
  eq('分支树：空名字丢弃、远程跳过 HEAD', [t.groups[0].branches.length, t.groups[1].branches.map((b) => b.name)], [4, ['main', 'feat/x']])
  eq('分支树：远程分支不带 current / upstream', [t.groups[1].branches[0].remote, t.groups[1].branches[0].current, t.groups[1].branches[0].upstream], ['origin', false, ''])
  eq('分支树：recent 只留字符串', t.recent, ['feat/a', 'gone', 'main', 'feat/a'])
  eq('分支树：noCommits 时 groups 清空', mapBranchTree({ ...raw, noCommits: true }).groups, [])
  eq('分支树：lastCommit 缺省', t.groups[0].branches[1].lastCommit, { sha: '', subject: '', time: 0 })
  eq('比较：状态取首字母（R100 → R）', mapBranchCompare({ files: [{ path: 'a', status: 'R100', oldPath: 'b' }, { path: '' }] }).files, [{ path: 'a', oldPath: 'b', status: 'R' }])

  // ---- `/` 文件夹 + 分组 ----
  eq('分组 + 文件夹折叠', show(buildTreeRows(t)), [
    '#最近', 'feat/a', 'main',
    '#收藏', 'main', 'origin/feat/x',
    '#本地分支', 'vfeat/', '  a', '  b', 'main', 'vrelease/', '  vv1/', '    rc',
    '#远程分支', 'vorigin/', '  vfeat/', '    x', '  main',
  ])
  eq('折叠组和文件夹', show(buildTreeRows(t, '', new Set(['g:recent', 'local/feat', 'remote/origin']))), [
    '#最近', '#收藏', 'main', 'origin/feat/x', '#本地分支', '>feat/', 'main', 'vrelease/', '  vv1/', '    rc', '#远程分支', '>origin/',
  ])
  eq('同名叶子和文件夹并存', show(buildTreeRows(mapBranchTree({ groups: [{ kind: 'local', branches: [{ name: 'a/b' }, { name: 'a' }] }] }))), ['#本地分支', 'va/', '  b', 'a'])
  eq('空组不显示（没有远程 / 收藏 / 最近）', show(buildTreeRows(mapBranchTree({ groups: [{ kind: 'local', branches: [{ name: 'main', current: true }] }] }))), ['#本地分支', 'main'])
  const many = mapBranchTree({ groups: [{ kind: 'local', branches: 'abcdefg'.split('').map((n) => ({ name: n })) }], recent: 'abcdefg'.split('') })
  eq('最近最多 5 个', buildTreeRows(many).filter((r) => r.key.startsWith('recent|')).length, 5)

  // ---- 搜索 ----
  eq('搜索：忽略大小写、命中自动展开', show(buildTreeRows(t, 'RC', new Set(['local/release', 'local/release/v1', 'g:local']))), ['#本地分支', 'vrelease/', '  vv1/', '    rc'])
  eq('搜索：远程按 origin/ 前缀也能搜', show(buildTreeRows(t, 'origin/fe')), ['#收藏', 'origin/feat/x', '#远程分支', 'vorigin/', '  vfeat/', '    x'])
  eq('搜索：没有结果', buildTreeRows(t, 'hotfix/ffprobe'), [])

  // ---- ↑↓ / 时间 ----
  eq('↑↓', [aheadBehindText({ ahead: 2, behind: 3 }), aheadBehindText({ ahead: 1, behind: 0 }), aheadBehindText({ ahead: 0, behind: 4 }), aheadBehindText({ ahead: 0, behind: 0 })], ['↑2 ↓3', '↑1', '↓4', ''])
  const now = new Date(2026, 9, 14, 19, 0).getTime()
  eq('上次获取：今天只有时分', fetchedAtText(new Date(2026, 9, 14, 14, 32).getTime() / 1000, now), '14:32')
  eq('上次获取：不是今天带日期', fetchedAtText(new Date(2026, 9, 13, 9, 5).getTime() / 1000, now), '10-13 09:05')
  eq('刷新按钮提示', [refreshTip({ fetchedAt: 0 }, now), refreshTip({ fetchedAt: new Date(2026, 9, 14, 14, 32).getTime() / 1000 }, now)], ['从远程获取最新分支', '上次获取 14:32'])
  eq('最后提交', [agoText(now / 1000 - 7200, now), agoText(0, now)], ['2 小时前', ''])

  // ---- 节流 ----
  const m = new Map<string, number>()
  eq('自动获取：5 分钟一次、按对话分开', [shouldAutoFetch('c1', 1000, m), shouldAutoFetch('c1', 1000 + FETCH_THROTTLE_MS - 1, m), shouldAutoFetch('c2', 2000, m), shouldAutoFetch('c1', 1000 + FETCH_THROTTLE_MS, m)], [true, false, true, true])

  // ---- 右键菜单 ----
  const cur = t.groups[0].branches.find((b) => b.current)!
  const other = t.groups[0].branches.find((b) => b.name === 'main')!
  const remote = t.groups[1].branches[0]
  eq('菜单：本地六项', branchMenuItems(other).map((i) => [i.label, i.disabled, i.danger, i.sepBefore]), [
    ['签出', false, false, false], ['从这里新建分支…', false, false, false], ['合并到当前分支', false, false, false], ['和当前分支比较', false, false, false], ['重命名…', false, false, true], ['删除', false, true, false],
  ])
  eq('菜单：当前分支合并 / 比较 / 删除置灰', branchMenuItems(cur).filter((i) => i.disabled).map((i) => [i.id, i.tip]), [['merge', ''], ['compare', ''], ['delete', '不能删除当前分支']])
  eq('菜单：远程分支没有重命名 / 删除', branchMenuItems(remote).map((i) => i.id), ['checkout', 'new', 'merge', 'compare'])

  // ---- 名字校验 / reason 映射 ----
  eq('名字校验', [localNameError('', []), localNameError('a b', []), localNameError('a~1', []), localNameError('a\\b', []), localNameError('x[', []), localNameError('main', ['main']), localNameError('main', ['main'], 'main'), localNameError('feat/ok-1', ['main'])], ['', C.invalidName, C.invalidName, C.invalidName, C.invalidName, C.exists, '', ''])
  eq('文案（产品 v0.51.1 定稿）', [C.invalidName, C.unmerged, C.delForce, C.noCommits, C.fetchFailed, C.newFromLine('feat/a'), C.compareTitle('a', 'b'), C.createdSwitched('x')], [
    '分支名不能包含空格或 ~ ^ : ? * [ \\ 这些字符。', '这个分支有还没合并的提交，删除后可能找不回来。', '仍然删除', '还没有任何提交，提交一次后才会有分支。', '获取远程失败，数量可能不是最新', '从 feat/a 新建', 'a 和 b 的差异', '已新建并切换到 x',
  ])
  const E = (code: 'TASK_CONFLICT' | 'INVALID_ARGUMENT' | 'NOT_FOUND', reason: string, rest = '') => new AppError(code, code === 'INVALID_ARGUMENT' && reason === 'git_failed' ? 'git 失败' : '', `reason=${reason}${rest}`)
  eq('reason 映射', [
    branchTreeError(E('TASK_CONFLICT', 'branch_exists')),
    branchTreeError(E('INVALID_ARGUMENT', 'invalid_branch_name')),
    branchTreeError(E('TASK_CONFLICT', 'branch_not_merged')).field,
    branchTreeError(E('TASK_CONFLICT', 'turn_running')).text,
    branchTreeError(E('TASK_CONFLICT', 'git_dirty')).text,
    branchTreeError(E('NOT_FOUND', 'branch_not_found')).text,
    branchTreeError(E('INVALID_ARGUMENT', 'not_git_repo')).text,
  ], [
    { text: C.exists, raw: '', field: 'name' }, { text: C.invalidName, raw: '', field: 'name' }, 'unmerged',
    '正在回复，等这一轮结束再切换分支。', '有未提交的改动，请先提交或暂存，再切换分支。', '找不到这个分支，可能已被删除。', '这个文件夹不是 Git 仓库。',
  ])
  eq('git_failed：通用失败 + 原始报错（去掉 reason 行）', branchTreeError(E('INVALID_ARGUMENT', 'git_failed', '\nfatal: boom')), { text: C.failed, raw: 'git 失败\nfatal: boom', field: '' })
  eq('is_current：走通用失败', branchTreeError(new AppError('INVALID_ARGUMENT', '', "reason=is_current\nerror: cannot delete branch 'a' used by worktree")), { text: C.failed, raw: "error: cannot delete branch 'a' used by worktree", field: '' })

  // ---- diff ----
  eq('unified diff 行号', parseUnifiedDiff('@@ -41,3 +41,4 @@ f\n a\n-b\n+c\n+d\n e').map((r) => [r.kind, r.no]), [['hunk', null], ['ctx', 41], ['del', 42], ['add', 42], ['add', 43], ['ctx', 44]])
}
