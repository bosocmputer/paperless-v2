<script setup>
import { authStore } from '@/stores/auth';
import { ADMIN_SIGNER_MENU_KEYS, SIGNING_DOCUMENT_MENU_KEYS } from '@/utils/signingQueue';
import { computed } from 'vue';
import AppMenuItem from './AppMenuItem.vue';

const isSuperAdmin = computed(() => authStore.user?.role === 'superadmin');
const internalDocumentsEnabled = computed(() => authStore.features?.internalDocuments === true);

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

    const sections = [
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
                    menuKey: SIGNING_DOCUMENT_MENU_KEYS.draft,
                    activeMatch: '/signing/documents/drafts'
                },
                ...(internalDocumentsEnabled.value
                    ? [
                          {
                              label: 'สร้างเอกสารภายใน',
                              icon: 'pi pi-fw pi-file-edit',
                              to: '/signing/internal-documents/new'
                          }
                      ]
                    : []),
                {
                    label: 'เอกสารรอเซ็น',
                    icon: 'pi pi-fw pi-send',
                    to: '/signing/documents',
                    menuKey: SIGNING_DOCUMENT_MENU_KEYS.active
                },
                {
                    label: 'ประวัติเอกสารเซ็น',
                    icon: 'pi pi-fw pi-history',
                    to: '/signing/documents/history',
                    menuKey: SIGNING_DOCUMENT_MENU_KEYS.history,
                    activeMatch: '/signing/documents/history'
                }
            ]
        },
        {
            label: 'งานของฉัน',
            items: [
                {
                    label: 'งานรอเซ็นของฉัน',
                    icon: 'pi pi-fw pi-inbox',
                    to: '/admin/signing/tasks',
                    menuKey: ADMIN_SIGNER_MENU_KEYS.tasks,
                    activeMatch: '/admin/signing/tasks'
                },
                {
                    label: 'ประวัติการเซ็นของฉัน',
                    icon: 'pi pi-fw pi-history',
                    to: '/admin/signing/history',
                    menuKey: ADMIN_SIGNER_MENU_KEYS.history,
                    activeMatch: '/admin/signing/history'
                }
            ]
        }
    ];
    if (isSuperAdmin.value) {
        sections.push({
            label: 'ตั้งค่าเอกสาร',
            items: [
                {
                    label: 'ตั้งค่า Workflow',
                    icon: 'pi pi-fw pi-file-edit',
                    to: '/config/documents',
                    activeMatch: '/config/documents'
                },
                ...(internalDocumentsEnabled.value
                    ? [
                          {
                              label: 'Master เอกสารภายใน',
                              icon: 'pi pi-fw pi-list-check',
                              to: '/config/internal-document-masters'
                          }
                      ]
                    : [])
            ]
        });
    }
    sections.push(
        {
            label: 'ระบบ',
            items: [
                ...(isSuperAdmin.value
                    ? [
                          {
                              label: 'ผู้ใช้งาน',
                              icon: 'pi pi-fw pi-users',
                              to: '/admin/users'
                          }
                      ]
                    : []),
                {
                    label: 'คู่มือการใช้งาน',
                    icon: 'pi pi-fw pi-book',
                    to: '/admin/guide'
                },
                {
                    label: 'คู่มือผู้เซ็น',
                    icon: 'pi pi-fw pi-info-circle',
                    to: '/admin/user-guide'
                }
            ]
        }
    );
    return sections;
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
