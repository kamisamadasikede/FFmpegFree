<template>
  <Teleport to="body">
    <div v-if="ffmpeg.dialogOpen" class="mask">
      <div class="dlg" role="dialog" aria-modal="true">
        <div class="big"><FIcon name="download" :size="24" /></div>

        <template v-if="!installing">
          <h3>需要安装 ffmpeg</h3>
          <p>转换、剪辑和直播功能依赖 ffmpeg。我们会自动下载适合你电脑的版本，安装到应用目录，不会修改系统环境变量。文档和 JSON 工具不受影响，可以照常使用。</p>
          <div class="dinfo">
            <div><span>系统</span><b>{{ platformText }}</b></div>
            <div>
              <span>下载源</span>
              <span>
                <el-select v-model="mirror" size="small" class="mirror">
                  <el-option label="默认源（GitHub）" value="default" />
                  <el-option label="国内镜像" value="cn" />
                </el-select>
              </span>
            </div>
          </div>
          <div class="dfoot">
            <el-button link type="primary" @click="safe(ffmpeg.pickPath)">手动指定路径</el-button>
            <span class="sp" />
            <el-button size="default" @click="ffmpeg.dismissPrompt()">稍后</el-button>
            <el-button size="default" type="primary" @click="safe(() => ffmpeg.startInstall(mirror === 'default' ? '' : mirror))">安装</el-button>
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

const ffmpeg = useFFmpegStore()
const mirror = ref('default')
const installing = computed(() => ffmpeg.status.state === 'installing')
const percent = computed(() => Math.round((ffmpeg.install?.progress ?? 0) * 100))
const STAGES = { download: '正在下载', verify: '正在校验', extract: '正在解压', validate: '正在验证' }
const stageText = computed(() => (ffmpeg.install ? STAGES[ffmpeg.install.stage] : '正在准备'))

const ua = navigator.userAgent
const platformText = ua.includes('Mac') ? 'macOS' : ua.includes('Windows') ? 'Windows' : 'Linux'

async function safe(fn: () => Promise<unknown>) {
  try {
    await fn()
  } catch (e: any) {
    ElMessage.error(e?.message || String(e))
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
  margin: 0 0 16px;
  color: var(--ff-text-2);
}
.dinfo {
  background: var(--ff-bg-hover);
  border-radius: var(--ff-radius-md);
  padding: 10px 12px;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 24px;
}
.dinfo div {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.dinfo b {
  color: var(--ff-text-1);
  font-weight: 500;
}
.mirror {
  width: 150px;
}
.progress {
  height: 6px;
  border-radius: 3px;
  background: var(--ff-border);
  overflow: hidden;
}
.progress i {
  display: block;
  height: 100%;
  background: var(--ff-primary);
  border-radius: 3px;
  transition: width var(--ff-dur-base) var(--ff-ease);
}
.pmeta {
  display: flex;
  justify-content: space-between;
  margin: 8px 0 24px;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.dfoot {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sp {
  flex: 1;
}
</style>
