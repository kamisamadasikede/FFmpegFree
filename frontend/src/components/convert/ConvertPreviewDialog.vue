<script setup lang="ts">
// 转换页预览弹窗（设计 §六）：复用剪辑页播放器外壳 PlayerShell（不用全屏的 MediaPreviewDialog）。
// 视频（880 / 784 宽，舞台 440 / 344）、音频（560 宽，封面 + 进度条，无波形、无全屏）、GIF（只留播放 / 暂停）。
// <video>/<audio> 报错或接口给 UNSUPPORTED reason=format → “无法在应用内播放”；地址 token 过期（404）时重新取一次地址。
import MidEllipsis from '@/components/common/MidEllipsis.vue'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import PlayerShell from '@/components/common/PlayerShell.vue'
import { toAppError } from '@/api/call'
import { getPreviewURL, getSourcePreviewURL, openSourceWithSystem, openWithSystem, type PreviewURL } from '@/api/convertRecords'
import { MOCK_UNPLAYABLE_URL, MOCK_URL_PREFIX } from '@/api/convertRecordsMock'
import { metaInfoOf, useConvertRecordsStore } from '@/stores/convertRecords'
import { usedDeviceText, useEncoderDeviceList } from '@/api/encoderTask'
import { channelText, sampleRateText, videoCodecText } from '@/utils/mediaText'
import { formatRecordTime, isAudioContainer, isAudioOnly, isHevcCodec, recordLine, REVEAL_LABEL, unplayableHint } from '@/utils/convertText'
import { fileBaseName, formatBytes, formatShortClock } from '@/utils/format'
import { PREVIEW_COPYING_TEXT } from '@/utils/convertSubmit'

export interface PreviewTarget {
  kind: 'source' | 'record'
  id: string
}
const props = defineProps<{ target: PreviewTarget | null; narrow: boolean }>()
const emit = defineEmits<{ close: [] }>()
const cv = useConvertRecordsStore()
const devices = useEncoderDeviceList()

type Stage = 'loading' | 'ready' | 'unplayable' | 'gone' | 'error'
const stage = ref<Stage>('loading')
const errText = ref('')
const url = ref<PreviewURL | null>(null)
const refetched = ref(false)
const playing = ref(false)
const muted = ref(readMuted())
const current = ref(0)
const duration = ref(0)
const volume = ref(0.7)
const mediaEl = ref<HTMLMediaElement | null>(null)
const gifEl = ref<HTMLImageElement | null>(null)
const gifCanvas = ref<HTMLCanvasElement | null>(null)
const playerWrap = ref<HTMLElement | null>(null)
const box = ref<HTMLElement | null>(null)
let returnTo: HTMLElement | null = null
let mockTimer: ReturnType<typeof setInterval> | undefined

function readMuted(): boolean {
  try {
    return globalThis.localStorage?.getItem('ffmpegfree.convert.preview.muted') === '1'
  } catch {
    return false
  }
}
watch(muted, (m) => {
  try {
    globalThis.localStorage?.setItem('ffmpegfree.convert.preview.muted', m ? '1' : '0')
  } catch {
    /* 存不下就算了 */
  }
  if (mediaEl.value) mediaEl.value.muted = m
})

