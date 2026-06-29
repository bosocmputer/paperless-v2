<script setup>
import { api } from '@/services/api';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const docFormats = ref([]);
const configs = ref([]);
const templateStates = ref({});
const loading = ref(false);
const error = ref('');

const rows = computed(() => {
    const groups = new Map();
    configs.value.forEach((config) => {
        const code = String(config.docFormatCode || '').trim();
        if (!code) return;
        if (!groups.has(code)) {
            groups.set(code, {
                docFormatCode: code,
                positions: [],
                state: null
            });
        }
        groups.get(code).positions.push(config);
    });

    return [...groups.values()]
        .map((row) => ({
            ...row,
            format: formatDetail(row.docFormatCode),
            state: templateStates.value[row.docFormatCode] || null
        }))
        .sort((left, right) => left.docFormatCode.localeCompare(right.docFormatCode, 'th'));
});

onMounted(loadPage);

async function loadPage() {
    loading.value = true;
    error.value = '';
    try {
        const [formatsResult, configsResult] = await Promise.all([api.listSMLDocFormats(), api.listDocumentConfigs()]);
        docFormats.value = formatsResult.docFormats || [];
        configs.value = configsResult.configs || [];

        const codes = [...new Set(configs.value.map((item) => item.docFormatCode).filter(Boolean))];
        const states = {};
        await Promise.all(
            codes.map(async (code) => {
                try {
                    states[code] = await api.getSignatureTemplateState(code);
                } catch (err) {
                    states[code] = { error: err.message };
                }
            })
        );
        templateStates.value = states;
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลดรายการ Template ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openDesigner(code) {
    router.push({ name: 'signature-template', params: { docFormatCode: code } });
}

function openDocumentConfig() {
    router.push({ name: 'document-config' });
}

function formatDetail(code) {
    return docFormats.value.find((item) => sameCode(item.code, code));
}

function formatName(row) {
    return row.format?.name_1 || row.format?.name_2 || row.format?.format || '-';
}

function statusLabel(row) {
    if (row.state?.draft) return `draft v${row.state.draft.version}`;
    if (row.state?.active) return `active v${row.state.active.version}`;
    if (row.state?.error) return 'โหลดสถานะไม่ได้';
    return 'ยังไม่ได้ตั้งค่า';
}

function statusSeverity(row) {
    if (row.state?.draft) return 'warn';
    if (row.state?.active) return 'success';
    if (row.state?.error) return 'danger';
    return 'secondary';
}

function lastUpdated(row) {
    const value = row.state?.draft?.updatedAt || row.state?.active?.updatedAt;
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

function conditionSummary(row) {
    const values = new Set(row.positions.map((item) => Number(item.conditionType)));
    return [...values].sort((a, b) => a - b);
}

function conditionLabel(value) {
    if (value === 1) return 'คนใดคนหนึ่ง';
    if (value === 2) return 'ทุกคน';
    return 'บุคคลภายนอก';
}

function conditionSeverity(value) {
    if (value === 1) return 'info';
    if (value === 2) return 'warn';
    return 'secondary';
}

function sameCode(left, right) {
    return String(left || '').toLowerCase() === String(right || '').toLowerCase();
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col xl:flex-row xl:items-start justify-between gap-4 mb-6">
            <div>
                <div class="font-semibold text-xl mb-1">ตั้งค่ากรอบลายเซ็น</div>
                <p class="text-muted-color m-0">เลือกเอกสารจาก Config ที่สร้างไว้ แล้วเปิด PDF Designer เพื่อวางกรอบลายเซ็น</p>
            </div>
            <div class="flex flex-wrap gap-2">
                <Button icon="pi pi-refresh" severity="secondary" outlined :loading="loading" aria-label="โหลดใหม่" @click="loadPage" />
                <Button label="Config เอกสาร" icon="pi pi-file-edit" severity="secondary" outlined @click="openDocumentConfig" />
            </div>
        </div>

        <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

        <div v-if="loading" class="py-6 text-muted-color">กำลังโหลดรายการ Template</div>

        <div v-else-if="rows.length === 0" class="py-6 text-center text-muted-color">
            ยังไม่มี Config เอกสาร ให้ไปเพิ่ม Position ก่อน
            <Button label="ไปที่ Config เอกสาร" link class="ml-2" @click="openDocumentConfig" />
        </div>

        <div v-else class="template-list">
            <div v-for="row in rows" :key="row.docFormatCode" class="template-row">
                <div>
                    <div class="text-sm text-muted-color">erp_doc_format.code</div>
                    <div class="font-semibold text-xl text-surface-900 dark:text-surface-0">{{ row.docFormatCode }}</div>
                    <div class="text-sm text-muted-color">{{ formatName(row) }}</div>
                </div>

                <div>
                    <div class="text-sm text-muted-color">Position</div>
                    <div class="font-medium">{{ row.positions.length }} positions</div>
                    <div class="text-sm text-muted-color">{{ row.positions.map((item) => `${item.positionCode}:${item.positionName}`).join(', ') }}</div>
                </div>

                <div>
                    <div class="text-sm text-muted-color mb-2">เงื่อนไข</div>
                    <div class="flex flex-wrap gap-2">
                        <Tag v-for="condition in conditionSummary(row)" :key="condition" :value="`${condition} - ${conditionLabel(condition)}`" :severity="conditionSeverity(condition)" />
                    </div>
                </div>

                <div>
                    <div class="text-sm text-muted-color mb-2">Template</div>
                    <Tag :value="statusLabel(row)" :severity="statusSeverity(row)" />
                    <div class="text-sm text-muted-color mt-2">แก้ไขล่าสุด {{ lastUpdated(row) }}</div>
                </div>

                <div class="template-actions">
                    <Button label="เปิด Designer" icon="pi pi-pencil" severity="info" @click="openDesigner(row.docFormatCode)" />
                </div>
            </div>
        </div>
    </div>
</template>

<style scoped>
.template-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}

.template-row {
    display: grid;
    grid-template-columns: minmax(11rem, 1fr) minmax(16rem, 1.3fr) minmax(12rem, 1fr) minmax(12rem, 1fr) auto;
    gap: 1rem;
    align-items: center;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 1rem;
    background: var(--surface-card);
}

.template-actions {
    display: flex;
    justify-content: flex-end;
}

@media (max-width: 1200px) {
    .template-row {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .template-actions {
        justify-content: flex-start;
    }
}

@media (max-width: 640px) {
    .template-row {
        grid-template-columns: 1fr;
    }
}
</style>
