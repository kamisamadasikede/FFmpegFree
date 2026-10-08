<template>
  <div
    ref="root"
    class="lp-stage"
    :class="{ dim: dimmed, idle: idleOn, 'is-full': fullOn }"
    :style="{ '--ar': String(stageAspect) }"
    role="region"
    :aria-roledescription="'直播播放器'"
    :aria-label="kind === 'pull' ? '拉流画面' : '推流预览'"
    aria-keyshortcuts="Space M F"
    tabindex="0"
    @mousemove="wake"
    @keydown="onKey"
    @dblclick="audioOnly || toggleFull()"
  >
    <div v-if="showVideo" class="lp-video" :class="fake ? 'f' + fake : ''">
      <video :key="videoKey" v-show="!fake && !frozen && !audioOnly" ref="videoEl" autoplay playsinline :muted="muted" />
      <!-- G1：结束 / 被中断时停在最后一帧（播放器销毁前把当前画面画到这里），再由 .dim 压暗 -->
      <canvas v-show="frozen && !fake" ref="shotEl" class="lp-shot" aria-hidden="true" />
    </div>
    <p v-if="phase === 'empty'" class="lp-wait">{{ emptyText || LP_EMPTY }}</p>
    <!-- 包 24 N2：只有声音的流——舞台保持黑色，中间 32 号音符 + 一行 12 号次要色说明；声音照常从 <video> 出 -->
    <div v-if="audioOnly && showBar" class="lp-audio"><FIcon name="music" :size="32" /><span>{{ LP_AUDIO_ONLY }}</span></div>

    <div v-if="showChip" class="lp-chip" :class="{ play: kind === 'pull' }">
      <i />{{ kind === 'pull' ? '播放中' : '直播中' }}<span class="t">{{ clock }}</span>
    </div>
    <button v-if="showHint" type="button" class="lp-hint" role="button" aria-label="已静音，点击开启声音" @click="unmute">
      <FIcon name="mute" :size="14" />{{ LP_MUTED_HINT }}
    </button>
    <div v-if="fullOn && showEsc" class="lp-esc">按 <kbd>Esc</kbd> 退出全屏</div>

    <div v-if="overlay" class="lp-ov" :class="{ dimbg: dimmed, buf: phase === 'buffering' }" :role="phase === 'interrupted' ? 'alert' : 'status'">
      <div class="in">
        <template v-if="phase === 'connecting' || phase === 'buffering'">
          <i class="lp-spin" aria-hidden="true" />
          <p v-if="phase === 'connecting'">{{ LP_CONNECTING }}</p>
        </template>
        <template v-else>
          <span class="ic" :class="{ warn: phase === 'interrupted' }"><FIcon :name="phase === 'interrupted' ? 'warn' : phase === 'ended' ? 'stop' : 'block'" :size="20" /></span>
          <p>{{ overlayText }}</p>
          <small v-if="phase === 'ended' && endedNote" class="lp-sub">{{ endedNote }}</small>
          <button v-if="phase === 'interrupted' || (phase === 'ended' && endedNote)" type="button" class="lp-act" @click="retry"><FIcon name="retry" :size="14" />{{ kind === 'pull' ? LP_RETRY_PULL : LP_RETRY_PUSH }}</button>
        </template>
      </div>
    </div>

    <div v-if="showBar" class="lp-bar">
      <button type="button" class="lp-btn lp-snd" :aria-label="muted ? '开启声音' : '静音'" :aria-pressed="muted" :title="muted ? '开启声音（M）' : '静音（M）'" :disabled="!hasAudio" @click="toggleMute">
        <FIcon :name="muted || !hasAudio ? 'mute' : 'vol'" :size="18" />
      </button>
      <div class="lp-vol" :class="{ open: volOpen }" @mouseenter="volHover = true" @mouseleave="scheduleVolClose">
        <div
          class="tr"
          role="slider"
          tabindex="0"
          aria-label="音量"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-valuenow="muted ? 0 : volume"
          :aria-valuetext="muted ? '已静音' : volume + '%'"
          @keydown="onVolKey"
          @pointerdown="onVolDown"
          @focus="volHover = true"
          @blur="scheduleVolClose"
        ><i :style="{ width: (muted ? 0 : volume) + '%' }" /><b :style="{ left: (muted ? 0 : volume) + '%' }" /></div>
      </div>
      <span class="sp" />
      <div v-if="lagShown != null && !audioOnly" class="lp-lagbox">
        <span class="lp-lag" aria-hidden="true">{{ LP_LAG(lagShown) }}</span>
        <button type="button" class="lp-live" :aria-label="`回到最新画面，当前落后约 ${lagShown} 秒`" @click="catchUp"><FIcon name="refresh" :size="14" />{{ LP_CATCHUP }}</button>
      </div>
      <button v-if="!audioOnly" type="button" class="lp-btn" :aria-label="fullOn ? '退出全屏' : '全屏'" :title="fullOn ? '退出全屏（F 或 Esc）' : '全屏（F）'" @click="toggleFull">
        <FIcon :name="fullOn ? 'zip' : 'full'" :size="18" />
      </button>
    </div>
    <div class="sr" aria-live="polite" data-live>{{ liveText }}</div>
  </div>
