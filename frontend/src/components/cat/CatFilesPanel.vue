<template>
  <section
    ref="rootEl"
    class="ct-r"
    :class="{ wide: !!openRel }"
    :aria-label="pane === 'git' ? GIT.git : GIT.files"
    data-testid="cat-files"
  >
    <div
      v-if="openRel && showFile"
      class="ct-resize"
      role="separator"
      tabindex="0"
      aria-orientation="vertical"
      :aria-label="t('cat.misc.s_fe98efeb19')"
      :aria-valuenow="previewW"
      :aria-valuemin="160"
      data-testid="cat-file-resize"
      @pointerdown="onResizeDown"
      @keydown="onResizeKey"
    />
    <div v-if="openRel || pptTabs.length" v-show="showFile" class="ct-previews">
    <div v-if="pptTabs.length" class="ct-ppt-tabs" role="tablist" :aria-label="GIT.files">
      <div v-for="tab in pptTabs" :key="tab.rel" class="ct-ppt-tab" :class="{ active: openRel === tab.rel && !openDiff }">
        <button type="button" role="tab" :aria-selected="openRel === tab.rel && !openDiff" :title="tab.rel" @click="wantFile(tab.rel, tab.name)">{{ tab.name }}</button><button type="button" :aria-label="t('cat.ppt.close')" @click="closePptTab(tab.rel)">×</button>
      </div>
    </div>
    <CatFilePreview
      v-for="tab in pptTabs"
      v-show="openRel === tab.rel && !openDiff"
      :key="convId + ':' + tab.rel"
      :conv-id="convId" :rel-path="tab.rel" :name="tab.name" :live="live"
      @close="closePptTab(tab.rel)" @pick="onPick"
    />
    <CatFilePreview
      v-if="openRel && (!officeCanvas(openRel) || openDiff)"
      ref="previewEl"
      :conv-id="convId"
      :rel-path="openRel"
      :name="openName"
      :live="live"
      :compare="openDiff"
      :conflict="!!openConflictPath && openConflictPath === openRel"
      @close="onPreviewClose"
      @pick="onPick"
      @conflict-saved="onConflictSaved"
    />
    </div>
    <div class="ct-col">
      <div
        class="ct-resize"
        role="separator"
        tabindex="0"
        aria-orientation="vertical"
        :aria-label="t('cat.misc.s_8eec0b3522')"
        :aria-valuenow="catPane.tree"
        :aria-valuemin="160"
        data-testid="cat-tree-resize"
        @pointerdown="onTreeResizeDown"
        @keydown="onTreeResizeKey"
      />
      <div class="ct-modes" role="tablist" :style="{ '--tab-i': pane === 'git' ? 1 : 0 }">
        <button type="button" role="tab" :class="{ on: pane === 'files' }" :aria-selected="pane === 'files'" :title="GIT.files" data-testid="cat-files-tab" @click="pane = 'files'">
          <FIcon name="file" :size="15" /><span>{{ GIT.files }}</span>
        </button>
        <button type="button" role="tab" :class="{ on: pane === 'git' }" :aria-selected="pane === 'git'" :title="GIT.git" data-testid="cat-git-tab" @click="pane = 'git'">
          <FIcon name="branch" :size="15" /><span>Git</span>
        </button>
        <i v-show="pane === 'files' || pane === 'git'" class="ink" aria-hidden="true" />
        <button v-if="showBrowser" type="button" role="tab" class="icon" :class="{ on: pane === 'browser' }" :aria-selected="pane === 'browser'" :title="t('cat.misc.s_912302cb02')" data-testid="cat-browser-tab" @click="pane = 'browser'">
          <FIcon name="window" :size="16" />
        </button>
        <button v-if="showTerm" type="button" role="tab" class="icon" :class="{ on: pane === 'term' }" :aria-selected="pane === 'term'" :title="t('cat.misc.s_4722bc0c56')" data-testid="cat-term-tab" @click="pane = 'term'">
          <FIcon name="term" :size="16" />
        </button>
        <div class="add-wrap" data-cat-pane-add>
          <button type="button" class="add" :title="t('cat.misc.s_b58c7549c0')" :aria-label="t('cat.misc.s_b58c7549c0')" :aria-expanded="addOpen" data-testid="cat-pane-add" @click="addOpen = !addOpen">
            <FIcon name="plus" :size="16" />
          </button>
          <div v-if="addOpen" class="add-menu" role="menu" :aria-label="t('cat.misc.s_b58c7549c0')" data-testid="cat-pane-add-menu">
            <button type="button" role="menuitem" data-testid="cat-pane-add-browser" @click="openExtra('browser')">
              <FIcon name="window" :size="14" /><span>浏览器</span>
            </button>
            <button type="button" role="menuitem" data-testid="cat-pane-add-term" @click="openExtra('term')">
              <FIcon name="term" :size="14" /><span>终端</span>
            </button>
          </div>
        </div>
      </div>
      <CatBrowserPane v-if="showBrowser" v-show="pane === 'browser'" @close="closeExtra('browser')" @pick="onPick" />
      <CatTermPane v-if="showTerm" v-show="pane === 'term'" :conv-id="convId" :live="live" @close="closeExtra('term')" />
      <template v-if="pane === 'files' || pane === 'git'">
      <div v-if="missing || error === PC.missing" class="ct-empty" data-testid="cat-files-missing">
        <FIcon name="warn" :size="24" /><span>{{ PC.missing }}</span>
      </div>
      <CatGitPanel
        v-else-if="pane === 'git'"
        ref="gitPanel"
        :conv-id="convId"
        :live="live"
        :status="shownGit"
        :loading="gitLoading"
        :load-error="gitError"
        :sim-branches="simGitBranches"
        @refresh="loadGit"
        @open="onGitOpen"
        @open-conflict="onGitConflictOpen"
        @git-conflict="onGitConflictNav"
      />
      <template v-else>
        <div class="ct-head">
          <span class="ct-root" :title="rootPath || rootLabel">{{ rootLabel }}</span>
          <button type="button" class="ib" :title="rootPath || COPY.open" :disabled="!canOpen" data-testid="cat-files-open" @click="openFolder">
            <FIcon name="folder" :size="15" />
          </button>
          <button type="button" class="ib" :title="COPY.refresh" :disabled="!canRefresh" data-testid="cat-files-refresh" @click="refresh">
            <FIcon name="refresh" :size="15" />
          </button>
        </div>
        <label class="ct-search">
          <FIcon name="search" :size="13" />
          <input v-model="query" type="search" :placeholder="COPY.find" data-testid="cat-files-search" />
        </label>
        <div class="ct-seg" role="tablist">
          <button type="button" role="tab" :class="{ on: findKind === 'name' }" :aria-selected="findKind === 'name'" data-testid="cat-files-find-name" @click="findKind = 'name'">{{ COPY.findName }}</button>
          <button type="button" role="tab" :class="{ on: findKind === 'content' }" :aria-selected="findKind === 'content'" data-testid="cat-files-find-content" @click="findKind = 'content'">{{ COPY.findContent }}</button>
        </div>
        <div v-if="!live && findKind === 'content' && query.trim()" class="ct-empty">{{ COPY.contentPreview }}</div>
        <div v-else-if="!live" class="ct-tree">
          <div
            v-for="(n, i) in mockRows"
            :key="i"
            class="ct-node"
            :class="{ on: picked === i }"
            :style="{ paddingLeft: n[0] * 14 + 4 + 'px' }"
            @click="onMock(n, i)"
          >
            <template v-if="n[1] === 'f'">
              <span class="car-slot" />
              <FIcon name="file" :size="14" class="fi" />
            </template>
            <template v-else>
              <FIcon :name="n[1] === 'd' ? 'down' : 'right'" :size="12" class="car" />
              <FIcon name="folder" :size="14" />
            </template>
            <span class="nm" :class="n[1] === 'f' && mockLetter(String(n[2])) ? 'st-' + mockLetter(String(n[2])) : ''">{{ n[2] }}</span>
            <span v-if="mockLetter(String(n[2]))" class="badge" :class="'st-' + mockLetter(String(n[2]))">{{ mockLetter(String(n[2])) }}</span>
          </div>
        </div>
        <div v-else-if="searching && !hits" class="ct-empty" role="status">{{ COPY.searching }}</div>
        <div v-else-if="hits" class="ct-tree" data-testid="cat-files-hits">
          <div v-if="!hits.length" class="ct-empty" data-testid="cat-files-empty">
            <FIcon name="file" :size="28" /><span>{{ COPY.noMatch }}</span>
          </div>
          <template v-else>
            <div
              v-for="hit in hits"
              :key="hit.relPath + ':' + hit.line"
              class="ct-node hit"
              :class="{ on: picked === hit.relPath }"
              @click="onHit(hit)"
            >
              <FIcon :name="hit.isDir ? 'folder' : 'file'" :size="14" :class="hit.isDir ? '' : 'fi'" />
              <span class="hit-main">
                <span class="nm" :class="!hit.isDir && hitLetter(hit) ? 'st-' + hitLetter(hit) : ''">{{ hit.name }}</span>
                <span v-if="parentOf(hit.relPath)" class="dir">{{ parentOf(hit.relPath) }}</span>
                <span v-if="hit.text" class="snip">{{ hit.text }}</span>
              </span>
              <span v-if="hitLetter(hit)" class="badge" :class="'st-' + hitLetter(hit)">{{ hitLetter(hit) }}</span>
            </div>
            <div v-if="hitsCut" class="ct-node note">{{ COPY.searchCut }}</div>
          </template>
        </div>
        <div v-else-if="loading && !nodes" class="ct-empty" role="status">{{ COPY.loading }}</div>
        <div v-else-if="error" class="ct-empty" role="alert" data-testid="cat-files-error">
          <FIcon name="warn" :size="24" />
          <span>{{ error }}</span>
          <button type="button" class="retry" @click="refresh">{{ COPY.retry }}</button>
        </div>
        <div v-else-if="!rows.length" class="ct-empty" data-testid="cat-files-empty">
          <FIcon name="folder-open" :size="28" /><span>{{ query.trim() ? COPY.noMatch : COPY.empty }}</span>
          <!-- I2：空状态给一个主操作 -->
          <button v-if="!query.trim() && canOpen" type="button" class="retry primary" data-testid="cat-files-empty-open" @click="openFolder">{{ COPY.open }}</button>
        </div>
        <div v-else :key="treeKey" class="ct-tree swap-in" data-testid="cat-files-tree">
          <div
            v-for="row in rows"
            :key="row.key"
            class="ct-node"
            :class="{ on: picked === row.key, note: row.kind !== 'entry' }"
            :style="{ paddingLeft: row.depth * 14 + 4 + 'px' }"
            @click="onRow(row)"
          >
            <template v-if="row.kind === 'entry' && row.isDir">
              <FIcon :name="row.expanded ? 'down' : 'right'" :size="12" class="car" />
              <FIcon name="folder" :size="14" />
            </template>
            <template v-else-if="row.kind === 'entry'">
              <span class="car-slot" />
              <FIcon name="file" :size="14" class="fi" />
            </template>
            <span class="nm" :class="row.kind === 'entry' && !row.isDir && rowLetter(row) ? 'st-' + rowLetter(row) : ''">{{ row.name }}</span>
            <span v-if="row.kind === 'entry' && rowLetter(row)" class="badge" :class="'st-' + rowLetter(row)">{{ rowLetter(row) }}</span>
          </div>
        </div>
      </template>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { catWorkflow } from '@/views/cat/catWorkflow'
