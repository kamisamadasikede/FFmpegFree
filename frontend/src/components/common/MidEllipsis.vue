<script setup lang="ts">
/**
 * 放不下时中间省略、保留扩展名（utils/midEllipsis），悬停 title 看全名；读屏读全名。
 * 根元素里放一份看不见、高度为 0 的全名撑出“本来的宽度”，这样在 flex / 表格里能正确拿到可用宽度（不会越缩越窄）。
 * 用法：<MidEllipsis tag="b" :text="name" />；class / 事件透传到根元素。title 默认是全名，传 title 可覆盖。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useAttrs, watch } from 'vue'
import { midEllipsis } from '@/utils/midEllipsis'

defineOptions({ inheritAttrs: false })
const props = withDefaults(defineProps<{ text: string; tag?: string }>(), { tag: 'span' })
const attrs = useAttrs()
const root = ref<HTMLElement | null>(null)
const shown = ref(props.text)
const title = computed(() => (attrs.title as string | undefined) ?? props.text)

let ctx: CanvasRenderingContext2D | null = null
function fit() {
  const el = root.value
  if (!el) return
  const w = el.clientWidth
  if (!w) {
    shown.value = props.text
    return
  }
  ctx ??= document.createElement('canvas').getContext('2d')
  if (!ctx) return
  const cs = getComputedStyle(el)
  ctx.font = cs.font || `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`
  const c = ctx
  shown.value = midEllipsis(props.text, w + 0.5, (s) => c.measureText(s).width)
}
let ro: ResizeObserver | null = null
onMounted(() => {
  fit()
  if (typeof ResizeObserver !== 'undefined' && root.value) {
    ro = new ResizeObserver(() => fit())
    ro.observe(root.value)
  }
  void document.fonts?.ready.then(() => fit())
})
onBeforeUnmount(() => ro?.disconnect())
watch(
  () => props.text,
  () => {
    shown.value = props.text
    void nextTick(fit)
  },
)
</script>

<template>
  <component :is="tag" ref="root" v-bind="attrs" class="mid-el" :title="title">
    <span class="mid-sz" aria-hidden="true">{{ text }}</span>
    <span class="mid-v" aria-hidden="true">{{ shown }}</span>
    <span class="mid-sr">{{ text }}</span>
  </component>
</template>

<style>
.mid-el { display: block; min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: clip; }
.mid-el > .mid-sz { display: block; height: 0; overflow: hidden; visibility: hidden; }
.mid-el > .mid-v { display: block; overflow: hidden; }
.mid-el > .mid-sr { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
</style>
