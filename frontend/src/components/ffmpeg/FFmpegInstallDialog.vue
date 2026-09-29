<template>
  <Teleport to="body">
    <div v-if="ffmpeg.dialogOpen" class="mask">
      <div class="dlg" role="dialog" aria-modal="true">
        <div class="big"><FIcon name="download" :size="24" /></div>

        <template v-if="!installing">
          <h3>需要安装 ffmpeg</h3>
          <p>应用需要 ffmpeg 才能转换、剪辑和直播。可以现在自动下载安装到应用数据目录，也可以稍后再说。</p>
          <div v-if="ffmpeg.status.state === 'failed' && ffmpeg.status.error" class="perr fail" role="alert">
            <FIcon name="warn" :size="14" />
            <span>
              下载失败：{{ ffmpeg.status.error.message }}<template v-if="ffmpeg.status.error.detail"><br /><small>{{ ffmpeg.status.error.detail }}</small></template>
              <button v-if="ffmpeg.canSwitchMirror" type="button" class="ff-link" @click="safe(ffmpeg.retryWithOtherMirror)">换下载源重试</button>
            </span>
          </div>
          <div v-if="!ffmpeg.installAvailable" class="soon" role="status">
            <FIcon name="warn" :size="14" />
            <span>安装功能即将上线。已经装过 ffmpeg 的话，可以手动指定位置。</span>
          </div>
          <div v-if="ffmpeg.manualInputOpen && !ffmpeg.canPickDirectory" class="manual">
            <el-input v-model="manualDir" size="default" placeholder="ffmpeg 所在目录，例如 /usr/local/bin" :class="{ 'ff-input-bad': pathError }" @keyup.enter="applyManual" />
            <el-button size="default" type="primary" :loading="busy" :disabled="!manualDir.trim()" @click="applyManual">确定</el-button>
          </div>
          <div v-if="pathError" class="perr" role="alert">
            <FIcon name="warn" :size="14" />
            <span>{{ pathError.message }}<template v-if="pathError.detail"><br /><small>{{ pathError.detail }}</small></template></span>
          </div>
          <div class="dfoot">
            <el-button size="default" @click="safe(ffmpeg.dismissPrompt)">稍后</el-button>
            <el-button
              size="default"
              type="primary"
              :disabled="!ffmpeg.installAvailable"
              :title="ffmpeg.installAvailable ? undefined : '安装功能即将上线'"
              @click="safe(() => ffmpeg.startInstall(ffmpeg.status.state === 'failed' ? undefined : ffmpeg.sources[0]))"
            >{{ ffmpeg.installAvailable ? '下载' : '下载（即将上线）' }}</el-button>
          </div>
          <div class="manual-link">
            <button type="button" class="ff-link" @click="onManual">手动指定 ffmpeg 位置</button>
          </div>
        </template>

        <template v-else>
          <h3>正在安装 ffmpeg</h3>
          <p>{{ stageText }}，完成后转换、剪辑和直播功能会自动解锁。</p>
          <div class="progress"><i :style="{ width: percent + '%' }" /></div>
          <div class="pmeta">
            <span>{{ percent }}%</span>
            <span>{{ ffmpeg.install?.speedText }}<template v-if="ffmpeg.install?.remainText"> · {{ ffmpeg.install.remainText }}</template></span>
          </div>
          <div class="dfoot">
            <span class="sp" />
            <el-button size="default" :loading="canceling" @click="cancelInstall">取消安装</el-button>
            <el-button size="default" @click="ffmpeg.dialogOpen = false">后台安装</el-button>
          </div>
        </template>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { toAppError, type AppError } from '@/api/call'

const ffmpeg = useFFmpegStore()
const installing = computed(() => ffmpeg.status.state === 'installing')
const percent = computed(() => Math.round((ffmpeg.install?.progress ?? 0) * 100))
const STAGES = { download: '正在下载', verify: '正在校验', extract: '正在解压', validate: '正在验证' }
const stageText = computed(() => (ffmpeg.install ? STAGES[ffmpeg.install.stage] : '正在准备'))

const manualDir = ref('')
const busy = ref(false)
const canceling = ref(false)

async function cancelInstall() {
  canceling.value = true
  try {
    await safe(ffmpeg.cancelInstall)
    ffmpeg.dialogOpen = false
  } finally {
    canceling.value = false
  }
}
const pathError = ref<AppError | null>(null)

async function safe(fn: () => Promise<unknown>) {
  try {
    await fn()
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}

function onManual() {
  pathError.value = null
  if (ffmpeg.canPickDirectory) {
    safe(() => ffmpeg.pickPath()) // 系统目录选择器（标题「选择 ffmpeg 所在文件夹」在 store 里传入）
  } else {
    ffmpeg.manualInputOpen = !ffmpeg.manualInputOpen
  }
}

/** 文本框提交：校验失败（INVALID_ARGUMENT）显示在框下面，其他错误弹提示 */
async function applyManual() {
  const dir = manualDir.value.trim()
  if (!dir || busy.value) return
  busy.value = true
  pathError.value = null
  try {
    await ffmpeg.pickPath(dir)
    ffmpeg.manualInputOpen = false
    manualDir.value = ''
  } catch (e) {
    const err = toAppError(e)
    if (err.code === 'INVALID_ARGUMENT') pathError.value = err
    else ElMessage.error(err.message)
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.mask {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: rgba(0, 0, 0, 0.45);
  display: grid;
  place-items: center;
}
.dlg {
  width: 440px;
  padding: 24px;
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-xl);
  box-shadow: var(--ff-shadow-dialog);
}
.big {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--ff-primary-soft);
  color: var(--ff-primary);
  display: grid;
  place-items: center;
  margin-bottom: 16px;
}
h3 {
  margin: 0 0 8px;
  font-size: var(--ff-fs-lg);
  font-weight: 600;
}
p {
  margin: 0;
  color: var(--ff-text-2);
}
.progress {
  margin-top: 16px;
  height: 8px;
  border-radius: 4px;
  background: var(--ff-border);
  overflow: hidden;
}
.progress i {
  display: block;
  height: 100%;
  background: var(--ff-primary);
  border-radius: 4px;
  transition: width var(--ff-dur-base) var(--ff-ease);
}
.pmeta {
  display: flex;
  justify-content: space-between;
  margin: 8px 0 24px;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.soon,
.perr {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  margin: 16px 0 0;
  padding: 8px 12px;
  border-radius: var(--ff-radius-md);
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
}
.soon {
  background: var(--ff-warning-soft);
  color: var(--ff-text-1);
}
.soon > :first-child {
  color: var(--ff-warning-text);
  margin-top: 2px;
}
.perr {
  color: var(--ff-danger-text);
}
.perr small {
  color: var(--ff-text-2);
  white-space: pre-line;
}
.manual {
  display: flex;
  gap: 8px;
  margin-top: 16px;
}
.dfoot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 24px;
}
.manual-link {
  margin-top: 16px;
  text-align: center;
  font-size: var(--ff-fs-xs);
}
.ff-link {
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
.perr .ff-link {
  margin-left: 12px;
}
.sp {
  flex: 1;
}
</style>