import { mockChanges, mockFiles, type CatTreeNode } from '@/api/catMock'
import { CAT_PROJECT_COPY as PC } from '@/api/catProjects'
import { isCatLive } from '@/api/cat'
import {
  CAT_FILES_COPY as COPY,
  catCallError,
  catFilesErrorText,
  collectExpanded,
  entriesToNodes,
  flattenCatFiles,
  fileEntryStamp,
  listCatFiles,
  mergeFileNodes,
  revealCatConversationFolder,
  searchCatFiles,
  type CatFileEntry,
  type CatFileNode,
  type CatFileRow,
  type CatSearchHit,
} from '@/api/catFiles'
import {
  CAT_GIT_COPY as GIT,
  gitCatStatus,
  gitLetterFor,
  gitRootLabel,
  type GitStatus,
} from '@/api/catGit'
import { catSim, type CatSimFlag } from '@/views/cat/catDevSim'
import { CAT_MERGE_CONFLICT_SIM_FILES } from '@/api/catBranches'
import { catScratch } from '@/views/cat/catScratchFiles'
import { catState, findConv, notify } from '@/views/cat/catState'
import { railCollapsed } from '@/views/cat/catLayout'
import { catGitUi, setCatGitUi } from './catGitUi'
import {
  CAT_CHAT_MIN,
  CAT_EXTRA_DEFAULT,
  CAT_EXTRA_MIN,
  CAT_EXTRA_WIDTH_KEY,
  CAT_PREVIEW_MIN,
  CAT_PREVIEW_WIDTH_KEY,
  CAT_SIDE_DEFAULT,
  CAT_TREE_MIN,
  CAT_TREE_WIDTH,
  CAT_TREE_WIDTH_KEY,
  catPane,
  clampPaneWidth,
  clampPreviewWidth,
  clampToLimits,
  dragPaneWidth,
  paneLimits,
  readPaneWidth,
  readPreviewWidth,
  rememberPaneWidth,
  settleWidths,
} from './catPreviewWidth'
import CatGitPanel from './CatGitPanel.vue'
import CatBrowserPane from './CatBrowserPane.vue'
import CatTermPane from './CatTermPane.vue'
import type { PickNote } from './catPick'

