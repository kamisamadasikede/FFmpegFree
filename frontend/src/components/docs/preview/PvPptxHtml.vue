<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import runtime from './pptxFrameRuntime.js?raw'
const props = defineProps<{ html: string; scale: number; slide?: number; scrollTo?: string; follow?: boolean }>()
const emit = defineEmits<{ (e: 'pages' | 'page' | 'scale', n: number): void; (e: 'ready' | 'failed'): void }>()
const frame = ref<HTMLIFrameElement>(), source = ref('')
const channel = crypto.randomUUID(), nonce = crypto.randomUUID().replaceAll('-', '')
let booted = false, reportedScale = 0, observer: ResizeObserver | undefined
function post(type: string, data: Record<string, unknown> = {}) { if (booted) frame.value?.contentWindow?.postMessage({ source: 'ffc-ppt-host', channel, type, ...data }, '*') }
function send() { post('html', { html: props.html, slide: props.slide, scrollTo: props.scrollTo }) }
function receive(e: MessageEvent) {
  if (e.source !== frame.value?.contentWindow || e.data?.source !== 'ffc-ppt-view' || e.data?.channel !== channel) return
  const m = e.data
  if (m.type === 'boot') { booted = true; post('follow', { follow: props.follow !== false }); send() }
  if (m.type === 'ready') { emit('pages', Number(m.pages) || 0); emit('ready') }
  if (m.type === 'scale') reportedScale = Number(m.scale) || 1
  if (m.type === 'page' || m.type === 'scale') emit(m.type, Number(m[m.type]) || 1)
}
function fitWidth() { post('fit') }
function scrollToPage(page: number) { post('page', { page }) }
defineExpose({ fitWidth, scrollToPage })
watch(() => [props.html, props.slide, props.scrollTo], send)
watch(() => props.scale, n => { if (Math.abs(n - reportedScale) > .0001) post('scale', { scale: n }) })
watch(() => props.follow, n => post('follow', { follow: n !== false }))
onMounted(() => {
  window.addEventListener('message', receive)
  observer = new ResizeObserver(() => post('layout'))
  if (frame.value) observer.observe(frame.value)
  source.value = `<!doctype html><html data-channel="${channel}"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'nonce-${nonce}'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; base-uri 'none'; form-action 'none'"><style id="viewport-style">html,body{margin:0!important;width:100%!important;height:100%!important;overflow:hidden!important;background:#eef0f3!important}.sidebar,.file-title,.page-counter,.slide-label,.slide-notes{display:none!important}.main{width:100%!important;height:100%!important;overflow:auto!important;padding:16px!important;gap:16px!important;box-sizing:border-box!important;align-items:center!important;scroll-behavior:auto!important}.slide-container{flex:none!important;gap:0!important}.slide-wrapper{display:block!important;position:relative!important;flex:none!important}.slide{position:absolute!important;left:0!important;top:0!important;border-radius:0!important}.slide-container{scroll-margin:16px}</style></head><body></body><script nonce="${nonce}">${runtime}</scr` + 'ipt></html>'
})
onBeforeUnmount(() => { observer?.disconnect(); window.removeEventListener('message', receive) })
</script>
<template><iframe ref="frame" class="ppt-cli-frame" title="PowerPoint" sandbox="allow-scripts" referrerpolicy="no-referrer" :srcdoc="source" /></template>
<style scoped>.ppt-cli-frame{display:block;flex:1;min-height:0;width:100%;height:100%;border:0}</style>
