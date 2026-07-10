<script setup>
import { api } from '@/services/api';
import { authStore } from '@/stores/auth';
import { formatDocumentDate } from '@/utils/signingFormatters';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const props = defineProps({
    visible: { type: Boolean, default: false }
});

const emit = defineEmits(['update:visible', 'completed']);
const confirm = useConfirm();
const toast = useToast();
const maxFiles = 30;
const maxFileBytes = 15 * 1024 * 1024;
const docFormats = ref([]);
const selectedDocFormatCode = ref('');
const contextReady = ref(false);
const contextMessage = ref('');
const rows = ref([]);
const phase = ref('select');
const busy = ref(false);
const contextVersion = ref('');
const batchID = ref(makeID());
const fileUpload = ref(null);
const cancelRequested = ref(false);
const activeControllers = new Set();

const docFormatOptions = computed(() =>
    docFormats.value.map((format) => ({
        label: `${format.code} - ${format.name_1 || format.name_2 || format.format || 'ไม่มีชื่อเอกสาร'}`,
        value: format.code
    }))
);
const selectedCount = computed(() => rows.value.length);
const readyCount = computed(() => rows.value.filter((row) => row.status === 'ready').length);
const warningCount = computed(() => rows.value.filter((row) => row.status === 'warning').length);
const invalidCount = computed(() => rows.value.filter((row) => row.status === 'invalid' || row.status === 'upload_failed').length);
const createdCount = computed(() => rows.value.filter((row) => row.status === 'created').length);
const failedCount = computed(() => rows.value.filter((row) => row.status === 'failed').length);
const processingCount = computed(() => rows.value.filter((row) => row.status === 'uploading' || row.status === 'creating').length);
const unconfirmedLockCount = computed(() => rows.value.filter((row) => row.status === 'warning' && hasIssue(row, 'sml_document_locked') && !row.confirmLocked).length);
const canCheck = computed(() => contextReady.value && rows.value.length > 0 && !busy.value && rows.value.some((row) => row.file || row.fileId));
const canImport = computed(() => !busy.value && rows.value.length > 0 && invalidCount.value === 0 && unconfirmedLockCount.value === 0 && readyCount.value + warningCount.value > 0);
const progressValue = computed(() => {
    const total = rows.value.filter((row) => ['creating', 'created', 'failed'].includes(row.status)).length;
    if (!total) return 0;
    return Math.round(((createdCount.value + failedCount.value) / total) * 100);
});
const hasRetryableUploadFailures = computed(() => rows.value.some((row) => row.status === 'upload_failed' && row.retryable && row.file));
const hasRetryableFailures = computed(() => rows.value.some((row) => row.status === 'failed' && row.retryable));
const needsRevalidation = computed(() => rows.value.some((row) => row.status === 'failed' && row.errorCode === 'batch_context_changed'));
const storageKey = computed(() => {
    const tenant = authStore.session?.smlTenant || authStore.session?.smlDataCode || 'tenant';
    const username = authStore.user?.username || 'user';
    return `paperless_batch_import_${String(tenant).toLowerCase()}_${String(username).toLowerCase()}`;
});

watch(
    () => props.visible,
    async (visible) => {
        if (!visible) return;
        await openDialog();
    }
);

watch(
    () => [selectedDocFormatCode.value, rows.value, contextVersion.value, phase.value],
    persistState,
    { deep: true }
);

onBeforeUnmount(() => abortActiveRequests());

async function openDialog() {
    if (!docFormats.value.length) {
        try {
            const result = await api.listSMLDocFormats();
            docFormats.value = result.docFormats || [];
        } catch (error) {
            toast.add({ severity: 'error', summary: 'โหลดชนิดเอกสารไม่สำเร็จ', detail: error.message, life: 4500 });
        }
    }
    const restored = restoreState();
    if (restored && selectedDocFormatCode.value) {
        await checkDocumentContext();
        if (rows.value.some((row) => row.fileId)) await recheckBatch();
    }
}

