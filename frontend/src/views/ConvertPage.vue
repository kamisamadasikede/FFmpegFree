<template>
  <div class="cv">
    <!-- 左：待转换文件 -->
    <section class="col">
      <!-- 转换中：整体进度（设计稿 ?state=running 的 rprog） -->
      <div v-if="cv.mode === 'running'" class="panel rprog" role="status">
        <div class="top">
          <b>正在转换</b>
          <span class="sub">已完成 {{ cv.overall.done }} / {{ cv.overall.total }} 个文件</span>
          <span class="pct">{{ cv.overall.pct }}%</span>
        </div>
        <div class="bar" role="progressbar" aria-label="整体进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="cv.overall.pct"><i class="run" :style="{ width: cv.overall.pct + '%' }" /></div>
        <div class="meta">
          <span>速度<b>{{ cv.overall.speed || '—' }}</b></span>
          <span>预计剩余<b>{{ formatEta(cv.overall.etaSec) || '—' }}</b></span>
        </div>
      </div>
      <div v-else-if="cv.mode === 'done'" class="okline" role="status">
        <FIcon name="check" :size="16" />
        <div class="okbody"><b>转换完成</b>{{ cv.succeededRows.length }} 个文件已保存<template v-if="cv.outputFolder"> · <span class="okdir" :title="cv.outputFolder">{{ cv.outputFolder }}</span></template></div>
      </div>

      <div class="panel list-panel">
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
        <div v-if="!cv.rows.length" class="dz big" :class="{ over }" :style="dropStyle" @dragenter.prevent="over = true" @dragover.prevent="over = true" @dragleave="over = false" @drop.prevent="over = false">
          <div class="ic"><FIcon name="upload" :size="32" /></div>
          <b>拖入视频或音频文件，或点击选择</b>
          <small>支持常见视频、音频格式，一次最多 {{ MAX_SUBMIT }} 个</small>
          <button type="button" class="btn pri" @click="cv.chooseFiles()">选择文件</button>
          <small v-if="cv.pickSoon" class="soon" role="status">{{ cv.pickSoon }}</small>
        </div>

        <template v-else>
          <div v-if="cv.mode !== 'running'" class="dz small" :class="{ over }" :style="dropStyle" @dragenter.prevent="over = true" @dragover.prevent="over = true" @dragleave="over = false" @drop.prevent="over = false" @click="cv.chooseFiles()">
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
              :log-text="logKey === r.key ? logText : null"
              @visible="cv.ensureThumb(r)"
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
        </template>
      </div>
    </section>

    <!-- 右：输出设置 -->
    <aside class="rcol">
      <div class="panel settings" :class="{ off: cv.mode === 'running' }">
        <div class="phead"><h2>输出设置</h2><span class="sp" /><span class="sub">应用到全部</span></div>
        <div class="pbody">
          <div class="seg" role="tablist" aria-label="预设类别">
            <button v-for="g in GROUPS" :id="`pg-${g.key}`" :key="g.key" type="button" role="tab" :aria-selected="group === g.key" :class="{ on: group === g.key }" @click="group = g.key">{{ g.label }}</button>
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
              <b><FIcon :name="iconOf(p)" :size="15" />{{ split(p.name).title }}</b>
              <small>{{ split(p.name).sub || optionLine(p) }}</small>
            </button>
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

        <div class="gate" v-if="gateText" role="status">
          <FIcon name="warn" :size="14" />
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
            <button type="button" class="btn pri lg" @click="onRevealOutput"><FIcon name="folder" :size="15" />打开输出位置</button>
          </template>
          <template v-else-if="cv.mode === 'failed'">
            <button type="button" class="btn lg" @click="cv.clear()">再转一个</button>
            <span class="sp" />
            <button type="button" class="btn pri lg" @click="cv.retryAllFailed()"><FIcon name="retry" :size="15" />重试失败项</button>
          </template>
          <template v-else>
            <small>{{ footHint }}</small>
            <span class="sp" />
            <button type="button" class="btn pri lg" :disabled="!!cv.startBlockReason || cv.submitting" @click="cv.submit()"><FIcon name="play" :size="15" />开始转换</button>
          </template>
        </div>
      </div>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import ErrorLine from '@/components/common/ErrorLine.vue'
import ConvertFileRow from '@/components/convert/ConvertFileRow.vue'
import type { IconName } from '@/components/icon/icons'
import { MAX_SUBMIT, type PresetItem } from '@/api/convert'
import { onFilesDropped } from '@/api/fileDrop'
import { toAppError } from '@/api/call'
import { hasWailsBackend } from '@/services/wails'
import { actionErrorText } from '@/errors/errorMessages'
import { PREVIEW_CONVERT, splitPresetName, useConvertStore, type ConvertRow } from '@/stores/convert'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { useTaskStore } from '@/stores/tasks'
import { formatBytes, formatEta } from '@/utils/format'

