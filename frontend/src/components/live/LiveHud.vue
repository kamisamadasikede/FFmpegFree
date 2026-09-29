<template>
  <div class="live-badge"><span class="on">{{ label }}</span><span>{{ time }}</span></div>
  <div v-if="lines.length" class="ov-stats"><template v-for="(l, i) in lines" :key="i">{{ l }}<br v-if="i < lines.length - 1" /></template></div>
</template>

<script setup lang="ts">
// 播放器左上角“直播中 + 时长”和右上角实时参数，样式来自 proto/pages.html 的 .live-badge / .ov-stats。
// 放在 PlayerShell 的 #overlay 插槽里。
defineProps<{ label: string; time: string; lines: string[] }>()
</script>

<style scoped>
.live-badge {
  position: absolute;
  left: 14px;
  top: 14px;
  display: flex;
  gap: 6px;
  z-index: 2;
}
.live-badge span {
  height: 22px;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 5px;
  color: #fff;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(6px);
}
.live-badge .on {
  background: #ef4444;
}
.live-badge .on::before {
  content: '';
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #fff;
}
.ov-stats {
  position: absolute;
  right: 14px;
  top: 14px;
  z-index: 2;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(6px);
  border-radius: 6px;
  padding: 8px 10px;
  color: #e5e7eb;
  font-family: var(--ff-font-mono);
  font-size: 11px;
  line-height: 1.6;
}
</style>
