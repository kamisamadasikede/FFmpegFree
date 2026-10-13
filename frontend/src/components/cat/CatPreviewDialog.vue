<template>
  <el-dialog
    :model-value="open"
    :title="title"
    width="min(880px, 94vw)"
    align-center
    append-to-body
    destroy-on-close
    @update:model-value="emit('update:open', $event)"
  >
    <div class="stage" :class="view || 'wait'" data-testid="cat-attach-preview">
      <p v-if="note" class="note">{{ note }}</p>
      <img v-else-if="view === 'image' || view === 'svg'" :src="src" alt="" />
      <video v-else-if="view === 'video'" :src="src" controls autoplay playsinline />
      <audio v-else-if="view === 'audio'" :src="src" controls />
      <iframe v-else-if="view === 'pdf'" class="frame" :src="src" :title="file?.name || '预览'" />
      <iframe v-else-if="view === 'html'" class="frame" sandbox="allow-scripts" referrerpolicy="no-referrer" :title="file?.name || '预览'" :srcdoc="text" />
      <iframe v-else-if="view === 'markdown'" class="frame" sandbox="" :title="file?.name || '预览'" :srcdoc="text" />
      <CatFileRich v-else-if="rich" :kind="rich" :mime="mime" :b64="b64" :name="file?.name || ''" />
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import CatFileRich from './CatFileRich.vue'
import { buildSrcdoc, renderMarkdown } from '@/utils/docSafeHtml'
import { canPreview, catMediaBase64, catMediaText, catMediaURL, imageDataUrl, showsThumb, type CatPreviewKind, type CatShownFile } from './catMedia'

const props = defineProps<{ open: boolean; file: CatShownFile | null }>()
const emit = defineEmits<{ 'update:open': [boolean] }>()

const title = computed(() => {
  const file = props.file
  if (!file || showsThumb(file.kind)) return ''
  return file.name || '预览'
})

const note = ref('')
const view = ref<CatPreviewKind | ''>('')
const src = ref('')
const text = ref('')
const b64 = ref('')
const mime = ref('')
const rich = ref('')
let token = 0

const FAIL = '这个文件没能显示出来。'
const LARGE = '这个文件太大，这里放不下。'

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

async function load() {
  const my = ++token
  reset()
  if (!props.open || !props.file || !canPreview(props.file.kind)) return
  const file = props.file
  note.value = '正在读取…'
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
  if (file.kind === 'markdown' || file.kind === 'html') {
    const body = await catMediaText(file.path)
    if (my !== token) return
    if (body === 'large') {
      note.value = LARGE
      return
    }
    if (!body) {
      note.value = FAIL
      return
    }
    if (file.kind === 'html') {
      note.value = ''
      view.value = 'html'
      text.value = body
      return
    }
    const html = await renderMarkdown(body)
    if (my !== token) return
    const dark = document.documentElement.classList.contains('dark')
    note.value = ''
    view.value = 'markdown'
    text.value = buildSrcdoc(html, { dark, title: file.name })
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
.stage {
  min-height: 180px;
  max-height: min(72vh, 720px);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: auto;
  border-radius: 12px;
  background: var(--ff-bg);
}
.stage.image,
.stage.video,
.stage.svg {
  background: #111;
}
.stage img,
.stage video {
  max-width: 100%;
  max-height: min(72vh, 720px);
  object-fit: contain;
}
.stage audio {
  width: min(440px, 100%);
}
.frame {
  width: 100%;
  height: min(72vh, 720px);
  border: 0;
  background: #fff;
}
.stage.docx,
.stage.xlsx,
.stage.pptx {
  display: block;
  height: min(72vh, 720px);
}
.stage :deep(.rich) {
  min-height: 240px;
}
.note {
  margin: 0;
  padding: 28px 16px;
  color: var(--ff-text-3);
  font-size: 13px;
}
</style>
