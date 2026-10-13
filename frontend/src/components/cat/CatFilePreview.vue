<template>
  <section class="pv" data-testid="cat-file-preview" :aria-label="title">
    <header class="hd">
      <span class="nm" :title="title">{{ title }}</span>
      <span v-if="dirty || saving" class="dot" :title="COPY.saving" />
      <span class="sp" />
      <div v-if="toggle" class="seg" role="tablist">
        <button type="button" :class="{ on: mode === 'preview' }" @click="mode = 'preview'">{{ COPY.preview }}</button>
        <button type="button" :class="{ on: mode === 'source' }" @click="mode = 'source'">{{ COPY.source }}</button>
      </div>
      <button
        v-if="htmlPage"
        type="button"
        class="icon-btn"
        :class="{ on: mode === 'preview' }"
        data-testid="cat-html-preview"
        :aria-pressed="mode === 'preview'"
        :aria-label="COPY.htmlPreview"
        :title="COPY.htmlPreview"
        @click="mode = mode === 'preview' ? 'source' : 'preview'"
      >
        <FIcon name="eye" :size="15" />
      </button>
      <button type="button" class="x" data-testid="cat-file-close" @click="onClose">{{ COPY.close }}</button>
    </header>
    <CatFileEditor
      v-if="deletedView && baseOriginal != null"
      :key="'gone:' + relPath"
      :text="baseOriginal"
      language=""
      :name="title"
      read-only
      :marks="deletedMarks"
      data-testid="cat-file-diff"
    />
    <div v-else-if="showDiff && diffBinary" class="state">{{ GIT.diffBinary }}</div>
    <div v-else-if="loading && !ready" class="state">{{ COPY.loading }}</div>
    <div v-else-if="error" class="state">{{ error }}</div>
    <div v-else-if="showDiff && diffError && !ready" class="state">{{ diffError }}</div>
    <template v-else-if="ready && doc">
      <iframe v-if="showMd && mdHtml" class="md" sandbox="" :title="title" :srcdoc="mdHtml" />
      <div v-else-if="showMd" class="state">{{ COPY.loading }}</div>
      <img v-else-if="showSvg && svgUrl" class="svg" :src="svgUrl" alt="" />
      <iframe v-else-if="showHtml" class="md" sandbox="allow-scripts" referrerpolicy="no-referrer" :title="title" :srcdoc="draft" />
      <CatFileEditor v-else-if="showEditor" :key="editorKey" :text="draft" :language="doc.language || 'plaintext'" :name="doc.name" :read-only="!doc.editable" :marks="editorMarks" @change="draft = $event" @save="flushSave" />
      <CatFileRich v-else-if="rich" :kind="doc.kind" :mime="doc.mime" :b64="doc.dataBase64" :name="doc.name" />
      <div v-else class="state">{{ message }}</div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatFileEditor from './CatFileEditor.vue'
import CatFileRich from './CatFileRich.vue'
import { CAT_GIT_COPY as GIT, catGitError, gitCatDiff } from '@/api/catGit'
import { diffSides, editorDiffMarks, removedFileMarks, type EditorDiffMarks } from './catInlineDiff'
import { buildSrcdoc, renderMarkdown } from '@/utils/docSafeHtml'
import { notify } from '@/views/cat/catState'
import {
  CAT_AUTOSAVE_MS,
  CAT_PREVIEW_COPY as COPY,
  catPreviewErrorText,
  mockCatFileBody,
  previewModeFor,
  readCatFile,
  writeCatFile,
  type CatFileBody,
} from '@/api/catFilePreview'

const props = defineProps<{ convId: string; relPath: string; name: string; live: boolean; compare?: boolean }>()
const emit = defineEmits<{ close: [] }>()

const doc = ref<CatFileBody | null>(null)
const draft = ref('')
const mode = ref<'preview' | 'source'>('source')
const loading = ref(false)
const ready = ref(false)
const saving = ref(false)
const error = ref('')
const mdHtml = ref('')
const editorKey = ref(0)
const diffLoading = ref(false)
const diffError = ref('')
const diffText = ref('')
const diffBinary = ref(false)
const baseOriginal = ref<string | null>(null)
let capturedFor = ''
let token = 0
let mdToken = 0
let diffToken = 0
let saveTimer: ReturnType<typeof setTimeout> | undefined
let writing = false
let writeAgain = false
let leaveAfterSave = false

