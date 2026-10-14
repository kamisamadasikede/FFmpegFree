<template>
  <!-- v0.51 分支树弹窗（契约 §6.19.26.5，线框 proto/cat-branch-tree）：从底栏分支胶囊向上弹 -->
  <Teleport to="body">
    <div v-if="open" ref="pop" class="bt-pop" :style="popStyle" role="dialog" :aria-label="C.ariaBranch" data-testid="cat-btree" @keydown.esc.stop="close()">
      <div class="bt-srch">
        <label class="bt-in">
          <FIcon name="search" :size="14" />
          <input ref="qEl" v-model="q" type="text" :placeholder="C.search" data-testid="cat-btree-search" />
        </label>
        <span v-if="tree?.fetching || fetching" class="bt-fetching" data-testid="cat-btree-fetching">{{ C.fetching }}</span>
        <button
          type="button"
          class="bt-ib"
          :aria-label="C.refreshTip"
          :disabled="!tree || fetching"
          data-testid="cat-btree-refresh"
          @mouseenter="tipAt($event, refreshTipText)"
          @mouseleave="tip = null"
          @click="doFetch(true)"
        ><FIcon name="refresh" :size="14" :class="{ spin: fetching || tree?.fetching }" /></button>
      </div>
      <div v-if="tree && tree.fetchError && !fetching && !tree.fetching" class="bt-ferr" data-testid="cat-btree-fetchfail">
        <div class="bt-ferr-line">
          <span>{{ C.fetchFailed }}</span>
          <button type="button" class="bt-more" :aria-expanded="ferrOpen" data-testid="cat-btree-fetchfail-more" @click="ferrOpen = !ferrOpen">
            <FIcon :name="ferrOpen ? 'down' : 'right'" :size="11" />{{ C.detail }}
          </button>
        </div>
        <CatRawDetail v-if="ferrOpen" :text="tree.fetchError" testid="cat-btree-fetchfail" />
      </div>
      <div class="bt-list" data-testid="cat-btree-list">
        <div v-if="loadErr" class="bt-empty" data-testid="cat-btree-empty">{{ loadErr }}</div>
        <div v-else-if="tree?.noCommits" class="bt-empty" data-testid="cat-btree-empty">{{ C.noCommits }}</div>
        <div v-else-if="tree && !rows.length && q.trim()" class="bt-empty" data-testid="cat-btree-empty">{{ C.noMatch }}</div>
        <template v-for="r in rows" :key="r.key">
          <button v-if="r.type === 'group'" type="button" class="bt-gt" :aria-expanded="r.open" @click="toggle(r.key)">
            <FIcon :name="r.open ? 'down' : 'right'" :size="12" />{{ r.label }}
          </button>
          <button v-else-if="r.type === 'folder'" type="button" class="bt-r" :style="{ paddingLeft: 10 + r.depth * 14 + 'px' }" :aria-expanded="r.open" @click="toggle(r.key)">
            <span class="bi"><FIcon :name="r.open ? 'down' : 'right'" :size="14" /></span>
            <span class="bi"><FIcon name="folder" :size="14" /></span>
            <span class="nm">{{ r.label }}</span>
          </button>
          <div
            v-else
            class="bt-r br"
            :class="{ cur: r.branch.current, act: menu && menu.key === r.key }"
            :style="{ paddingLeft: 10 + r.depth * 14 + 'px' }"
            role="button"
            tabindex="0"
            :data-testid="'cat-btree-row-' + r.key"
            @mouseenter="rowTip($event, r)"
            @mouseleave="tip = null"
            @contextmenu.prevent="openMenu($event, r)"
            @click="openMenu($event, r)"
            @keydown.enter.prevent="openMenu($event, r)"
          >
            <span v-if="r.branch.current" class="ck"><FIcon name="check" :size="14" /></span>
            <span v-else class="bi"><FIcon :name="r.branch.remote ? 'cloud' : 'branch'" :size="14" /></span>
            <span class="nm">{{ r.label }}</span>
            <span v-if="aheadBehindText(r.branch)" class="ab">{{ aheadBehindText(r.branch) }}</span>
            <button
              type="button"
              class="st"
              :class="{ f: r.branch.favorite }"
              :aria-label="r.branch.favorite ? C.unfavorite : C.favorite"
              :aria-pressed="r.branch.favorite"
              @click.stop="toggleFav(r.branch)"
            ><svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path d="m12 3 2.8 5.8 6.2.9-4.5 4.4 1 6.3L12 17.5 6.5 20.4l1-6.3L3 9.7l6.2-.9z" /></svg></button>
          </div>
        </template>
      </div>
      <div class="bt-sep" />
      <button type="button" class="bt-r foot" :disabled="!tree || !!loadErr || tree.noCommits" data-testid="cat-btree-merge" @click="doMerge(null)">
        <span class="bi"><svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true" class="ln"><circle cx="6" cy="5" r="2" /><circle cx="6" cy="19" r="2" /><circle cx="18" cy="12" r="2" /><path d="M6 7v10M6 7c0 4 5 5 10 5" /></svg></span>
        <span class="nm">{{ C.mergeFooter }}</span>
      </button>
    </div>

    <!-- 右键菜单 -->
    <div v-if="open && menu" ref="menuEl" class="bt-pop bt-menu" :style="{ left: menu.x + 'px', top: menu.y + 'px' }" role="menu" data-testid="cat-btree-menu">
      <template v-for="it in menu.items" :key="it.id">
        <div v-if="it.sepBefore" class="bt-sep" />
        <button
          type="button"
          role="menuitem"
          class="bt-r"
          :class="{ dis: it.disabled, del: it.danger }"
          :aria-disabled="it.disabled"
          :data-testid="'cat-btree-menu-' + it.id"
          @mouseenter="it.tip ? tipAt($event, it.tip, 'right') : null"
          @mouseleave="tip = null"
          @click="!it.disabled && act(it.id, menu.branch)"
        >{{ it.label }}</button>
      </template>
    </div>

    <div v-if="tip" class="bt-tip" :style="{ left: tip.x + 'px', top: tip.y + 'px' }" role="tooltip" data-testid="cat-btree-tip">{{ tip.text }}</div>

    <!-- 新建 / 重命名 / 删除弹窗 -->
    <div v-if="dlg" class="bt-scrim" @mousedown.self="!dlgBusy && (dlg = null)">
      <div class="bt-dlg" role="dialog" aria-modal="true" :data-testid="'cat-btree-dlg-' + dlg.kind" @keydown.esc.stop="!dlgBusy && (dlg = null)">
        <template v-if="dlg.kind === 'new' || dlg.kind === 'rename'">
          <h3>{{ dlg.kind === 'new' ? C.newTitle : C.renameTitle }}</h3>
          <div v-if="dlg.kind === 'new'" class="bt-sub">{{ C.newFromLine(branchRef(dlg.branch)) }}</div>
          <input
            ref="nameEl"
            v-model="dlg.name"
            class="bt-name"
            :class="{ bad: !!nameErr }"
            :placeholder="C.namePh"
            data-testid="cat-btree-name"
            @input="dlg.err = null"
            @keydown.enter.prevent="submitDlg"
          />
          <div v-if="nameErr" class="bt-err" data-testid="cat-btree-name-err">{{ nameErr }}</div>
          <label v-if="dlg.kind === 'new'" class="bt-chk"><input v-model="dlg.checkout" type="checkbox" />{{ C.checkoutAfter }}</label>
        </template>
        <template v-else>
          <h3>{{ C.delTitle(dlg.branch.name) }}</h3>
          <div v-if="dlg.unmerged" class="bt-red" data-testid="cat-btree-unmerged">{{ C.unmerged }}</div>
        </template>
        <div v-if="dlg.err && dlg.err.field !== 'name'" class="bt-err" data-testid="cat-btree-dlg-err">
          {{ dlg.err.text }}
          <button v-if="dlg.err.raw" type="button" class="bt-more" @click="dlgRawOpen = !dlgRawOpen"><FIcon :name="dlgRawOpen ? 'down' : 'right'" :size="11" />{{ C.detail }}</button>
          <CatRawDetail v-if="dlg.err.raw && dlgRawOpen" :text="dlg.err.raw" testid="cat-btree-dlg-err" />
        </div>
        <div class="bt-acts">
          <button type="button" class="bt-btn" :disabled="dlgBusy" @click="dlg = null">{{ C.cancel }}</button>
          <button
            v-if="dlg.kind === 'delete'"
            type="button"
            class="bt-btn dan"
            :disabled="dlgBusy"
            data-testid="cat-btree-dlg-ok"
            @click="submitDlg"
          >{{ dlg.unmerged ? C.delForce : C.delOk }}</button>
          <button v-else type="button" class="bt-btn pri" :disabled="dlgBusy || !canSubmitName" data-testid="cat-btree-dlg-ok" @click="submitDlg">
            {{ dlg.kind === 'new' ? C.create : C.renameOk }}
          </button>
        </div>
      </div>
    </div>

    <!-- 和当前分支比较 -->
    <div v-if="cmp" class="bt-cmp" role="dialog" aria-modal="true" data-testid="cat-btree-compare" @keydown.esc.stop="cmp = null">
      <div class="hd">
        <FIcon name="branch" :size="14" />
        <span class="tt">{{ C.compareTitle(branchRef(cmp.branch), cmp.data?.base || curName) }}</span>
        <span v-if="cmp.data && cmp.data.files.length" class="cnt">{{ C.compareFiles(cmp.data.files.length) }}</span>
        <button ref="cmpClose" type="button" class="bt-ib x" :aria-label="C.close" data-testid="cat-btree-compare-close" @click="cmp = null"><FIcon name="x" :size="14" /></button>
      </div>
      <div v-if="cmp.data && !cmp.data.files.length" class="bt-same" data-testid="cat-btree-compare-same">{{ C.compareSame }}</div>
      <div v-else-if="cmp.data" class="bd">
        <div class="fl">
          <button v-for="f in cmp.data.files" :key="f.path" type="button" class="bt-r" :class="{ sel: cmp.sel === f.path }" :title="f.oldPath ? f.oldPath + ' → ' + f.path : f.path" @click="pickFile(f.path)">
            <span class="tag" :class="'s' + f.status">{{ f.status }}</span><span class="nm">{{ f.path }}</span>
          </button>
        </div>
        <div class="df" data-testid="cat-btree-compare-diff">
          <div v-for="(l, i) in cmp.diff" :key="i" class="l" :class="l.kind"><span class="n">{{ l.no ?? '' }}</span>{{ l.kind === 'hunk' ? l.text : l.text }}</div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import CatRawDetail from './CatRawDetail.vue'