</template>

<script setup lang="ts">
// 直播播放器（设计说明 v0.1 §二–§六）。不持有业务：地址由父组件给出，mpegts.js 只负责播。
// 低延迟：enableStashBuffer 关、追帧上限 1.5 秒（契约 6.10.3.8）。默认静音。直播不能暂停、不能拖。
import { computed, nextTick, onActivated, onBeforeUnmount, onDeactivated, ref, watch } from 'vue'
import mpegts from 'mpegts.js'
import { guardCall, installMseGuard } from './mseGuard'
import FIcon from '@/components/icon/FIcon.vue'
import {
  LP_BREAK_PULL, LP_BREAK_PUSH, LP_CATCHUP, LP_CONNECTING, LP_EMPTY, LP_END_PULL, LP_END_PUSH, LP_LAG, LP_MUTED_HINT, LP_RETRY_PULL, LP_RETRY_PUSH, LP_UNAVAILABLE, LP_UNAVAILABLE_PULL, LP_UNSUP_PULL, LP_UNSUP_PUSH,
  LP_LIVE_BUFFERING, LP_LIVE_MUTED_SUFFIX, LP_LIVE_STARTED_PULL, LP_LIVE_STARTED_PUSH, LP_AUDIO_ONLY,
} from '@/errors/livePreviewMessages'
import { liveAnnouncement, nextLagShown, stageAspectOf } from './livePlayerLogic'

const props = withDefaults(defineProps<{
  kind: 'push' | 'pull'
  phase: 'empty' | 'connecting' | 'playing' | 'buffering' | 'unsupported' | 'ended' | 'interrupted'
  /** 播放地址；空 = 不建播放器（截图用模拟画面） */
  url?: string
  mime?: string
  hasAudio?: boolean
  /** 包 24 N2：这路流有没有画面（拉流：后端字段 / 播放器媒体信息）。false = 只有声音：按纯音频建播放器，舞台显示音符 */
  hasVideo?: boolean
  clock?: string
  /** 画面比例：不传时取视频自己的宽高比（G4，竖屏流按舞台高度完整显示），视频还没出来时按 16:9 */
  aspect?: number
  /** 截图：模拟画面 a/b/c */
  fake?: '' | 'a' | 'b' | 'c'
  /** 截图：强制控件淡出 / 音量展开 / 已静音提示 / 全屏 */
  forceIdle?: boolean
  forceVol?: boolean
  forceHint?: boolean
  forceFull?: boolean
  lag?: number | null
  /** unsupported 的原因：codec 按推流 / 拉流分；unavailable 推流页用推流那句，拉流页用「这路视频暂时无法在应用内播放。」（包 22） */
  reason?: '' | 'codec' | 'unavailable'
  lowLatency?: boolean
  /** phase=empty 时的文字（默认“还没有进行中的预览”） */
  emptyText?: string
  /** phase=ended 时的第二行（拉流：不是用户点停止而结束）；有它时旁边给「重新拉流」 */
  endedNote?: string
  /** phase=interrupted 时换掉默认正文（拉流开始前就失败：用后端 error 的 message） */
  breakText?: string
}>(), { url: '', mime: 'video/x-flv', hasAudio: true, hasVideo: true, clock: '00:00:00', aspect: undefined, fake: '', lag: null, reason: '', lowLatency: true })

