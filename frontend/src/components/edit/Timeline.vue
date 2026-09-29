<template>
  <section class="ed-panel ed-tl" aria-label="时间线">
    <!-- 工具栏：切割 / 删除 | 吸附 / 轨道 | 缩放 -->
    <div class="ed-tb" role="toolbar" aria-label="时间线工具栏">
      <button type="button" class="ed-tbtn" :class="{ tp: splitOff && ed.clipsFull.value }" aria-label="在播放头处切割（S）" :title="splitOff ? undefined : '在播放头处切割（S）'" :aria-disabled="splitOff ? 'true' : undefined" :data-tip="splitOff || undefined" @click="doSplit">
        <FIcon name="split" :size="16" /><span class="tx">切割</span>
      </button>
      <button type="button" class="ed-tbtn" aria-label="删除所选片段（Delete）" :title="delOff ? undefined : '删除所选片段（Delete）'" :aria-disabled="delOff ? 'true' : undefined" :data-tip="delOff || undefined" @click="doDelete">
        <FIcon name="trash" :size="16" /><span class="tx">删除</span>
      </button>
      <span class="vs"></span>
      <button type="button" class="ed-tbtn" :class="{ on: ed.snapOn.value }" :aria-pressed="ed.snapOn.value" :aria-label="`吸附（N）：${ed.snapOn.value ? '开' : '关'}`" :title="`吸附（N）：${ed.snapOn.value ? '开' : '关'}`" @click="ed.snapOn.value = !ed.snapOn.value">
        <FIcon name="magnet" :size="16" /><span class="tx">吸附</span>
      </button>
      <span class="ed-menu-wrap">
        <button ref="trackBtn" type="button" class="ed-tbtn" aria-label="添加轨道" aria-haspopup="menu" :aria-expanded="menuOpen" :aria-disabled="ffOff ? 'true' : undefined" :data-tip="ffOff || undefined" @click="toggleMenu">
          <FIcon name="plus" :size="16" /><span class="tx">轨道</span>
        </button>
        <div v-if="menuOpen" class="ed-menu" role="menu" style="left: 0; top: 32px" @keydown.esc.stop="closeMenu">
          <button v-for="k in KINDS" :key="k.kind" type="button" role="menuitem" :aria-disabled="!ed.canAddTrack(k.kind) ? 'true' : undefined" :data-tip="!ed.canAddTrack(k.kind) ? TEXT.trackLimitTip : undefined" @click="addTrack(k.kind)">
            {{ k.label }}
          </button>
        </div>
      </span>
      <span class="sp"></span>
      <button type="button" class="ed-tbtn ico" aria-label="缩小时间线（-）" title="缩小时间线（-）" @click="ed.zoomBy(0.8)"><FIcon name="zout" :size="16" /></button>
      <input class="ed-range ed-zoom" type="range" min="0" max="100" step="1" aria-label="时间线缩放" :value="zoomPos" :style="{ '--p': zoomPos + '%' }" @input="onZoom" />
      <button type="button" class="ed-tbtn ico" aria-label="放大时间线（+）" title="放大时间线（+）" @click="ed.zoomBy(1.25)"><FIcon name="zin" :size="16" /></button>
      <button type="button" class="ed-tbtn" aria-label="适应窗口（Shift+Z）" title="适应窗口（Shift+Z）" @click="ed.fitWindow()"><span class="tx">适应窗口</span><FIcon name="full" :size="16" /></button>
    </div>

    <div class="ed-tlbody">
      <div class="ed-heads" aria-hidden="true">
        <div v-for="t in ed.laneIds.value" :key="t" class="ed-head"><b>{{ t[0] }}</b>{{ t.slice(1) }}</div>
      </div>
      <div
        ref="lanesEl"
        class="ed-lanes"
        role="application"
        aria-label="时间线：Tab 选择片段，左右方向键移动，Alt 加左右方向键调整入点和出点，Delete 删除"
        @wheel="onWheel"
      >
        <div ref="rulerEl" class="ed-ruler" @pointerdown="onRulerDown">
          <template v-for="tk in ticks" :key="tk.t">
            <i :class="{ M: tk.major }" :style="{ left: tk.x + 'px' }"></i>
            <span :style="{ left: tk.x + 4 + 'px' }">{{ formatClock(tk.t) }}</span>
          </template>
        </div>

        <div
          v-for="t in ed.laneIds.value"
          :key="t"
          class="ed-lane"
          :class="{ deny: laneDeny(t) }"
          :data-t="t"
          @pointerdown.self="onLaneDown($event)"
        >
          <button
            v-for="c in clipsIn(t)"
            :key="c.id"
            type="button"
            class="ed-clip"
            :class="[isV(c) ? 'v' : 'a', { sel: ed.selectedId.value === c.id, bad: ed.errorClipId.value === c.id, dragging: drag?.id === c.id && drag.moved, tiny: clipW(c) < 36 }]"
            :style="{ left: px(c.startSec) + 'px', width: Math.max(2, clipW(c)) + 'px' }"
            :aria-label="clipLabel(c)"
            :aria-pressed="ed.selectedId.value === c.id"
            :aria-invalid="ed.errorClipId.value === c.id ? 'true' : undefined"
            @pointerdown="onClipDown($event, c)"
            @keydown="onClipKey($event, c)"
            @click.stop
          >
            <template v-if="clipW(c) >= 36">
              <span v-if="isV(c)" class="ed-cth" :style="{ background: thumbBg(c) }"></span>
              <span v-else class="wv"></span>
              <span v-if="ed.errorClipId.value === c.id" class="bang" aria-hidden="true"><FIcon name="warn" :size="10" /></span>
              <span class="nm" :title="ed.nameOfClip(c)">{{ ed.nameOfClip(c) }}</span>
              <span v-if="c.speed && c.speed !== 1" class="spd">{{ +c.speed.toFixed(2) }}×</span>
            </template>
            <template v-if="ed.selectedId.value === c.id">
              <span class="ed-hd l" @pointerdown.stop="onTrimDown($event, c, 'l')"></span>
              <span class="ed-hd r" @pointerdown.stop="onTrimDown($event, c, 'r')"></span>
            </template>
          </button>

          <div v-for="tr in transitionsIn(t)" :key="tr.id" class="ed-tr" :style="{ left: tr.x + 'px' }" :title="tr.title" role="img" :aria-label="tr.title">
            <FIcon name="magic" :size="12" />
          </div>

          <div v-if="ghost && ghost.trackId === t" class="ed-ghost" :class="ghost.problem ? 'dn' : 'ok'" :style="{ left: px(ghost.start) + 'px', width: Math.max(24, ghost.len * ed.pps.value) + 'px' }">
            <FIcon v-if="ghost.problem" name="block" :size="14" />
            <span class="gt">{{ ghost.text }}</span>
          </div>

          <div v-if="hintFor(t)" class="ed-lanes-hint">{{ hintFor(t) }}</div>
        </div>

        <div v-if="limitX !== null" class="ed-limitline" :style="{ left: limitX + 'px' }"><span>6 小时上限</span></div>
        <div v-if="snapX !== null" class="ed-snapline" :style="{ left: snapX + 'px' }"></div>
        <div v-if="playX !== null" class="ed-ph-line" :style="{ left: playX + 'px' }">
          <button
            type="button"
            class="ed-ph-grip"
            role="slider"
            aria-label="播放头"
            :aria-valuemin="0"
            :aria-valuemax="Math.max(ed.total.value, 1)"
            :aria-valuenow="ed.playhead.value"
            :aria-valuetext="formatTC(ed.playhead.value)"
            @pointerdown.stop="onRulerDown"
            @keydown.left.prevent="ed.seek(ed.playhead.value - ($event.shiftKey ? 1 : 1 / 30))"
            @keydown.right.prevent="ed.seek(ed.playhead.value + ($event.shiftKey ? 1 : 1 / 30))"
          ></button>
        </div>
      </div>
    </div>

    <div class="ed-tf">
      <span class="lim">{{ LIMIT_TEXT }}</span>
      <span class="sp"></span>
      <span>
        视频轨 {{ ed.usedVideoTracks.value }}/{{ MAX_VIDEO_TRACKS }} · 音频轨 {{ ed.usedAudioTracks.value }}/{{ MAX_AUDIO_TRACKS }} ·
        <span :class="{ warn: ed.allClips.value.length >= 95 }">片段 {{ ed.allClips.value.length }}/{{ MAX_CLIPS }}</span> ·
        <span :class="{ warn: ed.total.value >= 21300 }">{{ formatClock(ed.total.value) }} / {{ formatClock(MAX_TIMELINE_SEC, true) }}</span>
      </span>
    </div>

    <div class="ed-toast-live" role="status" aria-live="polite">
      <div v-if="ed.toast.value" :key="ed.toast.value.seq" class="ed-toast"><FIcon name="info" :size="16" /><span>{{ ed.toast.value.text }}</span></div>
    </div>
    <div v-if="floatLabel" class="ed-drag-float" :style="{ left: floatLabel.x + 12 + 'px', top: floatLabel.y + 12 + 'px' }"><FIcon name="film" :size="14" />{{ floatLabel.text }}</div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import type { VideoClip } from '@/api/edit'
