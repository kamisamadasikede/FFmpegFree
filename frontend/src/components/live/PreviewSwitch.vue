<template>
  <div class="pvs" :class="{ dis: disabled }">
    <button
      type="button"
      class="sw"
      role="switch"
      :aria-checked="modelValue"
      :aria-disabled="disabled || undefined"
      :aria-labelledby="labelId"
      :aria-describedby="noteId"
      @click="!disabled && emit('update:modelValue', !modelValue)"
    >
      <span :id="labelId" class="lb">{{ PREVIEW_SWITCH_LABEL }}</span>
      <i class="track" :class="{ on: modelValue }"><b /></i>
    </button>
    <small :id="noteId" class="note">{{ note }}</small>
  </div>
</template>

<script setup lang="ts">
// 设置栏底部的“开启预览”（设计说明 §2.5）。有进行中的当前会话时就是这一路的预览，拨动只连接 / 断开播放器，不重启推流（契约 v0.25 ⑤）；没有时是下一路的初始值。
// 第一行“开启预览”+ 右侧开关，整行是一个 button role=switch；第二行小字。禁用（ffmpeg 未就绪 / 正在开始 / 播放中）用 aria-disabled（仍可聚焦，读得到原因），聚焦环 2px。
import { useId } from 'vue'
import { PREVIEW_SWITCH_LABEL, PREVIEW_SWITCH_NOTE } from '@/errors/livePreviewMessages'

withDefaults(defineProps<{ modelValue: boolean; disabled?: boolean; note?: string }>(), { note: PREVIEW_SWITCH_NOTE })
const emit = defineEmits<{ 'update:modelValue': [v: boolean] }>()
const uid = useId()
const labelId = `ff-pvs-l-${uid}`
const noteId = `ff-pvs-n-${uid}`
</script>

<style scoped>
.pvs {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.pvs.dis {
  opacity: 0.55;
}
.sw {
  height: 24px;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0;
  border: 0;
  background: none;
  font: inherit;
  cursor: pointer;
}
.sw[aria-disabled='true'] {
  cursor: not-allowed;
}
.sw:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
  border-radius: 4px;
}
.lb {
  font-size: var(--ff-fs-sm);
  font-weight: 500;
  color: var(--ff-text-1);
}
.track {
  width: 28px;
  height: 16px;
  border-radius: 8px;
  background: var(--ff-border);
  position: relative;
  flex: none;
  transition: background 0.15s;
}
.track.on {
  background: var(--ff-primary);
}
.track b {
  position: absolute;
  left: 2px;
  top: 2px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #fff;
  transition: transform 0.15s;
}
.track.on b {
  transform: translateX(12px);
}
@media (prefers-reduced-motion: reduce) {
  .track,
  .track b {
    transition: none;
  }
}
.note {
  font-size: var(--ff-fs-xs);
  line-height: 18px;
  color: var(--ff-text-2);
}
</style>
