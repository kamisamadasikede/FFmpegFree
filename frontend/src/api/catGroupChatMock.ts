/**
 * 群聊走查假数据（浏览器预览，无 Wails 绑定时用；正式包有绑定不会走到这里）。
 * 地址带 ?cat_group_scene=<scene> 时按固定种子出静态画面（截图用，不跑定时器）：
 *   与设计稿 18 个场景同名（list … narrow-single）
 * 不带 scene：普通交互模拟（发送后约 1.2s 出派活灰字、3s 出汇报）。
 */
import type { CatGroup, CatGroupMember, CatGroupMessage, CatGroupMemberEvent, CatGroupRouteEvent, CatRouteDetail, RevertResult, CatCharter, CatGroupReport } from './catGroupChat'

type L<T> = Set<(p: T) => void>
const L = {
  msg: new Set() as L<CatGroupMessage>,
  mem: new Set() as L<CatGroupMemberEvent>,
  route: new Set() as L<CatGroupRouteEvent>,
}
const on = <T,>(set: L<T>, cb: (p: T) => void) => { set.add(cb); return () => { set.delete(cb) } }
const emit = <T,>(set: L<T>, p: T) => [...set].forEach((cb) => cb(p))

export const GROUP_SCENES = ['list', 'create', 'rolegen', 'main', 'members', 'mention', 'working', 'report', 'error', 'dispatch', 'coord', 'swap', 'overstep', 'overstep-undo', 'overstep-result', 'session', 'settings', 'narrow-drawer', 'narrow-single'] as const
export type GroupScene = (typeof GROUP_SCENES)[number]
export function groupScene(): GroupScene | null {
  if (typeof location === 'undefined' || (typeof window !== 'undefined' && (window as unknown as { go?: unknown }).go)) return null
  const s = new URLSearchParams(location.search).get('cat_group_scene')
  return (GROUP_SCENES as readonly string[]).includes(s ?? '') ? (s as GroupScene) : null
}

const T0 = new Date(2026, 9, 17, 0, 0, 0).getTime()
const at = (hm: string) => { const [h, m] = hm.split(':').map(Number); return T0 + (h * 60 + m) * 60_000 }
const PID = 'p3'
let seq = 0
let idn = 0
const nid = (p: string) => `${p}${++idn}`

const groups: CatGroup[] = []
const members: CatGroupMember[] = []
const messages: CatGroupMessage[] = []

function charter(owns: string, notTouch: string, handoff: string, dirs: string[], status: CatCharter['status'] = 'ok', edited: string[] = []): CatCharter {
  return { status, owns, notTouch, handoff, dirs, edited, updatedAt: T0 } as CatCharter
}
let lastAt = at('17:00')
function push(m: Partial<CatGroupMessage> & { kind: string }, when?: number): CatGroupMessage {
  lastAt = when ?? lastAt + 20_000
  const full = { groupId: 'g1', text: '', chainId: 'c1', hop: 0, ...m, seq: ++seq, createdAt: lastAt } as CatGroupMessage
  messages.push(full)
  return full
}
const foot = (hm: string, out: number, rate: number) => ({ finishedAt: at(hm), outputTokens: out, tokensPerSec: rate, totalMs: Math.round((out / rate) * 1000) })