const CatFilePreview = defineAsyncComponent(() => import('./CatFilePreview.vue'))

const props = defineProps<{ convId: string; missing?: boolean }>()
const emit = defineEmits<{ pick: [note: PickNote] }>()
const live = isCatLive()
const simGitGraph = !live && ['gitgraph', 'gitgraphempty', 'gitgraphchanged', 'gitgraphloadfail', 'detached'].some((k) => catSim.has(k as CatSimFlag))
const mergeSimConflict = ref(false)
const simGitBranches = !live && (catSim.has('gitconflict') || catSim.has('gitbranch') || catSim.has('gitrebase') || catSim.has('gitdone'))
const gitPanel = ref<{ openBranchMenu?: () => void; showHistory?: (scope: typeof catGitUi.historyScope) => void } | null>(null)
const openConflictPath = ref('')
const pane = ref<'files' | 'git' | 'browser' | 'term'>(simGitBranches || simGitGraph ? 'git' : 'files')
const showFile = computed(() => pane.value === 'files' || pane.value === 'git')
const extraW = ref(readPaneWidth(typeof localStorage !== 'undefined' ? localStorage.getItem(CAT_EXTRA_WIDTH_KEY) : null, CAT_EXTRA_DEFAULT))
const showBrowser = ref(false)
const showTerm = ref(false)
const addOpen = ref(false)
const query = ref('')
const findKind = ref<'name' | 'content'>('name')
const hits = ref<CatSearchHit[] | null>(null)
const hitsCut = ref(false)
const searching = ref(false)
const nodes = ref<CatFileNode[] | null>(null)
const rootPath = ref('')
const rootTruncated = ref(false)
const loading = ref(false)
const error = ref('')
const gitStatus = ref<GitStatus | null>(null)
const gitLoading = ref(false)
const gitError = ref('')
const picked = ref<number | string>(-1)
const openRel = ref('')
const pptTabs = ref<{ rel: string; name: string }[]>([])
function officeCanvas(rel: string) { return /\.(pptx|docx|xlsx|pdf|html|htm|md|markdown)$/i.test(rel) }
const openName = ref('')
const openDiff = ref(false)
const pending = ref<{ rel: string; name: string; picked: number | string; diff: boolean } | null>(null)
const previewEl = ref<{ requestClose: () => boolean } | null>(null)
const rootEl = ref<HTMLElement>()
const previewW = ref(readPreviewWidth(typeof localStorage !== 'undefined' ? localStorage.getItem(CAT_PREVIEW_WIDTH_KEY) : null))

function paintFiles() {
  const panel = rootEl.value
  if (!panel) return
  const wide = pane.value === 'browser' || pane.value === 'term'
  const tree = wide ? extraW.value : catPane.tree
  const preview = !wide && openRel.value ? previewW.value : 0
  panel.style.width = tree + preview + 'px'
  const col = panel.querySelector('.ct-col') as HTMLElement | null
  if (col) col.style.width = tree + 'px'
}

watch([() => catPane.tree, previewW, openRel, pane, extraW], paintFiles)
let token = 0
let gitToken = 0
let searchToken = 0
let searchTimer = 0
let treeStamp = ''
let syncing = false
let treeTimer: ReturnType<typeof setInterval> | undefined

