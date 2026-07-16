<script setup>
import { api } from '@/services/api';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const confirm = useConfirm();
const toast = useToast();
const users = ref([]);
const loading = ref(false);
const saving = ref(false);
const syncLoading = ref(false);
const syncSaving = ref(false);
const syncDialogVisible = ref(false);
const syncPreview = ref(null);
const signatureDialogVisible = ref(false);
const signaturePreviewUser = ref(null);
const signaturePreviewUrl = ref('');
const signaturePreviewLoading = ref(false);
const signaturePreviewError = ref('');
const dialogVisible = ref(false);
const editingUser = ref(null);
const error = ref('');
const syncError = ref('');
const searchQuery = ref('');
const form = ref(emptyForm());
let signaturePreviewRequestSeq = 0;
let signaturePreviewController = null;

const roleOptions = [
    { label: 'superadmin', value: 'superadmin' },
    { label: 'admin', value: 'admin' },
    { label: 'user', value: 'user' }
];

const statusOptions = [
    { label: 'active', value: 'active' },
    { label: 'inactive', value: 'inactive' }
];

const dialogTitle = computed(() => (editingUser.value ? 'แก้ไขผู้ใช้' : 'เพิ่มผู้ใช้'));
const passwordHint = computed(() => (editingUser.value ? 'เว้นว่างไว้ถ้าไม่ต้องการเปลี่ยนรหัสผ่าน' : 'รหัสผ่านอย่างน้อย 6 ตัวอักษร'));
const syncUsers = computed(() => syncPreview.value?.users || []);
const syncSignatures = computed(() => syncPreview.value?.signatures || []);
const canConfirmSync = computed(() => {
    if (!syncPreview.value || syncPreview.value.dryRun === false || syncSaving.value) return false;
    return Number(syncPreview.value.toCreate || 0) > 0 || Number(syncPreview.value.toActivate || 0) > 0 || Number(syncPreview.value.signatureNew || 0) > 0 || Number(syncPreview.value.signatureChanged || 0) > 0;
});
const filteredUsers = computed(() => {
    const query = normalizeSearch(searchQuery.value);
    if (!query) return users.value;
    return users.value.filter((user) =>
        normalizeSearch(`${user.displayName} ${user.username} ${user.role} ${user.status} ${user.accountSource} ${accountSourceLabel(user.accountSource)}`).includes(query)
    );
});

onMounted(loadUsers);
onBeforeUnmount(cleanupSignaturePreview);

function emptyForm() {
    return {
        displayName: '',
        username: '',
        password: '',
        role: 'user',
        status: 'active'
    };
}

async function loadUsers() {
    loading.value = true;
    error.value = '';
    try {
        const result = await api.listUsers();
        users.value = result.users || [];
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลดผู้ใช้ไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loading.value = false;
    }
}

