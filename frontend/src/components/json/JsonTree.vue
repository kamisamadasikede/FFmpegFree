<template>
  <div
    ref="boxRef"
    class="jtree"
    role="tree"
    tabindex="0"
    aria-label="JSON 树形视图"
    @scroll.passive="onScroll"
    @keydown="onKey"
  >
    <div class="jtree-space" :style="{ height: rows.length * ROW_H + PAD * 2 + 'px' }">
      <div
        v-for="(r, i) in visible"
        :key="r.node.id"
        class="tr"
        :class="{ active: start + i === active }"
        role="treeitem"
        :aria-expanded="r.node.children ? expanded.has(r.node.id) : undefined"
        :aria-level="r.node.depth + 1"
        :style="{ top: PAD + (start + i) * ROW_H + 'px', paddingLeft: 8 + r.node.depth * INDENT + 'px' }"
        @click="onRow(start + i, r.node)"
      >
        <span class="tw" :class="{ open: r.node.children && expanded.has(r.node.id) }">
          <svg v-if="r.node.children" viewBox="0 0 16 16"><path d="M6 4l5 4-5 4z" /></svg>
        </span>
        <template v-if="r.node.key !== undefined">
          <span :class="r.node.isIndex ? 'ix' : 'k'">{{ r.node.key }}</span
          ><span class="p">:</span>&nbsp;
        </template>
        <template v-if="r.node.children">
          <span class="p">{{ r.node.kind === 'array' ? '[' : '{' }}</span
          ><span class="cnt">{{ r.node.children.length }}</span
          ><span class="p">{{ r.node.kind === 'array' ? ']' : '}' }}</span>
        </template>
        <span v-else class="val" :class="valClass(r.node.kind)" :title="r.node.raw!.length > 120 ? r.node.raw : undefined">{{
          clip(r.node.raw!)
        }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// JSON 树形视图：行高 24px、缩进 16px。可见行由展开集合展开后按滚动位置切片渲染（虚拟滚动），
// 所以几十万节点的大 JSON 也只有屏幕内的一屏 DOM。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { defaultExpanded, flatten, parseTree, type JNode, type JKind } from '@/utils/jsonTree'

const ROW_H = 24
const INDENT = 16
const PAD = 8
const OVERSCAN = 8
const MAX_VALUE_CHARS = 2000

const props = defineProps<{ text: string }>()
const emit = defineEmits<{ nodeCount: [n: number] }>()

const boxRef = ref<HTMLElement | null>(null)
const root = shallowRef<JNode | null>(null)
const expanded = shallowRef<Set<number>>(new Set())
const scrollTop = ref(0)
const viewH = ref(400)
const active = ref(0)
let hadTree = false

const rows = computed(() => (root.value ? flatten(root.value, expanded.value) : []))
const start = computed(() => Math.max(0, Math.floor((scrollTop.value - PAD) / ROW_H) - OVERSCAN))
const visible = computed(() => {
  const end = Math.min(rows.value.length, Math.ceil((scrollTop.value + viewH.value) / ROW_H) + OVERSCAN)
  const out: { node: JNode }[] = []
  for (let i = start.value; i < end; i++) out.push({ node: rows.value[i] })
  return out
})

watch(
  () => props.text,
  (t) => {
    root.value = t ? parseTree(t) : null
    if (root.value && !hadTree) expanded.value = defaultExpanded(root.value)
    hadTree = !!root.value
    if (active.value >= rows.value.length) active.value = 0
  },
  { immediate: true },
)

const clip = (s: string) => (s.length > MAX_VALUE_CHARS ? s.slice(0, MAX_VALUE_CHARS) + '…' : s)
const valClass = (k: JKind) => ({ string: 's', number: 'n', boolean: 'b', null: 'nl' })[k as 'string'] ?? ''

function toggle(n: JNode, open?: boolean) {
  if (!n.children) return
  const s = new Set(expanded.value)
  const want = open ?? !s.has(n.id)
  if (want) s.add(n.id)
  else s.delete(n.id)
  expanded.value = s
}
function onRow(i: number, n: JNode) {
  active.value = i
  toggle(n)
}
function onScroll() {
  scrollTop.value = boxRef.value!.scrollTop
}
function reveal(i: number) {
  const el = boxRef.value!
  const top = PAD + i * ROW_H
  if (top < el.scrollTop) el.scrollTop = top - PAD
  else if (top + ROW_H > el.scrollTop + el.clientHeight) el.scrollTop = top + ROW_H - el.clientHeight + PAD
}
function onKey(e: KeyboardEvent) {
  const n = rows.value[active.value]
  if (!n) return
  let handled = true
  if (e.key === 'ArrowDown') active.value = Math.min(rows.value.length - 1, active.value + 1)
  else if (e.key === 'ArrowUp') active.value = Math.max(0, active.value - 1)
  else if (e.key === 'ArrowRight') {
    if (n.children && !expanded.value.has(n.id)) toggle(n, true)
    else active.value = Math.min(rows.value.length - 1, active.value + 1)
  } else if (e.key === 'ArrowLeft') {
    if (n.children && expanded.value.has(n.id)) toggle(n, false)
    else {
      for (let i = active.value - 1; i >= 0; i--) {
        if (rows.value[i].depth < n.depth) {
          active.value = i
          break
        }
      }
    }
  } else if (e.key === 'Enter' || e.key === ' ') toggle(n)
  else handled = false
  if (handled) {
    e.preventDefault()
    nextTick(() => reveal(active.value))
  }
}

let ro: ResizeObserver | null = null
onMounted(() => {
  viewH.value = boxRef.value!.clientHeight
  ro = new ResizeObserver(() => (viewH.value = boxRef.value!.clientHeight))
  ro.observe(boxRef.value!)
})
onBeforeUnmount(() => ro?.disconnect())
</script>

<style scoped>
.jtree {
  flex: 1;
  min-height: 0;
  overflow: auto;
  font-family: var(--ff-font-mono);
  font-size: var(--ff-fs-sm);
  line-height: 24px;
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  outline: none;
  user-select: text;
}
.jtree-space {
  position: relative;
}
.tr {
  position: absolute;
  left: 0;
  right: 0;
  height: 24px;
  display: flex;
  align-items: center;
  white-space: pre;
  cursor: default;
  overflow: hidden;
}
.tr:hover {
  background: var(--ff-bg-hover);
}
.jtree:focus-visible .tr.active {
  box-shadow: inset 0 0 0 1px var(--ff-primary);
}
.tw {
  width: 16px;
  height: 24px;
  flex: none;
  display: grid;
  place-items: center;
  color: var(--ff-text-3);
  cursor: pointer;
}
.tw svg {
  width: 16px;
  height: 16px;
  fill: currentColor;
  transition: transform 0.12s;
}
.tw.open svg {
  transform: rotate(90deg);
}
.tr:hover .tw {
  color: var(--ff-text-2);
}
.k {
  color: var(--ff-syntax-key);
}
.ix,
.p,
.cnt {
  color: var(--ff-text-3);
}
.cnt {
  margin-left: 0;
}
.val {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.s {
  color: var(--ff-syntax-string);
}
.n {
  color: var(--ff-syntax-number);
}
.b {
  color: var(--ff-syntax-bool);
}
.nl {
  color: var(--ff-syntax-null);
}
</style>
