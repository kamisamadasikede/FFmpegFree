<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { startTeamPreview, releaseTeamPreview } from '@/api/team'
import type { PptPreview } from '@/api/catPptx'
import { teamError } from './teamState'
const props = defineProps<{ roomId: string; path: string }>()
const { locale } = useI18n()
const preview = ref<PptPreview>(), frame = ref<HTMLIFrameElement>(), error = ref('')
let serial = 0, id = '', alive = true
function release(value: string) { if (value) void releaseTeamPreview(value).catch(() => undefined) }
async function start() {
 const ticket = ++serial; release(id); id = ''; error.value = ''; preview.value = undefined
 try { const result = await startTeamPreview(props.roomId, props.path); if (!alive || ticket !== serial) { release(result.id); return }; id = result.id; preview.value = result }
 catch (e) { if (alive && ticket === serial) error.value = teamError(e) }
}
function connect() { const p = preview.value; if (p?.url && p.token) frame.value?.contentWindow?.postMessage({ type: 'ffc-office-connect', id: p.documentId, token: p.token, locale: locale.value }, new URL(p.url).origin) }
function message(e: MessageEvent) { if (!preview.value?.url || e.source !== frame.value?.contentWindow || e.origin !== new URL(preview.value.url).origin) return; if (e.data?.type === 'ffc-office-ready') connect(); if (e.data?.type === 'ffc-office-error') error.value = String(e.data.message || '预览加载失败') }
window.addEventListener('message', message)
watch(() => [props.roomId, props.path], () => void start(), { immediate: true })
onBeforeUnmount(() => { alive = false; serial++; release(id); window.removeEventListener('message', message) })
</script>
<template>
 <section class="team-office">
  <div v-if="error" class="preview-error" role="alert">{{ error }}<button @click="start">重新加载</button></div>
  <iframe v-if="preview?.url" ref="frame" :src="preview.url" :title="path" sandbox="allow-scripts allow-same-origin" referrerpolicy="no-referrer" @load="connect" />
  <div v-else-if="!error" class="preview-wait">正在打开文档…</div>
 </section>
</template>
<style scoped>
.team-office{display:flex;flex:1;min-height:0;flex-direction:column;background:var(--cat-bg)}iframe{flex:1;border:0;width:100%;min-height:0}.preview-error{padding:14px;font-size:12px;color:var(--cat-danger-text);line-height:1.8;overflow-wrap:anywhere}.preview-error button{margin-left:10px}.preview-wait{margin:auto;color:var(--cat-muted)}
</style>
