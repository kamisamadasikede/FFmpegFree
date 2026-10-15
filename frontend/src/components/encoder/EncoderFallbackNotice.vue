<template>
  <!-- 动画 P1：点关闭后收起（高度 + 透明度 150ms），下面的内容不跳 -->
  <MotionCollapse>
  <div v-if="!closed" class="efn" :class="variant" role="status" v-bind="$attrs">
    <FIcon name="warn" :size="variant === 'row' ? 14 : 16" />
    <span class="t">{{ text }}</span>
    <button v-if="variant !== 'row'" type="button" class="lk" @click="emit('settings')">{{ ENCODER_FALLBACK_SETTINGS_LINK }}</button>
    <button v-else type="button" class="lk" @click="emit('log')">{{ ENCODER_FALLBACK_LOG_LINK }}</button>
    <button v-if="variant !== 'row' && !noClose" type="button" class="x" :aria-label="closeLabel ?? ENCODER_FALLBACK_CLOSE" @click="close"><FIcon name="x" :size="14" /></button>
  </div>
  </MotionCollapse>
</template>

<script setup lang="ts">
// “显卡编码失败，已自动改用 CPU”的提示（设计稿 §2.4）：回退是降级成功不是失败，三处都用警告色，不用红。
// 只是展示组件：**本版没有接线**。接入点（后端事件 / 字段确定后）见 api/README.md “编码设备 · 回退提示接入点”。
//   variant='convert'：转换页进度面板上方的提示条；'live'：直播页 Tab 条下方的提示条；'row'：任务中心该任务行下方的一行（末尾“查看日志”，无关闭）。
// 文案默认取 errors/encoderMessages.ts（待产品经理确认），调用方也可以传 text 覆盖。关闭只影响本次会话（组件内状态）。
import { computed, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MotionCollapse from '@/components/motion/MotionCollapse.vue'
import {
  ENCODER_FALLBACK_CLOSE, ENCODER_FALLBACK_CONVERT, ENCODER_FALLBACK_LIVE, ENCODER_FALLBACK_LOG_LINK, ENCODER_FALLBACK_SETTINGS_LINK, ENCODER_FALLBACK_TASK_ROW,
} from '@/errors/encoderMessages'

// class 等透传属性放到提示条本身（根是 MotionCollapse）
defineOptions({ inheritAttrs: false })
const props = withDefaults(defineProps<{ variant?: 'convert' | 'live' | 'row'; text?: string; closeLabel?: string; noClose?: boolean }>(), { variant: 'convert' })
const emit = defineEmits<{ settings: []; log: []; close: [] }>()
const closed = ref(false)
const text = computed(() => props.text ?? (props.variant === 'live' ? ENCODER_FALLBACK_LIVE : props.variant === 'row' ? ENCODER_FALLBACK_TASK_ROW : ENCODER_FALLBACK_CONVERT))
function close() {
  closed.value = true
  emit('close')
}
</script>

<style scoped>
.efn {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--ff-text-1);
  font-size: var(--ff-fs-sm);
}
.efn:not(.row) {
  min-height: 36px;
  box-sizing: border-box;
  padding: 0 8px 0 12px;
  border: 1px solid color-mix(in srgb, var(--ff-warning) 28%, var(--ff-bg-surface));
  border-radius: var(--ff-radius-md);
  background: color-mix(in srgb, var(--ff-warning) 10%, var(--ff-bg-surface));
}
.efn > svg {
  color: var(--ff-warning-text);
  flex: none;
}
.t {
  flex: 1;
  min-width: 0;
}
.lk {
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  color: var(--ff-primary-text);
  cursor: pointer;
  flex: none;
}
.lk:focus-visible,
.x:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
  border-radius: 2px;
}
.x {
  border: 0;
  background: none;
  width: 24px;
  height: 24px;
  display: grid;
  place-items: center;
  color: var(--ff-text-2);
  cursor: pointer;
  border-radius: 4px;
}
.x:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
/* 任务中心行内：12px，警告色文字（任务没有失败） */
.efn.row {
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  align-items: flex-start;
  gap: 4px;
  color: var(--ff-warning-text);
}
.efn.row > svg {
  margin-top: 2px;
}
.efn.row .lk {
  margin-left: 4px;
}
</style>
