<script setup>
import { api } from '@/services/api';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const confirm = useConfirm();
const toast = useToast();

const workflows = ref([]);
const docFormats = ref([]);
const loading = ref(false);
const loadingFormats = ref(false);
const error = ref('');
const searchQuery = ref('');
const createVisible = ref(false);
const selectedNewDocFormat = ref('');
const copyVisible = ref(false);
const copyTarget = ref(null);
const copySourceDocFormat = ref('');
const copyPreview = ref(null);
const loadingCopyPreview = ref(false);
const copying = ref(false);

const configuredCodes = computed(() => new Set(workflows.value.map((item) => normalizeCode(item.docFormatCode))));
const availableDocFormatOptions = computed(() =>
    docFormats.value
        .filter((format) => format.code && !configuredCodes.value.has(normalizeCode(format.code)))
        .map((format) => ({
            label: `${format.code} - ${docFormatName(format)}`,
            value: format.code
        }))
        .sort((left, right) => left.value.localeCompare(right.value, 'th'))
);
const workflowOptions = computed(() =>
    workflows.value
        .filter((workflow) => !copyTarget.value || !sameCode(workflow.docFormatCode, copyTarget.value.docFormatCode))
        .map((workflow) => ({
            label: `${workflow.docFormatCode} - ${docFormatName(workflow.docFormat)} (${workflow.stepCount} ขั้นตอน)`,
            value: workflow.docFormatCode
        }))
);
const filteredWorkflows = computed(() => {
    const query = normalizeSearch(searchQuery.value);
    if (!query) return workflows.value;
    return workflows.value.filter((workflow) =>
        normalizeSearch(`${workflow.docFormatCode} ${docFormatName(workflow.docFormat)} ${workflow.screenCode} ${conditionSummaryText(workflow)} ${workflow.stepCount}`).includes(query)
    );
});

onMounted(loadPage);

async function loadPage() {
    loading.value = true;
    error.value = '';
    try {
        const [workflowResult] = await Promise.all([api.listDocumentConfigWorkflows(), loadDocFormats()]);
        workflows.value = workflowResult.workflows || [];
        if (workflowResult.smlWarning) {
            toast.add({ severity: 'warn', summary: 'โหลดชื่อเอกสารจาก SML ไม่ครบ', detail: workflowResult.smlWarning, life: 4500 });
        }
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลด Workflow ไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        loading.value = false;
    }
}

