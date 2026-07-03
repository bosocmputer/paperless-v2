import AppLayout from '@/layout/AppLayout.vue';
import SignerLayout from '@/layout/SignerLayout.vue';
import { authStore } from '@/stores/auth';
import { SIGNING_DOCUMENT_MENU_KEYS } from '@/utils/signingQueue';
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
                    meta: { role: 'user', compactSignerLayout: true }
                },
                {
                    path: 'history',
                    name: 'my-signing-history',
                    component: () => import('@/views/signing/MySigningHistory.vue'),
                    meta: { role: 'user' }
                },
                {
                    path: 'guide',
                    name: 'my-signing-guide',
                    component: () => import('@/views/signing/UserGuide.vue'),
                    meta: { role: 'user' }
                },
                {
                    path: 'history/:taskId',
                    name: 'my-signing-history-detail',
                    component: () => import('@/views/signing/SigningHistoryDetail.vue'),
                    meta: { role: 'user', compactSignerLayout: true }
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
                    path: '/admin/guide',
                    name: 'admin-guide',
                    component: () => import('@/views/admin/AdminGuide.vue'),
                    meta: { role: 'admin' }
                },
                {
                    path: '/admin/user-guide',
                    name: 'admin-user-guide',
                    component: () => import('@/views/signing/UserGuide.vue'),
                    meta: { role: 'admin', guideAudience: 'admin' }
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
                    path: '/signing/documents/drafts',
                    name: 'signing-document-drafts',
                    component: () => import('@/views/signing/SigningDocuments.vue'),
                    meta: { role: 'admin', queue: 'draft', activeMenuKey: SIGNING_DOCUMENT_MENU_KEYS.draft }
                },
                {
                    path: '/signing/documents',
                    name: 'signing-documents',
                    component: () => import('@/views/signing/SigningDocuments.vue'),
                    meta: { role: 'admin', queue: 'active', activeMenuKey: SIGNING_DOCUMENT_MENU_KEYS.active }
                },
                {
                    path: '/signing/documents/history',
                    name: 'signing-document-history',
                    component: () => import('@/views/signing/SigningDocuments.vue'),
                    meta: { role: 'admin', queue: 'history', activeMenuKey: SIGNING_DOCUMENT_MENU_KEYS.history }
                },
                {
                    path: '/document-flow',
                    name: 'document-flow',
                    redirect: (to) => ({
                        name: 'signing-documents',
                        query: {
                            ...(to.query.doc_no ? { flow_doc_no: to.query.doc_no } : {}),
                            ...(to.query.doc_format_code ? { flow_doc_format_code: to.query.doc_format_code } : {})
                        }
                    })
                },
                {
                    path: '/signing/documents/new',
                    name: 'signing-document-new',
                    component: () => import('@/views/signing/SigningDocumentCreate.vue'),
                    meta: { role: 'admin', activeMenuKey: SIGNING_DOCUMENT_MENU_KEYS.draft }
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

async function validateSession() {
    if (!authStore.isAuthenticated()) return false;
    if (authStore.sessionChecked) return true;
    try {
        await authStore.loadMe();
        return true;
    } catch {
        authStore.clear();
        return false;
    }
}

router.beforeEach(async (to) => {
    if (to.name === 'public-signing') return true;

    if (to.meta.public) {
        if (await validateSession()) return authStore.user?.role === 'user' ? { name: 'my-signing-tasks' } : { name: 'dashboard' };
        return true;
    }

    if (!authStore.isAuthenticated()) return { name: 'login', query: { redirect: to.fullPath } };

    if (!(await validateSession())) return { name: 'login', query: { redirect: to.fullPath, session: 'expired' } };

    if (to.name === 'dashboard' && authStore.user?.role === 'user') {
        return { name: 'my-signing-tasks' };
    }

    if (to.meta.role && authStore.user?.role !== to.meta.role) {
        return authStore.user?.role === 'user' ? { name: 'my-signing-tasks' } : { name: 'dashboard' };
    }

    return true;
});

export default router;
