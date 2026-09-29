<template>
  <div class="srcs" role="radiogroup">
    <button
      v-for="o in options"
      :key="o.value"
      type="button"
      class="src"
      :class="{ on: model === o.value }"
      role="radio"
      :aria-checked="model === o.value"
      :disabled="disabled"
      @click="model = o.value"
    >
      <FIcon :name="o.icon" :size="20" />{{ o.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
// 画面来源三选一：屏幕 / 摄像头 / 窗口，样式来自 proto/pages.html 的 .srcs / .src。
import FIcon from '../icon/FIcon.vue'
import type { IconName } from '../icon/icons'

export type CaptureSource = 'screen' | 'camera' | 'window'
const model = defineModel<CaptureSource>({ default: 'screen' })
defineProps<{ disabled?: boolean }>()
const options: { value: CaptureSource; label: string; icon: IconName }[] = [
  { value: 'screen', label: '屏幕', icon: 'monitor' },
  { value: 'camera', label: '摄像头', icon: 'cam' },
  { value: 'window', label: '窗口', icon: 'film' },
]
</script>

<style scoped>
.srcs {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 8px;
}
.src {
  border: 1px solid var(--ff-border);
  border-radius: 8px;
  padding: 10px 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  font: inherit;
  font-size: 12px;
  line-height: 18px;
  color: var(--ff-text-2);
  background: none;
  cursor: pointer;
}
.src:hover:not(:disabled):not(.on) {
  background: var(--ff-bg-hover);
}
.src.on {
  border-color: var(--ff-primary);
  background: var(--ff-primary-soft);
  color: var(--ff-primary);
}
.src:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.src:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
</style>
