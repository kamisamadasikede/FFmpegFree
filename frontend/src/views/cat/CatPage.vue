<template>
  <div class="cat-page cat-root" data-testid="cat-page">
    <!-- 进入时：侧栏 250ms 收起（AppSidebar 的 cat-away），中间先显示约 1.2 秒 loading，再出三栏聊天页 -->
    <!-- A1 进入：先画三栏骨架，超过 300ms 才出现闪光 -->
    <div v-if="phase === 'loading'" class="ct-load" role="status" aria-live="polite">
      <span class="sr-only">正在进入 Cat…</span>
      <div class="sk-rail" aria-hidden="true">
        <i class="sk w60" /><i class="sk w90 tall" /><i class="sk w40 gap" /><i v-for="n in 6" :key="n" class="sk" :class="n % 2 ? 'w80' : 'w60'" />
      </div>
      <div class="sk-main" aria-hidden="true">
        <i class="sk w30 hd" />
        <div class="sk-msgs"><i class="sk w50 r" /><i class="sk w90" /><i class="sk w80" /><i class="sk w60" /></div>
        <i class="sk comp" />
      </div>
    </div>
    <div v-else class="ct cat-in" :class="{ 'no-right': isNew || catPanels.view !== 'chat' }">
      <CatSidePanel @back="goBack" />

      <CatSettingsView v-if="catPanels.view === 'settings'" />
      <CatSchedulesView v-else-if="catPanels.view === 'schedules'" />
      <section v-else class="ct-c" aria-label="对话">
        <!-- 中间顶部浮提示（设计 §07：重复文件夹等），2.4s 自动消失 -->
        <div
          v-if="catState.notice"
          :key="catState.notice.key"
          class="ct-notice"
          :class="[catState.notice.tone, { low: projectMissing }]"
          role="status"
          data-testid="cat-notice"
        >
          <FIcon :name="catState.notice.tone === 'ok' ? 'check' : catState.notice.tone === 'warn' ? 'warn' : 'info'" :size="14" />{{ catState.notice.text }}
        </div>
        <!-- 新对话欢迎页 -->
        <template v-if="isNew">
          <div class="ct-hd">
            <template v-if="currentProject">
              <span class="crumb-p" :class="{ miss: currentProject.missing }">
                <FIcon :name="currentProject.missing ? 'warn' : 'folder'" :size="15" />{{ currentProject.name }}
              </span><span class="slash">/</span>
            </template>
            <FIcon v-else name="chat" :size="16" />
            <h2>新对话</h2><span class="sp" />
          </div>
          <div v-if="projectMissing" class="ct-missing" role="status" data-testid="cat-project-missing-bar">
            <FIcon name="warn" :size="14" /><span class="tx">{{ PC.missing }}</span>
            <button
              v-if="currentProject"
              type="button"
              class="relink"
              data-testid="cat-project-relocate-bar"
              @click="relocateProject(currentProject.id)"
            >{{ PC.relocate }}</button>
          </div>
          <div :key="'new'" class="ct-welcome swap">
            <h3 class="wl-hi">Hi，今天有什么安排?</h3>
            <div class="wl-modes" role="tablist" aria-label="模式">
              <button
                v-for="m in CAT_MODES"
                :key="m.id"
                type="button"
                class="wl-mode"
                :class="{ on: m.enabled && catState.mode === m.id, disabled: !m.enabled }"
                role="tab"
                :aria-selected="m.enabled && catState.mode === m.id"
                :aria-disabled="!m.enabled"
                :title="m.enabled ? undefined : tipLater()"
                :data-testid="'cat-mode-' + m.id"
                @click="onModeClick(m)"
              >
                <CatAgentMark v-if="m.mark" :kind="m.mark" :size="15" />
                <FIcon v-else :name="m.icon" :size="15" /><span>{{ m.name }}</span>
              </button>
              <button
                type="button"
                class="wl-mode more disabled"
                aria-haspopup="true"
                aria-disabled="true"
                :title="tipLater()"
                @click="onLater()"
              >更多<FIcon name="down" :size="12" /></button>
            </div>
            <div v-if="catNotReady" :key="bannerKick" class="ct-banner" :class="{ kick: bannerKick > 0 }" role="status" data-testid="cat-not-ready">
              <FIcon name="info" :size="14" /><span class="tx">{{ notReadyText }}</span>
              <button type="button" class="recheck" :disabled="catChecking" data-testid="cat-recheck" @click="recheckCatStatus()">重新检测</button>
            </div>
            <CatComposer
              ref="composer"
              welcome
              placeholder="发消息、上传文件、打开文件夹、创建定时任务，或输入 / 唤起命令…"
              :busy="catState.creating"
              :not-ready="catNotReady"
              :checking="catChecking"
              :missing="projectMissing"
              :project-name="currentProject?.name ?? ''"
              @send="onSend($event, composer)"
              @blocked="kickBanner"
            />
            <div class="wl-try">
              <div class="wl-try-hd">试试这些指令</div>
              <button v-for="t in CAT_TRY" :key="t" type="button" class="wl-cmd" @click="composer?.setDraft(t)">{{ t }}</button>
            </div>
          </div>
        </template>
        <!-- 对话 -->
        <template v-else-if="current">
          <div class="ct-hd">
            <template v-if="current.project">
              <span class="crumb-p" :class="{ miss: current.project.missing }">
                <FIcon :name="current.project.missing ? 'warn' : 'folder'" :size="15" />{{ current.project.name }}
              </span><span class="slash">/</span>
            </template>
            <FIcon v-else name="chat" :size="16" />
            <h2 :title="current.conv.title">{{ current.conv.title }}</h2>
            <span v-if="current.conv.main" class="main-tag">主要</span>
            <span class="sp" />
            <button
              type="button"
              class="hd-ic"
              title="搜索"
              aria-label="搜索"
              data-testid="cat-conv-search"
              :aria-expanded="searchOn"
              @click="searchOn = !searchOn"
            >
              <FIcon name="search" :size="16" />
            </button>
            <button
              type="button"
              class="hd-ic"
              title="重新加载"
              aria-label="重新加载"
              data-testid="cat-conv-refresh"
              @click="onRefreshConv"
            >
              <FIcon name="refresh" :size="16" />
            </button>
            <CatBranchCapsule />
            <button type="button" class="hd-ib" :class="{ on: !rightCollapsed }" :title="rightCollapsed ? '展开右栏 (Ctrl J)' : '收起右栏 (Ctrl J)'" :aria-label="rightCollapsed ? '展开右栏' : '收起右栏'" :aria-pressed="!rightCollapsed" data-testid="cat-right-toggle" @click="rightCollapsed = !rightCollapsed"><FIcon name="split" :size="16" /></button>
          </div>
          <CatConvSearch v-if="searchOn" :blocks="messages" @close="searchOn = false" @pick="jumpSearch" />
          <div v-if="projectMissing" class="ct-missing" role="status" data-testid="cat-project-missing-bar">
            <FIcon name="warn" :size="14" /><span class="tx">{{ PC.missing }}</span>
            <button
              v-if="currentProject"
              type="button"
              class="relink"
              data-testid="cat-project-relocate-bar"
              @click="relocateProject(currentProject.id)"
            >{{ PC.relocate }}</button>
          </div>
          <div class="ct-stream">
            <div v-if="historyEmpty" class="ct-hist-empty" data-testid="cat-history-empty">{{ CAT_HISTORY_EMPTY }}</div>
            <div ref="msgsEl" :key="catState.sel" class="ct-msgs swap" @scroll.passive="onMsgsScroll">
              <div ref="msgsBody">
                <!-- B1：读记录超过 150ms 才显示消息骨架 -->
                <div v-if="histLoading && !messages.length" class="sk-msgs conv" role="status" aria-label="正在加载对话" data-testid="cat-msg-skeleton">
                  <i class="sk w50 r" /><i class="sk w90" /><i class="sk w80" /><i class="sk w60" /><i class="sk w40 r" /><i class="sk w90" /><i class="sk w70" />
                </div>
                <CatMessages :blocks="messages" :pending="thinking" :retry="catState.retry[catState.sel]" @retry="retryLastMessage" />
                <CatOfficeDownloadCard v-if="officeCanvas.slidesNeeded && !isNew" />
              </div>
            </div>
            <div
              v-show="overflowing && jumpH > 0"
              ref="jumpsEl"
              class="ct-jumps"
              :style="{ height: jumpH + 'px' }"
              data-testid="cat-msg-jumps"
              aria-label="消息位置"
              @wheel.stop="onJumpsWheel"
              @scroll.passive="onJumpsScroll"
              @mousemove="onJumpsMove"
              @mouseleave="onJumpsLeave"
            >
              <div class="ct-jump-in" :style="{ height: jumpContent + 'px' }">
                <button
                  v-for="t in ticks"
                  :key="t.key"
                  type="button"
                  class="ct-tick"
                  :class="{ on: t.tone === 0, near: t.tone === 1 }"
                  :style="{ top: t.top + 'px', '--tick-w': t.w + 'px' }"
                  :title="t.label || '用户消息'"
                  :aria-label="t.label || '用户消息'"
                  data-testid="cat-msg-tick"
                  @click.stop="jumpUser(t.offset)"
                />
              </div>
            </div>
            <div v-show="overflowing" ref="railEl" class="ct-rail" data-testid="cat-msg-rail" aria-label="滚动位置" @pointerdown="onTrackDown">
              <div
                v-if="thumb"
                class="ct-thumb"
                :style="{ top: thumb.top + 'px', height: thumb.height + 'px' }"
                title="拖动滚动"
                role="scrollbar"
                aria-orientation="vertical"
                aria-label="拖动滚动"
                data-testid="cat-msg-thumb"
                @pointerdown.stop="onThumbDown"
              />
            </div>
            <button v-if="away" type="button" class="ct-jump" title="回到底部" aria-label="回到底部" data-testid="cat-jump-bottom" @click="jumpBottom">
              <FIcon name="down" :size="16" />
            </button>
          </div>
          <div v-if="catNotReady" :key="bannerKick" class="ct-banner in-conv" :class="{ kick: bannerKick > 0 }" role="status" data-testid="cat-not-ready">
            <FIcon name="info" :size="14" /><span class="tx">{{ notReadyText }}</span>
              <button type="button" class="recheck" :disabled="catChecking" data-testid="cat-recheck" @click="recheckCatStatus()">重新检测</button>
          </div>
          <CatComposer
            ref="convComposer"
            :ctx-name="current.project?.name ?? 'Cat'"
            :ctx-branch="current.project?.branch ?? 'main'"
            :running="!!turn"
            :stopping="turn?.status === 'stopping'"
            :not-ready="catNotReady"
            :checking="catChecking"
            :missing="projectMissing"
            @send="onSend($event, convComposer)"
            @blocked="kickBanner"
            @stop="stopTurn()"
          />
        </template>
      </section>

      <!-- 右栏收起时内容保留，不重新加载 -->
      <div v-if="!isNew && catPanels.view === 'chat'" class="ct-r-wrap" :class="{ shut: rightCollapsed, canvas: showOfficeCanvas }" :aria-hidden="rightCollapsed || undefined">
        <CatOfficeCanvasPanel v-if="showOfficeCanvas" @close="officeCanvasClosed = true" />
        <CatFilesPanel v-else :conv-id="catState.sel" :missing="projectMissing" @pick="onFilePick" />
      </div>
    </div>
    <CatProjectDeleteDialog />
    <CatAddProjectDialog />
    <CatScheduleToast />
  </div>
