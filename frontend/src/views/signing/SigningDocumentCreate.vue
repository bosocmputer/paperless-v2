<script setup>
import { api } from '@/services/api';
import { formatDocumentDate } from '@/utils/signingFormatters';
import DocumentLayoutDesigner from './components/DocumentLayoutDesigner.vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const confirm = useConfirm();
const toast = useToast();

const legacyDraftKey = 'paperless_signing_wizard_draft_v1';
const draftKey = 'paperless_signing_wizard_draft_v2';
const docFormats = ref([]);
const candidates = ref([]);
const candidatePage = ref(1);
const candidateTotal = ref(0);
const candidateHasMore = ref(false);
const loading = ref(false);
const creating = ref(false);
const uploading = ref(false);
const searchingCandidates = ref(false);
const loadingLayoutContext = ref(false);
const activeStep = ref(0);
const fileInput = ref(null);
const form = ref(emptyForm());
const createSessionId = ref(makeClientId());
const createIdempotencyKey = ref(makeClientId());
let searchTimer;
let suppressSearchWatch = false;
let openedAt = Date.now();
let createFinished = false;

const wizardSteps = [
    { label: 'เลือกเอกสาร', shortLabel: 'เอกสาร', icon: 'pi pi-file' },
    { label: 'PDF และกรอบ', shortLabel: 'PDF/กรอบ', icon: 'pi pi-pencil' },
    { label: 'ตรวจสอบและส่งเซ็น', shortLabel: 'ส่งเซ็น', icon: 'pi pi-send' }
];

const docFormatOptions = computed(() =>
    docFormats.value.map((format) => ({
        label: `${format.code} - ${format.name_1 || format.name_2 || format.format || 'ไม่มีชื่อเอกสาร'}`,
        value: format.code
    }))
);

const selectedDocFormatLabel = computed(() => docFormatOptions.value.find((item) => item.value === form.value.docFormatCode)?.label || '-');
const headerSummary = computed(() => {
    if (form.value.selectedCandidate) return `${selectedDocFormatLabel.value} · ${form.value.selectedCandidate.doc_no}`;
    if (form.value.docFormatCode) return `${selectedDocFormatLabel.value} · ยังไม่เลือกเลขเอกสาร`;
    return 'เลือกเอกสารจาก SML แล้วอัปโหลด PDF เพื่อวางกรอบลายเซ็น';
});
const lockedBySML = computed(() => Number(form.value.selectedCandidate?.is_lock_record || 0) === 1);
const layoutValidationIssues = computed(() => validateLayout());
const createDisabledReason = computed(() => finalDisabledReason());
const dirty = computed(() => {
    if (createFinished) return false;
    return !!form.value.docFormatCode || !!form.value.search || !!form.value.selectedCandidate || !!form.value.fileId || form.value.layoutBoxes.length > 0;
});
const stepBlockedReason = computed(() => wizardSteps.map((_step, index) => blockedReasonForStep(index)));
const currentStepReason = computed(() => stepBlockedReason.value[activeStep.value] || '');
const canGoNext = computed(() => activeStep.value < wizardSteps.length - 1 && !currentStepReason.value);
const designerMode = computed(() => activeStep.value === 1 && !!form.value.fileUrl);
const maxAllowedStep = computed(() => {
    if (blockedReasonForStep(0)) return 0;
    if (blockedReasonForStep(1)) return 1;
    return wizardSteps.length - 1;
});
const activeStepValue = computed({
    get: () => activeStep.value + 1,
    set: (value) => setActiveStep(Number(value) - 1)
});

watch(
    () => form.value.search,
    () => {
        if (suppressSearchWatch) {
            suppressSearchWatch = false;
            persistDraft();
            return;
        }
        clearTimeout(searchTimer);
        form.value.docNo = '';
        form.value.selectedCandidate = null;
        candidates.value = [];
        candidatePage.value = 1;
        candidateHasMore.value = false;
        persistDraft();
        if (!form.value.docFormatCode || String(form.value.search || '').trim().length < 2) return;
        searchTimer = setTimeout(() => searchCandidates(1), 300);
    }
);

watch(
    () => [form.value.selectedCandidate, form.value.fileId, form.value.layoutBoxes, form.value.selectedPresetId, activeStep.value],
    persistDraft,
    { deep: true }
);

