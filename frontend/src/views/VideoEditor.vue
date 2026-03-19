<template>
  <div class="editor-workbench">
    <section class="panel topbar">
      <div class="title-wrap">
        <div class="logo">剪</div>
        <div>
          <div class="title">视频剪辑工作台</div>
          <div class="sub">{{ renderConfig.outputName || '未命名工程' }}</div>
        </div>
      </div>
      <div class="tool-list">
        <button class="tool-btn" :class="{ active: activeTopTool === item }" v-for="item in topTools" :key="item" @click="onTopToolClick(item)">{{ item }}</button>
      </div>
      <div class="actions">
        <el-button :loading="loadingSources" @click="fetchSources">刷新素材</el-button>
        <el-button type="primary" :loading="rendering" @click="renderProject">导出</el-button>
      </div>
    </section>

    <section class="panel config-bar">
      <el-input v-model="renderConfig.outputName" class="name-input" placeholder="输出文件名" />
      <el-select v-model="renderConfig.outputFormat" class="small-input">
        <el-option label="MP4" value="mp4" />
        <el-option label="MOV" value="mov" />
        <el-option label="MKV" value="mkv" />
        <el-option label="WEBM" value="webm" />
      </el-select>
      <el-input-number v-model="renderConfig.width" :min="320" :max="3840" :step="2" />
      <span>x</span>
      <el-input-number v-model="renderConfig.height" :min="240" :max="2160" :step="2" />
      <el-input-number v-model="renderConfig.fps" :min="12" :max="60" :step="1" />
    </section>

    <section class="workspace">
      <aside class="panel asset-panel">
        <div class="panel-head">
          <span>素材库</span>
          <el-select v-model="assetKindFilter" class="small-input">
            <el-option label="全部" value="all" />
            <el-option label="视频" value="video" />
            <el-option label="音频" value="audio" />
          </el-select>
        </div>
        <div class="target-row">
          <el-select v-model="activeVideoTrackId" class="small-input">
            <el-option v-for="track in videoTracks" :key="track.id" :label="track.id" :value="track.id" />
          </el-select>
          <el-select v-model="activeAudioTrackId" class="small-input">
            <el-option v-for="track in audioTracks" :key="track.id" :label="track.id" :value="track.id" />
          </el-select>
        </div>
        <el-input ref="assetSearchInputRef" v-model="assetSearch" clearable placeholder="搜索素材" />
        <div class="asset-grid">
          <div class="asset-card" v-for="item in filteredSources" :key="`${item.scope}-${item.name}`" @dblclick="quickInsert(item)">
            <div class="cover" :class="sourceKind(item)">
              <video v-if="sourceKind(item) === 'video'" :src="item.url" muted preload="metadata" />
              <div v-else class="audio-cover"></div>
              <span class="duration">{{ item.duration }}</span>
            </div>
            <div class="asset-name">{{ item.name }}</div>
            <div class="row-btn">
              <el-button link type="primary" @click="previewSource(item)">预览</el-button>
              <el-button link @click="addToVideoTrack(item)">视频轨</el-button>
              <el-button link @click="addToAudioTrack(item)">音轨</el-button>
            </div>
          </div>
        </div>
      </aside>

      <main class="panel monitor-panel">
        <div class="monitor-head">
          <div>
            <div class="panel-title">监看器</div>
            <div class="sub">播放头: {{ formatTime(playheadSec) }} / {{ formatTime(totalDurationSec) }} · {{ timelineStatsText }}</div>
          </div>
          <div class="row-btn">
            <el-button @click="jumpBy(-1)">-1s</el-button>
            <el-button type="primary" @click="togglePlay">{{ isPlaying ? '暂停' : '播放' }}</el-button>
            <el-button @click="jumpBy(1)">+1s</el-button>
            <el-button @click="stopPlayback">停止</el-button>
          </div>
        </div>

        <div class="preview-wrap">
          <canvas ref="monitorCanvasRef" class="preview-canvas" :style="previewVideoStyle"></canvas>
          <video ref="decoderVideoRef" class="decoder-video" muted playsinline preload="auto"></video>
          <div class="safe-area">Safe Area</div>
          <div class="badge">
            {{
              activeVideoSegment
                ? `${activeVideoSegment.clip.trackId} · #${activeVideoSegment.index + 1} · ${activeVideoSegment.clip.fileName}`
                : '无激活片段'
            }}
          </div>
        </div>

        <div class="slider-grid">
          <div>
            <label>亮度</label>
            <el-slider v-model="renderConfig.effects.brightness" :min="-0.5" :max="0.5" :step="0.01" @change="syncPreviewByPlayhead" />
          </div>
          <div>
            <label>对比度</label>
            <el-slider v-model="renderConfig.effects.contrast" :min="0.5" :max="2" :step="0.01" @change="syncPreviewByPlayhead" />
          </div>
          <div>
            <label>饱和度</label>
            <el-slider v-model="renderConfig.effects.saturation" :min="0" :max="2" :step="0.01" @change="syncPreviewByPlayhead" />
          </div>
          <div>
            <label>锐化（导出生效）</label>
            <el-slider v-model="renderConfig.effects.sharpen" :min="0" :max="2" :step="0.05" />
          </div>
        </div>

        <div class="diag-box">
          <div class="panel-head">
            <span>监看器自检</span>
            <el-button size="small" @click="runMonitorDiagnostics">运行校验</el-button>
          </div>
          <div v-if="monitorChecks.length === 0" class="sub">尚未执行</div>
          <div v-for="item in monitorChecks" :key="item.label" class="diag-item" :class="item.pass ? 'ok' : 'fail'">
            <span>{{ item.pass ? 'PASS' : 'FAIL' }} · {{ item.label }}</span>
            <span>{{ item.detail }}</span>
          </div>
        </div>
      </main>

      <aside class="panel inspector-panel">
        <div class="panel-title">检查器</div>
        <div v-if="selectedType === 'video' && selectedVideoClip">
          <el-form label-position="top">
            <el-form-item label="轨道">
              <el-select v-model="selectedVideoClip.trackId" @change="syncPreviewByPlayhead">
                <el-option v-for="track in videoTracks" :key="track.id" :label="track.id" :value="track.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="时间线起点(s)">
              <el-input-number v-model="selectedVideoClip.startSec" :min="0" :step="0.05" @change="syncPreviewByPlayhead" />
            </el-form-item>
            <el-form-item label="入点(s)">
              <el-input-number v-model="selectedVideoClip.inSec" :min="0" :step="0.05" @change="syncPreviewByPlayhead" />
            </el-form-item>
            <el-form-item label="出点(s)">
              <el-input-number v-model="selectedVideoClip.outSec" :min="0" :step="0.05" @change="syncPreviewByPlayhead" />
            </el-form-item>
            <el-form-item label="速度">
              <el-input-number v-model="selectedVideoClip.speed" :min="0.25" :max="4" :step="0.05" @change="syncPreviewByPlayhead" />
            </el-form-item>
            <el-form-item label="特效">
              <el-select v-model="selectedVideoClip.effectPreset" @change="syncPreviewByPlayhead">
                <el-option label="无" value="none" />
                <el-option label="黑白" value="grayscale" />
                <el-option label="棕褐" value="sepia" />
                <el-option label="复古" value="vintage" />
                <el-option label="电影感" value="cinematic" />
              </el-select>
            </el-form-item>
            <el-form-item label="到下一个片段转场">
              <el-select v-model="selectedVideoClip.transitionToNext">
                <el-option v-for="option in transitionOptions" :key="option.value" :label="option.label" :value="option.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="转场时长(s)">
              <el-input-number
                v-model="selectedVideoClip.transitionDurationSec"
                :min="0.1"
                :max="2"
                :step="0.05"
                :disabled="selectedVideoClip.transitionToNext === 'none'"
              />
            </el-form-item>
            <el-form-item label="模糊">
              <el-input-number v-model="selectedVideoClip.blur" :min="0" :max="4" :step="0.1" @change="syncPreviewByPlayhead" />
            </el-form-item>
          </el-form>
        </div>
        <div v-else-if="selectedType === 'audio' && selectedAudioClip">
          <el-form label-position="top">
            <el-form-item label="轨道">
              <el-select v-model="selectedAudioClip.trackId">
                <el-option v-for="track in audioTracks" :key="track.id" :label="track.id" :value="track.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="时间线起点(s)">
              <el-input-number v-model="selectedAudioClip.startSec" :min="0" :step="0.05" />
            </el-form-item>
            <el-form-item label="入点(s)">
              <el-input-number v-model="selectedAudioClip.inSec" :min="0" :step="0.05" />
            </el-form-item>
            <el-form-item label="出点(s)">
              <el-input-number v-model="selectedAudioClip.outSec" :min="0" :step="0.05" />
            </el-form-item>
            <el-form-item label="速度">
              <el-input-number v-model="selectedAudioClip.speed" :min="0.25" :max="4" :step="0.05" />
            </el-form-item>
            <el-form-item label="音量">
              <el-input-number v-model="selectedAudioClip.volume" :min="0" :max="4" :step="0.05" />
            </el-form-item>
            <el-form-item label="延迟(s)">
              <el-input-number v-model="selectedAudioClip.delaySec" :min="0" :step="0.05" />
            </el-form-item>
          </el-form>
        </div>
        <div v-else class="sub">选择片段后在这里编辑参数</div>
      </aside>
    </section>

    <section class="panel timeline-panel">
      <div class="panel-head">
        <div>
          <div class="panel-title">时间线</div>
          <div class="sub">支持拖动、裁剪、多轨手动添加和按时间导出</div>
        </div>
        <div class="row-btn">
          <el-button @click="addVideoTrackSlot">+ 视频轨</el-button>
          <el-button @click="addAudioTrackSlot">+ 音轨</el-button>
          <span>缩放</span>
          <el-slider v-model="timelineScale" :min="24" :max="100" :step="2" style="width: 150px" />
        </div>
      </div>

      <div class="timeline-scroll" ref="timelineScrollRef">
        <div class="timeline-canvas" :style="{ width: `${timelineCanvasWidth}px` }">
          <div class="ruler-row">
            <div class="track-label">时间</div>
            <div class="ruler-lane" :style="{ width: `${timelineContentWidth}px` }" @click="onRulerClick" @mousedown.left.prevent="beginScrub">
              <div v-for="tick in timelineTicks" :key="tick.time" class="tick" :style="{ left: `${tick.time * timelineScale}px` }">
                <span v-if="tick.major">{{ formatTime(tick.time) }}</span>
              </div>
            </div>
          </div>

          <div class="track-row" v-for="track in videoTracks" :key="track.id">
            <div class="track-label" :class="{ active: activeVideoTrackId === track.id }" @click="activeVideoTrackId = track.id">{{ track.id }}</div>
            <div class="track-lane" :style="{ width: `${timelineContentWidth}px` }" :data-kind="'video'" :data-track-id="track.id" @click="onLaneClick($event, 'video', track.id)" @mousedown.left.prevent="beginScrub">
              <div
                v-for="segment in videoSegmentsByTrack(track.id)"
                :key="`v-${segment.index}-${segment.startSec}`"
                class="clip video"
                :class="{ active: selectedType === 'video' && selectedIndex === segment.index }"
                :style="clipStyle(segment.startSec, segment.durationSec)"
                @click.stop="selectVideoSegment(segment.index)"
                @mousedown.stop="beginMove('video', segment.index, $event)"
              >
                <div class="trim left" @mousedown.stop="beginTrim('video', segment.index, 'left', $event)"></div>
                <div class="clip-title">{{ segment.clip.fileName }}</div>
                <div class="clip-meta">{{ segment.clip.effectPreset }} · {{ segment.clip.speed.toFixed(2) }}x · {{ formatTransitionMeta(segment.clip) }}</div>
                <div class="trim right" @mousedown.stop="beginTrim('video', segment.index, 'right', $event)"></div>
              </div>
            </div>
          </div>

          <div class="track-row audio" v-for="track in audioTracks" :key="track.id">
            <div class="track-label" :class="{ active: activeAudioTrackId === track.id }" @click="activeAudioTrackId = track.id">{{ track.id }}</div>
            <div class="track-lane" :style="{ width: `${timelineContentWidth}px` }" :data-kind="'audio'" :data-track-id="track.id" @click="onLaneClick($event, 'audio', track.id)" @mousedown.left.prevent="beginScrub">
              <div
                v-for="segment in audioSegmentsByTrack(track.id)"
                :key="`a-${segment.index}-${segment.startSec}`"
                class="clip audio"
                :class="{ active: selectedType === 'audio' && selectedIndex === segment.index }"
                :style="clipStyle(segment.startSec, segment.durationSec)"
                @click.stop="selectAudioSegment(segment.index)"
                @mousedown.stop="beginMove('audio', segment.index, $event)"
              >
                <div class="trim left" @mousedown.stop="beginTrim('audio', segment.index, 'left', $event)"></div>
                <div class="clip-title">{{ segment.clip.fileName }}</div>
                <div class="clip-meta">Vol {{ segment.clip.volume.toFixed(2) }}</div>
                <div class="trim right" @mousedown.stop="beginTrim('audio', segment.index, 'right', $event)"></div>
              </div>
            </div>
          </div>

          <div class="playhead" :style="{ transform: `translateX(${TRACK_LABEL_WIDTH + playheadSec * timelineScale}px)`, height: `${playheadHeight}px` }" @mousedown.left.prevent.stop="beginScrub">
            <div class="dot"></div>
          </div>
        </div>
      </div>

      <div class="row-btn">
        <el-button :disabled="selectedType !== 'video'" type="danger" @click="removeSelectedVideo">删除视频片段</el-button>
        <el-button :disabled="selectedType !== 'audio'" type="danger" @click="removeSelectedAudio">删除音频片段</el-button>
      </div>
    </section>

    <section class="panel export-panel">
      <div class="panel-title">导出结果</div>
      <video v-if="resultUrl" :src="resultUrl" controls class="result-video" />
      <div v-else class="sub">尚未导出</div>
      <el-link v-if="resultUrl" :href="resultUrl" target="_blank" type="primary">下载导出文件</el-link>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getEditSources,
  renderEditProject,
  type AudioTrackClip,
  type EditRenderRequest,
  type EditSourceItem,
  type VideoTrackClip,
} from '@/api/editor/editor'

