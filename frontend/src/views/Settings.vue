<template>
  <div class="settings">
    <!-- 外观 -->
    <section id="sec-appearance" class="panel group" aria-labelledby="h-appearance">
      <div class="phead"><h2 id="h-appearance">外观</h2></div>
      <div class="srow">
        <div class="l">
          <b id="theme-label">主题</b>
          <small>跟随系统会随操作系统的浅色和暗色设置自动切换。</small>
        </div>
        <div class="themes" role="radiogroup" aria-labelledby="theme-label">
          <label v-for="t in themeOptions" :key="t.value" class="th" :class="{ on: mode === t.value }">
            <input v-model="mode" type="radio" name="theme" class="sr-only" :value="t.value" />
            <span class="pv" aria-hidden="true">
              <!-- 预览用 .ff-light / .ff-dark 局部强制主题，颜色仍取自 tokens，不另存色值 -->
              <span v-if="t.value !== 'system'" class="half" :class="t.value === 'dark' ? 'ff-dark' : 'ff-light'">
                <i class="side" /><span class="main"><i class="bar" /><i class="card" /></span>
              </span>
              <template v-else>
                <span class="half ff-light"><i class="side" /><span class="main" /></span>
                <span class="half ff-dark"><span class="main" /></span>
              </template>
            </span>
            {{ t.label }}
          </label>
        </div>
      </div>
    </section>

    <!-- ffmpeg -->
    <section id="sec-ffmpeg" class="panel group" aria-labelledby="h-ffmpeg">
      <div class="phead">
        <h2 id="h-ffmpeg">ffmpeg</h2>
        <span class="tag" :class="ffView.tone">{{ ffView.tag }}</span>
      </div>
      <div class="srow">
        <div class="l">
          <b>当前版本</b>
          <small>{{ ffView.detail }}</small>
        </div>
        <button v-if="canInstall" type="button" class="btn pri" @click="ffmpeg.dialogOpen = true"><FIcon name="download" :size="15" />{{ installLabel }}</button>
        <button type="button" class="btn" :disabled="busy || ffmpeg.status.state === 'installing'" @click="run(ffmpeg.recheck)"><FIcon name="refresh" :size="15" />重新检测</button>
      </div>
      <div class="srow">
        <div class="l">
          <b>路径</b>
          <small v-if="ffmpeg.status.source === 'custom'">手动指定的位置，恢复后会重新自动查找。</small>
        </div>
        <div class="pathbox" :class="{ empty: !ffmpeg.status.path }" :title="ffmpeg.status.path || undefined">
          <span v-if="!ffmpeg.status.path" class="ph">未找到</span>
          <template v-else><span class="h">{{ pathParts.head }}</span><span class="t">{{ pathParts.tail }}</span></template>
        </div>
        <button type="button" class="btn" :disabled="busy" @click="run(() => ffmpeg.pickPath())"><FIcon name="folder" :size="15" />更换</button>
        <button v-if="ffmpeg.status.source === 'custom'" type="button" class="btn text" :disabled="busy" @click="run(ffmpeg.clearCustomPath)">恢复默认</button>
      </div>
    </section>

    <!-- 转换 -->
    <section id="sec-convert" class="panel group" aria-labelledby="h-convert">
      <div class="phead"><h2 id="h-convert">转换</h2></div>
      <div class="srow">
        <div class="l">
          <b id="mc-label">同时转换数量</b>
          <small id="mc-desc">{{ concurrentHint }}直播任务不占用名额。</small>
        </div>
        <div
          class="stepper"
          :class="{ busy: !loaded }"
          role="spinbutton"
          tabindex="0"
          aria-labelledby="mc-label"
          aria-describedby="mc-desc"
          :aria-valuemin="MAX_CONCURRENT_AUTO"
          :aria-valuemax="MAX_CONCURRENT_MAX"
          :aria-valuenow="concurrent"
          :aria-valuetext="concurrentText"
          :aria-disabled="!loaded"
          @keydown.up.prevent="step(1)"
          @keydown.down.prevent="step(-1)"
          @keydown.home.prevent="setTo(MAX_CONCURRENT_AUTO)"
          @keydown.end.prevent="setTo(MAX_CONCURRENT_MAX)"
        >
          <button type="button" class="sb" tabindex="-1" aria-label="减少" :disabled="!loaded || concurrent <= MAX_CONCURRENT_AUTO" @click="step(-1)">−</button>
          <b class="val" aria-hidden="true">{{ concurrentText }}</b>
          <button type="button" class="sb" tabindex="-1" aria-label="增加" :disabled="!loaded || concurrent >= MAX_CONCURRENT_MAX" @click="step(1)">+</button>
        </div>
      </div>
      <OutputDirRow class="srow" />
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import OutputDirRow from '@/components/settings/OutputDirRow.vue'
import { toAppError } from '@/api/call'
import { MAX_CONCURRENT_AUTO, MAX_CONCURRENT_MAX, getMaxConcurrent, setMaxConcurrent } from '@/api/system'
import { useTheme, type ThemeMode } from '@/composables/useTheme'
import { useFFmpegStore } from '@/stores/ffmpeg'

