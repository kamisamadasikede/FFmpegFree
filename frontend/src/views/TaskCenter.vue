<template>
  <div class="tc">
    <!-- 统计条：运行中 / 排队中 / 今日完成 / 失败 -->
    <div class="stats">
      <div class="card stat">
        <small>运行中</small>
        <b style="color: var(--ff-primary)">{{ tasks.runningOnly }}</b>
        <span v-if="tasks.liveActiveCount > 0" class="note">{{ LIVE_NOTE }}</span>
      </div>
      <div class="card stat"><small>排队中</small><b>{{ tasks.queuedCount }}</b></div>
      <div class="card stat"><small>今日完成</small><b style="color: var(--ff-success)">{{ tasks.todayDone }}{{ tasks.todayDoneCapped ? '+' : '' }}</b></div>
      <div class="card stat"><small>失败</small><b style="color: var(--ff-danger)">{{ tasks.failedTotal }}</b></div>
    </div>

    <div class="card main">
      <div class="tabbar">
        <div class="tabs" role="tablist" aria-label="任务分类" @keydown="onTabKeydown">
          <button
            v-for="t in tabList"
            :id="`tc-tab-${t.key}`"
            :key="t.key"
            type="button"
            role="tab"
            class="tab"
            :data-tab="t.key"
            :aria-selected="tab === t.key"
            aria-controls="tc-panel"
            :tabindex="tab === t.key ? 0 : -1"
            :class="{ on: tab === t.key }"
            @click="setTab(t.key)"
          >
            {{ t.label }}<em>{{ t.count }}</em>
          </button>
        </div>
        <div class="filters">
          <el-select v-model="typeFilter" size="small" class="type-select" aria-label="按类型筛选" @change="onTypeChange">
            <el-option v-for="o in TYPE_FILTERS" :key="o.key" :label="o.label" :value="o.key" />
          </el-select>
          <button v-if="tab !== 'active'" type="button" class="btn" :disabled="!tasks.historyTotal && !tasks.finishedTotal" @click="askClear"><FIcon name="trash" />清除已结束</button>
        </div>
      </div>

      <div v-if="tasks.loadError && tab !== 'history' && tab !== 'failed'" class="loaderr">
        <ErrorLine :code="tasks.loadError.code" :message="tasks.loadError.message" :detail="tasks.loadError.detail" :show-log="false" fallback-title="加载任务失败" />
      </div>

      <div id="tc-panel" class="scroll" role="tabpanel" :aria-labelledby="`tc-tab-${tab}`">
        <!-- 空状态 -->
        <div v-if="rows.length === 0 && !loading" class="empty">
          <div class="eic"><FIcon :name="tab === 'failed' ? 'check' : 'task'" :size="24" /></div>
          <b>{{ emptyText.title }}</b>
          <span>{{ emptyText.hint }}</span>
        </div>

        <table v-else class="tbl" aria-label="任务列表">
          <thead>
            <tr>
              <th scope="col" style="width: 30%">任务</th>
              <th scope="col" style="width: 8%">类型</th>
              <th scope="col" style="width: 11%">状态</th>
              <th scope="col" style="width: 24%">进度</th>
              <th scope="col">开始时间</th>
              <th scope="col" class="opsh"><span class="sr-only">操作</span></th>
            </tr>
          </thead>
          <tbody>
            <template v-for="t in rows" :key="t.id">
              <tr :class="{ sel: logId === t.id, haserr: hasErrLine(t) }">
                <td>
                  <div class="fname" :title="t.title">{{ t.title || fileBaseName(t.outputPath) }}</div>
                  <div class="finfo">{{ subInfo(t) }}</div>
                </td>
                <td>
                  <span v-if="isLiveType(t.type)" class="tag live">● 直播</span>
                  <span v-else class="tag type">{{ typeLabel(t.type) }}</span>
                </td>
                <td>
                  <span class="tag" :class="STATUS_TAG[t.status].cls">
                    <FIcon v-if="STATUS_TAG[t.status].icon" :name="STATUS_TAG[t.status].icon!" :size="12" />{{ STATUS_TAG[t.status].label }}
                  </span>
                </td>
                <td>
                  <div v-if="showBar(t)" class="prog">
                    <div class="pline"><span>{{ progressText(t) }}</span><span>{{ percent(t) }}%</span></div>
                    <div class="bar" role="progressbar" :aria-label="`${t.title} 进度`" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="percent(t)">
                      <i :class="barClass(t)" :style="{ width: percent(t) + '%' }" />
                    </div>
                  </div>
                  <span v-else class="plain" :class="{ dim: t.status === 'canceled' }">{{ progressText(t) }}</span>
                </td>
                <td class="when" :class="{ dim: !t.startedAt && !isTerminal(t.status) }">{{ formatStart(startTime(t)) }}</td>
                <td>
                  <div class="ops">
                    <button v-if="t.status === 'failed' || t.status === 'interrupted'" type="button" class="btn sm" @click="doRetry(t)"><FIcon name="retry" />重试</button>
                    <button v-if="t.status === 'queued' || t.status === 'running'" type="button" class="iconbtn" :title="`取消 ${t.title}`" :aria-label="`取消 ${t.title}`" @click="act(() => tasks.cancel(t.id))"><FIcon name="x" /></button>
                    <button v-if="t.status === 'succeeded' && t.outputPath" type="button" class="iconbtn" :title="`打开输出 ${t.title}`" :aria-label="`打开输出 ${t.title}`" @click="openOutput(t)"><FIcon name="folder" /></button>
                    <button type="button" class="iconbtn" :class="{ on: logId === t.id }" :title="`查看日志 ${t.title}`" :aria-label="`查看日志 ${t.title}`" :aria-pressed="logId === t.id" @click="toggleLog(t.id)"><FIcon name="doc" /></button>
                    <button v-if="isTerminal(t.status)" type="button" class="iconbtn" :title="`删除 ${t.title}`" :aria-label="`删除 ${t.title}`" @click="askRemove(t)"><FIcon name="trash" /></button>
                  </div>
                </td>
              </tr>
              <!-- 失败 / 已中断行：共享 ErrorLine；错误码来自 Task.error.code，未知码走兜底文案（带后端 message） -->
              <tr v-if="hasErrLine(t)" class="errrow">
                <td colspan="6">
                  <ErrorLine
                    v-if="t.error"
                    compact
                    :tone="t.status === 'interrupted' ? 'interrupted' : 'danger'"
                    :code="t.error.code"
                    :message="t.error.message"
                    :detail="t.error.detail"
                    :announce="isFresh(t)"
                    show-retry
                    :hide-retry="t.status === 'interrupted'"
                    @retry="doRetry(t)"
                    @change-output="changeOutput"
                    @view-log="toggleLog(t.id, true)"
                  />
                  <ErrorLine
                    v-else
                    compact
                    tone="interrupted"
                    code="INTERRUPTED"
                    title=""
                    description="应用退出时这个任务被中断，可以重试。"
                    hide-code
                    :announce="isFresh(t)"
                    hide-retry
                    @retry="doRetry(t)"
                    @view-log="toggleLog(t.id, true)"
                  />
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
        <ErrorLine :code="tasks.historyError.code" :message="tasks.historyError.message" :detail="tasks.historyError.detail" :show-log="false" fallback-title="加载任务失败" announce />
      </div>

      <!-- 日志面板 -->
      <div v-if="logTask" class="logwrap">
        <div class="loghead">
          <span>{{ isLogLive ? '实时日志' : '日志' }} · {{ logTask.title }}</span>
          <span class="grow" />
          <button type="button" class="iconbtn sm" title="刷新日志" aria-label="刷新日志" @click="loadLog"><FIcon name="refresh" :size="14" /></button>
          <button type="button" class="iconbtn sm" title="关闭日志" aria-label="关闭日志" @click="closeLog"><FIcon name="x" :size="14" /></button>
        </div>
        <pre ref="logEl" class="log selectable" tabindex="0" aria-label="任务日志">{{ logText || (logLoading ? '正在读取…' : '（暂无日志）') }}</pre>
      </div>
    </div>

    <!-- 删除 / 清除确认 -->
    <Teleport to="body">
      <div v-if="confirm" class="mask" @click.self="confirm = null" @keydown.esc="confirm = null">
        <div class="dlg" role="dialog" aria-modal="true" aria-labelledby="tc-dlg-title">
          <h3 id="tc-dlg-title">{{ confirm.title }}</h3>
          <p>{{ confirm.text }}</p>
          <label v-if="confirm.canDeleteOutput" class="chk"><el-checkbox v-model="deleteOutput">同时删除输出文件</el-checkbox></label>
          <div class="dfoot">
            <span class="sp" />
            <button type="button" class="btn lg" @click="confirm = null">取消</button>
            <button type="button" class="btn lg danger" @click="doConfirm">{{ confirm.ok }}</button>
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
import type { IconName } from '@/components/icon/icons'
import { toAppError } from '@/api/call'
import { revealInFolder } from '@/api/system'
import { fileBaseName, formatClock, formatDuration, formatEta, formatStart } from '@/utils/format'