type SourceScope = 'user' | 'steam' | 'converted'
type TrackKind = 'video' | 'audio'
type TrimSide = 'left' | 'right'
type TopTool = '素材' | '文本' | '贴纸' | '转场' | '特效' | '字幕' | '音频' | '调色' | '模板'

interface TrackSlot { id: string }
interface VideoSegment {
  index: number
  clip: VideoTrackClip
  source: EditSourceItem | undefined
  startSec: number
  endSec: number
  durationSec: number
  trackOrder: number
}
interface AudioSegment {
  index: number
  clip: AudioTrackClip
  startSec: number
  endSec: number
  durationSec: number
}
interface DragState {
  kind: TrackKind
  index: number
  mode: 'move' | 'trim-left' | 'trim-right'
  startClientX: number
  initialStart: number
  initialIn: number
  initialOut: number
  speed: number
  sourceDuration: number
}
interface MonitorCheckItem { label: string; pass: boolean; detail: string }

const TRACK_LABEL_WIDTH = 88
const RULER_HEIGHT = 34
const VIDEO_ROW_HEIGHT = 72
const AUDIO_ROW_HEIGHT = 62

const topTools: TopTool[] = ['素材', '文本', '贴纸', '转场', '特效', '字幕', '音频', '调色', '模板']
const transitionOptions: Array<{ label: string; value: VideoTrackClip['transitionToNext'] }> = [
  { label: '无', value: 'none' },
  { label: '淡入淡出', value: 'fade' },
  { label: '向左擦除', value: 'wipeleft' },
  { label: '向右擦除', value: 'wiperight' },
  { label: '向左滑动', value: 'slideleft' },
  { label: '向右滑动', value: 'slideright' },
  { label: '圆形打开', value: 'circleopen' },
  { label: '圆形关闭', value: 'circleclose' },
  { label: '溶解', value: 'dissolve' },
]
const sources = ref<EditSourceItem[]>([])
const loadingSources = ref(false)
const rendering = ref(false)
const activeTopTool = ref<TopTool>('素材')
const assetSearch = ref('')
const assetSearchInputRef = ref<{ focus?: () => void } | null>(null)
const assetKindFilter = ref<'all' | 'video' | 'audio'>('all')
const videoTracks = ref<TrackSlot[]>([{ id: 'V1' }])
const audioTracks = ref<TrackSlot[]>([{ id: 'A1' }])
const activeVideoTrackId = ref('V1')
const activeAudioTrackId = ref('A1')
const resultUrl = ref('')
const monitorChecks = ref<MonitorCheckItem[]>([])
const monitorCanvasRef = ref<HTMLCanvasElement | null>(null)
const decoderVideoRef = ref<HTMLVideoElement | null>(null)
const previewSourceUrl = ref('')
const isPlaying = ref(false)
const playheadSec = ref(0)
const timelineScale = ref(44)
const timelineScrollRef = ref<HTMLDivElement | null>(null)
const selectedType = ref<'none' | 'video' | 'audio'>('none')
const selectedIndex = ref(-1)
const rafId = ref<number | null>(null)
const lastActiveVideoIndex = ref(-1)
const dragState = ref<DragState | null>(null)
const isScrubbing = ref(false)
const decoderMetaReady = ref(false)
const renderTaskID = ref(0)
const lastUiSyncTs = ref(0)
const lastFollowScrollTs = ref(0)
const lastDrawMediaSec = ref(-1)

const renderConfig = reactive<EditRenderRequest>({
  outputName: 'timeline_cut',
  outputFormat: 'mp4',
  width: 1280,
  height: 720,
  fps: 30,
  videoTrack: [],
  audioTrack: [],
  effects: { brightness: 0, contrast: 1, saturation: 1, sharpen: 0 },
})

// parseDurationToSec 将 "HH:MM:SS.xx" 转成秒。
const parseDurationToSec = (duration: string): number => {
  const raw = duration.trim()
  if (!raw || raw.toLowerCase() === 'unknown') return 10
  const parts = raw.split(':')
  if (parts.length !== 3) return 10
  const h = Number(parts[0])
  const m = Number(parts[1])
  const s = Number(parts[2])
  if (Number.isNaN(h) || Number.isNaN(m) || Number.isNaN(s)) return 10
  return h * 3600 + m * 60 + s
}