// ---- 显示的数据 ----
const src = computed(() => {
  const t = props.target
  if (!t) return null
  if (t.kind === 'source') return cv.sources[t.id] ?? null
  const r = cv.records[t.id]
  return r ? cv.sources[r.sourceId] ?? null : null
})
const rec = computed(() => (props.target?.kind === 'record' ? cv.liveById.get(props.target.id) ?? null : null))
const isRecord = computed(() => props.target?.kind === 'record')
const srcInfo = computed(() => (src.value ? metaInfoOf(src.value) : undefined))
const container = computed(() => {
  if (rec.value) return (rec.value.options.container || fileBaseName(rec.value.outputPath).split('.').pop() || '').toLowerCase()
  return (src.value?.name.split('.').pop() ?? '').toLowerCase()
})
const FMT = computed(() => container.value.toUpperCase())
/** 媒体类型：先看接口给的 mime，没有就按格式猜 */
const kind = computed<'video' | 'audio' | 'gif'>(() => {
  const m = url.value?.mime ?? ''
  if (m === 'image/gif' || container.value === 'gif') return 'gif'
  if (m.startsWith('audio/')) return 'audio'
  if (m.startsWith('video/')) return 'video'
  if (rec.value) return isAudioContainer(container.value) ? 'audio' : 'video'
  return srcInfo.value && isAudioOnly(srcInfo.value) ? 'audio' : isAudioContainer(container.value) ? 'audio' : 'video'
})
const title = computed(() => (rec.value ? fileBaseName(rec.value.outputPath) : src.value?.name ?? ''))
function srcFacts(withDur: boolean): string[] {
  const i = srcInfo.value
  if (!i) return []
  const audio = isAudioOnly(i)
  const parts = audio ? [sampleRateText(i.sampleRate), channelText(i.channels)] : [i.width ? `${i.width}×${i.height}` : '', videoCodecText(i)]
  if (withDur) parts.push(formatShortClock(i.duration ?? 0))
  parts.push(i.size ? formatBytes(i.size) : '')
  return parts.filter(Boolean)
}
const subtitle = computed(() => {
  if (!isRecord.value) return ['源文件', FMT.value, ...srcFacts(true)].filter(Boolean).join(' · ')
  const r = rec.value?.result
  const audio = isAudioContainer(container.value)
  const parts = ['转换结果', FMT.value]
  if (r) {
    if (audio) parts.push(r.audioBitrateKbps ? `${r.audioBitrateKbps} kbps` : '')
    else parts.push(r.width && r.height ? `${r.width}×${r.height}` : '')
    parts.push(r.durationSec ? formatShortClock(r.durationSec) : '', r.sizeBytes ? formatBytes(r.sizeBytes) : '')
  }
  return parts.filter(Boolean).join(' · ')
})
const srcLine = computed(() => (isRecord.value && src.value ? srcFacts(false).join(' · ') : ''))
const srcAudio = computed(() => !!srcInfo.value && isAudioOnly(srcInfo.value))
/** 底栏：源文件 = 路径（省略中间）；结果 = “{时间} 转换 · 预设名 · 设备”（§六、§7.3 第 14 条） */
const footMeta = computed(() => {
  if (!isRecord.value) {
    const p = src.value?.path ?? ''
    return { text: p.length > 72 ? `${p.slice(0, 30)}…${p.slice(-38)}` : p, title: p }
  }
  const r = rec.value
  if (!r) return { text: '', title: '' }
  return recordLine(r, [`${formatRecordTime(r.createdAt)} 转换`], [usedDeviceText(r, devices.value)]) // 只用快照，和任务中心一致
})
const cover = computed(() => {
  const t = rec.value ? cv.recThumbs.get(rec.value.id) : src.value?.thumb
  return t?.kind === 'img' ? t.url : ''
})
/** 复验 N1：文件本身是 H.265 时，建议改成“转成 MP4 · H.264 后再预览” */
const hevc = computed(() => {
  const r = rec.value
  if (r) return isHevcCodec(r.options.videoCodec) || /H\.265/.test(r.paramsSummary ?? '')
  return isHevcCodec(srcInfo.value?.videoCodec)
})
const unplayableText = computed(() => unplayableHint(hevc.value))
const isMock = computed(() => !!url.value && url.value.url.startsWith(MOCK_URL_PREFIX))
const playable = computed(() => stage.value === 'ready')
/** 放不了 / 文件不在 / 出错：舞台显示原因，控制条整条收起（走查 X5：失败时播放键、时间、音量仍可点） */
const failed = computed(() => stage.value === 'unplayable' || stage.value === 'gone' || stage.value === 'error')
const parts = computed(() =>
  kind.value === 'gif' || failed.value ? { step: false, time: false, seek: false, mute: false, fullscreen: false } : { step: false, fullscreen: kind.value === 'video' },
)
/** 确定有画面的文件：结果是视频格式；源文件读到过宽度（没读到信息的不判断，避免把纯音频的 mp4 当成放不了） */
const expectsVideo = computed(() => (rec.value ? !isAudioContainer(container.value) : !!srcInfo.value && !isAudioOnly(srcInfo.value)))

