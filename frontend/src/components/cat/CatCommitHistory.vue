<template>
  <div ref="root" class="ch" :class="{ narrow }" data-testid="cat-commit-history">
    <div class="ch-h">
      <button type="button" class="ch-t" :aria-expanded="open" data-testid="cat-commit-toggle" @click="toggle">
        <FIcon :name="open ? 'down' : 'right'" :size="12" />
        <span>{{ C.title }}</span>
      </button>
      <template v-if="open">
        <select v-model="branchFilter" class="ch-flt" :disabled="!!query.trim()" data-testid="cat-commit-branch" @change="reload">
          <option value="">{{ C.allBranches }}</option>
          <option value="HEAD">{{ C.currentBranch }}</option>
        </select>
        <button type="button" class="ib" :title="'刷新'" data-testid="cat-commit-refresh" @click="reload"><FIcon name="refresh" :size="14" /></button>
      </template>
    </div>
    <template v-if="open">
      <label class="ch-search">
        <FIcon name="search" :size="13" />
        <input v-model="query" type="search" :placeholder="C.search" data-testid="cat-commit-search" />
      </label>
      <button v-if="changed" type="button" class="ch-changed" data-testid="cat-commit-changed" @click="reload">{{ C.changed }}</button>
      <div v-if="loading && !commits.length" class="ch-state" role="status">{{ C.loading }}</div>
      <div v-else-if="firstError && !commits.length" class="ch-state" role="alert">
        <span>{{ C.loadFail }}</span><button type="button" class="lnk" data-testid="cat-commit-retry" @click="reload">{{ C.retry }}</button>
      </div>
      <div v-else-if="noCommits" class="ch-state" data-testid="cat-commit-empty">{{ C.empty }}</div>
      <div v-else-if="!commits.length && query.trim()" class="ch-state" data-testid="cat-commit-nomatch">{{ C.noMatch }}</div>
      <div v-else class="ch-list" role="list">
        <template v-for="(c, i) in commits" :key="c.sha">
          <div
            class="ch-row"
            :class="{ sel: selected === c.sha, ctx: menu?.c.sha === c.sha }"
            role="listitem"
            :data-sha="c.shortSha"
            @click="pick(c)"
            @contextmenu.prevent="openMenu($event, c)"
          >
            <svg class="g" :width="graphW" height="28" aria-hidden="true">
              <path v-for="(s, k) in rows[i]?.segs ?? []" :key="k" :d="segPath(s)" :stroke="lane(s.color)" stroke-width="2" fill="none" />
              <circle v-if="c.isHead" :cx="lx(rows[i]?.col ?? 0)" cy="14" r="7" :fill="lane(rows[i]?.color ?? 0)" fill-opacity="0.22" />
              <circle
                :cx="lx(rows[i]?.col ?? 0)" cy="14" r="4"
                :fill="rows[i]?.merge ? 'var(--cat-surface)' : lane(rows[i]?.color ?? 0)"
                :stroke="lane(rows[i]?.color ?? 0)" stroke-width="2"
              />
            </svg>
            <span class="sub" :title="c.subject">{{ c.subject }}</span>
            <span v-for="r in c.refs" :key="r.kind + r.name" class="ref" :class="'k-' + r.kind" :title="r.name">
              <FIcon v-if="r.kind === 'tag'" name="tag" :size="11" />{{ r.name }}
            </span>
            <span v-if="!narrow" class="au" :title="c.authorName">{{ c.authorName }}</span>
            <span class="tm" :title="absTime(c.authorTime)">{{ relTime(c.authorTime) }}</span>
          </div>
          <div v-if="selected === c.sha" class="ch-detail" :style="{ marginLeft: Math.min(graphW, 46) + 'px' }" data-testid="cat-commit-detail">
            <div v-if="detailLoading" class="d-muted">正在读取…</div>
            <p v-else-if="detailError" class="d-err" role="alert">{{ detailError }}</p>
            <template v-else-if="detail">
              <div class="d-sha"><FIcon name="commit" :size="13" /><code>{{ detail.sha }}</code><span class="sp" /><button type="button" class="lnk" @click.stop="copySha(detail.sha)"><FIcon name="copy" :size="12" />{{ C.copy }}</button></div>
              <div class="d-muted">{{ detail.authorName }} &lt;{{ detail.authorEmail }}&gt; · {{ absTime(detail.authorTime) }}</div>
              <pre class="d-msg">{{ detail.message }}</pre>
              <button v-for="f in detail.files" :key="f.path" type="button" class="d-file" :title="f.oldPath ? f.oldPath + ' → ' + f.path : f.path" @click.stop="openFile(detail.sha, f.path)">
                <b :class="'s-' + f.status">{{ f.status }}</b><code>{{ f.path }}</code>
                <span class="d-num"><template v-if="f.binary">二进制</template><template v-else><span v-if="f.additions">+{{ f.additions }}</span> <span v-if="f.deletions">−{{ f.deletions }}</span></template></span>
              </button>
              <div v-if="detail.filesTruncated" class="d-muted">{{ C.truncated }}</div>
            </template>
          </div>
        </template>
        <div v-if="nextCursor || moreError" ref="sentinel" class="ch-more">
          <template v-if="moreError"><span>{{ C.loadFail }}</span><button type="button" class="lnk" data-testid="cat-commit-retry" @click="loadMore">{{ C.retry }}</button></template>
          <button v-else type="button" class="lnk" :disabled="loading" @click="loadMore">{{ loading ? C.loading : C.loadMore }}</button>
        </div>
      </div>
    </template>

    <Teleport to="body">
      <div v-if="menu" class="ch-menu" :style="{ left: menu.x + 'px', top: menu.y + 'px' }" role="menu" data-testid="cat-commit-menu" @click.stop>
        <button type="button" role="menuitem" @click="act('copy')">{{ C.copyHash }}</button>
        <button type="button" role="menuitem" @click="act('branch')">{{ C.newBranch }}</button>
        <button type="button" role="menuitem" :disabled="menu.c.isHead" @click="act('checkout')">{{ C.checkout }}</button>
        <hr />
        <button type="button" role="menuitem" :disabled="menu.c.isHead" @click="act('compare')">{{ C.compare }}</button>
      </div>
      <div v-if="dialog" class="ch-mask" @click.self="dialog = null">
        <div class="ch-dlg" :class="{ wide: dialog.kind === 'diff' || dialog.kind === 'compare' }" role="dialog" aria-modal="true" data-testid="cat-commit-dialog">
          <template v-if="dialog.kind === 'checkout'">
            <h3>{{ C.checkoutTitle(dialog.c.shortSha) }}</h3>
            <p class="gray">{{ C.checkoutBody }}</p>
            <p v-if="dialog.err" class="d-err">{{ dialog.err }}</p>
            <div class="btns">
              <button type="button" class="btn" @click="dialog = null">{{ C.cancel }}</button>
              <button type="button" class="btn primary" :disabled="dialog.busy" data-testid="cat-commit-checkout-ok" @click="doCheckout">{{ C.checkoutOk }}</button>
            </div>
          </template>
          <template v-else-if="dialog.kind === 'branch'">
            <h3>新建分支</h3>
            <p class="gray">从 {{ dialog.c.shortSha }} 新建</p>
            <input v-model="dialog.name" class="inp" placeholder="分支名" @keydown.enter="doBranch" />
            <label class="ck"><input v-model="dialog.checkout" type="checkbox" />创建后签出</label>
            <p v-if="dialog.err" class="d-err">{{ dialog.err }}</p>
            <div class="btns">
              <button type="button" class="btn" @click="dialog = null">{{ C.cancel }}</button>
              <button type="button" class="btn primary" :disabled="dialog.busy || !dialog.name.trim()" @click="doBranch">创建</button>
            </div>
          </template>
          <template v-else-if="dialog.kind === 'diff'">
            <h3><code>{{ dialog.c.shortSha }}</code> · {{ dialog.path }}</h3>
            <div v-if="dialog.busy" class="d-muted">正在读取…</div>
            <p v-else-if="dialog.err" class="d-err">{{ dialog.err }}</p>
            <div v-else-if="dialog.binary" class="d-muted">二进制文件，不显示内容。</div>
            <pre v-else class="diff"><span v-for="(l, k) in dialog.lines" :key="k" :class="diffCls(l)">{{ l }}
