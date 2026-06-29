<script setup>
import AppConfigurator from '@/layout/AppConfigurator.vue';
import { useLayout } from '@/layout/composables/layout';

const { toggleDarkMode, isDarkTheme } = useLayout();
const showThemeConfigurator = import.meta.env.DEV || import.meta.env.VITE_ENABLE_THEME_CONFIG === 'true';
</script>

<template>
    <div class="fixed flex gap-4 top-8 right-8">
        <Button type="button" @click="toggleDarkMode" rounded :icon="isDarkTheme ? 'pi pi-moon' : 'pi pi-sun'" severity="secondary" />
        <div v-if="showThemeConfigurator" class="relative">
            <Button
                icon="pi pi-palette"
                v-styleclass="{ selector: '@next', enterFromClass: 'hidden', enterActiveClass: 'animate-scalein', leaveToClass: 'hidden', leaveActiveClass: 'animate-fadeout', hideOnOutsideClick: true }"
                type="button"
                rounded
            />
            <AppConfigurator />
        </div>
    </div>
</template>