async function onDocFormatChange() {
    contextReady.value = false;
    contextMessage.value = '';
    contextVersion.value = '';
    if (!selectedDocFormatCode.value) return;
    await checkDocumentContext();
}

async function checkDocumentContext() {
    const code = selectedDocFormatCode.value;
    if (!code) return;
    busy.value = true;
    contextReady.value = false;
    contextMessage.value = 'กำลังตรวจสอบ Workflow และ Active Template';
    try {
        const [configsResult, templateResult] = await Promise.all([api.listDocumentConfigs({ docFormatCode: code }), api.getSignatureTemplateState(code)]);
        const configs = configsResult.configs || [];
        const active = templateResult.active;
        if (!configs.length) throw new Error('ยังไม่ได้กำหนด Workflow สำหรับชนิดเอกสารนี้');
        if (!active?.id || !active?.boxes?.length || !active?.legalNoticeBox) throw new Error('ยังไม่มี Active Template ที่มีกรอบลายเซ็นและข้อความกฎหมาย');
        contextReady.value = true;
        contextMessage.value = 'พร้อมรับไฟล์ PDF หลายเอกสาร';
    } catch (error) {
        contextMessage.value = error.message || 'ชนิดเอกสารนี้ยังไม่พร้อมสำหรับการนำเข้าหลายไฟล์';
    } finally {
        busy.value = false;
    }
}

function onFilesSelected(event) {
    const files = Array.from(event.files || []);
    const available = Math.max(0, maxFiles - rows.value.length);
    if (files.length > available) {
        toast.add({ severity: 'warn', summary: 'เลือกไฟล์เกินจำนวน', detail: `นำเข้าได้สูงสุด ${maxFiles} ไฟล์ต่อชุด`, life: 3500 });
    }
    for (const file of files.slice(0, available)) {
        const issues = [];
        if (!/\.pdf$/i.test(file.name || '')) issues.push(issue('filename_invalid', 'ชื่อไฟล์ต้องลงท้ายด้วย .pdf'));
        if (Number(file.size || 0) > maxFileBytes) issues.push(issue('pdf_too_large', 'ไฟล์ต้องมีขนาดไม่เกิน 15 MB'));
        rows.value.push({
            key: makeID(),
            file,
            fileId: '',
            originalName: file.name,
            sizeBytes: file.size || 0,
            pageCount: 0,
            docNo: deriveDisplayDocNo(file.name),
            status: issues.length ? 'invalid' : 'selected',
            issues,
            confirmLocked: false,
            retryable: false,
            errorCode: '',
            idempotencyKey: `batch-${batchID.value}-${makeID()}`
        });
    }
    fileUpload.value?.clear?.();
    phase.value = 'select';
}

async function uploadAndValidate() {
    if (!canCheck.value) return;
    cancelRequested.value = false;
    busy.value = true;
    phase.value = 'upload';
    try {
        const uploadRows = rows.value.filter((row) => row.file && !row.fileId && row.status !== 'invalid');
        await runPool(uploadRows, 2, uploadRow, () => cancelRequested.value);
        if (!cancelRequested.value) await validateUploadedRows();
    } finally {
        busy.value = false;
    }
}

async function uploadRow(row) {
    row.status = 'uploading';
    row.issues = [];
    const controller = new AbortController();
    activeControllers.add(controller);
    try {
        const result = await api.uploadSigningDocumentPDF(row.file, { signal: controller.signal });
        row.fileId = result.file?.id || '';
        row.originalName = result.file?.originalName || row.originalName;
        row.sizeBytes = Number(result.file?.sizeBytes || row.sizeBytes || 0);
        row.pageCount = Number(result.file?.pageCount || 0);
        row.file = null;
        row.status = 'uploaded';
    } catch (error) {
        row.status = 'upload_failed';
        row.issues = [issue(error.payload?.error || 'upload_failed', error.message || 'อัปโหลด PDF ไม่สำเร็จ', isRetryableError(error))];
        row.retryable = isRetryableError(error);
    } finally {
        activeControllers.delete(controller);
    }
}

