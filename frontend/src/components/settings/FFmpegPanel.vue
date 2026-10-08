<template>
  <section class="panel group" :aria-labelledby="headingId">
    <div class="phead">
      <h2 :id="headingId">转换组件</h2>
      <span v-if="!isReady && !ffmpeg.customBroken" class="tag" :class="view.tone">{{ view.tag }}</span>
    </div>
    <!-- X5（产品经理 10-08）：不显示组件路径（路径里是可执行文件名）；就绪时两行：绿点 + 已就绪、版本，右边「打开组件所在文件夹」（设置页才有，插槽 #ready-actions） -->
    <div v-if="isReady" class="srow fcomp">
      <div class="l">
        <span class="fok"><i class="fdot" aria-hidden="true" />转换组件已就绪</span>
        <small>{{ view.detail }}</small>
        <!-- 包 24 N5：手动指定的不可用、已回退到默认组件（12 号警告色 + 小图标） -->
        <small v-if="ffmpeg.customFellBack" class="fwarn"><FIcon name="warn" :size="12" />手动指定的转换组件不可用，已改用默认组件。</small>
      </div>
      <slot name="ready-actions" />
    </div>
    <!-- 包 24 N5：手动指定的不可用，也没有别的能用的组件：警告色圆点 +「转换组件未就绪」，右边「手动指定」「恢复默认」（插槽 #custom-actions） -->
    <div v-else-if="ffmpeg.customBroken" class="srow fcomp">
      <div class="l">
        <span class="fok"><i class="fdot bad" aria-hidden="true" />转换组件未就绪</span>
        <small>手动指定的转换组件不可用。</small>
      </div>
      <slot name="custom-actions" />
    </div>
    <div v-else class="srow">
      <div class="l">
        <b>当前版本</b>
        <small>{{ view.detail }}</small>
      </div>
      <slot name="version-actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
// 转换组件面板：设置页与关于页共用。就绪时「转换组件已就绪 / 转换组件版本 x」两行；其它状态是状态标签 + 「当前版本」行。
// 不显示组件路径（X5）。关于页只读：不传 #ready-actions / #version-actions 插槽，并传 readonly。
// 数据取自 ffmpeg store（与顶部提示条同源），状态变化（安装完成、手动指定）自动刷新。
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { FFPROBE_MISSING_TEXT, userVisibleMessage } from '@/errors/errorMessages'

defineProps<{ headingId?: string; readonly?: boolean }>()

const ffmpeg = useFFmpegStore()
const SOURCE: Record<string, string> = { bundled: '应用目录', system: '系统环境', custom: '手动指定', legacy: '旧版目录' }

const view = computed(() => {
  const s = ffmpeg.status
  const from = SOURCE[s.source ?? '']
  switch (s.state) {
    case 'ready':
      return { tag: '已就绪', tone: 'ok', detail: s.ffprobeMissing ? FFPROBE_MISSING_TEXT : s.version ? `转换组件版本 ${s.version}` : from ? `转换组件来自${from}` : '已检测到转换组件' }
    case 'installing':
      return { tag: '安装中', tone: 'run', detail: ffmpeg.install ? `下载中 ${Math.round(ffmpeg.install.progress * 100)}%` : '准备中' }
    case 'failed':
      return { tag: '安装失败', tone: 'fail', detail: userVisibleMessage(s.error?.message) || '安装没有成功，可以重试或手动指定位置。' }
    case 'outdated':
      return { tag: '版本过旧', tone: 'warn', detail: `${s.version ? `当前转换组件版本 ${s.version}，` : ''}需要 6.0 或更高版本。` }
    case 'missing':
      return { tag: '未安装', tone: 'warn', detail: '转换和直播暂不可用。' }
    default:
      return { tag: '检测中', tone: 'q', detail: '正在检测转换组件，请稍候。' }
  }
})

const isReady = computed(() => ffmpeg.status.state === 'ready')
</script>

<style scoped>
.fcomp { align-items: flex-start; }
.fok { display: flex; align-items: center; gap: 8px; font-size: 13px; line-height: 20px; font-weight: 500; color: var(--ff-text-1); }
.fdot { width: 8px; height: 8px; border-radius: 50%; background: var(--ff-success); flex: none; }
.fcomp .l small { font-size: 12px; line-height: 18px; color: var(--ff-text-2); margin-top: 2px; }
.fdot.bad { background: var(--ff-warning); }
.fcomp .l small.fwarn { display: flex; align-items: center; gap: 4px; color: var(--ff-warning-text); margin-top: 4px; }
.fwarn :deep(svg) { flex: none; }
/* 按钮和第一行垂直居中：第一行高 20，按钮高 28 */
.fcomp :slotted(.btn) { margin-top: -4px; }
.fcomp :slotted(.facts) { display: flex; gap: 8px; flex: none; margin-top: -4px; }
</style>
