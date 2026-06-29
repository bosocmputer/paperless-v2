<script setup>
import FloatingConfigurator from '@/components/FloatingConfigurator.vue';
import { authStore } from '@/stores/auth';
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

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
    <FloatingConfigurator />
    <div class="bg-surface-50 dark:bg-surface-950 flex items-center justify-center min-h-screen min-w-[100vw] overflow-hidden">
        <div class="flex flex-col items-center justify-center w-full px-6">
            <div style="border-radius: 56px; padding: 0.3rem; background: linear-gradient(180deg, var(--primary-color) 10%, rgba(33, 150, 243, 0) 30%)">
                <div class="w-full bg-surface-0 dark:bg-surface-900 py-12 px-6 sm:px-12" style="border-radius: 53px">
                    <div class="text-center mb-8">
                        <div class="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-primary text-primary-contrast mb-6">
                            <i class="pi pi-file-check text-3xl"></i>
                        </div>
                        <div class="text-surface-900 dark:text-surface-0 text-3xl font-medium mb-3">PaperLess V2</div>
                        <span class="text-muted-color font-medium">เข้าสู่ระบบเซ็นเอกสาร</span>
                    </div>

                    <form class="w-full md:w-[30rem]" @submit.prevent="submit">
                        <label for="username" class="block text-surface-900 dark:text-surface-0 text-xl font-medium mb-2">Username</label>
                        <InputText id="username" v-model="username" type="text" autocomplete="username" class="w-full mb-6" autofocus />

                        <label for="password" class="block text-surface-900 dark:text-surface-0 font-medium text-xl mb-2">Password</label>
                        <Password id="password" v-model="password" :toggleMask="true" class="mb-4" fluid :feedback="false" autocomplete="current-password" />

                        <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

                        <Button type="submit" label="เข้าสู่ระบบ" icon="pi pi-sign-in" class="w-full" :loading="loading" />
                    </form>

                    <div class="mt-8 pt-6 border-t border-surface flex flex-col gap-3 text-sm">
                        <div class="flex items-center justify-between gap-4">
                            <span class="text-muted-color">ชื่อ</span>
                            <strong>System Administrator</strong>
                        </div>
                        <div class="flex items-center justify-between gap-4">
                            <span class="text-muted-color">Username</span>
                            <strong>superadmin</strong>
                        </div>
                        <div class="flex items-center justify-between gap-4">
                            <span class="text-muted-color">ระดับ</span>
                            <Tag value="admin" severity="success" />
                        </div>
                        <div class="flex items-center justify-between gap-4">
                            <span class="text-muted-color">ระดับที่รองรับ</span>
                            <strong>admin, user</strong>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

<style scoped>
.pi-eye {
    transform: scale(1.6);
    margin-right: 1rem;
}

.pi-eye-slash {
    transform: scale(1.6);
    margin-right: 1rem;
}
</style>