async function validateUploadedRows() {
    const pending = rows.value.filter((row) => row.fileId && row.status !== 'created');
    if (!pending.length) {
        phase.value = 'review';
        return;
    }
    const controller = new AbortController();
    activeControllers.add(controller);
    try {
        const result = await api.validateSigningDocumentBatch(
            { docFormatCode: selectedDocFormatCode.value, fileIds: pending.map((row) => row.fileId) },
            { signal: controller.signal }
        );
        contextVersion.value = result.contextVersion || '';
        const byFileID = new Map((result.items || []).map((item) => [item.fileId, item]));
        for (const row of pending) {
            const item = byFileID.get(row.fileId);
            if (!item) {
                row.status = 'invalid';
                row.issues = [issue('validation_result_missing', 'ไม่ได้รับผลตรวจสอบของไฟล์นี้ กรุณาตรวจสอบใหม่', true)];
                continue;
            }
            row.originalName = item.originalName || row.originalName;
            row.docNo = item.docNo || row.docNo;
            row.pageCount = Number(item.pageCount || row.pageCount || 0);
            row.sizeBytes = Number(item.sizeBytes || row.sizeBytes || 0);
            row.candidate = item.candidate || null;
            row.duplicate = item.duplicate || null;
            row.status = item.status || 'invalid';
            row.issues = item.issues || [];
            row.retryable = false;
            row.errorCode = '';
            if (!hasIssue(row, 'sml_document_locked')) row.confirmLocked = false;
        }
        phase.value = 'review';
    } catch (error) {
        phase.value = 'review';
        if (!cancelRequested.value) toast.add({ severity: 'error', summary: 'ตรวจสอบรายการไม่สำเร็จ', detail: error.message, life: 5000 });
    } finally {
        activeControllers.delete(controller);
    }
}

async function importBatch(targetRows = null) {
    const targets = targetRows || rows.value.filter((row) => row.status === 'ready' || row.status === 'warning');
    if (!targets.length || !contextVersion.value) return;
    cancelRequested.value = false;
    busy.value = true;
    phase.value = 'import';
    await runPool(targets, 2, createRow, () => cancelRequested.value);
    busy.value = false;
    phase.value = cancelRequested.value ? 'review' : 'result';
    if (createdCount.value > 0) emit('completed', { created: createdCount.value, failed: failedCount.value });
    if (failedCount.value === 0) sessionStorage.removeItem(storageKey.value);
}

async function createRow(row) {
    row.status = 'creating';
    row.issues = [];
    const controller = new AbortController();
    activeControllers.add(controller);
    try {
        const result = await api.createSigningDocumentBatchItem(
            {
                docFormatCode: selectedDocFormatCode.value,
                fileId: row.fileId,
                contextVersion: contextVersion.value,
                confirmLocked: !!row.confirmLocked,
                idempotencyKey: row.idempotencyKey
            },
            { signal: controller.signal }
        );
        row.status = 'created';
        row.documentId = result.document?.id || '';
        row.retryable = false;
        row.errorCode = '';
    } catch (error) {
        row.status = 'failed';
        row.errorCode = error.payload?.error || 'batch_import_failed';
        row.retryable = isRetryableError(error);
        row.issues = [issue(row.errorCode, error.message || 'สร้าง Draft ไม่สำเร็จ', row.retryable)];
    } finally {
        activeControllers.delete(controller);
        persistState();
    }
}

async function retryFailures() {
    const targets = rows.value.filter((row) => row.status === 'failed' && row.retryable);
    if (!targets.length) return;
    void api.recordSigningDocumentBatchEvent({
        event: 'batch_retry',
        docFormatCode: selectedDocFormatCode.value,
        total: rows.value.length,
        created: createdCount.value,
        failed: targets.length
    }).catch(() => {});
    await importBatch(targets);
}

