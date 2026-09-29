<template>
  <div class="office" :style="dropStyle">
    <section class="panel" aria-labelledby="h-office">
      <div class="phead">
        <div class="ttl">
          <h2 id="h-office">Office 转 PDF<span class="exp-tag">{{ DOC_EXPERIMENTAL_LABEL }}</span></h2>
          <!-- 常驻说明（契约：仅提取文字，不保留图片和样式） -->
          <p class="exp-note">{{ DOC_EXPERIMENTAL_NOTE }}</p>
        </div>
        <span class="sp" />
        <button type="button" class="btn pri" :disabled="picking" @click="choose"><FIcon name="plus" :size="15" />转为 PDF</button>
      </div>

      <div v-if="demo" class="demo" role="status"><FIcon name="info" :size="14" />{{ DOC_DEMO_NOTE }}</div>

      <div class="dz" role="button" tabindex="0" aria-label="拖入 Word、Excel、PPT，或点击选择" @click="choose" @keydown.enter.prevent="choose" @keydown.space.prevent="choose">
        <div class="ic"><FIcon name="upload" :size="24" /></div>
        <b>拖入 Word、Excel、PPT，或点击选择</b>
        <small>支持 docx、xlsx、pptx，一次最多 {{ limits.maxInputsPerSubmit }} 个 · 本地转换，不上传</small>
      </div>

      <InlineError v-if="submitError" class="serr" code="DOC" :description="submitError" bare />
    </section>

    <section class="panel" aria-labelledby="h-recent">
      <div class="phead">
        <h2 id="h-recent">转换记录</h2>
        <span class="sub">{{ rows.length ? `${rows.length} 条` : '' }}</span>
      </div>
      <div v-if="!rows.length" class="empty">{{ loadingHistory ? '正在读取…' : '还没有转换记录' }}</div>
      <ul v-else class="list" aria-label="转换记录">
        <li v-for="r in rows" :key="r.id" class="ofile">
          <div class="fi" :class="r.kind.toLowerCase()" aria-hidden="true">{{ r.kind }}</div>
          <div class="m">
            <b :title="r.path">{{ r.name }}</b>
            <span :class="{ bad: r.state === 'failed' }">{{ r.detail }}</span>
          </div>
          <span class="tag" :class="r.tone">{{ r.tag }}</span>
          <div class="acts">
            <button v-if="r.state === 'done'" type="button" class="btn" @click="preview(r)"><FIcon name="doc" :size="15" />预览</button>
            <button v-if="r.state === 'done'" type="button" class="btn" @click="reveal(r)"><FIcon name="folder" :size="15" />打开所在文件夹</button>
            <button v-if="r.state === 'active'" type="button" class="btn" :disabled="tasks.isBusy(r.id)" @click="cancel(r)">取消</button>
            <button v-if="r.state === 'failed'" type="button" class="btn" :disabled="tasks.isBusy(r.id)" @click="retry(r)"><FIcon name="refresh" :size="15" />重试</button>
          </div>
        </li>
      </ul>
    </section>
  </div>
</template>

<script setup lang="ts">
// Office 转 PDF（契约 6.12）：选择 / 拖入 docx、xlsx、pptx → DocService.ConvertToPDF（先整体校验，一个不通过整体失败）→ 任务走 task:* 事件（任务 store），
// 刷新页面后 store 用 ListActive 接回进行中的任务，已结束的从 TaskService.List(office_pdf) 读回。错误文案统一走 errors/errorMessages.ts 的 docErrorText。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import InlineError from '@/components/common/InlineError.vue'
import { toAppError } from '@/api/call'
import { onFilesDropped } from '@/api/fileDrop'
import {
  DEFAULT_DOC_LIMITS, DEMO_OFFICE_PATHS, OFFICE_FILE_FILTER, convertToPDF, getDocCapabilities, isDocSim, listOfficeHistory, officeKind,
  type DocLimits,
} from '@/api/doc'
import { pickFiles, revealInFolder } from '@/api/system'
import { hasWailsBackend } from '@/services/wails'
import { normalizeTask, useTaskStore, type TaskItem } from '@/stores/tasks'
import { DOC_DEMO_NOTE, DOC_EXPERIMENTAL_LABEL, DOC_EXPERIMENTAL_NOTE, docErrorFile, docErrorText } from '@/errors/errorMessages'
import { fileBaseName } from '@/utils/format'
import { ElMessage } from 'element-plus'

const tasks = useTaskStore()
const router = useRouter()
const demo = isDocSim()
/** Wails 放置区标记：落在带这个样式的区域才回调 OnFileDrop（整个页面都是放置区） */
const dropStyle = { '--wails-drop-target': 'drop' } as Record<string, string>

