<template>
  <LiveTabFrame>
    <template #main>
      <PlayerShell
        v-model:muted="muted"
        fill
        mode="status"
        :status-text="statusText"
        :status-hint="statusHint"
        @fullscreen="fullscreen"
      >
        <LiveMockFrame v-if="session.phase.value !== 'idle'" variant="screen" />
        <LiveMockFrame v-else variant="idle" icon="monitor" hint="点击“开始推流”后，后端会直接采集所选显示器并推流" />
        <template #overlay>
          <LiveOverlays :session="session" :hud-lines="hudLines" @retry="start" @view-log="logOpen = true" />
        </template>
      </PlayerShell>
      <LiveStatCards :stats="session.stats" :series="session.series" />
    </template>

    <template #panel>
      <LivePanel title="推流设置" note="不占用转换队列">
        <LiveField label="画面来源" :control="false">
          <LiveSourcePicker v-model="source" :disabled="session.busy.value" />
          <!-- 选中屏幕推流时常驻，不弹窗 -->
          <p v-if="source === 'screen'" class="note-inline"><FIcon name="info" :size="14" />{{ LIVE_SCREEN_NO_AUDIO_TEXT }}</p>
          <p v-else class="note-inline"><FIcon name="info" :size="14" />摄像头和窗口推流暂不支持，请选择屏幕。</p>
        </LiveField>
        <LiveField v-if="screens.length > 1" v-slot="{ id }" label="显示器">
          <el-select :id="id" v-model="screenId" :disabled="session.busy.value" class="sel">
            <el-option v-for="sc in screens" :key="sc.id" :label="sc.name" :value="sc.id" />
          </el-select>
        </LiveField>
        <LiveField label="推流地址">
          <LiveInput v-model="baseUrl" :bad="urlInvalid" :disabled="session.busy.value" placeholder="rtmp://live.example.com/live" copyable @enter="start" @blur="checkUrl" />
          <InlineError v-if="urlInvalid" code="LIVE_URL_INVALID" :description="urlMessage" bare />
        </LiveField>
        <LiveField label="推流码">
          <LiveInput v-model="streamKey" secret :bad="keyBad" :disabled="session.busy.value" placeholder="留空则使用地址本身" />
          <InlineError v-if="keyBad" :code="session.errorCode.value" />
        </LiveField>
        <div class="two">
          <LiveField v-slot="{ id }" label="分辨率">
            <el-select :id="id" v-model="resolution" :disabled="session.busy.value" class="sel">
              <el-option v-for="r in resolutions" :key="r.value" :label="r.label" :value="r.value" />
            </el-select>
          </LiveField>
          <LiveField v-slot="{ id }" label="帧率">
            <el-select :id="id" v-model="fps" :disabled="session.busy.value" class="sel">
              <el-option v-for="f in [15, 24, 30, 60]" :key="f" :label="`${f} fps`" :value="f" />
            </el-select>
          </LiveField>
        </div>
        <LiveField label="视频码率" :control="false"><LiveSlider v-model="bitrate" :min="1000" :max="10000" :step="500" unit="k" :disabled="session.busy.value" /></LiveField>
        <div class="chk">隐藏鼠标指针<el-switch v-model="hideCursor" size="small" aria-label="隐藏鼠标指针" :disabled="session.busy.value" /></div>
        <div class="chk">补一路静音音轨<el-switch v-model="silentAudio" size="small" aria-label="补一路静音音轨" :disabled="session.busy.value" /></div>
        <div class="chk">断线自动重连<el-switch v-model="autoReconnect" size="small" aria-label="断线自动重连" /></div>
        <div class="chk">同时保存本地存档（mp4）<el-switch v-model="archiveEnabled" size="small" aria-label="同时保存本地存档" :disabled="session.busy.value" /></div>
        <div v-if="archiveEnabled" class="hint">{{ archiveDir || '开始推流时选择存档文件夹' }}</div>
        <ErrorLine v-if="startError" code="TASK_CONFLICT" :title="startError.title" :description="startError.description" :show-log="false" hide-code compact />
        <template #action>
          <LiveButton v-if="session.busy.value" variant="danger" lg icon="x" :disabled="stopping" @click="stop">{{ stopping ? '正在停止…' : '停止推流' }}</LiveButton>
          <LiveButton v-else variant="pri" lg icon="rec" @click="start">开始推流</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
  <LiveLogDialog v-model="logOpen" :lines="session.logs.value" />
</template>