</template>

<script setup lang="ts">
import { CAT_HISTORY_EMPTY } from '@/views/cat/catCopy'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import CatSidePanel from '@/components/cat/CatSidePanel.vue'
import CatFilesPanel from '@/components/cat/CatFilesPanel.vue'
import CatBranchCapsule from '@/components/cat/CatBranchCapsule.vue'
import CatComposer from '@/components/cat/CatComposer.vue'
import CatMessages from '@/components/cat/CatMessages.vue'
import CatConvSearch from '@/components/cat/CatConvSearch.vue'
import CatProjectDeleteDialog from '@/components/cat/CatProjectDeleteDialog.vue'
import CatAddProjectDialog from '@/components/cat/CatAddProjectDialog.vue'
import CatSettingsView from '@/components/cat/panels/CatSettingsView.vue'
import CatAgentMark from '@/components/cat/CatAgentMark.vue'
import CatSchedulesView from '@/components/cat/panels/CatSchedulesView.vue'
import CatScheduleToast from '@/components/cat/panels/CatScheduleToast.vue'
import CatOfficeDownloadCard from '@/components/cat/CatOfficeDownloadCard.vue'
import CatOfficeCanvasPanel from '@/components/cat/CatOfficeCanvasPanel.vue'
import { useOfficeCanvasStore } from '@/stores/officeCanvas'
import type { PickNote } from '@/components/cat/catPick'
import { catPanels, initCatPanels } from './catPanels'
import { CAT_PROJECT_COPY as PC } from '@/api/catProjects'
import { ElMessage } from 'element-plus'
import { CAT_COPY } from '@/api/cat'
import { CAT_MODES, CAT_TRY, type CatMode } from '@/api/catMock'
import { catReturnPath } from './catReturn'
import { nearBottom, keepFollowing } from './catFollow'
import { RAIL_TICK_PITCH, railThumb, railTickWindow, scrollTopForThumb, tickEdgeScales, tickStairWidths } from './catRail'
import { awaitingTurnOutput } from './catPending'
import { onLayoutKey, rightCollapsed } from './catLayout'
import { catChecking, catNotReady, catState, changeWelcomeMode, currentProject, findConv, projectMissing, relocateProject, initCat, messagesOf, NEW_CONV, reapplyNewConvDefaults, refreshCapabilities, recheckCatStatus, reloadConversation, selectConv, retryLastMessage, sendMessage, stopTurn, tipLater } from './catState'
import { hasWailsBackend } from '@/services/wails'
import { ClipboardSetText } from '../../../wailsjs/runtime/runtime'

