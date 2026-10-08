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
        <template v-if="session.busy.value" #status>{{ statusText }} <span class="pvr" :title="PREVIEW_ROW_TITLE">{{ sessionPreviewOn ? PREVIEW_ROW_ON : PREVIEW_ROW_OFF }}</span></template>
        <!-- 预览开：舞台显示预览帧（设计稿：播放器舞台就是预览区）；video 仍在播放（声音 / 播放统计），只是不显示。预览关：舞台显示真实播放画面 -->
        <video v-show="hasVideo && !previewShown" ref="videoRef" autoplay playsinline :muted="muted" />
        <PreviewStage v-if="previewShown" :state="stageState" :data="snap.data" :alt="stageAlt" kind="pull" @retry="retryPreview" />
        <div v-else-if="!hasVideo" class="idle"><FIcon name="play" :size="28" /><span>输入 HTTP-FLV / WS-FLV 地址后点击“开始播放”</span></div>
        <template #overlay>
          <LiveOverlays
            :session="session"
            hud-label="播放中"
            :hud-lines="hudLines"
            @retry="start"
            @view-log="logOpen = true"
          />
        </template>
      </PlayerShell>
      <LiveStatCards :stats="session.stats" bytes-label="已接收" />
    </template>

    <template #panel>
      <LivePanel title="拉流设置" note="不经过本地服务">
        <LiveField label="流地址">
          <LiveInput v-model="url" :bad="urlInvalid" :disabled="session.busy.value" placeholder="http://live.example.com/live/room.flv" @enter="start" />
          <InlineError v-if="urlInvalid" code="LIVE_URL_INVALID" description="请输入 http:// 或 ws:// 开头的流地址。" />
        </LiveField>
        <div class="tip">支持 HTTP-FLV（http:// 或 https://）和 WS-FLV（ws:// 或 wss://）。</div>
        <PreviewSwitch v-model="previewOn" :disabled="session.busy.value" :note="session.busy.value ? PREVIEW_SWITCH_NOTE_PLAYING : undefined" />
        <div class="chk">低延迟追帧<el-switch v-model="lowLatency" size="small" aria-label="低延迟追帧" :disabled="session.busy.value" /></div>
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
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, toRefs, watch } from 'vue'
import mpegts from 'mpegts.js'
import PlayerShell from '@/components/common/PlayerShell.vue'
import InlineError from '@/components/common/InlineError.vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LiveStatCards from '@/components/live/LiveStatCards.vue'
import PreviewStage, { type StageState } from '@/components/live/PreviewStage.vue'
import PreviewSwitch from '@/components/live/PreviewSwitch.vue'
import FIcon from '@/components/icon/FIcon.vue'
import { usePreviewPoller } from '@/composables/usePreviewPoller'
import { PullPreviewController } from '@/api/pullPreviewSession'
import * as liveApi from '@/api/live'
import { PREVIEW_ROW_OFF, PREVIEW_ROW_ON, PREVIEW_ROW_TITLE, PREVIEW_SWITCH_NOTE_PLAYING, previewAlt } from '@/errors/livePreviewMessages'
import LiveOverlays from '@/components/live/LiveOverlays.vue'
import LiveLogDialog from '@/components/live/LiveLogDialog.vue'
import { livePreview, useLiveSession } from '@/composables/useLiveSession'
import { previewParams } from '@/services/wails'
import { isValidPullUrl, mapPlayerError } from '@/errors/playerError'
import { useLiveFormsStore } from '@/stores/liveForms'

defineOptions({ name: 'LivePullPlay' })

const session = useLiveSession('pull')
const demo = !!livePreview // ?live=… 界面演示（不碰后端）

// 表单输入（流地址、低延迟追帧、静音）在 stores/liveForms：切换菜单不丢、下次启动恢复。拉流会话随页面卸载结束，不会把值恢复进进行中的会话
const forms = useLiveFormsStore()
const { url, lowLatency, muted } = toRefs(forms.pull)
if (livePreview) url.value = livePreview === 'invalid' ? 'http:/live.example' : 'http://live.example.com/live/room.flv' // 界面演示预置（演示模式不读写本机存档）
const logOpen = ref(false)
const urlInvalid = ref(livePreview === 'invalid')
const videoRef = ref<HTMLVideoElement | null>(null)
const hasVideo = ref(false)

