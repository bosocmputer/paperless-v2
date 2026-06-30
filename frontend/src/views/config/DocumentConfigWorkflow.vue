<script setup>
import { api } from '@/services/api';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const confirm = useConfirm();
const toast = useToast();

const conditionOptions = [
    { label: '1 - คนใดคนหนึ่ง', value: 1, short: 'คนใดคนหนึ่ง', severity: 'info' },
    { label: '2 - ทุกคน', value: 2, short: 'ทุกคน', severity: 'warn' },
    { label: '3 - บุคคลภายนอก', value: 3, short: 'บุคคลภายนอก', severity: 'secondary' }
];

const workflow = ref(null);
const steps = ref([]);
const users = ref([]);
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const conflictMessage = ref('');
const originalSnapshot = ref('');
const sessionId = `workflow-${Date.now()}-${Math.random().toString(16).slice(2)}`;
const openedAt = Date.now();

const docFormatCode = computed(() => String(route.params.docFormatCode || '').trim());
const activeUserOptions = computed(() => {
    const options = users.value
        .filter((user) => user.status === 'active')
        .map((user) => ({
            label: `${user.username}:${user.displayName}`,
            value: userValue(user)
        }))
        .sort((left, right) => left.value.localeCompare(right.value, 'th'));

    const seen = new Set(options.map((option) => option.value));
    steps.value.forEach((step) => {
        [step.user01, step.user02, step.user03].forEach((value) => {
            const normalized = String(value || '').trim();
            if (normalized && !seen.has(normalized)) {
                seen.add(normalized);
                options.push({ label: `${normalized} ${isActiveUserValue(normalized) ? '(เดิม)' : '(ไม่ได้ active)'}`, value: normalized });
            }
        });
    });
    return options;
});
const validationIssues = computed(() => validateSteps(steps.value));
const invalidByField = computed(() => {
    const map = new Set();
    validationIssues.value.forEach((issue) => map.add(`${issue.key}:${issue.field}`));
    return map;
});
const saveDisabledReason = computed(() => {
    if (loading.value) return 'กำลังโหลดข้อมูล';
    if (saving.value) return 'กำลังบันทึก';
    if (!dirty.value) return 'ยังไม่มีการเปลี่ยนแปลง';
    if (validationIssues.value.length > 0) return 'แก้ field ที่ผิดก่อนบันทึก';
    return '';
});
const dirty = computed(() => snapshotSteps(steps.value) !== originalSnapshot.value);
const deletedStepCount = computed(() => {
    const currentIds = new Set(steps.value.map((step) => step.id).filter(Boolean));
    return (workflow.value?.steps || []).filter((step) => step.id && !currentIds.has(step.id)).length;
});
const docFormatName = computed(() => {
    const format = workflow.value?.docFormat || {};
    return format.name_1 || format.name_2 || format.format || 'ไม่มีชื่อเอกสาร';
});

onMounted(async () => {
    window.addEventListener('beforeunload', beforeUnload);
    await loadWorkflow();
});
onBeforeUnmount(() => {
    window.removeEventListener('beforeunload', beforeUnload);
});
onBeforeRouteLeave((_to, _from, next) => {
    if (!dirty.value) {
        next();
        return;
    }
    if (window.confirm('มีการแก้ไข Workflow ที่ยังไม่ได้บันทึก ต้องการออกจากหน้านี้หรือไม่?')) {
        next();
        return;
    }
    next(false);
});

async function loadWorkflow() {
    loading.value = true;
    error.value = '';
    conflictMessage.value = '';
    try {
        const [workflowResult, userResult] = await Promise.all([api.getDocumentConfigWorkflow(docFormatCode.value), api.listUsers()]);
        workflow.value = workflowResult.workflow;
        users.value = userResult.users || [];
        steps.value = (workflow.value.steps || []).map(toEditorStep).sort((left, right) => Number(left.sequenceNo) - Number(right.sequenceNo));
        originalSnapshot.value = snapshotSteps(steps.value);
        recordEvent('workflow_open');
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลด Workflow ไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        loading.value = false;
    }
}

function addStep() {
    if (steps.value.length >= 30) return;
    const sequenceNo = nextSequenceNo();
    steps.value.push({
        key: `new-${Date.now()}-${Math.random().toString(16).slice(2)}`,
        id: '',
        positionCode: nextPositionCode(),
        positionName: '',
        user01: '',
        user02: '',
        user03: '',
        sequenceNo,
        conditionType: 1
    });
}

