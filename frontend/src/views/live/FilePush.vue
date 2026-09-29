<template>
  <LiveTabFrame>
    <template #main>
      <PlayerShell
        v-model:muted="muted"
        fill
        mode="status"
        :status-text="statusText"
        :status-hint="selected ? selected.name : '请选择要推流的素材'"
        @fullscreen="fullscreen"
      >
        <LiveMockFrame v-if="selected" variant="scene" />
        <LiveMockFrame v-else variant="idle" icon="upload" hint="选择素材后，这里会预览要推送的画面" />
        <template #overlay>
          <LiveOverlays :session="session" :hud-lines="hudLines" @retry="start" @view-log="logOpen = true" />
        </template>
      </PlayerShell>
      <LiveStatCards :stats="session.stats" :series="session.series" />
    </template>

    <template #panel>
      <LivePanel title="推流设置" note="不占用转换队列">
        <LiveField v-slot="{ id }" label="推流素材">
          <div class="mat">
            <el-select :id="id" v-model="selectedPath" placeholder="选择素材" :disabled="session.busy.value" no-data-text="还没有素材，先选择一个视频文件" class="mat-sel">
              <el-option v-for="m in materials" :key="m.path" :label="m.name" :value="m.path" />
            </el-select>
            <LiveButton icon="upload" :disabled="session.busy.value" @click="pickMaterial">选择文件</LiveButton>
          </div>
          <div v-if="materialsHint" class="hint">{{ materialsHint }}</div>
        </LiveField>
        <LiveField label="推流地址">
          <LiveInput v-model="baseUrl" :bad="urlBad" :disabled="session.busy.value" placeholder="rtmp://live.example.com/live" copyable @enter="start" @blur="checkUrl" />
          <InlineError v-if="urlBad" code="LIVE_URL_INVALID" :description="urlMessage" bare />
        </LiveField>
        <LiveField label="推流码">
          <LiveInput v-model="streamKey" secret :bad="keyBad" :disabled="session.busy.value" placeholder="留空则使用地址本身" />
          <InlineError v-if="keyBad" :code="session.errorCode.value" />
        </LiveField>
        <div class="chk">循环播放<el-switch v-model="loop" size="small" aria-label="循环播放" :disabled="session.busy.value" /></div>
        <div class="chk">断线自动重连<el-switch v-model="autoReconnect" size="small" aria-label="断线自动重连" /></div>
        <ErrorLine v-if="startError" code="TASK_CONFLICT" :title="startError.title" :description="startError.description" :show-log="false" hide-code compact />
        <template #action>
          <LiveButton v-if="session.busy.value" variant="danger" lg icon="x" :disabled="stopping" @click="stop">{{ stopping ? '正在停止…' : '停止推流' }}</LiveButton>
          <LiveButton v-else variant="pri" lg icon="play" @click="start">开始推流</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
  <LiveLogDialog v-model="logOpen" :lines="session.logs.value" />
</template>

