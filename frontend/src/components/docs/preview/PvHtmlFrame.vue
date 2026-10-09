<script setup lang="ts">
// md / html 的显示：过滤后的正文放进 sandbox 为空的 iframe（不能跑脚本、不同源、不能导航 / 弹窗 / 提交），srcdoc 开头是严格 CSP（契约 6.12.32.4 / 6.12.44）
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { buildSrcdoc, renderMarkdown, sanitizeHtml, PREVIEW_SANDBOX } from '@/utils/docSafeHtml'
import { useThemeDark } from './useThemeDark'

const props = defineProps<{ source: string; kind: 'md' | 'html'; zoom?: number; title?: string; debounce?: number }>()
const safe = ref('')
const dark = useThemeDark()
let timer = 0
let seq = 0
async function render() {
  const my = ++seq
  const out = props.kind === 'md' ? await renderMarkdown(props.source) : await sanitizeHtml(props.source)
  if (my === seq) safe.value = out
}
watch(
  () => [props.source, props.kind] as const,
  () => {
    clearTimeout(timer)
    if (props.debounce && safe.value) timer = window.setTimeout(render, props.debounce)
    else void render()
  },
  { immediate: true },
)
onBeforeUnmount(() => clearTimeout(timer))
const srcdoc = computed(() => buildSrcdoc(safe.value, { dark: dark.value, zoom: props.zoom, title: props.title }))
</script>

<template>
  <iframe class="pvx-frame" :sandbox="PREVIEW_SANDBOX" :srcdoc="srcdoc" :title="title || '预览'" referrerpolicy="no-referrer" />
</template>
