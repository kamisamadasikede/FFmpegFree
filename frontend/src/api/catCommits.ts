/**
 * Cat Git 面板「提交历史」提交图（契约 v0.53 §6.19.28）。
 * 绑定：ListCatCommits / GetCatCommitDetail / GetCatCommitFileDiff / GitCatCheckoutCommit；
 * 新建分支 / 比较复用 GitCatCreateBranch / GitCatCompareBranch（传 sha）。
 * 后端绑定未合入前：adapter 只经 callService 按契约字段名调用，浏览器走查走下方内存模拟（仅 DEV）。
 */
import { AppError, callService, toAppError } from '@/api/call'
import { catSim, type CatSimFlag } from '@/views/cat/catDevSim'
import { mapGitDiff, type GitDiff } from '@/api/catGit'
import { t } from '@/i18n'

export const CAT_COMMIT_COPY = {
  get title() { return t('cat.commit.title') },
  get search() { return t('cat.commit.search') },
  get noMatch() { return t('cat.commit.noMatch') },
  get empty() { return t('cat.commit.empty') },
  get loading() { return t('cat.commit.loading') },
  get loadFail() { return t('cat.commit.loadFail') },
  get retry() { return t('cat.commit.retry') },
  get loadMore() { return t('cat.commit.loadMore') },
  get changed() { return t('cat.commit.changed') },
  get allBranches() { return t('cat.commit.allBranches') },
  get currentBranch() { return t('cat.commit.currentBranch') },
  get copyHash() { return t('cat.commit.copyHash') },
  get copy() { return t('cat.commit.copy') },
  get copied() { return t('cat.commit.copied') },
  get newBranch() { return t('cat.commit.newBranch') },
  get checkout() { return t('cat.commit.checkout') },
  get compare() { return t('cat.commit.compare') },
  get notFound() { return t('cat.commit.notFound') },
  checkoutTitle: (s: string) => t('cat.commit.checkoutTitle', { sha: s }),
  get checkoutBody() { return t('cat.commit.checkoutBody') },
  get checkoutOk() { return t('cat.commit.checkoutOk') },
  get cancel() { return t('cat.commit.cancel') },
  checkedOut: (s: string) => t('cat.commit.checkedOut', { sha: s }),
  get dirty() { return t('cat.commit.dirty') },
  get running() { return t('cat.commit.running') },
  get merging() { return t('cat.commit.merging') },
  get failed() { return t('cat.commit.failed') },
  get detached() { return t('cat.commit.detached') },
  get detachedTip() { return t('cat.commit.detachedTip') },
  get truncated() { return t('cat.commit.truncated') },
  get notGit() { return t('cat.commit.notGit') },
  get refresh() { return t('cat.commit.refresh') },
  get copyFail() { return t('cat.commit.copyFail') },
  get reading() { return t('cat.commit.reading') },
  get binary() { return t('cat.commit.binary') },
  get binaryFile() { return t('cat.commit.binaryFile') },
  get newTitle() { return t('cat.commit.newTitle') },
  newFromLine: (sha: string) => t('cat.commit.newFromLine', { sha }),
  get namePh() { return t('cat.commit.namePh') },
  get checkoutAfter() { return t('cat.commit.checkoutAfter') },
  get create() { return t('cat.commit.create') },
  compareHead: (sha: string) => t('cat.commit.compareHead', { sha }),
  onlyIn: (sha: string, target: number, current: number) => t('cat.commit.onlyIn', { sha, target, current }),
  get close() { return t('cat.commit.close') },
}

export const COMMIT_PAGE = 200

export interface CatCommitRef { name: string; kind: 'head' | 'local' | 'remote' | 'tag' }
export interface CatCommit {
  sha: string
  shortSha: string
  parents: string[]
  subject: string
  authorName: string
  authorEmail: string
  authorTime: number
  refs: CatCommitRef[]
  isHead: boolean
}
export interface CatCommitPage { commits: CatCommit[]; nextCursor: string; total: number; noCommits: boolean }
export interface CatCommitFile { path: string; oldPath: string; status: string; additions: number; deletions: number; binary: boolean }
export interface CatCommitDetail {
  sha: string
  parents: string[]
  authorName: string
  authorEmail: string
  authorTime: number
  message: string
  files: CatCommitFile[]
  filesTruncated: boolean
  diffBase: string
}