/**
 * Cat 聊天页（设计 v0.4 / 契约 v0.30）。Wails 里接真实 CatService：流式回复、停止生成、组件未就绪横条（无下载按钮）。
 * 纯浏览器走查仍是布局壳 + typed mock。
 * 浏览器走查可加 ?cat=chat 跳过 1.2 秒 loading、?cat_conv=new|c21|d1… 直接打开某条对话（仅纯浏览器，Wails 里无效）。
 */
defineOptions({ name: 'CatPage' })

const officeCanvas = useOfficeCanvasStore()
const officeCanvasClosed = ref(false)
const showOfficeCanvas = computed(() => officeCanvas.showPreviewShell && !officeCanvasClosed.value)

const router = useRouter()
const preview = typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go
const q = new URLSearchParams(location.search)
const phase = ref<'loading' | 'chat'>(preview && q.get('cat') === 'chat' ? 'chat' : 'loading')
if (preview && q.get('cat_conv')) catState.sel = q.get('cat_conv') as string

const notReadyText = computed(() => catState.status.error?.message || CAT_COPY.notReady)
const isNew = computed(() => catState.sel === NEW_CONV)
const histLoading = computed(() => !!catState.histLoading[catState.sel])
/** 打开的对话一轮都没有（记录没找到）：居中灰字，输入框照常可用 */
const historyEmpty = computed(
  () => !isNew.value && !!catState.histLoaded[catState.sel] && !histLoading.value && !messagesOf(catState.sel).length && !catState.turns[catState.sel],
)
const current = computed(() => (isNew.value ? null : findConv(catState.sel)))
const messages = computed(() => (isNew.value ? [] : messagesOf(catState.sel)))
const composer = ref<InstanceType<typeof CatComposer>>()
const convComposer = ref<InstanceType<typeof CatComposer>>()
/** G2：未就绪时按 Enter 让横幅抖一次（改 key 重放动画） */
const bannerKick = ref(0)
function kickBanner() {
  bannerKick.value++
}
/** 发送；后端没收下（项目文件夹不见了）时把文字和文件放回输入框 */
function onFilePick(note: PickNote) {
  convComposer.value?.addPick(note)
}
async function onSend(payload: { text: string; files: { name: string; path: string }[]; own: string; notes: PickNote[] }, c: InstanceType<typeof CatComposer> | undefined) {
  stick.value = true
  const back = await sendMessage(payload.text, payload.files)
  if (back) {
    c?.restoreDraft(payload.own)
    c?.restoreFiles(back.files)
    if (payload.notes.length) c?.restorePicks(payload.notes)
  }
  nextTick(pinBottom)
}
/** 当前会话进行中的一轮 */
const turn = computed(() => (isNew.value ? undefined : catState.turns[catState.sel]))
/** 这一轮还没收到正文、思考或工具调用时马上显示「正在思考…」。上一轮的内容不算。 */
const thinking = computed(() => {
  if (!turn.value || turn.value.status !== 'running') return false
  return awaitingTurnOutput(messages.value)
})
/** 点刷新后整页加载至少停这么久，转圈才看得到。 */
const RELOAD_HOLD_MS = 500
let reloadTimer: ReturnType<typeof setTimeout> | undefined
let pageAlive = true
function holdReload(ms: number) {
  return new Promise<void>((resolve) => {
    reloadTimer = setTimeout(() => {
      reloadTimer = undefined
      resolve()
    }, ms)
  })
}
async function onRefreshConv() {
  const id = catState.sel
  if (!id || id === NEW_CONV || phase.value === 'loading') return
  searchOn.value = false
  const started = Date.now()
  phase.value = 'loading'
  try {
    await reloadConversation(id)
  } finally {
    const left = RELOAD_HOLD_MS - (Date.now() - started)
    if (left > 0) await holdReload(left)
    if (pageAlive) phase.value = 'chat'
  }
}
const msgsEl = ref<HTMLElement>()
const msgsBody = ref<HTMLElement>()
const railEl = ref<HTMLElement>()
/** 人在看最新一条时，回复变高就跟着往下；往上翻过之后停住 */
const stick = ref(true)
const searchOn = ref(false)
function jumpSearch(index: number) {
  searchOn.value = false
  nextTick(() => {
    const el = msgsEl.value?.querySelector(`[data-cat-user="user:${index}"]`)
    if (!(el instanceof HTMLElement)) return
    stick.value = false
    el.scrollIntoView({ block: 'center' })
  })
}
/** 人把消息往上翻开之后才出现「回到底部」 */
const away = ref(false)
const overflowing = ref(false)
const ticks = ref<{ key: string; top: number; offset: number; label: string; w: number; tone: number }[]>([])
const jumpH = ref(0)
const jumpContent = ref(0)
const jumpsEl = ref<HTMLElement>()
/** 鼠标正停在哪一条上。-1 时全部一样长，不在中间鼓起。 */
const hoverTick = ref(-1)
let railAnim = 0
const thumb = ref<{ top: number; height: number } | null>(null)
/** 和 .ct-rail 的 top/bottom 留白一致 */
const RAIL_INSET = 8
let pinning = false
let pinnedOnce = false
let followObserver: ResizeObserver | undefined