const emit = defineEmits<{ 'media-info': [m: { hasVideo: boolean; hasAudio: boolean }]; restart: []; catchup: []; 'media-unsupported': []; 'media-ended': []; 'media-broken': []; playing: []; stats: [s: { kbps: number; fps: number; dropped: number; bytes: number }] }>()
const muted = defineModel<boolean>('muted', { default: true })
const volume = ref(70)
const root = ref<HTMLElement | null>(null)
const videoEl = ref<HTMLVideoElement | null>(null)
const shotEl = ref<HTMLCanvasElement | null>(null)
/** G1：正在显示最后一帧的快照（只在真正结束 / 被中断时；离开页面不留这一帧） */
const frozen = ref(false)
/** 每次重连换一个新的 video，不用离开前的那个元素和缓冲 */
const videoKey = ref(0)
/** KeepAlive 把页面藏起来了：这期间不建播放器 */
const away = ref(false)
/** G4：视频自己的宽高比（videoWidth / videoHeight），没有画面时为 null */
const natural = ref<number | null>(null)
const stageAspect = computed(() => stageAspectOf(props.aspect, natural.value))
/** 追帧关闭时自己量的落后秒数（「回到最新」胶囊，3 秒出现、1.5 秒收起），null = 不显示 */
const lagOwn = ref<number | null>(null)
const lagShown = computed(() => (props.lag != null ? props.lag : props.lowLatency ? null : lagOwn.value))
/** 缓冲超过 2 秒才播报「正在缓冲」（§6.2） */
const bufLong = ref(false)
/** 进入播放中那一刻是否静音（播报「…已开始，已静音」用；之后切换静音不重播） */
const startMuted = ref(true)
let bufLongTimer: ReturnType<typeof setTimeout> | undefined
const idle = ref(false)
const volHover = ref(false)
const hintOn = ref(false)
const hinted = ref(false)
const full = ref(false)
const liveText = ref('')
let player: mpegts.Player | null = null
/** 离开页面后，下一次播放换一个新的 <video> 和新的 MediaSource */
let freshVideo = false
/** WebKitGTK 第一次 MSE 失败时，换元素再试，避免停在「正在连接…」 */
let mseRecoveries = 0
installMseGuard()
/** 正在拆播放器：拆的过程里 mpegts 会报错 / 报结束，这些不算断流 */
let quiet = false
let attachToken = 0
let idleTimer: ReturnType<typeof setTimeout> | undefined
let hintTimer: ReturnType<typeof setTimeout> | undefined
let volTimer: ReturnType<typeof setTimeout> | undefined
let bufTimer: ReturnType<typeof setTimeout> | undefined
const buffering = ref(false)

/** 播放器自己读到的媒体信息里没有画面（直接拉 ws / wss 时，FLV 头写明只有声音） */
const mediaNoVideo = ref(false)
/** 只有声音：不显示视频、不给全屏和「回到最新」，只留音量 */
const audioOnly = computed(() => !props.hasVideo || mediaNoVideo.value)
const fullOn = computed(() => props.forceFull || full.value)
const idleOn = computed(() => props.forceIdle || idle.value)
const volOpen = computed(() => props.forceVol || volHover.value)
// 连接中也要有 <video>（在转圈下面，不可见画面）：播放器要先挂上它才能出第一帧
const showVideo = computed(() => props.phase === 'connecting' || props.phase === 'playing' || props.phase === 'buffering' || props.phase === 'ended' || props.phase === 'interrupted')
const dimmed = computed(() => props.phase === 'ended' || props.phase === 'interrupted')
const showChip = computed(() => props.phase === 'playing' || props.phase === 'buffering' || (props.phase === 'unsupported' && props.kind === 'push'))
const showBar = computed(() => props.phase === 'playing' || props.phase === 'buffering')
// X1：没有声音（录屏推流没选音轨等）时不提示「已静音」
const showHint = computed(() => showBar.value && (props.forceHint || (hintOn.value && props.hasAudio)))
const showEsc = ref(true)
const overlay = computed(() => props.phase === 'connecting' || props.phase === 'buffering' || props.phase === 'unsupported' || props.phase === 'ended' || props.phase === 'interrupted')
const overlayText = computed(() => {
  if (props.phase === 'ended') return props.kind === 'pull' ? LP_END_PULL : LP_END_PUSH
  if (props.phase === 'interrupted') return props.breakText || (props.kind === 'pull' ? LP_BREAK_PULL : LP_BREAK_PUSH)
  if (props.reason === 'unavailable') return props.kind === 'pull' ? LP_UNAVAILABLE_PULL : LP_UNAVAILABLE
  return props.kind === 'pull' ? LP_UNSUP_PULL : LP_UNSUP_PUSH
})

watch(() => props.phase, (p, prev) => {
  if (p === 'playing' && prev !== 'playing' && prev !== 'buffering') startMuted.value = muted.value
  if (p === 'playing' && muted.value && props.hasAudio && !hinted.value && !props.forceIdle) {
    hintOn.value = true
    hinted.value = true
    clearTimeout(hintTimer)
    hintTimer = setTimeout(() => (hintOn.value = false), 5000)
  }
  if (p !== 'playing' && p !== 'buffering') {
    hintOn.value = false
    bufLong.value = false
    clearTimeout(bufLongTimer)
  }
  if (p === 'connecting' || p === 'empty' || p === 'unsupported') frozen.value = false
}, { immediate: true })
// G5：播报区跟着状态走（连接中 → 已开始 → 缓冲超过 2 秒 → 结束 / 中断 / 不支持），不随计时变化
let lastAnnounced = ''
watch(
  () => liveAnnouncement({
    phase: props.phase, kind: props.kind, startMuted: startMuted.value, hasAudio: props.hasAudio, bufferingLong: bufLong.value, overlayText: overlayText.value,
    endedNote: props.endedNote ?? '', text: { connecting: LP_CONNECTING, startedPush: LP_LIVE_STARTED_PUSH, startedPull: LP_LIVE_STARTED_PULL, mutedSuffix: LP_LIVE_MUTED_SUFFIX, buffering: LP_LIVE_BUFFERING },
  }),
  (t) => {
    if (t === null || t === lastAnnounced) return
    lastAnnounced = t
    liveText.value = t
  },
  { immediate: true },
)

