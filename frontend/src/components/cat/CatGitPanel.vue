<template>
  <div class="git" data-testid="cat-git">
    <div class="git-top">
      <button
        type="button"
        class="pr"
        :disabled="prDisabled"
        :title="prTitle"
        data-testid="cat-git-pr"
        @click="createPR"
      >
        <FIcon name="branch" :size="13" />
        <span>{{ COPY.pr }}</span>
      </button>
      <span class="sp" />
      <button type="button" class="ib" :class="{ on: filtering }" :title="COPY.filter" data-testid="cat-git-filter" @click="filtering = !filtering">
        <FIcon name="search" :size="15" />
      </button>
      <button type="button" class="ib" :title="COPY.refresh" :disabled="loading" data-testid="cat-git-refresh" @click="emit('refresh')">
        <FIcon name="refresh" :size="15" />
      </button>
    </div>
    <label v-if="filtering" class="git-filter">
      <FIcon name="search" :size="13" />
      <input v-model="filter" type="search" :placeholder="COPY.filter" />
    </label>
    <p v-if="loadError && status" class="git-err" role="alert">{{ loadError }}</p>
    <div v-if="loading && !status" class="git-empty" role="status">正在加载…</div>
    <div v-else-if="loadError && !status" class="git-empty" role="alert">
      <span>{{ loadError }}</span>
      <button type="button" class="retry" @click="emit('refresh')">重试</button>
    </div>
    <div v-else-if="!status?.repo" class="git-empty">{{ COPY.notRepo }}</div>
    <template v-else>
      <div class="git-branch-row">
        <button
          ref="branchBtn"
          type="button"
          class="branch-pill"
          :class="{ open: branchMenuOpen }"
          data-testid="cat-git-branch-btn"
          :aria-expanded="branchMenuOpen"
          aria-haspopup="listbox"
          @click.stop="toggleBranchMenu"
        >
          <FIcon name="branch" :size="13" />
          <span class="bn">{{ status.branch || 'HEAD' }}</span>
          <span v-if="status.ahead" class="sync up">↑{{ status.ahead }}</span>
          <span v-if="status.behind" class="sync dn">↓{{ status.behind }}</span>
          <FIcon name="down" :size="12" class="cv" />
        </button>
        <button v-if="status.web" type="button" class="ib" :title="COPY.openRemote" @click="openRemote">
          <FIcon name="external" :size="14" />
        </button>
        <div v-if="branchMenuOpen" ref="branchMenuEl" class="branch-menu" role="listbox" data-testid="cat-git-branch-menu" @click.stop>
          <label class="bm-search">
            <FIcon name="search" :size="13" />
            <input
              ref="branchFilterEl"
              v-model="branchFilter"
              type="search"
              :placeholder="CF.branchFilter"
              @keydown.enter.prevent="onBranchEnter"
            />
          </label>
          <div class="bm-sec">{{ CF.localSec }}</div>
          <button
            v-for="b in localBranches"
            :key="'L:' + b.name"
            type="button"
            role="option"
            class="bm-item"
            :class="{ cur: b.current }"
            :aria-selected="b.current"
            :disabled="busy || b.current"
            @click="checkoutBranch(b.name)"
          >
            <FIcon name="branch" :size="13" />
            <span class="nm">{{ b.name }}</span>
            <FIcon v-if="b.current" name="check" :size="13" class="ck" />
          </button>
          <template v-if="remoteBranches.length">
            <hr />
            <div class="bm-sec">{{ CF.remoteSec }}</div>
            <div v-for="b in remoteBranches" :key="'R:' + b.name" class="bm-item remote" role="option" aria-disabled="true">
              <FIcon name="branch" :size="13" />
              <span class="nm">{{ b.name }}</span>
              <span class="meta">↓</span>
            </div>
          </template>
        </div>
      </div>
      <textarea v-model="message" class="git-msg" rows="3" :placeholder="COPY.message" data-testid="cat-git-message" />
      <p v-if="actionError" class="git-err" role="alert">{{ actionError }}</p>
      <div class="commit-bar">
        <button type="button" class="stage main" :disabled="commitDisabled" data-testid="cat-git-primary" @click="doCommit()">
          {{ COPY.commit }}
        </button>
        <button
          ref="moreBtn"
          type="button"
          class="stage more"
          :disabled="busy"
          :aria-expanded="menuOpen"
          aria-haspopup="menu"
          :title="t('cat.misc.s_0ec9eaf9c3')"
          data-testid="cat-git-more"
          @click.stop="menuOpen = !menuOpen"
        >
          <FIcon name="down" :size="14" />
        </button>
        <div v-if="menuOpen" ref="menuEl" class="git-menu" role="menu" @click.stop>
          <template v-for="(e, i) in menuEntries" :key="i">
            <hr v-if="e.kind === 'sep'" />
            <button
              v-else
              type="button"
              role="menuitem"
              :disabled="e.disabled || busy"
              :title="e.title || undefined"
              :class="{ danger: e.danger }"
              @click="onMenu(e.id)"
            >{{ e.label }}</button>
          </template>
        </div>
      </div>

      <div v-if="inConflict" class="merge-bar" :class="{ rebase: status.mergeState === 'rebasing' }" data-testid="cat-git-merge-bar">
        <FIcon name="warn" :size="14" />
        <b>{{ labels.bar }}</b>
        <span class="meta">{{ labels.meta }}</span>
      </div>

      <div class="groups">
        <template v-if="inConflict">
          <div class="sec conflict-sec">
            <button type="button" class="sec-h" @click="toggleOpen('conflicts')">
              <FIcon :name="isOpen('conflicts') ? 'down' : 'right'" :size="12" />
              <span>{{ CF.group }}</span>
              <span class="cnt" :class="{ zero: unresolved.length === 0 }">{{ unresolved.length }}</span>
            </button>
          </div>
          <div v-if="isOpen('conflicts')" class="cf-list" data-testid="cat-git-conflicts">
            <div
              v-for="c in conflictRows"
              :key="c.path"
              class="cf-row"
              :class="{ on: selectedConflict === c.path, resolved: c.resolved }"
              data-testid="cat-git-conflict-row"
            >
              <button type="button" class="cf-top" :title="c.path" @click="openConflict(c.path)">
                <FIcon name="file" :size="14" class="fi" />
                <span class="path">{{ c.path }}</span>
                <span class="uu" :class="{ ok: c.resolved }">{{ c.resolved ? 'OK' : c.letter }}</span>
              </button>
              <div v-if="c.resolved" class="cf-acts">
                <span class="st ok"><FIcon name="check" :size="12" />{{ CF.resolved }}</span>
              </div>
              <div v-else class="cf-acts">
                <button type="button" class="btn" :disabled="busy || !c.ours" data-testid="cat-git-ours" @click="resolveOne(c.path, 'ours')">{{ CF.ours }}</button>
                <button type="button" class="btn" :disabled="busy || !c.theirs" data-testid="cat-git-theirs" @click="resolveOne(c.path, 'theirs')">{{ CF.theirs }}</button>
                <button type="button" class="btn" :disabled="busy" data-testid="cat-git-both" @click="resolveOne(c.path, 'both')">{{ CF.both }}</button>
              </div>
            </div>
            <div v-if="unresolved.length" class="note-2nd">
              <FIcon name="info" :size="13" />
              <span>{{ CF.phaseNote }}</span>
            </div>
          </div>
          <div class="cf-foot" data-testid="cat-git-conflict-foot">
            <div class="note" :class="{ clear: unresolved.length === 0 }">
              <template v-if="unresolved.length === 0"><FIcon name="check" :size="13" />{{ labels.allClear }}</template>
              <template v-else>{{ CF.needResolve(unresolved.length) }}</template>
            </div>
            <div v-if="status.mergeState === 'merging' || status.mergeState === 'cherry'" class="fld">
              <label>{{ CF.messageLabel }}</label>
              <input v-model="mergeMessage" class="inp" type="text" data-testid="cat-git-merge-msg" />
            </div>
            <div class="rowbtns">
              <button type="button" class="btn abort" :disabled="busy" data-testid="cat-git-abort" @click="abortConflict">{{ labels.abort }}</button>
              <button type="button" class="btn pri" :disabled="busy || unresolved.length > 0" data-testid="cat-git-complete" @click="completeConflict">{{ labels.complete }}</button>
            </div>
          </div>
        </template>

        <div v-if="!sections.length && !inConflict" class="git-empty flat">{{ COPY.empty }}</div>
        <div v-for="g in sections" :key="g.id">
          <div class="sec">
            <button type="button" class="sec-h" @click="toggleOpen(g.id)">
              <FIcon :name="isOpen(g.id) ? 'down' : 'right'" :size="12" />
              <span>{{ g.title }} {{ g.items.length }}</span>
            </button>
            <button
              type="button"
              class="ib"
              :title="g.act === 'unstage' ? COPY.unstage : COPY.stage"
              :disabled="busy"
              @click="g.act === 'unstage' ? unstagePaths(g.items) : stagePaths(g.items)"
            >
              <FIcon :name="g.act === 'unstage' ? 'minus' : 'plus'" :size="14" />
            </button>
          </div>
          <div v-if="isOpen(g.id)" class="list">
            <button
              v-for="c in g.items"
              :key="g.id + ':' + c.path"
              type="button"
              class="row"
              :title="c.path"
              @click="emit('open', c.path, c.name)"
            >
              <FIcon name="file" :size="14" class="fi" />
              <span class="nm" :class="'st-' + c.letter">{{ c.name }}</span>
              <span v-if="c.dir" class="dir">{{ c.dir }}</span>
              <span class="stat">
                <span v-if="c.added" class="add">+{{ c.added }}</span>
                <span v-if="c.deleted" class="del">−{{ c.deleted }}</span>
                <span class="letter" :class="'st-' + c.letter">{{ c.letter }}</span>
              </span>
              <span
                class="act"
                :title="g.act === 'unstage' ? COPY.unstage : COPY.stage"
                @click.stop="g.act === 'unstage' ? unstagePaths([c]) : stagePaths([c])"
              >
                <FIcon :name="g.act === 'unstage' ? 'x' : 'plus'" :size="12" />
              </span>
            </button>
          </div>
        </div>
        <CatCommitHistory ref="historyEl" :conv-id="convId" :default-open="simHistoryOpen" @changed="emit('refresh')" />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessageBox } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import CatCommitHistory from './CatCommitHistory.vue'