// parseDurationToSecStrict 仅在格式有效时返回真实时长，失败返回 0。
const parseDurationToSecStrict = (duration: string): number => {
  const raw = `${duration || ''}`.trim()
  if (!raw || raw.toLowerCase() === 'unknown') return 0
  const parts = raw.split(':')
  if (parts.length !== 3) return 0
  const h = Number(parts[0])
  const m = Number(parts[1])
  const s = Number(parts[2])
  if (Number.isNaN(h) || Number.isNaN(m) || Number.isNaN(s)) return 0
  const total = h * 3600 + m * 60 + s
  return Number.isFinite(total) && total > 0 ? total : 0
}

const roundSec = (value: number) => Math.round(Math.max(0, value) * 1000) / 1000
const parseTrackOrder = (id: string, fallback = 1) => Number(id.match(/\d+/)?.[0] || fallback)
const frameStepSec = computed(() => 1 / Math.max(1, Math.floor(renderConfig.fps || 30)))
const uiSyncIntervalMs = computed(() => Math.max(16, Math.round(1000 / Math.min(30, Math.max(12, Number(renderConfig.fps || 30))))))
const followScrollIntervalMs = computed(() => Math.max(40, uiSyncIntervalMs.value * 2))
const snapSec = (value: number) => {
  const safe = Math.max(0, value)
  const step = frameStepSec.value
  if (!Number.isFinite(step) || step <= 0) return roundSec(safe)
  return roundSec(Math.round(safe / step) * step)
}

const normalizeTrackId = (kind: TrackKind, value: string) => {
  const prefix = kind === 'video' ? 'V' : 'A'
  const text = value.trim().toUpperCase()
  if (!text) return `${prefix}1`
  return text.startsWith('V') || text.startsWith('A') ? text : `${prefix}${text}`
}

const normalizeTransitionName = (value: VideoTrackClip['transitionToNext'] | string): VideoTrackClip['transitionToNext'] => {
  const raw = `${value || ''}`.trim().toLowerCase()
  const allowed = new Set(transitionOptions.map((item) => item.value))
  if (allowed.has(raw as VideoTrackClip['transitionToNext'])) {
    return raw as VideoTrackClip['transitionToNext']
  }
  return 'none'
}
const sourceKind = (item: EditSourceItem): 'video' | 'audio' => {
  const lower = item.name.toLowerCase()
  if (lower.endsWith('.mp3') || lower.endsWith('.wav') || lower.endsWith('.aac') || lower.endsWith('.m4a') || lower.endsWith('.flac') || lower.endsWith('.ogg')) return 'audio'
  return 'video'
}

const sourceMap = computed(() => {
  const map = new Map<string, EditSourceItem>()
  for (const item of sources.value) map.set(`${item.scope}:${item.name}`, item)
  return map
})

const normalizeSourceScope = (scope: string): SourceScope => {
  const raw = `${scope || ''}`.trim().toLowerCase()
  if (raw === 'steam' || raw === 'converted') return raw
  return 'user'
}

// resolveSource 优先按 scope+name 精确匹配，失败时按文件名兜底，避免历史数据 scope 不一致导致无法预览。
const resolveSource = (fileName: string, scope: SourceScope | string) => {
  const normalizedScope = normalizeSourceScope(scope)
  const exact = sourceMap.value.get(`${normalizedScope}:${fileName}`)
  if (exact) return exact
  return sources.value.find((item) => item.name === fileName)
}

// clipSourceDuration 仅返回可确认的源时长，未知时返回 0，避免错误地把片段强行压到 10 秒。
const clipSourceDuration = (fileName: string, scope: SourceScope | string) => {
  const duration = resolveSource(fileName, scope)?.duration || ''
  return parseDurationToSecStrict(duration)
}

const filteredSources = computed(() => {
  const key = assetSearch.value.trim().toLowerCase()
  return sources.value.filter((item) => {
    const matchSearch = !key || item.name.toLowerCase().includes(key)
    const matchKind = assetKindFilter.value === 'all' || assetKindFilter.value === sourceKind(item)
    return matchSearch && matchKind
  })
})

const ensureTrackSlotsFromClips = () => {
  for (const clip of renderConfig.videoTrack) {
    clip.trackId = normalizeTrackId('video', clip.trackId)
    clip.transitionToNext = normalizeTransitionName(clip.transitionToNext)
    if (clip.transitionToNext === 'none') {
      clip.transitionDurationSec = 0
    } else if (!clip.transitionDurationSec || clip.transitionDurationSec <= 0) {
      clip.transitionDurationSec = 0.5
    }
    if (!videoTracks.value.some((item) => item.id === clip.trackId)) videoTracks.value.push({ id: clip.trackId })
  }
  for (const clip of renderConfig.audioTrack) {
    clip.trackId = normalizeTrackId('audio', clip.trackId)
    if (!audioTracks.value.some((item) => item.id === clip.trackId)) audioTracks.value.push({ id: clip.trackId })
  }
  videoTracks.value.sort((a, b) => parseTrackOrder(a.id) - parseTrackOrder(b.id))
  audioTracks.value.sort((a, b) => parseTrackOrder(a.id) - parseTrackOrder(b.id))
}

const effectiveVideoRange = (clip: VideoTrackClip) => {
  const sourceDuration = clipSourceDuration(clip.fileName, clip.scope)
  const inSec = Math.max(0, clip.inSec)
  // 当源时长未知时，以片段 outSec 为准，防止时间线被错误截断。
  const outSec = sourceDuration > 0
    ? (clip.outSec > inSec ? Math.min(clip.outSec, sourceDuration) : sourceDuration)
    : (clip.outSec > inSec ? clip.outSec : inSec + 10)
  const speed = clip.speed > 0 ? clip.speed : 1
  return { inSec, outSec, speed, duration: Math.max(0.05, (outSec - inSec) / speed) }
}

const effectiveAudioRange = (clip: AudioTrackClip) => {
  const sourceDuration = clipSourceDuration(clip.fileName, clip.scope)
  const inSec = Math.max(0, clip.inSec)
  // 音频规则同视频：源时长未知时保持 outSec，不做错误压缩。
  const outSec = sourceDuration > 0
    ? (clip.outSec > inSec ? Math.min(clip.outSec, sourceDuration) : sourceDuration)
    : (clip.outSec > inSec ? clip.outSec : inSec + 10)
  const speed = clip.speed > 0 ? clip.speed : 1
  return { inSec, outSec, speed, duration: Math.max(0.05, (outSec - inSec) / speed) }
}

// resolveAudioTimelineStart 统一音轨起点规则：优先 startSec，若为空则回退 delaySec。
const resolveAudioTimelineStart = (clip: AudioTrackClip) => {
  const startByDelay = clip.startSec <= 0 && clip.delaySec > 0 ? clip.delaySec : clip.startSec
  return snapSec(Math.max(0, startByDelay))
}

const videoSegments = computed<VideoSegment[]>(() =>
  renderConfig.videoTrack.map((clip, index) => {
    const range = effectiveVideoRange(clip)
    const start = Math.max(0, clip.startSec)
    return {
      index,
      clip,
      source: resolveSource(clip.fileName, clip.scope),
      startSec: start,
      endSec: start + range.duration,
      durationSec: range.duration,
      trackOrder: parseTrackOrder(clip.trackId),
    }
  }),
)

const audioSegments = computed<AudioSegment[]>(() =>
  renderConfig.audioTrack.map((clip, index) => {
    const range = effectiveAudioRange(clip)
    const start = resolveAudioTimelineStart(clip)
    return { index, clip, startSec: start, endSec: start + range.duration, durationSec: range.duration }
  }),
)

const videoSegmentsGrouped = computed(() => {
  const grouped = new Map<string, VideoSegment[]>()
  for (const track of videoTracks.value) grouped.set(track.id, [])
  for (const segment of videoSegments.value) {
    let list = grouped.get(segment.clip.trackId)
    if (!list) {
      list = []
      grouped.set(segment.clip.trackId, list)
    }
    list.push(segment)
  }
  for (const list of grouped.values()) list.sort((a, b) => a.startSec - b.startSec)
  return grouped
})

const audioSegmentsGrouped = computed(() => {
  const grouped = new Map<string, AudioSegment[]>()
  for (const track of audioTracks.value) grouped.set(track.id, [])
  for (const segment of audioSegments.value) {
    let list = grouped.get(segment.clip.trackId)
    if (!list) {
      list = []
      grouped.set(segment.clip.trackId, list)
    }
    list.push(segment)
  }
  for (const list of grouped.values()) list.sort((a, b) => a.startSec - b.startSec)
  return grouped
})

const videoSegmentsByTrack = (trackId: string) => videoSegmentsGrouped.value.get(trackId) || []
const audioSegmentsByTrack = (trackId: string) => audioSegmentsGrouped.value.get(trackId) || []

// totalVideoConcatDurationSec 表示所有视频片段按顺序拼接时的总时长。
const totalVideoConcatDurationSec = computed(() => {
  let sum = 0
  for (const seg of videoSegments.value) sum += seg.durationSec
  return Math.max(0, sum)
})