import { useFFmpegStore } from '@/stores/ffmpeg'
import {
  LIMIT_TEXT, MAX_AUDIO_TRACKS, MAX_CLIPS, MAX_TIMELINE_SEC, MAX_VIDEO_TRACKS, MIN_CLIP_SEC, TEXT, clipEnd, clipLen, clipsOnTrack, formatClock, formatTC, isVideoTrackId, placeProblemText,
  round6, type AnyClip, type PlaceProblem,
} from '@/utils/editLogic'
import { useEditor } from './editor'

const ed = useEditor()
const ffmpeg = useFFmpegStore()
const lanesEl = ref<HTMLElement | null>(null)
const rulerEl = ref<HTMLElement | null>(null)
const trackBtn = ref<HTMLElement | null>(null)
const menuOpen = ref(false)
const KINDS = [
  { kind: 'V', label: '添加视频轨' },
  { kind: 'A', label: '添加音频轨' },
] as const

const px = (t: number) => (t - ed.offSec.value) * ed.pps.value
const clipW = (c: AnyClip) => clipLen(c) * ed.pps.value
const isV = (c: AnyClip) => isVideoTrackId(c.trackId)
const clipsIn = (t: string) => clipsOnTrack(ed.allClips.value, t)
const ffOff = computed(() => (ffmpeg.featuresBlocked ? TEXT.ffmpegTip : ''))
const splitOff = computed(() => ffOff.value || ed.splitReason.value || '')
const delOff = computed(() => ffOff.value || (ed.selectedId.value ? '' : '先选中一个片段'))

