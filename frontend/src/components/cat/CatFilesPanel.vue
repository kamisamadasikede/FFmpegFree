<template>
  <section
    ref="rootEl"
    class="ct-r"
    :class="{ wide: !!openRel }"
    :style="openRel ? { width: panelWidth + 'px' } : undefined"
    :aria-label="pane === 'git' ? GIT.git : GIT.files"
    data-testid="cat-files"
  >
    <div
      v-if="openRel"
      class="ct-resize"
      role="separator"
      tabindex="0"
      aria-orientation="vertical"
      aria-label="拖动调整文件预览宽度"
      :aria-valuenow="previewW"
      :aria-valuemin="160"
      data-testid="cat-file-resize"
      @pointerdown="onResizeDown"
      @keydown="onResizeKey"
    />
    <CatFilePreview
      v-if="openRel"
      ref="previewEl"
      :conv-id="convId"
      :rel-path="openRel"
      :name="openName"
      :live="live"
      :compare="openDiff"
      @close="onPreviewClose"
    />
    <div class="ct-col">
      <div class="ct-modes" role="tablist">
        <button type="button" role="tab" :class="{ on: pane === 'files' }" :aria-selected="pane === 'files'" :title="GIT.files" data-testid="cat-files-tab" @click="pane = 'files'">
          <FIcon name="file" :size="16" />
        </button>
        <button type="button" role="tab" :class="{ on: pane === 'git' }" :aria-selected="pane === 'git'" :title="GIT.git" data-testid="cat-git-tab" @click="pane = 'git'">
          <FIcon name="branch" :size="16" />
        </button>
      </div>
      <div v-if="missing || error === PC.missing" class="ct-empty" data-testid="cat-files-missing">
        <FIcon name="warn" :size="24" /><span>{{ PC.missing }}</span>
      </div>
      <CatGitPanel
        v-else-if="pane === 'git'"
        :conv-id="convId"
        :live="live"
        :status="shownGit"
        :loading="gitLoading"
        :load-error="gitError"
        @refresh="loadGit"
        @open="onGitOpen"
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
          <FIcon name="file" :size="28" /><span>{{ query.trim() ? COPY.noMatch : COPY.empty }}</span>
        </div>
        <div v-else class="ct-tree" data-testid="cat-files-tree">
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
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
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
  listCatFiles,
  revealCatConversationFolder,
  searchCatFiles,
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
import { catState, notify } from '@/views/cat/catState'
import {
  CAT_PREVIEW_WIDTH_KEY,
  CAT_TREE_WIDTH,
  clampPreviewWidth,
  readPreviewWidth,
} from './catPreviewWidth'
import CatGitPanel from './CatGitPanel.vue'

const CatFilePreview = defineAsyncComponent(() => import('./CatFilePreview.vue'))

const props = defineProps<{ convId: string; missing?: boolean }>()
const live = isCatLive()
const pane = ref<'files' | 'git'>('files')
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
const openName = ref('')
const openDiff = ref(false)
const pending = ref<{ rel: string; name: string; picked: number | string; diff: boolean } | null>(null)
const previewEl = ref<{ requestClose: () => boolean } | null>(null)
const rootEl = ref<HTMLElement>()
const previewW = ref(readPreviewWidth(typeof localStorage !== 'undefined' ? localStorage.getItem(CAT_PREVIEW_WIDTH_KEY) : null))
const panelWidth = computed(() => CAT_TREE_WIDTH + previewW.value)
let token = 0
let gitToken = 0
let searchToken = 0
let searchTimer = 0

function pageBox(): { total: number; side: number } {
  const page = rootEl.value?.closest('.ct') as HTMLElement | null
  const side = (page?.querySelector('.ct-l') as HTMLElement | null)?.offsetWidth ?? 248
  return { total: page?.clientWidth ?? window.innerWidth, side }
}

function fit(want: number): number {
  const { total, side } = pageBox()
  return clampPreviewWidth(want, total, side)
}

function rememberWidth() {
  try {
    localStorage.setItem(CAT_PREVIEW_WIDTH_KEY, String(previewW.value))
  } catch {
    /* 隐私模式写不进就只在这次运行里生效 */
  }
}

function onResizeDown(e: PointerEvent) {
  if (e.button !== 0) return
  e.preventDefault()
  const handle = e.currentTarget as HTMLElement
  handle.setPointerCapture(e.pointerId)
  const startX = e.clientX
  const startW = previewW.value
  const prevCursor = document.body.style.cursor
  const prevSelect = document.body.style.userSelect
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  const move = (ev: PointerEvent) => {
    previewW.value = fit(startW + (startX - ev.clientX))
  }
  const up = (ev: PointerEvent) => {
    if (handle.hasPointerCapture(ev.pointerId)) handle.releasePointerCapture(ev.pointerId)
    handle.removeEventListener('pointermove', move)
    handle.removeEventListener('pointerup', up)
    document.body.style.cursor = prevCursor
    document.body.style.userSelect = prevSelect
    rememberWidth()
  }
  handle.addEventListener('pointermove', move)
  handle.addEventListener('pointerup', up)
}

function onResizeKey(e: KeyboardEvent) {
  const step = e.shiftKey ? 80 : 24
  if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
  e.preventDefault()
  previewW.value = fit(previewW.value + (e.key === 'ArrowLeft' ? step : -step))
  rememberWidth()
}

