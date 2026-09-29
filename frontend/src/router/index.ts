import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'

const SectionTabs = () => import('../views/sections/SectionTabs.vue')

// 七个一级入口（PRD / 设计规范第 6 节）。有多个子页面的入口用 SectionTabs 渲染页签。
const routes: RouteRecordRaw[] = [
  { path: '/', name: 'Convert', meta: { title: '格式转换', subtitle: '视频 · 音频 · 批量' }, component: () => import('../views/Home.vue') },
  { path: '/edit', name: 'Edit', meta: { title: '视频剪辑', subtitle: '多轨时间线 · 转场 · 调色' }, component: () => import('../views/VideoEditor.vue') },
  {
    path: '/live',
    meta: { title: '直播工具', subtitle: '推流 · 录屏 · 拉流' },
    component: SectionTabs,
    redirect: '/live/push',
    children: [
      { path: 'push', meta: { tab: '文件推流' }, component: () => import('../views/steamup.vue') },
      { path: 'streaming', meta: { tab: '正在推流' }, component: () => import('../views/steamlist.vue') },
      { path: 'record', meta: { tab: '录屏推流' }, component: () => import('../views/MediaRecorder.vue') },
      { path: 'player', meta: { tab: 'FLV 拉流' }, component: () => import('../views/LivePlayer.vue') },
      { path: 'ops', meta: { tab: '运维面板' }, component: () => import('../views/LiveOps.vue') },
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
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

// Wails 生产环境没有服务端回退，用 hash 路由避免刷新后 404
const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

export default router
