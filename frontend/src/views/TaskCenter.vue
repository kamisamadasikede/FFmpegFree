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
          <!-- “显示已隐藏”（契约 v0.23 §6.14.11；设计 state=taskcenter-hidden）。产品可能不要：去掉只需把 SHOW_HIDDEN_TOGGLE 改成 false -->
          <label v-if="SHOW_HIDDEN_TOGGLE && tab !== 'active'" class="tc-sw">
            <el-switch :model-value="tasks.historyFilter.includeHidden" size="small" aria-label="显示已隐藏" @update:model-value="(v: string | number | boolean) => setShowHidden(!!v)" />显示已隐藏
          </label>
          <button v-if="tab !== 'active'" type="button" class="btn" title="只从任务中心隐藏，转换记录仍保留在转换页" :disabled="!tasks.historyTotal && !tasks.finishedTotal" @click="askHide"><FIcon name="eyeoff" />{{ HIDE_FINISHED_LABEL }}</button>
        </div>
      </div>

      <MotionCollapse>
      <div v-if="showingHidden && hiddenShown > 0" class="tc-hint" role="status">
        <FIcon name="info" :size="14" /><span>正在显示 {{ hiddenShown }} 条已隐藏的任务（置灰）。<span class="w1280">隐藏只影响任务中心，</span>转换记录仍在转换页，要删除请到转换页。</span>
      </div>
      </MotionCollapse>

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

        <table v-else class="tbl" aria-label="任务列表" :style="{ '--ops-w': opsWidth + 'px' }">
          <thead>
            <tr>
              <th scope="col" class="c-task">任务</th>
              <th scope="col" class="c-type">类型</th>
              <th scope="col" class="c-st">状态</th>
              <th scope="col" class="c-prog">进度</th>
              <th scope="col" class="c-when">开始时间</th>
              <th scope="col" class="opsh"><span class="sr-only">操作</span></th>
            </tr>
          </thead>
          <!-- 新任务行淡入（换页签 / 筛选 / 翻页时整个 tbody 换 key，不播）；删除瞬间完成，表格行不做收高 -->
          <TransitionGroup :key="rowsKey" tag="tbody" name="ff-row">
            <template v-for="t in rows" :key="t.id">
              <tr :class="{ sel: logId === t.id, haserr: hasErrLine(t) || showFallbackNotice(t), hid: isHidden(t) }">
                <td>
                  <div class="tc-nm">
                    <div class="fname tc-dim" :title="isSim(t) ? `演示任务（模拟数据） · ${t.title}` : t.title"><span v-if="isSim(t)" class="simtag">演示</span><MidEllipsis :text="shownTitle(t)" :title="isSim(t) ? `演示任务（模拟数据） · ${t.title}` : t.title" /></div>
                    <span v-if="isHidden(t)" class="tc-hidtag"><FIcon name="eyeoff" :size="12" />已隐藏</span>
                  </div>
                  <div class="finfo tc-dim" :title="subTitle(t)">{{ subInfo(t) }}</div>
                </td>
                <td class="tc-dim">
                  <span v-if="isLiveType(t.type)" class="tag live">● 直播</span>
                  <span v-else class="tag type">{{ typeLabel(t.type) }}</span>
                </td>
                <td class="tc-dim">
                  <span class="tag" :class="[statusTag(t).cls, { 'ff-done-pop': justDone(t.id) }]">
                    <FIcon v-if="statusTag(t).icon" :name="statusTag(t).icon!" :size="12" />{{ statusLabel(t) }}
                  </span>
                </td>
                <td class="tc-dim">
                  <div v-if="showBar(t)" class="prog">
                    <div class="pline" :title="barText(t) || undefined">
                      <span class="wide">{{ barText(t) }}</span>
                      <!-- 1024（复验 N3）：进度列只有 104px，速度单独放、不省略；剩余时间挪到进度条下面，用 m:ss 短写 -->
                      <span class="narrow spd">{{ narrowSpeed(t) }}</span>
                      <span>{{ percent(t) }}%</span>
                    </div>
                    <div class="bar" role="progressbar" :aria-label="`${t.title} 进度`" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="percent(t)">
                      <i :class="barClass(t)" :style="{ width: percent(t) + '%' }" />
                    </div>
                    <span v-if="narrowEta(t)" class="narrow peta">{{ narrowEta(t) }}</span>
                  </div>
                  <span v-else class="plain" :class="{ dim: t.status === 'canceled' }" :title="progressShort(t) ? progressText(t) : undefined">
                    <template v-if="progressShort(t)"><span class="wide">{{ progressText(t) }}</span><span class="narrow">{{ progressShort(t) }}</span></template>
                    <template v-else>{{ progressText(t) }}</template>
                  </span>
                </td>
                <td class="when tc-dim" :class="{ dim: !t.startedAt && !isTerminal(t.status) }">{{ formatStart(startTime(t)) }}</td>
                <td>
                  <div class="ops">
                    <button v-if="canRetry(t)" type="button" class="btn sm tc-rt" title="重试" aria-label="重试" :disabled="tasks.isBusy(t.id)" :aria-busy="tasks.isBusy(t.id)" @click="doRetry(t)"><FIcon name="retry" /><span class="lbl">重试</span></button>
                    <button v-if="isHidden(t)" type="button" class="btn sm tc-unh" :title="`在任务中心重新显示 ${t.title}`" @click="act(() => tasks.unhide([t.id]))"><FIcon name="eye" />取消隐藏</button>
                    <button v-if="t.status === 'queued' || t.status === 'running'" type="button" class="iconbtn" :title="`取消 ${t.title}`" :aria-label="`取消 ${t.title}`" @click="act(() => tasks.cancel(t.id))"><FIcon name="x" /></button>
                    <button v-if="t.status === 'succeeded' && t.outputPath" type="button" class="iconbtn" :title="`打开输出 ${t.title}`" :aria-label="`打开输出 ${t.title}`" @click="openOutput(t)"><FIcon name="folder" /></button>
                    <button type="button" class="iconbtn" :class="{ on: logId === t.id }" :data-logbtn="t.id" :title="`查看日志 ${t.title}`" :aria-label="`查看日志 ${t.title}`" :aria-pressed="logId === t.id" @click="toggleLog(t.id)"><FIcon name="doc" /></button>
                    <!-- 转换记录只在格式转换页删除（契约 v0.23：Remove 遇到 convert 整体 INVALID_ARGUMENT），这里换成“在转换页查看” -->
                    <button v-if="isTerminal(t.status) && t.type === 'convert'" type="button" class="iconbtn" title="转换记录请在转换页删除" :aria-label="`在转换页查看 ${t.title}`" @click="goConvert(t)"><FIcon name="convert" /></button>
                    <button v-else-if="isTerminal(t.status)" type="button" class="iconbtn" :title="`移除 ${t.title}`" :aria-label="`移除 ${t.title}`" @click="askRemove(t)"><FIcon name="trash" /></button>
                  </div>
                </td>
              </tr>
              <!-- 硬件编码失败、已自动改用 CPU（契约 9.7）：警告色，不是失败；直播任务用直播那条文案 -->
              <tr v-if="showFallbackNotice(t)" class="errrow fbrow">
                <td colspan="6">
                  <EncoderFallbackNotice variant="row" :text="isLiveType(t.type) ? ENCODER_FALLBACK_LIVE : isTerminal(t.status) ? ENCODER_FALLBACK_TASK_ROW_DONE : undefined" @log="toggleLog(t.id, true)" />
                </td>
              </tr>
              <!-- 失败 / 已中断行：共享 ErrorLine；错误码来自 Task.error.code，未知码走兜底文案（带后端 message） -->
              <tr v-if="hasErrLine(t)" class="errrow">
                <td colspan="6">
                  <ErrorLine
                    v-if="t.error && t.error.code === 'LIVE_SOURCE_GONE'"
                    compact
                    tone="interrupted"
                    :code="t.error.code"
                    title=""
                    :description="liveSourceGoneText(goneKind(t))"
                    hide-code
                    :announce="isFresh(t)"
                    hide-retry
                    :busy="tasks.isBusy(t.id)"
                    @view-log="toggleLog(t.id, true)"
                  />
                  <ErrorLine
                    v-else-if="t.error && liveBroken(t)"
                    compact
                    tone="interrupted"
                    :code="t.error.code"
                    :title="interruptOf(t).title"
                    :description="interruptOf(t).description"
                    hide-code
                    :announce="isFresh(t)"
                    hide-retry
                    :busy="tasks.isBusy(t.id)"
                    @view-log="toggleLog(t.id, true)"
                  />
                  <ErrorLine
                    v-else-if="t.error"
                    compact
                    :tone="t.status === 'interrupted' ? 'interrupted' : 'danger'"
                    :code="t.error.code"
                    :message="errMessage(t)"
                    :detail="t.error.detail"
                    :task-type="t.type"
                    :announce="isFresh(t)"
                    show-retry
                    :busy="tasks.isBusy(t.id)"
                    :hide-retry="t.status === 'interrupted' || isLiveType(t.type) || isRetiredType(t.type)"
                    @retry="doRetry(t)"
                    @change-output="changeOutput(t)"
                    @view-log="toggleLog(t.id, true)"
                  />
                  <ErrorLine
                    v-else
                    compact
                    tone="interrupted"
                    code="INTERRUPTED"
                    title=""
                    :description="isLiveType(t.type) ? (liveTaskVerb(t.type) === '拉流' ? '应用退出时拉流被中断，请回到直播页重新拉流。' : '应用退出时推流被中断，请回到直播页重新推流。') : isRetiredType(t.type) ? RETIRED_INTERRUPTED_TEXT : '应用退出时这个任务被中断，可以重试。'"
                    hide-code
                    :announce="isFresh(t)"
                    hide-retry
                    :busy="tasks.isBusy(t.id)"
                    @retry="doRetry(t)"
                    @view-log="toggleLog(t.id, true)"
                  />
                </td>
              </tr>
            </template>
          </TransitionGroup>
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

      <!-- 日志面板：打开 / 关闭时高度 + 透明度收放（P1 动效），上面的列表跟着平滑让位 -->
      <MotionCollapse>
      <div v-if="logTask" ref="logWrapEl" class="logwrap" tabindex="-1" role="region" aria-label="任务日志面板">
        <div class="loghead">
          <span>{{ isLogLive ? '实时日志' : '日志' }} · {{ logTask.title }}</span>
          <span class="grow" />
          <button type="button" class="iconbtn sm" title="刷新日志" aria-label="刷新日志" @click="loadLog"><FIcon name="refresh" :size="14" /></button>
          <button type="button" class="iconbtn sm" title="关闭日志" aria-label="关闭日志" @click="closeLog"><FIcon name="x" :size="14" /></button>
        </div>
        <!-- 任务详情的编码设备信息：设备栏回退时是“CPU（已回退）”；原因（枚举 → 用户文案，未知走兜底）只放在这里，任务行不显示 -->
        <div v-if="logDevice" class="logdev">
          <span class="dv" :title="logDeviceFb ? ENCODER_DEVICE_CPU_FALLBACK_TITLE : `${ENCODER_DEVICE_LABEL}：${logDevice}`">{{ ENCODER_DEVICE_LABEL }}：<b v-if="logDeviceFb" class="dev-fb">{{ logDevice }}</b><template v-else>{{ logDevice }}</template></span>
          <span v-if="logFallbackReason" class="rs">{{ logFallbackReason }}</span>
        </div>
        <pre ref="logEl" class="log selectable" tabindex="0" aria-label="任务日志">{{ logText || (logLoading ? '正在读取…' : '（暂无日志）') }}</pre>
      </div>
      </MotionCollapse>
    </div>

    <!-- 删除 / 隐藏确认 -->
    <Teleport to="body">
      <MotionDialog>
      <div v-if="confirm" class="mask" @click.self="confirm = null" @keydown.esc="confirm = null">
        <div class="dlg ff-panel" role="dialog" aria-modal="true" aria-labelledby="tc-dlg-title">
          <h3 id="tc-dlg-title">{{ confirm.title }}</h3>
          <p>{{ confirm.text }}</p>
          <label v-if="confirm.canDeleteOutput" class="chk"><el-checkbox v-model="deleteOutput">同时删除输出文件</el-checkbox></label>
          <div class="dfoot">
            <span class="sp" />
            <button type="button" class="btn lg" @click="confirm = null">取消</button>
            <button type="button" class="btn lg" :class="confirm.safe ? 'pri' : 'danger'" @click="doConfirm">{{ confirm.ok }}</button>
          </div>
        </div>
      </div>
      </MotionDialog>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import MidEllipsis from '@/components/common/MidEllipsis.vue'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import MotionCollapse from '@/components/motion/MotionCollapse.vue'