function pageBox(): { total: number; side: number } {
  const page = rootEl.value?.closest('.ct') as HTMLElement | null
  const side = (page?.querySelector('.ct-l') as HTMLElement | null)?.offsetWidth ?? CAT_SIDE_DEFAULT
  return { total: page?.clientWidth ?? window.innerWidth, side }
}

function fit(want: number): number {
  const { total, side } = pageBox()
  return clampPreviewWidth(want, total, side, catPane.tree)
}

function fitTree(want: number): number {
  const { total, side } = pageBox()
  const preview = openRel.value ? previewW.value : 0
  return clampPaneWidth(want, total, side + preview + CAT_CHAT_MIN, CAT_TREE_MIN, CAT_TREE_WIDTH)
}

function rememberWidth() {
  rememberPaneWidth(CAT_PREVIEW_WIDTH_KEY, previewW.value)
}

function onResizeDown(e: PointerEvent) {
  const panel = rootEl.value
  if (!panel) return
  const startW = previewW.value
  const { total, side } = pageBox()
  const tree = catPane.tree
  const limits = paneLimits(total, side + tree + CAT_CHAT_MIN, CAT_PREVIEW_MIN)
  let next = startW
  dragPaneWidth(e.currentTarget as HTMLElement, e, -1, (dx) => {
    next = clampToLimits(startW + dx, limits, startW)
    panel.style.width = next + tree + 'px'
  }, () => {
    previewW.value = next
    rememberWidth()
  })
}

function onTreeResizeDown(e: PointerEvent) {
  const panel = rootEl.value
  const col = panel?.querySelector('.ct-col') as HTMLElement | null
  if (!panel || !col) return
  const wide = pane.value === 'browser' || pane.value === 'term'
  const startW = col.offsetWidth || (wide ? extraW.value : catPane.tree)
  const { total, side } = pageBox()
  const preview = wide || !openRel.value ? 0 : previewW.value
  const limits = paneLimits(total, side + preview + CAT_CHAT_MIN, wide ? CAT_EXTRA_MIN : CAT_TREE_MIN)
  let next = startW
  dragPaneWidth(e.currentTarget as HTMLElement, e, -1, (dx) => {
    next = clampToLimits(startW + dx, limits, startW)
    col.style.width = next + 'px'
    panel.style.width = next + preview + 'px'
  }, () => {
    if (wide) {
      extraW.value = next
      rememberPaneWidth(CAT_EXTRA_WIDTH_KEY, next)
      return
    }
    catPane.tree = next
    rememberPaneWidth(CAT_TREE_WIDTH_KEY, next)
  })
}

function onResizeKey(e: KeyboardEvent) {
  const step = e.shiftKey ? 80 : 24
  if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
  e.preventDefault()
  previewW.value = fit(previewW.value + (e.key === 'ArrowLeft' ? step : -step))
  rememberWidth()
}

function onTreeResizeKey(e: KeyboardEvent) {
  const step = e.shiftKey ? 80 : 24
  if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
  e.preventDefault()
  const delta = e.key === 'ArrowLeft' ? step : -step
  if (pane.value === 'browser' || pane.value === 'term') {
    const { total, side } = pageBox()
    extraW.value = clampPaneWidth(extraW.value + delta, total, side + CAT_CHAT_MIN, CAT_EXTRA_MIN, CAT_EXTRA_DEFAULT)
    rememberPaneWidth(CAT_EXTRA_WIDTH_KEY, extraW.value)
    return
  }
  catPane.tree = fitTree(catPane.tree + delta)
  rememberPaneWidth(CAT_TREE_WIDTH_KEY, catPane.tree)
}

function onWindowResize() {
  const { total } = pageBox()
  // 收起宽度和动画中的临时宽度不能覆盖用户的展开宽度。
  const side = railCollapsed.value ? 56 : catPane.side
  if (pane.value === 'browser' || pane.value === 'term') {
    extraW.value = clampPaneWidth(extraW.value, total, side + CAT_CHAT_MIN, CAT_EXTRA_MIN, CAT_EXTRA_DEFAULT)
    const next = settleWidths(total, side, catPane.tree, 0, false)
    if (!railCollapsed.value) catPane.side = next.side
    return
  }
  const fitted = openRel.value ? clampPreviewWidth(previewW.value, total, side, catPane.tree) : 0
  const next = settleWidths(total, side, catPane.tree, fitted, !!openRel.value)
  if (!railCollapsed.value) catPane.side = next.side
  catPane.tree = next.tree
  if (openRel.value) previewW.value = next.preview
}

watch(railCollapsed, () => { void nextTick(onWindowResize) })

function mockGit(id: string): GitStatus {
  const changes = mockChanges(id).map((c) => {
    const slash = c.path.replace(/\\/g, '/')
    const i = slash.lastIndexOf('/')
    return {
      path: slash,
      name: i >= 0 ? slash.slice(i + 1) : slash,
      dir: i >= 0 ? slash.slice(0, i) : '',
      letter: 'M',
      added: c.add,
      deleted: c.del,
      staged: false,
      unstaged: true,
    }
  })
  const base: GitStatus = {
    repo: true,
    branch: 'main',
    upstream: 'main',
    ahead: catSim.has('gitgraph') ? 3 : changes.length ? 1 : 0,
    behind: catSim.has('gitgraph') ? 2 : 0,
    root: 'D:/Projects/ffc',
    remote: 'https://github.com/example/ffc.git',
    web: '',
    changes,
    mergeState: 'none',
    conflicts: [],
    detached: catSim.has('detached'),
    headSha: catSim.has('detached') ? 'abc1234f00dbabe5' : '59b2c16' + '0'.repeat(33),
    headShortSha: catSim.has('detached') ? 'abc1234' : '59b2c16',
  }
  if (catSim.has('detached')) base.branch = ''
  if (!catSim.has('gitconflict') && !catSim.has('gitrebase') && !catSim.has('gitdone') && !mergeSimConflict.value) return base
  const conflicts = CAT_MERGE_CONFLICT_SIM_FILES.map((path) => ({ path, ours: true, theirs: true, letter: 'UU' }))
  if (catSim.has('gitdone')) {
    return { ...base, mergeState: 'merging', conflicts: [], upstream: 'origin/main' }
  }
  if (catSim.has('gitrebase')) {
    return { ...base, mergeState: 'rebasing', conflicts, upstream: 'origin/main' }
  }
  return { ...base, mergeState: 'merging', conflicts, upstream: 'origin/main' }
}


