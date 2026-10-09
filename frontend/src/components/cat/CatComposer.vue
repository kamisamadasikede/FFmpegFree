<template>
  <div ref="root" class="ct-comp" :class="{ bare: welcome }">
    <div v-if="!welcome" class="ctx-bar" aria-label="项目上下文">
      <span class="cx"><FIcon name="folder" :size="13" /><b :title="ctxName">{{ ctxName }}</b></span>
      <span class="cx"><FIcon name="monitor" :size="13" />本地</span>
      <span class="cx"><FIcon name="branch" :size="13" />{{ ctxBranch }}</span>
    </div>
    <div ref="inEl" class="ct-in">
      <textarea
        ref="ta"
        v-model="draft"
        class="ta"
        rows="1"
        :placeholder="placeholder"
        aria-label="输入消息"
        @input="autosize"
        @keydown.enter="onEnter"
      />
      <div class="row">
        <button type="button" class="ct-plus" title="添加" aria-label="添加"><FIcon name="plus" :size="15" :stroke="2" /></button>
        <button
          ref="chipAccess"
          type="button"
          class="chip-access"
          :class="{ on: catState.access === 'full' }"
          aria-haspopup="menu"
          :aria-expanded="menu === 'access'"
          @click.stop="toggle('access')"
        >
          <FIcon :name="catState.access === 'full' ? 'alert' : 'hand'" :size="14" /><span>{{ accessShort }}</span>
        </button>
        <span class="sp" />
        <button
          ref="chipModel"
          type="button"
          class="chip-model"
          aria-haspopup="menu"
          :aria-expanded="menu === 'casc'"
          @click.stop="toggle('casc')"
        >
          <FIcon name="gauge" :size="15" /><span>{{ capsuleLabel }}</span><FIcon name="down" :size="12" class="caret" />
        </button>
        <button
          v-if="running"
          type="button"
          class="ct-send ct-stop"
          :aria-label="stopping ? '正在停止' : '停止生成'"
          :title="stopping ? '正在停止' : '停止生成'"
          :disabled="stopping"
          data-testid="cat-stop"
          @click="onStop"
        ><i class="sq" aria-hidden="true" /></button>
        <button v-else type="button" class="ct-send" aria-label="发送" :aria-disabled="!canSend" @click="send"><FIcon name="up" :size="15" /></button>
      </div>

      <div v-if="menu" class="mn" :style="menuStyle" @click.stop>
        <!-- 访问模式 -->
        <div v-if="menu === 'access'" class="mn-access" role="menu" aria-label="访问模式">
          <div class="mn-ahd"><span>应如何批准 Cat 操作？</span><a class="more" href="#" @click.prevent>了解更多</a></div>
          <div
            v-for="a in CAT_ACCESS"
            :key="a.id"
            class="mn-acc"
            :class="{ on: catState.access === a.id, disabled: !a.enabled }"
            role="menuitemradio"
            tabindex="0"
            :aria-checked="catState.access === a.id"
            :aria-disabled="!a.enabled"
            :title="a.enabled ? undefined : laterTip"
            @click="pickAccess(a.id)"
            @keydown.enter.prevent="pickAccess(a.id)"
          >
            <span class="ic"><FIcon :name="a.icon" :size="18" /></span>
            <div><b>{{ a.name }}</b><small>{{ a.desc }}</small></div>
            <FIcon v-if="catState.access === a.id" name="check" :size="16" class="ok" />
          </div>
        </div>
        <!-- 模型 / 思考强度：一级菜单 + 左侧子菜单 -->
        <div v-else class="mn-casc">
          <div class="mn-main" role="menu" aria-label="模型设置">
            <div
              v-for="r in rows"
              :key="r.key"
              :ref="(el) => setRowRef(r.key, el)"
              class="mn-row"
              :class="{ on: sub === r.key }"
              role="menuitem"
              tabindex="0"
              aria-haspopup="menu"
              :aria-expanded="sub === r.key"
              @mouseenter="showSub(r.key)"
              @click.stop="showSub(r.key)"
              @keydown.enter.prevent="showSub(r.key)"
            >
              <span class="lb">{{ r.label }}</span><span class="v">{{ r.value }}</span><FIcon name="right" :size="14" />
            </div>
          </div>
          <div v-if="sub" :key="sub" ref="subEl" class="mn-sub" role="menu" :aria-label="sub === 'model' ? '模型' : '思考强度'" :style="{ top: subTop + 'px' }">
            <div
              v-for="o in subItems"
              :key="o.id"
              class="mn-op"
              :class="{ on: o.on }"
              role="menuitemradio"
              tabindex="0"
              :aria-checked="o.on"
              @click="pickSub(o.id)"
              @keydown.enter.prevent="pickSub(o.id)"
            >
              <span class="ck"><FIcon v-if="o.on" name="check" :size="15" :stroke="2.2" /></span><span class="nm">{{ o.name }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
    <div v-if="welcome" class="wl-tray">
      <button
        type="button"
        class="wl-proj"
        :class="{ on: catState.workInProject }"
        :aria-pressed="catState.workInProject"
        @click="catState.workInProject = !catState.workInProject"
      >
        <FIcon name="folder" :size="14" /><span>在项目中工作</span><FIcon name="down" :size="11" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, type ComponentPublicInstance } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { ElMessage } from 'element-plus'