async function retryUploadFailures() {
    void api
        .recordSigningDocumentBatchEvent({
            event: 'batch_retry',
            docFormatCode: selectedDocFormatCode.value,
            total: rows.value.length,
            created: createdCount.value,
            failed: rows.value.filter((row) => row.status === 'upload_failed').length
        })
        .catch(() => {});
    await uploadAndValidate();
}

async function revalidateFailures() {
    await checkDocumentContext();
    if (contextReady.value) await recheckBatch();
}

async function recheckBatch() {
    if (busy.value) return;
    cancelRequested.value = false;
    busy.value = true;
    try {
        await validateUploadedRows();
    } finally {
        busy.value = false;
    }
}

async function removeRow(row) {
    if (busy.value || row.status === 'created') return;
    if (row.fileId) await api.discardSigningDocumentUpload(row.fileId).catch(() => {});
    rows.value = rows.value.filter((item) => item.key !== row.key);
    if (!rows.value.length) {
        phase.value = 'select';
        contextVersion.value = '';
        return;
    }
    if (phase.value === 'review' && rows.value.some((item) => item.fileId && item.status !== 'created')) {
        await recheckBatch();
    }
}

async function removeInvalidRows() {
    const targets = rows.value.filter((row) => row.status === 'invalid' || row.status === 'upload_failed');
    if (!targets.length || busy.value) return;
    busy.value = true;
    await runPool(
        targets.filter((row) => row.fileId),
        2,
        async (row) => api.discardSigningDocumentUpload(row.fileId).catch(() => {})
    );
    const keys = new Set(targets.map((row) => row.key));
    rows.value = rows.value.filter((row) => !keys.has(row.key));
    busy.value = false;
    if (!rows.value.length) {
        phase.value = 'select';
        contextVersion.value = '';
    } else if (phase.value === 'review' && rows.value.some((row) => row.fileId && row.status !== 'created')) {
        await recheckBatch();
    }
}

function requestVisibility(next) {
    if (next || busy.value) return;
    const discardable = rows.value.some((row) => row.fileId && row.status !== 'created') || rows.value.some((row) => row.file);
    if (!discardable) {
        closeDialog(rows.value.length === 0 || rows.value.every((row) => row.status === 'created'));
        return;
    }
    confirm.require({
        header: 'ยกเลิกการนำเข้าชุดนี้?',
        message: 'ไฟล์ที่ยังไม่ได้สร้าง Draft จะถูกลบออกจากระบบ',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: { label: 'ทำรายการต่อ', severity: 'secondary', outlined: true },
        acceptProps: { label: 'ยกเลิกและลบไฟล์', severity: 'danger' },
        accept: discardAndClose
    });
}

async function discardAndClose() {
    busy.value = true;
    abortActiveRequests();
    void api.recordSigningDocumentBatchEvent({
        event: 'batch_discard',
        docFormatCode: selectedDocFormatCode.value,
        total: rows.value.length,
        created: createdCount.value,
        failed: failedCount.value
    }).catch(() => {});
    const discardable = rows.value.filter((row) => row.fileId && row.status !== 'created');
    await runPool(discardable, 2, async (row) => api.discardSigningDocumentUpload(row.fileId).catch(() => {}));
    busy.value = false;
    sessionStorage.removeItem(storageKey.value);
    closeDialog(true);
}

function closeDialog(reset = false) {
    if (reset) resetState();
    emit('update:visible', false);
}

function resetState() {
    sessionStorage.removeItem(storageKey.value);
    selectedDocFormatCode.value = '';
    contextReady.value = false;
    contextMessage.value = '';
    rows.value = [];
    phase.value = 'select';
    contextVersion.value = '';
    batchID.value = makeID();
}