// ---- 取地址 ----
let seq = 0
function errReason(e: unknown): { code: string; reason: string; message: string } {
  const a = toAppError(e)
  const m = /^reason=(\w+)/.exec(a.detail ?? '')
  return { code: a.code, reason: m?.[1] ?? '', message: a.message }
}
async function fetchUrl(): Promise<PreviewURL> {
  const t = props.target!
  return t.kind === 'source' ? getSourcePreviewURL(t.id) : getPreviewURL(t.id, 'output')
}
function markGone() {
  const t = props.target
  if (!t) return
  stage.value = 'gone'
  if (t.kind === 'source') cv.markSourceGone(t.id)
  else cv.markOutputGone(t.id)
}
/** 地址失效（token 过期 / 服务重启）时后端返回 404；只对真实 http 地址检查 */
async function is404(u: string): Promise<boolean> {
  if (!/^https?:/i.test(u)) return false
  try {
    const r = await fetch(u, { method: 'HEAD' })
    return r.status === 404
  } catch {
    return false
  }
}
async function load() {
  const my = ++seq
  stage.value = 'loading'
  url.value = null
  refetched.value = false
  playing.value = false
  current.value = 0
  duration.value = 0
  stopMock()
  try {
    let u = await fetchUrl()
    if (my !== seq) return
    if (await is404(u.url)) {
      refetched.value = true
      u = await fetchUrl()
      if (my !== seq) return
      if (await is404(u.url)) return markGone()
    }
    url.value = u
    if (u.url === MOCK_UNPLAYABLE_URL) {
      stage.value = 'unplayable'
      return
    }
    stage.value = 'ready'
    duration.value = mockDuration()
    await nextTick()
    if (isMock.value) startMockPlay()
  } catch (e) {
    if (my !== seq) return
    const r = errReason(e)
    if (r.reason === 'copying') {
      stage.value = 'error' // 副本还没复制好（走查 D4）：不管后端原话和错误码，统一说“准备中”
      errText.value = PREVIEW_COPYING_TEXT
    } else if (r.code === 'NOT_FOUND' && r.reason === 'file') markGone()
    else if (r.code === 'NOT_FOUND' && r.reason === 'record') {
      emit('close')
      cv.say('这条转换记录已被删除')
    } else if (r.code === 'UNSUPPORTED') stage.value = 'unplayable'
    else {
      stage.value = 'error'
      errText.value = r.message
    }
  }
}
/** <video>/<audio>/<img> 出错：先看是不是地址失效（重新取一次），否则当作应用内放不了 */
async function onMediaError() {
  if (stage.value !== 'ready' || !url.value || isMock.value) return
  if (!refetched.value && (await is404(url.value.url))) {
    refetched.value = true
    try {
      const u = await fetchUrl()
      if (await is404(u.url)) return markGone()
      url.value = u
      return
    } catch (e) {
      const r = errReason(e)
      if (r.code === 'NOT_FOUND' && r.reason === 'file') return markGone()
    }
  }
  playing.value = false
  stage.value = 'unplayable'
}

// ---- 模拟播放（浏览器预览：mock: 地址没有真实媒体） ----
function mockDuration(): number {
  if (!isMock.value) return 0
  return rec.value?.result?.durationSec || srcInfo.value?.duration || 60
}
function startMockPlay() {
  playing.value = true
}
function stopMock() {
  clearInterval(mockTimer)
  mockTimer = undefined
}
watch([playing, isMock], ([p, m]) => {
  stopMock()
  if (p && m && kind.value !== 'gif') {
    mockTimer = setInterval(() => {
      current.value = Math.min(duration.value, current.value + 0.25)
      if (current.value >= duration.value) playing.value = false
    }, 250)
  }
})

