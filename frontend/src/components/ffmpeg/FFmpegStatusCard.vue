<template>
  <!-- 侧栏左下角的 ffmpeg 状态：一个状态点加一句话。版本、来源、路径、手动指定入口都在设置页的 ffmpeg 区域 -->
  <button
    v-if="clickable"
    type="button"
    class="ffst"
    :class="[view.tone, { collapsed }]"
    :title="view.title"
    :aria-label="view.label"
    @click="ffmpeg.dialogOpen = true"
  >
    <i class="dot" aria-hidden="true" />
    <span v-if="!collapsed" class="txt">{{ view.text }}</span>
  </button>
  <div v-else class="ffst" :class="[view.tone, { collapsed }]" role="status" :title="view.title" :aria-label="view.label">
    <i class="dot" aria-hidden="true" />
    <span v-if="!collapsed" class="txt">{{ view.text }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useFFmpegStore } from '@/stores/ffmpeg'

defineProps<{ collapsed?: boolean }>()

const ffmpeg = useFFmpegStore()

/**
 * 只有三种表达：已就绪 / 未就绪 / 安装中。
 * 检测中（启动瞬间）、缺失、过旧、安装失败都归为“未就绪”；检测中用中性色，不当成需要处理的问题（点击也不开对话框）。
 */
const view = computed(() => {
  switch (ffmpeg.status.state) {
    case 'ready':
      return { tone: 'ok', text: 'ffmpeg 已就绪', title: 'ffmpeg 已就绪', label: 'ffmpeg 已就绪' }
    case 'installing':
      return { tone: 'run', text: 'ffmpeg 安装中…', title: 'ffmpeg 安装中…，点击查看进度', label: 'ffmpeg 安装中…' }
    case 'checking':
      return { tone: 'q', text: 'ffmpeg 未就绪', title: 'ffmpeg 未就绪', label: 'ffmpeg 未就绪' }
    default:
      return { tone: 'warn', text: 'ffmpeg 未就绪', title: 'ffmpeg 未就绪，点击安装', label: 'ffmpeg 未就绪' }
  }
})

// 未就绪（含安装中、失败、过旧）时点击打开安装对话框；已就绪和检测中没有可做的事
const clickable = computed(() => ffmpeg.needsAttention)
</script>

<style scoped>
.ffst {
  height: 36px;
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px; /* 与 .nav-item 一致 */
  padding: 0 10px;
  border: none;
  border-radius: var(--ff-radius-md);
  background: transparent;
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
  font-family: inherit;
  white-space: nowrap;
  text-align: left;
  --wails-draggable: no-drag;
}
button.ffst {
  cursor: pointer;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
button.ffst:hover {
  background: var(--ff-bg-hover);
}
button.ffst:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
}
.ffst.collapsed {
  justify-content: center;
  padding: 0;
}
.txt {
  overflow: hidden;
  text-overflow: ellipsis;
}
.dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ff-text-3);
}
.ok .dot {
  background: var(--ff-success);
}
.warn .dot {
  background: var(--ff-warning);
}
.run .dot {
  background: var(--ff-primary);
}
</style>