function trackHeight(el: HTMLElement): number {
  return Math.max(0, el.clientHeight - RAIL_INSET * 2)
}

function measureRail() {
  const el = msgsEl.value
  if (!el) {
    overflowing.value = false
    ticks.value = []
    jumpH.value = 0
    jumpContent.value = 0
    thumb.value = null
    away.value = false
    return
  }
  const sh = el.scrollHeight
  const ch = el.clientHeight
  const track = trackHeight(el)
  overflowing.value = sh > ch + 1
  away.value = pinnedOnce && !nearBottom(el.scrollTop, sh, ch)
  const nodes = Array.from(el.querySelectorAll<HTMLElement>('[data-cat-user]'))
  const win = railTickWindow(nodes.length, track)
  const next = nodes.map((n, i) => {
    const offset = n.getBoundingClientRect().top - el.getBoundingClientRect().top + el.scrollTop
    const label = (n.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 80)
    return {
      key: n.dataset.catUser || String(i),
      top: win.tops[i] ?? 0,
      offset,
      label,
      w: 8,
      tone: 2,
    }
  })
  ticks.value = next
  jumpH.value = win.box
  jumpContent.value = win.content
  thumb.value = railThumb(el.scrollTop, sh, ch, track)
  nextTick(paintTickStairs)
}

function paintTickStairs() {
  const el = jumpsEl.value
  const list = ticks.value
  if (!list.length) return
  if (hoverTick.value < 0) {
    const scales = tickEdgeScales(list.length, el?.scrollTop ?? 0, el?.clientHeight || jumpH.value, RAIL_TICK_PITCH)
    list.forEach((t, i) => {
      const w = Math.round(8 * (scales[i] ?? 1))
      if (t.w !== w) t.w = w
      if (t.tone !== 2) t.tone = 2
    })
    return
  }
  const focus = Math.min(hoverTick.value, list.length - 1)
  const widths = tickStairWidths(list.length, focus)
  list.forEach((t, i) => {
    const w = widths[i] ?? 8
    const tone = i === focus ? 0 : Math.abs(i - focus) === 1 ? 1 : 2
    if (t.w !== w) t.w = w
    if (t.tone !== tone) t.tone = tone
  })
}

function onJumpsMove(e: MouseEvent) {
  const el = jumpsEl.value
  if (!el || !ticks.value.length) return
  const y = e.clientY - el.getBoundingClientRect().top + el.scrollTop
  const i = Math.min(ticks.value.length - 1, Math.max(0, Math.floor(y / RAIL_TICK_PITCH)))
  if (hoverTick.value === i) return
  hoverTick.value = i
  paintTickStairs()
}

function onJumpsLeave() {
  if (hoverTick.value < 0) return
  hoverTick.value = -1
  paintTickStairs()
}

function onJumpsWheel() {
  cancelAnimationFrame(railAnim)
}

function onJumpsScroll() {
  paintTickStairs()
}

/** 对话往上或往下滚时，左侧整列跟着滑，能看出方向。 */
function animateRailTo(next: number) {
  const rail = jumpsEl.value
  if (!rail) return
  cancelAnimationFrame(railAnim)
  const from = rail.scrollTop
  const dist = next - from
  if (Math.abs(dist) < 0.5) return
  const t0 = performance.now()
  const step = (now: number) => {
    const el = jumpsEl.value
    if (!el) return
    const p = Math.min(1, (now - t0) / 220)
    const eased = 1 - (1 - p) ** 3
    el.scrollTop = from + dist * eased
    if (p < 1) railAnim = requestAnimationFrame(step)
  }
  railAnim = requestAnimationFrame(step)
}