import { onEvent } from '@/services/wails'
import { catBranchError } from '@/api/catBranches'
import {
  aheadBehindText, agoText, branchMenuItems, branchRef, branchTreeError, buildTreeRows, CAT_BTREE_COPY as C, checkoutTreeBranch, compareFileDiff,
  currentBranch, fetchCatRemotes, gitCatCompareBranch, gitCatCreateBranch, gitCatDeleteBranch, gitCatRenameBranch, listCatBranchTree, localBranches,
  localNameError, parseUnifiedDiff, refreshTip, requestCatBranchMerge, setCatBranchFavorite, shouldAutoFetch,
  type BTreeAction, type BTreeErr, type BTreeMenuItem, type BTreeRow, type CatBranchCompare, type CatBranchTree, type CatTreeBranch, type DiffRow,
} from '@/api/catBranchTree'

const props = defineProps<{ open: boolean; convId: string; anchor: HTMLElement | null }>()
const emit = defineEmits<{ close: []; changed: [] }>()

const tree = ref<CatBranchTree | null>(null)
const loadErr = ref('')
const fetching = ref(false)
const ferrOpen = ref(false)
const q = ref('')
const collapsed = ref(new Set<string>())
const qEl = ref<HTMLInputElement>()
const pop = ref<HTMLElement>()
const menuEl = ref<HTMLElement>()
const nameEl = ref<HTMLInputElement>()
const popStyle = ref<Record<string, string>>({})
let seq = 0

