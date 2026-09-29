import { createRouter, createWebHashHistory, START_LOCATION, type RouteRecordRaw } from 'vue-router'
import { routeNeedsFFmpeg } from '@/layout/navigation'
import { useFFmpegStore } from '@/stores/ffmpeg'

const SectionTabs = () => import('../views/sections/SectionTabs.vue')

// 七个一级入口（PRD / 设计规范第 6 节）。有多个子页面的入口用 SectionTabs 渲染页签。
const routes: RouteRecordRaw[] = [
  { path: '/', name: 'Convert', meta: { title: '格式转换', subtitle: '视频 · 音频 · 批量' }, component: () => import('../views/ConvertPage.vue') },
  { path: '/edit', name: 'Edit', meta: { title: '视频剪辑', subtitle: '多轨时间线 · 单工程最多 100 个片段' }, component: () => import('../views/VideoEditor.vue') },
  {
    path: '/live',
    meta: { title: '直播工具', subtitle: '文件推流 · 录屏推流 · 拉流播放' },
    component: () => import('../views/live/LiveLayout.vue'),
    redirect: '/live/push',
    children: [
      { path: 'push', component: () => import('../views/live/FilePush.vue') },
      { path: 'record', component: () => import('../views/live/RecordPush.vue') },
      { path: 'pull', component: () => import('../views/live/PullPlay.vue') },
    ],
  },
  {
    path: '/docs',
    meta: { title: '文档', subtitle: 'Office 转 PDF · PDF 预览', fill: true },
    component: () => import('../views/docs/DocsLayout.vue'), // 分段控件 + KeepAlive + 共用的最近列表（设计说明 2）
    redirect: '/docs/office',
    children: [
      { path: 'office', component: () => import('../views/OfficeConvert.vue') },
      { path: 'pdf', component: () => import('../views/PDFPreview.vue') },
    ],
  },
  {
    path: '/tools',
    meta: { title: '工具', subtitle: 'JSON 格式化与校验', fill: true },
    component: SectionTabs,
    redirect: '/tools/json',
    children: [{ path: 'json', meta: { tab: 'JSON 工具' }, component: () => import('../views/JsonTools.vue') }],
  },
  {
    path: '/tasks',
    name: 'Tasks',
    meta: { title: '任务中心', subtitle: '进度 · 历史 · 失败重试' },
    component: () => import('../views/TaskCenter.vue'),
  },
  // v1 的两个列表页路径不再使用，旧链接落到任务中心
  { path: '/tasks/:pathMatch(.*)*', redirect: '/tasks' },
  {
    path: '/settings',
    meta: { title: '设置', subtitle: '外观 · ffmpeg · 转换' },
    component: () => import('../views/settings/SettingsLayout.vue'),
    redirect: '/settings/general',
    children: [
      { path: 'general', component: () => import('../views/Settings.vue') },
      { path: 'about', component: () => import('../views/About.vue') },
    ],
  },
  // 临时开发预览页：只在 dev 下注册，生产构建里不存在
  ...(import.meta.env.DEV
    ? [{ path: '/dev/components', meta: { title: '组件预览' }, component: () => import('../views/dev/ComponentsPreview.vue') } as RouteRecordRaw]
    : []),
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

// Wails 生产环境没有服务端回退，用 hash 路由避免刷新后 404
const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

// ffmpeg 路由守卫：与侧栏置灰同一套判断（导航里 needsFFmpeg 的一级入口 + ffmpeg store 的 featuresBlocked）。
// ffmpeg 不可用时，直接改地址 / router.push 进入转换、剪辑、直播都会被拦下，行为和点击置灰的侧栏项一致：
//  - 不进入该页面，并重新打开安装对话框（「稍后」不会让入口恢复）；
//  - 应用内导航：留在当前页；地址栏直达（首次导航没有「当前页」）：落到任务中心，那里能看到安装进度。
// 设置、文档、工具、任务中心不依赖 ffmpeg，不受影响。启动时状态还是 checking 不算缺失（与侧栏一致），
// 只有首次导航直达受限页面时才等一下检测结果，否则 #/edit 这类地址在缺失时会漏过去。
router.beforeEach(async (to, from) => {
  if (!routeNeedsFFmpeg(to.matched[0]?.path)) return true
  const ffmpeg = useFFmpegStore()
  const initial = from === START_LOCATION
  // 启动落点 '/' 保持原样：ffmpeg 缺失时转换页自带置灰提示（设计稿 31/32），与侧栏的当前页置灰表现一致
  if (initial && to.path === '/') return true
  if (initial) await ffmpeg.whenSettled()
  if (!ffmpeg.featuresBlocked) return true
  ffmpeg.dialogOpen = true
  return initial ? { path: '/tasks', replace: true } : false
})

export default router