</span></pre>
            <div class="btns"><button type="button" class="btn" @click="dialog = null">关闭</button></div>
          </template>
          <template v-else-if="dialog.kind === 'compare'">
            <h3>和当前比较 · <code>{{ dialog.c.shortSha }}</code></h3>
            <div v-if="dialog.busy" class="d-muted">正在读取…</div>
            <p v-else-if="dialog.err" class="d-err">{{ dialog.err }}</p>
            <template v-else>
              <p class="gray">只在 {{ dialog.c.shortSha }} 里：{{ dialog.onlyTarget }} 个提交 · 只在当前：{{ dialog.onlyCurrent }} 个提交</p>
              <div v-for="f in dialog.files" :key="f.path" class="d-file static"><b :class="'s-' + f.status">{{ f.status }}</b><code>{{ f.path }}</code></div>
            </template>
            <div class="btns"><button type="button" class="btn" @click="dialog = null">关闭</button></div>
          </template>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import {
  CAT_COMMIT_COPY as C, absTime, catCommitErrorText, getCatCommitDetail, getCatCommitFileDiff, gitCatCheckoutCommit,
  gitCatCompareCommit, gitCatCreateBranchFrom, isHistoryChanged, layoutLanes, listCatCommits, newLaneState, relTime, searchRows,
  type CatCommit, type CatCommitDetail, type LaneRow, type LaneSeg, type LaneState,
} from '@/api/catCommits'
import { catGitError } from '@/api/catGit'
import { notify } from '@/views/cat/catState'

