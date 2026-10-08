<template>
  <div
    ref="root"
    class="lp-stage"
    :class="{ dim: dimmed, idle: idleOn, 'is-full': fullOn }"
    :style="{ '--ar': String(aspect) }"
    role="region"
    :aria-roledescription="'直播播放器'"
    :aria-label="kind === 'pull' ? '拉流画面' : '推流预览'"
    aria-keyshortcuts="Space M F"
    tabindex="0"
    @mousemove="wake"
    @keydown="onKey"
    @dblclick="toggleFull"
  >
    <div v-if="showVideo" class="lp-video" :class="fake ? 'f' + fake : ''">
      <video v-show="!fake" ref="videoEl" autoplay playsinline :muted="muted" />
    </div>
    <p v-if="phase === 'empty'" class="lp-wait">{{ emptyText || LP_EMPTY }}</p>

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
          <button v-if="phase === 'interrupted'" type="button" class="lp-act" @click="retry"><FIcon name="retry" :size="14" />{{ kind === 'pull' ? LP_RETRY_PULL : LP_RETRY_PUSH }}</button>
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
      <div v-if="lag != null" class="lp-lagbox">
        <span class="lp-lag" aria-hidden="true">{{ LP_LAG(lag) }}</span>
        <button type="button" class="lp-live" :aria-label="`回到最新画面，当前落后约 ${lag} 秒`" @click="emit('catchup')"><FIcon name="refresh" :size="14" />{{ LP_CATCHUP }}</button>
      </div>
      <button type="button" class="lp-btn" :aria-label="fullOn ? '退出全屏' : '全屏'" :title="fullOn ? '退出全屏（F 或 Esc）' : '全屏（F）'" @click="toggleFull">
        <FIcon :name="fullOn ? 'zip' : 'full'" :size="18" />
      </button>
    </div>
    <div class="sr" aria-live="polite">{{ liveText }}</div>
  </div>
</template>

<script setup lang="ts">
// 直播播放器（设计说明 v0.1 §二–§六）。不持有业务：地址由父组件给出，mpegts.js 只负责播。
// 低延迟：enableStashBuffer 关、追帧上限 1.5 秒（契约 6.10.3.8）。默认静音。直播不能暂停、不能拖。
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import mpegts from 'mpegts.js'
import FIcon from '@/components/icon/FIcon.vue'
import {
  LP_BREAK_PULL, LP_BREAK_PUSH, LP_CATCHUP, LP_CONNECTING, LP_EMPTY, LP_END_PULL, LP_END_PUSH, LP_LAG, LP_MUTED_HINT, LP_RETRY_PULL, LP_RETRY_PUSH, LP_UNAVAILABLE, LP_UNSUP_PULL, LP_UNSUP_PUSH,
} from '@/errors/livePreviewMessages'

const props = withDefaults(defineProps<{
  kind: 'push' | 'pull'
  phase: 'empty' | 'connecting' | 'playing' | 'buffering' | 'unsupported' | 'ended' | 'interrupted'
  /** 播放地址；空 = 不建播放器（截图用模拟画面） */
  url?: string
  mime?: string
  hasAudio?: boolean
  clock?: string
  aspect?: number
  /** 截图：模拟画面 a/b/c */
  fake?: '' | 'a' | 'b' | 'c'
  /** 截图：强制控件淡出 / 音量展开 / 已静音提示 / 全屏 */
  forceIdle?: boolean
  forceVol?: boolean
  forceHint?: boolean
  forceFull?: boolean
  lag?: number | null
  /** unsupported 的原因：unavailable 用推流那句，codec 按推流 / 拉流分 */
  reason?: '' | 'codec' | 'unavailable'
  lowLatency?: boolean
  /** phase=empty 时的文字（默认“还没有进行中的预览”） */
  emptyText?: string
}>(), { url: '', mime: 'video/x-flv', hasAudio: true, clock: '00:00:00', aspect: 16 / 9, fake: '', lag: null, reason: '', lowLatency: true })

