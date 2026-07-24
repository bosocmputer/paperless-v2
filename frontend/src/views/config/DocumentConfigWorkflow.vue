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
const searchQuery = ref('');
const originalSnapshot = ref('');
const stepDialogVisible = ref(false);
const editingStepKey = ref('');
const submitted = ref(false);
const stepForm = ref(emptyStepForm());
const sessionId = `workflow-${Date.now()}-${Math.random().toString(16).slice(2)}`;
const openedAt = Date.now();

const docFormatCode = computed(() => String(route.params.docFormatCode || '').trim());
const docFormatName = computed(() => {
    const format = workflow.value?.docFormat || {};
    return format.name_1 || format.name_2 || format.format || 'ไม่มีชื่อเอกสาร';
});
const isInternalDocument = computed(() => {
    const format = workflow.value?.docFormat || {};
    return String(format.source || '').toLowerCase() === 'internal' || String(format.screenCode || format.screen_code || '').toUpperCase() === 'INTERNAL';
});
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
        signerValues(step).forEach((value) => {
            if (!seen.has(value)) {
                seen.add(value);
                options.push({ label: `${value} ${isActiveUserValue(value) ? '(เดิม)' : '(ไม่ได้ active)'}`, value });
            }
        });
    });
    return options;
});
const filteredSteps = computed(() => {
    const query = normalizeSearch(searchQuery.value);
    if (!query) return steps.value;
    return steps.value.filter((step) =>
        normalizeSearch(`${step.sequenceNo} ${step.positionCode} ${step.positionName} ${conditionMeta(step.conditionType).short} ${signerValues(step).join(' ')}`).includes(query)
    );
});
const validationIssues = computed(() => validateSteps(steps.value));
const dirty = computed(() => snapshotSteps(steps.value) !== originalSnapshot.value);
const reorderDisabled = computed(() => Boolean(normalizeSearch(searchQuery.value)));
const deletedStepCount = computed(() => {
    const currentIds = new Set(steps.value.map((step) => step.id).filter(Boolean));
    return (workflow.value?.steps || []).filter((step) => step.id && !currentIds.has(step.id)).length;
});
const saveDisabledReason = computed(() => {
    if (loading.value) return 'กำลังโหลดข้อมูล';
    if (saving.value) return 'กำลังบันทึก';
    if (!dirty.value) return 'ยังไม่มีการเปลี่ยนแปลง';
    if (validationIssues.value.length > 0) return 'แก้ขั้นตอนที่ยังไม่สมบูรณ์ก่อนบันทึก';
    return '';
});
const stepDialogTitle = computed(() => (editingStepKey.value ? 'แก้ไขขั้นตอน' : 'เพิ่มขั้นตอน'));
const stepFormIssues = computed(() => validateStepForm(stepForm.value, editingStepKey.value));
const stepFormSignerSlotOptions = computed(() => {
    if (Number(stepForm.value.conditionType) === 3) return [{ label: 'บุคคลภายนอก', value: 1 }];
    const values = signerValues(stepForm.value);
    if (values.length === 0) return [{ label: 'ผู้เซ็น 1', value: 1 }];
    return values.map((value, index) => ({ label: `ผู้เซ็น ${index + 1} · ${value}`, value: index + 1 }));
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
    confirm.require({
        message: 'มีการแก้ไข Workflow ที่ยังไม่ได้บันทึก ต้องการออกจากหน้านี้หรือไม่?',
        header: 'ออกจากหน้าตั้งค่า Workflow',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'อยู่หน้านี้ต่อ',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ออกจากหน้านี้',
            severity: 'danger'
        },
        accept: () => next(),
        reject: () => next(false)
    });
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
        resequence(false);
        originalSnapshot.value = snapshotSteps(steps.value);
        recordEvent('workflow_open');
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลด Workflow ไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        loading.value = false;
    }
}

function openCreateStep() {
    editingStepKey.value = '';
    submitted.value = false;
    stepForm.value = {
        ...emptyStepForm(),
        positionCode: nextPositionCode(),
        conditionType: 1
    };
    stepDialogVisible.value = true;
}