function onWindowResize() {
  if (openRel.value) previewW.value = fit(previewW.value)
}

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
  return { repo: true, branch: 'main', upstream: 'main', ahead: changes.length ? 1 : 0, behind: 0, remote: '', web: '', changes }
}

const shownGit = computed(() => (live ? gitStatus.value : mockGit(props.convId)))
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

watch(
  () => props.convId,
  () => {
    query.value = ''
    findKind.value = 'name'
    hits.value = null
    hitsCut.value = false
    picked.value = -1
    openRel.value = ''
    openName.value = ''
    openDiff.value = false
    pending.value = null
    nodes.value = null
    rootPath.value = ''
    rootTruncated.value = false
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

async function loadRoot(keep: string[] = []) {
  if (!live || props.missing || !props.convId) return
  const id = props.convId
  const my = ++token
  const had = nodes.value != null
  if (!had) loading.value = true
  if (!had) error.value = ''
  try {
    const res = await listCatFiles(id, '')
    if (my !== token || props.convId !== id) return
    rootPath.value = res.root
    rootTruncated.value = res.truncated
    error.value = ''
    nodes.value = entriesToNodes(res.entries, 0)
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
    gitError.value = catCallError(e, 'git 没有完成。')
  } finally {
    if (my === gitToken) gitLoading.value = false
  }
}

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

function wantFile(rel: string, name: string, pick: number | string = rel, diff = false) {
  const gate = previewEl.value?.requestClose
  if (openRel.value && openRel.value !== rel && typeof gate === 'function' && !gate()) {
    pending.value = { rel, name, picked: pick, diff }
    return
  }
  pending.value = null
  openRel.value = rel
  openName.value = name
  picked.value = pick
  openDiff.value = diff
}

function onGitOpen(rel: string, name: string) {
  wantFile(rel, name, rel, true)
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

watch(openRel, (rel) => {
  if (rel) void nextTick(() => { previewW.value = fit(previewW.value) })
})
onMounted(() => window.addEventListener('resize', onWindowResize))
onBeforeUnmount(() => {
  window.removeEventListener('resize', onWindowResize)
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
</script>

<style scoped>
.ct-r {
  width: 236px;
  flex: none;
  display: flex;
  flex-direction: row;
  border-left: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  min-width: 0;
  min-height: 0;
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
  background: var(--ff-primary);
}
.ct-resize:focus-visible {
  outline: none;
}
.ct-col {
  width: 236px;
  flex: none;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.ct-r.wide .ct-col {
  border-left: 1px solid var(--ff-border);
}
.ct-modes {
  height: 36px;
  flex: none;
  display: flex;
  align-items: stretch;
  gap: 2px;
  padding: 0 6px;
  border-bottom: 1px solid var(--ff-border);
}
.ct-modes button {
  width: 32px;
  border: none;
  background: transparent;
  color: var(--ff-text-3);
  cursor: pointer;
  display: grid;
  place-items: center;
  padding: 0;
  box-shadow: inset 0 -2px 0 transparent;
}
.ct-modes button.on {
  color: var(--ff-text-1);
  box-shadow: inset 0 -2px 0 var(--ff-text-1);
}
.ct-modes button:hover {
  color: var(--ff-text-1);
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
  color: var(--ff-text-1);
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
  color: var(--ff-text-2);
  cursor: pointer;
}
.ib:hover:not(:disabled) {
  background: var(--ff-bg-hover);
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
  background: var(--ff-bg-hover);
  font-size: 12px;
  color: var(--ff-text-3);
}
.ct-search:focus-within {
  border-color: var(--ff-border);
  background: var(--ff-bg-surface);
}
.ct-search input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 12px;
  color: var(--ff-text-1);
}
.ct-search input::placeholder {
  color: var(--ff-text-3);
}
.ct-seg {
  margin: 0 10px 8px;
  height: 28px;
  flex: none;
  display: flex;
  padding: 2px;
  border-radius: 8px;
  background: var(--ff-bg-hover);
}
.ct-seg button {
  flex: 1;
  border: none;
  background: transparent;
  border-radius: 6px;
  font: inherit;
  font-size: 12px;
  color: var(--ff-text-2);
  cursor: pointer;
}
.ct-seg button.on {
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font-weight: 650;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
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
  color: var(--ff-text-1);
  border-radius: 6px;
  white-space: nowrap;
  flex: none;
  cursor: pointer;
}
.ct-node > :deep(svg) {
  color: var(--ff-text-2);
  flex: none;
}
.ct-node > :deep(svg.car) {
  color: var(--ff-text-3);
}
.ct-node > :deep(svg.fi) {
  color: #d9622b;
}
.car-slot {
  width: 12px;
  flex: none;
}
.ct-node.on {
  background: var(--ff-bg-hover);
}
.ct-node.note {
  color: var(--ff-text-3);
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
  color: var(--ff-text-3);
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
.st-M, .st-R { color: #b45309; }
.st-A, .st-U { color: var(--ff-success-text); }
.st-D { color: var(--ff-danger-text); }
html.dark .st-M,
html.dark .st-R { color: #f0a05a; }
.ct-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: var(--ff-text-3);
  font-size: 13px;
  padding: 0 16px 80px;
  text-align: center;
}
.retry {
  margin-top: 4px;
  border: 1px solid var(--ff-border);
  background: transparent;
  color: var(--ff-text-2);
  border-radius: 6px;
  padding: 4px 10px;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.retry:hover {
  background: var(--ff-bg-hover);
}
</style>
