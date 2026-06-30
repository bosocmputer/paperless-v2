<script setup>
import { useLayout } from '@/layout/composables/layout';
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import AppFooter from './AppFooter.vue';
import AppSidebar from './AppSidebar.vue';
import AppTopbar from './AppTopbar.vue';

const { layoutConfig, layoutState, hideMobileMenu } = useLayout();
const route = useRoute();

const containerClass = computed(() => {
    return {
        'layout-overlay': layoutConfig.menuMode === 'overlay',
        'layout-static': layoutConfig.menuMode === 'static',
        'layout-overlay-active': layoutState.overlayMenuActive,
        'layout-mobile-active': layoutState.mobileMenuActive,
        'layout-static-inactive': layoutState.staticMenuInactive
    };
});
const mainContainerClass = computed(() => ({
    'layout-main-container-dense': !!route.meta.denseContent,
    'layout-main-container-no-footer': !!route.meta.hideFooter
}));
</script>

<template>
    <div class="layout-wrapper" :class="containerClass">
        <AppTopbar />
        <AppSidebar />
        <div class="layout-main-container" :class="mainContainerClass">
            <div class="layout-main">
                <router-view />
            </div>
            <AppFooter v-if="!route.meta.hideFooter" />
        </div>
        <div class="layout-mask animate-fadein" @click="hideMobileMenu" />
    </div>
</template>

<style scoped>
.layout-main-container-dense {
    padding-top: 4.5rem;
}

.layout-main-container-no-footer .layout-main {
    padding-bottom: 1rem;
}

@media (max-width: 991px) {
    .layout-main-container-dense {
        padding: 4.25rem 1rem 0 1rem;
    }
}
</style>
