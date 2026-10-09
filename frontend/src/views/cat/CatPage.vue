<template>
  <div class="cat-page" data-testid="cat-page">
    <!-- 进入时：侧栏 250ms 收起（AppSidebar 的 cat-away），中间先显示约 1.2 秒 loading，再出三栏聊天页 -->
    <div v-if="phase === 'loading'" class="ct-load" role="status" aria-live="polite">
      <div class="in"><i class="ct-spin" aria-hidden="true" /><span>正在进入 Cat…</span></div>
    </div>
    <div v-else class="ct cat-in" :class="{ 'no-right': isNew }">
      <CatSidePanel @back="goBack" />

      <section class="ct-c" aria-label="对话">
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
            <div v-if="catNotReady" class="ct-banner" role="status" data-testid="cat-not-ready">
              <FIcon name="info" :size="14" />{{ CAT_COPY.notReady }}
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
              @send="sendMessage"
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
          <div ref="msgsEl" :key="catState.sel" class="ct-msgs swap">
            <CatMessages :blocks="messages" :pending="thinking" />
          </div>
          <div v-if="catNotReady" class="ct-banner in-conv" role="status" data-testid="cat-not-ready">
            <FIcon name="info" :size="14" />{{ CAT_COPY.notReady }}
          </div>
          <CatComposer
            :ctx-name="current.project?.name ?? 'Cat'"
            :ctx-branch="current.project?.branch ?? 'main'"
            :running="!!turn"
            :stopping="turn?.status === 'stopping'"
            :not-ready="catNotReady"
            :checking="catChecking"
            :missing="projectMissing"
            @send="sendMessage"
            @stop="stopTurn()"
          />
        </template>
      </section>

      <CatFilesPanel v-if="!isNew" :conv-id="catState.sel" :missing="projectMissing" />
    </div>
    <CatProjectDeleteDialog />
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
import { CAT_PROJECT_COPY as PC } from '@/api/catProjects'
import { ElMessage } from 'element-plus'
import { CAT_COPY } from '@/api/cat'
import { CAT_MODES, CAT_TRY, type CatMode } from '@/api/catMock'
import { catReturnPath } from './catReturn'
import { catChecking, catNotReady, catState, currentProject, findConv, projectMissing, relocateProject, initCat, messagesOf, NEW_CONV, refreshCapabilities, sendMessage, stopTurn, tipLater } from './catState'

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

const isNew = computed(() => catState.sel === NEW_CONV)
const current = computed(() => (isNew.value ? null : findConv(catState.sel)))
const messages = computed(() => (isNew.value ? [] : messagesOf(catState.sel)))
const composer = ref<InstanceType<typeof CatComposer>>()
/** 当前会话进行中的一轮 */
const turn = computed(() => (isNew.value ? undefined : catState.turns[catState.sel]))
/** 还没收到第一段文字时显示「正在思考…」 */
const thinking = computed(() => !!turn.value && !turn.value.assistantId && turn.value.status === 'running')
/** 流式时文字长度变化也要滚到底 */
const tailLen = computed(() => {
  const last = messages.value[messages.value.length - 1]
  return last && last.kind === 'a' ? last.text.length : 0
})
const msgsEl = ref<HTMLElement>()

// 选中的对话不存在（例如本地对话被清掉）时回到欢迎页
watch(current, (c) => {
  if (!isNew.value && !c) catState.sel = NEW_CONV
}, { immediate: true })

let timer: ReturnType<typeof setTimeout> | undefined
let disposeCat: (() => void) | undefined
onMounted(() => {
  if (phase.value === 'loading') timer = setTimeout(() => (phase.value = 'chat'), 1200)
  disposeCat = initCat()
  void refreshCapabilities()
})
onBeforeUnmount(() => {
  clearTimeout(timer)
  disposeCat?.()
})

watch(() => catState.sel, () => { void refreshCapabilities() })

