/**
 * 文档页 v0.26：源文件行、记录、格式表交集、排队（契约 6.12.13~6.12.21）。
 * 真实：DocService + task:* 事件 + doc:queue；模拟：本 store 自己推进（组件池并发 2，md↔html / 简易转换不排队）。
 */
import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import {
  addDocSources,
  cancelDocRecord,
  deleteDocRecords,
  deleteDocSource,
  docFamilyOf,
  docV26IsReal,
  docV28On,
  getFormatMatrix,
  listDocSources,
  retryDocRecord,
  submitDocConvert,
  watchDocQueue,
  watchDocTasks,
  type DocFormatMatrix,
  type DocRecord,
  type DocSource,
  type DocTarget,
} from '@/api/docV26'
import { callService, toAppError } from '@/api/call'
import { simParam } from '@/api/sim'
import { pickFiles } from '@/api/system'
import { useDocComponentStore } from '@/stores/docComponent'
import {
  DOC_CSV_HINT,
  DOC_MD_HINT,
  DOC_PDF_TEXT_ONLY_HINT,
  DOC_SIMPLE_HINT,
  docAddErrorText,
  docDiskFullText,
  docPdfNote,
  docErrorRetryable,
  docErrorText,
  intersectionWhy,
  sameFamilyWhy,
} from '@/utils/docV26Text'

export const DOC_FILE_FILTER = {
  name: '文档、表格和演示',
  patterns: ['*.doc', '*.docx', '*.odt', '*.rtf', '*.txt', '*.html', '*.htm', '*.md', '*.markdown', '*.xls', '*.xlsx', '*.ods', '*.csv', '*.ppt', '*.pptx', '*.odp'],
}
/** v0.28：DOC_V28_BACKEND_READY（或纯浏览器模拟）时文件选择也列 .pdf */
export function docFileFilter(): { name: string; patterns: string[] } {
  if (!docV28On()) return DOC_FILE_FILTER
  return { name: '文档、表格、演示和 PDF', patterns: [...DOC_FILE_FILTER.patterns, '*.pdf'] }
}

/** 格式块第二行（设计 §三；v0.2 可能调整） */
export const DOC_TILE_SUB: Record<string, string> = {
  pdf: '保留版式',
  docx: 'Word',
  doc: 'Word 旧版',
  odt: '开放文档',
  rtf: '富文本',
  txt: '纯文本',
  html: '网页',
  md: 'Markdown',
  xlsx: 'Excel',
  xls: 'Excel 旧版',
  ods: '开放表格',
  csv: '逗号分隔',
  pptx: 'PPT',
  ppt: 'PPT 旧版',
  odp: '开放演示',
}
export const DOC_GROUPS: { key: 'pdf' | 'text' | 'sheet' | 'slide'; label: string }[] = [
  { key: 'pdf', label: 'PDF' },
  { key: 'text', label: '文档' },
  { key: 'sheet', label: '表格' },
  { key: 'slide', label: '演示' },
]

export interface DocTile extends DocTarget {
  group: 'pdf' | 'text' | 'sheet' | 'slide'
  /** 组件还在检测中、这一格暂时不可用（或只有简易 PDF）：按中性占位显示，不出「需要文档组件」、不出下载 */
  pending?: boolean
}

export interface DocRow {
  src: DocSource
  records: DocRecord[]
  open: boolean
  isNew: boolean
}

const TERMINAL = ['succeeded', 'failed', 'canceled', 'interrupted']
const now = () => Date.now()