const totalDurationSec = computed(() => Math.max(1, ...videoSegments.value.map((item) => item.endSec), ...audioSegments.value.map((item) => item.endSec)))
const timelineStatsText = computed(() => `视频轨 ${videoTracks.value.length} · 音轨 ${audioTracks.value.length} · 视频片段 ${renderConfig.videoTrack.length} · 音频片段 ${renderConfig.audioTrack.length} · 视频总时长(拼接) ${formatTime(totalVideoConcatDurationSec.value)}`)
const timelineContentWidth = computed(() => Math.max(900, Math.ceil(totalDurationSec.value * timelineScale.value) + 120))
const timelineCanvasWidth = computed(() => TRACK_LABEL_WIDTH + timelineContentWidth.value)
const playheadHeight = computed(() => RULER_HEIGHT + videoTracks.value.length * VIDEO_ROW_HEIGHT + audioTracks.value.length * AUDIO_ROW_HEIGHT + 8)

const timelineTicks = computed(() => {
  const ticks: Array<{ time: number; major: boolean }> = []
  for (let t = 0; t <= Math.ceil(totalDurationSec.value); t += 1) ticks.push({ time: t, major: t % 5 === 0 })
  return ticks
})

const activeVideoSegment = computed(() => {
  let winner: VideoSegment | null = null
  const currentPlayhead = playheadSec.value
  for (const segment of videoSegments.value) {
    if (currentPlayhead < segment.startSec || currentPlayhead >= segment.endSec) continue
    if (!winner) {
      winner = segment
      continue
    }
    // 优先返回可解析 source 的片段，其次高轨优先，再按更晚起点兜底。
    const sourceDiff = Number(!!segment.source) - Number(!!winner.source)
    if (sourceDiff > 0) {
      winner = segment
      continue
    }
    if (sourceDiff < 0) continue
    if (segment.trackOrder > winner.trackOrder || (segment.trackOrder === winner.trackOrder && segment.startSec > winner.startSec)) {
      winner = segment
    }
  }
  return winner
})

const selectedVideoClip = computed(() => (selectedType.value === 'video' && selectedIndex.value >= 0 ? renderConfig.videoTrack[selectedIndex.value] || null : null))
const selectedAudioClip = computed(() => (selectedType.value === 'audio' && selectedIndex.value >= 0 ? renderConfig.audioTrack[selectedIndex.value] || null : null))

const presetToCssFilter = (preset: VideoTrackClip['effectPreset']) => {
  if (preset === 'grayscale') return 'grayscale(1)'
  if (preset === 'sepia') return 'sepia(0.85)'
  if (preset === 'vintage') return 'sepia(0.25) contrast(1.08) saturate(0.82)'
  if (preset === 'cinematic') return 'contrast(1.15) saturate(1.22) brightness(0.96)'
  return 'none'
}

const previewVideoStyle = computed(() => {
  const active = activeVideoSegment.value
  const filters = [
    `brightness(${Math.max(0.2, 1 + renderConfig.effects.brightness).toFixed(3)})`,
    `contrast(${Math.max(0.2, renderConfig.effects.contrast).toFixed(3)})`,
    `saturate(${Math.max(0, renderConfig.effects.saturation).toFixed(3)})`,
  ]
  if (active) {
    const preset = presetToCssFilter(active.clip.effectPreset)
    if (preset !== 'none') filters.push(preset)
    if (active.clip.blur > 0) filters.push(`blur(${(active.clip.blur * 1.3).toFixed(2)}px)`)
  }
  return { filter: filters.join(' ') }
})

const clipStyle = (startSec: number, durationSec: number) => ({ left: `${startSec * timelineScale.value}px`, width: `${Math.max(56, durationSec * timelineScale.value)}px` })

const formatTime = (sec: number) => {
  const hh = Math.floor(sec / 3600)
  const mm = Math.floor((sec % 3600) / 60)
  const ss = Math.floor(sec % 60)
  const ff = Math.floor((sec - Math.floor(sec)) * renderConfig.fps)
  const pad = (n: number) => `${n}`.padStart(2, '0')
  return `${pad(hh)}:${pad(mm)}:${pad(ss)}:${pad(ff)}`
}

const formatTransitionMeta = (clip: VideoTrackClip) => {
  if (clip.transitionToNext === 'none') return '无转场'
  const item = transitionOptions.find((option) => option.value === clip.transitionToNext)
  const label = item?.label || clip.transitionToNext
  return `${label} ${clip.transitionDurationSec.toFixed(2)}s`
}

// onTopToolClick 将顶部工具栏变为可用交互，避免仅展示无行为。
const onTopToolClick = async (tool: TopTool) => {
  activeTopTool.value = tool
  if (tool === '素材') {
    assetKindFilter.value = 'all'
    await nextTick()
    assetSearchInputRef.value?.focus?.()
    return
  }
  if (tool === '音频') {
    assetKindFilter.value = 'audio'
    await nextTick()
    assetSearchInputRef.value?.focus?.()
    return
  }
  if (tool === '转场') {
    if (!selectedVideoClip.value) {
      ElMessage.info('请先选中一个视频片段后再设置转场')
      return
    }
    selectedVideoClip.value.transitionToNext = selectedVideoClip.value.transitionToNext === 'none' ? 'fade' : selectedVideoClip.value.transitionToNext
    selectedVideoClip.value.transitionDurationSec = selectedVideoClip.value.transitionDurationSec > 0 ? selectedVideoClip.value.transitionDurationSec : 0.5
    ElMessage.success('已激活当前片段转场设置')
    return
  }
  if (tool === '特效') {
    if (!selectedVideoClip.value) {
      ElMessage.info('请先选中一个视频片段后再切换特效')
      return
    }
    const presets: VideoTrackClip['effectPreset'][] = ['none', 'grayscale', 'sepia', 'vintage', 'cinematic']
    const currentIndex = presets.indexOf(selectedVideoClip.value.effectPreset)
    const nextPreset = presets[(currentIndex + 1 + presets.length) % presets.length]
    selectedVideoClip.value.effectPreset = nextPreset
    syncPreviewByPlayhead(false)
    return
  }

  ElMessage.info(`${tool}工具已进入工作区，当前版本支持基础交互`)
}

const fetchSources = async () => {
  loadingSources.value = true
  try {
    const response = await getEditSources()
    if (response.data.code === 200) sources.value = response.data.data.items || []
    else ElMessage.error(response.data.message || '素材加载失败')
  } catch (error) {
    console.error(error)
    ElMessage.error('素材加载失败')
  } finally {
    loadingSources.value = false
  }
}

const previewSource = (item: EditSourceItem) => {
  previewSourceUrl.value = item.url
  stopPlayback()
}

const findAppendStart = (kind: TrackKind, trackId: string) => {
  const list = kind === 'video' ? videoSegmentsByTrack(trackId) : audioSegmentsByTrack(trackId)
  let maxEnd = 0
  for (const item of list) {
    maxEnd = Math.max(maxEnd, item.endSec)
  }
  return maxEnd
}

// findGlobalVideoAppendStart 在全时间线上追加视频，保证新增视频默认按顺序拼接。
const findGlobalVideoAppendStart = () => {
  let maxEnd = 0
  for (const item of videoSegments.value) {
    maxEnd = Math.max(maxEnd, item.endSec)
  }
  return maxEnd
}

const quickInsert = (item: EditSourceItem) => (sourceKind(item) === 'audio' ? addToAudioTrack(item) : addToVideoTrack(item))

const addToVideoTrack = (item: EditSourceItem) => {
  if (sourceKind(item) !== 'video') {
    ElMessage.warning('当前素材是音频，请加入音轨')
    return
  }
  const trackId = activeVideoTrackId.value
  renderConfig.videoTrack.push({
    fileName: item.name,
    scope: item.scope,
    trackId,
    // 视频默认按全局时间线尾部追加，满足“第二个视频=总时长累加拼接”。
    startSec: snapSec(findGlobalVideoAppendStart()),
    inSec: 0,
    outSec: parseDurationToSec(item.duration),
    speed: 1,
    effectPreset: 'none',
    transitionToNext: 'none',
    transitionDurationSec: 0,
    blur: 0,
  })
  selectedType.value = 'video'
  selectedIndex.value = renderConfig.videoTrack.length - 1
  syncPreviewByPlayhead(isPlaying.value)
}

const addToAudioTrack = (item: EditSourceItem) => {
  const trackId = activeAudioTrackId.value
  renderConfig.audioTrack.push({
    fileName: item.name,
    scope: item.scope,
    trackId,
    startSec: snapSec(findAppendStart('audio', trackId)),
    inSec: 0,
    outSec: parseDurationToSec(item.duration),
    speed: 1,
    volume: 1,
    delaySec: 0,
  })
  selectedType.value = 'audio'
  selectedIndex.value = renderConfig.audioTrack.length - 1
}

