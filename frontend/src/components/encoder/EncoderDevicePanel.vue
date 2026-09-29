<template>
  <section class="panel group encdev" :aria-labelledby="headingId">
    <div class="phead"><h2 :id="headingId">{{ ENCODER_PANEL_TITLE }}</h2></div>
    <div class="srow">
      <div class="l">
        <b>{{ ENCODER_PANEL_TITLE }}</b>
        <small>{{ ENCODER_ROW_DESC }}</small>
      </div>
      <div ref="root" class="dvc">
        <div
          class="select"
          :class="{ open, warn: view.selectWarn }"
          role="combobox"
          tabindex="0"
          aria-haspopup="listbox"
          :aria-expanded="open"
          :aria-controls="listId"
          :aria-label="encoderComboLabel(view.selectText)"
          :aria-disabled="view.selectDisabled || undefined"
          :title="view.selectText"
          @click="toggle"
          @keydown="onKey"
        >
          <FIcon v-if="view.selectWarn" class="wi" name="warn" :size="14" />
          <span class="txt">{{ view.selectText }}</span>
          <FIcon name="down" :size="14" />
        </div>
        <div v-if="open" :id="listId" class="dvm" role="listbox" :aria-label="ENCODER_PANEL_TITLE">
          <div
            v-for="(o, idx) in options"
            :key="o.key"
            class="it"
            :class="{ on: o.key === view.selectedKey, hv: idx === hover }"
            role="option"
            :aria-selected="o.key === view.selectedKey"
            @mouseenter="hover = idx"
            @click="choose(o.key)"
          >
            <FIcon name="check" :size="14" /><span class="nm" :title="o.label">{{ o.label }}</span>
          </div>
        </div>
      </div>
      <button type="button" class="btn" :aria-disabled="view.redetectDisabled || undefined" @click="!view.redetectDisabled && detect(true)">
        <FIcon name="refresh" :size="15" />{{ ENCODER_REDETECT }}
      </button>
    </div>
    <div class="srow dvn" :class="view.note.tone" :role="view.note.tone === 'err' ? 'alert' : 'status'">
      <i v-if="view.note.spinner" class="spin" aria-hidden="true" />
      <FIcon v-else :name="noteIcon" :size="16" />
      <div class="t">{{ view.note.text }}</div>
      <div v-if="view.note.action === 'resetAuto'" class="acts">
        <button type="button" class="btn text" @click="choose(PREF_AUTO)">{{ ENCODER_UNAVAILABLE_ACTION }}</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
// 设置页“编码设备”面板（设计稿 编码设备-设计说明-v0.1 §2）。下拉是自绘的 combobox/listbox（行高 32、分组“显卡”），置灰用 aria-disabled。
// 只在 encoderPanelVisible() 为真时由 Settings.vue 渲染：后端绑定接通（ENCODER_BACKEND_READY 且在 Wails 里）才对用户显示；纯浏览器需要 ?enc=。
// 偏好值存的是 'auto' | 'cpu' | 设备 id；所选设备不可用时偏好保持原值（后端约定），这里不擅自改写。
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { toAppError } from '@/api/call'
import { simParam } from '@/api/sim'
import {
  PREF_AUTO, PREF_CPU, encoderIsReal, getEncoderPreference, getEncoderPreferenceInfo, listEncoderDevices, refreshEncoderDevices, setEncoderPreference,
  type EncoderDeviceList, type EncoderPreferenceInfo,
} from '@/api/encoder'
import { createSeq, deriveEncoderView } from '@/api/encoderView'
import { useFFmpegStore } from '@/stores/ffmpeg'
import {
  ENCODER_OPTION_AUTO, ENCODER_OPTION_CPU, ENCODER_PANEL_TITLE, ENCODER_REDETECT, ENCODER_ROW_DESC, ENCODER_UNAVAILABLE_ACTION, encoderComboLabel,
} from '@/errors/encoderMessages'

defineProps<{ headingId?: string }>()
const ffmpeg = useFFmpegStore()
const uid = useId()
const listId = `ff-enc-${uid}`
const list = ref<EncoderDeviceList | null>(null)
const pref = ref<string>(PREF_AUTO)
const info = ref<EncoderPreferenceInfo | null>(null)
const loading = ref(true)
const failed = ref(false)
const open = ref(false)
const hover = ref(0)
const root = ref<HTMLElement | null>(null)

const view = computed(() => deriveEncoderView({ loading: loading.value, failed: failed.value, list: list.value, pref: pref.value, info: info.value, ffmpegReady: ffmpeg.ready || ffmpeg.status.state === 'checking' }))
const noteIcon = computed(() => (view.value.note.tone === 'ok' ? 'check' : view.value.note.tone === 'info' ? 'info' : 'warn'))
const options = computed(() => [
  { key: PREF_AUTO, label: ENCODER_OPTION_AUTO },
  { key: PREF_CPU, label: ENCODER_OPTION_CPU },
  ...view.value.gpus.map((g) => ({ key: g.id, label: g.name })),
])

// 事件序号（沿用 ffmpeg store 的做法）：detectSeq 管“列表 + loading”，prefSeq 管“偏好 + 偏好信息”。
// 后发起的请求会让先发起的返回作废，避免慢返回盖掉新结果（例如重新检测还没回来时又改了选择）。
const detectSeq = createSeq()
const prefSeq = createSeq()

/** 读偏好和偏好信息（GetEncoderPreference + GetEncoderPreferenceInfo）；信息读不到不算失败，退回用列表里的名字 */
async function readPref(): Promise<{ p: string; i: EncoderPreferenceInfo | null }> {
  const [p, i] = await Promise.all([getEncoderPreference(), getEncoderPreferenceInfo().catch(() => null)])
  return { p, i }
}

