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
        :aspect="vis?.aspect"
        :empty-text="LP_EMPTY_PULL"
        :ended-note="endedNote"
        :break-text="breakText"
        :has-audio="hasAudio"
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
          <LiveInput v-model="url" :bad="urlInvalid" :disabled="busy" :placeholder="LP_PULL_PLACEHOLDER" @enter="start" />
          <InlineError v-if="urlInvalid" code="LIVE_URL_INVALID" :description="LP_PULL_URL_INVALID" />
        </LiveField>
        <div class="tip">{{ LP_PULL_HINT }}</div>
        <div class="chk">低延迟追帧<el-switch v-model="lowLatency" size="small" aria-label="低延迟追帧" :disabled="busy" /></div>
        <template #action>
          <!-- 设计说明 §4.2 / 场景 17（10-08 改）：只有进行中（连接中 / 播放中 / 缓冲中）是普通按钮「停止播放」（不用 danger，§九 第 11 条）；结束、被中断、不支持、未开始都是「开始播放」 -->
          <LiveButton v-if="busy" icon="x" @click="stop">停止播放</LiveButton>
          <LiveButton v-else variant="pri" icon="play" @click="start">开始播放</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
</template>

<script setup lang="ts">
// 拉流播放（包 21 / 契约 v0.25 6.10.3.3、6.10.3.7）：http(s) 走后端 StartPullPreview 的 previewUrl（转封装成 FLV），ws(s) 前端直接拉；画面交给 LivePlayer（mpegts.js）。
// 状态（包 22，契约 v0.25.1）：只往终态走，先到先定（stores/eventOrder.ts PullOutcomeGate）。
//   · 用户自己点「停止播放」→ 只写「拉流已结束」；
//   · 没点停止：先到 ended → 「拉流已结束」+「直播已停止，或连接已断开。」+「重新拉流」；先到 interrupted / failed →「拉流被中断，请重新拉流。」+「重新拉流」；
//     后到的事件都不再改文案（产品经理 / 设计 10-08）；
//   · 播放器读到流结尾（LOADING_COMPLETE）或网络出错时，有后端会话就先等后端的 live:pull（走查 G2），等不到再按播放器的结果定。
import { computed, onBeforeUnmount, ref, toRefs, watch } from 'vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LivePlayer from '@/components/live/LivePlayer.vue'
import InlineError from '@/components/common/InlineError.vue'
import { LP_EMPTY_PULL, LP_PULL_HINT, LP_PULL_PLACEHOLDER, LP_PULL_URL_INVALID } from '@/errors/livePreviewMessages'
import { formatClock, livePreview, useLiveSession } from '@/composables/useLiveSession'
import { isValidPullUrl } from '@/errors/playerError'
import { useLiveFormsStore } from '@/stores/liveForms'
import { useLiveDockStore } from '@/stores/liveDock'
import { PullOutcomeGate, type PullOutcome } from '@/stores/eventOrder'
import { lpVisual, type LpPhase } from './lpVisual'
import { classifyPreviewError, pullBreakText, pullEndedView, startPullPlayback, stopPullPlayback, watchPull, type PullEvent, type PullPlayback } from '@/api/livePreviewStream'

defineOptions({ name: 'LivePullPlay' })

const session = useLiveSession('pull')
const forms = useLiveFormsStore()
const dock = useLiveDockStore()
const { url, lowLatency, muted } = toRefs(forms.pull)
if (livePreview) url.value = livePreview === 'invalid' ? 'http:/live.example' : LP_PULL_PLACEHOLDER
const urlInvalid = ref(livePreview === 'invalid')
const vis = lpVisual && lpVisual.tab === 'pull' ? lpVisual : null
const phase = ref<LpPhase>(vis?.phase ?? 'empty')
const reason = ref<'' | 'codec' | 'unavailable'>(vis?.reason ?? '')
const playUrl = ref('')
const hasAudio = ref(true)
/** 结束时的第二行：只有不是用户点停止而结束时才有 */
const endedNote = ref(vis?.phase === 'ended' && previewParamsRemote() ? pullEndedView(false).note : '')
/** 被中断时的正文：failed / 开始失败用后端 message（写着「推流」时换成拉流失败的兜底句），其余用默认的「拉流被中断，请重新拉流。」 */
const breakText = ref('')
let playback: PullPlayback | null = null
let unwatch: (() => void) | null = null
function previewParamsRemote() {
  return new URLSearchParams(window.location.search).get('remote') === '1' // 截图：?lpv=pull-ended&remote=1
}

const gate = new PullOutcomeGate(applyOutcome, () => !!playback?.session)