import { catSim, type CatSimFlag } from '@/views/cat/catDevSim'
import {
  CAT_GIT_CONFLICT_COPY as CF,
  CAT_GIT_COPY as COPY,
  catGitError,
  conflictLabels,
  filterBranchItems,
  gitBranchMenuItems,
  gitCatBranch,
  gitCatCheckout,
  gitCatCommit,
  gitCatConflictAbort,
  gitCatConflictComplete,
  gitCatConflictResolve,
  gitCatPullRequest,
  gitCatRemote,
  gitCatStage,
  gitCatUnstage,
  gitChangeGroups,
  gitMenuEntries,
  isGitConflictError,
  isGitDirtyError,
  SIM_GIT_BRANCHES,
  type GitChange,
  type GitConflictMode,
  type GitStatus,
} from '@/api/catGit'
import { openExternal } from '@/utils/openExternal'
import { notify } from '@/views/cat/catState'
import { catBranchMenuOpen, openCatBranchMenu, closeCatBranchMenu } from './catBranchMenu'

const props = defineProps<{
  convId: string
  live: boolean
  status: GitStatus | null
  loading?: boolean
  loadError?: string
  /** 浏览器走查时补全分支列表 */
  simBranches?: boolean
}>()

const emit = defineEmits<{
  refresh: []
  open: [rel: string, name: string]
  /** 打开冲突文件（工作区正文，手改后 Resolve content） */
  'open-conflict': [rel: string, name: string]
  /** pull/rebase 遇冲突：请父级切到 Git 页签 */
  'git-conflict': []
}>()