function syncJumpsToChat() {
  const rail = jumpsEl.value
  const msgs = msgsEl.value
  if (!rail || !msgs || hoverTick.value >= 0) return
  const span = msgs.scrollHeight - msgs.clientHeight
  const railSpan = rail.scrollHeight - rail.clientHeight
  if (railSpan <= 0) return
  const progress = span <= 0 ? 1 : msgs.scrollTop / span
  animateRailTo(Math.max(0, Math.min(railSpan, progress * railSpan)))
}

// 选中的对话不存在（例如被删掉）时回到欢迎页。列表还在加载时先不判断，避免把记下的对话清掉
watch(current, (c) => {
  if (!catState.convListReady) return
  if (!isNew.value && !c) catState.sel = NEW_CONV
})

let timer: ReturnType<typeof setTimeout> | undefined
let disposeCat: (() => void) | undefined
let stopReady: (() => void) | undefined
function pinBottom() {
  const el = msgsEl.value
  if (!el || !stick.value) return
  const top = el.scrollHeight - el.clientHeight
  if (top <= 0 || el.scrollTop >= top - 1) {
    pinnedOnce = true
    return
  }
  pinning = true
  el.scrollTop = el.scrollHeight
  pinning = false
  pinnedOnce = true
  measureRail()
}
function onMsgsScroll() {
  if (pinning || !pinnedOnce) return
  const el = msgsEl.value
  if (!el) return
  stick.value = keepFollowing(true, nearBottom(el.scrollTop, el.scrollHeight, el.clientHeight), stick.value)
  measureRail()
  syncJumpsToChat()
}
function jumpUser(offset: number) {
  const el = msgsEl.value
  if (!el) return
  el.scrollTop = Math.max(0, offset - 8)
  stick.value = nearBottom(el.scrollTop, el.scrollHeight, el.clientHeight)
  measureRail()
}
function jumpBottom() {
  stick.value = true
  pinBottom()
  away.value = false
}
function onTrackDown(e: PointerEvent) {
  const target = e.target
  if (!(target instanceof Element) || target.closest('.ct-tick, .ct-thumb')) return
  const el = msgsEl.value
  const rail = railEl.value
  if (!el || !rail) return
  const y = e.clientY - rail.getBoundingClientRect().top
  const half = (thumb.value?.height ?? 0) / 2
  el.scrollTop = scrollTopForThumb(y - half, el.scrollHeight, el.clientHeight, trackHeight(el))
}
function onThumbDown(e: PointerEvent) {
  const el = msgsEl.value
  const box = thumb.value
  if (!el || !box) return
  e.preventDefault()
  const handle = e.currentTarget
  if (!(handle instanceof HTMLElement)) return
  handle.setPointerCapture(e.pointerId)
  const startY = e.clientY
  const startTop = box.top
  const move = (ev: PointerEvent) => {
    const scroller = msgsEl.value
    if (!scroller) return
    scroller.scrollTop = scrollTopForThumb(startTop + ev.clientY - startY, scroller.scrollHeight, scroller.clientHeight, trackHeight(scroller))
  }
  const up = (ev: PointerEvent) => {
    if (handle.hasPointerCapture(ev.pointerId)) handle.releasePointerCapture(ev.pointerId)
    handle.removeEventListener('pointermove', move)
    handle.removeEventListener('pointerup', up)
    handle.removeEventListener('pointercancel', up)
  }
  handle.addEventListener('pointermove', move)
  handle.addEventListener('pointerup', up)
  handle.addEventListener('pointercancel', up)
}
function observeFollow() {
  followObserver?.disconnect()
  if (msgsBody.value) followObserver?.observe(msgsBody.value)
  if (msgsEl.value) followObserver?.observe(msgsEl.value)
}
if (typeof ResizeObserver !== 'undefined') {
  // 正文按帧才画出来，工具和文件改动也是原地变高。高度变了再贴底，才跟得上输出。
  followObserver = new ResizeObserver(() => {
    pinBottom()
    measureRail()
  })
}
/** 对话里划中的文字用 Ctrl/Cmd+C 写入剪贴板。输入框不在这块里，不拦它们自己的复制。 */
function onCopyKey(e: KeyboardEvent) {
  if (onLayoutKey(e)) return
  if (!(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey || (e.key !== 'c' && e.key !== 'C')) return
  const sel = window.getSelection()
  const text = sel?.toString() ?? ''
  if (!text) return
  const node = sel?.anchorNode
  const el = node instanceof Element ? node : node?.parentElement
  if (!el?.closest('.ct-msgs')) return
  e.preventDefault()
  const write = hasWailsBackend() ? ClipboardSetText(text) : navigator.clipboard.writeText(text)
  void write.catch(() => undefined)
}
let disposePanels: (() => void) | undefined
// 设置里的新对话默认值读到 / 改了：欢迎页没选过时跟着换
watch(
  () => [
    catPanels.settings.defaultAgentKind,
    catPanels.settings.defaultModeByAgent?.cat_build,
    catPanels.settings.defaultModeByAgent?.claude_cli,
    catPanels.settings.defaultModeByAgent?.codex_cli,
    catPanels.settings.defaultsByAgent.cat_build?.model,
    catPanels.settings.defaultsByAgent.cat_build?.effort,
    catPanels.settings.defaultsByAgent.claude_cli?.model,
    catPanels.settings.defaultsByAgent.claude_cli?.effort,
    catPanels.settings.defaultsByAgent.codex_cli?.model,
    catPanels.settings.defaultsByAgent.codex_cli?.effort,
  ],
  () => reapplyNewConvDefaults(),
)
watch(
  () => catPanels.settingsLoaded,
  (ok) => {
    if (!ok || catState.sel !== NEW_CONV) return
    const want = catPanels.settings.defaultAgentKind
    if (CAT_MODES.some((m) => m.id === want && m.enabled)) catState.mode = want
  },
)

watch(
  () => officeCanvas.artifact?.id,
  () => {
    officeCanvasClosed.value = false
  },
)
onMounted(() => {
  window.addEventListener('keydown', onCopyKey)
  observeFollow()
  disposeCat = initCat()
  disposePanels = initCatPanels((id) => catState.projects.find((p) => p.id === id)?.name ?? '', (id) => selectConv(id))
  void officeCanvas.init()
  void refreshCapabilities()
  if (phase.value !== 'loading') return
  // 至少停 1.2 秒；会话列表没回来之前继续停，避免先闪出「新对话」再跳到上次的对话
  const start = Date.now()
  const show = () => {
    timer = setTimeout(() => (phase.value = 'chat'), Math.max(0, 1200 - (Date.now() - start)))
  }
  if (catState.convListReady) show()
  else {
    stopReady = watch(() => catState.convListReady, (ready) => {
      if (!ready) return
      stopReady?.()
      stopReady = undefined
      show()
    })
  }
})
onBeforeUnmount(() => {
  pageAlive = false
  window.removeEventListener('keydown', onCopyKey)
  clearTimeout(timer)
  clearTimeout(reloadTimer)
  cancelAnimationFrame(railAnim)
  stopReady?.()
  disposeCat?.()
  disposePanels?.()
  followObserver?.disconnect()
})

watch(() => catState.sel, () => {
  stick.value = true
  away.value = false
  searchOn.value = false
  pinnedOnce = false
  ticks.value = []
  jumpH.value = 0
  jumpContent.value = 0
  hoverTick.value = -1
  thumb.value = null
  void refreshCapabilities()
})

watch(msgsBody, (el) => {
  pinnedOnce = false
  stick.value = true
  away.value = false
  if (!el) return
  observeFollow()
  nextTick(() => {
    pinBottom()
    measureRail()
  })
})

function goBack() {
  router.push(catReturnPath())
}

function onLater() {
  ElMessage.info(CAT_COPY.later)
}

function onModeClick(m: CatMode) {
  if (!m.enabled) {
    onLater()
    return
  }
  if (catState.mode === m.id) return
  changeWelcomeMode(m.id)
}
</script>

<style scoped>
.ct-c {
  position: relative;
}
.ct-notice {
  position: absolute;
  top: 54px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: calc(100% - 48px);
  padding: 8px 14px;
  border-radius: 10px;
  background: var(--cat-elevated);
  border: 1px solid var(--cat-border);
  box-shadow: var(--cat-shadow-md);
  font-size: 13px;
  color: var(--cat-text);
  white-space: nowrap;
  animation: ct-notice var(--ff-dur-base) var(--ff-ease-out) both;
}
.ct-notice :deep(svg) {
  flex: none;
  color: var(--cat-accent);
}
.ct-notice.ok :deep(svg) {
  color: var(--cat-success);
}
.ct-notice.warn {
  border-color: color-mix(in srgb, var(--cat-warn) 55%, var(--cat-border));
}
.ct-notice.warn :deep(svg) {
  color: var(--cat-warn);
}
/* 提示条存在时浮提示下移到提示条下方，不遮挡「重新选择文件夹」（设计 v0.2 §09d） */
.ct-notice.low {
  top: 90px;
}
@keyframes ct-notice {
  from { opacity: 0; transform: translate(-50%, -4px); }
  to { opacity: 1; transform: translate(-50%, 0); }
}
.ct-missing {
  height: 34px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 16px;
  font-size: 12.5px;
  color: var(--cat-text-2);
  background: color-mix(in srgb, var(--cat-warn) 7%, transparent);
  border-bottom: 1px solid var(--cat-border);
}
.ct-missing :deep(svg) {
  color: var(--cat-warn);
  flex: none;
}
.ct-missing .tx {
  flex: 1;
  min-width: 0;
}
.ct-missing .relink {
  flex: none;
  height: 24px;
  padding: 0 10px;
  border: none;
  border-radius: 6px;
  background: var(--cat-accent);
  color: var(--cat-on-accent);
  font: inherit;
  font-size: 12px;
  transition: background var(--ff-dur-quick), transform var(--ff-dur-quick);
  cursor: pointer;
}
.ct-missing .relink:hover {
  background: var(--cat-accent-hover);
}
.ct-missing .relink:active {
  transform: scale(0.94);
}
.ct-missing .relink:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.crumb-p.miss {
  color: var(--cat-muted);
}
.crumb-p.miss :deep(svg) {
  color: var(--cat-warn);
}
@media (prefers-reduced-motion: reduce) {
  .ct-notice {
    animation: none;
  }
}
.ct-banner {
  display: flex;
  align-items: center;
  gap: 6px;
  width: calc(100% - 96px);
  max-width: calc(100% - 96px);
  margin: 0 auto 10px;
  padding: 8px 12px;
  border-radius: 10px;
  border: 1px solid color-mix(in srgb, var(--cat-warn) 30%, transparent);
  background: var(--cat-warn-soft);
  color: var(--cat-warn-text);
  font-size: 12.5px;
  box-sizing: border-box;
  animation: cat-fade-in var(--ff-dur-base) var(--ff-ease-out) both;
}
.ct-banner :deep(svg) {
  flex: none;
  color: var(--cat-warn-text);
}
.ct-banner .tx {
  flex: 1;
  min-width: 0;
  text-align: left;
}
.ct-banner .recheck {
  flex: none;
  height: 24px;
  padding: 0 10px;
  border: 1px solid color-mix(in srgb, var(--cat-warn) 40%, transparent);
  border-radius: var(--cat-radius-sm);
  background: var(--cat-surface);
  color: var(--cat-warn-text);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: background var(--ff-dur-quick), transform var(--ff-dur-quick);
}
.ct-banner .recheck:hover {
  background: var(--cat-hover);
}
.ct-banner .recheck:active {
  transform: scale(0.94);
}
.ct-banner .recheck:disabled {
  opacity: 0.5;
  cursor: default;
}
.ct-banner .recheck:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
/* G2：未就绪时按 Enter 抖一次；减少动效下改成闪一下边框 */
.ct-banner.kick {
  animation: cat-shake var(--ff-dur-slow) var(--ff-ease-out) both;
}
@media (prefers-reduced-motion: reduce) {
  .ct-banner.kick {
    animation: none;
    border-color: var(--cat-warn);
  }
}
@media (prefers-reduced-motion: reduce) {
  .cat-in,
  .swap,
  .ct-spin {
    animation: none;
  }
  .ct-tick::before {
    transition: none;
  }
}
.cat-page {
  flex: 1;
  min-width: 0;
  display: flex;
  height: 100%;
  background: var(--cat-bg);
}
.ct-load {
  flex: 1;
  display: flex;
  min-width: 0;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
.sk-rail {
  width: 248px;
  flex: none;
  box-sizing: border-box;
  padding: 14px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: var(--cat-rail);
  border-right: 1px solid var(--cat-border);
}
.sk-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 14px 24px 16px;
  gap: 18px;
}
.sk-main .hd {
  align-self: flex-start;
}
.sk-msgs {
  flex: 1;
  width: min(560px, 100%);
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-top: 24px;
}
.sk-msgs.conv {
  flex: none;
  margin: 0 auto;
  padding: 8px 0;
  animation: sk-late var(--ff-dur-base) var(--ff-ease-out) 150ms both;
}
@keyframes sk-late {
  from { opacity: 0; }
  to { opacity: 1; }
}
.sk {
  display: block;
  height: 12px;
  border-radius: var(--cat-radius-sm);
  background: var(--cat-sunken) linear-gradient(90deg, transparent 0%, var(--cat-hover) 50%, transparent 100%) no-repeat;
  background-size: 200% 100%;
  background-position: 150% 0;
  /* 骨架闪光 1.4s 循环，>300ms 才开始 */
  animation: sk-shine 1.4s linear 300ms infinite;
}
html.dark .sk {
  background-color: var(--cat-elevated);
}
.sk.tall { height: 30px; }
.sk.gap { margin-top: 12px; }
.sk.r { align-self: flex-end; height: 32px; }
.sk.comp { width: min(560px, 100%); height: 96px; border-radius: 22px; }
.w30 { width: 30%; }
.w40 { width: 40%; }
.w50 { width: 50%; }
.w60 { width: 60%; }
.w70 { width: 70%; }
.w80 { width: 80%; }
.w90 { width: 90%; }
@keyframes sk-shine {
  from { background-position: 150% 0; }
  to { background-position: -50% 0; }
}
@media (prefers-reduced-motion: reduce) {
  .sk,
  .sk-msgs.conv {
    animation: none;
  }
}
@keyframes ct-spin {
  to { transform: rotate(360deg); }
}
/* A2 进入 Cat 页 / B1 切换对话：200ms ease-out */
.cat-in {
  animation: catIn var(--ff-dur-base) var(--ff-ease-out) both;
}
.swap {
  animation: catIn var(--ff-dur-base) var(--ff-ease-out) both;
}
@keyframes catIn {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: none; }
}
.ct {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  background: var(--cat-bg);
}
.ct-c {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow-x: clip;
  background: var(--cat-bg);
}
.ct-hd {
  height: 44px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  border-bottom: 1px solid var(--cat-border);
  --wails-draggable: drag;
}
.ct-hd h2 {
  font-size: 14px;
  font-weight: 600;
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ct-hd > :deep(svg) {
  color: var(--cat-text-2);
}
.hd-ic {
  flex: none;
  width: 28px;
  height: 28px;
  margin: 0;
  padding: 0;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--ff-text-2);
  cursor: pointer;
  --wails-draggable: no-drag;
}
.hd-ic:hover:not(:disabled) {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}

