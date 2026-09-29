<template>
  <LiveTabFrame>
    <template #main>
      <PlayerShell
        v-model:muted="muted"
        fill
        mode="status"
        :status-text="session.running.value ? '正在推流' : session.busy.value ? '正在连接' : '未开始推流'"
        :status-hint="selected ? selected.name : '请选择要推流的素材'"
        @fullscreen="fullscreen"
      >
        <video
          v-if="selected && !preview"
          ref="videoRef"
          :src="selected.url"
          :poster="selected.cover"
          :muted="muted"
          loop
          playsinline
          preload="metadata"
        />
        <LiveMockFrame v-else-if="selected" variant="scene" />
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
            <el-select :id="id" v-model="selectedName" placeholder="选择素材" :disabled="session.busy.value" no-data-text="还没有素材，先上传一个 MP4" class="mat-sel">
              <el-option v-for="m in materials" :key="m.name" :label="m.name" :value="m.name" />
            </el-select>
            <el-upload :show-file-list="false" accept="video/mp4" :before-upload="beforeUpload" :http-request="onUpload" :disabled="session.busy.value">
              <LiveButton icon="upload" :disabled="session.busy.value">上传</LiveButton>
            </el-upload>
          </div>
          <div v-if="uploadPercent > 0 && uploadPercent < 100" class="hint">上传中 {{ uploadPercent }}%</div>
          <div v-else-if="materialsHint" class="hint">{{ materialsHint }}</div>
          <button v-if="selected && !session.busy.value && !preview" type="button" class="del" @click="removeSelected">
            <FIcon name="trash" :size="13" />删除这个素材
          </button>
        </LiveField>
        <LiveField label="推流地址">
          <LiveInput v-model="baseUrl" :bad="urlBad" :disabled="session.busy.value" placeholder="rtmp://live.example.com/live" copyable @enter="start" />
          <InlineError v-if="urlBad" code="LIVE_URL_INVALID" />
        </LiveField>
        <LiveField label="推流码">
          <LiveInput v-model="streamKey" secret :bad="keyBad" :disabled="session.busy.value" placeholder="留空则使用地址本身" />
          <InlineError v-if="keyBad" :code="session.errorCode.value" />
        </LiveField>
        <div class="chk">断线自动重连<el-switch v-model="autoReconnect" size="small" aria-label="断线自动重连" /></div>
        <LiveAdvanced v-model:archive-enabled="archiveEnabled" v-model:segment-seconds="segmentSeconds" v-model:relay-text="relayText" />
        <template #action>
          <LiveButton v-if="session.busy.value" variant="danger" lg icon="x" @click="stop">停止推流</LiveButton>
          <LiveButton v-else variant="pri" lg icon="play" @click="start">开始推流</LiveButton>
        </template>
      </LivePanel>
    </template>
  </LiveTabFrame>
  <LiveLogDialog v-model="logOpen" :lines="session.logs.value" />
</template>

<script setup lang="ts">
// 文件推流：选一个素材，推到 RTMP 地址。UI 按 proto/pages.html?page=live 的布局（播放器 + 指标卡 + 320 设置面板）。
// 所有后端调用走 @/api/live（过渡期 v1 HTTP，之后换 Wails LiveService）。
import { computed, onActivated, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox, type UploadRequestOptions } from 'element-plus'
import PlayerShell from '@/components/common/PlayerShell.vue'
import InlineError from '@/components/common/InlineError.vue'
import FIcon from '@/components/icon/FIcon.vue'
import LiveTabFrame from '@/components/live/LiveTabFrame.vue'
import LivePanel from '@/components/live/LivePanel.vue'
import LiveField from '@/components/live/LiveField.vue'
import LiveInput from '@/components/live/LiveInput.vue'
import LiveButton from '@/components/live/LiveButton.vue'
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

defineOptions({ name: 'LiveFilePush' })

const session = useLiveSession('file')
const ffmpeg = useFFmpegStore()
const preview = !!livePreview