// ───────── 缩放滑块：2~64 像素/秒，对数刻度 ─────────
const zoomPos = computed(() => Math.round((Math.log(ed.pps.value / 2) / Math.log(32)) * 100))
function onZoom(e: Event) {
  const v = Number((e.target as HTMLInputElement).value)
  ed.zoomBy((2 * Math.pow(32, v / 100)) / ed.pps.value)
}
function onWheel(e: WheelEvent) {
  if (e.ctrlKey || e.metaKey) {
    e.preventDefault()
    const r = lanesEl.value!.getBoundingClientRect()
    ed.zoomBy(e.deltaY < 0 ? 1.15 : 1 / 1.15, ed.offSec.value + (e.clientX - r.left) / ed.pps.value)
  } else if (e.shiftKey || Math.abs(e.deltaX) > Math.abs(e.deltaY)) {
    e.preventDefault()
    ed.offSec.value = Math.max(0, ed.offSec.value + (e.deltaX || e.deltaY) / ed.pps.value)
  }
}

// ───────── 标尺 ─────────
const ticks = computed(() => {
  const steps = [1, 2, 5, 10, 30, 60, 300, 600, 1800, 3600]
  const step = steps.find((s) => s * ed.pps.value >= 64) ?? 3600
  const out: { t: number; x: number; major: boolean }[] = []
  const end = ed.offSec.value + ed.laneWidth.value / ed.pps.value
  for (let t = Math.floor(ed.offSec.value / step) * step; t <= end; t += step) out.push({ t, x: px(t), major: t % (step * 2) === 0 })
  return out
})
const playX = computed(() => {
  const x = px(ed.playhead.value)
  return x >= -1 && x <= ed.laneWidth.value + 1 ? x : null
})
const limitX = computed(() => {
  const x = px(MAX_TIMELINE_SEC)
  return x >= 0 && x <= ed.laneWidth.value ? x : null
})

function timeAt(clientX: number): number {
  const r = lanesEl.value!.getBoundingClientRect()
  return Math.max(0, ed.offSec.value + (clientX - r.left) / ed.pps.value)
}
function onRulerDown(e: PointerEvent) {
  if (e.button !== 0) return
  ed.stop()
  ed.seek(timeAt(e.clientX))
  const move = (ev: PointerEvent) => ed.seek(timeAt(ev.clientX))
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}
function onLaneDown(e: PointerEvent) {
  if (e.button !== 0) return
  ed.selectedId.value = null
  ed.seek(timeAt(e.clientX))
}

