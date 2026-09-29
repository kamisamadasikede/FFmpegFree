<template>
  <button type="button" class="lbtn" :class="[variant, { lg }]" :disabled="disabled">
    <FIcon v-if="icon" :name="icon" :size="15" /><slot />
  </button>
</template>

<script setup lang="ts">
// 原型的 .btn / .btn.pri / .btn.danger / .btn.lg。
import FIcon from '../icon/FIcon.vue'
import type { IconName } from '../icon/icons'

withDefaults(defineProps<{ variant?: 'default' | 'pri' | 'danger'; icon?: IconName; lg?: boolean; disabled?: boolean }>(), { variant: 'default' })
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
  gap: 6px;
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
.lbtn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.lbtn.lg {
  height: 32px;
  padding: 0 16px;
}
.lbtn.pri {
  background: var(--ff-primary);
  border-color: var(--ff-primary);
  color: #fff;
}
.lbtn.pri:hover:not(:disabled) {
  background: var(--ff-primary-hover);
  border-color: var(--ff-primary-hover);
}
.lbtn.danger {
  background: var(--ff-danger);
  border-color: var(--ff-danger);
  color: #fff;
}
.lbtn.danger:hover:not(:disabled) {
  background: color-mix(in srgb, var(--ff-danger) 88%, #000);
}
/* 暗色主题主色偏亮，白字对比度不够，与 ErrorOverlay 的主按钮一致改近黑字（设计评审） */
html.dark .lbtn.pri,
.ff-dark .lbtn.pri {
  color: #0b0c0e;
}
/* 暗色主题 danger 红底白字只有 3.76:1，改近黑字（与 pri 一致） */
html.dark .lbtn.danger,
.ff-dark .lbtn.danger {
  color: #0b0c0e;
}
.lbtn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.lbtn:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
</style>