const selectVideoSegment = (index: number) => {
  selectedType.value = 'video'
  selectedIndex.value = index
  const seg = videoSegments.value.find((item) => item.index === index)
  if (seg) {
    playheadSec.value = snapSec(seg.startSec)
    keepPlayheadVisible(isPlaying.value)
    syncPreviewByPlayhead(isPlaying.value)
  }
}
const selectAudioSegment = (index: number) => {
  selectedType.value = 'audio'
  selectedIndex.value = index
  const seg = audioSegments.value.find((item) => item.index === index)
  if (seg) {
    playheadSec.value = snapSec(seg.startSec)
    keepPlayheadVisible(isPlaying.value)
    if (isPlaying.value) syncPreviewByPlayhead(true)
  }
}
const removeSelectedVideo = () => {
  if (selectedType.value === 'video' && selectedIndex.value >= 0) {
    renderConfig.videoTrack.splice(selectedIndex.value, 1)
    selectedType.value = 'none'
    selectedIndex.value = -1
  }
}
const removeSelectedAudio = () => {
  if (selectedType.value === 'audio' && selectedIndex.value >= 0) {
    renderConfig.audioTrack.splice(selectedIndex.value, 1)
    selectedType.value = 'none'
    selectedIndex.value = -1
  }
}
const nextTrackID = (kind: TrackKind) => `${kind === 'video' ? 'V' : 'A'}${Math.max(0, ...(kind === 'video' ? videoTracks.value : audioTracks.value).map((item) => parseTrackOrder(item.id))) + 1}`
const addVideoTrackSlot = () => {
  const id = nextTrackID('video')
  videoTracks.value.push({ id })
  activeVideoTrackId.value = id
}
const addAudioTrackSlot = () => {
  const id = nextTrackID('audio')
  audioTracks.value.push({ id })
  activeAudioTrackId.value = id
}

const startPlaybackLoop = () => {
  if (rafId.value !== null) return
  lastUiSyncTs.value = 0
  lastFollowScrollTs.value = 0
  rafId.value = requestAnimationFrame(playbackTick)
}

const stopPlaybackLoop = () => {
  if (rafId.value !== null) cancelAnimationFrame(rafId.value)
  rafId.value = null
}

const stopPlayback = () => {
  isPlaying.value = false
  stopPlaybackLoop()
  lastUiSyncTs.value = 0
  lastFollowScrollTs.value = 0
  decoderVideoRef.value?.pause()
}

// keepPlayheadVisible 保证播放头在时间线可视区域内，播放时自动跟随滚动。
const keepPlayheadVisible = (followMode = false) => {
  const container = timelineScrollRef.value
  if (!container) return
  const playheadX = TRACK_LABEL_WIDTH + playheadSec.value * timelineScale.value
  const left = container.scrollLeft
  const right = left + container.clientWidth
  const margin = followMode ? Math.max(120, container.clientWidth * 0.2) : 72

  if (playheadX > right - margin) {
    container.scrollLeft = Math.max(0, playheadX - container.clientWidth + margin)
    return
  }
  if (playheadX < left + TRACK_LABEL_WIDTH + margin) {
    container.scrollLeft = Math.max(0, playheadX - TRACK_LABEL_WIDTH - margin)
  }
}

// keepPlayheadVisibleThrottled 在播放态限制滚动频率，减少频繁读写 scrollLeft 造成的抖动。
const keepPlayheadVisibleThrottled = (ts: number) => {
  if (ts - lastFollowScrollTs.value < followScrollIntervalMs.value) return
  lastFollowScrollTs.value = ts
  keepPlayheadVisible(true)
}

const jumpBy = (delta: number) => {
  playheadSec.value = snapSec(Math.min(totalDurationSec.value, Math.max(0, playheadSec.value + delta)))
  keepPlayheadVisible(isPlaying.value)
  syncPreviewByPlayhead(isPlaying.value)
}

const playbackTick = (ts: number) => {
  if (!isPlaying.value) return
  const decoder = decoderVideoRef.value
  const active = activeVideoSegment.value
  if (!decoder || !active || !active.source) {
    stopPlayback()
    return
  }

  const sourceReady = decoderMetaReady.value && previewSourceUrl.value === active.source.url
  if (!sourceReady) {
    syncPreviewByPlayhead(true).finally(() => {
      if (isPlaying.value) rafId.value = requestAnimationFrame(playbackTick)
    })
    return
  }

  const range = effectiveVideoRange(active.clip)
  if (Math.abs(decoder.playbackRate-range.speed) > 0.001) decoder.playbackRate = range.speed
  if (decoder.paused) {
    decoder.play().catch(() => {
      stopPlayback()
    }).finally(() => {
      if (isPlaying.value) rafId.value = requestAnimationFrame(playbackTick)
    })
    return
  }

  // 以解码器当前时间反推时间线，避免每帧 seek 导致卡顿。
  const localSec = Math.max(0, (decoder.currentTime - range.inSec) / range.speed)
  const nextTimelineSec = Math.min(totalDurationSec.value, Math.max(active.startSec, active.startSec + localSec))
  const shouldSyncUI = (ts - lastUiSyncTs.value >= uiSyncIntervalMs.value) || Math.abs(nextTimelineSec - playheadSec.value) >= 0.2
  if (shouldSyncUI) {
    playheadSec.value = snapSec(nextTimelineSec)
    lastUiSyncTs.value = ts
    keepPlayheadVisibleThrottled(ts)
  }

  drawDecoderFrameIfNeeded(decoder.currentTime)

  if (nextTimelineSec >= active.endSec - frameStepSec.value / 2) {
    playheadSec.value = snapSec(Math.min(totalDurationSec.value, active.endSec + frameStepSec.value))
    lastUiSyncTs.value = ts
    syncPreviewByPlayhead(true).finally(() => {
      if (isPlaying.value) rafId.value = requestAnimationFrame(playbackTick)
    })
    return
  }
  if (nextTimelineSec >= totalDurationSec.value - frameStepSec.value / 2) {
    stopPlayback()
    return
  }
  rafId.value = requestAnimationFrame(playbackTick)
}

const togglePlay = async () => {
  if (isPlaying.value) {
    stopPlayback()
    return
  }
  if (!activeVideoSegment.value && videoSegments.value.length > 0) playheadSec.value = 0
  isPlaying.value = true
  await syncPreviewByPlayhead(true)
  startPlaybackLoop()
}

const ensureMonitorCanvasSize = () => {
  const canvas = monitorCanvasRef.value
  if (!canvas) return
  // 播放中优先流畅度，降低画布像素密度避免高分屏卡顿。
  const dpr = isPlaying.value ? 1 : Math.min(2, window.devicePixelRatio || 1)
  const width = Math.max(2, Math.floor(canvas.clientWidth * dpr))
  const height = Math.max(2, Math.floor(canvas.clientHeight * dpr))
  if (canvas.width !== width || canvas.height !== height) {
    canvas.width = width
    canvas.height = height
  }
}

const clearMonitorCanvas = () => {
  ensureMonitorCanvasSize()
  const canvas = monitorCanvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.fillStyle = '#000'
  ctx.fillRect(0, 0, canvas.width, canvas.height)
  lastDrawMediaSec.value = -1
}

// drawDecoderFrame 将当前解码帧绘制到监看器画布（非 video 组件显示）。
const drawDecoderFrame = () => {
  ensureMonitorCanvasSize()
  const canvas = monitorCanvasRef.value
  const decoder = decoderVideoRef.value
  if (!canvas || !decoder) return false
  const ctx = canvas.getContext('2d')
  if (!ctx) return false

  ctx.fillStyle = '#000'
  ctx.fillRect(0, 0, canvas.width, canvas.height)

  const sourceWidth = decoder.videoWidth
  const sourceHeight = decoder.videoHeight
  if (sourceWidth <= 0 || sourceHeight <= 0) return false

  const scale = Math.min(canvas.width / sourceWidth, canvas.height / sourceHeight)
  const drawWidth = Math.max(1, Math.floor(sourceWidth * scale))
  const drawHeight = Math.max(1, Math.floor(sourceHeight * scale))
  const offsetX = Math.floor((canvas.width - drawWidth) / 2)
  const offsetY = Math.floor((canvas.height - drawHeight) / 2)
  ctx.drawImage(decoder, offsetX, offsetY, drawWidth, drawHeight)
  return true
}

// drawDecoderFrameIfNeeded 对同一帧跳过重复 drawImage，降低播放时 canvas 开销。
const drawDecoderFrameIfNeeded = (mediaSec: number, force = false) => {
  if (!force && lastDrawMediaSec.value >= 0 && Math.abs(mediaSec - lastDrawMediaSec.value) < frameStepSec.value * 0.55) return true
  const ok = drawDecoderFrame()
  if (ok) lastDrawMediaSec.value = mediaSec
  return ok
}

const ensureDecoderSource = async (sourceURL: string) => {
  const decoder = decoderVideoRef.value
  if (!decoder) return false
  if (!sourceURL) return false

  if (previewSourceUrl.value === sourceURL && decoderMetaReady.value && decoder.readyState >= 1) return true

  decoderMetaReady.value = false
  previewSourceUrl.value = sourceURL
  lastDrawMediaSec.value = -1

  await new Promise<void>((resolve) => {
    let done = false
    const finish = () => {
      if (done) return
      done = true
      window.clearTimeout(timer)
      decoder.removeEventListener('loadedmetadata', onLoaded)
      decoder.removeEventListener('error', onError)
      resolve()
    }
    const onLoaded = () => {
      decoderMetaReady.value = true
      finish()
    }
    const onError = () => finish()
    const timer = window.setTimeout(() => finish(), 2000)
    decoder.addEventListener('loadedmetadata', onLoaded, { once: true })
    decoder.addEventListener('error', onError, { once: true })

    if (decoder.src !== sourceURL) {
      decoder.src = sourceURL
      decoder.load()
    } else if (decoder.readyState >= 1) {
      decoderMetaReady.value = true
      finish()
    }
  })

  return decoderMetaReady.value
}

