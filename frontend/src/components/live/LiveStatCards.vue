<template>
  <div class="statrow">
    <div v-for="c in cards" :key="c.label" class="mini">
      <small>{{ c.label }}</small>
      <b :style="c.color ? { color: c.color } : undefined">{{ c.value }} <span v-if="c.unit" class="unit">{{ c.unit }}</span></b>
    </div>
  </div>
</template>

<script setup lang="ts">
// 直播页的四张实时指标卡（设计稿 v0.2：去掉 sparkline，数值 20px，说明字 --ff-text-2），样式来自 proto/pages.html 的 .statrow / .mini。
import { computed } from 'vue'
import { formatBytes, type LiveStats } from '@/composables/useLiveSession'

const props = withDefaults(defineProps<{ stats: LiveStats; bytesLabel?: string }>(), { bytesLabel: '已推送' })

const cards = computed(() => {
  const s = props.stats
  const bytes = formatBytes(s.bytes)
  return [
    { label: '实时码率', value: String(Math.round(s.bitrateKbps)), unit: 'kbps' },
    { label: '帧率', value: s.fps.toFixed(1), unit: '' },
    {
      label: '丢帧',
      value: String(s.dropped),
      unit: '',
      color: s.dropped > 0 ? 'var(--ff-warning)' : 'var(--ff-success)',
    },
    { label: props.bytesLabel, value: bytes.value, unit: bytes.unit },
  ]
})
</script>

<style scoped>
.statrow {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  flex: none;
}
.mini {
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: 10px;
  min-width: 0;
}
small {
  font-size: 12px;
  color: var(--ff-text-2);
}
b {
  font-size: var(--ff-fs-xl);
  font-weight: 600;
  font-family: var(--ff-font-mono);
  line-height: 1.5;
  white-space: nowrap;
}
.unit {
  font-size: var(--ff-fs-xs);
  font-weight: 400;
  color: var(--ff-text-2);
}
</style>
