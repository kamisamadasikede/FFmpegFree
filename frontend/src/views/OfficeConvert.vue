<template>
  <section class="panel main" :class="{ hover: simHover }" :style="dropStyle" aria-labelledby="h-office">
    <div class="head">
      <div class="t">
        <h2 id="h-office">Office 转 PDF</h2>
        <span v-if="experimental" class="exp">{{ DOC_EXPERIMENTAL_LABEL }}</span>
        <span class="sp" />
        <span v-if="!entries.length" class="cnt">未选择文件</span>
        <template v-else>
          <span class="cnt" :class="{ bad: invalidCount > 0 }">{{ countText }}</span>
          <button type="button" class="btn text" :disabled="picking" @click="choose">添加文件</button>
        </template>
      </div>
      <!-- 常驻说明：和「实验性」标签由同一个能力位（experimental）控制，任何状态都在 -->
      <p v-if="experimental" class="d">{{ DOC_EXPERIMENTAL_NOTE }}</p>
    </div>

    <div v-if="demo" class="demo" role="status"><FIcon name="info" :size="14" />{{ DOC_DEMO_NOTE }}</div>
    <div v-if="submitError" class="topErr" role="alert"><FIcon name="warn" :size="14" /><span>{{ SUBMIT_ERROR_TITLE }}。{{ submitError }}</span></div>

    <div v-if="!entries.length" class="drop" role="button" tabindex="0" aria-label="拖入 Word、Excel、PPT 文件，或选择文件" @keydown.enter.prevent="choose" @keydown.space.prevent="choose">
      <div class="ic"><FIcon name="upload" :size="24" /></div>
      <template v-if="hoverShown">
        <b>{{ DOC_DROP_HOVER }}</b>
      </template>
      <template v-else>
        <b>{{ DOC_DROP_TITLE }}</b>
        <span>支持 docx、xlsx、pptx，最多一次 {{ limits.maxInputsPerSubmit }} 个，本地转换不上传</span>
        <button type="button" class="btn pri" :disabled="picking" @click="choose">选择文件</button>
      </template>
    </div>

    <template v-else>
      <ul class="list" aria-label="文件列表">
        <li v-for="e in entries" :key="e.key" class="row" :class="{ badrow: !!e.invalid }">
          <div class="x" aria-hidden="true">{{ extBadge(e.path) }}</div>
          <div class="m">
            <MiddleEllipsis class="nm" :text="e.name" />
            <template v-for="v in [view(e)]" :key="'v' + e.key">
              <div v-if="e.invalid" class="err" role="alert"><FIcon name="warn" :size="14" /><span>{{ e.invalid }}</span></div>
              <div v-else-if="v.kind === 'pending'" class="s">等待开始</div>
              <div v-else-if="v.kind === 'queued'" class="s">{{ v.pos ? `排队中（第 ${v.pos} 位）` : '排队中' }}</div>
              <template v-else-if="v.kind === 'running'">
                <div class="s run">转换中 · {{ v.pct }}%</div>
                <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="v.pct" :aria-label="`${e.name} 转换进度`"><i :style="{ width: v.pct + '%' }" /></div>
              </template>
              <template v-else-if="v.kind === 'done'">
                <!-- 设计稿写「已完成 · PDF 86 KB」：任务没有产物大小，不编造，只显示「已完成」 -->
                <div class="s ok"><FIcon name="check" :size="14" />已完成</div>
                <div class="bt">
                  <button type="button" class="btn sm" @click="openPdf(e)">打开 PDF</button>
                  <button type="button" class="btn sm" @click="reveal(e)">打开所在文件夹</button>
                </div>
              </template>
              <template v-else-if="v.kind === 'failed'">
                <div class="err" role="alert"><FIcon name="warn" :size="14" /><span>{{ v.text }}</span></div>
                <div class="bt">
                  <button type="button" class="btn sm" :disabled="(!e.taskId && !e.fake) || tasks.isBusy(e.taskId)" @click="retry(e)">重试</button>
                  <button type="button" class="btn sm" :disabled="!e.taskId && !e.fake" @click="showLog(e)">查看日志</button>
                </div>
              </template>
              <div v-else-if="v.kind === 'canceled'" class="s">已取消 · 没有生成 PDF</div>
              <template v-else-if="v.kind === 'interrupted'">
                <div class="s int">已中断 · 应用退出，没有生成 PDF</div>
                <div class="bt"><button type="button" class="btn sm" :disabled="(!e.taskId && !e.fake) || tasks.isBusy(e.taskId)" @click="retry(e)">重试</button></div>
              </template>
            </template>
          </div>
          <div class="ac">
            <button v-if="isActive(e)" type="button" class="btn text rm" :disabled="!e.taskId || tasks.isBusy(e.taskId)" :aria-label="`取消 ${e.name}`" @click="cancel(e)">取消</button>
            <button v-else type="button" class="btn text rm" :aria-label="`移除 ${e.name}`" @click="removeEntry(e)">移除</button>
          </div>
        </li>
      </ul>
      <p v-if="invalidCount > 0" class="batch" role="alert">{{ DOC_BATCH_INVALID_HINT }}</p>
    </template>

    <div v-if="hoverShown && entries.length" class="hoverov" aria-hidden="true">{{ DOC_DROP_HOVER }}</div>

    <div class="foot">
      <div class="o">
        <span>输出位置：</span>
        <span class="pth" :title="outputText">{{ outputText }}</span>
        <button type="button" class="lk" :disabled="busy" @click="changeDir">更改</button>
      </div>
      <button v-if="!busy && entries.length && !pendingCount" type="button" class="btn text" @click="clearAll">清空列表</button>
      <button v-if="busy" type="button" class="btn" @click="router.push('/tasks')"><FIcon name="task" :size="15" />在任务中心查看</button>
      <button v-else-if="entries.length && !pendingCount" type="button" class="btn pri" :disabled="picking" @click="choose"><FIcon name="plus" :size="15" />继续添加文件</button>
      <button v-else type="button" class="btn pri" :disabled="!pendingCount || submitting" @click="start"><FIcon name="play" :size="15" />开始转换</button>
    </div>

    <el-dialog v-model="logOpen" title="查看日志" width="560px" append-to-body>
      <pre class="log selectable">{{ logText || '暂无日志' }}</pre>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
