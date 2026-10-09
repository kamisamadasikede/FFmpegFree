<template>
  <span class="ct-ctx" @pointerdown.stop>
    <button type="button" class="ct-ctx-btn" aria-label="上下文容量">
      <svg class="ct-ctx-ring" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="4" opacity="0.25" />
        <circle
          class="arc"
          cx="12"
          cy="12"
          r="10"
          fill="none"
          stroke="currentColor"
          stroke-width="4"
          stroke-linecap="round"
          opacity="0.7"
          :stroke-dasharray="dash.array"
          :stroke-dashoffset="dash.offset"
        />
      </svg>
    </button>
    <span class="ct-ctx-pop" role="tooltip">
      <span class="hd">
        <span class="ttl">上下文</span>
        <span class="num">{{ summary }}</span>
      </span>
      <span class="bar" aria-hidden="true"><i :style="{ width: bar }" /></span>
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { contextRingDash, contextUsagePercent, formatContextUsageSummary } from './catContextUsage'

const props = defineProps<{ used: number; size: number }>()

const dash = computed(() => contextRingDash(props.used, props.size))
const summary = computed(() => formatContextUsageSummary(props.used, props.size))
const bar = computed(() => {
  const pct = contextUsagePercent(props.used, props.size) * 100
  if (pct <= 0) return '0%'
  return `${Math.max(pct, 1.6)}%`
})
</script>

<style scoped>
.ct-ctx {
  position: relative;
  display: inline-flex;
  flex: none;
}
.ct-ctx-btn {
  width: 30px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: 15px;
  background: transparent;
  color: var(--ff-text-2);
  cursor: default;
}
.ct-ctx-btn:hover,
.ct-ctx:focus-within .ct-ctx-btn {
  background: #f2f3f5;
}
html.dark .ct-ctx-btn:hover,
html.dark .ct-ctx:focus-within .ct-ctx-btn {
  background: var(--ff-bg-hover);
}
.ct-ctx-btn:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 1px;
}
.ct-ctx-ring {
  width: 14px;
  height: 14px;
  display: block;
}
.arc {
  transform: rotate(-90deg);
  transform-origin: center;
}
.ct-ctx-pop {
  display: none;
  position: absolute;
  right: 0;
  bottom: calc(100% + 8px);
  z-index: 40;
  width: 256px;
  box-sizing: border-box;
  padding: 12px;
  border-radius: 12px;
  background: var(--ff-bg-elevated);
  border: 1px solid var(--ff-border);
  box-shadow: var(--ff-shadow-dialog);
}
.ct-ctx:hover .ct-ctx-pop,
.ct-ctx:focus-within .ct-ctx-pop {
  display: block;
}
.hd {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  margin-bottom: 8px;
}
.ttl {
  flex: none;
  font-size: 14px;
  font-weight: 500;
  color: var(--ff-text-1);
}
.num {
  margin-left: auto;
  min-width: 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--ff-text-2);
  text-align: right;
}
.bar {
  display: block;
  height: 8px;
  border-radius: 999px;
  background: var(--ff-bg-sunken, var(--ff-border));
  overflow: hidden;
}
.bar i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--ff-text-2);
  min-width: 8px;
}
</style>
