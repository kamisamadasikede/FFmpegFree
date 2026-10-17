/**
 * 群聊走查假数据（浏览器预览，无 Wails 绑定时用；正式包有绑定不会走到这里）。
 * 地址带 ?cat_group_scene=<scene> 时按固定种子出静态画面（截图用，不跑定时器）：
 *   main / working / report / overstep / create / charter / failure / settings / narrow
 * 不带 scene：普通交互模拟（发送后约 1.2s 出派活灰字、3s 出汇报）。
 */
import type { CatGroup, CatGroupMember, CatGroupMessage, CatGroupMemberEvent, CatGroupRouteEvent, CatRouteDetail, RevertResult, CatCharter } from './catGroupChat'

type L<T> = Set<(p: T) => void>
const L = {
  msg: new Set() as L<CatGroupMessage>,
  mem: new Set() as L<CatGroupMemberEvent>,
  route: new Set() as L<CatGroupRouteEvent>,
}
const on = <T,>(set: L<T>, cb: (p: T) => void) => { set.add(cb); return () => { set.delete(cb) } }
const emit = <T,>(set: L<T>, p: T) => [...set].forEach((cb) => cb(p))

export const GROUP_SCENES = ['main', 'working', 'report', 'overstep', 'create', 'charter', 'failure', 'settings', 'narrow'] as const
export type GroupScene = (typeof GROUP_SCENES)[number]
export function groupScene(): GroupScene | null {
  if (typeof location === 'undefined' || (typeof window !== 'undefined' && (window as unknown as { go?: unknown }).go)) return null
  const s = new URLSearchParams(location.search).get('cat_group_scene')
  return (GROUP_SCENES as readonly string[]).includes(s ?? '') ? (s as GroupScene) : null
}

const T0 = new Date(2026, 9, 17, 14, 0, 0).getTime() // 固定时间，截图稳定
const PID = 'p3'
let seq = 0
let idn = 0
const nid = (p: string) => `${p}${++idn}`

const groups: CatGroup[] = []
const members: CatGroupMember[] = []
const messages: CatGroupMessage[] = []

function charter(owns: string, notTouch: string, handoff: string, dirs: string[], status: CatCharter['status'] = 'ok', edited: string[] = []): CatCharter {
  return { status, owns, notTouch, handoff, dirs, edited, updatedAt: T0 }
}
function push(m: Omit<CatGroupMessage, 'seq' | 'createdAt' | 'groupId'> & { groupId?: string }, at?: number): CatGroupMessage {
  const full = { groupId: 'g1', ...m, seq: ++seq, createdAt: at ?? T0 + seq * 45_000 } as CatGroupMessage
  messages.push(full)
  return full
}

