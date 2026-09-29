<template>
  <div class="tc">
    <!-- 统计条：运行中 / 排队中 / 今日完成 / 失败 -->
    <div class="stats">
      <div class="card stat"><small>运行中</small><b style="color: var(--ff-primary)">{{ tasks.runningOnly }}</b></div>
      <div class="card stat"><small>排队中</small><b>{{ tasks.queuedCount }}</b></div>
      <div class="card stat"><small>今日完成</small><b style="color: var(--ff-success)">{{ tasks.todayDone }}{{ tasks.todayDoneCapped ? '+' : '' }}</b></div>
      <div class="card stat"><small>失败</small><b style="color: var(--ff-danger)">{{ tasks.failedTotal }}</b></div>
    </div>

    <div class="card main">
      <div class="tabs" role="tablist">
        <span v-for="t in tabList" :key="t.key" role="tab" :aria-selected="tab === t.key" :class="{ on: tab === t.key }" @click="setTab(t.key)">
          {{ t.label }}<em>{{ t.count }}</em>
        </span>
        <span class="grow" />
        <template v-if="tab !== 'active'">
          <el-select v-model="typeFilter" size="small" class="type-select" @change="onTypeChange">
            <el-option v-for="o in TYPE_FILTERS" :key="o.key" :label="o.label" :value="o.key" />
          </el-select>
          <button class="btn" :disabled="!tasks.historyTotal && !tasks.finishedTotal" @click="askClear"><FIcon name="trash" />清除已结束</button>
        </template>
      </div>

      <div v-if="tasks.loadError && tab === 'active'" class="loaderr">
        <ErrorLine :code="tasks.loadError.code" :message="tasks.loadError.message" :show-log="false" />
      </div>

      <div class="scroll">
        <!-- 空状态 -->
        <div v-if="rows.length === 0 && !loading" class="empty">
          <div class="eic"><FIcon :name="tab === 'failed' ? 'check' : 'task'" :size="24" /></div>
          <b>{{ emptyText.title }}</b>
          <span>{{ emptyText.hint }}</span>
        </div>

        <table v-else class="tbl">
          <thead>
            <tr>
              <th style="width: 36%">任务</th>
              <th>类型</th>
              <th style="width: 24%">{{ tab === 'active' ? '进度' : '结果' }}</th>
              <th>{{ tab === 'active' ? '开始时间' : '完成时间' }}</th>
              <th class="opsh"></th>
            </tr>
          </thead>
          <tbody>
            <template v-for="t in rows" :key="t.id">
              <tr :class="{ sel: logId === t.id, 'has-err': hasErrLine(t) }">
                <td>
                  <div class="fname" :title="t.title">{{ t.title || fileBaseName(t.outputPath) }}</div>
                  <div class="finfo">{{ subInfo(t) }}</div>
                </td>
                <td>
                  <span v-if="isLiveType(t.type)" class="tag live">● 直播</span>
                  <span v-else class="tag" :class="typeTagClass(t)">{{ typeLabel(t.type) }}</span>
                </td>
                <td>
                  <!-- 进行中 -->
                  <template v-if="t.status === 'running' && !isLiveType(t.type)">
                    <div class="prog">
                      <div class="pline"><span>{{ progressText(t) }}</span><span>{{ percent(t) }}%</span></div>
                      <div class="bar"><i :style="{ width: percent(t) + '%' }" /></div>
                    </div>
                  </template>
                  <span v-else-if="t.status === 'running'" class="plain">已推流 {{ formatClock(liveSeconds(t)) }} · 不占转换名额</span>
                  <span v-else-if="t.status === 'queued'" class="plain dim">{{ queueText(t) }}</span>
                  <!-- 已结束 -->
                  <template v-else>
                    <span class="tag" :class="STATUS_TAG[t.status].cls">{{ STATUS_TAG[t.status].label }}</span>
                    <span v-if="t.status === 'succeeded' && t.startedAt && t.finishedAt" class="dur">用时 {{ formatDuration(t.finishedAt - t.startedAt) }}</span>
                  </template>
                </td>
                <td class="when" :class="{ dim: !t.startedAt && tab === 'active' }">{{ tab === 'active' ? formatStart(t.startedAt) : formatStart(t.finishedAt || t.startedAt || t.createdAt) }}</td>
                <td>
                  <div class="ops">
                    <button v-if="t.status === 'queued' || t.status === 'running'" class="iconbtn" title="取消" aria-label="取消" @click="act(() => tasks.cancel(t.id))"><FIcon name="x" /></button>
                    <button v-if="canRetry(t)" class="btn" :class="{ pri: t.status === 'interrupted' }" @click="doRetry(t)"><FIcon name="retry" />重试</button>
                    <button v-if="t.status === 'succeeded' && t.outputPath" class="iconbtn" title="打开输出" aria-label="打开输出" @click="openOutput(t)"><FIcon name="folder" /></button>
                    <button class="iconbtn" :class="{ on: logId === t.id }" title="查看日志" aria-label="查看日志" @click="toggleLog(t.id)"><FIcon name="doc" /></button>
                    <button v-if="isTerminal(t.status)" class="iconbtn" title="删除" aria-label="删除" @click="askRemove(t)"><FIcon name="trash" /></button>
                  </div>
                </td>
              </tr>
              <!-- 失败行：共享 ErrorLine；错误码来自 Task.error.code，未知码走兜底文案 -->
              <tr v-if="hasErrLine(t)" class="errrow">
                <td colspan="5">
                  <ErrorLine v-if="t.error" :code="t.error.code" :message="t.error.message" @view-log="toggleLog(t.id, true)" />
                  <div v-else class="notice"><FIcon name="warn" :size="14" />应用退出时这个任务还没有结束，不会自动继续。点击“重试”重新开始。</div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>

        <div v-if="loading && rows.length === 0" class="loading">正在加载…</div>
      </div>

      <div v-if="tab !== 'active' && tasks.historyTotal > tasks.historyFilter.pageSize" class="pager">
        <el-pagination
          small
          background
          layout="total, prev, pager, next"
          :total="tasks.historyTotal"
          :page-size="tasks.historyFilter.pageSize"
          :current-page="tasks.historyFilter.page"
          @current-change="(p: number) => tasks.setHistoryPage(p)"
        />
      </div>
      <div v-if="tasks.historyError && tab !== 'active'" class="loaderr">
        <ErrorLine :code="tasks.historyError.code" :message="tasks.historyError.message" :show-log="false" />
      </div>

      <!-- 日志面板 -->
      <div v-if="logTask" class="logwrap">
        <div class="loghead">
          <span>{{ isLogLive ? '实时日志' : '日志' }} · {{ logTask.title }}</span>
          <span class="grow" />
          <button class="iconbtn sm" title="刷新" aria-label="刷新日志" @click="loadLog"><FIcon name="refresh" :size="14" /></button>
          <button class="iconbtn sm" title="关闭" aria-label="关闭日志" @click="closeLog"><FIcon name="x" :size="14" /></button>
        </div>
        <pre ref="logEl" class="log selectable">{{ logText || (logLoading ? '正在读取…' : '（暂无日志）') }}</pre>
      </div>
    </div>

    <!-- 删除 / 清除确认 -->
    <Teleport to="body">
      <div v-if="confirm" class="mask" @click.self="confirm = null">
        <div class="dlg" role="dialog" aria-modal="true">
          <h3>{{ confirm.title }}</h3>
          <p>{{ confirm.text }}</p>
          <label v-if="confirm.canDeleteOutput" class="chk"><el-checkbox v-model="deleteOutput">同时删除输出文件</el-checkbox></label>
          <div class="dfoot">
            <span class="sp" />
            <button class="btn lg" @click="confirm = null">取消</button>
            <button class="btn lg danger" @click="doConfirm">{{ confirm.ok }}</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import { isLiveType, isTerminal, useTaskStore, type TaskItem, type TaskStatus } from '@/stores/tasks'
