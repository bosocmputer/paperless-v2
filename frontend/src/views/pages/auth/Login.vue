<script setup>
import FloatingConfigurator from '@/components/FloatingConfigurator.vue';
import { api } from '@/services/api';
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
const provisioning = ref(false);
const verifying = ref(false);
const error = ref('');
const step = ref('credentials');
const databases = ref([]);
const selectedDatabase = ref('');
const authSource = ref('');
const verificationCompleted = ref(0);
const verificationTotal = ref(0);
const verificationCurrent = ref([]);
const verificationFailed = ref(0);
const verificationHasRun = ref(false);

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
const canVerifyDatabases = computed(() => authSource.value !== 'local' && step.value === 'database' && databaseOptions.value.length > 0);
const verificationProgress = computed(() => {
    if (verificationTotal.value > 0) return Math.round((verificationCompleted.value / verificationTotal.value) * 100);
    return verificationHasRun.value ? 100 : 0;
});
const verificationCurrentLabel = computed(() => verificationCurrent.value.map((item) => item.label).join(' และ '));
const verificationSummary = computed(() => {
    const summary = { ready: 0, notReady: 0, unchecked: 0 };
    for (const option of databaseOptions.value) {
        const state = readinessState(option.readiness);
        if (state === 'ready') summary.ready += 1;
        else if (state === 'unverified' || state === 'checking' || state === 'legacy_unknown') summary.unchecked += 1;
        else summary.notReady += 1;
    }
    return summary;
});
const credentialsComplete = computed(() => username.value.trim() !== '' && password.value !== '');
const canSubmit = computed(() => !provisioning.value && !verifying.value && (step.value === 'database' ? Boolean(selectedDatabase.value) && selectedDatabaseReady.value : credentialsComplete.value));
let readinessRequestSequence = 0;

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
    if (state === 'unverified') return 'ฐานข้อมูลนี้ยังไม่เคยตรวจ กด “ตรวจสอบฐานข้อมูลทั้งหมด” เพื่อตรวจครั้งเดียว';
    if (state === 'legacy_unknown') return 'พบชื่อฐานข้อมูลแล้ว แต่ยังไม่ได้ตรวจการเชื่อมต่อและ schema';
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
    resetVerificationProgress();
    error.value = '';
}