const props = defineProps<{ convId: string; defaultOpen?: boolean }>()
const emit = defineEmits<{ changed: [] }>()

const LANE_W = 12
const PAD = 10
const open = ref(!!props.defaultOpen)
const query = ref('')
const branchFilter = ref('')
const commits = ref<CatCommit[]>([])
const rows = ref<LaneRow[]>([])
const nextCursor = ref('')
const loading = ref(false)
const firstError = ref(false)
const moreError = ref(false)
const noCommits = ref(false)
const changed = ref(false)
const selected = ref('')
const detail = ref<CatCommitDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
const menu = ref<{ x: number; y: number; c: CatCommit } | null>(null)
type Dlg =
  | { kind: 'checkout'; c: CatCommit; busy: boolean; err: string }
  | { kind: 'branch'; c: CatCommit; busy: boolean; err: string; name: string; checkout: boolean }
  | { kind: 'diff'; c: { shortSha: string }; path: string; busy: boolean; err: string; binary: boolean; lines: string[] }
  | { kind: 'compare'; c: CatCommit; busy: boolean; err: string; onlyTarget: number; onlyCurrent: number; files: { path: string; status: string }[] }
const dialog = ref<Dlg | null>(null)
const root = ref<HTMLElement>()
const sentinel = ref<HTMLElement>()
const narrow = ref(false)

let lanes: LaneState = newLaneState()
/** 完整历史（无搜索）里见过的车道位置：搜索结果沿用，只隐藏连线 */
const known = new Map<string, { col: number; color: number }>()
let gen = 0

