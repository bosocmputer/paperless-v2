import { createRouter, createWebHistory } from 'vue-router';
import { authStore } from '@/stores/auth';
import AppLayout from '@/layout/AppLayout.vue';

const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { public: true }
  },
  {
    path: '/',
    component: AppLayout,
    children: [
      {
        path: '',
        name: 'dashboard',
        component: () => import('@/views/DashboardView.vue')
      }
    ]
  }
];

const router = createRouter({
  history: createWebHistory(),
  routes
});

router.beforeEach(async (to) => {
  if (to.meta.public) {
    if (authStore.isAuthenticated()) {
      return { name: 'dashboard' };
    }
    return true;
  }

  if (!authStore.isAuthenticated()) {
    return { name: 'login' };
  }

  if (!authStore.user) {
    try {
      await authStore.loadMe();
    } catch {
      authStore.clear();
      return { name: 'login' };
    }
  }

  return true;
});

export default router;