function requestRemoveStep(step) {
    if (!step.id) {
        removeStep(step);
        return;
    }
    confirm.require({
        message: `ลบ Position ${step.positionCode} - ${step.positionName || ''} ออกจาก Workflow?`,
        header: 'ยืนยันลบขั้นตอน',
        icon: 'pi pi-exclamation-triangle',
        acceptLabel: 'ลบขั้นตอน',
        rejectLabel: 'ยกเลิก',
        accept: () => removeStep(step)
    });
}

function removeStep(step) {
    steps.value = steps.value.filter((item) => item.key !== step.key);
    resequence();
}

function moveStep(index, direction) {
    const nextIndex = index + direction;
    if (nextIndex < 0 || nextIndex >= steps.value.length) return;
    const reordered = [...steps.value];
    const [item] = reordered.splice(index, 1);
    reordered.splice(nextIndex, 0, item);
    steps.value = reordered;
    resequence();
    recordEvent('workflow_reorder');
}

function resequence() {
    steps.value = steps.value.map((step, index) => ({ ...step, sequenceNo: index + 1 }));
}

function requestSave() {
    if (saveDisabledReason.value) return;
    if (deletedStepCount.value > 0) {
        confirm.require({
            message: `บันทึกครั้งนี้จะลบ ${deletedStepCount.value} ขั้นตอนที่ไม่มีในหน้าจอแล้ว ต้องการบันทึกต่อหรือไม่?`,
            header: 'ยืนยันการลบขั้นตอน',
            icon: 'pi pi-exclamation-triangle',
            acceptLabel: 'บันทึกและลบ',
            rejectLabel: 'ยกเลิก',
            accept: saveWorkflow
        });
        return;
    }
    saveWorkflow();
}

async function saveWorkflow() {
    saving.value = true;
    conflictMessage.value = '';
    recordEvent('workflow_save_attempt');
    try {
        const payload = {
            revision: workflow.value?.revision || '',
            steps: steps.value.map((step) => ({
                positionCode: String(step.positionCode || '').trim(),
                positionName: String(step.positionName || '').trim(),
                user01: String(step.user01 || '').trim(),
                user02: String(step.user02 || '').trim(),
                user03: String(step.user03 || '').trim(),
                sequenceNo: Number(step.sequenceNo || 0),
                conditionType: Number(step.conditionType || 0)
            }))
        };
        const result = await api.saveDocumentConfigWorkflow(docFormatCode.value, payload);
        workflow.value = result.workflow;
        steps.value = (workflow.value.steps || []).map(toEditorStep);
        originalSnapshot.value = snapshotSteps(steps.value);
        recordEvent('workflow_save_success');
        toast.add({ severity: 'success', summary: 'บันทึก Workflow แล้ว', life: 2500 });
    } catch (err) {
        recordEvent(err.status === 409 ? 'workflow_revision_conflict' : 'workflow_save_error', validationIssues.value.length);
        if (err.status === 409) {
            conflictMessage.value = err.message;
            toast.add({ severity: 'warn', summary: 'Workflow ถูกแก้จากที่อื่น', detail: 'กรุณา Refresh ก่อนบันทึกอีกครั้ง', life: 5000 });
        } else {
            toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: err.message, life: 5000 });
        }
    } finally {
        saving.value = false;
    }
}

function backToList() {
    router.push({ name: 'document-config' });
}

function openPreset() {
    router.push({ name: 'signature-template', params: { docFormatCode: docFormatCode.value } });
}

function toEditorStep(step) {
    return {
        key: step.id || `new-${Date.now()}-${Math.random().toString(16).slice(2)}`,
        id: step.id || '',
        positionCode: step.positionCode || '',
        positionName: step.positionName || '',
        user01: step.user01 || '',
        user02: step.user02 || '',
        user03: step.user03 || '',
        sequenceNo: Number(step.sequenceNo || 0),
        conditionType: Number(step.conditionType || 1)
    };
}

