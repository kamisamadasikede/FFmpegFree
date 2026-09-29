<template>
  <section class="pvp" aria-labelledby="pv-h">
    <div class="phead">
      <h2 id="pv-h">{{ PREVIEW_PANEL_TITLE }}</h2>
      <span v-if="cur" class="src" :title="cur.url"><FIcon :name="cur.kind === 'screen' ? 'monitor' : 'film'" :size="14" /><span class="hd">{{ head }}</span><span class="tl">{{ tail }}</span></span>
      <span class="note">{{ PREVIEW_RATE_NOTE }}</span>
    </div>
    <div class="stage"><PreviewStage :state="state" :data="snap.data" :alt="alt" kind="push" @retry="poller.retry()" /></div>
  </section>
</template>

<script setup lang="ts">
// 推流页左列上方的预览面板（设计稿 来源选择与预览 v0.1 §3.1、§6）：只显示“当前预览行”这一路（默认第一个进行中的会话，可在会话行点“查看预览”切换）。
// 取帧：usePreviewPoller（约 500ms，请求不重叠，页面不可见 / 会话结束 / 预览关闭都停；首帧 10 秒无画面或连续 5 次出错转失败）。
import { computed, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import PreviewStage, { type StageState } from './PreviewStage.vue'
import { usePreviewPoller } from '@/composables/usePreviewPoller'
import { useLiveSessionsStore, type LiveRow } from '@/stores/liveSessions'
import { PREVIEW_PANEL_TITLE, PREVIEW_RATE_NOTE, previewAlt } from '@/errors/livePreviewMessages'

const store = useLiveSessionsStore()
const { snap, poller } = usePreviewPoller()

const live = (r: LiveRow) => r.status === 'run' || r.status === 'stp'
/** 当前预览行：用户选的 → 否则第一个进行中的；当前行已结束时停留在它（保留末帧），直到有新的进行中会话 */
const cur = computed<LiveRow | undefined>(() => {
  const chosen = store.rows.find((r) => r.id === store.previewId)
  if (chosen && (live(chosen) || !store.rows.some(live))) return chosen
  return store.rows.find(live) ?? chosen ?? store.rows[0]
})
const wantsPreview = computed(() => !!cur.value && live(cur.value) && cur.value.preview !== false)
const state = computed<StageState>(() => {
  const r = cur.value
  if (!r) return 'empty'
  if (!live(r)) return 'ended'
  if (r.preview === false) return 'off'
  const p = snap.value.phase
  return p === 'ok' || p === 'failed' || p === 'ended' ? p : 'loading'
})

// 中间省略：尾部固定保留最后 14 个字符（地址末尾一般是流名），头部可收缩加省略号
const TAIL = 14
const head = computed(() => (cur.value && cur.value.url.length > TAIL * 2 ? cur.value.url.slice(0, -TAIL) : cur.value?.url ?? ''))
const tail = computed(() => (cur.value && cur.value.url.length > TAIL * 2 ? cur.value.url.slice(-TAIL) : ''))
const alt = computed(() => previewAlt('push', cur.value?.url ?? ''))

// 当前预览会话 / 是否需要取帧 变化时启停
watch(
  () => [cur.value?.id, wantsPreview.value] as const,
  ([id, want], old) => {
    if (id && want) {
      if (!old || old[0] !== id || !old[1]) poller.start(id)
    } else if (id && cur.value && !live(cur.value)) {
      if (poller.sessionId === id) poller.end() // 会话结束：保留末帧，不再请求
      else poller.reset() // 换到一个别的已结束会话：没有它的末帧
    } else {
      poller.reset()
    }
  },
  { immediate: true },
)
// 进行中 → 终态（同一个 id）
watch(
  () => cur.value && live(cur.value),
  (now, was) => {
    if (was && !now) poller.end()
  },
)
</script>

<style scoped>
.pvp {
  flex: none;
  width: 100%;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: 10px;
  overflow: hidden;
  margin-bottom: 4px;
}
.phead {
  height: 48px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  border-bottom: 1px solid var(--ff-border);
  min-width: 0;
}
h2 {
  margin: 0;
  font-size: var(--ff-fs-md);
  font-weight: 600;
  line-height: 21px;
  flex: none;
}
.src {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-family: var(--ff-font-mono);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  flex: 1;
}
.src svg {
  flex: none;
}
.hd {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tl {
  flex: none;
  white-space: nowrap;
}
.note {
  margin-left: auto;
  flex: none;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  white-space: nowrap;
}
.stage {
  position: relative;
  height: 216px;
}
@media (max-width: 1199px) {
  .stage {
    height: 144px;
  }
}
</style>
