<template>
  <section class="ed-panel ed-insp" aria-label="属性">
    <div class="ed-tabs" role="tablist" style="gap: 16px; flex: none">
      <button id="edt-clip" type="button" role="tab" :aria-selected="tab === 'clip'" :class="{ on: tab === 'clip' }" @click="tab = 'clip'">片段</button>
      <button id="edt-fx" type="button" role="tab" :aria-selected="tab === 'fx'" :class="{ on: tab === 'fx' }" @click="tab = 'fx'">全局效果</button>
    </div>

    <div v-if="tab === 'fx'" class="ed-ib" role="tabpanel" aria-labelledby="edt-fx">
      <div v-for="f in FX" :key="f.key" class="ed-f">
        <label :for="'fx-' + f.key">{{ f.label }}</label>
        <input :id="'fx-' + f.key" class="ed-range" type="range" :min="f.min" :max="f.max" :step="f.step" :aria-label="f.label" :value="ed.project.effects[f.key]" :style="{ '--p': pct(ed.project.effects[f.key], f.min, f.max) + '%' }" @input="setFx(f.key, ($event.target as HTMLInputElement).value)" />
        <span class="ed-v" style="text-align: right">{{ +ed.project.effects[f.key].toFixed(2) }}</span>
        <button type="button" class="ed-lk" style="font-size: 12px" :aria-label="`重置${f.label}`" @click="setFx(f.key, f.def)">重置</button>
      </div>
      <div class="ed-hint">效果作用于整个导出视频，预览为近似效果。</div>
    </div>

    <div v-else-if="!c" class="ed-empty" role="tabpanel" aria-labelledby="edt-clip">{{ ed.sources.value.length || ed.allClips.value.length ? '选中时间线上的一个片段，在这里调整它的属性。' : '导入素材并放到时间线后，选中片段就能在这里调整速度、滤镜和转场。' }}</div>

    <div v-else class="ed-ib" role="tabpanel" aria-labelledby="edt-clip">
      <div class="ed-sel">
        <span class="ed-tag" :class="isV ? 'v' : 'a'">{{ c.trackId }} · 片段 {{ number }}</span>
        <b :title="ed.nameOfClip(c)">{{ ed.nameOfClip(c) }}</b>
      </div>
      <div class="ed-g2">
        <div class="ed-f"><label for="ci-start">开始</label><input id="ci-start" v-model="txt.start" class="ed-in mono" :class="{ warn: warn.start }" :aria-describedby="warn.start ? 'ci-msg-start' : undefined" @keydown.enter="commit('start')" @blur="commit('start')" @focus="warn.start = ''" /></div>
        <div class="ed-f"><label for="ci-in">入点</label><input id="ci-in" v-model="txt.inSec" class="ed-in mono" :class="{ warn: warn.inSec }" @keydown.enter="commit('inSec')" @blur="commit('inSec')" @focus="warn.inSec = ''" /></div>
        <div v-if="warn.start" id="ci-msg-start" class="ed-fmsg warn" role="status"><FIcon name="warn" :size="14" /><span>{{ warn.start }}</span></div>
        <div v-if="warn.inSec" class="ed-fmsg warn" role="status"><FIcon name="warn" :size="14" /><span>{{ warn.inSec }}</span></div>
      </div>
      <div class="ed-g2">
        <div class="ed-f"><label for="ci-out">出点</label><input id="ci-out" v-model="txt.outSec" class="ed-in mono" :class="{ warn: warn.outSec }" @keydown.enter="commit('outSec')" @blur="commit('outSec')" @focus="warn.outSec = ''" /></div>
        <div class="ed-f"><label for="ci-len">时长</label><input id="ci-len" class="ed-in mono ro" :value="formatPrecise(clipLen(c))" readonly aria-readonly="true" /></div>
        <div v-if="warn.outSec" class="ed-fmsg warn" role="status"><FIcon name="warn" :size="14" /><span>{{ warn.outSec }}</span></div>
      </div>
      <div class="ed-hr"></div>

      <div class="ed-f">
        <label for="ci-speed">速度</label>
        <input id="ci-speed" class="ed-range" type="range" min="0.25" max="4" step="0.05" aria-label="速度" :value="c.speed || 1" :style="{ '--p': pct(c.speed || 1, 0.25, 4) + '%' }" @input="setSpeed(($event.target as HTMLInputElement).value)" />
        <input class="ed-v" :value="`${+(c.speed || 1).toFixed(2)}×`" aria-label="速度数值" @change="setSpeed(($event.target as HTMLInputElement).value.replace('×', ''))" />
      </div>
      <div v-if="warn.speed" class="ed-fmsg warn" role="status"><FIcon name="warn" :size="14" /><span>{{ warn.speed }}</span></div>

      <template v-if="isV">
        <div class="ed-f">
          <label for="ci-fx">滤镜</label>
          <select id="ci-fx" class="ed-in" :value="v!.effectPreset || 'none'" @change="upd({ effectPreset: ($event.target as HTMLSelectElement).value as any })">
            <option v-for="o in PRESETS" :key="o.v" :value="o.v">{{ o.t }}</option>
          </select>
        </div>
        <div class="ed-f">
          <label for="ci-blur">模糊</label>
          <input id="ci-blur" class="ed-range" type="range" min="0" max="4" step="0.1" aria-label="模糊" :value="v!.blur" :style="{ '--p': pct(v!.blur, 0, 4) + '%' }" @input="upd({ blur: Number(($event.target as HTMLInputElement).value) })" />
          <span class="ed-v">{{ +v!.blur.toFixed(1) }}</span>
        </div>
        <div class="ed-f">
          <label for="ci-tr">转场</label>
          <select id="ci-tr" class="ed-in" :value="v!.transitionToNext || 'none'" :aria-disabled="!fits ? 'true' : undefined" :aria-describedby="!fits ? 'ci-tr-tip' : undefined" @change="onTransition(($event.target as HTMLSelectElement).value)">
            <option v-for="o in TRANS" :key="o.v" :value="o.v">{{ o.t }}</option>
          </select>
        </div>
        <div v-if="!fits && hasNext" id="ci-tr-tip" class="ed-fmsg warn"><FIcon name="warn" :size="14" /><span>{{ TEXT.transitionTooShort }}</span></div>
        <div v-else-if="!hasNext" class="ed-hint" style="margin-left: 56px">转场用于首尾相接的两个片段之间。</div>
        <div v-if="showDur" class="ed-f">
          <label for="ci-trd">转场时长</label>
          <input id="ci-trd" v-model="txt.trd" class="ed-in mono" :class="{ warn: warn.trd }" :aria-describedby="warn.trd ? 'ci-trd-msg' : undefined" @keydown.enter="commitTrd" @blur="commitTrd" @focus="warn.trd = ''" />
        </div>
        <div v-if="warn.trd" id="ci-trd-msg" class="ed-fmsg warn" role="status"><FIcon name="warn" :size="14" /><span>{{ warn.trd }}</span></div>
      </template>
      <template v-else>
        <div class="ed-f">
          <label for="ci-vol">音量</label>
          <input id="ci-vol" class="ed-range" type="range" min="0" max="4" step="0.05" aria-label="音量" :value="a!.volume" :style="{ '--p': pct(a!.volume, 0, 4) + '%' }" @input="upd({ volume: Number(($event.target as HTMLInputElement).value) })" />
          <span class="ed-v">{{ Math.round(a!.volume * 100) }}%</span>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import type { AudioClip, GlobalEffects, VideoClip } from '@/api/edit'
