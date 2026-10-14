<template>
  <div
    v-if="parts.length"
    ref="root"
    class="ct-meta cmf"
    data-testid="cat-msg-footer"
    :tabindex="tip ? 0 : undefined"
    :aria-label="ariaText"
    @mouseenter="hover = true"
    @mouseleave="hover = false"
    @focus="hover = true"
    @blur="hover = false"
  >
    <span class="cmf-line"><template v-for="(p, i) in shown" :key="p.key"><span v-if="i" class="cmf-sep">{{ C.sep }}</span><span :data-k="p.key">{{ p.text }}</span></template></span>
    <span ref="probe" class="cmf-probe" aria-hidden="true">{{ fullText }}</span>
    <div v-if="tip && (hover || forceTip)" class="cmf-tip" role="tooltip" data-testid="cat-msg-footer-tip">{{ tip }}</div>
  </div>
</template>

<script setup lang="ts">
// 契约 v0.45 §6.19.19：消息尾；窄宽度按 CAT_FOOTER_HIDE_ORDER 从尾部隐藏，悬停一个提示（隐藏项 + 两行说明）
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CAT_FOOTER_COPY as C } from './catMessageFooterCopy'
import { catFooterSplit, catFooterTooltip, catMessageFooterParts, type CatMessageFooterFields } from './catMessageFooter'

const props = defineProps<{ fields: CatMessageFooterFields; forceTip?: boolean }>()
const root = ref<HTMLElement | null>(null)
const probe = ref<HTMLElement | null>(null)
const hover = ref(false)
const hideCount = ref(0)

const parts = computed(() => catMessageFooterParts(props.fields))
const split = computed(() => catFooterSplit(parts.value, hideCount.value))
const shown = computed(() => split.value[0])
const tip = computed(() => catFooterTooltip(parts.value, split.value[1]))
const fullText = computed(() => parts.value.map((p) => p.text).join(C.sep))
const ariaText = computed(() => [shown.value.map((p) => p.text).join(C.sep), tip.value].filter(Boolean).join('\n'))

/** 用隐藏的测量节点逐个去尾，直到放得下（至少留一段） */
function fit() {
  const el = root.value
  const pr = probe.value
  if (!el || !pr) return
  const avail = el.clientWidth
  let n = 0
  for (; n < parts.value.length - 1; n++) {
    pr.textContent = catFooterSplit(parts.value, n)[0].map((p) => p.text).join(C.sep)
    if (pr.offsetWidth <= avail) break
  }
  hideCount.value = n
}

let ro: ResizeObserver | null = null
onMounted(() => {
  void nextTick(fit)
  if (typeof ResizeObserver !== 'undefined' && root.value) {
    ro = new ResizeObserver(() => fit())
    ro.observe(root.value)
  }
})
watch(parts, () => void nextTick(fit))
onBeforeUnmount(() => ro?.disconnect())
</script>

<style scoped>
.cmf {
  position: relative;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
  outline: none;
}
.cmf-line {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
}
.cmf-probe {
  position: absolute;
  visibility: hidden;
  left: 0;
  top: 0;
  white-space: nowrap;
  pointer-events: none;
}
.cmf-tip {
  position: absolute;
  left: 0;
  bottom: calc(100% + 6px);
  z-index: 20;
  padding: 6px 10px;
  border-radius: 6px;
  background: var(--ff-tooltip-bg, rgba(32, 33, 36, 0.92));
  color: var(--ff-tooltip-fg, #fff);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.18);
}
</style>
