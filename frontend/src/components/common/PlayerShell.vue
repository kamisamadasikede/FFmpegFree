<template>
  <div class="ff-player" :class="{ fill, off: disabled }">
    <div class="stage">
      <!-- 16:9 画面区。放 <video> / mpegts 的 video 元素 / 预览画面，默认铺满 -->
      <div class="frame"><slot /></div>
      <!-- 盖在画面上的层（ErrorOverlay、直播角标等），铺满 stage -->
      <slot name="overlay" />
    </div>

    <div class="ctrl">
      <template v-if="mode === 'vod'">
        <button v-if="show.step" type="button" class="ci step" :aria-label="labels.prev ?? '上一帧'" :aria-disabled="disabled || undefined" @click="!disabled && emit('step', -1)"><FIcon name="left" :size="17" /></button>
        <button type="button" class="pb" :aria-label="playing ? (labels.pause ?? '暂停') : (labels.play ?? '播放')" :aria-disabled="disabled || undefined" @click="!disabled && togglePlay()">
          <FIcon :name="playing ? 'pause' : 'play'" :size="14" style="fill: #111" />
        </button>
        <button v-if="show.step" type="button" class="ci step" :aria-label="labels.next ?? '下一帧'" :aria-disabled="disabled || undefined" @click="!disabled && emit('step', 1)"><FIcon name="right" :size="17" /></button>
        <span v-if="show.time" class="tc">{{ fmt(current) }} <em>/ {{ fmt(duration) }}</em></span>
        <span v-if="!show.seek" class="grow"></span>
        <div
          v-if="show.seek"
          ref="seekEl"
          class="seek"
          role="slider"
          tabindex="0"
          aria-label="播放进度"
          :aria-disabled="disabled || undefined"
          :aria-valuemin="0"
          :aria-valuemax="duration"
          :aria-valuenow="current"
          @pointerdown="!disabled && onSeekDown($event)"
          @keydown.left.prevent="!disabled && seekBy(-5)"
          @keydown.right.prevent="!disabled && seekBy(5)"
        >
          <i :style="{ width: pct + '%' }"></i><b :style="{ left: pct + '%' }"></b>
        </div>
      </template>
      <template v-else>
        <!-- 状态模式（录屏推流）：没有进度条，左侧是状态，右侧是音量 / 开关 / 全屏 -->
        <span class="ci" :class="{ rec: statusIcon === 'rec' }"><FIcon :name="statusIcon" :size="17" /></span>
        <span class="tc"><slot name="status">{{ statusText }}</slot> <em v-if="statusHint">· {{ statusHint }}</em></span>
        <span class="grow"></span>
      </template>

      <button v-if="show.mute" type="button" class="ci" :aria-label="muted ? '取消静音' : '静音'" @click="muted = !muted">
        <FIcon :name="muted ? 'mute' : 'vol'" :size="17" />
      </button>
      <!-- 静音按钮右边的附加控件（转换页预览弹窗的音量条） -->
      <slot name="extra" />
      <button v-if="chip" type="button" class="chip" @click="emit('chip')">{{ chip }}</button>
      <button v-if="show.fullscreen" type="button" class="ci" aria-label="全屏" @click="emit('fullscreen')"><FIcon name="full" :size="17" /></button>
    </div>
  </div>
</template>

<script setup lang="ts">
// 播放器外壳：只负责外观和控制条 UI，不持有媒体元素。
// 编辑页和直播页各自把 video 放进默认插槽，并用 v-model 驱动控制条（playing / muted / current / duration，单位秒）。
// 样式来自 proto/extra.css 的 .player / .stage / .ctrl。
import { computed, ref } from 'vue'
import FIcon from '../icon/FIcon.vue'
import type { IconName } from '../icon/icons'

