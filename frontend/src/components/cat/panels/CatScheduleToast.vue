<template>
  <Transition name="ts">
    <button
      v-if="catPanels.toast"
      :key="catPanels.toast.key"
      type="button"
      class="ts"
      :class="catPanels.toast.tone"
      role="status"
      data-testid="cat-schedule-toast"
      @click="open"
    >
      <FIcon :name="catPanels.toast.tone === 'ok' ? 'check' : 'warn'" :size="16" class="ic" />
      <span class="tx"><b>{{ catPanels.toast.title }}</b><span>{{ catPanels.toast.body }}</span></span>
      <span class="x" role="button" :aria-label="CAT_SCHED_COPY.close" @click.stop="dismissToast"><FIcon name="x" :size="14" /></span>
    </button>
  </Transition>
</template>

<script setup lang="ts">
import FIcon from '@/components/icon/FIcon.vue'
import { CAT_SCHED_COPY } from '@/api/catSchedules'
import { catPanels, dismissToast } from '@/views/cat/catPanels'
import { findConv, notify, selectConv } from '@/views/cat/catState'

/** 窗口在前台时的定时任务完成 / 失败提示（契约 v0.35.3：只有这两种）；点了打开对话 */
function open() {
  const id = catPanels.toast?.convId
  dismissToast()
  if (!id) return
  if (!findConv(id)) return notify(CAT_SCHED_COPY.convGone, 'warn')
  selectConv(id)
}
</script>

<style scoped>
.ts {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 3300;
  width: 340px;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 12px 12px 14px;
  border-radius: var(--cat-radius-lg);
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated);
  box-shadow: var(--cat-shadow-lg);
  color: var(--cat-text);
  text-align: left;
  cursor: pointer;
}
.ts:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.ic {
  flex: none;
  margin-top: 1px;
}
.ok .ic {
  color: var(--cat-success);
}
.bad .ic {
  color: var(--cat-danger);
}
.tx {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tx b {
  font-size: 13px;
}
.tx span {
  font-size: 12px;
  color: var(--cat-text-2);
}
.x {
  color: var(--cat-muted);
  padding: 2px;
  border-radius: 4px;
}
.x:hover {
  background: var(--cat-hover);
}
.ts-enter-active,
.ts-leave-active {
  transition: opacity 200ms var(--ff-ease), transform 200ms var(--ff-ease);
}
.ts-enter-from,
.ts-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
@media (prefers-reduced-motion: reduce) {
  .ts-enter-active,
  .ts-leave-active {
    transition: none;
  }
}
</style>
