<template>
  <div class="cv">
    <!-- 左：待转换文件 -->
    <section class="col">
      <!-- 硬件编码失败已自动改用 CPU（契约 9.7）：整批只提示一条（批量时每个文件都回退也不刷屏） -->
      <EncoderFallbackNotice v-if="fallbackShown" variant="convert" class="fbnote" @settings="router.push(encoderSettingsLocation())" />
      <!-- 转换中：整体进度（设计稿 ?state=running 的 rprog） -->
      <div v-if="cv.mode === 'running'" class="panel rprog">
        <div class="top">
          <b>正在转换</b>
          <span class="sub">已完成 {{ cv.overall.done }} / {{ cv.overall.total }} 个文件</span>
          <span class="pct">{{ cv.overall.pct }}%</span>
        </div>
        <div class="bar" role="progressbar" aria-label="整体进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="cv.overall.pct"><i class="run" :style="{ width: cv.overall.pct + '%' }" /></div>
        <div class="meta">
          <span>速度<b>{{ cv.overall.speed || '—' }}</b></span>
          <span>当前文件剩余<b>{{ formatEta(cv.overall.etaSec) || '—' }}</b></span>
          <span v-if="deviceText" class="dev" :title="`${ENCODER_DEVICE_LABEL} ${deviceText}`">{{ ENCODER_DEVICE_LABEL }}<b>{{ deviceText }}</b></span>
        </div>
      </div>
      <div v-else-if="cv.mode === 'done'" class="okline" role="status">
        <FIcon name="check" :size="16" />
        <div class="okbody"><b>转换完成</b>{{ cv.succeededRows.length }} 个文件已保存<template v-if="elapsedText"> · 用时 {{ elapsedText }}</template><template v-if="deviceText"> · {{ ENCODER_DEVICE_LABEL }} {{ deviceText }}</template><template v-if="cv.outputFolder"> · <span class="okdir" :title="cv.outputFolder">{{ cv.outputFolder }}</span></template><template v-else-if="cv.outputFolders.length > 1"> · <span :title="cv.outputFolders.join('\n')">{{ cv.outputFolders.length }} 个文件夹</span></template></div>
      </div>

      <div class="panel list-panel" :style="dropStyle">
        <div class="phead">
          <h2>待转换文件</h2>
          <span class="sub">{{ subText }}</span>
          <span class="sp" />
          <template v-if="cv.rows.length && cv.mode !== 'running'">
            <button type="button" class="btn" @click="cv.chooseFiles()"><FIcon name="folder" :size="15" />添加文件</button>
            <button type="button" class="btn" @click="cv.clear()"><FIcon name="trash" :size="15" />清空</button>
          </template>
        </div>

        <!-- 空状态：大拖入区 -->
        <div v-if="!cv.rows.length" class="dz big" @click="cv.chooseFiles()">
          <div class="ic"><FIcon name="upload" :size="32" /></div>
          <b>拖入视频或音频文件，或点击选择</b>
          <small>支持常见视频、音频格式，一次最多 {{ MAX_SUBMIT }} 个</small>
          <button type="button" class="btn pri" @click.stop="cv.chooseFiles()">选择文件</button>
          <small v-if="cv.pickSoon" class="soon" role="status">{{ cv.pickSoon }}</small>
        </div>

        <template v-else>
          <div v-if="cv.mode !== 'running'" class="dz small" @click="cv.chooseFiles()">
            <div class="ic"><FIcon name="upload" :size="20" /></div>
            <div class="dzt"><b>拖入更多文件，或点击选择</b><small v-if="!cv.pickSoon">支持常见视频、音频格式</small><small v-else class="soon" role="status">{{ cv.pickSoon }}</small></div>
          </div>
          <div v-if="cv.notice" class="notice" role="status"><FIcon name="warn" :size="14" />{{ cv.notice }}</div>
          <div class="list" role="list" aria-label="待转换文件">
            <ConvertFileRow
              v-for="r in cv.rows"
              :key="r.key"
              role="listitem"
              :row="r"
              :state="cv.stateOf(r)"
              :task="cv.rowTask(r)"
              :conflict="cv.conflictOf(r)"
              :preset-short="cv.presetShort"
              :ffmpeg-ready="ffmpeg.ready"
              :single="cv.rows.length === 1"
              :selectable="cv.rows.length > 1 && r.probe === 'ok'"
              :selected="cv.focusRow === r"
              :log-text="logKey === r.key ? logText : null"
              :busy="tasks.isBusy(r.taskId)"
              @visible="cv.ensureThumb(r)"
              @select="cv.focusOn(r)"
              @remove="onRemove(r)"
              @cancel="cv.cancelRow(r)"
              @readd="cv.unbind(r)"
              @retry="cv.retryRow(r)"
              @change-output="onChangeOutput(r)"
              @view-log="toggleLog(r)"
              @close-log="closeLog"
              @reveal="onReveal(r)"
            />
          </div>

          <!-- 文件信息卡：只有 1 个文件时显示它；多个文件时点击某行显示该行 -->
          <section v-if="card" class="infocard" :class="{ multi: cv.rows.length > 1 }" aria-label="文件信息">
            <div v-if="card.audio" class="cover audio" aria-hidden="true">
              <FIcon name="music" :size="32" />
              <span v-if="card.duration" class="dur">{{ card.duration }}</span>
            </div>
            <div v-else class="cover" aria-hidden="true">
              <img v-if="card.cover" :src="card.cover" alt="" />
              <FIcon v-else name="play" :size="32" />
              <span v-if="card.duration" class="dur">{{ card.duration }}</span>
            </div>
            <div class="fn" :title="card.path">{{ card.name }}</div>
            <dl class="kv" :style="{ '--cols': card.items.length }">
              <div v-for="k in card.items" :key="k.label"><dt>{{ k.label }}</dt><dd>{{ k.value }}</dd></div>
            </dl>
          </section>
        </template>
      </div>
    </section>

    <!-- 右：输出设置 -->
    <aside class="rcol">
      <div class="panel settings" :class="{ off: cv.mode === 'running' }">
        <div class="phead"><h2>输出设置</h2><span class="sp" /><span class="sub">应用到全部</span></div>
        <div class="pbody">
          <!-- 页签 + 预设可以滚动；“保存到”固定在下面，窗口矮（1024×680）时也一直看得到 -->
          <div ref="pscrollEl" class="pscroll" :class="{ more: moreBelow }" @scroll.passive="updateMore">
          <div class="seg" role="tablist" aria-label="预设类别">
            <button v-for="g in GROUPS" :id="`pg-${g.key}`" :key="g.key" type="button" role="tab" :aria-selected="group === g.key" :class="{ on: group === g.key }" @click="pickGroup(g.key)">{{ g.label }}</button>
          </div>

          <div v-if="cv.presetsError" class="perr"><ErrorLine :code="cv.presetsError.code" :message="cv.presetsError.message" :detail="cv.presetsError.detail" :show-log="false" fallback-title="加载预设失败" show-retry compact @retry="cv.loadPresets()" /></div>
          <div v-else-if="!cv.presetsLoaded" class="hint">正在加载预设…</div>
          <div v-else class="presets" role="radiogroup" aria-label="输出预设" :aria-labelledby="`pg-${group}`">
            <button
              v-for="p in shownPresets"
              :key="p.id"
              type="button"
              role="radio"
              class="preset"
              :class="{ on: p.id === cv.selectedPresetId }"
              :aria-checked="p.id === cv.selectedPresetId"
              :disabled="cv.mode === 'running'"
              :title="p.name"
              @click="cv.selectedPresetId = p.id"
            >
              <b><span class="pt">{{ cv.presetTitle(p) }}</span></b>
              <small>{{ presetSub(p) }}</small>
            </button>
          </div>
          </div>

          <div class="field">
            <label id="out-label">保存到</label>
            <div class="out">
              <div class="chip" role="group" aria-labelledby="out-label" :title="cv.effectiveOutputDir || undefined">
                <span v-if="!cv.effectiveOutputDir" class="ph">与源文件相同的文件夹</span>
                <template v-else><span class="h">{{ pathParts.head }}</span><span class="t">{{ pathParts.tail }}</span></template>
              </div>
              <button type="button" class="btn icon" title="更换输出位置" aria-label="更换输出位置" :disabled="cv.mode === 'running'" @click="onPickDir"><FIcon name="folder" :size="15" /></button>
            </div>
            <div class="outnote">
              <template v-if="cv.outputOverride">仅本次转换使用 · <button type="button" class="ff-link" @click="cv.outputOverride = ''">恢复默认</button></template>
              <template v-else-if="cv.defaultOutputDir">来自设置里的默认输出位置</template>
              <template v-else>未设置默认输出位置，保存到各自源文件所在的文件夹</template>
            </div>
          </div>
        </div>

        <div v-if="gateText" id="cv-gate" class="gate" :class="{ info: gateInfo }" role="status">
          <FIcon :name="gateInfo ? 'refresh' : 'warn'" :size="14" />
          <span>{{ gateText }}<button v-if="cv.startBlockReason === 'ffmpeg'" type="button" class="ff-link" @click="ffmpeg.dialogOpen = true">安装 ffmpeg</button></span>
        </div>
        <div v-if="cv.submitError" class="suberr"><ErrorLine :code="cv.submitError.code" :message="cv.submitError.message" :detail="cv.submitError.detail" :show-log="false" fallback-title="无法开始转换" compact /></div>

        <div class="foot">
          <template v-if="cv.mode === 'running'">
            <router-link class="lnk" to="/tasks"><FIcon name="task" :size="15" />已加入任务中心</router-link>
            <span class="sp" />
            <button type="button" class="btn lg" @click="cv.cancelAll()"><FIcon name="x" :size="15" />取消</button>
          </template>
          <template v-else-if="cv.mode === 'done'">
            <button type="button" class="btn lg" @click="cv.clear()">再转一个</button>
            <span class="sp" />
            <button type="button" class="btn pri lg" :title="cv.outputFolders.length > 1 ? `输出在 ${cv.outputFolders.length} 个文件夹里，打开第一个` : undefined" @click="onRevealOutput"><FIcon name="folder" :size="15" />打开输出位置</button>
          </template>
          <template v-else-if="cv.mode === 'failed'">
            <!-- 三个按钮合计 383px 放不进 254px 的页脚：第一行“再转一个”“打开输出位置”，主按钮“重试失败项”单独第二行靠右 -->
            <div class="frow">
              <button type="button" class="btn lg" @click="cv.clear()">再转一个</button>
              <button v-if="cv.succeededRows.length" type="button" class="btn lg" :title="cv.outputFolders.length > 1 ? `输出在 ${cv.outputFolders.length} 个文件夹里，打开第一个` : undefined" @click="onRevealOutput"><FIcon name="folder" :size="15" />打开输出位置</button>
            </div>
            <button type="button" class="btn pri lg retry" :disabled="cv.retryingAll" :aria-busy="cv.retryingAll" @click="cv.retryAllFailed()"><FIcon name="retry" :size="15" />重试失败项</button>
          </template>
          <template v-else>
            <div v-if="footHint || (cv.blockedCount && cv.mode === 'ready' && !noneConvertible)" id="cv-foot-hint" class="fhint">
              <!-- 一个都转不了时，把“移出”做进提示句里，不再另占一行（页脚高约 101px，而不是 123px） -->
              <small v-if="footHint && noneConvertible && cv.blockedCount">没有可以转换的文件，请<button type="button" class="ff-link" @click="cv.removeBlocked()">移出无法转换的文件</button>，或换一个预设。</small>
              <small v-else-if="footHint">{{ footHint }}</small>
              <small v-if="cv.blockedCount && cv.mode === 'ready' && !noneConvertible" class="skipped">已跳过 {{ cv.blockedCount }} 个无法转换的文件 · <button type="button" class="ff-link" @click="cv.removeBlocked()">移出</button></small>
            </div>
            <button
              type="button"
              class="btn pri lg start"
              :aria-disabled="startDisabled"
              :aria-describedby="startDisabled ? (gateText ? 'cv-gate' : 'cv-foot-hint') : undefined"
              :aria-busy="cv.submitting"
              :title="startDisabled ? startWhy : undefined"
              @click="cv.submit()"
            ><FIcon name="play" :size="15" />{{ startLabel }}</button>
          </template>
        </div>
      </div>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import ConvertFileRow from '@/components/convert/ConvertFileRow.vue'
