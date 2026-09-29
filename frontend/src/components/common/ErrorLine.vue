<template>
  <div class="ff-error-line" role="alert">
    <FIcon name="warn" :size="16" />
    <div>
      <b>{{ resolved.title }}</b>{{ resolved.description }}
      <a v-if="showLog" href="#" @click.prevent="emit('viewLog')">查看日志</a><br />
      <span class="code">{{ resolved.code }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
// 任务中心失败行下方的错误说明，样式来自 proto/errors.html 的 .errline。
import { computed } from 'vue'
import FIcon from '../icon/FIcon.vue'
import { resolveError } from '../../errors/errorMessages'

const props = withDefaults(defineProps<{ code: string; message?: string; showLog?: boolean }>(), { showLog: true })
const emit = defineEmits<{ viewLog: [] }>()
const resolved = computed(() => resolveError(props.code, props.message))
</script>

<style scoped>
.ff-error-line {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px var(--ff-space-3);
  border-radius: 8px;
  background: color-mix(in srgb, var(--ff-danger) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--ff-danger) 28%, transparent);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.6;
}
.ff-error-line > svg {
  color: var(--ff-danger);
  margin-top: 1px;
}
b {
  display: block;
  color: var(--ff-text-1);
  font-weight: 500;
  font-size: var(--ff-fs-sm);
}
a {
  color: var(--ff-primary);
  cursor: pointer;
  text-decoration: underline;
}
.code {
  font-family: var(--ff-font-mono);
  font-size: 11px;
  color: var(--ff-text-2);
}
</style>
