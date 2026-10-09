<script setup lang="ts">
// csv：第一行当表头（固定），行号，数字右对齐。edit=true 时可编辑：双击单元格，Enter 确认，Esc 放弃这一格；插入行 / 插入列 / 删除行（设计场景 25 / 32）。
import { computed, nextTick, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { CSV_EDIT_HINT } from '@/utils/docV27Text'

const props = defineProps<{ rows: string[][]; edit?: boolean; scale: number }>()
const emit = defineEmits<{ (e: 'update:rows', v: string[][]): void }>()
const cols = computed(() => Math.max(1, ...props.rows.map((r) => r.length)))
const head = computed(() => props.rows[0] ?? [])
const body = computed(() => props.rows.slice(1))
const isNum = (v: string | undefined) => !!v && /^-?[\d,]+(\.\d+)?%?$/.test(v.trim())

/** 选中的格：r 是 rows 的下标（0 = 表头） */
const sel = ref<{ r: number; c: number } | null>(null)
const editing = ref<{ r: number; c: number } | null>(null)
const draft = ref('')
const dirtyCells = ref(new Set<string>())
const inputEl = ref<HTMLInputElement[] | null>(null)

function startEdit(r: number, c: number) {
  if (!props.edit) return
  sel.value = { r, c }
  editing.value = { r, c }
  draft.value = props.rows[r]?.[c] ?? ''
  void nextTick(() => {
    const el = inputEl.value?.[0]
    el?.focus()
    el?.select()
  })
}
function commit() {
  const e = editing.value
  if (!e) return
  editing.value = null
  const old = props.rows[e.r]?.[e.c] ?? ''
  if (old === draft.value) return
  const next = props.rows.map((r) => [...r])
  while (next[e.r].length < cols.value) next[e.r].push('')
  next[e.r][e.c] = draft.value
  dirtyCells.value.add(`${e.r}:${e.c}`)
  emit('update:rows', next)
}
const cancel = () => (editing.value = null)
function padded(): string[][] {
  return props.rows.map((r) => (r.length < cols.value ? [...r, ...Array(cols.value - r.length).fill('')] : [...r]))
}
function insertRow() {
  const at = sel.value ? Math.max(1, sel.value.r + 1) : props.rows.length
  const next = padded()
  next.splice(at, 0, Array(cols.value).fill(''))
  sel.value = { r: at, c: sel.value?.c ?? 0 }
  emit('update:rows', next)
}
function insertCol() {
  const at = sel.value ? sel.value.c + 1 : cols.value
  emit('update:rows', padded().map((r) => [...r.slice(0, at), '', ...r.slice(at)]))
  sel.value = { r: sel.value?.r ?? 0, c: at }
}
function deleteRow() {
  const s = sel.value
  if (!s || s.r < 1) return
  const next = props.rows.filter((_, i) => i !== s.r)
  sel.value = next.length > s.r ? s : next.length > 1 ? { r: next.length - 1, c: s.c } : null
  emit('update:rows', next)
}
const isSel = (r: number, c: number) => sel.value?.r === r && sel.value?.c === c
const isEd = (r: number, c: number) => editing.value?.r === r && editing.value?.c === c
</script>

<template>
  <div v-if="edit" class="pvx-etb">
    <button type="button" class="btn sm" @click="insertRow"><FIcon name="plus" :size="14" />插入行</button>
    <button type="button" class="btn sm" @click="insertCol"><FIcon name="plus" :size="14" />插入列</button>
    <button type="button" class="btn sm" :disabled="!sel || sel.r < 1" @click="deleteRow"><FIcon name="trash" :size="14" />删除行</button>
    <span class="sp" />
    <span class="hint">{{ CSV_EDIT_HINT }}</span>
  </div>
  <div class="pvx-scroll pvx-xyscroll">
    <table class="pvx-tbl" :class="{ ed: edit }" :style="{ zoom: scale }">
      <thead>
        <tr>
          <th class="n" />
          <th v-for="c in cols" :key="c" :class="{ cur: edit && isSel(0, c - 1) }" @click="edit && (sel = { r: 0, c: c - 1 })" @dblclick="startEdit(0, c - 1)">
            <input v-if="isEd(0, c - 1)" ref="inputEl" v-model="draft" class="pvx-cell-in" @keydown.enter.prevent="commit" @keydown.esc.stop.prevent="cancel" @blur="commit" />
            <template v-else>{{ head[c - 1] ?? '' }}</template>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(r, i) in body" :key="i" :class="{ sel: edit && sel?.r === i + 1 }">
          <td class="n">{{ i + 1 }}</td>
          <td
            v-for="c in cols"
            :key="c"
            :class="{ r: isNum(r[c - 1]), cur: edit && isSel(i + 1, c - 1) }"
            :data-dirty="dirtyCells.has(`${i + 1}:${c - 1}`) || undefined"
            @click="edit && (sel = { r: i + 1, c: c - 1 })"
            @dblclick="startEdit(i + 1, c - 1)"
          >
            <input v-if="isEd(i + 1, c - 1)" ref="inputEl" v-model="draft" class="pvx-cell-in" @keydown.enter.prevent="commit" @keydown.esc.stop.prevent="cancel" @blur="commit" />
            <template v-else>{{ r[c - 1] ?? '' }}</template>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
