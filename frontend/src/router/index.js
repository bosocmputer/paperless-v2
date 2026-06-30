import AppLayout from '@/layout/AppLayout.vue';
import SignerLayout from '@/layout/SignerLayout.vue';
import { authStore } from '@/stores/auth';
import { createRouter, createWebHistory } from 'vue-router';

const router = createRouter({
    history: createWebHistory(),
    routes: [
        {
            path: '/signing',
            component: SignerLayout,
            children: [
                {
                    path: 'tasks',
                    name: 'my-signing-tasks',
                    component: () => import('@/views/signing/MySigningTasks.vue'),
                    meta: { role: 'user' }
                },
                {
                    path: 'tasks/:taskId',
                    name: 'my-signing-task',
                    component: () => import('@/views/signing/SigningTask.vue'),
                    meta: { role: 'user' }
                }
            ]
        },
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
                    path: '/config/documents/:docFormatCode/workflow',
                    name: 'document-config-workflow',
                    component: () => import('@/views/config/DocumentConfigWorkflow.vue'),
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
                },
                {
                    path: '/signing/documents',
                    name: 'signing-documents',
                    component: () => import('@/views/signing/SigningDocuments.vue'),
                    meta: { role: 'admin' }
                },
                {
                    path: '/signing/documents/new',
                    name: 'signing-document-new',
                    component: () => import('@/views/signing/SigningDocumentCreate.vue'),
                    meta: { role: 'admin' }
                },
                {
                    path: '/signing/documents/:id',
                    name: 'signing-document-detail',
                    component: () => import('@/views/signing/SigningDocumentDetail.vue'),
                    meta: { role: 'admin' }
                }
            ]
        },
        {
            path: '/external/sign/:token',
            name: 'public-signing',
            component: () => import('@/views/signing/PublicSigning.vue'),
            meta: { public: true }
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
    if (to.name === 'public-signing') return true;

    if (to.meta.public) {
        if (authStore.isAuthenticated()) return authStore.user?.role === 'user' ? { name: 'my-signing-tasks' } : { name: 'dashboard' };
        return true;
    }

    if (!authStore.isAuthenticated()) return { name: 'login', query: { redirect: to.fullPath } };

    if (!authStore.user) {
        try {
            await authStore.loadMe();
        } catch {
            authStore.clear();
            return { name: 'login' };
        }
    }

    if (to.name === 'dashboard' && authStore.user?.role === 'user') {
        return { name: 'my-signing-tasks' };
    }

    if (to.meta.role && authStore.user?.role !== to.meta.role) {
        return authStore.user?.role === 'user' ? { name: 'my-signing-tasks' } : { name: 'dashboard' };
    }

    return true;
});

export default router;