import { MAX_SUBMIT, type PresetItem } from '@/api/convert'
import { onFilesDropped } from '@/api/fileDrop'
import { toAppError } from '@/api/call'
import { hasWailsBackend } from '@/services/wails'
import { useRouter } from 'vue-router'
import EncoderFallbackNotice from '@/components/encoder/EncoderFallbackNotice.vue'
import { encoderSettingsLocation, showFallbackNotice, usedDeviceText, useEncoderDeviceList } from '@/api/encoderTask'
import { ENCODER_DEVICE_LABEL } from '@/errors/encoderMessages'
import { actionErrorText } from '@/errors/errorMessages'
import { PREVIEW_CONVERT, splitPresetName, useConvertStore, type ConvertRow } from '@/stores/convert'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useTaskStore } from '@/stores/tasks'
import { formatBytes, formatDuration, formatEta, formatShortClock } from '@/utils/format'
import { channelText, codecName, isAudioInfo, sampleRateText } from '@/utils/mediaText'

const cv = useConvertStore()
const ffmpeg = useFFmpegStore()
const tasks = useTaskStore()

/** Wails 的拖放目标标记：只有落在带这个样式的区域才回调 OnFileDrop。加在整个列表面板上（空状态的大拖入区、
 * 小拖入区、文件行都在里面）；拖入时的高亮只用 Wails 加的 .wails-drop-target-active，不再自己监听 dragenter/dragleave（经过子元素会闪） */
