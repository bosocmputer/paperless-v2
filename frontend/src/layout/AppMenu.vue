<script setup>
import { authStore } from '@/stores/auth';
import { computed } from 'vue';
import AppMenuItem from './AppMenuItem.vue';

const model = computed(() => {
    if (authStore.user?.role === 'user') {
        return [
            {
                label: 'PaperLess',
                items: [
                    {
                        label: 'เอกสารรอเซ็น',
                        icon: 'pi pi-fw pi-inbox',
                        to: '/signing/tasks'
                    }
                ]
            }
        ];
    }

    return [
        {
            label: 'PaperLess',
            items: [
                {
                    label: 'Dashboard',
                    icon: 'pi pi-fw pi-home',
                    to: '/'
                },
                {
                    label: 'เอกสารรอเซ็น',
                    icon: 'pi pi-fw pi-inbox',
                    to: '/signing/tasks'
                },
                {
                    label: 'เอกสารเพื่อเซ็น',
                    icon: 'pi pi-fw pi-send',
                    to: '/signing/documents'
                },
                {
                    label: 'Config เอกสาร',
                    icon: 'pi pi-fw pi-file-edit',
                    to: '/config/documents'
                },
                {
                    label: 'ตั้งค่ากรอบลายเซ็น',
                    icon: 'pi pi-fw pi-pencil',
                    to: '/config/signature-templates'
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
                }
            ]
        }
    ];
});
</script>

<template>
    <ul class="layout-menu">
        <template v-for="item in model" :key="item.label">
            <app-menu-item v-if="!item.separator" :item="item" />
            <li v-if="item.separator" class="menu-separator"></li>
        </template>
    </ul>
</template>
