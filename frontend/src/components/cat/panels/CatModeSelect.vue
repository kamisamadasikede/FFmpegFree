<template>
  <div ref="root" class="cms" :class="{ open, compact }">
    <button
      ref="btn"
      type="button"
      class="cms-btn"
      :class="{ danger: cur?.risky, warn }"
      :disabled="disabled"
      :aria-label="ariaLabel"
      :aria-busy="busy || undefined"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :data-testid="testid"
      :data-value="modelValue"
      @click="toggle"
      @keydown="onBtnKey"
    >
      <svg v-if="cur?.risky" class="tri" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 1.2 11.2 10.5H.8Z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" /><path d="M6 4.6v2.6M6 8.7v.1" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>
      <span :title="compact ? cur?.label || undefined : undefined">{{ cur?.label || placeholder || '—' }}</span>
      <FIcon name="down" :size="12" class="caret" />
    </button>
    <Teleport to="body">
      <div
        v-if="open"
        ref="menu"
        class="cms-menu"
        :class="{ up, compact }"
        :style="menuStyle"
        role="listbox"
        :aria-label="ariaLabel"
        :data-testid="testid + '-menu'"
        @click.stop
        @mousedown.stop
      >
        <template v-for="(o, i) in options" :key="o.id">
          <div v-if="o.sep" class="cms-sep" role="separator" />
          <div v-else-if="o.header" class="cms-hd" :class="{ divided: o.divided }" role="presentation" :data-testid="testid + '-hd-' + o.id">{{ o.label }}</div>
          <button
            v-else
            type="button"
            role="option"
            class="cms-it"
            :class="{ on: o.id === modelValue, danger: o.risky, act: i === active, desc: !!o.desc, divided: o.divided }"
            :disabled="o.disabled"
            :aria-selected="o.id === modelValue"
            :data-testid="testid + '-opt-' + o.id"
            @mouseenter="active = i"
            @click="pick(o.id)"
          >
            <span class="tx" :title="compact ? o.label : undefined">
              <b><svg v-if="o.risky" class="tri" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 1.2 11.2 10.5H.8Z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" /><path d="M6 4.6v2.6M6 8.7v.1" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>{{ o.label }}<em v-if="defaultId !== undefined && o.id === defaultId" class="def">默认</em></b>
              <small v-if="o.desc">{{ o.desc }}</small>
            </span>
            <FIcon v-if="o.id === modelValue" name="check" :size="14" class="ok" />
          </button>
        </template>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
// Cat 设置页统一下拉（默认模式 / 默认模型 / 思考强度 / 协议 / 默认位置）。
// 菜单 Teleport 到 body + position:fixed（不被卡片 overflow 裁掉）；下方放不下就往上翻；滚动 / 缩放时重新定位；点外面或 Esc 关闭。
// 键盘：↑↓ 移动，Enter / 空格 选中，Esc 关闭。
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'

export interface CatSelectOption {
  id: string
  label: string
  desc?: string
  risky?: boolean
  disabled?: boolean
  /** 只画一条分隔线，不可选 */
  sep?: boolean
  /** 上方画 1px 分隔线 */
  divided?: boolean
  /** 分组小标题：不可点、不参与键盘移动（底栏「远程分支」） */
  header?: boolean
}

const props = defineProps<{
  modelValue: string
  options: CatSelectOption[]
  defaultId?: string
  disabled?: boolean
  busy?: boolean
  warn?: boolean
  placeholder?: string
  ariaLabel?: string
  testid: string
  /** 输入框底栏用：无边框、小字号的文字触发器（v0.49） */
  compact?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [id: string] }>()
const open = ref(false)
const up = ref(false)
const active = ref(-1)
const root = ref<HTMLElement | null>(null)
const btn = ref<HTMLButtonElement | null>(null)
const menu = ref<HTMLElement | null>(null)
const pos = ref({ left: 0, top: 0, bottom: 0, minWidth: 0, maxHeight: 360 })
const cur = computed(() => props.options.find((o) => o.id === props.modelValue && !o.sep))

