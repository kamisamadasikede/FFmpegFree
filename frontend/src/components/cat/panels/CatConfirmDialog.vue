<template>
  <Teleport to="body">
    <Transition name="cf">
      <div v-if="open" class="cf-mask" data-testid="cat-confirm" @mousedown.self="!busy && emit('cancel')">
        <div
          class="cf"
          role="alertdialog"
          aria-modal="true"
          :aria-labelledby="id + '-t'"
          :aria-describedby="id + '-b'"
          :aria-busy="busy"
          @keydown.esc.stop="!busy && emit('cancel')"
        >
          <div class="cf-ic" :class="tone"><FIcon :name="tone === 'danger' ? 'trash' : 'alert'" :size="18" /></div>
          <h3 :id="id + '-t'">{{ title }}</h3>
          <p :id="id + '-b'">{{ body }}</p>
          <div class="cf-btns">
            <button ref="cancelBtn" type="button" class="cf-b" :disabled="busy" @click="emit('cancel')">{{ cancelText }}</button>
            <button type="button" class="cf-b" :class="tone" :disabled="busy" data-testid="cat-confirm-ok" @click="emit('ok')">
              <i v-if="busy" class="cf-spin" aria-hidden="true" />{{ okText }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'

/** 二次确认框：「取消」默认焦点；tone=danger 红色主按钮，warn 橙色 */
const props = defineProps<{ open: boolean; title: string; body: string; okText: string; cancelText: string; tone: 'danger' | 'warn'; busy?: boolean }>()
const emit = defineEmits<{ ok: []; cancel: [] }>()
const id = `cf-${Math.random().toString(36).slice(2, 8)}`
const cancelBtn = ref<HTMLButtonElement>()
watch(
  () => props.open,
  async (v) => {
    if (!v) return
    await nextTick()
    cancelBtn.value?.focus()
  },
  { immediate: true },
)
</script>

<style scoped>
.cf-mask {
  position: fixed;
  inset: 0;
  z-index: 3200;
  display: grid;
  place-items: center;
  background: rgba(0, 0, 0, 0.32);
}
html.dark .cf-mask {
  background: rgba(0, 0, 0, 0.55);
}
.cf {
  width: 420px;
  max-width: calc(100vw - 32px);
  padding: 20px 20px 16px;
  border-radius: 12px;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  box-shadow: var(--ff-shadow-dialog);
}
html.dark .cf {
  background: var(--ff-bg-elevated);
}
.cf-ic {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  margin-bottom: 12px;
}
.cf-ic.danger {
  color: var(--ff-danger);
  background: color-mix(in srgb, var(--ff-danger) 12%, transparent);
}
.cf-ic.warn {
  color: var(--ff-warning-text);
  background: var(--ff-warning-soft);
}
.cf h3 {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 600;
  color: var(--ff-text-1);
}
.cf p {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--ff-text-2);
}
.cf-btns {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 18px;
}
.cf-b {
  height: 32px;
  padding: 0 14px;
  border-radius: 7px;
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font-size: 13px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: background-color 120ms var(--ff-ease), border-color 120ms var(--ff-ease);
}
.cf-b:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.cf-b:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.cf-b.danger {
  background: var(--ff-danger);
  border-color: var(--ff-danger);
  color: var(--ff-on-danger);
}
.cf-b.warn {
  background: var(--ff-warning);
  border-color: var(--ff-warning);
  color: #fff;
}
.cf-b.danger:hover:not(:disabled),
.cf-b.warn:hover:not(:disabled) {
  filter: brightness(1.06);
  background: revert-layer;
}
.cf-b:disabled {
  opacity: 0.6;
  cursor: default;
}
.cf-spin {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid currentColor;
  border-right-color: transparent;
  animation: cf-rot 800ms linear infinite;
}
@keyframes cf-rot {
  to {
    transform: rotate(360deg);
  }
}
.cf-enter-active,
.cf-leave-active {
  transition: opacity 200ms var(--ff-ease);
}
.cf-enter-active .cf,
.cf-leave-active .cf {
  transition: transform 200ms var(--ff-ease), opacity 200ms var(--ff-ease);
}
.cf-enter-from,
.cf-leave-to {
  opacity: 0;
}
.cf-enter-from .cf,
.cf-leave-to .cf {
  transform: translateY(6px) scale(0.98);
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .cf-enter-active,
  .cf-leave-active,
  .cf-enter-active .cf,
  .cf-leave-active .cf {
    transition: none;
  }
  .cf-spin {
    animation-duration: 2s;
  }
}
</style>