const materials = ref<liveApi.LiveMaterial[]>([])
const selectedName = ref('')
const selected = computed(() => materials.value.find((m) => m.name === selectedName.value))
const materialsHint = ref('')
const uploadPercent = ref(0)
const baseUrl = ref(livePreview === 'invalid' ? 'http:/live.example' : livePreview ? 'rtmp://live-push.example.com/live' : '')
const streamKey = ref(livePreview ? '••••••••••••••••' : '')
const autoReconnect = ref(true)
const archiveEnabled = ref(false)
const segmentSeconds = ref(300)
const relayText = ref('')
const muted = ref(false)
const logOpen = ref(false)
const urlInvalid = ref(livePreview === 'invalid')
const videoRef = ref<HTMLVideoElement | null>(null)

let streamId = ''
let stopStats: (() => void) | null = null
let stopEvents: (() => void) | null = null
let userStopped = false
let reconnects = 0
const MAX_RECONNECT = 5

const urlBad = computed(() => urlInvalid.value)
// 推流码被拒（LIVE_PUSH_REJECTED）时推流码输入框变红并给行内说明，和原型一致
const keyBad = computed(() => session.phase.value === 'error' && session.errorCode.value === 'LIVE_PUSH_REJECTED')
const hudLines = computed(() => {
  const s = session.stats
  const res = s.width ? `${s.width}×${s.height} · ` : ''
  return [`${res}${Math.round(s.fps)} fps`, `${Math.round(s.bitrateKbps)} kbps · 丢帧 ${s.dropped}`]
})

watch(baseUrl, () => (urlInvalid.value = false))
watch(
  () => ffmpeg.ready,
  (ok) => {
    if (ok && session.errorCode.value === 'FFMPEG_NOT_FOUND') session.setIdle()
  },
)
watch(
  () => session.running.value,
  (r) => {
    const v = videoRef.value
    if (!v) return
    if (r) v.play().catch(() => undefined)
    else v.pause()
  },
)
watch(muted, (m) => {
  if (videoRef.value) videoRef.value.muted = m
})

async function loadMaterials() {
  if (preview) {
    materials.value = [
      { name: '产品发布会.mp4', url: '', duration: '00:42:18', date: '2026-09-28 20:11:03' },
      { name: '直播预热片.mp4', url: '', duration: '00:03:12', date: '2026-09-27 09:30:45' },
    ]
    if (!selectedName.value) selectedName.value = materials.value[0].name
    return
  }
  try {
    materials.value = await liveApi.listMaterials()
    materialsHint.value = materials.value.length ? '' : '还没有素材，点“上传”添加 MP4 文件'
    if (!materials.value.some((m) => m.name === selectedName.value)) selectedName.value = materials.value[0]?.name ?? ''
  } catch (e) {
    materials.value = []
    materialsHint.value = `读取素材列表失败：${(e as Error).message}`
  }
}

function beforeUpload(file: File) {
  if (!['video/mp4'].includes(file.type)) {
    ElMessage.error('只支持上传 MP4 视频')
    return false
  }
  return true
}

async function onUpload(opt: UploadRequestOptions) {
  uploadPercent.value = 1
  try {
    await liveApi.uploadMaterial(opt.file as File, (p) => (uploadPercent.value = p))
    ElMessage.success('上传成功')
    await loadMaterials()
    selectedName.value = (opt.file as File).name
  } catch (e) {
    ElMessage.error(`上传失败：${(e as Error).message}`)
  } finally {
    uploadPercent.value = 0
  }
}

