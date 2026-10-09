<script setup lang="ts">
// 文档页统一预览弹窗（契约 6.12.32 / 6.12.37~6.12.52；设计 v0.3 §九 / §十一，场景 16~19、25~49）。
// 查看：pdf / raw(docx、xlsx) / text / md / html / csv / 各种状态；编辑：md（左写右预览）、csv（表格）、txt / html（源码）、docx（开关后）。
// 所有保存出错都保留编辑内容；错误只按 code + reason 映射，不显示后端原文。
import MotionMenu from '@/components/motion/MotionMenu.vue'
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import PreviewShell from '@/components/preview/PreviewShell.vue'
import MotionToast from '@/components/motion/MotionToast.vue'
import PreviewConfirm, { type ConfirmButton } from '@/components/preview/PreviewConfirm.vue'
import PvPdf from './PvPdf.vue'
import PvRawDocx from './PvRawDocx.vue'
import PvRawXlsx from './PvRawXlsx.vue'
import PvCsv from './PvCsv.vue'
import PvHtmlFrame from './PvHtmlFrame.vue'
import PvSourceEdit from './PvSourceEdit.vue'
import { usePreviewStore } from '@/stores/docPreview'
import { useDocComponentStore } from '@/stores/docComponent'
import { toAppError } from '@/api/call'
import { simParam } from '@/api/sim'
import { DOCX_EDIT_ENABLED } from '@/api/flags'
import {
  DOCX_EDIT_MAX_BYTES,
  extOf,
  fetchRawBytes,
  isSameFormat,
  openWithSystem,
  revealDoc,
  revealPath,
  saveDocText,
  saveDocTextAs,
  saveDocxBytes,
  saveFileDialog,
  saveFilters,
} from '@/api/docV27'
import {
  EDIT_FORMAT_TIP,
  EDIT_MISSING_BANNER,
  OVERWRITE_CONFIRM_BODY,
  OVERWRITE_CONFIRM_TITLE,
  PV_FAILED,
  PV_GENERATING,
  PV_GENERATING_SUB,
  PV_SIMPLE,
  PV_TRUNCATED,
  REOPEN_DISCARD_CONFIRM,
  SAVED_BACKUP_TOAST,
  SAVED_TOAST,
  SAVE_AS_UTF8_LABEL,
  UNMAPPED,
  UNSAVED_CONFIRM,
  editBlockTip,
  previewFailedNotice,
  previewUnavailableNotice,
  pvCsvFooter,
  pvIndexText,
  saveErrorView,
  savedAsToast,
  type PreviewNotice,
  type SaveAction,
  type SaveErrorView,
} from '@/utils/docV27Text'
import { formatBytes } from '@/utils/format'

const PvDocxEdit = defineAsyncComponent(() => import('./PvDocxEdit.vue'))
const st = usePreviewStore()
const comp = useDocComponentStore()
const pv = computed(() => st.preview)
const it = computed(() => st.current)

// ── 顶部栏 ──
const ext = computed(() => (pv.value?.ext || extOf(it.value?.name ?? '')).toLowerCase())
const TONE: Record<string, string> = { doc: 't-doc', docx: 't-doc', odt: 't-doc', rtf: 't-doc', txt: 't-txt', html: 't-web', htm: 't-web', md: 't-md', markdown: 't-md', xls: 't-sheet', xlsx: 't-sheet', ods: 't-sheet', csv: 't-sheet', ppt: 't-slide', pptx: 't-slide', odp: 't-slide', pdf: 't-pdf' }
const ICON: Record<string, string> = { 't-doc': 'doc', 't-txt': 'doc', 't-web': 'link', 't-md': 'edit', 't-sheet': 'list', 't-slide': 'monitor', 't-pdf': 'doc' }
const tone = computed(() => TONE[ext.value] ?? 't-doc')
const KIND_NAME: Record<string, string> = { md: 'Markdown', markdown: 'Markdown' }
const sub = computed(() => {
  const parts = [KIND_NAME[ext.value] ?? ext.value.toUpperCase(), formatBytes(pv.value?.sizeBytes || it.value?.sizeBytes || 0)]
  if (st.multi) parts.push(pvIndexText(st.index + 1, st.items.length))
  return parts.filter(Boolean).join(' · ')
})
const title = computed(() => pv.value?.name || it.value?.name || '')