const message = ref('')
const historyEl = ref<{ reload: () => void } | null>(null)
/** 走查：?cat_sim=gitgraph… 时「提交历史」默认展开（正式默认折叠） */
const simHistoryOpen = import.meta.env.DEV && ['gitgraph', 'gitgraphempty', 'gitgraphchanged', 'gitgraphloadfail', 'gitgraphopen'].some((k) => catSim.has(k as CatSimFlag))
const mergeMessage = ref('')
const filter = ref('')
const filtering = ref(false)
const opened = ref<Record<string, boolean>>({ work: true, staged: true, untracked: true, conflicts: true })
const busy = ref(false)
const actionError = ref('')
const menuOpen = ref(false)
const moreBtn = ref<HTMLButtonElement>()
const menuEl = ref<HTMLElement>()
const branchBtn = ref<HTMLButtonElement>()
const branchMenuEl = ref<HTMLElement>()
const branchFilterEl = ref<HTMLInputElement>()
const branchFilter = ref('')
const selectedConflict = ref('')
const resolvedLocal = ref<Set<string>>(new Set())

const branchMenuOpen = computed(() => catBranchMenuOpen.value === 'panel')

watch(
  () => props.status?.mergeState,
  (s) => {
    if (s && s !== 'none') {
      const tip = props.status?.upstream || props.status?.branch || '…'
      mergeMessage.value = CF.defaultMergeMsg(tip.includes('/') ? tip.split('/').pop() || tip : tip)
      resolvedLocal.value = new Set()
    }
  },
  { immediate: true },
)