const dropStyle = { '--wails-drop-target': 'drop' } as Record<string, string>

// ---- 预设：视频 / 音频两组 ----
const GROUPS = [
  { key: 'video', label: '视频' },
  { key: 'audio', label: '音频' },
] as const
const group = ref<'video' | 'audio'>('video')
const AUDIO = ['mp3', 'aac', 'm4a', 'wav', 'flac', 'ogg', 'opus']
const isAudioPreset = (p: PresetItem) => AUDIO.includes(p.options.container)
const shownPresets = computed(() => cv.presets.filter((p) => (group.value === 'audio') === isAudioPreset(p)))
function optionLine(p: PresetItem) {
  return p.options.container.toUpperCase()
}
/** 预设卡的说明：括号里的文字；没有就用格式名 */
const presetSub = (p: PresetItem) => splitPresetName(p.name).sub || optionLine(p)
/** 切页签后当前选中项不在这一页时，自动选中该页第一个预设（否则切过去看不到选中项） */
function pickGroup(key: 'video' | 'audio') {
  group.value = key
  if (cv.mode === 'running') return
  const list = cv.presets.filter((p) => (key === 'audio') === isAudioPreset(p))
  if (list.length && !list.some((p) => p.id === cv.selectedPresetId)) cv.selectedPresetId = list[0].id
}
// 选中的预设换类别时，分组页签跟着走（如从预览参数进入）
watch(() => cv.selectedPreset, (p) => {
  if (p) group.value = isAudioPreset(p) ? 'audio' : 'video'
}, { immediate: true })