/** 直播推流的说明文案（统计条和进行中的直播行共用）。角标 runningCount 仍包含直播推流 */
const LIVE_NOTE = '直播推流单独计数，不占转换名额'

const tasks = useTaskStore()
const route = useRoute()

type Tab = 'all' | 'active' | 'history' | 'failed'
const TAB_KEYS: Tab[] = ['all', 'active', 'history', 'failed']
const tab = ref<Tab>(TAB_KEYS.includes(route.query.tab as Tab) ? (route.query.tab as Tab) : 'all')

// 全部 = 进行中 + 已结束；历史 = 已结束（含失败 / 取消 / 中断）；失败 = failed + interrupted
const tabList = computed(() => [
  { key: 'all' as Tab, label: '全部', count: tasks.runningCount + tasks.finishedTotal },
  { key: 'active' as Tab, label: '进行中', count: tasks.runningCount },
  { key: 'history' as Tab, label: '历史', count: tasks.finishedTotal },
  { key: 'failed' as Tab, label: '失败', count: tasks.failedTotal },
])

/** 页签键盘操作（WAI-ARIA tabs：左右键切换，Home / End 到首尾，自动激活） */
function onTabKeydown(e: KeyboardEvent) {
  const keys = tabList.value.map((t) => t.key)
  const i = keys.indexOf(tab.value)
  let next = -1
  if (e.key === 'ArrowRight') next = (i + 1) % keys.length
  else if (e.key === 'ArrowLeft') next = (i - 1 + keys.length) % keys.length
  else if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = keys.length - 1
  if (next < 0) return
  e.preventDefault()
  setTab(keys[next])
  nextTick(() => document.getElementById(`tc-tab-${keys[next]}`)?.focus())
}