/** refresh=true 走 RefreshEncoderDevices（强制重测），否则走 ListEncoderDevices（后端有缓存） */
async function detect(refresh = false) {
  const my = detectSeq.next()
  const myP = prefSeq.next()
  loading.value = true
  failed.value = false
  open.value = false
  try {
    const [l, r] = await Promise.all([refresh ? refreshEncoderDevices() : listEncoderDevices(), readPref()])
    if (detectSeq.isCurrent(my)) list.value = l
    if (prefSeq.isCurrent(myP)) {
      pref.value = r.p
      info.value = r.i
    }
    if (detectSeq.isCurrent(my) && !encoderIsReal() && (simParam('enc') ?? '').endsWith('-open')) open.value = true // 演示：?enc=found-open / none-open 直接展开下拉
  } catch (e) {
    if (!detectSeq.isCurrent(my)) return
    failed.value = true
    void toAppError(e) // 详情不给用户看，界面只显示定稿的失败文案
  } finally {
    if (detectSeq.isCurrent(my)) loading.value = false
  }
}

/** 选择后：先乐观显示，SetEncoderPreference 成功后重读偏好信息（以后端为准）；失败回滚并提示 */
async function choose(key: string) {
  open.value = false
  if (key === pref.value) return
  const prev = pref.value
  const prevInfo = info.value
  const myP = prefSeq.next()
  pref.value = key
  info.value = null
  try {
    await setEncoderPreference(key)
  } catch (e) {
    if (prefSeq.isCurrent(myP)) {
      pref.value = prev
      info.value = prevInfo
    }
    ElMessage.error(toAppError(e).message)
    return
  }
  try {
    const r = await readPref()
    if (!prefSeq.isCurrent(myP)) return
    pref.value = r.p
    info.value = r.i
  } catch (e) {
    void toAppError(e) // 重读失败：保留刚选的值，名字退回用列表里的
  }
}

function toggle() {
  if (view.value.selectDisabled) return
  open.value = !open.value
  if (open.value) hover.value = Math.max(0, options.value.findIndex((o) => o.key === view.value.selectedKey))
}
function onKey(e: KeyboardEvent) {
  if (view.value.selectDisabled) return
  const n = options.value.length
  if (e.key === 'Escape') return void (open.value = false)
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    if (!open.value) return toggle()
    hover.value = (hover.value + (e.key === 'ArrowDown' ? 1 : n - 1)) % n
  } else if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    if (open.value) void choose(options.value[hover.value].key)
    else toggle()
  }
}
function onDoc(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node)) open.value = false
}

onMounted(() => {
  document.addEventListener('mousedown', onDoc)
  void detect()
})
onBeforeUnmount(() => document.removeEventListener('mousedown', onDoc))
// ffmpeg 安装完成后自动重新检测
watch(() => ffmpeg.ready, (ok) => ok && void detect())
</script>

<style scoped>
.dvc {
  position: relative;
  width: 280px;
  flex: none;
}
.select {
  height: 28px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  background: var(--ff-bg-surface);
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  cursor: pointer;
}
.select .txt {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.select > svg {
  color: var(--ff-text-2);
}
.select:hover {
  background: var(--ff-bg-hover);
}
.select:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.select.open {
  border-color: var(--ff-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ff-primary) 14%, transparent);
}
.select.open > svg:last-child {
  transform: rotate(180deg);
}
.select.warn {
  border-color: var(--ff-warning);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ff-warning) 14%, transparent);
}
.select.warn .wi {
  color: var(--ff-warning-text);
}
.select[aria-disabled='true'] {
  background: var(--ff-bg-hover);
  color: var(--ff-text-3);
  cursor: not-allowed;
}
.select[aria-disabled='true'] > svg {
  color: var(--ff-text-3);
}
.dvm {
  position: absolute;
  left: 0;
  top: 32px;
  width: 100%;
  z-index: 12;
  padding: 4px;
  border-radius: 8px;
  background: var(--ff-bg-elevated);
  border: 1px solid var(--ff-border);
  box-shadow: var(--ff-shadow-dialog);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.it {
  height: 32px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  white-space: nowrap;
  cursor: pointer;
}
.it > svg {
  color: var(--ff-primary-text);
  visibility: hidden;
}
.it.on > svg {
  visibility: visible;
}
.it.on {
  font-weight: 500;
}
.it.hv {
  background: var(--ff-bg-hover);
}
.nm {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.btn[aria-disabled='true'] {
  opacity: 0.45;
  cursor: not-allowed;
}
.srow.dvn {
  gap: 8px;
  align-items: flex-start;
  padding-top: 12px;
  padding-bottom: 12px;
}
.dvn > svg {
  margin-top: 2px;
  color: var(--ff-text-2);
}
.dvn .t {
  flex: 1;
  min-width: 0;
  font-size: var(--ff-fs-sm);
  line-height: 20px;
  color: var(--ff-text-2);
}
.dvn.info > svg {
  color: var(--ff-primary-text);
}
.dvn.ok > svg {
  color: var(--ff-success-text);
}
.dvn.warn > svg,
.dvn.warn .t {
  color: var(--ff-warning-text);
}
.dvn.err > svg,
.dvn.err .t {
  color: var(--ff-danger-text);
}
.acts {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: none;
  margin: -4px 0;
}
.btn.text {
  border-color: transparent;
  background: transparent;
  color: var(--ff-primary-text);
}
.spin {
  width: 12px;
  height: 12px;
  margin: 4px 2px 0;
  border-radius: 50%;
  border: 2px solid var(--ff-border);
  border-top-color: var(--ff-primary);
  animation: encspin 1s linear infinite;
  flex: none;
}
@keyframes encspin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .spin {
    animation: none;
  }
}
</style>
