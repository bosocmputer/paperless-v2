<script setup>
import FloatingConfigurator from '@/components/FloatingConfigurator.vue';
import { authStore } from '@/stores/auth';
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();
const username = ref('');
const password = ref('');
const loading = ref(false);
const error = ref('');
const step = ref('credentials');
const databases = ref([]);
const selectedDatabase = ref('');
const authSource = ref('');

const databaseOptions = computed(() =>
    databases.value.map((database) => ({
        label: databaseLabel(database),
        value: database.databaseName || database.tenant || database.dataCode,
        description: [database.dataCode, database.tenant].filter(Boolean).join(' · ')
    }))
);
const credentialsComplete = computed(() => username.value.trim() !== '' && password.value !== '');
const canSubmit = computed(() => (step.value === 'database' ? Boolean(selectedDatabase.value) : credentialsComplete.value));

function databaseLabel(database) {
    const name = database.dataName || database.databaseName || database.tenant || database.dataCode;
    const code = database.databaseName || database.tenant || database.dataCode;
    if (!name) return code || 'Database';
    if (!code || name === code) return name;
    return `${name} (${code})`;
}

function goToApp() {
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '';
    if (redirect && redirect.startsWith('/')) {
        router.push(redirect);
    } else {
        router.push(authStore.user?.role === 'user' ? { name: 'my-signing-tasks' } : { name: 'dashboard' });
    }
}

function resetDatabaseStep() {
    step.value = 'credentials';
    databases.value = [];
    selectedDatabase.value = '';
    authSource.value = '';
    error.value = '';
}

async function submit() {
    error.value = '';
    if (!canSubmit.value) return;
    loading.value = true;
    try {
        const result = await authStore.login(username.value.trim(), password.value, step.value === 'database' ? selectedDatabase.value : '', authSource.value);
        if (result.databaseRequired) {
            databases.value = result.databases || [];
            authSource.value = result.authSource || 'sml';
            selectedDatabase.value = databaseOptions.value.find((option) => option.value === 'sml1_2026')?.value || databaseOptions.value[0]?.value || '';
            step.value = 'database';
            if (!selectedDatabase.value) {
                error.value = 'บัญชีนี้ยังไม่มีสิทธิ์เข้า database ใน SML';
            }
            return;
        }
        goToApp();
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
    <div class="bg-surface-50 dark:bg-surface-950 flex items-center justify-center min-h-screen min-w-[100vw] overflow-hidden px-4 py-6">
        <div class="flex flex-col items-center justify-center w-full">
            <div class="w-full max-w-[38rem]" style="border-radius: 24px; padding: 0.25rem; background: linear-gradient(180deg, var(--primary-color) 8%, rgba(33, 150, 243, 0) 32%)">
                <div class="w-full bg-surface-0 dark:bg-surface-900 py-10 px-5 sm:px-8 md:py-14 md:px-14" style="border-radius: 20px">
                    <div class="text-center mb-8">
                        <div class="inline-flex items-center justify-center w-14 h-14 rounded-xl bg-primary text-primary-contrast mb-5">
                            <i class="pi pi-file-check text-2xl"></i>
                        </div>
                        <div class="text-surface-900 dark:text-surface-0 text-3xl font-medium">PaperLess</div>
                    </div>

                    <div class="w-full md:w-[30rem] mx-auto mb-6 grid grid-cols-2 gap-2">
                        <div class="rounded-md border px-3 py-2" :class="step === 'credentials' ? 'border-primary bg-primary-50 dark:bg-primary-950' : 'border-surface-200 dark:border-surface-700'">
                            <div class="text-xs text-muted-color">ขั้นตอนที่ 1</div>
                            <div class="font-medium">บัญชี SML</div>
                        </div>
                        <div class="rounded-md border px-3 py-2" :class="step === 'database' ? 'border-primary bg-primary-50 dark:bg-primary-950' : 'border-surface-200 dark:border-surface-700'">
                            <div class="text-xs text-muted-color">ขั้นตอนที่ 2</div>
                            <div class="font-medium">เลือก database</div>
                        </div>
                    </div>

                    <form class="w-full md:w-[30rem] mx-auto" @submit.prevent="submit">
                        <template v-if="step === 'credentials'">
                            <label for="username" class="block text-surface-900 dark:text-surface-0 font-medium mb-2">ชื่อผู้ใช้ SML</label>
                            <InputText id="username" v-model="username" type="text" autocomplete="username" class="w-full mb-5" autofocus />

                            <label for="password" class="block text-surface-900 dark:text-surface-0 font-medium mb-2">รหัสผ่าน SML</label>
                            <Password id="password" v-model="password" :toggleMask="true" class="mb-4" fluid :feedback="false" autocomplete="current-password" />
                        </template>

                        <template v-else>
                            <div class="rounded-md border border-surface-200 dark:border-surface-700 p-3 mb-5">
                                <div class="text-sm text-muted-color">ผู้ใช้</div>
                                <div class="font-semibold break-all">{{ username }}</div>
                            </div>

                            <label for="database" class="block text-surface-900 dark:text-surface-0 font-medium mb-2">Database</label>
                            <Select id="database" v-model="selectedDatabase" :options="databaseOptions" optionLabel="label" optionValue="value" filter fluid class="mb-4">
                                <template #option="{ option }">
                                    <div class="flex flex-col">
                                        <span>{{ option.label }}</span>
                                        <small v-if="option.description" class="text-muted-color">{{ option.description }}</small>
                                    </div>
                                </template>
                            </Select>
                        </template>

                        <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

                        <div class="flex flex-col sm:flex-row gap-3">
                            <Button v-if="step === 'database'" type="button" label="ย้อนกลับ" icon="pi pi-arrow-left" severity="secondary" outlined class="w-full sm:w-auto" :disabled="loading" @click="resetDatabaseStep" />
                            <Button type="submit" :label="step === 'database' ? 'เข้าสู่ PaperLess' : 'ตรวจสอบบัญชี'" :icon="step === 'database' ? 'pi pi-sign-in' : 'pi pi-arrow-right'" class="w-full" :loading="loading" :disabled="!canSubmit" />
                        </div>
                    </form>

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