<script setup lang="ts">
// 文件推流：选一个本地视频文件，推到 rtmp / rtmps / srt 地址。UI 按 proto/pages.html?page=live 的布局（播放器 + 指标卡 + 320 设置面板）。
// 所有后端调用走 @/api/live（契约 v0.10：StartFilePush 返回 Task，停止 = TaskService.Cancel，指标走 task:progress）。
// 文件推流始终重编码（架构师定），表单里不提示；文件推流没有存档选项（只有屏幕推流有）。完整推流地址只存在于输入框和调用参数里，不写日志。
import { computed, onActivated, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import PlayerShell from '@/components/common/PlayerShell.vue'
import InlineError from '@/components/common/InlineError.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
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
import { useFFmpegStore } from '@/stores/ffmpeg'
import { liveFailureMessage, liveUrlInvalidText, LIVE_STOP_TEXT, liveStartErrorLine } from '@/errors/errorMessages'
import { joinPushUrl, parsePushUrl } from '@/utils/liveUrl'
import * as liveApi from '@/api/live'
import { liveIsReal } from '@/api/live'
import { toAppError } from '@/api/call'

defineOptions({ name: 'LiveFilePush' })

const session = useLiveSession('file')
const ffmpeg = useFFmpegStore()
const preview = !!livePreview
// 后端 LiveService 未接入时素材是演示数据
const demo = !liveIsReal()

const materials = ref<liveApi.LiveMaterial[]>([])
const selectedPath = ref('')
const selected = computed(() => materials.value.find((m) => m.path === selectedPath.value))
const materialsHint = ref('')
const baseUrl = ref(livePreview === 'invalid' ? 'http:/live.example' : livePreview ? 'rtmp://live-push.example.com/live' : '')
const streamKey = ref(livePreview ? '••••••••••••••••' : '')
const autoReconnect = ref(true)
const loop = ref(true)
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

const urlBad = computed(() => urlInvalid.value)
const statusText = computed(() => (session.running.value ? '正在推流' : session.busy.value ? (stopping.value ? '正在停止' : '正在连接') : '未开始推流'))
// 推流码被拒（LIVE_PUSH_REJECTED）时推流码输入框变红并给行内说明，和原型一致
const keyBad = computed(() => session.phase.value === 'error' && session.errorCode.value === 'LIVE_PUSH_REJECTED')
const hudLines = computed(() => {
  const s = session.stats
  const res = s.width ? `${s.width}×${s.height} · ` : ''
  return [`${res}${Math.round(s.fps)} fps`, `${Math.round(s.bitrateKbps)} kbps · 丢帧 ${s.dropped}`]
})

watch(baseUrl, () => {
  urlInvalid.value = false
  startError.value = null
})
watch(
  () => ffmpeg.ready,
  (ok) => {
    if (ok && session.errorCode.value === 'FFMPEG_NOT_FOUND') session.setIdle()
  },
)

/** 地址框失焦校验：协议不支持（rtsp、http-flv 等）显示“暂不支持这种推流地址…”，红框规则不变 */
function checkUrl() {
  if (!baseUrl.value.trim()) return
  const r = parsePushUrl(joinPushUrl(baseUrl.value, ''))
  urlInvalid.value = !r.ok
  urlMessage.value = r.ok ? '' : r.message
}

async function loadMaterials() {
  if (demo) {
    materials.value = liveApi.demoMaterials()
    materialsHint.value = ''
    if (!materials.value.some((m) => m.path === selectedPath.value)) selectedPath.value = materials.value[0]?.path ?? ''
  } else if (!materials.value.length) {
    materialsHint.value = '还没有素材，点“选择文件”添加视频'
  }
}

async function pickMaterial() {
  try {
    const picked = await liveApi.pickMaterial()
    for (const m of picked) {
      if (!materials.value.some((x) => x.path === m.path)) materials.value.push(m)
      selectedPath.value = m.path
    }
    if (materials.value.length) materialsHint.value = ''
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}

function cleanup() {
  stopWatch?.()
  stopWatch = null
}

function watchTask(id: string) {
  taskId = id
  cleanup()
  stopWatch = liveApi.watchLiveTask(id, {
    // 收到第一条 task:progress 才算“已经在推”；之前是“连接中”
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
      // 停止文案只看 status（后端保证 succeeded 时 error 为空、canceled 时不带错误码）
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
  // 只有推流已开始后被中断（LIVE_PUSH_INTERRUPTED）才自动重连；开始前的连接失败 / 被拒绝重连也不会好
  if (!userStopped && autoReconnect.value && code === 'LIVE_PUSH_INTERRUPTED' && reconnects < MAX_RECONNECT) {
    reconnects++
    session.log(`断线，3 秒后自动重连（${reconnects}/${MAX_RECONNECT}）`)
    setTimeout(() => !userStopped && start(true), 3000)
    return
  }
  // scheme 以 detail 首行 scheme= 为准，页面上的地址只是兜底
  session.fail(code, '', liveFailureMessage({ code, ...error }, currentScheme()))
}

function currentScheme(): string {
  const r = parsePushUrl(joinPushUrl(baseUrl.value, streamKey.value))
  return r.ok ? r.info.scheme : ''
}

async function start(isReconnect: unknown = false) {
  const reconnecting = isReconnect === true
  if (session.busy.value && !reconnecting) return
  if (ffmpeg.needsAttention) return session.fail('FFMPEG_NOT_FOUND')
  const material = selected.value
  if (!material) {
    ElMessage.warning('请先选择推流素材')
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
  session.setStarting()
  // 日志里只写脱敏后的地址
  session.log(`开始推流 ${material.name} → ${check.info.redacted}`)
  try {
    const task = await liveApi.startFilePush({
      inputPath: material.path,
      url: full,
      loop: loop.value,
      options: liveApi.defaultPushOptions(),
    })
    watchTask(task.id)
  } catch (e) {
    onStartFailed(toAppError(e), check.info.scheme)
  }
}

function onStartFailed(err: liveApi.LiveError, scheme: string) {
  const line = liveStartErrorLine(err, { scheme })
  if (line && (err.code === 'TASK_CONFLICT' || err.code === 'UNSUPPORTED')) {
    // 点击开始之后的错误行，不提前置灰按钮
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
  session.fail(err.code, '', liveFailureMessage(err, scheme))
}

async function stop() {
  userStopped = true
  if (preview) {
    session.setIdle()
    return
  }
  if (!taskId) {
    session.setIdle()
    return
  }
  try {
    // 停止 = TaskService.Cancel；立即返回，结果以 task:status 为准（onEnd 里显示“已结束推流”/“已强制停止”）
    stopping.value = true
    await liveApi.stopPush(taskId)
    session.log('正在停止推流…')
  } catch (e) {
    stopping.value = false
    const err = toAppError(e)
    if (err.code === 'TASK_CONFLICT' || err.code === 'NOT_FOUND') {
      // 会话已经结束了：以事件为准，补一次收尾
      return
    }
    ElMessage.error(`停止失败：${err.message}`)
  }
}

function fullscreen() {
  document.querySelector<HTMLElement>('.ff-player')?.requestFullscreen?.().catch(() => undefined)
}

/** 页面刷新后接回还在推的会话（ListActive 里的 live_file_push；params 已脱敏，拿不到完整地址） */
async function recover() {
  try {
    const running = (await liveApi.listRunning()).filter((x) => x.type === 'live_file_push')
    const r = running.find((x) => materials.value.some((m) => x.inputPaths.includes(m.path))) ?? running[0]
    if (!r) return
    const m = materials.value.find((x) => r.inputPaths.includes(x.path))
    if (m) selectedPath.value = m.path
    session.setStarting()
    session.setRunning()
    session.startClock(r.startedAt ? Math.max(0, (Date.now() - r.startedAt) / 1000) : 0)
    watchTask(r.streamId)
    session.log(`已接回正在推流的任务：${r.title}`)
  } catch {
    /* 后端没起就算了 */
  }
}

onMounted(async () => {
  await loadMaterials()
  if (preview) session.initPreview()
  else await recover()
})
onActivated(() => {
  if (!preview && !session.busy.value) loadMaterials()
})
onBeforeUnmount(cleanup)
</script>

<style scoped>
.mat {
  display: flex;
  gap: 6px;
}
.mat-sel {
  flex: 1;
  min-width: 0;
}
.hint {
  margin-top: 6px;
  font-size: 12px;
  color: var(--ff-text-2);
  line-height: 1.5;
}
.del {
  margin-top: 6px;
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  font-size: 12px;
  color: var(--ff-text-3);
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
}
.del:hover {
  color: var(--ff-danger);
}
.chk {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
}
</style>
