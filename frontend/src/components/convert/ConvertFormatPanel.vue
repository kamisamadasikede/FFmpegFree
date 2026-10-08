<script setup lang="ts">
// v0.24 右栏主体（设计说明 §13.1、§八 第 35–38、42、43、66 条；截图 19–25、27、32）：
// 格式搜索框 → 分段（视频 / 音频 / 图片，搜索时换成“找到 n 个格式”）→ 格式块 → 说明（图片 / M4R）→ 参数 → 保存到。
import { computed, nextTick, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useConvertRecordsStore } from '@/stores/convertRecords'
import { convertV2IsReal, type FormatEntry } from '@/api/convertRecords'
import { simParam } from '@/api/sim'
import {
  FALLBACK_SAVE_HINT, FORMAT_CATEGORIES, FORMAT_CATEGORY_LABEL, FORMAT_SEARCH_CLEAR, FORMAT_SEARCH_PLACEHOLDER, IMAGE_FROM_VIDEO_NOTE, M4R_NOTE,
  firstUsableHit, formatFoundText, formatHitCount, formatNoneText, formatSubText, formatTileTitle, highlightParts, searchFormats, splitPathLastTwo,
} from '@/utils/convertV24Text'

const cv = useConvertRecordsStore()
const FALLBACK_HINT_TIP = '程序所在文件夹无法写入，文件已改存到用户数据目录'
/** 模拟截图用：?cv_hover=fmt:AMR 强制显示置灰格式的悬停提示 */
const mockHover = convertV2IsReal() ? '' : (simParam('cv_hover') ?? '')

const q = computed(() => cv.formatQuery.trim())
const groups = computed(() => (q.value ? searchFormats(cv.catalog, q.value) : []))
const hitCount = computed(() => formatHitCount(groups.value))
const tabItems = computed(() => cv.catalog.filter((f) => f.category === cv.tab))
const selExt = computed(() => cv.selectedPreset?.options.container ?? '')
const isOn = (f: FormatEntry) => f.extension === selExt.value
const nSel = computed(() => cv.selectedRows.length)

const note = computed(() => {
  if (q.value) return ''
  if (cv.tab === 'image' && cv.selectedHasVideo) return IMAGE_FROM_VIDEO_NOTE
  if (selExt.value === 'm4r') return M4R_NOTE
  return ''
})

/** 预设名按预设卡的标题显示（重名时带编码，如“MP4 · H.264”），和记录第 2 行一致 */
const presetLabel = (p: { id: string; name: string }) => {
  const item = cv.presets.find((x) => x.id === p.id)
  return item ? cv.presetTitle(item) : p.name
}
const curPreset = computed(() => cv.formatPresets.find((p) => p.id === cv.selectedPresetId))
const save = computed(() => {
  const p = cv.storage?.outputDir ?? ''
  return { full: p, ...splitPathLastTwo(p) }
})

// ---- 置灰格式的悬停提示：浮在右栏外层，不被格式区的滚动裁掉（原型同） ----
const rp = ref<HTMLElement | null>(null)
const tip = ref<{ text: string; top: number; right: number } | null>(null)
function showTip(e: Event, f: FormatEntry) {
  if (f.encodable || !rp.value) return
  const t = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const r = rp.value.getBoundingClientRect()
  tip.value = { text: f.reason || '当前转换组件不支持输出这个格式', top: t.bottom - r.top + 6, right: r.right - t.right }
}
const hideTip = () => (tip.value = null)
function forceTip(el: unknown, f: FormatEntry) {
  if (!mockHover.startsWith('fmt:') || el === null || mockHover.slice(4).toLowerCase() !== f.extension) return
  void nextTick(() => {
    const node = el as HTMLElement
    node.classList.add('hv')
    showTip({ currentTarget: node } as unknown as Event, f)
  })
}