const rows = computed<BTreeRow[]>(() => (tree.value && !loadErr.value ? buildTreeRows(tree.value, q.value, collapsed.value) : []))
const refreshTipText = computed(() => (tree.value ? refreshTip(tree.value) : C.refreshTip))
const curName = computed(() => (tree.value ? currentBranch(tree.value)?.name ?? '' : ''))

function place() {
  const a = props.anchor?.getBoundingClientRect()
  if (!a) return
  const left = Math.max(8, Math.min(a.left, window.innerWidth - 328))
  // 向上弹；上方放不下就向下
  if (a.top > 260) popStyle.value = { left: left + 'px', bottom: window.innerHeight - a.top + 6 + 'px', maxHeight: Math.min(560, a.top - 14) + 'px' }
  else popStyle.value = { left: left + 'px', top: a.bottom + 6 + 'px', maxHeight: Math.min(560, window.innerHeight - a.bottom - 14) + 'px' }
}

async function load() {
  const s = ++seq
  try {
    const t = await listCatBranchTree(props.convId)
    if (s !== seq) return
    tree.value = t
    loadErr.value = ''
  } catch (e) {
    if (s !== seq) return
    const er = branchTreeError(e)
    tree.value = null
    loadErr.value = er.text
  }
}

async function doFetch(force: boolean) {
  if (!props.convId || fetching.value) return
  if (!force && !shouldAutoFetch(props.convId)) return
  fetching.value = true
  ferrOpen.value = false
  const s = seq
  try {
    const t = await fetchCatRemotes(props.convId, force)
    if (s === seq && props.open) tree.value = t
  } catch {
    // FetchCatRemotes 失败不返回错误（写 fetchError）；这里只兜底：重拉一次本地
    void load()
  } finally {
    fetching.value = false
  }
}