function seed() {
  if (groups.length) return
  groups.push(
    { id: 'g1', projectId: PID, name: '字幕改版', autoRoute: true, createdAt: T0, updatedAt: T0 },
    { id: 'g2', projectId: PID, name: '发版检查', autoRoute: false, createdAt: T0, updatedAt: T0 },
  )
  members.push(
    { id: 'm1', groupId: 'g1', name: '小周', title: '产品经理', agentKind: 'claude_cli', modelId: 'claude-sonnet-4.5', order: 0,
      charter: charter('拆需求、写验收标准、排优先级', '不改代码和配置', '需求定了交给 小林（前端）和 老陈（后端）', ['docs/'], 'ok', ['owns']) },
    { id: 'm2', groupId: 'g1', name: '小林', title: '前端工程师', agentKind: 'cat_build', modelId: 'grok-code-fast-1', order: 1,
      charter: charter('frontend/ 下的界面、样式、文案和前端测试', '不改 internal/ 和数据库迁移', '界面做完交给 阿杰（测试）', ['frontend/']) },
    { id: 'm3', groupId: 'g1', name: '老陈', title: '后端工程师', agentKind: 'codex_cli', modelId: 'gpt-5.3-codex', order: 2,
      charter: charter('internal/ 下的服务、绑定和迁移', '不碰 frontend/', '接口好了交给 小林（前端）', ['internal/', 'app/'], 'ok', ['notTouch', 'dirs']) },
    { id: 'm4', groupId: 'g1', name: '阿杰', title: '测试工程师', agentKind: 'claude_cli', modelId: 'claude-haiku-4.5', order: 3,
      charter: charter('写用例、跑回归、记录问题', '不改业务代码', '有问题交回对应的人', ['tests/']) },
  )
  const c1 = 'c1'
  push({ kind: 'user', text: '字幕面板要支持批量改时间轴，@小周 先把需求拆一下', mentions: ['m1'], chainId: c1, hop: 0 })
  push({ kind: 'member', memberId: 'm1', chainId: c1, hop: 0, turnId: 't1', text: '',
    report: { did: '把「批量改时间轴」拆成 3 条：多选字幕、整体平移、按比例拉伸；验收标准写进 docs/subtitle-batch.md。', give: '@小林 做多选和平移的界面；@老陈 加 ShiftCues 绑定', parsed: true,
      files: [{ path: 'docs/subtitle-batch.md', added: 48 }], footer: { totalMs: 41_000, outputTokens: 1830 } } })
  push({ kind: 'system', systemKind: 'route', chainId: c1, hop: 1, toMemberId: 'm2', route: { mode: 'handoff', memberId: 'm2' }, text: '' })
  push({ kind: 'system', systemKind: 'route', chainId: c1, hop: 1, toMemberId: 'm3', route: { mode: 'handoff', memberId: 'm3' }, text: '' })
  push({ kind: 'member', memberId: 'm3', chainId: c1, hop: 1, turnId: 't2', text: '',
    report: { did: '加了 ShiftCues(convId, ids[], offsetMs) 绑定和单测，越界时间裁到 0。', attn: '比例拉伸要等产品确认锚点是首条还是选区中点，先没做。', give: '@小林 绑定已就绪，可以接', parsed: true,
      files: [{ path: 'internal/subtitle/shift.go', added: 86, deleted: 4 }, { path: 'internal/subtitle/shift_test.go', added: 120 }, { path: 'app/subtitle_bindings.go', added: 22 }], footer: { totalMs: 128_000, outputTokens: 4210 } } })
}

function sceneSeed(s: GroupScene) {
  seed()
  const c2 = 'c2'
  if (s === 'report' || s === 'overstep' || s === 'failure' || s === 'narrow') {
    push({ kind: 'user', text: '时间轴面板顺手把快捷键也加上', chainId: c2, hop: 0 })
    push({ kind: 'system', systemKind: 'route', chainId: c2, hop: 0, toMemberId: 'm2', route: { mode: 'ai', memberId: 'm2', reason: '快捷键属于前端界面' }, text: '' })
  }
  if (s === 'report' || s === 'narrow') {
    push({ kind: 'member', memberId: 'm2', chainId: c2, hop: 0, turnId: 't3', text: '',
      report: { did: '时间轴面板支持多选（Shift / Ctrl）和整体平移，快捷键 Alt+← / Alt+→ 每次 100ms。', attn: '平移后没自动保存，沿用现在的「未保存」提示。', give: '@阿杰 帮忙回归一下多选 + 撤销', parsed: true,
        files: [{ path: 'frontend/src/components/subtitle/Timeline.vue', added: 132, deleted: 18 }, { path: 'frontend/src/locales/zh-CN.json', added: 6 }], footer: { totalMs: 96_000, outputTokens: 3120 } } })
    push({ kind: 'system', systemKind: 'route', chainId: c2, hop: 1, toMemberId: 'm4', route: { mode: 'handoff', memberId: 'm4' }, text: '' })
    push({ kind: 'member', memberId: 'm4', chainId: c2, hop: 1, turnId: 't4', text: '',
      report: { did: '回归跑完了，多选和撤销都正常，Alt+→ 在最后一条会越过视频时长，已经记下来了。顺手看了下导出也没问题。', parsed: false, files: [], footer: { totalMs: 52_000, outputTokens: 980 } } })
    push({ kind: 'system', systemKind: 'report_format', memberId: 'm4', chainId: c2, hop: 1, text: '' })
    push({ kind: 'system', systemKind: 'no_handoff', memberId: 'm4', chainId: c2, hop: 1, text: '' })
  }
  if (s === 'overstep') {
    push({ kind: 'member', memberId: 'm2', chainId: c2, hop: 0, turnId: 't3', text: '',
      report: { did: '加了快捷键 Alt+← / Alt+→，顺手把后端的步长常量也改成了 100ms。', give: '@阿杰 回归快捷键', parsed: true,
        files: [
          { path: 'frontend/src/components/subtitle/Timeline.vue', added: 40, deleted: 3 },
          { path: 'go.sum', tag: 'unknown', added: 2 },
          { path: 'internal/subtitle/shift.go', tag: 'overreach', added: 2, deleted: 2 },
          { path: 'internal/subtitle/const.go', tag: 'overreach', added: 1, deleted: 1 },
          { path: 'app/subtitle_bindings.go', tag: 'overreach', added: 4 },
          { path: 'internal/store/migrate.go', tag: 'overreach', added: 3 },
        ], footer: { totalMs: 74_000, outputTokens: 2240 } } })
  }
  if (s === 'failure') {
    push({ kind: 'system', systemKind: 'failed', memberId: 'm2', chainId: c2, hop: 0, failReason: '模型服务商返回 429（请求过多），稍后再试。', text: '' })
    push({ kind: 'system', systemKind: 'stopped', memberId: 'm3', chainId: 'c1', hop: 1, text: '' })
  }
  if (s === 'charter') {
    members.push({ id: 'm5', groupId: 'g1', name: '小吴', title: '设计师', agentKind: 'codex_cli', modelId: 'gpt-5.3-codex', order: 4, charter: charter('', '', '', [], 'generating') })
    members[3].charter = charter('', '', '', [], 'failed')
  }
}

