<template>
  <div class="cat-page cat-root" data-testid="cat-page">
    <!-- 进入时：侧栏 250ms 收起（AppSidebar 的 cat-away），中间先显示约 1.2 秒 loading，再出三栏聊天页 -->
    <div v-if="phase === 'loading'" class="ct-load" role="status" aria-live="polite">
      <div class="in"><i class="ct-spin" aria-hidden="true" /><span>正在进入 Cat…</span></div>
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
            <h2>新对话</h2><span class="sp" /><FIcon name="more" :size="16" />
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
                @click="onModeClick(m)"
              >
                <FIcon :name="m.icon" :size="15" /><span>{{ m.name }}</span>
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
            <FIcon name="refresh" :size="16" /><FIcon name="more" :size="16" />
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
          <div class="ct-stream">
            <div ref="msgsEl" :key="catState.sel" class="ct-msgs swap" @scroll.passive="onMsgsScroll">
              <div ref="msgsBody">
                <CatMessages :blocks="messages" :pending="thinking" />
              </div>
            </div>
            <div v-show="overflowing" ref="railEl" class="ct-rail" data-testid="cat-msg-rail" aria-label="消息位置" @pointerdown="onTrackDown">
              <button
                v-for="t in ticks"
                :key="t.key"
                type="button"
                class="ct-tick"
                :style="{ top: t.top + 'px' }"
                :title="t.label || '用户消息'"
                :aria-label="t.label || '用户消息'"
                data-testid="cat-msg-tick"
                @click.stop="jumpUser(t.offset)"
              />
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

      <CatFilesPanel v-if="!isNew && catPanels.view === 'chat'" :conv-id="catState.sel" :missing="projectMissing" />
    </div>
    <CatProjectDeleteDialog />
    <CatScheduleToast />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import CatSidePanel from '@/components/cat/CatSidePanel.vue'
import CatFilesPanel from '@/components/cat/CatFilesPanel.vue'
import CatComposer from '@/components/cat/CatComposer.vue'
import CatMessages from '@/components/cat/CatMessages.vue'
import CatProjectDeleteDialog from '@/components/cat/CatProjectDeleteDialog.vue'
import CatSettingsView from '@/components/cat/panels/CatSettingsView.vue'
import CatSchedulesView from '@/components/cat/panels/CatSchedulesView.vue'
import CatScheduleToast from '@/components/cat/panels/CatScheduleToast.vue'
import { catPanels, initCatPanels } from './catPanels'
import { reapplyNewConvDefaults, selectConv } from './catState'
import { CAT_PROJECT_COPY as PC } from '@/api/catProjects'
import { ElMessage } from 'element-plus'
import { CAT_COPY } from '@/api/cat'
import { CAT_MODES, CAT_TRY, type CatMode } from '@/api/catMock'
import { catReturnPath } from './catReturn'
import { nearBottom, keepFollowing } from './catFollow'
import { railThumb, railTickTops, scrollTopForThumb } from './catRail'
import { awaitingTurnOutput } from './catPending'
import { catChecking, catNotReady, catState, currentProject, findConv, projectMissing, relocateProject, initCat, messagesOf, NEW_CONV, refreshCapabilities, recheckCatStatus, sendMessage, stopTurn, tipLater } from './catState'
import { hasWailsBackend } from '@/services/wails'
import { ClipboardSetText } from '../../../wailsjs/runtime/runtime'

/**
 * Cat 聊天页（设计 v0.4 / 契约 v0.30）。Wails 里接真实 CatService：流式回复、停止生成、组件未就绪横条（无下载按钮）。
 * 纯浏览器走查仍是布局壳 + typed mock。
 * 浏览器走查可加 ?cat=chat 跳过 1.2 秒 loading、?cat_conv=new|c21|d1… 直接打开某条对话（仅纯浏览器，Wails 里无效）。
 */
defineOptions({ name: 'CatPage' })

const router = useRouter()
const preview = typeof window !== 'undefined' && !(window as unknown as { go?: unknown }).go
const q = new URLSearchParams(location.search)
const phase = ref<'loading' | 'chat'>(preview && q.get('cat') === 'chat' ? 'chat' : 'loading')
if (preview && q.get('cat_conv')) catState.sel = q.get('cat_conv') as string

const notReadyText = computed(() => catState.status.error?.message || CAT_COPY.notReady)
const isNew = computed(() => catState.sel === NEW_CONV)
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
async function onSend(payload: { text: string; files: { name: string; path: string }[] }, c: InstanceType<typeof CatComposer> | undefined) {
  stick.value = true
  const back = await sendMessage(payload.text, payload.files)
  if (back) {
    c?.restoreDraft(back.text)
    c?.restoreFiles(back.files)
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
const msgsEl = ref<HTMLElement>()
const msgsBody = ref<HTMLElement>()
const railEl = ref<HTMLElement>()
/** 人在看最新一条时，回复变高就跟着往下；往上翻过之后停住 */
const stick = ref(true)
/** 人把消息往上翻开之后才出现「回到底部」 */
const away = ref(false)
const overflowing = ref(false)
const ticks = ref<{ key: string; top: number; offset: number; label: string }[]>([])
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
  const tops = railTickTops(nodes.length, track)
  const next = nodes.map((n, i) => {
    const offset = n.getBoundingClientRect().top - el.getBoundingClientRect().top + el.scrollTop
    const label = (n.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 80)
    return {
      key: n.dataset.catUser || String(i),
      top: tops[i] ?? 0,
      offset,
      label,
    }
  })
  ticks.value = next
  thumb.value = railThumb(el.scrollTop, sh, ch, track)
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
  () => [catPanels.settings.defaultModel, catPanels.settings.defaultEffort, catPanels.settings.defaultApprovalMode],
  () => reapplyNewConvDefaults(),
)
onMounted(() => {
  window.addEventListener('keydown', onCopyKey)
  observeFollow()
  disposeCat = initCat()
  disposePanels = initCatPanels((id) => catState.projects.find((p) => p.id === id)?.name ?? '', (id) => selectConv(id))
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
  window.removeEventListener('keydown', onCopyKey)
  clearTimeout(timer)
  stopReady?.()
  disposeCat?.()
  disposePanels?.()
  followObserver?.disconnect()
})

watch(() => catState.sel, () => {
  stick.value = true
  away.value = false
  pinnedOnce = false
  ticks.value = []
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
  catState.mode = m.id
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
  width: 560px;
  max-width: calc(100% - 32px);
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
  display: grid;
  place-items: center;
}
.ct-load .in {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  color: var(--cat-text-2);
  font-size: 13px;
}
.ct-spin {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 3px solid var(--cat-border);
  border-top-color: var(--cat-accent);
  animation: ct-spin 0.9s linear infinite; /* 循环动画，不属于过渡时长档 */
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
  padding: 16px 22px 16px 0;
  overflow-anchor: none;
  user-select: text;
  -webkit-user-select: text;
  scrollbar-width: none;
}
.ct-msgs::-webkit-scrollbar {
  width: 0;
  height: 0;
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
  width: 12px;
  height: 8px;
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
  top: 3px;
  width: 8px;
  height: 2px;
  border-radius: 1px;
  background: var(--cat-muted);
}
.ct-tick:hover::before,
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
  width: min(640px, 100%);
  max-width: 100%;
}
.wl-try {
  margin-top: 18px;
  width: min(640px, 100%);
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
</style>