const seekDecoderFrame = async (targetTime: number) => {
  const decoder = decoderVideoRef.value
  if (!decoder) return false
  if (!Number.isFinite(targetTime)) return false

  const duration = Number.isFinite(decoder.duration) ? decoder.duration : 0
  const maxTime = duration > 0 ? Math.max(0, duration-0.001) : targetTime
  const clamped = Math.max(0, Math.min(maxTime, targetTime))
  if (Math.abs(decoder.currentTime - clamped) <= frameStepSec.value / 2) return true

  return new Promise<boolean>((resolve) => {
    let done = false
    const finish = (ok: boolean) => {
      if (done) return
      done = true
      window.clearTimeout(timer)
      decoder.removeEventListener('seeked', onSeeked)
      decoder.removeEventListener('error', onError)
      resolve(ok)
    }
    const onSeeked = () => finish(true)
    const onError = () => finish(false)
    const timer = window.setTimeout(() => finish(false), 1200)
    decoder.addEventListener('seeked', onSeeked, { once: true })
    decoder.addEventListener('error', onError, { once: true })
    try {
      decoder.currentTime = clamped
    } catch (_error) {
      finish(false)
    }
  })
}

// syncPreviewByPlayhead 是实时预览核心：根据播放头定位片段并将帧渲染到 canvas。
const syncPreviewByPlayhead = async (allowPlay = false) => {
  const currentTaskID = ++renderTaskID.value
  const active = activeVideoSegment.value
  if (!active || !active.source) {
    previewSourceUrl.value = ''
    lastActiveVideoIndex.value = -1
    clearMonitorCanvas()
    return
  }

  const range = effectiveVideoRange(active.clip)
  const localTimelineSec = playheadSec.value - active.startSec
  const targetMediaTime = Math.max(range.inSec, Math.min(range.outSec, range.inSec + localTimelineSec * range.speed))
  const decoder = decoderVideoRef.value
  if (!decoder) {
    clearMonitorCanvas()
    return
  }

  const sourceReady = await ensureDecoderSource(active.source.url)
  if (currentTaskID !== renderTaskID.value) return
  if (!sourceReady) {
    clearMonitorCanvas()
    return
  }

  lastActiveVideoIndex.value = active.index
  const needSeek = Math.abs(decoder.currentTime - targetMediaTime) > (allowPlay ? 0.18 : frameStepSec.value / 2)
  if (needSeek) {
    const seekOK = await seekDecoderFrame(targetMediaTime)
    if (currentTaskID !== renderTaskID.value) return
    if (!seekOK) {
      clearMonitorCanvas()
      return
    }
  }

  decoder.playbackRate = range.speed
  drawDecoderFrameIfNeeded(decoder.currentTime, !allowPlay || needSeek)
  if (allowPlay) {
    try {
      if (decoder.paused) await decoder.play()
      isPlaying.value = true
      startPlaybackLoop()
    } catch (_error) {
      stopPlayback()
    }
  } else {
    decoder.pause()
  }
}

const updatePlayheadByClientX = (clientX: number) => {
  const ruler = timelineScrollRef.value?.querySelector('.ruler-lane') as HTMLDivElement | null
  if (!ruler) return
  const rect = ruler.getBoundingClientRect()
  const offsetX = Math.min(Math.max(0, clientX-rect.left), timelineContentWidth.value)
  playheadSec.value = snapSec(Math.min(totalDurationSec.value, Math.max(0, offsetX / timelineScale.value)))
}

const locatePlayheadByEvent = (event: MouseEvent) => updatePlayheadByClientX(event.clientX)

const handleScrubMove = (event: MouseEvent) => {
  if (!isScrubbing.value) return
  locatePlayheadByEvent(event)
  keepPlayheadVisible(false)
  syncPreviewByPlayhead(false)
}

const endScrub = () => {
  isScrubbing.value = false
  window.removeEventListener('mousemove', handleScrubMove)
  window.removeEventListener('mouseup', endScrub)
}

// beginScrub 支持拖动播放头实时预览，贴近剪辑软件监看行为。
const beginScrub = (event: MouseEvent) => {
  if (event.button !== 0) return
  stopPlayback()
  isScrubbing.value = true
  locatePlayheadByEvent(event)
  keepPlayheadVisible(false)
  syncPreviewByPlayhead(false)
  window.addEventListener('mousemove', handleScrubMove)
  window.addEventListener('mouseup', endScrub)
}

const onRulerClick = (event: MouseEvent) => {
  locatePlayheadByEvent(event)
  keepPlayheadVisible(isPlaying.value)
  syncPreviewByPlayhead(isPlaying.value)
}
const onLaneClick = (event: MouseEvent, kind: TrackKind, trackId: string) => {
  if (kind === 'video') activeVideoTrackId.value = trackId
  else activeAudioTrackId.value = trackId
  locatePlayheadByEvent(event)
  keepPlayheadVisible(isPlaying.value)
  syncPreviewByPlayhead(isPlaying.value)
}

const getVideoClip = (index: number) => renderConfig.videoTrack[index] || null
const getAudioClip = (index: number) => renderConfig.audioTrack[index] || null

// beginMove 开始拖动片段，更新 startSec。
const beginMove = (kind: TrackKind, index: number, event: MouseEvent) => {
  const clip = kind === 'video' ? getVideoClip(index) : getAudioClip(index)
  if (!clip) return
  dragState.value = {
    kind,
    index,
    mode: 'move',
    startClientX: event.clientX,
    initialStart: clip.startSec,
    initialIn: clip.inSec,
    initialOut: clip.outSec,
    speed: clip.speed || 1,
    sourceDuration: clipSourceDuration(clip.fileName, clip.scope),
  }
  window.addEventListener('mousemove', handleDragMove)
  window.addEventListener('mouseup', handleDragEnd)
}

// beginTrim 开始裁剪，左手柄调整入点，右手柄调整出点。
const beginTrim = (kind: TrackKind, index: number, side: TrimSide, event: MouseEvent) => {
  const clip = kind === 'video' ? getVideoClip(index) : getAudioClip(index)
  if (!clip) return
  dragState.value = {
    kind,
    index,
    mode: side === 'left' ? 'trim-left' : 'trim-right',
    startClientX: event.clientX,
    initialStart: clip.startSec,
    initialIn: clip.inSec,
    initialOut: clip.outSec,
    speed: clip.speed || 1,
    sourceDuration: clipSourceDuration(clip.fileName, clip.scope),
  }
  window.addEventListener('mousemove', handleDragMove)
  window.addEventListener('mouseup', handleDragEnd)
}

const handleDragMove = (event: MouseEvent) => {
  const state = dragState.value
  if (!state) return
  const deltaSec = (event.clientX - state.startClientX) / timelineScale.value
  const minDurationSec = 0.05
  const clip = state.kind === 'video' ? getVideoClip(state.index) : getAudioClip(state.index)
  if (!clip) return

  if (state.mode === 'move') {
    clip.startSec = snapSec(Math.max(0, state.initialStart + deltaSec))
    // 拖动片段时允许跨轨道移动，轨道取鼠标当前位置下的轨道行。
    const laneElement = (document.elementFromPoint(event.clientX, event.clientY) as HTMLElement | null)?.closest('.track-lane') as HTMLElement | null
    if (laneElement) {
      const laneKind = laneElement.dataset.kind as TrackKind | undefined
      const laneTrackID = laneElement.dataset.trackId
      if (laneKind === state.kind && laneTrackID) {
        clip.trackId = laneTrackID
        if (laneKind === 'video') activeVideoTrackId.value = laneTrackID
        else activeAudioTrackId.value = laneTrackID
      }
    }
  }
  if (state.mode === 'trim-left') {
    const targetIn = state.initialIn + deltaSec * state.speed
    const maxIn = Math.max(0, state.initialOut - minDurationSec)
    const nextIn = Math.min(maxIn, Math.max(0, targetIn))
    clip.inSec = snapSec(nextIn)
    clip.startSec = snapSec(Math.max(0, state.initialStart + (nextIn - state.initialIn) / state.speed))
  }
  if (state.mode === 'trim-right') {
    const targetOut = state.initialOut + deltaSec * state.speed
    const minOut = state.initialIn + minDurationSec
    if (state.sourceDuration > 0) {
      const maxOut = Math.max(minOut, state.sourceDuration)
      clip.outSec = snapSec(Math.min(maxOut, Math.max(minOut, targetOut)))
    } else {
      // 源时长未知时不做上限裁剪，保证时间线长度按编辑结果实时扩展。
      clip.outSec = snapSec(Math.max(minOut, targetOut))
    }
  }

  keepPlayheadVisible(isPlaying.value)
  syncPreviewByPlayhead(isPlaying.value)
}

const handleDragEnd = () => {
  dragState.value = null
  window.removeEventListener('mousemove', handleDragMove)
  window.removeEventListener('mouseup', handleDragEnd)
}