watch(
  () => props.status?.conflicts,
  () => {
    // 后端刷新后清掉本地已解决缓存里已不在列表中的
    const paths = new Set((props.status?.conflicts ?? []).map((c) => c.path))
    const next = new Set<string>()
    for (const p of resolvedLocal.value) {
      if (!paths.has(p)) next.add(p) // 已从 conflicts 消失 = 真解决
    }
    // 仍在 conflicts 里的本地标记保留到刷新拿掉
    for (const p of resolvedLocal.value) {
      if (paths.has(p)) next.add(p)
    }
    // 实际：仍在 conflicts = 未解决；离开 = 已解决。本地 set 只用于乐观 UI。
    resolvedLocal.value = new Set([...resolvedLocal.value].filter((p) => paths.has(p)))
  },
)

const inConflict = computed(() => !!props.status?.repo && props.status.mergeState !== 'none')
const unresolved = computed(() => (props.status?.conflicts ?? []).filter((c) => !resolvedLocal.value.has(c.path)))
const labels = computed(() => conflictLabels(props.status?.mergeState ?? 'none'))

const conflictRows = computed(() => {
  const list = props.status?.conflicts ?? []
  return list.map((c) => ({
    ...c,
    resolved: resolvedLocal.value.has(c.path),
  }))
})

const branchItems = computed(() => {
  const st = props.status
  if (!st) return []
  const extra = props.simBranches
    ? { locals: [...SIM_GIT_BRANCHES.locals], remotes: [...SIM_GIT_BRANCHES.remotes] }
    : {}
  return gitBranchMenuItems({
    branch: st.branch,
    upstream: st.upstream,
    ...extra,
  })
})

const filteredBranches = computed(() => filterBranchItems(branchItems.value, branchFilter.value))
const localBranches = computed(() => filteredBranches.value.filter((b) => b.kind === 'local'))
const remoteBranches = computed(() => filteredBranches.value.filter((b) => b.kind === 'remote'))

const shown = computed(() => {
  const list = props.status?.changes ?? []
  const q = filter.value.trim().toLowerCase()
  if (!filtering.value || !q) return list
  return list.filter((c) => c.path.toLowerCase().includes(q) || c.name.toLowerCase().includes(q))
})

const sections = computed(() => {
  const g = gitChangeGroups(shown.value)
  const out: { id: 'work' | 'staged' | 'untracked'; title: string; items: GitChange[]; act: 'stage' | 'unstage' }[] = []
  if (g.work.length) out.push({ id: 'work', title: COPY.changes, items: g.work, act: 'stage' })
  if (g.staged.length) out.push({ id: 'staged', title: COPY.staged, items: g.staged, act: 'unstage' })
  if (g.untracked.length) out.push({ id: 'untracked', title: COPY.untracked, items: g.untracked, act: 'stage' })
  return out
})

function isOpen(id: string) {
  return opened.value[id] !== false
}
function toggleOpen(id: string) {
  opened.value = { ...opened.value, [id]: !isOpen(id) }
}

const prDisabled = computed(() => busy.value || !props.live || !props.status?.repo || !props.status.web)
const prTitle = computed(() => {
  if (!props.status?.repo) return COPY.notRepo
  return props.status.web ? COPY.pr : COPY.notGitHub
})
const commitDisabled = computed(() => busy.value || !props.status?.repo || !message.value.trim() || !props.status.changes.length || inConflict.value)
const menuEntries = computed(() => gitMenuEntries({
  message: message.value,
  changeCount: props.status?.changes.length ?? 0,
  upstream: props.status?.upstream ?? '',
  web: props.status?.web ?? '',
}))

