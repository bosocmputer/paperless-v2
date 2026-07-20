<script setup>
import FloatingConfigurator from '@/components/FloatingConfigurator.vue';
import { api } from '@/services/api';
import { authStore } from '@/stores/auth';
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();
const username = ref('');
const password = ref('');
const loading = ref(false);
const provisioning = ref(false);
const verifying = ref(false);
const error = ref('');
const step = ref('credentials');
const databases = ref([]);
const selectedDatabase = ref('');
const authSource = ref('');

const databaseOptions = computed(() =>
    databases.value.map((database) => ({
        label: databaseLabel(database),
        value: databaseValue(database),
        description: [database.dataCode, database.tenant].filter(Boolean).join(' · '),
        readiness: database.readiness || null
    }))
);
const selectedDatabaseOption = computed(() => databaseOptions.value.find((option) => option.value === selectedDatabase.value) || null);
const selectedReadiness = computed(() => selectedDatabaseOption.value?.readiness || null);
const selectedDatabaseReady = computed(() => authSource.value.startsWith('local') || selectedReadiness.value?.ok === true);
const canProvisionSelectedDatabase = computed(
    () => authSource.value !== 'local' && step.value === 'database' && Boolean(selectedDatabase.value) && ['image_db_missing', 'doc_images_table_missing'].includes(selectedReadiness.value?.status)
);
const canVerifySelectedDatabase = computed(
    () =>
        authSource.value !== 'local' &&
        step.value === 'database' &&
        Boolean(selectedDatabase.value) &&
        (automaticVerificationFailed.value || ['not_ready', 'legacy_unknown'].includes(readinessState(selectedReadiness.value)))
);
const credentialsComplete = computed(() => username.value.trim() !== '' && password.value !== '');
const canSubmit = computed(() => !provisioning.value && !verifying.value && (step.value === 'database' ? Boolean(selectedDatabase.value) && selectedDatabaseReady.value : credentialsComplete.value));
let readinessRequestSequence = 0;
let automaticVerificationKey = '';
const automaticVerificationFailed = ref(false);

function databaseValue(database) {
    return database.databaseName || database.tenant || database.dataCode;
}

function databaseLabel(database) {
    const name = database.dataName || database.databaseName || database.tenant || database.dataCode;
    const code = database.databaseName || database.tenant || database.dataCode;
    if (!name) return code || 'Database';
    if (!code || name === code) return name;
    return `${name} (${code})`;
}

function readinessLabel(readiness) {
    const state = readinessState(readiness);
    if (state === 'ready') return 'พร้อมใช้งาน';
    if (state === 'checking') return 'กำลังตรวจสอบ';
    if (state === 'unverified') return 'ยังไม่เคยตรวจ';
    if (state === 'legacy_unknown') return 'ต้องตรวจสอบ';
    if (!readiness) return 'ยังไม่เคยตรวจ';
    const status = readiness.status || '';
    if (status === 'image_db_missing') return 'ไม่พบ image DB';
    if (status === 'doc_images_table_missing') return 'ไม่พบตารางรูป';
    if (status === 'main_db_missing') return 'ไม่พบ DB หลัก';
    if (status === 'main_db_unreachable') return 'DB หลักเปิดไม่ได้';
    if (status === 'image_db_unreachable') return 'image DB เปิดไม่ได้';
    if (status === 'main_db_corrupted') return 'DB หลักเสียหาย';
    if (status === 'image_db_corrupted') return 'image DB เสียหาย';
    if (status.endsWith('_permission_denied')) return 'สิทธิ์ DB ไม่เพียงพอ';
    if (status.endsWith('_connection_limit')) return 'connection เต็ม';
    if (status.endsWith('_temporarily_unavailable')) return 'DB ยังไม่พร้อม';
    if (status === 'main_schema_inspection_failed' || status === 'image_schema_inspection_failed') return 'ตรวจ schema ไม่ได้';
    if (status === 'verification_timeout') return 'ตรวจสอบหมดเวลา';
    if (status === 'template_not_ready') return 'ระบบมาตรฐานไม่พร้อม';
    if (status === 'readiness_service_unavailable') return 'ระบบตรวจสอบขัดข้อง';
    if (status === 'schema_mismatch') return 'schema ไม่พร้อม';
    return 'ตรวจไม่ได้';
}

function readinessSeverity(readiness) {
    const state = readinessState(readiness);
    if (state === 'ready') return 'success';
    if (state === 'checking') return 'info';
    if (state === 'unverified') return 'secondary';
    if (state === 'legacy_unknown') return 'warn';
    return 'danger';
}