// ───────── 轨道菜单 ─────────
function toggleMenu() {
  if (ffOff.value) return ed.say(ffOff.value)
  menuOpen.value = !menuOpen.value
}
const closeMenu = () => {
  menuOpen.value = false
  trackBtn.value?.focus()
}
function addTrack(k: 'V' | 'A') {
  if (!ed.canAddTrack(k)) return ed.say(TEXT.trackLimitTip)
  ed.addTrack(k)
  menuOpen.value = false
}
function onDocDown(e: PointerEvent) {
  if (menuOpen.value && !(e.target as HTMLElement).closest('.ed-menu-wrap')) menuOpen.value = false
}

// ───────── 切割 / 删除 ─────────
function doSplit() {
  if (ffOff.value) return ed.say(ffOff.value)
  ed.splitAtPlayhead()
}
function doDelete() {
  if (ffOff.value) return ed.say(ffOff.value)
  if (!ed.selectedId.value) return ed.say('先选中一个片段')
  ed.deleteSelected()
}

// ───────── 片段显示 ─────────
const GRADS = ['var(--ff-smp-1)', 'var(--ff-smp-2)', 'var(--ff-smp-3)', 'var(--ff-smp-4)']
function thumbBg(c: AnyClip) {
  return GRADS[[...ed.nameOfClip(c)].reduce((h, ch) => (h * 31 + ch.charCodeAt(0)) >>> 0, 0) % 4]
}
const TRANS_NAME: Record<string, string> = {
  fade: '淡入淡出', wipeleft: '左擦除', wiperight: '右擦除', slideleft: '左滑动', slideright: '右滑动', circleopen: '圆形展开', circleclose: '圆形收拢', dissolve: '溶解',
}
function transitionsIn(t: string) {
  if (!isVideoTrackId(t)) return []
  return (clipsIn(t) as VideoClip[])
    .filter((c) => c.transitionToNext && c.transitionToNext !== 'none' && ed.nextTouching(c))
    .map((c) => ({ id: c.id, x: px(clipEnd(c)), title: `${TRANS_NAME[c.transitionToNext] ?? '转场'}，${+(c.transitionDurationSec || 0.5).toFixed(2)} 秒` }))
}
function clipLabel(c: AnyClip): string {
  const n = clipsOnTrack(ed.allClips.value, c.trackId).findIndex((x) => x.id === c.id) + 1
  const bits = [`${c.trackId} ${ed.nameOfClip(c)}`, `片段 ${n}`, `开始 ${+c.startSec.toFixed(2)} 秒`, `时长 ${+clipLen(c).toFixed(2)} 秒`]
  if (ed.selectedId.value === c.id) bits.push('已选中')
  if (ed.errorClipId.value === c.id) bits.push('导出时出错')
  return bits.join('，')
}
function hintFor(t: string): string {
  if (t !== 'V1' || ed.allClips.value.length) return ''
  return ed.sources.value.length ? '把素材拖到这里，或点素材右边的 +' : '先导入素材'
}

// ───────── 拖动（片段移动 / 素材放入 / 裁剪手柄）─────────
interface Ghost { trackId: string; start: number; len: number; problem: PlaceProblem; text: string }
const drag = ref<{ id: string; moved: boolean } | null>(null)
const ghost = ref<Ghost | null>(null)
const snapAt = ref<number | null>(null)
const denyLane = ref('')
const snapX = computed(() => (snapAt.value === null ? null : px(snapAt.value)))
const laneDeny = (t: string) => denyLane.value === t
const floatLabel = computed(() => {
  const d = ed.sourceDrag.value
  return d ? { x: d.x, y: d.y, text: ed.srcOf(d.path)?.name ?? '' } : null
})

function laneAt(clientY: number, clientX: number): string | null {
  const el = lanesEl.value
  if (!el) return null
  const r = el.getBoundingClientRect()
  if (clientX < r.left || clientX > r.right) return null
  const i = Math.floor((clientY - r.top - 24) / 40)
  return i >= 0 && i < ed.laneIds.value.length ? ed.laneIds.value[i] : null
}
function autoScroll(clientX: number) {
  const r = lanesEl.value!.getBoundingClientRect()
  if (clientX > r.right - 24) ed.offSec.value += 12 / ed.pps.value
  else if (clientX < r.left + 24) ed.offSec.value = Math.max(0, ed.offSec.value - 12 / ed.pps.value)
}

