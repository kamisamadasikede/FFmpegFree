import { conflictLabels, filterBranchItems, gitBranchMenuItems, gitChangeGroups, gitLetterFor, gitMenuEntries, gitRootLabel, mapGitDiff, mapGitStatus, normalizeMergeState, type GitChange } from './catGit'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catGitChecks(eq: Eq): void {
  const changes = [
    { path: 'docs/a.md', letter: 'U' },
    { path: 'docs/b.md', letter: 'M' },
    { path: 'src/main.go', letter: 'D' },
    { path: 'wails.json', letter: 'A' },
  ]
  eq('文件对上自己的字母', gitLetterFor('wails.json', false, changes), 'A')
  eq('对不上就是空', gitLetterFor('missing.ts', false, changes), '')
  eq('文件夹：改过的盖过未跟踪', gitLetterFor('docs', true, changes), 'M')
  eq('文件夹：删除更重', gitLetterFor('src', true, changes), 'D')
  eq('兄弟目录不算', gitLetterFor('other', true, changes), '')
  eq('反斜杠路径也能对上', gitLetterFor('docs\\a.md', false, changes), 'U')
  eq('根标签取最后一段', [gitRootLabel('D:\\GoProject\\FFmpegFree'), gitRootLabel('/tmp/proj/'), gitRootLabel('')], ['FFmpegFree', 'proj', ''])
  eq('状态映射：缺字段当空仓库', mapGitStatus(null), { repo: false, branch: '', upstream: '', ahead: 0, behind: 0, remote: '', web: '', changes: [], mergeState: 'none', conflicts: [], detached: false, headSha: '', headShortSha: '' })
  eq(
    '状态映射：留下有路径的改动',
    mapGitStatus({ repo: true, branch: 'v2', upstream: 'v2', ahead: 7, behind: 1, remote: 'git@github.com:a/b.git', web: 'https://github.com/a/b', changes: [{ path: 'wails.json', name: 'wails.json', letter: 'M', added: 1, deleted: 1, staged: true }, { letter: 'U' }] }),
    { repo: true, branch: 'v2', upstream: 'v2', ahead: 7, behind: 1, remote: 'git@github.com:a/b.git', web: 'https://github.com/a/b', changes: [{ path: 'wails.json', name: 'wails.json', dir: '', letter: 'M', added: 1, deleted: 1, staged: true, unstaged: false }], mergeState: 'none', conflicts: [], detached: false, headSha: '', headShortSha: '' },
  )
  const rows: GitChange[] = [
    { path: 'a.txt', name: 'a.txt', dir: '', letter: 'M', added: 1, deleted: 0, staged: false, unstaged: true },
    { path: 'b.txt', name: 'b.txt', dir: '', letter: 'M', added: 2, deleted: 1, staged: true, unstaged: true },
    { path: 'c.txt', name: 'c.txt', dir: '', letter: 'A', added: 3, deleted: 0, staged: true, unstaged: false },
    { path: 'd.txt', name: 'd.txt', dir: '', letter: 'U', added: 4, deleted: 0, staged: false, unstaged: true },
  ]
  const groups = gitChangeGroups(rows)
  eq('分组：更改只含还没暂存的跟踪文件', groups.work.map((c) => c.path), ['a.txt', 'b.txt'])
  eq('分组：已暂存的更改', groups.staged.map((c) => c.path), ['b.txt', 'c.txt'])
  eq('分组：未跟踪文件', groups.untracked.map((c) => c.path), ['d.txt'])
  const flag = (list: ReturnType<typeof gitMenuEntries>, id: string) => {
    const e = list.find((x) => x.kind === 'item' && x.id === id)
    return !e || e.kind !== 'item' ? { disabled: true, label: '' } : { disabled: e.disabled, label: e.label }
  }
  const menu = gitMenuEntries({ message: '改标题', changeCount: 2, upstream: 'v2', web: 'https://github.com/a/b' })
  eq('菜单：有说明和上游时可以提交并推送', [flag(menu, 'commit').disabled, flag(menu, 'commit-push').disabled, flag(menu, 'publish').disabled, flag(menu, 'rebase').label], [false, false, true, '变基到 v2'])
  const bare = gitMenuEntries({ message: '', changeCount: 2, upstream: '', web: '' })
  eq('菜单：没写说明不能提交，没上游不能推送，可以发布', [flag(bare, 'commit').disabled, flag(bare, 'push').disabled, flag(bare, 'publish').disabled, flag(bare, 'rebase').label], [true, true, false, '变基到上游'])
  eq('对比映射：二进制没有正文', mapGitDiff({ path: 'a.bin', binary: true }), { path: 'a.bin', text: '', binary: true })
  eq('对比映射：缺字段当空', mapGitDiff(null), { path: '', text: '', binary: false })
  eq('mergeState 归一', [normalizeMergeState('merging'), normalizeMergeState('x'), normalizeMergeState(null)], ['merging', 'none', 'none'])
  const conflicted = mapGitStatus({
    repo: true, branch: 'main', mergeState: 'merging',
    conflicts: [{ path: 'a.txt', ours: true, theirs: true, letter: 'UU' }, { letter: 'UU' }],
  })
  eq('冲突映射：留下有 path 的项', [conflicted.mergeState, conflicted.conflicts], ['merging', [{ path: 'a.txt', ours: true, theirs: true, letter: 'UU' }]])
  eq('冲突文案：变基对称', conflictLabels('rebasing').abortTitle, '中止变基？')
  eq('冲突文案：合并全角问号', conflictLabels('merging').abortTitle, '中止合并？')
  const branches = gitBranchMenuItems({ branch: 'main', upstream: 'main', locals: ['main', 'feat/x'], remotes: ['origin/main'] })
  eq('分支菜单：本地当前打钩', branches.filter((b) => b.kind === 'local' && b.current).map((b) => b.name), ['main'])
  eq('分支菜单：远端只读', branches.filter((b) => b.kind === 'remote').map((b) => b.name), ['origin/main'])
  eq('分支筛选', filterBranchItems(branches, 'feat').map((b) => b.name), ['feat/x'])
}

