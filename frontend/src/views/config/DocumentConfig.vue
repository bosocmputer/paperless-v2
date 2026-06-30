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
const hasWorkflows = computed(() => workflows.value.length > 0);

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
        acceptLabel: 'คัดลอก Workflow',
        rejectLabel: 'ยกเลิก',
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
        { label: 'ภายนอก', value: Number(counts['3'] || 0), severity: 'secondary' }
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
    <div class="document-config-page">
        <section class="page-heading">
            <div>
                <h1>Config เอกสาร</h1>
                <p>ตั้งค่า workflow ต่อชนิดเอกสารครั้งเดียว แล้วใช้ซ้ำตอนสร้างเอกสารเพื่อเซ็น</p>
            </div>
            <div class="heading-actions">
                <Button label="เพิ่ม Workflow" icon="pi pi-plus" :disabled="loading || loadingFormats || availableDocFormatOptions.length === 0" @click="openCreate" />
            </div>
        </section>

        <Message v-if="availableDocFormatOptions.length === 0 && !loading && docFormats.length > 0" severity="info" :closable="false">
            Doc Format จาก SML ถูกตั้งค่า Workflow ครบแล้ว ถ้าต้องเพิ่มชนิดใหม่ให้เพิ่มใน SML ก่อน
        </Message>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

        <section class="workflow-toolbar">
            <span class="p-input-icon-left workflow-search">
                <i class="pi pi-search" />
                <InputText v-model="searchQuery" placeholder="ค้นหา PO, ชื่อเอกสาร, เงื่อนไข" />
            </span>
            <Button label="โหลดใหม่" icon="pi pi-refresh" outlined :loading="loading" @click="loadPage" />
        </section>

        <div v-if="loading" class="workflow-grid">
            <div v-for="index in 6" :key="index" class="workflow-card">
                <Skeleton width="8rem" height="1.5rem" />
                <Skeleton width="100%" height="1rem" class="mt-3" />
                <Skeleton width="70%" height="1rem" class="mt-2" />
            </div>
        </div>

        <div v-else-if="!hasWorkflows" class="empty-state">
            <i class="pi pi-file-edit" />
            <h2>ยังไม่มี Workflow เอกสาร</h2>
            <p>เริ่มจากเลือก Doc Format จาก SML แล้วเพิ่มขั้นตอนผู้เซ็นเป็นชุดเดียว</p>
            <Button label="เพิ่ม Workflow" icon="pi pi-plus" :disabled="availableDocFormatOptions.length === 0" @click="openCreate" />
        </div>

        <div v-else-if="filteredWorkflows.length === 0" class="empty-state">
            <i class="pi pi-search" />
            <h2>ไม่พบ Workflow ที่ค้นหา</h2>
            <p>ลองค้นด้วยรหัสเอกสาร เช่น PO หรือชื่อเอกสารจาก SML</p>
        </div>

        <div v-else class="workflow-grid">
            <article v-for="workflow in filteredWorkflows" :key="workflow.docFormatCode" class="workflow-card">
                <div class="workflow-card-top">
                    <div>
                        <div class="doc-code">{{ workflow.docFormatCode }}</div>
                        <h2>{{ docFormatName(workflow.docFormat) }}</h2>
                        <p>{{ workflow.screenCode || '-' }}</p>
                    </div>
                    <Tag v-if="workflow.warningCount > 0" severity="warn" :value="`${workflow.warningCount} warning`" />
                    <Tag v-else severity="success" value="พร้อมใช้งาน" />
                </div>

                <div class="workflow-stats">
                    <div>
                        <strong>{{ workflow.stepCount }}</strong>
                        <span>ขั้นตอน</span>
                    </div>
                    <div>
                        <strong>{{ workflow.userCount }}</strong>
                        <span>ผู้เกี่ยวข้อง</span>
                    </div>
                    <div>
                        <strong>{{ formatDateTime(workflow.updatedAt) }}</strong>
                        <span>แก้ไขล่าสุด</span>
                    </div>
                </div>

                <div class="condition-tags">
                    <Tag v-for="item in conditionSummary(workflow)" :key="item.label" :severity="item.severity" :value="`${item.label} ${item.value}`" rounded />
                    <Tag v-if="conditionSummary(workflow).length === 0" severity="secondary" value="ยังไม่มีขั้นตอน" rounded />
                </div>

                <div class="workflow-actions">
                    <Button label="แก้ Workflow" icon="pi pi-pencil" @click="openWorkflow(workflow.docFormatCode)" />
                    <Button label="คัดลอก Workflow" icon="pi pi-copy" outlined :disabled="workflows.length < 2" @click="openCopy(workflow)" />
                    <Button label="Preset กรอบ" icon="pi pi-map-marker" text @click="openPreset(workflow.docFormatCode)" />
                </div>
            </article>
        </div>

        <Dialog v-model:visible="createVisible" modal header="เพิ่ม Workflow เอกสาร" :style="{ width: 'min(36rem, 94vw)' }">
            <div class="dialog-stack">
                <label for="newDocFormat">Doc Format จาก SML</label>
                <Select id="newDocFormat" v-model="selectedNewDocFormat" :options="availableDocFormatOptions" optionLabel="label" optionValue="value" filter fluid />
                <small>เมื่อเลือกแล้วจะเข้าไปเพิ่มขั้นตอนทั้งหมดของเอกสารนั้นในหน้าเดียว</small>
            </div>
            <template #footer>
                <Button label="ยกเลิก" text @click="createVisible = false" />
                <Button label="เริ่มตั้งค่า Workflow" icon="pi pi-arrow-right" :disabled="!selectedNewDocFormat" @click="createWorkflow" />
            </template>
        </Dialog>

        <Dialog v-model:visible="copyVisible" modal header="คัดลอก Workflow" :style="{ width: 'min(48rem, 96vw)' }">
            <div v-if="copyTarget" class="dialog-stack">
                <Message severity="warn" :closable="false">
                    การคัดลอกจะแทนที่ Workflow ปัจจุบันของ {{ copyTarget.docFormatCode }} ทั้งชุด
                </Message>
                <label for="sourceWorkflow">เลือก Workflow ต้นทาง</label>
                <Select id="sourceWorkflow" v-model="copySourceDocFormat" :options="workflowOptions" optionLabel="label" optionValue="value" filter fluid @change="loadCopyPreview" />

                <div class="copy-preview">
                    <div class="copy-preview-title">
                        <strong>Preview ขั้นตอนที่จะคัดลอก</strong>
                        <ProgressSpinner v-if="loadingCopyPreview" style="width: 1.25rem; height: 1.25rem" strokeWidth="6" />
                    </div>
                    <div v-if="copyPreview?.steps?.length" class="preview-step-list">
                        <div v-for="step in copyPreview.steps" :key="step.id" class="preview-step">
                            <span>{{ step.sequenceNo }}.</span>
                            <strong>{{ step.positionCode }} - {{ step.positionName }}</strong>
                            <small>{{ step.user01 || '-' }} {{ step.user02 || '' }} {{ step.user03 || '' }}</small>
                        </div>
                    </div>
                    <p v-else class="muted-text">เลือก workflow ต้นทางเพื่อดู preview</p>
                </div>
            </div>
            <template #footer>
                <Button label="ยกเลิก" text :disabled="copying" @click="copyVisible = false" />
                <Button label="คัดลอก Workflow" icon="pi pi-copy" severity="warn" :loading="copying" :disabled="!copyPreview?.steps?.length" @click="confirmCopy" />
            </template>
        </Dialog>
    </div>