// ── 状态 ──
const localFailed = ref(false)
const KNOWN_KINDS = ['pdf', 'raw', 'text', 'md', 'html', 'csv', 'unavailable']
const notice = computed<(PreviewNotice & { tone: string; icon: string }) | null>(() => {
  const p = pv.value
  if (st.callFailed || localFailed.value) return { text: PV_FAILED, tone: 'bad', icon: 'warn' }
  if (!p) return null
  if (p.state === 'failed') {
    const n = previewFailedNotice(p.error?.code)
    const lock = p.error?.code === 'DOC_ENCRYPTED'
    return { ...n, tone: lock ? 'lock' : n.retry ? 'warn' : 'bad', icon: lock ? 'lock' : 'warn' }
  }
  if (p.kind === 'unavailable') {
    const n = previewUnavailableNotice(p.reason, { linux: comp.isLinux, outdated: comp.compState === 'outdated', engineReady: comp.ready })
    return n.download ? { ...n, tone: 'info', icon: 'download' } : { ...n, tone: 'bad', icon: 'warn' }
  }
  if (!KNOWN_KINDS.includes(p.kind) || (p.state !== 'ready' && p.state !== 'generating')) return { text: PV_FAILED, tone: 'bad', icon: 'warn' }
  if (p.kind === 'raw' && !['docx', 'xlsx'].includes(ext.value)) return { text: PV_FAILED, tone: 'bad', icon: 'warn' }
  return null
})
const generating = computed(() => !notice.value && (pv.value?.state === 'generating' || (!pv.value && st.loading)))
const showContent = computed(() => !!pv.value && pv.value.state === 'ready' && !notice.value)
watch(() => pv.value?.previewId, () => {
  localFailed.value = false
  pages.value = 0
  page.value = 1
  zoom.value = null
})

// ── 缩放：null = 适合宽度（打开时默认）；A4 宽 794px = 100% ──
const zoom = ref<number | null>(null)
const bodyW = ref(1000)
const bodyEl = ref<HTMLElement | null>(null)
let ro: ResizeObserver | null = null
onMounted(() => {
  ro = new ResizeObserver(() => (bodyW.value = bodyEl.value?.clientWidth || 1000))
  if (bodyEl.value) ro.observe(bodyEl.value)
})
onBeforeUnmount(() => ro?.disconnect())
const paged = computed(() => pv.value?.kind === 'pdf' || (pv.value?.kind === 'raw' && ext.value === 'docx'))
const fitScale = computed(() => (paged.value ? Math.min(2, Math.max(0.5, (bodyW.value - 144) / 794)) : 1))
const scale = computed(() => zoom.value ?? fitScale.value)
const pct = computed(() => Math.round(scale.value * 100))
const zoomOff = computed(() => !showContent.value || mode.value === 'edit')
function step(d: number) {
  const cur = Math.round(scale.value * 10) / 10
  const next = Math.min(2, Math.max(0.5, Math.round((cur + d) * 10) / 10))
  zoom.value = next
}

// ── 页码（分页内容）──
const pages = ref(0)
const page = ref(1)

// ── 编辑 ──
type Mode = 'view' | 'edit'
const mode = ref<Mode>('view')
const draftText = ref('')
const draftRows = shallowRef<string[][]>([])
const dirty = ref(false)
const saving = ref(false)
const saveErr = ref<SaveErrorView | null>(null)
const revision = ref('')
const docxBytes = shallowRef<Uint8Array | null>(null)
const docxSavedOnce = ref(false)
const docxRef = ref<{ save(): Promise<Uint8Array | null> } | null>(null)
const lastOp = ref<(() => Promise<void>) | null>(null)