const title = computed(() => doc.value?.name || props.name || props.relPath)
const view = computed(() => previewModeFor(doc.value?.name || props.name || props.relPath))
const toggle = computed(() => !!doc.value && doc.value.kind === 'text' && (view.value === 'markdown' || view.value === 'svg'))
const htmlPage = computed(() => !!doc.value && doc.value.kind === 'text' && view.value === 'html')
const showMd = computed(() => !!doc.value && doc.value.kind === 'text' && view.value === 'markdown' && mode.value === 'preview')
const showSvg = computed(() => !!doc.value && doc.value.kind === 'text' && view.value === 'svg' && mode.value === 'preview')
/** html 页用独立来源的 iframe 渲染：脚本能跑，但碰不到外面的应用 */
const showHtml = computed(() => htmlPage.value && mode.value === 'preview')
const showEditor = computed(() => !!doc.value && doc.value.kind === 'text' && !showMd.value && !showSvg.value && !showHtml.value)
const rich = computed(() => !!doc.value && ['image', 'media', 'pdf', 'docx', 'xlsx', 'pptx'].includes(doc.value.kind))
const dirty = computed(() => !!doc.value?.editable && draft.value !== doc.value.content)
const svgUrl = computed(() => (showSvg.value ? 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(draft.value) : ''))
const message = computed(() => {
  if (doc.value?.kind === 'tooLarge') return COPY.tooLarge
  if (doc.value?.kind === 'doc') return COPY.doc
  return COPY.binary
})
/** 只有从源代码管理打开时，才把新增和删除标进正文。编辑器本身保持可改。 */
const showDiff = computed(() => props.compare === true && props.live)
const deletedView = computed(() => showDiff.value && !ready.value && !!error.value && baseOriginal.value != null)
const deletedMarks = computed(() => (baseOriginal.value != null ? removedFileMarks(baseOriginal.value) : null))
const editorMarks = computed((): EditorDiffMarks | null => {
  if (!showDiff.value || !ready.value || baseOriginal.value == null) return null
  const marks = editorDiffMarks(baseOriginal.value, draft.value)
  if (!marks.added.length && !marks.deleted.length) return null
  return marks
})

watch(
  () => [props.convId, props.relPath] as const,
  () => {
    void load(true)
  },
  { immediate: true },
)

watch(
  () => [props.convId, props.relPath, props.compare] as const,
  () => {
    baseOriginal.value = null
    capturedFor = ''
    void loadDiff()
  },
  { immediate: true },
)

watch(
  () => [props.relPath, props.compare, diffText.value, diffBinary.value, diffLoading.value, ready.value, error.value, editorKey.value] as const,
  () => {
    if (!props.compare || diffBinary.value || diffLoading.value || !diffText.value) {
      if (!props.compare || diffBinary.value) {
        baseOriginal.value = null
        capturedFor = ''
      }
      return
    }
    if (!ready.value && !error.value) return
    const key = `${props.relPath}:${editorKey.value}:${error.value ? 'gone' : 'file'}`
    if (capturedFor === key) return
    const sides = diffSides(ready.value ? draft.value : '', diffText.value)
    capturedFor = key
    baseOriginal.value = sides ? sides.original : null
  },
)

watch([showMd, draft], () => {
  void paintMarkdown()
})

watch(draft, () => {
  const body = doc.value
  if (!body?.editable || draft.value === body.content) return
  scheduleSave()
})

async function loadDiff() {
  const my = ++diffToken
  const id = props.convId
  const rel = props.relPath
  const want = props.compare === true && props.live
  diffError.value = ''
  diffText.value = ''
  diffBinary.value = false
  diffLoading.value = want
  if (!want) return
  try {
    const body = await gitCatDiff(id, rel)
    if (my !== diffToken || props.convId !== id || props.relPath !== rel || props.compare !== true) return
    diffText.value = body.text
    diffBinary.value = body.binary
  } catch (e) {
    if (my !== diffToken || props.convId !== id || props.relPath !== rel) return
    diffError.value = catGitError(e)
  } finally {
    if (my === diffToken) diffLoading.value = false
  }
}

