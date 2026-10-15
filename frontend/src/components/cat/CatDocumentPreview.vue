<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import PvPdf from '@/components/docs/preview/PvPdf.vue'
import PvRawDocx from '@/components/docs/preview/PvRawDocx.vue'
import PvRawXlsx from '@/components/docs/preview/PvRawXlsx.vue'
import PvPptxHtml from '@/components/docs/preview/PvPptxHtml.vue'
import { previewDocument, releaseDocument, statFile, type DocumentPreview } from '@/api/catWorkflow'
import { onTaskEvent } from '@/services/wails'
import { toAppError } from '@/api/call'
import { ElDialog } from 'element-plus'
const DocComponentPanel = defineAsyncComponent(() => import('@/components/settings/DocComponentPanel.vue'))
const engineDialog = ref(false)
const props = defineProps<{ convId: string; relPath: string }>()
const { t } = useI18n()
const current = ref<DocumentPreview | null>(null), pending = ref<DocumentPreview | null>(null)
const error = ref(''), writing = ref(false), busy = ref(false), scale = ref(.85), page = ref(1), pages = ref(0)
const pdfViewer = ref<InstanceType<typeof PvPdf>>()
const pptxViewer = ref<InstanceType<typeof PvPptxHtml>>()
const pptx = computed(() => current.value?.kind === 'pptx-html')
const paged = computed(() => pdf.value || pptx.value)
const pdf = computed(() => current.value?.kind === 'pdf' && !!current.value.url)
let alive = true, ticket = 0, revision = '', candidate = '', seen = 0, polling = false
const events = new Map<string, { previewId: string; state: string; kind: string; url?: string; error?: { message: string } }>()
function release(id?: string) { if (id) void releaseDocument(id).catch(() => undefined) }
function accept(p: DocumentPreview) {
  if (p.state === 'generating') { pending.value = p; busy.value = true; return }
  if (p.state === 'failed') { error.value = p.error?.message || t('cat.preview.failed'); release(p.previewId); pending.value = null; busy.value = false; return }
  const old = current.value
  current.value = p; pending.value = null; busy.value = false; error.value = ''
  if (old?.previewId !== p.previewId) release(old?.previewId)
}
const off = onTaskEvent<{ previewId: string; state: string; kind: string; url?: string; error?: { message: string } }>('doc:preview', e => {
  events.set(e.previewId, e); if (events.size > 16) events.delete(events.keys().next().value!)
  if (pending.value?.previewId === e.previewId) accept({ ...pending.value, ...e })
})
async function refresh(force = false) {
  if (!alive || busy.value && !force) return
  const my = ++ticket, convId = props.convId, path = props.relPath
  if (force) { release(pending.value?.previewId); pending.value = null }
  busy.value = true; error.value = ''
  try {
    const rev = await statFile(convId, path)
    if (!alive || ticket !== my) return
    if (!rev.stable) { writing.value = true; busy.value = false; return }
    const p = await previewDocument(convId, path)
    if (!alive || ticket !== my || props.convId !== convId || props.relPath !== path) { release(p.previewId); return }
    revision = rev.revision; writing.value = false
    const early = events.get(p.previewId)
    accept(early ? { ...p, ...early } : p)
  } catch(e) { if (alive && ticket === my) { busy.value = false; error.value = toAppError(e).message } }
}
async function poll() {
  if (!alive || polling || document.hidden) return
  polling = true
  const my = ticket, convId = props.convId, path = props.relPath
  try {
    const r = await statFile(convId, path)
    if (!alive || ticket !== my || props.convId !== convId || props.relPath !== path) return
    writing.value = !r.stable
    if (r.revision === candidate) seen++; else { candidate = r.revision; seen = 1 }
    if (!r.stable && seen >= 6) error.value = t('cat.preview.invalidOffice')
    if (r.stable && seen >= 2 && r.revision !== revision && !busy.value) await refresh()
  } catch(e) { if (alive && ticket === my) error.value = toAppError(e).message }
  finally { polling = false }
}
const timer = setInterval(() => void poll(), 1000)
watch(() => [props.convId, props.relPath], () => {
  ticket++; release(current.value?.previewId); release(pending.value?.previewId)
  current.value = null; pending.value = null; busy.value = false; revision = ''; candidate = ''; seen = 0; page.value = 1; pages.value = 0
  void refresh()
}, { immediate: true })
function go(n: number) { page.value = Math.max(1, Math.min(pages.value, n)); pdfViewer.value?.scrollToPage(page.value); pptxViewer.value?.scrollToPage(page.value) }
function fit() { pdfViewer.value?.fitWidth(); pptxViewer.value?.fitWidth() }
onBeforeUnmount(() => { alive = false; ticket++; clearInterval(timer); off(); release(current.value?.previewId); release(pending.value?.previewId) })
</script>
<template>
  <div class="document-preview" data-testid="cat-document-preview">
    <div class="tools"><span class="format">{{ current?.ext?.toUpperCase() || relPath.split('.').pop()?.toUpperCase() }}</span><span v-if="paged" class="pagination"><button type="button" :disabled="page <= 1" :aria-label="t('cat.preview.previous')" @click="go(page - 1)"><FIcon name="left" :size="13" /></button><input :value="page" type="number" min="1" :max="pages" :aria-label="t('cat.preview.page')" @change="go(Number(($event.target as HTMLInputElement).value) || 1)" /><span>/ {{ pages || '—' }}</span><button type="button" :disabled="page >= pages" :aria-label="t('cat.preview.next')" @click="go(page + 1)"><FIcon name="right" :size="13" /></button></span><span class="sp" /><button type="button" :title="t('cat.preview.engineSettings')" :aria-label="t('cat.preview.engineSettings')" @click="engineDialog = true"><FIcon name="set" :size="13" /></button><button type="button" :aria-label="t('cat.preview.zoomOut')" @click="scale = Math.max(.25, scale - .1)">−</button><span class="zoom">{{ Math.round(scale * 100) }}%</span><button type="button" :aria-label="t('cat.preview.zoomIn')" @click="scale = Math.min(2.5, scale + .1)">+</button><button v-if="paged" type="button" @click="fit">{{ t('cat.preview.fit') }}</button><button type="button" :title="t('cat.preview.refresh')" :aria-label="t('cat.preview.refresh')" @click="refresh(true)"><FIcon name="retry" :size="13" /></button></div>
    <div class="status" role="status"><span :class="{ pulse: busy || writing }" />{{ t(writing ? 'cat.preview.writing' : busy ? 'cat.preview.generating' : 'cat.preview.live') }}</div>
    <div v-if="error" class="error" role="status">{{ error }}<button type="button" @click="refresh(true)">{{ t('cat.preview.retry') }}</button></div>
    <PvPdf v-if="pdf" ref="pdfViewer" :url="current!.url!" :scale="scale" :initial-page="page" @pages="pages = $event" @page="page = $event" @scale="scale = $event" @failed="error = t('cat.preview.failed')" @gone="error = t('cat.preview.gone')" />
    <PvPptxHtml v-else-if="pptx" ref="pptxViewer" :html="current!.text || ''" :scale="scale" @pages="pages = $event" @page="page = $event" @scale="scale = $event" /><template v-else-if="current?.kind === 'raw'"><p class="basic">{{ t(pptx ? 'cat.preview.pptVisual' : 'cat.preview.basic') }}</p><PvRawDocx v-if="current.ext === 'docx'" :key="current.previewId" :url="current.url!" :size-bytes="current.sizeBytes" :scale="scale" @failed="error = t('cat.preview.failed')" /><PvRawXlsx v-else :key="current.previewId" :url="current.url!" :size-bytes="current.sizeBytes" :scale="scale" @failed="error = t('cat.preview.failed')" /></template>
    <template v-else-if="current?.kind === 'unavailable'"><div class="unavailable"><FIcon name="file" :size="28" /><p>{{ t('cat.preview.needsEngine') }}</p><button type="button" @click="engineDialog = true">{{ t('cat.preview.settings') }}</button></div></template>
    <ElDialog v-model="engineDialog" :title="t('cat.preview.engineSettings')" width="min(640px, 92vw)" append-to-body @closed="refresh(true)"><DocComponentPanel /></ElDialog>
    <div v-if="!current && !error" class="loading">{{ t(writing ? 'cat.preview.writing' : 'cat.preview.generating') }}</div>
  </div>