const GAP = 4
const MENU_MAX = 360
/** 紧凑（底栏）：选项 28px，超过 8 项滚动 → 8×28 + 上下 padding 8 + 分组标题余量 */
const COMPACT_MAX = 8 * 28 + 8
const menuStyle = computed(() => {
  const p = pos.value
  const s: Record<string, string> = { left: p.left + 'px', minWidth: p.minWidth + 'px', maxHeight: p.maxHeight + 'px' }
  if (up.value) s.bottom = p.bottom + 'px'
  else s.top = p.top + 'px'
  return s
})

function place() {
  const r = btn.value?.getBoundingClientRect()
  if (!r) return
  const vh = window.innerHeight
  const vw = window.innerWidth
  const below = vh - r.bottom - GAP - 8
  const above = r.top - GAP - 8
  const cap = props.compact ? COMPACT_MAX + (props.options.some((o) => o.header) ? 26 : 0) : MENU_MAX
  const need = Math.min(cap, menu.value?.scrollHeight ?? cap)
  // 紧凑（底栏）默认往上弹，上面放不下才往下
  up.value = props.compact ? !(above < need && below > above) : below < need && above > below
  const minWidth = Math.max(r.width, props.compact ? 200 : 160)
  const w = Math.max(minWidth, menu.value?.offsetWidth ?? minWidth)
  // 左对齐触发器，右边不出屏
  const left = Math.max(8, Math.min(r.left, vw - w - 8))
  pos.value = { left, top: r.bottom + GAP, bottom: vh - r.top + GAP, minWidth, maxHeight: Math.max(120, Math.min(cap, up.value ? above : below)) }
}
function onDoc(e: MouseEvent) {
  const t = e.target as Node
  if (root.value?.contains(t) || menu.value?.contains(t)) return
  close()
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    close()
    btn.value?.focus()
  }
}
function selectable(i: number) {
  const o = props.options[i]
  return !!o && !o.sep && !o.header && !o.disabled
}
function move(d: number) {
  const n = props.options.length
  if (!n) return
  let i = active.value
  for (let k = 0; k < n; k++) {
    i = (i + d + n) % n
    if (selectable(i)) break
  }
  active.value = i
  nextTick(() => menu.value?.querySelectorAll<HTMLElement>('[role=option]')[optIndex(i)]?.scrollIntoView({ block: 'nearest' }))
}
function optIndex(i: number) {
  return props.options.slice(0, i).filter((o) => !o.sep && !o.header).length
}
function onBtnKey(e: KeyboardEvent) {
  if (props.disabled) return
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    if (!open.value) void openMenu()
    else move(e.key === 'ArrowDown' ? 1 : -1)
  } else if ((e.key === 'Enter' || e.key === ' ') && open.value) {
    e.preventDefault()
    if (selectable(active.value)) pick(props.options[active.value].id)
  } else if (e.key === 'Tab' && open.value) close()
}
function onReflow() {
  if (open.value) place()
}
function close() {
  if (!open.value) return
  open.value = false
  document.removeEventListener('mousedown', onDoc, true)
  document.removeEventListener('keydown', onKey, true)
  window.removeEventListener('scroll', onReflow, true)
  window.removeEventListener('resize', onReflow)
}
async function openMenu() {
  active.value = props.options.findIndex((o) => o.id === props.modelValue && !o.sep)
  place()
  open.value = true
  await nextTick()
  place() // 量到菜单真实高度后再定一次方向
  document.addEventListener('mousedown', onDoc, true)
  document.addEventListener('keydown', onKey, true)
  window.addEventListener('scroll', onReflow, true)
  window.addEventListener('resize', onReflow)
}
function toggle() {
  if (open.value) close()
  else void openMenu()
}
function pick(id: string) {
  close()
  btn.value?.focus()
  if (id !== props.modelValue) emit('update:modelValue', id)
}
onBeforeUnmount(close)
</script>