function scrollToEnd() {
  nextTick(() => {
    const el = msgsEl.value
    if (el) el.scrollTop = el.scrollHeight
  })
}
watch(() => [catState.sel, messages.value.length, thinking.value, tailLen.value, phase.value], scrollToEnd)

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
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.1);
  font-size: 13px;
  color: var(--ff-text-1);
  white-space: nowrap;
  animation: ct-notice 180ms ease both;
}
html.dark .ct-notice {
  background: var(--ff-bg-elevated);
}
.ct-notice :deep(svg) {
  flex: none;
  color: var(--ff-primary);
}
.ct-notice.ok :deep(svg) {
  color: var(--ff-success);
}
.ct-notice.warn {
  border-color: color-mix(in srgb, var(--ff-warning) 55%, var(--ff-border));
}
.ct-notice.warn :deep(svg) {
  color: var(--ff-warning);
}
/* 提示条存在时浮提示下移到提示条下方，不遮挡「重新选择文件夹」（设计 v0.2 §09d） */
.ct-notice.low {
  top: 90px;
}
@keyframes ct-notice {
  from { opacity: 0; transform: translate(-50%, -6px); }
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
  color: var(--ff-text-2);
  background: color-mix(in srgb, var(--ff-warning) 7%, transparent);
  border-bottom: 1px solid var(--ff-border);
}
.ct-missing :deep(svg) {
  color: var(--ff-warning);
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
  background: var(--ff-primary);
  color: #fff;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.ct-missing .relink:hover {
  filter: brightness(1.06);
}
.ct-missing .relink:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.crumb-p.miss {
  color: var(--ff-text-3);
}
.crumb-p.miss :deep(svg) {
  color: var(--ff-warning);
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
  background: var(--ff-bg-hover);
  color: var(--ff-text-2);
  font-size: 12.5px;
}
.ct-banner :deep(svg) {
  flex: none;
  color: var(--ff-text-3);
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
  background: var(--ff-bg-app);
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
  color: var(--ff-text-2);
  font-size: 13px;
}
.ct-spin {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 3px solid var(--ff-border);
  border-top-color: var(--ff-primary);
  animation: ct-spin 0.9s linear infinite;
}
@keyframes ct-spin {
  to { transform: rotate(360deg); }
}
.cat-in {
  animation: catIn 250ms ease both;
}
.swap {
  animation: catIn 200ms ease both;
}
@keyframes catIn {
  from { opacity: 0; transform: translateY(6px); }
  to { opacity: 1; transform: none; }
}
.ct {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  background: var(--ff-bg-app);
}
.ct-c {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--ff-bg-surface);
}
.ct-hd {
  height: 44px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  border-bottom: 1px solid var(--ff-border);
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
  color: var(--ff-text-2);
}
.ct-hd .sp {
  flex: 1;
}
.crumb-p {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 13px;
  color: var(--ff-text-2);
  white-space: nowrap;
}
.slash {
  color: var(--ff-text-3);
}
.main-tag {
  flex: none;
  font-size: 10.5px;
  line-height: 16px;
  padding: 0 5px;
  border-radius: 4px;
  border: 1px solid var(--ff-border);
  color: var(--ff-text-2);
  background: var(--ff-bg-hover);
  font-weight: 500;
}
.ct-msgs {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 16px 0;
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
  color: var(--ff-text-1);
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
  color: var(--ff-text-2);
  cursor: pointer;
  white-space: nowrap;
}
.wl-mode > :deep(svg) {
  color: var(--ff-text-3);
}
.wl-mode:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.wl-mode.on {
  background: var(--ff-bg-surface);
  border-color: var(--ff-border);
  color: var(--ff-text-1);
  font-weight: 500;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
}
.wl-mode.on > :deep(svg) {
  color: var(--ff-text-1);
}
.wl-mode.more {
  color: var(--ff-text-3);
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
  color: var(--ff-text-3);
  margin-bottom: 8px;
  padding: 0 2px;
}
.wl-cmd {
  display: block;
  width: 100%;
  text-align: left;
  padding: 10px 12px;
  margin: 0 0 4px;
  border: none;
  border-radius: 8px;
  background: transparent;
  font: inherit;
  font-size: 13.5px;
  line-height: 1.5;
  color: var(--ff-text-1);
  cursor: pointer;
}
.wl-cmd:hover {
  background: var(--ff-bg-hover);
}
.wl-mode:focus-visible,
.wl-cmd:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
}
.wl-mode.disabled,
.wl-mode.disabled:hover {
  color: var(--ff-text-3);
  background: transparent;
  border-color: transparent;
  box-shadow: none;
  cursor: not-allowed;
  opacity: 0.55;
  font-weight: 400;
}
.wl-mode.disabled > :deep(svg) {
  color: var(--ff-text-3);
}
@media (prefers-reduced-motion: reduce) {
  .ct-spin,
  .cat-in,
  .swap {
    animation: none !important;
  }
}
</style>