const isText = computed(() => ['text', 'md', 'html', 'csv'].includes(pv.value?.kind ?? ''))
const isDocx = computed(() => ext.value === 'docx')
const docxOn = computed(() => DOCX_EDIT_ENABLED || (import.meta.env.DEV && simParam('docx_edit') === '1') || simParam('docx_edit') === '1')
const missing = computed(() => pv.value?.editBlock === 'missing')
const canEdit = computed(() => {
  const p = pv.value
  if (!p || p.state === 'failed' || p.kind === 'unavailable') return false
  if (isDocx.value) return docxOn.value && !!p.rawUrl && (p.editable || missing.value) && p.sizeBytes <= DOCX_EDIT_MAX_BYTES
  if (!isText.value) return false
  return p.editable || missing.value
})
const editTip = computed(() => {
  const p = pv.value
  if (!p || canEdit.value) return ''
  if (isDocx.value && !docxOn.value) return EDIT_FORMAT_TIP
  if (isDocx.value && p.sizeBytes > DOCX_EDIT_MAX_BYTES) return editBlockTip('too_large', ext.value)
  return editBlockTip(p.editBlock || (isText.value ? '' : 'format'), ext.value) || EDIT_FORMAT_TIP
})
const showEditBtn = computed(() => !!pv.value && !notice.value)
const editingDocx = computed(() => mode.value === 'edit' && isDocx.value)

async function enterEdit() {
  const p = pv.value
  if (!p || !canEdit.value) return
  saveErr.value = null
  dirty.value = false
  docxSavedOnce.value = false
  revision.value = p.revision ?? ''
  if (isDocx.value) {
    try {
      docxBytes.value = await fetchRawBytes(p.rawUrl!, p.rawUrl!.startsWith('blob:') ? 0 : p.sizeBytes)
    } catch (e) {
      if (toAppError(e).code === 'NOT_FOUND') st.onUrlGone()
      else saveErr.value = { text: UNMAPPED, actions: [] }
      return
    }
  } else if (p.kind === 'csv') draftRows.value = (p.rows ?? []).map((r) => [...r])
  else draftText.value = p.text ?? ''
  mode.value = 'edit'
}
function leaveEdit() {
  mode.value = 'view'
  dirty.value = false
  saveErr.value = null
  docxBytes.value = null
  if (docxSavedOnce.value) void st.refresh()
}
watch(draftText, () => mode.value === 'edit' && (dirty.value = true))
function onRows(v: string[][]) {
  draftRows.value = v
  dirty.value = true
}

const target = () => (it.value?.req.sourceId ? { sourceId: it.value.req.sourceId } : { taskId: it.value?.req.taskId })
const textPayload = () => (pv.value?.kind === 'csv' ? { rows: draftRows.value } : { text: draftText.value })

// ── toast（弹窗内，可带「打开所在文件夹」）──
const toast = ref<{ text: string; path?: string } | null>(null)
let toastTimer = 0
function showToast(text: string, path?: string) {
  toast.value = { text, path }
  clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = null), 6000)
}
onBeforeUnmount(() => clearTimeout(toastTimer))

function fail(e: unknown, m: 'text' | 'textAs' | 'docx' | 'docxAs') {
  const ae = toAppError(e)
  if (ae.code === 'CANCELED') return
  saveErr.value = saveErrorView(ae.code, ae.reason, m, ae.message)
}

async function saveText(): Promise<boolean> {
  if (missing.value) return saveAs('keep')
  lastOp.value = async () => void (await saveText())
  saving.value = true
  saveErr.value = null
  try {
    const r = await saveDocText({ ...target(), revision: revision.value, ...textPayload() })
    revision.value = r.revision
    dirty.value = false
    st.markEdited(it.value?.req.taskId)
    showToast(SAVED_TOAST)
    void st.refresh()
    return true
  } catch (e) {
    fail(e, 'text')
    return false
  } finally {
    saving.value = false
  }
}

async function pickTarget(): Promise<string | null> {
  const name = pv.value?.name || it.value?.name || ''
  const path = await saveFileDialog(name, saveFilters(ext.value))
  if (!path) return null
  if (!isSameFormat(ext.value, extOf(path))) {
    saveErr.value = { text: '只能保存成同一种格式。', actions: [] }
    return null
  }
  return path
}
const baseName = (p: string) => p.replace(/^.*[\\/]/, '')

