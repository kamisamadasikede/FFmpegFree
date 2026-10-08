<template>
  <LiveTabFrame>
    <template #main>
      <LivePlayer
        v-model:muted="muted"
        kind="pull"
        :phase="phase"
        :url="playUrl"
        :reason="reason"
        :clock="clock"
        :low-latency="lowLatency"
        :fake="vis?.fake || ''"
        :force-idle="!!vis?.idle"
        :force-vol="!!vis?.vol"
        :force-hint="!!vis?.hint"
        :force-full="!!vis?.full"
        :lag="vis?.lag ?? null"
        :aspect="vis?.aspect ?? 16 / 9"
        :empty-text="LP_EMPTY_PULL"
        @restart="start"
        @playing="onPlaying"
        @media-broken="onBroken"
        @media-ended="onEnded"
        @media-unsupported="onUnsup"
        @stats="onStats"
      />
    </template>
    <template #panel>
      <LivePanel title="拉流设置">
        <LiveField label="流地址">
          <LiveInput v-model="url" :bad="urlInvalid" :disabled="busy" placeholder="http://live.example.com/live/room.flv" @enter="start" />
          <InlineError v-if="urlInvalid" code="LIVE_URL_INVALID" description="请输入 http:// 或 ws:// 开头的流地址。" />
        </LiveField>
        <div class="tip">{{ LP_PULL_HINT }}</div>
        <div class="chk">低延迟追帧<el-switch v-model="lowLatency" size="small" aria-label="低延迟追帧" :disabled="busy" /></div>
        <template #action>
          <!-- 设计稿 04 / 17：播放中、被中断时是普通按钮「停止播放」（不用 danger，见设计说明 §九 第 11 条）；结束 / 不支持 / 未开始是「开始播放」 -->
          <LiveButton v-if="busy || phase === 'interrupted'" icon="x" @click="stop">停止播放</LiveButton>
          <LiveButton v-else variant="pri" icon="play" @click="start">开始播放</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
</template>

<script setup lang="ts">
// 拉流播放（包 21 / 契约 v0.25）：画面交给 LivePlayer（mpegts.js）。
// 开关关闭时 http(s) / ws(s) 直接播用户填的地址，不调用 StartPullPreview / GetPreview。
import { computed, onBeforeUnmount, ref, toRefs, watch } from 'vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LivePlayer from '@/components/live/LivePlayer.vue'
import InlineError from '@/components/common/InlineError.vue'
import { LP_EMPTY_PULL, LP_PULL_HINT } from '@/errors/livePreviewMessages'
import { formatClock, livePreview, useLiveSession } from '@/composables/useLiveSession'
import { isValidPullUrl } from '@/errors/playerError'
import { useLiveFormsStore } from '@/stores/liveForms'
import { useLiveDockStore } from '@/stores/liveDock'
import { lpVisual, type LpPhase } from './lpVisual'
import { classifyPreviewError, startPullPlayback, stopPullPlayback, type PullPlayback } from '@/api/livePreviewStream'

defineOptions({ name: 'LivePullPlay' })

const session = useLiveSession('pull')
const forms = useLiveFormsStore()
const dock = useLiveDockStore()
const { url, lowLatency, muted } = toRefs(forms.pull)
if (livePreview) url.value = livePreview === 'invalid' ? 'http:/live.example' : 'http://live.example.com/live/room.flv'
const urlInvalid = ref(livePreview === 'invalid')
const vis = lpVisual && lpVisual.tab === 'pull' ? lpVisual : null
const phase = ref<LpPhase>(vis?.phase ?? 'empty')
const reason = ref<'' | 'codec' | 'unavailable'>(vis?.reason ?? '')
const playUrl = ref('')
let playback: PullPlayback | null = null

const busy = computed(() => phase.value === 'connecting' || phase.value === 'playing' || phase.value === 'buffering')
const clock = computed(() => (vis ? vis.clock : formatClock(session.uptimeSec.value)))
// 角标 / 面板只数进行中的：被中断、结束、不支持都是 0 路
watch(busy, (b) => {
  if (b) dock.pull.active = true
  else dock.resetPull()
}, { immediate: true })
if (vis && (vis.phase === 'playing' || vis.phase === 'buffering')) Object.assign(dock.pull, { bitrate: '5986', fps: '30.0', dropped: '0', bytes: '812.4', unit: 'MB' })

async function start() {
  if (busy.value || vis) return
  const u = url.value.trim()
  if (!isValidPullUrl(u)) {
    urlInvalid.value = true
    session.log('流地址格式不正确')
    return
  }
  await stopPullPlayback(playback)
  playback = null
  playUrl.value = ''
  reason.value = ''
  phase.value = 'connecting'
  session.setStarting()
  session.log('开始拉流')
  try {
    playback = await startPullPlayback(u)
    const stream = playback.stream
    if (!stream?.url) {
      phase.value = 'unsupported'
      reason.value = 'unavailable'
      session.fail('UNSUPPORTED', 'reason=preview_unavailable')
      return
    }
    playUrl.value = stream.url
  } catch (e) {
    const k = classifyPreviewError(e)
    phase.value = k === 'unsupported' || k === 'unavailable' ? 'unsupported' : 'interrupted'
    reason.value = k === 'unsupported' ? 'codec' : k === 'unavailable' ? 'unavailable' : ''
    session.fail('LIVE_PLAY_FAILED', '')
  }
}

function onPlaying() {
  phase.value = 'playing'
  session.setRunning()
}
function onBroken() {
  phase.value = 'interrupted'
  session.fail('LIVE_PLAY_FAILED', '')
}
function onEnded() {
  phase.value = 'ended'
  session.setIdle()
}
function onUnsup() {
  phase.value = 'unsupported'
  reason.value = 'codec'
  session.fail('UNSUPPORTED', 'reason=codec')
}
function onStats(s: { kbps: number; fps: number; dropped: number; bytes: number }) {
  if (!busy.value) return
  dock.pull.bitrate = String(Math.round(s.kbps))
  dock.pull.fps = String(Math.round(s.fps))
  dock.pull.dropped = String(s.dropped)
  if (s.bytes >= 1048576) { dock.pull.bytes = (s.bytes / 1048576).toFixed(1); dock.pull.unit = 'MB' }
  else { dock.pull.bytes = String(Math.max(0, Math.round(s.bytes / 1024))); dock.pull.unit = 'KB' }
}
async function stop() {
  const wasBroken = phase.value === 'interrupted'
  playUrl.value = ''
  await stopPullPlayback(playback)
  playback = null
  phase.value = wasBroken ? 'empty' : 'ended'
  session.setIdle()
  session.log('已停止播放')
}
onBeforeUnmount(() => { void stopPullPlayback(playback) })
</script>

<style scoped>
.tip { font-size: var(--ff-fs-xs); color: var(--ff-text-2); line-height: 1.5; margin-top: -4px; }
.chk { display: flex; align-items: center; justify-content: space-between; font-size: var(--ff-fs-sm); }
</style>