const busy = computed(() => phase.value === 'connecting' || phase.value === 'playing' || phase.value === 'buffering')
const clock = computed(() => (vis ? vis.clock : formatClock(session.uptimeSec.value)))
// 角标 / 面板只数进行中的：被中断、结束、不支持都是 0 路
watch(busy, (b) => {
  if (b) dock.pull.active = true
  else dock.resetPull()
}, { immediate: true })
if (vis && (vis.phase === 'playing' || vis.phase === 'buffering')) Object.assign(dock.pull, { bitrate: '5986', fps: '30.0', dropped: '0', bytes: '812.4', unit: 'MB' })
// G6：地址改过就收起报错（开始时再校验一次）
watch(url, () => { if (!livePreview) urlInvalid.value = false })

async function start() {
  if (busy.value || vis) return
  const u = url.value.trim()
  if (!isValidPullUrl(u)) {
    urlInvalid.value = true
    session.log('流地址格式不正确')
    return
  }
  urlInvalid.value = false
  unwatch?.()
  unwatch = null
  await stopPullPlayback(playback)
  playback = null
  gate.reset()
  playUrl.value = ''
  reason.value = ''
  endedNote.value = ''
  breakText.value = ''
  phase.value = 'connecting'
  session.setStarting()
  session.log('开始拉流')
  try {
    const pb = await startPullPlayback(u)
    if (gate.settled) {
      // start 期间用户已经点了停止 / 离开页面
      void stopPullPlayback(pb)
      return
    }
    playback = pb
    if (pb.session) unwatch = watchPull(pb.session.id, onPullEvent)
    const stream = pb.stream
    if (!stream?.url) {
      reason.value = 'unavailable'
      gate.startFailed('unsupported')
      return
    }
    hasAudio.value = stream.hasAudio
    playUrl.value = stream.url
  } catch (e) {
    const k = classifyPreviewError(e)
    reason.value = k === 'unsupported' ? 'codec' : k === 'unavailable' ? 'unavailable' : ''
    gate.startFailed(k === 'unsupported' || k === 'unavailable' ? 'unsupported' : 'interrupted', pullBreakText(''))
  }
}

function onPullEvent(e: PullEvent) {
  if (e.state === 'playing') return // 画面以播放器真的出帧为准（onPlaying）
  if (e.state === 'unsupported') reason.value = reason.value || 'codec'
  gate.event(e.state, e.state === 'failed' ? pullBreakText(e.error?.message) : undefined)
}
function onPlaying() {
  if (!busy.value) return
  phase.value = 'playing'
  session.setRunning()
}
/** 终态定下来了（只会来一次，直到下次开始）：停后端会话、换界面 */
function applyOutcome(o: PullOutcome) {
  playUrl.value = ''
  unwatch?.()
  unwatch = null
  void stopPullPlayback(playback)
  playback = null
  if (o.phase === 'unsupported') {
    phase.value = 'unsupported'
    reason.value = reason.value || 'codec'
    session.fail('UNSUPPORTED', reason.value === 'unavailable' ? 'reason=preview_unavailable' : 'reason=codec')
  } else if (o.phase === 'interrupted') {
    breakText.value = o.message ?? ''
    phase.value = 'interrupted'
    session.fail('LIVE_PLAY_FAILED', '')
  } else {
    endedNote.value = pullEndedView(o.byUser).note
    phase.value = 'ended'
    session.setIdle()
    if (o.byUser) session.log('已停止播放')
  }
}
function onBroken() {
  if (busy.value) gate.player('interrupted')
}
function onEnded() {
  if (busy.value) gate.player('ended')
}
function onUnsup() {
  if (!busy.value) return
  reason.value = 'codec'
  gate.player('unsupported')
}
function onStats(s: { kbps: number; fps: number; dropped: number; bytes: number }) {
  if (!busy.value) return
  dock.pull.bitrate = String(Math.round(s.kbps))
  dock.pull.fps = String(Math.round(s.fps))
  dock.pull.dropped = String(s.dropped)
  if (s.bytes >= 1048576) { dock.pull.bytes = (s.bytes / 1048576).toFixed(1); dock.pull.unit = 'MB' }
  else { dock.pull.bytes = String(Math.max(0, Math.round(s.bytes / 1024))); dock.pull.unit = 'KB' }
}
/** 用户自己点「停止播放」：只写「拉流已结束」，没有第二行 */
function stop() {
  if (!busy.value) return
  gate.user()
}
onBeforeUnmount(() => {
  gate.close() // 之后到的事件都不再处理
  unwatch?.()
  void stopPullPlayback(playback)
  playback = null
})
</script>


<style scoped>
.tip { font-size: var(--ff-fs-xs); color: var(--ff-text-2); line-height: 1.5; margin-top: -4px; }
.chk { display: flex; align-items: center; justify-content: space-between; font-size: var(--ff-fs-sm); }
</style>