function onClipDown(e: PointerEvent, c: AnyClip) {
  if (e.button !== 0) return
  e.stopPropagation()
  ed.stop()
  ed.selectClip(c.id)
  if (ffmpeg.featuresBlocked) return
  const x0 = e.clientX
  const start0 = c.startSec
  const track0 = c.trackId
  const len = clipLen(c)
  const state = { id: c.id, moved: false }
  drag.value = state
  let last: { trackId: string; start: number; problem: PlaceProblem } | null = null
  const move = (ev: PointerEvent) => {
    if (!state.moved && Math.abs(ev.clientX - x0) < 4 && laneAt(ev.clientY, ev.clientX) === track0) return
    state.moved = true
    autoScroll(ev.clientX)
    const t = laneAt(ev.clientY, ev.clientX) ?? last?.trackId ?? track0
    let start = Math.max(0, start0 + (ev.clientX - x0) / ed.pps.value)
    const sn = ed.snapTime(start, len, c.id, ev.altKey)
    start = sn.start
    snapAt.value = sn.at
    const problem = ed.evaluateMove(c.id, t, start)
    last = { trackId: t, start, problem }
    denyLane.value = problem === 'overlap' ? t : ''
    ghost.value = { trackId: t, start, len, problem, text: problem ? `${ed.nameOfClip(c)} · 不能放在这里` : ed.nameOfClip(c) }
  }
  const finish = (cancel: boolean) => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('keydown', key, true)
    ghost.value = null
    snapAt.value = null
    denyLane.value = ''
    drag.value = null
    if (cancel || !state.moved || !last) return
    if (last.problem) {
      // 松手回弹：片段本来就没动，直接保持原位（不做位移动画，减少动效设置下也一致）
      ed.say(placeProblemText(last.problem))
      return
    }
    ed.moveClip(c.id, last.trackId, last.start)
  }
  const up = () => finish(false)
  const key = (ev: KeyboardEvent) => {
    if (ev.key === 'Escape') {
      ev.stopPropagation()
      finish(true)
    }
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('keydown', key, true)
}

function onTrimDown(e: PointerEvent, c: AnyClip, side: 'l' | 'r') {
  if (e.button !== 0 || ffmpeg.featuresBlocked) return
  e.preventDefault()
  const x0 = e.clientX
  const orig = { start: c.startSec, inSec: c.inSec, outSec: c.outSec }
  const speed = c.speed > 0 ? c.speed : 1
  const srcDur = ed.durOf(c)
  let bad: PlaceProblem = null
  const move = (ev: PointerEvent) => {
    const dsec = (ev.clientX - x0) / ed.pps.value
    let r: PlaceProblem
    if (side === 'l') {
      const maxD = (orig.outSec - orig.inSec) / speed - MIN_CLIP_SEC
      const d = Math.min(Math.max(dsec, -Math.min(orig.start, orig.inSec / speed)), maxD)
      r = ed.tryUpdate(c.id, { startSec: round6(orig.start + d), inSec: round6(orig.inSec + d * speed) })
    } else {
      const d = Math.max(dsec, -((orig.outSec - orig.inSec) / speed - MIN_CLIP_SEC))
      const out = Math.min(srcDur, Math.max(orig.inSec + MIN_CLIP_SEC * speed, orig.outSec + d * speed))
      r = ed.tryUpdate(c.id, { outSec: round6(out) })
    }
    bad = r
  }
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    if (bad) ed.say(placeProblemText(bad))
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

// 键盘：←/→ 移动 1 帧（Shift 1 秒），Alt+←/→ 调整入点 / 出点，Delete 删除
function onClipKey(e: KeyboardEvent, c: AnyClip) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    e.stopPropagation()
    ed.selectClip(c.id)
    return
  }
  if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
    e.preventDefault()
    e.stopPropagation()
    if (ffmpeg.featuresBlocked) return
    ed.selectedId.value = c.id
    const dir = e.key === 'ArrowLeft' ? -1 : 1
    const step = e.shiftKey ? 1 : 1 / 30
    let r: PlaceProblem
    if (e.altKey) {
      // Alt+←/→ 调整入点（片段头部缩短 / 拉长）；Alt+Shift+←/→ 调整出点（片段尾部），每次 1 帧
      const speed = c.speed > 0 ? c.speed : 1
      const fr = 1 / 30
      if (!e.shiftKey) {
        const d = dir * fr
        const okD = Math.max(-Math.min(c.startSec, c.inSec / speed), Math.min(d, clipLen(c) - MIN_CLIP_SEC))
        r = ed.tryUpdate(c.id, { startSec: round6(c.startSec + okD), inSec: round6(c.inSec + okD * speed) })
      } else {
        const out = Math.min(ed.durOf(c), Math.max(c.inSec + MIN_CLIP_SEC * speed, c.outSec + dir * fr * speed))
        r = ed.tryUpdate(c.id, { outSec: round6(out) })
      }
    } else {
      r = ed.tryUpdate(c.id, { startSec: Math.max(0, round6(c.startSec + dir * step)) })
    }
    if (r) ed.say(placeProblemText(r))
  }
}