</template>
<style scoped>
.document-preview{display:flex;flex-direction:column;flex:1;min-width:0;min-height:0;background:var(--cat-bg)}.tools{display:flex;align-items:center;gap:5px;flex-wrap:wrap;padding:8px 10px;border-bottom:1px solid var(--cat-border);font-size:11px;color:var(--cat-text-2)}.tools button,.error button,.unavailable button{border:1px solid var(--cat-border);background:var(--cat-surface);color:var(--cat-text-2);border-radius:6px;padding:4px 6px;font:inherit;cursor:pointer}.tools button:disabled{opacity:.4;cursor:default}.tools button:hover{background:var(--cat-hover)}.sp{flex:1}.format{font-weight:600;letter-spacing:.05em}.pagination{display:flex;align-items:center;gap:4px}.pagination input{width:38px;border:1px solid var(--cat-border);background:var(--cat-surface);color:var(--cat-text);border-radius:5px;padding:4px;text-align:center;font:inherit;appearance:textfield}.pagination input::-webkit-inner-spin-button{appearance:none}.zoom{min-width:36px;text-align:center}.status{display:flex;gap:6px;align-items:center;padding:7px 12px;font-size:11px;color:var(--cat-muted);border-bottom:1px solid var(--cat-border)}.status>span{height:5px;width:5px;border-radius:50%;background:#3c9b79}.status .pulse{background:#c99a3c}.error{padding:10px;font-size:12px;display:flex;align-items:center;gap:10px;color:var(--cat-text-2)}.loading{margin:auto;padding:32px;color:var(--cat-muted);font-size:12px}.basic{font-size:11px;color:var(--cat-muted);margin:0;padding:8px 12px}.unavailable{text-align:center;padding:20px;color:var(--cat-muted);font-size:12px}.document-preview :deep(.pvx-scroll){flex:1;min-height:0;overflow:auto;padding:16px}.document-preview :deep(.pvx-pgs){display:flex;flex-direction:column;gap:16px;align-items:center;min-width:fit-content}.document-preview :deep(.pvx-pdfpg){background:white;box-shadow:0 2px 12px #00000015;flex:none}.document-preview :deep(.pvx-tbl){border-collapse:collapse;font-size:12px}.document-preview :deep(.pvx-tbl td),.document-preview :deep(.pvx-tbl th){border:1px solid var(--cat-border);padding:4px 8px;white-space:nowrap}.document-preview :deep(.pvx-docx){display:flex;flex-direction:column;align-items:center}
.document-preview :deep(.pvx-tbl th),.document-preview :deep(.pvx-tbl .n){background:var(--cat-surface);color:var(--cat-muted);font-weight:400;position:sticky;z-index:1}.document-preview :deep(.pvx-tbl th){top:0}.document-preview :deep(.pvx-tbl .n){left:0}.document-preview :deep(.pvx-tbl td.r){text-align:right}.document-preview :deep(.pvx-tabs){display:flex;gap:4px;padding:6px 10px;border-top:1px solid var(--cat-border);overflow:auto;flex:none}.document-preview :deep(.pvx-tabs button){border:0;background:transparent;font:inherit;font-size:12px;color:var(--cat-muted);padding:6px 10px;cursor:pointer;border-radius:6px;white-space:nowrap}.document-preview :deep(.pvx-tabs button.on){background:var(--cat-surface);color:var(--cat-text);box-shadow:0 0 0 1px var(--cat-border)}.document-preview :deep(.pvx-docx){width:fit-content;min-width:100%}.document-preview :deep(section.docx){color:#20252b;background:white;box-shadow:0 2px 12px #0001;margin-bottom:12px}
</style>