import { CAT_COPY } from '@/api/cat'
import { CAT_ACCESS } from '@/api/catMock'
import { accessShort, capsuleLabel, catState, modelName, thinkName } from '@/views/cat/catState'

/**
 * Cat 输入框（设计 v0.4 / 原型 cat-v3）：访问默认「请求批准」，「完全访问」灰掉；
 * 合一胶囊只显示 List API 有的模型/强度（无强度不显示「·」半截）。
 */
const props = withDefaults(
  defineProps<{ welcome?: boolean; placeholder?: string; ctxName?: string; ctxBranch?: string; busy?: boolean; running?: boolean; stopping?: boolean }>(),
  {
    welcome: false,
    placeholder: '随心输入',
    ctxName: 'Cat',
    ctxBranch: 'main',
    busy: false,
    running: false,
    stopping: false,
  },
)
const emit = defineEmits<{ send: [text: string]; stop: [] }>()

/** 停止：父组件把 stopping 置真后按钮立即置灰，重复点击无效 */
function onStop() {
  if (props.stopping) return
  emit('stop')
}

const draft = ref('')
const canSend = computed(() => !!draft.value.trim() && !props.busy && !props.running)
const root = ref<HTMLElement>()
const inEl = ref<HTMLElement>()
const ta = ref<HTMLTextAreaElement>()
const chipAccess = ref<HTMLElement>()
const chipModel = ref<HTMLElement>()
const subEl = ref<HTMLElement>()

type MenuKind = 'access' | 'casc' | null
type SubKind = 'model' | 'think'
const menu = ref<MenuKind>(null)
const sub = ref<SubKind | null>(null)
const subTop = ref(0)
const menuStyle = ref<Record<string, string>>({})
const rowEls: Partial<Record<SubKind, HTMLElement>> = {}
function setRowRef(k: SubKind, el: Element | ComponentPublicInstance | null) {
  if (el instanceof HTMLElement) rowEls[k] = el
}

const laterTip = CAT_COPY.later

const rows = computed(() => {
  const list: Array<{ key: 'model' | 'think'; label: string; value: string }> = [
    { key: 'model', label: '模型', value: modelName.value || 'Cat 助手' },
  ]
  // 无强度列表时不展示「思考强度」行（设计 v0.4 §5.2）
  if (catState.thinks.length) {
    list.push({ key: 'think', label: '思考强度', value: thinkName.value })
  }
  return list
})
const subItems = computed(() =>
  sub.value === 'model'
    ? catState.models.map((m) => ({ id: m.id, name: m.displayName, on: catState.model === m.id }))
    : catState.thinks.map((t) => ({ id: t.id, name: t.displayName, on: catState.think === t.id })),
)

