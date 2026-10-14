<template>
  <div ref="root" class="cms" :class="{ open }">
    <button
      type="button"
      class="cms-btn"
      :class="{ danger: cur?.risky }"
      :disabled="disabled"
      :aria-label="ariaLabel"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :data-testid="testid"
      @click="toggle"
    >
      <svg v-if="cur?.risky" class="tri" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 1.2 11.2 10.5H.8Z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" /><path d="M6 4.6v2.6M6 8.7v.1" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>
      <span>{{ cur?.label || '—' }}</span>
      <FIcon name="down" :size="11" class="caret" />
    </button>
    <div v-if="open" class="cms-menu" :class="{ up }" role="listbox" :aria-label="ariaLabel" :data-testid="testid + '-menu'" @click.stop>
      <button
        v-for="o in options"
        :key="o.id"
        type="button"
        role="option"
        class="cms-it"
        :class="{ on: o.id === modelValue, danger: o.risky }"
        :aria-selected="o.id === modelValue"
        :data-testid="testid + '-opt-' + o.id"
        @click="pick(o.id)"
      >
        <span class="tx">
          <b><svg v-if="o.risky" class="tri" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 1.2 11.2 10.5H.8Z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" /><path d="M6 4.6v2.6M6 8.7v.1" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>{{ o.label }}<em v-if="o.id === defaultId" class="def">默认</em></b>
          <small v-if="o.desc">{{ o.desc }}</small>
        </span>
        <FIcon v-if="o.id === modelValue" name="check" :size="14" class="ok" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
// 设置页「默认模式」下拉（契约 v0.44.4 §6.19.18.9 第 5 条）：选项样式同输入框模式菜单（名字 + 灰色说明；danger 橙色 + 三角）。
// 这里选 danger 不弹确认框（确认只在对话内按项目弹）。
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import type { CatOption } from '@/views/cat/catCapabilities'

const props = defineProps<{ modelValue: string; options: CatOption[]; defaultId: string; disabled?: boolean; ariaLabel?: string; testid: string }>()
const emit = defineEmits<{ 'update:modelValue': [id: string] }>()
const open = ref(false)
const up = ref(false)
const root = ref<HTMLElement | null>(null)
const cur = computed(() => props.options.find((o) => o.id === props.modelValue))

function onDoc(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) close()
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') close()
}
function close() {
  open.value = false
  document.removeEventListener('mousedown', onDoc)
  document.removeEventListener('keydown', onKey)
}
async function toggle() {
  if (open.value) return close()
  const r = root.value?.getBoundingClientRect()
  // 哪边空间够往哪边弹（同输入框菜单规则）
  up.value = !!r && window.innerHeight - r.bottom < 300 && r.top > window.innerHeight - r.bottom
  open.value = true
  await nextTick()
  document.addEventListener('mousedown', onDoc)
  document.addEventListener('keydown', onKey)
}
function pick(id: string) {
  close()
  if (id !== props.modelValue) emit('update:modelValue', id)
}
onBeforeUnmount(close)
</script>

<style scoped>
.cms { position: relative; }
.cms-btn {
  display: inline-flex; align-items: center; gap: 6px; height: 30px; min-width: 140px; padding: 0 10px;
  border: 1px solid var(--cat-border); border-radius: 8px; background: var(--cat-bg); color: var(--cat-text);
  font: inherit; font-size: 13px; cursor: pointer;
}
.cms-btn span { flex: 1; text-align: left; }
.cms-btn:hover:not(:disabled) { background: var(--cat-hover); }
.cms-btn:disabled { opacity: 0.5; cursor: default; }
.cms-btn.danger { color: var(--cat-orange); border-color: var(--cat-orange-line); }
.caret { color: var(--cat-muted); }
.tri { width: 12px; height: 12px; flex: none; margin-right: 4px; vertical-align: -1px; }
.cms-menu {
  position: absolute; right: 0; top: calc(100% + 4px); z-index: 30; width: 240px; max-height: min(360px, 70vh); overflow-y: auto;
  padding: 4px; border: 1px solid var(--cat-border); border-radius: 12px; background: var(--cat-elevated);
  box-shadow: var(--cat-shadow-lg);
}
.cms-menu.up { top: auto; bottom: calc(100% + 4px); }
.cms-it {
  display: flex; align-items: flex-start; gap: 8px; width: 100%; padding: 7px 8px; border: 0; border-radius: 7px;
  background: none; color: var(--cat-text); font: inherit; text-align: left; cursor: pointer;
}
.cms-it:hover, .cms-it.on { background: var(--cat-hover); }
.cms-it .tx { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.cms-it b { font-size: 13px; font-weight: 500; display: flex; align-items: center; }
.cms-it small { font-size: 12px; line-height: 1.4; color: var(--cat-muted); white-space: normal; }
.cms-it .def { margin-left: 6px; font-style: normal; font-size: 11px; font-weight: 400; color: var(--cat-muted); }
.cms-it.danger b, .cms-it.danger .ok { color: var(--cat-orange); }
.cms-it.danger:hover, .cms-it.danger.on { background: var(--cat-orange-soft); }
.cms-it .ok { margin-top: 2px; color: var(--cat-accent, currentColor); }
</style>