<script setup lang="ts">
// 屏幕推流（契约 v0.10）：由后端 ffmpeg 直接采集显示器并推到 rtmp / rtmps / srt 地址，前端不再用 getDisplayMedia / MediaRecorder / WebSocket。
// 屏幕推流首版不包含声音（只有 none / silent）；可同时在本地存 mp4 存档。停止 = TaskService.Cancel。完整推流地址不写日志。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import PlayerShell from '@/components/common/PlayerShell.vue'
import InlineError from '@/components/common/InlineError.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import FIcon from '@/components/icon/FIcon.vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
import LiveSlider from '@/components/live/LiveSlider.vue'
import LiveSourcePicker, { type CaptureSource } from '@/components/live/LiveSourcePicker.vue'
import LiveStatCards from '@/components/live/LiveStatCards.vue'
import LiveMockFrame from '@/components/live/LiveMockFrame.vue'
import LiveOverlays from '@/components/live/LiveOverlays.vue'
import LiveLogDialog from '@/components/live/LiveLogDialog.vue'
import { livePreview, useLiveSession } from '@/composables/useLiveSession'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { LIVE_SCREEN_NO_AUDIO_TEXT, liveFailureMessage, liveUrlInvalidText, LIVE_STOP_TEXT, liveStartErrorLine } from '@/errors/errorMessages'
import { joinPushUrl, parsePushUrl } from '@/utils/liveUrl'
import * as liveApi from '@/api/live'
import { toAppError } from '@/api/call'
import { getDefaultOutputDir, pickDirectory } from '@/api/system'

defineOptions({ name: 'LiveRecordPush' })

const session = useLiveSession('record')
const ffmpeg = useFFmpegStore()
const preview = !!livePreview

const source = ref<CaptureSource>('screen')
const screens = ref<liveApi.ScreenInfo[]>([])
const screenId = ref('')
const baseUrl = ref(livePreview === 'invalid' ? 'http:/live.example' : livePreview ? 'rtmp://live-push.example.com/live' : '')
const streamKey = ref(livePreview ? '••••••••••••••••' : '')
const resolutions = [
  { label: '原始', value: 0 },
  { label: '720p', value: 720 },
  { label: '1080p', value: 1080 },
]
const resolution = ref(1080)
const fps = ref(30)
const bitrate = ref(6000)
const hideCursor = ref(false)
const silentAudio = ref(false)
const autoReconnect = ref(true)
const archiveEnabled = ref(false)
const archiveDir = ref('')
const muted = ref(false)
const logOpen = ref(false)
const urlInvalid = ref(livePreview === 'invalid')
const urlMessage = ref('')
const stopping = ref(false)
/** 点“开始推流”之后才出现的错误行（最多同时推 4 路 / 这个地址已经在推流 / 当前 ffmpeg 不支持这种推流协议…） */
const startError = ref<{ title: string; description: string } | null>(null)

let taskId = ''
let stopWatch: (() => void) | null = null
let userStopped = false
let reconnects = 0
const MAX_RECONNECT = 5

const keyBad = computed(() => session.phase.value === 'error' && session.errorCode.value === 'LIVE_PUSH_REJECTED')
const statusText = computed(() => (session.running.value ? '正在推流' : session.busy.value ? (stopping.value ? '正在停止' : '正在连接') : '未开始推流'))
const statusHint = computed(() => screens.value.find((s) => s.id === screenId.value)?.name ?? (source.value === 'screen' ? '屏幕' : '暂不支持'))
const hudLines = computed(() => {
  const s = session.stats
  const res = s.width ? `${s.width}×${s.height} · ` : ''
  return [`${res}${Math.round(s.fps)} fps`, `${Math.round(s.bitrateKbps)} kbps · 丢帧 ${s.dropped}`]
})

watch([baseUrl, streamKey], () => {
  urlInvalid.value = false
  startError.value = null
})

/** 地址框失焦校验：rtsp、http-flv 等不支持的协议显示“暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt”，红框规则不变 */
function checkUrl() {
  if (!baseUrl.value.trim()) return
  const r = parsePushUrl(joinPushUrl(baseUrl.value, ''))
  urlInvalid.value = !r.ok
  urlMessage.value = r.ok ? '' : r.message
}

function cleanup() {
  stopWatch?.()
  stopWatch = null
}

function watchTask(id: string) {
  taskId = id
  cleanup()
  stopWatch = liveApi.watchLiveTask(id, {
    onConnected: () => session.setRunning(),
    onProgress: (p) => {
      if (!session.busy.value) return
      session.addSample({ bitrateKbps: p.bitrateKbps, fps: p.fps, dropped: p.droppedFrames })
      reconnects = 0
    },
    onEnd: (e) => {
      cleanup()
      stopping.value = false
      taskId = ''
      // 停止文案只看 status（succeeded → 已结束推流，canceled → 已强制停止）
      if (e.status === 'succeeded') {
        session.setIdle()
        session.log(LIVE_STOP_TEXT.succeeded)
        ElMessage.success(LIVE_STOP_TEXT.succeeded)
      } else if (e.status === 'canceled') {
        session.setIdle()
        session.log(LIVE_STOP_TEXT.canceled)
        ElMessage.warning(LIVE_STOP_TEXT.canceled)
      } else if (e.status === 'interrupted') {
        session.setIdle()
        session.log('应用退出，推流已中断')
      } else {
        onFailed(e.error?.code ?? 'INTERNAL', e.error ?? undefined)
      }
    },
  })
}

