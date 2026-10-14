<template>
  <div v-if="catState.notice" :key="catState.notice.key" class="pn" :class="catState.notice.tone" role="status" data-testid="cat-notice">
    <FIcon :name="catState.notice.tone === 'ok' ? 'check' : catState.notice.tone === 'warn' ? 'warn' : 'info'" :size="14" />{{ catState.notice.text }}
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

import FIcon from '@/components/icon/FIcon.vue'
import { catState } from '@/views/cat/catState'
/** 设置 / 定时任务页里的顶部浮提示（与对话页 .ct-notice 同样式；对话页的那个只在聊天区里渲染） */
</script>

<style scoped>
.pn {
  position: sticky;
  top: 12px;
  z-index: 20;
  width: max-content;
  max-width: calc(100% - 48px);
  margin: 0 auto -38px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border-radius: 10px;
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  box-shadow: var(--cat-shadow-md);
  font-size: 13px;
  color: var(--cat-text);
  white-space: nowrap;
  animation: cat-pop-in 200ms var(--ff-ease) both;
}
.pn :deep(svg) {
  flex: none;
  color: var(--cat-accent);
}
.pn.ok :deep(svg) {
  color: var(--cat-success);
}
.pn.warn {
  border-color: color-mix(in srgb, var(--cat-warn) 55%, var(--cat-border));
}
.pn.warn :deep(svg) {
  color: var(--cat-warn);
}
</style>