const shownGit = computed(() => (live ? gitStatus.value : mockGit(props.convId)))
watch(
  shownGit,
  (st) => {
    setCatGitUi({
      convId: props.convId,
      live,
      status: st,
      simBranches: simGitBranches,
    })
  },
  { immediate: true },
)
const gitChanges = computed(() => shownGit.value?.changes ?? [])
const rootLabel = computed(() => {
  if (!live) {
    const dir = mockFiles(props.convId).find((n) => n[0] === 0 && n[1] !== 'f')
    return dir ? String(dir[2]) : GIT.files
  }
  return gitRootLabel(rootPath.value) || GIT.files
})

const mockRows = computed(() => {
  const q = findKind.value === 'name' ? query.value.trim().toLowerCase() : ''
  const list = mockFiles(props.convId)
  if (!q) return list
  return list.filter((n) => String(n[2]).toLowerCase().includes(q))
})

const rows = computed(() => {
  const list = flattenCatFiles(nodes.value, '')
  if (rootTruncated.value) {
    list.push({ key: '#root-cut', depth: 0, name: COPY.truncated, isDir: false, expanded: false, kind: 'note' })
  }
  return list
})

const canOpen = computed(() => live && !props.missing && error.value !== PC.missing)
const canRefresh = computed(() => live && !props.missing && !loading.value)

/** 换对话后旧树还留着，新的根目录还没回来 */
let swapping = false
const treeKey = ref(0)

watch(
  () => props.convId,
  () => {
    query.value = ''
    findKind.value = 'name'
    hits.value = null
    hitsCut.value = false
    picked.value = -1
    openRel.value = ''
    pptTabs.value = []
    openName.value = ''
    openDiff.value = false
    pending.value = null
    // B1：换对话时先留着旧的树，新数据到了再整体淡入
    swapping = nodes.value != null
    if (!swapping) nodes.value = null
    treeStamp = ''
    error.value = ''
    gitStatus.value = null
    gitError.value = ''
    void loadRoot()
    void loadGit()
  },
  { immediate: true },
)

watch(
  () => props.missing,
  (m) => {
    if (m) {
      token++
      gitToken++
      nodes.value = null
      error.value = ''
      gitStatus.value = null
      gitError.value = ''
      openRel.value = ''
      pptTabs.value = []
      openName.value = ''
      openDiff.value = false
      pending.value = null
      return
    }
    void loadRoot(collectExpanded(nodes.value))
    void loadGit()
  },
)

watch(
  () => catState.filesRev,
  () => {
    if (!live || props.missing) return
    void loadRoot(collectExpanded(nodes.value))
    void loadGit()
  },
)

watch([query, findKind], () => scheduleSearch())

watch(mockRows, (list) => {
  if (!live) picked.value = list.findIndex((n: CatTreeNode) => n[3])
}, { immediate: true })

function mockLetter(name: string): string {
  return mockChanges(props.convId).some((c) => c.path === name || c.path.endsWith('/' + name)) ? 'M' : ''
}

function rowLetter(row: CatFileRow): string {
  if (row.kind !== 'entry') return ''
  return gitLetterFor(row.key, row.isDir, gitChanges.value)
}

function hitLetter(hit: CatSearchHit): string {
  return gitLetterFor(hit.relPath, hit.isDir, gitChanges.value)
}

function parentOf(rel: string): string {
  const s = rel.replace(/\\/g, '/')
  const i = s.lastIndexOf('/')
  return i >= 0 ? s.slice(0, i) : ''
}

function scheduleSearch() {
  window.clearTimeout(searchTimer)
  const q = query.value.trim()
  if (!live || !q || props.missing) {
    hits.value = null
    hitsCut.value = false
    searching.value = false
    return
  }
  searching.value = true
  const kind = findKind.value
  const id = props.convId
  searchTimer = window.setTimeout(() => { void runSearch(id, q, kind) }, 280)
}

async function runSearch(id: string, q: string, kind: 'name' | 'content') {
  const my = ++searchToken
  try {
    const res = await searchCatFiles(id, q, kind)
    if (my !== searchToken || props.convId !== id || query.value.trim() !== q || findKind.value !== kind) return
    hits.value = res.hits
    hitsCut.value = res.truncated
  } catch (e) {
    if (my !== searchToken) return
    hits.value = null
    notify(catCallError(e, COPY.failed), 'warn')
  } finally {
    if (my === searchToken) searching.value = false
  }
}

function findNode(list: CatFileNode[] | null, rel: string): CatFileNode | null {
  if (!list) return null
  for (const n of list) {
    if (n.relPath === rel) return n
    const child = findNode(n.children, rel)
    if (child) return child
  }
  return null
}

async function loadChildren(n: CatFileNode, my: number) {
  n.loading = true
  try {
    const res = await listCatFiles(props.convId, n.relPath)
    if (my !== token) return
    n.children = entriesToNodes(res.entries, n.depth + 1)
    n.truncated = res.truncated
  } catch (e) {
    if (my !== token) return
    n.expanded = false
    n.children = null
    notify(catFilesErrorText(e), 'warn')
  } finally {
    n.loading = false
  }
}

