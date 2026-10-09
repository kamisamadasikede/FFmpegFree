<script setup lang="ts" generic="T">
// 可变高度的虚拟滚动列表：只渲染可视区域上下各 overscan 像素内的项，高度用 ResizeObserver 实测（没测到的按 estimate 估）。
// 结构：滚动容器 > 头部插槽（正常流）> 定高占位层（项绝对定位）> 尾部插槽。转换页的源文件行用它（设计 §7.3 第 8 条）。
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { DUR, EASE_IN, prefersReducedMotion } from '@/utils/motion'

const props = withDefaults(
  defineProps<{
    items: readonly T[]
    itemKey: (item: T) => string
    /** 没测到高度时的估计值 */
    estimate?: (item: T) => number
    overscan?: number
    /** 增删行时平滑：下面的行滑到新位置、新行淡入、删掉的行淡出（动画 P1） */
    animate?: boolean
  }>(),
  { estimate: () => 64, overscan: 600, animate: false },
)

const scrollEl = ref<HTMLElement | null>(null)
const layerEl = ref<HTMLElement | null>(null)
const heights = reactive(new Map<string, number>())
const scrollTop = ref(0)
const viewH = ref(0)
const layerTop = ref(0)

const keys = computed(() => props.items.map(props.itemKey))
const offsets = computed(() => {
  const out = new Array<number>(props.items.length + 1)
  out[0] = 0
  for (let i = 0; i < props.items.length; i++) out[i + 1] = out[i] + (heights.get(keys.value[i]) ?? props.estimate(props.items[i]))
  return out
})
const total = computed(() => offsets.value[props.items.length] ?? 0)
/** 第一个底边在 y 之下的项 */
function indexAt(y: number): number {
  const o = offsets.value
  let lo = 0
  let hi = props.items.length - 1
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (o[mid + 1] <= y) lo = mid + 1
    else hi = mid
  }
  return Math.max(0, lo)
}
const range = computed(() => {
  if (!props.items.length) return { from: 0, to: 0 }
  const top = scrollTop.value - layerTop.value - props.overscan
  const bottom = scrollTop.value - layerTop.value + viewH.value + props.overscan
  const from = indexAt(Math.max(0, top))
  let to = from
  while (to < props.items.length && offsets.value[to] < bottom) to++
  return { from, to: Math.max(to, Math.min(props.items.length, from + 1)) }
})
const visible = computed(() => {
  const out: { item: T; key: string; top: number; index: number }[] = []
  for (let i = range.value.from; i < range.value.to; i++) out.push({ item: props.items[i], key: keys.value[i], top: offsets.value[i], index: i })
  return out
})

