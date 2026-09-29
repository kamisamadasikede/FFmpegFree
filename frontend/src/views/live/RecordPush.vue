<template>
  <LiveTabFrame>
    <template #main>
      <PlayerShell
        v-model:muted="pushMuted"
        fill
        mode="status"
        :chip="`麦克风 ${mic ? '开' : '关'}`"
        :status-text="session.running.value ? '正在推流' : session.busy.value ? '正在连接' : '未开始推流'"
        :status-hint="statusHint"
        @chip="toggleMic"
        @fullscreen="fullscreen"
      >
        <video v-show="hasVideo" ref="videoRef" autoplay muted playsinline />
        <LiveMockFrame v-if="preview && session.phase.value !== 'idle'" variant="screen" />
        <LiveMockFrame v-else-if="!hasVideo" variant="idle" icon="monitor" hint="点击“开始推流”后选择要共享的画面，这里会显示预览" />
        <template #overlay>
          <LiveOverlays :session="session" :hud-lines="hudLines" @retry="start" @view-log="logOpen = true" />
        </template>
      </PlayerShell>
      <LiveStatCards :stats="session.stats" :series="session.series" />
    </template>

    <template #panel>
      <LivePanel title="推流设置" note="不占用转换队列">
        <LiveField label="画面来源"><LiveSourcePicker v-model="source" :disabled="session.busy.value" /></LiveField>
        <LiveField label="推流地址">
          <LiveInput v-model="baseUrl" :bad="urlInvalid" :disabled="session.busy.value" placeholder="rtmp://live.example.com/live" copyable @enter="start" />
          <InlineError v-if="urlInvalid" code="LIVE_URL_INVALID" />
        </LiveField>
        <LiveField label="推流码">
          <LiveInput v-model="streamKey" secret :bad="keyBad" :disabled="session.busy.value" placeholder="留空则使用地址本身" />
          <InlineError v-if="keyBad" :code="session.errorCode.value" />
        </LiveField>
        <div class="two">
          <LiveField label="分辨率">
            <el-select v-model="resolution" :disabled="session.busy.value" class="sel">
              <el-option v-for="r in resolutions" :key="r.value" :label="r.label" :value="r.value" />
            </el-select>
          </LiveField>
          <LiveField label="帧率">
            <el-select v-model="fps" :disabled="session.busy.value" class="sel">
              <el-option v-for="f in [15, 24, 30, 60]" :key="f" :label="`${f} fps`" :value="f" />
            </el-select>
          </LiveField>
        </div>
        <LiveField label="视频码率"><LiveSlider v-model="bitrate" :min="1000" :max="10000" :step="500" unit="k" :disabled="session.busy.value" /></LiveField>
        <div class="chk">摄像头画中画<el-switch v-model="pip" size="small" :disabled="session.busy.value || source === 'camera'" /></div>
        <div class="chk">断线自动重连<el-switch v-model="autoReconnect" size="small" /></div>
        <LiveAdvanced v-model:archive-enabled="archiveEnabled" v-model:segment-seconds="segmentSeconds" v-model:relay-text="relayText" />
        <template #action>
          <LiveButton v-if="session.busy.value" variant="danger" lg icon="x" @click="stop">停止推流</LiveButton>
          <LiveButton v-else variant="pri" lg icon="rec" @click="start">开始推流</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
  <LiveLogDialog v-model="logOpen" :lines="session.logs.value" />
</template>

