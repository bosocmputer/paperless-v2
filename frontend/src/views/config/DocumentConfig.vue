<script setup>
import { api } from '@/services/api';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const confirm = useConfirm();
const toast = useToast();
const router = useRouter();

const conditionOptions = [
    { label: '1 - คนใดคนหนึ่ง', value: 1, severity: 'info' },
    { label: '2 - ทุกคน', value: 2, severity: 'warn' },
    { label: '3 - บุคคลภายนอก', value: 3, severity: 'secondary' }
];

const docFormats = ref([]);
const users = ref([]);
const configs = ref([]);
const loadingFormats = ref(false);
const loadingUsers = ref(false);
const loadingConfigs = ref(false);
const saving = ref(false);
const dialogVisible = ref(false);
const editingConfig = ref(null);
const error = ref('');
const form = ref(emptyForm());

const docFormatOptions = computed(() =>
    docFormats.value.map((format) => ({
        label: `${format.code} - ${format.name_1 || format.name_2 || format.format || 'ไม่มีชื่อเอกสาร'}`,
        value: format.code,
        format
    }))
);

const userOptions = computed(() => {
    const options = users.value
        .filter((user) => user.status === 'active')
        .map((user) => ({
            label: `${user.username}:${user.displayName}`,
            value: userValue(user),
            user
        }))
        .sort((left, right) => left.value.localeCompare(right.value, 'th'));

    const existingValues = new Set(options.map((option) => option.value));
    [...configs.value, form.value].forEach((item) => {
        [item.user01, item.user02, item.user03].forEach((value) => {
            const normalized = String(value || '').trim();
            if (normalized && !existingValues.has(normalized)) {
                existingValues.add(normalized);
                options.push({ label: normalized, value: normalized });
            }
        });
    });

    return options;
});

const dialogTitle = computed(() => (editingConfig.value ? 'แก้ไข Config เอกสาร' : 'เพิ่ม Config เอกสาร'));
const canAdd = computed(() => !loadingFormats.value && !loadingUsers.value && docFormatOptions.value.length > 0 && userOptions.value.length > 0);
const loadingPage = computed(() => loadingFormats.value || loadingUsers.value || loadingConfigs.value);

onMounted(initializePage);

function emptyForm(docFormatCode = '') {
    const code = docFormatCode || docFormats.value[0]?.code || '';
    return {
        docFormatCode: code,
        positionCode: '',
        positionName: '',
        user01: '',
        user02: '',
        user03: '',
        sequenceNo: nextSequenceNo(code),
        conditionType: 1
    };
}

function nextSequenceNo(docFormatCode = '') {
    const max = configs.value.reduce((current, item) => {
        if (docFormatCode && !sameCode(item.docFormatCode, docFormatCode)) return current;
        return Math.max(current, Number(item.sequenceNo || 0));
    }, 0);
    return max + 1;
}

async function initializePage() {
    await Promise.all([loadDocFormats(), loadUsers(), loadConfigs()]);
}

async function loadDocFormats() {
    loadingFormats.value = true;
    error.value = '';
    try {
        const result = await api.listSMLDocFormats();
        docFormats.value = result.docFormats || [];
        if (!form.value.docFormatCode && docFormats.value.length > 0) {
            form.value.docFormatCode = docFormats.value[0].code;
        }
    } catch (err) {
        docFormats.value = [];
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลด Doc Format ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loadingFormats.value = false;
    }
}

async function loadUsers() {
    loadingUsers.value = true;
    error.value = '';
    try {
        const result = await api.listUsers();
        users.value = result.users || [];
    } catch (err) {
        users.value = [];
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลด User ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loadingUsers.value = false;
    }
}

async function loadConfigs() {
    loadingConfigs.value = true;
    error.value = '';
    try {
        const result = await api.listDocumentConfigs();
        configs.value = result.configs || [];
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลด Config ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loadingConfigs.value = false;
    }
}

function openCreate() {
    editingConfig.value = null;
    form.value = emptyForm();
    dialogVisible.value = true;
}

function openEdit(config) {
    editingConfig.value = config;
    form.value = {
        docFormatCode: config.docFormatCode,
        positionCode: config.positionCode,
        positionName: config.positionName,
        user01: config.user01,
        user02: config.user02,
        user03: config.user03,
        sequenceNo: Number(config.sequenceNo),
        conditionType: config.conditionType
    };
    dialogVisible.value = true;
}

function openSignatureTemplate(docFormatCode) {
    router.push({ name: 'signature-template', params: { docFormatCode } });
}

function handleDocFormatChange() {
    if (editingConfig.value) return;
    form.value.sequenceNo = nextSequenceNo(form.value.docFormatCode);
}

function closeDialog() {
    if (saving.value) return;
    dialogVisible.value = false;
}

