import { createRouter, createWebHashHistory, START_LOCATION, type RouteRecordRaw } from 'vue-router'
import { routeNeedsFFmpeg } from '@/layout/navigation'
import { useFFmpegStore } from '@/stores/ffmpeg'
import { LOGIN_UI_ENABLED } from '@/api/flags'

const SectionTabs = () => import('../views/sections/SectionTabs.vue')
import LiveLayout from '../views/live/LiveLayout.vue'

// 七个一级入口（PRD / 设计规范第 6 节）。有多个子页面的入口用 SectionTabs 渲染页签。
const routes: RouteRecordRaw[] = [
  { path: '/', name: 'Convert', meta: { title: '格式转换', subtitle: '视频 · 音频 · 转换记录' }, component: () => import('../views/ConvertPage.vue') },
  {
    path: '/voice',
    name: 'Voice',
    meta: { title: '语音工具', subtitle: '转字幕', fill: true },
    component: () => import('../views/voice/VoicePage.vue'),
  },
  // 剪辑功能已移除（老板决定，2026-10-08，应用只做转换）：旧的 #/edit 地址一律回到转换页
  { path: '/edit/:pathMatch(.*)*', redirect: '/' },
  {
    path: '/live',
    meta: { title: '直播工具', subtitle: '文件推流 · 录屏推流 · 拉流播放' },
    component: LiveLayout, // 同步引入：KeepAlive 的 include 要靠组件名 LiveLayout，离开直播菜单时整页留着
    redirect: '/live/push',
    children: [
      { path: 'push', component: () => import('../views/live/FilePush.vue') },
      { path: 'record', component: () => import('../views/live/RecordPush.vue') },
      { path: 'pull', component: () => import('../views/live/PullPlay.vue') },
    ],
  },
  {
    path: '/docs',
    meta: { title: '文档', subtitle: '文档转换 · PDF 预览', fill: true },
    component: () => import('../views/docs/DocsLayout.vue'), // 分段控件 + KeepAlive + 共用的最近列表（设计说明 2）
    redirect: '/docs/office',
    children: [
      // v0.26：文档多格式转换（旧的 Office 转 PDF 页 OfficeConvert.vue 不再挂路由，旧记录在新页面里照常显示）
      { path: 'office', meta: { title: '文档转换', subtitle: '文档 · 表格 · 演示 · 转换记录' }, component: () => import('../views/DocConvertPage.vue') },
      { path: 'pdf', component: () => import('../views/PDFPreview.vue') },
    ],
  },
  {
    path: '/tools',
    meta: { title: 'JSON工具', subtitle: 'JSON 格式化与校验', fill: true },
    component: SectionTabs,
    redirect: '/tools/json',
    children: [{ path: 'json', meta: { tab: 'JSON工具' }, component: () => import('../views/JsonTools.vue') }],
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
    meta: { title: '设置', subtitle: '外观 · 转换组件 · 语音识别组件 · 文档组件 · 转换' },
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
  // 登录页 UI 预留：仅 LOGIN_UI_ENABLED=true 可达；默认 false 时直达 /login 也回到转换页。不进侧栏、不作启动页。
  {
    path: '/login',
    name: 'Login',
    meta: { title: '登录', layout: 'blank' },
    component: () => import('../views/LoginView.vue'),
    beforeEnter: () => (LOGIN_UI_ENABLED ? true : { path: '/', replace: true }),
  },
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
// 设置、文档、JSON工具、任务中心不依赖 ffmpeg，不受影响。启动时状态还是 checking 不算缺失（与侧栏一致），
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