// ---- 头部文字 ----
const subText = computed(() => {
  const n = cv.rows.length
  if (!n) return '未选择文件'
  const bytes = cv.totalBytes
  return `${n} 个文件${bytes ? ` · 共 ${formatBytes(bytes)}` : ''}`
})

// ---- 输出位置 ----
const pathParts = computed(() => {
  const p = cv.effectiveOutputDir.replace(/[\\/]+$/, '')
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i < 0 ? { head: '', tail: p } : { head: p.slice(0, i + 1), tail: p.slice(i + 1) }
})
async function guard(fn: () => Promise<unknown>) {
  try {
    await fn()
  } catch (e) {
    const err = toAppError(e)
    ElMessage.error(actionErrorText(err.code, err.message))
  }
}
const onPickDir = () => guard(() => cv.chooseOutputDir())

// ---- 开始转换不可用的提示 ----
// 灰着的“开始转换”页面上必须有原因：ffmpeg / 读取中 / 预设 → 输出设置里的提示条（#cv-gate）；
// 没有可提交的行 → 页脚提示（#cv-foot-hint）。按钮用 aria-disabled，aria-describedby 指向对应提示。
const gateText = computed(() => {
  if (cv.mode === 'running' || cv.mode === 'done' || cv.mode === 'failed') return ''
  switch (cv.startBlockReason) {
    case 'ffmpeg': return ffmpeg.status.state === 'checking' ? '正在检测 ffmpeg…' : ffmpeg.status.state === 'installing' ? 'ffmpeg 正在安装，装好后就可以转换。' : '需要先安装 ffmpeg 才能转换。'
    case 'probing': return probeText.value
    case 'preset': return cv.presetsError ? '没有加载到输出预设，请先重试加载。' : '正在加载预设…'
    default: return ''
  }
})
// 读取文件信息 / 检测 / 安装 ffmpeg 都是正常过程：用主色软底；只有 ffmpeg 缺失、预设没加载才用警告色
const gateInfo = computed(() => {
  const r = cv.startBlockReason
  return r === 'probing' || (r === 'ffmpeg' && !ffmpegMissing.value)
})
const probeText = computed(() => {
  const { done, total } = cv.probeProgress
  return total > 1 ? `正在读取文件信息（${done}/${total}）…` : '正在读取文件信息…'
})
const footHint = computed(() => {
  if (gateText.value) return ''
  const n = cv.submittableRows.length
  if (!cv.rows.length) return '先添加文件'
  if (n) return `${n} 个文件 · 转为 ${cv.presetShort || '…'}`
  if (cv.blockedCount) return '没有可以转换的文件，请移出无法转换的文件，或换一个预设。'
  if (cv.canceledCount) return '列表里只剩已取消的文件，点“重新加入”后再开始。'
  return '没有可以转换的文件。'
})
const noneConvertible = computed(() => cv.rows.length > 0 && !cv.submittableRows.length)
const startDisabled = computed(() => !!cv.startBlockReason || cv.submitting)
// 规范 7.1：ffmpeg 缺失时被门控按钮的 tooltip 逐字为“需要先安装 ffmpeg”（右栏提示条文案另算）
const ffmpegMissing = computed(() => cv.startBlockReason === 'ffmpeg' && ffmpeg.status.state !== 'checking' && ffmpeg.status.state !== 'installing')
const startWhy = computed(() => (ffmpegMissing.value ? '需要先安装 ffmpeg' : gateText.value || footHint.value))
const startLabel = computed(() => {
  const n = cv.submittableRows.length
  return n > 0 && !cv.startBlockReason ? `开始转换 ${n} 个` : '开始转换'
})