const props = withDefaults(
  defineProps<{
    /** vod：上一帧/播放/下一帧 + 时间码 + 进度条；status：录屏推流那种只有状态文字的控制条 */
    mode?: 'vod' | 'status'
    /** 右侧小标签，例如倍速“1.0×”或“麦克风 开”；不传则不显示 */
    chip?: string
    /** status 模式的状态文字和补充说明 */
    statusText?: string
    statusHint?: string
    /** 时间码里“帧”的进制，原型的 00:01:12.08 是 25fps 的帧号 */
    fps?: number
    /** status 模式左侧图标，默认录制红点；拉流播放用 play */
    statusIcon?: IconName
    /** 撑满父容器高度：画面 16:9 居中留黑边（直播页用）；默认按宽度算 16:9 高度（编辑页用） */
    fill?: boolean
    /** 控制条的播放 / 上一帧 / 下一帧 / 进度条置为 aria-disabled（40% 不透明度，仍可聚焦）；剪辑页预览失败时用 */
    disabled?: boolean
    /** 控制条按钮的读屏名称；不传用默认（剪辑页要“停止播放”而不是“暂停”） */
    labels?: { play?: string; pause?: string; prev?: string; next?: string }
    /** 时间码格式：hms = 00:01:12.08（默认）；ms = 00:12.08，满 1 小时才带小时（剪辑页）；clock = 00:48，满 1 小时才带小时、不带帧号（转换页预览） */
    timeFormat?: 'hms' | 'ms' | 'clock'
    /** 控制条上显示哪些部件（缺省都显示，编辑页 / 直播页不传）。转换页预览：不要上一帧 / 下一帧；音频不要全屏；GIF 只留播放 / 暂停 */
    parts?: { step?: boolean; time?: boolean; seek?: boolean; mute?: boolean; fullscreen?: boolean }
  }>(),
  { mode: 'vod', fps: 25, statusIcon: 'rec', fill: false, disabled: false, labels: () => ({}), timeFormat: 'hms', parts: () => ({}) },
)
const show = computed(() => ({
  step: props.parts.step ?? true,
  time: props.parts.time ?? true,
  seek: props.parts.seek ?? true,
  mute: props.parts.mute ?? true,
  fullscreen: props.parts.fullscreen ?? true,
}))

const playing = defineModel<boolean>('playing', { default: false })
const muted = defineModel<boolean>('muted', { default: false })
const current = defineModel<number>('current', { default: 0 })
const duration = defineModel<number>('duration', { default: 0 })

const emit = defineEmits<{
  play: []
  pause: []
  /** 上一帧 -1 / 下一帧 +1 */
  step: [dir: -1 | 1]
  /** 用户拖动或点击进度条，参数为目标秒数（同时已写入 v-model:current） */
  seek: [time: number]
  chip: []
  fullscreen: []
}>()

const seekEl = ref<HTMLElement | null>(null)
const pct = computed(() => (duration.value > 0 ? Math.min(100, Math.max(0, (current.value / duration.value) * 100)) : 0))

function togglePlay() {
  playing.value = !playing.value
  if (playing.value) emit('play')
  else emit('pause')
}

function fmt(sec: number) {
  const s = Math.max(0, sec || 0)
  const whole = Math.floor(s)
  const frames = Math.min(props.fps - 1, Math.floor((s - whole) * props.fps + 1e-6)) // 加 1e-6 防止 72.32*25 这类浮点误差少算一帧
  const p = (n: number) => String(n).padStart(2, '0')
  const h = Math.floor(whole / 3600)
  if (props.timeFormat === 'clock') return `${h ? `${h}:` : ''}${p(Math.floor((whole % 3600) / 60))}:${p(whole % 60)}`
  const hh = props.timeFormat === 'ms' && !h ? '' : `${p(h)}:`
  return `${hh}${p(Math.floor((whole % 3600) / 60))}:${p(whole % 60)}.${p(frames)}`
}