import { toAppError } from '@/api/call'
import { canRevealInFolder, revealInFolder } from '@/api/system'
import { fileBaseName, formatClock, formatDuration, formatEta, formatStart } from '@/utils/format'

const tasks = useTaskStore()
const route = useRoute()

type Tab = 'active' | 'history' | 'failed'
const tab = ref<Tab>((['active', 'history', 'failed'] as string[]).includes(route.query.tab as string) ? (route.query.tab as Tab) : 'active')

const tabList = computed(() => [
  { key: 'active' as Tab, label: '进行中', count: tasks.runningCount },
  { key: 'history' as Tab, label: '历史', count: tasks.finishedTotal },
  { key: 'failed' as Tab, label: '失败', count: tasks.failedTotal },
])

const TYPE_FILTERS = [
  { key: 'all', label: '全部类型', types: [] as string[] },
  { key: 'convert', label: '转换', types: ['convert'] },
  { key: 'edit', label: '剪辑', types: ['edit_render'] },
  { key: 'doc', label: '文档', types: ['office_pdf'] },
  { key: 'live', label: '直播', types: ['live_file_push', 'live_relay', 'live_record_push'] },
  { key: 'install', label: '安装', types: ['ffmpeg_install'] },
]
const typeFilter = ref('all')

const rows = computed<TaskItem[]>(() => (tab.value === 'active' ? tasks.active : tasks.history))
const loading = computed(() => (tab.value === 'active' ? !tasks.ready : tasks.historyLoading && !tasks.history.length))