let offEv: (() => void) | null = null
watch(
  () => props.open,
  async (v) => {
    if (!v) {
      offEv?.()
      offEv = null
      menu.value = null
      tip.value = null
      return
    }
    q.value = ''
    ferrOpen.value = false
    place()
    offEv = onEvent<{ convId?: string }>('cat:branchtree', (p) => { if (!p?.convId || p.convId === props.convId) void load() })
    await load()
    void nextTick(() => qEl.value?.focus())
    if (tree.value && !tree.value.noCommits) void doFetch(false)
  },
)

function toggle(key: string) {
  const s = new Set(collapsed.value)
  if (s.has(key)) s.delete(key)
  else s.add(key)
  collapsed.value = s
}

// ---- 悬停提示 ----
const tip = ref<{ text: string; x: number; y: number } | null>(null)
function tipAt(e: MouseEvent, text: string, side: 'below' | 'right' = 'below') {
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  tip.value = side === 'right' ? { text, x: r.right + 6, y: r.top + 2 } : { text, x: r.left, y: r.bottom + 6 }
}
function rowTip(e: MouseEvent, r: BTreeRow) {
  if (r.type !== 'branch' || !r.branch.lastCommit.time) return
  const el = (e.currentTarget as HTMLElement).getBoundingClientRect()
  tip.value = { text: C.lastCommitTip(branchRef(r.branch), agoText(r.branch.lastCommit.time)), x: Math.min(el.left + 180, window.innerWidth - 260), y: el.bottom + 4 }
}

// ---- 收藏 ----
async function toggleFav(b: CatTreeBranch) {
  const on = !b.favorite
  b.favorite = on
  try {
    await setCatBranchFavorite(props.convId, b, on)
    await load()
  } catch (e) {
    b.favorite = !on
    ElMessage.warning(branchTreeError(e).text)
  }
}

// ---- 右键菜单 ----
const menu = ref<{ key: string; branch: CatTreeBranch; items: BTreeMenuItem[]; x: number; y: number } | null>(null)
function openMenu(e: MouseEvent | KeyboardEvent, r: BTreeRow) {
  if (r.type !== 'branch') return
  const el = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const x = e instanceof MouseEvent && e.type === 'contextmenu' ? e.clientX : el.right - 24
  const y = e instanceof MouseEvent && e.type === 'contextmenu' ? e.clientY : el.top
  const items = branchMenuItems(r.branch)
  const h = items.length * 28 + (items.some((i) => i.sepBefore) ? 9 : 0) + 8
  menu.value = { key: r.key, branch: r.branch, items, x: Math.min(x, window.innerWidth - 208), y: Math.max(8, Math.min(y, window.innerHeight - h - 8)) }
  tip.value = null
}

async function act(id: BTreeAction, b: CatTreeBranch) {
  menu.value = null
  tip.value = null
  if (id === 'checkout') return checkout(b)
  if (id === 'merge') return doMerge(b)
  if (id === 'compare') return openCompare(b)
  if (id === 'new') dlg.value = { kind: 'new', branch: b, name: '', checkout: true, unmerged: false, err: null }
  if (id === 'rename') dlg.value = { kind: 'rename', branch: b, name: b.name, checkout: false, unmerged: false, err: null }
  if (id === 'delete') dlg.value = { kind: 'delete', branch: b, name: '', checkout: false, unmerged: false, err: null }
  dlgRawOpen.value = false
  void nextTick(() => { nameEl.value?.focus(); nameEl.value?.select() })
}