import { useJustDone } from '@/composables/useJustDone'
import MotionDialog from '@/components/motion/MotionDialog.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import EncoderFallbackNotice from '@/components/encoder/EncoderFallbackNotice.vue'
import { DUR, prefersReducedMotion, scrollBehavior } from '@/utils/motion'
import { useNarrow } from '@/components/convert/useNarrow'
import { getSource, parseParams } from '@/api/convertRecords'
import { recordParamsText, setPresetCatalog } from '@/utils/convertText'
import { showFallbackNotice, usedDeviceText, useEncoderDeviceList } from '@/api/encoderTask'
import { ENCODER_DEVICE_CPU_FALLBACK_NAME, ENCODER_DEVICE_CPU_FALLBACK_TITLE, ENCODER_DEVICE_LABEL, ENCODER_FALLBACK_LIVE, ENCODER_FALLBACK_TASK_ROW_DONE, encoderFallbackReasonText } from '@/errors/encoderMessages'
import { canRetryTask, elapsedMs, isLiveType, isRetiredType, isTerminal, useTaskStore, type TaskItem, type TaskStatus } from '@/stores/tasks'
import type { IconName } from '@/components/icon/icons'
import { toAppError } from '@/api/call'
import { isSimTask, SIM_TITLE_PREFIX } from '@/api/sim'
import { actionErrorText, docUnsupportedText, liveFailureMessage, liveSourceGoneText, liveTaskVerb, LIVE_STOP_TEXT, schemeFromParams } from '@/errors/errorMessages'
import { liveInterruptView } from '@/errors/livePreviewMessages'
import { parseDetailHead } from '@/api/call'
import { pickDirectory, revealInFolder } from '@/api/system'
import { listPresets, parseConvertParams, resubmitToDir } from '@/api/convert'
import { fileBaseName, formatClock, formatDuration, formatEta, formatStart } from '@/utils/format'

