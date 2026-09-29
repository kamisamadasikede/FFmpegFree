<template>
  <LiveTabFrame>
    <template #main>
      <PlayerShell
        v-model:muted="muted"
        fill
        mode="status"
        status-icon="play"
        :status-text="statusText"
        :status-hint="statusHint"
        @fullscreen="fullscreen"
      >
        <video v-show="hasVideo" ref="videoRef" autoplay playsinline :muted="muted" />
        <LiveMockFrame v-if="preview && session.phase.value !== 'idle'" variant="scene" />
        <LiveMockFrame v-else-if="!hasVideo" variant="idle" icon="play" hint="输入 HTTP-FLV / WS-FLV 地址后点击“开始播放”" />
        <template #overlay>
          <LiveOverlays
            :session="session"
            :hud-label="reconnecting ? '重连中' : '播放中'"
            :hud-lines="hudLines"
            @retry="start"
            @view-log="logOpen = true"
          />
        </template>
      </PlayerShell>
      <LiveStatCards :stats="session.stats" :series="session.series" bytes-label="已接收" />
    </template>

    <template #panel>
      <LivePanel title="拉流设置" note="不经过本地服务">
        <LiveField label="流地址">
          <LiveInput v-model="url" :bad="urlInvalid" :disabled="session.busy.value" placeholder="http://live.example.com/live/room.flv" copyable @enter="start" />
          <InlineError v-if="urlInvalid" code="LIVE_URL_INVALID" description="请输入 http:// 或 ws:// 开头的流地址" />
        </LiveField>
        <div class="tip">支持 HTTP-FLV（http:// 或 https://）和 WS-FLV（ws:// 或 wss://）。</div>
        <div class="chk">低延迟追帧<el-switch v-model="lowLatency" size="small" aria-label="低延迟追帧" :disabled="session.busy.value" /></div>
        <div class="chk">断线自动重连<el-switch v-model="autoReconnect" size="small" aria-label="断线自动重连" /></div>
        <template #action>
          <LiveButton v-if="session.busy.value" variant="danger" lg icon="x" @click="stop">停止播放</LiveButton>
          <LiveButton v-else variant="pri" lg icon="play" @click="start">开始播放</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
  <LiveLogDialog v-model="logOpen" :lines="session.logs.value" />
</template>

<script setup lang="ts">
// 拉流播放：mpegts.js 直接播放远端 FLV 地址，不经过本地服务，也不调用任何后端（契约里的 LiveService.GetPlayURL 不再使用，PRD v0.3）。
// 播放错误统一走 mapPlayerError → 错误码 → ErrorOverlay。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import mpegts from 'mpegts.js'
import PlayerShell from '@/components/common/PlayerShell.vue'
import InlineError from '@/components/common/InlineError.vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LiveStatCards from '@/components/live/LiveStatCards.vue'
import LiveMockFrame from '@/components/live/LiveMockFrame.vue'
import LiveOverlays from '@/components/live/LiveOverlays.vue'
import LiveLogDialog from '@/components/live/LiveLogDialog.vue'
import { livePreview, useLiveSession } from '@/composables/useLiveSession'
import { isPersistentPlayerError, isValidPullUrl, mapPlayerError } from '@/errors/playerError'

defineOptions({ name: 'LivePullPlay' })

const session = useLiveSession('pull')
const preview = !!livePreview

const url = ref(livePreview === 'invalid' ? 'http:/live.example' : livePreview ? 'http://live.example.com/live/room.flv' : '')
const lowLatency = ref(true)
const autoReconnect = ref(true)
const muted = ref(false)
const logOpen = ref(false)
const urlInvalid = ref(livePreview === 'invalid')
const videoRef = ref<HTMLVideoElement | null>(null)
const hasVideo = ref(false)

let player: mpegts.Player | null = null
let statTimer: ReturnType<typeof setInterval> | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let userStopped = false
// 自动重连次数；开始播放（starting）和出画面（playing）时都归零
let reconnects = 0
const reconnectCount = ref(0)
let lastDecoded = 0
const MAX_RECONNECT = 5

// 自动重连等待期间 reconnecting 为 true，状态栏显示“正在重连 n/5”，不再显示“正在播放”
const reconnecting = ref(false)
const statusText = computed(() =>
  reconnecting.value ? `正在重连 ${reconnectCount.value}/${MAX_RECONNECT}` : session.running.value ? '正在播放' : session.busy.value ? '正在连接' : '未开始播放',
)
const statusHint = computed(() => (url.value && session.busy.value ? url.value : '拉流播放'))
const hudLines = computed(() => {
  const s = session.stats
  const res = s.width ? `${s.width}×${s.height} · ` : ''
  return [`${res}${Math.round(s.fps)} fps`, `${Math.round(s.bitrateKbps)} kbps · 丢帧 ${s.dropped}`]
})

watch(url, () => (urlInvalid.value = false))
watch(muted, (m) => {
  if (videoRef.value) videoRef.value.muted = m
})

function clearReconnectTimer() {
  if (reconnectTimer) clearTimeout(reconnectTimer)
  reconnectTimer = null
}

function resetReconnects() {
  reconnects = 0
  reconnectCount.value = 0
  reconnecting.value = false
}