const emptyText = computed(() => {
  if (tab.value === 'active') return { title: '没有进行中的任务', hint: '在转换、剪辑或直播页面开始任务后，会显示在这里。' }
  if (tab.value === 'failed') return { title: '没有失败的任务', hint: '失败或被中断的任务会显示在这里，可以重试。' }
  return { title: '还没有历史任务', hint: '完成、失败或取消的任务会保留在这里。' }
})

// ---- 页签 / 过滤 ----
async function loadTab() {
  if (tab.value === 'active') return
  const group = tab.value === 'failed' ? 'failed' : 'all'
  tasks.historyFilter.types = TYPE_FILTERS.find((o) => o.key === typeFilter.value)!.types
  tasks.historyFilter.group = group
  tasks.historyFilter.page = 1
  await tasks.loadHistory()
}
function setTab(t: Tab) {
  if (tab.value === t) return
  tab.value = t
  loadTab()
}
function onTypeChange() {
  loadTab()
}

// ---- 展示辅助 ----
const TYPE_LABEL: Record<string, string> = {
  convert: '转换', edit_render: '剪辑', office_pdf: '文档', ffmpeg_install: '安装',
  live_file_push: '直播', live_relay: '直播', live_record_push: '直播',
}
const typeLabel = (t: string) => TYPE_LABEL[t] ?? t
const typeTagClass = (t: TaskItem) => (t.status === 'running' ? 'run' : 'q')

const STATUS_TAG: Record<TaskStatus, { label: string; cls: string }> = {
  queued: { label: '排队中', cls: 'q' },
  running: { label: '进行中', cls: 'run' },
  succeeded: { label: '完成', cls: 'ok' },
  failed: { label: '失败', cls: 'fail' },
  canceled: { label: '已取消', cls: 'q' },
  interrupted: { label: '已中断', cls: 'warn' },
}

const percent = (t: TaskItem) => Math.round(Math.min(1, Math.max(0, t.progress)) * 100)
const liveSeconds = (t: TaskItem) => (t.outTimeSec > 0 ? t.outTimeSec : t.startedAt ? (Date.now() - t.startedAt) / 1000 : 0)