/** 直播推流的说明文案（统计条和进行中的直播行共用）。角标 runningCount 仍包含直播推流 */
const LIVE_NOTE = '直播推流单独计数，不占转换名额'

const tasks = useTaskStore()
const route = useRoute()
const router = useRouter()

/** “显示已隐藏”开关（契约 v0.23 §6.14.11）。产品经理可能不要：改成 false 即整块去掉（开关、提示条、置灰行都不会出现） */
const SHOW_HIDDEN_TOGGLE = true
/** 按钮文案（产品经理 2026-10-08 定：隐藏已结束） */
const HIDE_FINISHED_LABEL = '隐藏已结束'
const showingHidden = computed(() => SHOW_HIDDEN_TOGGLE && tasks.historyFilter.includeHidden && tab.value !== 'active')
const isHidden = (t: TaskItem) => showingHidden.value && !!t.hiddenInTaskCenter
const hiddenShown = computed(() => (showingHidden.value ? rows.value.filter((t) => t.hiddenInTaskCenter).length : 0))
function setShowHidden(on: boolean) {
  void act(() => tasks.setShowHidden(on))
}
/** 重试（原地，契约 v0.23）：失败 / 已中断 / 已取消的非直播任务 */
/**
 * 操作列宽（表格 table-layout:fixed，任务列拿剩下的宽度）：按当前页最宽的一行估算——“重试”约 64、“取消隐藏”约 88、图标按钮各 30。
 * 这样任务名优先显示，不再被操作区挤成“…”（预审第 1 条；稿子 1024 下任务列约 250px）。
 */