function destroyPlayer() {
  if (statTimer) clearInterval(statTimer)
  statTimer = null
  if (player) {
    try {
      player.pause()
      player.unload()
      player.detachMediaElement()
      player.destroy()
    } catch {
      /* 已销毁 */
    }
    player = null
  }
  hasVideo.value = false
}

function start() {
  if (session.busy.value && !player) return
  const u = url.value.trim()
  if (!isValidPullUrl(u)) {
    urlInvalid.value = true
    session.log('流地址格式不正确')
    return
  }
  clearReconnectTimer()
  userStopped = false
  resetReconnects()
  session.setStarting()
  session.log(`开始拉流 ${u}`)
  if (preview) {
    session.simRunning()
    return
  }
  open(u)
}

function open(u: string) {
  clearReconnectTimer()
  destroyPlayer()
  const el = videoRef.value
  if (!el) return
  if (!mpegts.getFeatureList().mseLivePlayback) {
    session.fail('LIVE_PLAY_FAILED', '当前环境不支持 FLV 直播播放')
    return
  }
  const isWs = /^wss?:/i.test(u)
  player = mpegts.createPlayer(
    { type: 'flv', isLive: true, url: u },
    lowLatency.value
      ? {
          enableWorker: true,
          enableStashBuffer: false,
          stashInitialSize: 128,
          lazyLoad: true,
          lazyLoadMaxDuration: 3,
          autoCleanupSourceBuffer: true,
          liveBufferLatencyChasing: true,
          liveSync: true,
          liveSyncTargetLatency: 1,
        }
      : { enableWorker: true, autoCleanupSourceBuffer: true },
  )
  player.on(mpegts.Events.ERROR, (type: string, detail: string, info: { code?: number; msg?: string }) => {
    if (userStopped) return
    // mpegts 的 detail 可能是 Exception / HttpStatusCodeInvalid / ConnectingTimeout 等
    const code = mapPlayerError({ kind: 'mpegts', type, detail, info, url: u })
    session.log(`播放器错误 ${type} / ${detail}${info?.code !== undefined ? ` / code=${info.code}` : ''}${info?.msg ? ` / ${info.msg}` : ''}`)
    const status = info?.code && info.code > 0 ? info.code : undefined
    onFailed(code, status ? `服务器返回 ${status}` : isWs ? 'WebSocket' : '', status)
  })
  player.on(mpegts.Events.MEDIA_INFO, (mi: { width?: number; height?: number }) => {
    session.stats.width = mi.width ?? 0
    session.stats.height = mi.height ?? 0
    session.log(`已收到媒体信息 ${mi.width}×${mi.height}`)
  })
  player.attachMediaElement(el)
  el.muted = muted.value
  hasVideo.value = true
  player.load()
  const p = player.play()
  if (p && typeof (p as Promise<void>).catch === 'function') (p as Promise<void>).catch(() => undefined)
  lastDecoded = 0
  // 播放器出画面后才算“播放中”
  const onPlaying = () => {
    el.removeEventListener('playing', onPlaying)
    // 出画面即视为恢复：无论首次播放还是重连成功，重连计数都归零
    resetReconnects()
    if (session.phase.value === 'starting') session.setRunning()
  }
  el.addEventListener('playing', onPlaying)
  statTimer = setInterval(sample, 1000)
}

function sample() {
  if (!player) return
  const si = (player as mpegts.MSEPlayer).statisticsInfo
  const decoded = si?.decodedFrames ?? 0
  const kbps = ((si?.speed ?? 0) * 8) // KB/s → kbit/s
  const fps = lastDecoded ? Math.max(0, decoded - lastDecoded) : 0
  lastDecoded = decoded
  if (session.phase.value !== 'running') return
  session.addSample({
    bitrateKbps: kbps,
    fps,
    dropped: si?.droppedFrames ?? 0,
    // 已接收字节数按每秒速度累加
    bytes: session.stats.bytes + (si?.speed ?? 0) * 1024,
  })
}

function onFailed(code: string, detail = '', httpStatus?: number) {
  destroyPlayer()
  clearReconnectTimer()
  if (code === 'LIVE_URL_INVALID') {
    resetReconnects()
    session.setIdle()
    urlInvalid.value = true
    return
  }
  // 跨域 / 连接失败 / 4xx 这类持久性错误重连也没用：不重连，直接出错误遮罩
  const persistent = isPersistentPlayerError(code, httpStatus)
  if (!userStopped && autoReconnect.value && !persistent && reconnects < MAX_RECONNECT) {
    reconnects++
    reconnectCount.value = reconnects
    reconnecting.value = true
    session.log(`断线，3 秒后自动重连（${reconnects}/${MAX_RECONNECT}）`)
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      if (!userStopped && session.busy.value) open(url.value.trim())
    }, 3000)
    return
  }
  reconnecting.value = false
  session.fail(code, detail)
}

function stop() {
  userStopped = true
  clearReconnectTimer()
  resetReconnects()
  destroyPlayer()
  session.setIdle()
  session.log('已停止播放')
}

function fullscreen() {
  videoRef.value?.requestFullscreen?.().catch(() => undefined)
}

onMounted(() => {
  if (preview) session.initPreview()
})
onBeforeUnmount(() => {
  userStopped = true
  clearReconnectTimer()
  destroyPlayer()
})
</script>

<style scoped>
.tip {
  font-size: 12px;
  color: var(--ff-text-3);
  line-height: 1.5;
  margin-top: -6px;
}
.chk {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
}
</style>