const { mode } = useTheme()
const themeOptions: { value: ThemeMode; label: string }[] = [
  { value: 'light', label: '浅色' },
  { value: 'dark', label: '暗色' },
  { value: 'system', label: '跟随系统' },
]

// ---- ffmpeg ----
const ffmpeg = useFFmpegStore()
const busy = ref(false)
const SOURCE: Record<string, string> = { bundled: '应用目录', system: '系统环境', custom: '手动指定', legacy: '旧版目录' }

const ffView = computed(() => {
  const s = ffmpeg.status
  const from = SOURCE[s.source ?? '']
  switch (s.state) {
    case 'ready':
      return { tag: '已就绪', tone: 'ok', detail: [s.version && `ffmpeg ${s.version}`, from && `来自${from}`].filter(Boolean).join(' · ') || '已检测到 ffmpeg' + (s.ffprobeMissing ? '，但缺少 ffprobe' : '') }
    case 'installing':
      return { tag: '安装中', tone: 'run', detail: ffmpeg.install ? `下载中 ${Math.round(ffmpeg.install.progress * 100)}%` : '准备中' }
    case 'failed':
      return { tag: '安装失败', tone: 'fail', detail: s.error?.message || '安装没有成功，可以重试或手动指定位置。' }
    case 'outdated':
      return { tag: '版本过旧', tone: 'warn', detail: `${s.version ? `当前 ffmpeg ${s.version}，` : ''}需要 6.0 或更高版本。` }
    case 'missing':
      return { tag: '未安装', tone: 'warn', detail: '转换、剪辑、直播暂不可用。' }
    default:
      return { tag: '检测中', tone: 'q', detail: '正在检测 ffmpeg，请稍候。' }
  }
})
// 安装入口：缺失 / 过旧 / 失败时显示，打开与侧栏、提示条同一个安装对话框
const canInstall = computed(() => ['missing', 'outdated', 'failed'].includes(ffmpeg.status.state))
const installLabel = computed(() => (ffmpeg.status.state === 'failed' ? '重试安装' : '安装…'))

const pathParts = computed(() => {
  const p = ffmpeg.status.path ?? ''
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i < 0 ? { head: '', tail: p } : { head: p.slice(0, i + 1), tail: p.slice(i + 1) }
})

async function run(fn: () => Promise<unknown>) {
  if (busy.value) return
  busy.value = true
  try {
    await fn()
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  } finally {
    busy.value = false
  }
}

// ---- 同时转换数量（Settings.maxConcurrent：0 = 自动，1~8 固定值）----
/** 界面上的值；点击立即变化，保存失败时回退到 saved */
const concurrent = ref(MAX_CONCURRENT_AUTO)
/** 后端确认过的值 */
let saved = MAX_CONCURRENT_AUTO
const loaded = ref(false)
const concurrentText = computed(() => (concurrent.value === MAX_CONCURRENT_AUTO ? '自动' : String(concurrent.value)))
const concurrentHint = computed(() => (concurrent.value === MAX_CONCURRENT_AUTO ? '自动：按 CPU 核数决定，最多同时转换 1 到 3 个。' : '同时进行的转换、剪辑等任务数量，减小不会打断正在运行的任务。'))

onMounted(async () => {
  try {
    saved = await getMaxConcurrent()
    concurrent.value = saved
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  } finally {
    loaded.value = true
  }
})

// 连续点击时只保存最后一次：保存串行执行，落地后如果界面值又变了就再存一次
let saving = false
async function flush() {
  if (saving) return
  saving = true
  try {
    while (concurrent.value !== saved) {
      const want = concurrent.value
      try {
        await setMaxConcurrent(want)
        saved = want
      } catch (e) {
        concurrent.value = saved // 后端拒绝 / IO 出错：整体不生效，回退到已确认的值
        ElMessage.error(toAppError(e).message)
      }
    }
  } finally {
    saving = false
  }
}

function setTo(n: number) {
  if (!loaded.value) return
  concurrent.value = Math.min(MAX_CONCURRENT_MAX, Math.max(MAX_CONCURRENT_AUTO, n))
  flush()
}
function step(d: number) {
  setTo(concurrent.value + d)
}
</script>

