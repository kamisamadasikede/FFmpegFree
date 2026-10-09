<script setup lang="ts">
// 转换页子记录（每次转换一条，设计 §3.2）：三行 + 右侧操作。状态来自 KidView（记录 + 任务 store 的实时状态）。
import MidEllipsis from '@/components/common/MidEllipsis.vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { prefersReducedMotion } from '@/utils/motion'
import ErrorLine from '@/components/common/ErrorLine.vue'
import ConvertThumb from './ConvertThumb.vue'
import type { ThumbState } from '@/api/convertRecords'
import { useConvertRecordsStore, type KidView } from '@/stores/convertRecords'
import { convertV2IsReal } from '@/api/convertRecords'
import { simParam } from '@/api/sim'
import { RECONVERT_CANCEL, RECONVERT_MENU, SHORT_TAG, SHORT_TIP, hasShortOutput } from '@/utils/convertV24Text'
import { showFallbackNotice, usedDeviceText, useEncoderDeviceList } from '@/api/encoderTask'
import { ENCODER_DEVICE_CPU_FALLBACK_NAME, ENCODER_DEVICE_CPU_FALLBACK_TITLE, ENCODER_FALLBACK_CONVERT, ENCODER_FALLBACK_CONVERT_DONE } from '@/errors/encoderMessages'
import { coverKindOf, formatRecordTime, isAudioContainer, recordLine, shortEta } from '@/utils/convertText'
import { fileBaseName, formatBytes, formatShortClock } from '@/utils/format'

const props = defineProps<{
  kid: KidView
  thumb?: ThumbState
  /** 排队序号（从 1 开始；0 = 不知道） */
  queuePos: number
  busy: boolean
  /** 搜索命中（高亮） */
  hit?: boolean
  /** 任务中心“在转换页查看”定位到这条（闪一下） */
  focused?: boolean
  /** 预设名按预设卡的标题显示（重名时带编码，如“MP4 · H.264”）；预设已不在时用快照里的名字 */
}>()
const emit = defineEmits<{ preview: []; cancel: []; retry: []; reveal: []; remove: []; log: []; changeOutput: []; reconvert: [] }>()
const cv = useConvertRecordsStore()
const v24 = cv.v24
/** 模拟截图：?cv_hover=short 显示“时长偏短”的悬停提示（截图 28） */
const forceShort = !convertV2IsReal() && simParam('cv_hover') === 'short'