async function run(fn: () => Promise<void>, opts?: { quietConflict?: boolean }) {
  if (!props.live) {
    notify(COPY.preview, 'warn')
    return
  }
  if (busy.value) return
  busy.value = true
  actionError.value = ''
  try {
    await fn()
  } catch (e) {
    if (isGitConflictError(e)) {
      actionError.value = CF.conflictNeed
      emit('git-conflict')
      if (!opts?.quietConflict) notify(CF.conflictNeed, 'warn')
    } else {
      actionError.value = catGitError(e)
    }
  } finally {
    busy.value = false
    emit('refresh')
  }
}

function createPR() {
  void run(async () => {
    const url = await gitCatPullRequest(props.convId)
    if (url) openExternal(url)
  })
}

function openRemote() {
  if (props.status?.web) openExternal(props.status.web)
}

function toggleBranchMenu() {
  if (branchMenuOpen.value) closeCatBranchMenu()
  else {
    branchFilter.value = ''
    openCatBranchMenu('panel')
    void nextTick(() => branchFilterEl.value?.focus())
  }
}

function onBranchEnter() {
  const q = branchFilter.value.trim()
  if (!q) return
  const hit = localBranches.value.find((b) => b.name === q) || localBranches.value[0]
  if (hit && !hit.current) void checkoutBranch(hit.name)
}

async function checkoutBranch(name: string) {
  closeCatBranchMenu()
  if (!props.live) {
    notify(COPY.preview, 'warn')
    return
  }
  if (busy.value) return
  busy.value = true
  actionError.value = ''
  try {
    await gitCatCheckout(props.convId, name, false)
  } catch (e) {
    if (isGitDirtyError(e)) {
      busy.value = false
      try {
        await ElMessageBox.confirm(CF.dirtyBody(name), CF.dirtyTitle, {
          confirmButtonText: CF.dirtyOk,
          cancelButtonText: CF.dirtyCancel,
          type: 'warning',
          distinguishCancelAndClose: true,
        })
      } catch {
        return
      }
      busy.value = true
      try {
        await gitCatCheckout(props.convId, name, true)
      } catch (e2) {
        actionError.value = catGitError(e2)
      }
    } else {
      actionError.value = catGitError(e)
    }
  } finally {
    busy.value = false
    emit('refresh')
  }
}

function onPointerDown(e: PointerEvent) {
  const t = e.target
  if (!(t instanceof Node)) return
  if (moreBtn.value?.contains(t) || menuEl.value?.contains(t)) return
  menuOpen.value = false
  if (branchBtn.value?.contains(t) || branchMenuEl.value?.contains(t)) return
  if (branchMenuOpen.value) closeCatBranchMenu()
}
onMounted(() => document.addEventListener('pointerdown', onPointerDown))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onPointerDown))

async function commitOnly() {
  const msg = message.value.trim()
  await gitCatCommit(props.convId, msg)
  message.value = ''
}

function doCommit() {
  void run(() => commitOnly())
}

async function afterCommit(op: string) {
  await commitOnly()
  await gitCatRemote(props.convId, op)
}

function remote(op: string) {
  menuOpen.value = false
  void run(() => gitCatRemote(props.convId, op))
}

async function onMenu(id: string) {
  menuOpen.value = false
  if (id === 'commit') {
    doCommit()
    return
  }
  if (id === 'commit-push') {
    void run(() => afterCommit('push'))
    return
  }
  if (id === 'commit-sync') {
    void run(() => afterCommit('sync'))
    return
  }
  if (id === 'force') {
    try {
      await ElMessageBox.confirm(COPY.forceAsk, COPY.force, { confirmButtonText: t('cat.misc.s_27ca568be2'), cancelButtonText: t('cat.misc.s_625fb26b4b'), type: 'warning' })
    } catch {
      return
    }
    remote('force')
    return
  }
  if (id === 'pr') {
    createPR()
    return
  }
  if (id === 'branch') {
    let name = ''
    try {
      const r = await ElMessageBox.prompt(COPY.branchAsk, COPY.newBranch, { confirmButtonText: t('cat.misc.s_bec7e4d621'), cancelButtonText: t('cat.misc.s_625fb26b4b') })
      name = r.value.trim()
    } catch {
      return
    }
    if (!name) return
    void run(() => gitCatBranch(props.convId, name))
    return
  }
  if (id === 'push-pr') {
    void run(async () => {
      await gitCatRemote(props.convId, 'push')
      const url = await gitCatPullRequest(props.convId)
      if (url) openExternal(url)
    })
    return
  }
  const op: Record<string, string> = { push: 'push', pull: 'pull', ff: 'ff', sync: 'sync', rebase: 'rebase', fetch: 'fetch', publish: 'publish' }
  if (op[id]) remote(op[id])
}

