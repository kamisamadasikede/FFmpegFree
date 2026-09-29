<template>
  <div class="field">
    <label>{{ label }}</label>
    <slot />
  </div>
</template>

<script setup lang="ts">
// 面板里的一个表单项：12px 灰色标签 + 控件，标签与控件间距 6px。
defineProps<{ label: string }>()
</script>

<style scoped>
label {
  display: block;
  font-size: 12px;
  color: var(--ff-text-2);
  margin-bottom: 6px;
  line-height: 18px;
}
</style>