async function expandTo(rel: string, my: number) {
  let acc = ''
  for (const part of rel.split('/').filter(Boolean)) {
    acc = acc ? `${acc}/${part}` : part
    const n = findNode(nodes.value, acc)
    if (!n?.isDir) return
    n.expanded = true
    if (n.children == null) await loadChildren(n, my)
    if (my !== token) return
  }
}

async function syncTree() {
  if (syncing || !live || props.missing || !props.convId || document.hidden || !catState.turns[props.convId] || nodes.value == null) return
  syncing = true
  const id = props.convId
  const seen = token
  try {
    const root = await listCatFiles(id, '')
    if (seen !== token || props.convId !== id) return
    const layers: { rel: string; entries: CatFileEntry[]; truncated: boolean }[] = [{ rel: '', entries: root.entries, truncated: root.truncated }]
    for (const rel of collectExpanded(nodes.value)) {
      const res = await listCatFiles(id, rel)
      if (seen !== token || props.convId !== id) return
      layers.push({ rel, entries: res.entries, truncated: res.truncated })
    }
    const stamp = layers.map((layer) => `${layer.rel}\n${layer.truncated ? 1 : 0}\n${fileEntryStamp(layer.entries)}`).join('\n\n')
    if (stamp === treeStamp) return
    treeStamp = stamp
    rootPath.value = root.root
    rootTruncated.value = root.truncated
    error.value = ''
    nodes.value = mergeFileNodes(nodes.value, root.entries, 0)
    for (const layer of layers) {
      if (!layer.rel) continue
      const dir = findNode(nodes.value, layer.rel)
      if (!dir?.isDir) continue
      dir.expanded = true
      dir.truncated = layer.truncated
      dir.children = mergeFileNodes(dir.children, layer.entries, dir.depth + 1)
    }
    if (root.entries.length && !findConv(id)?.project) catScratch.has[id] = true
  } catch {
    // A quiet refresh keeps the last tree. The refresh button still reports a failure.
  } finally {
    syncing = false
  }
}
watch(() => !!catState.turns[props.convId], (on) => {
  clearInterval(treeTimer)
  treeTimer = undefined
  if (!on) { treeStamp = ''; return }
  void syncTree()
  treeTimer = setInterval(() => void syncTree(), 1500)
}, { immediate: true })

async function loadRoot(keep: string[] = []) {
  if (!live || props.missing || !props.convId) return
  const id = props.convId
  const my = ++token
  const fresh = swapping
  swapping = false
  const had = nodes.value != null && !fresh
  if (!had && !fresh) loading.value = true
  if (!had) error.value = ''
  try {
    const res = await listCatFiles(id, '')
    if (my !== token || props.convId !== id) return
    rootPath.value = res.root
    rootTruncated.value = res.truncated
    error.value = ''
    nodes.value = entriesToNodes(res.entries, 0)
    if (fresh) treeKey.value++
    for (const rel of keep) {
      if (my !== token) return
      await expandTo(rel, my)
    }
  } catch (e) {
    if (my !== token || props.convId !== id) return
    const text = catFilesErrorText(e)
    if (had) notify(text, 'warn')
    else {
      nodes.value = []
      error.value = text
    }
  } finally {
    if (my === token) loading.value = false
  }
}

async function loadGit() {
  if (!live || props.missing || !props.convId) return
  const id = props.convId
  const my = ++gitToken
  if (!gitStatus.value) gitLoading.value = true
  try {
    const st = await gitCatStatus(id)
    if (my !== gitToken || props.convId !== id) return
    gitStatus.value = st
    gitError.value = ''
  } catch (e) {
    if (my !== gitToken || props.convId !== id) return
    gitError.value = catCallError(e, t('cat.misc.s_97847f0dd5'))
  } finally {
    if (my === gitToken) gitLoading.value = false
  }
}

function refreshGitOnFocus() {
  if (document.visibilityState === 'visible' && live && !gitLoading.value) void loadGit()
}
let gitRefreshTimer: ReturnType<typeof setInterval> | undefined
watch(() => catGitUi.historyNav, async () => {
  const scope = catGitUi.historyScope
  pane.value = 'git'
  await nextTick()
  gitPanel.value?.showHistory?.(scope)
})
watch(pane, (v) => { if (v === 'git') void loadGit() })

// v0.50 合并：出冲突 → 切到 Git 页（现有冲突解决 UI）并刷新；成功 → 只刷新
watch(() => catGitUi.conflictNav, () => { pane.value = 'git'; mergeSimConflict.value = !live && catSim.has('mergeconflict'); refresh() })
watch(() => catGitUi.gitRefresh, () => refresh())

function refresh() {
  void loadRoot(collectExpanded(nodes.value))
  void loadGit()
}

async function openFolder() {
  if (!canOpen.value) return
  try {
    await revealCatConversationFolder(props.convId)
  } catch (e) {
    const text = catFilesErrorText(e)
    if (text === PC.missing) error.value = text
    else notify(text, 'warn')
  }
}

watch(() => catWorkflow.opens[props.convId]?.serial, () => {
  const request = catWorkflow.opens[props.convId]
  if (!request) return
  pane.value = 'files'
  wantFile(request.relPath, request.name)
}, { immediate: true, flush: 'sync' })

function wantFile(rel: string, name: string, pick: number | string = rel, diff = false) {
  const gate = previewEl.value?.requestClose
  if (openRel.value && openRel.value !== rel && typeof gate === 'function' && !gate()) {
    pending.value = { rel, name, picked: pick, diff }
    return
  }
  pending.value = null
  if (officeCanvas(rel) && !diff && !pptTabs.value.some(tab => tab.rel === rel)) {
    pptTabs.value.push({ rel, name })
  }
  openRel.value = rel
  openName.value = name
  picked.value = pick
  openDiff.value = diff
}

