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
      <button
        v-if="htmlPage"
        type="button"
        class="icon-btn pick"
        :class="{ on: pickOn }"
        data-testid="cat-html-pick"
        :aria-pressed="pickOn"
        :aria-label="COPY.pickTitle"
        :title="COPY.pickTitle"
        @click="togglePick"
      >
        <FIcon name="pick" :size="15" />
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
      <div v-else-if="showHtml" class="html-wrap">
        <iframe ref="htmlFrame" class="md" :class="{ picking: pickOn }" sandbox="allow-scripts" referrerpolicy="no-referrer" :title="title" :srcdoc="htmlSrc" @load="onHtmlLoad" />
        <p v-if="pickOn" class="pick-bar" data-testid="cat-pick-bar">{{ pickSlow && !pickReady ? COPY.pickBlocked : COPY.pickHint }}</p>
      </div>
      <CatFileEditor v-else-if="showEditor" :key="editorKey" :text="draft" :language="doc.language || 'plaintext'" :name="doc.name" :read-only="!doc.editable" :marks="editorMarks" @change="draft = $event" @save="flushSave" />
      <CatFileRich v-else-if="rich" :kind="doc.kind" :mime="doc.mime" :b64="doc.dataBase64" :name="doc.name" />
      <div v-else class="state">{{ message }}</div>
    </template>
    <CatPickPop
      v-if="pop"
      :key="pop.key"
      :title="pop.title"
      :selector="pop.selector"
      :anchor="pop.anchor"
      @add="onPopAdd"
      @close="pop = null"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatFileEditor from './CatFileEditor.vue'
import CatFileRich from './CatFileRich.vue'
import CatPickPop from './CatPickPop.vue'
import { CAT_GIT_COPY as GIT, catGitError, gitCatConflictResolve, gitCatDiff } from '@/api/catGit'
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
import { newPickToken, pickLabel, PICK_SOURCE, readPickEvent, withPickScript, type PickBox, type PickNote, type PickSelect } from './catPick'

const props = defineProps<{ convId: string; relPath: string; name: string; live: boolean; compare?: boolean; conflict?: boolean }>()
const emit = defineEmits<{ close: []; pick: [note: PickNote]; 'conflict-saved': [] }>()

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
const htmlFrame = ref<HTMLIFrameElement | null>(null)
const pickOn = ref(false)
const pickReady = ref(false)
const pickSlow = ref(false)
const pickToken = ref(newPickToken())
const relBox = ref<PickBox | null>(null)
const pop = ref<{ key: string; title: string; selector: string; anchor: PickBox; element: PickSelect; file: string } | null>(null)
let pickTimer: ReturnType<typeof setTimeout> | undefined
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
/** html 页用独立来源的 iframe 渲染：脚本能跑，但读不到外面的文档。点选脚本只在预览里，不写回文件。 */
const showHtml = computed(() => htmlPage.value && mode.value === 'preview')
const htmlSrc = computed(() => (showHtml.value ? withPickScript(draft.value, pickToken.value) : ''))
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
  pickOn.value = false
  pickReady.value = false
  pickSlow.value = false
  pop.value = null
  relBox.value = null
  pickToken.value = newPickToken()
  clearTimeout(saveTimer)
  clearTimeout(pickTimer)
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
      if (props.conflict) {
        await gitCatConflictResolve(props.convId, body.relPath, 'content', text)
        if (doc.value !== body) return
        emit('conflict-saved')
      } else {
        const wrote = await writeCatFile(props.convId, body.relPath, text)
        if (doc.value !== body) return
        body.size = wrote.size
        body.modTime = wrote.modTime
      }
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

function armPick(on: boolean) {
  const frame = htmlFrame.value?.contentWindow
  if (!frame) return
  frame.postMessage({ source: PICK_SOURCE, token: pickToken.value, kind: 'arm', on }, '*')
}

function clearPickSelection() {
  const frame = htmlFrame.value?.contentWindow
  if (!frame) return
  frame.postMessage({ source: PICK_SOURCE, token: pickToken.value, kind: 'clear' }, '*')
}

function togglePick() {
  if (mode.value !== 'preview') {
    mode.value = 'preview'
    pickOn.value = true
    return
  }
  pickOn.value = !pickOn.value
}

function anchorOf(box: PickBox): PickBox | null {
  const frame = htmlFrame.value?.getBoundingClientRect()
  if (!frame) return null
  return { left: frame.left + box.left, top: frame.top + box.top, width: box.width, height: box.height }
}

function openPop(el: PickSelect) {
  relBox.value = el.box
  const anchor = anchorOf(el.box)
  if (!anchor) return
  pop.value = {
    key: [el.tag, el.id, el.selector, el.text].join('|'),
    title: pickLabel(el),
    selector: el.selector,
    anchor,
    element: el,
    file: doc.value?.relPath || title.value,
  }
}

function followPop(box: PickBox) {
  if (!pop.value) return
  relBox.value = box
  const anchor = anchorOf(box)
  if (!anchor) return
  pop.value = { ...pop.value, anchor }
}