function readinessDetail(readiness) {
    const state = readinessState(readiness);
    if (state === 'ready') return `พร้อมใช้งาน${readiness.imageDatabase ? ` · ${readiness.imageDatabase}` : ''}`;
    if (state === 'checking') return 'กำลังตรวจสอบความพร้อมครั้งแรก กรุณารอสักครู่';
    if (state === 'unverified') return 'ฐานข้อมูลนี้ยังไม่เคยตรวจ ระบบจะตรวจให้อัตโนมัติ';
    if (state === 'legacy_unknown') return 'พบชื่อฐานข้อมูลแล้ว แต่ยังไม่ได้ตรวจการเชื่อมต่อและ schema กรุณากด “ตรวจสอบอีกครั้ง”';
    if (!readiness) return '';
    if (Array.isArray(readiness.issues) && readiness.issues.length > 0) return `ฐานข้อมูลนี้ยังไม่พร้อมใช้งานใน PaperLess · พบ ${readiness.issues.length} ปัญหา`;
    if (readiness.status === 'image_db_missing') return `ไม่พบฐานข้อมูล ${readiness.imageDatabase || `${readiness.tenant || 'ฐานนี้'}_images`} กรุณาแจ้งผู้ดูแลระบบ SML`;
    if (readiness.status === 'doc_images_table_missing') return `ฐานข้อมูล ${readiness.imageDatabase || 'รูปเอกสาร'} ยังไม่มีตาราง public.sml_doc_images กรุณาแจ้งผู้ดูแลระบบ SML`;
    if (readiness.status === 'main_db_missing') return `ไม่พบฐานข้อมูล SML หลัก${readiness.tenant ? ` ${readiness.tenant}` : ''} กรุณาแจ้งผู้ดูแลระบบ SML`;
    if (readiness.status === 'schema_mismatch') return `schema ตารางรูปเอกสาร${readiness.imageDatabase ? ` ของ ${readiness.imageDatabase}` : ''} ไม่ตรงกับมาตรฐาน กรุณาแจ้งผู้ดูแลระบบ SML แล้วกดตรวจสอบอีกครั้ง`;
    return readiness.message || 'ยังตรวจความพร้อมไม่ได้ในขณะนี้';
}

function readinessState(readiness) {
    if (readiness?.ok || readiness?.registryStatus === 'ready') return 'ready';
    if (readiness?.registryStatus === 'checking' || readiness?.isChecking || readiness?.status === 'checking') return 'checking';
    if (readiness?.registryStatus === 'unverified' || readiness?.status === 'unverified') return 'unverified';
    if (readiness?.status === 'unknown' && !readiness?.registryStatus && readiness?.source !== 'registry') return 'legacy_unknown';
    if (!readiness) return 'unverified';
    return 'not_ready';
}

function readinessMessageSeverity(readiness) {
    const state = readinessState(readiness);
    if (state === 'ready') return 'success';
    if (state === 'checking') return 'info';
    if (state === 'unverified') return 'secondary';
    if (state === 'legacy_unknown') return 'warn';
    return 'error';
}

function readinessOwnerLabel(owner) {
    if (owner === 'sml_erp') return 'แจ้งผู้ดูแล SML ERP';
    if (owner === 'paperless') return 'แจ้งผู้ดูแล PaperLess';
    if (owner === 'infrastructure') return 'แจ้งผู้ดูแล Server/PaperLess หรือ SML ERP';
    return 'แจ้งผู้ดูแลระบบ';
}

function readinessIssues(readiness) {
    return Array.isArray(readiness?.issues) ? readiness.issues : [];
}

