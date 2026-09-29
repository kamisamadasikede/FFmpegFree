<template>
  <div v-if="kind" class="banner" :class="kind">
    <FIcon :name="kind === 'info' ? 'download' : kind === 'ok' ? 'check' : 'warn'" />

    <template v-if="state === 'missing' || state === 'outdated'">
      <span>{{ state === 'missing' ? '未检测到 ffmpeg，转换、剪辑和直播功能暂不可用。' : 'ffmpeg 版本过旧，需要 6.0 或更高版本。' }}</span>
      <span class="sp" />
      <el-button link type="primary" @click="safe(ffmpeg.pickPath)">手动指定</el-button>
      <el-button link type="primary" @click="safe(ffmpeg.recheck)">重新检测</el-button>
      <el-button type="primary" :disabled="!ffmpeg.installAvailable" @click="safe(() => ffmpeg.startInstall())">{{ ffmpeg.installAvailable ? '立即安装' : '安装功能即将上线' }}</el-button>
      <button class="iconbtn" title="本次不再显示" @click="ffmpeg.bannerClosed = true"><FIcon name="x" :size="16" /></button>
    </template>

    <template v-else-if="state === 'installing'">
      <span>正在安装 ffmpeg… {{ stageText }} {{ percent }}%</span>
      <div class="bar"><i :style="{ width: percent + '%' }" /></div>
      <span v-if="ffmpeg.install?.speedText" class="meta">{{ ffmpeg.install.speedText }} · {{ ffmpeg.install.remainText }}</span>
      <span class="sp" />
      <el-button link type="primary" @click="router.push('/tasks')">查看详情</el-button>
    </template>

    <template v-else-if="state === 'failed'">
      <span>ffmpeg 安装失败：{{ ffmpeg.status.error?.message || '未知错误' }}</span>
      <span class="sp" />
      <el-button link type="primary" @click="safe(ffmpeg.pickPath)">手动指定</el-button>
      <el-button link type="primary" @click="ffmpeg.dialogOpen = true">更换下载源</el-button>
      <el-button type="primary" :disabled="!ffmpeg.installAvailable" @click="safe(() => ffmpeg.startInstall())">重试</el-button>
    </template>

    <template v-else-if="kind === 'ok'">
      <span>ffmpeg 已就绪（版本 {{ ffmpeg.status.version }}）</span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { toAppError } from '@/api/call'

const ffmpeg = useFFmpegStore()
const router = useRouter()
const state = computed(() => ffmpeg.status.state)

const kind = computed(() => {
  if (ffmpeg.justBecameReady) return 'ok'
  if (state.value === 'installing') return 'info'
  if (state.value === 'failed') return 'fail'
  if ((state.value === 'missing' || state.value === 'outdated') && !ffmpeg.bannerClosed) return 'warn'
  return ''
})

const percent = computed(() => Math.round((ffmpeg.install?.progress ?? 0) * 100))
const STAGES = { download: '下载中', verify: '校验中', extract: '解压中', validate: '验证中' }
const stageText = computed(() => (ffmpeg.install ? STAGES[ffmpeg.install.stage] : '准备中'))

async function safe(fn: () => Promise<unknown>) {
  try {
    await fn()
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}
</script>

<style scoped>
.banner {
  position: relative;
  height: 36px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px 0 21px;
  border-bottom: 1px solid var(--ff-border);
  font-size: var(--ff-fs-sm);
}
.banner::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
}
.banner.warn { background: var(--ff-warning-soft); }
.banner.warn::before { background: var(--ff-warning); }
.banner.warn > .f-icon { color: var(--ff-warning); }
.banner.info { background: var(--ff-primary-soft); }
.banner.info::before { background: var(--ff-primary); }
.banner.info > .f-icon { color: var(--ff-primary); }
.banner.fail { background: color-mix(in srgb, var(--ff-danger) 10%, var(--ff-bg-surface)); }
.banner.fail::before { background: var(--ff-danger); }
.banner.fail > .f-icon { color: var(--ff-danger); }
.banner.ok { background: color-mix(in srgb, var(--ff-success) 10%, var(--ff-bg-surface)); }
.banner.ok::before { background: var(--ff-success); }
.banner.ok > .f-icon { color: var(--ff-success); }
.sp { flex: 1; }
.meta { color: var(--ff-text-3); font-size: var(--ff-fs-xs); }
.bar {
  width: 120px;
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
.iconbtn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: var(--ff-radius-md);
  display: grid;
  place-items: center;
  color: var(--ff-text-2);
  cursor: pointer;
}
.iconbtn:hover { background: var(--ff-bg-hover); }
</style>