const devices = useEncoderDeviceList()
// 窄列表的“更多”菜单（打开所在文件夹、删除记录）
const menuOpen = ref(false)
const moreBtn = ref<HTMLElement | null>(null)
function closeMenu(e?: Event) {
  if (e && moreBtn.value?.parentElement?.contains(e.target as Node)) return
  menuOpen.value = false
}
watch(menuOpen, (o) => {
  if (o) {
    document.addEventListener('pointerdown', closeMenu, true)
    if (v24) void cv.refreshPathCheck(k.value.id) // v0.24：打开时再查一次“能否重转”（文件可能刚被移走或换掉）
  } else document.removeEventListener('pointerdown', closeMenu, true)
})
onBeforeUnmount(() => document.removeEventListener('pointerdown', closeMenu, true))
function menu(fn: () => void) {
  menuOpen.value = false
  fn()
}
const k = computed(() => props.kid)
const name = computed(() => fileBaseName(k.value.outputPath) || k.value.title)
const container = computed(() => (k.value.options.container || name.value.split('.').pop() || '').toLowerCase())
const fmt = computed(() => container.value.toUpperCase())
const audio = computed(() => isAudioContainer(container.value))
/** 封面类型：按输出格式（图片格式 = 图片封面，音频容器 = 音符封面，其余含 GIF = 胶片） */
const cover = computed(() => coverKindOf(container.value, null, cv.catalogCategoryOf(container.value)))
const pct = computed(() => Math.round(Math.min(1, Math.max(0, k.value.progress)) * 100))
const active = computed(() => k.value.status === 'running' || k.value.status === 'queued')
/** v0.24 原地重转中（status 是 queued / running）：旧文件照常预览、打开（§八 第 59 条） */
const rc = computed(() => v24 && active.value && !!k.value.reconverting)
const failed = computed(() => k.value.status === 'failed' || k.value.status === 'interrupted')
const done = computed(() => k.value.status === 'succeeded')
const gone = computed(() => done.value && k.value.outputGone)
const canPreview = computed(() => (done.value || rc.value) && !gone.value)
/** 时长偏短（§八 第 55 条）：只看 result.warnings 有没有 short_output */
const short = computed(() => v24 && done.value && hasShortOutput(k.value.result))
/** 成功记录“更多”里的“重转…”（§八 第 59、64 条；v0.24.1 reconvertMode / reconvertBlock） */
const rcState = computed(() => (v24 ? cv.reconvertStateOf(k.value) : { mode: '' as const, tip: '' }))
function onReconvert() {
  if (rcState.value.mode) menu(() => emit('reconvert'))
}
const device = computed(() => usedDeviceText(k.value, devices.value))
const deviceFb = computed(() => device.value === ENCODER_DEVICE_CPU_FALLBACK_NAME)
const fallback = computed(() => showFallbackNotice(k.value) && (done.value || k.value.status === 'running'))
/** 第 2 行：时间 · 预设名（悬停 = 参数摘要）/ 时间 · 自定义 · 摘要（§3.2）。只用快照 presetName，不读当前预设卡片 */
// v0.24（§八 第 67 条）：成功的记录用 finishedAt（重转后就是重转完成的时间），其他状态仍用 createdAt
const line2 = computed(() => recordLine(k.value, [formatRecordTime(v24 && (done.value || rc.value) && k.value.finishedAt ? k.value.finishedAt : k.value.createdAt)]))
/** 完成：大小 · 时长 · 分辨率（音频是码率；1024 不显示）· 设备 */
const result = computed(() => {
  const r = k.value.result
  if (!r) return { size: '', dur: '', res: '' }
  return {
    size: r.sizeBytes ? formatBytes(r.sizeBytes) : '',
    dur: r.durationSec ? formatShortClock(r.durationSec) : '',
    res: audio.value ? (r.audioBitrateKbps ? `${r.audioBitrateKbps} kbps` : '') : r.width && r.height ? `${r.width}×${r.height}` : '',
  }
})
const previewTip = computed(() => {
  if (rc.value) return ''
  if (active.value) return '转换完成后才能预览'
  if (failed.value) return '转换失败，没有可预览的文件'
  if (gone.value) return '文件已被移动或删除，无法预览'
  return ''
})
/** 排队 / 转换中 → 完成 的那一刻，完成标签弹一下（打开页面时已完成的、滚回来重新挂载的不播） */
const justDone = ref(false)
let doneTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => k.value.status,
  (s, old) => {
    if (s !== 'succeeded' || !old || old === 'succeeded' || prefersReducedMotion()) return
    justDone.value = true
    clearTimeout(doneTimer)
    doneTimer = setTimeout(() => (justDone.value = false), 1200)
  },
)
onBeforeUnmount(() => clearTimeout(doneTimer))
const tag = computed(() => {
  if (rc.value) return { cls: 't-run t-rc', text: '重转中', icon: '' }
  switch (k.value.status) {
    case 'queued': return { cls: 't-q', text: '排队中', icon: '' }
    case 'running': return { cls: 't-run', text: '转换中', icon: '' }
    case 'succeeded': return { cls: 't-ok', text: '完成', icon: 'check' }
    case 'failed': return { cls: 't-fail', text: '失败', icon: 'warn' }
    case 'interrupted': return { cls: 't-warn', text: '已中断', icon: 'warn' }
    default: return { cls: 't-cx', text: '已取消', icon: '' }
  }
})
</script>
<template>
  <div class="cv-kid" :class="{ q: k.status === 'queued', hit, focus: focused }" :data-kid="k.id">
    <ConvertThumb
      sm
      :state="canPreview ? thumb : null"
      :gone="gone"
      :pend="!done"
      :cover="cover"
      :fmt="fmt"
      :clickable="canPreview"
      :label="`预览 ${name}`"
      @click="emit('preview')"
    />
    <div class="cv-km">
      <div class="l1">
        <MidEllipsis tag="b" :class="{ gone, lnk: canPreview }" :text="name" @click="canPreview && emit('preview')" />
        <span class="cv-tag" :class="[tag.cls, { 'ff-done-pop': justDone && done }]" :title="tag.text" :aria-label="tag.text"><FIcon v-if="tag.icon" :name="tag.icon === 'check' ? 'check' : 'warn'" /><i class="tx">{{ tag.text }}</i></span>
        <span v-if="short" class="cv-stag" :class="{ hv: forceShort }" tabindex="0" :title="SHORT_TIP" :aria-label="`${SHORT_TAG}：${SHORT_TIP}`"><FIcon name="warn" :size="12" />{{ SHORT_TAG }}<span class="cv-tip" role="tooltip">{{ SHORT_TIP }}</span></span>
      </div>
      <div class="l2" :title="line2.title">{{ line2.text }}</div>
      <div class="l3">
        <template v-if="k.status === 'running'">
          <div class="bar" role="progressbar" aria-label="转换进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="pct"><i class="run" :style="{ width: pct + '%' }" /></div>
          <span class="num">{{ pct }}%</span>
          <template v-if="k.speed"><span class="cv-d n9">·</span><span class="n9">{{ k.speed }}</span></template>
          <template v-if="shortEta(k.etaSec)"><span class="cv-d">·</span><span>剩余 {{ shortEta(k.etaSec) }}</span></template>
          <template v-if="device"><span class="cv-d n9">·</span><span v-if="deviceFb" class="cv-fb n9" :title="ENCODER_DEVICE_CPU_FALLBACK_TITLE">{{ device }}</span><span v-else class="dev n9" :title="device">{{ device }}</span></template>
        </template>
        <template v-else-if="rc && k.status === 'queued'">
          <div class="bar q"><i style="width: 0" /></div>
          <span>{{ queuePos > 1 ? `前面还有 ${queuePos - 1} 项` : '排队中' }}</span>
        </template>
        <template v-else-if="k.status === 'queued'">
          <div class="bar q"><i style="width: 0" /></div>
          <span>{{ queuePos > 1 ? `前面还有 ${queuePos - 1} 项` : '排队中' }}</span>
        </template>
        <template v-else-if="failed">
          <div class="bar fail"><i :style="{ width: pct + '%' }" /></div>
          <span>停在 {{ pct }}%</span>
        </template>
        <template v-else-if="k.status === 'canceled'">
          <div class="bar cx"><i :style="{ width: pct + '%' }" /></div>
          <span>{{ pct > 0 ? `停在 ${pct}%，没有生成文件` : '排队时已取消，没有生成文件' }}</span>
        </template>
        <template v-else-if="gone">
          <span class="gone"><FIcon name="warn" />文件已被移动或删除</span>
        </template>
        <template v-else>
          <span v-if="result.size" class="cv-sz">{{ result.size }}</span>
          <template v-if="result.dur"><span v-if="result.size" class="cv-d">·</span><span>{{ result.dur }}</span></template>
          <template v-if="result.res"><span v-if="result.size || result.dur" class="cv-d only1280">·</span><span class="only1280">{{ result.res }}</span></template>
          <template v-if="device">
            <span v-if="result.size || result.dur || result.res" class="cv-d">·</span>
            <span v-if="deviceFb" class="cv-fb" :title="ENCODER_DEVICE_CPU_FALLBACK_TITLE">{{ device }}</span>
            <span v-else class="dev" :title="device">{{ device }}</span>
          </template>
        </template>
      </div>
      <div v-if="fallback" class="cv-note"><FIcon name="warn" /><span>{{ done ? ENCODER_FALLBACK_CONVERT_DONE : ENCODER_FALLBACK_CONVERT }}</span></div>
      <div v-if="failed" class="cv-errwrap">
        <ErrorLine
          v-if="k.error"
          :tone="k.status === 'interrupted' ? 'interrupted' : 'danger'"
          actions-row
          :code="k.error.code"
          :message="k.error.message"
          :detail="k.error.detail"
          show-retry
          :busy="busy"
          @retry="emit('retry')"
          @view-log="emit('log')"
          @change-output="emit('changeOutput')"
        />
        <ErrorLine
          v-else
          tone="interrupted"
          actions-row
          code="INTERRUPTED"
          title=""
          description="应用退出时这个转换被中断，可以重试。"
          hide-code
          show-retry
          :busy="busy"
          @retry="emit('retry')"
          @view-log="emit('log')"
        />
      </div>
    </div>
    <div class="cv-ops">
      <template v-if="rc">
        <button type="button" class="cv-ib" :aria-label="`预览 ${name}`" title="预览" @click="emit('preview')"><FIcon name="eye" /></button>
        <button type="button" class="cv-ib nfold" :aria-label="`打开所在文件夹 ${name}`" title="打开所在文件夹" @click="emit('reveal')"><FIcon name="folder" /></button>
        <button type="button" class="cv-ib" :aria-label="RECONVERT_CANCEL" :title="RECONVERT_CANCEL" @click="emit('cancel')"><FIcon name="x" /></button>
      </template>
      <template v-else-if="active">
        <button type="button" class="cv-ib" aria-disabled="true" :aria-label="`预览 ${name}：${previewTip}`" :data-tip="previewTip"><FIcon name="eye" /></button>
        <button type="button" class="cv-ib" :aria-label="`取消 ${name}`" title="取消" @click="emit('cancel')"><FIcon name="x" /></button>
      </template>
      <template v-else-if="failed">
        <button type="button" class="cv-ib" aria-disabled="true" :aria-label="`预览 ${name}：${previewTip}`" :data-tip="previewTip"><FIcon name="eye" /></button>
        <button type="button" class="cv-ib" :aria-label="`重试 ${name}`" title="重试" :aria-busy="busy" :disabled="busy" @click="emit('retry')"><FIcon name="retry" /></button>
        <button type="button" class="cv-ib del" :aria-label="`删除记录 ${name}`" title="删除记录" @click="emit('remove')"><FIcon name="trash" /></button>
      </template>
      <template v-else-if="k.status === 'canceled'">
        <button type="button" class="cv-tbtn" :aria-busy="busy" :disabled="busy" @click="emit('retry')"><FIcon name="retry" />重新转换</button>
        <button type="button" class="cv-ib del" :aria-label="`删除记录 ${name}`" title="删除记录" @click="emit('remove')"><FIcon name="trash" /></button>
      </template>
      <template v-else>
        <button v-if="gone" type="button" class="cv-ib" aria-disabled="true" :aria-label="`预览 ${name}：${previewTip}`" :data-tip="previewTip"><FIcon name="eye" /></button>
        <button v-else type="button" class="cv-ib" :aria-label="`预览 ${name}`" title="预览" @click="emit('preview')"><FIcon name="eye" /></button>
        <button v-if="gone" type="button" class="cv-ib nfold" aria-disabled="true" :aria-label="`打开所在文件夹 ${name}：文件已被移动或删除`" data-tip="文件已被移动或删除"><FIcon name="folder" /></button>
        <button v-else type="button" class="cv-ib nfold" :aria-label="`打开所在文件夹 ${name}`" title="打开所在文件夹" @click="emit('reveal')"><FIcon name="folder" /></button>
        <button type="button" class="cv-ib del nfold" :class="{ only1280: v24 }" :aria-label="`删除记录 ${name}`" title="删除记录" @click="emit('remove')"><FIcon name="trash" /></button>
        <!-- v0.24：成功记录（含文件被移走的）总有“更多”：重转… / 删除记录（1024 下删除记录收进来）；列表很窄时打开所在文件夹也收进来 -->
        <span v-if="v24" class="cv-morewrap">
          <button ref="moreBtn" type="button" class="cv-ib" :aria-label="`更多：${RECONVERT_MENU}、删除记录 ${name}`" title="更多" aria-haspopup="menu" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen"><FIcon name="more" /></button>
          <div v-if="menuOpen" class="cv-more-menu" role="menu">
            <button type="button" role="menuitem" class="cv-nmenu" :aria-disabled="gone || undefined" :title="gone ? '文件已被移动或删除' : undefined" @click="!gone && menu(() => emit('reveal'))"><FIcon name="folder" />打开所在文件夹</button>
            <button type="button" role="menuitem" :aria-disabled="!rcState.mode || undefined" :title="rcState.tip || undefined" :aria-label="rcState.tip ? `${RECONVERT_MENU}（${rcState.tip}）` : RECONVERT_MENU" @click="onReconvert"><FIcon name="retry" />{{ RECONVERT_MENU }}</button>
            <button type="button" role="menuitem" @click="menu(() => emit('remove'))"><FIcon name="trash" />删除记录</button>
          </div>
        </span>
        <!-- 列表宽度 < 480px（§14.5）：打开所在文件夹、删除记录收进“更多” -->
        <span v-else class="cv-kmore">
          <button ref="moreBtn" type="button" class="cv-ib nmore" :aria-label="`更多：打开所在文件夹、删除记录 ${name}`" title="更多" aria-haspopup="menu" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen"><FIcon name="more" /></button>
          <div v-if="menuOpen" class="cv-more-menu" role="menu">
            <button type="button" role="menuitem" :aria-disabled="gone || undefined" :title="gone ? '文件已被移动或删除' : undefined" @click="!gone && menu(() => emit('reveal'))"><FIcon name="folder" />打开所在文件夹</button>
            <button type="button" role="menuitem" @click="menu(() => emit('remove'))"><FIcon name="trash" />删除记录</button>
          </div>
        </span>
      </template>
    </div>
  </div>
</template>
