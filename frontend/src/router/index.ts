import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'

const SectionTabs = () => import('../views/sections/SectionTabs.vue')

// 七个一级入口（PRD / 设计规范第 6 节）。有多个子页面的入口用 SectionTabs 渲染页签。
const routes: RouteRecordRaw[] = [
  { path: '/', name: 'Convert', meta: { title: '格式转换', subtitle: '视频 · 音频 · 批量' }, component: () => import('../views/Home.vue') },
  { path: '/edit', name: 'Edit', meta: { title: '视频剪辑', subtitle: '多轨时间线 · 转场 · 调色' }, component: () => import('../views/VideoEditor.vue') },
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
    meta: { title: '文档', subtitle: 'Office 转 PDF · PDF 预览' },
    component: SectionTabs,
    redirect: '/docs/office',
    children: [
      { path: 'office', meta: { tab: 'Office 转 PDF' }, component: () => import('../views/OfficeConvert.vue') },
      { path: 'pdf', meta: { tab: 'PDF 预览' }, component: () => import('../views/PDFPreview.vue') },
    ],
  },
  {
    path: '/tools',
    meta: { title: '工具', subtitle: 'JSON 格式化 · 对比 · 校验' },
    component: SectionTabs,
    redirect: '/tools/json',
    children: [{ path: 'json', meta: { tab: 'JSON 工具' }, component: () => import('../views/JsonTools.vue') }],
  },
  {
    path: '/tasks',
    meta: { title: '任务中心', subtitle: '进度 · 历史 · 失败重试' },
    component: SectionTabs,
    redirect: '/tasks/running',
    // 过渡期沿用 v1 的两个列表页，任务 store 接上后换成统一的任务中心
    children: [
      { path: 'running', meta: { tab: '进行中' }, component: () => import('../views/convert.vue') },
      { path: 'done', meta: { tab: '已完成' }, component: () => import('../views/convertup.vue') },
    ],
  },
  {
    path: '/settings',
    meta: { title: '设置' },
    component: SectionTabs,
    redirect: '/settings/general',
    children: [
      { path: 'general', meta: { tab: '通用' }, component: () => import('../views/Settings.vue') },
      { path: 'about', meta: { tab: '关于' }, component: () => import('../views/About.vue') },
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

export default router