const limits = ref<DocLimits>(DEFAULT_DOC_LIMITS)
const submitError = ref('')
const picking = ref(false)
const loadingHistory = ref(true)
const history = ref<TaskItem[]>([])

// ---- 提交 ----
async function submit(paths: string[]) {
  submitError.value = ''
  const list = [...new Set(paths)]
  if (!list.length) return
  if (list.length > limits.value.maxInputsPerSubmit) {
    submitError.value = `一次最多转换 ${limits.value.maxInputsPerSubmit} 个文件，请分批添加`
    return
  }
  try {
    const created = await convertToPDF(list, '') // 输出目录由后端解析：设置里的默认位置，没有则源文件所在文件夹
    tasks.track(created.map((t) => normalizeTask(t as unknown as TaskItem)))
  } catch (e) {
    const err = toAppError(e)
    const file = docErrorFile(err.code, err.detail)
    // 整体校验失败：detail 第一行是出错文件，不提交任何任务
    submitError.value = `${file ? `${file}：` : ''}${docErrorText(err.code, err.message, err.detail)}${list.length > 1 && file ? '。没有开始转换，请移除这个文件后重试' : ''}`
  }
}

async function choose() {
  if (picking.value) return
  picking.value = true
  try {
    // 浏览器演示环境没有系统对话框，给几个假路径
    const paths = hasWailsBackend() ? await pickFiles(OFFICE_FILE_FILTER, true) : DEMO_OFFICE_PATHS
    await submit(paths)
  } catch (e) {
    submitError.value = toAppError(e).message
  } finally {
    picking.value = false
  }
}

// ---- 列表：进行中的（任务 store，刷新后由 ListActive 接回）+ 已结束的（TaskService.List）----
interface Row {
  id: string
  name: string
  path: string
  kind: 'DOC' | 'XLS' | 'PPT'
  state: 'active' | 'done' | 'failed'
  tag: string
  tone: string
  detail: string
}

function toRow(t: TaskItem): Row {
  const input = t.inputPaths[0] ?? ''
  const name = fileBaseName(input || t.outputPath || t.title) || t.title
  const base = { id: t.id, name, path: input, kind: officeKind(input) }
  if (t.status === 'succeeded') {
    return { ...base, state: 'done', tag: '完成', tone: 'ok', detail: `已转换 · ${t.outputPath ? fileBaseName(t.outputPath) : 'PDF'}`, path: t.outputPath || input }
  }
  if (t.status === 'failed') {
    return { ...base, state: 'failed', tag: '失败', tone: 'fail', detail: docErrorText(t.error?.code ?? 'INTERNAL', t.error?.message, t.error?.detail) }
  }
  if (t.status === 'canceled' || t.status === 'interrupted') {
    return { ...base, state: 'failed', tag: t.status === 'canceled' ? '已取消' : '已中断', tone: 'q', detail: t.status === 'canceled' ? '已取消' : '应用退出时被中断，可以重试' }
  }
  if (t.status === 'running') {
    const pct = Math.max(0, Math.min(100, Math.round((t.progress || 0) * 100)))
    return { ...base, state: 'active', tag: `${pct}%`, tone: 'run', detail: '正在转换' }
  }
  return { ...base, state: 'active', tag: '等待', tone: 'q', detail: '排队中' }
}

const activeOffice = computed(() => tasks.active.filter((t) => t.type === 'office_pdf'))
const rows = computed<Row[]>(() => {
  const activeIds = new Set(activeOffice.value.map((t) => t.id))
  return [...activeOffice.value.map(toRow), ...history.value.filter((t) => !activeIds.has(t.id)).map(toRow)]
})

async function loadHistory() {
  try {
    history.value = (await listOfficeHistory(20)).map((t) => normalizeTask(t as unknown as TaskItem))
  } catch (e) {
    console.error('读取转换记录失败', e)
  } finally {
    loadingHistory.value = false
  }
}
// 任务进入 / 离开进行中列表时重读历史（终态事件之后记录才落库，稍后再读一次兜底）
let timer: ReturnType<typeof setTimeout> | undefined
watch(
  () => activeOffice.value.map((t) => t.id).join(','),
  () => {
    loadHistory()
    clearTimeout(timer)
    timer = setTimeout(loadHistory, 800)
  },
)

// ---- 行操作 ----
function preview(r: Row) {
  router.push({ path: '/docs/pdf', query: { path: r.path } })
}
async function reveal(r: Row) {
  try {
    await revealInFolder(r.path)
  } catch (e) {
    ElMessage.error(docErrorText(toAppError(e).code, toAppError(e).message, toAppError(e).detail))
  }
}
async function cancel(r: Row) {
  try {
    await tasks.cancel(r.id)
  } catch (e) {
    const err = toAppError(e)
    if (err.code !== 'TASK_CONFLICT' && err.code !== 'NOT_FOUND') ElMessage.error(docErrorText(err.code, err.message, err.detail)) // 已结束：以事件为准
  }
}
async function retry(r: Row) {
  try {
    await tasks.retry(r.id)
  } catch (e) {
    const err = toAppError(e)
    ElMessage.error(docErrorText(err.code, err.message, err.detail))
  }
}