import {
  MIN_CLIP_SEC, TEXT, clampSilently, clampTransitionInput, clipLen, clipsOnTrack, formatPrecise, isVideoTrackId, maxTransitionSec, parseTC, placeProblemText, rangeText, round6, transitionFits,
} from '@/utils/editLogic'
import { useEditor } from './editor'

const ed = useEditor()
const tab = ref<'clip' | 'fx'>('clip')
const c = computed(() => ed.selected.value)
const isV = computed(() => !!c.value && isVideoTrackId(c.value.trackId))
const v = computed(() => (isV.value ? (c.value as VideoClip) : null))
const a = computed(() => (!isV.value ? (c.value as AudioClip) : null))
const number = computed(() => (c.value ? clipsOnTrack(ed.allClips.value, c.value.trackId).findIndex((x) => x.id === c.value!.id) + 1 : 0))

const PRESETS = [
  { v: 'none', t: '无' }, { v: 'grayscale', t: '黑白' }, { v: 'sepia', t: '复古褐' }, { v: 'vintage', t: '怀旧' }, { v: 'cinematic', t: '电影感' },
]
const TRANS = [
  { v: 'none', t: '无' }, { v: 'fade', t: '淡入淡出' }, { v: 'wipeleft', t: '左擦除' }, { v: 'wiperight', t: '右擦除' }, { v: 'slideleft', t: '左滑动' },
  { v: 'slideright', t: '右滑动' }, { v: 'circleopen', t: '圆形展开' }, { v: 'circleclose', t: '圆形收拢' }, { v: 'dissolve', t: '溶解' },
]
const FX: { key: keyof GlobalEffects; label: string; min: number; max: number; step: number; def: number }[] = [
  { key: 'brightness', label: '亮度', min: -0.5, max: 0.5, step: 0.01, def: 0 },
  { key: 'contrast', label: '对比度', min: 0.5, max: 2, step: 0.01, def: 1 },
  { key: 'saturation', label: '饱和度', min: 0, max: 2, step: 0.01, def: 1 },
  { key: 'sharpen', label: '锐化', min: 0, max: 2, step: 0.05, def: 0 },
]
const pct = (val: number, min: number, max: number) => Math.round(((val - min) / (max - min)) * 100)
function setFx(k: keyof GlobalEffects, raw: number | string) {
  const f = FX.find((x) => x.key === k)!
  ed.project.effects[k] = Math.min(f.max, Math.max(f.min, Number(raw) || 0))
  ed.touch()
}