async function removeSelected() {
  if (!selected.value) return
  try {
    await ElMessageBox.confirm(`确定删除素材“${selected.value.name}”？`, '删除素材', { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' })
  } catch {
    return
  }
  try {
    await liveApi.deleteMaterial(selected.value.name)
    ElMessage.success('已删除')
    await loadMaterials()
  } catch (e) {
    ElMessage.error(`删除失败：${(e as Error).message}`)
  }
}

function cleanup() {
  stopStats?.()
  stopEvents?.()
  stopStats = stopEvents = null
}

function watchStream(id: string) {
  streamId = id
  cleanup()
  stopStats = liveApi.subscribeStats({ streamId: id }, (s) => {
    if (!s || !session.busy.value) return
    if (s.status === 'running') session.setRunning()
    session.addSample({ bitrateKbps: s.bitrateKbps, fps: s.fps, dropped: s.droppedFrames, width: videoRef.value?.videoWidth || 0, height: videoRef.value?.videoHeight || 0 })
    reconnects = 0
  })
  stopEvents = liveApi.onLiveEvent((e) => {
    if (e.streamId !== streamId) return
    session.log(`${e.status}${e.error ? ' · ' + e.error : ''}`)
    if (e.status === 'failed') onFailed(e.error)
    else {
      cleanup()
      session.setIdle()
      if (e.status === 'completed') ElMessage.success('推流已完成')
    }
  })
}

function onFailed(detail?: string) {
  cleanup()
  if (!userStopped && autoReconnect.value && reconnects < MAX_RECONNECT) {
    reconnects++
    session.log(`断线，3 秒后自动重连（${reconnects}/${MAX_RECONNECT}）`)
    setTimeout(() => !userStopped && start(true), 3000)
    return
  }
  // v1 后端不区分失败原因：还没有出过数据算“连不上”，出过数据算“中断”
  const neverRan = session.stats.bitrateKbps === 0
  session.fail(neverRan ? 'LIVE_CONNECT_FAILED' : 'LIVE_PUSH_INTERRUPTED', '', detail ?? '')
}

async function start(isReconnect = false) {
  if (session.busy.value && isReconnect !== true) return
  if (ffmpeg.needsAttention) return session.fail('FFMPEG_NOT_FOUND')
  const material = selected.value
  if (!material) {
    ElMessage.warning('请先选择推流素材')
    return
  }
  const full = joinPushUrl(baseUrl.value, streamKey.value)
  if (!isValidStreamUrl(full)) {
    urlInvalid.value = true
    session.log('推流地址格式不正确')
    return
  }
  userStopped = false
  if (isReconnect !== true) reconnects = 0
  session.setStarting()
  session.log(`开始推流 ${material.name}`)
  try {
    const id = await liveApi.startFilePush({
      material,
      url: full,
      archiveEnabled: archiveEnabled.value,
      segmentSeconds: segmentSeconds.value,
      relayTargets: parseTargets(relayText.value),
    })
    watchStream(id)
    session.setRunning()
  } catch (e) {
    const err = e as liveApi.LiveError
    onFailedStart(err)
  }
}

function onFailedStart(err: liveApi.LiveError) {
  if (autoReconnect.value && reconnects > 0 && reconnects < MAX_RECONNECT) return onFailed(err.message)
  session.fail(err.code === 'INTERNAL' ? 'LIVE_CONNECT_FAILED' : err.code, '', err.message)
}

async function stop() {
  userStopped = true
  if (preview) {
    session.setIdle()
    return
  }
  try {
    if (streamId) await liveApi.stopStream({ streamId })
  } catch (e) {
    ElMessage.error(`停止失败：${(e as Error).message}`)
  }
  cleanup()
  session.setIdle()
  session.log('已停止推流')
}

function fullscreen() {
  videoRef.value?.requestFullscreen?.().catch(() => undefined)
}

/** 页面刷新后接回还在推的会话 */
async function recover() {
  try {
    const running = await liveApi.listRunning()
    const r = running.find((x) => materials.value.some((m) => m.name === x.name))
    if (!r) return
    selectedName.value = r.name
    baseUrl.value = r.url
    session.setStarting()
    session.setRunning()
    session.startClock(0)
    watchStream(r.streamId)
    session.log(`已接回正在推流的任务 ${r.name}`)
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
  color: var(--ff-text-3);
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