function openEditStep(step) {
    editingStepKey.value = step.key;
    submitted.value = false;
    stepForm.value = {
        positionCode: step.positionCode || '',
        positionName: step.positionName || '',
        user01: step.user01 || '',
        user02: step.user02 || '',
        user03: step.user03 || '',
        conditionType: Number(step.conditionType || 1),
        attachmentRequirements: cloneAttachmentRequirements(step.attachmentRequirements)
    };
    stepDialogVisible.value = true;
}

function saveStepDialog() {
    submitted.value = true;
    if (stepFormIssues.value.length > 0) return;

    const payload = {
        positionCode: String(stepForm.value.positionCode || '').trim(),
        positionName: String(stepForm.value.positionName || '').trim(),
        user01: Number(stepForm.value.conditionType) === 3 ? '' : String(stepForm.value.user01 || '').trim(),
        user02: Number(stepForm.value.conditionType) === 3 ? '' : String(stepForm.value.user02 || '').trim(),
        user03: Number(stepForm.value.conditionType) === 3 ? '' : String(stepForm.value.user03 || '').trim(),
        conditionType: Number(stepForm.value.conditionType || 1),
        attachmentRequirements: normalizeAttachmentRequirements(stepForm.value)
    };

    if (editingStepKey.value) {
        steps.value = steps.value.map((step) => (step.key === editingStepKey.value ? { ...step, ...payload } : step));
    } else {
        steps.value.push({
            key: `new-${Date.now()}-${Math.random().toString(16).slice(2)}`,
            id: '',
            sequenceNo: steps.value.length + 1,
            ...payload
        });
    }
    resequence();
    stepDialogVisible.value = false;
}

function requestRemoveStep(step) {
    confirm.require({
        message: `ลบ Position ${step.positionCode} - ${step.positionName || ''} ออกจาก Workflow?`,
        header: 'ยืนยันลบขั้นตอน',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'ยกเลิก',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ลบขั้นตอน',
            severity: 'danger'
        },
        accept: () => {
            steps.value = steps.value.filter((item) => item.key !== step.key);
            resequence();
        }
    });
}

function moveStepByKey(stepKey, direction) {
    if (reorderDisabled.value) return;
    const source = steps.value.findIndex((step) => step.key === stepKey);
    if (source < 0) return;
    const nextIndex = source + direction;
    if (nextIndex < 0 || nextIndex >= steps.value.length) return;
    const reordered = [...steps.value];
    const [item] = reordered.splice(source, 1);
    reordered.splice(nextIndex, 0, item);
    steps.value = reordered;
    resequence();
    recordEvent('workflow_reorder');
}

function resequence(markDirty = true) {
    steps.value = steps.value.map((step, index) => ({ ...step, sequenceNo: index + 1 }));
    if (markDirty) conflictMessage.value = '';
}

function canMoveStep(stepKey, direction) {
    if (reorderDisabled.value) return false;
    const source = steps.value.findIndex((step) => step.key === stepKey);
    if (source < 0) return false;
    const nextIndex = source + direction;
    return nextIndex >= 0 && nextIndex < steps.value.length;
}

function requestSave() {
    if (saveDisabledReason.value) return;
    if (deletedStepCount.value > 0) {
        confirm.require({
            message: `บันทึกครั้งนี้จะลบ ${deletedStepCount.value} ขั้นตอนที่ไม่มีในตารางแล้ว ต้องการบันทึกต่อหรือไม่?`,
            header: 'ยืนยันการลบขั้นตอน',
            icon: 'pi pi-exclamation-triangle',
            rejectProps: {
                label: 'ยกเลิก',
                severity: 'secondary',
                outlined: true
            },
            acceptProps: {
                label: 'บันทึกและลบ',
                severity: 'warn'
            },
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
            steps: steps.value.map((step, index) => ({
                positionCode: String(step.positionCode || '').trim(),
                positionName: String(step.positionName || '').trim(),
                user01: String(step.user01 || '').trim(),
                user02: String(step.user02 || '').trim(),
                user03: String(step.user03 || '').trim(),
                sequenceNo: index + 1,
                conditionType: Number(step.conditionType || 0),
                attachmentRequirements: normalizeAttachmentRequirements(step)
            }))
        };
        const result = await api.saveDocumentConfigWorkflow(docFormatCode.value, payload);
        workflow.value = result.workflow;
        steps.value = (workflow.value.steps || []).map(toEditorStep);
        resequence(false);
        originalSnapshot.value = snapshotSteps(steps.value);
        recordEvent('workflow_save_success');
        toast.add({ severity: 'success', summary: 'บันทึก Workflow แล้ว', life: 2500 });
    } catch (err) {
        recordEvent(err.status === 409 ? 'workflow_revision_conflict' : 'workflow_save_error', validationIssues.value.length);
        if (err.status === 409) {
            conflictMessage.value = err.message;
            toast.add({ severity: 'warn', summary: 'Workflow ถูกแก้จากที่อื่น', detail: 'กรุณาโหลดใหม่ก่อนบันทึกอีกครั้ง', life: 5000 });
        } else {
            toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: err.message, life: 5000 });
        }
    } finally {
        saving.value = false;
    }
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
        conditionType: Number(step.conditionType || 1),
        attachmentRequirements: cloneAttachmentRequirements(step.attachmentRequirements)
    };
}

