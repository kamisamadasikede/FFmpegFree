<script setup lang="ts">
// 文档页左栏的一行源文件 + 它的转换记录（结构同转换页 v2 的 cv-src / cv-kid）
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MotionCollapse from '@/components/motion/MotionCollapse.vue'
import { useJustDone } from '@/composables/useJustDone'
import DocTypeCover from '@/components/docs/DocTypeCover.vue'
import { useDocConvertStore, type DocRow } from '@/stores/docConvert'
import type { DocRecord } from '@/api/docV26'
import { DOC_QUEUE_LINE, DOC_SIMPLE_PDF_LABEL, docResultWarnings } from '@/utils/docV26Text'
import { formatBytes, formatStart } from '@/utils/format'
import { ElMessageBox } from 'element-plus'
import { usePreviewStore, type PreviewItem } from '@/stores/docPreview'
import { EDITED_IN_APP, RECONVERT_EDITED_CONFIRM, engineRecordText } from '@/utils/docV27Text'

const props = defineProps<{ row: DocRow }>()
/** 排队 / 转换中 → 完成 的那一刻，完成标签弹一下 */
const justDone = useJustDone(() => props.row.records)
// 组件不报进度：转换中只显示不确定进度条 +「已用 m:ss」（设计 v0.2 §二.8）
const nowMs = ref(Date.now())
let tick = 0
onMounted(() => (tick = window.setInterval(() => (nowMs.value = Date.now()), 1000)))
onBeforeUnmount(() => clearInterval(tick))
// v0.27.2（6.12.56）：只按 startedAt 算；排队中 / 还没开始为空
function elapsed(r: DocRecord): string {
  if (!r.startedAt) return ''
  const s = Math.max(0, Math.floor((nowMs.value - r.startedAt) / 1000))
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
  pdf: 'PDF 文档',
}
const FAM: Record<string, string> = { text: '文档', sheet: '表格', slide: '演示', pdf: 'PDF' }
const meta = computed(() => {
  const parts = [KIND[src.value.ext] ?? FAM[src.value.family] ?? '']
  if (src.value.family === 'sheet' && src.value.ext !== 'csv' && src.value.sheetCount > 0) parts.push(`${src.value.sheetCount} 个工作表`)
  if (src.value.totalBytes) parts.push(formatBytes(src.value.totalBytes))
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
  if (r.type === 'office_pdf' || r.result?.engine === 'simple') return true
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
// ── v0.27 预览：眼睛图标 / 点封面打开统一预览弹窗；多选时 ← → 在选中的文件之间切换 ──
const pst = usePreviewStore()
const srcItem = (s: typeof src.value): PreviewItem => ({ req: { sourceId: s.sourceId }, name: s.name, sizeBytes: s.totalBytes ?? 0, converting: false })
function previewSource(e: Event) {
  const rows = sel.value && dc.selected.size > 1 ? dc.rows.filter((r) => dc.selected.has(r.src.sourceId)) : [props.row]
  const list = rows.map((r) => ({ ...srcItem(r.src), converting: r.records.some((x) => x.status === 'running' || x.status === 'queued') }))
  pst.show(list, Math.max(0, rows.findIndex((r) => r.src.sourceId === src.value.sourceId)), e.currentTarget as HTMLElement)
}
const okRecords = computed(() => props.row.records.filter((r) => r.status === 'succeeded'))
function previewRecord(r: DocRecord, e: Event) {
  const list = okRecords.value.map((x) => ({ req: { taskId: x.id }, name: x.title, sizeBytes: 0 }))
  pst.show(list, Math.max(0, okRecords.value.findIndex((x) => x.id === r.id)), e.currentTarget as HTMLElement)
}
// 在应用里改过的结果点重转：先确认（6.12.45 / 设计场景 44）
async function reconvert(r: DocRecord) {
  if (pst.editedTasks.has(r.id)) {
    try {
      await ElMessageBox.confirm(RECONVERT_EDITED_CONFIRM, `重转“${r.title}”`, { confirmButtonText: '重转', cancelButtonText: '取消', type: 'warning', customClass: 'dc-reconv' })
    } catch {
      return
    }
  }
  void dc.reconvert(r.id)
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
      <button type="button" class="dc-covbtn" :aria-label="`预览 ${src.name}`" @click.stop="previewSource"><DocTypeCover :ext="src.ext" /></button>
      <div class="cv-pm">
        <div class="cv-nm"><b :title="src.name">{{ src.name }}</b><span v-if="row.isNew" class="cv-tag t-new">新添加</span></div>
        <div class="m" :title="meta">{{ meta }}</div>
      </div>
      <span v-if="summary" class="cv-sum">{{ summary }}</span>
      <div class="cv-ops">
        <button type="button" class="cv-ib" aria-label="预览" title="预览" @click.stop="previewSource"><FIcon name="eye" /></button>
        <button type="button" class="cv-ib" aria-label="从列表移除" title="从列表移除" @click.stop="dc.removeSource(src.sourceId)"><FIcon name="trash" /></button>
      </div>
    </div>
    <MotionCollapse>
    <MotionCollapse v-if="row.open && row.records.length" group tag="div" class="cv-kids">
      <div v-for="r in row.records" :key="r.id" class="cv-kid" :class="{ q: r.status === 'queued' }" :data-kid="r.id">
        <button v-if="r.status === 'succeeded'" type="button" class="dc-covbtn" :aria-label="`预览 ${r.title}`" @click="previewRecord(r, $event)"><DocTypeCover :ext="targetOf(r)" sm /></button>
        <DocTypeCover v-else :ext="targetOf(r)" sm />
        <div class="cv-km">
          <div class="l1">
            <b :title="r.title">{{ r.title }}</b>
            <span class="cv-tag" :class="[TAG[r.status]?.cls, { 'ff-done-pop': justDone(r.id) }]"><FIcon v-if="r.status === 'succeeded'" name="check" /><FIcon v-else-if="r.status === 'failed'" name="warn" /><i class="tx">{{ TAG[r.status]?.text }}</i></span>
          </div>
          <div class="l2" :title="line2(r)">{{ line2(r) }}</div>
          <div class="l3">
            <template v-if="r.status === 'running'">
              <div class="bar ind dc-kbar" role="progressbar" aria-label="转换进度" aria-busy="true"><i /></div>
              <span v-if="elapsed(r)" class="dc-qtx">已用 {{ elapsed(r) }}</span>
            </template>
            <template v-else-if="r.status === 'queued'">
              <div class="bar q"><i style="width: 0" /></div>
              <span class="dc-qtx">{{ queueText(r) }}</span>
            </template>
          </div>
          <!-- v0.27（6.12.31）：由哪个引擎转的；go / simple / 旧记录不写。改过的结果多一句「在应用里改过」 -->
          <div v-if="r.status === 'succeeded' && (engineRecordText(r.result?.engine) || pst.editedTasks.has(r.id))" class="dc-eng">
            <FIcon name="info" /><span>{{ [engineRecordText(r.result?.engine), pst.editedTasks.has(r.id) ? EDITED_IN_APP : ''].filter(Boolean).join(' · ') }}</span>
          </div>
          <div v-if="r.status === 'succeeded' && docResultWarnings(r.result?.warnings).length" class="cv-fnote warn dc-rwarn">
            <FIcon name="info" /><span>{{ docResultWarnings(r.result?.warnings).join(' ') }}</span>
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
          <button v-if="r.status === 'succeeded'" type="button" class="cv-ib" aria-label="预览" title="预览" @click="previewRecord(r, $event)"><FIcon name="eye" /></button>
          <button v-if="r.status === 'succeeded'" type="button" class="cv-ib" aria-label="重转" title="重转" @click="reconvert(r)"><FIcon name="refresh" /></button>
          <button v-if="r.status !== 'running' && r.status !== 'queued'" type="button" class="cv-ib" aria-label="删除记录" title="删除记录" @click="dc.removeRecord(r.id)"><FIcon name="trash" /></button>
        </div>
      </div>
    </MotionCollapse>
    <div v-else-if="!row.records.length" class="cv-empty-kid">还没有转换记录。勾选后在右侧选择格式，点“转换”。</div>
    </MotionCollapse>
  </div>
</template>
