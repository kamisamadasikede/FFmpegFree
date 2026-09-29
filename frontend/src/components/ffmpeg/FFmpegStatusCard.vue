<template>
  <!-- 侧栏左下角的 ffmpeg 状态（设计说明 编码设备-v0.1 第一节）：整块外层 role="status"，一个 8px 状态点加一句 13px 的话。
       版本、来源、路径、手动指定入口都在设置页的 ffmpeg 区域。 -->
  <div class="ffst" :class="[view.tone, { collapsed }]" role="status" :aria-label="view.label" :data-tip="view.text">
    <button v-if="clickable" type="button" class="ffr" :aria-label="`${view.label}，点击打开安装对话框`" @click="ffmpeg.dialogOpen = true">
      <i class="ffd" aria-hidden="true" />
      <span v-if="!collapsed" class="fft">{{ view.text }}</span>
    </button>
    <span v-else class="ffr">
      <i class="ffd" aria-hidden="true" />
      <span v-if="!collapsed" class="fft">{{ view.text }}</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useFFmpegStore } from '@/stores/ffmpeg'

defineProps<{ collapsed?: boolean }>()

const ffmpeg = useFFmpegStore()

/**
 * 只有三种表达：已就绪 / 未就绪 / 安装中。
 * 缺失、过旧、安装失败都归为“未就绪”（点开安装对话框后在对话框里看失败原因）。
 * 检测中（启动瞬间）设计稿没有这一态：仍显示“未就绪”，但用中性色且不可点，避免启动瞬间闪一下警告色。
 */
const view = computed(() => {
  switch (ffmpeg.status.state) {
    case 'ready':
      return { tone: 'ok', text: 'ffmpeg 已就绪', label: 'ffmpeg 已就绪' }
    case 'installing':
      return { tone: 'run', text: 'ffmpeg 安装中…', label: 'ffmpeg 安装中…' }
    case 'checking':
      return { tone: 'q', text: 'ffmpeg 未就绪', label: 'ffmpeg 未就绪' }
    default:
      return { tone: 'warn', text: 'ffmpeg 未就绪', label: 'ffmpeg 未就绪' }
  }
})

// 未就绪（缺失、过旧、失败）时整行可点，打开安装对话框；已就绪、检测中、安装中不可点（安装进度看顶部提示条 / 任务中心）
const clickable = computed(() => view.value.tone === 'warn')
</script>

<style scoped>
.ffst {
  position: relative;
  height: 32px;
  margin: 0 0 8px;
  font-size: 13px;
  line-height: 16px;
  --wails-draggable: no-drag;
}
.ffr {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 32px;
  box-sizing: border-box;
  padding: 8px 12px;
  border-radius: 6px;
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 13px;
  line-height: 16px;
  color: var(--ff-text-2);
  text-align: left;
  white-space: nowrap;
}
.fft {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ffd {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ff-success);
}
.q .ffd {
  background: var(--ff-text-3);
}
.warn .ffd {
  background: var(--ff-warning);
}
.warn .ffr {
  color: var(--ff-warning-text);
  cursor: pointer;
}
.warn .ffr:hover {
  background: var(--ff-bg-hover);
}
.warn .ffr:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.run .ffd {
  box-sizing: border-box; /* 12px 含 2px 描边 */
  width: 12px;
  height: 12px;
  background: transparent;
  border: 2px solid color-mix(in srgb, var(--ff-primary) 24%, transparent);
  border-top-color: var(--ff-primary);
  animation: ffspin 1s linear infinite;
}
@keyframes ffspin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .run .ffd {
    animation: none; /* 转圈不转，仍是一个环 */
  }
}
/* 折叠（侧栏 56px）：只留圆点（安装中为转圈），hover / 键盘聚焦显示气泡，文字同状态文字；气泡在侧栏外右侧，不撑宽侧栏 */
.ffst.collapsed .ffr {
  justify-content: center;
  padding: 0;
  gap: 0;
}
.ffst::after {
  content: attr(data-tip);
  position: absolute;
  left: calc(100% + 4px);
  top: 50%;
  transform: translateY(-50%);
  z-index: 20;
  white-space: nowrap;
  pointer-events: none;
  height: 28px;
  padding: 0 8px;
  border-radius: 6px;
  display: none;
  align-items: center;
  font-size: 12px;
  font-weight: 400;
  line-height: 16px;
  color: var(--ff-text-1);
  background: var(--ff-bg-elevated);
  border: 1px solid var(--ff-border);
  box-shadow: var(--ff-shadow-dialog);
  opacity: 0;
  transition: opacity 0.12s ease 0s;
}
.ffst.collapsed::after {
  display: flex;
}
.ffst.collapsed:hover::after,
.ffst.collapsed:focus-within::after {
  opacity: 1;
  transition-delay: 0.3s;
}
</style>
