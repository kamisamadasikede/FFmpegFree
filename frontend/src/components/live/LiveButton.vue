<template>
  <el-tooltip v-if="tip" :content="tip" effect="light" placement="top" :show-after="300" :hide-after="0">
    <button ref="el" type="button" :class="cls" :aria-disabled="blocked || undefined" @click="onClick"><FIcon v-if="icon" :name="icon" :size="iconSize" /><slot /></button>
  </el-tooltip>
  <button v-else ref="el" type="button" :class="cls" :aria-disabled="blocked || undefined" @click="onClick"><FIcon v-if="icon" :name="icon" :size="iconSize" /><slot /></button>
</template>

<script setup lang="ts">
// 设计稿 v0.2 的按钮：.btn / .btn.pri / .btn.danger / .btn.lg / .btn.sm / .btn.text(.dg / .rm)。
// 置灰一律用 aria-disabled（不用 disabled：保持可聚焦、能出 tooltip）；置灰时点击不触发。tip 只在置灰时显示（如“需要先安装 ffmpeg”）。
import { computed } from 'vue'
import FIcon from '../icon/FIcon.vue'
import type { IconName } from '../icon/icons'

const props = withDefaults(
  defineProps<{ variant?: 'default' | 'pri' | 'danger' | 'text' | 'textdanger' | 'rm'; icon?: IconName; lg?: boolean; sm?: boolean; disabled?: boolean; tipWhenDisabled?: string }>(),
  { variant: 'default' },
)
const emit = defineEmits<{ click: [e: MouseEvent] }>()
const blocked = computed(() => !!props.disabled)
const tip = computed(() => (props.disabled ? props.tipWhenDisabled : undefined))
const iconSize = computed(() => (props.sm ? 13 : 15))
const cls = computed(() => ['lbtn', props.variant, { lg: props.lg, sm: props.sm }])
function onClick(e: MouseEvent) {
  if (props.disabled) {
    e.preventDefault()
    return
  }
  emit('click', e)
}
</script>

<style scoped>
.lbtn {
  height: 28px;
  padding: 0 12px;
  border-radius: 6px;
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font: inherit;
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  cursor: pointer;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
.lbtn:hover:not([aria-disabled='true']) {
  background: var(--ff-bg-hover);
}
.lbtn.lg {
  height: 32px;
  padding: 0 16px;
}
.lbtn.sm {
  height: 24px;
  padding: 0 8px;
  font-size: var(--ff-fs-xs);
}
.lbtn.pri {
  background: var(--ff-badge-bg);
  border-color: var(--ff-badge-bg);
  color: var(--ff-on-primary);
}
.lbtn.pri:hover:not([aria-disabled='true']) {
  background: color-mix(in srgb, var(--ff-badge-bg) 88%, #000);
  border-color: transparent;
}
.lbtn.danger {
  background: var(--ff-danger);
  border-color: var(--ff-danger);
  color: var(--ff-on-primary);
}
.lbtn.danger:hover:not([aria-disabled='true']) {
  background: color-mix(in srgb, var(--ff-danger) 88%, #000);
}
.lbtn.text,
.lbtn.textdanger,
.lbtn.rm {
  background: transparent;
  border-color: transparent;
  color: var(--ff-primary-text);
}
.lbtn.textdanger {
  color: var(--ff-danger-text);
}
/* [移除]：次要文字按钮，13px、24px 高、左右 8px（设计稿 §2.2 btn text rm） */
.lbtn.rm {
  height: 24px;
  padding: 0 8px;
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-2);
}
.lbtn.rm:hover:not([aria-disabled='true']) {
  color: var(--ff-text-1);
}
/* 置灰：aria-disabled */
.lbtn[aria-disabled='true'] {
  background: var(--ff-bg-hover);
  border-color: var(--ff-border);
  color: var(--ff-text-3);
  cursor: not-allowed;
}
.lbtn.text[aria-disabled='true'],
.lbtn.textdanger[aria-disabled='true'],
.lbtn.rm[aria-disabled='true'] {
  background: transparent;
  border-color: transparent;
}
.lbtn.pri[aria-disabled='true'] {
  background: color-mix(in srgb, var(--ff-badge-bg) 40%, var(--ff-bg-surface));
  border-color: transparent;
  color: var(--ff-on-primary);
}
.lbtn:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.lbtn.rm:focus-visible {
  outline-offset: 1px;
}
</style>
