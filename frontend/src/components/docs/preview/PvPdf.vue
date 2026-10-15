<script setup lang="ts">
// kind=pdf：沿用 @tato30/vue-pdf（usePDF / VuePDF），本地地址交给 pdf.js（它自己按 Range 取）。按页竖向滚动，只画看得见的页。
import { computed, onBeforeUnmount, ref, shallowRef, watch, nextTick } from 'vue'
import { VuePDF, usePDF } from '@tato30/vue-pdf'

const props = defineProps<{ url: string; scale: number; initialPage?: number }>()
const emit = defineEmits<{ (e: 'pages', n: number): void; (e: 'page', n: number): void; (e: 'gone'): void; (e: 'failed'): void; (e: 'scale', n: number): void }>()
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

const sizes = ref<Record<number, {width: number; height: number}>>({})
const sizeFor = (n: number) => sizes.value[n] || sizes.value[1] || {width: 612, height: 792}
const width = computed(() => Math.round(sizeFor(1).width * props.scale))
const slotStyle = (n: number) => ({width: Math.round(sizeFor(n).width * props.scale) + 'px', height: Math.round(sizeFor(n).height * props.scale) + 'px'})
let sizeToken = 0
watch(pdf, async task => {
  const token = ++sizeToken, restorePage = props.initialPage || 1
  sizes.value = {}
  if (!task) return
  try {
    const doc = await task.promise
    for (let n = 1; n <= doc.numPages; n++) {
      if (token !== sizeToken) return
      const p = await doc.getPage(n)
      const viewport = p.getViewport({scale: 1})
      sizes.value[n] = {width: viewport.width, height: viewport.height}
    }
    await nextTick()
    if (token === sizeToken) { observe(); scrollToPage(restorePage) }
  } catch { if (token === sizeToken) emit('failed') }
})
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
onBeforeUnmount(() => { sizeToken++; io?.disconnect() })

function onScroll() {
  const root = scrollEl.value
  if (!root || !pages.value) return
  const rect = root.getBoundingClientRect()
  const mid = rect.top + root.clientHeight / 2
  let best = 1, nearest = Infinity
  root.querySelectorAll<HTMLElement>('[data-pg]').forEach(el => {
    const pg = el.getBoundingClientRect(), distance = Math.abs(pg.top + pg.height / 2 - mid)
    if (distance < nearest) { nearest = distance; best = Number(el.dataset.pg) }
  })
  emit('page', best)
}
function scrollToPage(n: number) {
  const root = scrollEl.value, el = root?.querySelector<HTMLElement>('[data-pg="' + n + '"]')
  if (root && el) root.scrollTop += el.getBoundingClientRect().top - root.getBoundingClientRect().top - 16
}
function fitWidth() { if (scrollEl.value) emit('scale', Math.max(.25, Math.min(2.5, (scrollEl.value.clientWidth - 48) / sizeFor(1).width))) }
defineExpose({ scrollEl, scrollToPage, fitWidth })
</script>

<template>
  <div ref="scrollEl" class="pvx-scroll pvx-yscroll" @scroll.passive="onScroll">
    <div class="pvx-pgs">
      <div v-for="n in pages" :key="n" class="pvx-pdfpg" :data-pg="n" :style="slotStyle(n)">
        <VuePDF v-if="pdf && visible.has(n)" :pdf="pdf" :page="n" :width="Math.round(sizeFor(n).width * scale)" />
      </div>
    </div>
  </div>
</template>