async function openSyncDialog() {
    syncLoading.value = true;
    syncError.value = '';
    try {
        syncPreview.value = await api.syncSMLUsers({ dryRun: true });
        syncDialogVisible.value = true;
    } catch (err) {
        syncError.value = err.message;
        toast.add({ severity: 'error', summary: 'ตรวจผู้ใช้จาก SML ไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        syncLoading.value = false;
    }
}

async function confirmSyncSMLUsers() {
    if (!canConfirmSync.value) return;
    syncSaving.value = true;
    syncError.value = '';
    try {
        const result = await api.syncSMLUsers({ dryRun: false });
        syncPreview.value = result;
        const created = Number(result.created || 0);
        const activated = Number(result.activated || 0);
        const signatures = Number(result.signatureSynced || 0);
        const signatureFailed = Number(result.signatureFailed || 0);
        const signatureError = Boolean(result.signatureError);
        if (created > 0 || activated > 0 || signatures > 0 || signatureFailed > 0) {
            const details = [];
            if (created > 0) details.push(`เพิ่มผู้ใช้ใหม่ ${created} คน`);
            if (activated > 0) details.push(`เปิดใช้งาน ${activated} คน`);
            if (signatures > 0) details.push(`อัปเดตลายเซ็น ${signatures} คน`);
            if (signatureFailed > 0) details.push(`ลายเซ็นมีปัญหา ${signatureFailed} คน`);
            if (signatureError) details.push('ระบบลายเซ็นยังไม่พร้อม กรุณาลอง Sync อีกครั้ง');
            toast.add({ severity: signatureFailed > 0 || signatureError ? 'warn' : 'success', summary: 'Sync จาก SML เสร็จแล้ว', detail: details.join(' · '), life: 4500 });
        } else if (signatureError) {
            toast.add({ severity: 'warn', summary: 'Sync ผู้ใช้เสร็จแล้ว', detail: 'ยังตรวจสอบลายเซ็นจาก SML ไม่สำเร็จ กรุณาลองอีกครั้ง', life: 4500 });
        } else {
            toast.add({ severity: 'info', summary: 'ไม่มีผู้ใช้ใหม่จาก SML', life: 3000 });
        }
        await loadUsers();
    } catch (err) {
        syncError.value = err.message;
        toast.add({ severity: 'error', summary: 'Sync จาก SML ไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        syncSaving.value = false;
    }
}

async function openSignaturePreview(user) {
    if (!user?.savedSignature?.available) return;
    cleanupSignaturePreview();
    const requestSeq = ++signaturePreviewRequestSeq;
    signaturePreviewUser.value = user;
    signatureDialogVisible.value = true;
    signaturePreviewLoading.value = true;
    signaturePreviewError.value = '';
    signaturePreviewController = new AbortController();
    try {
        const blob = await api.getUserSavedSignatureBlob(user.id, { signal: signaturePreviewController.signal });
        if (requestSeq !== signaturePreviewRequestSeq) return;
        signaturePreviewUrl.value = URL.createObjectURL(blob);
    } catch (err) {
        if (err?.name === 'AbortError' || requestSeq !== signaturePreviewRequestSeq) return;
        signaturePreviewError.value = err.message || 'โหลดลายเซ็นไม่สำเร็จ';
    } finally {
        if (requestSeq === signaturePreviewRequestSeq) signaturePreviewLoading.value = false;
    }
}

function closeSignaturePreview() {
    signatureDialogVisible.value = false;
}

function cleanupSignaturePreview() {
    signaturePreviewRequestSeq += 1;
    signaturePreviewController?.abort();
    signaturePreviewController = null;
    if (signaturePreviewUrl.value) URL.revokeObjectURL(signaturePreviewUrl.value);
    signaturePreviewUrl.value = '';
    signaturePreviewLoading.value = false;
    signaturePreviewError.value = '';
    signaturePreviewUser.value = null;
}

function openCreate() {
    editingUser.value = null;
    form.value = emptyForm();
    dialogVisible.value = true;
}

function openEdit(user) {
    editingUser.value = user;
    form.value = {
        displayName: user.displayName,
        username: user.username,
        password: '',
        role: user.role,
        status: user.status
    };
    dialogVisible.value = true;
}

function closeDialog() {
    if (saving.value) return;
    dialogVisible.value = false;
}

async function saveUser() {
    saving.value = true;
    error.value = '';
    try {
        const payload = { ...form.value };
        if (editingUser.value && !payload.password) delete payload.password;

        if (editingUser.value) {
            await api.updateUser(editingUser.value.id, payload);
            toast.add({ severity: 'success', summary: 'บันทึกผู้ใช้แล้ว', life: 2500 });
        } else {
            await api.createUser(payload);
            toast.add({ severity: 'success', summary: 'เพิ่มผู้ใช้แล้ว', life: 2500 });
        }

        dialogVisible.value = false;
        await loadUsers();
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        saving.value = false;
    }
}

function confirmDeactivate(user) {
    confirm.require({
        message: `ปิดใช้งาน ${user.displayName} ใช่ไหม?`,
        header: 'ปิดใช้งานผู้ใช้',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'ยกเลิก',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ปิดใช้งาน',
            severity: 'danger'
        },
        accept: () => deactivateUser(user)
    });
}

async function deactivateUser(user) {
    try {
        await api.deactivateUser(user.id);
        toast.add({ severity: 'success', summary: 'ปิดใช้งานผู้ใช้แล้ว', life: 2500 });
        await loadUsers();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ปิดใช้งานไม่สำเร็จ', detail: err.message, life: 3500 });
    }
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', {
        dateStyle: 'medium',
        timeStyle: 'short'
    }).format(new Date(value));
}

function roleSeverity(role) {
    if (role === 'superadmin') return 'danger';
    return role === 'admin' ? 'success' : 'info';
}

function statusSeverity(status) {
    return status === 'active' ? 'success' : 'secondary';
}

function accountSourceLabel(source) {
    return source === 'sml' ? 'SML' : 'PaperLess';
}

function accountSourceSeverity(source) {
    return source === 'sml' ? 'info' : 'secondary';
}

function accountSourceIcon(source) {
    return source === 'sml' ? 'pi pi-database' : 'pi pi-file-edit';
}

function accountSourceHint(source) {
    return source === 'sml' ? 'บัญชีที่เชื่อมต่อหรือ Sync มาจาก SML' : 'บัญชีที่สร้างภายใน PaperLess';
}

function savedSignatureLabel(user) {
    if (user?.savedSignature?.available && user?.savedSignature?.lastError) return 'พร้อมใช้ (รูปเดิม)';
    if (user?.savedSignature?.available) return 'พร้อมใช้';
    if (savedSignatureIssueType(user) === 'missing') return 'ไม่มีลายเซ็นใน SML';
    if (savedSignatureIssueType(user) === 'invalid') return 'รูปลายเซ็นใช้ไม่ได้';
    if (savedSignatureIssueType(user) === 'failed') return 'Sync ลายเซ็นมีปัญหา';
    return 'ยังไม่มีข้อมูลลายเซ็น';
}

function savedSignatureSeverity(user) {
    if (user?.savedSignature?.available && user?.savedSignature?.lastError) return 'warn';
    if (user?.savedSignature?.available) return 'success';
    if (savedSignatureIssueType(user) === 'invalid') return 'warn';
    if (savedSignatureIssueType(user) === 'failed') return 'danger';
    return 'secondary';
}

function savedSignatureIssueType(user) {
    const issue = String(user?.savedSignature?.lastError || '').trim().toLowerCase();
    if (!issue) return 'none';
    if (issue === 'signature_missing') return 'missing';
    if (['signature_invalid', 'signature_content_type_invalid', 'signature_normalize_failed'].includes(issue)) return 'invalid';
    return 'failed';
}

function savedSignatureHint(user) {
    if (!user?.savedSignature?.available || !user?.savedSignature?.lastError) return '';
    if (savedSignatureIssueType(user) === 'missing') return 'ไม่พบลายเซ็นใน SML จึงคงรูปเดิมไว้';
    if (savedSignatureIssueType(user) === 'invalid') return 'รูปใหม่ใน SML ใช้ไม่ได้ จึงคงรูปเดิมไว้';
    return 'อัปเดตล่าสุดไม่สำเร็จ จึงคงรูปเดิมไว้';
}

function signatureSyncLabel(status) {
    return {
        new: 'ลายเซ็นใหม่',
        changed: 'มีการเปลี่ยนแปลง',
        unchanged: 'เป็นรุ่นล่าสุดแล้ว',
        missing: 'ไม่มีรูปใน SML',
        invalid: 'รูปใช้ไม่ได้',
        synced: 'Sync สำเร็จ',
        failed: 'Sync ไม่สำเร็จ'
    }[status] || status || '-';
}

function signatureSyncSeverity(status) {
    if (status === 'synced' || status === 'unchanged') return 'success';
    if (status === 'new' || status === 'changed') return 'info';
    if (status === 'missing') return 'secondary';
    return 'warn';
}

function syncTenantLabel(result) {
    if (!result) return '-';
    return [result.dataCode, result.dataName].filter(Boolean).join(' · ') || result.tenant || '-';
}

function normalizeSearch(value) {
    return String(value || '').toLowerCase().trim();
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div>
                <div class="font-semibold text-xl mb-1">ผู้ใช้งาน</div>
                <p class="text-muted-color m-0">จัดการชื่อผู้ใช้ รหัสผ่าน และระดับสิทธิ์ superadmin/admin/user</p>
            </div>
            <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหา user, ชื่อ, สิทธิ์, แหล่งบัญชี" class="w-full sm:w-80" />
                <Button label="Sync จาก SML" icon="pi pi-sync" severity="secondary" outlined :loading="syncLoading" @click="openSyncDialog" />
                <Button label="เพิ่มผู้ใช้" icon="pi pi-plus" @click="openCreate" />
            </div>
        </div>

        <Message v-if="error && !dialogVisible" severity="error" class="mb-4">{{ error }}</Message>

        <DataTable :value="filteredUsers" :loading="loading" dataKey="id" paginator :rows="10" responsiveLayout="scroll" stripedRows>
            <template #empty>
                <div class="py-6 text-center text-muted-color">{{ searchQuery ? 'ไม่พบผู้ใช้ที่ค้นหา' : 'ยังไม่มีผู้ใช้' }}</div>
            </template>
            <Column field="displayName" header="ชื่อ" sortable>
                <template #body="{ data }">
                    <div class="font-medium text-surface-900 dark:text-surface-0">{{ data.displayName }}</div>
                    <div class="text-sm text-muted-color">@{{ data.username }}</div>
                </template>
            </Column>
            <Column field="username" header="Username" sortable />
            <Column field="role" header="ระดับ" sortable>
                <template #body="{ data }">
                    <Tag :value="data.role" :severity="roleSeverity(data.role)" />
                </template>
            </Column>
            <Column field="status" header="สถานะ" sortable>
                <template #body="{ data }">
                    <Tag :value="data.status" :severity="statusSeverity(data.status)" />
                </template>
            </Column>
            <Column field="accountSource" header="แหล่งบัญชี" sortable>
                <template #body="{ data }">
                    <Tag
                        :value="accountSourceLabel(data.accountSource)"
                        :severity="accountSourceSeverity(data.accountSource)"
                        :icon="accountSourceIcon(data.accountSource)"
                        :title="accountSourceHint(data.accountSource)"
                    />
                </template>
            </Column>
            <Column header="ลายเซ็น SML">
                <template #body="{ data }">
                    <div class="flex flex-col gap-1 items-start">
                        <div class="flex items-center gap-1">
                            <Tag :value="savedSignatureLabel(data)" :severity="savedSignatureSeverity(data)" />
                            <Button
                                v-if="data.savedSignature?.available"
                                icon="pi pi-eye"
                                severity="secondary"
                                text
                                rounded
                                size="small"
                                :aria-label="`ดูลายเซ็นของ ${data.displayName}`"
                                title="ดูลายเซ็น"
                                @click="openSignaturePreview(data)"
                            />
                        </div>
                        <small v-if="data.savedSignature?.syncedAt" class="text-muted-color">{{ formatDate(data.savedSignature.syncedAt) }}</small>
                        <small v-if="savedSignatureHint(data)" class="text-orange-600">{{ savedSignatureHint(data) }}</small>
                    </div>
                </template>
            </Column>
            <Column field="createdAt" header="วันที่สร้าง" sortable>
                <template #body="{ data }">{{ formatDate(data.createdAt) }}</template>
            </Column>
            <Column header="จัดการ" style="width: 10rem">
                <template #body="{ data }">
                    <div class="flex gap-2">
                        <Button icon="pi pi-pencil" severity="secondary" rounded outlined aria-label="แก้ไขผู้ใช้" @click="openEdit(data)" />
                        <Button icon="pi pi-ban" severity="danger" rounded outlined aria-label="ปิดใช้งานผู้ใช้" :disabled="data.status !== 'active'" @click="confirmDeactivate(data)" />
                    </div>
                </template>
            </Column>
        </DataTable>
    </div>

    <Dialog v-model:visible="dialogVisible" modal :header="dialogTitle" :style="{ width: 'min(42rem, 92vw)' }" @hide="closeDialog">
        <form class="flex flex-col gap-4" @submit.prevent="saveUser">
            <Message v-if="error && dialogVisible" severity="error">{{ error }}</Message>

            <div class="flex flex-col gap-2">
                <label for="displayName" class="font-medium">ชื่อ</label>
                <InputText id="displayName" v-model="form.displayName" autocomplete="name" />
            </div>

            <div class="flex flex-col gap-2">
                <label for="username" class="font-medium">Username</label>
                <InputText id="username" v-model="form.username" autocomplete="username" />
            </div>

            <div class="flex flex-col gap-2">
                <label for="password" class="font-medium">Password</label>
                <Password id="password" v-model="form.password" :feedback="false" toggleMask fluid autocomplete="new-password" />
                <small class="text-muted-color">{{ passwordHint }}</small>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div class="flex flex-col gap-2">
                    <label for="role" class="font-medium">ระดับ</label>
                    <Select id="role" v-model="form.role" :options="roleOptions" optionLabel="label" optionValue="value" />
                </div>
                <div class="flex flex-col gap-2">
                    <label for="status" class="font-medium">สถานะ</label>
                    <Select id="status" v-model="form.status" :options="statusOptions" optionLabel="label" optionValue="value" />
                </div>
            </div>

            <div class="flex justify-end gap-2 pt-2">
                <Button type="button" label="ยกเลิก" severity="secondary" outlined @click="closeDialog" />
                <Button type="submit" label="บันทึกผู้ใช้" icon="pi pi-save" :loading="saving" />
            </div>
        </form>
    </Dialog>

    <Dialog v-model:visible="syncDialogVisible" modal header="Sync ผู้ใช้และลายเซ็นจาก SML" :style="{ width: 'min(64rem, 94vw)' }">
        <div class="flex flex-col gap-4">
            <Message v-if="syncError" severity="error">{{ syncError }}</Message>
            <Message v-if="syncPreview?.signatureError" severity="warn" :closable="false">
                ระบบลายเซ็นจาก SML ยังไม่พร้อม ข้อมูลผู้ใช้ยัง Sync ได้ตามปกติ กรุณาลอง Sync ลายเซ็นอีกครั้งภายหลัง
            </Message>
            <Message v-if="syncPreview && !syncPreview.dryRun" severity="success" :closable="false">
                Sync เสร็จแล้ว เพิ่มผู้ใช้ {{ syncPreview.created || 0 }} คน เปิดใช้งาน {{ syncPreview.activated || 0 }} คน และอัปเดตลายเซ็น {{ syncPreview.signatureSynced || 0 }} คน
            </Message>
            <Message v-else-if="syncPreview && syncPreview.passwordNotSynced > 0" severity="warn" :closable="false">
                มี {{ syncPreview.passwordNotSynced }} user ที่ไม่สามารถ sync รหัส local ได้ ระบบยังให้ login ผ่าน SML ได้ตามปกติ
            </Message>

            <div class="grid grid-cols-2 md:grid-cols-6 gap-3">
                <div>
                    <div class="text-sm text-muted-color">Database</div>
                    <div class="font-semibold">{{ syncTenantLabel(syncPreview) }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">มีสิทธิ์ทั้งหมด</div>
                    <div class="font-semibold">{{ syncPreview?.totalAllowed ?? '-' }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">มีใน PaperLess แล้ว</div>
                    <div class="font-semibold">{{ syncPreview?.existing ?? '-' }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">จะเพิ่ม</div>
                    <div class="font-semibold text-primary">{{ syncPreview?.toCreate ?? '-' }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">จะเปิดใช้งาน</div>
                    <div class="font-semibold text-green-600">{{ syncPreview?.toActivate ?? '-' }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">SML inactive</div>
                    <div class="font-semibold">{{ syncPreview?.skippedInactive ?? '-' }}</div>
                </div>
            </div>

            <div class="grid grid-cols-2 md:grid-cols-5 gap-3 surface-ground border-round p-3">
                <div>
                    <div class="text-sm text-muted-color">มีรูปใน SML</div>
                    <div class="font-semibold">{{ syncPreview?.signatureAvailable ?? '-' }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">ลายเซ็นใหม่</div>
                    <div class="font-semibold text-primary">{{ syncPreview?.signatureNew ?? '-' }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">มีการเปลี่ยนแปลง</div>
                    <div class="font-semibold text-orange-600">{{ syncPreview?.signatureChanged ?? '-' }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">เป็นรุ่นล่าสุด</div>
                    <div class="font-semibold text-green-600">{{ syncPreview?.signatureUnchanged ?? '-' }}</div>
                </div>
                <div>
                    <div class="text-sm text-muted-color">ไม่มี/รูปใช้ไม่ได้</div>
                    <div class="font-semibold">{{ Number(syncPreview?.signatureMissing || 0) + Number(syncPreview?.signatureInvalid || 0) }}</div>
                </div>
            </div>

            <DataTable :value="syncUsers" dataKey="username" responsiveLayout="scroll" :rows="8" paginator size="small">
                <template #empty>
                    <div class="py-5 text-center text-muted-color">ไม่มีผู้ใช้ใหม่จาก SML ที่ต้องเพิ่ม</div>
                </template>
                <Column field="displayName" header="ชื่อ">
                    <template #body="{ data }">
                        <div class="font-medium">{{ data.displayName }}</div>
                        <div class="text-sm text-muted-color">@{{ data.username }}</div>
                    </template>
                </Column>
                <Column field="username" header="Username" />
                <Column header="ระดับ">
                    <template #body>
                        <Tag value="admin" severity="success" />
                    </template>
                </Column>
                <Column header="รหัสผ่าน">
                    <template #body="{ data }">
                        <Tag :value="data.passwordSynced ? 'ตรงกับ SML' : 'ใช้ผ่าน SML เท่านั้น'" :severity="data.passwordSynced ? 'success' : 'warn'" />
                    </template>
                </Column>
            </DataTable>

            <div>
                <div class="font-semibold mb-2">สถานะลายเซ็น</div>
                <DataTable :value="syncSignatures" dataKey="username" responsiveLayout="scroll" :rows="8" paginator size="small">
                    <template #empty>
                        <div class="py-5 text-center text-muted-color">ไม่พบข้อมูลลายเซ็นจาก SML</div>
                    </template>
                    <Column field="displayName" header="ผู้ใช้">
                        <template #body="{ data }">
                            <div class="font-medium">{{ data.displayName || data.username }}</div>
                            <div class="text-sm text-muted-color">@{{ data.username }}</div>
                        </template>
                    </Column>
                    <Column header="สถานะ">
                        <template #body="{ data }">
                            <Tag :value="signatureSyncLabel(data.status)" :severity="signatureSyncSeverity(data.status)" />
                        </template>
                    </Column>
                    <Column header="ข้อมูลเดิม">
                        <template #body="{ data }">{{ data.previousExists ? 'มีลายเซ็นเดิม' : '-' }}</template>
                    </Column>
                    <Column field="issue" header="หมายเหตุ">
                        <template #body="{ data }"><span class="text-sm text-muted-color">{{ data.issue || '-' }}</span></template>
                    </Column>
                </DataTable>
            </div>
        </div>
        <template #footer>
            <Button label="ปิด" icon="pi pi-times" text severity="secondary" :disabled="syncSaving" @click="syncDialogVisible = false" />
            <Button label="ยืนยัน Sync" icon="pi pi-check" :loading="syncSaving" :disabled="!canConfirmSync" @click="confirmSyncSMLUsers" />
        </template>
    </Dialog>

    <Dialog
        v-model:visible="signatureDialogVisible"
        modal
        :header="`ลายเซ็นของ ${signaturePreviewUser?.displayName || ''}`"
        :style="{ width: 'min(46rem, 94vw)' }"
        @hide="cleanupSignaturePreview"
    >
        <div class="flex flex-col gap-3">
            <div v-if="signaturePreviewLoading" class="flex items-center justify-center surface-ground border-round min-h-64">
                <ProgressSpinner aria-label="กำลังโหลดลายเซ็น" />
            </div>
            <Message v-else-if="signaturePreviewError" severity="error" :closable="false">{{ signaturePreviewError }}</Message>
            <div v-else-if="signaturePreviewUrl" class="flex items-center justify-center surface-ground border-round p-4 min-h-64">
                <img
                    :src="signaturePreviewUrl"
                    :alt="`ลายเซ็นของ ${signaturePreviewUser?.displayName || ''}`"
                    class="max-w-full object-contain"
                    style="max-height: 55vh"
                />
            </div>
            <div v-if="signaturePreviewUser?.savedSignature?.syncedAt" class="text-sm text-muted-color">
                Sync ล่าสุด {{ formatDate(signaturePreviewUser.savedSignature.syncedAt) }} · {{ signaturePreviewUser.username }}
            </div>
        </div>
        <template #footer>
            <Button label="ปิด" severity="secondary" outlined @click="closeSignaturePreview" />
        </template>
    </Dialog>
</template>