// runMonitorDiagnostics 校验画布监看器链路：片段绑定、解码元数据、seek、逐帧绘制。
const runMonitorDiagnostics = async () => {
  stopPlayback()
  const checks: MonitorCheckItem[] = []
  const canvas = monitorCanvasRef.value
  const decoder = decoderVideoRef.value
  if (!canvas || !decoder) {
    checks.push({ label: '监看器节点', pass: false, detail: '未找到 canvas 或 decoder 节点' })
    monitorChecks.value = checks
    return
  }

  // 自检前先自动跳到最近可播放片段，避免“当前播放头无片段”造成连锁失败。
  let active = activeVideoSegment.value
  if (!active && videoSegments.value.length > 0) {
    const sorted = videoSegments.value
      .slice()
      .sort((a, b) => a.startSec - b.startSec || b.trackOrder - a.trackOrder)
    const target = sorted.find((seg) => !!seg.source) || sorted[0]
    playheadSec.value = snapSec(target.startSec)
    await syncPreviewByPlayhead(false)
    await nextTick()
    active = activeVideoSegment.value
  }

  if (active && !previewSourceUrl.value && active.source?.url) {
    previewSourceUrl.value = active.source.url
    await nextTick()
  }

  const hasSource = !!previewSourceUrl.value
  checks.push({ label: '激活片段', pass: !!active, detail: active ? `${active.clip.trackId} / ${active.clip.fileName}` : '未找到可播放片段' })
  checks.push({ label: '视频源绑定', pass: hasSource, detail: hasSource ? '已绑定 source URL' : '未找到可用 source URL' })

  if (!hasSource) {
    checks.push({ label: '元数据加载', pass: false, detail: '无视频源，跳过元数据检测' })
    checks.push({ label: 'Seek 定位', pass: false, detail: '无视频源，跳过 seek 检测' })
    checks.push({ label: '帧渲染', pass: false, detail: '无视频源，跳过逐帧渲染' })
    monitorChecks.value = checks
    return
  }

  const metadataReady = await ensureDecoderSource(previewSourceUrl.value)
  checks.push({ label: '元数据加载', pass: metadataReady, detail: metadataReady ? 'loadedmetadata 已触发' : '超时未触发' })

  const segment = activeVideoSegment.value
  const range = segment ? effectiveVideoRange(segment.clip) : null
  const targetMediaTime = range ? Math.max(range.inSec, Math.min(range.outSec, range.inSec + (playheadSec.value - (segment?.startSec || 0)) * range.speed)) : 0
  const seekOK = metadataReady ? await seekDecoderFrame(targetMediaTime) : false
  checks.push({ label: 'Seek 定位', pass: seekOK, detail: seekOK ? `定位到 ${targetMediaTime.toFixed(3)}s` : 'seek 失败' })

  const drawOK = seekOK ? drawDecoderFrame() : false
  checks.push({ label: '帧渲染', pass: drawOK, detail: drawOK ? '画布已完成 drawImage' : '未成功绘制视频帧' })

  try {
    const prev = playheadSec.value
    playheadSec.value = snapSec(Math.min(totalDurationSec.value, prev + frameStepSec.value))
    await syncPreviewByPlayhead(false)
    const moved = playheadSec.value > prev
    checks.push({ label: '时间线联动', pass: moved, detail: moved ? '播放头推进后画布已同步' : '播放头未推进' })
  } catch (error: any) {
    checks.push({ label: '时间线联动', pass: false, detail: error?.message || '联动校验异常' })
  }
  monitorChecks.value = checks
  syncPreviewByPlayhead(false)
}

const hasOverlapInTrack = (kind: TrackKind, trackID: string) => {
  const list = (kind === 'video' ? videoSegmentsByTrack(trackID) : audioSegmentsByTrack(trackID)).slice().sort((a, b) => a.startSec - b.startSec)
  for (let i = 1; i < list.length; i += 1) {
    if (list[i].startSec < list[i - 1].endSec - frameStepSec.value / 2) {
      return true
    }
  }
  return false
}

// validateTimelineBeforeRender 导出前做完整校验，避免 ffmpeg 失败后才反馈。
const validateTimelineBeforeRender = () => {
  if (renderConfig.videoTrack.length === 0) return '请至少添加一个视频片段'

  for (const clip of renderConfig.videoTrack) {
    if (!resolveSource(clip.fileName, clip.scope)) return `视频素材不存在：${clip.fileName}`
  }
  for (const clip of renderConfig.audioTrack) {
    if (!resolveSource(clip.fileName, clip.scope)) return `音频素材不存在：${clip.fileName}`
  }

  for (const track of videoTracks.value) {
    if (hasOverlapInTrack('video', track.id)) return `视频轨 ${track.id} 存在片段重叠，请先错开时间`
  }
  for (const track of audioTracks.value) {
    if (hasOverlapInTrack('audio', track.id)) return `音轨 ${track.id} 存在片段重叠，请先错开时间`
  }
  return ''
}

const sanitizeClipsBeforeRender = () => {
  for (const clip of renderConfig.videoTrack) {
    clip.trackId = normalizeTrackId('video', clip.trackId)
    clip.transitionToNext = normalizeTransitionName(clip.transitionToNext)
    clip.startSec = snapSec(Math.max(0, clip.startSec))
    clip.inSec = snapSec(Math.max(0, clip.inSec))
    clip.speed = Math.max(0.25, Math.min(4, Number.isFinite(clip.speed) ? clip.speed : 1))
    clip.blur = Math.max(0, Math.min(4, Number.isFinite(clip.blur) ? clip.blur : 0))
    if (clip.transitionToNext === 'none') {
      clip.transitionDurationSec = 0
    } else {
      clip.transitionDurationSec = snapSec(Math.min(2, Math.max(0.1, clip.transitionDurationSec || 0.5)))
    }
    const sourceDuration = clipSourceDuration(clip.fileName, clip.scope)
    if (clip.outSec <= clip.inSec) {
      const fallbackOut = clip.inSec + Math.max(frameStepSec.value, 10)
      clip.outSec = snapSec(sourceDuration > 0 ? Math.max(clip.inSec + frameStepSec.value, sourceDuration) : fallbackOut)
    } else {
      clip.outSec = snapSec(sourceDuration > 0 ? Math.min(sourceDuration, clip.outSec) : clip.outSec)
    }
  }
  for (const clip of renderConfig.audioTrack) {
    clip.trackId = normalizeTrackId('audio', clip.trackId)
    clip.delaySec = snapSec(Math.max(0, clip.delaySec || 0))
    if (clip.startSec <= 0 && clip.delaySec > 0) clip.startSec = clip.delaySec
    clip.startSec = snapSec(Math.max(0, clip.startSec))
    clip.inSec = snapSec(Math.max(0, clip.inSec))
    clip.speed = Math.max(0.25, Math.min(4, Number.isFinite(clip.speed) ? clip.speed : 1))
    clip.volume = Math.max(0, Math.min(4, Number.isFinite(clip.volume) ? clip.volume : 1))
    const sourceDuration = clipSourceDuration(clip.fileName, clip.scope)
    if (clip.outSec <= clip.inSec) {
      const fallbackOut = clip.inSec + Math.max(frameStepSec.value, 10)
      clip.outSec = snapSec(sourceDuration > 0 ? Math.max(clip.inSec + frameStepSec.value, sourceDuration) : fallbackOut)
    } else {
      clip.outSec = snapSec(sourceDuration > 0 ? Math.min(sourceDuration, clip.outSec) : clip.outSec)
    }
  }
}

const renderProject = async () => {
  sanitizeClipsBeforeRender()
  const validateMessage = validateTimelineBeforeRender()
  if (validateMessage) {
    ElMessage.warning(validateMessage)
    return
  }
  rendering.value = true
  try {
    const payload: EditRenderRequest = JSON.parse(JSON.stringify(renderConfig))
    const response = await renderEditProject(payload)
    if (response.data.code === 200) {
      resultUrl.value = response.data.data.outputUrl
      ElMessage.success('合成完成')
    } else ElMessage.error(response.data.message || '合成失败')
  } catch (error: any) {
    console.error(error)
    ElMessage.error(error?.response?.data?.message || '合成失败')
  } finally {
    rendering.value = false
  }
}

const isTypingTarget = (target: EventTarget | null) => {
  const node = target as HTMLElement | null
  if (!node) return false
  const tag = node.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || node.isContentEditable || !!node.closest('.el-input') || !!node.closest('.el-textarea')
}

// handleGlobalKeydown 提供剪辑常用快捷键：空格播放、Delete 删除、Ctrl+S 导出。
const handleGlobalKeydown = (event: KeyboardEvent) => {
  const typing = isTypingTarget(event.target)
  const key = event.key.toLowerCase()

  if ((event.ctrlKey || event.metaKey) && key === 's') {
    event.preventDefault()
    if (!rendering.value) renderProject()
    return
  }

  if (typing) return

  if (event.code === 'Space') {
    event.preventDefault()
    togglePlay()
    return
  }

  if (key === 'delete' || key === 'backspace') {
    event.preventDefault()
    if (selectedType.value === 'video') removeSelectedVideo()
    if (selectedType.value === 'audio') removeSelectedAudio()
    return
  }

  if (key === 'arrowleft') {
    event.preventDefault()
    jumpBy(event.shiftKey ? -5 : -1)
    return
  }

  if (key === 'arrowright') {
    event.preventDefault()
    jumpBy(event.shiftKey ? 5 : 1)
  }
}