async function saveAs(encoding: 'keep' | 'utf8'): Promise<boolean> {
  if (isDocx.value) return saveDocx('save_as')
  lastOp.value = async () => void (await saveAs(encoding))
  saveErr.value = null
  let path: string | null
  try {
    path = await pickTarget()
  } catch (e) {
    fail(e, 'textAs')
    return false
  }
  if (!path) return false
  saving.value = true
  try {
    const r = await saveDocTextAs({ ...target(), targetPath: path, ...textPayload(), encoding })
    dirty.value = false
    showToast(savedAsToast(baseName(r.path || path)), r.path || path)
    await afterSaveAs(r.revision)
    return true
  } catch (e) {
    fail(e, 'textAs')
    return false
  } finally {
    saving.value = false
  }
}

/**
 * 另存为之后重新拿一次预览：如果另存为的目标就是这条记录自己的文件（后端允许，等于覆盖），
 * 新预览的 revision 会等于这次保存的 revision → 跟着换 revision（不然下次「保存」会误报「文件在别处被改过」），并记「在应用里改过」。
 * 目标是别处时新预览 revision 不变，什么都不动。
 */
async function afterSaveAs(savedRevision: string) {
  const p = await st.refresh()
  if (p && savedRevision && p.revision === savedRevision && savedRevision !== revision.value) {
    revision.value = savedRevision
    if (isDocx.value) docxSavedOnce.value = true
    st.markEdited(it.value?.req.taskId)
  }
}

async function saveDocx(m: 'overwrite' | 'save_as'): Promise<boolean> {
  lastOp.value = async () => void (await saveDocx(m))
  saveErr.value = null
  let path: string | undefined
  if (m === 'save_as') {
    try {
      path = (await pickTarget()) ?? undefined
    } catch (e) {
      fail(e, 'docxAs')
      return false
    }
    if (!path) return false
  } else if ((await ask({ title: OVERWRITE_CONFIRM_TITLE, text: OVERWRITE_CONFIRM_BODY, buttons: [{ label: '取消', value: 'cancel' }, { label: '覆盖', value: 'ok', kind: 'pri' }] })) !== 'ok') return false
  saving.value = true
  try {
    const bytes = await docxRef.value?.save()
    if (!bytes) throw new Error('empty')
    const r = await saveDocxBytes(bytes, { ...target(), mode: m, targetPath: path, revision: revision.value })
    dirty.value = false
    if (m === 'overwrite') {
      revision.value = r.revision
      docxSavedOnce.value = true
      st.markEdited(it.value?.req.taskId)
      showToast(SAVED_BACKUP_TOAST, r.backupPath || r.path)
    } else {
      showToast(savedAsToast(baseName(r.path || path || '')), r.path || path)
      await afterSaveAs(r.revision)
    }
    return true
  } catch (e) {
    fail(e, m === 'overwrite' ? 'docx' : 'docxAs')
    return false
  } finally {
    saving.value = false
  }
}

// 另存为：原文件不是 UTF-8 时多给一个「另存为 UTF-8」（设计 §十一「另存为：可以选编码 UTF-8」）
const asMenu = ref(false)
const offerUtf8 = computed(() => !isDocx.value && !!pv.value?.encoding && pv.value.encoding !== 'utf8')
function onSaveAsClick() {
  if (saving.value) return
  if (offerUtf8.value) asMenu.value = !asMenu.value
  else void saveAs('keep')
}

async function onErrAction(a: SaveAction) {
  if (saving.value) return
  if (a === 'retry') await lastOp.value?.()
  else if (a === 'saveAs') await saveAs('keep')
  else if (a === 'saveAsUtf8') await saveAs('utf8')
  else if (a === 'reopen') {
    // 场景 35b：只问会不会丢改动（这时原位置保存不了，不走「要保存吗」）
    if ((await ask({ text: REOPEN_DISCARD_CONFIRM, buttons: [{ label: '取消', value: 'cancel' }, { label: '重新打开', value: 'ok', kind: 'danger' }] })) !== 'ok') return
    mode.value = 'view'
    dirty.value = false
    saveErr.value = null
    docxBytes.value = null
    await st.reload()
    await nextTick()
    if (canEdit.value) await enterEdit()
  }
}
const ERR_LABEL: Record<SaveAction, string> = { retry: '重试', saveAs: '另存为', saveAsUtf8: SAVE_AS_UTF8_LABEL, reopen: '重新打开' }