function seekTo(t: number) {
  const v = Math.min(duration.value, Math.max(0, t))
  current.value = v
  emit('seek', v)
}
function seekBy(delta: number) {
  seekTo(current.value + delta)
}
function seekFromEvent(e: PointerEvent) {
  const el = seekEl.value
  if (!el || duration.value <= 0) return
  const rect = el.getBoundingClientRect()
  seekTo(((e.clientX - rect.left) / rect.width) * duration.value)
}
function onSeekDown(e: PointerEvent) {
  seekFromEvent(e)
  const move = (ev: PointerEvent) => seekFromEvent(ev)
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}
</script>

<style scoped>
/* fill：对应原型直播页的 .player{flex:1} + .stage{display:grid;place-items:center} + .frame{aspect-ratio:16/9} */
.ff-player.fill {
  flex: 1;
  min-height: 0;
}
.fill .stage {
  aspect-ratio: auto;
  flex: 1;
  min-height: 0;
  display: grid;
  place-items: center;
}
.fill .frame {
  position: relative;
  inset: auto;
  width: 100%;
  max-height: 100%;
  aspect-ratio: 16 / 9;
}
/* 播放器区域固定深色，不跟主题 */
.ff-player {
  background: #0b0c0e;
  border-radius: var(--ff-radius-lg);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--ff-border);
  width: 100%;
  min-width: 320px;
}
.stage {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  min-height: 180px; /* 没有内容时也不塌 */
  flex: none;
}
.frame {
  position: absolute;
  inset: 0;
  overflow: hidden;
}
.frame :deep(video),
.frame :deep(canvas) {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #000;
}
.ctrl {
  height: 44px;
  background: linear-gradient(0deg, #16171b, #131417);
  display: flex;
  align-items: center;
  gap: var(--ff-space-3);
  padding: 0 14px;
  color: #d6d9de;
  flex: none;
}
.ctrl button {
  border: 0;
  padding: 0;
  font: inherit;
  cursor: pointer;
  background: none;
}
.pb {
  width: 30px;
  height: 30px;
  border-radius: 50%;
  background: #fff !important;
  color: #111;
  display: grid;
  place-items: center;
  flex: none;
}
.pb :deep(svg) {
  stroke: #111;
}
.ci {
  /* 点击热区 28×28，图标视觉尺寸（17px）不变 */
  width: 28px;
  height: 28px;
  color: #aeb3bb;
  display: grid;
  place-items: center;
  flex: none;
}
.ci:hover {
  color: #fff;
}
.ci.rec {
  color: #ef4444;
}
.tc {
  font-family: var(--ff-font-mono);
  font-size: var(--ff-fs-xs);
  color: #fff;
  white-space: nowrap;
}
.tc em {
  font-style: normal;
  color: #7c828c;
}
.grow {
  flex: 1;
}
.seek {
  flex: 1;
  min-width: 60px;
  height: 4px;
  border-radius: 2px;
  background: #33363d;
  position: relative;
  cursor: pointer;
}
/* 加大点击热区，不改变视觉 */
.seek::before {
  content: '';
  position: absolute;
  inset: -10px 0;
}
.seek i {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  border-radius: 2px;
  background: var(--ff-primary);
}
.seek b {
  position: absolute;
  top: 50%;
  width: 12px;
  height: 12px;
  margin: -6px 0 0 -6px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 0 0 3px rgba(91, 140, 255, 0.35);
}
.ctrl .chip {
  height: 22px;
  padding: 0 var(--ff-space-2) !important;
  border-radius: var(--ff-radius-sm);
  background: #26282e !important;
  font-size: var(--ff-fs-xs);
  display: flex;
  align-items: center;
  color: #d6d9de;
  flex: none;
}
/* 预览失败等：只有播放 / 上一帧 / 下一帧 / 进度条变灰，静音和全屏仍可用 */
.off .pb,
.off .seek,
.off .ci[aria-disabled='true'] {
  opacity: 0.4;
  cursor: not-allowed;
}
.ctrl button:focus-visible,
.seek:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
</style>
