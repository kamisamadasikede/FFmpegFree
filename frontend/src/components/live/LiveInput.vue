<template>
  <div class="input" :class="{ 'ff-input-bad': bad, disabled, ro: readonly }">
    <input
      :id="inputId"
      :value="shown"
      :aria-describedby="bad ? errorId : undefined"
      :type="secret ? 'password' : 'text'"
      :class="{ secret }"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      :aria-invalid="bad || undefined"
      spellcheck="false"
      autocomplete="off"
      @input="onInput"
      @focus="focused = true"
      @keyup.enter="emit('enter')"
      @blur="onBlur"
    />
    <!-- 推流地址（mask-key）：地址里的推流码 / 口令默认显示成圆点，行尾眼睛按钮切换；聚焦编辑时显示完整地址。只遮显示，值不变 -->
    <button
      v-if="maskKey && !secret && secretPart"
      type="button"
      class="eye"
      :aria-label="revealed ? '隐藏推流码' : '显示推流码'"
      :aria-pressed="revealed"
      :title="revealed ? '隐藏推流码' : '显示推流码'"
      @mousedown.prevent
      @click="revealed = !revealed"
    >
      <FIcon :name="revealed ? 'eyeoff' : 'eye'" :size="14" />
    </button>
    <!-- 推流码 / 口令：始终是密码框，行尾锁图标，没有“显示明文”按钮（设计稿 v0.2 §2.1） -->
    <FIcon v-if="secret" class="lock" name="lock" :size="14" />
  </div>
</template>

<script setup lang="ts">
// 设计稿 v0.2 的输入框：28px 高；地址类 12px 等宽，口令类 13px 密码框 + 锁图标；占位 13px --ff-text-3。
// 红色错误态复用 InlineError 文件里的全局类 ff-input-bad。
// mask-key（推流地址，包 20）：恢复的地址里常带推流码，录屏 / 共享屏幕时不能露出来。默认把推流码部分显示成圆点（maskPushUrlSecret），
// 眼睛按钮切换；聚焦编辑时显示完整地址，失焦再遮上。“显示”只在本次挂载内有效：切换菜单 / 页签、重启后重新遮挡。
import { computed, inject, onDeactivated, ref } from 'vue'
import FIcon from '../icon/FIcon.vue'
import { hasPushUrlSecret, maskPushUrlSecret } from '@/utils/liveUrl'

const model = defineModel<string>({ default: '' })
const props = withDefaults(defineProps<{ placeholder?: string; secret?: boolean; maskKey?: boolean; bad?: boolean; disabled?: boolean; readonly?: boolean }>(), {})
const emit = defineEmits<{ enter: []; blur: [] }>()
const focused = ref(false)
const revealed = ref(false)
const secretPart = computed(() => !!props.maskKey && hasPushUrlSecret(model.value))
const shown = computed(() => (secretPart.value && !revealed.value && !focused.value ? maskPushUrlSecret(model.value) : model.value))
function onInput(e: Event) {
  // 遮挡状态下不会有输入（聚焦即显示完整地址），这里写回的一定是完整值
  if (shown.value === model.value || focused.value) model.value = (e.target as HTMLInputElement).value
}
function onBlur() {
  focused.value = false
  emit('blur')
}
onDeactivated(() => (revealed.value = false))
// 由外层 LiveField 提供：label[for] 指向输入框，错误行通过 aria-describedby 关联
const inputId = inject<string | undefined>('ff-field-input-id', undefined)
const errorId = inject<string | undefined>('ff-field-error-id', undefined)
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
  transition: border-color var(--ff-dur-fast) var(--ff-ease), box-shadow var(--ff-dur-fast) var(--ff-ease);
}
.input:focus-within:not(.ff-input-bad) {
  border-color: var(--ff-primary);
}
/* 错误态聚焦：ff-input-bad 的红边保留，另加 2px 红色聚焦环（间隔 2px，避免和边框糊在一起） */
.input.ff-input-bad:focus-within {
  outline: 2px solid var(--ff-danger);
  outline-offset: 2px;
  box-shadow: none !important;
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
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-1);
  text-overflow: ellipsis;
}
input.secret {
  font-family: var(--ff-font);
  font-size: var(--ff-fs-sm);
}
input::placeholder {
  font-family: var(--ff-font);
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-3);
}
.lock {
  color: var(--ff-text-2);
}
.eye {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  margin-right: -4px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: none;
  color: var(--ff-text-2);
  cursor: pointer;
}
.eye:hover {
  color: var(--ff-text-1);
  background: var(--ff-bg-hover);
}
.eye:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 1px;
}
</style>