const cv = useConvertStore()
const ffmpeg = useFFmpegStore()
const tasks = useTaskStore()

const split = splitPresetName
const over = ref(false)
/** Wails 的拖放目标标记：只有落在带这个样式的区域才回调 OnFileDrop */
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
function iconOf(p: PresetItem): IconName {
  const o = p.options
  if (isAudioPreset(p)) return 'music'
  if (o.container === 'gif') return 'gif'
  if (o.videoCodec === 'h265') return 'zip'
  if (o.videoCodec === 'copy') return 'convert'
  if (o.container === 'mp4' && !o.width) return 'phone'
  return 'play'
}
function optionLine(p: PresetItem) {
  return p.options.container.toUpperCase()
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
const gateText = computed(() => {
  if (cv.mode === 'running' || cv.mode === 'done' || cv.mode === 'failed') return ''
  switch (cv.startBlockReason) {
    case 'ffmpeg': return ffmpeg.status.state === 'checking' ? '正在检测 ffmpeg…' : ffmpeg.status.state === 'installing' ? 'ffmpeg 正在安装，装好后就可以转换。' : '需要先安装 ffmpeg 才能转换。'
    case 'probing': return '正在读取文件信息…'
    case 'blocked': return `有 ${cv.blockedCount} 个文件无法转换，请先把它们移出列表。`
    default: return ''
  }
})
const footHint = computed(() => {
  const n = cv.submittableRows.length
  if (!cv.rows.length) return '先添加文件'
  return n ? `${n} 个文件 · 转为 ${cv.presetShort || '…'}` : ''
})

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
  void cv.init()
  cv.refreshDefaultDir()
  offDrop = onFilesDropped((paths) => cv.addPaths(paths))
  if (PREVIEW_CONVERT && !cv.rows.length && PREVIEW_CONVERT !== 'idle') cv.seedPreview(PREVIEW_CONVERT)
})
onUnmounted(() => offDrop())
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
  color: var(--ff-text-3);
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
  gap: 6px;
  font: inherit;
  font-size: var(--ff-fs-sm);
  white-space: nowrap;
  cursor: pointer;
}
.btn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.btn:disabled {
  opacity: 0.45;
  cursor: default;
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
.dz.over,
.dz.wails-drop-target-active {
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
  color: var(--ff-text-3);
}
.dz .soon {
  color: var(--ff-warning-text);
}
.dz.big {
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
  border-radius: 10px;
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
  margin-top: 2px;
}
.list {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0 var(--ff-space-2) var(--ff-space-2);
}

/* 整体进度 / 完成提示 */
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
  padding: 8px var(--ff-space-3);
  border-radius: 8px;
  background: color-mix(in srgb, var(--ff-success) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--ff-success) 28%, transparent);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.5;
}
.okline svg {
  color: var(--ff-success);
  margin-top: 1px;
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
  overflow: auto;
  padding: var(--ff-space-4);
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.settings.off .pbody {
  opacity: 0.45;
  pointer-events: none;
}
.seg {
  display: flex;
  background: var(--ff-bg-hover);
  border-radius: var(--ff-radius-md);
  padding: 2px;
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
  border-radius: 8px;
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
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
.preset b {
  font-weight: 500;
  font-size: var(--ff-fs-sm);
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}
.preset b svg {
  color: var(--ff-text-2);
}
.preset small {
  font-size: 11px;
  color: var(--ff-text-3);
  line-height: 1.4;
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
.preset.on b,
.preset.on b svg {
  color: var(--ff-primary-text);
}
.preset:focus-visible,
.seg button:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.hint {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-3);
}
.field label {
  display: block;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  margin-bottom: 6px;
}
.out {
  display: flex;
  gap: 6px;
}
.chip {
  flex: 1;
  min-width: 0;
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-md);
  background: var(--ff-bg-surface);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  overflow: hidden;
  white-space: nowrap;
}
.chip .ph {
  color: var(--ff-text-3);
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
  margin-top: 6px;
  font-size: 11px;
  color: var(--ff-text-3);
  line-height: 1.5;
}
.outnote .ff-link {
  margin-left: 0;
  font-size: 11px;
}
.gate {
  display: flex;
  gap: var(--ff-space-2);
  align-items: flex-start;
  margin: 0 var(--ff-space-4) var(--ff-space-3);
  padding: 8px var(--ff-space-3);
  border-radius: 8px;
  background: color-mix(in srgb, var(--ff-warning) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--ff-warning) 28%, transparent);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  line-height: 1.5;
}
.gate svg {
  color: var(--ff-warning-text);
  margin-top: 2px;
  flex: none;
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
  align-items: center;
  gap: var(--ff-space-2);
  flex: none;
}
.foot small {
  color: var(--ff-text-3);
  font-size: var(--ff-fs-xs);
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
