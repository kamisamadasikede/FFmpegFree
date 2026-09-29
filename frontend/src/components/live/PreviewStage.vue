<template>
  <div class="pv-stage" :data-state="state">
    <img v-if="(state === 'ok' || state === 'ended') && data" class="pv-img" :class="{ ended: state === 'ended' }" :src="previewDataUrl(data)" :alt="alt" draggable="false" />
    <div v-if="state === 'ended' && data" class="pv-pill"><FIcon name="stop" :size="12" />{{ PREVIEW_ENDED_TITLE }}</div>
    <div v-else-if="state !== 'ok'" class="pv-msg" :class="{ warn: state === 'failed' }">
      <i v-if="state === 'loading'" class="pv-spin" aria-hidden="true" />
      <span v-else class="pv-ic" :class="{ warn: state === 'failed' }"><FIcon :name="icon" :size="state === 'failed' ? 20 : 16" /></span>
      <b>{{ title }}</b>
      <span v-if="hint" class="h">{{ hint }}</span>
      <button v-if="state === 'failed'" type="button" class="pv-retry" @click="emit('retry')">{{ PREVIEW_RETRY }}</button>
    </div>
    <!-- 读屏：只在状态变化时播报，不随换帧播报 -->
    <div class="sr-only" role="status" aria-live="polite">{{ live }}</div>
  </div>
</template>

<script setup lang="ts">
// 预览舞台（设计稿 来源选择与预览 v0.1 §6）：深色底 #0B0C0E，画面 16:9 居中，状态：空 / 加载中 / 正常 / 失败 / 未开启 / 会话已结束（末帧灰化）。
// 直播页（推流）的预览面板和拉流页的播放器舞台共用。**不显示时间戳，也没有“示意画面”角标**（设计稿里那是占位标）。
// 用 data:image/jpeg;base64, 直接显示，不建 blob，不累积对象 URL；alt 只随会话变化，不随帧变化。
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { previewDataUrl } from '@/api/livePreviewPoller'
import {
  PREVIEW_EMPTY_HINT, PREVIEW_EMPTY_TITLE, PREVIEW_ENDED_NOFRAME_HINT, PREVIEW_ENDED_TITLE, PREVIEW_FAILED_HINT_PULL, PREVIEW_FAILED_HINT_PUSH,
  PREVIEW_FAILED_TITLE, PREVIEW_LIVE_ENDED, PREVIEW_LIVE_FAILED_PULL, PREVIEW_LIVE_FAILED_PUSH, PREVIEW_LIVE_LOADING, PREVIEW_LIVE_OFF, PREVIEW_LIVE_OK,
  PREVIEW_LOADING_HINT_PULL, PREVIEW_LOADING_HINT_PUSH, PREVIEW_LOADING_TITLE, PREVIEW_OFF_HINT_PULL, PREVIEW_OFF_HINT_PUSH, PREVIEW_OFF_TITLE, PREVIEW_RETRY, PREVIEW_UNSUPPORTED_HINT, PREVIEW_UNSUPPORTED_TITLE,
} from '@/errors/livePreviewMessages'

export type StageState = 'empty' | 'loading' | 'ok' | 'failed' | 'off' | 'ended' | 'unsupported'
const props = defineProps<{ state: StageState; data?: string; alt?: string; kind: 'push' | 'pull' }>()
const emit = defineEmits<{ retry: [] }>()
const pull = computed(() => props.kind === 'pull')
const icon = computed(() => (props.state === 'failed' ? 'warn' : props.state === 'ended' ? 'stop' : props.state === 'off' || props.state === 'unsupported' ? 'block' : 'eye'))
const title = computed(
  () =>
    ({
      empty: PREVIEW_EMPTY_TITLE, loading: PREVIEW_LOADING_TITLE, failed: PREVIEW_FAILED_TITLE, off: PREVIEW_OFF_TITLE, ended: PREVIEW_ENDED_TITLE,
      unsupported: PREVIEW_UNSUPPORTED_TITLE, ok: '',
    })[props.state],
)
const hint = computed(
  () =>
    ({
      empty: PREVIEW_EMPTY_HINT, loading: pull.value ? PREVIEW_LOADING_HINT_PULL : PREVIEW_LOADING_HINT_PUSH, failed: pull.value ? PREVIEW_FAILED_HINT_PULL : PREVIEW_FAILED_HINT_PUSH, off: pull.value ? PREVIEW_OFF_HINT_PULL : PREVIEW_OFF_HINT_PUSH,
      ended: PREVIEW_ENDED_NOFRAME_HINT, unsupported: PREVIEW_UNSUPPORTED_HINT, ok: '',
    })[props.state],
)
const live = computed(
  () =>
    ({
      empty: '', loading: PREVIEW_LIVE_LOADING, ok: PREVIEW_LIVE_OK, failed: pull.value ? PREVIEW_LIVE_FAILED_PULL : PREVIEW_LIVE_FAILED_PUSH, off: PREVIEW_LIVE_OFF,
      ended: PREVIEW_LIVE_ENDED, unsupported: '',
    })[props.state],
)
void PREVIEW_RETRY
</script>

<style scoped>
.pv-stage {
  position: absolute;
  inset: 0;
  background: #0b0c0e;
  display: grid;
  place-items: center;
  overflow: hidden;
  color: #e5e7eb;
}
.pv-img {
  width: 100%;
  height: 100%;
  object-fit: contain; /* 按画面原比例放进舞台（推流源可能是 4:3 / 16:9 / 屏幕比例） */
  display: block;
}
.pv-img.ended {
  filter: grayscale(1);
  opacity: 0.4;
}
.pv-pill {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  height: 28px;
  padding: 0 12px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(0, 0, 0, 0.6);
  color: #fff;
  font-size: var(--ff-fs-sm);
  font-weight: 500;
}
.pv-msg {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  text-align: center;
  padding: 0 16px;
  max-width: 100%;
}
.pv-msg b {
  font-size: var(--ff-fs-md);
  font-weight: 600;
  color: #e5e7eb;
}
.pv-msg .h {
  font-size: var(--ff-fs-xs);
  color: #9ca3af;
}
.pv-ic {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.08);
  display: grid;
  place-items: center;
  color: #9ca3af;
  margin-bottom: 4px;
}
.pv-ic.warn {
  color: #f59e0b;
  background: rgba(245, 158, 11, 0.14);
}
.pv-spin {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 2px solid rgba(255, 255, 255, 0.2);
  border-top-color: #fff;
  animation: pvspin 1s linear infinite;
  margin-bottom: 4px;
}
@keyframes pvspin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .pv-spin {
    animation: none;
  }
}
.pv-retry {
  margin-top: 8px;
  height: 24px;
  padding: 0 12px;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.28);
  background: transparent;
  color: #fff;
  font: inherit;
  font-size: var(--ff-fs-xs);
  cursor: pointer;
}
.pv-retry:hover {
  background: rgba(255, 255, 255, 0.1);
}
.pv-retry:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
