<script setup>
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import Button from 'primevue/button';
import Card from 'primevue/card';
import InputText from 'primevue/inputtext';
import Password from 'primevue/password';
import Tag from 'primevue/tag';
import { authStore } from '@/stores/auth';

const router = useRouter();
const toast = useToast();
const username = ref('superadmin');
const password = ref('superadmin');
const loading = ref(false);
const error = ref('');

async function submit() {
  error.value = '';
  loading.value = true;
  try {
    await authStore.login(username.value, password.value);
    router.push({ name: 'dashboard' });
  } catch (err) {
    error.value = err.message;
    toast.add({
      severity: 'error',
      summary: 'เข้าสู่ระบบไม่สำเร็จ',
      detail: err.message,
      life: 3500
    });
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <main class="login-page">
    <section class="login-panel" aria-label="PaperLess login">
      <div class="login-intro">
        <div class="login-logo">
          <i class="pi pi-file-check"></i>
        </div>
        <p class="eyebrow">PaperLess V2</p>
        <h1>เข้าสู่ระบบเซ็นเอกสาร</h1>
        <p class="login-copy">ใช้บัญชีที่มีสิทธิ์เพื่อเข้า workspace เอกสารและ workflow</p>
      </div>

      <Card class="login-card">
        <template #content>
          <form class="login-form" @submit.prevent="submit">
            <label for="username">Username</label>
            <InputText id="username" v-model="username" autocomplete="username" autofocus />

            <label for="password">Password</label>
            <Password id="password" v-model="password" :feedback="false" toggle-mask autocomplete="current-password" />

            <p v-if="error" class="form-error">{{ error }}</p>

            <Button type="submit" label="เข้าสู่ระบบ" icon="pi pi-sign-in" :loading="loading" />
          </form>

          <div class="seed-account">
            <div>
              <span>ชื่อ</span>
              <strong>System Administrator</strong>
            </div>
            <div>
              <span>Username</span>
              <strong>superadmin</strong>
            </div>
            <div>
              <span>ระดับ</span>
              <Tag value="admin" severity="success" />
            </div>
            <div>
              <span>ระดับที่รองรับ</span>
              <strong>admin, user</strong>
            </div>
          </div>
        </template>
      </Card>
    </section>
  </main>
</template>

