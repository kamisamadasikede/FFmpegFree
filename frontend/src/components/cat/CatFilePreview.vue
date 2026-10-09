<template>
  <section class="pv" data-testid="cat-file-preview" :aria-label="title">
    <header class="hd">
      <span class="nm" :title="title">{{ title }}</span>
      <span v-if="dirty" class="dot" :title="COPY.unsaved" />
      <span class="sp" />
      <div v-if="toggle" class="seg" role="tablist">
        <button type="button" :class="{ on: mode === 'preview' }" @click="mode = 'preview'">{{ COPY.preview }}</button>
        <button type="button" :class="{ on: mode === 'source' }" @click="mode = 'source'">{{ COPY.source }}</button>
      </div>
      <button v-if="doc?.editable && (showEditor || dirty)" type="button" class="save" data-testid="cat-file-save" :disabled="!dirty || saving" @click="save">
        {{ saving ? COPY.saving : COPY.save }}
      </button>
      <button type="button" class="x" data-testid="cat-file-close" @click="onClose">{{ COPY.close }}</button>
    </header>
    <div v-if="blocked" class="hold">
      <span>{{ COPY.unsaved }}</span>
      <button type="button" @click="save">{{ COPY.save }}</button>
      <button type="button" @click="discard">{{ COPY.discard }}</button>
    </div>
    <div v-if="loading && !ready" class="state">{{ COPY.loading }}</div>
    <div v-else-if="error" class="state">{{ error }}</div>
    <template v-else-if="ready && doc">
      <iframe v-if="showMd && mdHtml" class="md" sandbox="" :title="title" :srcdoc="mdHtml" />
      <div v-else-if="showMd" class="state">{{ COPY.loading }}</div>
      <img v-else-if="showSvg && svgUrl" class="svg" :src="svgUrl" alt="" />
      <CatFileEditor v-else-if="showEditor" :key="editorKey" :text="draft" :language="doc.language || 'plaintext'" :read-only="!doc.editable" @change="draft = $event" @save="save" />
      <CatFileRich v-else-if="rich" :kind="doc.kind" :mime="doc.mime" :b64="doc.dataBase64" :name="doc.name" />
      <div v-else class="state">{{ message }}</div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import CatFileEditor from './CatFileEditor.vue'
import CatFileRich from './CatFileRich.vue'
import { buildSrcdoc, renderMarkdown } from '@/utils/docSafeHtml'
import { notify } from '@/views/cat/catState'
import {
  CAT_PREVIEW_COPY as COPY,
  catPreviewErrorText,
  mockCatFileBody,
  previewModeFor,
  readCatFile,
  writeCatFile,
  type CatFileBody,
} from '@/api/catFilePreview'

const props = defineProps<{ convId: string; relPath: string; name: string; live: boolean }>()
const emit = defineEmits<{ close: [] }>()

const doc = ref<CatFileBody | null>(null)
const draft = ref('')
const mode = ref<'preview' | 'source'>('source')
const loading = ref(false)
const ready = ref(false)
const saving = ref(false)
const error = ref('')
const blocked = ref(false)
const mdHtml = ref('')
const editorKey = ref(0)
let token = 0
let mdToken = 0
let leaveAfterSave = false

const title = computed(() => doc.value?.name || props.name || props.relPath)
const view = computed(() => previewModeFor(doc.value?.name || props.name))
const toggle = computed(() => doc.value?.kind === 'text' && view.value !== 'code')
const showMd = computed(() => !!doc.value && doc.value.kind === 'text' && view.value === 'markdown' && mode.value === 'preview')
const showSvg = computed(() => !!doc.value && doc.value.kind === 'text' && view.value === 'svg' && mode.value === 'preview')
const showEditor = computed(() => !!doc.value && doc.value.kind === 'text' && (!toggle.value || mode.value === 'source'))
const rich = computed(() => !!doc.value && ['image', 'media', 'pdf', 'docx', 'xlsx', 'pptx'].includes(doc.value.kind))
const dirty = computed(() => !!doc.value?.editable && draft.value !== doc.value.content)
const svgUrl = computed(() => (showSvg.value ? 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(draft.value) : ''))
const message = computed(() => {
  if (doc.value?.kind === 'tooLarge') return COPY.tooLarge
  if (doc.value?.kind === 'doc') return COPY.doc
  return COPY.binary
})

watch(
  () => [props.convId, props.relPath] as const,
  () => {
    void load(true)
  },
  { immediate: true },
)

watch([showMd, draft], () => {
  void paintMarkdown()
})

async function load(resetMode: boolean) {
  const my = ++token
  const id = props.convId
  const rel = props.relPath
  loading.value = true
  ready.value = false
  error.value = ''
  blocked.value = false
  leaveAfterSave = false
  try {
    const body = props.live ? await readCatFile(id, rel) : mockCatFileBody(props.name || rel)
    if (my !== token || props.convId !== id || props.relPath !== rel) return
    doc.value = body
    draft.value = body.content
    editorKey.value++
    ready.value = true
    if (resetMode) mode.value = previewModeFor(body.name) === 'code' ? 'source' : 'preview'
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

async function save() {
  const body = doc.value
  if (!body?.editable || saving.value) return
  if (draft.value === body.content) {
    if (leaveAfterSave) finishLeave()
    return
  }
  saving.value = true
  try {
    if (props.live) {
      const wrote = await writeCatFile(props.convId, body.relPath, draft.value)
      if (doc.value !== body) return
      body.size = wrote.size
      body.modTime = wrote.modTime
    }
    body.content = draft.value
    notify(COPY.saved, 'ok')
    if (leaveAfterSave) finishLeave()
  } catch (e) {
    notify(catPreviewErrorText(e, COPY.saveFailed), 'warn')
  } finally {
    saving.value = false
  }
}

function discard() {
  draft.value = doc.value?.content ?? ''
  editorKey.value++
  finishLeave()
}

function finishLeave() {
  blocked.value = false
  leaveAfterSave = false
  emit('close')
}

function requestClose(): boolean {
  if (!dirty.value) return true
  blocked.value = true
  leaveAfterSave = true
  return false
}

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
.save,
.x,
.hold button {
  border: none;
  background: transparent;
  color: var(--ff-text-2);
  font: inherit;
  font-size: 12px;
  border-radius: 5px;
  padding: 3px 8px;
  cursor: pointer;
}
.seg button.on,
.save:not(:disabled),
.hold button {
  color: var(--ff-text-1);
}
.save:not(:disabled),
.hold button {
  background: var(--ff-bg-hover);
}
.save:disabled {
  opacity: 0.45;
  cursor: default;
}
.hold {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--ff-text-2);
  background: color-mix(in srgb, var(--ff-warning) 8%, transparent);
  border-bottom: 1px solid var(--ff-border);
}
.hold span {
  flex: 1;
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
}
.md,
.svg {
  flex: 1;
  min-height: 0;
  width: 100%;
  border: none;
  background: var(--ff-bg-app);
}
.svg {
  object-fit: contain;
}
</style>