export const useDocConvertStore = defineStore('docConvert', () => {
  const comp = useDocComponentStore()
  const matrix = ref<DocFormatMatrix | null>(null)
  const matrixError = ref(false)
  const rows = ref<DocRow[]>([])
  const selected = reactive(new Set<string>())
  const target = ref('')
  const loaded = ref(false)
  const toast = ref<{ text: string; warn?: boolean; t: number } | null>(null)
  const roundBanner = ref<{ ok: number; fail: number } | null>(null)
  const queuePos = reactive<Record<string, number>>({})
  /** 事件先于行到达时按 id 暂存（契约 v0.25.1 第⑦条） */
  const pending = new Map<string, Partial<DocRecord>>()
  let inited = false
  const roundIds = ref<string[]>([])

  // ── 格式表 ──
  /** 组件整体状态还在检测中（有本机 Office / WPS 时整体已是 ready，不算） */
  const checking = computed(() => comp.status.state === 'checking')
  // 请求序号：doc:component 连发时只认比已用结果更新的响应，旧响应晚到直接丢（不倒退）
  let matrixSeq = 0
  let matrixApplied = 0
  /** 当前格式表是不是在「检测完成」后拿到的；是的话检测中再拉到的表不替换它（避免闪成不可用） */
  let matrixSettled = false
  async function loadMatrix() {
    const my = ++matrixSeq
    try {
      const m = await getFormatMatrix()
      if (my <= matrixApplied) return
      matrixApplied = my
      matrixError.value = false
      if (checking.value && matrix.value && matrixSettled) return
      matrix.value = m
      matrixSettled = !checking.value
    } catch (e) {
      console.error('GetFormatMatrix failed', e)
      if (my <= matrixApplied) return
      // 已有格式表就留着上一份，只有一份都没有时才出「重试」
      if (!matrix.value) matrixError.value = true
    }
  }

  function targetsOf(ext: string): DocTarget[] {
    const m = matrix.value
    if (!m) return []
    const s = m.sources.find((x) => x.ext === ext || x.aliases?.includes(ext))
    return s?.targets ?? []
  }

  const selectedRows = computed(() => rows.value.filter((r) => selected.has(r.src.sourceId)))
  const selectedExts = computed(() => selectedRows.value.map((r) => r.src.ext))
  const selectedFamilies = computed(() => selectedRows.value.map((r) => r.src.family))

  /** 选中文件可选目标的交集；没选时列出全部目标（55% 透明度、可点） */
  const tiles = computed<DocTile[]>(() => {
    const m = matrix.value
    if (!m) return []
    const exts = [...new Set(selectedExts.value)]
    let list: DocTarget[]
    if (!exts.length) {
      // 没选文件：列出全部目标；任何一种输入能用就算可用（组件未就绪时 PDF 只剩简易转换、HTML / MD 可用，其余置灰）
      const seen = new Map<string, DocTarget & { full?: boolean }>()
      for (const s of m.sources)
        for (const t of s.targets) {
          const cur = seen.get(t.ext) ?? { ...t, available: false, simple: false, hint: undefined, hintKey: undefined, full: false }
          if (t.available) {
            cur.available = true
            if (t.simple) cur.simple = !cur.full
            else {
              cur.full = true
              cur.simple = false
            }
          }
          seen.set(t.ext, cur)
        }
      list = [...seen.values()].map(({ full: _f, ...t }) => t)
    } else {
      const per = exts.map(targetsOf)
      list = per[0]
        .filter((t) => per.every((p) => p.some((x) => x.ext === t.ext)))
        .map((t) => {
          const all = per.map((p) => p.find((x) => x.ext === t.ext)!)
          const available = all.every((x) => x.available)
          return {
            ...t,
            available,
            simple: all.some((x) => x.simple),
            needsComponent: all.some((x) => x.needsComponent),
            hintKey: all.find((x) => x.hintKey)?.hintKey,
            hint: all.find((x) => x.hint)?.hint,
            disabledReason: available ? undefined : all.find((x) => x.disabledReason)?.disabledReason || '需要文档组件',
          }
        })
    }
    const order = ['pdf', 'docx', 'doc', 'odt', 'rtf', 'txt', 'html', 'md', 'xlsx', 'xls', 'ods', 'csv', 'pptx', 'ppt', 'odp']
    const chk = checking.value
    return list
      .map((t) => ({
        ...t,
        group: t.ext === 'pdf' ? ('pdf' as const) : docFamilyOf(t.ext),
        pending: chk && (!t.available || (t.simple && t.ext === 'pdf')),
      }))
      .sort((a, b) => order.indexOf(a.ext) - order.indexOf(b.ext))
  })

  const groups = computed(() =>
    DOC_GROUPS.map((g) => ({ ...g, items: tiles.value.filter((t) => t.group === g.key) })).filter((g) => g.items.length),
  )

  const currentTile = computed(() => tiles.value.find((t) => t.ext === target.value) ?? null)

  /** 交集说明：跨类时一行；同类不同扩展名时也一行（设计 v0.1 定） */
  const whyText = computed(() => {
    if (selectedRows.value.length < 2) return ''
    const fams = [...new Set(selectedFamilies.value)]
    if (fams.length > 1) return intersectionWhy(fams)
    const labels: Record<string, string> = {}
    for (const e of selectedExts.value) labels[e] = e.toUpperCase()
    return sameFamilyWhy(selectedExts.value, labels)
  })

  /** 说明行（格式区下面） */
  const note = computed<{ text: string; tone: 'info' | 'warn'; download?: boolean } | null>(() => {
    const n = rawNote.value
    // 检测中：不出「需要文档组件」/ 简易转换提醒 / 下载入口，检测完再按结果显示
    if (n && checking.value && (n.tone === 'warn' || n.download || currentTile.value?.pending)) return null
    return n
  })
  const rawNote = computed<{ text: string; tone: 'info' | 'warn'; download?: boolean } | null>(() => {
    const t = currentTile.value
    if (!selectedRows.value.length) return null
    // v0.28：选中里有 PDF 时按 PDF 的提示（排版 / 只提取文字），文案在 utils/docV26Text.ts
    const pdfNote = docPdfNote(selectedFamilies.value, t, comp.isLinux)
    if (pdfNote) return pdfNote
    // pdf_text：PDF → txt / md（产品定稿）；万一没走 docPdfNote 也直接映射，绝不落到 md_lossy
    if (t?.hintKey === 'pdf_text' && t.available) return { text: DOC_PDF_TEXT_ONLY_HINT, tone: 'info' }
    if (t?.simple) return { text: `${t.hint || DOC_SIMPLE_HINT}。`.replace(/。。$/, '。'), tone: 'warn', download: !comp.isLinux }
    if (t?.ext === 'md' && t.available) return { text: t.hint || DOC_MD_HINT, tone: 'info' }
    if (t?.ext === 'csv' && t.available) {
      const multi = selectedRows.value.some((r) => r.src.family === 'sheet' && r.src.ext !== 'csv' && (r.src.sheetCount > 1 || r.src.sheetCount === -1))
      if (multi) return { text: t.hint || DOC_CSV_HINT, tone: 'info' }
    }
    if (!comp.ready && tiles.value.some((x) => !x.available)) return { text: '其他格式需要文档组件。', tone: 'warn', download: !comp.isLinux }
    return null
  })

  const canSubmit = computed(() => selectedRows.value.length > 0 && !!currentTile.value?.available && !currentTile.value.pending)
  const noneUsable = computed(() => selectedRows.value.length > 0 && !checking.value && !tiles.value.some((t) => t.available))

  function ensureTarget() {
    const t = tiles.value
    if (!selectedRows.value.length) return
    if (currentTile.value?.available) return
    // 检测中暂时不可用的目标先留着，检测完再决定（不在检测中途把用户选的格式换掉）
    if (currentTile.value?.pending) return
    target.value = t.find((x) => x.ext === 'pdf' && x.available)?.ext ?? t.find((x) => x.available)?.ext ?? ''
  }

  function pickTarget(ext: string) {
    const t = tiles.value.find((x) => x.ext === ext)
    if (!t || (!t.available && !t.pending && selectedRows.value.length)) return
    target.value = ext
  }

  function toggle(id: string) {
    if (selected.has(id)) selected.delete(id)
    else selected.add(id)
    ensureTarget()
  }
  function clearSelection() {
    selected.clear()
  }

  // ── 记录 / 排队 ──
  function findRecord(id: string): { row: DocRow; rec: DocRecord } | null {
    for (const row of rows.value) {
      const rec = row.records.find((r) => r.id === id)
      if (rec) return { row, rec }
    }
    return null
  }

  function applyPatch(id: string, patch: Partial<DocRecord>) {
    const f = findRecord(id)
    if (!f) {
      pending.set(id, { ...(pending.get(id) ?? {}), ...patch })
      return
    }
    if (TERMINAL.includes(f.rec.status) && patch.status && !TERMINAL.includes(patch.status)) {
      // 重试会回到 queued；允许
    }
    Object.assign(f.rec, patch)
    if (patch.status && patch.status !== 'queued') delete queuePos[id]
    if (typeof patch.queuePosition === 'number') queuePos[id] = patch.queuePosition
    if (patch.status && TERMINAL.includes(patch.status)) checkRound()
  }

  function addRecord(rec: DocRecord) {
    const row = rows.value.find((r) => r.src.sourceId === rec.sourceId)
    if (!row) return
    if (row.records.some((r) => r.id === rec.id)) return
    row.records.unshift(rec)
    row.open = true
    if (typeof rec.queuePosition === 'number') queuePos[rec.id] = rec.queuePosition
    const p = pending.get(rec.id)
    if (p) {
      pending.delete(rec.id)
      applyPatch(rec.id, p)
    }
  }

  const allRecords = computed(() => rows.value.flatMap((r) => r.records))
  const total = computed(() => {
    const running = allRecords.value.filter((r) => r.status === 'running').length
    const queued = allRecords.value.filter((r) => r.status === 'queued').length
    const done = allRecords.value.filter((r) => r.status === 'succeeded').length
    const inRound = roundIds.value.length ? roundIds.value.length : running + queued
    const roundDone = roundIds.value.length ? roundIds.value.map((id) => findRecord(id)?.rec).filter((r) => r && r.status === 'succeeded').length : 0
    return { running, queued, done, inRound, roundDone }
  })

  function checkRound() {
    if (!roundIds.value.length) return
    const recs = roundIds.value.map((id) => findRecord(id)?.rec).filter(Boolean) as DocRecord[]
    if (recs.some((r) => !TERMINAL.includes(r.status))) return
    roundBanner.value = { ok: recs.filter((r) => r.status === 'succeeded').length, fail: recs.filter((r) => r.status !== 'succeeded').length }
    roundIds.value = []
  }

  function recordQueueText(id: string): number | null {
    const v = queuePos[id]
    return typeof v === 'number' ? v : null
  }

  function recordError(r: DocRecord): { text: string; retryable: boolean } | null {
    if (!r.error || r.status !== 'failed') return null
    const e = r.error
    if (e.code === 'CONVERT_DISK_FULL') return { text: docDiskFullText(e.detail, e.message), retryable: true }
    const srcPath = rows.value.find((x) => x.src.sourceId === r.sourceId)?.src.name ?? ''
    return { text: docAddErrorText({ code: e.code, message: e.message, detail: e.detail }, srcPath, comp.isLinux), retryable: docErrorRetryable(e.code, e.detail) }
  }

  // ── 模拟推进 ──
  const simRunning = new Set<string>()
  function simPump() {
    const queued = allRecords.value.filter((r) => r.status === 'queued' && (r.params ?? '').includes('component')).sort((a, b) => a.createdAt - b.createdAt)
    while (simRunning.size < 2 && queued.length) {
      const r = queued.shift()!
      simStart(r.id, true)
    }
    allRecords.value
      .filter((r) => r.status === 'queued' && (r.params ?? '').includes('component'))
      .sort((a, b) => a.createdAt - b.createdAt)
      .forEach((r, i) => applyPatch(r.id, { queuePosition: i }))
  }
  function simStart(id: string, pooled: boolean) {
    if (pooled) simRunning.add(id)
    applyPatch(id, { status: 'running', progress: 0, startedAt: Date.now() })
    const f = findRecord(id)
    const name = f?.row.src.name ?? ''
    const dur = simParam('doc_scene') === 'running' ? 600_000 : 1800 + Math.random() * 1500
    setTimeout(() => {
      if (pooled) simRunning.delete(id)
      const pdfGo = /"engine":"go"/.test(f?.rec.params ?? '') && f?.row.src.family === 'pdf'
      if (pdfGo && /扫描|scan/i.test(name)) {
        // 模拟 6.12.62：扫描件取不出文字；有组件就改用组件（多等一会儿后成功），没有就 DOC_PDF_NO_TEXT
        if ((comp.status.componentState || comp.status.state) === 'ready') applyPatch(id, { status: 'succeeded', progress: 1, finishedAt: now(), result: { engine: 'component' } })
        else applyPatch(id, { status: 'failed', error: { code: 'DOC_PDF_NO_TEXT', message: '', detail: 'reason=no_text\nquality=empty' }, finishedAt: now() })
      } else if (/超时|timeout/i.test(name)) applyPatch(id, { status: 'failed', error: { code: 'DOC_TIMEOUT', message: '' }, finishedAt: now() })
      else if (/崩溃|crash/i.test(name)) applyPatch(id, { status: 'failed', error: { code: 'DOC_COMPONENT_CRASHED', message: '', detail: 'exit=1' }, finishedAt: now() })
      else {
        const eng = /"engine":"go"/.test(f?.rec.params ?? '') ? 'go' : simParam('engine') === 'office' ? 'office' : 'component'
        const simple = /"simple":true/.test(f?.rec.params ?? '') && f?.row.src.family === 'pdf' && /"target":"html"/.test(f?.rec.params ?? '')
        applyPatch(id, { status: 'succeeded', progress: 1, finishedAt: now(), result: { engine: eng, warnings: simple ? ['simple_fallback'] : [] } })
      }
      simPump()
    }, dur)
  }

  // ── 动作 ──
  async function addPaths(paths: string[]) {
    if (!paths.length) return
    try {
      const res = await addDocSources(paths)
      const other: string[] = []
      for (const r of res) {
        if (r.source) {
          const exists = rows.value.find((x) => x.src.sourceId === r.source!.sourceId)
          if (!exists) rows.value.unshift({ src: r.source, records: [], open: false, isNew: true })
          selected.add(r.source.sourceId)
        } else if (r.error) {
          other.push(docAddErrorText(r.error, r.path, comp.isLinux))
        }
      }
      if (other.length) toast.value = { text: other[0], warn: true, t: now() }
      ensureTarget()
    } catch (e) {
      const ae = toAppError(e)
      toast.value = { text: docErrorText(ae.code, ae.message), warn: true, t: now() }
    }
  }

  async function chooseFiles() {
    try {
      const paths = await pickFiles(docFileFilter(), true)
      await addPaths(paths)
    } catch (e) {
      toast.value = { text: toAppError(e).message, warn: true, t: now() }
    }
  }

  async function submit(outputDir = '') {
    const tile = currentTile.value
    if (!canSubmit.value || !tile) return
    const ids = selectedRows.value.map((r) => r.src.sourceId)
    roundBanner.value = null
    if (docV26IsReal()) {
      try {
        const r = await submitDocConvert({ sourceIds: ids, target: tile.ext, outputDir })
        for (const t of r.tasks) addRecord(t)
        roundIds.value = r.tasks.map((t) => t.id)
        if (r.skipped.length) toast.value = { text: `有 ${r.skipped.length} 个文件还没准备好，这次没有转换。`, t: now() }
      } catch (e) {
        const ae = toAppError(e)
        toast.value = { text: ae.code === 'CONVERT_DISK_FULL' ? docDiskFullText(ae.detail, ae.message) : docErrorText(ae.code, ae.message, comp.isLinux), warn: true, t: now() }
        return
      }
    } else {
      roundIds.value = []
      for (const row of selectedRows.value) {
        const per = targetsOf(row.src.ext).find((x) => x.ext === tile.ext)
        // v0.28：PDF → txt / md / 简易 html 走纯 Go（不排队）；PDF → Word 类 / 有组件的 html 进组件池。
        // 只看格式表的 needsComponent / simple（engines 里没有 go，不从那里找）
        const pdfGo = row.src.family === 'pdf' && !!per && per.available && !per.needsComponent && per.simple
        const pooled = row.src.family === 'pdf' ? !pdfGo : !!per && per.needsComponent && !per.simple
        const id = `simdoc-${Math.random().toString(36).slice(2, 9)}`
        const base = row.src.name.replace(/\.[^.]+$/, '')
        const rec: DocRecord = {
          id,
          type: per?.simple && row.src.family !== 'pdf' ? 'office_pdf' : 'doc_convert',
          status: 'queued',
          title: `${base}.${tile.ext}`,
          outputPath: `D:\\Tools\\FFmpegFree\\output\\${base}.${tile.ext}`,
          progress: 0,
          params: JSON.stringify({ target: tile.ext, engine: pooled ? 'component' : 'go', simple: !!per?.simple }),
          sourceId: row.src.sourceId,
          createdAt: now() + roundIds.value.length,
        }
        addRecord(rec)
        roundIds.value.push(id)
        if (!pooled) simStart(id, false)
      }
      simPump()
    }
    selected.clear()
  }

  async function retry(id: string) {
    const f = findRecord(id)
    if (!f) return
    if (docV26IsReal()) {
      try {
        await retryDocRecord(id)
      } catch (e) {
        toast.value = { text: docErrorText(toAppError(e).code, toAppError(e).message), warn: true, t: now() }
      }
      return
    }
    const pooled = (f.rec.params ?? '').includes('component')
    applyPatch(id, { status: 'queued', error: null, progress: 0, startedAt: 0, createdAt: now() })
    if (!pooled) simStart(id, false)
    simPump()
  }

  /** 重转（ConvertService.Reconvert，契约 6.17；doc_convert 记录按当时的设置重新挑引擎） */
  async function reconvert(id: string) {
    const f = findRecord(id)
    if (!f) return
    if (docV26IsReal()) {
      try {
        await callService('ConvertService', 'Reconvert', { taskId: id })
      } catch (e) {
        toast.value = { text: docErrorText(toAppError(e).code, toAppError(e).message), warn: true, t: now() }
      }
      return
    }
    const pooled = (f.rec.params ?? '').includes('component')
    applyPatch(id, { status: 'queued', error: null, progress: 0, startedAt: 0, createdAt: now() })
    if (!pooled) simStart(id, false)
    simPump()
  }

  async function cancel(id: string) {
    if (docV26IsReal()) {
      await cancelDocRecord(id).catch((e) => (toast.value = { text: toAppError(e).message, warn: true, t: now() }))
      return
    }
    simRunning.delete(id)
    applyPatch(id, { status: 'canceled', finishedAt: now() })
    simPump()
  }

  async function removeRecord(id: string) {
    const f = findRecord(id)
    if (!f) return
    if (docV26IsReal()) await deleteDocRecords([id]).catch(() => {})
    f.row.records = f.row.records.filter((r) => r.id !== id)
  }

  async function removeSource(sourceId: string) {
    if (docV26IsReal()) await deleteDocSource(sourceId).catch(() => {})
    rows.value = rows.value.filter((r) => r.src.sourceId !== sourceId)
    selected.delete(sourceId)
  }

  // ── 初始化 ──
  async function reload() {
    if (docV26IsReal()) {
      try {
        const page = await listDocSources({ limit: 50, offset: 0, recordLimit: 20 })
        rows.value = page.items.map((e) => ({ src: e.source, records: e.records ?? [], open: !!e.records?.length, isNew: false }))
        for (const e of page.items) for (const r of e.records ?? []) if (typeof r.queuePosition === 'number') queuePos[r.id] = r.queuePosition
      } catch (e) {
        console.error('ListDocSources failed', e)
      }
    } else {
      seedScene()
    }
    loaded.value = true
  }

  async function init() {
    if (inited) return
    inited = true
    // 先订阅事件，再取状态 / 格式表 / 列表（契约 6.12.15）
    comp.onReady(() => void loadMatrix())
    watchDocQueue((q) => {
      for (const it of q.items) {
        queuePos[it.id] = it.queuePosition
        const f = findRecord(it.id)
        if (f) f.rec.queuePosition = it.queuePosition
      }
    })
    watchDocTasks({
      created: (t) => {
        if ((t.type === 'doc_convert' || t.type === 'office_pdf') && t.sourceId) addRecord(t)
      },
      progress: (p) => applyPatch(p.id, { progress: p.progress }),
      status: (p) => {
        const { id, ...rest } = p
        applyPatch(id, rest)
      },
    })
    // 三个请求并行，各自到了就更新自己那块；格式区不等组件状态 / 文件列表（老板反馈：进页面格式出来慢）
    await Promise.allSettled([comp.init(), loadMatrix(), reload()])
    ensureTarget()
  }

  // ── 模拟场景（?doc_scene=…，走查 / 截图） ──
  function mkSrc(name: string, extra: Partial<DocSource> = {}): DocSource {
    const raw = (name.split('.').pop() ?? '').toLowerCase()
    const ext = raw === 'htm' ? 'html' : raw === 'markdown' ? 'md' : raw
    const fam = docFamilyOf(ext)
    return {
      sourceId: `seed-${name}`,
      originalPath: `D:\\资料\\${name}`,
      name,
      ext,
      family: fam,
      sheetCount: fam === 'sheet' ? (ext === 'csv' ? 1 : 3) : 0,
      copyState: 'ready',
      totalBytes: 2_400_000,
      lastActivityAt: now(),
      ...extra,
    }
  }
  function mkRec(src: DocSource, ext: string, status: DocRecord['status'], extra: Partial<DocRecord> = {}): DocRecord {
    const base = src.name.replace(/\.[^.]+$/, '')
    return {
      id: `seedrec-${src.name}-${ext}-${status}-${Math.random().toString(36).slice(2, 6)}`,
      type: 'doc_convert',
      status,
      title: `${base}.${ext}`,
      outputPath: `D:\\Tools\\FFmpegFree\\output\\${base}.${ext}`,
      progress: status === 'succeeded' ? 1 : 0,
      params: JSON.stringify({ target: ext, engine: 'component' }),
      sourceId: src.sourceId,
      createdAt: now(),
      startedAt: status === 'queued' ? 0 : now() - 42_000,
      finishedAt: TERMINAL.includes(status) ? now() : undefined,
      // v0.27：成功记录带引擎（?engine=office|wps 模拟本机引擎）
      result: status === 'succeeded' ? { engine: (extra.params ?? '').includes('"go"') ? 'go' : simParam('engine') === 'office' ? 'office' : simParam('engine') === 'wps' ? 'wps' : 'component' } : null,
      ...extra,
    }
  }
  function seedScene() {
    const scene = simParam('doc_scene') ?? (simParam('doc') ? 'files' : 'empty')
    const contract = mkSrc('合同-终版.docx')
    const report = mkSrc('季度汇报.pptx')
    const budget = mkSrc('2026 预算.xlsx')
    const notes = mkSrc('会议纪要.md')
    const odt = mkSrc('投标书.odt')
    const locked = mkSrc('薪酬方案.xlsx')
    const broken = mkSrc('旧报价-副本.docx')
    const big = mkSrc('产品手册-全册.docx')
    const oldDoc = mkSrc('2009 年度总结.doc')
    const row = (src: DocSource, records: DocRecord[] = [], open = true, isNew = false): DocRow => ({ src, records, open: open && records.length > 0, isNew })
    const sel = (...s: DocSource[]) => s.forEach((x) => selected.add(x.sourceId))
    switch (scene) {
      case 'empty':
        rows.value = []
        break
      case 'preview': {
        // v0.27 预览 / 编辑走查：各种类型各一个
        const html = mkSrc('活动通知.html', { totalBytes: 3_200 })
        const csv = mkSrc('订单-10月.csv', { totalBytes: 46_000 })
        const txt = mkSrc('说明.txt', { totalBytes: 1_800 })
        const md2 = mkSrc('会议纪要.md', { totalBytes: 12_000 })
        const longTxt = mkSrc('访问日志-长.txt', { totalBytes: 3_400_000 })
        const gone = mkSrc('旧稿-原文件不在.md', { totalBytes: 6_000 })
        rows.value = [
          row(contract, [mkRec(contract, 'pdf', 'succeeded')]),
          row(budget, [mkRec(budget, 'csv', 'succeeded')], false),
          row(md2, [mkRec(md2, 'html', 'succeeded', { params: JSON.stringify({ target: 'html', engine: 'go' }), result: { engine: 'go' } })], false),
          row(html, [], false),
          row(csv, [], false),
          row(txt, [], false),
          row(report, [], false),
          row(longTxt, [], false),
          row(gone, [], false),
        ]
        const pick = simParam('doc_sel')
        if (pick === 'all') sel(contract, budget, md2)
        break
      }
      case 'running': {
        const r1 = mkRec(contract, 'pdf', 'running')
        const r2 = mkRec(report, 'pdf', 'running')
        const q1 = mkRec(budget, 'pdf', 'queued', { queuePosition: 0 })
        const q2 = mkRec(odt, 'pdf', 'queued', { queuePosition: 1 })
        const md = mkRec(notes, 'html', 'succeeded', { params: JSON.stringify({ target: 'html', engine: 'go' }) })
        rows.value = [row(contract, [r1]), row(report, [r2]), row(budget, [q1]), row(odt, [q2]), row(notes, [md])]
        queuePos[q1.id] = 0
        queuePos[q2.id] = 1
        break
      }
      case 'failed': {
        rows.value = [
          row(locked, [mkRec(locked, 'pdf', 'failed', { error: { code: 'DOC_ENCRYPTED', message: '' } })]),
          row(broken, [mkRec(broken, 'pdf', 'failed', { error: { code: 'DOC_CORRUPT', message: '' } })]),
          row(big, [mkRec(big, 'pdf', 'failed', { error: { code: 'DOC_TIMEOUT', message: '' } })]),
          row(report, [mkRec(report, 'pdf', 'failed', { error: { code: 'DOC_COMPONENT_CRASHED', message: '', detail: 'exit=1' } })]),
          row(budget, [mkRec(budget, 'pdf', 'failed', { error: { code: 'CONVERT_DISK_FULL', message: '', detail: 'reason=no_space\nneedBytes=4509715660\nfreeBytes=1073741824' } })]),
          row(odt, [mkRec(odt, 'pdf', 'failed', { error: { code: 'SOMETHING_NEW', message: '' } })]),
          row(contract, [mkRec(contract, 'pdf', 'failed', { error: { code: 'DOC_ENGINE_BUSY', message: '' } })]),
          row(notes, [mkRec(notes, 'pdf', 'failed', { error: { code: 'DOC_PRESENTATION_BUSY', message: '' } })]),
        ]
        roundBanner.value = { ok: 0, fail: 8 }
        break
      }
      case 'csv':
        rows.value = [row(budget, [mkRec(budget, 'pdf', 'succeeded')]), row(report, [mkRec(report, 'pdf', 'succeeded')], false)]
        sel(budget)
        target.value = 'csv'
        break
      case 'docx':
        rows.value = [row(contract, [], false, true), row(notes, [mkRec(notes, 'html', 'succeeded', { params: JSON.stringify({ target: 'html', engine: 'go' }) })], false)]
        sel(contract)
        target.value = 'pdf'
        break
      case 'csv-one': {
        const one = mkSrc('客户名单.xlsx', { sheetCount: 1 })
        rows.value = [row(one, [], false, true), row(budget, [mkRec(budget, 'pdf', 'succeeded')], false)]
        sel(one)
        target.value = 'csv'
        break
      }
      case 'done':
        rows.value = [
          row(contract, [mkRec(contract, 'pdf', 'succeeded')]),
          row(report, [mkRec(report, 'pdf', 'succeeded')]),
          row(odt, [mkRec(odt, 'pdf', 'succeeded', { result: { engine: 'simple', warnings: ['simple_fallback'] } })]),
          row(budget, [mkRec(budget, 'pdf', 'succeeded')]),
        ]
        roundBanner.value = { ok: 4, fail: 0 }
        break
      case 'legacy': {
        const xls = mkSrc('2010 报表.xls')
        const ppt = mkSrc('旧版宣讲.ppt')
        rows.value = [row(oldDoc, [mkRec(oldDoc, 'docx', 'succeeded')]), row(xls, [mkRec(xls, 'xlsx', 'succeeded')]), row(ppt, [mkRec(ppt, 'pptx', 'succeeded')])]
        break
      }
      case 'pdf':
      case 'pdf-txt':
      case 'pdf-html': {
        // v0.28 PDF 源：选中一个 PDF，目标分别是 Word / TXT / HTML（?doc=missing 看没有组件时）
        const manual = mkSrc('产品说明书.pdf', { totalBytes: 8_600_000 })
        const scan = mkSrc('扫描合同.pdf', { totalBytes: 21_000_000 })
        const go = (ext: string, extra: Partial<DocRecord> = {}) =>
          mkRec(manual, ext, 'succeeded', { params: JSON.stringify({ target: ext, engine: 'go' }), result: { engine: 'go' }, ...extra })
        rows.value = [
          row(manual, [mkRec(manual, 'docx', 'succeeded', { result: { engine: simParam('engine') === 'office' ? 'office' : 'component' } }), go('txt'), go('html', { result: { engine: 'go', warnings: ['simple_fallback'] } })], true, true),
          row(scan, [mkRec(scan, 'txt', 'failed', { params: JSON.stringify({ target: 'txt', engine: 'go' }), error: { code: 'DOC_PDF_NO_TEXT', message: '', detail: 'reason=no_text\nquality=empty' } })]),
          row(contract, [mkRec(contract, 'pdf', 'succeeded')], false),
        ]
        sel(manual)
        target.value = scene === 'pdf-txt' ? 'txt' : scene === 'pdf-html' ? 'html' : 'docx'
        break
      }
      case 'pdf-mixed':
        rows.value = [row(mkSrc('产品说明书.pdf'), [], false, true), row(contract, [], false, true)]
        sel(rows.value[0].src, contract)
        target.value = 'docx'
        break
      case 'md':
      case 'docx-md':
        rows.value = [row(contract, [], false, true), row(report, [mkRec(report, 'pdf', 'succeeded')], false)]
        sel(contract)
        target.value = 'md'
        break
      case 'inter':
        rows.value = [row(contract, [], false, true), row(report, [mkRec(report, 'pdf', 'succeeded')], false), row(budget, [], false)]
        sel(contract, report)
        target.value = 'pdf'
        break
      case 'same':
        rows.value = [row(contract, [], false, true), row(odt, [], false, true)]
        sel(contract, odt)
        target.value = 'pdf'
        break
      case 'fb-word':
        rows.value = [row(contract, [mkRec(contract, 'pdf', 'succeeded', { type: 'office_pdf', params: JSON.stringify({ target: 'pdf', simple: true }) })]), row(notes, [], false)]
        sel(contract)
        target.value = 'pdf'
        break
      case 'fb-md':
        rows.value = [row(notes, [], false, true), row(contract, [], false)]
        sel(notes)
        target.value = 'html'
        break
      case 'fb-doc':
        rows.value = [row(oldDoc, [], false, true)]
        sel(oldDoc)
        break
      default:
        rows.value = [
          row(contract, [mkRec(contract, 'pdf', 'succeeded')]),
          row(report, [mkRec(report, 'pdf', 'succeeded')], false),
          row(budget, [mkRec(budget, 'csv', 'succeeded')], false),
          row(notes, [mkRec(notes, 'html', 'succeeded', { params: JSON.stringify({ target: 'html', engine: 'go' }) })], false),
        ]
    }
  }

  return {
    matrix,
    matrixError,
    checking,
    rows,
    selected,
    target,
    loaded,
    toast,
    roundBanner,
    queuePos,
    tiles,
    groups,
    currentTile,
    whyText,
    note,
    canSubmit,
    noneUsable,
    selectedRows,
    total,
    init,
    reload,
    loadMatrix,
    toggle,
    clearSelection,
    pickTarget,
    ensureTarget,
    addPaths,
    chooseFiles,
    submit,
    retry,
    reconvert,
    cancel,
    removeRecord,
    removeSource,
    recordError,
    recordQueueText,
  }
})
