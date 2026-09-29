<template>
  <div class="settings">
    <section class="group">
      <h3 class="group-title">外观</h3>
      <div class="row">
        <div>
          <div class="row-label">主题</div>
          <div class="row-desc">跟随系统时会随操作系统的浅色和暗色设置自动切换。</div>
        </div>
        <el-radio-group v-model="mode">
          <el-radio-button value="light">浅色</el-radio-button>
          <el-radio-button value="dark">暗色</el-radio-button>
          <el-radio-button value="system">跟随系统</el-radio-button>
        </el-radio-group>
      </div>
    </section>
    <!-- 输出目录、并发数、ffmpeg 路径、语言等在接上 SettingsService 后补充 -->
  </div>
</template>

<script setup lang="ts">
import { useTheme } from '@/composables/useTheme'

const { mode } = useTheme()
</script>

<style scoped>
.settings {
  max-width: 720px;
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
}
.group {
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-lg);
  padding: var(--ff-space-4);
}
.group-title {
  margin: 0 0 var(--ff-space-3);
  font-size: var(--ff-fs-lg);
  font-weight: 600;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ff-space-4);
}
.row-label {
  font-weight: 500;
}
.row-desc {
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
}
</style>
