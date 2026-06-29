<script setup>
import Card from 'primevue/card';
import Tag from 'primevue/tag';
import { authStore } from '@/stores/auth';

const nextModules = [
  { label: 'Workflow Config', status: 'next' },
  { label: 'Document Inbox', status: 'next' },
  { label: 'Signature Capture', status: 'next' },
  { label: 'Audit Trail', status: 'next' }
];
</script>

<template>
  <section class="dashboard-grid">
    <Card class="hero-card">
      <template #content>
        <div class="hero-card-content">
          <div>
            <p class="eyebrow">Signed in</p>
            <h2>{{ authStore.user?.displayName }}</h2>
            <p class="body-copy">@{{ authStore.user?.username }}</p>
          </div>
          <Tag :value="authStore.user?.role" severity="success" />
        </div>
      </template>
    </Card>

    <Card>
      <template #title>สถานะระบบ</template>
      <template #content>
        <div class="status-list">
          <div>
            <i class="pi pi-check-circle"></i>
            <span>Backend API</span>
            <Tag value="ready" severity="success" />
          </div>
          <div>
            <i class="pi pi-database"></i>
            <span>Postgres</span>
            <Tag value="ready" severity="success" />
          </div>
          <div>
            <i class="pi pi-shield"></i>
            <span>Auth seed</span>
            <Tag value="superadmin" severity="info" />
          </div>
        </div>
      </template>
    </Card>

    <Card class="wide-card">
      <template #title>โมดูลถัดไป</template>
      <template #content>
        <div class="module-list">
          <div v-for="module in nextModules" :key="module.label" class="module-row">
            <span>{{ module.label }}</span>
            <Tag value="queued" severity="secondary" />
          </div>
        </div>
      </template>
    </Card>
  </section>
</template>