// Office 转 PDF（契约 6.12，设计说明 2.1~2.3）：选择 / 拖入 docx、xlsx、pptx 先作为「待转换」行放进本页列表（本地状态，不产生任务）→
// 点「开始转换」DocService.ConvertToPDF（先整体校验，一个不通过整体失败、不提交任何任务）→ 任务走 task:* 事件（任务 store）。
// 错误文案统一走 errors/errorMessages.ts 的 docErrorText。列表在 KeepAlive 下保留；刷新页面后 store 用 ListActive 接回进行中的任务。
import { computed, onActivated, onDeactivated, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import MiddleEllipsis from '@/components/docs/MiddleEllipsis.vue'
import { toAppError } from '@/api/call'
import { DEFAULT_DOC_LIMITS, DEMO_OFFICE_PATHS, OFFICE_FILE_FILTER, convertToPDF, getDocCapabilities, isDocSim, isExperimental, type DocLimits } from '@/api/doc'
import { simParam } from '@/api/sim'
import { getOutputDirShown, pickDirectory, pickFiles, revealInFolder } from '@/api/system'
import { hasWailsBackend } from '@/services/wails'
import { normalizeTask, useTaskStore, type TaskItem } from '@/stores/tasks'
import { useDocsStore, dropHandlers } from '@/stores/docs'
import {
  DOC_BATCH_INVALID_HINT, DOC_DEMO_NOTE, DOC_DROP_HOVER, DOC_DROP_TITLE, DOC_EXPERIMENTAL_LABEL, DOC_EXPERIMENTAL_NOTE, OUTPUT_DIR_DEFAULT_TEXT,
  SUBMIT_ERROR_TITLE, docErrorPath, docErrorText, publicErrorText } from '@/errors/errorMessages'
import { fileBaseName } from '@/utils/format'
import { extBadge } from '@/utils/docLogic'
import { ElMessage } from 'element-plus'

const tasks = useTaskStore()
const docs = useDocsStore()
const router = useRouter()
const demo = isDocSim()
/** Wails 放置区标记：落在带这个样式的区域才回调 OnFileDrop */
const dropStyle = { '--wails-drop-target': 'drop' } as Record<string, string>

// 浏览器模拟环境的预览参数（真实环境不读）：sim_hover=1 显示拖入悬停态；sim_seed=<pending|invalid|all> 预置各状态的行，给截图和评审用
const simHover = demo && simParam('sim_hover') === '1'

const limits = ref<DocLimits>(DEFAULT_DOC_LIMITS)
const experimental = ref(true) // GetDocCapabilities().experimental，缺省 true
const picking = ref(false)
const submitting = ref(false)
const submitError = ref('')
const outputDir = ref('') // 本次选的；空 = 用设置里的默认输出位置（后端解析），再空 = 应用的输出文件夹（v0.24.1）
const defaultDir = ref('')
const logOpen = ref(false)
const logText = ref('')

// ---- 列表 ----
interface FakeState {
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  progress?: number
  pos?: number
  error?: { code: string; message: string; detail?: string }
}
interface Entry {
  key: string
  path: string
  name: string
  taskId?: string
  /** 整体校验未通过时定位到的错误文案 */
  invalid?: string
  /** 仅模拟环境：固定某个状态（截图用） */
  fake?: FakeState
}
const entries = ref<Entry[]>([])
let seq = 0
const newEntry = (path: string, extra: Partial<Entry> = {}): Entry => ({ key: `e${++seq}`, path, name: fileBaseName(path), ...extra })

function addPaths(paths: string[]) {
  submitError.value = ''
  const have = new Set(entries.value.map((e) => e.path))
  for (const p of paths) {
    if (have.has(p)) continue
    have.add(p)
    entries.value.push(newEntry(p))
  }
}

type Kind = 'pending' | 'queued' | 'running' | 'done' | 'failed' | 'canceled' | 'interrupted'
interface View {
  kind: Kind
  pct: number
  pos: number
  text: string
}
function view(e: Entry): View {
  const t = e.fake ?? (e.taskId ? tasks.taskById(e.taskId) : undefined)
  if (!e.taskId && !e.fake) return { kind: 'pending', pct: 0, pos: 0, text: '' }
  const status = t?.status ?? 'queued'
  const pct = Math.max(0, Math.min(100, Math.round(((t?.progress ?? 0) as number) * 100)))
  const err = (t as { error?: FakeState['error'] | null } | undefined)?.error
  switch (status) {
    case 'running':
      return { kind: 'running', pct, pos: 0, text: '' }
    case 'succeeded':
      return { kind: 'done', pct: 100, pos: 0, text: '' }
    case 'failed':
      return { kind: 'failed', pct, pos: 0, text: docErrorText(err?.code ?? 'INTERNAL', err?.message, err?.detail, limits.value.maxPages) }
    case 'canceled':
      return { kind: 'canceled', pct, pos: 0, text: '' }
    case 'interrupted':
      return { kind: 'interrupted', pct, pos: 0, text: '' }
    default:
      return { kind: 'queued', pct: 0, pos: e.fake?.pos ?? (e.taskId ? tasks.queuePosition(e.taskId) : 0), text: '' }
  }
}
const isActive = (e: Entry) => {
  const k = view(e).kind
  return k === 'queued' || k === 'running'
}
const busy = computed(() => entries.value.some(isActive))
const pendingCount = computed(() => entries.value.filter((e) => view(e).kind === 'pending').length)
const invalidCount = computed(() => entries.value.filter((e) => e.invalid).length)
const countText = computed(() => {
  if (invalidCount.value) return `${invalidCount.value} 个文件不能转换`
  return pendingCount.value === entries.value.length ? `已选 ${entries.value.length} 个文件` : `${entries.value.length} 个文件`
})
const outputText = computed(() => outputDir.value || defaultDir.value || OUTPUT_DIR_DEFAULT_TEXT)

// ---- 拖入悬停（Wails 给放置区加 wails-drop-target-active 类，样式里处理；这里只为模拟环境和覆盖层文案）----
const hoverShown = computed(() => simHover)

// ---- 提交 ----
async function start() {
  if (submitting.value) return
  submitError.value = ''
  for (const e of entries.value) e.invalid = undefined
  const targets = entries.value.filter((e) => view(e).kind === 'pending')
  if (!targets.length) return
  if (targets.length > limits.value.maxInputsPerSubmit) {
    submitError.value = `一次最多转换 ${limits.value.maxInputsPerSubmit} 个文件，请分批添加`
    return
  }
  submitting.value = true
  try {
    const created = await convertToPDF(targets.map((e) => e.path), outputDir.value) // 输出目录空 = 由后端解析：设置里的默认位置，没有则应用的输出文件夹所在文件夹
    const items = created.map((t) => normalizeTask(t as unknown as TaskItem))
    tasks.track(items)
    targets.forEach((e, i) => {
      if (items[i]) e.taskId = items[i].id
    })
  } catch (e) {
    const err = toAppError(e)
    const text = docErrorText(err.code, err.message, err.detail, limits.value.maxPages)
    // 整体校验失败：detail 第一行是出错文件的路径（契约 6.12.3），不提交任何任务；据此把对应行标红
    const bad = docErrorPath(err.detail)
    const hit = bad ? targets.find((t) => t.path === bad) : undefined
    if (hit) hit.invalid = text
    else submitError.value = text // 定位不到具体文件：列表顶部显示通用错误
  } finally {
    submitting.value = false
  }
}

async function choose() {
  if (picking.value) return
  picking.value = true
  try {
    // 浏览器演示环境没有系统对话框，给几个假路径
    const paths = hasWailsBackend() ? await pickFiles(OFFICE_FILE_FILTER, true) : DEMO_OFFICE_PATHS
    addPaths(paths)
  } catch (e) {
    submitError.value = publicErrorText(toAppError(e).message)
  } finally {
    picking.value = false
  }
}

async function changeDir() {
  if (busy.value) return
  try {
    const dir = hasWailsBackend() ? await pickDirectory('选择输出位置') : '/Users/me/Documents/转换输出'
    if (dir) outputDir.value = dir // 只改本次，不写设置
  } catch (e) {
    const err = toAppError(e)
    ElMessage.error(docErrorText(err.code, err.message, err.detail))
  }
}

// ---- 行操作 ----
function removeEntry(e: Entry) {
  entries.value = entries.value.filter((x) => x.key !== e.key)
  if (!entries.value.length) submitError.value = ''
}
function clearAll() {
  entries.value = []
  submitError.value = ''
}
function openPdf(e: Entry) {
  const t = e.taskId ? tasks.taskById(e.taskId) : undefined
  // 打开 PDF = 在 PDF 预览里打开（预览加载时 OpenPDF 才登记句柄并写最近列表；不在转换成功时自动调用，见汇报）
  router.push({ path: '/docs/pdf', query: { path: t?.outputPath || e.path.replace(/\.[^./\\]+$/, '.pdf') } })
}
async function reveal(e: Entry) {
  const t = e.taskId ? tasks.taskById(e.taskId) : undefined
  try {
    await revealInFolder(t?.outputPath || e.path)
  } catch (err) {
    const a = toAppError(err)
    ElMessage.error(docErrorText(a.code, a.message, a.detail))
  }
}
async function cancel(e: Entry) {
  if (!e.taskId) return
  try {
    await tasks.cancel(e.taskId)
  } catch (err) {
    const a = toAppError(err)
    if (a.code !== 'TASK_CONFLICT' && a.code !== 'NOT_FOUND') ElMessage.error(docErrorText(a.code, a.message, a.detail)) // 已结束：以事件为准
  }
}
async function retry(e: Entry) {
  if (!e.taskId) return
  try {
    const nt = await tasks.retry(e.taskId)
    if (nt) e.taskId = nt.id
  } catch (err) {
    const a = toAppError(err)
    ElMessage.error(docErrorText(a.code, a.message, a.detail))
  }
}
async function showLog(e: Entry) {
  if (!e.taskId) return
  try {
    logText.value = await tasks.getLog(e.taskId, 200)
  } catch (err) {
    const a = toAppError(err)
    logText.value = docErrorText(a.code, a.message, a.detail)
  }
  logOpen.value = true
}

// ---- 接回进行中的任务（刷新页面后 store 用 ListActive 恢复）----
function adoptActive() {
  const known = new Set(entries.value.map((e) => e.taskId).filter(Boolean))
  for (const t of tasks.active) {
    if (t.type !== 'office_pdf' || known.has(t.id)) continue
    const p = t.inputPaths[0] ?? t.title
    entries.value.push(newEntry(p, { taskId: t.id }))
  }
}

function seedSim(kind: string) {
  const d = '/Users/me/Documents/'
  const mk = (n: string, extra: Partial<Entry> = {}) => newEntry(d + n, extra)
  if (kind === 'pending') {
    entries.value = [mk('用户调研报告.docx'), mk('渠道数据汇总.xlsx'), mk('2026年第三季度华东区域渠道商务拓展与用户增长复盘汇报材料（终稿-已审阅-v12）.pptx')]
  } else if (kind === 'invalid' || kind === 'invalid2') {
    entries.value = [
      mk('用户调研报告.docx'),
      mk(kind === 'invalid' ? '旧版报价单.doc' : '损坏的合同.xlsx', { invalid: kind === 'invalid' ? docErrorText('UNSUPPORTED', '暂不支持这种格式', 'reason=format\n/Users/me/Documents/旧版报价单.doc\n.doc：旧版') : docErrorText('INVALID_ARGUMENT', '不是有效的 OOXML 文件', 'reason=invalid_ooxml\n/Users/me/Documents/损坏的合同.xlsx\nzip: not a valid zip file') }),
      mk('2026 Q3 产品回顾.pptx'),
    ]
  } else if (kind === 'running') {
    entries.value = [
      mk('用户调研报告.docx', { fake: { status: 'running', progress: 0.42 } }),
      mk('渠道数据汇总.xlsx', { fake: { status: 'queued', pos: 1 } }),
      mk('2026 Q3 产品回顾.pptx', { fake: { status: 'queued', pos: 2 } }),
    ]
  } else if (kind === 'done') {
    entries.value = [mk('用户调研报告.docx', { fake: { status: 'succeeded' } }), mk('渠道数据汇总.xlsx', { fake: { status: 'succeeded' } }), mk('2026 Q3 产品回顾.pptx', { fake: { status: 'succeeded' } })]
  } else if (kind === 'failed') {
    entries.value = [
      mk('用户调研报告.docx', { fake: { status: 'failed', error: { code: 'UNSUPPORTED', message: '超过 5000 页', detail: 'reason=too_many_pages\n已排到第 5000 页仍未结束' } } }),
      mk('渠道数据汇总.xlsx', { fake: { status: 'failed', error: { code: 'INVALID_ARGUMENT', message: '不是有效的 OOXML 文件', detail: 'reason=invalid_ooxml\n缺少 xl/workbook.xml' } } }),
      mk('2026年第三季度华东区域渠道商务拓展与用户增长复盘汇报材料（终稿-已审阅-v12）.pptx', { fake: { status: 'failed', error: { code: 'CONVERT_DISK_FULL', message: '磁盘空间不足，无法写入输出文件' } } }),
    ]
  } else if (kind === 'canceled') {
    entries.value = [mk('用户调研报告.docx', { fake: { status: 'canceled' } }), mk('渠道数据汇总.xlsx', { fake: { status: 'canceled' } })]
  } else if (kind === 'all') {
    entries.value = [
      mk('2026 Q3 产品回顾.pptx', { fake: { status: 'succeeded' } }),
      mk('用户调研报告.docx', { fake: { status: 'running', progress: 0.67 } }),
      mk('渠道数据汇总.xlsx', { fake: { status: 'queued', pos: 1 } }),
      mk('季度复盘.pptx', { fake: { status: 'queued', pos: 2 } }),
      mk('合同附件.docx', { fake: { status: 'failed', error: { code: 'UNSUPPORTED', message: '超过 5000 页', detail: 'reason=too_many_pages\n文档文字量超过上限' } } }),
      mk('费用明细.xlsx', { fake: { status: 'failed', error: { code: 'INVALID_ARGUMENT', message: '不是有效的 OOXML 文件' } } }),
      mk('培训材料.pptx', { fake: { status: 'failed', error: { code: 'CONVERT_DISK_FULL', message: '磁盘空间不足，无法写入输出文件' } } }),
      mk('会议纪要.docx', { fake: { status: 'canceled' } }),
      mk('周报.docx', { fake: { status: 'interrupted' } }),
    ]
  }
}

function attachDrop() {
  dropHandlers.office = (paths) => addPaths(paths)
}
onMounted(() => {
  attachDrop()
  getDocCapabilities()
    .then((c) => {
      limits.value = c.limits
      experimental.value = isExperimental(c)
    })
    .catch(() => undefined)
  if (demo) {
    const seed = simParam('sim_seed')
    if (seed) seedSim(seed)
  }
})
onActivated(() => {
  attachDrop()
  adoptActive()
  getOutputDirShown().then((d) => (defaultDir.value = d)).catch(() => undefined)
  docs.loadRecent()
})
onDeactivated(() => {
  if (dropHandlers.office) delete dropHandlers.office
})
</script>

<style scoped>
.main {
  flex: 1;
  min-width: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}
.main.wails-drop-target-active,
.main.hover {
  border-color: var(--ff-primary);
}
.head {
  flex: none;
  padding: 12px 16px;
  border-bottom: 1px solid var(--ff-border);
}
.head .t {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 24px;
}
.head h2 {
  margin: 0;
  font-size: var(--ff-fs-md);
  font-weight: 600;
  color: var(--ff-text-1);
  white-space: nowrap;
}
.sp {
  flex: 1;
}
.exp {
  height: 20px;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  display: inline-flex;
  align-items: center;
  white-space: nowrap;
  color: var(--ff-warning-text);
  background: color-mix(in srgb, var(--ff-warning) 14%, transparent);
  border: 1px solid color-mix(in srgb, var(--ff-warning) 28%, transparent);
}
.head .d {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-text-2);
}
.cnt {
  font-size: 12px;
  color: var(--ff-text-2);
  white-space: nowrap;
}
.cnt.bad {
  color: var(--ff-danger-text);
}
.demo {
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-2) var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.topErr {
  flex: none;
  display: flex;
  align-items: flex-start;
  gap: 4px;
  padding: 8px 16px;
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-danger-text);
  border-bottom: 1px solid var(--ff-border);
}
.topErr svg {
  margin-top: 1px;
  flex: none;
}
.drop {
  flex: 1;
  min-height: 0;
  margin: 16px;
  border: 1.5px dashed var(--ff-border);
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  text-align: center;
  padding: 24px;
}
.main.wails-drop-target-active .drop,
.main.hover .drop {
  border-color: var(--ff-primary);
  background: color-mix(in srgb, var(--ff-primary) 8%, transparent);
}
.drop .ic {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
  display: grid;
  place-items: center;
  margin-bottom: 8px;
}
.drop b {
  font-size: 14px;
  font-weight: 600;
  color: var(--ff-text-1);
}
.drop span {
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-text-2);
}
.drop .btn {
  margin-top: 8px;
}
.hoverov {
  position: absolute;
  inset: 49px 0 57px;
  display: grid;
  place-items: center;
  background: color-mix(in srgb, var(--ff-primary) 8%, var(--ff-bg-surface));
  border: 1.5px dashed var(--ff-primary);
  border-radius: 10px;
  margin: 16px;
  font-size: 14px;
  font-weight: 600;
  color: var(--ff-primary-text);
  pointer-events: none;
}
.list {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}
.row {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr) auto;
  column-gap: 12px;
  align-items: start;
  padding: 12px 16px;
  border-bottom: 1px solid var(--ff-border);
}
.row:last-child {
  border-bottom: none;
}
.x {
  width: 40px;
  height: 32px;
  border-radius: 6px;
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
  display: grid;
  place-items: center;
  font-size: 12px;
  font-weight: 600;
}
.badrow .x {
  background: color-mix(in srgb, var(--ff-danger) 12%, transparent);
  color: var(--ff-danger-text);
}
.m {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nm {
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
  color: var(--ff-text-1);
}
.s {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-text-2);
  white-space: nowrap;
}
.s.run {
  color: var(--ff-primary-text);
  font-weight: 500;
}
.s.ok {
  color: var(--ff-success-text);
  font-weight: 500;
}
.s.int {
  color: var(--ff-interrupted);
  font-weight: 500;
}
.bar {
  height: 4px;
  border-radius: 2px;
  background: var(--ff-border);
  overflow: hidden;
  margin-top: 4px;
}
.bar i {
  display: block;
  height: 100%;
  background: var(--ff-primary);
  border-radius: 2px;
}
.bt {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
}
.err {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-danger-text);
}
.err svg {
  margin-top: 1px;
  flex: none;
}
.ac {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  height: 20px;
}
.batch {
  flex: none;
  margin: 0;
  padding: 8px 16px 12px;
  border-top: 1px solid var(--ff-border);
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-danger-text);
}
.foot {
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-top: 1px solid var(--ff-border);
}
.o {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  line-height: 16px;
  color: var(--ff-text-2);
  white-space: nowrap;
}
.o > span:first-child {
  flex: none;
  margin-right: -6px;
}
.pth {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--ff-text-1);
}
.lk {
  flex: none;
  padding: 0;
  border: 0;
  background: transparent;
  font: inherit;
  color: var(--ff-primary-text);
  cursor: pointer;
  border-radius: 2px;
}
.lk:disabled {
  color: var(--ff-text-3);
  cursor: not-allowed;
}
.lk:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
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
  cursor: not-allowed;
}
.btn.sm {
  height: 24px;
  padding: 0 8px;
  font-size: 12px;
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
.btn.text {
  border-color: transparent;
  background: transparent;
  color: var(--ff-primary-text);
}
.btn.text.rm {
  color: var(--ff-text-2);
  height: 24px;
  padding: 0 8px;
  font-size: 13px;
}
.btn.text.rm:hover:not(:disabled) {
  color: var(--ff-text-1);
  background: var(--ff-bg-hover);
}
.btn:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.log {
  margin: 0;
  max-height: 320px;
  overflow: auto;
  font-family: var(--ff-font-mono);
  font-size: 12px;
  line-height: 1.7;
  color: var(--ff-text-2);
  background: var(--ff-bg-app);
  border-radius: 6px;
  padding: 10px 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