let offDrop: () => void = () => {}
onMounted(async () => {
  offDrop = onFilesDropped((paths) => submit(paths))
  getDocCapabilities().then((c) => (limits.value = c.limits)).catch(() => undefined)
  await loadHistory()
})
onBeforeUnmount(() => {
  offDrop()
  clearTimeout(timer)
})
</script>

<style scoped>
.office {
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
}
.office.wails-drop-target-active .dz {
  border-color: var(--ff-primary);
  background: var(--ff-primary-soft);
}
.phead {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-3) var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
}
.phead h2 {
  margin: 0;
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  font-size: var(--ff-fs-md);
  font-weight: 600;
  color: var(--ff-text-1);
}
.panel {
  padding: 0;
}
.exp-tag {
  font-size: var(--ff-fs-xs);
  font-weight: 400;
  line-height: 18px;
  padding: 0 var(--ff-space-2);
  border-radius: var(--ff-radius-sm);
  color: var(--ff-warning-text);
  background: color-mix(in srgb, var(--ff-warning) 10%, transparent);
}
.exp-note {
  margin: var(--ff-space-1) 0 0;
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-text-2);
}
.sub {
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
}
.sp {
  flex: 1;
}
.demo {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  margin: var(--ff-space-3) var(--ff-space-4) 0;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.dz {
  margin: var(--ff-space-4);
  height: 120px;
  border: 1.5px dashed var(--ff-border);
  border-radius: var(--ff-radius-lg);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--ff-space-1);
  color: var(--ff-text-2);
  cursor: pointer;
  transition: border-color var(--ff-dur-fast) var(--ff-ease), background var(--ff-dur-fast) var(--ff-ease);
}
.dz:hover {
  border-color: var(--ff-primary);
}
.dz:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.dz .ic {
  color: var(--ff-primary-text);
}
.dz b {
  color: var(--ff-text-1);
  font-weight: 500;
}
.dz small {
  font-size: var(--ff-fs-xs);
}
.serr {
  margin: calc(-1 * var(--ff-space-2)) var(--ff-space-4) var(--ff-space-4);
}
.empty {
  padding: var(--ff-space-6) var(--ff-space-4);
  text-align: center;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.list {
  list-style: none;
  margin: 0;
  padding: var(--ff-space-3) var(--ff-space-4) var(--ff-space-4);
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-2);
}
.ofile {
  display: flex;
  align-items: center;
  gap: var(--ff-space-3);
  padding: var(--ff-space-3);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-lg);
}
.fi {
  width: 32px;
  height: 32px;
  border-radius: var(--ff-radius-md);
  display: grid;
  place-items: center;
  flex: none;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
}
.fi.doc {
  background: var(--ff-badge-bg);
}
.fi.xls {
  background: var(--ff-syntax-string);
}
.fi.ppt {
  background: var(--ff-syntax-number);
}
.m {
  flex: 1;
  min-width: 0;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.m b {
  display: block;
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.m .bad {
  color: var(--ff-danger-text);
}
.tag {
  height: 20px;
  padding: 0 7px;
  border-radius: var(--ff-radius-sm);
  font-size: var(--ff-fs-xs);
  display: inline-flex;
  align-items: center;
  flex: none;
}
.tag.ok {
  background: color-mix(in srgb, var(--ff-success) 14%, transparent);
  color: var(--ff-success-text);
}
.tag.run {
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
}
.tag.fail {
  background: color-mix(in srgb, var(--ff-danger) 14%, transparent);
  color: var(--ff-danger-text);
}
.tag.q {
  background: var(--ff-bg-hover);
  color: var(--ff-text-2);
}
.acts {
  display: flex;
  gap: var(--ff-space-2);
  flex: none;
}
.btn {
  height: 28px;
  padding: 0 12px;
  border-radius: var(--ff-radius-md);
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  display: inline-flex;
  align-items: center;
  gap: var(--ff-space-2);
  font: inherit;
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  cursor: pointer;
}
.btn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.btn:disabled {
  opacity: 0.45;
  cursor: default;
}
.btn.pri {
  background: var(--ff-badge-bg);
  border-color: var(--ff-badge-bg);
  color: var(--ff-on-primary);
}
.btn.pri:hover:not(:disabled) {
  background: var(--ff-primary-hover);
  border-color: var(--ff-primary-hover);
}
@media (prefers-reduced-motion: reduce) {
  .dz {
    transition: none;
  }
}
</style>