watch(() => props.forceHint, (v) => { if (v) hintOn.value = true })

function wake() {
  idle.value = false
  if (fullOn.value) showEsc.value = true
  clearTimeout(idleTimer)
  idleTimer = setTimeout(() => { idle.value = true; showEsc.value = false }, 3000)
}
function unmute() { muted.value = false; volume.value = volume.value || 70; hintOn.value = false }
function toggleMute() { if (!props.hasAudio) return; muted.value = !muted.value; if (!muted.value && !volume.value) volume.value = 70; hintOn.value = false }
function scheduleVolClose() { clearTimeout(volTimer); volTimer = setTimeout(() => (volHover.value = false), 300) }
function toggleFull() {
  if (props.forceFull) return
  const el = root.value
  if (!el) return
  if (document.fullscreenElement) { document.exitFullscreen().catch(() => undefined); full.value = false }
  else { el.requestFullscreen?.().catch(() => undefined); full.value = true }
}
function onFs() { full.value = document.fullscreenElement === root.value }
document.addEventListener('fullscreenchange', onFs)
function retry() {
  if (full.value || document.fullscreenElement) { document.exitFullscreen().catch(() => undefined); full.value = false }
  emit('restart')
}
function onKey(e: KeyboardEvent) {
  wake()
  if (e.key === ' ' || e.key === 'm' || e.key === 'M') { if ((e.target as HTMLElement).getAttribute('role') === 'slider') return; e.preventDefault(); toggleMute() }
  else if ((e.key === 'f' || e.key === 'F') && !audioOnly.value) { e.preventDefault(); toggleFull() }
  else if (e.key === 'Escape' && full.value) { full.value = false }
}
function setVol(n: number) {
  volume.value = Math.max(0, Math.min(100, n))
  muted.value = volume.value === 0
}
function onVolKey(e: KeyboardEvent) {
  const step = e.key === 'Home' ? -100 : e.key === 'End' ? 100 : e.key === 'ArrowLeft' || e.key === 'ArrowDown' ? -5 : e.key === 'ArrowRight' || e.key === 'ArrowUp' ? 5 : 0
  if (!step && e.key !== 'Home' && e.key !== 'End') return
  e.preventDefault()
  setVol(e.key === 'Home' ? 0 : e.key === 'End' ? 100 : volume.value + step)
}
function onVolDown(e: PointerEvent) {
  const el = e.currentTarget as HTMLElement
  const move = (ev: PointerEvent) => { const r = el.getBoundingClientRect(); setVol(Math.round(((ev.clientX - r.left) / r.width) * 100)) }
  move(e)
  const up = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up) }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

