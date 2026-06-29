<script setup>
import Button from 'primevue/button';
import Tag from 'primevue/tag';
import { useRouter } from 'vue-router';
import { authStore } from '@/stores/auth';

const router = useRouter();

async function logout() {
  await authStore.logout();
  router.push({ name: 'login' });
}
</script>

<template>
  <header class="topbar">
    <div>
      <p class="eyebrow">PaperLess Workspace</p>
      <h1>ระบบเซ็นเอกสาร</h1>
    </div>
    <div class="topbar-actions">
      <Tag :value="authStore.user?.role || 'user'" severity="info" />
      <span class="user-chip">
        <i class="pi pi-user"></i>
        {{ authStore.user?.displayName }}
      </span>
      <Button icon="pi pi-sign-out" label="ออกจากระบบ" severity="secondary" outlined @click="logout" />
    </div>
  </header>
</template>

