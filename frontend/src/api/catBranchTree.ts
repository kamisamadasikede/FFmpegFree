/**
 * Cat v0.51 分支树（契约 §6.19.26 / §6.19.26.5，对标 IDEA Git Branches）。
 *
 * 适配层：界面只用这里导出的类型和函数。7 个绑定（ListCatBranchTree / FetchCatRemotes / SetCatBranchFavorite /
 * GitCatCreateBranch / GitCatCompareBranch / GitCatRenameBranch / GitCatDeleteBranch）经 callService 调；
 * branchTreeAvailable() 看 ListCatBranchTree 是否已生成（#314 合进 v2 后为 true），否则底栏仍走 v0.49 下拉。
 * 纯浏览器走查：?cat_sim=btree|btreeempty|btreefetchfail|btreenocommits|btreenogit（仅 vite dev）。
 */
import { AppError, callService, hasServiceBinding, toAppError } from '@/api/call'
import { switchCatBranch, CAT_BRANCH_COPY } from '@/api/catBranches'
import { catSim } from '@/views/cat/catDevSim'

// ---------------------------------------------------------------- 文案（产品 v0.51.1 定稿，照抄）
export const CAT_BTREE_COPY = {
  search: '搜索分支',
  groupRecent: '最近',
  groupFav: '收藏',
  groupLocal: '本地分支',
  groupRemote: '远程分支',
  refreshTip: '从远程获取最新分支',
  lastFetch: (t: string) => `上次获取 ${t}`,
  fetching: '正在获取…',
  fetchFailed: '获取远程失败，数量可能不是最新',
  detail: '查看详情',
  noMatch: '没有匹配的分支',
  notGit: '这个文件夹不是 Git 仓库。',
  noCommits: '还没有任何提交，提交一次后才会有分支。',
  mergeFooter: '合并到当前分支…',
  lastCommitTip: (b: string, ago: string) => `${b} · 最后提交 ${ago}`,
  // 右键菜单
  checkout: '签出',
  newFrom: '从这里新建分支…',
  merge: '合并到当前分支',
  compare: '和当前分支比较',
  rename: '重命名…',
  del: '删除',
  cantDeleteCurrent: '不能删除当前分支',
  // 新建
  newTitle: '新建分支',
  newFromLine: (from: string) => `从 ${from} 新建`,
  namePh: '分支名',
  checkoutAfter: '创建后签出',
  create: '创建',
  cancel: '取消',
  exists: '已经有同名分支了。',
  invalidName: '分支名不能包含空格或 ~ ^ : ? * [ \\ 这些字符。',
  created: (b: string) => `已新建分支 ${b}`,
  createdSwitched: (b: string) => `已新建并切换到 ${b}`,
  // 重命名
  renameTitle: '重命名分支',
  renameOk: '重命名',
  renamed: (b: string) => `已重命名为 ${b}`,
  // 删除
  delTitle: (b: string) => `删除分支 ${b}？`,
  delOk: '删除',
  unmerged: '这个分支有还没合并的提交，删除后可能找不回来。',
  delForce: '仍然删除',
  deleted: (b: string) => `已删除分支 ${b}`,
  // 比较
  compareTitle: (b: string, cur: string) => `${b} 和 ${cur} 的差异`,
  compareSame: '两个分支内容相同。',
  compareFiles: (n: number) => `${n} 个文件`,
  // 沿用 6.19.24
  dirty: CAT_BRANCH_COPY.dirty,
  running: CAT_BRANCH_COPY.running,
  notFound: CAT_BRANCH_COPY.notFound,
  switched: CAT_BRANCH_COPY.switched,
  /** 【前端暂定，产品未出】git_failed / 未知 reason 的通用失败句，后跟「查看详情」 */
  failed: '操作失败，请重试。',
} as const

// ---------------------------------------------------------------- 绑定形状（§6.19.26.3）
export interface CatBranchCommit { sha: string; subject: string; time: number }
export interface CatTreeBranch {
  name: string
  remote: string
  upstream: string
  ahead: number
  behind: number
  current: boolean
  favorite: boolean
  lastCommit: CatBranchCommit
}
export interface CatBranchGroup { kind: 'local' | 'remote'; remote: string; branches: CatTreeBranch[] }
export interface CatBranchTree {
  groups: CatBranchGroup[]
  recent: string[]
  detached: boolean
  fetchedAt: number
  fetching: boolean
  fetchError: string
  noCommits: boolean
}
export interface CatCompareFile { path: string; oldPath: string; status: string }
export interface CatBranchCompare {
  base: string
  target: string
  onlyInTarget: CatBranchCommit[]
  onlyInCurrent: CatBranchCommit[]
  truncated: boolean
  files: CatCompareFile[]
}