const TYPE_FILTERS = [
  { key: 'all', label: '全部类型', types: [] as string[] },
  { key: 'convert', label: '转换', types: ['convert'] },
  { key: 'edit', label: '剪辑', types: ['edit_render'] },
  { key: 'doc', label: '文档', types: ['office_pdf'] },
  { key: 'live', label: '直播', types: ['live_file_push', 'live_relay', 'live_record_push'] },
  { key: 'install', label: '安装', types: ['ffmpeg_install'] },
]
const typeFilter = ref('all')

const typeSet = computed(() => TYPE_FILTERS.find((o) => o.key === typeFilter.value)!.types)
const activeFiltered = computed(() => (typeSet.value.length ? tasks.active.filter((t) => typeSet.value.includes(t.type)) : tasks.active))
const rows = computed<TaskItem[]>(() => {
  if (tab.value === 'active') return activeFiltered.value
  if (tab.value === 'history') return tasks.history
  if (tab.value === 'failed') return tasks.history
  // 全部：进行中的排在最前（只在第一页），后面是已结束的历史
  return tasks.historyFilter.page === 1 ? [...activeFiltered.value, ...tasks.history] : tasks.history
})
const loading = computed(() => {
  if (tab.value === 'active') return !tasks.ready
  return tasks.historyLoading && !tasks.history.length
})