function progressText(t: TaskItem): string {
  const parts: string[] = []
  if (t.speed) parts.push(t.speed)
  const eta = formatEta(t.etaSec)
  if (eta) parts.push(`剩余 ${eta}`)
  return parts.join(' · ') || '处理中'
}
function queueText(t: TaskItem): string {
  const pos = tasks.queuePosition(t.id)
  return pos > 0 ? `排队中（第 ${pos} 位）` : '排队中'
}

/** 第二行小字：从 params 里取转换选项，取不到就显示输出文件名；参数格式不假设，解析失败静默跳过 */
function subInfo(t: TaskItem): string {
  if (t.type === 'ffmpeg_install') return '下载并安装到应用目录'
  const parts: string[] = []
  try {
    const p = t.params ? JSON.parse(t.params) : null
    const o = p?.options ?? p
    if (o?.container) parts.push(`转为 ${String(o.container).toUpperCase()}`)
    if (o?.targetSizeMb > 0) parts.push(`压缩到 ${o.targetSizeMb} MB`)
  } catch {
    /* params 不是 JSON 就不解析 */
  }
  if (!parts.length && t.outputPath && t.status === 'succeeded') parts.push(fileBaseName(t.outputPath))
  if (!parts.length && t.inputPaths.length) parts.push(t.inputPaths.length > 1 ? `${t.inputPaths.length} 个输入文件` : typeLabel(t.type))
  return parts.join(' · ') || typeLabel(t.type)
}

const hasErrLine = (t: TaskItem) => t.status === 'failed' || t.status === 'interrupted'
const canRetry = (t: TaskItem) => t.status === 'failed' || t.status === 'interrupted' || t.status === 'canceled'

// ---- 动作 ----
async function act(fn: () => Promise<unknown>) {
  try {
    await fn()
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}
async function doRetry(t: TaskItem) {
  await act(async () => {
    await tasks.retry(t.id)
    ElMessage.success('已重新提交')
    tab.value = 'active'
  })
}
async function openOutput(t: TaskItem) {
  await act(async () => {
    if (await revealInFolder(t.outputPath)) return
    // RevealInFolder 后端未实现：退回复制路径
    try {
      await navigator.clipboard.writeText(t.outputPath)
      ElMessage.info(`已复制输出路径：${t.outputPath}`)
    } catch {
      ElMessage.info(t.outputPath)
    }
  })
}

interface Confirm { title: string; text: string; ok: string; canDeleteOutput: boolean; run: () => Promise<void> }
const confirm = ref<Confirm | null>(null)
const deleteOutput = ref(false)
function askRemove(t: TaskItem) {
  deleteOutput.value = false
  confirm.value = {
    title: '删除这条任务记录？',
    text: `“${t.title}”的记录和日志会被删除。`,
    ok: '删除',
    canDeleteOutput: t.status === 'succeeded' && !!t.outputPath,
    run: async () => {
      if (logId.value === t.id) closeLog()
      await tasks.remove([t.id], deleteOutput.value)
    },
  }
}
function askClear() {
  confirm.value = {
    title: '清除所有已结束的任务？',
    text: '只删除任务记录和日志，不会删除已生成的输出文件。进行中的任务不受影响。',
    ok: '清除',
    canDeleteOutput: false,
    run: async () => {
      closeLog()
      await tasks.clearFinished()
    },
  }
}
async function doConfirm() {
  const c = confirm.value
  confirm.value = null
  if (c) await act(c.run)
}

// ---- 日志面板 ----
const logId = ref<string | null>(null)
const logText = ref('')
const logLoading = ref(false)
const logEl = ref<HTMLElement | null>(null)
const logTask = computed<TaskItem | undefined>(() => {
  if (!logId.value) return undefined
  return tasks.active.find((t) => t.id === logId.value) ?? tasks.history.find((t) => t.id === logId.value)
})
const isLogLive = computed(() => logTask.value?.status === 'running')

async function loadLog() {
  const id = logId.value
  if (!id) return
  logLoading.value = true
  try {
    const text = await tasks.getLog(id, 200)
    if (logId.value !== id) return
    logText.value = text.replace(/\s+$/, '')
    await nextTick()
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  } catch (e) {
    if (logId.value === id) logText.value = `读取日志失败：${toAppError(e).message}`
  } finally {
    logLoading.value = false
  }
}
function toggleLog(id: string, forceOpen = false) {
  if (logId.value === id && !forceOpen) closeLog()
  else {
    logId.value = id
    logText.value = ''
    loadLog()
  }
}
function closeLog() {
  logId.value = null
  logText.value = ''
}
// 日志面板打开且任务在运行时每 2 秒刷新一次（日志没有事件推送）
let logTimer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  logTimer = setInterval(() => {
    if (logId.value && isLogLive.value && !document.hidden) loadLog()
  }, 2000)
})
onUnmounted(() => clearInterval(logTimer))
// 进行中页签默认展示第一个运行中任务的日志（与原型一致）；用户手动关闭后不再自动打开
let autoLogDismissed = false
watch(
  () => [tab.value, tasks.ready, tasks.active.find((t) => t.status === 'running' && !isLiveType(t.type))?.id] as const,
  ([tb, ready, firstRunning]) => {
    if (tb === 'active' && ready && firstRunning && !logId.value && !autoLogDismissed) toggleLog(firstRunning)
  },
  { immediate: true },
)
watch(logId, (v, old) => {
  if (v === null && old) autoLogDismissed = true
})