async function load(resetMode: boolean) {
  const my = ++token
  const id = props.convId
  const rel = props.relPath
  loading.value = true
  ready.value = false
  error.value = ''
  leaveAfterSave = false
  clearTimeout(saveTimer)
  try {
    const body = props.live ? await readCatFile(id, rel) : mockCatFileBody(props.name || rel)
    if (my !== token || props.convId !== id || props.relPath !== rel) return
    doc.value = body
    draft.value = body.content
    editorKey.value++
    ready.value = true
    if (resetMode) {
      const kind = previewModeFor(body.name)
      mode.value = props.compare ? 'source' : kind === 'markdown' || kind === 'svg' ? 'preview' : 'source'
    }
  } catch (e) {
    if (my !== token || props.convId !== id || props.relPath !== rel) return
    doc.value = null
    ready.value = false
    error.value = catPreviewErrorText(e, COPY.failed)
  } finally {
    if (my === token) loading.value = false
  }
}

async function paintMarkdown() {
  const my = ++mdToken
  if (!showMd.value) {
    mdHtml.value = ''
    return
  }
  const html = await renderMarkdown(draft.value)
  if (my !== mdToken) return
  const dark = document.documentElement.classList.contains('dark')
  mdHtml.value = buildSrcdoc(html, { dark, title: title.value })
}

function scheduleSave() {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => { void writeDraft() }, CAT_AUTOSAVE_MS)
}

function flushSave() {
  clearTimeout(saveTimer)
  void writeDraft()
}

async function writeDraft() {
  clearTimeout(saveTimer)
  const body = doc.value
  if (!body?.editable) {
    if (leaveAfterSave) finishLeave()
    return
  }
  const text = draft.value
  if (text === body.content) {
    if (leaveAfterSave) finishLeave()
    return
  }
  if (writing) {
    writeAgain = true
    return
  }
  writing = true
  saving.value = true
  let failed = false
  try {
    if (props.live) {
      const wrote = await writeCatFile(props.convId, body.relPath, text)
      if (doc.value !== body) return
      body.size = wrote.size
      body.modTime = wrote.modTime
    }
    if (doc.value !== body) return
    if (draft.value === text) body.content = text
  } catch (e) {
    failed = true
    if (doc.value === body) notify(catPreviewErrorText(e, COPY.saveFailed), 'warn')
    leaveAfterSave = false
  } finally {
    writing = false
    saving.value = false
    if (doc.value !== body) return
    if (!failed && (writeAgain || draft.value !== body.content)) {
      writeAgain = false
      if (leaveAfterSave) void writeDraft()
      else scheduleSave()
      return
    }
    writeAgain = false
    if (leaveAfterSave && !failed) finishLeave()
  }
}

function finishLeave() {
  leaveAfterSave = false
  emit('close')
}

function requestClose(): boolean {
  if (!doc.value?.editable || (!dirty.value && !saving.value)) return true
  leaveAfterSave = true
  flushSave()
  return false
}

onBeforeUnmount(() => {
  clearTimeout(saveTimer)
  const body = doc.value
  if (!body?.editable || !props.live || draft.value === body.content) return
  void writeCatFile(props.convId, body.relPath, draft.value)
})

function onClose() {
  if (!requestClose()) return
  emit('close')
}

defineExpose({ requestClose })
</script>

<style scoped>
.pv {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--ff-bg-surface);
}
.hd {
  height: 44px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border-bottom: 1px solid var(--ff-border);
}
.nm {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--ff-text-1);
  user-select: text;
  -webkit-user-select: text;
}
.dot {
  width: 6px;
  height: 6px;
  flex: none;
  border-radius: 50%;
  background: var(--ff-warning);
}
.sp {
  flex: 1;
}
.seg {
  display: flex;
  padding: 2px;
  border-radius: 6px;
  background: var(--ff-bg-hover);
}
.seg button,
.icon-btn,
.x {
  border: none;
  background: transparent;
  color: var(--ff-text-2);
  font: inherit;
  font-size: 12px;
  border-radius: 5px;
  padding: 3px 8px;
  cursor: pointer;
}
.icon-btn {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
}
.seg button.on,
.icon-btn.on {
  color: var(--ff-text-1);
  background: var(--ff-bg-hover);
}
.state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 16px;
  text-align: center;
  color: var(--ff-text-3);
  font-size: 13px;
  user-select: text;
  -webkit-user-select: text;
}
.md,
.svg {
  flex: 1;
  min-height: 0;
  width: 100%;
  border: none;
  background: var(--ff-bg-app);
  user-select: text;
  -webkit-user-select: text;
}
.svg {
  object-fit: contain;
}
</style>