type R = Record<string, unknown>
const obj = (v: unknown): R => (v && typeof v === 'object' ? (v as R) : {})
const str = (v: unknown) => (typeof v === 'string' ? v : '')
const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0)
const strs = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string' && x !== '') : [])
const KINDS = ['head', 'local', 'remote', 'tag'] as const

export function mapCommit(raw: unknown): CatCommit {
  const r = obj(raw)
  const sha = str(r.sha)
  return {
    sha,
    shortSha: str(r.shortSha) || sha.slice(0, 7),
    parents: strs(r.parents),
    subject: str(r.subject),
    authorName: str(r.authorName),
    authorEmail: str(r.authorEmail),
    authorTime: num(r.authorTime),
    refs: (Array.isArray(r.refs) ? r.refs : []).map(obj)
      .map((x) => ({ name: str(x.name), kind: (KINDS as readonly string[]).includes(str(x.kind)) ? (str(x.kind) as CatCommitRef['kind']) : 'local' }))
      .filter((x) => x.name),
    isHead: r.isHead === true,
  }
}
export function mapCommitPage(raw: unknown): CatCommitPage {
  const r = obj(raw)
  return {
    commits: (Array.isArray(r.commits) ? r.commits : []).map(mapCommit).filter((c) => c.sha),
    nextCursor: str(r.nextCursor),
    total: num(r.total),
    noCommits: r.noCommits === true,
  }
}
export function mapCommitDetail(raw: unknown): CatCommitDetail {
  const r = obj(raw)
  return {
    sha: str(r.sha),
    parents: strs(r.parents),
    authorName: str(r.authorName),
    authorEmail: str(r.authorEmail),
    authorTime: num(r.authorTime),
    message: str(r.message),
    files: (Array.isArray(r.files) ? r.files : []).map(obj).map((f) => ({
      path: str(f.path), oldPath: str(f.oldPath), status: str(f.status) || 'M',
      additions: num(f.additions), deletions: num(f.deletions), binary: f.binary === true,
    })).filter((f) => f.path),
    filesTruncated: r.filesTruncated === true,
    diffBase: str(r.diffBase),
  }
}

export const isHistoryChanged = (e: unknown) => {
  const x = toAppError(e)
  return x.code === 'INVALID_ARGUMENT' && x.reason === 'history_changed'
}
export const isCommitNotFound = (e: unknown) => {
  const x = toAppError(e)
  return x.code === 'NOT_FOUND' && x.reason === 'commit_not_found'
}
/** 签出 / 详情等错误文案；界面不显示错误码 */
export function catCommitErrorText(e: unknown): string {
  const x = toAppError(e)
  if (isCommitNotFound(e)) return CAT_COMMIT_COPY.notFound
  if (x.reason === 'git_dirty' || /would be overwritten/i.test(`${x.message}\n${x.detail ?? ''}`)) return CAT_COMMIT_COPY.dirty
  if (x.reason === 'turn_running') return CAT_COMMIT_COPY.running
  if (x.reason === 'merge_in_progress') return CAT_COMMIT_COPY.merging
  if (x.reason === 'not_git_repo') return CAT_COMMIT_COPY.notGit
  return CAT_COMMIT_COPY.failed
}

/* ───────────── 车道布局（§6.19.28.3，前端贪心、跨页增量） ───────────── */

export const LANE_COLORS = 6
interface Lane { sha: string; color: number }
export interface LaneState { lanes: (Lane | null)[]; nextColor: number }
/** 线段：x 为车道序号，y 为行内位置（0 顶、0.5 点、1 底） */
export interface LaneSeg { x1: number; y1: number; x2: number; y2: number; color: number }
export interface LaneRow { sha: string; col: number; color: number; segs: LaneSeg[]; width: number; merge: boolean }

export const newLaneState = (): LaneState => ({ lanes: [], nextColor: 0 })