const narrow = useNarrow()
const opsWidth = computed(() => {
  let w = 30
  for (const t of rows.value) {
    let x = 0
    if (canRetry(t)) x += narrow.value && isHidden(t) ? 34 : 64 // 窄窗口下已隐藏行的“重试”只留图标（title 仍是“重试”），给文件名让位
    if (isHidden(t)) x += 88
    let icons = 1 // 日志
    if (t.status === 'queued' || t.status === 'running') icons++
    if (t.status === 'succeeded' && t.outputPath) icons++
    if (isTerminal(t.status)) icons++ // 移除 / 在转换页查看
    w = Math.max(w, x + icons * 30)
  }
  return w
})
const canRetry = (t: TaskItem) => canRetryTask(t)
/** “在转换页查看”：跳到转换页并定位到这条记录（设计 §7.3 第 11 条） */
async function goConvert(t: TaskItem) {
  // 先用 GetSource 确认这一行还在（v0.23.1）；已被删就留在任务中心并提示
  if (t.sourceId) {
    try {
      await getSource(t.sourceId)
    } catch (e) {
      const err = toAppError(e)
      if (err.code === 'NOT_FOUND') return void ElMessage.info('这条转换记录已被删除')
    }
  }
  void router.push({ path: '/', query: { record: t.id, ...(t.sourceId ? { source: t.sourceId } : {}) } })
}

type Tab = 'all' | 'active' | 'history' | 'failed'
const TAB_KEYS: Tab[] = ['all', 'active', 'history', 'failed']
const tab = ref<Tab>(TAB_KEYS.includes(route.query.tab as Tab) ? (route.query.tab as Tab) : 'all')

// 全部 = 进行中 + 已结束；历史 = 已结束（含失败 / 取消 / 中断）；失败只计 failed，不含已中断
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
  { key: 'doc', label: '文档', types: ['office_pdf', 'doc_convert'] },
  { key: 'live', label: '直播', types: ['live_file_push', 'live_screen_push'] },
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
/** 换页签 / 类型 / 页码 / 显示已隐藏 时整个 tbody 重建，新行淡入只在同一个列表里新出现任务时播 */
const rowsKey = computed(() => `${tab.value}|${typeFilter.value}|${tasks.historyFilter.page}|${tasks.historyFilter.includeHidden ? 1 : 0}`)
/** 运行中 / 排队 → 完成 的那一刻，状态标签弹一下 */
const justDone = useJustDone(() => rows.value)
const loading = computed(() => {
  if (tab.value === 'active') return !tasks.ready
  return tasks.historyLoading && !tasks.history.length
})