// ── 确认框 ──
const confirmBox = ref<{ title?: string; text: string; sub?: string; buttons: ConfirmButton[]; resolve: (v: string) => void } | null>(null)
function ask(o: { title?: string; text: string; sub?: string; buttons: ConfirmButton[] }): Promise<string> {
  return new Promise((resolve) => (confirmBox.value = { ...o, resolve }))
}
function onPick(v: string) {
  const c = confirmBox.value
  confirmBox.value = null
  c?.resolve(v)
}

/** 有改动时先问（场景 33）：保存成功 / 不保存 → true；取消或保存失败 → false */
async function guard(): Promise<boolean> {
  if (mode.value !== 'edit' || !dirty.value) return true
  const v = await ask({ text: UNSAVED_CONFIRM, sub: title.value, buttons: [{ label: '取消', value: 'cancel' }, { label: '不保存', value: 'discard' }, { label: '保存', value: 'save', kind: 'pri' }] })
  if (v === 'discard') return true
  if (v !== 'save') return false
  if (isDocx.value) return saveDocx('save_as')
  return saveText()
}
async function requestClose() {
  if (confirmBox.value) return onPick('cancel')
  if (saving.value) return
  if (!(await guard())) return
  mode.value = 'view'
  void st.close()
}
async function cancelEdit() {
  if (saving.value) return
  if (!(await guard())) return
  leaveEdit()
}
async function nav(d: number) {
  if (!(await guard())) return
  if (mode.value === 'edit') leaveEdit()
  st.go(d)
}

// ── 打开 / 显示 ──
const openSys = () => it.value && void openWithSystem(it.value.req).catch(() => showToast(UNMAPPED))
const reveal = () => it.value && void revealDoc(it.value.req).catch(() => showToast(UNMAPPED))
const revealSaved = (p: string) => void revealPath(p).catch(() => showToast(UNMAPPED))
function onDownload() {
  void comp.install()
  void st.close()
}
const retryPreview = () => {
  localFailed.value = false
  void st.reload()
}
const onGone = () => st.onUrlGone()
</script>

