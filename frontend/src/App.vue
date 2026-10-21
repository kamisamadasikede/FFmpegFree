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
  <!-- 关窗收尾：后端 beforeClose 先做收尾（最长 16 秒），完成后才真正退出 -->
  <div v-if="closing" class="app-closing" role="status" aria-live="assertive">
    <div class="app-closing-box">
      <span class="app-closing-spin" aria-hidden="true" />
      <p>正在结束任务…</p>
      <small>有直播存档时最多需要 16 秒，请不要强制结束进程。</small>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import AppSidebar from './layout/AppSidebar.vue'
import AppTitlebar from './layout/AppTitlebar.vue'
import FFmpegBanner from './components/ffmpeg/FFmpegBanner.vue'
import FFmpegInstallDialog from './components/ffmpeg/FFmpegInstallDialog.vue'
import { useTheme } from './composables/useTheme'
import { useFFmpegStore } from './stores/ffmpeg'
import { useTaskStore } from './stores/tasks'
import { onMounted, onBeforeUnmount } from 'vue'
import { onEvent } from './services/wails'
import { watchWorkflow } from './views/cat/catWorkflow'
const stopWorkflow = watchWorkflow()
onBeforeUnmount(stopWorkflow)
import { ElMessage } from 'element-plus'
import { convertV24On, takeInterruptedReconverts } from './api/convertRecords'
import { interruptedReconvertsText } from './utils/convertV24Text'

useTheme()
useFFmpegStore().init()
useTaskStore().init()

const route = useRoute()
const blankLayout = computed(() => route.meta.layout === 'blank')
// 关窗收尾提示：后端 beforeClose 发 app:closing，收尾完成后进程退出
const closing = ref(false)
const offClosing = onEvent('app:closing', () => { closing.value = true })
onBeforeUnmount(offClosing)
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
.app-closing {
  position: fixed;
  inset: 0;
  z-index: 4000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(18, 22, 28, 0.55);
  backdrop-filter: blur(2px);
}
.app-closing-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 24px 32px;
  border-radius: 12px;
  background: #fff;
  color: #1f2329;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.28);
  text-align: center;
  max-width: 360px;
}
.app-closing-box p { margin: 0; font-size: 15px; font-weight: 600; }
.app-closing-box small { color: #646a73; font-size: 12px; line-height: 1.5; }
.app-closing-spin {
  width: 22px;
  height: 22px;
  border: 2px solid #d0d3d6;
  border-top-color: #3370ff;
  border-radius: 50%;
  animation: app-closing-rotate 0.8s linear infinite;
}
@keyframes app-closing-rotate { to { transform: rotate(360deg); } }
/* 应用最小尺寸：原来硬写 1024×680，窗口拖不到更窄，响应式永远生效不了。
   v0.58 改为 720×560（与 base.css 的 720 断点对齐），再窄就由 CSS 自己处理溢出。

   注意两点：
   1) 内边距不能再写死 20/24/24 —— 各页（直播 / 转换）用负外边距抵消它，
      写死会和 1240/1024/720 三档变量错位。统一走 --ff-page-pad-*。
   2) min-width 不能也写 720：窗口最小宽 720 时，内容区还要扣掉竖直滚动条
      （约 16px），壳再撑 720 就会横向溢出、整页被裁掉右边一条。
      窗口下限由 main.go 的 MinWidth 保证，这里只保一个更小的兜底值。 */
.app-blank {
  height: 100vh;
  min-height: 560px;
  overflow: hidden;
}
.app-shell {
  display: flex;
  height: 100vh;
  min-height: 560px;
  min-width: 0;
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
  padding: var(--ff-page-pad-y) var(--ff-page-pad-x);
}
.app-content.is-cat {
  padding: 0;
  overflow: hidden;
  display: flex;
}
</style>
