<script setup lang="ts">
// kind=raw 的 xlsx：Range 分段取字节 → SheetJS（按需加载，0.20.3）；只读显示，底部工作表标签（只有简易预览 xlsx 有）。
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { fetchRawBytes } from '@/api/docV27'
import { toAppError } from '@/api/call'

const props = defineProps<{ url: string; sizeBytes: number; scale: number }>()
const emit = defineEmits<{ (e: 'gone'): void; (e: 'failed'): void }>()
const MAX_ROWS = 2000
const MAX_COLS = 100
type Sheet = { name: string; rows: (string | number)[][]; cols: number }
const sheets = shallowRef<Sheet[]>([])
const cur = ref(0)
const ac = new AbortController()
const colName = (i: number) => {
  let s = ''
  for (let n = i + 1; n > 0; n = Math.floor((n - 1) / 26)) s = String.fromCharCode(65 + ((n - 1) % 26)) + s
  return s
}
onMounted(async () => {
  try {
    const bytes = await fetchRawBytes(props.url, props.url.startsWith('blob:') ? 0 : props.sizeBytes, { signal: ac.signal })
    const XLSX = await import('xlsx')
    const wb = XLSX.read(bytes, { type: 'array', cellFormula: false, cellHTML: false, sheetRows: MAX_ROWS + 1 })
    sheets.value = wb.SheetNames.map((name) => {
      const rows = (XLSX.utils.sheet_to_json(wb.Sheets[name], { header: 1, raw: false, defval: '', blankrows: true }) as (string | number)[][]).slice(0, MAX_ROWS).map((r) => r.slice(0, MAX_COLS))
      return { name, rows, cols: Math.max(1, ...rows.map((r) => r.length)) }
    })
  } catch (e) {
    if (ac.signal.aborted) return
    if (toAppError(e).code === 'NOT_FOUND') emit('gone')
    else emit('failed')
  }
})
onBeforeUnmount(() => ac.abort())
const sheet = computed(() => sheets.value[cur.value])
const isNum = (v: unknown) => typeof v === 'number' || (typeof v === 'string' && /^-?[\d,]+(\.\d+)?%?$/.test(v.trim()) && v.trim() !== '')
</script>

<template>
  <div class="pvx-scroll pvx-xyscroll">
    <table v-if="sheet" class="pvx-tbl" :style="{ zoom: scale }">
      <thead>
        <tr><th class="n" /><th v-for="c in sheet.cols" :key="c">{{ colName(c - 1) }}</th></tr>
      </thead>
      <tbody>
        <tr v-for="(r, i) in sheet.rows" :key="i">
          <td class="n">{{ i + 1 }}</td>
          <td v-for="c in sheet.cols" :key="c" :class="{ r: isNum(r[c - 1]) }">{{ r[c - 1] ?? '' }}</td>
        </tr>
      </tbody>
    </table>
  </div>
  <div v-if="sheets.length" class="pvx-tabs" role="tablist" aria-label="工作表">
    <button v-for="(s, i) in sheets" :key="s.name" type="button" role="tab" :aria-selected="i === cur" :class="{ on: i === cur }" @click="cur = i">{{ s.name }}</button>
  </div>
</template>