watch(
  () => [renderConfig.effects.brightness, renderConfig.effects.contrast, renderConfig.effects.saturation, renderConfig.effects.sharpen],
  () => !isPlaying.value && syncPreviewByPlayhead(false),
)

watch(
  () => renderConfig.videoTrack,
  () => {
    ensureTrackSlotsFromClips()
    if (playheadSec.value > totalDurationSec.value) playheadSec.value = totalDurationSec.value
    if (!isPlaying.value) syncPreviewByPlayhead(false)
  },
  { deep: true },
)

watch(
  () => renderConfig.audioTrack,
  () => {
    ensureTrackSlotsFromClips()
    if (playheadSec.value > totalDurationSec.value) playheadSec.value = totalDurationSec.value
  },
  { deep: true },
)

watch(playheadSec, () => {
  if (isPlaying.value) return
  keepPlayheadVisible(false)
  syncPreviewByPlayhead(false)
})

const handleWindowResize = () => {
  ensureMonitorCanvasSize()
  if (previewSourceUrl.value) {
    syncPreviewByPlayhead(false)
    return
  }
  clearMonitorCanvas()
}

onMounted(async () => {
  await fetchSources()
  ensureTrackSlotsFromClips()
  ensureMonitorCanvasSize()
  clearMonitorCanvas()
  window.addEventListener('keydown', handleGlobalKeydown)
  window.addEventListener('resize', handleWindowResize)
})

onUnmounted(() => {
  stopPlayback()
  endScrub()
  window.removeEventListener('mousemove', handleDragMove)
  window.removeEventListener('mouseup', handleDragEnd)
  window.removeEventListener('keydown', handleGlobalKeydown)
  window.removeEventListener('resize', handleWindowResize)
})
</script>

<style scoped>
.editor-workbench { display: flex; flex-direction: column; gap: 8px; color: #d7dbe3; min-height: 0; }
.panel { background: #171a21; border: 1px solid #2b3140; border-radius: 10px; padding: 10px; }
.topbar { display: grid; grid-template-columns: 260px minmax(0,1fr) 220px; align-items: center; gap: 10px; }
.title-wrap { display: flex; align-items: center; gap: 10px; }
.logo { width: 34px; height: 34px; border-radius: 8px; background: linear-gradient(145deg,#00c9ff,#2f6bff); display: flex; align-items: center; justify-content: center; color: #fff; font-weight: 800; }
.title { font-size: 13px; font-weight: 700; }
.sub { color: #8d97aa; font-size: 12px; }
.tool-list { display: flex; gap: 6px; overflow: auto; }
.tool-btn { border: 1px solid #3a4255; background: #202632; color: #d8deea; border-radius: 8px; font-size: 12px; padding: 6px 10px; white-space: nowrap; cursor: pointer; }
.tool-btn.active { border-color: #4f93ff; background: #20314f; color: #e8f1ff; }
.actions { display: flex; justify-content: flex-end; gap: 8px; }
.config-bar { display: flex; align-items: center; gap: 8px; }
.name-input { width: 260px; }
.small-input { width: 120px; }
.workspace { display: grid; grid-template-columns: 320px minmax(0,1fr) 300px; gap: 10px; min-height: 0; }
.panel-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.panel-title { font-size: 14px; font-weight: 700; }
.asset-panel { display: flex; flex-direction: column; gap: 8px; }
.target-row { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
.asset-grid { display: grid; grid-template-columns: repeat(2, minmax(0,1fr)); gap: 8px; max-height: 560px; overflow: auto; }
.asset-card { background: #232833; border: 1px solid #323a4d; border-radius: 8px; padding: 6px; }
.cover { position: relative; width: 100%; aspect-ratio: 16/9; border-radius: 6px; overflow: hidden; background: #10141d; }
.cover video { width: 100%; height: 100%; object-fit: cover; }
.cover.audio { background: linear-gradient(140deg,#0f3d7d,#165aa9); }
.audio-cover { width: 100%; height: 100%; background: repeating-linear-gradient(90deg, rgba(54,175,255,.95) 0, rgba(54,175,255,.95) 2px, rgba(8,39,91,.85) 2px, rgba(8,39,91,.85) 6px); }
.duration { position: absolute; right: 6px; top: 6px; background: rgba(0,0,0,.68); color: #f0f4ff; border-radius: 6px; padding: 1px 6px; font-size: 11px; }
.asset-name { margin-top: 6px; font-size: 12px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-btn { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.monitor-panel { display: flex; flex-direction: column; gap: 10px; }
.monitor-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.preview-wrap { position: relative; width: 100%; aspect-ratio: 16/9; border-radius: 10px; overflow: hidden; border: 1px solid #2e3545; background: #0d1017; }
.preview-canvas { width: 100%; height: 100%; display: block; background: #000; }
.decoder-video { display: none; }
.safe-area { position: absolute; inset: 8% 10%; border: 1px dashed rgba(255,255,255,.5); pointer-events: none; color: rgba(255,255,255,.8); font-size: 11px; padding: 4px; }
.badge { position: absolute; left: 10px; bottom: 10px; background: rgba(0,0,0,.58); color: #fff; border-radius: 8px; padding: 4px 8px; font-size: 12px; }
.slider-grid { display: grid; grid-template-columns: repeat(2, minmax(0,1fr)); gap: 8px 14px; }
.slider-grid label { display: block; font-size: 12px; color: #a5afc1; margin-bottom: 4px; }
.diag-box { border: 1px solid #2e3545; border-radius: 8px; padding: 8px; background: #131823; }
.diag-item { display: flex; justify-content: space-between; gap: 10px; padding: 6px; border-radius: 6px; font-size: 12px; margin-top: 6px; }
.diag-item.ok { background: rgba(36,170,106,.15); color: #96f0c7; }
.diag-item.fail { background: rgba(209,65,65,.15); color: #ffb8b8; }
.timeline-panel { display: flex; flex-direction: column; gap: 8px; }
.timeline-scroll { overflow: auto; border: 1px solid #30384a; border-radius: 8px; background: #11151d; }
.timeline-canvas { position: relative; }
.ruler-row, .track-row { display: grid; grid-template-columns: 88px 1fr; border-bottom: 1px solid #262d3b; }
.ruler-row { height: 34px; }
.track-row { height: 72px; }
.track-row.audio { height: 62px; }
.track-label { display: flex; align-items: center; justify-content: center; font-weight: 700; border-right: 1px solid #30384a; background: #1b2230; cursor: pointer; user-select: none; }
.track-label.active { background: #243046; color: #fff; }
.ruler-lane, .track-lane { position: relative; cursor: pointer; }
.tick { position: absolute; top: 0; width: 1px; height: 100%; background: rgba(255,255,255,.2); }
.tick span { position: absolute; top: 2px; left: 4px; font-size: 10px; color: rgba(255,255,255,.78); }
.track-lane::before { content: ''; position: absolute; inset: 0; background: linear-gradient(90deg, rgba(255,255,255,.03) 1px, transparent 1px); background-size: 44px 100%; pointer-events: none; }
.clip { position: absolute; top: 10px; height: 50px; border-radius: 8px; padding: 6px 12px; cursor: grab; overflow: hidden; border: 1px solid transparent; user-select: none; }
.track-row.audio .clip { top: 7px; height: 46px; }
.clip.video { background: linear-gradient(145deg, rgba(22,169,255,.38), rgba(26,121,255,.25)); }
.clip.audio { background: linear-gradient(145deg, rgba(41,138,255,.45), rgba(20,86,173,.34)); }
.clip.active { border-color: #fff; box-shadow: 0 0 0 1px rgba(255,255,255,.45); }
.clip-title { color: #fff; font-size: 12px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.clip-meta { color: rgba(255,255,255,.82); font-size: 11px; margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.trim { position: absolute; top: 4px; bottom: 4px; width: 7px; border-radius: 4px; background: rgba(255,255,255,.55); cursor: ew-resize; z-index: 2; }
.trim.left { left: 2px; }
.trim.right { right: 2px; }
.playhead { position: absolute; top: 0; left: 0; width: 2px; background: #fff; z-index: 20; cursor: ew-resize; will-change: transform; }
.dot { position: absolute; top: 0; left: -5px; width: 12px; height: 12px; border-radius: 50%; background: #fff; }
.export-panel { display: flex; flex-direction: column; gap: 8px; }
.result-video { width: 100%; max-height: 320px; background: #000; border-radius: 8px; }

:deep(.el-input__wrapper), :deep(.el-select__wrapper), :deep(.el-input-number), :deep(.el-input-number .el-input__wrapper) { background: #1f2532; border-color: #374055; box-shadow: none; }
:deep(.el-input__inner), :deep(.el-select__selected-item), :deep(.el-input-number .el-input__inner) { color: #e2e7f0; }
:deep(.el-form-item__label) { color: #aeb8cb; }

@media (max-width: 1400px) {
  .workspace { grid-template-columns: 1fr; }
  .topbar { grid-template-columns: 1fr; }
  .actions { justify-content: flex-start; }
}
</style>
