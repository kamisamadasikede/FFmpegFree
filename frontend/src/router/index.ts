
import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router';

const routes: Array<RouteRecordRaw> = [
  {
    path: '/',
    name: 'Home',
    component: () => import('../views/Home.vue')
  },
  {
    path: '/about',
    name: 'About',
    component: () => import('../views/About.vue')
  },
  { path: '/convert',
    name: 'Convert',
    component:() => import('../views/Convert.vue')
  },
  { path: '/convertup',
    name: 'Convertup',
    component:() => import('../views/convertup.vue')
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router