const graphW = computed(() => PAD * 2 + LANE_W * Math.max(0, Math.max(1, ...rows.value.map((r) => r.width)) - 1))
const lx = (col: number) => PAD + col * LANE_W
const lane = (color: number) => `var(--cat-lane-${(color % 6) + 1})`
function segPath(s: LaneSeg): string {
  const x1 = lx(s.x1), x2 = lx(s.x2), y1 = s.y1 * 28, y2 = s.y2 * 28
  if (x1 === x2) return `M${x1} ${y1}L${x2} ${y2}`
  const m = (y1 + y2) / 2
  return `M${x1} ${y1}C${x1} ${m} ${x2} ${m} ${x2} ${y2}`
}

function reset() {
  lanes = newLaneState()
  commits.value = []
  rows.value = []
  nextCursor.value = ''
  moreError.value = false
  firstError.value = false
  noCommits.value = false
  changed.value = false
  selected.value = ''
  detail.value = null
}

async function fetchPage(first: boolean) {
  const my = gen
  const q = query.value.trim()
  loading.value = true
  try {
    const page = await listCatCommits(props.convId, { cursor: first ? '' : nextCursor.value, query: q, branch: q ? '' : branchFilter.value })
    if (my !== gen) return
    noCommits.value = page.noCommits
    const add = q ? searchRows(page.commits, known) : layoutLanes(page.commits, lanes)
    if (!q) for (const r of add) known.set(r.sha, { col: r.col, color: r.color })
    commits.value = [...commits.value, ...page.commits]
    rows.value = [...rows.value, ...add]
    nextCursor.value = page.nextCursor
    moreError.value = false
  } catch (e) {
    if (my !== gen) return
    if (isHistoryChanged(e)) {
      // 不弹错、不自动重拉：顶部一行提示，点了再从首页拉
      changed.value = true
      nextCursor.value = ''
    } else if (first) firstError.value = true
    else moreError.value = true
  } finally {
    if (my === gen) loading.value = false
  }
}

function reload() {
  gen++
  if (!query.value.trim()) known.clear()
  reset()
  void fetchPage(true)
}
function loadMore() {
  if (loading.value || !nextCursor.value && !moreError.value) return
  if (moreError.value && !nextCursor.value) return
  void fetchPage(false)
}
function toggle() {
  open.value = !open.value
  if (open.value && !commits.value.length && !loading.value) reload()
}

let qTimer: ReturnType<typeof setTimeout> | undefined
watch(query, () => {
  clearTimeout(qTimer)
  qTimer = setTimeout(reload, 250)
})
watch(() => props.convId, () => { if (open.value) reload() })

async function pick(c: CatCommit) {
  if (selected.value === c.sha) { selected.value = ''; return }
  selected.value = c.sha
  detail.value = null
  detailError.value = ''
  detailLoading.value = true
  try {
    const d = await getCatCommitDetail(props.convId, c.sha)
    if (selected.value === c.sha) detail.value = d
  } catch (e) {
    if (selected.value === c.sha) detailError.value = catCommitErrorText(e)
  } finally {
    if (selected.value === c.sha) detailLoading.value = false
  }
}

function openMenu(ev: MouseEvent, c: CatCommit) {
  const x = Math.min(ev.clientX, window.innerWidth - 188)
  const y = Math.min(ev.clientY, window.innerHeight - 150)
  menu.value = { x, y, c }
}
const closeMenu = () => (menu.value = null)

async function copySha(sha: string) {
  try {
    await navigator.clipboard.writeText(sha)
    notify(C.copied)
  } catch {
    notify('没能复制。', 'warn')
  }
}