<style scoped>
.cms { position: relative; display: inline-flex; }
.cms.compact { min-width: 0; }
.cms.compact .cms-btn { height: auto; min-height: 0; padding: 0 2px; gap: 3px; border: 0; background: none; box-shadow: none; font-size: 12px; color: inherit; border-radius: 4px; max-width: none; }
.cms.compact .cms-btn span { max-width: 160px; }
.cms.compact .cms-btn:hover:not(:disabled) { color: var(--cat-text); background: var(--cat-hover); }
.cms.compact.open .cms-btn, .cms.compact .cms-btn:focus-visible { box-shadow: var(--cat-focus); }
.cms-btn {
  display: inline-flex; align-items: center; gap: 6px; height: 32px; min-width: 140px; max-width: 260px; padding: 0 10px;
  border: 1px solid var(--ff-border); border-radius: 6px; background: var(--cat-bg); color: var(--cat-text);
  font: inherit; font-size: 13px; cursor: pointer;
  transition: border-color 120ms var(--ff-ease), box-shadow 120ms var(--ff-ease);
}
.cms-btn span { flex: 1; min-width: 0; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cms-btn:hover:not(:disabled) { border-color: var(--cat-border-strong); }
.cms-btn:focus-visible, .open .cms-btn { outline: none; border-color: var(--cat-accent); box-shadow: var(--cat-focus); }
.cms-btn:disabled { opacity: 0.55; cursor: default; }
.cms-btn.danger { color: var(--cat-orange); border-color: var(--cat-orange-line); }
.cms-btn.warn { color: var(--cat-orange); border-color: var(--cat-orange-line); background: var(--cat-orange-soft); }
.caret { flex: none; color: var(--cat-muted); transition: transform 120ms var(--ff-ease); }
.open .caret { transform: rotate(180deg); }
.tri { width: 12px; height: 12px; flex: none; margin-right: 4px; vertical-align: -1px; }
.cms-menu {
  position: fixed; z-index: 3000; width: max-content; max-width: 320px; overflow-y: auto;
  padding: 4px; border: 1px solid var(--ff-border); border-radius: 8px; background: var(--ff-bg-elevated);
  box-shadow: var(--ff-shadow-dialog); box-sizing: border-box;
}
.cms-sep { height: 1px; margin: 4px 2px; background: var(--ff-border); }
.cms-it {
  display: flex; align-items: center; gap: 8px; width: 100%; min-height: 32px; padding: 0 8px; border: 0; border-radius: 6px;
  background: none; color: var(--cat-text); font: inherit; text-align: left; cursor: pointer; box-sizing: border-box;
}
.cms-it.desc { padding-top: 6px; padding-bottom: 6px; align-items: flex-start; }
.cms-it.divided { position: relative; margin-top: 5px; }
.cms-it.divided::before { content: ''; position: absolute; left: 2px; right: 2px; top: -3px; height: 1px; background: var(--ff-border); }
.cms-it:hover:not(:disabled), .cms-it.act:not(:disabled) { background: var(--cat-hover); }
.cms-it:disabled { opacity: 0.5; cursor: default; }
.cms-it .tx { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.cms-it b { font-size: 13px; font-weight: 400; display: flex; align-items: center; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.cms-it.on b { font-weight: 500; }
.cms-it small { font-size: 12px; line-height: 1.4; color: var(--cat-muted); white-space: normal; }
.cms-it .def { margin-left: 6px; font-style: normal; font-size: 11px; font-weight: 400; color: var(--cat-muted); }
/* 紧凑（输入框底栏分支下拉，设计规格 v0.49）：选项 28px / 12px，最小宽 200 */
.cms-menu.compact { max-width: 360px; }
.cms-menu.compact .cms-it { min-height: 28px; height: 28px; }
.cms-menu.compact .cms-it b { font-size: 12px; max-width: 300px; display: block; }
.cms-menu.compact .cms-it .ok { width: 12px; height: 12px; }
.cms-hd { margin: 6px 8px 2px; font-size: 11px; line-height: 16px; color: var(--cat-muted); cursor: default; user-select: none; }
.cms-hd.divided { position: relative; margin-top: 11px; }
.cms-hd.divided::before { content: ''; position: absolute; left: -6px; right: -6px; top: -6px; height: 1px; background: var(--ff-border); }
.cms-it.danger b, .cms-it.danger .ok { color: var(--cat-orange); }
.cms-it.danger:hover, .cms-it.danger.act { background: var(--cat-orange-soft); }
.cms-it .ok { flex: none; margin-left: auto; color: var(--cat-accent, currentColor); }
.cms-it.desc .ok { margin-top: 2px; }
</style>
