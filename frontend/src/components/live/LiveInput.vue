<template>
  <div class="input" :class="{ 'ff-input-bad': bad, disabled }">
    <input
      v-model="model"
      :type="secret && !revealed ? 'password' : 'text'"
      :placeholder="placeholder"
      :disabled="disabled"
      :aria-invalid="bad || undefined"
      spellcheck="false"
      autocomplete="off"
      @keyup.enter="emit('enter')"
    />
    <button v-if="secret" type="button" class="ic" :title="revealed ? '隐藏' : '显示'" @click="revealed = !revealed">
      <FIcon :name="revealed ? 'eyeoff' : 'eye'" :size="14" />
    </button>
    <button v-else-if="copyable" type="button" class="ic" title="复制" :disabled="!model" @click="copy">
      <FIcon name="copy" :size="14" />
    </button>
  </div>
</template>

<script setup lang="ts">
// 原型的 .input：28px 高、12px 等宽字、行尾一个 14px 图标（复制 / 显示推流码）。
// 红色错误态复用 InlineError 文件里的全局类 ff-input-bad。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '../icon/FIcon.vue'

const model = defineModel<string>({ default: '' })
withDefaults(defineProps<{ placeholder?: string; secret?: boolean; copyable?: boolean; bad?: boolean; disabled?: boolean }>(), {})
const emit = defineEmits<{ enter: [] }>()
const revealed = ref(false)

async function copy() {
  try {
    await navigator.clipboard.writeText(model.value)
    ElMessage.success('已复制')
  } catch {
    ElMessage.warning('复制失败')
  }
}
</script>

<style scoped>
.input {
  height: 28px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  gap: 8px;
  background: var(--ff-bg-surface);
  overflow: hidden;
  font-size: 12px;
  transition: border-color var(--ff-dur-fast) var(--ff-ease), box-shadow var(--ff-dur-fast) var(--ff-ease);
}
.input:focus-within:not(.ff-input-bad) {
  border-color: var(--ff-primary);
}
.input.disabled {
  opacity: 0.6;
}
input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  padding: 0;
  background: none;
  font-family: var(--ff-font-mono);
  font-size: 12px;
  color: var(--ff-text-1);
  text-overflow: ellipsis;
}
input::placeholder {
  color: var(--ff-text-3);
}
.ic {
  border: 0;
  background: none;
  padding: 0;
  color: var(--ff-text-3);
  display: grid;
  place-items: center;
  cursor: pointer;
  flex: none;
}
.ic:hover:not(:disabled) {
  color: var(--ff-text-1);
}
.ic:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
</style>