async function saveConfig() {
    saving.value = true;
    error.value = '';
    try {
        const payload = {
            docFormatCode: form.value.docFormatCode,
            positionCode: form.value.positionCode,
            positionName: form.value.positionName,
            user01: form.value.user01 || '',
            user02: form.value.user02 || '',
            user03: form.value.user03 || '',
            sequenceNo: Number(form.value.sequenceNo),
            conditionType: Number(form.value.conditionType)
        };

        if (editingConfig.value) {
            await api.updateDocumentConfig(editingConfig.value.id, payload);
            toast.add({ severity: 'success', summary: 'บันทึก Config แล้ว', life: 2500 });
        } else {
            await api.createDocumentConfig(payload);
            toast.add({ severity: 'success', summary: 'เพิ่ม Config แล้ว', life: 2500 });
        }

        dialogVisible.value = false;
        await loadConfigs();
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        saving.value = false;
    }
}

function confirmDelete(config) {
    confirm.require({
        message: `ลบ Position ${config.positionCode} (${config.positionName}) ใช่ไหม?`,
        header: 'ลบ Config เอกสาร',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'ยกเลิก',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ลบ Config',
            severity: 'danger'
        },
        accept: () => deleteConfig(config)
    });
}

async function deleteConfig(config) {
    try {
        await api.deleteDocumentConfig(config.id);
        toast.add({ severity: 'success', summary: 'ลบ Config แล้ว', life: 2500 });
        await loadConfigs();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ลบไม่สำเร็จ', detail: err.message, life: 4000 });
    }
}

function conditionLabel(value) {
    return conditionOptions.find((option) => option.value === Number(value))?.label || value;
}

function conditionSeverity(value) {
    return conditionOptions.find((option) => option.value === Number(value))?.severity || 'secondary';
}

function formatDetail(code) {
    return docFormats.value.find((item) => sameCode(item.code, code));
}

function formatName(code) {
    const format = formatDetail(code);
    return format?.name_1 || format?.name_2 || format?.format || '-';
}

function formatPattern(code) {
    return formatDetail(code)?.format || 'ไม่มี format';
}

function sameCode(left, right) {
    return String(left || '').toLowerCase() === String(right || '').toLowerCase();
}