<style scoped>
.settings {
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
}
.group {
  padding: 0; /* 面板内边距由 phead / srow 提供，与原型一致 */
  scroll-margin-top: var(--ff-space-2);
}
.phead {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-3) var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
}
.phead h2 {
  margin: 0;
  font-size: var(--ff-fs-md);
  font-weight: 600;
}
.srow,
.group :deep(.srow) {
  display: flex;
  align-items: center;
  gap: var(--ff-space-4);
  padding: var(--ff-space-3) var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
}
.group :deep(.srow.od) {
  align-items: flex-start; /* 输出位置行带错误提示时高度会变，控件顶对齐（原型 .srow.od） */
}
.srow:last-child,
.group :deep(.srow:last-child) {
  border-bottom: none;
}
.l {
  flex: 1;
  min-width: 0;
}
.l b {
  display: block;
  font-weight: 500;
}
.l small {
  display: block;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}

/* 状态标签：文字色取 *-text token（14% 着色底上 ≥4.5:1） */
.tag {
  height: 20px;
  padding: 0 7px;
  border-radius: var(--ff-radius-sm);
  font-size: var(--ff-fs-xs);
  display: inline-flex;
  align-items: center;
}
.tag.ok {
  background: color-mix(in srgb, var(--ff-success) 14%, transparent);
  color: var(--ff-success-text);
}
.tag.warn {
  background: color-mix(in srgb, var(--ff-warning) 14%, transparent);
  color: var(--ff-warning-text);
}
.tag.fail {
  background: color-mix(in srgb, var(--ff-danger) 14%, transparent);
  color: var(--ff-danger-text);
}
.tag.run {
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
}
.tag.q {
  background: var(--ff-bg-hover);
  color: var(--ff-text-2);
}

/* 按钮：与 OutputDirRow 同一套（28px 高，边框 / 文字色取 token） */
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
  flex: none;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
.btn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.btn:disabled {
  opacity: 0.45;
  cursor: default;
}
.btn:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.btn.pri {
  background: var(--ff-badge-bg);
  border-color: var(--ff-badge-bg);
  color: var(--ff-on-primary);
}
.btn.pri:hover:not(:disabled) {
  background: var(--ff-primary-hover);
  border-color: var(--ff-primary-hover);
}
.btn.text {
  border-color: transparent;
  background: transparent;
  color: var(--ff-primary-text);
  padding: 0 8px;
}

/* ffmpeg 路径：只读展示，最后一级单独一段不被省略 */
.pathbox {
  width: 360px;
  flex: none;
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 12px;
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-md);
  background: var(--ff-bg-surface);
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  overflow: hidden;
  white-space: nowrap;
}
.pathbox .ph {
  color: var(--ff-text-2);
}
.pathbox .h {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pathbox .t {
  flex: none;
}

/* 主题卡片 */
.themes {
  display: flex;
  gap: var(--ff-space-3);
}
.th {
  width: 118px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  cursor: pointer;
}
.th .pv {
  width: 118px;
  height: 72px;
  display: flex;
  border-radius: 8px;
  border: 1px solid var(--ff-border);
  overflow: hidden;
}
.th.on {
  color: var(--ff-primary-text);
  font-weight: 500;
}
.th.on .pv {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.th:has(input:focus-visible) .pv {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.th:hover:not(.on) {
  color: var(--ff-text-1);
}
.half {
  flex: 1;
  display: flex;
  min-width: 0;
}
.half .side {
  width: 26%;
  background: var(--ff-bg-sidebar);
}
.half .main {
  flex: 1;
  background: var(--ff-bg-app);
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.half .bar {
  height: 10px;
  border-radius: 3px;
  background: var(--ff-bg-surface);
}
.half .card {
  height: 26px;
  border-radius: 3px;
  background: var(--ff-bg-surface);
}

/* 步进器：非文字边界用 --ff-text-2 保证 ≥3:1（--ff-border 只有约 1.3:1） */
.stepper {
  display: flex;
  align-items: center;
  height: 28px;
  flex: none;
  border: 1px solid var(--ff-text-2);
  border-radius: var(--ff-radius-md);
  background: var(--ff-bg-surface);
}
.stepper:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.stepper .sb {
  width: 28px;
  height: 100%;
  border: 0;
  background: transparent;
  color: var(--ff-text-1);
  font: inherit;
  font-size: var(--ff-fs-lg);
  line-height: 1;
  cursor: pointer;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
.stepper .sb:first-child {
  border-radius: 5px 0 0 5px;
}
.stepper .sb:last-child {
  border-radius: 0 5px 5px 0;
}
.stepper .sb:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.stepper .sb:disabled {
  color: var(--ff-text-3);
  cursor: default;
}
.stepper .val {
  min-width: 44px;
  padding: 0 6px;
  text-align: center;
  line-height: 26px;
  font-weight: 500;
  border-left: 1px solid var(--ff-text-2);
  border-right: 1px solid var(--ff-text-2);
}
.stepper.busy {
  opacity: 0.6;
}

@media (prefers-reduced-motion: reduce) {
  .btn,
  .stepper .sb {
    transition: none;
  }
}
</style>
