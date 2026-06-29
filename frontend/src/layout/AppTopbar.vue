<script setup>
import { authStore } from '@/stores/auth';
import { useLayout } from '@/layout/composables/layout';
import { useRouter } from 'vue-router';
import AppConfigurator from './AppConfigurator.vue';

const router = useRouter();
const { toggleMenu, toggleDarkMode, isDarkTheme } = useLayout();
const showThemeConfigurator = import.meta.env.DEV || import.meta.env.VITE_ENABLE_THEME_CONFIG === 'true';

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
            <router-link to="/" class="layout-topbar-logo">
                <span class="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-primary text-primary-contrast">
                    <i class="pi pi-file-edit text-xl"></i>
                </span>
                <span>PaperLess</span>
            </router-link>
        </div>

        <div class="layout-topbar-actions">
            <div class="layout-config-menu">
                <button type="button" class="layout-topbar-action" @click="toggleDarkMode" aria-label="Toggle dark mode">
                    <i :class="['pi', { 'pi-moon': isDarkTheme, 'pi-sun': !isDarkTheme }]"></i>
                </button>
                <div v-if="showThemeConfigurator" class="relative">
                    <button
                        v-styleclass="{ selector: '@next', enterFromClass: 'hidden', enterActiveClass: 'p-anchored-overlay-enter-active', leaveToClass: 'hidden', leaveActiveClass: 'p-anchored-overlay-leave-active', hideOnOutsideClick: true }"
                        type="button"
                        class="layout-topbar-action layout-topbar-action-highlight"
                        aria-label="Theme settings"
                    >
                        <i class="pi pi-palette"></i>
                    </button>
                    <AppConfigurator />
                </div>
            </div>

            <button
                class="layout-topbar-menu-button layout-topbar-action"
                v-styleclass="{ selector: '@next', enterFromClass: 'hidden', enterActiveClass: 'p-anchored-overlay-enter-active', leaveToClass: 'hidden', leaveActiveClass: 'p-anchored-overlay-leave-active', hideOnOutsideClick: true }"
                aria-label="Open account menu"
            >
                <i class="pi pi-ellipsis-v"></i>
            </button>

            <div class="layout-topbar-menu hidden lg:block">
                <div class="layout-topbar-menu-content">
                    <span class="layout-topbar-action">
                        <i class="pi pi-user"></i>
                        <span>{{ authStore.user?.displayName }}</span>
                    </span>
                    <span class="layout-topbar-action">
                        <i class="pi pi-shield"></i>
                        <span>{{ authStore.user?.role }}</span>
                    </span>
                    <button type="button" class="layout-topbar-action" aria-label="ออกจากระบบ" @click="logout">
                        <i class="pi pi-sign-out"></i>
                        <span>ออกจากระบบ</span>
                    </button>
                </div>
            </div>
        </div>
    </div>
</template>