function sceneRunning(s: GroupScene | null): CatGroupMemberEvent[] {
  if (s !== 'working' && s !== 'narrow') return []
  const r = (id: string, ms: number, tools: number): CatGroupMemberEvent => ({ groupId: 'g1', member: members.find((m) => m.id === id)!, running: { turnId: `tr-${id}`, startedAt: T0, toolCount: tools, elapsedMs: ms } })
  return s === 'working' ? [r('m2', 83_000, 7), r('m3', 12_000, 0)] : [r('m2', 83_000, 7)]
}

const sc = groupScene()
let booted = false
function boot() {
  if (booted) return
  booted = true
  if (sc) sceneSeed(sc)
  else seed()
}

const clone = <T,>(v: T): T => JSON.parse(JSON.stringify(v))
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))
const byGroup = (gid: string) => members.filter((m) => m.groupId === gid).sort((a, b) => a.order - b.order)
const routeDetails = new Map<number, CatRouteDetail>()
const running = new Map<string, () => void>()

function fail(reason: string): never {
  const e = new Error(reason) as Error & { code: string; detail: string }
  e.code = 'INVALID_ARGUMENT'
  e.detail = reason
  throw e
}

function genCharter(m: CatGroupMember, ok = true) {
  if (sc) return
  setTimeout(() => {
    m.charter = ok ? charter(`${m.title}职责内的工作`, '不改其他成员负责的目录', '做完交给排第一的成员', [], 'ok') : { ...m.charter, status: 'failed' }
    emit(L.mem, { groupId: m.groupId, member: clone(m) })
  }, 2500)
}

function fakeTurn(gid: string, mid: string, chainId: string, hop: number) {
  const m = members.find((x) => x.id === mid)
  if (!m) return
  const startedAt = Date.now()
  let tools = 0
  const turnId = `t-${chainId}-${mid}`
  const tick = setInterval(() => {
    tools++
    emit(L.mem, { groupId: gid, member: clone(m), running: { turnId, startedAt, toolCount: tools, elapsedMs: Date.now() - startedAt } })
  }, 700)
  emit(L.mem, { groupId: gid, member: clone(m), running: { turnId, startedAt, toolCount: 0, elapsedMs: 0 } })
  running.set(mid, () => clearInterval(tick))
  setTimeout(() => {
    if (!running.has(mid)) return
    clearInterval(tick)
    running.delete(mid)
    emit(L.mem, { groupId: gid, member: clone(m) })
    emit(L.msg, push({ groupId: gid, kind: 'member', memberId: mid, chainId, hop, turnId, text: '',
      report: { did: `${m.name}处理完了这条任务（走查假数据）。`, parsed: true, files: [{ path: `${m.charter.dirs[0] ?? ''}README.md`, added: 3 }], footer: { totalMs: Date.now() - startedAt, outputTokens: 420 } } }, Date.now()))
    emit(L.msg, push({ groupId: gid, kind: 'system', systemKind: 'no_handoff', memberId: mid, chainId, hop, text: '' }, Date.now()))
  }, 3200)
}