/* 与设计原型 proto/cat-project-group/g.js 同一套示例数据（对照截图用） */
function seedBase() {
  groups.push(
    { id: 'g1', projectId: PID, name: 'v0.55 群聊开发', autoRoute: true, createdAt: T0, updatedAt: T0 } as CatGroup,
    { id: 'g2', projectId: PID, name: '发版检查', autoRoute: true, createdAt: T0, updatedAt: T0 } as CatGroup,
  )
  const mem = (id: string, gid: string, name: string, title: string, agentKind: string, modelId: string, order: number, ch: CatCharter) =>
    ({ id, groupId: gid, name, title, agentKind, modelId, order, charter: ch }) as CatGroupMember
  members.push(
    mem('a', 'g1', '架构师', '架构师', 'claude_cli', 'Claude Opus 4.5', 0, charter('契约与技术方案，拆任务、定接口和验收口径', '不写业务代码，不改前端页面', '方案定了 → @后端工程师 @前端工程师', [])),
    mem('b', 'g1', '后端工程师', '后端工程师', 'codex_cli', 'gpt-5.2-codex', 1, charter('Go 后端：群聊存储、派活、汇报解析、越界检查', '不改 frontend/ 和契约', '接口好了 → @前端工程师；要验证 → @测试', ['internal/catagent/', 'internal/store/', 'app_cat_*.go'], 'ok', ['dirs'])),
    mem('f', 'g1', '前端工程师', '前端工程师', 'cat_build', 'grok-code-fast', 2, charter('Vue 页面：群消息流、汇报卡、干活条、查看过程', '不改 Go 代码和契约', '页面好了 → @测试', ['frontend/src/'])),
    mem('q', 'g1', '测试', '测试工程师', 'claude_cli', 'Claude Sonnet 4.5', 3, charter('按 6.21.11 跑验收用例，报缺陷', '不改业务代码', '有缺陷 → 对应负责人', ['tests/'])),
    mem('a2', 'g2', '架构师', '架构师', 'claude_cli', 'Claude Opus 4.5', 0, charter('发版前检查清单', '不改代码', '有问题 → @测试', [])),
    mem('q2', 'g2', '测试', '测试工程师', 'claude_cli', 'Claude Sonnet 4.5', 1, charter('跑回归', '不改业务代码', '有缺陷 → 对应负责人', ['tests/'])),
  )
}
const T1 = '群聊失败灰字旁边加「重试」：同一任务再跑一轮，hop 不加。先出方案，再分给前后端。'
const rep = (memberId: string, hop: number, hm: string, r: Partial<CatGroupReport> & { did: string }, chainId = 'c1') =>
  push({ kind: 'member', memberId, chainId, hop, turnId: `t-${memberId}-${seq}`, report: { files: [], parsed: true, ...r } }, at(hm))
const rArch = (hm = '17:12', o: Partial<CatGroupReport> = {}, chainId = 'c1') => rep('a', 0, hm, {
  did: '重试方案定了：新增绑定 `RetryCatGroupMemberTurn`，按同一 chainId 把该成员的任务再跑一轮，hop 不加；失败灰字旁加「重试」按钮。已写进契约 6.21.15。',
  files: [{ path: 'docs/architecture/contract.md', added: 38, deleted: 2 }],
  give: '@后端工程师 实现绑定和单测，失败/停止后才允许重试\n@前端工程师 失败灰字旁接「重试」按钮，点了乐观显示「正在安排…」', footer: foot(hm, 2100, 38), ...o }, chainId)
const rBe = () => rep('b', 1, '17:19', {
  did: '`RetryCatGroupMemberTurn` 做好了：校验该成员本链最后一轮是失败或停止，复用原任务再跑一轮，hop 不变。单测 9 个全过。',
  files: [{ path: 'internal/catagent/group_retry.go', added: 86 }, { path: 'internal/catagent/group_retry_test.go', added: 124 }, { path: 'app_cat_group.go', added: 14, deleted: 2 }],
  attn: '成员已被移出群时返回 `member_not_found`，前端按现有错误文案显示即可。', give: '@测试 跑 6.21.11 ④ 的重试用例，三家各一次', footer: foot('17:19', 4600, 52) })
const user = (text: string, hm: string, chainId: string, mentions: string[] = []) => push({ kind: 'user', text, mentions, chainId, hop: 0 }, at(hm))
const route = (to: string, chainId: string, hop: number, mode: string, reason = '') => push({ kind: 'system', systemKind: 'route', chainId, hop, toMemberId: to, route: { mode, memberId: to, memberIds: [to], reason: reason || undefined, durationMs: mode === 'ai' ? 6_400 : mode === 'fallback' ? 20_000 : undefined } })
/** v0.57：系统身份「协调」短气泡；正文 = 理由 + @名…/@全体（与 BE formatCoordBubbleText 同序） */
const coord = (ids: string[], chainId: string, reason: string, everyone = false) => {
  const at = everyone
    ? '@全体'
    : ids.map((id) => `@${members.find((m) => m.id === id)?.name || id}`).join(' ')
  const text = `${reason} ${at}`.trim()
  return push({
    kind: 'system', systemKind: 'coord', chainId, hop: 0, mentions: ids, text,
    route: {
      mode: 'ai', memberIds: ids, memberId: ids[0], reason,
      ...(everyone ? { everyone: true } : {}),
      durationMs: 6_400,
    },
  })
}
const sys = (kind: string, extra: Partial<CatGroupMessage>) => push({ kind: 'system', systemKind: kind, ...extra })
/** v0.56 短复：无四段卡；可显式 replyKind=short，或省略 report 仅带 text */
const short = (memberId: string, hop: number, hm: string, text: string, chainId = 'c1', extra: Partial<CatGroupMessage> = {}) =>
  push({ kind: 'member', memberId, chainId, hop, turnId: `t-${memberId}-short-${seq}`, replyKind: 'short', text, ...extra }, at(hm))
