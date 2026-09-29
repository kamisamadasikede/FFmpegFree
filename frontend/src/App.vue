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

useTheme()
useFFmpegStore().init()
useTaskStore().init()
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