// ---- 单文件完成态“用时” ----
// 用时由前端自己算：任务开始到结束时间（startedAt → finishedAt）。只在整个列表就 1 个文件且已成功时显示；
// 体积变化（源大小 → 输出大小）需要任务对象里有输出文件大小，目前没有，所以不做（也不向后端要字段）。
const router = useRouter()
const encDevices = useEncoderDeviceList()
/** 有任务回退了 CPU（且真的运行过）就显示一条提示 */
const fallbackShown = computed(() => cv.rows.some((r) => showFallbackNotice(cv.rowTask(r))))
/** 转换中 / 完成后显示“设备”：正在运行（完成后：已成功）的任务用的设备名（多个设备时按出现顺序用“、”连接；没有就不显示） */
const deviceText = computed(() => {
  const names: string[] = []
  for (const r of cv.rows) {
    const t = cv.rowTask(r)
    if (!t || t.status !== (cv.mode === 'done' ? 'succeeded' : 'running')) continue
    const n = usedDeviceText(t, encDevices.value)
    if (n && !names.includes(n)) names.push(n)
  }
  return names.join('、')
})
const elapsedText = computed(() => {
  if (cv.rows.length !== 1 || cv.succeededRows.length !== 1) return ''
  const t = cv.rowTask(cv.succeededRows[0])
  if (!t?.startedAt || !t.finishedAt || t.finishedAt < t.startedAt) return ''
  return formatDuration(t.finishedAt - t.startedAt)
})

// ---- 文件信息卡 ----
const card = computed(() => {
  const r = cv.focusRow
  const i = r?.info
  if (!r || r.probe !== 'ok' || !i) return null
  const audio = isAudioInfo(i)
  const dash = (v: string) => v || '—'
  const items = audio
    ? // 音频：时长 / 编码 / 采样率 · 声道 / 大小；缺失的项整项省略（不出现单独的“—”），其余列均分
      [
        { label: '时长', value: formatShortClock(i.duration) },
        { label: '编码', value: codecName(i.audioCodec) },
        { label: '采样率 · 声道', value: [sampleRateText(i.sampleRate), channelText(i.channels)].filter(Boolean).join(' · ') },
        { label: '大小', value: formatBytes(i.size) },
      ].filter((k) => k.value)
    : [
        { label: '时长', value: dash(formatShortClock(i.duration)) },
        { label: '分辨率', value: i.width ? `${i.width}×${i.height}` : '—' },
        { label: '编码', value: dash([codecName(i.videoCodec), codecName(i.audioCodec)].filter(Boolean).join(' / ')) },
        { label: '大小', value: formatBytes(i.size) },
      ]
  return { audio, name: r.name, path: r.path, cover: r.cover, duration: formatShortClock(i.duration), items }
})
watch(() => [cv.focusRow, cv.focusRow?.probe] as const, ([r]) => cv.ensureCover(r), { immediate: true })

// ---- 预设区“下面还有”提示：内容溢出且没滚到底时，底部 16px 渐隐 ----
const pscrollEl = ref<HTMLElement | null>(null)
const moreBelow = ref(false)
function updateMore() {
  const el = pscrollEl.value
  moreBelow.value = !!el && el.scrollHeight - el.scrollTop - el.clientHeight > 2
}
let roPscroll: ResizeObserver | null = null
watch(() => [shownPresets.value.length, group.value, cv.presetsLoaded, cv.presetsError] as const, () => void nextTick(updateMore), { flush: 'post' })

// ---- 行操作 ----
function onRemove(r: ConvertRow) {
  if (logKey.value === r.key) closeLog()
  cv.removeRow(r.key)
}
async function onReveal(r: ConvertRow) {
  await guard(() => cv.reveal(r))
}
async function onRevealOutput() {
  await guard(() => cv.revealOutput())
}
async function onChangeOutput(r: ConvertRow) {
  await guard(async () => {
    const res = await cv.changeOutputAndResubmit(r)
    if (res === 'ok') ElMessage.success('已用新的输出位置重新提交')
    else if (res === 'busy') return
    else if (res === 'no-params') ElMessage.info('没能读到这个任务的原始参数，请先清理磁盘空间后点“重试”。')
  })
}

// ---- 日志 ----
const logKey = ref('')
const logText = ref('')
async function toggleLog(r: ConvertRow) {
  if (logKey.value === r.key) return closeLog()
  logKey.value = r.key
  logText.value = ''
  if (!r.taskId) return
  try {
    logText.value = await tasks.getLog(r.taskId, 200)
  } catch (e) {
    logText.value = toAppError(e).message
  }
}
function closeLog() {
  logKey.value = ''
  logText.value = ''
}