function onGitOpen(rel: string, name: string) {
  openConflictPath.value = ''
  wantFile(rel, name, rel, true)
}

function onGitConflictOpen(rel: string, name: string) {
  openConflictPath.value = rel
  wantFile(rel, name, rel, false)
}

function onGitConflictNav() {
  pane.value = 'git'
  notify(t('cat.misc.s_e51abec0c8'), 'warn')
}

function onConflictSaved() {
  openConflictPath.value = ''
  void loadGit()
}

function onPick(note: PickNote) {
  emit('pick', note)
}

function onPreviewClose() {
  const next = pending.value
  pending.value = null
  if (next) {
    openRel.value = next.rel
    openName.value = next.name
    picked.value = next.picked
    openDiff.value = next.diff
    return
  }
  openRel.value = ''
  openName.value = ''
  openDiff.value = false
}

function closePptTab(rel: string) {
  const index = pptTabs.value.findIndex(tab => tab.rel === rel)
  pptTabs.value = pptTabs.value.filter(tab => tab.rel !== rel)
  if (openRel.value !== rel) return
  const next = pptTabs.value[Math.max(0, index - 1)]
  if (next) wantFile(next.rel, next.name)
  else onPreviewClose()
}

watch([openRel, pane], () => {
  void nextTick(onWindowResize)
})
function openExtra(kind: 'browser' | 'term') {
  addOpen.value = false
  if (kind === 'browser') showBrowser.value = true
  else showTerm.value = true
  pane.value = kind
}

function closeExtra(kind: 'browser' | 'term') {
  if (kind === 'browser') showBrowser.value = false
  else showTerm.value = false
  if (pane.value === kind) pane.value = 'files'
}

function onAddDoc(e: PointerEvent) {
  if (!addOpen.value) return
  const t = e.target
  if (t instanceof Element && t.closest('[data-cat-pane-add]')) return
  addOpen.value = false
}

onMounted(() => {
  window.addEventListener('focus', refreshGitOnFocus)
  document.addEventListener('visibilitychange', refreshGitOnFocus)
  gitRefreshTimer = setInterval(refreshGitOnFocus, 20000)
  paintFiles()
  window.addEventListener('resize', onWindowResize)
  window.addEventListener('pointerdown', onAddDoc)
  onWindowResize()
})
onBeforeUnmount(() => {
  clearInterval(treeTimer)
  window.removeEventListener('focus', refreshGitOnFocus)
  document.removeEventListener('visibilitychange', refreshGitOnFocus)
  clearInterval(gitRefreshTimer)
  window.removeEventListener('resize', onWindowResize)
  window.removeEventListener('pointerdown', onAddDoc)
  window.clearTimeout(searchTimer)
})

function onMock(n: CatTreeNode, i: number) {
  if (n[1] !== 'f') return
  wantFile(String(n[2]), String(n[2]), i)
}

function onRow(row: CatFileRow) {
  if (row.kind !== 'entry') return
  if (!row.isDir) {
    wantFile(row.key, row.name, row.key)
    return
  }
  const n = findNode(nodes.value, row.key)
  if (!n) return
  if (n.expanded) {
    n.expanded = false
    return
  }
  n.expanded = true
  if (n.children == null) void loadChildren(n, token)
}

async function onHit(hit: CatSearchHit) {
  if (hit.isDir) {
    query.value = ''
    hits.value = null
    await expandTo(hit.relPath, token)
    picked.value = hit.relPath
    return
  }
  wantFile(hit.relPath, hit.name, hit.relPath)
}

defineExpose({
  showGit() {
    pane.value = 'git'
  },
  openBranchMenu() {
    pane.value = 'git'
    void Promise.resolve().then(() => gitPanel.value?.openBranchMenu?.())
  },
  getStatus: () => shownGit.value,
})
</script>