// ---- 真实媒体元素 ----
function onMeta() {
  const el = mediaEl.value
  if (!el) return
  // 走查 G4：ProRes 等 WebView 解不了画面的编码不会报错，只是 videoWidth = 0、黑屏而时间照走 → 按“无法在应用内播放”处理
  if (kind.value === 'video' && expectsVideo.value && (el as HTMLVideoElement).videoWidth === 0) {
    el.pause()
    playing.value = false
    stage.value = 'unplayable'
    return
  }
  duration.value = isFinite(el.duration) ? el.duration : 0
  el.muted = muted.value
  el.volume = volume.value
  playing.value = true // 打开时自动播放
}
function onTime() {
  if (mediaEl.value) current.value = mediaEl.value.currentTime
}
watch(playing, async (p) => {
  const el = mediaEl.value
  if (kind.value === 'gif') return freezeGif(!p)
  if (!el || isMock.value) return
  if (p && el.paused) {
    try {
      await el.play()
    } catch {
      playing.value = false
    }
  } else if (!p && !el.paused) el.pause()
})
function onSeek(t: number) {
  if (mediaEl.value && !isMock.value) mediaEl.value.currentTime = t
  current.value = t
}
function seekBy(d: number) {
  if (!playable.value || kind.value === 'gif') return
  onSeek(Math.min(duration.value || 0, Math.max(0, current.value + d)))
}
/** GIF 暂停：把当前帧画到 canvas 上盖住动图 */
const gifFrozen = ref(false)
function freezeGif(on: boolean) {
  const img = gifEl.value
  const c = gifCanvas.value
  if (on && img && c && img.naturalWidth) {
    c.width = img.naturalWidth
    c.height = img.naturalHeight
    c.getContext('2d')?.drawImage(img, 0, 0)
  }
  gifFrozen.value = on
}
function setVolume(e: MouseEvent) {
  const el = e.currentTarget as HTMLElement
  const r = el.getBoundingClientRect()
  volume.value = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width))
  if (mediaEl.value) mediaEl.value.volume = volume.value
  if (volume.value > 0 && muted.value) muted.value = false
}
function fullscreen() {
  const el = playerWrap.value
  if (!el || kind.value !== 'video') return
  if (document.fullscreenElement) void document.exitFullscreen()
  else void el.requestFullscreen?.().catch(() => {})
}

// ---- 动作 ----
async function openSystem() {
  const t = props.target
  if (!t) return
  try {
    if (t.kind === 'source') await openSourceWithSystem(t.id)
    else await openWithSystem(t.id, 'output')
  } catch (e) {
    const r = errReason(e)
    if (r.code === 'NOT_FOUND' && r.reason === 'no_app') cv.say('没有找到能打开这个文件的程序')
    else if (r.code === 'NOT_FOUND' && r.reason === 'file') markGone()
    else cv.say(r.message)
  }
}
async function reveal() {
  const t = props.target
  if (!t) return
  const ok = t.kind === 'source' ? await cv.revealSource(t.id) : await cv.revealOutput(t.id)
  if (!ok) {
    stage.value = 'gone'
    cv.say('文件已被移动或删除')
  }
}

