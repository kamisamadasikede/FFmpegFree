<template>
  <!-- 侧栏左下角的 ffmpeg 状态（设计说明 编码设备-v0.1 第一节 + 设计师定稿）：整块外层 role="status"，一个状态点加一句 13px 的话。
       版本、来源、路径、手动指定入口都在设置页的 ffmpeg 区域。 -->
  <div class="ffst" :class="[view.tone, { collapsed }]" role="status" :aria-label="view.label" :title="view.label" :data-tip="collapsed ? view.label : view.text">
    <button v-if="view.clickable" type="button" class="ffr" :aria-label="view.actionLabel" :title="view.actionLabel" @click="ffmpeg.dialogOpen = true">
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
import { ffmpegStatusView } from './statusView'

defineProps<{ collapsed?: boolean }>()

const ffmpeg = useFFmpegStore()

/**
 * 四种表达：已就绪 / 检测中（启动瞬间，中性灰点，不可点）/ 未就绪（缺失、过旧、失败）/ 安装中。
 * 未就绪和安装中整行可点，都是打开安装对话框（安装中的对话框里已有进度，不跳任务中心）；#53 的 dialogVisible 逻辑不动。
 */
const view = computed(() => ffmpegStatusView(ffmpeg.status.state))
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
}
button.ffr {
  cursor: pointer;
}
button.ffr:hover {
  background: var(--ff-bg-hover);
}
button.ffr:focus-visible {
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