function onFailed(code: string, error?: { message?: string; detail?: string }) {
  if (!userStopped && autoReconnect.value && code === 'LIVE_PUSH_INTERRUPTED' && reconnects < MAX_RECONNECT) {
    reconnects++
    session.log(`断线，3 秒后自动重连（${reconnects}/${MAX_RECONNECT}）`)
    setTimeout(() => !userStopped && start(true), 3000)
    return
  }
  const r = parsePushUrl(joinPushUrl(baseUrl.value, streamKey.value))
  // scheme 以 detail 首行 scheme= 为准，页面上的地址只是兜底
  session.fail(code, '', liveFailureMessage({ code, ...error }, r.ok ? r.info.scheme : ''))
}

async function ensureArchiveDir(): Promise<string | null> {
  if (!archiveEnabled.value) return ''
  if (archiveDir.value) return archiveDir.value
  const dir = preview || !liveApi.LIVE_BACKEND_READY ? await getDefaultOutputDir().catch(() => '') || '/Users/me/Movies/FFmpegFree' : await getDefaultOutputDir()
  if (dir) return (archiveDir.value = dir)
  const picked = await pickDirectory('选择存档文件夹')
  if (!picked) return null // 用户取消
  return (archiveDir.value = picked)
}

async function start(isReconnect: unknown = false) {
  const reconnecting = isReconnect === true
  if (session.busy.value && !reconnecting) return
  if (ffmpeg.needsAttention) return session.fail('FFMPEG_NOT_FOUND')
  if (source.value !== 'screen') {
    startError.value = { title: '无法开始推流', description: '摄像头和窗口推流暂不支持，请选择屏幕。' }
    return
  }
  startError.value = null
  const full = joinPushUrl(baseUrl.value, streamKey.value)
  const check = parsePushUrl(full)
  if (!check.ok) {
    urlInvalid.value = true
    urlMessage.value = check.message
    session.log('推流地址格式不正确')
    return
  }
  userStopped = false
  stopping.value = false
  if (!reconnecting) reconnects = 0
  let dir: string | null
  try {
    dir = await ensureArchiveDir()
  } catch (e) {
    ElMessage.error(toAppError(e).message)
    return
  }
  if (dir === null) return
  session.setStarting()
  session.log(`开始屏幕推流（${statusHint.value}）→ ${check.info.redacted}`)
  const h = resolution.value
  try {
    const task = await liveApi.startScreenPush({
      url: full,
      screenId: screenId.value,
      hideCursor: hideCursor.value,
      audio: silentAudio.value ? 'silent' : 'none',
      archiveDir: dir,
      options: { ...liveApi.defaultPushOptions(), height: h, fps: fps.value, videoBitrateKbps: bitrate.value },
    })
    watchTask(task.id)
  } catch (e) {
    onStartFailed(toAppError(e), check.info.scheme)
  }
}

function onStartFailed(err: liveApi.LiveError, scheme: string) {
  const line = liveStartErrorLine(err, { scheme })
  if (line && (err.code === 'TASK_CONFLICT' || err.code === 'UNSUPPORTED')) {
    session.setIdle()
    startError.value = line
    session.log(`${line.title}：${line.description}`)
    return
  }
  if (err.code === 'LIVE_URL_INVALID') {
    session.setIdle()
    urlInvalid.value = true
    urlMessage.value = liveUrlInvalidText(err.reason) // detail 首行 reason=；未知 / 缺失 → 通用文案
    return
  }
  // UNSUPPORTED_PLATFORM / SCREEN_PERMISSION_DENIED / FFMPEG_NOT_FOUND 等走遮罩
  session.fail(err.code, '', liveFailureMessage(err, scheme))
}

async function stop() {
  userStopped = true
  if (preview || !taskId) {
    session.setIdle()
    return
  }
  try {
    stopping.value = true
    await liveApi.stopPush(taskId)
    session.log('正在停止推流…')
  } catch (e) {
    stopping.value = false
    const err = toAppError(e)
    if (err.code === 'TASK_CONFLICT' || err.code === 'NOT_FOUND') return // 已经结束，以事件为准
    ElMessage.error(`停止失败：${err.message}`)
  }
}

function fullscreen() {
  document.querySelector<HTMLElement>('.ff-player')?.requestFullscreen?.().catch(() => undefined)
}

onMounted(async () => {
  if (preview) session.initPreview()
  try {
    screens.value = await liveApi.listScreens()
    screenId.value = screens.value.find((s) => s.primary)?.id ?? screens.value[0]?.id ?? ''
  } catch (e) {
    const err = toAppError(e)
    if (err.code === 'UNSUPPORTED_PLATFORM') session.log(err.message)
  }
})
onBeforeUnmount(() => {
  userStopped = true
  cleanup()
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
.hint {
  margin-top: -6px;
  font-size: 12px;
  color: var(--ff-text-2);
  word-break: break-all;
}
.note-inline {
  margin: 8px 0 0;
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ff-text-2);
}
.note-inline > svg {
  margin-top: 2px;
  flex: none;
}
.chk {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
}
</style>