/** 在 state 上继续排一批提交（原地修改 state），返回每行的绘制信息 */
export function layoutLanes(commits: Pick<CatCommit, 'sha' | 'parents'>[], state: LaneState): LaneRow[] {
  const rows: LaneRow[] = []
  const take = () => state.nextColor++ % LANE_COLORS
  for (const c of commits) {
    const before = state.lanes.slice()
    const segs: LaneSeg[] = []
    let col = before.findIndex((l) => l?.sha === c.sha)
    let color: number
    if (col < 0) {
      col = before.findIndex((l) => l === null)
      if (col < 0) col = before.length
      color = take()
    } else color = before[col]!.color
    const lanes = state.lanes
    // 汇入线 + 直通线
    before.forEach((l, i) => {
      if (!l) return
      if (l.sha === c.sha) {
        segs.push({ x1: i, y1: 0, x2: col, y2: 0.5, color: l.color })
        lanes[i] = null
      }
    })
    while (lanes.length <= col) lanes.push(null)
    const [p0, ...rest] = c.parents
    if (p0) {
      // 第一父已被别的车道等着：汇过去，本车道结束
      const j = lanes.findIndex((l, i) => i !== col && l?.sha === p0)
      if (j >= 0) {
        lanes[col] = null
        segs.push({ x1: col, y1: 0.5, x2: j, y2: 1, color: lanes[j]!.color })
      } else {
        lanes[col] = { sha: p0, color }
        segs.push({ x1: col, y1: 0.5, x2: col, y2: 1, color })
      }
    } else lanes[col] = null
    for (const p of rest) {
      const j = lanes.findIndex((l) => l?.sha === p)
      if (j >= 0) {
        segs.push({ x1: col, y1: 0.5, x2: j, y2: 1, color: lanes[j]!.color })
        continue
      }
      let k = lanes.findIndex((l, i) => l === null && i > col)
      if (k < 0) k = lanes.findIndex((l) => l === null)
      if (k < 0) k = lanes.length
      const lane = { sha: p, color: take() }
      lanes[k] = lane
      segs.push({ x1: col, y1: 0.5, x2: k, y2: 1, color: lane.color })
    }
    // 直通：进来时就有、且不是本提交的车道
    before.forEach((l, i) => {
      if (l && l.sha !== c.sha && lanes[i] === l) segs.push({ x1: i, y1: 0, x2: i, y2: 1, color: l.color })
    })
    while (lanes.length && lanes[lanes.length - 1] === null) lanes.pop()
    rows.push({ sha: c.sha, col, color, segs, width: Math.max(before.length, lanes.length, col + 1), merge: c.parents.length > 1 })
  }
  return rows
}

/** 搜索：提交信息（子串）/ 提交号前缀（4–40 位十六进制）/ 作者名或邮箱，忽略大小写，OR（后端同规则；走查模拟用） */
export function commitMatches(c: Pick<CatCommit, 'sha' | 'subject' | 'authorName' | 'authorEmail'> & { message?: string }, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  if (/^[0-9a-f]{4,40}$/.test(q) && c.sha.toLowerCase().startsWith(q)) return true
  return [c.message || c.subject, c.authorName, c.authorEmail].some((s) => s.toLowerCase().includes(q))
}

/** 搜索结果只画点：车道位置沿用已加载完整历史里的位置，没见过的放第 0 道 */
export function searchRows(commits: CatCommit[], known: Map<string, { col: number; color: number }>): LaneRow[] {
  return commits.map((c) => {
    const k = known.get(c.sha)
    const col = k?.col ?? 0
    return { sha: c.sha, col, color: k?.color ?? 0, segs: [], width: col + 1, merge: c.parents.length > 1 }
  })
}