async function checkout(b: CatTreeBranch) {
  if (!tree.value || (b.current && !b.remote)) return
  try {
    const name = await checkoutTreeBranch(props.convId, tree.value, b)
    ElMessage.success(C.switched(name))
    emit('changed')
    close()
  } catch (e) {
    // 签出沿用 6.19.24 的切换文案
    const er = catBranchError(e)
    ElMessage.warning(er.text)
    void load()
  }
}

function doMerge(b: CatTreeBranch | null) {
  const ok = requestCatBranchMerge(props.convId, b ? { name: b.name, remote: b.remote } : null)
  if (!ok && import.meta.env.DEV) console.info('[cat-btree] merge entry: setCatBranchMergeHandler not registered', b)
  close()
}

// ---- 弹窗 ----
const dlg = ref<{ kind: 'new' | 'rename' | 'delete'; branch: CatTreeBranch; name: string; checkout: boolean; unmerged: boolean; err: BTreeErr | null } | null>(null)
const dlgBusy = ref(false)
const dlgRawOpen = ref(false)
const localNames = computed(() => (tree.value ? localBranches(tree.value).map((b) => b.name) : []))
const nameErr = computed(() => {
  const d = dlg.value
  if (!d || d.kind === 'delete') return ''
  if (d.err?.field === 'name') return d.err.text
  return localNameError(d.name, localNames.value, d.kind === 'rename' ? d.branch.name : '')
})
const canSubmitName = computed(() => {
  const d = dlg.value
  if (!d) return false
  return !!d.name && !nameErr.value && !(d.kind === 'rename' && d.name === d.branch.name)
})

async function submitDlg() {
  const d = dlg.value
  if (!d || dlgBusy.value) return
  if (d.kind !== 'delete' && !canSubmitName.value) return
  dlgBusy.value = true
  d.err = null
  try {
    if (d.kind === 'new') {
      tree.value = await gitCatCreateBranch(props.convId, branchRef(d.branch), d.name, d.checkout)
      ElMessage.success(d.checkout ? C.createdSwitched(d.name) : C.created(d.name))
    } else if (d.kind === 'rename') {
      tree.value = await gitCatRenameBranch(props.convId, d.branch.name, d.name)
      ElMessage.success(C.renamed(d.name))
    } else {
      tree.value = await gitCatDeleteBranch(props.convId, d.branch.name, d.unmerged)
      ElMessage.success(C.deleted(d.branch.name))
    }
    dlg.value = null
    emit('changed')
  } catch (e) {
    const er = branchTreeError(e)
    if (er.field === 'unmerged' && d.kind === 'delete') d.unmerged = true
    else d.err = er
  } finally {
    dlgBusy.value = false
  }
}

// ---- 比较 ----
const cmp = ref<{ branch: CatTreeBranch; data: CatBranchCompare | null; sel: string; diff: DiffRow[] } | null>(null)
const cmpClose = ref<HTMLButtonElement>()
async function openCompare(b: CatTreeBranch) {
  cmp.value = { branch: b, data: null, sel: '', diff: [] }
  close()
  try {
    const data = await gitCatCompareBranch(props.convId, b)
    if (cmp.value?.branch !== b) return
    cmp.value.data = data
    void nextTick(() => cmpClose.value?.focus())
    if (data.files.length) void pickFile(data.files[0].path)
  } catch (e) {
    cmp.value = null
    ElMessage.warning(branchTreeError(e).text)
  }
}
async function pickFile(path: string) {
  const c = cmp.value
  if (!c) return
  c.sel = path
  c.diff = []
  try {
    const d = await compareFileDiff(props.convId, c.branch, path)
    if (cmp.value === c && c.sel === path) c.diff = d ? parseUnifiedDiff(d) : []
  } catch {
    /* 右侧留空 */
  }
}