function userValue(user) {
    return `${String(user.username || '').trim()}:${String(user.displayName || '').trim()}`;
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col xl:flex-row xl:items-start justify-between gap-4 mb-6">
            <div>
                <div class="font-semibold text-xl mb-1">Config เอกสาร</div>
                <p class="text-muted-color m-0">กำหนดลำดับ Position และผู้รับเอกสารตาม erp_doc_format จาก SML</p>
            </div>
            <div class="flex gap-2">
                <Button icon="pi pi-refresh" severity="secondary" outlined :loading="loadingPage" aria-label="โหลดใหม่" @click="initializePage" />
                <Button label="ตั้งค่ากรอบลายเซ็น" icon="pi pi-pencil" severity="secondary" outlined @click="router.push({ name: 'signature-templates' })" />
                <Button label="เพิ่ม Position" icon="pi pi-plus" :disabled="!canAdd" @click="openCreate" />
            </div>
        </div>

        <Message v-if="error && !dialogVisible" severity="error" class="mb-4">{{ error }}</Message>

        <DataTable :value="configs" :loading="loadingConfigs" dataKey="id" paginator :rows="10" responsiveLayout="scroll" stripedRows sortField="sequenceNo" :sortOrder="1">
            <template #empty>
                <div class="py-6 text-center text-muted-color">
                    {{ loadingFormats ? 'กำลังโหลด Doc Format จาก SML' : 'ยังไม่มี Config เอกสาร' }}
                </div>
            </template>
            <Column field="docFormatCode" header="erp_doc_format.code" sortable style="min-width: 13rem">
                <template #body="{ data }">
                    <div class="font-medium text-surface-900 dark:text-surface-0">{{ data.docFormatCode }}</div>
                    <div class="text-sm text-muted-color">{{ formatName(data.docFormatCode) }}</div>
                    <div class="text-xs text-muted-color">{{ formatPattern(data.docFormatCode) }}</div>
                </template>
            </Column>
            <Column field="positionCode" header="รหัส Position" sortable style="min-width: 9rem" />
            <Column field="positionName" header="ชื่อ Position" sortable style="min-width: 12rem" />
            <Column field="user01" header="User01" style="min-width: 12rem" />
            <Column field="user02" header="User02" style="min-width: 12rem">
                <template #body="{ data }">{{ data.user02 || '-' }}</template>
            </Column>
            <Column field="user03" header="User03" style="min-width: 12rem">
                <template #body="{ data }">{{ data.user03 || '-' }}</template>
            </Column>
            <Column field="sequenceNo" header="ลำดับ" sortable style="min-width: 8rem">
                <template #body="{ data }">{{ Number(data.sequenceNo).toFixed(2) }}</template>
            </Column>
            <Column field="conditionType" header="เงื่อนไข" sortable style="min-width: 12rem">
                <template #body="{ data }">
                    <Tag :value="conditionLabel(data.conditionType)" :severity="conditionSeverity(data.conditionType)" />
                </template>
            </Column>
            <Column header="จัดการ" style="min-width: 18rem">
                <template #body="{ data }">
                    <div class="flex flex-wrap gap-2">
                        <Button icon="pi pi-pencil" severity="secondary" rounded outlined aria-label="แก้ไข Config เอกสาร" @click="openEdit(data)" />
                        <Button label="กรอบลายเซ็น" icon="pi pi-pencil" severity="info" outlined @click="openSignatureTemplate(data.docFormatCode)" />
                        <Button icon="pi pi-trash" severity="danger" rounded outlined aria-label="ลบ Config เอกสาร" @click="confirmDelete(data)" />
                    </div>
                </template>
            </Column>
        </DataTable>
    </div>

    <Dialog v-model:visible="dialogVisible" modal :header="dialogTitle" :style="{ width: 'min(54rem, 94vw)' }" @hide="closeDialog">
        <form class="flex flex-col gap-4" @submit.prevent="saveConfig">
            <Message v-if="error && dialogVisible" severity="error">{{ error }}</Message>

            <div class="flex flex-col gap-2">
                <label for="dialogDocFormat" class="font-medium">erp_doc_format.code</label>
                <Select
                    id="dialogDocFormat"
                    v-model="form.docFormatCode"
                    :options="docFormatOptions"
                    optionLabel="label"
                    optionValue="value"
                    :loading="loadingFormats"
                    :disabled="loadingFormats || docFormatOptions.length === 0"
                    filter
                    @change="handleDocFormatChange"
                >
                    <template #value="{ value, placeholder }">
                        <span v-if="value">{{ value }} - {{ formatName(value) }}</span>
                        <span v-else>{{ placeholder }}</span>
                    </template>
                    <template #option="{ option }">
                        <div class="flex flex-col">
                            <span class="font-medium">{{ option.format.code }} - {{ option.format.name_1 || option.format.name_2 || '-' }}</span>
                            <span class="text-sm text-muted-color">{{ option.format.format || 'ไม่มี format' }}</span>
                        </div>
                    </template>
                </Select>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div class="flex flex-col gap-2">
                    <label for="positionCode" class="font-medium">รหัส Position</label>
                    <InputText id="positionCode" v-model="form.positionCode" autocomplete="off" />
                </div>
                <div class="flex flex-col gap-2">
                    <label for="positionName" class="font-medium">ชื่อ Position</label>
                    <InputText id="positionName" v-model="form.positionName" autocomplete="off" />
                </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div class="flex flex-col gap-2">
                    <label for="user01" class="font-medium">User01</label>
                    <Select
                        id="user01"
                        v-model="form.user01"
                        :options="userOptions"
                        optionLabel="label"
                        optionValue="value"
                        :loading="loadingUsers"
                        :disabled="loadingUsers || userOptions.length === 0"
                        filter
                        placeholder="เลือก User01"
                    />
                </div>
                <div class="flex flex-col gap-2">
                    <label for="user02" class="font-medium">User02</label>
                    <Select
                        id="user02"
                        v-model="form.user02"
                        :options="userOptions"
                        optionLabel="label"
                        optionValue="value"
                        :loading="loadingUsers"
                        :disabled="loadingUsers || userOptions.length === 0"
                        filter
                        showClear
                        placeholder="เลือก User02"
                    />
                </div>
                <div class="flex flex-col gap-2">
                    <label for="user03" class="font-medium">User03</label>
                    <Select
                        id="user03"
                        v-model="form.user03"
                        :options="userOptions"
                        optionLabel="label"
                        optionValue="value"
                        :loading="loadingUsers"
                        :disabled="loadingUsers || userOptions.length === 0"
                        filter
                        showClear
                        placeholder="เลือก User03"
                    />
                </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div class="flex flex-col gap-2">
                    <label for="sequenceNo" class="font-medium">ลำดับ</label>
                    <InputNumber id="sequenceNo" v-model="form.sequenceNo" :min="0.01" :minFractionDigits="2" :maxFractionDigits="2" mode="decimal" />
                </div>
                <div class="flex flex-col gap-2">
                    <label for="conditionType" class="font-medium">เงื่อนไข</label>
                    <Select id="conditionType" v-model="form.conditionType" :options="conditionOptions" optionLabel="label" optionValue="value" />
                </div>
            </div>

            <Message v-if="form.conditionType === 3" severity="info">บุคคลภายนอกใช้ User01 เช่น 999:Temp user หรือชื่อผู้รับภายนอก</Message>

            <div class="flex justify-end gap-2 pt-2">
                <Button type="button" label="ยกเลิก" severity="secondary" outlined @click="closeDialog" />
                <Button type="submit" label="บันทึก Config" icon="pi pi-save" :loading="saving" />
            </div>
        </form>
    </Dialog>
</template>
