<template>
  <teleport to="body">
    <div v-if="open" class="mask" role="dialog" aria-modal="true" :aria-label="t('cat.misc.s_199cf9f0e4')" @mousedown.self="close">
      <div class="frame" :class="{ hug }" @click.stop>
        <button ref="xBtn" type="button" class="x" :aria-label="t('cat.misc.s_b15d91274e')" :title="t('cat.misc.s_b15d91274e')" @click="close"><FIcon name="x" :size="16" /></button>
        <div class="sheet" :class="view || 'wait'" data-testid="cat-attach-preview">
          <p v-if="note" class="note">{{ note }}</p>
          <img v-else-if="view === 'image' || view === 'svg'" :src="src" alt="" @error="onImgError" />
          <video v-else-if="view === 'video'" :src="src" controls autoplay playsinline />
          <audio v-else-if="view === 'audio'" :src="src" controls />
          <iframe v-else-if="view === 'pdf'" class="doc" :src="src" :title="t('cat.misc.s_199cf9f0e4')" />
          <iframe v-else-if="view === 'html'" class="doc" sandbox="allow-scripts" referrerpolicy="no-referrer" :title="t('cat.misc.s_199cf9f0e4')" :srcdoc="text" />
          <iframe v-else-if="view === 'markdown' || view === 'text' || view === 'csv'" class="doc" sandbox="" :title="t('cat.misc.s_199cf9f0e4')" :srcdoc="text" />
          <CatFileRich v-else-if="rich" :kind="rich" :mime="mime" :b64="b64" :name="file?.name || ''" />
        </div>
      </div>
    </div>
  </teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import CatFileRich from './CatFileRich.vue'
import { buildSrcdoc, renderMarkdown } from '@/utils/docSafeHtml'
import { canPreview, catMediaBase64, catMediaObjectURL, catMediaText, catMediaURL, imageDataUrl, plainDocumentHtml, tableDocumentHtml, type CatPreviewKind, type CatShownFile } from './catMedia'

const props = defineProps<{ open: boolean; file: CatShownFile | null }>()
const emit = defineEmits<{ 'update:open': [boolean] }>()

const xBtn = ref<HTMLButtonElement>()

function close() {
  emit('update:open', false)
}

function onImgError() {
  src.value = ''
  view.value = ''
  note.value = FAIL
}

function onKey(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  e.preventDefault()
  e.stopPropagation()
  close()
}

const note = ref('')
const view = ref<CatPreviewKind | ''>('')
const hug = computed(() => view.value === 'image' || view.value === 'video' || view.value === 'svg')
const src = ref('')
const text = ref('')
const b64 = ref('')
const mime = ref('')
const rich = ref('')
let token = 0

const FAIL = t('cat.misc.s_f8f6e371cb')
const LARGE = t('cat.misc.s_bf2e7c2378')

function reset() {
  note.value = ''
  view.value = ''
  src.value = ''
  text.value = ''
  b64.value = ''
  mime.value = ''
  rich.value = ''
}

watch(
  () => [props.open, props.file?.path, props.file?.data, props.file?.kind] as const,
  () => {
    void load()
  },
)

watch(
  () => props.open,
  (on) => {
    if (on) {
      window.addEventListener('keydown', onKey)
      void nextTick(() => xBtn.value?.focus())
    } else {
      window.removeEventListener('keydown', onKey)
    }
  },
)

onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