const obj = (v: unknown): Record<string, unknown> => (typeof v === 'object' && v !== null ? (v as Record<string, unknown>) : {})
const str = (v: unknown): string => (typeof v === 'string' ? v : '')
const int = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? Math.floor(v) : 0)
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : [])

export function mapCommit(raw: unknown): CatBranchCommit {
  const r = obj(raw)
  return { sha: str(r.sha), subject: str(r.subject), time: int(r.time) }
}

export function mapBranchTree(raw: unknown): CatBranchTree {
  const r = obj(raw)
  const noCommits = r.noCommits === true
  const groups: CatBranchGroup[] = noCommits
    ? []
    : arr(r.groups).map((g0) => {
        const g = obj(g0)
        const kind = g.kind === 'remote' ? 'remote' : 'local'
        const remote = kind === 'remote' ? str(g.remote) : ''
        const branches = arr(g.branches)
          .map((b0): CatTreeBranch => {
            const b = obj(b0)
            return {
              name: str(b.name),
              remote: kind === 'remote' ? str(b.remote) || remote : '',
              upstream: kind === 'local' ? str(b.upstream) : '',
              ahead: int(b.ahead),
              behind: int(b.behind),
              current: kind === 'local' && b.current === true,
              favorite: b.favorite === true,
              lastCommit: mapCommit(b.lastCommit),
            }
          })
          // 远程跳过 <remote>/HEAD（后端已跳，前端兜底）
          .filter((b) => b.name !== '' && !(kind === 'remote' && b.name === 'HEAD'))
        return { kind, remote, branches }
      })
  return {
    groups,
    recent: arr(r.recent).filter((x): x is string => typeof x === 'string' && x !== ''),
    detached: r.detached === true,
    fetchedAt: int(r.fetchedAt),
    fetching: r.fetching === true,
    fetchError: str(r.fetchError),
    noCommits,
  }
}

export function mapBranchCompare(raw: unknown): CatBranchCompare {
  const r = obj(raw)
  return {
    base: str(r.base),
    target: str(r.target),
    onlyInTarget: arr(r.onlyInTarget).map(mapCommit),
    onlyInCurrent: arr(r.onlyInCurrent).map(mapCommit),
    truncated: r.truncated === true,
    files: arr(r.files)
      .map((f0) => {
        const f = obj(f0)
        return { path: str(f.path), oldPath: str(f.oldPath), status: str(f.status).slice(0, 1).toUpperCase() || 'M' }
      })
      .filter((f) => f.path !== ''),
  }
}

/** 分支的引用名：本地 = name；远程 = <remote>/<name>（GitCatCreateBranch.from 用这个形状） */
export const branchRef = (b: Pick<CatTreeBranch, 'name' | 'remote'>): string => (b.remote ? `${b.remote}/${b.name}` : b.name)
export const branchKey = (b: Pick<CatTreeBranch, 'name' | 'remote'>): string => (b.remote ? `R:${b.remote}/${b.name}` : `L:${b.name}`)

export function currentBranch(t: CatBranchTree): CatTreeBranch | null {
  for (const g of t.groups) for (const b of g.branches) if (b.current) return b
  return null
}
export const localBranches = (t: CatBranchTree): CatTreeBranch[] => t.groups.filter((g) => g.kind === 'local').flatMap((g) => g.branches)

/** 「↑2 ↓3」；都是 0 返回空串（不显示） */
export function aheadBehindText(b: Pick<CatTreeBranch, 'ahead' | 'behind'>): string {
  return [b.ahead ? `↑${b.ahead}` : '', b.behind ? `↓${b.behind}` : ''].filter(Boolean).join(' ')
}