watch(maxAllowedStep, (value) => {
    if (activeStep.value > value) activeStep.value = value;
});

onMounted(async () => {
    window.addEventListener('beforeunload', beforeUnload);
    openedAt = Date.now();
    restoreDraft();
    await loadPage();
    if (form.value.docFormatCode) await loadLayoutContext();
    recordCreateEvent('wizard_open');
});

onBeforeUnmount(() => {
    clearTimeout(searchTimer);
    window.removeEventListener('beforeunload', beforeUnload);
});

onBeforeRouteLeave((_to, _from, next) => {
    if (!dirty.value) {
        next();
        return;
    }
    confirm.require({
        message: 'ยังไม่ได้ส่งเอกสาร ต้องการออกจากหน้านี้หรือไม่?',
        header: 'ออกจากหน้าส่งเอกสาร',
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

function emptyForm() {
    return {
        docFormatCode: '',
        search: '',
        docNo: '',
        fileId: '',
        fileUrl: '',
        uploadedFile: null,
        selectedCandidate: null,
        confirmLocked: false,
        configs: [],
        presetTemplate: null,
        selectedPresetId: '',
        layoutBoxes: []
    };
}

async function loadPage() {
    loading.value = true;
    try {
        const result = await api.listSMLDocFormats();
        docFormats.value = result.docFormats || [];
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดชนิดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function searchCandidates(page = 1) {
    if (!form.value.docFormatCode) return;
    searchingCandidates.value = true;
    try {
        const result = await api.listSMLDocumentCandidates({
            docFormatCode: form.value.docFormatCode,
            search: form.value.search,
            page,
            size: 20
        });
        const rows = result.documents || [];
        candidates.value = page === 1 ? rows : [...candidates.value, ...rows];
        candidatePage.value = result.page || page;
        candidateTotal.value = result.total || 0;
        candidateHasMore.value = !!result.hasMore;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ค้นหาเอกสาร SML ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        searchingCandidates.value = false;
    }
}

function loadMoreCandidates() {
    if (!candidateHasMore.value || searchingCandidates.value) return;
    searchCandidates(candidatePage.value + 1);
}

function selectCandidate(candidate) {
    form.value.selectedCandidate = candidate;
    form.value.docNo = candidate.doc_no;
    form.value.confirmLocked = false;
    suppressSearchWatch = true;
    form.value.search = candidate.doc_no;
    persistDraft();
}

function triggerUpload() {
    fileInput.value?.click();
}

async function onFileChange(event) {
    const file = event.target.files?.[0] || null;
    event.target.value = '';
    if (!file) return;
    if (form.value.layoutBoxes.length > 0) {
        confirm.require({
            message: 'การเปลี่ยน PDF จะล้างกรอบลายเซ็นเดิมทั้งหมด ต้องการดำเนินการต่อหรือไม่?',
            header: 'เปลี่ยน PDF',
            icon: 'pi pi-exclamation-triangle',
            rejectProps: { label: 'ยกเลิก', severity: 'secondary', outlined: true },
            acceptProps: { label: 'เปลี่ยน PDF และล้างกรอบ', severity: 'warn' },
            accept: () => {
                void uploadSelectedPDF(file);
            }
        });
        return;
    }
    await uploadSelectedPDF(file);
}

async function uploadSelectedPDF(file) {
    form.value.fileId = '';
    form.value.fileUrl = '';
    form.value.uploadedFile = null;
    form.value.layoutBoxes = [];
    form.value.selectedPresetId = '';
    uploading.value = true;
    try {
        const result = await api.uploadSigningDocumentPDF(file);
        form.value.uploadedFile = result.file;
        form.value.fileId = result.file?.id || '';
        form.value.fileUrl = result.fileUrl || api.signingDocumentUploadPDFUrl(form.value.fileId);
        activeStep.value = Math.max(activeStep.value, 1);
        recordCreateEvent('pdf_upload_success');
        toast.add({ severity: 'success', summary: 'อัปโหลด PDF แล้ว', detail: `${result.file?.pageCount || 0} หน้า`, life: 2500 });
    } catch (err) {
        recordCreateEvent('pdf_upload_error');
        toast.add({ severity: 'error', summary: 'อัปโหลด PDF ไม่สำเร็จ', detail: err.message, life: 5000 });
    } finally {
        uploading.value = false;
    }
}

async function loadLayoutContext() {
    if (!form.value.docFormatCode) return;
    loadingLayoutContext.value = true;
    try {
        const [configsResult, templateResult] = await Promise.all([api.listDocumentConfigs({ docFormatCode: form.value.docFormatCode }), api.getSignatureTemplateState(form.value.docFormatCode).catch(() => ({}))]);
        form.value.configs = configsResult.configs || [];
        form.value.presetTemplate = templateResult.active || templateResult.draft || null;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลด Workflow ไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        loadingLayoutContext.value = false;
    }
}

function onApplyPreset(template) {
    form.value.selectedPresetId = template?.id || '';
    recordCreateEvent('preset_applied');
    toast.add({ severity: 'success', summary: 'ใช้กรอบเริ่มต้นแล้ว', detail: 'ตรวจตำแหน่งกับ PDF จริงก่อนส่งเซ็น', life: 3000 });
}

function onDesignerEvent(eventName) {
    if (eventName === 'preset_page_mismatch') {
        recordCreateEvent('validation_blocked');
        toast.add({ severity: 'warn', summary: 'ใช้กรอบเริ่มต้นไม่ได้', detail: 'จำนวนหน้า PDF ไม่ตรงกัน', life: 4000 });
        return;
    }
    if (eventName === 'layout_validation_error') recordCreateEvent('validation_blocked');
    else recordCreateEvent(eventName);
}

async function onDocFormatChange(nextCode) {
    if (nextCode === form.value.docFormatCode) return;
    const hasWorkToClear = !!form.value.selectedCandidate || !!form.value.fileId || form.value.layoutBoxes.length > 0;
    if (hasWorkToClear) {
        confirm.require({
            message: 'การเปลี่ยนชนิดเอกสารจะล้างเลขเอกสาร PDF และกรอบลายเซ็นเดิม ต้องการดำเนินการต่อหรือไม่?',
            header: 'เปลี่ยนชนิดเอกสาร',
            icon: 'pi pi-exclamation-triangle',
            rejectProps: { label: 'ยกเลิก', severity: 'secondary', outlined: true },
            acceptProps: { label: 'เปลี่ยนและล้างข้อมูลเดิม', severity: 'warn' },
            accept: () => {
                void applyDocFormatChange(nextCode);
            }
        });
        return;
    }
    await applyDocFormatChange(nextCode);
}

async function applyDocFormatChange(nextCode) {
    clearTimeout(searchTimer);
    form.value.docFormatCode = nextCode || '';
    resetCandidateSearch();
    resetUploadedLayout();
    activeStep.value = 0;
    persistDraft();
    if (form.value.docFormatCode) await loadLayoutContext();
}

function canOpenStep(index) {
    return index <= maxAllowedStep.value;
}

function setActiveStep(index) {
    if (!Number.isFinite(index)) return;
    if (canOpenStep(index)) {
        activeStep.value = Math.max(0, Math.min(index, wizardSteps.length - 1));
        return;
    }
    recordCreateEvent('validation_blocked');
    toast.add({ severity: 'warn', summary: 'ยังไปขั้นนี้ไม่ได้', detail: blockedReasonForStep(maxAllowedStep.value) || 'ทำขั้นตอนก่อนหน้าให้ครบก่อน', life: 3000 });
}

function nextStep() {
    if (currentStepReason.value) {
        recordCreateEvent('validation_blocked');
        toast.add({ severity: 'warn', summary: 'ยังไปขั้นถัดไปไม่ได้', detail: currentStepReason.value, life: 3000 });
        return;
    }
    recordCreateEvent('step_complete');
    activeStep.value = Math.min(activeStep.value + 1, wizardSteps.length - 1);
}

function backStep() {
    activeStep.value = Math.max(activeStep.value - 1, 0);
}

async function submitDocument() {
    const disabledReason = createDisabledReason.value;
    if (disabledReason) {
        recordCreateEvent('validation_blocked');
        toast.add({ severity: 'warn', summary: 'ยังส่งเซ็นไม่ได้', detail: disabledReason, life: 3500 });
        return;
    }
    if (lockedBySML.value && !form.value.confirmLocked) {
        confirm.require({
            message: 'เอกสารนี้ถูก lock ใน SML อยู่แล้ว ต้องการสร้างเอกสาร PaperLess จากเลขนี้หรือไม่?',
            header: 'ยืนยันเอกสาร SML ที่ lock แล้ว',
            icon: 'pi pi-exclamation-triangle',
            rejectProps: { label: 'ยกเลิก', severity: 'secondary', outlined: true },
            acceptProps: { label: 'ยืนยันและส่งเซ็น', severity: 'warn' },
            accept: () => {
                form.value.confirmLocked = true;
                void submitDocument();
            }
        });
        return;
    }

    creating.value = true;
    try {
        const result = await api.createSigningDocument({
            docFormatCode: form.value.docFormatCode,
            docNo: form.value.selectedCandidate.doc_no,
            fileId: form.value.fileId,
            signatureTemplateId: form.value.selectedPresetId,
            confirmLocked: form.value.confirmLocked,
            layoutBoxes: form.value.layoutBoxes.map(toLayoutPayload),
            idempotencyKey: createIdempotencyKey.value
        });
        createFinished = true;
        sessionStorage.removeItem(draftKey);
        sessionStorage.removeItem(legacyDraftKey);
        recordCreateEvent('create_success');
        toast.add({ severity: 'success', summary: 'ส่งเอกสารให้ผู้เซ็นแล้ว', life: 2500 });
        router.push({ name: 'signing-document-detail', params: { id: result.document.id } });
    } catch (err) {
        recordCreateEvent('create_error');
        toast.add({ severity: 'error', summary: 'สร้างเอกสารไม่สำเร็จ', detail: err.message, life: 5000 });
    } finally {
        creating.value = false;
    }
}

function blockedReasonForStep(index) {
    if (index === 0) {
        if (!form.value.docFormatCode) return 'เลือกชนิดเอกสารก่อน';
        if (!form.value.selectedCandidate) return 'เลือกเลขเอกสารจากผลค้นหา SML ก่อน';
    }
    if (index === 1) {
        if (!form.value.fileId) return 'อัปโหลด PDF จริงก่อน';
        if (form.value.layoutBoxes.length === 0) return 'วางกรอบลายเซ็นอย่างน้อย 1 กรอบ';
        if (layoutValidationIssues.value.length > 0) return layoutValidationIssues.value[0];
    }
    return '';
}

function finalDisabledReason() {
    for (let index = 0; index < wizardSteps.length - 1; index += 1) {
        const reason = blockedReasonForStep(index);
        if (reason) return reason;
    }
    return '';
}

function validateLayout() {
    const issues = [];
    const pageCount = Number(form.value.uploadedFile?.pageCount || 0);
    const boxes = form.value.layoutBoxes || [];
    const configsByPosition = new Map((form.value.configs || []).map((step) => [String(step.positionCode), step]));
    boxes.forEach((box) => {
        if (!configsByPosition.has(String(box.positionCode))) issues.push(`ตำแหน่ง ${box.positionCode} ไม่อยู่ใน Workflow`);
        if (box.pageNo < 1 || (pageCount && box.pageNo > pageCount)) issues.push(`กรอบ ${box.label || box.positionCode} อยู่หน้าที่ไม่ถูกต้อง`);
        if (box.xRatio < 0 || box.yRatio < 0 || box.widthRatio <= 0 || box.heightRatio <= 0 || box.xRatio + box.widthRatio > 1 || box.yRatio + box.heightRatio > 1) {
            issues.push(`กรอบ ${box.label || box.positionCode} อยู่นอกหน้า PDF`);
        }
    });
    (form.value.configs || []).forEach((step) => {
        const stepBoxes = boxes.filter((box) => String(box.positionCode) === String(step.positionCode));
        if (stepBoxes.length === 0) return;
        if (step.conditionType === 1 && stepBoxes.length !== 1) issues.push(`${step.positionName} ต้องมี 1 กรอบ`);
        if (step.conditionType === 3 && stepBoxes.length !== 1) issues.push(`${step.positionName} ต้องมี 1 กรอบบุคคลภายนอก`);
        if (step.conditionType === 2) {
            const seen = new Set();
            stepBoxes.forEach((box) => {
                const user = signerUsername(box.signerUser);
                if (!user) issues.push(`${step.positionName} ต้องเลือก user ทุกกรอบ`);
                if (user && seen.has(user)) issues.push(`${step.positionName} มี user ซ้ำ`);
                if (user) seen.add(user);
            });
        }
    });
    return [...new Set(issues)];
}

function resetCandidateSearch() {
    form.value.search = '';
    form.value.docNo = '';
    form.value.selectedCandidate = null;
    form.value.confirmLocked = false;
    candidates.value = [];
    candidatePage.value = 1;
    candidateTotal.value = 0;
    candidateHasMore.value = false;
}

function resetUploadedLayout() {
    form.value.fileId = '';
    form.value.fileUrl = '';
    form.value.uploadedFile = null;
    form.value.layoutBoxes = [];
    form.value.selectedPresetId = '';
}

function toLayoutPayload(box) {
    return {
        positionCode: box.positionCode,
        signerSlot: box.signerSlot,
        signerType: box.signerType,
        signerUser: box.signerUser || '',
        pageNo: box.pageNo,
        xRatio: box.xRatio,
        yRatio: box.yRatio,
        widthRatio: box.widthRatio,
        heightRatio: box.heightRatio,
        label: box.label || ''
    };
}

function persistDraft() {
    if (createFinished) return;
    const draft = {
        activeStep: activeStep.value,
        idempotencyKey: createIdempotencyKey.value,
        form: {
            ...form.value,
            configs: [],
            presetTemplate: null
        }
    };
    sessionStorage.setItem(draftKey, JSON.stringify(draft));
}

function restoreDraft() {
    try {
        let raw = sessionStorage.getItem(draftKey);
        const isLegacyDraft = !raw && !!sessionStorage.getItem(legacyDraftKey);
        if (!raw) raw = sessionStorage.getItem(legacyDraftKey);
        if (!raw) return;
        const parsed = JSON.parse(raw);
        if (parsed.idempotencyKey) createIdempotencyKey.value = parsed.idempotencyKey;
        suppressSearchWatch = true;
        form.value = { ...emptyForm(), ...(parsed.form || {}) };
        if (form.value.fileId && !form.value.fileUrl) form.value.fileUrl = api.signingDocumentUploadPDFUrl(form.value.fileId);
        const restoredStep = isLegacyDraft ? migrateLegacyStep(parsed.activeStep) : Number(parsed.activeStep || 0);
        activeStep.value = Math.min(Math.max(restoredStep, 0), wizardSteps.length - 1);
        if (activeStep.value > maxAllowedStep.value) activeStep.value = maxAllowedStep.value;
        if (isLegacyDraft) {
            sessionStorage.removeItem(legacyDraftKey);
            persistDraft();
        }
    } catch {
        sessionStorage.removeItem(draftKey);
        sessionStorage.removeItem(legacyDraftKey);
    }
}

function migrateLegacyStep(step) {
    const legacyStep = Number(step || 0);
    if (legacyStep <= 1) return 0;
    if (legacyStep <= 3) return 1;
    return 2;
}

function beforeUnload(event) {
    if (!dirty.value) return;
    event.preventDefault();
    event.returnValue = '';
}

function signerUsername(value) {
    return String(value || '').split(':')[0].trim().toLowerCase();
}

function recordCreateEvent(event) {
    const allowed = new Set([
        'wizard_open',
        'step_complete',
        'validation_blocked',
        'create_success',
        'create_error',
        'pdf_upload_success',
        'pdf_upload_error',
        'preset_applied',
        'box_add',
        'box_delete',
        'pdf_render_error'
    ]);
    if (!allowed.has(event)) return;
    void api
        .recordSigningDocumentCreateEvent({
            event,
            sessionId: createSessionId.value,
            docFormatCode: form.value.docFormatCode,
            elapsedMs: Date.now() - openedAt,
            boxCount: form.value.layoutBoxes.length,
            validationIssueCount: layoutValidationIssues.value.length,
            viewport: {
                width: window.innerWidth || 0,
                height: window.innerHeight || 0
            }
        })
        .catch(() => {});
}

function makeClientId() {
    return crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
}
</script>

<template>
    <div class="card min-w-0 overflow-hidden signing-create-card" :class="{ 'signing-create-card-designer': designerMode }">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div class="min-w-0">
                <div class="font-semibold text-xl mb-1 truncate">
                    {{ designerMode ? form.selectedCandidate?.doc_no || 'ส่งเอกสารใหม่' : 'ส่งเอกสารใหม่' }}
                </div>
                <p class="text-muted-color m-0 truncate">{{ designerMode ? selectedDocFormatLabel : headerSummary }}</p>
            </div>
            <div class="flex flex-wrap gap-2 md:justify-end">
                <Tag v-if="designerMode" value="PDF และกรอบ" severity="success" />
                <Button icon="pi pi-arrow-left" severity="secondary" rounded outlined aria-label="กลับ" @click="router.push({ name: 'signing-documents' })" />
            </div>
        </div>

        <Stepper v-model:value="activeStepValue" linear class="min-w-0 signing-create-stepper">
            <StepList v-show="!designerMode" class="min-w-0">
                <Step v-for="(step, index) in wizardSteps" :key="step.label" :value="index + 1" :disabled="!canOpenStep(index)">
                    <span class="hidden md:inline">{{ step.label }}</span>
                    <span class="md:hidden">{{ step.shortLabel }}</span>
                </Step>
            </StepList>

            <StepPanels class="min-w-0">
                <StepPanel :value="1">
                    <Panel header="เลือกเอกสาร" class="min-w-0">
                        <div class="flex min-w-0 flex-col gap-4">
                            <div class="grid min-w-0 grid-cols-12 gap-4">
                                <div class="col-span-12 min-w-0 lg:col-span-4">
                                    <label class="block font-bold mb-3">ชนิดเอกสาร</label>
                                    <Select
                                        :modelValue="form.docFormatCode"
                                        :options="docFormatOptions"
                                        optionLabel="label"
                                        optionValue="value"
                                        filter
                                        placeholder="เลือกชนิดเอกสาร"
                                        fluid
                                        :loading="loading"
                                        @update:modelValue="onDocFormatChange"
                                    />
                                    <small class="text-muted-color">ระบบไม่เลือกให้อัตโนมัติ เพื่อป้องกันส่งเอกสารผิดชนิด</small>
                                </div>
                                <div class="col-span-12 min-w-0 lg:col-span-8">
                                    <label class="block font-bold mb-3">ค้นหาเลขเอกสารจาก SML</label>
                                    <IconField>
                                        <InputIcon><i class="pi pi-search" /></InputIcon>
                                        <InputText v-model="form.search" placeholder="เช่น PO2606" fluid :disabled="!form.docFormatCode" />
                                    </IconField>
                                    <small class="text-muted-color">ต้องเลือกจากผลค้นหา SML เท่านั้น ระบบไม่รับเลขเอกสารที่พิมพ์เอง</small>
                                </div>
                            </div>

                            <Message v-if="form.selectedCandidate" severity="success">
                                เลือกเอกสาร {{ form.selectedCandidate.doc_no }} · {{ form.selectedCandidate.party_name || form.selectedCandidate.party_code || '-' }}
                            </Message>

                            <div class="min-w-0 max-w-full overflow-x-auto">
                                <DataTable
                                    :value="candidates"
                                    :loading="searchingCandidates"
                                    dataKey="doc_no"
                                    responsiveLayout="scroll"
                                    stripedRows
                                    scrollable
                                    scrollHeight="18rem"
                                    @row-click="selectCandidate($event.data)"
                                >
                                    <template #empty>
                                        <div class="py-6 text-center text-muted-color">{{ form.search?.length >= 2 ? 'ไม่พบเอกสาร' : 'พิมพ์อย่างน้อย 2 ตัวอักษรเพื่อค้นหา' }}</div>
                                    </template>
                                    <Column field="doc_no" header="เลขที่เอกสาร" style="min-width: 12rem">
                                        <template #body="{ data }">
                                            <div class="font-bold">{{ data.doc_no }}</div>
                                            <small class="text-muted-color">{{ data.party_name || data.party_code || '-' }}</small>
                                        </template>
                                    </Column>
                                    <Column field="doc_date" header="วันที่เอกสาร" style="min-width: 10rem">
                                        <template #body="{ data }">{{ formatDocumentDate(data.doc_date) }}</template>
                                    </Column>
                                    <Column field="total_amount" header="ยอดเงิน" style="min-width: 10rem">
                                        <template #body="{ data }">{{ Number(data.total_amount || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 }) }}</template>
                                    </Column>
                                    <Column header="สถานะ SML" style="min-width: 10rem">
                                        <template #body="{ data }">
                                            <Tag :value="Number(data.is_lock_record || 0) === 1 ? 'lock แล้ว' : 'ใช้งานได้'" :severity="Number(data.is_lock_record || 0) === 1 ? 'warn' : 'success'" />
                                        </template>
                                    </Column>
                                    <Column header="เลือก" style="width: 7rem">
                                        <template #body="{ data }">
                                            <Button
                                                :label="form.selectedCandidate?.doc_no === data.doc_no ? 'เลือกแล้ว' : 'เลือก'"
                                                :icon="form.selectedCandidate?.doc_no === data.doc_no ? 'pi pi-check' : 'pi pi-plus'"
                                                size="small"
                                                :severity="form.selectedCandidate?.doc_no === data.doc_no ? 'success' : 'secondary'"
                                                @click.stop="selectCandidate(data)"
                                            />
                                        </template>
                                    </Column>
                                </DataTable>
                            </div>

                            <div class="flex flex-wrap justify-between items-center gap-3">
                                <small class="text-muted-color">พบ {{ candidateTotal }} รายการ</small>
                                <Button v-if="candidateHasMore" label="โหลดเพิ่ม" severity="secondary" outlined :loading="searchingCandidates" @click="loadMoreCandidates" />
                            </div>
                            <Message v-if="lockedBySML" severity="warn">เอกสารนี้ lock ใน SML แล้ว ระบบจะถามยืนยันอีกครั้งก่อนส่งเซ็น</Message>
                        </div>
                    </Panel>
                </StepPanel>

                <StepPanel :value="2">
                    <div class="pdf-editor-shell">
                        <Toolbar class="pdf-editor-status">
                            <template #start>
                                <div>
                                    <div class="font-bold">ไฟล์ PDF สำหรับ {{ form.selectedCandidate?.doc_no || '-' }}</div>
                                    <small class="text-muted-color">
                                        {{ form.uploadedFile ? `${form.uploadedFile.originalName} · ${form.uploadedFile.pageCount} หน้า` : 'อัปโหลด PDF จริงจาก SML แล้ววางกรอบลายเซ็นบนไฟล์นี้' }}
                                    </small>
                                </div>
                            </template>
                            <template #end>
                                <input ref="fileInput" type="file" accept="application/pdf" class="hidden" @change="onFileChange" />
                                <Button :label="form.fileId ? 'เปลี่ยน PDF' : 'เลือกไฟล์ PDF'" :icon="form.fileId ? 'pi pi-refresh' : 'pi pi-upload'" :loading="uploading" @click="triggerUpload" />
                            </template>
                        </Toolbar>

                        <Message v-if="!form.uploadedFile" severity="info">อัปโหลด PDF ก่อน แล้วระบบจะแสดงพื้นที่วางกรอบลายเซ็นในหน้านี้ทันที</Message>
                        <Message v-if="loadingLayoutContext" severity="info">กำลังโหลด Workflow และกรอบเริ่มต้น...</Message>

                        <DocumentLayoutDesigner
                            v-if="activeStep === 1 && form.fileUrl"
                            v-model="form.layoutBoxes"
                            :pdfUrl="form.fileUrl"
                            :pageCount="form.uploadedFile?.pageCount || 0"
                            :configs="form.configs"
                            :presetTemplate="form.presetTemplate"
                            :fullHeight="designerMode"
                            @apply-preset="onApplyPreset"
                            @event="onDesignerEvent"
                        />
                    </div>
                </StepPanel>

                <StepPanel :value="3">
                    <Panel header="ตรวจสอบและส่งเซ็น" class="min-w-0">
                        <div class="grid min-w-0 grid-cols-12 gap-4">
                            <div class="col-span-12 min-w-0 md:col-span-6">
                                <dl class="grid grid-cols-12 gap-2 m-0">
                                    <dt class="col-span-5 text-muted-color">ชนิดเอกสาร</dt>
                                    <dd class="col-span-7 m-0">{{ selectedDocFormatLabel }}</dd>
                                    <dt class="col-span-5 text-muted-color">เลขที่เอกสาร</dt>
                                    <dd class="col-span-7 m-0">{{ form.selectedCandidate?.doc_no || '-' }}</dd>
                                    <dt class="col-span-5 text-muted-color">คู่ค้า</dt>
                                    <dd class="col-span-7 m-0">{{ form.selectedCandidate?.party_name || form.selectedCandidate?.party_code || '-' }}</dd>
                                    <dt class="col-span-5 text-muted-color">PDF</dt>
                                    <dd class="col-span-7 m-0">{{ form.uploadedFile?.originalName || '-' }}</dd>
                                    <dt class="col-span-5 text-muted-color">กรอบลายเซ็น</dt>
                                    <dd class="col-span-7 m-0">{{ form.layoutBoxes.length }} กรอบ</dd>
                                </dl>
                            </div>
                            <div class="col-span-12 min-w-0 md:col-span-6">
                                <Message v-if="createDisabledReason" severity="warn">{{ createDisabledReason }}</Message>
                                <Message v-else severity="success">ข้อมูลพร้อมส่งเซ็นแล้ว</Message>
                                <Message v-if="lockedBySML" severity="warn" class="mt-3">เอกสาร SML lock แล้ว ต้องยืนยันก่อนสร้างเอกสาร</Message>
                            </div>
                        </div>
                    </Panel>
                </StepPanel>
            </StepPanels>
        </Stepper>

        <Toolbar class="mt-3 signing-create-actions">
            <template #start>
                <Button label="ย้อนกลับ" icon="pi pi-arrow-left" severity="secondary" outlined :disabled="activeStep === 0" @click="backStep" />
            </template>
            <template #end>
                <div class="flex flex-wrap justify-end items-center gap-3">
                    <small v-if="activeStep < wizardSteps.length - 1 && currentStepReason" class="text-muted-color">{{ currentStepReason }}</small>
                    <Button v-if="activeStep < wizardSteps.length - 1" label="ถัดไป" icon="pi pi-arrow-right" iconPos="right" :disabled="!canGoNext" @click="nextStep" />
                    <Button v-else label="ส่งเซ็น" icon="pi pi-send" :loading="creating" :disabled="!!createDisabledReason || uploading" @click="submitDocument" />
                </div>
            </template>
        </Toolbar>
    </div>
</template>

<style scoped>
.signing-create-card {
    min-height: calc(100dvh - 6.25rem);
}

.signing-create-card-designer {
    display: flex;
    height: calc(100dvh - 6.25rem);
    min-height: 0;
    flex-direction: column;
    padding: 1rem;
}

.signing-create-card-designer .signing-create-stepper {
    display: flex;
    min-height: 0;
    flex: 1 1 auto;
    flex-direction: column;
}

.signing-create-card-designer :deep(.p-steppanels) {
    min-height: 0;
    flex: 1 1 auto;
    padding: 0;
}

.pdf-editor-shell {
    display: flex;
    min-height: 0;
    flex-direction: column;
    gap: 0.75rem;
}

.signing-create-card-designer .pdf-editor-shell {
    min-height: 0;
    flex: 1 1 auto;
}

.pdf-editor-status {
    padding: 0.55rem 0.75rem;
}

.signing-create-actions {
    position: sticky;
    bottom: 0;
    z-index: 2;
}

.signing-create-card-designer .signing-create-actions {
    padding: 0.55rem 0.75rem;
}

@media (max-width: 640px) {
    .signing-create-card-designer {
        padding: 0.75rem;
    }

    .pdf-editor-status :deep(.p-toolbar-start),
    .pdf-editor-status :deep(.p-toolbar-end) {
        width: 100%;
    }

    .pdf-editor-status :deep(.p-toolbar-end) {
        justify-content: flex-start;
    }
}
</style>