function open(kind: Exclude<MenuKind, null>) {
  const ci = inEl.value?.getBoundingClientRect()
  if (kind === 'access') {
    const a = chipAccess.value?.getBoundingClientRect()
    menuStyle.value = { left: Math.max(0, (a?.left ?? 0) - (ci?.left ?? 0) - 8) + 'px', right: 'auto' }
  } else {
    const a = chipModel.value?.getBoundingClientRect()
    menuStyle.value = { right: Math.max(0, (ci?.right ?? 0) - (a?.right ?? 0)) + 'px', left: 'auto' }
  }
  sub.value = null
  menu.value = kind
}
function close() {
  menu.value = null
  sub.value = null
}
function toggle(kind: Exclude<MenuKind, null>) {
  if (menu.value === kind) close()
  else open(kind)
}
/** 子菜单顶部与触发行顶部对齐（减去自身 4px 内边距 + 1px 边框），向下展开；靠近窗口边缘时挪回窗口内 */
async function showSub(k: SubKind) {
  if (sub.value === k) return
  sub.value = k
  const row = rowEls[k]
  const top = (row?.offsetTop ?? 0) - 5
  subTop.value = top
  await nextTick()
  const r = subEl.value?.getBoundingClientRect()
  if (!r) return
  if (r.bottom > innerHeight - 8) subTop.value = top - (r.bottom - innerHeight + 8)
  else if (r.top < 8) subTop.value = top + 8 - r.top
}
function pickSub(id: string) {
  if (sub.value === 'model') catState.model = id
  else catState.think = id
  close()
}
function pickAccess(id: 'ask' | 'full') {
  const a = CAT_ACCESS.find((x) => x.id === id)
  if (!a?.enabled) {
    ElMessage.info(CAT_COPY.later)
    return
  }
  catState.access = id
  close()
}

function autosize() {
  const el = ta.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 160) + 'px'
}
function send() {
  if (!canSend.value) return
  emit('send', draft.value)
  draft.value = ''
  nextTick(autosize)
}
function onEnter(e: KeyboardEvent) {
  if (e.shiftKey || e.isComposing) return
  e.preventDefault()
  send()
}

function onDocClick(e: MouseEvent) {
  if (menu.value && !(e.target instanceof Node && root.value?.querySelector('.mn')?.contains(e.target))) close()
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && menu.value) {
    close()
    e.stopPropagation()
  }
}
onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKey)
})
defineExpose({ focus: () => ta.value?.focus(), setDraft: (t: string) => { draft.value = t; nextTick(autosize); ta.value?.focus() } })
</script>

<style scoped>
.ct-comp {
  position: relative;
  flex: none;
  margin: 0 auto 16px;
  width: 560px;
  max-width: calc(100% - 32px);
  padding-top: 22px;
}
.ct-comp.bare {
  padding-top: 0;
}
.ctx-bar {
  position: absolute;
  top: 0;
  left: 10px;
  right: 10px;
  height: 36px;
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 0 14px 10px;
  background: var(--ff-bg-sidebar);
  border-radius: 14px 14px 0 0;
  z-index: 0;
  font-size: 12px;
  color: var(--ff-text-2);
  overflow: hidden;
}
html.dark .ctx-bar {
  background: #2a2c33;
}
.ctx-bar .cx {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  white-space: nowrap;
}
.ctx-bar .cx:first-child {
  flex: 0 1 auto;
  overflow: hidden;
}
.ctx-bar .cx b {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ctx-bar .cx :deep(svg) {
  color: var(--ff-text-3);
}
.ct-in {
  position: relative;
  z-index: 1;
  border: 1px solid #dcdde1;
  border-radius: 22px;
  padding: 14px 14px 10px;
  background: var(--ff-bg-surface);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.06), 0 1px 3px rgba(0, 0, 0, 0.04);
}
html.dark .ct-in {
  border-color: var(--ff-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.35);
}
.ta {
  display: block;
  width: 100%;
  min-height: 40px;
  max-height: 160px;
  resize: none;
  border: none;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 14px;
  line-height: 22px;
  padding: 9px 2px;
  color: var(--ff-text-1);
}
.ta::placeholder {
  color: var(--ff-text-3);
}
.bare .ct-in {
  min-height: 113px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}