const emptyText = computed(() => {
  if (tab.value === 'active') return { title: '没有进行中的任务', hint: '在转换、剪辑或直播页面开始任务后，会显示在这里。' }
  if (tab.value === 'all') return { title: '还没有任务', hint: '在转换、剪辑或直播页面开始任务后，会显示在这里。' }
  if (tab.value === 'failed') return { title: '没有失败的任务', hint: '失败或被中断的任务会显示在这里，可以重试。' }
  return { title: '还没有历史任务', hint: '完成、失败或取消的任务会保留在这里。' }
})

// ---- 页签 / 过滤 ----
async function loadTab() {
  if (tab.value === 'active') return
  const group = tab.value === 'failed' ? 'failed' : 'all'
  tasks.historyFilter.types = typeSet.value
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

// 类型标签默认中性色，颜色主要出现在状态列；唯一例外是「● 直播」标签用危险色（设计稿如此）。canceled 是无底色 1px 描边，和 queued 的灰底区分开
const STATUS_TAG: Record<TaskStatus, { label: string; cls: string; icon?: IconName }> = {
  queued: { label: '排队中', cls: 'q' },
  running: { label: '运行中', cls: 'run' },
  succeeded: { label: '已完成', cls: 'ok', icon: 'check' },
  failed: { label: '失败', cls: 'fail' },
  canceled: { label: '已取消', cls: 'cx' },
  interrupted: { label: '已中断', cls: 'int', icon: 'warn' },
}

const percent = (t: TaskItem) => Math.round(Math.min(1, Math.max(0, t.progress)) * 100)
const liveSeconds = (t: TaskItem) => (t.outTimeSec > 0 ? t.outTimeSec : t.startedAt ? (Date.now() - t.startedAt) / 1000 : 0)
const startTime = (t: TaskItem) => (isTerminal(t.status) ? t.startedAt || t.createdAt : t.startedAt)

/** 进度条：非直播的运行中任务，以及保留了中断 / 失败时进度的任务 */
function showBar(t: TaskItem): boolean {
  if (isLiveType(t.type)) return false
  if (t.status === 'running') return true
  if (t.status === 'failed' || t.status === 'interrupted') return t.progress > 0
  return false
}
const barClass = (t: TaskItem) => ({ run: t.status === 'running', fail: t.status === 'failed', int: t.status === 'interrupted' })

function progressText(t: TaskItem): string {
  switch (t.status) {
    case 'running': {
      if (isLiveType(t.type)) return `已推流 ${formatClock(liveSeconds(t))} · ${LIVE_NOTE}`
      const parts: string[] = []
      if (t.speed) parts.push(t.speed)
      const eta = formatEta(t.etaSec)
      if (eta) parts.push(`剩余 ${eta}`)
      return parts.join(' · ') || '处理中'
    }
    case 'queued': {
      const pos = tasks.queuePosition(t.id)
      return pos > 0 ? `排队中（第 ${pos} 位）` : '排队中'
    }
    case 'succeeded':
      return t.startedAt && t.finishedAt ? `用时 ${formatDuration(t.finishedAt - t.startedAt)}` : '已完成'
    case 'failed':
      return '失败'
    case 'interrupted':
      return '应用退出，已中断'
    case 'canceled':
      return '用户取消'
  }
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
/** 页面打开之后才结束的失败：ErrorLine 用 role="alert" 播报；打开页面时就已存在的历史失败只是 group */
const mountedAt = Date.now()
const isFresh = (t: TaskItem) => t.finishedAt > mountedAt

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
    if (tab.value === 'failed') setTab('active')
  })
}
/**
 * 「更换输出位置」：桩。契约有 SystemService.PickDirectory，但绑定里还没有，
 * Settings 也还没有输出目录字段（UpdateSettings 只有 ffmpegPath / ffmpegPromptDismissed），选了目录也没处保存。
 * 所以先只提示；后端补上 PickDirectory 和输出目录设置后，在这里选目录、保存，再 doRetry。
 */