function validateSteps(list) {
    const issues = [];
    const seen = new Map();
    if (list.length > 30) {
        issues.push({ key: 'global', field: 'steps', message: 'Workflow มีได้สูงสุด 30 ขั้นตอน' });
    }
    list.forEach((step, index) => {
        const key = step.key;
        const label = step.positionCode ? `Position ${step.positionCode}` : `แถวที่ ${index + 1}`;
        if (!String(step.positionCode || '').trim()) issues.push({ key, field: 'positionCode', message: `${label}: ต้องระบุรหัส Position` });
        if (!String(step.positionName || '').trim()) issues.push({ key, field: 'positionName', message: `${label}: ต้องระบุชื่อ Position` });
        if (Number(step.sequenceNo || 0) <= 0) issues.push({ key, field: 'sequenceNo', message: `${label}: ลำดับต้องมากกว่า 0` });
        if (![1, 2, 3].includes(Number(step.conditionType))) issues.push({ key, field: 'conditionType', message: `${label}: เงื่อนไขไม่ถูกต้อง` });
        const duplicateKey = normalizeCode(step.positionCode);
        if (duplicateKey) {
            if (seen.has(duplicateKey)) {
                issues.push({ key, field: 'positionCode', message: `${label}: รหัส Position ซ้ำกับแถวอื่น` });
                issues.push({ key: seen.get(duplicateKey), field: 'positionCode', message: `${label}: รหัส Position ซ้ำกับแถวอื่น` });
            } else {
                seen.set(duplicateKey, key);
            }
        }
        if ([1, 2].includes(Number(step.conditionType)) && stepUsers(step).length === 0) {
            issues.push({ key, field: 'user01', message: `${label}: เงื่อนไข 1/2 ต้องมี user อย่างน้อย 1 คน` });
        }
        if ([1, 2].includes(Number(step.conditionType))) {
            stepUsers(step).forEach((value) => {
                if (!isActiveUserValue(value)) {
                    issues.push({ key, field: userFieldForValue(step, value), message: `${label}: ${value} ไม่ใช่ user active` });
                }
            });
        }
    });
    return issues;
}

function fieldInvalid(step, field) {
    return invalidByField.value.has(`${step.key}:${field}`);
}

function stepUsers(step) {
    return [step.user01, step.user02, step.user03].map((value) => String(value || '').trim()).filter(Boolean);
}

function userFieldForValue(step, value) {
    if (step.user01 === value) return 'user01';
    if (step.user02 === value) return 'user02';
    if (step.user03 === value) return 'user03';
    return 'user01';
}

function isActiveUserValue(value) {
    const username = String(value || '').split(':')[0]?.trim().toLowerCase();
    return users.value.some((user) => user.status === 'active' && String(user.username || '').trim().toLowerCase() === username);
}

function conditionMeta(value) {
    return conditionOptions.find((item) => item.value === Number(value)) || conditionOptions[0];
}

function nextSequenceNo() {
    return steps.value.reduce((max, step) => Math.max(max, Number(step.sequenceNo || 0)), 0) + 1;
}

function nextPositionCode() {
    const numericCodes = steps.value
        .map((step) => Number(step.positionCode))
        .filter((value) => Number.isFinite(value) && value > 0);
    if (numericCodes.length === steps.value.length) {
        return String(Math.max(0, ...numericCodes) + 1);
    }
    return '';
}

function snapshotSteps(list) {
    return JSON.stringify(
        list.map((step) => ({
            positionCode: String(step.positionCode || '').trim(),
            positionName: String(step.positionName || '').trim(),
            user01: String(step.user01 || '').trim(),
            user02: String(step.user02 || '').trim(),
            user03: String(step.user03 || '').trim(),
            sequenceNo: Number(step.sequenceNo || 0),
            conditionType: Number(step.conditionType || 0)
        }))
    );
}

function userValue(user) {
    return `${user.username}:${user.displayName}`;
}

function normalizeCode(value) {
    return String(value || '').trim().toUpperCase();
}

function beforeUnload(event) {
    if (!dirty.value) return;
    event.preventDefault();
    event.returnValue = '';
}

function recordEvent(event, issueCount = validationIssues.value.length) {
    api.recordDocumentConfigWorkflowEvent(docFormatCode.value, {
        event,
        sessionId,
        stepCount: steps.value.length,
        validationIssueCount: issueCount,
        elapsedMs: Date.now() - openedAt
    }).catch(() => {});
}
</script>

