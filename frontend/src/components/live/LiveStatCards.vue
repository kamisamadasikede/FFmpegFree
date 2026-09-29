<template>
  <div class="statrow">
    <div v-for="c in cards" :key="c.label" class="mini">
      <small>{{ c.label }}</small>
      <b :style="c.color ? { color: c.color } : undefined">{{ c.value }} <span v-if="c.unit" class="unit">{{ c.unit }}</span></b>
      <svg class="spark" viewBox="0 0 120 22" preserveAspectRatio="none" :style="{ stroke: c.stroke }"><path :d="c.path" /></svg>
    </div>
  </div>
</template>

<script setup lang="ts">
// 直播页的四张实时指标卡，样式来自 proto/pages.html 的 .statrow / .mini。
import { computed } from 'vue'
import { formatBytes, type LiveSeries, type LiveStats } from '@/composables/useLiveSession'

const props = withDefaults(defineProps<{ stats: LiveStats; series: LiveSeries; bytesLabel?: string }>(), { bytesLabel: '已推送' })

/** 数值序列 → 迷你折线的 path（viewBox 120×22）。平的序列画在 flatY 上，不足两个点画一条直线 */
function sparkPath(values: number[], flatY: number, top = 3, bottom = 19): string {
  if (values.length < 2) return `M0 ${flatY} 120 ${flatY}`
  const min = Math.min(...values)
  const max = Math.max(...values)
  if (max === min) return `M0 ${flatY} 120 ${flatY}`
  const step = 120 / (values.length - 1)
  return (
    'M' +
    values
      .map((v, i) => `${+(i * step).toFixed(1)} ${+(bottom - ((v - min) / (max - min)) * (bottom - top)).toFixed(1)}`)
      .join(' ')
  )
}

const cards = computed(() => {
  const s = props.stats
  const bytes = formatBytes(s.bytes)
  return [
    { label: '实时码率', value: String(Math.round(s.bitrateKbps)), unit: 'kbps', stroke: 'var(--ff-primary)', path: sparkPath(props.series.bitrate, 11, 7, 14) },
    { label: '帧率', value: s.fps.toFixed(1), unit: '', stroke: 'var(--ff-success)', path: sparkPath(props.series.fps, 8, 5, 11) },
    {
      label: '丢帧',
      value: String(s.dropped),
      unit: '',
      color: s.dropped > 0 ? 'var(--ff-warning)' : 'var(--ff-success)',
      stroke: 'var(--ff-text-3)',
      path: sparkPath(props.series.dropped, 20, 5, 20),
    },
    { label: props.bytesLabel, value: bytes.value, unit: bytes.unit, stroke: '#8b5cf6', path: sparkPath(props.series.bytes, 20, 3, 20) },
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
  color: var(--ff-text-3);
}
b {
  font-size: 18px;
  font-weight: 600;
  font-family: var(--ff-font-mono);
  line-height: 1.5;
  white-space: nowrap;
}
.unit {
  font-size: 12px;
  font-weight: 400;
  color: var(--ff-text-3);
}
.spark {
  width: 100%;
  height: 22px;
  margin-top: 4px;
  fill: none;
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
}
</style>
