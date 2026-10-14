import { AppError } from './call'
import { mapGitStatus } from './catGit'
import { CAT_COMMIT_COPY as C, catCommitErrorText, commitMatches, isHistoryChanged, layoutLanes, mapCommitPage, newLaneState, searchRows, simCommits, type LaneRow } from './catCommits'

type Eq = (name: string, got: unknown, want: unknown) => void
const brief = (rows: LaneRow[]) => rows.map((r) => [r.col, r.color, r.segs.map((s) => `${s.x1}${s.y1}>${s.x2}${s.y2}`).sort().join(' ')])

export function catCommitsChecks(eq: Eq): void {
  // 线性
  const lin = layoutLanes([{ sha: 'c', parents: ['b'] }, { sha: 'b', parents: ['a'] }, { sha: 'a', parents: [] }], newLaneState())
  eq('车道：线性同一列同一色', lin.map((r) => [r.col, r.color]), [[0, 0], [0, 0], [0, 0]])
  // 合并：m(p1=a, p2=f) f(a) a()
  const st = newLaneState()
  const m = layoutLanes([{ sha: 'm', parents: ['x', 'f'] }, { sha: 'f', parents: ['x'] }, { sha: 'x', parents: [] }], st)
  eq('车道：合并分叉到右侧新车道、再汇回', brief(m), [
    [0, 0, '00.5>01 00.5>11'],
    [1, 1, '00>01 10.5>01 10>10.5'],
    [0, 0, '00>00.5'],
  ])
  eq('车道：结束后 lanes 清空', st.lanes.length, 0)
  // 两个子提交指向同一父：汇入线
  const two = layoutLanes([{ sha: 'a', parents: ['r'] }, { sha: 'b', parents: ['r'] }, { sha: 'r', parents: [] }], newLaneState())
  eq('车道：并行分支在父提交处汇入', [two[1].col, two[2].col, two[2].segs.filter((s) => s.y2 === 0.5).length], [1, 0, 1])
  // 跨页连续：整段一次算 == 分两页增量算
  const all = simCommits()
  const whole = layoutLanes(all, newLaneState())
  const s2 = newLaneState()
  const paged = [...layoutLanes(all.slice(0, 200), s2), ...layoutLanes(all.slice(200, 400), s2), ...layoutLanes(all.slice(400), s2)]
  eq('车道：跨页增量与一次算完全一致', JSON.stringify(paged) === JSON.stringify(whole), true)
  eq('车道：走查数据有合并且宽度有界', [whole.some((r) => r.merge), Math.max(...whole.map((r) => r.width)) <= 3], [true, true])
  eq('车道：第 7 条起颜色循环', (() => { const s = newLaneState(); s.nextColor = 6; return layoutLanes([{ sha: 'z', parents: [] }], s)[0].color })(), 0)
  eq('状态：游离 HEAD 字段（后端 detached / headSha / headShortSha）', (() => { const g = mapGitStatus({ repo: true, detached: true, headSha: 'abc1234def' }); return [g.detached, g.headSha, g.headShortSha] })(), [true, 'abc1234def', 'abc1234'])
  // 搜索
  const c = { sha: 'abcdef0123', subject: 'Fix: 底栏分支', authorName: 'Kami', authorEmail: 'kami@x.dev' }
  eq('搜索：信息 / 作者 / 邮箱 / 提交号前缀，忽略大小写', [
    commitMatches(c, '底栏'), commitMatches(c, 'fix'), commitMatches(c, 'KAMI'), commitMatches(c, 'x.dev'),
    commitMatches(c, 'ABCD'), commitMatches(c, 'abc'), commitMatches(c, 'bcde'), commitMatches(c, '  '), commitMatches(c, 'zzz'),
  ], [true, true, true, true, true, false, false, true, false])
  const known = new Map([['abcdef0123', { col: 2, color: 4 }]])
  eq('搜索结果：只画点、保留车道位置', searchRows([{ ...c, shortSha: 'abcdef0', parents: ['p', 'q'], authorTime: 0, refs: [], isHead: false }], known).map((r) => [r.col, r.color, r.segs.length, r.merge]), [[2, 4, 0, true]])
  // adapter / 错误
  eq('分页映射：容错', mapCommitPage({ commits: [{ sha: 'abcdef0123456', refs: [{ name: 'main', kind: 'weird' }, {}] }, {}], noCommits: true }), {
    commits: [{ sha: 'abcdef0123456', shortSha: 'abcdef0', parents: [], subject: '', authorName: '', authorEmail: '', authorTime: 0, refs: [{ name: 'main', kind: 'local' }], isHead: false }],
    nextCursor: '', total: 0, noCommits: true,
  })
  eq('错误：history_changed 静默 / commit_not_found 文案 / 签出 dirty', [
    isHistoryChanged(new AppError('INVALID_ARGUMENT', '', 'reason=history_changed')),
    catCommitErrorText(new AppError('NOT_FOUND', '', 'reason=commit_not_found')),
    catCommitErrorText(new AppError('TASK_CONFLICT', '', 'reason=git_dirty')),
  ], [true, '找不到这个提交，可能已被删除。', C.dirty])
  eq('文案', [C.checkoutTitle('abc1234'), C.checkedOut('abc1234'), C.empty, C.changed, C.search, C.noMatch], ['签出提交 abc1234？', '已签出 abc1234', '还没有任何提交。', '提交历史已更新', '搜索提交', '没有匹配的提交'])
}