async function act(kind: 'copy' | 'branch' | 'checkout' | 'compare') {
  const c = menu.value?.c
  closeMenu()
  if (!c) return
  if (kind === 'copy') return copySha(c.sha)
  if (kind === 'checkout') { dialog.value = { kind, c, busy: false, err: '' }; return }
  if (kind === 'branch') { dialog.value = { kind, c, busy: false, err: '', name: '', checkout: true }; return }
  const d: Dlg = { kind: 'compare', c, busy: true, err: '', onlyTarget: 0, onlyCurrent: 0, files: [] }
  dialog.value = d
  try {
    const r = (await gitCatCompareCommit(props.convId, c.sha)) as Record<string, unknown> | null
    const arr = (v: unknown) => (Array.isArray(v) ? v : [])
    if (dialog.value?.kind !== 'compare') return
    dialog.value = { ...d, busy: false, onlyTarget: arr(r?.onlyInTarget).length, onlyCurrent: arr(r?.onlyInCurrent).length, files: arr(r?.files).map((f) => ({ path: String((f as Record<string, unknown>).path ?? ''), status: String((f as Record<string, unknown>).status ?? 'M') })) }
  } catch (e) {
    if (dialog.value?.kind === 'compare') dialog.value = { ...d, busy: false, err: catCommitErrorText(e) }
  }
}

async function doCheckout() {
  const d = dialog.value
  if (d?.kind !== 'checkout') return
  d.busy = true
  d.err = ''
  try {
    await gitCatCheckoutCommit(props.convId, d.c.sha)
    dialog.value = null
    notify(C.checkedOut(d.c.shortSha))
    emit('changed')
    reload()
  } catch (e) {
    d.busy = false
    d.err = catCommitErrorText(e)
  }
}

const BAD_NAME = /[\s~^:?*[\\]/
async function doBranch() {
  const d = dialog.value
  if (d?.kind !== 'branch' || !d.name.trim() || d.busy) return
  const name = d.name.trim()
  if (BAD_NAME.test(name)) { d.err = '分支名不能包含空格或 ~ ^ : ? * [ \\ 这些字符。'; return }
  d.busy = true
  d.err = ''
  try {
    await gitCatCreateBranchFrom(props.convId, name, d.c.sha, d.checkout)
    dialog.value = null
    notify(d.checkout ? `已新建并切换到 ${name}` : `已新建分支 ${name}`)
    emit('changed')
    reload()
  } catch (e) {
    d.busy = false
    const reason = (e as { reason?: string })?.reason
    d.err = reason === 'branch_exists' ? '已经有同名分支了。' : reason === 'invalid_branch_name' ? '分支名不能包含空格或 ~ ^ : ? * [ \\ 这些字符。' : catCommitErrorText(e)
  }
}

async function openFile(sha: string, path: string) {
  const c = commits.value.find((x) => x.sha === sha)
  const d: Dlg = { kind: 'diff', c: { shortSha: c?.shortSha ?? sha.slice(0, 7) }, path, busy: true, err: '', binary: false, lines: [] }
  dialog.value = d
  try {
    const r = await getCatCommitFileDiff(props.convId, sha, path)
    if (dialog.value?.kind === 'diff') dialog.value = { ...d, busy: false, binary: r.binary, lines: r.text.replace(/\n$/, '').split('\n') }
  } catch (e) {
    if (dialog.value?.kind === 'diff') dialog.value = { ...d, busy: false, err: catCommitErrorText(e) === C.failed ? catGitError(e) : catCommitErrorText(e) }
  }
}
const diffCls = (l: string) => (l.startsWith('+++') || l.startsWith('---') || l.startsWith('diff ') ? 'meta' : l.startsWith('@@') ? 'hunk' : l.startsWith('+') ? 'add' : l.startsWith('-') ? 'del' : '')

let io: IntersectionObserver | undefined
let ro: ResizeObserver | undefined
watch(sentinel, (el) => {
  io?.disconnect()
  if (!el) return
  io = new IntersectionObserver((es) => { if (es.some((x) => x.isIntersecting) && !moreError.value) loadMore() })
  io.observe(el)
})
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') { closeMenu(); dialog.value = null }
}
onMounted(() => {
  document.addEventListener('click', closeMenu)
  document.addEventListener('keydown', onKey)
  if (root.value) {
    ro = new ResizeObserver(() => { narrow.value = (root.value?.clientWidth ?? 999) < 420 })
    ro.observe(root.value)
  }
  if (open.value) reload()
})
onBeforeUnmount(() => {
  document.removeEventListener('click', closeMenu)
  document.removeEventListener('keydown', onKey)
  io?.disconnect()
  ro?.disconnect()
  clearTimeout(qTimer)
})