function stagePaths(items: GitChange[]) {
  if (!items.length) return
  void run(() => gitCatStage(props.convId, items.map((c) => c.path)))
}

function unstagePaths(items: GitChange[]) {
  if (!items.length) return
  void run(() => gitCatUnstage(props.convId, items.map((c) => c.path)))
}

function conflictFileName(path: string) {
  const slash = path.replace(/\\/g, '/')
  const i = slash.lastIndexOf('/')
  return i >= 0 ? slash.slice(i + 1) : slash
}

function openConflict(path: string) {
  selectedConflict.value = path
  emit('open-conflict', path, conflictFileName(path))
}

function resolveOne(path: string, mode: Exclude<GitConflictMode, 'content'>) {
  selectedConflict.value = path
  void run(async () => {
    await gitCatConflictResolve(props.convId, path, mode)
    resolvedLocal.value = new Set([...resolvedLocal.value, path])
  })
}

function completeConflict() {
  if (unresolved.value.length) return
  void run(async () => {
    await gitCatConflictComplete(props.convId, mergeMessage.value.trim())
    resolvedLocal.value = new Set()
  })
}

async function abortConflict() {
  try {
    await ElMessageBox.confirm(labels.value.abortBody, labels.value.abortTitle, {
      confirmButtonText: CF.abortOk,
      cancelButtonText: CF.abortCancel,
      type: 'warning',
      // EP 默认 confirmButtonType=primary（蓝）；危险确认须显式 danger（同删除项目实心红）
      confirmButtonType: 'danger',
      confirmButtonClass: 'el-button--danger',
      distinguishCancelAndClose: true,
    })
  } catch {
    return
  }
  void run(async () => {
    await gitCatConflictAbort(props.convId)
    resolvedLocal.value = new Set()
  })
}

defineExpose({
  openBranchMenu: () => {
    branchFilter.value = ''
    openCatBranchMenu('panel')
  },
  checkoutBranch,
})
</script>

<style scoped>
.git {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.git-top {
  height: 40px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
}
.sp { flex: 1; }
.pr {
  height: 26px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 10px;
  border: none;
  border-radius: 999px;
  background: var(--cat-text);
  color: var(--cat-surface);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.pr:disabled {
  opacity: 0.4;
  cursor: default;
}
.ib {
  width: 24px;
  height: 24px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 6px;
  padding: 0;
  background: transparent;
  color: var(--cat-text-2);
  cursor: pointer;
}
.ib:hover:not(:disabled),
.ib.on {
  background: var(--cat-hover);
  color: var(--cat-text);
}
.ib:disabled {
  opacity: 0.4;
  cursor: default;
}
.git-filter {
  margin: 0 10px 8px;
  height: 28px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border-radius: 8px;
  background: var(--cat-hover);
  color: var(--cat-muted);
}
.git-filter input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 12px;
  color: var(--cat-text);
}
.git-branch-row {
  position: relative;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 10px 8px;
}
.branch-pill {
  flex: 1;
  min-width: 0;
  height: 30px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-surface);
  color: var(--cat-text);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.branch-pill:hover,
.branch-pill.open {
  background: var(--cat-hover);
}
.branch-pill .bn {
  flex: 1;
  min-width: 0;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.branch-pill .sync { font-size: 11px; font-weight: 650; }
.branch-pill .up { color: var(--cat-success-text); }
.branch-pill .dn { color: var(--cat-danger-text); }
.branch-pill .cv { color: var(--cat-muted); flex: none; }
.branch-menu {
  position: absolute;
  top: calc(100% - 2px);
  left: 10px;
  right: 10px;
  z-index: 8;
  max-height: min(360px, 55vh);
  overflow-y: auto;
  padding: 6px;
  border: 1px solid var(--cat-border);
  border-radius: 10px;
  background: var(--cat-elevated);
  box-shadow: var(--cat-shadow-lg);
  animation: cat-pop-in var(--ff-dur-base) var(--ff-ease-out) both;
}
.bm-search {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 8px;
  margin-bottom: 4px;
  border-radius: 8px;
  background: var(--cat-hover);
  color: var(--cat-muted);
}
.bm-search input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 12px;
  color: var(--cat-text);
}
.bm-sec {
  padding: 6px 8px 2px;
  font-size: 11px;
  font-weight: 650;
  color: var(--cat-muted);
}
.bm-item {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--cat-text);
  font: inherit;
  font-size: 12.5px;
  text-align: left;
  cursor: pointer;
}
.bm-item:hover:not(:disabled) { background: var(--cat-hover); }
.bm-item:disabled { cursor: default; }
.bm-item.cur { font-weight: 650; }
.bm-item.remote { opacity: 0.75; cursor: default; }
.bm-item .nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bm-item .ck { color: var(--cat-success-text); flex: none; }
.bm-item .meta { color: var(--cat-muted); font-size: 11px; }
.branch-menu hr {
  border: none;
  border-top: 1px solid var(--cat-border);
  margin: 4px 2px;
}
.git-msg {
  margin: 0 10px;
  min-height: 64px;
  resize: none;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-surface);
  color: var(--cat-text);
  font: inherit;
  font-size: 13px;
  line-height: 1.4;
  padding: 8px;
}
.git-msg::placeholder { color: var(--cat-muted); }
.git-err {
  margin: 6px 12px 0;
  font-size: 12px;
  line-height: 1.4;
  color: var(--cat-danger-text);
}
.commit-bar {
  position: relative;
  margin: 8px 10px 0;
  display: flex;
}
.stage {
  height: 30px;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-surface);
  color: var(--cat-text);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.commit-bar .stage { margin: 0; }