function emptyStepForm() {
    return {
        positionCode: '',
        positionName: '',
        user01: '',
        user02: '',
        user03: '',
        conditionType: 1,
        attachmentRequirements: []
    };
}

function validateSteps(list) {
    const issues = [];
    const seen = new Map();
    list.forEach((step) => {
        issues.push(...validateSingleStep(step, step.key));
        const duplicateKey = normalizeCode(step.positionCode);
        if (duplicateKey) {
            if (seen.has(duplicateKey)) {
                issues.push({ key: step.key, message: `Position ${step.positionCode} ซ้ำกับแถวอื่น` });
                issues.push({ key: seen.get(duplicateKey), message: `Position ${step.positionCode} ซ้ำกับแถวอื่น` });
            } else {
                seen.set(duplicateKey, step.key);
            }
        }
    });
    return issues;
}

function validateStepForm(form, editingKey) {
    const draft = {
        key: editingKey || 'draft',
        positionCode: form.positionCode,
        positionName: form.positionName,
        user01: form.user01,
        user02: form.user02,
        user03: form.user03,
        conditionType: form.conditionType,
        attachmentRequirements: form.attachmentRequirements
    };
    const issues = validateSingleStep(draft, draft.key);
    const duplicate = steps.value.find((step) => step.key !== editingKey && normalizeCode(step.positionCode) === normalizeCode(form.positionCode));
    if (duplicate && form.positionCode) issues.push({ key: draft.key, message: `รหัส Position ${form.positionCode} ซ้ำกับขั้นตอนอื่น` });
    return issues;
}

function validateSingleStep(step, key) {
    const issues = [];
    const label = step.positionCode ? `Position ${step.positionCode}` : 'Step';
    if (!String(step.positionCode || '').trim()) issues.push({ key, message: `${label}: ต้องระบุรหัส Position` });
    if (!String(step.positionName || '').trim()) issues.push({ key, message: `${label}: ต้องระบุชื่อ Position` });
    if (![1, 2, 3].includes(Number(step.conditionType))) issues.push({ key, message: `${label}: เงื่อนไขไม่ถูกต้อง` });
    if ([1, 2].includes(Number(step.conditionType)) && signerValues(step).length === 0) {
        issues.push({ key, message: `${label}: เงื่อนไข 1/2 ต้องมีผู้เซ็นอย่างน้อย 1 คน` });
    }
    if ([1, 2].includes(Number(step.conditionType))) {
        const seenUsers = new Set();
        signerValues(step).forEach((value) => {
            const username = signerUsername(value);
            if (!isActiveUserValue(value)) issues.push({ key, message: `${label}: ${value} ไม่ใช่ user active` });
            if (username && seenUsers.has(username)) issues.push({ key, message: `${label}: ผู้เซ็นซ้ำ ${username}` });
            if (username) seenUsers.add(username);
        });
    }
    const requirements = normalizeAttachmentRequirements(step);
    if (requirements.length > 12) issues.push({ key, message: `${label}: เอกสารแนบบังคับได้ไม่เกิน 12 รายการ` });
    const seenRequirements = new Set();
    const slotCount = Number(step.conditionType) === 3 ? 1 : Math.max(1, signerValues(step).length);
    requirements.forEach((requirement) => {
        if (!requirement.label) issues.push({ key, message: `${label}: ต้องระบุชื่อเอกสารแนบบังคับ` });
        if (requirement.label.length > 80) issues.push({ key, message: `${label}: ชื่อเอกสารแนบบังคับยาวเกิน 80 ตัวอักษร` });
        if (requirement.signerSlot < 1 || requirement.signerSlot > slotCount) issues.push({ key, message: `${label}: ช่องเอกสารแนบบังคับไม่ตรงกับผู้เซ็น` });
        const duplicateKey = `${requirement.signerSlot}:${requirement.label.trim().toLowerCase()}`;
        if (seenRequirements.has(duplicateKey)) issues.push({ key, message: `${label}: เอกสารแนบบังคับ "${requirement.label}" ซ้ำในผู้เซ็นเดียวกัน` });
        seenRequirements.add(duplicateKey);
    });
    return issues;
}

