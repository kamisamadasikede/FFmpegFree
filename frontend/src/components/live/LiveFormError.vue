<template>
  <div :id="errorId" class="lv-err" role="alert">
    <FIcon name="warn" :size="14" />
    <span>{{ text }}</span>
    <slot />
  </div>
</template>

<script setup lang="ts">
// 设计稿 v0.2 的错误行：12px、--ff-danger-text，前置 14px 警告图标，role="alert"。
// 放在 LiveField 里（字段级错误）时自动取该字段的错误行 id，供输入框 aria-describedby 关联；表单级错误传 id 或不传。文案由调用方从 errors/errorMessages.ts 取，这里不改字。
import { inject } from 'vue'
import FIcon from '../icon/FIcon.vue'

const props = defineProps<{ text: string; id?: string }>()
const fieldErrorId = inject<string | undefined>('ff-field-error-id', undefined)
const errorId = props.id ?? fieldErrorId
</script>

<style scoped>
.lv-err {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  margin-top: 4px;
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-danger-text);
}
.lv-err > svg {
  margin-top: 2px;
  flex: none;
}
</style>