const OV_FILES = [
  { path: 'frontend/src/views/cat/GroupStream.vue', added: 64, deleted: 8 }, { path: 'frontend/src/views/cat/RetryButton.vue', added: 41 },
  { path: 'internal/catagent/group_retry.go', added: 6, deleted: 2, tag: 'overreach' }, { path: 'app_cat_group.go', added: 3, deleted: 1, tag: 'overreach' },
  { path: 'frontend/src/i18n/ja.json', added: 2, tag: 'unknown' },
]
const rFe = () => rep('f', 2, '17:24', {
  did: '失败灰字旁的「重试」接上了：点了先显示「正在安排…」，成员状态回到正在干活；中英日文案都加了。', files: OV_FILES,
  attn: '为了联调方便顺手改了两处后端文件（重试的错误码映射），不在我负责的目录里，请后端确认或撤销。', give: '@测试 前端好了，可以连同后端一起跑重试用例', footer: foot('17:24', 5800, 47) })

function mainStream() {
  user('大家好，今天继续推群聊', '17:04', 'c0')
  coord(['a'], 'c0', '开场招呼')
  short('a', 0, '17:05', '收到。有具体活再 @ 我，闲聊不必硬派。', 'c0')
  sys('no_handoff', { memberId: 'a', hop: 0, chainId: 'c0' })
  user(T1, '17:08', 'c1')
  coord(['a'], 'c1', '先出方案再拆给前后端')
  rArch()
  route('b', 'c1', 1, 'handoff')
  route('f', 'c1', 1, 'handoff')
  rBe()
  route('q', 'c1', 2, 'handoff')
}

