<template>
  <div class="git" data-testid="cat-git" @keydown.esc="menuOpen = false; commitOptionsOpen = false">
    <div class="git-top">
      <FIcon name="branch" :size="14" /><b class="git-title">{{ t('cat.gitWorkbench.title') }}</b>
      <button
        type="button"
        class="pr"
        :disabled="prDisabled"
        :title="prTitle"
        data-testid="cat-git-pr"
        @click="createPR"
      >
        <FIcon name="branch" :size="13" />
        <span>PR</span>
      </button>
      <span class="sp" />
      <button v-if="tab === 'changes'" type="button" class="ib" :class="{ on: filtering }" :title="COPY.filter" data-testid="cat-git-filter" @click="filtering = !filtering">
        <FIcon name="search" :size="15" />
      </button>
      <button type="button" class="ib" :title="COPY.refresh" :disabled="loading" data-testid="cat-git-refresh" @click="emit('refresh')">
        <FIcon name="refresh" :size="15" />
      </button>
      <button ref="moreBtn" type="button" class="ib" :title="t('cat.gitWorkbench.repoActions')" :aria-label="t('cat.gitWorkbench.repoActions')" :aria-expanded="menuOpen" aria-haspopup="menu" data-testid="cat-git-actions" @click.stop="menuOpen = !menuOpen"><FIcon name="more" :size="15" /></button>
      <div v-if="menuOpen && status?.repo" ref="menuEl" class="git-menu repo-menu" role="menu" @click.stop>
        <div class="menu-heading">{{ t('cat.gitWorkbench.remoteActions') }}</div>
        <template v-for="(e, i) in repositoryEntries" :key="i">
          <hr v-if="e.kind === 'sep'" />
          <button v-else type="button" role="menuitem" :disabled="e.disabled || busy" :title="e.title || undefined" :class="{ danger: e.danger }" @click="onMenu(e.id)">{{ e.label }}</button>
        </template>
        <hr />
        <button type="button" role="menuitem" :disabled="busy || inConflict" @click="tab = 'stashes'; menuOpen = false">{{ t('cat.gitWorkbench.saveStash') }}</button>
        <button type="button" role="menuitem" :disabled="busy || inConflict || status.detached || !status.headSha || (!!status.upstream && !status.ahead)" @click="undoCommit">{{ t('cat.gitWorkbench.undoCommit') }}</button>
      </div>
    </div>
    <label v-if="filtering && tab === 'changes'" class="git-filter">
      <FIcon name="search" :size="13" />
      <input v-model="filter" type="search" :placeholder="COPY.filter" />
    </label>
    <p v-if="loadError && status" class="git-err" role="alert">{{ loadError }}</p>
    <div v-if="loading && !status" class="git-empty" role="status">正在加载…</div>
    <div v-else-if="loadError && !status" class="git-empty" role="alert">
      <span>{{ loadError }}</span>
      <button type="button" class="retry" @click="emit('refresh')">重试</button>
    </div>
    <div v-else-if="!status?.repo" class="git-empty"><span>{{ COPY.notRepo }}</span><button type="button" class="retry" :disabled="busy" @click="run(id => gitCatInit(id))">{{ t('cat.gitWorkbench.init') }}</button><p v-if="actionError" class="git-err" role="alert">{{ actionError }}</p></div>
    <template v-else>
      <div class="git-branch-row">
        <button
          type="button"
          class="branch-pill"
          :class="{ open: tab === 'branches' }"
          data-testid="cat-git-branch-btn"
          :aria-expanded="tab === 'branches'"
          @click.stop="tab = 'branches'"
        >
          <FIcon name="branch" :size="13" />
          <span class="bn">{{ status.branch || 'HEAD' }}</span>
          <span v-if="status.detached" class="head-sha">{{ status.headShortSha }}</span>
          <FIcon name="down" :size="12" class="cv" />
        </button>
        <button v-if="status.web" type="button" class="ib" :title="COPY.openRemote" @click="openRemote">
          <FIcon name="external" :size="14" />
        </button>

      </div>
      <div class="sync-card" data-testid="cat-git-sync-card">
        <div class="repo-meta"><span :title="status.root">{{ repoLabel }}</span><code :title="status.upstream || status.remote">{{ status.upstream || t('cat.gitWorkbench.unpublished') }}</code></div>
        <div v-if="status.headSha && !status.detached" class="sync-counts">
          <button type="button" :class="{ selected: tab === 'history' && historyScope === 'outgoing' }" data-testid="cat-git-outgoing" @click="showHistory('outgoing')"><FIcon name="upload" :size="13" /><b>{{ status.ahead }}</b><span>{{ t('cat.gitWorkbench.outgoing') }}</span></button>
          <button v-if="status.upstream" type="button" :class="{ selected: tab === 'history' && historyScope === 'incoming' }" data-testid="cat-git-incoming" @click="showHistory('incoming')"><FIcon name="download" :size="13" /><b>{{ status.behind }}</b><span>{{ t('cat.gitWorkbench.incoming') }}</span></button>
        </div>
        <p v-if="syncState === 'detached'" class="sync-hint">{{ t('cat.gitWorkbench.detachedHint') }}</p>
        <p v-else-if="syncState === 'diverged'" class="sync-hint warn">{{ t('cat.gitWorkbench.divergedHint') }}</p>
        <p v-else-if="syncState === 'synced'" class="sync-hint">{{ t('cat.gitWorkbench.syncedHint') }}</p>
        <p v-else-if="!status.upstream" class="sync-hint">{{ t(status.remote ? 'cat.gitWorkbench.publishHint' : 'cat.gitWorkbench.noRemote') }}</p>
        <div class="sync-actions">
          <button v-if="!status.remote" type="button" class="sync-primary" :disabled="busy || inConflict" @click="addRemote">{{ t('cat.gitWorkbench.addRemote') }}</button>
          <button v-else type="button" class="sync-primary" :disabled="busy || primaryRemote.disabled" :title="primaryRemote.reason || undefined" data-testid="cat-git-remote-primary" @click="remote(primaryRemote.op)"><FIcon :name="primaryRemote.op === 'push' || primaryRemote.op === 'publish' ? 'upload' : 'refresh'" :size="13" />{{ busy ? t('cat.gitWorkbench.working') : primaryRemote.label }}</button>
          <button v-if="status.upstream && !status.detached && primaryRemote.op !== 'push'" type="button" class="sync-secondary" :disabled="busy || inConflict" :title="t('cat.gitWorkbench.pushHint')" @click="remote('push')">{{ COPY.push }}</button>
          <button v-if="status.upstream && !status.detached && primaryRemote.op !== 'ff'" type="button" class="sync-secondary" :disabled="busy || inConflict" :title="t('cat.gitWorkbench.pullHint')" @click="remote('ff')">{{ COPY.pull }}</button>
        </div>
        <p v-if="lastFetch" class="fetch-meta">{{ t('cat.gitWorkbench.fetchedAt', { time: fetchTime }) }}</p>
      </div>
      <p v-if="actionError" class="git-err" role="alert">{{ actionError }}</p>
      <nav class="workbench-tabs" role="tablist" :aria-label="t('cat.gitWorkbench.tabs')">
        <button v-for="name in tabNames" :key="name" type="button" role="tab" :aria-selected="tab === name" :class="{ active: tab === name }" :data-testid="'cat-git-tab-' + name" @click="tab = name">{{ t(workbenchTabKey + name) }}<span v-if="name === 'changes' && status.changes.length">{{ status.changes.length }}</span></button>
      </nav>
      <template v-if="tab === 'changes'">
      <textarea v-model="message" class="git-msg" rows="3" :placeholder="COPY.message" :aria-label="COPY.message" data-testid="cat-git-message" @keydown.ctrl.enter.prevent="doCommit" @keydown.meta.enter.prevent="doCommit" />
      <p class="commit-hint">{{ stagedCount ? t('cat.gitWorkbench.stagedHint', { count: stagedCount }) : t('cat.gitWorkbench.allChangesHint') }}</p>
      <div class="commit-bar">
        <button type="button" class="stage main" :disabled="commitDisabled" data-testid="cat-git-primary" @click="doCommit()">
          {{ stagedCount ? t('cat.gitWorkbench.commitStaged', { count: stagedCount }) : t('cat.gitWorkbench.commitAll') }}
        </button>
        <button
          ref="commitMoreBtn"
          type="button"
          class="stage more"
          :disabled="busy"
          :aria-expanded="commitOptionsOpen"
          aria-haspopup="menu"
          :title="t('cat.misc.s_0ec9eaf9c3')"
          data-testid="cat-git-more"
          @click.stop="commitOptionsOpen = !commitOptionsOpen"
        >
          <FIcon name="down" :size="14" />
        </button>
        <div v-if="commitOptionsOpen" ref="commitMenuEl" class="git-menu" role="menu" @click.stop>
          <template v-for="(e, i) in commitEntries" :key="i">
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
              <template v-if="status.mergeState !== 'none'">
                <button type="button" class="btn abort" :disabled="busy" data-testid="cat-git-abort" @click="abortConflict">{{ labels.abort }}</button>
                <button v-if="status.mergeState !== 'merging'" type="button" class="btn" :disabled="busy" @click="skipConflict">{{ t('cat.gitWorkbench.skipCommit') }}</button>
                <button type="button" class="btn pri" :disabled="busy || unresolved.length > 0" data-testid="cat-git-complete" @click="completeConflict">{{ labels.complete }}</button>
              </template>
              <span v-else class="sync-hint">{{ t('cat.gitWorkbench.stashConflictHint') }}</span>
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
            <div
              v-for="c in g.items"
              :key="g.id + ':' + c.path"
              class="row"
            >
              <button type="button" class="file-open" :title="c.path" @click="openDiff(c, g.id)"><FIcon name="file" :size="14" class="fi" /><span class="nm" :class="'st-' + c.letter">{{ c.name }}</span><span v-if="c.dir" class="dir">{{ c.dir }}</span></button>
              <span class="stat">
                <span v-if="c.added" class="add">+{{ c.added }}</span>
                <span v-if="c.deleted" class="del">−{{ c.deleted }}</span>
                <span class="letter" :class="'st-' + c.letter">{{ c.letter }}</span>
              </span>
              <button v-if="g.act === 'stage'" type="button" class="act discard" :title="t('cat.gitWorkbench.discard')" :aria-label="t('cat.gitWorkbench.discard') + ' ' + c.name" :disabled="busy" @click="discard([c])"><FIcon name="retry" :size="13" /></button>
              <button
                type="button"
                class="act"
                :title="g.act === 'unstage' ? COPY.unstage : COPY.stage"
                :aria-label="(g.act === 'unstage' ? COPY.unstage : COPY.stage) + ' ' + c.name"
                :disabled="busy"
                @click.stop="g.act === 'unstage' ? unstagePaths([c]) : stagePaths([c])"
              >
                <FIcon :name="g.act === 'unstage' ? 'x' : 'plus'" :size="12" />
              </button>
            </div>
          </div>
        </div>
      </div>
      </template>
      <div v-else-if="tab === 'history'" class="workbench-content">
        <CatCommitHistory ref="historyEl" :conv-id="convId" :scope="historyScope" :has-upstream="!!status.upstream && !status.detached" :locked="busy || inConflict" :commit-blocked="status.detached || !!status.changes.length" :can-outgoing="!!status.headSha && !status.detached" embedded @commit-action="onCommitAction" @scope-change="historyScope = $event" @changed="emit('refresh')" />
      </div>
      <div v-else-if="tab === 'branches'" class="branch-content">
        <div class="branch-heading"><span>{{ t('cat.gitWorkbench.branchHint') }}</span><button type="button" class="ib" :disabled="busy || inConflict" :title="COPY.newBranch" @click="onMenu('branch')"><FIcon name="plus" :size="14" /></button></div>
        <CatBranchTree ref="treeEl" :open="true" :conv-id="convId" :anchor="null" embedded @changed="emit('refresh')" />
      </div>
      <div v-if="tab === 'stashes' || stashBusy" v-show="tab === 'stashes'" class="workbench-content">
        <CatGitStashes ref="stashesEl" :conv-id="convId" :live="live" :status="status" :locked="busy && !stashBusy" @busy="onStashBusy" @changed="emit('refresh')" @conflict="onStashConflict" />
      </div>
    </template>
    <CatGitDiffDialog :visible="!!diffTitle" :title="diffTitle" :loading="diffLoading" :error="diffError" :diff="diffData" @close="closeDiff" />
  </div>
