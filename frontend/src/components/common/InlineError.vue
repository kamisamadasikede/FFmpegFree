<template>
  <div :id="errorId" class="ff-inline-error" role="alert">
    <FIcon name="warn" :size="14" />
    <span v-if="bare">{{ description ?? resolved.description }}</span>
    <span v-else>{{ resolved.title }}。{{ description ?? resolved.description }}</span>
  </div>
</template>

<script setup lang="ts">
// 表单行内错误：一行红字，样式来自 proto/errors.html 的 .ferr。
// 输入框变红请给输入框（或 el-input）加 class "ff-input-bad"，样式见下方全局部分。
import { computed, inject } from 'vue'
import FIcon from '../icon/FIcon.vue'
import { resolveError } from '../../errors/errorMessages'

const props = defineProps<{ code: string; message?: string; id?: string; description?: string; bare?: boolean }>()
// 放在表单项（LiveField）里时自动取它提供的错误行 id，供输入框 aria-describedby 关联
const fieldErrorId = inject<string | undefined>('ff-field-error-id', undefined)
const errorId = computed(() => props.id ?? fieldErrorId)
const resolved = computed(() => resolveError(props.code, props.message))
</script>

<style scoped>
.ff-inline-error {
  display: flex;
  align-items: flex-start;
  gap: var(--ff-space-2);
  margin-top: 6px;
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-danger);
}
.ff-inline-error > svg {
  margin-top: 1px;
}
</style>

<style>
/* 输入框错误态：红色边框加 3px 淡红外圈，对应原型的 .input.bad。
   原生输入框直接加 class；Element Plus 的 el-input / el-select 走 wrapper。 */
.ff-input-bad {
  border-color: var(--ff-danger) !important;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ff-danger) 14%, transparent);
}
.ff-input-bad .el-input__wrapper,
.ff-input-bad .el-select__wrapper,
.ff-input-bad .el-textarea__inner {
  box-shadow:
    0 0 0 1px var(--ff-danger) inset,
    0 0 0 3px color-mix(in srgb, var(--ff-danger) 14%, transparent) !important;
}
</style>