export function relTime(unix: number, now = Date.now()): string {
  const s = Math.max(0, Math.floor(now / 1000 - unix))
  if (s < 60) return t('cat.commit.agoJustNow')
  if (s < 3600) return t('cat.commit.agoMinutes', { n: Math.floor(s / 60) })
  if (s < 86400) return t('cat.commit.agoHours', { n: Math.floor(s / 3600) })
  const d = Math.floor(s / 86400)
  if (d === 1) return t('cat.commit.agoYesterday')
  if (d < 30) return t('cat.commit.agoDays', { n: d })
  const dt = new Date(unix * 1000)
  return `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`
}
export function absTime(unix: number): string {
  const t = new Date(unix * 1000)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${t.getFullYear()}-${p(t.getMonth() + 1)}-${p(t.getDate())} ${p(t.getHours())}:${p(t.getMinutes())}`
}

/* ───────────── 走查模拟（仅 vite dev + ?cat_sim=gitgraph…） ───────────── */

export const commitSimOn = () =>
  import.meta.env.DEV && typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go &&
  ['gitgraph', 'gitgraphempty', 'gitgraphchanged', 'gitgraphloadfail', 'detached'].some((k) => catSim.has(k as CatSimFlag))

const SUBJECTS = ['feat: 底栏分支胶囊', 'fix: 长路径中间省略，悬停显示完整路径', 'docs: v0.50 合并契约 GitCatMerge', 'feat: 底栏显示远程分支和本地分支', 'fix: 没填 API 地址时拒绝打开自定义', 'chore: 打包 55', 'fix: 打开自定义时清空旧的思考强度', 'feat: 新增 GitCatBranch / Checkout 接口', 'release: v0.50', 'fix: 回复成功却标成发送失败', 'feat: 系统通知', 'feat(cat): Codex 和 Claude 显示剩余上下文', 'refactor: 「查看详情」抽成公共组件', 'fix: 弹层挂到 body，空间不够时往上翻', 'feat: 设置页下拉统一为自定义组件', 'fix: 保存失败时保留已填的地址和 Key']
const AUTHORS = ['kami', 'arch', 'be', 'fe']
function hex(n: number) {
  let h = 2166136261 ^ n
  let s = ''
  for (let i = 0; i < 5; i++) { h = Math.imul(h ^ (h >>> 13), 16777619) >>> 0; s += h.toString(16).padStart(8, '0') }
  return s
}
let simData: CatCommit[] | null = null
export let simHeadSha = ''
/** 450 条：主干 + 两条并行特性分支 + 合并，跨页处也有车道 */
export function simCommits(): CatCommit[] {
  if (simData) return simData
  const N = 450
  const sha = (i: number) => hex(i + 1)
  const now = Math.floor(Date.now() / 1000)
  const list: CatCommit[] = []
  for (let i = 0; i < N; i++) {
    const NEXT: Record<number, number[]> = { 0: [1], 1: [2, 3], 2: [4], 3: [5], 4: [6], 5: [7], 6: [8], 7: [8], 8: [9], 9: [10] }
    const k = i % 10
    const parents = i === N - 1 ? [] : NEXT[k].map((d) => sha(Math.min(N - 1, i - k + d)))
    list.push({
      sha: sha(i), shortSha: sha(i).slice(0, 7), parents,
      subject: i === 1 ? '合并 main 到 feat/cat-footer-branch' : SUBJECTS[i % SUBJECTS.length] + (i >= SUBJECTS.length ? ` #${i}` : ''),
      authorName: AUTHORS[i % 4], authorEmail: `${AUTHORS[i % 4]}@ffmpegfree.dev`,
      authorTime: now - 720 - i * 3600 * 3, refs: [], isHead: i === 0,
    })
  }
  list[0].refs = [{ name: 'feat/cat-footer-branch', kind: 'head' }, { name: 'origin/feat/cat-footer-branch', kind: 'remote' }]
  list[4].refs = [{ name: 'main', kind: 'local' }, { name: 'origin/main', kind: 'remote' }]
  list[3].refs = [{ name: 'fix/codex-provider', kind: 'local' }]
  list[9].refs = [{ name: 'v0.50', kind: 'tag' }]
  simHeadSha = list[0].sha
  simData = list
  return list
}
const delay = (ms: number) => new Promise((r) => setTimeout(r, ms))
let simPages = 0
async function simList(cursor: string, query: string): Promise<CatCommitPage> {
  await delay(cursor ? 500 : 250)
  if (catSim.has('gitgraphempty')) return { commits: [], nextCursor: '', total: 0, noCommits: true }
  // 首屏失败：走查「加载失败」+「重试」（首屏 firstError，不是翻页 moreError）
  if (catSim.has('gitgraphloadfail') && !cursor) throw new AppError('PROCESS_FAILED', 'git log 失败', 'reason=git_failed')
  const skip = cursor ? Number(atob(cursor)) : 0
  if (cursor) {
    simPages++
    if (catSim.has('gitgraphchanged') && simPages === 1) throw new AppError('INVALID_ARGUMENT', '', 'reason=history_changed')
  }
  const all = simCommits().filter((c) => commitMatches(c, query))
  const commits = all.slice(skip, skip + COMMIT_PAGE)
  const next = skip + COMMIT_PAGE < all.length ? btoa(String(skip + COMMIT_PAGE)) : ''
  return { commits: commits.map((c) => ({ ...c, isHead: c.sha === simHeadSha })), nextCursor: next, total: query ? 0 : all.length, noCommits: false }
}

