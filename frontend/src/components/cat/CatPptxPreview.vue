<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import CatDocumentPreview from './CatDocumentPreview.vue'
import { startPptPreview, releasePptPreview, retryPptTool, type PptPreview } from '@/api/catPptx'
import { toAppError } from '@/api/call'
const props = defineProps<{ convId: string; relPath: string }>()
const { t, locale } = useI18n()
const snapshot = ref<PptPreview>(), error = ref(''), fallback = ref(false), frame = ref<HTMLIFrameElement>()
let alive = true, serial = 0, id = ''
function release(value: string) { if (value) void releasePptPreview(value).catch(() => undefined) }
async function start() {
  const own = ++serial; release(id); id = ''; snapshot.value = undefined; error.value = ''
  try {
    const p = await startPptPreview(props.convId, props.relPath)
    if (!alive || own !== serial || fallback.value) { release(p.id); return }
    id = p.id; snapshot.value = p
  } catch(e) { if (alive && own === serial) error.value = toAppError(e).message }
}
function connect() {
  const p = snapshot.value
  if (!p?.url || !p.token || !frame.value?.contentWindow) return
  frame.value.contentWindow.postMessage({ type: 'ffc-office-connect', id: p.documentId, token: p.token, locale: locale.value }, new URL(p.url).origin)
}
function message(event: MessageEvent) {
  if (!snapshot.value?.url || event.source !== frame.value?.contentWindow || event.origin !== new URL(snapshot.value.url).origin) return
  if (event.data?.type === 'ffc-office-ready') connect()
  if (event.data?.type === 'ffc-office-error' && typeof event.data.message === 'string') error.value = event.data.message
}
window.addEventListener('message', message)
async function retry() { fallback.value = false; error.value = ''; await retryPptTool().catch(() => undefined); if (alive) await start() }
function staticPreview() { ++serial; release(id); id = ''; fallback.value = true }
watch(() => [props.convId, props.relPath], () => { fallback.value = false; void start() }, { immediate: true })
onBeforeUnmount(() => { alive = false; ++serial; release(id); window.removeEventListener('message', message) })
</script>
<template>
  <section class="ppt-preview" data-testid="cat-ppt-preview">
    <div v-if="fallback" class="ppt-hostbar"><button @click="retry">{{ t('cat.preview.retry') }}</button></div>
    <div v-if="error && !fallback" class="ppt-error" role="status">{{ error }}<button @click="retry">{{ t('cat.preview.retry') }}</button><button @click="staticPreview">{{ t('cat.ppt.static') }}</button></div>
    <CatDocumentPreview v-if="fallback" :conv-id="convId" :rel-path="relPath" />
    <iframe v-else-if="snapshot?.url" ref="frame" class="ppt-frame" :src="snapshot.url" title="PowerPoint · GenOffice" sandbox="allow-scripts allow-same-origin" referrerpolicy="no-referrer" @load="connect" />
    <div v-else class="ppt-wait">{{ t('cat.ppt.preparing') }}</div>
  </section>
</template>
<style scoped>
.ppt-preview{display:flex;flex:1;flex-direction:column;min-width:0;min-height:0;background:var(--cat-bg)}.ppt-hostbar{display:flex;gap:6px;align-items:center;padding:5px 10px;font-size:11px;color:var(--cat-muted);border-bottom:1px solid var(--cat-border)}.ppt-preview button{font:inherit;color:var(--cat-text-2);border:1px solid var(--cat-border);background:var(--cat-surface);border-radius:6px;cursor:pointer;padding:4px 7px}.sp{flex:1}.ppt-frame{display:block;width:100%;flex:1;min-height:0;border:0;background:var(--cat-bg)}.ppt-error{padding:10px;font-size:12px;color:var(--cat-text-2);overflow-wrap:anywhere}.ppt-error button{margin-left:8px}.ppt-wait{padding:28px;margin:auto;color:var(--cat-muted);font-size:12px;text-align:center}
</style>
