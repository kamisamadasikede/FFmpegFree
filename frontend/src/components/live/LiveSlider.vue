<template>
  <div class="slider">
    <el-slider v-model="model" :aria-labelledby="labelId" :min="min" :max="max" :step="step" :show-tooltip="false" :disabled="disabled" class="tr" />
    <span class="val">{{ model }} {{ unit }}</span>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'

const labelId = inject<string | undefined>('ff-field-label-id', undefined)
// 原型的 .slider：4px 轨道、12px 白色圆点（主色描边）、右侧 70px 数值。
const model = defineModel<number>({ required: true })
withDefaults(defineProps<{ min?: number; max?: number; step?: number; unit?: string; disabled?: boolean }>(), {
  min: 0,
  max: 100,
  step: 1,
  unit: '',
})
</script>

<style scoped>
.slider {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  color: var(--ff-text-2);
  height: 18px;
}
.tr {
  flex: 1;
  height: 18px;
  --el-slider-height: 4px;
  --el-slider-border-radius: 2px;
  --el-slider-button-size: 12px;
  --el-slider-button-wrapper-size: 24px;
  --el-slider-button-wrapper-offset: -10px;
  --el-slider-runway-bg-color: var(--ff-border);
}
.tr :deep(.el-slider__button) {
  width: 12px;
  height: 12px;
  background: #fff;
  border: 2px solid var(--ff-primary);
  box-sizing: border-box;
}
.val {
  width: 70px;
  text-align: right;
  font-family: var(--ff-font-mono);
  color: var(--ff-text-1);
  white-space: nowrap;
}
</style>