</template>

<script setup lang="ts">
const workbenchTabKey = 'cat.gitWorkbench.tab_'
const workbenchKey = 'cat.gitWorkbench.'
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessageBox } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import CatCommitHistory from './CatCommitHistory.vue'
import CatBranchTree from './CatBranchTree.vue'
import CatGitStashes from './CatGitStashes.vue'
import CatGitDiffDialog from './CatGitDiffDialog.vue'
import { gitCatConflictSkip, gitCatInit, gitCatAddRemote, gitCatCommitAction, gitCatDiscard, gitCatUndoCommit, gitPrimaryRemote, gitSyncState, type GitHistoryScope, type GitWorkbenchTab } from '@/api/catGitWorkflows'
import { catSim, type CatSimFlag } from '@/views/cat/catDevSim'
import {
  CAT_GIT_CONFLICT_COPY as CF,
  CAT_GIT_COPY as COPY,
  catGitError,
  conflictLabels,
  gitCatBranch,
  gitCatCommit,
  gitCatDiff,
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
  type GitChange,
  type GitConflictMode,
  type GitStatus,
  type GitDiff,
} from '@/api/catGit'
import type { CatCommit } from '@/api/catCommits'
import { openExternal } from '@/utils/openExternal'
import { notify } from '@/views/cat/catState'

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
const commitDrafts = new Map<string, string>()
const historyEl = ref<{ reload: () => void } | null>(null)
const treeEl = ref<{ reload: () => void } | null>(null)
const stashesEl = ref<{ reload: () => void } | null>(null)
/** 走查：?cat_sim=gitgraph… 时「提交历史」默认展开（正式默认折叠） */
const simHistoryOpen = import.meta.env.DEV && ['gitgraph', 'gitgraphempty', 'gitgraphchanged', 'gitgraphloadfail', 'gitgraphopen'].some((k) => catSim.has(k as CatSimFlag))
const tab = ref<GitWorkbenchTab>(simHistoryOpen ? 'history' : 'changes')
const tabNames: GitWorkbenchTab[] = ['changes', 'history', 'branches', 'stashes']
const historyScope = ref<GitHistoryScope>('current')
const primaryRemote = computed(() => gitPrimaryRemote(props.status))
const syncState = computed(() => gitSyncState(props.status))
const repoLabel = computed(() => props.status?.root?.replace(/\\/g, '/').replace(/\/$/, '').split('/').pop() || t('cat.gitWorkbench.repository'))
const stagedCount = computed(() => props.status?.changes.filter(c => c.staged).length || 0)
const fetchedAt = ref(0)
const lastFetch = computed(() => Math.max(fetchedAt.value, (props.status?.fetchedAt || 0) * 1000))
const fetchTime = computed(() => new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(lastFetch.value))
const stashBusy = ref(false)
const diffTitle = ref(''), diffLoading = ref(false), diffError = ref(''), diffData = ref<GitDiff | null>(null)
let diffSeq = 0
const mergeMessage = ref('')
const filter = ref('')
const filtering = ref(false)
const opened = ref<Record<string, boolean>>({ work: true, staged: true, untracked: true, conflicts: true })
const busy = ref(false)
const actionError = ref('')
const menuOpen = ref(false)
const commitOptionsOpen = ref(false)
const moreBtn = ref<HTMLButtonElement>()
const menuEl = ref<HTMLElement>()
const commitMoreBtn = ref<HTMLButtonElement>()
const commitMenuEl = ref<HTMLElement>()
const selectedConflict = ref('')
const resolvedLocal = ref<Set<string>>(new Set())


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