<script setup lang="ts">
// 录屏推流：浏览器采集（getDisplayMedia / getUserMedia）+ MediaRecorder，经 @/api/live 的 WebSocket 交给后端 ffmpeg 推流。
// 契约 v0.4 规定 macOS / Linux 默认改由后端 ffmpeg 直接采集（captureMode = native），那部分要等 LiveService 才有，这里只做 webview 模式。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import PlayerShell from '@/components/common/PlayerShell.vue'
import InlineError from '@/components/common/InlineError.vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LiveSlider from '@/components/live/LiveSlider.vue'
import LiveSourcePicker, { type CaptureSource } from '@/components/live/LiveSourcePicker.vue'
import LiveAdvanced from '@/components/live/LiveAdvanced.vue'
import LiveStatCards from '@/components/live/LiveStatCards.vue'
import LiveMockFrame from '@/components/live/LiveMockFrame.vue'
import LiveOverlays from '@/components/live/LiveOverlays.vue'
import LiveLogDialog from '@/components/live/LiveLogDialog.vue'
import { livePreview, useLiveSession } from '@/composables/useLiveSession'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { isValidStreamUrl } from '@/errors/playerError'
import { joinPushUrl, parseTargets } from '@/utils/liveUrl'
import * as liveApi from '@/api/live'

defineOptions({ name: 'LiveRecordPush' })

const session = useLiveSession('record')
const ffmpeg = useFFmpegStore()
const preview = !!livePreview

const source = ref<CaptureSource>('screen')
const baseUrl = ref(livePreview === 'invalid' ? 'http:/live.example' : livePreview ? 'rtmp://live-push.example.com/live' : '')
const streamKey = ref(livePreview ? '••••••••••••••••' : '')
const resolutions = [
  { label: '720p', value: 720 },
  { label: '1080p', value: 1080 },
]
const resolution = ref(1080)
const fps = ref(30)
const bitrate = ref(6000)
const pip = ref(livePreview ? true : false)
const autoReconnect = ref(true)
const mic = ref(livePreview ? true : false)
const archiveEnabled = ref(false)
const segmentSeconds = ref(300)
const relayText = ref('')
// 控制条上的喇叭：静音推出去的声音（关掉音轨），本地预览画面始终静音避免回声
const pushMuted = ref(false)
const logOpen = ref(false)
const urlInvalid = ref(livePreview === 'invalid')
const videoRef = ref<HTMLVideoElement | null>(null)
const hasVideo = ref(false)

let capture: MediaStream | null = null
let cleanupCapture: (() => void) | null = null
let handle: liveApi.RecordHandle | null = null
let fullUrl = ''
let stopStats: (() => void) | null = null
let stopEvents: (() => void) | null = null
let userStopped = false
let reconnects = 0
const MAX_RECONNECT = 5

const keyBad = computed(() => session.phase.value === 'error' && session.errorCode.value === 'LIVE_PUSH_REJECTED')
const statusHint = computed(() => {
  const names = { screen: preview ? '主显示器' : '屏幕', camera: '摄像头', window: '窗口' }
  return names[source.value] + (pip.value && source.value !== 'camera' ? ' · 摄像头画中画' : '')
})
const hudLines = computed(() => {
  const s = session.stats
  const res = s.width ? `${s.width}×${s.height} · ` : ''
  return [`${res}${Math.round(s.fps)} fps`, `${Math.round(s.bitrateKbps)} kbps · 丢帧 ${s.dropped}`]
})

watch([baseUrl, streamKey], () => (urlInvalid.value = false))
watch(pushMuted, applyMute)
watch(source, (v) => {
  if (v === 'camera') pip.value = false
})

function applyMute() {
  capture?.getAudioTracks().forEach((t) => (t.enabled = !pushMuted.value))
}

function toggleMic() {
  if (session.busy.value) {
    ElMessage.info('推流中不能切换麦克风，停止后再改')
    return
  }
  mic.value = !mic.value
}

