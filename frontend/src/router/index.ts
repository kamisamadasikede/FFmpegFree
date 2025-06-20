
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
  { path: '/steamup',
    name: 'steamup',
    component:() => import('../views/steamup.vue')
  },
  { path: '/steamlist',
    name: 'steamlist',
    component:() => import('../views/steamlist.vue')
  },
  { path: '/MediaRecorder',
    name: 'MediaRecorder',
    component:() => import('../views/MediaRecorder.vue')
  },
  { path: '/LivePlayer',
    name: 'LivePlayer',
    component:() => import('../views/LivePlayer.vue')
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router