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
                        label: 'งานรอเซ็น',
                        icon: 'pi pi-fw pi-inbox',
                        to: '/signing/tasks'
                    },
                    {
                        label: 'ประวัติการเซ็น',
                        icon: 'pi pi-fw pi-history',
                        to: '/signing/history'
                    }
                ]
            }
        ];
    }

    return [
        {
            label: 'Admin Console',
            items: [
                {
                    label: 'ภาพรวม',
                    icon: 'pi pi-fw pi-home',
                    to: '/'
                },
                {
                    label: 'เอกสารเตรียมส่ง',
                    icon: 'pi pi-fw pi-file-plus',
                    to: '/signing/documents/drafts',
                    activeMatch: '/signing/documents/drafts'
                },
                {
                    label: 'เอกสารรอเซ็น',
                    icon: 'pi pi-fw pi-send',
                    to: '/signing/documents'
                },
                {
                    label: 'ประวัติเอกสารเซ็น',
                    icon: 'pi pi-fw pi-history',
                    to: '/signing/documents/history',
                    activeMatch: '/signing/documents/history'
                }
            ]
        },
        {
            label: 'ตั้งค่าเอกสาร',
            items: [
                {
                    label: 'ตั้งค่า Workflow',
                    icon: 'pi pi-fw pi-file-edit',
                    to: '/config/documents',
                    activeMatch: '/config/documents'
                }
            ]
        },
        {
            label: 'ระบบ',
            items: [
                {
                    label: 'ผู้ใช้งาน',
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