/* ───────────── 绑定 ───────────── */

export async function listCatCommits(convId: string, opts: { cursor?: string; query?: string; branch?: string } = {}): Promise<CatCommitPage> {
  if (commitSimOn()) return simList(opts.cursor ?? '', opts.query ?? '')
  const req: R = { convId, limit: COMMIT_PAGE }
  if (opts.cursor) req.cursor = opts.cursor
  if (opts.query?.trim()) req.query = opts.query.trim()
  if (opts.branch) req.branch = opts.branch
  return mapCommitPage(await callService<unknown>('CatService', 'ListCatCommits', req))
}

export async function getCatCommitDetail(convId: string, sha: string): Promise<CatCommitDetail> {
  if (commitSimOn()) {
    await delay(200)
    const c = simCommits().find((x) => x.sha === sha)
    if (!c || simCommits().indexOf(c) === 12) throw new AppError('NOT_FOUND', '', 'reason=commit_not_found')
    return {
      sha, parents: c.parents, authorName: c.authorName, authorEmail: c.authorEmail, authorTime: c.authorTime,
      message: `${c.subject}\n\n保留盘符和最后一级文件夹，最大宽 240px。`,
      files: [
        { path: 'frontend/src/cat/CatInputBar.vue', oldPath: '', status: 'M', additions: 38, deletions: 6, binary: false },
        { path: 'frontend/src/cat/utils/middleEllipsis.ts', oldPath: '', status: 'A', additions: 52, deletions: 0, binary: false },
        { path: 'frontend/src/cat/catState.ts', oldPath: '', status: 'M', additions: 4, deletions: 1, binary: false },
      ],
      filesTruncated: false, diffBase: c.parents[0] ?? '',
    }
  }
  return mapCommitDetail(await callService<unknown>('CatService', 'GetCatCommitDetail', { convId, sha }))
}

export async function getCatCommitFileDiff(convId: string, sha: string, path: string): Promise<GitDiff> {
  if (commitSimOn()) {
    await delay(150)
    return { path, binary: false, text: `diff --git a/${path} b/${path}\n--- a/${path}\n+++ b/${path}\n@@ -1,4 +1,5 @@\n import { computed } from 'vue'\n-const max = 200\n+const max = 240\n+export const keepDrive = true\n export function f() {}\n` }
  }
  return mapGitDiff(await callService<unknown>('CatService', 'GetCatCommitFileDiff', { convId, sha, path }))
}

/** 成功返回 CatBranchTree（detached=true）；这里只关心成功与否，调用方随后刷新 GitCatStatus 与首页 */
export async function gitCatCheckoutCommit(convId: string, sha: string): Promise<void> {
  if (commitSimOn()) { await delay(300); simHeadSha = sha; return }
  await callService('CatService', 'GitCatCheckoutCommit', { convId, sha })
}

/** 从这里新建分支：复用 GitCatCreateBranch，from 传完整 sha（§6.19.28.1）。字段按 §6.19.26 */
export async function gitCatCreateBranchFrom(convId: string, name: string, from: string, checkout: boolean): Promise<void> {
  if (commitSimOn()) { await delay(250); return }
  await callService('CatService', 'GitCatCreateBranch', { convId, name, from, checkout })
}

/** 和当前比较：复用 GitCatCompareBranch，branch 传 sha、remote=false */
export async function gitCatCompareCommit(convId: string, sha: string): Promise<unknown> {
  if (commitSimOn()) { await delay(200); return { target: sha.slice(0, 7) } }
  return callService<unknown>('CatService', 'GitCatCompareBranch', { convId, branch: sha, remote: false })
}