/** 场景界面状态：正在干活的人 [id, elapsedMs, toolCount] 等 */
export interface SceneUi { running: [string, number, number][]; /** 成员 id → 排队条数（真实状态：可以同时正在干活 + 排队） */ queued?: Record<string, number>; scroll?: 'top' | 'bottom'; openRoute?: boolean; swapOpen?: boolean; revertDone?: boolean }
let ui: SceneUi = { running: [] }
function sceneSeed(s: GroupScene): SceneUi {
  seedBase()
  const W: [string, number, number][] = [['f', 133_000, 14], ['q', 38_000, 0]]
  switch (s) {
    case 'list': return { running: [] }
    case 'main': case 'create': case 'rolegen': case 'mention': case 'settings':
      mainStream()
      if (s === 'rolegen') { members[2].charter = charter('', '', '', [], 'generating'); members[3].charter = { ...charter('', '', '', [], 'failed'), failReason: '超时：60 秒内没有返回职责 JSON' } }
      return { running: s === 'main' || s === 'create' ? [['q', 38_000, 3]] : [] }
    case 'members': case 'working': case 'session': case 'narrow-drawer': case 'narrow-single':
      mainStream()
      return { running: W, queued: s === 'members' || s === 'working' ? { f: 2 } : undefined }
    case 'report':
      rArch(); route('b', 'c1', 1, 'handoff'); route('f', 'c1', 1, 'handoff'); rBe(); route('q', 'c1', 2, 'handoff')
      rep('q', 2, '17:31', { parsed: false, footer: foot('17:31', 1900, 36),
        did: '三家都跑了一遍重试：Claude 和 Codex 失败后点重试都能接着跑，hop 没变；Grok Build 登录过期那次重试仍然失败，报错文案正常。另外停止后重试也正常。用例记录我放在 tests/group/retry.md 了。' })
      sys('no_handoff', { memberId: 'q', hop: 2 })
      return { running: [] }
    case 'error':
      rBe(); route('f', 'c1', 2, 'handoff'); route('q', 'c1', 2, 'handoff')
      sys('failed', { memberId: 'f', hop: 2, failReason: 'Grok Build 登录已过期，请到设置里重新登录' })
      sys('stopped', { memberId: 'q', hop: 2 })
      user('@架构师 把评审意见也加进契约，然后继续交下去', '17:40', 'c3', ['a'])
      rArch('17:43', { did: '评审意见已补进契约 6.21.15。', files: [{ path: 'docs/architecture/contract.md', added: 6 }], give: '@后端工程师 照新口径复核一遍' }, 'c3')
      route('b', 'c3', 6, 'handoff')
      sys('hop_limit', { chainId: 'c3', hop: 6 })
      return { running: [] }
    case 'dispatch':
      user('在吗？', '17:02', 'c0'); coord(['f'], 'c0', '打招呼')
      // 兜底形状：无 replyKind、无 report，仅 text（后端短复路径可能这样发）
      push({ kind: 'member', memberId: 'f', chainId: 'c0', hop: 0, turnId: 't-f-hi', text: '在的，说下要改哪块就行。' }, at('17:03'))
      user(T1, '17:08', 'c1'); coord(['a'], 'c1', '先出方案再拆给前后端'); rArch()
      user('再改一下，按钮文案用「重试」两个字就行', '17:15', 'c4'); route('a', 'c4', 0, 'followup')
      user('发版前把更新日志整理一下', '17:33', 'c5'); route('a', 'c5', 0, 'fallback')
      user('顺便看下 Windows 打包脚本', '17:36', 'c6'); route('a', 'c6', 0, 'off')
      user('@架构师 顺手把 6.21.17 的排队口径也过一遍', '17:37', 'c7', ['a'])
      push({ kind: 'system', systemKind: 'route', chainId: 'c7', hop: 0, toMemberId: 'a', route: { mode: 'mention', memberId: 'a', memberIds: ['a'], queued: true } })
      return { running: [['a', 65_000, 6]], queued: { a: 1 }, openRoute: true }
    case 'coord':
      // v0.57 四态示意：单人协调@ + @全体 招呼并行短复（成功不叠「正在安排…」）
      user('帮我看下登录页间距', '14:18', 'c8')
      coord(['f'], 'c8', '看登录页间距与规范')
      short('f', 0, '14:19', '好，我先对一下现有 token 和登录页边距。', 'c8')
      user('你们好介绍下自己', '14:20', 'c9')
      coord(['a', 'b', 'f', 'q'], 'c9', '开场介绍一下自己', true)
      short('a', 0, '14:21', '我是架构师，负责契约与方案。', 'c9')
      short('b', 0, '14:21', '我是后端，负责服务与契约实现。', 'c9')
      short('f', 0, '14:21', '我是前端，负责界面实现和走查落地。', 'c9')
      short('q', 0, '14:21', '我是测试，负责按用例验收。', 'c9')
      return { running: [] }
    case 'swap':
      user('重试按钮点了以后，成员卡片要回到「正在干活」状态', '17:46', 'c8'); coord(['b'], 'c8', '涉及成员状态推送')
      sys('reassign', { chainId: 'c8', fromMemberId: 'b', toMemberId: 'f' })
      sys('stopped', { chainId: 'c8', memberId: 'b', fromMemberId: 'b', toMemberId: 'f' })
      route('f', 'c8', 0, 'reassign')
      return { running: [['f', 12_000, 1]], swapOpen: true }
    case 'overstep': case 'overstep-undo': case 'overstep-result':
      rBe(); route('f', 'c1', 2, 'handoff'); rFe(); route('q', 'c1', 3, 'handoff')
      return { running: s === 'overstep-result' ? [] : [['q', 21_000, 2]], revertDone: s === 'overstep-result' }
  }
  return { running: [] }
}

