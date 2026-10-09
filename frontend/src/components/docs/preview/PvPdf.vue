<script setup lang="ts">
// kind=pdf：沿用 @tato30/vue-pdf（usePDF / VuePDF），本地地址交给 pdf.js（它自己按 Range 取）。按页竖向滚动，只画看得见的页。
import { computed, onBeforeUnmount, ref, shallowRef, watch, nextTick } from 'vue'
import { VuePDF, usePDF } from '@tato30/vue-pdf'

const props = defineProps<{ url: string; scale: number }>()
const emit = defineEmits<{ (e: 'pages', n: number): void; (e: 'page', n: number): void; (e: 'gone'): void; (e: 'failed'): void }>()
const src = shallowRef<string | null>(props.url)
watch(() => props.url, (u) => (src.value = u))
const { pdf, pages } = usePDF(src, {
  onError: (err: unknown) => {
    const e = err as { status?: number; name?: string } | null
    if (e?.status === 404 || e?.name === 'MissingPDFException') emit('gone')
    else emit('failed')
  },
})
watch(pages, (n) => emit('pages', n || 0))

const A4_W = 794
const width = computed(() => Math.round(A4_W * props.scale))
const height = computed(() => Math.round(width.value * 1.414))
const visible = ref(new Set<number>([1, 2]))
const scrollEl = ref<HTMLElement | null>(null)
let io: IntersectionObserver | null = null
function observe() {
  io?.disconnect()
  const root = scrollEl.value
  if (!root) return
  io = new IntersectionObserver(
    (ents) => {
      const s = new Set(visible.value)
      for (const en of ents) {
        const n = Number((en.target as HTMLElement).dataset.pg)
        if (en.isIntersecting) s.add(n)
        else s.delete(n)
      }
      visible.value = s
    },
    { root, rootMargin: '600px 0px' },
  )
  root.querySelectorAll<HTMLElement>('[data-pg]').forEach((el) => io!.observe(el))
}
watch([pages, width], () => nextTick(observe))
onBeforeUnmount(() => io?.disconnect())

function onScroll() {
  const root = scrollEl.value
  if (!root || !pages.value) return
  const slot = height.value + 16
  const n = Math.min(pages.value, Math.max(1, Math.floor((root.scrollTop + root.clientHeight / 2 - 24) / slot) + 1))
  emit('page', n)
}
defineExpose({ scrollEl })
</script>

<template>
  <div ref="scrollEl" class="pvx-scroll pvx-yscroll" @scroll.passive="onScroll">
    <div class="pvx-pgs">
      <div v-for="n in pages" :key="n" class="pvx-pdfpg" :data-pg="n" :style="{ width: width + 'px', height: height + 'px' }">
        <VuePDF v-if="pdf && visible.has(n)" :pdf="pdf" :page="n" :width="width" />
      </div>
    </div>
  </div>
</template>
