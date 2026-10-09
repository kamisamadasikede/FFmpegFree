<template>
  <div v-if="project" class="pd-mask" data-testid="cat-project-delete" @mousedown.self="cancel">
    <div
      ref="box"
      class="pd"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="pd-title"
      aria-describedby="pd-body"
      @keydown="onKey"
    >
      <div class="pd-ic"><FIcon name="trash" :size="18" /></div>
      <h3 id="pd-title">{{ PC.delTitle(project.name) }}</h3>
      <p id="pd-body">{{ PC.delBody }}</p>
      <div class="pd-btns">
        <button ref="cancelBtn" type="button" class="pd-b" :disabled="busy" @click="cancel">{{ PC.cancel }}</button>
        <button type="button" class="pd-b danger" :disabled="busy" data-testid="cat-project-delete-ok" @click="confirm">{{ PC.del }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { CAT_PROJECT_COPY as PC } from '@/api/catProjects'
import { catState, deleteProject } from '@/views/cat/catState'

/**
 * 删除项目确认（设计「项目状态 v0.1」§04）：420 宽；默认焦点在「取消」（防误删）；Tab 在两个按钮间循环；
 * Esc / 点遮罩 = 取消；「删除项目」红色实心在最右。确认后只删应用里的记录和对话，不动文件夹。
 */
const project = computed(() => catState.projects.find((p) => p.id === catState.deleting))
const box = ref<HTMLElement>()
const cancelBtn = ref<HTMLButtonElement>()
const busy = ref(false)
let opener: HTMLElement | null = null

watch(project, async (p, old) => {
  if (p && !old) {
    opener = document.activeElement as HTMLElement | null
    await nextTick()
    cancelBtn.value?.focus()
  }
})

function close() {
  catState.deleting = ''
  busy.value = false
}
function cancel() {
  if (busy.value) return
  const id = project.value?.id
  close()
  nextTick(() => {
    const more = id ? document.querySelector<HTMLElement>(`.pj[data-pid="${CSS.escape(id)}"] [data-testid="cat-project-more"]`) : null
    ;(more ?? opener)?.focus()
  })
}
async function confirm() {
  const p = project.value
  if (!p || busy.value) return
  busy.value = true
  const ok = await deleteProject(p.id)
  close()
  if (ok) nextTick(() => document.querySelector<HTMLElement>('.ct-it.new')?.focus())
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    cancel()
  } else if (e.key === 'Tab') {
    const btns = Array.from(box.value?.querySelectorAll<HTMLButtonElement>('.pd-b') ?? [])
    if (!btns.length) return
    e.preventDefault()
    const i = btns.indexOf(document.activeElement as HTMLButtonElement)
    btns[(i + (e.shiftKey ? -1 : 1) + btns.length) % btns.length].focus()
  }
}
</script>

<style scoped>
.pd-mask {
  position: fixed;
  inset: 0;
  z-index: 3100;
  display: grid;
  place-items: center;
  background: rgba(0, 0, 0, 0.32);
  animation: pd-fade 150ms ease both;
}
html.dark .pd-mask {
  background: rgba(0, 0, 0, 0.55);
}
.pd {
  width: 420px;
  max-width: calc(100vw - 32px);
  padding: 20px 20px 16px;
  border-radius: 12px;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.18);
  animation: pd-pop 160ms ease both;
}
html.dark .pd {
  background: var(--ff-bg-elevated);
}
.pd-ic {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  color: var(--ff-danger);
  background: color-mix(in srgb, var(--ff-danger) 12%, transparent);
  margin-bottom: 12px;
}
.pd h3 {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 600;
  color: var(--ff-text-1);
  word-break: break-all;
}
.pd p {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--ff-text-2);
}
.pd-btns {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 18px;
}
.pd-b {
  height: 32px;
  padding: 0 14px;
  border-radius: 7px;
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.pd-b:hover {
  background: var(--ff-bg-hover);
}
.pd-b.danger {
  border-color: var(--ff-danger);
  background: var(--ff-danger);
  color: #fff;
  font-weight: 500;
}
.pd-b.danger:hover {
  filter: brightness(0.94);
}
.pd-b:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.pd-b:disabled {
  opacity: 0.6;
  cursor: default;
}
@keyframes pd-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}
@keyframes pd-pop {
  from { opacity: 0; transform: scale(0.96); }
  to { opacity: 1; transform: none; }
}
@media (prefers-reduced-motion: reduce) {
  .pd-mask,
  .pd {
    animation: none;
  }
}
</style>