<template>
    <div class="workflow-page">
        <section class="editor-bar">
            <Button icon="pi pi-arrow-left" text rounded aria-label="กลับ" @click="backToList" />
            <div class="editor-title">
                <h1>{{ docFormatCode }} - {{ docFormatName }}</h1>
                <p>{{ workflow?.docFormat?.screen_code || workflow?.docFormat?.screenCode || '-' }} · {{ steps.length }} ขั้นตอน</p>
            </div>
            <div class="editor-actions">
                <Tag v-if="dirty" severity="warn" value="ยังไม่บันทึก" />
                <Tag v-else severity="success" value="บันทึกแล้ว" />
                <Button label="Preset กรอบ" icon="pi pi-map-marker" outlined @click="openPreset" />
                <Button label="บันทึก Workflow" icon="pi pi-save" :loading="saving" :disabled="Boolean(saveDisabledReason)" @click="requestSave" />
            </div>
        </section>

        <Message v-if="saveDisabledReason && dirty" severity="warn" :closable="false">{{ saveDisabledReason }}</Message>
        <Message v-if="conflictMessage" severity="warn" :closable="false">
            {{ conflictMessage }}
            <Button label="โหลดใหม่" icon="pi pi-refresh" text size="small" @click="loadWorkflow" />
        </Message>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
        <Message v-for="warning in workflow?.presetWarnings || []" :key="`${warning.code}-${warning.positionCode}`" severity="warn" :closable="false">
            {{ warning.message }} Preset เป็นตัวช่วยเท่านั้น จึงยังบันทึก Workflow ได้
        </Message>

        <section class="workflow-tools">
            <div>
                <h2>ขั้นตอนผู้เซ็น</h2>
                <p>แก้ทั้งชุดในหน้านี้ เพิ่มขั้นตอนแล้วระบบเติม Doc Format และลำดับให้เอง</p>
            </div>
            <Button label="เพิ่ม Step" icon="pi pi-plus" outlined :disabled="steps.length >= 30" @click="addStep" />
        </section>

        <section v-if="loading" class="workflow-list">
            <div v-for="index in 4" :key="index" class="workflow-row">
                <Skeleton width="10rem" height="1.4rem" />
                <Skeleton width="100%" height="4rem" class="mt-3" />
            </div>
        </section>

        <section v-else-if="steps.length === 0" class="empty-state">
            <i class="pi pi-list-check" />
            <h2>ยังไม่มีขั้นตอนใน Workflow นี้</h2>
            <p>เพิ่ม Step แรกเพื่อกำหนดตำแหน่งและผู้เซ็นของเอกสาร {{ docFormatCode }}</p>
            <Button label="เพิ่ม Step" icon="pi pi-plus" @click="addStep" />
        </section>

        <section v-else class="workflow-list">
            <article v-for="(step, index) in steps" :key="step.key" class="workflow-row">
                <div class="row-head">
                    <div class="sequence-pill">{{ index + 1 }}</div>
                    <div>
                        <h3>{{ step.positionCode || 'ยังไม่ระบุรหัส' }} - {{ step.positionName || 'ยังไม่ระบุชื่อ' }}</h3>
                        <Tag :severity="conditionMeta(step.conditionType).severity" :value="conditionMeta(step.conditionType).short" rounded />
                    </div>
                    <div class="row-actions">
                        <Button icon="pi pi-arrow-up" text rounded aria-label="เลื่อนขึ้น" :disabled="index === 0" @click="moveStep(index, -1)" />
                        <Button icon="pi pi-arrow-down" text rounded aria-label="เลื่อนลง" :disabled="index === steps.length - 1" @click="moveStep(index, 1)" />
                        <Button icon="pi pi-trash" text rounded severity="danger" aria-label="ลบขั้นตอน" @click="requestRemoveStep(step)" />
                    </div>
                </div>

                <div class="row-grid">
                    <div class="field">
                        <label>รหัส Position</label>
                        <InputText v-model="step.positionCode" :class="{ 'p-invalid': fieldInvalid(step, 'positionCode') }" />
                    </div>
                    <div class="field field-wide">
                        <label>ชื่อ Position</label>
                        <InputText v-model="step.positionName" :class="{ 'p-invalid': fieldInvalid(step, 'positionName') }" />
                    </div>
                    <div class="field">
                        <label>ลำดับ</label>
                        <InputNumber v-model="step.sequenceNo" :min="1" :max="30" :class="{ 'p-invalid': fieldInvalid(step, 'sequenceNo') }" showButtons />
                    </div>
                    <div class="field">
                        <label>เงื่อนไข</label>
                        <Select v-model="step.conditionType" :options="conditionOptions" optionLabel="label" optionValue="value" :class="{ 'p-invalid': fieldInvalid(step, 'conditionType') }" fluid />
                    </div>
                    <div class="field">
                        <label>User01</label>
                        <Select v-model="step.user01" :options="activeUserOptions" optionLabel="label" optionValue="value" showClear filter fluid :disabled="Number(step.conditionType) === 3" :class="{ 'p-invalid': fieldInvalid(step, 'user01') }" />
                    </div>
                    <div class="field">
                        <label>User02</label>
                        <Select v-model="step.user02" :options="activeUserOptions" optionLabel="label" optionValue="value" showClear filter fluid :disabled="Number(step.conditionType) === 3" :class="{ 'p-invalid': fieldInvalid(step, 'user02') }" />
                    </div>
                    <div class="field">
                        <label>User03</label>
                        <Select v-model="step.user03" :options="activeUserOptions" optionLabel="label" optionValue="value" showClear filter fluid :disabled="Number(step.conditionType) === 3" :class="{ 'p-invalid': fieldInvalid(step, 'user03') }" />
                    </div>
                </div>

                <div v-if="validationIssues.some((issue) => issue.key === step.key)" class="row-issues">
                    <div v-for="issue in validationIssues.filter((item) => item.key === step.key)" :key="`${issue.field}-${issue.message}`">
                        <i class="pi pi-exclamation-circle" />
                        <span>{{ issue.message }}</span>
                    </div>
                </div>
            </article>
        </section>
    </div>