const emit = defineEmits<{ restart: []; catchup: []; 'media-unsupported': []; 'media-ended': []; 'media-broken': []; playing: []; stats: [s: { kbps: number; fps: number; dropped: number; bytes: number }] }>()
const muted = defineModel<boolean>('muted', { default: true })
const volume = ref(70)
const root = ref<HTMLElement | null>(null)
const videoEl = ref<HTMLVideoElement | null>(null)
const idle = ref(false)
const volHover = ref(false)
const hintOn = ref(false)
const hinted = ref(false)
const full = ref(false)
const liveText = ref('')
let player: mpegts.Player | null = null
let idleTimer: ReturnType<typeof setTimeout> | undefined
let hintTimer: ReturnType<typeof setTimeout> | undefined
let volTimer: ReturnType<typeof setTimeout> | undefined
let bufTimer: ReturnType<typeof setTimeout> | undefined
const buffering = ref(false)

const fullOn = computed(() => props.forceFull || full.value)
const idleOn = computed(() => props.forceIdle || idle.value)
const volOpen = computed(() => props.forceVol || volHover.value)
const showVideo = computed(() => props.phase === 'playing' || props.phase === 'buffering' || props.phase === 'ended' || props.phase === 'interrupted')
const dimmed = computed(() => props.phase === 'ended' || props.phase === 'interrupted')
const showChip = computed(() => props.phase === 'playing' || props.phase === 'buffering' || (props.phase === 'unsupported' && props.kind === 'push'))
const showBar = computed(() => props.phase === 'playing' || props.phase === 'buffering')
const showHint = computed(() => showBar.value && (props.forceHint || hintOn.value))
const showEsc = ref(true)
const overlay = computed(() => props.phase === 'connecting' || props.phase === 'buffering' || props.phase === 'unsupported' || props.phase === 'ended' || props.phase === 'interrupted')
const overlayText = computed(() => {
  if (props.phase === 'ended') return props.kind === 'pull' ? LP_END_PULL : LP_END_PUSH
  if (props.phase === 'interrupted') return props.kind === 'pull' ? LP_BREAK_PULL : LP_BREAK_PUSH
  if (props.reason === 'unavailable') return LP_UNAVAILABLE
  return props.kind === 'pull' ? LP_UNSUP_PULL : LP_UNSUP_PUSH
})

