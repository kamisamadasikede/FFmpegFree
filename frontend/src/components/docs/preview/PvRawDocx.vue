<script setup lang="ts">
// kind=raw 的 docx：按 Range 分段取字节（每段 ≤ 4 MiB），再交给 docx-preview（按需加载）。只显示，不运行任何脚本。
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { fetchRawBytes } from '@/api/docV27'
import { toAppError } from '@/api/call'

const props = defineProps<{ url: string; sizeBytes: number; scale: number }>()
const emit = defineEmits<{ (e: 'pages', n: number): void; (e: 'page', n: number): void; (e: 'gone'): void; (e: 'failed'): void; (e: 'ready'): void }>()
const host = ref<HTMLElement | null>(null)
const styleHost = ref<HTMLElement | null>(null)
const scrollEl = ref<HTMLElement | null>(null)
const ac = new AbortController()
onMounted(async () => {
  try {
    const bytes = await fetchRawBytes(props.url, props.url.startsWith('blob:') ? 0 : props.sizeBytes, { signal: ac.signal })
    const { renderAsync } = await import('docx-preview')
    if (!host.value) return
    await renderAsync(bytes, host.value, styleHost.value ?? undefined, {
      inWrapper: false,
      ignoreLastRenderedPageBreak: true,
      renderHeaders: true,
      renderFooters: true,
      useBase64URL: true, // 图片用 data: 地址，不出 blob / 网络请求
      experimental: false,
    })
    // 链接只留文字
    host.value.querySelectorAll('a[href]').forEach((a) => a.removeAttribute('href'))
    emit('pages', host.value.querySelectorAll('section.docx').length || 1)
    emit('ready')
  } catch (e) {
    if (ac.signal.aborted) return
    if (toAppError(e).code === 'NOT_FOUND') emit('gone')
    else emit('failed')
  }
})
onBeforeUnmount(() => ac.abort())
function onScroll() {
  const root = scrollEl.value
  if (!root || !host.value) return
  const secs = host.value.querySelectorAll<HTMLElement>('section.docx')
  const mid = root.scrollTop + root.clientHeight / 2
  let n = 1
  secs.forEach((s, i) => {
    if (s.offsetTop * props.scale <= mid) n = i + 1
  })
  emit('page', n)
}
</script>

<template>
  <div ref="scrollEl" class="pvx-scroll pvx-yscroll" @scroll.passive="onScroll">
    <div ref="styleHost" />
    <div ref="host" class="pvx-docx" :style="{ zoom: scale }" />
  </div>
</template>