export const groupMock = {
  async listGroups(projectId: string) { boot(); return clone(groups.filter((g) => g.projectId === projectId)) },
  async getGroup(groupId: string) {
    boot()
    const group = groups.find((g) => g.id === groupId)
    if (!group) fail('group_not_found')
    return clone({ group, members: byGroup(groupId) })
  },
  async createGroup(projectId: string, name: string) {
    boot()
    const g: CatGroup = { id: nid('g-new-'), projectId, name: name.trim(), autoRoute: true, createdAt: Date.now(), updatedAt: Date.now() }
    groups.push(g)
    return clone(g)
  },
  async renameGroup(groupId: string, name: string) { const g = groups.find((x) => x.id === groupId)!; g.name = name.trim(); return clone(g) },
  async deleteGroup(groupId: string) { const i = groups.findIndex((x) => x.id === groupId); if (i >= 0) groups.splice(i, 1) },
  async setAutoRoute(groupId: string, on: boolean) { const g = groups.find((x) => x.id === groupId)!; g.autoRoute = on; return clone(g) },
  async addMember(req: { groupId: string; name: string; title: string; agentKind: string; modelId: string; thinkLevelId?: string }) {
    const key = req.name.trim().toLowerCase()
    if (byGroup(req.groupId).some((m) => m.name.trim().toLowerCase() === key)) fail('member_name_taken')
    const m: CatGroupMember = { id: nid('m-new-'), groupId: req.groupId, name: req.name.trim(), title: req.title.trim(), agentKind: req.agentKind, modelId: req.modelId, thinkLevelId: req.thinkLevelId, order: byGroup(req.groupId).length, charter: charter('', '', '', [], 'generating') }
    members.push(m)
    genCharter(m, !/fail/i.test(req.title))
    return clone(m)
  },
  async updateMember(req: { memberId: string; name?: string; title?: string; agentKind?: string; modelId?: string; thinkLevelId?: string }) {
    const m = members.find((x) => x.id === req.memberId)!
    if (req.name && byGroup(m.groupId).some((o) => o.id !== m.id && o.name.toLowerCase() === req.name!.trim().toLowerCase())) fail('member_name_taken')
    Object.assign(m, Object.fromEntries(Object.entries(req).filter(([key, v]) => key !== 'memberId' && v !== undefined)))
    return clone(m)
  },
  async removeMember(memberId: string) {
    const i = members.findIndex((x) => x.id === memberId)
    if (i < 0) return
    const gid = members[i].groupId
    members.splice(i, 1)
    byGroup(gid).forEach((m, n) => (m.order = n))
  },
  async reorderMembers(groupId: string, memberIds: string[]) {
    memberIds.forEach((id, n) => { const m = members.find((x) => x.id === id && x.groupId === groupId); if (m) m.order = n })
    return clone(byGroup(groupId))
  },
  async regenerateCharter(memberId: string) {
    const m = members.find((x) => x.id === memberId)!
    if (m.charter.status === 'generating') fail('charter_generating')
    m.charter = { ...m.charter, status: 'generating', edited: [] }
    genCharter(m)
    return clone(m)
  },
  async updateCharter(req: { memberId: string; owns?: string; notTouch?: string; dirs: string[] }) {
    const m = members.find((x) => x.id === req.memberId)!
    const c = m.charter
    const edited = new Set(c.edited)
    if (req.owns !== undefined && req.owns !== c.owns) { c.owns = req.owns; edited.add('owns') }
    if (req.notTouch !== undefined && req.notTouch !== c.notTouch) { c.notTouch = req.notTouch; edited.add('notTouch') }
    if (req.dirs.join('\n') !== c.dirs.join('\n')) { c.dirs = [...req.dirs]; edited.add('dirs') }
    c.edited = [...edited]
    c.status = 'ok'
    c.updatedAt = Date.now()
    return clone(m)
  },
  async send(groupId: string, text: string, mentions: string[]) {
    const chainId = nid('chain-')
    const userMessage = push({ groupId, kind: 'user', text, mentions, chainId, hop: 0 }, Date.now())
    const g = groups.find((x) => x.id === groupId)!
    if (mentions.length) {
      mentions.forEach((mid) => fakeTurn(groupId, mid, chainId, 0))
    } else {
      void (async () => {
        const first = byGroup(groupId)[0]
        if (!first) return
        if (g.autoRoute) {
          emit(L.route, { groupId, chainId, state: 'routing', route: { mode: 'ai' } })
          await wait(1200)
        }
        const route = g.autoRoute ? { mode: 'ai', memberId: first.id, reason: '和需求相关' } : { mode: 'off', memberId: first.id }
        const sys = push({ groupId, kind: 'system', systemKind: 'route', chainId, hop: 0, toMemberId: first.id, route, text: '' }, Date.now())
        routeDetails.set(sys.seq, { input: `【你】${text}`, output: JSON.stringify({ memberId: first.id, reason: route.reason ?? '' }), mode: route.mode, memberId: first.id, reason: route.reason ?? '' })
        emit(L.route, { groupId, chainId, state: 'routed', route })
        emit(L.msg, sys)
        fakeTurn(groupId, first.id, chainId, 0)
      })()
    }
    return { userMessage: clone(userMessage), chainId }
  },
  async reassign(groupId: string, chainId: string, toMemberId: string) {
    const last = [...messages].reverse().find((m) => m.chainId === chainId && m.systemKind === 'route')
    const from = last?.toMemberId ?? ''
    if (running.has(from)) { running.get(from)!(); running.delete(from) }
    const system = push({ groupId, kind: 'system', systemKind: 'reassign', chainId, hop: 0, fromMemberId: from, toMemberId, text: '' }, Date.now())
    setTimeout(() => {
      emit(L.msg, push({ groupId, kind: 'system', systemKind: 'route', chainId, hop: 0, toMemberId, route: { mode: 'reassign', memberId: toMemberId }, text: '' }, Date.now()))
      fakeTurn(groupId, toMemberId, chainId, 0)
    }, 50)
    return { system: clone(system) }
  },
  async stopGroup(groupId: string) { for (const m of byGroup(groupId)) await groupMock.stopMember(m.id) },
  async stopMember(memberId: string) {
    const stop = running.get(memberId)
    if (!stop) return
    stop()
    running.delete(memberId)
    const m = members.find((x) => x.id === memberId)!
    emit(L.mem, { groupId: m.groupId, member: clone(m) })
    emit(L.msg, push({ groupId: m.groupId, kind: 'system', systemKind: 'stopped', memberId, chainId: '', hop: 0, text: '' }, Date.now()))
  },
  async listMessages(groupId: string, beforeSeq = 0, limit = 50) {
    boot()
    const all = messages.filter((m) => m.groupId === groupId && (!beforeSeq || m.seq < beforeSeq))
    return { messages: clone(all.slice(-limit)), hasMore: all.length > limit }
  },
  async routeDetail(_groupId: string, seq: number) {
    return routeDetails.get(seq) ?? { input: '【你】时间轴面板顺手把快捷键也加上\n成员：小周（产品经理）/ 小林（前端工程师）/ 老陈（后端工程师）/ 阿杰（测试工程师）', output: '{"memberId":"m2","reason":"快捷键属于前端界面"}', mode: 'ai', memberId: 'm2', reason: '快捷键属于前端界面' }
  },
  async revert(_memberId: string, _turnId: string, files: string[]): Promise<RevertResult> {
    await wait(sc ? 0 : 400)
    const [conflict, ...rest] = files
    return files.length > 2 ? { reverted: rest, skipped: [{ path: conflict, reason: 'changed_after' }] } : { reverted: files, skipped: [] }
  },
  async retry(groupId: string, memberId: string, chainId: string) { fakeTurn(groupId, memberId, chainId, 0) },
  onMessage: (cb: (m: CatGroupMessage) => void) => on(L.msg, cb),
  onMember: (cb: (e: CatGroupMemberEvent) => void) => on(L.mem, cb),
  onRoute: (cb: (e: CatGroupRouteEvent) => void) => on(L.route, cb),
  /** 场景种子里「正在干活」的成员（store 打开群时读一次） */
  sceneRunning: () => { boot(); return sceneRunning(sc) },
}