const inConflict = computed(() => !!props.status?.repo && (props.status.mergeState !== 'none' || props.status.conflicts.length > 0))
const unresolved = computed(() => (props.status?.conflicts ?? []).filter((c) => !resolvedLocal.value.has(c.path)))
const labels = computed(() => props.status?.mergeState === 'none' && props.status.conflicts.length > 0 ? { ...conflictLabels('none'), bar: t('cat.gitWorkbench.stashConflicts'), meta: t('cat.gitWorkbench.stashConflictHint') } : conflictLabels(props.status?.mergeState ?? 'none'))

const conflictRows = computed(() => {
  const list = props.status?.conflicts ?? []
  return list.map((c) => ({
    ...c,
    resolved: resolvedLocal.value.has(c.path),
  }))
})

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
const commitEntries = computed(() => menuEntries.value.filter(e => e.kind === 'item' && ['commit', 'commit-push', 'commit-sync'].includes(e.id)))
const repositoryEntries = computed(() => menuEntries.value.filter(e => e.kind === 'item' && ['push', 'pull', 'ff', 'sync', 'rebase', 'fetch', 'publish', 'force'].includes(e.id) && (e.id !== 'publish' || !props.status?.upstream)).map(e => e.kind === 'item' ? { ...e, disabled: e.disabled || !props.status?.remote || (e.id !== 'fetch' && (inConflict.value || !!props.status?.detached)), title: inConflict.value ? t('cat.gitWorkbench.resolveFirst') : e.title } : e))