let statTimer: ReturnType<typeof setInterval> | null = null
let lastDecoded = 0
/** 最近几秒的解码帧数：WebKitGTK 的解码计数是成批更新的（实测每秒读数在 18 / 37 之间跳），按 3 秒平均 */
let fpsWin: number[] = []
let accBytes = 0
function stopStats() { if (statTimer) clearInterval(statTimer); statTimer = null }
/** G1：把当前画面画到快照上（MSE 的 blob 地址同源，画布不会被污染）；没有画面时什么也不做 */
function freeze() {
  const v = videoEl.value
  const c = shotEl.value
  if (!v || !c || !v.videoWidth || !v.videoHeight) return
  try {
    c.width = v.videoWidth
    c.height = v.videoHeight
    c.getContext('2d')?.drawImage(v, 0, 0, c.width, c.height)
    frozen.value = true
  } catch {
    frozen.value = false
  }
}
function catchUp() {
  emit('catchup')
  const el = videoEl.value
  if (!el || props.lag != null) return
  guardCall(() => {
    const b = el.buffered
    if (b.length) el.currentTime = Math.max(el.currentTime, b.end(b.length - 1) - 0.3)
  })
  lagOwn.value = null
}
function destroyPlayer() {
  quiet = true
  clearTimeout(bufTimer)
  clearTimeout(bufLongTimer)
  lagOwn.value = null
  clearTimeout(retryTimer)
  stopStats()
  const dying = player
  player = null
  if (!dying) return
  // 只拆一次。unload 会 flush，detach 会 endOfStream；拆的时候元素必须还在文档里。
  try { dying.pause() } catch { /* 已销毁 */ }
  try { dying.unload() } catch { /* 已销毁 */ }
  try { dying.detachMediaElement() } catch { /* 已销毁 */ }
  try { dying.destroy() } catch { /* 已销毁 */ }
}
/** 等一拍，让排队的 updateend / endOfStream 在旧 <video> 还在时跑完，再换元素。 */
function afterMseSettles(): Promise<void> {
  return new Promise((r) => setTimeout(r, 0))
}
const CONNECT_WAIT_MS = 12000
let connectSince = 0
let retryTimer: ReturnType<typeof setTimeout> | undefined
async function attach(url: string, retry = false) {
  const my = ++attachToken
  if (away.value || props.fake) return
  if (!retry) connectSince = Date.now()
  // 先把旧播放器拆干净，再换 <video>。反过来的话 WebKitGTK 会在 MediaSource 已关闭时继续 append / endOfStream。
  destroyPlayer()
  await afterMseSettles()
  if (my !== attachToken || away.value || wantUrl.value !== url) return
  if (!retry) {
    natural.value = null
    mediaNoVideo.value = false
    frozen.value = false
    // 第一次用挂载时那个 <video>（和包 22 一样）。只有离开过页面才换新元素，避免一上来就把 MSE 拆掉。
    if (freshVideo) {
      freshVideo = false
      videoKey.value++
      await nextTick()
    }
  }
  if (my !== attachToken || away.value || wantUrl.value !== url) return
  quiet = false
  const el = videoEl.value
  if (!el) return
  if (!mpegts.getFeatureList().mseLivePlayback) { emit('media-unsupported'); return }
  const chase = props.lowLatency
  player = mpegts.createPlayer(
    // 包 24 N2：只在确定没有画面 / 声音时才传 false。mpegts 对传进去的布尔值一律当真（传 true 也会盖掉 FLV 头），
    // 以前写死 hasVideo: true，纯音频流就一直等画面的元数据，停在「正在连接…」。不知道时交给 FLV 头和后续的音视频标签判断。
    { type: /mp2t|mpegts/i.test(props.mime) ? 'mpegts' : 'flv', isLive: true, url, hasAudio: props.hasAudio ? undefined : false, hasVideo: props.hasVideo ? undefined : false },
    chase
      ? // 追帧主要靠略微加速（liveSync，不跳帧）；跳到最新位置只在落后很多时兜底。
      // 实测 Linux WebKitGTK：分段到达的源（HLS 等）每到一段就超过 1.5 秒，按跳转追帧会落在 GOP 中间，灰色花屏直到下一个关键帧。
      { enableWorker: false, enableStashBuffer: false, stashInitialSize: 128, liveSync: true, liveSyncMaxLatency: 1.2, liveSyncTargetLatency: 0.6, liveSyncPlaybackRate: 1.2, liveBufferLatencyChasing: true, liveBufferLatencyMaxLatency: 6, liveBufferLatencyMinRemain: 1, autoCleanupSourceBuffer: true }
      : { enableWorker: false, enableStashBuffer: true, autoCleanupSourceBuffer: true },
  )
  player.on(mpegts.Events.ERROR, (type: string, detail: string) => {
    if (quiet) return
    if (detail === mpegts.ErrorDetails.MEDIA_CODEC_UNSUPPORTED || detail === mpegts.ErrorDetails.MEDIA_FORMAT_UNSUPPORTED) emit('media-unsupported')
    else if (props.phase === 'playing' || props.phase === 'buffering') emit('media-broken')
    else if (type === mpegts.ErrorTypes.NETWORK_ERROR && props.phase === 'connecting' && Date.now() - connectSince < CONNECT_WAIT_MS) {
      // 契约 6.10.3.4：还没收到 FLV 头时本机预览服务回 503（最多约 10 秒），隔 1 秒重连
      clearTimeout(retryTimer)
      retryTimer = setTimeout(() => { if (wantUrl.value === url) attach(url, true) }, 1000)
    } else if (detail === mpegts.ErrorDetails.MEDIA_MSE_ERROR && props.phase === 'connecting' && mseRecoveries < 2) {
      // WebKitGTK：拆播放器时 MediaSource 已关闭，append 报 InvalidStateError。换一个新元素再连，不当成断流。
      mseRecoveries++
      freshVideo = true
      clearTimeout(retryTimer)
      retryTimer = setTimeout(() => { if (wantUrl.value === url && !away.value) void attach(url) }, 300)
    } else emit('media-broken')
  })
  player.on(mpegts.Events.MEDIA_INFO, (mi: { hasVideo?: boolean; hasAudio?: boolean } | undefined) => {
    if (quiet || !mi || mi.hasVideo !== false) return
    mediaNoVideo.value = true
    emit('media-info', { hasVideo: false, hasAudio: mi.hasAudio !== false })
  })
  player.on(mpegts.Events.LOADING_COMPLETE, () => { if (!quiet) emit('media-ended') })
  el.addEventListener('playing', () => { mseRecoveries = 0; if (!quiet) emit('playing') }, { once: true })
  el.addEventListener('error', () => {
    if (quiet || my !== attachToken || retry) return
    if (mseRecoveries >= 2 || props.phase !== 'connecting') return
    mseRecoveries++
    freshVideo = true
    clearTimeout(retryTimer)
    retryTimer = setTimeout(() => { if (wantUrl.value === url && !away.value) void attach(url) }, 300)
  })
  // 缓冲：不加转圈（追帧加速和落后 6 秒跳到最新都不能出转圈，设计 10-08）；超过 2 秒只在读屏里播报「正在缓冲」
  el.addEventListener('waiting', () => {
    clearTimeout(bufTimer)
    clearTimeout(bufLongTimer)
    bufTimer = setTimeout(() => { if (props.phase === 'playing') buffering.value = true }, 500)
    bufLongTimer = setTimeout(() => { if (props.phase === 'playing' || props.phase === 'buffering') bufLong.value = true }, 2000)
  })
  el.addEventListener('playing', () => { clearTimeout(bufTimer); clearTimeout(bufLongTimer); buffering.value = false; bufLong.value = false })
  // G4：画面比例按视频自己的宽高（竖屏流铺满舞台高度、左右补黑）
  const onSize = () => { if (el.videoWidth && el.videoHeight) natural.value = el.videoWidth / el.videoHeight }
  el.addEventListener('loadedmetadata', onSize)
  el.addEventListener('resize', onSize)
  // WebKitGTK 不会自己跳过开头的空档：第一段缓冲从 1.x 秒开始而 currentTime 还是 0 时会一直卡在“正在连接”，这里手动跳到缓冲开头
  const jumpGap = () => {
    guardCall(() => {
      const b = el.buffered
      if (b.length && el.currentTime < b.start(0) - 0.01) el.currentTime = b.start(0) + 0.05
    })
  }
  el.addEventListener('progress', jumpGap)
  el.addEventListener('loadedmetadata', jumpGap)
  player.attachMediaElement(el)
  el.muted = muted.value
  player.load()
  lastDecoded = 0
  fpsWin = []
  accBytes = 0
  stopStats()
  statTimer = setInterval(() => {
    const si = (player as { statisticsInfo?: { decodedFrames?: number; speed?: number; droppedFrames?: number } } | null)?.statisticsInfo
    jumpGap()
    // 追帧关闭时量落后多少（缓冲末尾 - 当前播放位置），3 秒出现、1.5 秒以下收起
    if (!props.lowLatency) {
      guardCall(() => {
        if (el.buffered.length) lagOwn.value = nextLagShown(lagOwn.value, el.buffered.end(el.buffered.length - 1) - el.currentTime)
        else lagOwn.value = null
      })
    } else lagOwn.value = null
    if (!si) return
    const decoded = si.decodedFrames ?? 0
    if (lastDecoded) fpsWin = [...fpsWin, Math.max(0, decoded - lastDecoded)].slice(-3)
    const fps = fpsWin.length ? fpsWin.reduce((a, b) => a + b, 0) / fpsWin.length : 0
    lastDecoded = decoded
    accBytes += (si.speed ?? 0) * 1024
    emit('stats', { kbps: (si.speed ?? 0) * 8, fps, dropped: si.droppedFrames ?? 0, bytes: accBytes })
  }, 1000)
  const p = player.play()
  if (p && typeof (p as Promise<void>).catch === 'function') (p as Promise<void>).catch(() => { muted.value = true; hintOn.value = true; void el.play().catch(() => undefined) })
}
// 只在地址变了、或从非进行中变成进行中时建播放器；连接中 → 播放中不重建（否则会断开重连一次）
const wantUrl = computed(() => (props.url && !props.fake && (props.phase === 'connecting' || props.phase === 'playing' || props.phase === 'buffering') ? props.url : ''))
watch(wantUrl, (url) => {
  if (away.value) {
    destroyPlayer() // 离开页面：只断开，不截最后一帧
    return
  }
  if (url) void attach(url)
  else {
    if (dimmed.value) freeze() // G1：结束 / 中断前留下最后一帧
    destroyPlayer()
  }
}, { flush: 'post' }) // 等 <video> 渲染出来再挂
// 包 24 N2：连接中才知道只有声音（后端 live:pull playing / GetPreviewStream 说没有画面）：按纯音频重建播放器，不然会一直等画面
watch(() => props.hasVideo, (v, old) => {
  if (v === old || away.value) return
  const url = wantUrl.value
  if (url && props.phase === 'connecting') void attach(url)
})
watch(muted, (m) => { if (videoEl.value) videoEl.value.muted = m })
watch(volume, (v) => { if (videoEl.value) videoEl.value.volume = v / 100 })