// ───── 预览（后端拉流预览会话 StartPullPreview / StopPullPreview + GetPreview 轮询）─────
/** 表单里的开关（默认开；产品经理已定：不记住上次选择，每次打开表单默认开）。开始播放后置灰，值保持开始时的值 */
const previewOn = ref(!(demo && previewParams.get('pvon') === '0')) // 开发演示：?live=running&pvon=0 预置“预览关”
/** 这一次播放开始时定下的预览值（只读显示用） */
const sessionPreviewOn = ref(true)
const { snap, poller } = usePreviewPoller()
const pullState = ref<'idle' | 'starting' | 'active' | 'off' | 'failed'>('idle')
const wsUrl = ref(false)
const pv = new PullPreviewController({ start: liveApi.startPullPreview, stop: liveApi.stopPullPreview }, (st, sess) => {
  pullState.value = st
  if (st === 'active' && sess) poller.start(sess.id)
  else poller.reset()
})
// 设计稿：拉流页播放器舞台就是预览区——播放中一直显示预览舞台（画面 / 加载中 / 失败 / 未开启）；<video> 继续播放（声音、统计），只是不显示
const previewShown = computed(() => session.busy.value)
const stageState = computed<StageState>(() => {
  if (!sessionPreviewOn.value) return 'off'
  if (wsUrl.value) return 'unsupported'
  if (pullState.value === 'failed') return 'failed'
  const p = snap.value.phase
  if (pullState.value === 'active') return p === 'ok' || p === 'failed' || p === 'ended' ? p : 'loading'
  return 'loading'
})
const stageAlt = computed(() => previewAlt('pull', url.value ? url.value.replace(/[?#].*$/, '') : ''))
async function startPreview(u: string) {
  sessionPreviewOn.value = previewOn.value
  wsUrl.value = /^wss?:/i.test(u) // 后端拉流预览不支持 ws / wss
  await pv.begin(u, previewOn.value && !wsUrl.value)
}
function retryPreview() {
  const u = url.value.trim()
  if (pullState.value === 'failed') void pv.begin(u, true) // 会话没建成：重新 Start
  else poller.retry() // 会话在，只是取帧失败：重新计时
}

let player: mpegts.Player | null = null
let statTimer: ReturnType<typeof setInterval> | null = null
let userStopped = false
let lastDecoded = 0

const statusText = computed(() => (session.running.value ? '正在播放' : session.busy.value ? '正在连接' : '未开始播放'))
const statusHint = computed(() => (url.value && session.busy.value ? url.value : '拉流播放'))
const hudLines = computed(() => {
  const s = session.stats
  const res = s.width ? `${s.width}×${s.height} · ` : ''
  return [`${res}${Math.round(s.fps)} fps`, `${Math.round(s.bitrateKbps)} kbps · 丢帧 ${s.dropped}`]
})

watch(url, () => (urlInvalid.value = false))
// 产品经理已定：预览开关不记住上次选择。播放期间开关置灰并显示本次的值，播放结束（停止 / 出错）后复位为开（页面被 KeepAlive 保留时也一样）
watch(() => session.busy.value, (b) => {
  if (!b) previewOn.value = true
})
watch(muted, (m) => {
  if (videoRef.value) videoRef.value.muted = m
})

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
  userStopped = false
  session.setStarting()
  session.log(`开始拉流 ${u}`)
  void startPreview(u) // 演示模式也走模拟层的拉流预览会话（?preview=… 才出画面）
  if (demo) {
    session.simRunning()
    return
  }
  open(u)
}

function open(u: string) {
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
    onFailed(code, status ? `服务器返回 ${status}` : isWs ? 'WebSocket' : '')
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

function onFailed(code: string, detail = '') {
  destroyPlayer()
  void pv.end() // 播放出错：预览会话必须 Stop
  if (code === 'LIVE_URL_INVALID') {
    session.setIdle()
    urlInvalid.value = true
    return
  }
  // 设计稿 v0.2：删掉“断线自动重连”，出错直接显示错误遮罩（用户点“重试”再来）
  session.fail(code, detail)
}

function stop() {
  userStopped = true
  destroyPlayer()
  void pv.end()
  session.setIdle()
  session.log('已停止播放')
}

function fullscreen() {
  videoRef.value?.requestFullscreen?.().catch(() => undefined)
}

onMounted(() => {
  if (demo) {
    session.initPreview()
    if (session.busy.value) void startPreview(url.value.trim()) // ?live=running：预置“播放中”，预览会话一并开
  }
})
// 页面在 KeepAlive 里：切到别的页签时播放继续（v0.2），但后端预览会话（一个 ffmpeg 在读远端流）不该白占资源 → 离开时 Stop，回来仍在播放就重新开一个
let previewDropped = false
onDeactivated(() => {
  if (pullState.value === 'active' || pullState.value === 'starting') {
    previewDropped = true
    void pv.end()
  }
})
onActivated(() => {
  if (previewDropped && session.busy.value && sessionPreviewOn.value) void startPreview(url.value.trim())
  previewDropped = false
})
onBeforeUnmount(() => {
  userStopped = true
  destroyPlayer()
  void pv.end() // 离开页面 / 卸载：预览会话必须 Stop
})
</script>

<style scoped>
.idle {
  position: absolute;
  inset: 0;
  background: #0b0c0e;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #7c828c;
  font-size: var(--ff-fs-xs);
}
.pvr {
  margin-left: 8px;
  font-size: var(--ff-fs-xs);
  opacity: 0.85;
}
.tip {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.5;
  margin-top: -4px;
}
.chk {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: var(--ff-fs-sm);
}
</style>