/** 查看过程抽屉的走查内容 */
export const SESSION_DEMO = '先看一下现有的失败灰字组件和群消息流的事件订阅。\n\n[search] systemKind === \'failed\' in frontend/src\n[read] frontend/src/views/cat/GroupStream.vue\n[edit] frontend/src/views/cat/RetryButton.vue  +41\n+ const onRetry = () => RetryCatGroupMemberTurn({ groupId, memberId, chainId })\n[run] npm run typecheck\nvue-tsc --noEmit  ✓ 0 errors\n\n类型检查通过。接下来把中英日文案补上，然后写汇报'

function sceneRunning(): CatGroupMemberEvent[] {
  return ui.running.map(([id, ms, tools]) => ({ groupId: 'g1', running: { turnId: `tr-${id}`, startedAt: T0, toolCount: tools, elapsedMs: ms }, member: { ...members.find((m) => m.id === id)!, status: 'running' }, queue: Array.from({ length: ui.queued?.[id] ?? 0 }, (_, i) => ({ chainId: `cq${i}`, hop: 1, fromSeq: 0, queuePosition: i })) }))
}

const sc = groupScene()
let booted = false
function boot() {
  if (booted) return
  booted = true
  if (sc) ui = sceneSeed(sc)
  else { seedBase(); mainStream() }
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
    m.charter = ok ? charter(`${m.title}职责内的工作`, '不改其他成员负责的目录', '做完交给排第一的成员', [], 'ok') : { ...m.charter, status: 'failed', failReason: 'Grok Build 未登录或已过期，请到设置里重新登录' }
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
      report: { did: `${m.name}处理完了这条任务（走查假数据）。`, parsed: true, files: [{ path: `${m.charter.dirs[0] ?? ''}README.md`, added: 3 }], footer: { finishedAt: Date.now(), totalMs: Date.now() - startedAt, outputTokens: 420, tokensPerSec: 40 } } }, Date.now()))
    emit(L.msg, push({ groupId: gid, kind: 'system', systemKind: 'no_handoff', memberId: mid, chainId, hop, text: '' }, Date.now()))
  }, 3200)
}