// 离开页面（KeepAlive 藏起来，或整页卸载）：拆掉播放器并断开预览连接。不在这里截最后一帧。
onDeactivated(() => {
  away.value = true
  frozen.value = false
  // 不在这一拍换 video：Vue 会先把元素卸掉，mpegts 的 endOfStream 还在队列里。回来时 attach 再换。
  freshVideo = true
  destroyPlayer()
})
onActivated(() => {
  away.value = false
  if (wantUrl.value) void attach(wantUrl.value)
})
onBeforeUnmount(() => { destroyPlayer(); document.removeEventListener('fullscreenchange', onFs); clearTimeout(idleTimer); clearTimeout(hintTimer); clearTimeout(volTimer) })
</script>

<style scoped>
.lp-stage { --lp-scrim: rgba(0, 0, 0, .6); --lp-fg: #fff; --lp-fg-2: rgba(255, 255, 255, .78); --lp-hover: rgba(255, 255, 255, .16); --lp-track: rgba(255, 255, 255, .32); --lp-live: #ff4d4f; --lp-play: #22c55e; --lp-warn: #fbbf24; --lp-focus: #7aa2ff; position: relative; overflow: hidden; background: #000; container-type: size; display: grid; grid-template: minmax(0, 1fr) / minmax(0, 1fr); place-items: center; color: #fff; outline: none; flex: 1; min-height: 0; width: 100%; }
.lp-stage:focus-visible { box-shadow: inset 0 0 0 2px var(--lp-focus); }
.lp-video { width: min(100cqw, calc(100cqh * var(--ar))); aspect-ratio: var(--ar); position: relative; overflow: hidden; background: #000; }
.lp-video video, .lp-video .lp-shot { width: 100%; height: 100%; object-fit: contain; background: #000; display: block; }
.lp-stage.dim .lp-video { filter: brightness(.42) saturate(.4); }
.fa, .lp-video.fa { background: radial-gradient(circle at 28% 42%, #67e8f9 0 11%, rgba(103, 232, 249, 0) 11.5%), radial-gradient(circle at 70% 58%, #f472b6 0 17%, rgba(244, 114, 182, 0) 17.5%), linear-gradient(135deg, #0f172a, #312e81 55%, #0e7490); }
.lp-video.fb { background: repeating-radial-gradient(ellipse at 50% 120%, rgba(255, 255, 255, .1) 0 6px, rgba(255, 255, 255, 0) 6px 22px), linear-gradient(180deg, #0c4a6e, #0e7490 45%, #14b8a6 70%, #f59e0b); }
.lp-video.fc { background: radial-gradient(circle at 50% 30%, #fbcfe8 0 14%, rgba(251, 207, 232, 0) 14.5%), linear-gradient(160deg, #4c1d95, #7c3aed 45%, #db2777 80%, #f97316); }
.lp-chip { position: absolute; left: 12px; top: 12px; z-index: 2; height: 24px; padding: 0 8px; border-radius: 4px; background: var(--lp-scrim); display: flex; align-items: center; gap: 6px; font-size: 12px; white-space: nowrap; }
.lp-chip i { width: 8px; height: 8px; border-radius: 50%; background: var(--lp-live); }
.lp-chip.play i { background: var(--lp-play); }
.lp-chip .t { font-family: var(--ff-font-mono); }
.lp-hint { position: absolute; top: 12px; left: 50%; transform: translateX(-50%); z-index: 2; height: 28px; padding: 0 12px 0 10px; border-radius: 14px; background: var(--lp-scrim); display: flex; align-items: center; gap: 6px; font-size: 12px; color: #fff; border: 0; cursor: pointer; font: inherit; font-size: 12px; }
.lp-bar { position: absolute; left: 0; right: 0; bottom: 0; z-index: 2; height: 56px; padding: 0 8px 8px; display: flex; align-items: flex-end; gap: 4px; background: linear-gradient(180deg, rgba(0, 0, 0, 0), rgba(0, 0, 0, .6)); }
.lp-bar .sp { flex: 1; }
.lp-btn { width: 32px; height: 32px; border-radius: 6px; border: 0; background: transparent; color: #fff; display: grid; place-items: center; cursor: pointer; flex: none; }
.lp-btn:hover { background: var(--lp-hover); }
.lp-btn:focus-visible, .lp-hint:focus-visible, .lp-live:focus-visible, .lp-act:focus-visible, .tr:focus-visible { outline: 2px solid var(--lp-focus); outline-offset: 2px; }
.lp-btn:disabled { opacity: .4; cursor: not-allowed; }
.lp-vol { width: 0; height: 32px; overflow: hidden; display: flex; align-items: center; flex: none; transition: width .15s; }
.lp-vol.open { width: 80px; padding: 0 8px 0 4px; }
.tr { position: relative; width: 68px; height: 4px; border-radius: 2px; background: var(--lp-track); cursor: pointer; }
.tr i { position: absolute; left: 0; top: 0; bottom: 0; border-radius: 2px; background: #fff; }
.tr b { position: absolute; top: 50%; width: 12px; height: 12px; margin: -6px 0 0 -6px; border-radius: 50%; background: #fff; }
.lp-lagbox { height: 28px; margin: 0 4px 2px 0; border-radius: 14px; background: var(--lp-scrim); display: flex; align-items: center; flex: none; }
.lp-lag { font-size: 12px; padding: 0 10px 0 12px; border-right: 1px solid var(--lp-track); }
.lp-live { height: 28px; padding: 0 12px 0 8px; border: 0; background: transparent; color: #fff; display: flex; align-items: center; gap: 6px; font-size: 12px; font-weight: 500; cursor: pointer; font: inherit; font-size: 12px; font-weight: 500; }
.lp-live:hover { background: var(--lp-hover); }
.lp-stage.idle .lp-chip, .lp-stage.idle .lp-bar > *:not(.lp-snd) { opacity: 0; transition: opacity .2s; }
.lp-stage.idle .lp-bar { background: none; }
.lp-stage.idle .lp-snd { background: var(--lp-scrim); border-radius: 6px; }
.lp-ov { position: absolute; inset: 0; z-index: 3; display: grid; place-items: center; padding: 16px; text-align: center; }
.lp-ov.dimbg { background: rgba(0, 0, 0, .55); }
.lp-ov .in { display: flex; flex-direction: column; align-items: center; gap: 8px; max-width: 360px; }
.lp-ov .ic { width: 40px; height: 40px; border-radius: 50%; background: rgba(255, 255, 255, .14); display: grid; place-items: center; }
.lp-ov .ic.warn { color: var(--lp-warn); background: rgba(251, 191, 36, .16); }
.lp-ov .lp-sub { font-size: 12px; color: var(--lp-fg-2); margin-top: -4px; }
.lp-ov p { margin: 0; font-size: 13px; font-weight: 500; line-height: 1.5; }
.lp-act { display: inline-flex; align-items: center; gap: 6px; height: 28px; margin-top: 4px; padding: 0 12px; border-radius: 6px; border: 1px solid rgba(255, 255, 255, .28); background: rgba(255, 255, 255, .14); color: #fff; font: inherit; font-size: 13px; cursor: pointer; }
.lp-spin { width: 28px; height: 28px; border-radius: 50%; border: 2.5px solid rgba(255, 255, 255, .25); border-top-color: #fff; animation: lprot .9s linear infinite; }
.lp-ov.buf .in { width: 48px; height: 48px; border-radius: 50%; background: var(--lp-scrim); display: grid; place-items: center; gap: 0; }
.lp-audio { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; color: var(--lp-fg-2); pointer-events: none; }
.lp-audio span { font-size: 12px; line-height: 18px; }
.lp-wait { position: absolute; left: 0; right: 0; top: 50%; transform: translateY(-50%); text-align: center; font-size: 13px; color: var(--lp-fg-2); margin: 0; }
.lp-esc { position: absolute; top: 20px; left: 50%; transform: translateX(-50%); z-index: 4; height: 32px; padding: 0 14px; border-radius: 16px; background: var(--lp-scrim); display: flex; align-items: center; font-size: 13px; white-space: nowrap; }
.lp-esc kbd { font-family: var(--ff-font-mono); font-size: 12px; border: 1px solid rgba(255, 255, 255, .4); border-radius: 4px; padding: 0 4px; margin: 0 4px; }
.is-full { position: fixed; inset: 0; z-index: 40; }
.is-full .lp-chip { left: 20px; top: 20px; }
.is-full .lp-bar { height: 72px; padding: 0 16px 16px; gap: 8px; }
.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
@keyframes lprot { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .lp-spin, .lp-vol { animation: none; transition: none; } }
</style>