function persistState() {
    if (!props.visible) return;
    const pending = rows.value.filter((row) => row.fileId && row.status !== 'created');
    if (!pending.length || !selectedDocFormatCode.value) {
        if (createdCount.value > 0 && failedCount.value === 0) sessionStorage.removeItem(storageKey.value);
        return;
    }
    const value = {
        batchID: batchID.value,
        docFormatCode: selectedDocFormatCode.value,
        contextVersion: contextVersion.value,
        rows: pending.map((row) => ({
            key: row.key,
            fileId: row.fileId,
            originalName: row.originalName,
            sizeBytes: row.sizeBytes,
            pageCount: row.pageCount,
            docNo: row.docNo,
            status: row.status,
            issues: row.issues,
            confirmLocked: row.confirmLocked,
            retryable: row.retryable,
            errorCode: row.errorCode,
            idempotencyKey: row.idempotencyKey
        }))
    };
    sessionStorage.setItem(storageKey.value, JSON.stringify(value));
}

function restoreState() {
    if (rows.value.length) return false;
    try {
        const saved = JSON.parse(sessionStorage.getItem(storageKey.value) || 'null');
        if (!saved?.docFormatCode || !Array.isArray(saved.rows) || !saved.rows.length) return false;
        batchID.value = saved.batchID || makeID();
        selectedDocFormatCode.value = saved.docFormatCode;
        contextVersion.value = saved.contextVersion || '';
        rows.value = saved.rows.map((row) => ({ ...row, file: null, candidate: null, duplicate: null }));
        phase.value = 'review';
        return true;
    } catch {
        sessionStorage.removeItem(storageKey.value);
        return false;
    }
}

function abortActiveRequests() {
    for (const controller of activeControllers) controller.abort();
    activeControllers.clear();
}

function cancelBatchWork() {
    cancelRequested.value = true;
    abortActiveRequests();
    toast.add({ severity: 'info', summary: 'กำลังยกเลิกงาน', detail: 'รายการที่สร้างสำเร็จแล้วจะยังคงอยู่', life: 3000 });
}

async function runPool(items, concurrency, worker, shouldStop = () => false) {
    let cursor = 0;
    const runners = Array.from({ length: Math.min(concurrency, items.length) }, async () => {
        while (cursor < items.length && !shouldStop()) {
            const index = cursor++;
            await worker(items[index]);
        }
    });
    await Promise.all(runners);
}

function issue(code, message, retryable = false) {
    return { code, message, retryable };
}

function hasIssue(row, code) {
    return (row.issues || []).some((item) => item.code === code);
}

function statusLabel(row) {
    const labels = {
        selected: 'รออัปโหลด',
        uploading: 'กำลังอัปโหลด',
        uploaded: 'รอตรวจสอบ',
        ready: 'พร้อมนำเข้า',
        warning: 'ต้องยืนยัน SML Lock',
        invalid: 'ต้องแก้ไข',
        upload_failed: 'อัปโหลดไม่สำเร็จ',
        creating: 'กำลังสร้าง Draft',
        created: 'สร้างสำเร็จ',
        failed: 'สร้างไม่สำเร็จ'
    };
    return labels[row.status] || row.status;
}

function statusSeverity(row) {
    if (row.status === 'created' || row.status === 'ready') return 'success';
    if (row.status === 'warning' || row.status === 'selected' || row.status === 'uploaded') return 'warn';
    if (row.status === 'invalid' || row.status === 'upload_failed' || row.status === 'failed') return 'danger';
    return 'info';
}

function issueText(row) {
    return (row.issues || []).map((item) => item.message).filter(Boolean).join(' · ');
}

function deriveDisplayDocNo(name) {
    return String(name || '').replace(/\.pdf$/i, '').trim().toUpperCase();
}