function resetVerificationProgress() {
    verificationCompleted.value = 0;
    verificationTotal.value = 0;
    verificationCurrent.value = [];
    verificationFailed.value = 0;
    verificationHasRun.value = false;
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
            resetVerificationProgress();
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

async function verifyAllDatabases() {
    error.value = '';
    if (!canVerifyDatabases.value || verifying.value) return;

    const targets = databaseOptions.value.filter((option) => readinessState(option.readiness) !== 'ready');
    verificationHasRun.value = true;
    verificationCompleted.value = 0;
    verificationTotal.value = targets.length;
    verificationCurrent.value = [];
    verificationFailed.value = 0;

    if (targets.length === 0) {
        toast.add({
            severity: 'success',
            summary: 'Database พร้อมใช้งานครบแล้ว',
            detail: `ตรวจสอบแล้ว ${databaseOptions.value.length} database`,
            life: 3000
        });
        return;
    }

    const requestSequence = ++readinessRequestSequence;
    const queue = [...targets];
    verifying.value = true;

    async function worker() {
        while (requestSequence === readinessRequestSequence) {
            const target = queue.shift();
            if (!target) return;
            const databaseName = target.value;
            verificationCurrent.value = [...verificationCurrent.value, { value: target.value, label: target.label }];
            updateDatabaseReadiness(databaseName, {
                ...(target.readiness || {}),
                ok: false,
                status: 'checking',
                registryStatus: 'checking',
                isChecking: true,
                message: 'กำลังตรวจสอบความพร้อมของฐานข้อมูล'
            });
            try {
                const result = await api.verifySMLDatabaseReadiness(username.value.trim(), password.value, databaseName, authSource.value || 'sml');
                if (requestSequence !== readinessRequestSequence) return;
                if (!result.readiness) throw new Error('ระบบไม่ได้ส่งผลตรวจสอบ Database กลับมา');
                updateDatabaseReadiness(databaseName, result.readiness);
                if (!result.readiness.ok) verificationFailed.value += 1;
            } catch (err) {
                if (requestSequence !== readinessRequestSequence) return;
                verificationFailed.value += 1;
                updateDatabaseReadiness(databaseName, {
                    ...(target.readiness || {}),
                    ok: false,
                    status: 'readiness_service_unavailable',
                    registryStatus: 'not_ready',
                    isChecking: false,
                    message: err.message || 'ยังตรวจความพร้อมไม่ได้ในขณะนี้'
                });
            } finally {
                if (requestSequence === readinessRequestSequence) {
                    verificationCurrent.value = verificationCurrent.value.filter((item) => item.value !== target.value);
                    verificationCompleted.value += 1;
                }
            }
        }
    }

    try {
        await Promise.all([worker(), worker()]);
    } finally {
        if (requestSequence === readinessRequestSequence) verifying.value = false;
    }
    if (requestSequence !== readinessRequestSequence) return;
    toast.add({
        severity: verificationFailed.value > 0 ? 'warn' : 'success',
        summary: 'ตรวจสอบ Database ครบแล้ว',
        detail: verificationFailed.value > 0 ? `พร้อมใช้งาน ${verificationSummary.value.ready} จาก ${databaseOptions.value.length} database` : `พร้อมใช้งานครบ ${databaseOptions.value.length} database`,
        life: 4500
    });
}

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
    <div class="bg-surface-50 dark:bg-surface-950 flex items-center justify-center min-h-screen min-w-[100vw] overflow-x-hidden overflow-y-auto px-4 py-6">
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

                            <div v-if="canVerifyDatabases" class="mb-4 flex flex-col gap-3">
                                <Button
                                    type="button"
                                    label="ตรวจสอบฐานข้อมูลทั้งหมด"
                                    icon="pi pi-database"
                                    severity="secondary"
                                    outlined
                                    class="w-full"
                                    :loading="verifying"
                                    :disabled="loading || provisioning"
                                    @click="verifyAllDatabases"
                                />

                                <div v-if="verifying || verificationHasRun" class="rounded-md border border-surface-200 dark:border-surface-700 bg-surface-50 dark:bg-surface-800 p-3 flex flex-col gap-3" aria-live="polite">
                                    <div class="flex items-center justify-between gap-3 text-sm">
                                        <span v-if="verificationTotal" class="font-medium">ตรวจสอบแล้ว {{ verificationCompleted }}/{{ verificationTotal }}</span>
                                        <span v-else class="font-medium">พร้อมใช้งานครบ {{ databaseOptions.length }} database</span>
                                        <span class="text-muted-color">{{ verificationProgress }}%</span>
                                    </div>
                                    <ProgressBar :value="verificationProgress" :showValue="false" class="h-2" />
                                    <div v-if="verifying && verificationCurrent.length" class="flex items-start gap-2 text-sm text-color">
                                        <i class="pi pi-spin pi-spinner mt-1 text-primary" aria-hidden="true"></i>
                                        <span class="min-w-0 break-words">กำลังตรวจสอบ {{ verificationCurrentLabel }}</span>
                                    </div>
                                    <div v-else class="flex flex-wrap gap-2">
                                        <Tag :value="`พร้อม ${verificationSummary.ready}`" severity="success" />
                                        <Tag v-if="verificationSummary.notReady" :value="`ไม่พร้อม ${verificationSummary.notReady}`" severity="danger" />
                                        <Tag v-if="verificationSummary.unchecked" :value="`ยังไม่ได้ตรวจ ${verificationSummary.unchecked}`" severity="secondary" />
                                    </div>
                                </div>
                            </div>

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

                        <div v-if="step === 'database' && canProvisionSelectedDatabase" class="mb-3">
                            <Button
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
