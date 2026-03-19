
import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router';

const routes: Array<RouteRecordRaw> = [
  { path: '/', name: 'Home', component: () => import('../views/Home.vue') },
  { path: '/about', name: 'About', component: () => import('../views/About.vue') },
  { path: '/convert', name: 'Convert', component: () => import('../views/convert.vue') },
  { path: '/convertup', name: 'Convertup', component: () => import('../views/convertup.vue') },
  { path: '/steamup', name: 'steamup', component: () => import('../views/steamup.vue') },
  { path: '/steamlist', name: 'steamlist', component: () => import('../views/steamlist.vue') },
  { path: '/MediaRecorder', name: 'MediaRecorder', component: () => import('../views/MediaRecorder.vue') },
  { path: '/LivePlayer', name: 'LivePlayer', component: () => import('../views/LivePlayer.vue') },
  { path: '/live-ops', name: 'LiveOps', component: () => import('../views/LiveOps.vue') },
  { path: '/pdf-preview', name: 'PDFPreview', component: () => import('../views/PDFPreview.vue') },
  { path: '/office-convert', name: 'OfficeConvert', component: () => import('../views/OfficeConvert.vue') },
  { path: '/json-tools', name: 'JsonTools', component: () => import('../views/JsonTools.vue') },
  { path: '/openclaw-install', name: 'OpenClawInstall', component: () => import('../views/OpenClawInstall.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