function changeOutput() {
  ElMessage.info('更换输出位置功能即将上线，目前请先清理磁盘空间后重试。')
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
    if ((tb === 'active' || tb === 'all') && ready && firstRunning && !logId.value && !autoLogDismissed) toggleLog(firstRunning)
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
.stat .note {
  display: block;
  margin-top: 2px;
  font-size: 11px;
  color: var(--ff-text-3);
  line-height: 1.4;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.tabbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  column-gap: 20px;
  padding: 0 16px;
  border-bottom: 1px solid var(--ff-border);
}
.tabs {
  display: flex;
  gap: 20px;
}
.tab {
  height: 40px;
  line-height: 40px;
  padding: 0;
  border: none;
  background: transparent;
  font: inherit;
  color: var(--ff-text-2);
  position: relative;
  cursor: pointer;
  border-radius: 2px;
}
.tab.on {
  color: var(--ff-text-1);
  font-weight: 500;
}
.tab.on::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: -1px;
  height: 2px;
  background: var(--ff-primary);
  border-radius: 1px;
}
.tab em {
  font-style: normal;
  color: var(--ff-text-3);
  margin-left: 4px;
  font-size: 12px;
}
.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
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
tr.haserr > td {
  border-bottom: none;
  padding-bottom: 6px;
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
/* 类型标签默认中性（直播标签是设计稿里的红色例外）；状态列有颜色。文字色用 *-text 变量，浅色主题下在着色底上 ≥4.5:1 */
.tag.type { background: var(--ff-bg-hover); color: var(--ff-text-2); }
.tag.live { background: color-mix(in srgb, var(--ff-danger) 14%, transparent); color: var(--ff-danger-text); }
.tag.run { background: var(--ff-primary-soft); color: var(--ff-primary-text); }
.tag.ok { background: color-mix(in srgb, var(--ff-success) 14%, transparent); color: var(--ff-success-text); }
.tag.fail { background: color-mix(in srgb, var(--ff-danger) 14%, transparent); color: var(--ff-danger-text); }
.tag.int { background: color-mix(in srgb, var(--ff-interrupted) 14%, transparent); color: var(--ff-interrupted); }
.tag.q { background: var(--ff-bg-hover); color: var(--ff-text-2); }
.tag.cx { background: transparent; border: 1px solid var(--ff-border); color: var(--ff-text-2); }
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
  position: relative;
  height: 100%;
  background: var(--ff-primary);
  border-radius: 2px;
  overflow: hidden;
  transition: width var(--ff-dur-base) var(--ff-ease);
}
.bar i.fail { background: var(--ff-danger); }
.bar i.int { background: var(--ff-interrupted); }
/* 运行中进度条的轻微流光（设计规范 §状态）。减少动效时停用 */
.bar i.run::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(100deg, transparent 20%, rgba(255, 255, 255, 0.45) 50%, transparent 80%);
  transform: translateX(-100%);
  animation: ff-shimmer 1.8s linear infinite;
}
@keyframes ff-shimmer {
  to { transform: translateX(100%); }
}
@media (prefers-reduced-motion: reduce) {
  .bar i.run::after {
    animation: none;
    display: none;
  }
  .bar i {
    transition: none;
  }
}
/* 设置里的「减少动效」开关会给 <html> 加 reduce-motion（设置页还没有该开关，钩子先留好） */
:global(html.reduce-motion) .bar i.run::after {
  animation: none;
  display: none;
}
:global(html.reduce-motion) .bar i {
  transition: none;
}
.plain {
  color: var(--ff-text-2);
  font-size: 12px;
}
.plain.dim,
.dim {
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
.btn.sm {
  height: 24px;
  padding: 0 8px;
  font-size: 12px;
  gap: 4px;
}
.btn.sm svg {
  width: 13px;
  height: 13px;
}
.btn.lg {
  height: 32px;
  padding: 0 16px;
}
.btn.danger {
  background: var(--ff-danger);
  border-color: var(--ff-danger);
  color: var(--ff-on-danger);
}
.btn svg {
  width: 15px;
  height: 15px;
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
.log:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
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