// ───── 采集 ─────
async function buildStream(): Promise<MediaStream> {
  const size = { width: { ideal: Math.round((resolution.value * 16) / 9) }, height: { ideal: resolution.value }, frameRate: { ideal: fps.value } }
  const tracks: MediaStreamTrack[] = []
  const closers: (() => void)[] = []
  const grab = (s: MediaStream) => {
    closers.push(() => s.getTracks().forEach((t) => t.stop()))
    return s
  }
  let video: MediaStream
  let systemAudio: MediaStream | null = null
  if (source.value === 'camera') {
    video = grab(await navigator.mediaDevices.getUserMedia({ video: size, audio: false }))
  } else {
    const display = grab(
      await navigator.mediaDevices.getDisplayMedia({
        video: { ...size, displaySurface: source.value === 'window' ? 'window' : 'monitor' } as MediaTrackConstraints,
        audio: true,
      }),
    )
    video = new MediaStream(display.getVideoTracks())
    if (display.getAudioTracks().length) systemAudio = new MediaStream(display.getAudioTracks())
  }
  let out = video
  if (pip.value && source.value !== 'camera') {
    const cam = grab(await navigator.mediaDevices.getUserMedia({ video: { width: 320, height: 320 }, audio: false }))
    const composed = composePip(video, cam, fps.value)
    closers.push(composed.stop)
    out = composed.stream
  }
  tracks.push(...out.getVideoTracks())
  // 系统声音和麦克风混成一条音轨：MediaRecorder 不接受多条音轨
  const audioSources: MediaStream[] = []
  if (systemAudio) audioSources.push(systemAudio)
  if (mic.value) audioSources.push(grab(await navigator.mediaDevices.getUserMedia({ audio: true, video: false })))
  if (audioSources.length) {
    const ctx = new AudioContext()
    const dest = ctx.createMediaStreamDestination()
    audioSources.forEach((s) => ctx.createMediaStreamSource(s).connect(dest))
    closers.push(() => void ctx.close())
    tracks.push(...dest.stream.getAudioTracks())
  }
  cleanupCapture = () => closers.forEach((c) => c())
  return new MediaStream(tracks)
}

/** 把摄像头画面画在屏幕画面右下角（圆形），输出 canvas 的流 */
function composePip(screen: MediaStream, cam: MediaStream, rate: number) {
  const sv = document.createElement('video')
  const cv = document.createElement('video')
  for (const [v, s] of [[sv, screen], [cv, cam]] as const) {
    v.srcObject = s
    v.muted = true
    v.playsInline = true
    void v.play()
  }
  const canvas = document.createElement('canvas')
  const g = canvas.getContext('2d')!
  const timer = setInterval(() => {
    const w = sv.videoWidth
    const h = sv.videoHeight
    if (!w || !h) return
    if (canvas.width !== w) {
      canvas.width = w
      canvas.height = h
    }
    g.drawImage(sv, 0, 0, w, h)
    const d = Math.round(h * 0.22)
    const x = w - d - Math.round(w * 0.03)
    const y = h - d - Math.round(h * 0.05)
    g.save()
    g.beginPath()
    g.arc(x + d / 2, y + d / 2, d / 2, 0, Math.PI * 2)
    g.clip()
    const side = Math.min(cv.videoWidth, cv.videoHeight) || 1
    g.drawImage(cv, (cv.videoWidth - side) / 2, (cv.videoHeight - side) / 2, side, side, x, y, d, d)
    g.restore()
  }, 1000 / rate)
  return {
    stream: canvas.captureStream(rate),
    stop: () => {
      clearInterval(timer)
      sv.srcObject = cv.srcObject = null
    },
  }
}

function releaseCapture() {
  cleanupCapture?.()
  cleanupCapture = null
  capture?.getTracks().forEach((t) => t.stop())
  capture = null
  hasVideo.value = false
  if (videoRef.value) videoRef.value.srcObject = null
}

function cleanupAll() {
  stopStats?.()
  stopEvents?.()
  stopStats = stopEvents = null
  handle?.stop()
  handle = null
  releaseCapture()
}