defineExpose({ reload: () => { if (open.value) reload() } })
void nextTick
</script>

<style scoped>
.ch { border-top: 1px solid var(--cat-divider); margin-top: 10px; font-size: 12px; }
.ch-h { height: 32px; display: flex; align-items: center; gap: 4px; padding: 0 6px 0 8px; }
.ch-t { flex: 1; min-width: 0; display: flex; align-items: center; gap: 4px; border: 0; background: transparent; color: var(--cat-text-2); font: inherit; font-size: 12px; cursor: pointer; padding: 0; text-align: left; }
.ch-flt { height: 22px; font-size: 11.5px; border: 1px solid var(--cat-border); border-radius: 6px; background: var(--cat-surface); color: var(--cat-text-2); padding: 0 4px; }
.ib { width: 24px; height: 24px; display: inline-flex; align-items: center; justify-content: center; border: 0; border-radius: 6px; background: transparent; color: var(--cat-muted); cursor: pointer; }
.ib:hover { background: var(--cat-hover); color: var(--cat-text); }
.ch-search { margin: 0 8px 4px; height: 28px; display: flex; align-items: center; gap: 6px; padding: 0 8px; border: 1px solid var(--cat-border); border-radius: 6px; background: var(--cat-surface); color: var(--cat-muted); }
.ch-search input { flex: 1; min-width: 0; border: 0; outline: 0; background: transparent; color: var(--cat-text); font: inherit; font-size: 12px; }
.ch-changed { display: block; width: 100%; height: 28px; border: 0; background: transparent; color: var(--cat-muted); font: inherit; font-size: 12px; text-align: left; padding: 0 12px; cursor: pointer; }
.ch-changed:hover { background: var(--cat-hover); }
.ch-state { padding: 24px 12px; text-align: center; color: var(--cat-muted); display: flex; gap: 8px; justify-content: center; }
.lnk { border: 0; background: transparent; color: var(--ff-primary); font: inherit; font-size: 12px; cursor: pointer; display: inline-flex; align-items: center; gap: 3px; padding: 0; }
.lnk:disabled { color: var(--cat-muted); cursor: default; }
.ch-list { padding: 0 4px 8px; }
.ch-row { height: 28px; display: flex; align-items: center; gap: 6px; padding-right: 8px; border-radius: 6px; cursor: pointer; color: var(--cat-text); }
.ch-row:hover, .ch-row.ctx { background: var(--cat-hover); }
.ch-row.sel { background: var(--cat-active); }
.g { flex: none; overflow: visible; }
.sub { flex: 1 1 auto; min-width: 40px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ref { flex: 0 1 auto; max-width: 180px; height: 18px; line-height: 16px; font-size: 11px; padding: 0 6px; border: 1px solid var(--cat-border-strong); border-radius: 9px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--cat-text-2); display: inline-flex; align-items: center; gap: 3px; }
.ref.k-remote { border-style: dashed; }
.ref.k-tag { background: var(--cat-warn-soft); border-color: transparent; color: var(--cat-orange); }
.ref.k-head { background: var(--ff-primary); border-color: var(--ff-primary); color: #fff; font-weight: 600; }
.au { flex: none; width: 44px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--cat-muted); }
.tm { flex: none; min-width: 52px; text-align: right; color: var(--cat-muted); white-space: nowrap; }
.ch-detail { margin: 2px 8px 6px 46px; padding: 10px 12px; border: 1px solid var(--cat-border); border-radius: 8px; background: var(--cat-surface); }
.d-sha { display: flex; align-items: center; gap: 6px; }
.d-sha code, .d-file code, h3 code { font-family: var(--ff-font-mono); font-size: 12px; }
.sp { flex: 1; }
.d-muted { color: var(--cat-muted); margin: 4px 0; }
.d-err { color: var(--ff-danger); font-size: 12px; margin: 0; }
.d-msg { white-space: pre-wrap; font: inherit; font-size: 12.5px; margin: 8px 0; color: var(--cat-text); }
.d-file { width: 100%; height: 26px; display: flex; align-items: center; gap: 8px; border: 0; border-radius: 6px; background: transparent; color: var(--cat-text); padding: 0 6px; cursor: pointer; font: inherit; text-align: left; }
.d-file:hover { background: var(--cat-hover); }
.d-file.static { cursor: default; }
.d-file code { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.d-file b { width: 12px; font-size: 11px; }
.s-M { color: var(--cat-orange); } .s-A { color: var(--ff-success); } .s-D { color: var(--ff-danger); } .s-R, .s-C, .s-T { color: var(--ff-primary); }
.d-num { color: var(--cat-muted); font-size: 11px; white-space: nowrap; }
.ch-more { height: 36px; display: flex; align-items: center; justify-content: center; gap: 8px; color: var(--cat-muted); }
.ch.narrow .ref { max-width: 110px; }
.ch-menu { position: fixed; z-index: 3000; width: 180px; padding: 4px; border-radius: 8px; background: var(--cat-elevated); box-shadow: var(--cat-shadow-lg); border: 1px solid var(--cat-border); }
.ch-menu button { width: 100%; height: 28px; border: 0; border-radius: 6px; background: transparent; color: var(--cat-text); font: inherit; font-size: 12.5px; text-align: left; padding: 0 10px; cursor: pointer; }
.ch-menu button:hover:not(:disabled) { background: var(--cat-hover); }
.ch-menu button:disabled { color: var(--cat-muted); cursor: default; }
.ch-menu hr { border: 0; border-top: 1px solid var(--cat-divider); margin: 4px 0; }
.ch-mask { position: fixed; inset: 0; z-index: 3001; background: rgba(0, 0, 0, 0.32); display: flex; align-items: center; justify-content: center; }
.ch-dlg { width: 400px; max-width: calc(100vw - 32px); padding: 20px; border-radius: 12px; background: var(--cat-elevated); color: var(--cat-text); box-shadow: var(--cat-shadow-lg); font-size: 13px; }
.ch-dlg.wide { width: 640px; }
.ch-dlg h3 { margin: 0 0 8px; font-size: 15px; font-weight: 600; }
.gray { color: var(--cat-muted); margin: 0 0 12px; line-height: 1.6; }
.inp { width: 100%; box-sizing: border-box; height: 32px; padding: 0 10px; border: 1px solid var(--cat-border); border-radius: 6px; background: var(--cat-surface); color: var(--cat-text); font: inherit; }
.ck { display: flex; align-items: center; gap: 6px; margin: 10px 0; color: var(--cat-text-2); }
.btns { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
.btn { height: 30px; padding: 0 14px; border: 1px solid var(--cat-border); border-radius: 6px; background: var(--cat-surface); color: var(--cat-text); font: inherit; cursor: pointer; }
.btn.primary { background: var(--ff-primary); border-color: var(--ff-primary); color: #fff; }
.btn:disabled { opacity: 0.6; cursor: default; }
.diff { max-height: 50vh; overflow: auto; margin: 0; padding: 8px; border: 1px solid var(--cat-border); border-radius: 6px; font-family: var(--ff-font-mono); font-size: 12px; line-height: 1.5; background: var(--cat-surface); }
.diff span { display: block; white-space: pre; }
.diff .add { background: color-mix(in srgb, var(--ff-success) 14%, transparent); }
.diff .del { background: color-mix(in srgb, var(--ff-danger) 14%, transparent); }
.diff .hunk { color: var(--ff-primary); }
.diff .meta { color: var(--cat-muted); }
</style>