</template>

<style scoped>
.workflow-page {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.editor-bar {
    position: sticky;
    top: 0;
    z-index: 4;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.75rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 0.75rem;
}

.editor-title {
    min-width: 0;
}

.editor-title h1 {
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 1.12rem;
    font-weight: 800;
}

.editor-title p,
.workflow-tools p {
    margin: 0.25rem 0 0;
    color: var(--text-color-secondary);
}

.editor-actions,
.row-actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
}

.workflow-tools {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
}

.workflow-tools h2 {
    margin: 0;
    font-size: 1.1rem;
}

.workflow-list {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
}

.workflow-row {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 0.9rem;
}

.row-head {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.85rem;
}

.sequence-pill {
    display: grid;
    width: 2.25rem;
    height: 2.25rem;
    place-items: center;
    border-radius: 999px;
    background: color-mix(in srgb, var(--primary-color) 12%, var(--surface-card));
    color: var(--primary-color);
    font-weight: 800;
}

.row-head h3 {
    margin: 0 0 0.35rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 1rem;
}

.row-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(10rem, 1fr));
    gap: 0.75rem;
}

.field {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 0.35rem;
}

.field-wide {
    grid-column: span 2;
}

.field label {
    font-size: 0.85rem;
    font-weight: 700;
}

.field :deep(.p-inputtext),
.field :deep(.p-inputnumber),
.field :deep(.p-select) {
    width: 100%;
}

.row-issues {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    margin-top: 0.75rem;
    color: var(--red-600);
    font-size: 0.9rem;
}

.row-issues div {
    display: flex;
    gap: 0.45rem;
}

.empty-state {
    display: flex;
    min-height: 18rem;
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

.empty-state h2,
.empty-state p {
    margin: 0;
}

.empty-state p {
    color: var(--text-color-secondary);
}

@media (max-width: 980px) {
    .editor-bar {
        grid-template-columns: auto minmax(0, 1fr);
    }

    .editor-actions {
        grid-column: 1 / -1;
        flex-wrap: wrap;
        justify-content: flex-end;
    }

    .row-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}

@media (max-width: 640px) {
    .editor-actions :deep(.p-button),
    .workflow-tools :deep(.p-button) {
        width: 100%;
    }

    .workflow-tools {
        align-items: stretch;
        flex-direction: column;
    }

    .row-head {
        grid-template-columns: auto minmax(0, 1fr);
    }

    .row-actions {
        grid-column: 1 / -1;
        justify-content: flex-end;
    }

    .row-grid {
        grid-template-columns: 1fr;
    }

    .field-wide {
        grid-column: auto;
    }
}
</style>
