<script setup>
import { authStore } from '@/stores/auth';
import { useLayout } from '@/layout/composables/layout';
import { computed } from 'vue';
import { useRouter } from 'vue-router';

const router = useRouter();
const { toggleMenu } = useLayout();
const homeRoute = computed(() => (authStore.user?.role === 'user' ? { name: 'my-signing-tasks' } : { name: 'dashboard' }));
const consoleLabel = computed(() => (authStore.user?.role === 'user' ? 'งานรอเซ็น' : 'ภาพรวมผู้ดูแล'));
const displayName = computed(() => authStore.user?.displayName || authStore.user?.username || 'ผู้ใช้งาน');
const roleLabel = computed(() => {
    const labels = {
        superadmin: 'Superadmin',
        admin: 'Admin',
        user: 'User'
    };
    return labels[authStore.user?.role] || authStore.user?.role || '';
});

async function logout() {
    await authStore.logout();
    router.push({ name: 'login' });
}
</script>

<template>
    <div class="layout-topbar">
        <div class="layout-topbar-logo-container">
            <button class="layout-menu-button layout-topbar-action" @click="toggleMenu">
                <i class="pi pi-bars"></i>
            </button>
            <router-link :to="homeRoute" class="layout-topbar-logo">
                <span class="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-primary text-primary-contrast">
                    <i class="pi pi-file-edit text-xl"></i>
                </span>
                <span>
                    <strong>PaperLess</strong>
                    <small>{{ consoleLabel }}</small>
                </span>
            </router-link>
        </div>

        <div class="layout-topbar-actions">
            <button
                class="layout-topbar-menu-button layout-topbar-action"
                v-styleclass="{ selector: '@next', enterFromClass: 'hidden', enterActiveClass: 'p-anchored-overlay-enter-active', leaveToClass: 'hidden', leaveActiveClass: 'p-anchored-overlay-leave-active', hideOnOutsideClick: true }"
                aria-label="Open account menu"
            >
                <i class="pi pi-ellipsis-v"></i>
            </button>

            <div class="layout-topbar-menu hidden lg:block">
                <div class="layout-topbar-menu-content">
                    <span class="topbar-user-summary">
                        <strong>{{ displayName }}</strong>
                        <small v-if="roleLabel">{{ roleLabel }}</small>
                    </span>
                    <button type="button" class="layout-topbar-action topbar-logout-action" aria-label="ออกจากระบบ" @click="logout">
                        <i class="pi pi-sign-out"></i>
                        <span>ออกจากระบบ</span>
                    </button>
                </div>
            </div>
        </div>
    </div>
</template>
