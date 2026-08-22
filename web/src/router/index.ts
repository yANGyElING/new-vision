import { createRouter, createWebHistory } from 'vue-router'
import ConsoleView from '@/views/ConsoleView.vue'
import DevicesView from '@/views/DevicesView.vue'
import IdentityView from '@/views/IdentityView.vue'
import LoginView from '@/views/LoginView.vue'
import NotFoundView from '@/views/NotFoundView.vue'
import { isAuthenticated } from '@/api/http'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: ConsoleView, meta: { requiresAuth: true } },
    { path: '/devices', component: DevicesView, meta: { requiresAuth: true } },
    { path: '/identity', component: IdentityView, meta: { requiresAuth: true } },
    { path: '/login', component: LoginView },
    { path: '/:pathMatch(.*)*', component: NotFoundView },
  ],
})

router.beforeEach((to) => {
  if (to.meta.requiresAuth && !isAuthenticated()) {
    return { path: '/login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : {} }
  }
  if (to.path === '/login' && isAuthenticated()) {
    return { path: '/devices' }
  }
  return true
})