.commit-bar .main {
  flex: 1;
  border-radius: 8px 0 0 8px;
  border-right: none;
}
.commit-bar .more {
  width: 32px;
  border-radius: 0 8px 8px 0;
  display: grid;
  place-items: center;
  color: var(--cat-text-2);
}
.git-menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  z-index: 6;
  max-height: min(420px, 70vh);
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-elevated);
  box-shadow: var(--cat-shadow-lg);
  animation: cat-pop-in var(--ff-dur-base) var(--ff-ease-out) both;
  transform-origin: top center;
}
.git-menu button {
  display: block;
  width: 100%;
  text-align: left;
  border: none;
  background: transparent;
  padding: 6px 8px;
  border-radius: 6px;
  font: inherit;
  font-size: 12.5px;
  color: var(--cat-text);
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.git-menu button:hover:not(:disabled) { background: var(--cat-hover); }
.git-menu button:disabled { opacity: 0.4; cursor: default; }
.git-menu button.danger:not(:disabled) { color: var(--cat-danger-text); }
.git-menu hr {
  border: none;
  border-top: 1px solid var(--cat-border);
  margin: 4px 2px;
}
.stage:hover:not(:disabled) { background: var(--cat-hover); }
.stage:disabled { opacity: 0.45; cursor: default; }
.merge-bar {
  margin: 10px 10px 0;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid color-mix(in srgb, var(--cat-warn, #c9a227) 35%, var(--cat-border));
  background: color-mix(in srgb, var(--cat-warn, #c9a227) 12%, var(--cat-surface));
  color: var(--cat-text);
  font-size: 12.5px;
}
.merge-bar.rebase {
  border-color: color-mix(in srgb, var(--cat-git-mod, #3b82f6) 35%, var(--cat-border));
  background: color-mix(in srgb, var(--cat-git-mod, #3b82f6) 12%, var(--cat-surface));
}
.merge-bar .meta { color: var(--cat-muted); font-size: 12px; }
.groups {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.sec {
  margin-top: 10px;
  height: 28px;
  flex: none;
  display: flex;
  align-items: center;
  padding: 0 6px 0 8px;
}
.conflict-sec .cnt {
  margin-left: 6px;
  min-width: 16px;
  height: 16px;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--cat-danger-text);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
}
.conflict-sec .cnt.zero {
  background: var(--cat-success-text);
}
.sec-h {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--cat-text-2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}
.cf-list { padding: 0 8px 4px; }
.cf-row {
  margin-bottom: 6px;
  padding: 8px;
  border: 1px solid var(--cat-border);
  border-radius: 10px;
  background: var(--cat-surface);
  box-shadow: var(--cat-shadow-sm, 0 1px 2px rgba(0,0,0,.04));
}
.cf-row.on {
  border-color: color-mix(in srgb, var(--cat-text) 28%, var(--cat-border));
  background: var(--cat-hover);
}
.cf-row.resolved { opacity: 0.85; }
.cf-top {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: transparent;
  padding: 0;
  color: var(--cat-text);
  font: inherit;
  font-size: 12.5px;
  text-align: left;
  cursor: pointer;
}
.cf-top .path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}
.cf-top .uu {
  flex: none;
  font-size: 11px;
  font-weight: 700;
  color: var(--cat-danger-text);
}
.cf-top .uu.ok { color: var(--cat-success-text); }
.cf-top :deep(svg.fi) { color: var(--cat-file-icon); flex: none; }
.cf-acts {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.cf-acts .btn {
  height: 26px;
  padding: 0 10px;
  border: 1px solid var(--cat-border);
  border-radius: 7px;
  background: var(--cat-elevated, var(--cat-surface));
  color: var(--cat-text);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.cf-acts .btn:hover:not(:disabled) { background: var(--cat-hover); }
.cf-acts .btn:disabled { opacity: 0.45; cursor: default; }
.cf-acts .st.ok {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--cat-success-text);
  font-size: 12px;
  font-weight: 650;
}
.note-2nd {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  margin: 4px 0 8px;
  padding: 8px 10px;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  color: var(--cat-muted);
  font-size: 11.5px;
  line-height: 1.45;
}
.cf-foot {
  margin: 8px 10px 12px;
  padding: 10px;
  border: 1px solid var(--cat-border);
  border-radius: 10px;
  background: var(--cat-surface);
}
.cf-foot .note {
  margin-bottom: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--cat-warn, #c9a227) 16%, transparent);
  color: var(--cat-text);
  font-size: 12px;
  font-weight: 600;
}
.cf-foot .note.clear {
  display: flex;
  align-items: center;
  gap: 4px;
  background: color-mix(in srgb, var(--cat-success-text) 14%, transparent);
  color: var(--cat-success-text);
}
.cf-foot .fld { margin-bottom: 8px; }
.cf-foot label {
  display: block;
  margin-bottom: 4px;
  font-size: 11px;
  color: var(--cat-muted);
}
.cf-foot .inp {
  width: 100%;
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--cat-border);
  border-radius: 7px;
  background: var(--cat-elevated, var(--cat-surface));
  color: var(--cat-text);
  font: inherit;
  font-size: 12.5px;
  box-sizing: border-box;
}
.rowbtns {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}
.rowbtns .btn {
  height: 30px;
  padding: 0 12px;
  border: 1px solid var(--cat-border);
  border-radius: 8px;
  background: var(--cat-surface);
  color: var(--cat-text);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.rowbtns .btn.abort {
  color: var(--cat-danger-text);
  border-color: color-mix(in srgb, var(--cat-danger-text) 35%, var(--cat-border));
  background: color-mix(in srgb, var(--cat-danger-text) 8%, var(--cat-surface));
}
.rowbtns .btn.pri {
  background: var(--cat-text);
  color: var(--cat-surface);
  border-color: transparent;
  font-weight: 650;
}
.rowbtns .btn:disabled { opacity: 0.45; cursor: default; }
.list {
  padding: 0 6px 4px;
}
.row {
  width: 100%;
  min-height: 28px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 6px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--cat-text);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.row:hover { background: var(--cat-hover); }
.row :deep(svg.fi) { color: var(--cat-file-icon); flex: none; }
.nm {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dir {
  flex: none;
  max-width: 42%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--cat-muted);
  font-size: 12px;
}
.stat {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 4px;
  flex: none;
  font-size: 11px;
  font-weight: 650;
}
.add { color: var(--cat-success-text); }
.del { color: var(--cat-danger-text); }
.letter { font-size: 12px; }
.act {
  width: 16px;
  height: 16px;
  display: none;
  place-items: center;
  color: var(--cat-text-2);
  flex: none;
}
.row:hover .act { display: grid; }
.st-M, .st-R { color: var(--cat-git-mod); }
.st-A, .st-U { color: var(--cat-success-text); }
.st-D { color: var(--cat-danger-text); }
.git-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px 16px;
  color: var(--cat-muted);
  font-size: 13px;
  text-align: center;
}
.git-empty.flat {
  flex: none;
  padding: 12px 8px;
  align-items: flex-start;
}
.retry {
  border: 1px solid var(--cat-border);
  background: transparent;
  color: var(--cat-text-2);
  border-radius: 6px;
  padding: 4px 10px;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
</style>