// ---- 关闭 ----
function close() {
  menu.value = null
  tip.value = null
  emit('close')
}
function onPointer(e: PointerEvent) {
  const t = e.target
  if (!(t instanceof Node) || !props.open) return
  if (menuEl.value?.contains(t)) return
  if (menu.value && !menuEl.value?.contains(t)) {
    menu.value = null
    if (pop.value?.contains(t)) return
  }
  if (pop.value?.contains(t) || props.anchor?.contains(t)) return
  if (dlg.value || cmp.value) return
  close()
}
document.addEventListener('pointerdown', onPointer, true)
window.addEventListener('resize', place)
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onPointer, true)
  window.removeEventListener('resize', place)
  offEv?.()
})
</script>

<style scoped>
.bt-pop {
  position: fixed;
  z-index: 2000;
  width: 320px;
  display: flex;
  flex-direction: column;
  background: var(--ff-bg-elevated);
  box-shadow: var(--ff-shadow-dialog);
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  font-size: 12px;
  color: var(--cat-text);
}
.bt-srch { display: flex; align-items: center; gap: 6px; padding: 8px; border-bottom: 1px solid var(--cat-divider); }
.bt-in {
  flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; height: 28px; padding: 0 8px;
  border: 1px solid var(--cat-border); border-radius: 6px; background: var(--cat-surface); color: var(--cat-muted);
}
.bt-in input { flex: 1; min-width: 0; border: none; outline: none; background: transparent; font: inherit; font-size: 12px; color: var(--cat-text); }
.bt-in input::placeholder { color: var(--cat-muted); }
.bt-in:focus-within { border-color: var(--ff-primary); }
.bt-fetching { font-size: 11px; color: var(--cat-muted); white-space: nowrap; }
.bt-ib {
  width: 28px; height: 28px; flex: none; border: none; border-radius: 6px; display: grid; place-items: center;
  background: transparent; color: var(--cat-text-2); cursor: pointer;
}
.bt-ib:hover:not(:disabled) { background: var(--cat-hover); }
.bt-ib:disabled { cursor: default; }
.spin { animation: bt-spin 0.9s linear infinite; }
@keyframes bt-spin { to { transform: rotate(360deg); } }
.bt-ferr { padding: 6px 10px 2px; font-size: 11px; color: var(--cat-muted); }
.bt-ferr-line { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.bt-more {
  display: inline-flex; align-items: center; gap: 2px; border: none; background: none; padding: 0;
  font: inherit; font-size: 11px; color: var(--ff-primary-text); cursor: pointer;
}
.bt-list { flex: 1; min-height: 0; overflow-y: auto; padding: 4px 0; }
.bt-gt {
  width: 100%; display: flex; align-items: center; gap: 4px; height: 24px; padding: 6px 10px 2px;
  border: none; background: none; font: inherit; font-size: 11px; color: var(--cat-muted); cursor: pointer; text-align: left;
}
.bt-r {
  width: 100%; display: flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px;
  border: none; background: none; font: inherit; font-size: 12px; color: var(--cat-text); text-align: left; cursor: pointer;
}
.bt-r:hover, .bt-r.act, .bt-r:focus-visible { background: var(--cat-hover); outline: none; }
.bt-r .nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bt-r.cur .nm { font-weight: 600; }
.bt-r .ck { display: inline-flex; color: var(--cat-active-text); }
.bt-r .bi { display: inline-flex; color: var(--cat-muted); }
.bt-r .ab { font-size: 11px; color: var(--cat-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
.bt-r .st {
  display: inline-flex; border: none; background: none; padding: 0; cursor: pointer;
  color: var(--cat-muted); visibility: hidden;
}
.bt-r .st svg { fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linejoin: round; }
.bt-r:hover .st, .bt-r:focus-within .st { visibility: visible; }
.bt-r .st.f { visibility: visible; color: var(--ff-warning); }
.bt-r .st.f svg { fill: currentColor; }
.bt-r .ln { fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.bt-r.foot { margin-bottom: 4px; }
.bt-r.foot .bi { color: var(--cat-text); }
.bt-r:disabled { cursor: default; color: var(--cat-muted); background: none; }
.bt-sep { height: 1px; background: var(--cat-divider); margin: 4px 0; flex: none; }
.bt-empty { padding: 28px 12px; text-align: center; color: var(--cat-muted); }
.bt-menu { width: 200px; padding: 4px 0; z-index: 2001; }
.bt-menu .bt-r { gap: 8px; }
.bt-menu .bt-r.dis { color: var(--cat-muted); cursor: default; }
.bt-menu .bt-r.del { color: var(--ff-danger-text); }
.bt-tip {
  position: fixed; z-index: 2010; pointer-events: none; background: var(--ff-text-1); color: var(--ff-bg-surface);
  font-size: 12px; padding: 4px 8px; border-radius: 6px; white-space: nowrap;
}
:global(html.dark) .bt-tip { background: #3a3d45; color: var(--ff-text-1); }
.bt-scrim { position: fixed; inset: 0; z-index: 2020; background: var(--cat-scrim); display: grid; place-items: center; }
.bt-dlg { width: 400px; background: var(--ff-bg-elevated); box-shadow: var(--ff-shadow-dialog); border-radius: 12px; padding: 20px; color: var(--cat-text); font-size: 13px; }
.bt-dlg h3 { font-size: 14px; font-weight: 600; margin: 0 0 14px; overflow-wrap: anywhere; }
.bt-sub { font-size: 12px; color: var(--cat-text-2); margin-bottom: 6px; overflow-wrap: anywhere; }
.bt-name {
  width: 100%; box-sizing: border-box; height: 32px; padding: 0 10px; border: 1px solid var(--cat-border-strong); border-radius: 6px;
  background: var(--cat-surface); color: var(--cat-text); font: inherit; font-size: 13px; outline: none;
}
.bt-name:focus { border-color: var(--ff-primary); }
.bt-name.bad { border-color: var(--ff-danger); }
.bt-err { color: var(--ff-danger); font-size: 12px; margin-top: 4px; }
.bt-red { color: var(--ff-danger); font-size: 13px; }
.bt-chk { display: flex; gap: 6px; align-items: center; margin-top: 12px; font-size: 13px; cursor: pointer; }
.bt-chk input { width: 14px; height: 14px; margin: 0; accent-color: var(--ff-primary); }
.bt-acts { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
.bt-btn {
  height: 32px; padding: 0 14px; border-radius: 6px; font: inherit; font-size: 13px; cursor: pointer;
  border: 1px solid var(--cat-border-strong); background: var(--cat-surface); color: var(--cat-text);
}
.bt-btn.pri { background: var(--ff-primary); border-color: var(--ff-primary); color: var(--ff-on-primary); }
.bt-btn.dan { background: var(--ff-danger); border-color: var(--ff-danger); color: #fff; }
.bt-btn:disabled { opacity: 0.45; cursor: default; }
.bt-cmp { position: fixed; inset: 0; z-index: 2015; display: flex; flex-direction: column; background: var(--cat-bg); color: var(--cat-text); font-size: 13px; }
.bt-cmp .hd { height: 44px; flex: none; display: flex; align-items: center; gap: 8px; padding: 0 8px 0 16px; border-bottom: 1px solid var(--cat-border); font-weight: 600; }
.bt-cmp .tt { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bt-cmp .cnt { margin-left: auto; font-weight: 400; font-size: 12px; color: var(--cat-muted); }
.bt-cmp .x { margin-left: 8px; }
.bt-cmp .hd .tt + .x { margin-left: auto; }
.bt-cmp .bd { flex: 1; display: flex; min-height: 0; }
.bt-cmp .fl { width: 240px; flex: none; border-right: 1px solid var(--cat-border); padding: 6px 0; overflow-y: auto; }
.bt-cmp .fl .bt-r.sel { background: var(--cat-active); }
.bt-cmp .tag { font: 600 10px var(--ff-font-mono); width: 14px; text-align: center; color: var(--ff-warning); }
.bt-cmp .tag.sA { color: var(--ff-success); }
.bt-cmp .tag.sD { color: var(--ff-danger); }
.bt-cmp .df { flex: 1; min-width: 0; overflow: auto; font: 12px/20px var(--ff-font-mono); padding: 8px 0; }
.bt-cmp .df .l { display: flex; white-space: pre; }
.bt-cmp .df .n { width: 44px; flex: none; text-align: right; padding-right: 10px; color: var(--cat-muted); }
.bt-cmp .df .add { background: color-mix(in srgb, var(--ff-success) 14%, transparent); }
.bt-cmp .df .del { background: color-mix(in srgb, var(--ff-danger) 12%, transparent); }
.bt-cmp .df .hunk { color: var(--cat-muted); background: var(--cat-sunken); }
.bt-cmp .df .hunk .n { width: 54px; }
.bt-same { flex: 1; display: grid; place-items: center; color: var(--cat-muted); }
</style>