function rowIssues(step) {
    return validationIssues.value.filter((issue) => issue.key === step.key);
}

function rowSeverity(step) {
    return rowIssues(step).length > 0 ? 'danger' : 'success';
}

function rowStatus(step) {
    return rowIssues(step).length > 0 ? 'ต้องแก้ไข' : 'สมบูรณ์';
}

function signerValues(step) {
    return [step.user01, step.user02, step.user03].map((value) => String(value || '').trim()).filter(Boolean);
}

function signerUsername(value) {
    return String(value || '').split(':')[0]?.trim().toLowerCase() || '';
}

function isActiveUserValue(value) {
    const username = signerUsername(value);
    return users.value.some((user) => user.status === 'active' && String(user.username || '').trim().toLowerCase() === username);
}

function conditionMeta(value) {
    return conditionOptions.find((item) => item.value === Number(value)) || conditionOptions[0];
}

function routePreview(step) {
    const count = signerValues(step).length;
    if (Number(step.conditionType) === 3) return 'ส่งให้บุคคลภายนอกผ่าน link/OTP';
    if (Number(step.conditionType) === 2) return `ต้องเซ็นครบ ${count} คน`;
    return count > 0 ? `ส่งให้ ${count} คน, ใครเซ็นก่อนถือว่าผ่าน` : 'ยังไม่ได้เลือกผู้เซ็น';
}

function attachmentRequirementCount(step) {
    return normalizeAttachmentRequirements(step).length;
}

function cloneAttachmentRequirements(requirements = []) {
    return (requirements || []).map((item, index) => ({
        key: item.key || `req-${Date.now()}-${index}-${Math.random().toString(16).slice(2)}`,
        label: item.label || '',
        signerSlot: Number(item.signerSlot || 1)
    }));
}

function normalizeAttachmentRequirements(step) {
    return (step.attachmentRequirements || [])
        .map((item, index) => ({
            key: String(item.key || `req-${index + 1}`).trim(),
            label: String(item.label || '').trim(),
            signerSlot: Number(item.signerSlot || 1)
        }))
        .filter((item) => item.key || item.label);
}

function addAttachmentRequirement() {
    stepForm.value.attachmentRequirements = [
        ...(stepForm.value.attachmentRequirements || []),
        {
            key: `req-${Date.now()}-${Math.random().toString(16).slice(2)}`,
            label: '',
            signerSlot: stepFormSignerSlotOptions.value[0]?.value || 1
        }
    ];
}

function removeAttachmentRequirement(key) {
    stepForm.value.attachmentRequirements = (stepForm.value.attachmentRequirements || []).filter((item) => item.key !== key);
}

function applyRequirementsToAllSigners() {
    const current = normalizeAttachmentRequirements(stepForm.value);
    const labels = [...new Set(current.map((item) => item.label).filter(Boolean))];
    if (!labels.length) return;
    const next = [];
    for (const option of stepFormSignerSlotOptions.value) {
        for (const label of labels) {
            next.push({
                key: `req-${option.value}-${next.length + 1}-${Date.now()}`,
                label,
                signerSlot: option.value
            });
        }
    }
    stepForm.value.attachmentRequirements = next;
}