function onPopAdd(note: string, intent: 'edit' | 'ask') {
  const cur = pop.value
  if (!cur) return
  const el = cur.element
  emit('pick', {
    file: cur.file,
    tag: el.tag,
    id: el.id,
    classes: el.classes,
    selector: el.selector,
    text: el.text,
    styles: el.styles,
    snippet: el.snippet,
    intent,
    note: note.trim().slice(0, 500),
  })
  pop.value = null
  relBox.value = null
}

function onPickMessage(ev: MessageEvent) {
  const parsed = readPickEvent(ev.data, pickToken.value)
  if (!parsed) return
  const frame = htmlFrame.value?.contentWindow
  if (frame && ev.source !== frame) return
  if (!frame && parsed.kind !== 'ready') return
  if (parsed.kind === 'ready') {
    pickReady.value = true
    pickSlow.value = false
    if (pickOn.value) armPick(true)
    return
  }
  if (!pickOn.value) return
  if (parsed.kind === 'esc') {
    if (pop.value) {
      pop.value = null
      return
    }
    pickOn.value = false
    return
  }
  if (parsed.kind === 'move') {
    followPop(parsed.box)
    return
  }
  openPop(parsed)
}

function onOutside(e: PointerEvent) {
  if (!pop.value) return
  const t = e.target
  if (!(t instanceof Element)) return
  if (t.closest('[data-cat-pick-pop]')) return
  if (t === htmlFrame.value) return
  pop.value = null
}

function onHtmlLoad() {
  if (!pickOn.value) return
  armPick(true)
  if (pickReady.value) return
  clearTimeout(pickTimer)
  pickTimer = setTimeout(() => {
    if (pickOn.value && !pickReady.value) pickSlow.value = true
  }, 700)
}

function onPickKey(e: KeyboardEvent) {
  if (!pickOn.value || e.key !== 'Escape') return
  if (pop.value) {
    pop.value = null
    e.preventDefault()
    return
  }
  pickOn.value = false
}

watch(pop, (cur, prev) => {
  if (prev && !cur) clearPickSelection()
})

watch(pickOn, (on) => {
  clearTimeout(pickTimer)
  pickSlow.value = false
  if (!on) {
    pop.value = null
    relBox.value = null
    armPick(false)
    return
  }
  armPick(true)
  if (pickReady.value || !htmlFrame.value) return
  pickTimer = setTimeout(() => {
    if (pickOn.value && !pickReady.value) pickSlow.value = true
  }, 700)
})

function onReflow() {
  if (!pop.value || !relBox.value) return
  const anchor = anchorOf(relBox.value)
  if (anchor) pop.value = { ...pop.value, anchor }
}

watch(showHtml, (on) => {
  if (on) return
  pickOn.value = false
  pickReady.value = false
  pickSlow.value = false
  pop.value = null
})

window.addEventListener('message', onPickMessage)
window.addEventListener('keydown', onPickKey)
window.addEventListener('pointerdown', onOutside)
window.addEventListener('resize', onReflow)
window.addEventListener('scroll', onReflow, true)

function requestClose(): boolean {
  if (!doc.value?.editable || (!dirty.value && !saving.value)) return true
  leaveAfterSave = true
  flushSave()
  return false
}

onBeforeUnmount(() => {
  clearTimeout(saveTimer)
  clearTimeout(pickTimer)
  window.removeEventListener('message', onPickMessage)
  window.removeEventListener('keydown', onPickKey)
  window.removeEventListener('pointerdown', onOutside)
  window.removeEventListener('resize', onReflow)
  window.removeEventListener('scroll', onReflow, true)
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
  background: var(--cat-surface);
}
.hd {
  height: 44px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border-bottom: 1px solid var(--cat-border);
}
.nm {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--cat-text);
  user-select: text;
  -webkit-user-select: text;
}
.dot {
  width: 6px;
  height: 6px;
  flex: none;
  border-radius: 50%;
  background: var(--cat-warn);
}
.sp {
  flex: 1;
}
.seg {
  display: flex;
  padding: 2px;
  border-radius: 6px;
  background: var(--cat-hover);
}
.seg button,
.icon-btn,
.x {
  border: none;
  background: transparent;
  color: var(--cat-text-2);
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
  color: var(--cat-text);
  background: var(--cat-hover);
}
.icon-btn.pick.on {
  color: var(--ff-primary-text);
  background: var(--ff-primary-soft);
}
.pick-bar {
  flex: none;
  margin: 0;
  padding: 6px 10px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--ff-text-2);
  background: var(--ff-bg-hover);
  border-top: 1px solid var(--ff-border);
}
.md.picking {
  cursor: crosshair;
}
.html-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 16px;
  text-align: center;
  color: var(--cat-muted);
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
  background: var(--cat-bg);
  user-select: text;
  -webkit-user-select: text;
}
.svg {
  object-fit: contain;
}
</style>