const emptyText = computed(() => {
  if (tab.value === 'active') return { title: '没有进行中的任务', hint: '在转换或直播页面开始任务后，会显示在这里。' }
  if (tab.value === 'all') return { title: '还没有任务', hint: '在转换或直播页面开始任务后，会显示在这里。' }
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
/**
 * 已下线功能的旧记录（edit_export）：包 24 N3，产品经理 10-08 定——改名不隐藏，类型显示「旧版导出」，
 * 照样能看、能删，不能重试；界面里不出现那个已下线功能的名字。
 */
const RETIRED_INTERRUPTED_TEXT = '应用退出时这个任务被中断。这类任务已不再支持，不能重试，可以移除这条记录。'
const TYPE_LABEL: Record<string, string> = {
  convert: '转换', edit_export: '旧版导出', office_pdf: '文档', doc_convert: '文档', ffmpeg_install: '安装',
  live_file_push: '直播', live_screen_push: '直播',
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

/**
 * 包 24 N4：直播任务进程意外退出（kill、崩溃）时，直播页显示“推流被中断”，但后端任务状态目前落成 failed + INTERNAL。
 * 任务中心按直播页的口径显示：状态「已中断」、标题「推流被中断」/「拉流被中断」，不显示原始错误码，也不用「转换失败」。
 * 连接阶段的失败（LIVE_CONNECT_FAILED、被服务器拒绝等有专属码的）仍按失败显示。后端改成 interrupted 之后这里自然不再命中 failed 分支。
 */
const LIVE_EXIT_CODES = new Set(['', 'INTERNAL', 'PROCESS_FAILED', 'LIVE_PUSH_INTERRUPTED'])
const liveBroken = (t: TaskItem) =>
  isLiveType(t.type) && (t.status === 'interrupted' || (t.status === 'failed' && LIVE_EXIT_CODES.has(t.error?.code ?? '')))
const goneKind = (t: TaskItem) => parseDetailHead(t.error?.detail).kind
/** reason=push / pull 优先；还没有 reason 时按任务类型。窗口关掉走 LIVE_SOURCE_GONE，不进这里 */
const interruptOf = (t: TaskItem) =>
  liveInterruptView({ reason: parseDetailHead(t.error?.detail).reason, code: t.error?.code, taskType: t.type, onLivePage: false })
  ?? { title: `${liveTaskVerb(t.type)}被中断`, description: `请回到直播页重新${liveTaskVerb(t.type)}。`, sentence: '' }
const statusTag = (t: TaskItem) => (liveBroken(t) ? STATUS_TAG.interrupted : STATUS_TAG[t.status])

/** 直播任务的停止文案只看 status（契约 v0.10：succeeded=优雅停止，error 为空；canceled=超时强杀，不带错误码）；其他类型沿用通用文案 */
function statusLabel(t: TaskItem): string {
  if (isLiveType(t.type)) {
    if (t.status === 'succeeded') return LIVE_STOP_TEXT.succeeded
    if (t.status === 'canceled') return LIVE_STOP_TEXT.canceled
  }
  return statusTag(t).label
}

/** 失败行的说明：office_pdf 的 UNSUPPORTED 用产品文案（“暂不支持这种格式，请先另存为 docx、xlsx 或 pptx”），其余沿用后端 message */
function errMessage(t: TaskItem): string | undefined {
  if (t.type === 'office_pdf' && t.error?.code === 'UNSUPPORTED') return docUnsupportedText(t.error.message, t.error.detail)
  // 连接失败：按 detail 首行 scheme=（srt 一句、rtmp/rtmps 一句）；detail 没有时才用脱敏 params.url 的 scheme 兜底
  if (isLiveType(t.type) && t.error?.code === 'LIVE_CONNECT_FAILED') return liveFailureMessage(t.error, schemeFromParams(t.params))
  return t.error?.message
}

/** 接口层模拟出来的任务（后端还没接入时的演示数据），任务中心里加“演示”标记，避免被当成真实任务 */
const isSim = (t: TaskItem) => isSimTask(t.id)
/** 演示任务的标题去掉“【演示】”前缀（由标签代替） */
const shownTitle = (t: TaskItem) => (isSim(t) && t.title.startsWith(SIM_TITLE_PREFIX) ? t.title.slice(SIM_TITLE_PREFIX.length) : t.title) || fileBaseName(t.outputPath)

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

/** 进度条上方的小字：失败 / 已中断只放百分比，原因在下面的错误行（走查 X10：1024 下“应用退出，已中断”被省略成“应用…”） */
const barText = (t: TaskItem): string => (t.status === 'failed' || t.status === 'interrupted' ? '' : progressText(t))
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
      {
        const ms = elapsedMs(t.startedAt, t.finishedAt)
        if (isLiveType(t.type)) return ms !== null ? `推流 ${formatDuration(ms)}` : '推流已结束'
        return ms !== null ? `用时 ${formatDuration(ms)}` : '已完成'
      }
    case 'failed':
      return liveBroken(t) ? '已中断' : '失败' // 包 24 N4：直播意外退出按中断显示
    case 'interrupted':
      return isLiveType(t.type) && t.error?.code ? '已中断' : '应用退出，已中断'
    case 'canceled':
      return isLiveType(t.type) ? LIVE_STOP_TEXT.canceled : '用户取消'
  }
}

// ---- 1024 下进度列的短写（复验 N3）：列宽 104px，速度不省略、“用时”不折行 ----
/** 秒数 → m:ss / h:mm:ss（937 → "15:37"） */
function shortClock(sec: number): string {
  const s = Math.max(0, Math.round(sec))
  const p = (n: number) => String(n).padStart(2, '0')
  return s >= 3600 ? `${Math.floor(s / 3600)}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}` : `${Math.floor(s / 60)}:${p(s % 60)}`
}
const narrowRun = (t: TaskItem) => t.status === 'running' && !isLiveType(t.type)
/** 进度条上方左边：只放速度（没有速度时放“处理中”）；其余状态和宽屏一样 */
const narrowSpeed = (t: TaskItem): string => (narrowRun(t) ? t.speed || (formatEta(t.etaSec) ? '' : '处理中') : barText(t))
/** 进度条下面：剩余 0:52 */
const narrowEta = (t: TaskItem): string => (narrowRun(t) && t.etaSec > 0 && isFinite(t.etaSec) ? `剩余 ${shortClock(t.etaSec)}` : '')
/** 没有进度条的行：完成的“用时 15 分 37 秒”短写成“用时 15:37”；其余不变（返回 ''） */
function progressShort(t: TaskItem): string {
  if (t.status !== 'succeeded') return ''
  const ms = elapsedMs(t.startedAt, t.finishedAt)
  if (ms === null || ms < 60_000) return ''
  return `${isLiveType(t.type) ? '推流' : '用时'} ${shortClock(ms / 1000)}`
}

/** 第二行小字：从 params 里取转换选项，取不到就显示输出文件名；参数格式不假设，解析失败静默跳过 */
function subInfo(t: TaskItem): string {
  if (t.type === 'ffmpeg_install') return '下载并安装到应用目录'
  // 转换行：和转换页子记录第 2 行同一个格式化函数（不带时间；设计 §7.3 第 14 条）
  if (t.type === 'convert') return recordParamsText({ ...parseParams(t.params), title: shownTitle(t) }).text
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

/** 转换行第二行的悬停：预设记录给出参数摘要（设计 §7.3 第 11 / 14 条） */
function subTitle(t: TaskItem): string | undefined {
  if (t.type !== 'convert') return undefined
  const snap = parseParams(t.params)
  const p = recordParamsText({ ...snap, title: t.title })
  return p.preset && p.tip ? p.tip : undefined
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
    const err = toAppError(e)
    ElMessage.error(actionErrorText(err.code, err.message))
  }
}
/** 重试：转换任务原地重试（同一条记录、同一个任务 id，旧错误和回退提示清掉），由任务 store 处理；与转换页的重试是同一条路径 */
async function doRetry(t: TaskItem) {
  if (tasks.isBusy(t.id)) return // 该任务已有重试 / 换输出位置在途：忽略连点
  await act(async () => {
    const nt = await tasks.retry(t.id)
    if (!nt) return // 被 store 的在途保护忽略
    ElMessage.success('已重新提交')
    if (tab.value === 'failed') setTab('active')
  })
}
/**
 * 「更换输出位置」（磁盘空间不足时）：弹系统选择文件夹对话框（PickDirectory），
 * 选中的文件夹只用于「这一次」，不会写进设置里的默认输出位置（设计师确认）。
 *
 * convert 任务：params 是 {input, options, outputDir}（契约 6.9），能完整解析时用原输入和原参数、
 * 新文件夹调用 ConvertService.Submit 重新提交；原失败任务保留在历史里。
 * params 解析不出来（旧数据 / 格式不符）就不猜，先不弹选择框，保留提示让用户清理磁盘后点「重试」。
 * ffmpeg_install 的安装位置固定在应用目录，不涉及输出文件夹，不弹选择框。
 */
async function changeOutput(t: TaskItem) {
  if (t.type === 'ffmpeg_install') {
    ElMessage.info('转换组件安装在应用目录，位置不能更换。')
    return
  }
  const params = t.type === 'convert' ? parseConvertParams(t.params) : null
  if (!params) {
    ElMessage.info('没能读到这个任务的原始参数，暂时不能换输出位置。请先清理磁盘空间后点“重试”。')
    return
  }
  if (tasks.isBusy(t.id)) return
  await act(async () => {
    // 与「重试」共用同一个在途保护（按原任务 id）：选目录的对话框开着、或提交未返回时，重试 / 换位置都被忽略
    await tasks.exclusive(t.id, async () => {
      const chosenDir = await pickDirectory('选择这次转换的输出文件夹')
      if (!chosenDir) return // 用户取消
      const nt = await resubmitToDir(params, chosenDir)
      tasks.track([nt])
      ElMessage.success('已用新的输出位置重新提交')
      if (tab.value === 'failed') setTab('active')
    })
  })
}
async function openOutput(t: TaskItem) {
  try {
    await revealInFolder(t.outputPath)
  } catch (e) {
    const err = toAppError(e)
    // 该平台 / 该对象不支持显示位置：退回复制路径；其他错误（文件已不存在等）如实提示
    if (err.code === 'UNSUPPORTED' || err.code === 'UNSUPPORTED_PLATFORM') {
      try {
        await navigator.clipboard.writeText(t.outputPath)
        ElMessage.info(`已复制输出路径：${t.outputPath}`)
      } catch {
        ElMessage.info(t.outputPath)
      }
    } else {
      ElMessage.error(actionErrorText(err.code, err.message))
    }
  }
}

interface Confirm { title: string; text: string; ok: string; canDeleteOutput: boolean; run: () => Promise<void>; /** 不删除东西的确认（隐藏）：主按钮用主色，不用红色 */ safe?: boolean }
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
/**
 * “隐藏已结束”（产品决定 v1：原“清除已结束”改为只隐藏）：已结束的任务只从任务中心列表里隐藏，不删除记录、日志和文件；
 * 转换任务的记录仍在格式转换页的“转换记录”里，真正删除只在转换页做。
 */
function askHide() {
  confirm.value = {
    title: '隐藏所有已结束的任务？',
    text: '只从任务中心隐藏，不删除记录和文件。转换记录仍可以在格式转换页查看。进行中的任务不受影响。',
    ok: '隐藏',
    canDeleteOutput: false,
    safe: true,
    run: async () => {
      closeLog()
      await tasks.hideFinished()
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
const logWrapEl = ref<HTMLElement | null>(null)
const logTask = computed<TaskItem | undefined>(() => {
  if (!logId.value) return undefined
  return tasks.active.find((t) => t.id === logId.value) ?? tasks.history.find((t) => t.id === logId.value)
})
const encDevices = useEncoderDeviceList()
const logDevice = computed(() => usedDeviceText(logTask.value, encDevices.value))
const logDeviceFb = computed(() => logDevice.value === ENCODER_DEVICE_CPU_FALLBACK_NAME)
const logFallbackReason = computed(() => (showFallbackNotice(logTask.value) ? encoderFallbackReasonText(logTask.value?.hwFallbackReason) : ''))
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
    revealLog()
  }
}
/**
 * 打开日志后把日志面板滚进视口并把焦点移过去：面板在列表下方，长列表里点了“查看日志”屏幕上看不出变化。
 * 选滚动而不是就地展开：面板本来就是整页共用的一块（不随行变化），滚动不改动列表布局；behavior 尊重“减少动效”；
 * 焦点放到面板（tabindex=-1，读屏会读“任务日志面板”区域），preventScroll 避免二次跳动。关闭时焦点回到触发它的按钮。
 */
async function revealLog() {
  await nextTick()
  const el = logWrapEl.value
  if (!el) return
  el.scrollIntoView({ block: 'nearest', behavior: scrollBehavior() })
  el.focus({ preventScroll: true })
  // 面板是展开着进来的（MotionCollapse，200ms）：展开完再对一次位置，免得只露出半截
  if (!prefersReducedMotion()) setTimeout(() => logWrapEl.value === el && el.scrollIntoView({ block: 'nearest', behavior: scrollBehavior() }), DUR.base + 20)
}
function closeLog() {
  const id = logId.value
  const hadFocus = !!logWrapEl.value && logWrapEl.value.contains(document.activeElement)
  logId.value = null
  logText.value = ''
  if (id && hadFocus) nextTick(() => document.querySelector<HTMLElement>(`[data-logbtn="${CSS.escape(id)}"]`)?.focus())
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
  // 转换行第二行的预设名和转换页共用规则：同名预设（MP4 H.264 / H.265）补编码，需要当前预设列表（走查 G2）
  void listPresets().then((ps) => setPresetCatalog(ps.map((p) => p.name))).catch(() => undefined)
  // “显示已隐藏”默认关、不记住（设计 §7.3 第 11 条）：每次进任务中心都从关开始
  tasks.historyFilter.includeHidden = false
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
  font-size: 12px;
  color: var(--ff-text-2);
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
.simtag {
  display: inline-block;
  margin-right: 6px;
  padding: 0 6px;
  border: 1px solid var(--ff-border);
  border-radius: 4px;
  font-size: 11px;
  font-weight: 400;
  line-height: 18px;
  color: var(--ff-text-2);
  vertical-align: 1px;
}
.fname {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  max-width: 100%;
  display: flex; /* “演示”标记 + 文件名（MidEllipsis：放不下时中间省略、保留扩展名和“→ 格式”，复核 N1） */
  align-items: center;
  min-width: 0;
}
.fname :deep(.mid-el) {
  flex: 0 1 auto;
}
td:first-child {
  max-width: 0; /* 让长文件名在表格里省略而不是撑宽列 */
}
/* 固定列宽，任务列拿剩下的（预审第 1 条：任务名优先显示，太长才在末尾省略）。窗口最小 1024 宽时表格约 774px，
   窄窗口下单元格左右内边距从 16 收到 12，各列也收窄一点（见下面的 @media） */
.tbl { table-layout: fixed; }
.c-type { width: 72px; }
.c-st { width: 104px; }
.c-prog { width: 20%; }
.c-when { width: 112px; }
.opsh { width: calc(var(--ops-w, 120px) + 32px); }
th { white-space: nowrap; } /* “开始时间”不换行 */
@media (max-width: 1100px) {
  th,
  td { padding-left: 12px; padding-right: 12px; }
  tr.errrow > td { padding-left: 12px; padding-right: 12px; }
  .opsh { width: calc(var(--ops-w, 120px) + 24px); }
  .simtag { display: none; } /* 窄窗口下“演示”标记只留在 title 里，给文件名让位 */
  tr.hid .tc-rt .lbl { display: none; }
  .c-type { width: 56px; }
  .c-st { width: 80px; } /* 复核 N1：状态列收到 80；进度列保留 104（92 时“用时 5 分 34 秒”会折行），任务列仍有约 250 */
  .c-prog { width: 104px; }
  .c-when { width: 92px; }
}
.finfo {
  font-size: 12px;
  color: var(--ff-text-2);
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
.pline span:last-child,
.pline .spd,
.plain .narrow,
.peta {
  white-space: nowrap;
}
.pline .spd {
  flex: none;
  overflow: visible;
}
.peta {
  color: var(--ff-text-2);
}
.narrow {
  display: none;
}
@media (max-width: 1100px) {
  .prog .wide,
  .plain .wide {
    display: none;
  }
  .narrow {
    display: inline;
  }
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
/* 减少动效：prefers-reduced-motion 与 html.reduce-motion 钩子统一放在 styles/base.css */
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
.btn.pri {
  background: var(--ff-badge-bg);
  border-color: var(--ff-badge-bg);
  color: var(--ff-on-primary);
}
.btn.danger {
  background: var(--ff-danger);
  border-color: var(--ff-danger);
  color: var(--ff-on-danger);
}
/* 必须写在 .btn:hover 之后：否则通用悬停底色 bg-hover 会盖住危险按钮，文字几乎看不见 */
.btn.danger:hover:not(:disabled) {
  background: color-mix(in srgb, var(--ff-danger) 88%, #000);
  border-color: transparent;
}
/* 暗色下 --ff-on-danger 是近黑字，把底色压暗会让对比度掉到 4.15:1，所以暗色改为向白色提亮 12%（≈5.6:1） */
:global(html.dark) .btn.danger:hover:not(:disabled) {
  background: color-mix(in srgb, var(--ff-danger) 88%, #fff);
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
.logwrap:focus {
  outline: none;
}
.logwrap:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
  border-radius: var(--ff-radius-md);
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
.logdev .dv {
  min-width: 0;
  max-width: 100%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.logdev {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  font-size: 12px;
  color: var(--ff-text-2);
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

/* “显示已隐藏”（契约 v0.23 §6.14.11；设计 state=taskcenter-hidden） */
.tc-sw {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--ff-text-1);
  white-space: nowrap;
  flex: none;
  cursor: pointer;
}
.tc-hint {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 12px 0;
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 12px;
  color: var(--ff-text-2);
  background: var(--ff-bg-hover);
  white-space: nowrap;
  min-width: 0;
}
.tc-hint > svg {
  color: var(--ff-text-3);
  flex: none;
}
.tc-hint > span {
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
@media (max-width: 1100px) {
  .tc-hint .w1280 {
    display: none;
  }
}
.tc-nm {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.tc-nm .fname {
  min-width: 0;
}
/* 已隐藏的行：内容置灰（约 0.55），操作列和“已隐藏”标签不置灰 */
tr.hid > td {
  background: color-mix(in srgb, var(--ff-bg-hover) 55%, transparent);
}
tr.hid .tc-dim {
  opacity: 0.55;
}
.tc-hidtag {
  height: 18px;
  padding: 0 6px;
  border-radius: 4px;
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex: none;
  white-space: nowrap;
  color: var(--ff-text-2);
  border: 1px dashed var(--ff-text-3);
}
.btn.sm.tc-unh {
  color: var(--ff-primary-text);
}
</style>