.bare .ta {
  padding: 1px 2px 0;
  min-height: 0;
}
.row {
  min-height: 52px; /* 与原型实测一致（输入框里控件行 52 高） */
  display: flex;
  align-items: center;
  gap: 8px;
}
.row .sp {
  flex: 1;
}
button {
  font: inherit;
}
.ct-plus {
  width: 30px;
  height: 30px;
  border-radius: 50%;
  border: none;
  background: #f2f3f5;
  color: var(--ff-text-2);
  display: grid;
  place-items: center;
  cursor: pointer;
  flex: none;
}
.ct-plus:hover {
  background: #e8e9ec;
}
html.dark .ct-plus {
  background: var(--ff-bg-hover);
}
.ct-send {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: none;
  display: grid;
  place-items: center;
  background: #000;
  color: #fff;
  cursor: pointer;
  flex: none;
  transition: background var(--ff-dur-quick) var(--ff-ease);
}
.ct-send:hover {
  background: #222;
}
.ct-send[aria-disabled='true'] {
  cursor: default;
}
html.dark .ct-send {
  background: #f2f3f5;
  color: #000;
}
html.dark .ct-send:hover {
  background: #fff;
}
.ct-stop .sq {
  width: 10px;
  height: 10px;
  border-radius: 2px;
  background: currentColor;
}
.ct-stop:disabled,
.ct-stop:disabled:hover {
  cursor: default;
  opacity: 0.4;
}

/* 访问模式胶囊（完全访问时橙色） */
.chip-access {
  height: 28px;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 0 10px 0 8px;
  border-radius: 14px;
  border: 1px solid transparent;
  background: transparent;
  font-size: 12.5px;
  font-weight: 500;
  color: var(--ff-text-2);
  cursor: pointer;
  white-space: nowrap;
  transition: background var(--ff-dur-quick), border-color var(--ff-dur-quick), color var(--ff-dur-quick);
}
.chip-access:hover {
  background: var(--ff-bg-hover);
}
.chip-access.on {
  color: #e87a1f;
  background: rgba(232, 122, 31, 0.08);
  border-color: rgba(232, 122, 31, 0.35);
}
.chip-access.on:hover,
.chip-access[aria-expanded='true'] {
  background: rgba(232, 122, 31, 0.14);
}
.chip-access:focus-visible {
  outline: 2px solid #e87a1f;
  outline-offset: 1px;
}

/* 合一模型胶囊 */
.chip-model {
  height: 30px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 8px 0 9px;
  border-radius: 15px;
  border: none;
  background: transparent;
  font-size: 14px;
  color: var(--ff-text-1);
  cursor: pointer;
  white-space: nowrap;
  transition: background var(--ff-dur-quick);
}
.chip-model > :deep(svg) {
  color: var(--ff-text-2);
}
.chip-model > :deep(svg.caret) {
  color: var(--ff-text-3);
}
.chip-model:hover,
.chip-model[aria-expanded='true'] {
  background: #f2f3f5;
}
html.dark .chip-model:hover,
html.dark .chip-model[aria-expanded='true'] {
  background: var(--ff-bg-hover);
}
.chip-model:focus-visible,
.ct-plus:focus-visible,
.ct-send:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 1px;
}