export const groupMock = {
  async listGroups(projectId: string) {
    boot()
    // 6.21.17 ②：带成员摘要（avatarIndex = order % 5 + 1）
    return clone(groups.filter((g) => g.projectId === projectId).map((g) => ({ ...g, members: byGroup(g.id).map((m) => ({ id: m.id, name: m.name, title: m.title, agentKind: m.agentKind, order: m.order, avatarIndex: (m.order % 5) + 1 })) })))
  },
  /** 6.21.17 ①：按 path 聚合汇报文件，updatedAt 降序，最多 500；走查里 ja.json 当作已删除 */
  async listFiles(groupId: string) {
    boot()
    const byPath = new Map<string, { path: string; lastMemberId: string; lastSeq: number; lastTurnId: string; changeCount: number; overreach: boolean; tag?: string; exists: boolean; updatedAt: number }>()
    for (const m of messages) {
      if (m.groupId !== groupId || m.kind !== 'member' || !m.report) continue
      for (const f of m.report.files) {
        const prev = byPath.get(f.path)
        byPath.set(f.path, { path: f.path, lastMemberId: m.memberId ?? '', lastSeq: m.seq, lastTurnId: m.turnId ?? '', changeCount: (prev?.changeCount ?? 0) + 1, overreach: !!prev?.overreach || f.tag === 'overreach', tag: f.tag, exists: !f.path.endsWith('ja.json'), updatedAt: m.createdAt })
      }
    }
    const out = [...byPath.values()]
    // 走查：成员/文件页补一条「已不存在」和一条越界，两种标记同屏可见
    if (groupId === 'g1' && !out.some((f) => !f.exists)) out.push({ path: 'frontend/src/views/cat/group/legacy/OldRetryBanner.vue', lastMemberId: 'f', lastSeq: 1, lastTurnId: '', changeCount: 1, overreach: false, exists: false, updatedAt: T0 - 3_600_000 })
    if (groupId === 'g1' && !out.some((f) => f.overreach)) out.push({ path: 'internal/catagent/store/group_queue_store.go', lastMemberId: 'f', lastSeq: 2, lastTurnId: '', changeCount: 1, overreach: true, tag: 'overreach', exists: true, updatedAt: T0 - 1_800_000 })
    return out.sort((a, b) => b.updatedAt - a.updatedAt).slice(0, 500)
  },
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
  async send(groupId: string, text: string, mentions: string[], files: { name: string; path: string }[] = [], _clientMessageId?: string) {
    const chainId = nid('chain-')
    const userMessage = push({ groupId, kind: 'user', text, mentions, files, chainId, hop: 0 }, Date.now())
    const g = groups.find((x) => x.id === groupId)!
    if (mentions.length) {
      mentions.forEach((mid) => fakeTurn(groupId, mid, chainId, 0))
    } else {
      void (async () => {
        const all = byGroup(groupId)
        const first = all[0]
        if (!first) return
        if (g.autoRoute) {
          emit(L.route, { groupId, chainId, state: 'routing', route: { mode: 'ai' } })
          await wait(1200)
          // 招呼 / 介绍类 → @全体；其它单人（走查模拟）
          const intro = /介绍|大家好|你们好|hello|hi\b/i.test(text)
          const ids = intro ? all.map((m) => m.id) : [first.id]
          const reason = intro ? '开场介绍一下自己' : '和需求相关'
          const routeInfo = { mode: 'ai', memberIds: ids, memberId: ids[0], reason, ...(intro ? { everyone: true } : {}), durationMs: 1_200 }
          const at = intro ? '@全体' : ids.map((id) => `@${all.find((m) => m.id === id)?.name || id}`).join(' ')
          const sys = push({ groupId, kind: 'system', systemKind: 'coord', chainId, hop: 0, mentions: ids, route: routeInfo, text: `${reason} ${at}`.trim() }, Date.now())
          routeDetails.set(sys.seq, {
            input: `【你】${text}`,
            output: JSON.stringify({ memberIds: ids, everyone: intro, reason }),
            mode: 'ai', memberId: ids[0], memberIds: ids, everyone: intro, reason,
            durationMs: 1_200,
          })
          emit(L.route, { groupId, chainId, state: 'routed', route: routeInfo })
          emit(L.msg, sys)
          ids.forEach((mid) => fakeTurn(groupId, mid, chainId, 0))
        } else {
          const routeInfo = { mode: 'off', memberId: first.id, memberIds: [first.id] }
          const sys = push({ groupId, kind: 'system', systemKind: 'route', chainId, hop: 0, toMemberId: first.id, route: routeInfo, text: '' }, Date.now())
          routeDetails.set(sys.seq, { input: `【你】${text}`, output: '', mode: 'off', memberId: first.id, reason: '' })
          emit(L.route, { groupId, chainId, state: 'routed', route: routeInfo })
          emit(L.msg, sys)
          fakeTurn(groupId, first.id, chainId, 0)
        }
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
    return routeDetails.get(seq) ?? { input: '这条任务 + 最近 10 条群消息 + 4 位成员的职责', output: '{\n  "memberIds": ["a"],\n  "reason": "先出方案再拆给前后端"\n}', mode: 'ai', memberId: 'a', memberIds: ['a'], reason: '先出方案再拆给前后端', startedAt: T0, finishedAt: T0 + 6_400, durationMs: 6_400 }
  },
  async revert(_memberId: string, _turnId: string, files: string[]): Promise<RevertResult> {
    await wait(sc ? 0 : 400)
    return files.length >= 2 ? { reverted: files.slice(0, -1), skipped: [{ path: files[files.length - 1], reason: 'changed_after' }] } : { reverted: files, skipped: [] }
  },
  async retry(groupId: string, memberId: string, chainId: string) { fakeTurn(groupId, memberId, chainId, 0) },
  async dismissWorkflow(_groupId: string, _chainId: string) {},
  async clearWorkflowHistory(_groupId: string): Promise<string[]> { return [] },
  onMessage: (cb: (m: CatGroupMessage) => void) => on(L.msg, cb),
  onMember: (cb: (e: CatGroupMemberEvent) => void) => on(L.mem, cb),
  onRoute: (cb: (e: CatGroupRouteEvent) => void) => on(L.route, cb),
  /** 场景种子里「正在干活」的成员（store 打开群时读一次） */
  sceneRunning: () => { boot(); return sceneRunning() },
  sceneUi: () => { boot(); return ui },
}
