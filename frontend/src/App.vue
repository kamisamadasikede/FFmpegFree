<template>
  <div class="app-shell">
    <AppSidebar />
    <div class="app-main">
      <AppTitlebar />
      <FFmpegBanner />
      <main class="app-content">
        <RouterView />
      </main>
    </div>
    <FFmpegInstallDialog />
  </div>
</template>

<script lang="ts" setup>
import AppSidebar from './layout/AppSidebar.vue'
import AppTitlebar from './layout/AppTitlebar.vue'
import FFmpegBanner from './components/ffmpeg/FFmpegBanner.vue'
import FFmpegInstallDialog from './components/ffmpeg/FFmpegInstallDialog.vue'
import { useTheme } from './composables/useTheme'
import { useFFmpegStore } from './stores/ffmpeg'
import { useTaskStore } from './stores/tasks'
import { onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { convertV24On, takeInterruptedReconverts } from './api/convertRecords'
import { interruptedReconvertsText } from './utils/convertV24Text'

useTheme()
useFFmpegStore().init()
useTaskStore().init()

// v0.24.1（§八 第 65 条）：启动时取一次上次退出时被中断的重转条数；n > 0 时普通提示 4 秒，只这一次
onMounted(async () => {
  if (!convertV24On()) return
  try {
    const text = interruptedReconvertsText(await takeInterruptedReconverts())
    if (text) ElMessage({ message: text, type: 'info', duration: 4000 })
  } catch (e) {
    console.warn('TakeInterruptedReconverts failed', e)
  }
})
</script>

<style scoped>
.app-shell {
  display: flex;
  height: 100vh;
  min-width: 1024px;
  min-height: 680px;
  overflow: hidden;
}
.app-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.app-content {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 20px 24px 24px;
}
</style>
