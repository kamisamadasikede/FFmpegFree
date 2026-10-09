<script setup lang="ts">
// 文档页左栏的一行源文件 + 它的转换记录（结构同转换页 v2 的 cv-src / cv-kid）
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import DocTypeCover from '@/components/docs/DocTypeCover.vue'
import { useDocConvertStore, type DocRow } from '@/stores/docConvert'
import type { DocRecord } from '@/api/docV26'
import { DOC_QUEUE_LINE, DOC_SIMPLE_PDF_LABEL } from '@/utils/docV26Text'
import { formatBytes, formatStart } from '@/utils/format'

const props = defineProps<{ row: DocRow }>()
// 组件不报进度：转换中只显示不确定进度条 +「已用 m:ss」（设计 v0.2 §二.8）
const nowMs = ref(Date.now())
let tick = 0
onMounted(() => (tick = window.setInterval(() => (nowMs.value = Date.now()), 1000)))
onBeforeUnmount(() => clearInterval(tick))
function elapsed(r: DocRecord): string {
  const s = Math.max(0, Math.floor((nowMs.value - (r.startedAt || r.createdAt)) / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}
// 标签已写「排队中」，进度条右侧只写「前面还有 n 项」/「下一个」
function queueText(r: DocRecord): string {
  const n = dc.recordQueueText(r.id)
  return n === null ? '' : DOC_QUEUE_LINE(n)
}
const dc = useDocConvertStore()
const src = computed(() => props.row.src)
const sel = computed(() => dc.selected.has(src.value.sourceId))
// 设计 v0.2：源文件第二行写类型名（Word 文档 · 2.4 MB；表格带工作表数）
const KIND: Record<string, string> = {
  doc: 'Word 文档（旧格式）', docx: 'Word 文档', odt: '开放文档', rtf: '富文本', txt: '纯文本', html: '网页', md: 'Markdown',
  xls: 'Excel 表格（旧格式）', xlsx: 'Excel 表格', ods: '开放表格', csv: 'CSV 表格',
  ppt: 'PowerPoint 演示（旧格式）', pptx: 'PowerPoint 演示', odp: '开放演示',
}
const FAM: Record<string, string> = { text: '文档', sheet: '表格', slide: '演示' }
const meta = computed(() => {
  const parts = [KIND[src.value.ext] ?? FAM[src.value.family] ?? '']
  if (src.value.family === 'sheet' && src.value.ext !== 'csv' && src.value.sheetCount > 0) parts.push(`${src.value.sheetCount} 个工作表`)
  if (src.value.sizeBytes) parts.push(formatBytes(src.value.sizeBytes))
  return parts.filter(Boolean).join(' · ')
})

function targetOf(r: DocRecord): string {
  try {
    return String(JSON.parse(r.params || '{}').target || '').toLowerCase() || (r.title.split('.').pop() ?? '')
  } catch {
    return r.title.split('.').pop() ?? ''
  }
}
function isSimple(r: DocRecord): boolean {
  if (r.type === 'office_pdf') return true
  try {
    return !!JSON.parse(r.params || '{}').simple
  } catch {
    return false
  }
}
function line2(r: DocRecord): string {
  const ext = targetOf(r)
  const t = formatStart(r.finishedAt || r.createdAt)
  if (ext === 'pdf') return `${t} · ${isSimple(r) ? `PDF · ${DOC_SIMPLE_PDF_LABEL}` : 'PDF（保留版式）'}`
  return `${t} · ${ext.toUpperCase()}`
}
const TAG: Record<string, { cls: string; text: string }> = {
  running: { cls: 't-run', text: '转换中' },
  queued: { cls: 't-q', text: '排队中' },
  succeeded: { cls: 't-ok', text: '完成' },
  failed: { cls: 't-fail', text: '失败' },
  canceled: { cls: 't-cx', text: '已取消' },
  interrupted: { cls: 't-cx', text: '已中断' },
}
const summary = computed(() => {
  const rs = props.row.records
  if (!rs.length) return ''
  const fail = rs.filter((r) => r.status === 'failed').length
  if (fail) return `${fail} 项失败 · 共 ${rs.length} 条`
  const run = rs.filter((r) => r.status === 'running').length
  if (run) return `${run} 项转换中 · 共 ${rs.length} 条`
  const q = rs.filter((r) => r.status === 'queued').length
  if (q) return `${q} 项排队中 · 共 ${rs.length} 条`
  return `${rs.length} 条记录`
})
</script>

<template>
  <div class="cv-src" :class="{ sel, open: row.open && row.records.length }" :data-src="src.sourceId">
    <div class="cv-prow" @click="dc.toggle(src.sourceId)">
      <button type="button" class="cv-chk" :class="{ on: sel }" role="checkbox" :aria-checked="sel" :aria-label="`勾选 ${src.name}`" @click.stop="dc.toggle(src.sourceId)"><FIcon v-if="sel" name="check" /></button>
      <button type="button" class="cv-fold" :class="{ closed: !row.open, none: !row.records.length }" :aria-expanded="row.records.length ? row.open : undefined" :aria-label="row.open ? `收起 ${src.name} 的转换记录` : `展开 ${src.name} 的转换记录`" @click.stop="row.open = !row.open"><FIcon name="down" /></button>
      <DocTypeCover :ext="src.ext" />
      <div class="cv-pm">
        <div class="cv-nm"><b :title="src.name">{{ src.name }}</b><span v-if="row.isNew" class="cv-tag t-new">新添加</span></div>
        <div class="m" :title="meta">{{ meta }}</div>
      </div>
      <span v-if="summary" class="cv-sum">{{ summary }}</span>
      <div class="cv-ops">
        <button type="button" class="cv-ib" aria-label="从列表移除" title="从列表移除" @click.stop="dc.removeSource(src.sourceId)"><FIcon name="trash" /></button>
      </div>
    </div>
    <div v-if="row.open && row.records.length" class="cv-kids">
      <div v-for="r in row.records" :key="r.id" class="cv-kid" :class="{ q: r.status === 'queued' }" :data-kid="r.id">
        <DocTypeCover :ext="targetOf(r)" sm />
        <div class="cv-km">
          <div class="l1">
            <b :title="r.title">{{ r.title }}</b>
            <span class="cv-tag" :class="TAG[r.status]?.cls"><FIcon v-if="r.status === 'succeeded'" name="check" /><FIcon v-else-if="r.status === 'failed'" name="warn" /><i class="tx">{{ TAG[r.status]?.text }}</i></span>
          </div>
          <div class="l2" :title="line2(r)">{{ line2(r) }}</div>
          <div class="l3">
            <template v-if="r.status === 'running'">
              <div class="bar ind dc-kbar" role="progressbar" aria-label="转换进度" aria-busy="true"><i /></div>
              <span class="dc-qtx">已用 {{ elapsed(r) }}</span>
            </template>
            <template v-else-if="r.status === 'queued'">
              <div class="bar q"><i style="width: 0" /></div>
              <span class="dc-qtx">{{ queueText(r) }}</span>
            </template>
          </div>
          <div v-if="dc.recordError(r)" class="cv-err" role="alert">
            <FIcon name="warn" />
            <div class="t">
              <b>转换失败</b>{{ dc.recordError(r)!.text }}
              <div class="acts">
                <button v-if="dc.recordError(r)!.retryable" type="button" class="ff-link" @click="dc.retry(r.id)">重试</button>
                <button v-else type="button" class="ff-link" @click="dc.removeSource(src.sourceId)">从列表移除</button>
              </div>
            </div>
          </div>
        </div>
        <div class="cv-ops">
          <button v-if="r.status === 'running' || r.status === 'queued'" type="button" class="cv-ib" aria-label="取消" title="取消" @click="dc.cancel(r.id)"><FIcon name="x" /></button>
          <button v-else-if="r.status === 'failed' && dc.recordError(r)?.retryable" type="button" class="cv-ib" aria-label="重试" title="重试" @click="dc.retry(r.id)"><FIcon name="retry" /></button>
          <button v-if="r.status !== 'running' && r.status !== 'queued'" type="button" class="cv-ib" aria-label="删除记录" title="删除记录" @click="dc.removeRecord(r.id)"><FIcon name="trash" /></button>
        </div>
      </div>
    </div>
    <div v-else-if="!row.records.length" class="cv-empty-kid">还没有转换记录。勾选后在右侧选择格式，点“转换”。</div>
  </div>
</template>
