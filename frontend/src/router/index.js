import AppLayout from '@/layout/AppLayout.vue';
import { authStore } from '@/stores/auth';
import { createRouter, createWebHistory } from 'vue-router';

const router = createRouter({
    history: createWebHistory(),
    routes: [
        {
            path: '/',
            component: AppLayout,
            children: [
                {
                    path: '/',
                    name: 'dashboard',
                    component: () => import('@/views/Dashboard.vue')
                },
                {
                    path: '/pages/empty',
                    name: 'empty',
                    component: () => import('@/views/pages/Empty.vue')
                },
                {
                    path: '/admin/users',
                    name: 'users',
                    component: () => import('@/views/admin/Users.vue'),
                    meta: { role: 'admin' }
                },
                {
                    path: '/config/documents',
                    name: 'document-config',
                    component: () => import('@/views/config/DocumentConfig.vue'),
                    meta: { role: 'admin' }
                },
                {
                    path: '/config/signature-templates',
                    name: 'signature-templates',
                    component: () => import('@/views/config/SignatureTemplateList.vue'),
                    meta: { role: 'admin' }
                },
                {
                    path: '/config/documents/:docFormatCode/signature-template',
                    name: 'signature-template',
                    component: () => import('@/views/config/SignatureTemplateDesigner.vue'),
                    meta: { role: 'admin' }
                }
            ]
        },
        {
            path: '/auth/login',
            name: 'login',
            component: () => import('@/views/pages/auth/Login.vue'),
            meta: { public: true }
        },
        {
            path: '/login',
            redirect: '/auth/login'
        },
        {
            path: '/:pathMatch(.*)*',
            name: 'notfound',
            component: () => import('@/views/pages/NotFound.vue')
        }
    ]
});

router.beforeEach(async (to) => {
    if (to.meta.public) {
        if (authStore.isAuthenticated()) return { name: 'dashboard' };
        return true;
    }

    if (!authStore.isAuthenticated()) return { name: 'login' };

    if (!authStore.user) {
        try {
            await authStore.loadMe();
        } catch {
            authStore.clear();
            return { name: 'login' };
        }
    }

    if (to.meta.role && authStore.user?.role !== to.meta.role) {
        return { name: 'dashboard' };
    }

    return true;
});

export default router;
