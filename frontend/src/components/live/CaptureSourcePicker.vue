<template>
  <!-- Windows：分组下拉（设计说明 §4.1–4.6） -->
  <div v-if="mode === 'dropdown'" class="dd">
    <button
      :id="triggerId"
      ref="trigger"
      type="button"
      class="tg"
      :class="{ bad: invalid, ph: !current, open }"
      role="combobox"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="listId"
      :aria-labelledby="labelId ? `${labelId} ${triggerId}` : undefined"
      :aria-invalid="invalid || undefined"
      :aria-describedby="invalid ? errorId : undefined"
      :aria-busy="state === 'loading' || undefined"
      :title="current?.title"
      @click="toggle"
      @keydown="onTriggerKey"
    >
      <template v-if="current">
        <FIcon :name="current.kind === 'window' ? 'window' : 'monitor'" :size="14" />
        <span class="nm"><span class="hd">{{ split(current.title).head }}</span><span class="tl">{{ split(current.title).tail }}</span></span>
        <em v-if="gone" class="gn">{{ LIVE_SOURCE_GONE_TAG }}</em>
        <em v-else-if="current.width > 0 && current.height > 0">{{ current.width }}×{{ current.height }}</em>
      </template>
      <span v-else class="nm ph">{{ state === 'loading' ? LIVE_SOURCE_PLACEHOLDER_LOADING : LIVE_SOURCE_PLACEHOLDER }}</span>
      <FIcon name="down" :size="14" class="ar" />
    </button>
    <Teleport to="body">
      <div v-if="open" ref="pop" class="pop" :style="popStyle" @keydown="onPopKey" @mousedown.stop>
        <div v-if="state === 'failed' && !sources.length" class="fl" role="alert">
          <FIcon name="warn" :size="20" />
          <b>{{ LIVE_SOURCE_FAIL_TITLE }}</b>
          <span>{{ LIVE_SOURCE_FAIL_HINT }}</span>
          <LiveButton sm @click="emit('refresh')">{{ LIVE_SOURCE_RETRY }}</LiveButton>
        </div>
        <template v-else>
          <div :id="listId" ref="listEl" class="ls" role="listbox" :aria-labelledby="labelId" :aria-busy="state === 'loading' || undefined" @scroll="hover = ''">
            <template v-if="state === 'loading' && !sources.length">
              <div class="gh">{{ LIVE_SOURCE_GROUP_SCREEN }}</div>
              <div v-for="n in 2" :key="`s${n}`" class="sk" />
              <div class="gh">{{ LIVE_SOURCE_GROUP_WINDOW }}</div>
              <div v-for="n in 3" :key="`w${n}`" class="sk" />
            </template>
            <template v-else>
              <div v-for="g in groupsAll" :key="g.kind" role="group" :aria-labelledby="`${uid}-${g.kind}`">
                <div :id="`${uid}-${g.kind}`" class="gh">{{ g.title }}</div>
                <template v-if="g.items.length">
                  <div
                    v-for="s in g.items"
                    :id="`${uid}-o-${s.id}`"
                    :key="s.id"
                    class="op"
                    :class="{ on: s.id === modelValue && !gone, act: s.id === active }"
                    role="option"
                    :aria-selected="s.id === modelValue && !gone"
                    :title="tip(s)"
                    tabindex="-1"
                    @click="choose(s.id)"
                    @mouseenter="hoverOn(s, $event)"
                    @mouseleave="hoverOff"
                    @focus="hoverOn(s, $event)"
                    @blur="hoverOff"
                  >
                    <FIcon :name="s.kind === 'window' ? 'window' : 'monitor'" :size="14" />
                    <span class="nm"><span class="hd">{{ split(s.title).head }}</span><span class="tl">{{ split(s.title).tail }}</span></span>
                    <em v-if="s.width > 0 && s.height > 0">{{ s.width }}×{{ s.height }}</em>
                    <FIcon v-if="s.id === modelValue && !gone" name="check" :size="14" class="ck" />
                  </div>
                </template>
                <div v-else-if="g.kind === 'window'" class="none">
                  <b>{{ LIVE_SOURCE_NO_WINDOW_TITLE }}</b>
                  <span>{{ LIVE_SOURCE_NO_WINDOW_HINT }}</span>
                </div>
              </div>
            </template>
          </div>
          <div v-if="hover && tipItem" class="bub" :style="{ top: bubTop + 'px' }" role="tooltip">{{ tipItem.title }}</div>
        </template>
        <div class="ft">
          <span v-if="state === 'loading'" class="ftl"><i class="spin" aria-hidden="true" />{{ sources.length ? LIVE_SOURCE_REFRESHING : LIVE_SOURCE_PLACEHOLDER_LOADING }}</span>
          <span v-else-if="state === 'failed'" class="ftl bad" role="alert">{{ LIVE_SOURCE_STALE }}</span>
          <span v-else class="ftl">{{ liveSourceWindowCount(windowCount) }}</span>
          <button type="button" class="rf" :aria-label="LIVE_SOURCE_REFRESH_ARIA" :aria-disabled="state === 'loading' || undefined" @click="state !== 'loading' && emit('refresh')">
            <FIcon name="refresh" :size="12" />{{ state === 'failed' && sources.length ? LIVE_SOURCE_RETRY : LIVE_SOURCE_REFRESH_SHORT }}
          </button>
        </div>
      </div>
    </Teleport>
  </div>

  <!-- macOS / Linux：屏幕单选列表（v0.2，主屏名“屏幕 1（主显示器）”） -->
  <div v-else class="csp">
    <div class="bar">
      <LiveButton variant="text" sm icon="refresh" :disabled="state === 'loading'" @click="emit('refresh')">{{ LIVE_SOURCE_REFRESH }}</LiveButton>
    </div>
    <div v-if="state === 'loading'" class="st" role="status" aria-live="polite"><i class="spin" aria-hidden="true" />{{ LIVE_SOURCE_LOADING }}</div>
    <div v-else-if="state === 'failed'" class="st bad" role="alert">
      <FIcon name="warn" :size="14" /><span>{{ LIVE_SOURCE_FAILED }}</span>
      <LiveButton variant="text" sm @click="emit('refresh')">{{ LIVE_SOURCE_RETRY }}</LiveButton>
    </div>
    <div v-else-if="state === 'empty'" class="st"><FIcon name="info" :size="14" />{{ LIVE_SOURCE_EMPTY }}</div>
    <div v-else class="lst" role="radiogroup" :aria-labelledby="labelId" :aria-invalid="invalid || undefined" :aria-describedby="invalid ? errorId : undefined">
      <button
        v-for="s in screens"
        :key="s.id"
        type="button"
        class="so"
        :class="{ on: modelValue === s.id }"
        role="radio"
        :aria-checked="modelValue === s.id"
        :title="tip(s)"
        @click="emit('update:modelValue', s.id)"
      >
        <i class="rd" /><FIcon name="monitor" :size="14" /><span class="nm">{{ s.title }}</span><em v-if="s.width > 0 && s.height > 0">{{ s.width }}×{{ s.height }}</em>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