// ───── 开始 / 停止 ─────
async function start(isReconnect: unknown = false) {
  const reconnecting = isReconnect === true
  if (session.busy.value && !reconnecting) return
  if (ffmpeg.needsAttention) return session.fail('FFMPEG_NOT_FOUND')
  fullUrl = joinPushUrl(baseUrl.value, streamKey.value)
  if (!isValidStreamUrl(fullUrl)) {
    urlInvalid.value = true
    session.log('推流地址格式不正确')
    return
  }
  userStopped = false
  if (!reconnecting) reconnects = 0
  session.setStarting()
  session.log(`开始录屏推流（${statusHint.value}，${resolution.value}p ${fps.value}fps）`)
  if (preview) {
    session.simRunning()
    return
  }
  try {
    releaseCapture()
    capture = await buildStream()
  } catch (e) {
    releaseCapture()
    const err = e as DOMException
    if (err?.name === 'NotAllowedError' || err?.name === 'SecurityError') {
      // 用户在选择窗口里点了取消：不算错误；被系统拒绝才提示去开权限
      if (/by system/i.test(err.message)) session.fail('SCREEN_PERMISSION_DENIED')
      else {
        session.setIdle()
        session.log('已取消选择画面')
      }
    } else {
      session.fail('SCREEN_PERMISSION_DENIED', '', err?.message)
    }
    return
  }
  hasVideo.value = true
  applyMute()
  if (videoRef.value) videoRef.value.srcObject = capture
  // 用户点浏览器自带的“停止共享”时，当作用户主动停止
  capture.getVideoTracks()[0]?.addEventListener('ended', () => session.busy.value && stop())
  const opts: liveApi.RecordPushOptions = {
    url: fullUrl,
    archiveEnabled: archiveEnabled.value,
    segmentSeconds: segmentSeconds.value,
    relayTargets: parseTargets(relayText.value),
    videoBitsPerSecond: bitrate.value * 1000,
  }
  handle = liveApi.startRecordPush(capture, opts, {
    onStarted: () => {
      session.setRunning()
      const track = capture?.getVideoTracks()[0]?.getSettings()
      session.stats.width = track?.width ?? 0
      session.stats.height = track?.height ?? 0
    },
    onClosed: (err) => {
      handle = null
      if (userStopped || !session.busy.value) return
      onFailed(err?.message, err ? 'LIVE_CONNECT_FAILED' : undefined)
    },
  })
  stopStats = liveApi.subscribeStats({ source: 'screen' }, (s) => {
    if (!s || !session.busy.value) return
    session.addSample({ bitrateKbps: s.bitrateKbps, fps: s.fps, dropped: s.droppedFrames })
    reconnects = 0
  })
  stopEvents = liveApi.onLiveEvent((e) => {
    if (e.streamUrl !== fullUrl) return
    session.log(`${e.status}${e.error ? ' · ' + e.error : ''}`)
    if (e.status === 'failed' && !userStopped) onFailed(e.error)
  })
}

function onFailed(message?: string, code?: string) {
  cleanupAll()
  if (!userStopped && autoReconnect.value && reconnects < MAX_RECONNECT) {
    reconnects++
    session.log(`断线，3 秒后自动重连（${reconnects}/${MAX_RECONNECT}）`)
    setTimeout(() => !userStopped && start(true), 3000)
    return
  }
  // v1 后端不区分失败原因：还没出过数据算“连不上”，出过数据算“中断”
  session.fail(code ?? (session.stats.bitrateKbps === 0 ? 'LIVE_CONNECT_FAILED' : 'LIVE_PUSH_INTERRUPTED'), '', message ?? '')
}

function stop() {
  userStopped = true
  cleanupAll()
  session.setIdle()
  session.log('已停止推流')
}

function fullscreen() {
  videoRef.value?.requestFullscreen?.().catch(() => undefined)
}

onMounted(() => {
  if (preview) session.initPreview()
})
onBeforeUnmount(() => {
  userStopped = true
  cleanupAll()
})
</script>

<style scoped>
.two {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.sel {
  width: 100%;
}
.chk {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
}
</style>
