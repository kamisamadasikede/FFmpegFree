<template>
  <section class="ed-panel ed-bin" aria-label="素材库" :class="{ over: dropOver }">
    <div class="ed-ph">
      <h2>素材库</h2>
      <span class="cnt" :aria-label="`素材 ${ed.sources.value.length} 个，最多 ${MAX_SOURCES} 个`">{{ ed.sources.value.length }}/{{ MAX_SOURCES }}</span>
      <span class="sp"></span>
      <button
        type="button"
        class="ed-btn sm"
        :aria-disabled="importOff ? 'true' : undefined"
        :data-tip="importOff" :class="{ 'tp-r': true }"
        @click="onImport"
      >
        <FIcon name="plus" :size="13" />导入
      </button>
    </div>

    <div v-if="!ed.sources.value.length" class="ed-drop" :class="{ over: dropOver }">
      <div class="ic"><FIcon name="upload" :size="20" /></div>
      <b>拖入音视频文件</b>
      <span>或点击上方“导入”选择文件</span>
    </div>
    <template v-else>
      <div class="ed-seg" role="group" aria-label="素材类型">
        <button v-for="g in GROUPS" :key="g.key" type="button" :class="{ on: group === g.key }" :aria-pressed="group === g.key" @click="group = g.key">{{ g.label }}</button>
      </div>
      <div class="ed-list" role="list">
        <template v-for="s in shown" :key="s.path">
          <div v-if="s.state === 'fail'" class="ed-row fail" role="alert">
            <FIcon name="warn" :size="16" />
            <div class="ed-rm">
              <span class="t">{{ PROBE_ERROR_TITLE }}</span>
              <b :title="s.path">{{ s.name }}</b>
              <span class="d">{{ probeErrorText(s.err?.code ?? '', s.err?.message) }}</span>
              <span class="a">
                <button type="button" @click="ed.retryProbe(s.path)">重试</button>
                <button type="button" @click="ed.removeSourceRequest(s.path)">从素材库移除</button>
              </span>
            </div>
          </div>
          <div v-else class="ed-row" :class="{ sel: ed.binSel.value === s.path, 'drag-src': dragging === s.path }" role="listitem">
            <button
              type="button"
              class="sel-hit"
              :aria-pressed="ed.binSel.value === s.path"
              :title="s.path"
              @pointerdown="onDown($event, s)"
              @click="onPick(s)"
              @keydown.delete.prevent="ed.removeSourceRequest(s.path)"
            >
              <span class="ed-thumb" :class="{ a: !s.hasVideo && s.state === 'ok' }">
                <img v-if="s.thumb" :src="s.thumb" alt="" />
                <span v-else-if="s.state === 'probing'" class="ed-probing" />
                <FIcon v-else-if="!s.hasVideo" name="music" :size="16" />
                <span v-else class="ed-thumb-ph" :style="{ background: 'var(--ff-smp-' + (hash(s.name) % 4 === 0 ? 4 : hash(s.name) % 4 === 1 ? 1 : hash(s.name) % 4 === 2 ? 2 : 3) + ')' }" />
              </span>
              <span class="ed-rm">
                <b>{{ s.name }}</b>
                <small>{{ infoOf(s) }}</small>
              </span>
            </button>
            <button
              type="button"
              class="ed-add tp-r"
              :aria-label="addOff(s) ? '添加到时间线' : '添加到时间线（播放头位置）'"
              :aria-disabled="addOff(s) ? 'true' : undefined"
              :data-tip="addOff(s) || undefined"
              :title="addOff(s) ? undefined : '添加到时间线'"
              @click="onAdd(s)"
            >
              <FIcon name="plus" :size="16" />
            </button>
          </div>
        </template>
        <div v-if="!shown.length" class="ed-note-inline">{{ group === 'video' ? '还没有视频素材' : '还没有音频素材' }}</div>
      </div>
    </template>
    <div class="ed-binfoot">{{ LIMIT_TEXT }}</div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { onFilesDropped } from '@/api/fileDrop'
