<template>
  <!-- 登录等 blank 布局：无侧栏 / 顶栏，全屏；默认 LOGIN_UI_ENABLED=false 时路由会拦到 /，此处不会落到登录 -->
  <div v-if="blankLayout" class="app-blank">
    <RouterView />
  </div>
  <div v-else class="app-shell">
    <AppSidebar />
    <div class="app-main">
      <!-- Cat 页（layout=cat）：没有顶栏和转换组件横条，内容区铺满（原型 cat-v3） -->
      <AppTitlebar v-if="!catLayout" />
      <FFmpegBanner v-if="!catLayout" />
      <main class="app-content" :class="{ 'is-cat': catLayout }">
        <!-- 直播页切到别的菜单再回来：整页留着（推流 / 拉流不中断），预览播放器自己在离开时拆掉 -->
        <RouterView v-slot="{ Component }">
          <KeepAlive include="LiveLayout">
            <component :is="Component" />
          </KeepAlive>
        </RouterView>
      </main>
    </div>
    <FFmpegInstallDialog />
  </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
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

const route = useRoute()
const blankLayout = computed(() => route.meta.layout === 'blank')
const catLayout = computed(() => route.meta.layout === 'cat')

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
.app-blank {
  height: 100vh;
  min-width: 1024px;
  min-height: 680px;
  overflow: hidden;
}
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
.app-content.is-cat {
  padding: 0;
  overflow: hidden;
  display: flex;
}
</style>