watch(() => props.phase, (p) => {
  if (p === 'playing' && muted.value && !hinted.value && !props.forceIdle) {
    hintOn.value = true
    hinted.value = true
    liveText.value = props.kind === 'pull' ? '拉流已开始，已静音' : '推流预览已开始，已静音'
    clearTimeout(hintTimer)
    hintTimer = setTimeout(() => (hintOn.value = false), 5000)
  }
  if (p === 'connecting') liveText.value = LP_CONNECTING
  if (p !== 'playing' && p !== 'buffering') hintOn.value = false
}, { immediate: true })

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
  else if (e.key === 'f' || e.key === 'F') { e.preventDefault(); toggleFull() }
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
let accBytes = 0
function stopStats() { if (statTimer) clearInterval(statTimer); statTimer = null }
function destroyPlayer() {
  clearTimeout(bufTimer)
  stopStats()
  if (player) {
    try { player.pause(); player.unload(); player.detachMediaElement(); player.destroy() } catch { /* 已销毁 */ }
    player = null
  }
}
function attach(url: string) {
  destroyPlayer()
  const el = videoEl.value
  if (!el || props.fake) return
  if (!mpegts.getFeatureList().mseLivePlayback) { emit('media-unsupported'); return }
  const chase = props.lowLatency
  player = mpegts.createPlayer(
    { type: /mp2t|mpegts/i.test(props.mime) ? 'mpegts' : 'flv', isLive: true, url, hasAudio: props.hasAudio, hasVideo: true },
    chase
      ? { enableWorker: false, enableStashBuffer: false, stashInitialSize: 128, liveBufferLatencyChasing: true, liveBufferLatencyMaxLatency: 1.5, liveBufferLatencyMinRemain: 0.3, autoCleanupSourceBuffer: true }
      : { enableWorker: false, enableStashBuffer: true, autoCleanupSourceBuffer: true },
  )
  player.on(mpegts.Events.ERROR, (_t: string, detail: string) => {
    if (detail === mpegts.ErrorDetails.MEDIA_CODEC_UNSUPPORTED || detail === mpegts.ErrorDetails.MEDIA_FORMAT_UNSUPPORTED) emit('media-unsupported')
    else if (props.phase === 'playing' || props.phase === 'buffering') emit('media-broken')
  })
  player.on(mpegts.Events.LOADING_COMPLETE, () => emit('media-ended'))
  el.addEventListener('playing', () => emit('playing'), { once: true })
  el.addEventListener('waiting', () => {
    clearTimeout(bufTimer)
    bufTimer = setTimeout(() => { if (props.phase === 'playing') buffering.value = true }, 500)
  })
  el.addEventListener('playing', () => { clearTimeout(bufTimer); buffering.value = false })
  player.attachMediaElement(el)
  el.muted = muted.value
  player.load()
  lastDecoded = 0
  accBytes = 0
  stopStats()
  statTimer = setInterval(() => {
    const si = (player as { statisticsInfo?: { decodedFrames?: number; speed?: number; droppedFrames?: number } } | null)?.statisticsInfo
    if (!si) return
    const decoded = si.decodedFrames ?? 0
    const fps = lastDecoded ? Math.max(0, decoded - lastDecoded) : 0
    lastDecoded = decoded
    accBytes += (si.speed ?? 0) * 1024
    emit('stats', { kbps: (si.speed ?? 0) * 8, fps, dropped: si.droppedFrames ?? 0, bytes: accBytes })
  }, 1000)
  const p = player.play()
  if (p && typeof (p as Promise<void>).catch === 'function') (p as Promise<void>).catch(() => { muted.value = true; hintOn.value = true; void el.play().catch(() => undefined) })
}
watch(() => [props.url, props.phase] as const, ([url, phase]) => {
  if (url && (phase === 'connecting' || phase === 'playing' || phase === 'buffering') && !props.fake) attach(url)
  else destroyPlayer()
})
watch(muted, (m) => { if (videoEl.value) videoEl.value.muted = m })
watch(volume, (v) => { if (videoEl.value) videoEl.value.volume = v / 100 })

onBeforeUnmount(() => { destroyPlayer(); document.removeEventListener('fullscreenchange', onFs); clearTimeout(idleTimer); clearTimeout(hintTimer); clearTimeout(volTimer) })
</script>

<style scoped>
.lp-stage { --lp-scrim: rgba(0, 0, 0, .6); --lp-fg: #fff; --lp-fg-2: rgba(255, 255, 255, .78); --lp-hover: rgba(255, 255, 255, .16); --lp-track: rgba(255, 255, 255, .32); --lp-live: #ff4d4f; --lp-play: #22c55e; --lp-warn: #fbbf24; --lp-focus: #7aa2ff; position: relative; overflow: hidden; background: #000; container-type: size; display: grid; grid-template: minmax(0, 1fr) / minmax(0, 1fr); place-items: center; color: #fff; outline: none; flex: 1; min-height: 0; width: 100%; }
.lp-stage:focus-visible { box-shadow: inset 0 0 0 2px var(--lp-focus); }
.lp-video { width: min(100cqw, calc(100cqh * var(--ar))); aspect-ratio: var(--ar); position: relative; overflow: hidden; background: #000; }
.lp-video video { width: 100%; height: 100%; object-fit: contain; background: #000; }
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
.lp-ov p { margin: 0; font-size: 13px; font-weight: 500; line-height: 1.5; }
.lp-act { display: inline-flex; align-items: center; gap: 6px; height: 28px; margin-top: 4px; padding: 0 12px; border-radius: 6px; border: 1px solid rgba(255, 255, 255, .28); background: rgba(255, 255, 255, .14); color: #fff; font: inherit; font-size: 13px; cursor: pointer; }
.lp-spin { width: 28px; height: 28px; border-radius: 50%; border: 2.5px solid rgba(255, 255, 255, .25); border-top-color: #fff; animation: lprot .9s linear infinite; }
.lp-ov.buf .in { width: 48px; height: 48px; border-radius: 50%; background: var(--lp-scrim); display: grid; place-items: center; gap: 0; }
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
