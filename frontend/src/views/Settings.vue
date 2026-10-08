<template>
  <div class="settings spanels">
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

    <!-- ffmpeg（与关于页共用 FFmpegPanel；这里带操作按钮） -->
    <FFmpegPanel id="sec-ffmpeg" heading-id="h-ffmpeg">
      <template #version-actions>
        <button v-if="canInstall" type="button" class="btn pri" @click="ffmpeg.dialogOpen = true"><FIcon name="download" :size="15" />{{ installLabel }}</button>
        <button type="button" class="btn" :disabled="busy || ffmpeg.status.state === 'installing'" @click="run(ffmpeg.recheck)"><FIcon name="refresh" :size="15" />重新检测</button>
      </template>
      <template #path-actions>
        <button type="button" class="btn" :disabled="busy" @click="run(() => ffmpeg.pickPath())"><FIcon name="folder" :size="15" />更换</button>
        <button v-if="ffmpeg.status.source === 'custom'" type="button" class="btn text" :disabled="busy" @click="run(ffmpeg.clearCustomPath)">恢复默认</button>
      </template>
    </FFmpegPanel>

    <!-- 编码设备：后端绑定接通（ENCODER_BACKEND_READY）才显示；纯浏览器只有 ?enc= 才显示模拟层 -->
    <EncoderDevicePanel v-if="encoderVisible" id="sec-encoder" heading-id="h-encoder" />
    <!-- 回退提示的展示预览（仅浏览器 ?enc=…&fb=1；真实运行不出现，也没有接线） -->
    <section v-if="encoderVisible && fbPreview" class="panel group" aria-label="回退提示预览">
      <div class="phead"><h2>回退提示（展示预览，未接线）</h2></div>
      <div class="fbp">
        <EncoderFallbackNotice variant="convert" />
        <EncoderFallbackNotice variant="live" />
        <EncoderFallbackNotice variant="row" />
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
      <!-- v0.24：默认输出位置移到“存储”（转换结果），这一行不再显示 -->
      <OutputDirRow v-if="!storageOn" class="srow" />
    </section>

    <!-- 存储（v0.24：转换结果 / 上传文件两个目录；CONVERT_V24_BACKEND_READY 关时真实运行不显示） -->
    <StoragePanel v-if="storageOn" id="sec-storage" heading-id="h-storage" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ENCODER_SECTION_ID, ENCODER_SECTION_QUERY } from '@/api/encoderTask'
import { scrollBehavior } from '@/utils/motion'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import FFmpegPanel from '@/components/settings/FFmpegPanel.vue'
import OutputDirRow from '@/components/settings/OutputDirRow.vue'
import StoragePanel from '@/components/settings/StoragePanel.vue'
import { convertV24On } from '@/api/convertRecords'
import EncoderDevicePanel from '@/components/encoder/EncoderDevicePanel.vue'
import EncoderFallbackNotice from '@/components/encoder/EncoderFallbackNotice.vue'
import { encoderPanelVisible } from '@/api/encoder'
import { simParam } from '@/api/sim'
import { toAppError } from '@/api/call'
import { MAX_CONCURRENT_AUTO, MAX_CONCURRENT_MAX, getMaxConcurrent, setMaxConcurrent } from '@/api/system'
import { useTheme, type ThemeMode } from '@/composables/useTheme'
import { useFFmpegStore } from '@/stores/ffmpeg'

const { mode } = useTheme()
const storageOn = convertV24On()
const encoderVisible = encoderPanelVisible()

// 从提示条“编码设置”跳来（?section=encoder）：滚到“编码设备”并把焦点放到它的标题（tabindex=-1，读屏会读出小节名）。
// 这一块不显示时（标志关 / 纯浏览器没有 ?enc=）什么也不做，停在页顶。
const route = useRoute()
async function focusEncoderSection() {
  if (route.query.section !== ENCODER_SECTION_QUERY || !encoderVisible) return
  await nextTick()
  const sec = document.getElementById(ENCODER_SECTION_ID)
  if (!sec) return
  sec.scrollIntoView({ behavior: scrollBehavior(), block: 'start' })
  sec.querySelector<HTMLElement>('h2')?.focus({ preventScroll: true })
}
watch(() => route.query.section, focusEncoderSection)
// 从转换页“打开存储设置”跳来（?section=storage）：滚到“存储”
async function focusStorageSection() {
  if (route.query.section !== 'storage' || !storageOn) return
  await nextTick()
  const sec = document.getElementById('sec-storage')
  if (!sec) return
  sec.scrollIntoView({ behavior: scrollBehavior(), block: 'start' })
  sec.querySelector<HTMLElement>('h2')?.focus({ preventScroll: true })
}
watch(() => route.query.section, focusStorageSection)
const fbPreview = encoderVisible && simParam('fb') === '1'
const themeOptions: { value: ThemeMode; label: string }[] = [
  { value: 'light', label: '浅色' },
  { value: 'dark', label: '暗色' },
  { value: 'system', label: '跟随系统' },
]

// ---- ffmpeg ----
const ffmpeg = useFFmpegStore()
const busy = ref(false)

// 安装入口：缺失 / 过旧 / 失败时显示，打开与侧栏、提示条同一个安装对话框
const canInstall = computed(() => ['missing', 'outdated', 'failed'].includes(ffmpeg.status.state))
const installLabel = computed(() => (ffmpeg.status.state === 'failed' ? '重试安装' : '安装…'))

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
const concurrentHint = computed(() => (concurrent.value === MAX_CONCURRENT_AUTO ? '自动：按 CPU 核数决定，最多同时转换 1 到 3 个。' : '同时进行的转换等任务数量，减小不会打断正在运行的任务。'))

onMounted(async () => {
  void focusEncoderSection()
  void focusStorageSection()
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
.fbp {
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
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
  .stepper .sb {
    transition: none;
  }
}
</style>
