<script setup>
import { authStore } from '@/stores/auth';

const modules = [
    { label: 'Workflow Config', icon: 'pi pi-sitemap', status: 'queued' },
    { label: 'Document Inbox', icon: 'pi pi-inbox', status: 'queued' },
    { label: 'Signature Capture', icon: 'pi pi-pencil', status: 'queued' },
    { label: 'Audit Trail', icon: 'pi pi-history', status: 'queued' }
];
</script>

<template>
    <div class="grid grid-cols-12 gap-8">
        <div class="col-span-12 xl:col-span-8">
            <div class="card mb-8">
                <div class="flex flex-col md:flex-row md:items-center justify-between gap-6">
                    <div>
                        <div class="text-muted-color font-medium mb-2">Signed in</div>
                        <div class="text-surface-900 dark:text-surface-0 font-semibold text-3xl mb-2">{{ authStore.user?.displayName }}</div>
                        <div class="text-muted-color">@{{ authStore.user?.username }}</div>
                    </div>
                    <Tag :value="authStore.user?.role" severity="success" class="w-fit" />
                </div>
            </div>

            <div class="card">
                <div class="font-semibold text-xl mb-4">โมดูลถัดไป</div>
                <div class="flex flex-col gap-4">
                    <div v-for="module in modules" :key="module.label" class="flex items-center justify-between gap-4 rounded-lg border border-surface p-4">
                        <div class="flex items-center gap-3">
                            <span class="inline-flex items-center justify-center w-10 h-10 rounded-lg bg-primary-50 text-primary">
                                <i :class="module.icon"></i>
                            </span>
                            <span class="font-medium">{{ module.label }}</span>
                        </div>
                        <Tag :value="module.status" severity="secondary" />
                    </div>
                </div>
            </div>
        </div>

        <div class="col-span-12 xl:col-span-4">
            <div class="card">
                <div class="font-semibold text-xl mb-4">สถานะระบบ</div>
                <div class="flex flex-col gap-4">
                    <div class="flex items-center justify-between">
                        <div class="flex items-center gap-3">
                            <i class="pi pi-server text-primary"></i>
                            <span>Backend API</span>
                        </div>
                        <Tag value="ready" severity="success" />
                    </div>
                    <div class="flex items-center justify-between">
                        <div class="flex items-center gap-3">
                            <i class="pi pi-database text-primary"></i>
                            <span>Postgres</span>
                        </div>
                        <Tag value="ready" severity="success" />
                    </div>
                    <div class="flex items-center justify-between">
                        <div class="flex items-center gap-3">
                            <i class="pi pi-shield text-primary"></i>
                            <span>Auth seed</span>
                        </div>
                        <Tag value="superadmin" severity="info" />
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