import { PROBE_ERROR_TITLE, probeErrorText } from '@/errors/errorMessages'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { channelText } from '@/utils/mediaText'
import { formatShortClock } from '@/utils/format'
import { LIMIT_TEXT, MAX_SOURCES, TEXT, resolutionTier } from '@/utils/editLogic'
import { useEditor, type SourceItem } from './editor'

const ed = useEditor()
const ffmpeg = useFFmpegStore()
const GROUPS = [
  { key: 'all', label: '全部' },
  { key: 'video', label: '视频' },
  { key: 'audio', label: '音频' },
] as const
const group = ref<'all' | 'video' | 'audio'>('all')
const dropOver = ref(false)
const dragging = ref('')

const shown = computed(() =>
  ed.sources.value.filter((s) => group.value === 'all' || s.state !== 'ok' || (group.value === 'video' ? s.hasVideo : !s.hasVideo)),
)
const importOff = computed(() => (ffmpeg.featuresBlocked ? TEXT.ffmpegTip : ed.sourcesFull.value ? TEXT.sourceLimitToast : undefined))
const hash = (t: string) => [...t].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 0)

function infoOf(s: SourceItem): string {
  if (s.state === 'probing') return '正在读取文件信息…'
  const clock = formatShortClock(s.duration)
  const rest = s.hasVideo ? resolutionTier(s.height) : channelText(s.channels)
  return [clock, rest].filter(Boolean).join(' · ')
}
function addOff(s: SourceItem): string {
  if (ffmpeg.featuresBlocked) return TEXT.ffmpegTip
  if (s.state !== 'ok') return ''
  return ed.clipsFull.value ? TEXT.addFull : ''
}
function onImport() {
  if (importOff.value) return ed.say(importOff.value)
  ed.importFiles()
}
function onAdd(s: SourceItem) {
  if (addOff(s) || s.state !== 'ok') return
  ed.addToTimeline(s.path)
}
function onPick(s: SourceItem) {
  if (suppressClick) {
    suppressClick = false
    return
  }
  if (s.state === 'ok') ed.previewSourceRow(s.path)
}

// 素材行 → 时间线的拖动：按下后移动超过 4px 才算拖动，松手时由时间线判断落点
let suppressClick = false
function onDown(e: PointerEvent, s: SourceItem) {
  if (e.button !== 0 || s.state !== 'ok' || ffmpeg.featuresBlocked) return
  const x0 = e.clientX
  const y0 = e.clientY
  let active = false
  const move = (ev: PointerEvent) => {
    if (!active && Math.hypot(ev.clientX - x0, ev.clientY - y0) < 4) return
    active = true
    dragging.value = s.path
    ed.sourceDrag.value = { path: s.path, x: ev.clientX, y: ev.clientY }
  }
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('keydown', key, true)
    if (active) {
      suppressClick = true
      setTimeout(() => (suppressClick = false), 0)
      ed.hooks.dropSource?.()
    }
    ed.sourceDrag.value = null
    dragging.value = ''
  }
  const key = (ev: KeyboardEvent) => {
    if (ev.key === 'Escape') {
      ev.stopPropagation()
      active = false
      up()
    }
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('keydown', key, true)
}

let off: () => void = () => {}
onMounted(() => {
  off = onFilesDropped((paths) => {
    if (ffmpeg.featuresBlocked) return ed.say(TEXT.ffmpegTip)
    ed.addSources(paths)
  })
})
onBeforeUnmount(() => off())
</script>

<style scoped>
.ed-probing {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 2px solid var(--ff-border);
  border-top-color: var(--ff-primary);
  animation: edspin 0.8s linear infinite;
}
.ed-thumb-ph {
  width: 100%;
  height: 100%;
  display: block;
}
.ed-thumb {
  background: var(--ff-bg-hover);
  color: var(--ff-text-2);
}
.ed-thumb.a {
  color: var(--ff-on-clip);
}
@keyframes edspin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .ed-probing {
    animation: none;
  }
}
</style>