// ---------------------------------------------------------------- 时间
const pad = (n: number) => String(n).padStart(2, '0')
/** 「上次获取 HH:mm」，不是今天带「MM-DD」，不是今年带「YYYY-MM-DD」（年月日格式为前端暂定） */
export function fetchedAtText(sec: number, now = Date.now()): string {
  const d = new Date(sec * 1000)
  const n = new Date(now)
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (d.toDateString() === n.toDateString()) return hm
  const md = `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  return d.getFullYear() === n.getFullYear() ? `${md} ${hm}` : `${d.getFullYear()}-${md} ${hm}`
}
export function refreshTip(t: Pick<CatBranchTree, 'fetchedAt'>, now = Date.now()): string {
  return t.fetchedAt > 0 ? CAT_BTREE_COPY.lastFetch(fetchedAtText(t.fetchedAt, now)) : CAT_BTREE_COPY.refreshTip
}
/** 最后提交「2 小时前」（只进悬停提示） */
export function agoText(sec: number, now = Date.now()): string {
  if (!sec) return ''
  const s = Math.max(0, Math.floor(now / 1000 - sec))
  if (s < 60) return '刚刚'
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  if (s < 86400 * 30) return `${Math.floor(s / 86400)} 天前`
  const d = new Date(sec * 1000)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

// ---------------------------------------------------------------- 行模型：分组 / `/` 文件夹 / 搜索
export type BTreeRow =
  | { type: 'group'; key: string; label: string; open: boolean }
  | { type: 'folder'; key: string; label: string; depth: number; open: boolean }
  | { type: 'branch'; key: string; label: string; depth: number; branch: CatTreeBranch }

interface Node { name: string; branch?: CatTreeBranch; kids: Map<string, Node> }

function folderRows(list: CatTreeBranch[], prefix: string, depth: number, collapsed: ReadonlySet<string>, forceOpen: boolean, out: BTreeRow[]) {
  const root: Node = { name: '', kids: new Map() }
  for (const b of list) {
    const parts = b.name.split('/').filter(Boolean)
    let n = root
    parts.forEach((p, i) => {
      const k = i === parts.length - 1 ? '\u0000' + p : p // 叶子与同名文件夹分开（a 和 a/b 可以并存）
      if (!n.kids.has(k)) n.kids.set(k, { name: p, kids: new Map() })
      n = n.kids.get(k)!
      if (i === parts.length - 1) n.branch = b
    })
  }
  const walk = (n: Node, path: string, d: number) => {
    const kids = [...n.kids.entries()].sort((a, b) => {
      const x = a[1].name.localeCompare(b[1].name)
      return x !== 0 ? x : a[0].startsWith('\u0000') ? 1 : -1 // 同名时文件夹在前
    })
    for (const [k, c] of kids) {
      if (k.startsWith('\u0000') && c.branch) {
        out.push({ type: 'branch', key: `${prefix}|${branchKey(c.branch)}`, label: c.name, depth: d, branch: c.branch })
      } else {
        const fk = `${prefix}/${path}${c.name}`
        const open = forceOpen || !collapsed.has(fk)
        out.push({ type: 'folder', key: fk, label: c.name, depth: d, open })
        if (open) walk(c, `${path}${c.name}/`, d + 1)
      }
    }
  }
  walk(root, '', depth)
}

/** 名字子串、忽略大小写；本地按 name，远程按 <remote>/<name> */
export function matchBranch(b: CatTreeBranch, q: string): boolean {
  const s = q.trim().toLowerCase()
  return !s || branchRef(b).toLowerCase().includes(s)
}

/**
 * 分组顺序：最近（≤5，只认还存在的本地分支）/ 收藏 / 本地分支 / 远程分支（每个 remote 一个顶层文件夹）；
 * 空组（含搜索后为空）不显示；搜索时文件夹全部展开。collapsed 存组 key（g:*）和文件夹 key。
 */
export function buildTreeRows(t: CatBranchTree, query = '', collapsed: ReadonlySet<string> = new Set()): BTreeRow[] {
  const q = query.trim()
  const searching = q !== ''
  const out: BTreeRow[] = []
  const locals = localBranches(t)
  const group = (key: string, label: string, body: (rows: BTreeRow[]) => void) => {
    const rows: BTreeRow[] = []
    body(rows)
    if (!rows.length) return
    const open = searching || !collapsed.has(key)
    out.push({ type: 'group', key, label, open })
    if (open) out.push(...rows)
  }
  group('g:recent', CAT_BTREE_COPY.groupRecent, (rows) => {
    const seen = new Set<string>()
    for (const n of t.recent) {
      if (seen.has(n) || seen.size >= 5) continue
      seen.add(n)
      const b = locals.find((x) => x.name === n)
      if (b && matchBranch(b, q)) rows.push({ type: 'branch', key: `recent|${branchKey(b)}`, label: b.name, depth: 0, branch: b })
    }
  })
  group('g:fav', CAT_BTREE_COPY.groupFav, (rows) => {
    for (const g of t.groups) for (const b of g.branches)
      if (b.favorite && matchBranch(b, q)) rows.push({ type: 'branch', key: `fav|${branchKey(b)}`, label: branchRef(b), depth: 0, branch: b })
  })
  group('g:local', CAT_BTREE_COPY.groupLocal, (rows) => folderRows(locals.filter((b) => matchBranch(b, q)), 'local', 0, collapsed, searching, rows))
  group('g:remote', CAT_BTREE_COPY.groupRemote, (rows) => {
    for (const g of t.groups) {
      if (g.kind !== 'remote') continue
      const hits = g.branches.filter((b) => matchBranch(b, q))
      if (!hits.length) continue
      const fk = `remote/${g.remote}`
      const open = searching || !collapsed.has(fk)
      rows.push({ type: 'folder', key: fk, label: g.remote, depth: 0, open })
      if (open) folderRows(hits, fk, 1, collapsed, searching, rows)
    }
  })
  return out
}

// ---------------------------------------------------------------- 右键菜单
export type BTreeAction = 'checkout' | 'new' | 'merge' | 'compare' | 'rename' | 'delete'
export interface BTreeMenuItem { id: BTreeAction; label: string; disabled: boolean; tip: string; danger: boolean; sepBefore: boolean }
/** 第一步六项；远程分支没有重命名 / 删除；当前分支上合并 / 比较 / 删除置灰（删除带悬停提示） */
export function branchMenuItems(b: CatTreeBranch): BTreeMenuItem[] {
  const C = CAT_BTREE_COPY
  const it = (id: BTreeAction, label: string, o: Partial<BTreeMenuItem> = {}): BTreeMenuItem => ({ id, label, disabled: false, tip: '', danger: false, sepBefore: false, ...o })
  const items = [it('checkout', C.checkout), it('new', C.newFrom), it('merge', C.merge, { disabled: b.current }), it('compare', C.compare, { disabled: b.current })]
  if (!b.remote) {
    items.push(it('rename', C.rename, { sepBefore: true }))
    items.push(it('delete', C.del, { disabled: b.current, tip: b.current ? C.cantDeleteCurrent : '', danger: !b.current }))
  }
  return items
}

// ---------------------------------------------------------------- 名字校验 / 错误映射
/** 产品文案点名的字符（空格 ~ ^ : ? * [ \）；其余 git 规则（..、@{、结尾 .lock 等）交给后端 check-ref-format */
export function localNameError(name: string, existing: string[], self = ''): string {
  const n = name
  if (!n) return ''
  if (/[\s~^:?*[\\]/.test(n)) return CAT_BTREE_COPY.invalidName
  if (n !== self && existing.includes(n)) return CAT_BTREE_COPY.exists
  return ''
}

export interface BTreeErr { text: string; raw: string; field: '' | 'name' | 'unmerged' }
/** reason → 文案（§6.19.26.4 / 6.19.26.5）。name：弹窗输入框下红字；unmerged：删除弹窗换强删态；raw 非空时带「查看详情」 */
export function branchTreeError(e: unknown): BTreeErr {
  const err = toAppError(e)
  const C = CAT_BTREE_COPY
  switch (err.reason) {
    case 'branch_exists': return { text: C.exists, raw: '', field: 'name' }
    case 'invalid_branch_name': return { text: C.invalidName, raw: '', field: 'name' }
    case 'branch_not_merged': return { text: C.unmerged, raw: '', field: 'unmerged' }
    case 'turn_running': return { text: C.running, raw: '', field: '' }
    case 'git_dirty': return { text: C.dirty, raw: '', field: '' }
    case 'branch_not_found': return { text: C.notFound, raw: '', field: '' }
    case 'not_git_repo': return { text: C.notGit, raw: '', field: '' }
  }
  if (/would be overwritten/i.test(`${err.message ?? ''}\n${err.detail ?? ''}`)) return { text: C.dirty, raw: '', field: '' }
  // git_failed / is_current / 未知：通用失败 + 原始报错（detail 去掉首行 reason=）
  const detail = (err.detail ?? '').replace(/^reason=[A-Za-z0-9_-]+\n?/, '')
  const raw = [err.message, detail].filter((x) => !!x && x.trim()).join('\n')
  return { text: C.failed, raw, field: '' }
}

// ---------------------------------------------------------------- 节流：打开时自动 fetch，同一对话（≈同一仓库）5 分钟最多一次
export const FETCH_THROTTLE_MS = 5 * 60 * 1000
const lastAutoFetch = new Map<string, number>()
export function shouldAutoFetch(key: string, now = Date.now(), map: Map<string, number> = lastAutoFetch): boolean {
  const t = map.get(key)
  if (t !== undefined && now - t < FETCH_THROTTLE_MS) return false
  map.set(key, now)
  return true
}

// ---------------------------------------------------------------- 合并入口钩子（v0.50 合并弹窗在 fe/cat-merge-v050，合进来后在那边注册）
export type CatBranchMergeHandler = (convId: string, branch: { name: string; remote: string } | null) => void
let mergeHandler: CatBranchMergeHandler | null = null
export function setCatBranchMergeHandler(fn: CatBranchMergeHandler | null): void { mergeHandler = fn }
export function requestCatBranchMerge(convId: string, branch: { name: string; remote: string } | null): boolean {
  if (!mergeHandler) return false
  mergeHandler(convId, branch)
  return true
}

// ---------------------------------------------------------------- 绑定调用（+ dev 模拟）
const simOn = () => import.meta.env.DEV && typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go && [...catSim].some((f) => f.startsWith('btree'))

/** 后端绑定生成了，或在走查模拟里 → 底栏用分支树 */
export function branchTreeAvailable(): boolean {
  return simOn() || hasServiceBinding('CatService', 'ListCatBranchTree')
}

export async function listCatBranchTree(convId: string): Promise<CatBranchTree> {
  if (simOn()) return mapBranchTree(await sim.list())
  return mapBranchTree(await callService<unknown>('CatService', 'ListCatBranchTree', { convId }))
}
export async function fetchCatRemotes(convId: string, force: boolean): Promise<CatBranchTree> {
  if (simOn()) return mapBranchTree(await sim.fetch(force))
  return mapBranchTree(await callService<unknown>('CatService', 'FetchCatRemotes', { convId, force }))
}
export async function setCatBranchFavorite(convId: string, b: Pick<CatTreeBranch, 'name' | 'remote'>, favorite: boolean): Promise<void> {
  if (simOn()) return sim.fav(b, favorite)
  await callService('CatService', 'SetCatBranchFavorite', { convId, branch: b.name, remote: b.remote, favorite })
}
export async function gitCatCreateBranch(convId: string, from: string, name: string, checkout: boolean): Promise<CatBranchTree> {
  if (simOn()) return mapBranchTree(await sim.create(from, name, checkout))
  return mapBranchTree(await callService<unknown>('CatService', 'GitCatCreateBranch', { convId, from, name, checkout }))
}
/** 假设：远程分支传 branch=<remote>/<name> 且 remote=true（契约只写了 Branch + Remote bool） */
export async function gitCatCompareBranch(convId: string, b: Pick<CatTreeBranch, 'name' | 'remote'>): Promise<CatBranchCompare> {
  if (simOn()) return mapBranchCompare(await sim.compare(b))
  return mapBranchCompare(await callService<unknown>('CatService', 'GitCatCompareBranch', { convId, branch: branchRef(b), remote: !!b.remote }))
}
export async function gitCatRenameBranch(convId: string, from: string, to: string): Promise<CatBranchTree> {
  if (simOn()) return mapBranchTree(await sim.rename(from, to))
  return mapBranchTree(await callService<unknown>('CatService', 'GitCatRenameBranch', { convId, from, to }))
}
export async function gitCatDeleteBranch(convId: string, branch: string, force: boolean): Promise<CatBranchTree> {
  if (simOn()) return mapBranchTree(await sim.del(branch, force))
  return mapBranchTree(await callService<unknown>('CatService', 'GitCatDeleteBranch', { convId, branch, force }))
}
/**
 * 签出：本地走 SwitchCatBranch；远程按契约走 GitCatCreateBranch({from:'origin/x', name:'x', checkout:true})，
 * 同名本地已存在则改为签出本地。返回最后签出的本地分支名。
 */
export async function checkoutTreeBranch(convId: string, t: CatBranchTree, b: CatTreeBranch): Promise<string> {
  if (!b.remote) {
    if (simOn()) await sim.checkout(b.name)
    else await switchCatBranch(convId, b.name, false)
    return b.name
  }
  if (localBranches(t).some((x) => x.name === b.name)) {
    if (simOn()) await sim.checkout(b.name)
    else await switchCatBranch(convId, b.name, false)
    return b.name
  }
  await gitCatCreateBranch(convId, branchRef(b), b.name, true)
  return b.name
}
/**
 * 比较视图右侧单文件 diff：契约 §6.19.26.3 没有这个接口（只给文件列表）。
 * 绑定 GitCatCompareFileDiff 出现前，真机返回 null（右侧留空）；走查给一段示例 unified diff。
 */
export async function compareFileDiff(convId: string, b: Pick<CatTreeBranch, 'name' | 'remote'>, path: string): Promise<string | null> {
  if (simOn()) return sim.fileDiff(path)
  if (!hasServiceBinding('CatService', 'GitCatCompareFileDiff')) return null
  const r = obj(await callService<unknown>('CatService', 'GitCatCompareFileDiff', { convId, branch: branchRef(b), remote: !!b.remote, path }))
  return str(r.diff)
}

// ---------------------------------------------------------------- 走查模拟（DEV + ?cat_sim=btree*）
const sim = (() => {
  const H = 3600
  const now = () => Math.floor(Date.now() / 1000)
  const mk = (name: string, o: Partial<CatTreeBranch> = {}): CatTreeBranch => ({ name, remote: '', upstream: '', ahead: 0, behind: 0, current: false, favorite: false, lastCommit: { sha: 'a1b2c3d', subject: '走查提交', time: now() - 2 * H }, ...o })
  const empty = catSim.has('btreeempty')
  let locals: CatTreeBranch[] = empty
    ? [mk('main', { current: true })]
    : [
        mk('feat/cat-footer-branch', { current: true, upstream: 'origin/feat/cat-footer-branch', ahead: 2, behind: 3 }),
        mk('feat/cat-merge-3way', { lastCommit: { sha: 'b', subject: 'x', time: now() - 26 * H } }),
        mk('fix/codex-provider', { upstream: 'origin/fix/codex-provider', ahead: 1 }),
        mk('main', { upstream: 'origin/main', behind: 4, favorite: true }),
        mk('release/0.51', { upstream: 'origin/release/0.51' }),
      ]
  let remotes: CatTreeBranch[] = empty ? [] : [mk('main', { remote: 'origin' }), mk('release/0.51', { remote: 'origin' }), mk('feat/system-notify', { remote: 'origin' })]
  let recent = empty ? [] : ['feat/cat-footer-branch', 'fix/codex-provider', 'release/0.51']
  let fetchedAt = now() - 40 * 60
  let fetchError = ''
  let lastTry = 0
  const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))
  const tree = () => ({
    groups: catSim.has('btreenocommits') ? [] : [{ kind: 'local', branches: locals }, ...(remotes.length ? [{ kind: 'remote', remote: 'origin', branches: remotes }] : [])],
    recent, detached: false, fetchedAt, fetching: false, fetchError, noCommits: catSim.has('btreenocommits'),
  })
  const err = (code: 'TASK_CONFLICT' | 'INVALID_ARGUMENT' | 'NOT_FOUND', reason: string, msg = '', stderr = '') => new AppError(code, msg, `reason=${reason}${stderr ? '\n' + stderr : ''}`)
  const find = (n: string) => locals.find((b) => b.name === n)
  const nameCheck = (n: string) => {
    if (localNameError(n, []) || /\.\.|@\{|\.lock$|\/$|^-/.test(n)) throw err('INVALID_ARGUMENT', 'invalid_branch_name')
    if (find(n)) throw err('TASK_CONFLICT', 'branch_exists')
  }
  const touch = (n: string) => { recent = [n, ...recent.filter((x) => x !== n)].slice(0, 5) }
  const setCur = (n: string) => { locals = locals.map((b) => ({ ...b, current: b.name === n })); touch(n) }
  return {
    async list() {
      await wait(120)
      if (catSim.has('btreenogit')) throw err('INVALID_ARGUMENT', 'not_git_repo')
      return tree()
    },
    async fetch(force: boolean) {
      if (!force && Date.now() - lastTry < FETCH_THROTTLE_MS) return tree()
      lastTry = Date.now()
      await wait(catSim.has('btreefetchfail') ? 900 : 1400)
      if (catSim.has('btreefetchfail')) fetchError = "fatal: unable to access 'https://github.com/kamisamadasikede/FFmpegFree.git/': Could not resolve host: github.com\nerror: fetch timed out after 60s (GIT_TERMINAL_PROMPT=0)"
      else { fetchError = ''; fetchedAt = now() }
      return tree()
    },
    async fav(b: Pick<CatTreeBranch, 'name' | 'remote'>, on: boolean) {
      await wait(80)
      const list = b.remote ? remotes : locals
      const hit = list.find((x) => x.name === b.name)
      if (!hit) throw err('NOT_FOUND', 'branch_not_found')
      hit.favorite = on
    },
    async create(from: string, name: string, checkout: boolean) {
      await wait(250)
      nameCheck(name)
      const src = find(from) || remotes.find((r) => `${r.remote}/${r.name}` === from)
      if (!src) throw err('NOT_FOUND', 'branch_not_found')
      locals = [...locals, mk(name, { upstream: src.remote ? from : '' })].sort((a, b) => a.name.localeCompare(b.name))
      if (checkout) setCur(name)
      return tree()
    },
    async checkout(n: string) { await wait(250); if (!find(n)) throw err('NOT_FOUND', 'branch_not_found'); setCur(n) },
    async rename(from: string, to: string) {
      await wait(250)
      const b = find(from)
      if (!b) throw err('NOT_FOUND', 'branch_not_found')
      if (from === to) return tree()
      nameCheck(to)
      locals = locals.map((x) => (x.name === from ? { ...x, name: to } : x)).sort((a, c) => a.name.localeCompare(c.name))
      recent = recent.map((r) => (r === from ? to : r))
      return tree()
    },
    async del(n: string, force: boolean) {
      await wait(250)
      const b = find(n)
      if (!b) throw err('NOT_FOUND', 'branch_not_found')
      if (b.current) throw err('INVALID_ARGUMENT', 'is_current', 'git branch 失败', `error: cannot delete branch '${n}' used by worktree`)
      if (!force && !b.upstream) throw err('TASK_CONFLICT', 'branch_not_merged')
      locals = locals.filter((x) => x.name !== n)
      recent = recent.filter((r) => r !== n)
      return tree()
    },
    async compare(b: Pick<CatTreeBranch, 'name' | 'remote'>) {
      await wait(200)
      const cur = locals.find((x) => x.current)?.name ?? ''
      if (b.name === 'release/0.51') return { base: cur, target: branchRef(b), onlyInTarget: [], onlyInCurrent: [], truncated: false, files: [] }
      return {
        base: cur, target: branchRef(b), onlyInTarget: [], onlyInCurrent: [], truncated: false,
        files: [{ path: 'internal/cat/provider.go', status: 'M' }, { path: 'frontend/src/cat/CatSettings.vue', status: 'M' }, { path: 'internal/cat/provider_test.go', status: 'A' }],
      }
    },
    async fileDiff(path: string) {
      await wait(80)
      if (path.endsWith('_test.go')) return '@@ -0,0 +1,3 @@\n+package cat\n+\n+func TestProvider(t *testing.T) {}'
      return '@@ -41,6 +41,9 @@ func (s *CatService) SaveCatSettings\n   if in.Custom {\n-    return validateAll(in)\n+    if in.APIURL == "" {\n+      return errReason("api_url")\n+    }\n+    in.Effort = ""\n   }\n   return s.store.Save(in)'
    },
  }
})()

// ---------------------------------------------------------------- unified diff → 行（比较视图右侧）
export interface DiffRow { kind: 'hunk' | 'add' | 'del' | 'ctx'; no: number | null; text: string }
export function parseUnifiedDiff(diff: string): DiffRow[] {
  const out: DiffRow[] = []
  let oldNo = 0
  let newNo = 0
  for (const line of diff.split('\n')) {
    if (line.startsWith('diff --git') || line.startsWith('index ') || line.startsWith('--- ') || line.startsWith('+++ ')) continue
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line)
    if (h) { oldNo = +h[1]; newNo = +h[2]; out.push({ kind: 'hunk', no: null, text: line }); continue }
    if (line.startsWith('+')) out.push({ kind: 'add', no: newNo++, text: line })
    else if (line.startsWith('-')) out.push({ kind: 'del', no: oldNo++, text: line })
    else if (line.startsWith('\\')) continue
    else { out.push({ kind: 'ctx', no: newNo, text: line }); oldNo++; newNo++ }
  }
  return out
}