// 素材库拖入
const sd = ed.sourceDrag
function computeSourceGhost() {
  const d = sd.value
  if (!d) {
    ghost.value = null
    denyLane.value = ''
    snapAt.value = null
    return
  }
  const t = laneAt(d.y, d.x)
  const s = ed.srcOf(d.path)
  if (!t || !s) {
    ghost.value = null
    denyLane.value = ''
    return
  }
  autoScroll(d.x)
  const len = s.duration
  const sn = ed.snapTime(Math.max(0, timeAt(d.x)), len, null)
  snapAt.value = sn.at
  let problem: PlaceProblem = null
  let text = s.name
  if (!ed.kindOkFor(s, t)) {
    problem = 'track'
    text = TEXT.trackMismatch
    denyLane.value = t
  } else {
    const cand = { path: s.path, trackId: t, startSec: sn.start, inSec: 0, outSec: len, speed: 1 } as AnyClip
    problem = ed.evaluateCandidate(cand)
    denyLane.value = problem === 'overlap' ? t : ''
    if (problem) text = problem === 'overlap' ? `${s.name} · 不能放在这里` : placeProblemText(problem)
  }
  ghost.value = { trackId: t, start: sn.start, len, problem, text }
}
import { watch } from 'vue'
watch(() => [sd.value?.x, sd.value?.y, sd.value?.path], computeSourceGhost)

function dropFromBin() {
  const d = sd.value
  const g = ghost.value
  const gh = g
  ghost.value = null
  denyLane.value = ''
  snapAt.value = null
  if (!d) return
  const panel = lanesEl.value?.closest('.ed-tl')?.getBoundingClientRect()
  const inPanel = !!panel && d.x >= panel.left && d.x <= panel.right && d.y >= panel.top && d.y <= panel.bottom
  if (!inPanel) return
  const s = ed.srcOf(d.path)
  if (!s) return
  if (gh) {
    const p = ed.dropSource(d.path, gh.trackId, gh.start)
    if (p) ed.say(placeProblemText(p))
    return
  }
  // 落在轨道之外：放到最上面一条有空位的同类轨道的播放头位置
  const cands = ed.laneIds.value.filter((t) => ed.kindOkFor(s, t))
  let last: PlaceProblem = null
  for (const t of cands) {
    const p = ed.dropSource(d.path, t, ed.playhead.value)
    if (!p) return
    last = p
  }
  ed.say(last ? placeProblemText(last) : TEXT.trackMismatch)
}

// ───────── 尺寸 ─────────
let ro: ResizeObserver | null = null
onMounted(() => {
  ed.hooks.dropSource = dropFromBin
  ro = new ResizeObserver(() => (ed.laneWidth.value = lanesEl.value?.clientWidth || 720))
  if (lanesEl.value) ro.observe(lanesEl.value)
  document.addEventListener('pointerdown', onDocDown)
})
onBeforeUnmount(() => {
  ro?.disconnect()
  document.removeEventListener('pointerdown', onDocDown)
  if (ed.hooks.dropSource === dropFromBin) ed.hooks.dropSource = undefined
})
</script>

<style scoped>
.ed-menu-wrap {
  position: relative;
  display: inline-flex;
}
.ed-toast-live {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 40px;
  display: flex;
  justify-content: center;
  pointer-events: none;
}
.ed-toast-live .ed-toast {
  position: static;
  transform: none;
}
</style>
