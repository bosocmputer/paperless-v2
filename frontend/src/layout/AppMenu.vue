<script setup>
import { ref } from 'vue';
import AppMenuItem from './AppMenuItem.vue';

const model = ref([
    {
        label: 'PaperLess',
        items: [
            {
                label: 'Dashboard',
                icon: 'pi pi-fw pi-home',
                to: '/'
            },
            {
                label: 'Inbox',
                icon: 'pi pi-fw pi-inbox',
                to: '/pages/empty'
            },
            {
                label: 'Workflow Config',
                icon: 'pi pi-fw pi-sitemap',
                to: '/pages/empty'
            },
            {
                label: 'Audit Trail',
                icon: 'pi pi-fw pi-history',
                to: '/pages/empty'
            }
        ]
    },
    {
        label: 'Admin',
        items: [
            {
                label: 'Users',
                icon: 'pi pi-fw pi-users',
                to: '/admin/users'
            },
            {
                label: 'System Status',
                icon: 'pi pi-fw pi-server',
                to: '/'
            }
        ]
    }
]);
</script>

<template>
    <ul class="layout-menu">
        <template v-for="item in model" :key="item.label">
            <app-menu-item v-if="!item.separator" :item="item" />
            <li v-if="item.separator" class="menu-separator"></li>
        </template>
    </ul>
</template>