// ---- 打开 / 关闭、快捷键、焦点 ----
function onKey(e: KeyboardEvent) {
  if (!props.target) return
  const tgt = e.target as HTMLElement
  if (e.key === 'Escape') {
    if (document.fullscreenElement) return
    e.preventDefault()
    emit('close')
    return
  }
  if (tgt.closest('.seek') && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) return // 进度条自己处理
  if (e.key === ' ' || e.code === 'Space') {
    e.preventDefault()
    if (playable.value) playing.value = !playing.value
  } else if (e.key === 'ArrowLeft') {
    e.preventDefault()
    seekBy(-5)
  } else if (e.key === 'ArrowRight') {
    e.preventDefault()
    seekBy(5)
  } else if ((e.key === 'f' || e.key === 'F') && !e.ctrlKey && !e.metaKey && !e.altKey) {
    e.preventDefault()
    fullscreen()
  } else if (e.key === 'Tab' && box.value) {
    const f = Array.from(box.value.querySelectorAll<HTMLElement>('button:not([disabled]), [tabindex="0"]'))
    if (!f.length) return
    const i = f.indexOf(document.activeElement as HTMLElement)
    if (e.shiftKey && i <= 0) {
      e.preventDefault()
      f[f.length - 1].focus()
    } else if (!e.shiftKey && i === f.length - 1) {
      e.preventDefault()
      f[0].focus()
    }
  }
}
watch(
  () => props.target,
  async (t, old) => {
    if (t && !old) {
      returnTo = document.activeElement as HTMLElement | null
      window.addEventListener('keydown', onKey, true)
    }
    if (!t) {
      seq++
      stopMock()
      playing.value = false
      if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
      window.removeEventListener('keydown', onKey, true)
      const el = returnTo
      returnTo = null
      if (el?.isConnected) el.focus()
      return
    }
    void load()
    await nextTick()
    box.value?.focus()
  },
)
onBeforeUnmount(() => {
  stopMock()
  window.removeEventListener('keydown', onKey, true)
})
</script>
<template>
  <Teleport to="body">
    <div v-if="target" class="cv2 cv-layer" :class="{ w1024: narrow }">
      <div class="cv-mask" @click.self="emit('close')">
        <div ref="box" class="cv-pv" :class="{ audio: kind === 'audio', gif: kind === 'gif' }" role="dialog" aria-modal="true" :aria-label="isRecord ? `预览转换结果 ${title}` : `预览 ${title}`" tabindex="-1">
          <div class="cv-pvh">
            <div class="tt">
              <MidEllipsis tag="h3" :text="title" />
              <div class="sub" :title="subtitle">{{ subtitle }}</div>
              <div v-if="isRecord && src" class="cv-srcline"><FIcon :name="srcAudio ? 'music' : 'film'" /><span :title="src.path">源文件：<b>{{ src.name }}</b><template v-if="srcLine"> · {{ srcLine }}</template></span></div>
            </div>
            <button type="button" class="x" aria-label="关闭" title="关闭" @click="emit('close')"><FIcon name="x" /></button>
          </div>
          <div ref="playerWrap" class="cv-pvbody" :class="{ nobar: failed }">
            <PlayerShell
              v-model:playing="playing"
              v-model:muted="muted"
              v-model:current="current"
              v-model:duration="duration"
              fill
              time-format="clock"
              :parts="parts"
              :disabled="!playable"
              @seek="onSeek"
              @fullscreen="fullscreen"
            >
              <template v-if="playable && url">
                <template v-if="isMock">
                  <div v-if="kind !== 'audio'" class="cv-sample moving" :class="{ paused: !playing }"><div class="sun" /><div class="m1" /><div class="m2" /></div>
                </template>
                <video v-else-if="kind === 'video'" ref="mediaEl" class="media" :src="url.url" preload="metadata" playsinline @loadedmetadata="onMeta" @timeupdate="onTime" @ended="playing = false" @error="onMediaError" />
                <template v-else-if="kind === 'gif'">
                  <img v-show="!gifFrozen" ref="gifEl" class="media" :src="url.url" alt="" @error="onMediaError" @load="playing = true" />
                  <canvas v-show="gifFrozen" ref="gifCanvas" class="media" />
                </template>
              </template>
              <template #overlay>
                <audio v-if="playable && url && kind === 'audio' && !isMock" ref="mediaEl" class="cv-ph-hidden" :src="url.url" preload="metadata" @loadedmetadata="onMeta" @timeupdate="onTime" @ended="playing = false" @error="onMediaError" />
                <div v-if="kind === 'audio' && stage === 'ready'" class="cv-astage solo" style="position: absolute; inset: 0">
                  <div class="cv-cover" :style="cover ? { background: `center / cover no-repeat url(${cover})` } : undefined"><FIcon v-if="!cover" name="music" :size="40" /></div>
                </div>
                <span v-if="isRecord && stage === 'ready' && kind !== 'audio'" class="cv-chip">结果 · {{ FMT }}</span>
                <div v-if="stage === 'loading'" class="cv-loading">正在加载…</div>
                <div v-else-if="stage === 'unplayable'" class="cv-perr">
                  <div class="in">
                    <div class="eic"><FIcon name="info" /></div>
                    <h5>无法在应用内播放</h5>
                    <p>{{ unplayableText }}</p>
                    <div class="acts">
                      <button type="button" class="btn pri" @click="openSystem"><FIcon name="play" />用系统播放器打开</button>
                      <button type="button" class="btn" @click="reveal"><FIcon name="folder" />{{ REVEAL_LABEL }}</button>
                    </div>
                  </div>
                </div>
                <div v-else-if="stage === 'gone'" class="cv-perr">
                  <div class="in"><div class="eic bad"><FIcon name="warn" /></div><h5>文件已被移动或删除</h5></div>
                </div>
                <div v-else-if="stage === 'error'" class="cv-perr">
                  <div class="in"><div class="eic bad"><FIcon name="warn" /></div><h5>无法预览</h5><p>{{ errText }}</p></div>
                </div>
              </template>
              <template #extra>
                <button v-if="kind !== 'gif' && !failed" type="button" class="cv-vol" :style="{ '--v': (muted ? 0 : volume * 100) + '%' }" aria-label="音量" @click="setVolume"><i /></button>
              </template>
            </PlayerShell>
          </div>
          <div class="cv-pvf">
            <span class="meta" :title="footMeta.title">{{ footMeta.text }}</span>
            <span class="sp" />
            <button v-if="stage !== 'unplayable' && stage !== 'gone'" type="button" class="btn" @click="reveal"><FIcon name="folder" />打开所在文件夹</button>
            <button type="button" class="btn" @click="emit('close')">关闭</button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
