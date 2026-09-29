<template>
  <div class="csp">
    <div class="bar">
      <LiveButton variant="text" sm icon="refresh" :disabled="state === 'loading'" @click="emit('refresh')">{{ LIVE_SOURCE_REFRESH }}</LiveButton>
    </div>
    <div v-if="state === 'loading'" class="st" role="status" aria-live="polite"><i class="spin" aria-hidden="true" />{{ LIVE_SOURCE_LOADING }}</div>
    <div v-else-if="state === 'failed'" class="st bad" role="alert">
      <FIcon name="warn" :size="14" /><span>{{ LIVE_SOURCE_FAILED }}</span>
      <LiveButton variant="text" sm @click="emit('refresh')">{{ LIVE_SOURCE_RETRY }}</LiveButton>
    </div>
    <div v-else-if="state === 'empty'" class="st"><FIcon name="info" :size="14" />{{ LIVE_SOURCE_EMPTY }}</div>
    <div v-else class="lst" role="radiogroup" :aria-labelledby="labelId" :aria-invalid="invalid || undefined" :aria-describedby="invalid ? errorId : undefined">
      <template v-for="g in groups" :key="g.kind">
        <div :id="`${uid}-${g.kind}`" class="gh">{{ g.title }}</div>
        <div class="grp" role="group" :aria-labelledby="`${uid}-${g.kind}`">
          <button
            v-for="s in g.items"
            :key="s.id"
            type="button"
            class="so"
            :class="{ on: modelValue === s.id }"
            role="radio"
            :aria-checked="modelValue === s.id"
            :title="tip(s)"
            @click="emit('update:modelValue', s.id)"
          >
            <i class="rd" /><FIcon :name="s.kind === 'window' ? 'window' : 'monitor'" :size="14" /><span class="nm">{{ s.title }}</span><em v-if="s.width > 0 && s.height > 0">{{ s.width }}×{{ s.height }}</em>
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
// 直播 v1.1 采集来源选择器（屏幕 / 应用窗口两个分组）。设计稿未出，按 v0.2 风格与 --ff-* 令牌先做。
// 名称过长省略，完整名称 + 分辨率放 title（悬停可见）；窗口标题可能含隐私，只在本机界面显示，不写日志、不进错误 detail。
// 平台不返回 window（macOS / Linux）时没有“应用窗口”分组，也不留空标题。三态（加载中 / 空 / 失败）由父组件给 state。
import { computed, inject, useId } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import LiveButton from './LiveButton.vue'
import type { CaptureSource } from '@/api/live'
import {
  LIVE_SOURCE_EMPTY, LIVE_SOURCE_FAILED, LIVE_SOURCE_GROUP_SCREEN, LIVE_SOURCE_GROUP_WINDOW, LIVE_SOURCE_LOADING, LIVE_SOURCE_REFRESH, LIVE_SOURCE_RETRY,
} from '@/errors/errorMessages'

const props = defineProps<{ sources: CaptureSource[]; modelValue: string; state: 'loading' | 'ready' | 'empty' | 'failed'; invalid?: boolean; errorId?: string }>()
const emit = defineEmits<{ 'update:modelValue': [id: string]; refresh: [] }>()
const uid = `ff-src-${useId()}`
const labelId = inject<string | undefined>('ff-field-label-id', undefined)
const groups = computed(() =>
  [
    { kind: 'screen', title: LIVE_SOURCE_GROUP_SCREEN, items: props.sources.filter((s) => s.kind === 'screen') },
    { kind: 'window', title: LIVE_SOURCE_GROUP_WINDOW, items: props.sources.filter((s) => s.kind === 'window') },
  ].filter((g) => g.items.length > 0),
)
const tip = (s: CaptureSource) => (s.width > 0 && s.height > 0 ? `${s.title}（${s.width}×${s.height}）` : s.title)
</script>

<style scoped>
.bar {
  display: flex;
  justify-content: flex-end;
  margin: -22px 0 4px;
  height: 18px;
  align-items: center;
}
.lst {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 232px;
  overflow-y: auto;
}
.grp {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.gh {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-3);
  line-height: 18px;
  margin-top: 4px;
}
.gh:first-child {
  margin-top: 0;
}
.so {
  height: 32px;
  flex: none;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  font: inherit;
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  background: var(--ff-bg-surface);
  cursor: pointer;
  text-align: left;
  min-width: 0;
}
.so svg {
  color: var(--ff-text-2);
  flex: none;
}
.nm {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.so em {
  flex: none;
  font-style: normal;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.so .rd {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1.5px solid var(--ff-text-2);
  flex: none;
  position: relative;
}
.so:hover {
  background: var(--ff-bg-hover);
}
.so.on {
  border-color: var(--ff-primary);
  background: var(--ff-primary-soft);
}
.so.on .rd {
  border-color: var(--ff-primary);
}
.so.on .rd::after {
  content: '';
  position: absolute;
  inset: 2px;
  border-radius: 50%;
  background: var(--ff-primary);
}
.so:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.st {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  padding: 0 10px;
  border: 1px dashed var(--ff-border);
  border-radius: 6px;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.st.bad {
  border-style: solid;
  color: var(--ff-danger-text);
  border-color: color-mix(in srgb, var(--ff-danger) 40%, var(--ff-border));
}
.st.bad span {
  flex: 1;
  min-width: 0;
}
.spin {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid var(--ff-border);
  border-top-color: var(--ff-primary);
  animation: cspin 1s linear infinite;
  flex: none;
}
@keyframes cspin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .spin {
    animation: none;
  }
}
</style>
