<template>
  <div class="notice" role="status">
    <FIcon name="warn" :size="16" />
    <span>{{ text ?? '该功能正在迁移到 v2，暂不可用' }}</span>
  </div>
</template>

<script setup lang="ts">
// 固定提示条（不可关闭）：warning 10% 混合底 + 28% 混合边，36px，样式与顶部 ffmpeg 横幅同源、更矮。
// 用于直播页（演示数据）和依赖 v1 接口、尚未迁移的页面（剪辑、Office 转 PDF、PDF 预览）。
import FIcon from '@/components/icon/FIcon.vue'

defineProps<{ text?: string }>()
</script>

<style scoped>
.notice {
  height: 36px;
  flex: none;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border: 1px solid color-mix(in srgb, var(--ff-warning) 28%, var(--ff-bg-surface));
  border-radius: var(--ff-radius-md);
  background: color-mix(in srgb, var(--ff-warning) 10%, var(--ff-bg-surface));
  color: var(--ff-text-1);
  font-size: var(--ff-fs-sm);
}
.notice > :first-child {
  flex: none;
  color: var(--ff-warning-text);
}
</style>