// ---- 生命周期 ----
let offDrop: () => void = () => {}
onMounted(() => {
  updateMore()
  if (pscrollEl.value && typeof ResizeObserver !== 'undefined') {
    roPscroll = new ResizeObserver(updateMore)
    roPscroll.observe(pscrollEl.value)
  }
  void cv.init()
  cv.refreshDefaultDir()
  offDrop = onFilesDropped((paths) => cv.addPaths(paths))
  if (PREVIEW_CONVERT && !cv.rows.length && PREVIEW_CONVERT !== 'idle') cv.seedPreview(PREVIEW_CONVERT)
})
onUnmounted(() => {
  offDrop()
  roPscroll?.disconnect()
})
// ffmpeg 从未就绪变为就绪：补探测之前加进来的文件
watch(() => ffmpeg.ready, (ok) => {
  if (ok) void cv.probePending()
})
void hasWailsBackend
</script>

<style scoped>
.cv {
  display: flex;
  gap: var(--ff-space-4);
  height: 100%;
  min-height: 0;
}
.col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
  min-height: 0;
}
.rcol {
  width: 320px;
  flex: none;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.panel {
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-lg);
}
.list-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.phead {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-3) var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
  flex: none;
}
.phead h2 {
  margin: 0;
  font-size: var(--ff-fs-md);
  font-weight: 600;
}
.sub {
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
}
.sp {
  flex: 1;
}
.btn {
  height: 28px;
  padding: 0 12px;
  border-radius: var(--ff-radius-md);
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  display: inline-flex;
  align-items: center;
  gap: var(--ff-space-2);
  font: inherit;
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  cursor: pointer;
}
.btn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.btn:disabled,
.btn[aria-disabled='true'] {
  opacity: 0.45;
  cursor: default;
}
.btn[aria-disabled='true']:hover {
  background: var(--ff-primary);
}
.btn.lg {
  height: 32px;
  padding: 0 16px;
}
.btn.icon {
  padding: 0;
  width: 28px;
  justify-content: center;
  flex: none;
}
.btn.pri {
  background: var(--ff-primary);
  border-color: var(--ff-primary);
  color: var(--ff-on-primary);
}
.btn.pri:hover:not(:disabled) {
  background: var(--ff-primary-hover);
}
.btn.pri[aria-disabled='true']:hover {
  background: var(--ff-primary);
}
.ff-link {
  margin-left: 8px;
  padding: 0;
  border: none;
  background: transparent;
  font: inherit;
  color: var(--ff-primary-text);
  cursor: pointer;
  border-radius: 2px;
}
.ff-link:hover {
  text-decoration: underline;
}

/* 拖入区 */
.dz {
  border: 1.5px dashed var(--ff-border);
  border-radius: var(--ff-radius-lg);
  color: var(--ff-text-2);
  transition: border-color var(--ff-dur-fast) var(--ff-ease), background var(--ff-dur-fast) var(--ff-ease);
}
/* 拖入高亮：Wails 会给命中的 --wails-drop-target 元素及其祖先加 .wails-drop-target-active，
   列表面板整块都是放置区，所以以面板为准，不自己监听 dragenter / dragleave */
.list-panel.wails-drop-target-active {
  border-color: var(--ff-primary);
}
.list-panel.wails-drop-target-active .dz {
  border-color: var(--ff-primary);
  background: var(--ff-primary-soft);
}
.dz .ic {
  border-radius: var(--ff-radius-lg);
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
  display: grid;
  place-items: center;
  flex: none;
}
.dz b {
  color: var(--ff-text-1);
  font-weight: 500;
}
.dz small {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  margin-top: var(--ff-space-1);
}
.dz .soon {
  color: var(--ff-warning-text);
}
.dz.big {
  cursor: pointer;
  flex: 1;
  margin: var(--ff-space-4);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--ff-space-3);
}
.dz.big .ic {
  width: 64px;
  height: 64px;
  border-radius: 16px;
}
.dz.big b {
  font-size: var(--ff-fs-lg);
}
.dz.small {
  margin: var(--ff-space-4) var(--ff-space-4) var(--ff-space-2);
  height: 64px;
  display: flex;
  align-items: center;
  gap: var(--ff-space-3);
  padding: 0 var(--ff-space-4);
  cursor: pointer;
  flex: none;
}
.dz.small .ic {
  width: 40px;
  height: 40px;
  border-radius: var(--ff-radius-lg);
}
.dzt {
  display: flex;
  flex-direction: column;
  line-height: 1.4;
}
.notice {
  display: flex;
  gap: var(--ff-space-2);
  align-items: flex-start;
  margin: 0 var(--ff-space-4) var(--ff-space-2);
  font-size: var(--ff-fs-xs);
  color: var(--ff-warning-text);
}
.notice svg {
  margin-top: var(--ff-space-1);
}
.list {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0 var(--ff-space-2) var(--ff-space-2);
}