function showHistory(scope: GitHistoryScope) { historyScope.value = scope; tab.value = 'history' }
function onStashBusy(value: boolean) { stashBusy.value = value; busy.value = value }
function onStashConflict() { tab.value = 'changes'; emit('git-conflict') }
function closeDiff() { diffTitle.value = ''; diffSeq++ }
async function openDiff(c: GitChange, group: string) {
  const my = ++diffSeq, conv = props.convId
  const scope = group === 'staged' ? 'staged' : 'worktree'
  diffTitle.value = `${scope === 'staged' ? COPY.staged : COPY.changes} · ${c.path}`
  diffLoading.value = true; diffError.value = ''; diffData.value = null
  try {
    const diff = props.live ? await gitCatDiff(conv, c.path, scope) : { path: c.path, binary: false, text: `@@ -1,2 +1,2 @@\n-Original content\n+${scope === 'staged' ? 'Staged changes' : 'Unstaged changes'}` }
    if (my === diffSeq) diffData.value = diff
  } catch (e) { if (my === diffSeq) diffError.value = catGitError(e) }
  finally { if (my === diffSeq) diffLoading.value = false }
}
async function discard(items: GitChange[]) {
  const conv = props.convId
  try { await ElMessageBox.confirm(t('cat.gitWorkbench.discardAsk', { paths: items.map(c => c.path).join('\n') }), t('cat.gitWorkbench.discard'), { type: 'warning', confirmButtonText: t('cat.gitWorkbench.discard'), cancelButtonText: t('cat.commit.cancel'), confirmButtonType: 'danger' }) } catch { return }
  if (conv !== props.convId) return
  void run(id => gitCatDiscard(id, items.map(c => c.path)))
}
async function addRemote() {
  const conv = props.convId
  let url = '', name = ''
  try {
    const u = await ElMessageBox.prompt(t('cat.gitWorkbench.remoteUrl'), t('cat.gitWorkbench.addRemote'), { inputPlaceholder: 'https://github.com/owner/repo.git', confirmButtonText: t('cat.commit.create'), cancelButtonText: t('cat.commit.cancel') })
    url = u.value.trim()
    if (!url || conv !== props.convId) return
    const n = await ElMessageBox.prompt(t('cat.gitWorkbench.remoteName'), t('cat.gitWorkbench.addRemote'), { inputValue: 'origin', confirmButtonText: t('cat.commit.create'), cancelButtonText: t('cat.commit.cancel') })
    name = n.value.trim()
  } catch { return }
  if (!name || conv !== props.convId) return
  void run(async id => { await gitCatAddRemote(id, name, url); if (id === props.convId) notify(t('cat.gitWorkbench.addedRemote')) })
}
async function onCommitAction(c: CatCommit, op: 'cherry-pick' | 'revert') {
  const conv = props.convId, head = props.status?.headSha || ''
  const key = op === 'revert' ? 'revert' : 'cherryPick'
  try { await ElMessageBox.confirm(t(workbenchKey + (op === 'revert' ? 'revertAsk' : 'cherryAsk'), { sha: c.shortSha }), t(workbenchKey + key), { type: 'warning', confirmButtonText: t(workbenchKey + key), cancelButtonText: t('cat.commit.cancel') }) } catch { return }
  if (conv !== props.convId) return
  void run(id => gitCatCommitAction(id, c.sha, head, op))
}
async function undoCommit() {
  menuOpen.value = false
  const conv = props.convId, sha = props.status?.headSha
  if (!sha) return
  try { await ElMessageBox.confirm(t('cat.gitWorkbench.undoAsk'), t('cat.gitWorkbench.undoCommit'), { type: 'warning', confirmButtonText: t('cat.gitWorkbench.undoCommit'), cancelButtonText: t('cat.commit.cancel') }) } catch { return }
  if (conv !== props.convId) return
  void run(async id => { const restored = await gitCatUndoCommit(id, sha); commitDrafts.set(id, restored); if (id === props.convId) { message.value = restored; tab.value = 'changes'; notify(t('cat.gitWorkbench.undone')) } })
}