onMounted(async () => {
  await tasks.loadStats()
  await loadTab()
})
</script>

<style scoped>
.tc {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 100%;
}
.card {
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: 10px;
}
.stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}
.stat {
  padding: 14px 16px;
}
.stat small {
  color: var(--ff-text-3);
  font-size: 12px;
}
.stat b {
  display: block;
  font-size: 20px;
  font-weight: 600;
  margin-top: 2px;
  line-height: 1.5;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.tabs {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 0 16px;
  border-bottom: 1px solid var(--ff-border);
}
.tabs > span[role='tab'] {
  height: 40px;
  line-height: 40px;
  color: var(--ff-text-2);
  position: relative;
  cursor: pointer;
}
.tabs > span.on {
  color: var(--ff-text-1);
  font-weight: 500;
}
.tabs > span.on::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: -1px;
  height: 2px;
  background: var(--ff-primary);
  border-radius: 1px;
}
.tabs em {
  font-style: normal;
  color: var(--ff-text-3);
  margin-left: 4px;
  font-size: 12px;
}
.grow {
  flex: 1;
}
.type-select {
  width: 104px;
}
.scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.tbl {
  width: 100%;
  border-collapse: collapse;
}
th {
  font-weight: 500;
  font-size: 12px;
  color: var(--ff-text-3);
  text-align: left;
  padding: 10px 16px;
  border-bottom: 1px solid var(--ff-border);
}
td {
  padding: 12px 16px;
  border-bottom: 1px solid var(--ff-border);
  vertical-align: middle;
}
tbody tr:last-child td {
  border-bottom: none;
}
tr.has-err > td {
  border-bottom: none;
  padding-bottom: 8px;
}
tr.errrow > td {
  padding: 0 16px 12px;
}
.fname {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}
td:first-child {
  max-width: 0; /* 让长文件名在表格里省略而不是撑宽列 */
}
.finfo {
  font-size: 12px;
  color: var(--ff-text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tag {
  height: 20px;
  padding: 0 7px;
  border-radius: 4px;
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
}
.tag.run { background: var(--ff-primary-soft); color: var(--ff-primary); }
.tag.ok { background: color-mix(in srgb, var(--ff-success) 14%, transparent); color: var(--ff-success); }
.tag.fail { background: color-mix(in srgb, var(--ff-danger) 14%, transparent); color: var(--ff-danger); }
.tag.warn { background: color-mix(in srgb, var(--ff-warning) 16%, transparent); color: var(--ff-warning); }
.tag.q { background: var(--ff-bg-hover); color: var(--ff-text-2); }
.tag.live { background: color-mix(in srgb, #ef4444 14%, transparent); color: #ef4444; }
.prog {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--ff-text-2);
}
.pline {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.pline span:first-child {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bar {
  width: 100%;
  height: 4px;
  border-radius: 2px;
  background: var(--ff-border);
  overflow: hidden;
}
.bar i {
  display: block;
  height: 100%;
  background: var(--ff-primary);
  border-radius: 2px;
  transition: width var(--ff-dur-base) var(--ff-ease);
}
.plain {
  color: var(--ff-text-2);
}
.dim {
  color: var(--ff-text-3);
}
.dur {
  margin-left: 8px;
  font-size: 12px;
  color: var(--ff-text-3);
}
.when {
  color: var(--ff-text-2);
  white-space: nowrap;
}
.when.dim {
  color: var(--ff-text-3);
}
.opsh {
  width: 1%;
}
.ops {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 2px;
  color: var(--ff-text-3);
}
.ops .btn {
  margin-right: 4px;
}
.iconbtn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: 6px;
  display: grid;
  place-items: center;
  color: var(--ff-text-2);
  cursor: pointer;
  padding: 0;
}
.iconbtn:hover {
  background: var(--ff-bg-hover);
}
.iconbtn.on {
  color: var(--ff-primary);
}
.iconbtn.sm {
  width: 22px;
  height: 22px;
}
.btn {
  height: 28px;
  padding: 0 12px;
  border-radius: 6px;
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font: inherit;
  font-size: 13px;
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
  background: var(--ff-primary);
  border-color: var(--ff-primary);
  color: #fff;
}
.btn.pri:hover {
  background: var(--ff-primary-hover);
}
.btn.lg {
  height: 32px;
  padding: 0 16px;
}
.btn.danger {
  background: var(--ff-danger);
  border-color: var(--ff-danger);
  color: #fff;
}
.btn svg {
  width: 15px;
  height: 15px;
}
.tabs .btn {
  margin-left: 0;
}
.notice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--ff-warning-soft);
  color: var(--ff-text-2);
  font-size: 12px;
}
.notice svg {
  color: var(--ff-warning);
}
.loaderr {
  padding: 12px 16px 0;
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 64px 16px;
  color: var(--ff-text-3);
  font-size: 12px;
}
.empty b {
  color: var(--ff-text-1);
  font-weight: 500;
  font-size: 14px;
}
.eic {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--ff-primary-soft);
  color: var(--ff-primary);
  display: grid;
  place-items: center;
  margin-bottom: 8px;
}
.loading {
  padding: 32px;
  text-align: center;
  color: var(--ff-text-3);
}
.pager {
  display: flex;
  justify-content: flex-end;
  padding: 8px 16px;
  border-top: 1px solid var(--ff-border);
}
.logwrap {
  padding: 0 16px 16px;
  margin-top: auto;
}
.loghead {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--ff-text-3);
  margin-bottom: 6px;
}
.log {
  margin: 0;
  font-family: var(--ff-font-mono);
  font-size: 12px;
  color: var(--ff-text-2);
  background: var(--ff-bg-app);
  border-radius: 6px;
  padding: 10px 12px;
  line-height: 1.7;
  white-space: pre;
  max-height: 150px;
  min-height: 44px;
  overflow: auto;
}
.mask {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: rgba(0, 0, 0, 0.45);
  display: grid;
  place-items: center;
}
.dlg {
  width: 400px;
  padding: 24px;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: 12px;
  box-shadow: var(--ff-shadow-dialog);
}
.dlg h3 {
  margin: 0 0 8px;
  font-size: 16px;
  font-weight: 600;
}
.dlg p {
  margin: 0 0 16px;
  color: var(--ff-text-2);
  word-break: break-all;
}
.chk {
  display: block;
  margin: -4px 0 16px;
}
.dfoot {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sp {
  flex: 1;
}
</style>