<template>
  <PreviewShell
    :title="title"
    :sub="sub"
    :tone="tone"
    :icon="ICON[tone]"
    :multi="st.multi"
    :can-prev="st.index > 0"
    :can-next="st.index < st.items.length - 1"
    :lock-nav="mode === 'edit'"
    @close="requestClose"
    @prev="nav(-1)"
    @next="nav(1)"
  >
    <template #actions>
      <template v-if="mode === 'view'">
        <div class="pvx-zm" role="group" aria-label="缩放">
          <button type="button" class="pvx-ib" aria-label="缩小" :aria-disabled="zoomOff || scale <= 0.5 || undefined" @click="!zoomOff && scale > 0.5 && step(-0.1)"><FIcon name="zout" /></button>
          <span class="zv" :aria-disabled="zoomOff || undefined" aria-live="polite">{{ zoomOff ? '—' : pct + '%' }}</span>
          <button type="button" class="pvx-ib" aria-label="放大" :aria-disabled="zoomOff || scale >= 2 || undefined" @click="!zoomOff && scale < 2 && step(0.1)"><FIcon name="zin" /></button>
          <button type="button" class="btn sm" :aria-pressed="zoom === null" :aria-disabled="zoomOff || undefined" @click="!zoomOff && (zoom = null)"><FIcon name="split" :size="14" />适合宽度</button>
        </div>
        <span class="pvx-dv" />
        <span v-if="showEditBtn" class="pvx-tipwrap" :title="editTip || undefined">
          <button type="button" class="btn sm pvx-ed" :aria-disabled="!canEdit || undefined" :aria-describedby="editTip ? 'pvx-edtip' : undefined" @click="canEdit && enterEdit()"><FIcon name="edit" :size="14" />编辑</button>
          <span v-if="editTip" id="pvx-edtip" class="pvx-tip" role="tooltip">{{ editTip }}</span>
        </span>
        <button type="button" class="btn sm" @click="openSys"><FIcon name="link" :size="14" />用默认程序打开</button>
        <button type="button" class="btn sm" @click="reveal"><FIcon name="folder" :size="14" />打开所在文件夹</button>
      </template>
      <template v-else-if="editingDocx">
        <span v-if="dirty" class="pvx-dirty">已修改</span>
        <button type="button" class="btn sm" :aria-disabled="saving || undefined" @click="cancelEdit">{{ docxSavedOnce && !dirty ? '完成' : '取消' }}</button>
      </template>
      <template v-else>
        <span v-if="dirty" class="pvx-dirty">已修改</span>
        <span class="pvx-asw">
          <button type="button" class="btn sm" :aria-disabled="saving || undefined" :aria-expanded="offerUtf8 ? asMenu : undefined" @click="onSaveAsClick">另存为<FIcon v-if="offerUtf8" name="down" :size="12" /></button>
          <MotionMenu>
          <span v-if="asMenu" class="pvx-menu" role="menu" @mouseleave="asMenu = false">
            <button type="button" role="menuitem" @click="asMenu = false; saveAs('keep')">另存为</button>
            <button type="button" role="menuitem" @click="asMenu = false; saveAs('utf8')">{{ SAVE_AS_UTF8_LABEL }}</button>
          </span>
          </MotionMenu>
        </span>
        <button type="button" class="btn sm" :aria-disabled="saving || undefined" @click="cancelEdit">取消</button>
        <button type="button" class="btn sm pri" :aria-disabled="saving || missing || undefined" :title="missing ? EDIT_MISSING_BANNER : undefined" @click="!saving && !missing && saveText()">
          <span v-if="saving" class="pvx-spin sm" /><FIcon v-else name="check" :size="14" />{{ saving ? '正在保存…' : '保存' }}
        </button>
      </template>
    </template>

    <div ref="bodyEl" class="pvx-body">
      <!-- 编辑 -->
      <template v-if="mode === 'edit'">
        <div v-if="saveErr" class="pvx-serr" role="alert">
          <FIcon name="warn" /><span>{{ saveErr.text }}</span><span class="sp" />
          <button v-for="a in saveErr.actions" :key="a" type="button" class="btn sm" :class="{ pri: a === 'saveAs' || a === 'saveAsUtf8' }" :aria-disabled="saving || undefined" @click="onErrAction(a)">{{ ERR_LABEL[a] }}</button>
        </div>
        <div v-if="missing" class="pvx-rough" role="status"><FIcon name="info" />{{ EDIT_MISSING_BANNER }}</div>
        <template v-if="editingDocx">
          <PvDocxEdit v-if="docxBytes" ref="docxRef" :bytes="docxBytes" :title="title" @dirty="dirty = true" @failed="saveErr = { text: UNMAPPED, actions: [] }" />
          <div class="pvx-efoot">
            <span class="sp" />
            <button type="button" class="btn" :aria-disabled="saving || missing || undefined" @click="!saving && !missing && saveDocx('overwrite')">覆盖原文件</button>
            <button type="button" class="btn pri" :aria-disabled="saving || undefined" @click="!saving && saveDocx('save_as')"><span v-if="saving" class="pvx-spin sm" />{{ saving ? '正在保存…' : '另存为' }}</button>
          </div>
        </template>
        <PvCsv v-else-if="pv?.kind === 'csv'" :rows="draftRows" edit :scale="1" @update:rows="onRows" />
        <div v-else-if="pv?.kind === 'md'" class="pvx-split">
          <PvSourceEdit v-model="draftText" label="Markdown 源码" />
          <div class="pvx-live"><PvHtmlFrame :source="draftText" kind="md" :debounce="300" :title="title" /></div>
        </div>
        <PvSourceEdit v-else v-model="draftText" :label="pv?.kind === 'html' ? '网页源码' : '文本'" />
      </template>

      <!-- 查看：状态 -->
      <div v-else-if="notice" class="pvx-msg" :class="notice.tone" role="status">
        <span class="ic"><FIcon :name="(notice.icon as any)" /></span>
        <h5>{{ notice.text }}</h5>
        <div v-if="notice.retry || notice.download" class="acts">
          <button v-if="notice.download" type="button" class="btn pri" @click="onDownload"><FIcon name="download" :size="14" />{{ notice.download }}</button>
          <button v-if="notice.retry" type="button" class="btn pri" @click="retryPreview"><FIcon name="retry" :size="14" />重试</button>
        </div>
      </div>
      <div v-else-if="generating" class="pvx-msg" role="status" aria-live="polite">
        <span class="pvx-spin" />
        <h5>{{ PV_GENERATING }}</h5>
        <p>{{ PV_GENERATING_SUB }}</p>
      </div>

      <!-- 查看：内容 -->
      <template v-else-if="showContent && pv">
        <PvPdf v-if="pv.kind === 'pdf' && pv.url" :key="pv.previewId" :url="pv.url" :scale="scale" @pages="pages = $event" @page="page = $event" @gone="onGone" @failed="localFailed = true" />
        <template v-else-if="pv.kind === 'raw' && pv.url">
          <div class="pvx-rough" role="status"><FIcon name="info" />{{ PV_SIMPLE }}</div>
          <PvRawDocx v-if="ext === 'docx'" :key="pv.previewId" :url="pv.url" :size-bytes="pv.sizeBytes" :scale="scale" @pages="pages = $event" @page="page = $event" @gone="onGone" @failed="localFailed = true" />
          <PvRawXlsx v-else :key="pv.previewId" :url="pv.url" :size-bytes="pv.sizeBytes" :scale="scale" @gone="onGone" @failed="localFailed = true" />
        </template>
        <template v-else-if="pv.kind === 'csv'">
          <PvCsv :rows="pv.rows ?? []" :scale="scale" />
          <div v-if="(pv.totalRows ?? 0) > 1000 || pv.truncated" class="pvx-foot"><FIcon name="info" />{{ pvCsvFooter(pv.totalRows ?? 0) }}</div>
        </template>
        <template v-else-if="pv.kind === 'md' || pv.kind === 'html'">
          <PvHtmlFrame :source="pv.text ?? ''" :kind="pv.kind === 'md' ? 'md' : 'html'" :zoom="scale" :title="title" />
          <div v-if="pv.truncated" class="pvx-foot"><FIcon name="info" />{{ PV_TRUNCATED }}</div>
        </template>
        <template v-else-if="pv.kind === 'text'">
          <div class="pvx-scroll pvx-yscroll"><pre class="pvx-txt" :style="{ zoom: scale }">{{ pv.text }}</pre></div>
          <div v-if="pv.truncated" class="pvx-foot"><FIcon name="info" />{{ PV_TRUNCATED }}</div>
        </template>
        <div v-else class="pvx-msg bad" role="status"><span class="ic"><FIcon name="warn" /></span><h5>{{ PV_FAILED }}</h5></div>
        <div v-if="paged && pages > 0" class="pvx-pn" aria-live="polite">{{ page }} / {{ pages }}</div>
      </template>

      <MotionToast>
        <div v-if="toast" class="pvx-toast" role="status">
          <FIcon name="check" class="ff-check-draw" /><span>{{ toast.text }}</span>
          <a v-if="toast.path" role="button" tabindex="0" @click="revealSaved(toast.path)" @keydown.enter="revealSaved(toast.path)">打开所在文件夹</a>
        </div>
      </MotionToast>
    </div>

    <template #overlay>
      <PreviewConfirm v-if="confirmBox" :title="confirmBox.title" :text="confirmBox.text" :sub="confirmBox.sub" :buttons="confirmBox.buttons" @pick="onPick" />
    </template>
  </PreviewShell>
</template>