function nextPositionCode() {
    const numericCodes = steps.value
        .map((step) => Number(step.positionCode))
        .filter((value) => Number.isFinite(value) && value > 0);
    if (numericCodes.length === steps.value.length) return String(Math.max(0, ...numericCodes) + 1);
    return '';
}

function snapshotSteps(list) {
    return JSON.stringify(
        list.map((step, index) => ({
            positionCode: String(step.positionCode || '').trim(),
            positionName: String(step.positionName || '').trim(),
            user01: String(step.user01 || '').trim(),
            user02: String(step.user02 || '').trim(),
            user03: String(step.user03 || '').trim(),
            sequenceNo: index + 1,
            conditionType: Number(step.conditionType || 0),
            attachmentRequirements: normalizeAttachmentRequirements(step)
        }))
    );
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

function userValue(user) {
    return `${user.username}:${user.displayName}`;
}

function normalizeSearch(value) {
    return String(value || '').trim().toLowerCase();
}

function normalizeCode(value) {
    return String(value || '').trim().toUpperCase();
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
            <div class="flex min-w-0 items-center gap-3">
                <Button icon="pi pi-arrow-left" severity="secondary" rounded outlined aria-label="กลับ" @click="router.push({ name: 'document-config' })" />
                <div class="min-w-0 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <div class="font-semibold text-xl whitespace-nowrap truncate">{{ docFormatCode }} - {{ docFormatName }}</div>
                    <p class="text-muted-color m-0 min-w-0 truncate">แก้ Workflow ทั้งชุด การเปลี่ยนแปลงมีผลกับเอกสารใหม่เท่านั้น</p>
                </div>
            </div>
            <div class="flex flex-wrap gap-2 items-center xl:justify-end">
                <Tag v-if="dirty" severity="warn" value="ยังไม่บันทึก" />
                <Tag v-else severity="success" value="บันทึกแล้ว" />
                <Button label="เพิ่มขั้นตอน" icon="pi pi-plus" severity="secondary" @click="openCreateStep" />
                <Button v-if="!isInternalDocument" label="กรอบเริ่มต้น" icon="pi pi-map-marker" severity="secondary" outlined @click="router.push({ name: 'signature-template', params: { docFormatCode } })" />
                <Button label="บันทึก Workflow" icon="pi pi-save" :loading="saving" :disabled="Boolean(saveDisabledReason)" @click="requestSave" />
            </div>
        </div>

        <Message v-if="saveDisabledReason && dirty" severity="warn" class="mb-4" :closable="false">{{ saveDisabledReason }}</Message>
        <Message v-if="isInternalDocument" severity="info" class="mb-4" :closable="false">เอกสารภายในใช้ Workflow สำหรับลำดับและผู้เซ็นเท่านั้น ผู้สร้างจะวางกรอบลายเซ็นและข้อความกฎหมายบน PDF ฉบับจริงใน Draft ก่อนส่ง</Message>
        <Message v-if="conflictMessage" severity="warn" class="mb-4" :closable="false">
            {{ conflictMessage }}
            <Button label="โหลดใหม่" icon="pi pi-refresh" text size="small" @click="loadWorkflow" />
        </Message>
        <Message v-if="error" severity="error" class="mb-4" :closable="false">{{ error }}</Message>
        <Message v-for="warning in workflow?.presetWarnings || []" :key="`${warning.code}-${warning.positionCode}`" severity="warn" class="mb-4" :closable="false">
            {{ warning.message }} กรอบเริ่มต้นเป็นตัวช่วยเท่านั้น จึงยังบันทึก Workflow ได้
        </Message>

        <DataTable :value="filteredSteps" :loading="loading" dataKey="key" responsiveLayout="scroll" stripedRows>
            <template #header>
                <div class="flex justify-end">
                    <IconField class="w-full md:w-80">
                        <InputIcon>
                            <i class="pi pi-search" />
                        </InputIcon>
                        <InputText v-model="searchQuery" placeholder="ค้นหา Position หรือผู้เซ็น" class="w-full" />
                    </IconField>
                </div>
            </template>

            <template #empty>
                <div class="py-6 text-center text-muted-color">{{ searchQuery ? 'ไม่พบขั้นตอนที่ค้นหา' : 'ยังไม่มีขั้นตอนใน Workflow นี้' }}</div>
            </template>

            <Column field="sequenceNo" header="ลำดับ" style="width: 7rem">
                <template #body="{ data }">
                    <Tag :value="data.sequenceNo" severity="secondary" />
                </template>
            </Column>
            <Column header="Position" style="min-width: 16rem">
                <template #body="{ data }">
                    <div class="font-medium text-surface-900 dark:text-surface-0">{{ data.positionCode }} - {{ data.positionName }}</div>
                    <div class="text-sm text-muted-color">{{ routePreview(data) }}</div>
                </template>
            </Column>
            <Column header="เงื่อนไข" style="min-width: 10rem">
                <template #body="{ data }">
                    <Tag :severity="conditionMeta(data.conditionType).severity" :value="conditionMeta(data.conditionType).short" />
                </template>
            </Column>
            <Column header="ผู้เซ็น" style="min-width: 22rem">
                <template #body="{ data }">
                    <div v-if="signerValues(data).length" class="flex flex-wrap gap-2">
                        <Tag v-for="signer in signerValues(data)" :key="signer" :value="signer" severity="secondary" />
                    </div>
                    <span v-else class="text-muted-color">ไม่ใช้ user ภายใน</span>
                </template>
            </Column>
            <Column header="เอกสารแนบ" style="min-width: 10rem">
                <template #body="{ data }">
                    <Tag v-if="attachmentRequirementCount(data)" :value="`บังคับ ${attachmentRequirementCount(data)}`" severity="warn" />
                    <span v-else class="text-muted-color">ไม่บังคับ</span>
                </template>
            </Column>
            <Column header="สถานะ" style="min-width: 9rem">
                <template #body="{ data }">
                    <Tag :severity="rowSeverity(data)" :value="rowStatus(data)" />
                </template>
            </Column>
            <Column header="จัดการ" :exportable="false" style="min-width: 13rem">
                <template #body="{ data }">
                    <div class="flex gap-2">
                        <Button icon="pi pi-arrow-up" severity="secondary" rounded outlined aria-label="เลื่อนขึ้น" :disabled="!canMoveStep(data.key, -1)" @click.stop="moveStepByKey(data.key, -1)" />
                        <Button icon="pi pi-arrow-down" severity="secondary" rounded outlined aria-label="เลื่อนลง" :disabled="!canMoveStep(data.key, 1)" @click.stop="moveStepByKey(data.key, 1)" />
                        <Button icon="pi pi-pencil" severity="secondary" rounded outlined aria-label="แก้ไขขั้นตอน" @click.stop="openEditStep(data)" />
                        <Button icon="pi pi-trash" severity="danger" rounded outlined aria-label="ลบขั้นตอน" @click.stop="requestRemoveStep(data)" />
                    </div>
                </template>
            </Column>
        </DataTable>
    </div>

    <Dialog v-model:visible="stepDialogVisible" modal :header="stepDialogTitle" :style="{ width: 'min(46rem, 94vw)' }">
        <div class="flex flex-col gap-5">
            <Message v-if="submitted && stepFormIssues.length" severity="error" :closable="false">
                <ul class="m-0 pl-4">
                    <li v-for="issue in stepFormIssues" :key="issue.message">{{ issue.message }}</li>
                </ul>
            </Message>

            <div class="grid grid-cols-12 gap-4">
                <div class="col-span-12 md:col-span-4">
                    <label for="positionCode" class="block font-bold mb-3">รหัส Position</label>
                    <InputText id="positionCode" v-model.trim="stepForm.positionCode" fluid autofocus />
                </div>
                <div class="col-span-12 md:col-span-8">
                    <label for="positionName" class="block font-bold mb-3">ชื่อ Position</label>
                    <InputText id="positionName" v-model.trim="stepForm.positionName" fluid />
                </div>
            </div>

            <div>
                <label for="conditionType" class="block font-bold mb-3">เงื่อนไข</label>
                <Select id="conditionType" v-model="stepForm.conditionType" :options="conditionOptions" optionLabel="label" optionValue="value" fluid />
                <small class="text-muted-color">{{ Number(stepForm.conditionType) === 1 ? 'ส่งให้ทุกคนในขั้นตอนนี้ ใครเซ็นก่อนถือว่าผ่าน' : Number(stepForm.conditionType) === 2 ? 'ทุกคนในขั้นตอนนี้ต้องเซ็นครบ' : 'ใช้สำหรับบุคคลภายนอก ไม่ต้องเลือก user ภายใน' }}</small>
            </div>

            <div class="grid grid-cols-12 gap-4">
                <div class="col-span-12 md:col-span-4">
                    <label for="user01" class="block font-bold mb-3">ผู้เซ็น 1</label>
                    <Select id="user01" v-model="stepForm.user01" :options="activeUserOptions" optionLabel="label" optionValue="value" showClear filter fluid :disabled="Number(stepForm.conditionType) === 3" />
                </div>
                <div class="col-span-12 md:col-span-4">
                    <label for="user02" class="block font-bold mb-3">ผู้เซ็น 2</label>
                    <Select id="user02" v-model="stepForm.user02" :options="activeUserOptions" optionLabel="label" optionValue="value" showClear filter fluid :disabled="Number(stepForm.conditionType) === 3" />
                </div>
                <div class="col-span-12 md:col-span-4">
                    <label for="user03" class="block font-bold mb-3">ผู้เซ็น 3</label>
                    <Select id="user03" v-model="stepForm.user03" :options="activeUserOptions" optionLabel="label" optionValue="value" showClear filter fluid :disabled="Number(stepForm.conditionType) === 3" />
                </div>
            </div>

            <div class="border border-surface rounded-lg p-4 bg-surface-50 dark:bg-surface-900">
                <div class="flex flex-col md:flex-row md:items-center justify-between gap-3 mb-4">
                    <div>
                        <div class="font-bold">เอกสารแนบบังคับ</div>
                        <small class="text-muted-color">กำหนดช่องเอกสารที่ผู้เซ็นต้องแนบก่อนกดยืนยันเซ็น</small>
                    </div>
                    <div class="flex flex-wrap gap-2">
                        <Button label="ใช้กับทุกผู้เซ็น" icon="pi pi-copy" severity="secondary" outlined size="small" :disabled="!normalizeAttachmentRequirements(stepForm).some((item) => item.label)" @click="applyRequirementsToAllSigners" />
                        <Button label="เพิ่มเอกสาร" icon="pi pi-plus" severity="secondary" size="small" @click="addAttachmentRequirement" />
                    </div>
                </div>
                <div v-if="stepForm.attachmentRequirements.length" class="grid gap-3">
                    <div v-for="requirement in stepForm.attachmentRequirements" :key="requirement.key" class="grid grid-cols-12 gap-3 items-end">
                        <div class="col-span-12 md:col-span-7">
                            <label class="block font-medium mb-2">ชื่อเอกสาร</label>
                            <InputText v-model.trim="requirement.label" placeholder="เช่น ใบเสนอราคา" fluid />
                        </div>
                        <div class="col-span-10 md:col-span-4">
                            <label class="block font-medium mb-2">บังคับกับ</label>
                            <Select v-model="requirement.signerSlot" :options="stepFormSignerSlotOptions" optionLabel="label" optionValue="value" fluid />
                        </div>
                        <div class="col-span-2 md:col-span-1 flex justify-end">
                            <Button icon="pi pi-trash" severity="danger" outlined rounded aria-label="ลบเอกสารแนบบังคับ" @click="removeAttachmentRequirement(requirement.key)" />
                        </div>
                    </div>
                </div>
                <Message v-else severity="info" class="m-0">ถ้าไม่กำหนด ผู้เซ็นสามารถเซ็นได้โดยไม่ต้องแนบไฟล์</Message>
            </div>
        </div>

        <template #footer>
            <Button label="ยกเลิก" icon="pi pi-times" text @click="stepDialogVisible = false" />
            <Button label="บันทึกขั้นตอน" icon="pi pi-check" @click="saveStepDialog" />
        </template>
    </Dialog>
</template>