</template>

<style scoped>
.document-config-page {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.page-heading,
.workflow-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
}

.page-heading h1 {
    margin: 0;
    font-size: 1.5rem;
    font-weight: 700;
}

.page-heading p {
    margin: 0.35rem 0 0;
    color: var(--text-color-secondary);
}

.heading-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
}

.workflow-search {
    width: min(32rem, 100%);
}

.workflow-search :deep(.p-inputtext) {
    width: 100%;
}

.workflow-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(21rem, 1fr));
    gap: 1rem;
}

.workflow-card {
    display: flex;
    min-height: 16rem;
    flex-direction: column;
    gap: 1rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 1rem;
}

.workflow-card-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.doc-code {
    color: var(--primary-color);
    font-size: 1.2rem;
    font-weight: 800;
}

.workflow-card h2 {
    margin: 0.15rem 0;
    font-size: 1rem;
    font-weight: 700;
}

.workflow-card p,
.muted-text {
    margin: 0;
    color: var(--text-color-secondary);
}

.workflow-stats {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.75rem;
}

.workflow-stats div {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 0.2rem;
}

.workflow-stats strong {
    min-height: 1.25rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.workflow-stats span {
    color: var(--text-color-secondary);
    font-size: 0.82rem;
}

.condition-tags,
.workflow-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
}

.workflow-actions {
    margin-top: auto;
}

.empty-state {
    display: flex;
    min-height: 20rem;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    border: 1px dashed var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 2rem;
    text-align: center;
}

.empty-state i {
    color: var(--primary-color);
    font-size: 2rem;
}

.empty-state h2 {
    margin: 0;
    font-size: 1.2rem;
}

.empty-state p {
    max-width: 34rem;
    margin: 0;
    color: var(--text-color-secondary);
}

.dialog-stack {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}

.dialog-stack label {
    font-weight: 700;
}

.dialog-stack small {
    color: var(--text-color-secondary);
}

.copy-preview {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.85rem;
}

.copy-preview-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.65rem;
}

.preview-step-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
}

.preview-step {
    display: grid;
    grid-template-columns: 2.5rem minmax(8rem, 1fr);
    gap: 0.25rem 0.5rem;
    border-bottom: 1px solid var(--surface-border);
    padding-bottom: 0.5rem;
}

.preview-step:last-child {
    border-bottom: 0;
    padding-bottom: 0;
}

.preview-step small {
    grid-column: 2;
}

@media (max-width: 720px) {
    .page-heading,
    .workflow-toolbar {
        align-items: stretch;
        flex-direction: column;
    }

    .heading-actions,
    .workflow-toolbar > * {
        width: 100%;
    }

    .workflow-grid {
        grid-template-columns: minmax(0, 1fr);
    }

    .workflow-stats {
        grid-template-columns: 1fr;
    }

    .workflow-actions :deep(.p-button) {
        width: 100%;
    }
}
</style>