.ct-hd .sp {
  flex: 1;
}
.crumb-p {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 13px;
  color: var(--cat-text-2);
  white-space: nowrap;
}
.slash {
  color: var(--cat-muted);
}
.main-tag {
  flex: none;
  font-size: 10.5px;
  line-height: 16px;
  padding: 0 5px;
  border-radius: 4px;
  border: 1px solid var(--cat-border);
  color: var(--cat-text-2);
  background: var(--cat-hover);
  font-weight: 500;
}
.ct-stream {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.ct-msgs {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 16px 0;
  overflow-anchor: none;
  user-select: text;
  -webkit-user-select: text;
  scrollbar-width: none;
}
.ct-msgs::-webkit-scrollbar {
  width: 0;
  height: 0;
}
.ct-jumps {
  position: absolute;
  left: 6px;
  top: 50%;
  width: 44px;
  transform: translateY(-50%);
  z-index: 3;
  overflow-x: hidden;
  overflow-y: auto;
  scrollbar-width: none;
  -webkit-mask-image: linear-gradient(to bottom, transparent 0, #000 16px, #000 calc(100% - 16px), transparent 100%);
  mask-image: linear-gradient(to bottom, transparent 0, #000 16px, #000 calc(100% - 16px), transparent 100%);
}
.ct-jumps::-webkit-scrollbar {
  width: 0;
  height: 0;
}
.ct-jump-in {
  position: relative;
  width: 100%;
}
.ct-rail {
  position: absolute;
  right: 2px;
  top: 8px;
  bottom: 8px;
  width: 18px;
  z-index: 3;
}
.ct-tick {
  position: absolute;
  left: 0;
  width: 44px;
  height: 12px;
  margin: 0;
  padding: 0;
  border: none;
  background: transparent;
  cursor: pointer;
}
.ct-tick::before {
  content: '';
  position: absolute;
  left: 0;
  top: 5px;
  width: var(--tick-w, 8px);
  height: 2px;
  border-radius: 999px;
  background: var(--cat-muted);
  transform-origin: left center;
  transition: width 180ms ease, background-color 180ms ease;
}
.ct-tick.near::before {
  background: var(--cat-text-2);
}
.ct-tick.on::before,
.ct-tick:focus-visible::before {
  background: var(--cat-text);
}
.ct-tick:focus-visible {
  outline: none;
}
.ct-thumb {
  position: absolute;
  right: 0;
  width: 5px;
  border-radius: 999px;
  background: var(--cat-muted);
  opacity: 0.7;
  cursor: grab;
  touch-action: none;
}
.ct-thumb:hover,
.ct-thumb:active {
  opacity: 1;
  cursor: grabbing;
}
.ct-jump {
  position: absolute;
  left: 50%;
  bottom: 14px;
  z-index: 4;
  width: 32px;
  height: 32px;
  margin: 0;
  padding: 0;
  display: grid;
  place-items: center;
  transform: translateX(-50%);
  border: 1px solid var(--cat-border);
  border-radius: 50%;
  background: var(--cat-surface);
  color: var(--cat-text-2);
  box-shadow: var(--cat-shadow-md);
  cursor: pointer;
  animation: cat-fade-in var(--ff-dur-base) var(--ff-ease-out) both;
  transition: background var(--ff-dur-quick), color var(--ff-dur-quick);
}
.ct-jump:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.ct-jump:hover {
  color: var(--cat-text);
  background: var(--cat-hover);
}

/* 新对话欢迎页 */
.ct-welcome {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px 24px 48px;
  text-align: center;
  min-height: 0;
  overflow: auto;
}
.wl-hi {
  font-size: 28px;
  font-weight: 700;
  color: var(--cat-text);
  margin: 0 0 18px;
  letter-spacing: -0.02em;
  line-height: 1.3;
}
.wl-modes {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 22px;
  flex-wrap: wrap;
  justify-content: center;
}
.wl-mode {
  height: 34px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 12px;
  border-radius: 17px;
  border: 1px solid transparent;
  background: transparent;
  font: inherit;
  font-size: 13px;
  color: var(--cat-text-2);
  cursor: pointer;
  white-space: nowrap;
  transition: background var(--ff-dur-quick), color var(--ff-dur-quick);
}
.wl-mode > :deep(svg) {
  color: var(--cat-muted);
}
.wl-mode:hover {
  background: var(--cat-hover);
  color: var(--cat-text);
}
.wl-mode.on {
  background: var(--cat-surface);
  border-color: var(--cat-border);
  color: var(--cat-text);
  font-weight: 500;
  box-shadow: var(--cat-shadow-sm);
}
.wl-mode.on > :deep(svg) {
  color: var(--cat-text);
}
.wl-mode.more {
  color: var(--cat-muted);
}
.ct-welcome :deep(.ct-comp) {
  margin: 0;
  text-align: left;
  width: 100%;
  max-width: 100%;
}
.wl-try {
  margin-top: 18px;
  width: 100%;
  text-align: left;
}
.wl-try-hd {
  font-size: 12.5px;
  color: var(--cat-muted);
  margin-bottom: 8px;
  padding: 0 2px;
  grid-column: 1 / -1;
}
/* I1 建议指令：3 张卡片，hover 上浮 2px */
.wl-try {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}
.wl-cmd {
  display: block;
  width: 100%;
  text-align: left;
  padding: 12px;
  margin: 0;
  border: 1px solid var(--cat-border);
  border-radius: var(--cat-radius-lg);
  background: var(--cat-surface);
  box-shadow: var(--cat-shadow-sm);
  font: inherit;
  font-size: 13px;
  line-height: 1.5;
  color: var(--cat-text);
  cursor: pointer;
  transition: transform var(--ff-dur-quick), border-color var(--ff-dur-quick), box-shadow var(--ff-dur-quick);
}
.wl-cmd:hover {
  transform: translateY(-2px);
  border-color: var(--cat-border-strong);
  box-shadow: var(--cat-shadow-md);
}
.wl-cmd:active {
  transform: scale(0.98);
}
.wl-mode:focus-visible,
.wl-cmd:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.wl-mode.disabled,
.wl-mode.disabled:hover {
  color: var(--cat-muted);
  background: transparent;
  border-color: transparent;
  box-shadow: none;
  cursor: not-allowed;
  opacity: 0.55;
  font-weight: 400;
}
.wl-mode.disabled > :deep(svg) {
  color: var(--cat-muted);
}
@media (prefers-reduced-motion: reduce) {
  .ct-spin,
  .cat-in,
  .swap {
    animation: none !important;
  }
}
.ct-r-wrap.canvas {
  width: 400px;
  max-width: 42vw;
}
.ct-r-wrap {
  flex: none;
  display: flex;
  min-width: 0;
  max-width: 1600px;
  overflow: hidden;
  transition: max-width var(--ff-dur-slow) var(--ff-ease-out), opacity var(--ff-dur-base) var(--ff-ease-out);
}
.ct-r-wrap.shut {
  max-width: 0;
  opacity: 0;
  pointer-events: none;
}
.hd-ib {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: var(--cat-radius-sm);
  background: transparent;
  color: var(--cat-text-2);
  cursor: pointer;
  --wails-draggable: no-drag;
  transition: background var(--ff-dur-quick), transform var(--ff-dur-quick);
}
.hd-ib:hover {
  background: var(--cat-hover);
}
.hd-ib:active {
  transform: scale(0.94);
}
.hd-ib:focus-visible {
  outline: none;
  box-shadow: var(--cat-focus);
}
.ct-stream:has(> .ct-hist-empty) {
  position: relative;
}
/* 对话区正中（上下左右），不挡输入框 */
.ct-hist-empty {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 16px;
  pointer-events: none;
  color: var(--cat-muted);
  font-size: 13px;
  text-align: center;
}
</style>