async function load() {
  const my = ++token
  reset()
  if (!props.open || !props.file || !canPreview(props.file.kind)) return
  const file = props.file
  note.value = t('cat.misc.s_62a33abd9b')
  const inline = file.kind === 'image' ? imageDataUrl(file) : ''
  if (inline) {
    if (my !== token) return
    note.value = ''
    view.value = 'image'
    src.value = inline
    return
  }
  if (!file.path) {
    note.value = FAIL
    return
  }
  if (file.kind === 'image' || file.kind === 'svg') {
    const obj = await catMediaObjectURL(file.path)
    if (my !== token) return
    if (obj) {
      note.value = ''
      view.value = file.kind
      src.value = obj
      return
    }
  }
  if (file.kind === 'image' || file.kind === 'video' || file.kind === 'audio' || file.kind === 'pdf' || file.kind === 'svg') {
    const hit = await catMediaURL(file.path)
    if (my !== token) return
    if (!hit) {
      note.value = FAIL
      return
    }
    note.value = ''
    view.value = file.kind
    src.value = hit.url
    return
  }
  if (file.kind === 'markdown' || file.kind === 'html' || file.kind === 'text' || file.kind === 'csv') {
    const body = await catMediaText(file.path)
    if (my !== token) return
    if (body === 'large') {
      note.value = LARGE
      return
    }
    if (body == null) {
      note.value = FAIL
      return
    }
    try {
      if (file.kind === 'html') {
        note.value = ''
        view.value = 'html'
        text.value = body
        return
      }
      const dark = document.documentElement.classList.contains('dark')
      if (file.kind === 'text') {
        note.value = ''
        view.value = 'text'
        text.value = plainDocumentHtml(file.name, body, dark)
        return
      }
      if (file.kind === 'csv') {
        const sep = file.name.toLowerCase().endsWith('.tsv') ? '\t' : ','
        note.value = ''
        view.value = 'csv'
        text.value = tableDocumentHtml(file.name, body, sep, dark)
        return
      }
      const html = await renderMarkdown(body)
      if (my !== token) return
      note.value = ''
      view.value = 'markdown'
      text.value = buildSrcdoc(html, { dark, title: file.name })
    } catch {
      if (my === token) note.value = FAIL
    }
    return
  }
  const packed = await catMediaBase64(file.path)
  if (my !== token) return
  if (packed === 'large') {
    note.value = LARGE
    return
  }
  if (!packed) {
    note.value = FAIL
    return
  }
  note.value = ''
  view.value = file.kind
  rich.value = file.kind
  b64.value = packed.b64
  mime.value = packed.mime || file.mime || ''
}
</script>

<style scoped>
.mask {
  position: fixed;
  inset: 0;
  z-index: 5000;
  display: flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  padding: 24px 72px 24px 24px;
  background: var(--cat-scrim);
}
.frame {
  position: relative;
  max-width: 100%;
  max-height: 100%;
}
.frame:not(.hug) {
  width: min(880px, 100%);
  height: min(72vh, 680px);
}
.x {
  position: absolute;
  top: 0;
  right: -44px;
  z-index: 1;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 50%;
  padding: 0;
  display: grid;
  place-items: center;
  background: #fff;
  color: #1f2328;
  cursor: pointer;
}
.x:hover {
  background: #f3f4f6;
}
.x:focus-visible {
  outline: 2px solid #fff;
  outline-offset: 2px;
}
.sheet {
  overflow: hidden;
  border-radius: 12px;
  background: var(--ff-bg-surface, #fff);
}
.frame:not(.hug) .sheet {
  display: flex;
  width: 100%;
  height: 100%;
}
.sheet.image,
.sheet.video,
.sheet.svg {
  background: var(--cat-media-bg);
  line-height: 0;
}
.sheet img,
.sheet video {
  display: block;
  max-width: min(960px, calc(100vw - 96px));
  max-height: calc(100vh - 48px);
  width: auto;
  height: auto;
  margin: 0;
  padding: 0;
  border: 0;
}
.sheet audio {
  width: min(440px, 100%);
  margin: 36px auto;
}
.doc {
  flex: 1;
  width: 100%;
  height: 100%;
  border: 0;
  background: var(--cat-paper);
}
.sheet :deep(.rich) {
  flex: 1;
  width: 100%;
  height: 100%;
  min-height: 0;
}
.note {
  margin: 0;
  padding: 28px 16px;
  color: var(--cat-muted);
  font-size: 13px;
}
</style>