// 直播 v1.1 采集来源选择器（设计说明 §4）。
//   mode='dropdown'（Windows：平台是 windows 或列表里有窗口）：combobox + listbox 分组下拉（“屏幕”“应用窗口”两组），标题中间省略（尾部保留 10 个字符，title / 悬停气泡给全名），
//     底栏“共 n 个窗口”+“刷新”；首次加载骨架行；刷新中保留旧列表；空 / 失败 / 刷新失败（旧列表保留）三态；gone=true 时红边 + 尺寸位置换成红字“已不可用”。
//   mode='list'（macOS / Linux）：屏幕单选列表（v0.2），没有应用窗口分组也没有下拉。
// 键盘：↑↓ 移动、Home / End、Enter / Space 选择、Esc 关闭并把焦点还给触发器。弹层 Teleport 到 body（避开表单面板的 overflow），滚动 / 改变窗口大小时关闭。窗口标题可能含隐私，只在界面里显示，不写日志。
import { computed, inject, nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import LiveButton from './LiveButton.vue'
import type { CaptureSource } from '@/api/live'
import { splitSourceTitle } from '@/utils/liveSource'
import {
  LIVE_SOURCE_EMPTY, LIVE_SOURCE_FAIL_HINT, LIVE_SOURCE_FAIL_TITLE, LIVE_SOURCE_FAILED, LIVE_SOURCE_GONE_TAG, LIVE_SOURCE_GROUP_SCREEN, LIVE_SOURCE_GROUP_WINDOW, LIVE_SOURCE_LOADING,
  LIVE_SOURCE_NO_WINDOW_HINT, LIVE_SOURCE_NO_WINDOW_TITLE, LIVE_SOURCE_PLACEHOLDER, LIVE_SOURCE_PLACEHOLDER_LOADING, LIVE_SOURCE_REFRESH, LIVE_SOURCE_REFRESH_ARIA, LIVE_SOURCE_REFRESH_SHORT,
  LIVE_SOURCE_REFRESHING, LIVE_SOURCE_RETRY, LIVE_SOURCE_STALE, liveSourceWindowCount,
} from '@/errors/errorMessages'

const props = withDefaults(
  defineProps<{
    sources: CaptureSource[]
    modelValue: string
    state: 'loading' | 'ready' | 'empty' | 'failed'
    mode?: 'dropdown' | 'list'
    invalid?: boolean
    /** LIVE_SOURCE_GONE：所选来源已不可用（名称保留，尺寸位置显示“已不可用”） */
    gone?: boolean
    /** gone 时列表里已经没有它了：用这份快照显示名称 */
    goneItem?: CaptureSource | null
    errorId?: string
  }>(),
  { mode: 'list' },
)
const emit = defineEmits<{ 'update:modelValue': [id: string]; refresh: [] }>()
const uid = `ff-src-${useId()}`
const triggerId = `${uid}-tg`
const listId = `${uid}-ls`
const labelId = inject<string | undefined>('ff-field-label-id', undefined)

const split = splitSourceTitle
const screens = computed(() => props.sources.filter((s) => s.kind === 'screen'))
const windows = computed(() => props.sources.filter((s) => s.kind === 'window'))
const windowCount = computed(() => windows.value.length)
const groupsAll = computed(() => [
  { kind: 'screen', title: LIVE_SOURCE_GROUP_SCREEN, items: screens.value },
  { kind: 'window', title: LIVE_SOURCE_GROUP_WINDOW, items: windows.value },
])
const flat = computed(() => [...screens.value, ...windows.value])
const current = computed<CaptureSource | undefined>(() => props.sources.find((s) => s.id === props.modelValue) ?? (props.gone ? props.goneItem ?? undefined : undefined))
const tip = (s: CaptureSource) => (s.width > 0 && s.height > 0 ? `${s.title}（${s.width}×${s.height}）` : s.title)

// ── 弹层 ──
const trigger = ref<HTMLButtonElement | null>(null)
const pop = ref<HTMLElement | null>(null)
const listEl = ref<HTMLElement | null>(null)
const open = ref(false)
const active = ref('')
const popStyle = ref<Record<string, string>>({})
function place() {
  const r = trigger.value?.getBoundingClientRect()
  if (!r) return
  popStyle.value = { left: `${r.left}px`, top: `${r.bottom + 4}px`, width: `${r.width}px` }
}
function toggle() {
  if (open.value) return close(false)
  openPop()
}
function openPop() {
  place()
  open.value = true
  active.value = props.modelValue && flat.value.some((s) => s.id === props.modelValue) ? props.modelValue : flat.value[0]?.id ?? ''
  void nextTick(() => scrollToActive())
  // 失效来源 / 首次失败：一打开就刷新（设计说明 §4.6：点选择器即展开并立即刷新窗口列表）
  if (props.gone && props.state !== 'loading') emit('refresh')
}
function close(focus = true) {
  open.value = false
  hover.value = ''
  if (focus) void nextTick(() => trigger.value?.focus())
}
function choose(id: string) {
  emit('update:modelValue', id)
  close()
}
function scrollToActive() {
  const el = active.value ? document.getElementById(`${uid}-o-${active.value}`) : null
  el?.scrollIntoView?.({ block: 'nearest' })
}
function move(delta: number | 'first' | 'last') {
  const l = flat.value
  if (!l.length) return
  const i = l.findIndex((s) => s.id === active.value)
  const n = delta === 'first' ? 0 : delta === 'last' ? l.length - 1 : Math.min(l.length - 1, Math.max(0, (i < 0 ? 0 : i) + delta))
  active.value = l[n].id
  void nextTick(scrollToActive)
}
function onTriggerKey(e: KeyboardEvent) {
  if (!open.value && ['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) {
    e.preventDefault()
    openPop()
  } else if (open.value) onPopKey(e)
}
function onPopKey(e: KeyboardEvent) {
  switch (e.key) {
    case 'ArrowDown': e.preventDefault(); move(1); break
    case 'ArrowUp': e.preventDefault(); move(-1); break
    case 'Home': e.preventDefault(); move('first'); break
    case 'End': e.preventDefault(); move('last'); break
    case 'Enter':
    case ' ':
      if ((e.target as HTMLElement)?.closest?.('.rf,.fl button')) return // 刷新 / 重试按钮自己处理
      e.preventDefault()
      if (active.value) choose(active.value)
      break
    case 'Escape': e.preventDefault(); e.stopPropagation(); close(); break
  }
}
function onDocDown(e: MouseEvent) {
  const t = e.target as Node
  if (pop.value?.contains(t) || trigger.value?.contains(t)) return
  close(false)
}
const onWin = () => close(false)
/** 弹层自己的列表滚动不算“页面滚动”（否则悬停 / 键盘滚到选项时弹层会被关掉） */
const onScroll = (e: Event) => {
  if (!pop.value?.contains(e.target as Node)) close(false)
}
watch(open, (o) => {
  if (o) {
    document.addEventListener('mousedown', onDocDown, true)
    window.addEventListener('resize', onWin)
    window.addEventListener('scroll', onScroll, true)
  } else {
    document.removeEventListener('mousedown', onDocDown, true)
    window.removeEventListener('resize', onWin)
    window.removeEventListener('scroll', onScroll, true)
  }
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocDown, true)
  window.removeEventListener('resize', onWin)
  window.removeEventListener('scroll', onScroll, true)
})
/** 父组件（点错误行“刷新列表”）要求展开 */
defineExpose({ openPop })

// ── 悬停 300ms 显示全名气泡（被省略的项）──
const hover = ref('')
const bubTop = ref(0)
let hoverTimer: ReturnType<typeof setTimeout> | null = null
const tipItem = computed(() => flat.value.find((s) => s.id === hover.value))
function hoverOn(s: CaptureSource, e: Event) {
  hoverOff()
  if (!split(s.title).tail) return // 没被省略的不用气泡
  const el = e.currentTarget as HTMLElement
  hoverTimer = setTimeout(() => {
    const host = pop.value?.getBoundingClientRect()
    const r = el.getBoundingClientRect()
    if (!host) return
    bubTop.value = r.bottom - host.top + 2
    hover.value = s.id
  }, 300)
}
function hoverOff() {
  if (hoverTimer) clearTimeout(hoverTimer)
  hoverTimer = null
  hover.value = ''
}
</script>

<style scoped>
.bar {
  display: flex;
  justify-content: flex-end;
  margin: -22px 0 4px;
  height: 18px;
  align-items: center;
}
.lst {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 232px;
  overflow-y: auto;
}
.grp {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.gh {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-3);
  line-height: 18px;
  margin-top: 4px;
}
.gh:first-child {
  margin-top: 0;
}
.so {
  height: 32px;
  flex: none;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  font: inherit;
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  background: var(--ff-bg-surface);
  cursor: pointer;
  text-align: left;
  min-width: 0;
}
.so svg {
  color: var(--ff-text-2);
  flex: none;
}
.nm {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.so em {
  flex: none;
  font-style: normal;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.so .rd {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1.5px solid var(--ff-text-2);
  flex: none;
  position: relative;
}
.so:hover {
  background: var(--ff-bg-hover);
}
.so.on {
  border-color: var(--ff-primary);
  background: var(--ff-primary-soft);
}
.so.on .rd {
  border-color: var(--ff-primary);
}
.so.on .rd::after {
  content: '';
  position: absolute;
  inset: 2px;
  border-radius: 50%;
  background: var(--ff-primary);
}
.so:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.st {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  padding: 0 10px;
  border: 1px dashed var(--ff-border);
  border-radius: 6px;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.st.bad {
  border-style: solid;
  color: var(--ff-danger-text);
  border-color: color-mix(in srgb, var(--ff-danger) 40%, var(--ff-border));
}
.st.bad span {
  flex: 1;
  min-width: 0;
}
.spin {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid var(--ff-border);
  border-top-color: var(--ff-primary);
  animation: cspin 1s linear infinite;
  flex: none;
}
@keyframes cspin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .spin {
    animation: none;
  }
}

/* ── Windows 分组下拉 ── */
.tg {
  width: 100%;
  height: 32px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 0 12px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font: inherit;
  font-size: var(--ff-fs-sm);
  cursor: pointer;
  text-align: left;
  min-width: 0;
}
.tg:hover {
  background: var(--ff-bg-hover);
}
.tg.open,
.tg:focus-visible {
  border-color: var(--ff-primary);
  outline: none;
}
.tg:focus-visible {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--ff-primary) 30%, transparent);
}
.tg.bad {
  border-color: var(--ff-danger);
}
.tg > svg:first-child {
  color: var(--ff-text-2);
  flex: none;
}
.tg .ar {
  margin-left: auto;
  color: var(--ff-text-2);
  flex: none;
}
.tg.open .ar {
  transform: rotate(180deg);
}
.tg em,
.op em {
  flex: none;
  font-style: normal;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.tg em.gn {
  color: var(--ff-danger-text);
}
.tg.ph .nm,
.nm.ph {
  color: var(--ff-text-3);
}
.tg .nm,
.op .nm {
  flex: 1;
  min-width: 0;
  display: flex;
  overflow: hidden;
  white-space: nowrap;
}
.tg .nm.ph {
  display: block;
  text-overflow: ellipsis;
}
.hd {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tl {
  flex: none;
}
.pop {
  position: fixed;
  z-index: 3000;
  box-sizing: border-box;
  background: var(--ff-bg-elevated);
  border: 1px solid var(--ff-border);
  border-radius: 8px;
  box-shadow: var(--ff-shadow-dialog);
  display: flex;
  flex-direction: column;
  color: var(--ff-text-1);
}
.pop .ls {
  max-height: 288px;
  overflow-y: auto;
  padding: 4px;
}
.pop .gh {
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 8px;
  margin: 0;
  font-size: var(--ff-fs-xs);
  font-weight: 600;
  color: var(--ff-text-2);
}
.pop [role='group'] + [role='group'] {
  margin-top: 4px;
}
.op {
  height: 32px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  border-radius: 6px;
  font-size: var(--ff-fs-sm);
  cursor: pointer;
  min-width: 0;
}
.op > svg:first-child {
  color: var(--ff-text-2);
  flex: none;
}
.op:hover,
.op.act {
  background: var(--ff-bg-hover);
}
.op.on {
  background: var(--ff-primary-soft);
}
.op .ck {
  color: var(--ff-primary-text);
  flex: none;
}
.sk {
  height: 32px;
  margin: 0 0 2px;
  border-radius: 6px;
  background: linear-gradient(90deg, var(--ff-bg-hover), var(--ff-border), var(--ff-bg-hover));
}
.none {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 8px 8px;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.5;
  overflow-wrap: anywhere;
}
.none b {
  font-size: var(--ff-fs-sm);
  font-weight: 600;
  color: var(--ff-text-1);
}
.fl {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 20px 16px;
  text-align: center;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.fl svg {
  color: var(--ff-warning-text);
}
.fl b {
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
}
.fl .lbtn {
  margin-top: 6px;
}
.ft {
  height: 36px;
  box-sizing: border-box;
  flex: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 12px;
  border-top: 1px solid var(--ff-border);
  font-size: var(--ff-fs-xs);
}
.ftl {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--ff-text-2);
  min-width: 0;
}
.ftl.bad {
  color: var(--ff-danger-text);
}
.rf {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  color: var(--ff-primary-text);
  cursor: pointer;
  flex: none;
}
.rf[aria-disabled='true'] {
  opacity: 0.5;
  cursor: default;
}
.rf:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
  border-radius: 2px;
}
.bub {
  position: absolute;
  left: 0;
  right: 0;
  z-index: 1;
  box-sizing: border-box;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--ff-text-1);
  color: var(--ff-bg-surface);
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  overflow-wrap: anywhere;
  pointer-events: none;
}
</style>
