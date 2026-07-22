import AppLayout from '@/layout/AppLayout.vue';
import SignerLayout from '@/layout/SignerLayout.vue';
import { authStore } from '@/stores/auth';
import { ADMIN_SIGNER_MENU_KEYS, SIGNING_DOCUMENT_MENU_KEYS } from '@/utils/signingQueue';
import { createRouter, createWebHistory } from 'vue-router';

const ADMIN_SIGNER_ROUTE_BY_USER_ROUTE = Object.freeze({
    'my-signing-tasks': 'admin-my-signing-tasks',
    'my-signing-task': 'admin-my-signing-task',
    'my-signing-history': 'admin-my-signing-history',
    'my-signing-history-detail': 'admin-my-signing-history-detail'
});

const USER_SIGNER_ROUTE_BY_ADMIN_ROUTE = Object.freeze({
    'admin-my-signing-tasks': 'my-signing-tasks',
    'admin-my-signing-task': 'my-signing-task',
    'admin-my-signing-history': 'my-signing-history',
    'admin-my-signing-history-detail': 'my-signing-history-detail'
});

function redirectToSiblingRoute(routeName, to) {
    return {
        name: routeName,
        params: to.params,
        query: to.query,
        hash: to.hash
    };
}

function isAdminRole(role) {
    return role === 'admin' || role === 'superadmin';
}

function hasRequiredRole(requiredRole, actualRole) {
    if (!requiredRole) return true;
    if (requiredRole === 'admin') return isAdminRole(actualRole);
    return actualRole === requiredRole;
}

function defaultRouteForRole(role) {
    return role === 'user' ? { name: 'my-signing-tasks' } : { name: 'dashboard' };
}

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
                    meta: { role: 'superadmin' }
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
                    path: '/admin/signing/tasks',
                    name: 'admin-my-signing-tasks',
                    component: () => import('@/views/signing/AdminMySigningTasks.vue'),
                    meta: { role: 'admin', adminSignerWorkspace: true, activeMenuKey: ADMIN_SIGNER_MENU_KEYS.tasks }
                },
                {
                    path: '/admin/signing/tasks/:taskId',
                    name: 'admin-my-signing-task',
                    component: () => import('@/views/signing/SigningTask.vue'),
                    meta: { role: 'admin', adminSignerWorkspace: true, activeMenuKey: ADMIN_SIGNER_MENU_KEYS.tasks }
                },
                {
                    path: '/admin/signing/history',
                    name: 'admin-my-signing-history',
                    component: () => import('@/views/signing/AdminMySigningHistory.vue'),
                    meta: { role: 'admin', adminSignerWorkspace: true, activeMenuKey: ADMIN_SIGNER_MENU_KEYS.history }
                },
                {
                    path: '/admin/signing/history/:taskId',
                    name: 'admin-my-signing-history-detail',
                    component: () => import('@/views/signing/SigningHistoryDetail.vue'),
                    meta: { role: 'admin', adminSignerWorkspace: true, activeMenuKey: ADMIN_SIGNER_MENU_KEYS.history }
                },
                {
                    path: '/config/documents',
                    name: 'document-config',
                    component: () => import('@/views/config/DocumentConfig.vue'),
                    meta: { role: 'superadmin' }
                },
                {
                    path: '/config/internal-document-masters',
                    name: 'internal-document-masters',
                    component: () => import('@/views/config/InternalDocumentMasters.vue'),
                    meta: { role: 'superadmin', feature: 'internalDocuments' }
                },
                {
                    path: '/config/documents/:docFormatCode/workflow',
                    name: 'document-config-workflow',
                    component: () => import('@/views/config/DocumentConfigWorkflow.vue'),
                    meta: { role: 'superadmin' }
                },
                {
                    path: '/config/signature-templates',
                    name: 'signature-templates',
                    component: () => import('@/views/config/SignatureTemplateList.vue'),
                    meta: { role: 'superadmin' }
                },
                {
                    path: '/config/documents/:docFormatCode/signature-template',
                    name: 'signature-template',
                    component: () => import('@/views/config/SignatureTemplateDesigner.vue'),
                    meta: { role: 'superadmin' }
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
                    path: '/signing/internal-documents/new',
                    name: 'internal-document-new',
                    component: () => import('@/views/signing/InternalDocumentForm.vue'),
                    meta: { role: 'admin', feature: 'internalDocuments', activeMenuKey: SIGNING_DOCUMENT_MENU_KEYS.draft }
                },
                {
                    path: '/signing/internal-documents/:id/edit',
                    name: 'internal-document-edit',
                    component: () => import('@/views/signing/InternalDocumentForm.vue'),
                    meta: { role: 'admin', feature: 'internalDocuments', activeMenuKey: SIGNING_DOCUMENT_MENU_KEYS.draft }
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
        if (await validateSession()) return defaultRouteForRole(authStore.user?.role);
        return true;
    }

    if (!authStore.isAuthenticated()) return { name: 'login', query: { redirect: to.fullPath } };

    if (!(await validateSession())) return { name: 'login', query: { redirect: to.fullPath, session: 'expired' } };

    if (to.name === 'dashboard' && authStore.user?.role === 'user') {
        return { name: 'my-signing-tasks' };
    }

    if (isAdminRole(authStore.user?.role) && ADMIN_SIGNER_ROUTE_BY_USER_ROUTE[to.name]) {
        return redirectToSiblingRoute(ADMIN_SIGNER_ROUTE_BY_USER_ROUTE[to.name], to);
    }

    if (authStore.user?.role === 'user' && USER_SIGNER_ROUTE_BY_ADMIN_ROUTE[to.name]) {
        return redirectToSiblingRoute(USER_SIGNER_ROUTE_BY_ADMIN_ROUTE[to.name], to);
    }

    if (!hasRequiredRole(to.meta.role, authStore.user?.role)) {
        return defaultRouteForRole(authStore.user?.role);
    }

    if (to.meta.feature && authStore.features?.[to.meta.feature] !== true) {
        return defaultRouteForRole(authStore.user?.role);
    }

    return true;
});

export default router;