function updateDatabaseReadiness(databaseName, readiness) {
    const index = databases.value.findIndex((database) => databaseValue(database) === databaseName);
    if (index < 0) return;
    const next = [...databases.value];
    next[index] = {
        ...next[index],
        readiness
    };
    databases.value = next;
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
    readinessRequestSequence += 1;
    step.value = 'credentials';
    databases.value = [];
    selectedDatabase.value = '';
    authSource.value = '';
    automaticVerificationKey = '';
    automaticVerificationFailed.value = false;
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
        if (step.value === 'database' && err.payload?.readiness) {
            updateDatabaseReadiness(selectedDatabase.value, err.payload.readiness);
        }
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

async function verifySelectedDatabase({ automatic = false } = {}) {
    error.value = '';
    const state = readinessState(selectedReadiness.value);
    if (automatic) {
        if (!['unverified', 'checking'].includes(state)) return;
    } else if (!canVerifySelectedDatabase.value) {
        return;
    }
    const databaseName = selectedDatabase.value;
    const verificationKey = `${databaseName}:${state}`;
    if (automatic && automaticVerificationKey === verificationKey) return;
    if (automatic) automaticVerificationKey = verificationKey;
    automaticVerificationFailed.value = false;
    const requestSequence = ++readinessRequestSequence;
    verifying.value = true;
    try {
        const result = await api.verifySMLDatabaseReadiness(username.value.trim(), password.value, databaseName, authSource.value || 'sml');
        if (requestSequence !== readinessRequestSequence || databaseName !== selectedDatabase.value) return;
        if (result.readiness) updateDatabaseReadiness(databaseName, result.readiness);
        if (!automatic) {
            toast.add({
                severity: result.readiness?.ok ? 'success' : 'warn',
                summary: result.readiness?.ok ? 'Database พร้อมใช้งาน' : 'ตรวจสอบแล้ว แต่ยังไม่พร้อม',
                detail: readinessDetail(result.readiness),
                life: 4000
            });
        }
    } catch (err) {
        if (requestSequence !== readinessRequestSequence || databaseName !== selectedDatabase.value) return;
        automaticVerificationFailed.value = true;
        error.value = err.message;
        toast.add({
            severity: 'error',
            summary: 'ตรวจสอบ Database ไม่สำเร็จ',
            detail: err.message,
            life: 4500
        });
    } finally {
        if (requestSequence === readinessRequestSequence) verifying.value = false;
    }
}

watch(selectedDatabase, () => {
    automaticVerificationKey = '';
    automaticVerificationFailed.value = false;
    if (step.value !== 'database' || !selectedDatabase.value) return;
    void verifySelectedDatabase({ automatic: true });
});

async function provisionSelectedDatabase() {
    error.value = '';
    if (!canProvisionSelectedDatabase.value) return;
    const databaseName = selectedDatabase.value;
    const requestSequence = ++readinessRequestSequence;
    provisioning.value = true;
    try {
        const result = await api.provisionSMLImageDatabase(username.value.trim(), password.value, databaseName, authSource.value || 'sml');
        if (requestSequence !== readinessRequestSequence || databaseName !== selectedDatabase.value) return;
        if (result.readiness) updateDatabaseReadiness(databaseName, result.readiness);
        toast.add({
            severity: 'success',
            summary: result.provisioned ? 'ตั้งค่า image DB สำเร็จ' : 'Database พร้อมใช้งานแล้ว',
            detail: result.readiness?.imageDatabase ? `${result.readiness.imageDatabase} พร้อมใช้งาน` : 'ฐานข้อมูลนี้พร้อมใช้งานใน PaperLess แล้ว',
            life: 3500
        });
    } catch (err) {
        if (requestSequence !== readinessRequestSequence || databaseName !== selectedDatabase.value) return;
        error.value = err.message;
        toast.add({
            severity: 'error',
            summary: 'ตั้งค่า image DB ไม่สำเร็จ',
            detail: err.message,
            life: 4500
        });
    } finally {
        if (requestSequence === readinessRequestSequence) provisioning.value = false;
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
                                    <div class="flex items-start justify-between gap-3 w-full">
                                        <div class="flex flex-col min-w-0">
                                            <span class="truncate">{{ option.label }}</span>
                                            <small v-if="option.description" class="text-muted-color">{{ option.description }}</small>
                                        </div>
                                        <Tag :value="readinessLabel(option.readiness)" :severity="readinessSeverity(option.readiness)" />
                                    </div>
                                </template>
                                <template #value="{ value, placeholder }">
                                    <div v-if="selectedDatabaseOption" class="flex items-center justify-between gap-3 w-full">
                                        <div class="flex flex-col min-w-0">
                                            <span class="truncate">{{ selectedDatabaseOption.label }}</span>
                                            <small v-if="selectedDatabaseOption.description" class="text-muted-color">{{ selectedDatabaseOption.description }}</small>
                                        </div>
                                        <Tag :value="readinessLabel(selectedDatabaseOption.readiness)" :severity="readinessSeverity(selectedDatabaseOption.readiness)" />
                                    </div>
                                    <span v-else>{{ placeholder || value }}</span>
                                </template>
                            </Select>
                            <Message v-if="selectedReadiness" :severity="readinessMessageSeverity(selectedReadiness)" class="mb-4">
                                <div class="flex flex-col gap-2">
                                    <div class="font-medium">{{ readinessDetail(selectedReadiness) }}</div>
                                    <ul v-if="readinessIssues(selectedReadiness).length" class="m-0 pl-5 flex flex-col gap-2">
                                        <li v-for="issue in readinessIssues(selectedReadiness)" :key="`${issue.code}-${issue.database || ''}`">
                                            <div>{{ issue.message }}</div>
                                            <small class="opacity-80">{{ readinessOwnerLabel(issue.owner) }}</small>
                                        </li>
                                    </ul>
                                </div>
                            </Message>
                        </template>

                        <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

                        <div v-if="step === 'database' && (canVerifySelectedDatabase || canProvisionSelectedDatabase)" class="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
                            <Button
                                v-if="canVerifySelectedDatabase"
                                type="button"
                                label="ตรวจสอบอีกครั้ง"
                                icon="pi pi-refresh"
                                severity="secondary"
                                outlined
                                class="w-full"
                                :loading="verifying"
                                :disabled="loading || provisioning"
                                @click="verifySelectedDatabase"
                            />
                            <Button
                                v-if="canProvisionSelectedDatabase"
                                type="button"
                                label="ตั้งค่า image DB"
                                icon="pi pi-database"
                                severity="warn"
                                outlined
                                class="w-full"
                                :loading="provisioning"
                                :disabled="loading || verifying"
                                @click="provisionSelectedDatabase"
                            />
                        </div>
                        <div class="flex flex-col sm:flex-row gap-3">
                            <Button v-if="step === 'database'" type="button" label="ย้อนกลับ" icon="pi pi-arrow-left" severity="secondary" outlined class="w-full sm:w-auto" :disabled="loading || verifying || provisioning" @click="resetDatabaseStep" />
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