/* 菜单：外层只定位，在输入框上方 8px */
.mn {
  position: absolute;
  bottom: calc(100% + 8px);
  z-index: 30;
  animation: mnUp 140ms ease both;
}
@keyframes mnUp {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: none; }
}
@keyframes subIn {
  from { opacity: 0; transform: translateX(4px); }
  to { opacity: 1; transform: none; }
}
.mn-access {
  width: 340px;
  padding: 10px;
  background: var(--ff-bg-elevated);
  border: 1px solid var(--ff-border);
  border-radius: 14px;
  box-shadow: var(--ff-shadow-dialog);
}
.mn-ahd {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 2px 6px 10px;
  font-size: 12px;
  color: var(--ff-text-3);
}
.mn-ahd .more {
  color: var(--ff-text-2);
  text-decoration: underline;
  font-size: 12px;
}
.mn-ahd .more:hover {
  color: var(--ff-text-1);
}
.mn-acc {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px;
  border-radius: 10px;
  cursor: pointer;
  color: var(--ff-text-1);
}
.mn-acc:hover {
  background: var(--ff-bg-hover);
}
.mn-acc .ic {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  flex: none;
  margin-top: 1px;
}
.mn-acc div {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.mn-acc b {
  font-size: 13.5px;
  font-weight: 600;
}
.mn-acc small {
  font-size: 12px;
  color: var(--ff-text-3);
  line-height: 1.45;
}
.mn-acc .ok {
  margin-top: 2px;
  color: #e87a1f;
}
.mn-acc.on,
.mn-acc.on b,
.mn-acc.on small {
  color: #e87a1f;
}
.mn-acc.on:hover {
  background: rgba(232, 122, 31, 0.08);
}
.mn-acc.disabled,
.mn-acc.disabled:hover {
  opacity: 0.45;
  cursor: not-allowed;
  background: transparent;
  color: var(--ff-text-3);
}
.mn-acc.disabled b,
.mn-acc.disabled small {
  color: var(--ff-text-3);
}

.mn-casc {
  position: relative;
}
.mn-main,
.mn-sub {
  background: var(--ff-bg-elevated);
  border: 1px solid #e6e7ea;
  border-radius: 6px;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.08), 0 1px 3px rgba(0, 0, 0, 0.05);
  padding: 4px;
}
html.dark .mn-main,
html.dark .mn-sub {
  border-color: var(--ff-border);
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.5);
}
.mn-main {
  width: 182px;
}
.mn-row {
  height: 38px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 0 10px;
  border-radius: 4px;
  font-size: 14px;
  color: var(--ff-text-1);
  cursor: pointer;
  white-space: nowrap;
}
.mn-row .v {
  margin-left: auto;
  font-size: 13.5px;
  color: #7a7d85;
}
html.dark .mn-row .v {
  color: var(--ff-text-3);
}
.mn-row:hover,
.mn-row.on,
.mn-op:hover,
.mn-op.on {
  background: #f2f3f5;
}
html.dark .mn-row:hover,
html.dark .mn-row.on,
html.dark .mn-op:hover,
html.dark .mn-op.on {
  background: var(--ff-bg-hover);
}
.mn-sub {
  position: absolute;
  right: calc(100% + 3px);
  width: 180px;
  animation: subIn 120ms ease both;
}
.mn-op {
  height: 36px;
  display: flex;
  align-items: center;
  padding: 0 10px 0 8px;
  border-radius: 4px;
  font-size: 14px;
  color: var(--ff-text-1);
  cursor: pointer;
  white-space: nowrap;
}
.mn-op .ck {
  width: 28px;
  flex: none;
  display: flex;
  align-items: center;
  color: #3b6ef5;
}
html.dark .mn-op .ck {
  color: #7aa2ff;
}
.mn-row:focus-visible,
.mn-op:focus-visible,
.mn-acc:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
}

/* 欢迎页：输入框下方的灰色底条，顶边被输入框压住 */
.wl-tray {
  position: relative;
  z-index: 0;
  margin-top: -20px;
  padding: 26px 18px 10px;
  background: #f2f3f5;
  border-radius: 0 0 22px 22px;
  display: flex;
  align-items: center;
}
html.dark .wl-tray {
  background: #2a2c33;
}
.wl-proj {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: none;
  font-size: 13.5px;
  color: var(--ff-text-2);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
}
.wl-proj:not(.on) {
  color: var(--ff-text-3);
}
.wl-proj:hover {
  color: var(--ff-text-1);
}
.wl-proj :deep(svg) {
  color: var(--ff-text-3);
}
</style>
