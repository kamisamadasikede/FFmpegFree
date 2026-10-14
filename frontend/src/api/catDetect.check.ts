import { mapCatStatus } from './cat'
import { allDetectRowsChecking, detectRowView, formatCheckedAt, markRowChecking, mergeCatStatus } from './catDetect'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catDetectChecks(eq: Eq): void {
  const at = new Date(2026, 9, 14, 14, 32).getTime()
  const now = new Date(2026, 9, 14, 18, 0).getTime()
  const row = (k: string, o: Record<string, unknown> = {}) => ({ agentKind: k, ready: true, version: '1', checking: false, checkedAt: at, ...o })
  const full = mapCatStatus({ state: 'ready', version: '1', agents: [row('cat_build'), row('claude_cli'), row('codex_cli', { ready: false })] })
  const empty = mapCatStatus({ state: 'checking', version: '', agents: [] })
  const kinds = (s: { agents: { agentKind: string }[] }) => s.agents.map((a) => a.agentKind).join(',')

  eq('全量 → 三行', kinds(mergeCatStatus(empty, full, true)), 'cat_build,claude_cli,codex_cli')
  eq('mapCatStatus 无 agents[] 不造假行', mapCatStatus({ state: 'ready', version: '1' }).agents.length, 0)
  eq('mapCatStatus hasAgents 标记', [mapCatStatus({ state: 'ready' }).hasAgents, full.hasAgents], [false, true])

  const one = mapCatStatus({ state: 'ready', version: '1', agents: [row('claude_cli', { ready: false, checkedAt: at + 1 })] })
  const m1 = mergeCatStatus(full, one, true)
  eq('只一家 → 该家更新', m1.agents.find((a) => a.agentKind === 'claude_cli')?.ready, false)
  eq('只一家 → 其它家不变', [m1.agents.find((a) => a.agentKind === 'cat_build')?.ready, kinds(m1)], [true, 'cat_build,claude_cli,codex_cli'])

  const legacy = mapCatStatus({ state: 'missing', version: '' })
  const m2 = mergeCatStatus(full, legacy, legacy.hasAgents === true)
  eq('旧载荷无 agents[] → 不重置其它家', m2.agents.map((a) => a.ready), [true, true, false])
  eq('旧载荷无 agents[] → 顶层更新', m2.state, 'missing')

  const partial = mergeCatStatus(empty, one, true)
  eq('部分到达：未到的家 = 正在检测…', detectRowView(partial.agents.find((a) => a.agentKind === 'codex_cli'), { now }).meta, '正在检测…')
  eq('未到不算失败', detectRowView(undefined).failed, false)

  eq('字段容错（旧绑定无新字段）', mapCatStatus({ agents: [{ agentKind: 'codex_cli', ready: true }] }).agents[0].checking, false)
  eq('exePath 解析', mapCatStatus({ agents: [row('codex_cli', { exePath: 'C:\\x\\codex.exe' })] }).agents[0].exePath, 'C:\\x\\codex.exe')

  eq('checkedAt 今天', formatCheckedAt(at, now), '14:32')
  eq('checkedAt 跨天', formatCheckedAt(at - 86400000, now), '10月13日 14:32')

  const v = (o: Record<string, unknown>) => detectRowView(mapCatStatus({ agents: [row('cat_build', o)] }).agents[0], { now })
  eq('正常', v({}).meta, '已安装 · 上次检测 14:32')
  eq('检测中替换上次检测', v({ checking: true }).meta, '已安装 · 正在检测…')
  eq('首次检测中只显示正在检测', v({ checking: true, checkedAt: undefined, ready: false, version: '' }).meta, '正在检测…')
  eq('ready 已知无 checkedAt 也保留已安装', v({ checking: true, checkedAt: undefined, ready: true }).meta, '已安装 · 正在检测…')
  eq('未检测到 + 重检保留未检测到', v({ ready: false, checking: true }).meta, '未检测到 · 正在检测…')
  eq('stale', [v({ stale: true }).meta, v({ stale: true }).showRecheck], ['已安装 · 上次检测 14:32 · 结果可能已过期', true])
  eq('stale + checking 不出按钮和过期文案', [v({ stale: true, checking: true }).meta, v({ stale: true, checking: true }).showRecheck], ['已安装 · 正在检测…', false])
  eq('未检测到 + 失败', [v({ ready: false }).meta, v({ ready: false }).failed], ['未检测到 · 上次检测 14:32', true])
  eq('路径 tooltip', v({ exePath: 'D:\\a b\\grok.exe' }).pathTip, '路径：D:\\a b\\grok.exe')
  eq('全量重检 forceChecking', detectRowView(full.agents[0], { forceChecking: true, now }).meta, '已安装 · 正在检测…')

  const order = ['cat_build', 'claude_cli', 'codex_cli'] as const
  eq('全部检测中 → 底部置灰', allDetectRowsChecking(order, full.agents.map((a) => ({ ...a, checking: true }))), true)
  eq('部分检测中 → 底部不置灰', allDetectRowsChecking(order, markRowChecking(full, 'claude_cli').agents), false)
  eq('forceChecking → 底部置灰', allDetectRowsChecking(order, full.agents, { forceChecking: true }), true)
  eq('结果未到 → 底部置灰', allDetectRowsChecking(order, []), true)

  const mk = markRowChecking(full, 'claude_cli')
  eq('单家重检只动该行', mk.agents.map((a) => a.checking), [false, true, false])
}