async function loadDocFormats() {
    loadingFormats.value = true;
    try {
        const result = await api.listSMLDocFormats();
        docFormats.value = result.docFormats || [];
    } catch (err) {
        docFormats.value = [];
        toast.add({ severity: 'warn', summary: 'โหลด Doc Format จาก SML ไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        loadingFormats.value = false;
    }
}

function openWorkflow(docFormatCode) {
    router.push({ name: 'document-config-workflow', params: { docFormatCode } });
}

function openPreset(docFormatCode) {
    router.push({ name: 'signature-template', params: { docFormatCode } });
}

function openCreate() {
    selectedNewDocFormat.value = availableDocFormatOptions.value[0]?.value || '';
    createVisible.value = true;
}

function createWorkflow() {
    if (!selectedNewDocFormat.value) return;
    createVisible.value = false;
    openWorkflow(selectedNewDocFormat.value);
}

function openCopy(workflow) {
    copyTarget.value = workflow;
    copySourceDocFormat.value = workflowOptions.value[0]?.value || '';
    copyPreview.value = null;
    copyVisible.value = true;
    if (copySourceDocFormat.value) loadCopyPreview();
}

async function loadCopyPreview() {
    if (!copySourceDocFormat.value) {
        copyPreview.value = null;
        return;
    }
    loadingCopyPreview.value = true;
    try {
        const result = await api.getDocumentConfigWorkflow(copySourceDocFormat.value);
        copyPreview.value = result.workflow;
    } catch (err) {
        copyPreview.value = null;
        toast.add({ severity: 'error', summary: 'โหลด Preview ไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loadingCopyPreview.value = false;
    }
}

function confirmCopy() {
    if (!copyTarget.value || !copySourceDocFormat.value || !copyPreview.value) return;
    confirm.require({
        message: `คัดลอก Workflow จาก ${copySourceDocFormat.value} มาแทน ${copyTarget.value.docFormatCode}? ขั้นตอนเดิมของ ${copyTarget.value.docFormatCode} จะถูกแทนที่ทั้งหมด`,
        header: 'ยืนยันคัดลอก Workflow',
        icon: 'pi pi-copy',
        rejectProps: {
            label: 'ยกเลิก',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'คัดลอก Workflow',
            severity: 'warn'
        },
        accept: copyWorkflow
    });
}

async function copyWorkflow() {
    if (!copyTarget.value || !copySourceDocFormat.value) return;
    copying.value = true;
    try {
        await api.copyDocumentConfigWorkflow(copyTarget.value.docFormatCode, {
            sourceDocFormatCode: copySourceDocFormat.value,
            revision: copyTarget.value.revision
        });
        toast.add({ severity: 'success', summary: 'คัดลอก Workflow แล้ว', life: 2500 });
        copyVisible.value = false;
        await loadPage();
    } catch (err) {
        const severity = err.status === 409 ? 'warn' : 'error';
        toast.add({ severity, summary: err.status === 409 ? 'มีคนแก้ Workflow นี้แล้ว' : 'คัดลอกไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        copying.value = false;
    }
}

function docFormatName(format = {}) {
    return format.name_1 || format.name_2 || format.format || 'ไม่มีชื่อเอกสาร';
}

function conditionSummary(workflow) {
    const counts = workflow.conditionCounts || {};
    return [
        { label: 'คนใดคนหนึ่ง', value: Number(counts['1'] || 0), severity: 'info' },
        { label: 'ทุกคน', value: Number(counts['2'] || 0), severity: 'warn' },
        { label: 'บุคคลภายนอก', value: Number(counts['3'] || 0), severity: 'secondary' }
    ].filter((item) => item.value > 0);
}

function conditionSummaryText(workflow) {
    return conditionSummary(workflow)
        .map((item) => `${item.label} ${item.value}`)
        .join(' ');
}

function formatDateTime(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    }).format(new Date(value));
}

function normalizeSearch(value) {
    return String(value || '').trim().toLowerCase();
}

function normalizeCode(value) {
    return String(value || '').trim().toUpperCase();
}

function sameCode(left, right) {
    return normalizeCode(left) === normalizeCode(right);
}
</script>

<template>
    <div class="card">
        <Toolbar class="mb-6">
            <template #start>
                <Button label="เพิ่ม Workflow" icon="pi pi-plus" severity="secondary" :disabled="loading || loadingFormats || availableDocFormatOptions.length === 0" @click="openCreate" />
            </template>
            <template #end>
                <Button label="โหลดใหม่" icon="pi pi-refresh" severity="secondary" outlined :loading="loading" @click="loadPage" />
            </template>
        </Toolbar>

        <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>
        <Message v-if="availableDocFormatOptions.length === 0 && !loading && docFormats.length > 0" severity="info" class="mb-4" :closable="false">
            Doc Format จาก SML ถูกตั้งค่า Workflow ครบแล้ว
        </Message>

        <DataTable :value="filteredWorkflows" :loading="loading" dataKey="docFormatCode" paginator :rows="10" responsiveLayout="scroll" stripedRows>
            <template #header>
                <div class="flex flex-wrap gap-2 items-center justify-between">
                    <div>
                        <h4 class="m-0">ตั้งค่า Workflow เอกสาร</h4>
                        <p class="text-muted-color m-0 mt-1">1 รายการต่อ Doc Format ใช้กับเอกสารใหม่เท่านั้น</p>
                    </div>
                    <IconField>
                        <InputIcon>
                            <i class="pi pi-search" />
                        </InputIcon>
                        <InputText v-model="searchQuery" placeholder="ค้นหา Doc Format หรือชื่อเอกสาร" />
                    </IconField>
                </div>
            </template>

            <template #empty>
                <div class="py-6 text-center text-muted-color">{{ searchQuery ? 'ไม่พบ Workflow ที่ค้นหา' : 'ยังไม่มี Workflow เอกสาร' }}</div>
            </template>

            <Column field="docFormatCode" header="Doc Format" sortable style="min-width: 14rem">
                <template #body="{ data }">
                    <div class="font-medium text-surface-900 dark:text-surface-0">{{ data.docFormatCode }}</div>
                    <div class="text-sm text-muted-color">{{ docFormatName(data.docFormat) }}</div>
                </template>
            </Column>
            <Column field="screenCode" header="Screen" sortable style="min-width: 8rem">
                <template #body="{ data }">{{ data.screenCode || '-' }}</template>
            </Column>
            <Column field="stepCount" header="ขั้นตอน" sortable style="min-width: 9rem">
                <template #body="{ data }">
                    <div class="font-medium">{{ data.stepCount }} ขั้นตอน</div>
                    <div class="text-sm text-muted-color">{{ data.userCount }} ผู้เกี่ยวข้อง</div>
                </template>
            </Column>
            <Column header="เงื่อนไข" style="min-width: 18rem">
                <template #body="{ data }">
                    <div class="flex flex-wrap gap-2">
                        <Tag v-for="item in conditionSummary(data)" :key="item.label" :severity="item.severity" :value="`${item.label} ${item.value}`" />
                        <Tag v-if="conditionSummary(data).length === 0" severity="secondary" value="ยังไม่มีขั้นตอน" />
                    </div>
                </template>
            </Column>
            <Column field="warningCount" header="Preset" sortable style="min-width: 10rem">
                <template #body="{ data }">
                    <Tag v-if="data.warningCount > 0" severity="warn" :value="`${data.warningCount} warning`" />
                    <Tag v-else severity="success" value="ปกติ" />
                </template>
            </Column>
            <Column field="updatedAt" header="แก้ไขล่าสุด" sortable style="min-width: 12rem">
                <template #body="{ data }">{{ formatDateTime(data.updatedAt) }}</template>
            </Column>
            <Column header="จัดการ" :exportable="false" style="min-width: 13rem">
                <template #body="{ data }">
                    <div class="flex gap-2">
                        <Button icon="pi pi-pencil" severity="secondary" rounded outlined aria-label="แก้ Workflow" @click="openWorkflow(data.docFormatCode)" />
                        <Button icon="pi pi-copy" severity="secondary" rounded outlined aria-label="คัดลอก Workflow" :disabled="workflows.length < 2" @click="openCopy(data)" />
                        <Button icon="pi pi-map-marker" severity="secondary" rounded outlined aria-label="Preset กรอบลายเซ็น" @click="openPreset(data.docFormatCode)" />
                    </div>
                </template>
            </Column>
        </DataTable>
    </div>

    <Dialog v-model:visible="createVisible" modal header="เพิ่ม Workflow เอกสาร" :style="{ width: 'min(34rem, 92vw)' }">
        <div class="flex flex-col gap-4">
            <div>
                <label for="newDocFormat" class="block font-bold mb-3">Doc Format จาก SML</label>
                <Select id="newDocFormat" v-model="selectedNewDocFormat" :options="availableDocFormatOptions" optionLabel="label" optionValue="value" filter fluid />
                <small class="text-muted-color">เลือกครั้งเดียว แล้วเพิ่มขั้นตอนผู้เซ็นในหน้าถัดไป</small>
            </div>
        </div>
        <template #footer>
            <Button label="ยกเลิก" icon="pi pi-times" text @click="createVisible = false" />
            <Button label="เริ่มตั้งค่า" icon="pi pi-arrow-right" :disabled="!selectedNewDocFormat" @click="createWorkflow" />
        </template>
    </Dialog>

    <Dialog v-model:visible="copyVisible" modal header="คัดลอก Workflow" :style="{ width: 'min(48rem, 94vw)' }">
        <div v-if="copyTarget" class="flex flex-col gap-4">
            <Message severity="warn" :closable="false">การคัดลอกจะแทนที่ Workflow ปัจจุบันของ {{ copyTarget.docFormatCode }} ทั้งชุด</Message>

            <div>
                <label for="sourceWorkflow" class="block font-bold mb-3">Workflow ต้นทาง</label>
                <Select id="sourceWorkflow" v-model="copySourceDocFormat" :options="workflowOptions" optionLabel="label" optionValue="value" filter fluid @change="loadCopyPreview" />
            </div>

            <DataTable :value="copyPreview?.steps || []" :loading="loadingCopyPreview" dataKey="id" responsiveLayout="scroll" size="small" stripedRows>
                <template #header>
                    <div class="font-semibold">Preview ขั้นตอนที่จะคัดลอก</div>
                </template>
                <template #empty>
                    <div class="py-4 text-center text-muted-color">เลือก workflow ต้นทางเพื่อดู preview</div>
                </template>
                <Column field="sequenceNo" header="ลำดับ" style="width: 6rem" />
                <Column header="Position">
                    <template #body="{ data }">
                        <div class="font-medium">{{ data.positionCode }} - {{ data.positionName }}</div>
                    </template>
                </Column>
                <Column header="ผู้เซ็น">
                    <template #body="{ data }">{{ [data.user01, data.user02, data.user03].filter(Boolean).join(', ') || '-' }}</template>
                </Column>
            </DataTable>
        </div>
        <template #footer>
            <Button label="ยกเลิก" icon="pi pi-times" text :disabled="copying" @click="copyVisible = false" />
            <Button label="คัดลอก Workflow" icon="pi pi-copy" severity="warn" :loading="copying" :disabled="!copyPreview?.steps?.length" @click="confirmCopy" />
        </template>
    </Dialog>
</template>