watch(() => props.convId, (id, old) => {
  if (old) commitDrafts.set(old, message.value)
  message.value = commitDrafts.get(id) || ''; actionError.value = ''; closeDiff(); menuOpen.value = false; commitOptionsOpen.value = false; fetchedAt.value = 0
})
watch(() => [props.status?.branch, props.status?.upstream, props.status?.headSha, props.status?.ahead, props.status?.behind].join('|'), () => {
  if (props.status && ((props.status.detached && ['outgoing', 'incoming'].includes(historyScope.value)) || (!props.status.upstream && historyScope.value === 'incoming'))) historyScope.value = 'current'
  historyEl.value?.reload(); treeEl.value?.reload()
})

async function run(fn: (convId: string) => Promise<void>, opts?: { quietConflict?: boolean }) {
  if (!props.live) {
    notify(COPY.preview, 'warn')
    return
  }
  if (busy.value) return
  busy.value = true
  actionError.value = ''
  const conv = props.convId
  try {
    await fn(conv)
  } catch (e) {
    if (conv !== props.convId) return
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
  void run(async id => {
    const url = await gitCatPullRequest(id)
    if (url) openExternal(url)
  })
}

function openRemote() {
  if (props.status?.web) openExternal(props.status.web)
}

function onPointerDown(e: PointerEvent) {
  const t = e.target
  if (!(t instanceof Node)) return
  if (moreBtn.value?.contains(t) || menuEl.value?.contains(t)) return
  menuOpen.value = false
  if (commitMoreBtn.value?.contains(t) || commitMenuEl.value?.contains(t)) return
  commitOptionsOpen.value = false
}
onMounted(() => document.addEventListener('pointerdown', onPointerDown))
onBeforeUnmount(() => { diffSeq++; document.removeEventListener('pointerdown', onPointerDown) })

async function commitOnly(id: string) {
  const msg = message.value.trim()
  await gitCatCommit(id, msg)
  if (id === props.convId) { message.value = ''; notify(t('cat.gitWorkbench.committed')) }
}

function doCommit() {
  if (commitDisabled.value) return
  void run(id => commitOnly(id))
}

async function afterCommit(id: string, op: string) {
  await commitOnly(id)
  await gitCatRemote(id, op)
}

function remote(op: string) {
  menuOpen.value = false
  void run(async id => { await gitCatRemote(id, op); if (id === props.convId) { if (['fetch', 'ff', 'pull', 'sync', 'rebase'].includes(op)) fetchedAt.value = Date.now(); notify(t('cat.gitWorkbench.remoteDone')) } })
}

async function onMenu(id: string) {
  const conv = props.convId
  menuOpen.value = false
  commitOptionsOpen.value = false
  if (id === 'commit') {
    doCommit()
    return
  }
  if (id === 'commit-push') {
    void run(id => afterCommit(id, 'push'))
    return
  }
  if (id === 'commit-sync') {
    void run(id => afterCommit(id, 'sync'))
    return
  }
  if (id === 'force') {
    try {
      await ElMessageBox.confirm(COPY.forceAsk, COPY.force, { confirmButtonText: t('cat.misc.s_27ca568be2'), cancelButtonText: t('cat.misc.s_625fb26b4b'), type: 'warning' })
    } catch {
      return
    }
    if (conv !== props.convId) return
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
    if (!name || conv !== props.convId) return
    void run(id => gitCatBranch(id, name))
    return
  }
  if (id === 'push-pr') {
    void run(async id => {
      await gitCatRemote(id, 'push')
      const url = await gitCatPullRequest(id)
      if (url) openExternal(url)
    })
    return
  }
  const op: Record<string, string> = { push: 'push', pull: 'pull', ff: 'ff', sync: 'sync', rebase: 'rebase', fetch: 'fetch', publish: 'publish' }
  if (op[id]) remote(op[id])
}

function stagePaths(items: GitChange[]) {
  if (!items.length) return
  void run(id => gitCatStage(id, items.map((c) => c.path)))
}

function unstagePaths(items: GitChange[]) {
  if (!items.length) return
  void run(id => gitCatUnstage(id, items.map((c) => c.path)))
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
  void run(async id => {
    await gitCatConflictResolve(id, path, mode)
    if (id === props.convId) resolvedLocal.value = new Set([...resolvedLocal.value, path])
  })
}

function completeConflict() {
  if (unresolved.value.length) return
  void run(async id => {
    await gitCatConflictComplete(id, mergeMessage.value.trim())
    if (id === props.convId) resolvedLocal.value = new Set()
  })
}

async function abortConflict() {
  const conv = props.convId
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
  if (conv !== props.convId) return
  void run(async id => {
    await gitCatConflictAbort(id)
    if (id === props.convId) resolvedLocal.value = new Set()
  })
}

async function skipConflict() {
  const conv = props.convId
  try { await ElMessageBox.confirm(t('cat.gitWorkbench.skipAsk'), t('cat.gitWorkbench.skipCommit'), { type: 'warning', confirmButtonText: t('cat.gitWorkbench.skipCommit'), cancelButtonText: t('cat.commit.cancel') }) } catch { return }
  if (conv === props.convId) void run(id => gitCatConflictSkip(id))
}

defineExpose({
  showHistory,
  openBranchMenu: () => {
    tab.value = 'branches'
  },
})
</script>

<style scoped>
.git {
  container-type: inline-size;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.git-top {
  position: relative;
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
.branch-pill .cv { color: var(--cat-muted); flex: none; }
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
.git-title { color: var(--cat-text); font-size: 12px; font-weight: 650; margin-right: 4px; }
.pr { border-radius: 6px; padding: 0 6px; background: transparent; color: var(--cat-text-2); font-weight: 500; }
.pr:hover:not(:disabled) { background: var(--cat-hover); }
.repo-menu { left: auto; right: 8px; width: min(240px, calc(100% - 16px)); z-index: 20; }
.menu-heading { padding: 6px 8px; color: var(--cat-muted); font-size: 10.5px; }
.sync-card { flex: none; margin: 0 10px 10px; padding: 10px; border: 1px solid var(--cat-border); border-radius: 9px; background: var(--cat-surface); }
.repo-meta { display: flex; flex-wrap: wrap; gap: 4px 8px; margin-bottom: 8px; font-size: 11px; color: var(--cat-muted); }
.repo-meta > * { min-width: 0; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.repo-meta > span { color: var(--cat-text-2); font-weight: 600; }
.repo-meta code { font-family: var(--ff-font-mono); }
.sync-counts { display: flex; gap: 6px; }
.sync-counts button { flex: 1; min-width: 0; display: flex; align-items: center; justify-content: center; gap: 5px; height: 32px; padding: 0 4px; border: 1px solid var(--cat-border); border-radius: 6px; background: var(--cat-hover); color: var(--cat-text-2); font: inherit; font-size: 11px; cursor: pointer; }
.sync-counts b { color: var(--cat-text); font-size: 14px; font-variant-numeric: tabular-nums; }
.sync-counts button:hover, .sync-counts button.selected { color: var(--cat-accent); border-color: var(--cat-accent); }
.sync-hint { margin: 8px 0; font-size: 11px; color: var(--cat-muted); line-height: 1.5; }
.sync-hint.warn { color: var(--cat-orange); }
.sync-actions { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 8px; }
.sync-actions button { display: inline-flex; align-items: center; justify-content: center; gap: 5px; height: 28px; border: 1px solid var(--cat-border); border-radius: 6px; padding: 0 8px; font: inherit; font-size: 11.5px; cursor: pointer; }
.sync-primary { flex: 1; background: var(--cat-text); color: var(--cat-surface); }
.sync-secondary { background: transparent; color: var(--cat-text-2); }
.sync-actions button:disabled { opacity: .45; cursor: default; }
.sync-actions button:hover:not(:disabled) { filter: brightness(.93); }
.fetch-meta { margin: 6px 0 0; color: var(--cat-muted); font-size: 10px; }
.workbench-tabs { flex: none; display: flex; margin: 0 8px 10px; border-bottom: 1px solid var(--cat-border); }
.workbench-tabs button { flex: 1; min-width: 0; display: flex; align-items: center; justify-content: center; gap: 4px; height: 32px; padding: 0 3px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--cat-muted); font: inherit; font-size: 12px; cursor: pointer; }
.workbench-tabs button.active { color: var(--cat-text); font-weight: 600; border-bottom-color: var(--cat-accent); }
.workbench-tabs button:hover { background: var(--cat-hover); }
.workbench-tabs button span { font-size: 10px; padding: 0 4px; border-radius: 4px; background: var(--cat-hover); }
.workbench-content { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; }
.branch-content { flex: 1; min-height: 0; display: flex; flex-direction: column; overflow: hidden; }
.branch-heading { flex: none; display: flex; align-items: center; gap: 6px; padding: 0 10px 6px; color: var(--cat-muted); font-size: 11px; line-height: 1.5; }
.branch-heading span { flex: 1; }
.commit-hint { margin: 6px 12px 0; color: var(--cat-muted); font-size: 10.5px; line-height: 1.5; }
.commit-bar .stage { background: var(--cat-accent); color: var(--cat-on-accent); border-color: var(--cat-accent); }
.commit-bar .stage:hover:not(:disabled) { background: var(--cat-accent-hover); }
.file-open { display: flex; align-items: center; gap: 5px; min-width: 0; flex: 1; height: 28px; padding: 0; border: 0; background: transparent; color: inherit; font: inherit; text-align: left; cursor: pointer; }
.file-open .nm { flex: 0 1 auto; }
.file-open .dir { min-width: 0; }
.act { border: 0; background: transparent; padding: 0; cursor: pointer; color: var(--cat-text-2); }
.row:focus-within .act { display: grid; }
.row .discard:hover { color: var(--cat-danger-text); }
.git button:focus-visible, .git textarea:focus-visible, .git input:focus-visible { outline: 2px solid var(--cat-accent); outline-offset: -2px; }
@container (max-width: 220px) {
  .git-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pr { display: none; }
  .sync-counts button { height: 42px; flex-wrap: wrap; gap: 2px; }
  .sync-counts button span { flex-basis: 100%; text-align: center; }
  .sync-primary { flex-basis: 100%; }
  .sync-secondary { flex: 1; }
  .workbench-tabs button { font-size: 10px; }
  .workbench-tabs button span, .file-open .dir { display: none; }
}
</style>