// ---- 键盘（§13.3 第 7 条）：搜索框 ↓ 进入结果，方向键在格式块之间移动，Enter 选中，Esc 清空 ----
const grid = ref<HTMLElement | null>(null)
const tiles = () => Array.from(grid.value?.querySelectorAll<HTMLElement>('.cv-fx') ?? [])
function onSearchKey(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    tiles()[0]?.focus()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const f = firstUsableHit(groups.value)
    if (f) cv.selectFormat(f.extension)
  } else if (e.key === 'Escape') {
    e.preventDefault()
    cv.clearFormatQuery()
  }
}
const search = ref<HTMLInputElement | null>(null)
function onTileKey(e: KeyboardEvent) {
  const list = tiles()
  const i = list.indexOf(e.currentTarget as HTMLElement)
  const step: Record<string, number> = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: 3, ArrowUp: -3 }
  if (e.key in step) {
    e.preventDefault()
    const j = i + step[e.key]
    if (j < 0 && q.value) search.value?.focus()
    else list[Math.max(0, Math.min(list.length - 1, j))]?.focus()
  } else if (e.key === 'Escape' && q.value) {
    e.preventDefault()
    cv.clearFormatQuery()
    search.value?.focus()
  }
}
function pick(f: FormatEntry) {
  if (f.encodable) cv.selectFormat(f.extension)
}
function clear() {
  cv.clearFormatQuery()
  search.value?.focus()
}
</script>
<template>
  <div ref="rp" class="cv-rp cv-rp24" style="position: relative" @scroll.capture="hideTip">
    <div class="cv-fsearch" :class="{ has: !!cv.formatQuery }">
      <FIcon name="search" :size="14" />
      <input
        ref="search"
        type="text"
        :value="cv.formatQuery"
        :placeholder="FORMAT_SEARCH_PLACEHOLDER"
        aria-label="搜索格式"
        autocomplete="off"
        spellcheck="false"
        @input="cv.setFormatQuery(($event.target as HTMLInputElement).value)"
        @keydown="onSearchKey"
      />
      <button v-if="cv.formatQuery" type="button" class="x" :aria-label="FORMAT_SEARCH_CLEAR" :title="FORMAT_SEARCH_CLEAR" @click="clear"><FIcon name="x" :size="12" /></button>
    </div>
    <div v-if="q && hitCount" class="cv-fcount" role="status">{{ formatFoundText(hitCount) }}</div>
    <div v-else-if="!q" class="seg cv-seg3" role="tablist" aria-label="格式分类">
      <button v-for="c in FORMAT_CATEGORIES" :key="c" type="button" role="tab" :class="{ on: cv.tab === c }" :aria-selected="cv.tab === c" @click="cv.setTab(c)">{{ FORMAT_CATEGORY_LABEL[c] }}</button>
    </div>
    <div v-if="cv.presetsError" class="cv-gate" role="status"><FIcon name="warn" /><span>没有加载到格式列表。<button type="button" class="ff-link" @click="cv.loadPresets()">重试</button></span></div>
    <div ref="grid" class="cv-presets cv-fxs" :class="{ dim: !q && !nSel }" role="radiogroup" aria-label="输出格式" @scroll="hideTip">
      <template v-if="q">
        <div v-if="!hitCount" class="cv-fnone">
          <FIcon name="search" />
          <p>{{ formatNoneText(q) }}</p>
          <button type="button" @click="clear">{{ FORMAT_SEARCH_CLEAR }}</button>
        </div>
        <template v-for="g in groups" v-else :key="g.category">
          <div class="cv-fgrp">{{ g.label }}<i /></div>
          <div class="cv-fxg">
            <button
              v-for="h in g.items"
              :key="h.entry.extension"
              :ref="(el) => forceTip(el, h.entry)"
              type="button"
              class="preset cv-fx"
              :class="{ on: isOn(h.entry), off: !h.entry.encodable }"
              role="radio"
              :aria-checked="isOn(h.entry)"
              :aria-disabled="!h.entry.encodable || undefined"
              :title="h.entry.encodable ? formatTileTitle(h.entry) : undefined"
              @click="pick(h.entry)"
              @keydown="onTileKey"
              @mouseenter="showTip($event, h.entry)"
              @mouseleave="hideTip"
              @focus="showTip($event, h.entry)"
              @blur="hideTip"
            >
              <b><template v-for="(p, i) in highlightParts(h.entry.displayName, q)" :key="i"><mark v-if="p.hit">{{ p.t }}</mark><template v-else>{{ p.t }}</template></template><FIcon v-if="!h.entry.encodable" name="block" :size="12" /></b>
              <small :title="h.sub"><template v-for="(p, i) in highlightParts(h.sub, q)" :key="i"><mark v-if="p.hit">{{ p.t }}</mark><template v-else>{{ p.t }}</template></template></small>
            </button>
          </div>
        </template>
      </template>
      <div v-else class="cv-fxg">
        <button
          v-for="f in tabItems"
          :key="f.extension"
          :ref="(el) => forceTip(el, f)"
          type="button"
          class="preset cv-fx"
          :class="{ on: isOn(f), off: !f.encodable }"
          role="radio"
          :aria-checked="isOn(f)"
          :aria-disabled="!f.encodable || undefined"
          :title="f.encodable ? formatTileTitle(f) : undefined"
          @click="pick(f)"
          @keydown="onTileKey"
          @mouseenter="showTip($event, f)"
          @mouseleave="hideTip"
          @focus="showTip($event, f)"
          @blur="hideTip"
        >
          <b>{{ f.displayName }}<FIcon v-if="!f.encodable" name="block" :size="12" /></b>
          <small :title="formatSubText(f)">{{ formatSubText(f) }}</small>
        </button>
      </div>
    </div>
    <div v-if="note" class="cv-fnote"><FIcon name="info" :size="14" /><span>{{ note }}</span></div>
    <div class="cv-par">
      <label for="cv-par-sel">参数</label>
      <div class="select" :title="curPreset?.paramsSummary">
        <span>{{ curPreset ? presetLabel(curPreset) : '' }}</span><FIcon name="down" :size="14" />
        <select id="cv-par-sel" :value="cv.selectedPresetId" :disabled="!cv.formatPresets.length" @change="cv.selectPreset(($event.target as HTMLSelectElement).value)">
          <option v-for="p in cv.formatPresets" :key="p.id" :value="p.id" :title="p.paramsSummary">{{ presetLabel(p) }}</option>
        </select>
      </div>
    </div>
    <div class="cv-save">
      <label>保存到</label>
      <div class="row2">
        <div class="input cv-path" :title="save.full"><span class="h">{{ save.head }}</span><span class="t">{{ save.tail }}</span></div>
        <button type="button" class="btn cv-open" aria-label="打开文件夹" title="打开文件夹" :disabled="!cv.storage" @click="cv.openStorage('output')"><FIcon name="folder" :size="15" /></button>
      </div>
      <div v-if="cv.storage?.fellBack" class="cv-hint warn"><FIcon name="warn" :size="13" /><span :title="FALLBACK_HINT_TIP">{{ FALLBACK_SAVE_HINT }}</span></div>
      <div class="cv-hint">同名文件自动加序号，不会覆盖。<button type="button" class="ff-link" @click="cv.changeOutputDir()">更改</button></div>
    </div>
    <span v-if="tip" class="cv-tip cv-ftip cv-ftip-float" role="tooltip" :style="{ top: tip.top + 'px', right: tip.right + 'px', left: 'auto' }">{{ tip.text }}</span>
  </div>
</template>
