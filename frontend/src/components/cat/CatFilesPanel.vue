<template>
  <section class="ct-r" :class="{ wide: !!openRel }" aria-label="文件" data-testid="cat-files">
    <CatFilePreview
      v-if="openRel"
      ref="previewEl"
      :conv-id="convId"
      :rel-path="openRel"
      :name="openName"
      :live="live"
      @close="onPreviewClose"
    />
    <div class="ct-col">
    <div class="ct-tabs" role="tablist">
      <button type="button" role="tab" class="on" aria-selected="true">文件</button>
      <span class="sp" />
      <button type="button" class="ib" :title="rootPath || COPY.open" :disabled="!canOpen" data-testid="cat-files-open" @click="openFolder">
        <FIcon name="folder" :size="15" />
      </button>
      <button type="button" class="ib" :title="COPY.refresh" :disabled="!canRefresh" data-testid="cat-files-refresh" @click="refresh">
        <FIcon name="refresh" :size="15" />
      </button>
    </div>
    <div v-if="missing || error === PC.missing" class="ct-empty" data-testid="cat-files-missing">
      <FIcon name="warn" :size="24" /><span>{{ PC.missing }}</span>
    </div>
    <template v-else>
      <label class="ct-search">
        <FIcon name="search" :size="13" />
        <input v-model="query" type="search" :placeholder="COPY.search" data-testid="cat-files-search" />
      </label>
      <div v-if="!live" class="ct-tree">
        <div
          v-for="(n, i) in mockRows"
          :key="i"
          class="ct-node"
          :class="{ on: picked === i }"
          :style="{ paddingLeft: n[0] * 16 + (n[1] === 'f' ? 18 : 0) + 'px' }"
          @click="onMock(n, i)"
        >
          <template v-if="n[1] === 'f'"><FIcon name="file" :size="14" class="fi" /></template>
          <template v-else>
            <FIcon :name="n[1] === 'd' ? 'down' : 'right'" :size="12" class="car" /><FIcon name="folder" :size="14" />
          </template>
          <span class="nm">{{ n[2] }}</span>
        </div>
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
          :style="{ paddingLeft: row.depth * 16 + (row.kind === 'entry' && !row.isDir ? 18 : 0) + 'px' }"
          @click="onRow(row)"
        >
          <template v-if="row.kind === 'entry' && row.isDir">
            <FIcon :name="row.expanded ? 'down' : 'right'" :size="12" class="car" /><FIcon name="folder" :size="14" />
          </template>
          <FIcon v-else-if="row.kind === 'entry'" name="file" :size="14" class="fi" />
          <span class="nm">{{ row.name }}</span>
        </div>
      </div>
    </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { mockFiles, type CatTreeNode } from '@/api/catMock'
import { CAT_PROJECT_COPY as PC } from '@/api/catProjects'
import { isCatLive } from '@/api/cat'
import {
  CAT_FILES_COPY as COPY,
  catFilesErrorText,
  collectExpanded,
  entriesToNodes,
  flattenCatFiles,
  listCatFiles,
  revealCatConversationFolder,
  type CatFileNode,
  type CatFileRow,
} from '@/api/catFiles'
import { catState, notify } from '@/views/cat/catState'

const CatFilePreview = defineAsyncComponent(() => import('./CatFilePreview.vue'))

const props = defineProps<{ convId: string; missing?: boolean }>()
const live = isCatLive()
const query = ref('')
const nodes = ref<CatFileNode[] | null>(null)
const rootPath = ref('')
const rootTruncated = ref(false)
const loading = ref(false)
const error = ref('')
const picked = ref<number | string>(-1)
const openRel = ref('')
const openName = ref('')
const pending = ref<{ rel: string; name: string; picked: number | string } | null>(null)
const previewEl = ref<{ requestClose: () => boolean } | null>(null)
let token = 0

const mockRows = computed(() => {
  const q = query.value.trim().toLowerCase()
  const list = mockFiles(props.convId)
  if (!q) return list
  return list.filter((n) => String(n[2]).toLowerCase().includes(q))
})

const rows = computed(() => {
  const list = flattenCatFiles(nodes.value, query.value)
  const filtering = !!query.value.trim()
  if (rootTruncated.value && (!filtering || list.length > 0)) {
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
    picked.value = -1
    openRel.value = ''
    openName.value = ''
    pending.value = null
    nodes.value = null
    rootPath.value = ''
    rootTruncated.value = false
    error.value = ''
    void loadRoot()
  },
  { immediate: true },
)

watch(
  () => props.missing,
  (m) => {
    if (m) {
      token++
      nodes.value = null
      error.value = ''
      openRel.value = ''
      openName.value = ''
      pending.value = null
      return
    }
    void loadRoot(collectExpanded(nodes.value))
  },
)

watch(
  () => catState.filesRev,
  () => {
    if (!live || props.missing) return
    void loadRoot(collectExpanded(nodes.value))
  },
)

watch(mockRows, (list) => {
  if (!live) picked.value = list.findIndex((n: CatTreeNode) => n[3])
}, { immediate: true })

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

function refresh() {
  void loadRoot(collectExpanded(nodes.value))
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

function wantFile(rel: string, name: string, pick: number | string) {
  const gate = previewEl.value?.requestClose
  if (openRel.value && openRel.value !== rel && typeof gate === 'function' && !gate()) {
    pending.value = { rel, name, picked: pick }
    return
  }
  pending.value = null
  openRel.value = rel
  openName.value = name
  picked.value = pick
}

function onPreviewClose() {
  const next = pending.value
  pending.value = null
  if (next) {
    openRel.value = next.rel
    openName.value = next.name
    picked.value = next.picked
    return
  }
  openRel.value = ''
  openName.value = ''
}

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
</script>

<style scoped>
.ct-r {
  width: 236px;
  flex: none;
  display: flex;
  flex-direction: row;
  border-left: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  min-height: 0;
}
.ct-r.wide {
  width: min(820px, 68vw);
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
.ct-tabs {
  height: 44px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 10px;
  border-bottom: 1px solid var(--ff-border);
}
.ct-tabs button[role='tab'] {
  font: inherit;
  font-size: 13px;
  padding: 4px 8px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--ff-text-2);
  cursor: pointer;
}
.ct-tabs button.on {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
  font-weight: 500;
}
.ct-tabs .sp {
  flex: 1;
}
.ib {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  border-radius: 5px;
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
  margin: 10px;
  height: 30px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  font-size: 12px;
  color: var(--ff-text-3);
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
.ct-tree {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0 6px 8px;
  overflow-y: auto;
  min-height: 0;
}
.ct-node {
  height: 28px;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--ff-text-1);
  border-radius: 6px;
  white-space: nowrap;
  flex: none;
  cursor: default;
}
.ct-node > :deep(svg) {
  color: var(--ff-text-2);
}
.ct-node > :deep(svg.car) {
  color: var(--ff-text-3);
}
.ct-node > :deep(svg.fi) {
  color: #d9622b;
}
.ct-node.on {
  background: var(--ff-bg-hover);
}
.ct-node.note {
  color: var(--ff-text-3);
  font-size: 12px;
}
.ct-node .nm {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
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