function formatMoney(value) {
    return Number(value || 0).toLocaleString('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

function formatBytes(value) {
    const bytes = Number(value || 0);
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function isRetryableError(error) {
    const code = error?.payload?.error || '';
    if (code === 'batch_context_changed' || code === 'signing_document_duplicate' || code === 'filename_invalid') return false;
    return !error?.status || error.status >= 500 || code === 'idempotency_in_progress';
}

function makeID() {
    return crypto.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
</script>

<template>
    <Dialog
        :visible="visible"
        modal
        header="นำเข้า PDF หลายเอกสาร"
        :closable="!busy"
        :closeOnEscape="!busy"
        :style="{ width: 'min(1200px, 96vw)' }"
        :contentStyle="{ maxHeight: '76vh', overflow: 'auto' }"
        @update:visible="requestVisibility"
    >
        <div class="flex flex-col gap-5">
            <Message severity="info" :closable="false">
                เลือกชนิดเอกสารหนึ่งครั้ง แล้วตั้งชื่อ PDF ให้ตรงกับเลขเอกสาร เช่น <strong>QT26070001.pdf</strong> รองรับสูงสุด 30 ไฟล์ต่อชุด
            </Message>

            <div class="grid grid-cols-12 gap-4 items-end">
                <div class="col-span-12 md:col-span-7">
                    <label for="batch-doc-format" class="block font-bold mb-2">ชนิดเอกสาร</label>
                    <Select
                        id="batch-doc-format"
                        v-model="selectedDocFormatCode"
                        :options="docFormatOptions"
                        optionLabel="label"
                        optionValue="value"
                        filter
                        fluid
                        placeholder="เลือกชนิดเอกสาร"
                        :disabled="busy || rows.length > 0"
                        @change="onDocFormatChange"
                    />
                </div>
                <div class="col-span-12 md:col-span-5 flex md:justify-end">
                    <FileUpload
                        ref="fileUpload"
                        mode="basic"
                        name="batchDocuments[]"
                        accept="application/pdf,.pdf"
                        :multiple="true"
                        :maxFileSize="maxFileBytes"
                        :fileLimit="maxFiles"
                        customUpload
                        chooseLabel="เลือกไฟล์ PDF"
                        chooseIcon="pi pi-file-plus"
                        :disabled="!contextReady || busy || rows.length >= maxFiles"
                        @select="onFilesSelected"
                    />
                </div>
            </div>

            <Message v-if="selectedDocFormatCode" :severity="contextReady ? 'success' : 'warn'" :closable="false">
                {{ contextMessage || 'กรุณารอการตรวจสอบ Workflow และ Active Template' }}
            </Message>

            <div v-if="rows.length" class="flex flex-wrap items-center justify-between gap-3">
                <div class="flex flex-wrap gap-2">
                    <Tag :value="`ทั้งหมด ${selectedCount}`" severity="info" />
                    <Tag v-if="readyCount" :value="`พร้อม ${readyCount}`" severity="success" />
                    <Tag v-if="warningCount" :value="`ต้องยืนยัน ${warningCount}`" severity="warn" />
                    <Tag v-if="invalidCount" :value="`มีปัญหา ${invalidCount}`" severity="danger" />
                    <Tag v-if="createdCount" :value="`สำเร็จ ${createdCount}`" severity="success" />
                    <Tag v-if="failedCount" :value="`ล้มเหลว ${failedCount}`" severity="danger" />
                </div>
                <Button v-if="invalidCount && !busy" label="ลบรายการมีปัญหาทั้งหมด" icon="pi pi-trash" severity="danger" text @click="removeInvalidRows" />
            </div>

            <ProgressBar v-if="phase === 'import' || processingCount" :value="progressValue" />

            <DataTable v-if="rows.length" :value="rows" dataKey="key" responsiveLayout="scroll" stripedRows size="small">
                <Column header="ไฟล์ / เลขเอกสาร" style="min-width: 15rem">
                    <template #body="{ data }">
                        <div class="font-semibold">{{ data.originalName }}</div>
                        <small class="text-muted-color">{{ data.docNo || '-' }} · {{ formatBytes(data.sizeBytes) }}<span v-if="data.pageCount"> · {{ data.pageCount }} หน้า</span></small>
                    </template>
                </Column>
                <Column header="ข้อมูลจาก SML" style="min-width: 17rem">
                    <template #body="{ data }">
                        <template v-if="data.candidate">
                            <div>{{ formatDocumentDate(data.candidate.doc_date) }} · {{ data.candidate.party_name || data.candidate.party_code || '-' }}</div>
                            <small class="text-muted-color">มูลค่า {{ formatMoney(data.candidate.total_amount) }}</small>
                        </template>
                        <span v-else class="text-muted-color">-</span>
                    </template>
                </Column>
                <Column header="สถานะ" style="min-width: 12rem">
                    <template #body="{ data }">
                        <Tag :value="statusLabel(data)" :severity="statusSeverity(data)" />
                    </template>
                </Column>
                <Column header="รายละเอียด" style="min-width: 20rem">
                    <template #body="{ data }">
                        <div v-if="issueText(data)" :class="data.status === 'warning' ? 'text-orange-600' : 'text-red-600'">{{ issueText(data) }}</div>
                        <div v-else-if="data.status === 'created'" class="text-green-600">สร้างเอกสารเตรียมส่งเรียบร้อยแล้ว</div>
                        <span v-else class="text-muted-color">-</span>
                        <div v-if="data.status === 'warning' && hasIssue(data, 'sml_document_locked')" class="flex items-center gap-2 mt-2">
                            <Checkbox v-model="data.confirmLocked" binary :inputId="`locked-${data.key}`" />
                            <label :for="`locked-${data.key}`">ยืนยันนำเข้าเอกสารที่ Lock แล้ว</label>
                        </div>
                    </template>
                </Column>
                <Column header="จัดการ" :exportable="false" style="width: 6rem">
                    <template #body="{ data }">
                        <Button
                            v-if="data.status !== 'created'"
                            icon="pi pi-trash"
                            severity="danger"
                            text
                            rounded
                            aria-label="ลบรายการ"
                            :disabled="busy"
                            @click="removeRow(data)"
                        />
                        <Button
                            v-else-if="data.documentId"
                            icon="pi pi-check"
                            severity="success"
                            text
                            rounded
                            aria-label="สร้างสำเร็จ"
                            disabled
                        />
                    </template>
                </Column>
                <template #empty>
                    <div class="py-8 text-center text-muted-color">ยังไม่ได้เลือกไฟล์ PDF</div>
                </template>
            </DataTable>
        </div>

        <template #footer>
            <div class="flex flex-wrap justify-between gap-3 w-full">
                <Button v-if="busy" label="ยกเลิกงาน" icon="pi pi-stop-circle" severity="danger" outlined @click="cancelBatchWork" />
                <Button v-else label="ปิด" icon="pi pi-times" severity="secondary" text @click="requestVisibility(false)" />
                <div class="flex flex-wrap gap-2">
                    <Button
                        v-if="phase === 'select' || phase === 'upload'"
                        label="อัปโหลดและตรวจสอบ"
                        icon="pi pi-search"
                        :loading="busy"
                        :disabled="!canCheck || invalidCount > 0"
                        @click="uploadAndValidate"
                    />
                    <Button v-if="hasRetryableUploadFailures && !busy" label="ลองอัปโหลดอีกครั้ง" icon="pi pi-replay" severity="warn" @click="retryUploadFailures" />
                    <Button
                        v-if="phase === 'review'"
                        label="ตรวจสอบใหม่"
                        icon="pi pi-refresh"
                        severity="secondary"
                        outlined
                        :loading="busy"
                        :disabled="!rows.some((row) => row.fileId && row.status !== 'created')"
                        @click="recheckBatch"
                    />
                    <Button
                        v-if="phase === 'review'"
                        :label="`ยืนยันนำเข้า ${readyCount + warningCount} เอกสาร`"
                        icon="pi pi-check"
                        :disabled="!canImport"
                        @click="importBatch()"
                    />
                    <Button v-if="needsRevalidation" label="ตรวจสอบ Workflow/Template ใหม่" icon="pi pi-refresh" severity="warn" @click="revalidateFailures" />
                    <Button v-if="hasRetryableFailures" label="ลองรายการล้มเหลวอีกครั้ง" icon="pi pi-replay" severity="warn" @click="retryFailures" />
                    <Button v-if="phase === 'result' && failedCount === 0" label="เสร็จสิ้น" icon="pi pi-check" @click="closeDialog(true)" />
                </div>
            </div>
        </template>
    </Dialog>
</template>