// 文本框（开始 / 入点 / 出点 / 转场时长）：输入中不改工程，回车或失焦才提交；越界给行内提示并回退到最近的合法值
const txt = reactive({ start: '', inSec: '', outSec: '', trd: '' })
const warn = reactive({ start: '', inSec: '', outSec: '', speed: '', trd: '' })
function sync() {
  const x = c.value
  if (!x) return
  txt.start = formatPrecise(x.startSec)
  txt.inSec = formatPrecise(x.inSec)
  txt.outSec = formatPrecise(x.outSec)
  txt.trd = `${+(v.value?.transitionDurationSec || 0.5).toFixed(2)} 秒`
}
watch(() => [c.value?.id, c.value?.startSec, c.value?.inSec, c.value?.outSec, c.value?.speed, v.value?.transitionDurationSec], sync, { immediate: true })
watch(() => c.value?.id, () => Object.assign(warn, { start: '', inSec: '', outSec: '', speed: '', trd: '' }))

type Field = 'start' | 'inSec' | 'outSec'
function commit(f: Field) {
  const x = c.value
  if (!x) return
  const t = parseTC(txt[f])
  warn[f] = ''
  const cur = f === 'start' ? x.startSec : f === 'inSec' ? x.inSec : x.outSec
  if (t === null) {
    txt[f] = formatPrecise(cur)
    warn[f] = '请输入时间，例如 00:12.50'
    return
  }
  const speed = x.speed > 0 ? x.speed : 1
  const srcDur = ed.durOf(x)
  let patch: Partial<VideoClip & AudioClip>
  let val = t
  if (f === 'start') {
    patch = { startSec: round6(val) }
  } else if (f === 'inSec') {
    const max = Math.max(0, x.outSec - MIN_CLIP_SEC * speed)
    if (val > max) { val = max; warn[f] = `入点最晚到 ${formatPrecise(max)}` }
    patch = { inSec: round6(val) }
  } else {
    const min = x.inSec + MIN_CLIP_SEC * speed
    if (val < min) { val = min; warn[f] = `出点最早到 ${formatPrecise(min)}` }
    if (srcDur && val > srcDur) { val = srcDur; warn[f] = `出点最晚到素材末尾 ${formatPrecise(srcDur)}` }
    patch = { outSec: round6(val) }
  }
  const r = ed.tryUpdate(x.id, patch)
  if (r) {
    warn[f] = r === 'overlap' ? TEXT.overlapAfterInline.replace('会和后面的片段重叠', '会和同轨的其它片段重叠') : placeProblemText(r)
    txt[f] = formatPrecise(cur)
  } else txt[f] = formatPrecise(f === 'start' ? patch.startSec! : f === 'inSec' ? patch.inSec! : patch.outSec!)
}
function setSpeed(raw: string | number) {
  const x = c.value
  if (!x) return
  warn.speed = ''
  const n = Number(raw)
  if (!Number.isFinite(n) || n < 0.25 || n > 4) {
    warn.speed = rangeText(0.25, 4)
    return
  }
  const r = ed.tryUpdate(x.id, { speed: Math.round(n * 100) / 100 })
  // 速度调小让片段变长、撞到后一个片段：行内提示并回退（文案待产品确认）
  if (r === 'overlap') warn.speed = TEXT.overlapAfterInline
  else if (r) warn.speed = placeProblemText(r)
}
function upd(patch: Partial<VideoClip & AudioClip>) {
  if (c.value) ed.tryUpdate(c.value.id, patch)
}

// 转场：只在与后一个首尾相接的片段之间；时长上限 = 较短片段的一半（且 ≤ 2 秒、≥ 0.1 秒）
const next = computed(() => (c.value && isV.value ? ed.nextTouching(c.value) : null))
const hasNext = computed(() => !!next.value)
const maxTr = computed(() => (c.value && next.value ? maxTransitionSec(c.value, next.value) : 0))
const fits = computed(() => !hasNext.value || transitionFits(maxTr.value))
const showDur = computed(() => hasNext.value && fits.value && v.value && v.value.transitionToNext && v.value.transitionToNext !== 'none')
function onTransition(val: string) {
  if (!fits.value || !hasNext.value) {
    sync()
    return
  }
  const x = v.value!
  warn.trd = ''
  // 只选了类型、没手动改时长：默认 0.5 秒超过上限时静默缩短，不提示
  ed.tryUpdate(x.id, { transitionToNext: val as any, transitionDurationSec: val === 'none' ? 0 : clampSilently(x.transitionDurationSec || 0.5, maxTr.value) })
}
function commitTrd() {
  const x = v.value
  if (!x || !hasNext.value) return
  const t = parseFloat(txt.trd)
  if (!Number.isFinite(t)) {
    txt.trd = `${+(x.transitionDurationSec || 0.5).toFixed(2)} 秒`
    return
  }
  const r = clampTransitionInput(Math.min(t, 2), maxTr.value)
  const over = r.over || t > maxTr.value + 1e-9
  ed.tryUpdate(x.id, { transitionDurationSec: r.value })
  txt.trd = `${+r.value.toFixed(2)} 秒`
  warn.trd = over ? TEXT.transitionOver : ''
}
</script>
