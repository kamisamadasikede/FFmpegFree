<template>
  <div class="ffst" :class="{ miss: ffmpeg.status.state === 'missing' }" :title="ffmpeg.status.path || undefined">
    <b><i :style="{ background: view.color }" />{{ view.title }}</b>
    <el-button v-if="ffmpeg.status.state === 'missing'" link type="primary" class="act" @click="ffmpeg.dialogOpen = true">安装</el-button>
    <span class="sub">{{ view.sub }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useFFmpegStore } from '@/stores/ffmpeg'

const ffmpeg = useFFmpegStore()
const SOURCE: Record<string, string> = { bundled: '应用目录', system: '系统环境', custom: '手动指定', legacy: '旧版目录' }

const view = computed(() => {
  const s = ffmpeg.status
  switch (s.state) {
    case 'ready':
      return { color: 'var(--ff-success)', title: 'ffmpeg 已就绪', sub: [s.version, SOURCE[s.source ?? '']].filter(Boolean).join(' · ') }
    case 'installing':
      return {
        color: 'var(--ff-primary)',
        title: '正在安装 ffmpeg',
        sub: ffmpeg.install ? `下载中 ${Math.round(ffmpeg.install.progress * 100)}%` : '准备中',
      }
    case 'failed':
      return { color: 'var(--ff-danger)', title: 'ffmpeg 安装失败', sub: '点击顶部提示条重试' }
    case 'outdated':
      return { color: 'var(--ff-warning)', title: 'ffmpeg 版本过旧', sub: '需要 6.0 或更高版本' }
    case 'missing':
      return { color: 'var(--ff-warning)', title: 'ffmpeg 未安装', sub: '转换、剪辑、直播暂不可用' }
    default:
      return { color: 'var(--ff-text-3)', title: '正在检测 ffmpeg', sub: '请稍候' }
  }
})
</script>

<style scoped>
.ffst {
  margin: 0 2px 8px;
  padding: 8px 12px;
  border: 1px solid var(--ff-border);
  border-radius: 8px;
  background: var(--ff-bg-surface);
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.ffst b {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--ff-text-1);
  font-weight: 500;
  margin-bottom: 4px;
}
.ffst i {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: block;
}
/* 缺失状态（原型 .ffst.miss）：标题 + 右侧文字按钮一行，副文案独占一整行，整体高 58px */
.ffst.miss {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: center;
  column-gap: 8px;
  padding: 8px 8px 8px 12px;
}
.ffst.miss b {
  margin-bottom: 0;
}
.ffst.miss .sub {
  grid-column: 1 / -1;
}
.ffst.miss .act {
  height: 24px;
  padding: 0 8px;
  font-size: var(--ff-fs-xs);
  --el-button-text-color: var(--ff-primary-text);
}
</style>