/* 文件信息卡（原型 .finfobar：封面 16:9 + 文件名 14/500 + 四列信息；不做播放） */
.infocard {
  flex: none;
  padding: var(--ff-space-4);
  border-top: 1px solid var(--ff-border);
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-3);
  /* 封面宽度随窗口高度收缩，保证 1024×680 下封面、信息和列表都放得下：
     宽 = 16/9 × (窗口高 − 其余内容占用的高度)，夹在 [最小宽, 100%] 之间。
     --cover-rest = 封面以外一屏里其余部分（顶栏、标题、拖入区、文件行、信息卡文字、页边距）大约占的高度；
     多文件时列表更高，rest 更大、最小宽更小 */
  --cover-min: 192px;
  --cover-rest: 540px;
  --cover-w: min(100%, max(var(--cover-min), calc((100vh - var(--cover-rest)) * 16 / 9)));
}
.infocard.multi {
  --cover-min: 160px;
  --cover-rest: 640px;
}
.cover {
  position: relative;
  width: var(--cover-w);
  aspect-ratio: 16 / 9;
  margin: 0; /* 左对齐：封面左缘与下面的文件名、四列信息对齐 */
  border-radius: var(--ff-radius-lg);
  overflow: hidden;
  background: var(--ff-bg-hover);
  color: var(--ff-text-2);
  display: grid;
  place-items: center;
}
.cover.audio {
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
}
.cover img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover .dur {
  position: absolute;
  right: var(--ff-space-2);
  bottom: var(--ff-space-2);
  font-size: var(--ff-fs-xs);
  line-height: 20px;
  padding: 0 var(--ff-space-2);
  border-radius: var(--ff-radius-sm);
  color: var(--ff-text-1);
  background: color-mix(in srgb, var(--ff-bg-surface) 88%, transparent);
}
.infocard .fn {
  font-size: var(--ff-fs-md);
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.kv {
  margin: 0;
  display: grid;
  grid-template-columns: repeat(var(--cols, 4), minmax(0, 1fr));
  gap: var(--ff-space-4);
}
.kv dt {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.kv dd {
  margin: 0;
  font-size: var(--ff-fs-sm);
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 整体进度 / 完成提示 */
.fbnote {
  flex: none;
}
.rprog {
  padding: var(--ff-space-4);
  flex: none;
}
.rprog .top {
  display: flex;
  align-items: baseline;
  gap: var(--ff-space-2);
  margin-bottom: var(--ff-space-2);
}
.rprog .top b {
  font-size: var(--ff-fs-md);
  font-weight: 600;
}
.rprog .pct {
  margin-left: auto;
  font-size: var(--ff-fs-xl);
  font-weight: 600;
  color: var(--ff-primary-text);
}
.bar {
  width: 100%;
  height: 4px;
  border-radius: 2px;
  background: var(--ff-border);
  overflow: hidden;
}
.bar i {
  display: block;
  height: 100%;
  background: var(--ff-primary);
  border-radius: 2px;
  transition: width var(--ff-dur-base) var(--ff-ease);
}
.rprog .meta {
  display: flex;
  gap: var(--ff-space-4);
  margin-top: var(--ff-space-2);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.rprog .meta span {
  white-space: nowrap;
}
/* 设备名太长（如“NVIDIA GeForce RTX 4060 Laptop GPU”）时只截自己，不把“当前文件剩余”挤成两行；全名在 title 里 */
.rprog .meta .dev {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rprog .meta b {
  font-weight: 500;
  color: var(--ff-text-1);
  margin-left: 4px;
}
.okline {
  flex: none;
  display: flex;
  gap: var(--ff-space-2);
  align-items: flex-start;
  padding: var(--ff-space-2) var(--ff-space-3);
  border-radius: var(--ff-radius-lg);
  background: color-mix(in srgb, var(--ff-success) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--ff-success) 28%, transparent);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.5;
}
.okline svg {
  color: var(--ff-success-text);
}
.okbody {
  flex: 1;
  min-width: 0;
}
.okline b {
  color: var(--ff-text-1);
  font-weight: 500;
  font-size: var(--ff-fs-sm);
  margin-right: var(--ff-space-2);
}
.okdir {
  font-family: var(--ff-font-mono);
  word-break: break-all;
}

/* 输出设置 */
.settings {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.pbody {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  padding: var(--ff-space-4);
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
}
.pscroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
  /* 给焦点描边留位置，避免被 overflow 裁掉 */
  margin: calc(-1 * var(--ff-space-1));
  /* 底部多补 4px：实测有约 7px 溢出残余，未滚动时卡片底缘比可视区低约 3px，底部描边被裁、最后一行小字落在渐隐区 */
  padding: var(--ff-space-1) var(--ff-space-1) calc(var(--ff-space-1) + 4px);
}
/* 有溢出时滚动条 6px 可见（覆盖全局“悬停才显示”的透明度），让人看出下面还有 */
.pscroll::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--ff-text-3) 60%, transparent);
}
/* 还没滚到底：底部 16px 渐隐，提示下面还有预设 */
.pscroll.more {
  -webkit-mask-image: linear-gradient(to bottom, #000 calc(100% - 16px), transparent);
  mask-image: linear-gradient(to bottom, #000 calc(100% - 16px), transparent);
}
.pscroll > * {
  flex: none;
}
.field {
  flex: none;
}
.settings.off .pbody {
  opacity: 0.45;
  pointer-events: none;
}
.seg {
  display: flex;
  background: var(--ff-bg-hover);
  border-radius: var(--ff-radius-md);
  padding: var(--ff-space-1);
  flex: none;
}
.seg button {
  flex: 1;
  height: 24px;
  border: none;
  border-radius: var(--ff-radius-sm);
  background: transparent;
  color: var(--ff-text-2);
  font: inherit;
  font-size: var(--ff-fs-xs);
  cursor: pointer;
}
.seg button.on {
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  font-weight: 500;
  box-shadow: var(--ff-shadow-1);
}
.presets {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--ff-space-2);
}
.preset {
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-lg);
  padding: var(--ff-space-3);
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-1);
  text-align: left;
  background: transparent;
  color: var(--ff-text-1);
  font: inherit;
  cursor: pointer;
  min-width: 0;
}
.preset:hover:not(.on):not(:disabled) {
  background: var(--ff-bg-hover);
}
/* 标题单行完整显示（“MP4 · H.264”13/500 约 80px，卡片内宽 97px）；.pt 的省略号只作兜底，完整名在 title 里 */
.preset b {
  font-weight: 500;
  font-size: var(--ff-fs-sm);
  display: block;
  min-width: 0;
  line-height: 1.5;
  white-space: nowrap;
}
.preset b .pt {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.preset small {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.preset.on {
  border-color: var(--ff-primary);
  background: var(--ff-primary-soft);
}
.preset.on b {
  color: var(--ff-primary-text);
}
.preset:focus-visible,
.seg button:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.hint {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.field label {
  display: block;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  margin-bottom: var(--ff-space-2);
}
.out {
  display: flex;
  gap: var(--ff-space-2);
}
.chip {
  flex: 1;
  min-width: 0;
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 var(--ff-space-3);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-md);
  background: var(--ff-bg-surface);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  overflow: hidden;
  white-space: nowrap;
}
.chip .ph {
  color: var(--ff-text-2);
}
.chip .h {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.chip .t {
  flex: none;
  color: var(--ff-text-1);
}
.outnote {
  margin-top: var(--ff-space-2);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.5;
}
.outnote .ff-link {
  margin-left: 0;
  font-size: var(--ff-fs-xs);
}
.gate {
  display: flex;
  gap: var(--ff-space-2);
  align-items: flex-start;
  margin: 0 var(--ff-space-4) var(--ff-space-3);
  padding: var(--ff-space-2) var(--ff-space-3);
  border-radius: var(--ff-radius-lg);
  background: color-mix(in srgb, var(--ff-warning) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--ff-warning) 28%, transparent);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.5;
}
.gate svg {
  color: var(--ff-warning-text);
  margin-top: var(--ff-space-1);
  flex: none;
}
/* 读取文件信息等正常过程：主色软底（规范 7.1“安装中”同款），不用警告色 */
.gate.info {
  background: var(--ff-primary-soft);
  border-color: color-mix(in srgb, var(--ff-primary) 28%, transparent);
}
.gate.info svg {
  color: var(--ff-primary-text);
}
.suberr {
  margin: 0 var(--ff-space-4) var(--ff-space-3);
}
.perr {
  flex: none;
}
.foot {
  padding: var(--ff-space-3) var(--ff-space-4);
  border-top: 1px solid var(--ff-border);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ff-space-2);
  row-gap: var(--ff-space-2);
  flex: none;
}
/* 有提示文字时：提示占满一行（254px，最多约 2 行），按钮另起一行靠右 */
.fhint {
  flex: 1 1 100%;
}
.foot .start {
  margin-left: auto;
}
/* failed：第一行“再转一个”“打开输出位置”，主按钮“重试失败项”单独第二行靠右 */
.frow {
  flex: 1 1 100%;
  display: flex;
  gap: var(--ff-space-2);
}
.foot .retry {
  margin-left: auto;
}
.foot small {
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
}
.fhint {
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-1);
  min-width: 0;
  line-height: 1.5;
}
.fhint .ff-link {
  margin-left: 0;
}
.foot .start {
  flex: none;
}
.lnk {
  color: var(--ff-primary-text);
  font-size: var(--ff-fs-sm);
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.lnk:hover {
  text-decoration: underline;
}
</style>