let ro: ResizeObserver | null = null
function measureAll() {
  const el = scrollEl.value
  if (!el) return
  viewH.value = el.clientHeight
  layerTop.value = layerEl.value?.offsetTop ?? 0
}
function onItemResize(entries: ResizeObserverEntry[]) {
  for (const e of entries) {
    const k = (e.target as HTMLElement).dataset.vkey
    if (!k) continue
    const h = Math.round((e.target as HTMLElement).offsetHeight)
    if (h > 0 && heights.get(k) !== h) heights.set(k, h)
  }
  measureAll()
}
const observed = new Set<Element>()
function bindItem(el: Element | null) {
  if (!el || !ro || observed.has(el)) return
  observed.add(el)
  ro.observe(el)
}
watch(visible, () => {
  // 卸载了的项不再观察（Vue 会移除元素；这里清掉引用）
  for (const el of [...observed]) if (!el.isConnected) {
    ro?.unobserve(el)
    observed.delete(el)
  }
})
// ── 增删行动效（animate=true 时）──
// 只在「纯增加」或「纯删除」且不超过 BATCH 行时播：首次读到列表、筛选 / 搜索换了一批、一次加很多文件都直接出现。
// 实现只动 transform / opacity：在一小段时间里给 .cv-vl-item 的 transform（translateY 定位）开过渡，
// 这期间新测到的高度让下面的行滑过去而不是跳；新行用独立的 translate + opacity 淡入；
// 删掉的行留一个 inert、不可点、aria-hidden 的快照淡出 150ms 后移除（快照里的视频 / iframe 先拿掉，不会加载任何东西）。
const BATCH = 6
const moving = ref(false)
const fresh = reactive(new Set<string>())
let moveTimer: ReturnType<typeof setTimeout> | undefined
let freshTimer: ReturnType<typeof setTimeout> | undefined
function ghost(el: HTMLElement) {
  const layer = layerEl.value
  if (!layer || typeof el.animate !== 'function') return
  const g = el.cloneNode(true) as HTMLElement
  g.querySelectorAll('video,audio,iframe,object,embed').forEach((x) => x.remove())
  g.removeAttribute('data-vkey')
  g.setAttribute('inert', '')
  g.setAttribute('aria-hidden', 'true')
  g.classList.add('cv-vl-ghost')
  g.style.pointerEvents = 'none'
  layer.prepend(g) // 放在最前面：滑上来的行盖在快照上面
  const a = g.animate([{ opacity: 1 }, { opacity: 0 }], { duration: DUR.fast, easing: EASE_IN, fill: 'forwards' })
  const rm = () => g.remove()
  a.onfinish = rm
  a.oncancel = rm
}
watch(
  keys,
  (now, before) => {
    if (!props.animate || !before?.length || prefersReducedMotion()) return
    const was = new Set(before)
    const is = new Set(now)
    const added = now.filter((k) => !was.has(k))
    const removed = before.filter((k) => !is.has(k))
    if (!added.length && !removed.length) return
    if ((added.length && removed.length) || added.length > BATCH || removed.length > BATCH) return
    // flush:'pre'：DOM 还是旧的，先给要删的行做快照
    for (const k of removed) {
      const el = layerEl.value?.querySelector<HTMLElement>(`:scope > [data-vkey="${CSS.escape(k)}"]`)
      if (el) ghost(el)
    }
    for (const k of added) fresh.add(k)
    moving.value = true
    clearTimeout(moveTimer)
    clearTimeout(freshTimer)
    // 新行要等 ResizeObserver 测到真实高度，下面的行才移到最终位置：多留一点时间
    moveTimer = setTimeout(() => (moving.value = false), DUR.base * 2)
    freshTimer = setTimeout(() => fresh.clear(), DUR.base + 50)
  },
  { flush: 'pre' },
)
onBeforeUnmount(() => {
  clearTimeout(moveTimer)
  clearTimeout(freshTimer)
})

function onScroll() {
  scrollTop.value = scrollEl.value?.scrollTop ?? 0
}
onMounted(() => {
  ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(onItemResize) : null
  if (ro && scrollEl.value) ro.observe(scrollEl.value)
  if (ro && scrollEl.value?.firstElementChild && scrollEl.value.firstElementChild !== layerEl.value) ro.observe(scrollEl.value.firstElementChild)
  measureAll()
})
onBeforeUnmount(() => ro?.disconnect())

/** 滚到某一项（align=start 放在顶部；nearest 只在看不见时滚） */
async function scrollToKey(key: string, align: 'start' | 'nearest' = 'start') {
  const i = keys.value.indexOf(key)
  const el = scrollEl.value
  if (i < 0 || !el) return
  measureAll()
  const top = layerTop.value + offsets.value[i]
  if (align === 'nearest' && top >= el.scrollTop && top + 52 <= el.scrollTop + el.clientHeight) return
  el.scrollTop = Math.max(0, top - 8)
  onScroll()
  await nextTick()
}
function scrollToTop() {
  if (scrollEl.value) scrollEl.value.scrollTop = 0
  onScroll()
}
defineExpose({ scrollToKey, scrollToTop, el: scrollEl })
</script>
<template>
  <div ref="scrollEl" class="cv-scroll" @scroll.passive="onScroll">
    <div class="cv-vl-head"><slot name="header" /></div>
    <div ref="layerEl" class="cv-vl" :class="{ 'cv-vl-moving': moving }" :style="{ height: total + 'px' }">
      <div v-for="v in visible" :key="v.key" :ref="(el) => bindItem(el as Element | null)" class="cv-vl-item" :class="{ 'cv-vl-new': fresh.has(v.key) }" :data-vkey="v.key" :style="{ transform: `translateY(${v.top}px)` }">
        <slot :item="v.item" :index="v.index" />
      </div>
    </div>
    <slot name="footer" />
  </div>
</template>