<style scoped>
.ct-previews{display:flex;flex-direction:column;flex:1;min-width:0;min-height:0;overflow:hidden}.ct-ppt-tabs{display:flex;flex:none;overflow:auto;border-bottom:1px solid var(--cat-border);background:var(--cat-bg)}.ct-ppt-tab{display:flex;align-items:center;max-width:220px;border-right:1px solid var(--cat-border)}.ct-ppt-tab.active{background:var(--cat-surface);box-shadow:inset 0 -2px var(--cat-accent)}.ct-ppt-tab button{border:0;background:transparent;color:var(--cat-muted);font:inherit;font-size:11px;cursor:pointer;padding:8px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.ct-ppt-tab.active button{color:var(--cat-text)}.ct-ppt-tab button:last-child{flex:none}.ct-previews :deep(.pv){width:100%}
.ct-r {
  flex: none;
  display: flex;
  flex-direction: row;
  border-left: 1px solid var(--cat-border);
  background: var(--cat-surface);
  min-width: 0;
  min-height: 0;
  position: relative;
}
.ct-r.wide {
  position: relative;
  overflow: hidden;
}
.ct-resize {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 6px;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: col-resize;
  z-index: 5;
  touch-action: none;
  --wails-draggable: no-drag;
}
.ct-resize::after {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 2px;
  background: transparent;
}
.ct-resize:hover::after,
.ct-resize:focus-visible::after {
  background: var(--cat-accent);
}
.ct-resize:focus-visible {
  outline: none;
}
.ct-col {
  position: relative;
  flex: none;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.ct-r.wide .ct-col {
  border-left: 1px solid var(--cat-border);
}
.ct-modes {
  position: relative;
  height: 36px;
  flex: none;
  display: flex;
  align-items: stretch;
  gap: 2px;
  padding: 0 6px;
  border-bottom: 1px solid var(--cat-border);
}
/* E1 图标 + 文字页签；底部主色指示条在页签间滑动 200ms */
.ct-modes button {
  width: 64px;
  border: none;
  background: transparent;
  color: var(--cat-muted);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 0;
  font: inherit;
  font-size: 12.5px;
  border-radius: var(--cat-radius-sm) var(--cat-radius-sm) 0 0;
  transition: color var(--ff-dur-quick), background var(--ff-dur-quick);
}
.ct-modes button.on {
  color: var(--cat-text);
  font-weight: 500;
}
.ct-modes button:hover {
  color: var(--cat-text);
  background: var(--cat-hover);
}
.ct-modes button:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.ct-modes .ink {
  position: absolute;
  left: 6px;
  bottom: -1px;
  width: 64px;
  height: 2px;
  border-radius: 1px;
  background: var(--cat-accent);
  transform: translateX(calc(var(--tab-i, 0) * 66px));
  transition: transform var(--ff-dur-base) var(--ff-ease-out);
}
.ct-modes button.icon,
.ct-modes .add {
  width: 32px;
  box-shadow: none;
}
.ct-modes .add {
  height: 28px;
  margin: auto 0;
  border-radius: var(--cat-radius-sm);
}
.add-wrap {
  position: relative;
  margin-left: auto;
  display: flex;
}
.add-menu {
  position: absolute;
  top: calc(100% + 4px);
  right: 0;
  z-index: 8;
  min-width: 132px;
  padding: 4px;
  border-radius: 10px;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
}
.add-menu button {
  width: 100%;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
  padding: 0 8px;
  border-radius: 6px;
  box-shadow: none;
  color: var(--ff-text-1);
  font: inherit;
  font-size: 13px;
}
.add-menu button:hover {
  background: var(--ff-bg-hover);
}
.ct-head {
  height: 32px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 0 6px 0 12px;
}
.ct-root {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 650;
  color: var(--cat-text);
}
.ib {
  width: 24px;
  height: 24px;
  display: grid;
  place-items: center;
  border-radius: 6px;
  border: none;
  padding: 0;
  background: transparent;
  color: var(--cat-text-2);
  cursor: pointer;
}
.ib:hover:not(:disabled) {
  background: var(--cat-hover);
}
.ib:disabled {
  opacity: 0.4;
  cursor: default;
}
.ct-search {
  margin: 8px 10px;
  height: 28px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: var(--cat-hover);
  font-size: 12px;
  color: var(--cat-muted);
}
.ct-search:focus-within {
  border-color: var(--cat-border);
  background: var(--cat-surface);
}
.ct-search input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 12px;
  color: var(--cat-text);
}
.ct-search input::placeholder {
  color: var(--cat-muted);
}
.ct-seg {
  margin: 0 10px 8px;
  height: 28px;
  flex: none;
  display: flex;
  padding: 2px;
  border-radius: 8px;
  background: var(--cat-hover);
}
.ct-seg button {
  flex: 1;
  border: none;
  background: transparent;
  border-radius: 6px;
  font: inherit;
  font-size: 12px;
  color: var(--cat-text-2);
  cursor: pointer;
}
.ct-seg button.on {
  background: var(--cat-surface);
  color: var(--cat-text);
  font-weight: 650;
  box-shadow: var(--cat-shadow-sm);
}
.ct-tree {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0 6px 8px;
  overflow-y: auto;
  min-height: 0;
}
.ct-node {
  min-height: 26px;
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  color: var(--cat-text);
  border-radius: 6px;
  white-space: nowrap;
  flex: none;
  cursor: pointer;
}
.ct-node > :deep(svg) {
  color: var(--cat-text-2);
  flex: none;
}
.ct-node > :deep(svg.car) {
  color: var(--cat-muted);
}
.ct-node > :deep(svg.fi) {
  color: var(--cat-file-icon);
}
.car-slot {
  width: 12px;
  flex: none;
}
.ct-node.on {
  background: var(--cat-hover);
}
.ct-node.note {
  color: var(--cat-muted);
  font-size: 12px;
  padding-left: 8px;
  cursor: default;
}
.ct-node .nm {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ct-node.hit {
  height: auto;
  align-items: flex-start;
  padding: 4px 4px 4px 8px;
  cursor: pointer;
}
.hit-main {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px;
}
.dir, .snip {
  color: var(--cat-muted);
  font-size: 12px;
}
.dir {
  flex: none;
  max-width: 46%;
  overflow: hidden;
  text-overflow: ellipsis;
}
.snip {
  flex-basis: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}
.badge {
  margin-left: auto;
  flex: none;
  font-size: 11px;
  font-weight: 700;
  padding-right: 4px;
}
.st-M, .st-R { color: var(--cat-git-mod); }
.st-A, .st-U { color: var(--cat-success-text); }
.st-D { color: var(--cat-danger-text); }
.ct-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: var(--cat-muted);
  font-size: 13px;
  padding: 0 16px 80px;
  text-align: center;
}
.retry {
  margin-top: 4px;
  border: 1px solid var(--cat-border);
  background: transparent;
  color: var(--cat-text-2);
  border-radius: 6px;
  padding: 4px 10px;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.retry:hover {
  background: var(--cat-hover);
}
.retry.primary {
  background: var(--cat-accent);
  border-color: var(--cat-accent);
  color: var(--cat-on-accent);
}
.retry.primary:hover {
  background: var(--cat-accent-hover);
}
.swap-in {
  animation: cat-fade-in var(--ff-dur-base) var(--ff-ease-out) both;
}
</style>
