<script setup>
import { api } from '@/services/api';
import { authStore } from '@/stores/auth';
import { formatDocumentDate, formatThaiDateTime, signingStatusLabel, signingStatusSeverity, smlImageFailureDetail } from '@/utils/signingFormatters';
import DocumentAttachmentActionButton from '@/views/signing/components/DocumentAttachmentActionButton.vue';
import DocumentAttachmentsDialog from '@/views/signing/components/DocumentAttachmentsDialog.vue';
import DocumentFlowDialog from '@/views/signing/components/DocumentFlowDialog.vue';
import DocumentReferenceCheck from '@/views/signing/components/DocumentReferenceCheck.vue';
import BatchDocumentImportDialog from '@/views/signing/components/BatchDocumentImportDialog.vue';
import ReadOnlyPdfDialog from '@/views/signing/components/ReadOnlyPdfDialog.vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const confirm = useConfirm();
const toast = useToast();

const documents = ref([]);
const loading = ref(false);
const searchQuery = ref('');
const transitioningIds = ref(new Set());
const flowDialog = ref(false);
const flowDocument = ref(null);
const referenceDialog = ref(false);
const referenceDocument = ref(null);
const readonlyPdfDialog = ref(false);
const readonlyPdfUrl = ref('');
const readonlyPdfTitle = ref('');
const attachmentsDialog = ref(false);
const attachmentsDocument = ref(null);
const externalSignerDialog = ref(false);
const externalSignerDocument = ref(null);
const tokenDialog = ref(false);
const generatedToken = ref(null);
const generatingExternalIds = ref(new Set());
const copyFallbackVisible = ref(false);
const copyFallbackValue = ref('');
const batchImportDialog = ref(false);

let searchTimer = null;

const queue = computed(() => route.meta.queue || 'active');
const internalDocumentsEnabled = computed(() => authStore.features?.internalDocuments === true);
const pageConfig = computed(() => {
    if (queue.value === 'draft') {
        return {
            title: 'เอกสารเตรียมส่ง',
            subtitle: 'เอกสารที่สร้างไว้แล้ว แต่ยังไม่ส่งให้ผู้เซ็น',
            empty: 'ยังไม่มีเอกสารเตรียมส่ง',
            searchPlaceholder: 'ค้นหาเลขเอกสาร หรือคู่ค้า',
            countSeverity: 'secondary',
            showCreate: true
        };
    }
    if (queue.value === 'history') {
        return {
            title: 'ประวัติเอกสารเซ็น',
            subtitle: 'เอกสารที่เซ็นครบและจัดเก็บเสร็จสมบูรณ์',
            empty: 'ยังไม่มีประวัติเอกสารเซ็น',
            searchPlaceholder: 'ค้นหาเลขเอกสาร หรือคู่ค้า',
            countSeverity: 'success',
            showCreate: false
        };
    }
    return {
        title: 'เอกสารรอเซ็น',
        subtitle: 'ติดตามเอกสารที่ส่งไปเซ็น รอยืนยัน หรือมีปัญหาที่ต้องแก้',
        empty: 'ยังไม่มีเอกสารรอเซ็น',
        searchPlaceholder: 'ค้นหาเลขเอกสาร คู่ค้า สถานะ',
        countSeverity: 'info',
        showCreate: false
    };
});
const filteredDocuments = computed(() => documents.value);
const referenceDialogTitle = computed(() => {
    const doc = referenceDocument.value || {};
    const docNo = doc.docNo || doc.doc_no || '';
    const formatCode = doc.docFormatCode || doc.doc_format_code || '';
    const party = doc.partyName || doc.party_name || doc.partyCode || doc.party_code || '';
    const parts = [[docNo, formatCode].filter(Boolean).join(' ~ '), party].filter((part) => part && part !== '-');
    return parts.join(' · ');
});
const attachmentsDialogTitle = computed(() => {
    const docNo = attachmentsDocument.value?.docNo || attachmentsDocument.value?.doc_no || '';
    return docNo ? `ไฟล์แนบอ้างอิง · ${docNo}` : 'ไฟล์แนบอ้างอิง';
});
const attachmentsDialogSubtitle = computed(() => {
    const doc = attachmentsDocument.value || {};
    const parts = [
        doc.docFormatCode || doc.doc_format_code,
        doc.partyName || doc.party_name || doc.partyCode || doc.party_code,
        formatDocumentDate(doc.docDate || doc.doc_date)
    ].filter((part) => part && part !== '-');
    return parts.join(' · ');
});
const attachmentsDialogKey = computed(() => attachmentsDocument.value?.id || '');

onMounted(loadPage);

watch(
    () => route.name,
    () => {
        documents.value = [];
        void loadPage();
    }
);

watch(searchQuery, () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => void loadPage(), 300);
});

watch(
    () => [route.query.flow_doc_no, route.query.flow_doc_format_code],
    ([docNo, docFormatCode]) => {
        if (!docNo) return;
        void openDocumentFlow(
            {
                docNo: String(docNo),
                docFormatCode: String(docFormatCode || '')
            },
            { syncQuery: false }
        );
    },
    { immediate: true }
);

onBeforeUnmount(() => {
    clearTimeout(searchTimer);
});

async function loadPage() {
    loading.value = true;
    try {
        const result = await api.listSigningDocuments({ queue: queue.value, search: searchQuery.value, page: 1, size: 100 });
        documents.value = result.documents || [];
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openCreate() {
    router.push({ name: 'signing-document-new' });
}

function openInternalCreate() {
    router.push({ name: 'internal-document-new' });
}

function openInternalEdit(doc) {
    if (!doc?.internalDocumentId) return;
    router.push({ name: 'internal-document-edit', params: { id: doc.internalDocumentId } });
}

function openBatchImport() {
    batchImportDialog.value = true;
}

async function onBatchImportCompleted(result = {}) {
    await loadPage();
    toast.add({
        severity: result.failed ? 'warn' : 'success',
        summary: result.failed ? 'นำเข้าแล้วบางส่วน' : 'สร้างเอกสารเตรียมส่งแล้ว',
        detail: `สำเร็จ ${result.created || 0} รายการ${result.failed ? ` · ล้มเหลว ${result.failed}` : ''}`,
        life: 3500
    });
}

function openDetail(doc) {
    if (!doc?.id) return;
    router.push({ name: 'signing-document-detail', params: { id: doc.id }, query: { from_queue: queue.value } });
}

function referenceDocumentUrl(documentId) {
    return router.resolve({ name: 'signing-document-detail', params: { id: documentId }, query: { from_queue: queue.value } }).href;
}

function openDocumentFlow(doc, options = {}) {
    const docNo = String(doc?.docNo || doc?.doc_no || '').trim().toUpperCase();
    if (!docNo) return;
    const docFormatCode = String(doc?.docFormatCode || doc?.doc_format_code || '').trim().toUpperCase();
    flowDocument.value = {
        docNo,
        docFormatCode,
        partyName: doc?.partyName || doc?.party_name || '',
        partyCode: doc?.partyCode || doc?.party_code || ''
    };
    flowDialog.value = true;

    if (options.syncQuery !== false) {
        const { flow_doc_no: _flowDocNo, flow_doc_format_code: _flowDocFormatCode, ...rest } = route.query;
        router.replace({
            name: route.name,
            query: {
                ...rest,
                flow_doc_no: docNo,
                ...(docFormatCode ? { flow_doc_format_code: docFormatCode } : {})
            }
        });
    }
}

function closeFlowDialog() {
    flowDialog.value = false;
    const { flow_doc_no: _flowDocNo, flow_doc_format_code: _flowDocFormatCode, ...rest } = route.query;
    if (_flowDocNo || _flowDocFormatCode) router.replace({ name: route.name, query: rest });
}

function setFlowDialogVisible(value) {
    if (value) {
        flowDialog.value = true;
        return;
    }
    closeFlowDialog();
}

function openDocumentFlowFromRow(doc) {
    if (!doc?.docNo) return;
    openDocumentFlow(doc);
}

function openReferenceCheck(doc) {
    if (!doc?.id) return;
    referenceDocument.value = doc;
    referenceDialog.value = true;
}

function loadReferenceCheckForDialog() {
    if (!referenceDocument.value?.id) return Promise.resolve({ referenceCheck: { items: [], summary: { total: 0, missing: 0, inProgress: 0, completed: 0 } } });
    return api.getSigningDocumentReferenceCheck(referenceDocument.value.id);
}

function previewDocumentPDF(doc, version = 'current') {
    if (!doc?.id) return;
    const url = api.signingDocumentPDFUrlForDocument(doc, version);
    if (!url) {
        toast.add({ severity: 'info', summary: 'ยังไม่มี PDF', detail: `${doc.docNo || 'เอกสารนี้'} ยังไม่มีไฟล์ PDF`, life: 3000 });
        return;
    }
    readonlyPdfUrl.value = url;
    readonlyPdfTitle.value = `${doc.docNo || 'เอกสาร'} · ${version === 'final' ? 'หลักฐานการลงนาม' : 'เอกสารเซ็นครบ'}`;
    readonlyPdfDialog.value = true;
}

function openAttachmentsDialog(doc) {
    if (!doc?.id || attachmentCount(doc) <= 0) return;
    attachmentsDocument.value = doc;
    attachmentsDialog.value = true;
}

function loadAttachmentsForDialog() {
    if (!attachmentsDocument.value?.id) return Promise.resolve({ attachments: [] });
    return api.getSigningDocumentAttachments(attachmentsDocument.value.id);
}

function attachmentFileUrlForDialog(attachment) {
    if (!attachmentsDocument.value?.id || !attachment?.id) return '';
    return api.signingDocumentAttachmentFileUrl(attachmentsDocument.value.id, attachment.id);
}

function confirmSend(doc) {
    if (isInternalDocument(doc) && !doc.internalCurrentRevisionPrinted) {
        toast.add({ severity: 'warn', summary: 'กรุณาพิมพ์ PDF ก่อนส่ง', detail: 'ต้องพิมพ์ revision ล่าสุดของเอกสารภายในก่อนส่งเข้า Workflow', life: 3500 });
        return;
    }
    confirm.require({
        header: 'ส่งเอกสารไปเซ็น',
        message: `ต้องการส่ง ${doc.docNo} ให้ผู้เซ็นใช่ไหม? หลังส่งแล้วเอกสารจะย้ายไปเมนูเอกสารรอเซ็น`,
        icon: 'pi pi-send',
        acceptLabel: 'ส่งไปเซ็น',
        rejectLabel: 'ยกเลิก',
        accept: () => transitionDocument(doc, 'send')
    });
}

async function printInternalDraft(doc) {
    if (!doc?.internalDocumentId) return;
    const popup = window.open('', '_blank');
    setTransitioning(doc.id, true);
    try {
        const result = await api.printInternalDocument(doc.internalDocumentId);
        const response = await fetch(result.pdfUrl || api.internalDocumentPDFUrl(doc.internalDocumentId, doc.internalRevision), { headers: api.authHeaders(), cache: 'no-store' });
        if (!response.ok) throw new Error('โหลด PDF สำหรับพิมพ์ไม่สำเร็จ');
        const objectUrl = URL.createObjectURL(await response.blob());
        if (popup) popup.location.href = objectUrl;
        else window.open(objectUrl, '_blank');
        setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000);
        toast.add({ severity: 'success', summary: 'PDF revision ล่าสุดพร้อมพิมพ์แล้ว', life: 2600 });
        await loadPage();
    } catch (error) {
        if (popup) popup.close();
        toast.add({ severity: 'error', summary: 'พิมพ์ PDF ไม่สำเร็จ', detail: error.message, life: 4200 });
    } finally {
        setTransitioning(doc.id, false);
    }
}

function confirmAdminConfirm(doc) {
    const message = isInternalDocument(doc)
        ? `ต้องการยืนยัน ${doc.docNo} ใช่ไหม? ระบบจะสร้าง PDF ฉบับสมบูรณ์และหลักฐานการลงนามใน PaperLess`
        : `ต้องการยืนยัน ${doc.docNo} ใช่ไหม? ระบบจะสร้าง final PDF/evidence ส่งรูปเข้า SML และ Lock SML`;
    confirm.require({
        header: 'ยืนยันเอกสาร',
        message,
        icon: 'pi pi-check-circle',
        acceptLabel: 'ยืนยันเอกสาร',
        rejectLabel: 'ยกเลิก',
        accept: () => transitionDocument(doc, 'confirm')
    });
}

function confirmCancel(doc) {
    confirm.require({
        header: 'ยกเลิกเอกสาร',
        message: `ต้องการยกเลิก ${doc.docNo} ใช่ไหม? เอกสารจะไม่ถูกส่งไปเซ็น`,
        icon: 'pi pi-exclamation-triangle',
        acceptLabel: 'ยกเลิกเอกสาร',
        rejectLabel: 'กลับ',
        acceptClass: 'p-button-danger',
        accept: () => transitionDocument(doc, 'cancel')
    });
}

async function transitionDocument(doc, action) {
    if (!doc?.id) return;
    setTransitioning(doc.id, true);
    try {
        const payload = { idempotencyKey: makeIdempotencyKey(action, doc.id) };
        let result;
        if (action === 'send') result = await api.sendSigningDocument(doc.id, payload);
        if (action === 'confirm') result = await api.confirmSigningDocument(doc.id, payload);
        if (action === 'cancel') result = await api.cancelSigningDocument(doc.id, payload);

        if (action === 'send') {
            toast.add({ severity: 'success', summary: 'ส่งเอกสารไปเซ็นแล้ว', life: 2500 });
        } else if (action === 'confirm') {
            toast.add({
                severity: confirmResultSeverity(result),
                summary: confirmResultSummary(result),
                detail: confirmResultDetail(result),
                life: 4000
            });
        } else if (action === 'cancel') {
            toast.add({ severity: 'success', summary: 'ยกเลิกเอกสารแล้ว', life: 2500 });
        }
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: actionErrorTitle(action), detail: err.message, life: 4000 });
    } finally {
        setTransitioning(doc.id, false);
    }
}

function setTransitioning(id, active) {
    const next = new Set(transitioningIds.value);
    if (active) next.add(id);
    else next.delete(id);
    transitioningIds.value = next;
}

function isTransitioning(id) {
    return transitioningIds.value.has(id);
}

function setGeneratingExternal(id, active) {
    const next = new Set(generatingExternalIds.value);
    if (active) next.add(id);
    else next.delete(id);
    generatingExternalIds.value = next;
}

function isGeneratingExternal(id) {
    return generatingExternalIds.value.has(id);
}

function makeIdempotencyKey(action, id) {
    return `${action}-${id}-${crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`}`;
}

function actionErrorTitle(action) {
    if (action === 'send') return 'ส่งเอกสารไม่สำเร็จ';
    if (action === 'confirm') return 'ยืนยันเอกสารไม่สำเร็จ';
    if (action === 'cancel') return 'ยกเลิกเอกสารไม่สำเร็จ';
    return 'ดำเนินการไม่สำเร็จ';
}

function confirmResultSeverity(result = {}) {
    return result.finalOk && result.imageOk && result.lockOk ? 'success' : 'warn';
}

function confirmResultSummary(result = {}) {
    return result.finalOk && result.imageOk && result.lockOk ? 'ยืนยันเอกสารสำเร็จ' : 'ยืนยันแล้วแต่ยังมีงานต้องตรวจสอบ';
}

function confirmResultDetail(result = {}) {
    if (!result.finalOk) return 'สร้าง final PDF/evidence ไม่สำเร็จ กรุณา retry';
    if (!result.imageOk) return smlImageFailureDetail(result, 'ส่งรูปเอกสารเข้า SML ไม่สำเร็จ กรุณาเปิดเอกสารเพื่อ retry');
    if (!result.lockOk) return ['Lock SML ไม่สำเร็จ กรุณาเปิดเอกสารเพื่อ retry', imageTruncatedDetail(result)].filter(Boolean).join(' · ');
    return imageTruncatedDetail(result);
}

function imageTruncatedDetail(result = {}) {
    const image = result.image || {};
    if (!image.truncated) return '';
    return `ส่งรูปเข้า SML เฉพาะ ${image.imageCount || 8} จาก ${image.totalPages || '-'} หน้าแรก`;
}

function documentLine(doc) {
    return `${doc.docNo || '-'} ~ ${doc.docFormatCode || '-'} · ${doc.partyName || doc.partyCode || '-'}`;
}

function attachmentCount(doc) {
    return Number(doc?.attachmentCount || 0);
}

function formatMoney(value) {
    return Number(value || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 });
}

function pendingSigners(doc) {
    const list = Array.isArray(doc?.pendingSigners) ? doc.pendingSigners : [];
    if (list.length > 0) return list;
    return (doc?.signers || []).filter((signer) => signer.status === 'pending');
}

function pendingExternalSigners(doc) {
    return pendingSigners(doc).filter((signer) => signer.signerType === 'external');
}

function canGenerateExternalFromList(doc) {
    return doc?.status === 'in_progress' && pendingExternalSigners(doc).length > 0;
}

function isGeneratingExternalForDocument(doc) {
    return pendingExternalSigners(doc).some((signer) => isGeneratingExternal(signer.id));
}

function waitingSummary(doc) {
    if (doc?.status === 'in_progress') {
        const signers = pendingSigners(doc);
        if (signers.length === 0) return 'ยังไม่พบผู้เซ็นที่รอดำเนินการ';
        const first = signerDisplayName(signers[0]);
        return signers.length > 1 ? `รอ: ${first} +${signers.length - 1} คน` : `รอ: ${first}`;
    }
    if (doc?.status === 'pending_confirm') return isInternalDocument(doc) ? 'เซ็นครบแล้ว รอระบบสร้างเอกสารฉบับสมบูรณ์' : 'เซ็นครบแล้ว รอระบบส่งเข้า SML';
    if (doc?.status === 'auto_confirming') return isInternalDocument(doc) ? 'กำลังสร้าง PDF ฉบับสมบูรณ์' : 'กำลังสร้าง PDF และส่งเข้า SML';
    if (doc?.status === 'completed_evidence_failed') return 'ต้องสร้าง PDF หลักฐานอีกครั้ง';
    if (doc?.status === 'completed_image_failed') return 'ต้องส่งรูปเอกสารเข้า SML อีกครั้ง';
    if (doc?.status === 'completed_lock_failed') return 'ต้อง Lock SML อีกครั้ง';
    if (doc?.status === 'rejected') return 'workflow หยุดแล้ว';
    return '';
}

function isInternalDocument(doc) {
    return doc?.documentSource === 'internal';
}

function documentStatusLabel(doc) {
    if (isInternalDocument(doc) && doc?.status === 'pending_confirm') return 'รอสร้างเอกสารสมบูรณ์';
    if (isInternalDocument(doc) && doc?.status === 'auto_confirming') return 'กำลังสร้าง PDF';
    return signingStatusLabel(doc?.status);
}

function signerDisplayName(signer) {
    const name = signer?.signerType === 'external' ? signer?.signerName || 'บุคคลภายนอก' : signer?.signerName || signer?.signerUser || 'ไม่ระบุผู้เซ็น';
    const position = signer?.positionName || signer?.positionCode || '';
    return position ? `${name} · ${position}` : name;
}

function signerTypeLabel(signer) {
    return signer?.signerType === 'external' ? 'บุคคลภายนอก' : 'ผู้ใช้ระบบ';
}

function openExternalSignerFromRow(doc) {
    const signers = pendingExternalSigners(doc);
    if (signers.length === 0) return;
    if (signers.length === 1) {
        requestExternalToken(signers[0], doc);
        return;
    }
    externalSignerDocument.value = doc;
    externalSignerDialog.value = true;
}

function requestExternalToken(signer, doc = externalSignerDocument.value) {
    if (!signer?.id) return;
    if (signer.status !== 'pending') {
        toast.add({ severity: 'info', summary: 'ยังสร้างลิงก์ไม่ได้', detail: signer.status === 'waiting' ? 'ยังไม่ถึงคิวผู้เซ็นภายนอกคนนี้' : 'ผู้เซ็นภายนอกคนนี้ไม่พร้อมใช้งานแล้ว', life: 3000 });
        return;
    }
    if (!signer.externalTokenId) {
        void generateExternalToken(signer, doc);
        return;
    }
    confirm.require({
        header: 'สร้างลิงก์ใหม่?',
        message: 'ลิงก์และ OTP เดิมของผู้เซ็นภายนอกคนนี้จะใช้ไม่ได้ ต้องส่งลิงก์ใหม่ให้ลูกค้าอีกครั้ง',
        icon: 'pi pi-exclamation-triangle',
        rejectLabel: 'ยกเลิก',
        acceptLabel: 'สร้างใหม่',
        accept: () => generateExternalToken(signer, doc)
    });
}

async function generateExternalToken(signer, doc) {
    setGeneratingExternal(signer.id, true);
    copyFallbackVisible.value = false;
    copyFallbackValue.value = '';
    try {
        const result = await api.regenerateExternalToken(signer.id);
        generatedToken.value = {
            ...(result.external || {}),
            docNo: doc?.docNo || '',
            signerLabel: signerDisplayName(signer)
        };
        tokenDialog.value = true;
        externalSignerDialog.value = false;
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'สร้างลิงก์ภายนอกไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        setGeneratingExternal(signer.id, false);
    }
}

async function copyText(value) {
    const text = String(value || '');
    if (!text) return;
    copyFallbackVisible.value = false;
    copyFallbackValue.value = '';
    try {
        await navigator.clipboard.writeText(text);
        toast.add({ severity: 'success', summary: 'คัดลอกแล้ว', life: 1800 });
        return;
    } catch {
        if (legacyCopy(text)) {
            toast.add({ severity: 'success', summary: 'คัดลอกแล้ว', life: 1800 });
            return;
        }
    }
    copyFallbackValue.value = text;
    copyFallbackVisible.value = true;
    toast.add({ severity: 'warn', summary: 'คัดลอกอัตโนมัติไม่ได้', detail: 'กรุณาเลือกข้อความแล้วคัดลอกเอง', life: 4000 });
}

function legacyCopy(value) {
    const textarea = window.document.createElement('textarea');
    textarea.value = value;
    textarea.setAttribute('readonly', '');
    textarea.style.position = 'fixed';
    textarea.style.top = '-1000px';
    textarea.style.opacity = '0';
    window.document.body.appendChild(textarea);
    textarea.select();
    try {
        return window.document.execCommand('copy');
    } catch {
        return false;
    } finally {
        window.document.body.removeChild(textarea);
    }
}

function selectInput(event) {
    event?.target?.select?.();
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div class="min-w-0 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                <div class="font-semibold text-xl whitespace-nowrap truncate">{{ pageConfig.title }}</div>
                <p class="text-muted-color m-0 min-w-0 truncate">{{ pageConfig.subtitle }}</p>
                <Tag :value="`${documents.length} รายการ`" :severity="pageConfig.countSeverity" />
            </div>
            <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                <IconField class="w-full sm:w-80">
                    <InputIcon><i class="pi pi-search" /></InputIcon>
                    <InputText v-model="searchQuery" type="search" :placeholder="pageConfig.searchPlaceholder" class="w-full" />
                </IconField>
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadPage" />
                <Button v-if="pageConfig.showCreate" label="นำเข้าหลายไฟล์" icon="pi pi-upload" severity="secondary" outlined @click="openBatchImport" />
                <Button v-if="pageConfig.showCreate && internalDocumentsEnabled" label="สร้างเอกสารภายใน" icon="pi pi-file-edit" severity="secondary" outlined @click="openInternalCreate" />
                <Button v-if="pageConfig.showCreate" label="สร้างจาก SML" icon="pi pi-plus" @click="openCreate" />
            </div>
        </div>

        <DataTable :value="filteredDocuments" :loading="loading" dataKey="id" paginator :rows="10" responsiveLayout="scroll" stripedRows>
            <template #empty>
                <div class="py-8 text-center text-muted-color">
                    {{ searchQuery ? 'ไม่พบเอกสารที่ค้นหา' : pageConfig.empty }}
                </div>
            </template>

            <Column field="docNo" header="เลขที่เอกสาร" sortable style="min-width: 16rem">
                <template #body="{ data }">
                    <Button link class="p-0 font-bold text-left" @click="openDetail(data)">
                        <span class="whitespace-nowrap">{{ documentLine(data) }}</span>
                    </Button>
                    <Tag v-if="isInternalDocument(data)" value="เอกสารภายใน" severity="info" class="ml-2" />
                </template>
            </Column>
            <Column field="docDate" header="วันที่เอกสาร" sortable style="min-width: 10rem">
                <template #body="{ data }">{{ formatDocumentDate(data.docDate) }}</template>
            </Column>
            <Column field="totalAmount" header="ยอดเงิน" sortable style="min-width: 10rem">
                <template #body="{ data }">{{ formatMoney(data.totalAmount) }}</template>
            </Column>
            <Column field="status" header="สถานะ" sortable style="min-width: 18rem">
                <template #body="{ data }">
                    <div class="status-cell">
                        <Tag :value="documentStatusLabel(data)" :severity="signingStatusSeverity(data.status)" />
                        <small v-if="waitingSummary(data)" class="status-hint">{{ waitingSummary(data) }}</small>
                    </div>
                </template>
            </Column>
            <Column field="updatedAt" header="อัปเดตล่าสุด" sortable style="min-width: 14rem">
                <template #body="{ data }">{{ formatThaiDateTime(data.updatedAt) }}</template>
            </Column>
            <Column header="จัดการ" :exportable="false" style="min-width: 16rem">
                <template #body="{ data }">
                    <div class="flex gap-2">
                        <Button
                            v-if="data.status === 'draft'"
                            icon="pi pi-send"
                            rounded
                            outlined
                            severity="success"
                            aria-label="ส่งไปเซ็น"
                            :disabled="isInternalDocument(data) && !data.internalCurrentRevisionPrinted"
                            v-tooltip.top="isInternalDocument(data) && !data.internalCurrentRevisionPrinted ? 'กรุณาพิมพ์ PDF revision ล่าสุดก่อนส่ง' : 'ส่งไปเซ็น'"
                            :loading="isTransitioning(data.id)"
                            @click="confirmSend(data)"
                        />
                        <Button v-if="data.status === 'draft' && isInternalDocument(data)" icon="pi pi-pencil" rounded outlined severity="secondary" aria-label="แก้ไขแบบฟอร์ม" v-tooltip.top="'แก้ไขแบบฟอร์ม'" @click="openInternalEdit(data)" />
                        <Button
                            v-if="data.status === 'draft' && isInternalDocument(data)"
                            icon="pi pi-print"
                            rounded
                            outlined
                            :severity="data.internalCurrentRevisionPrinted ? 'secondary' : 'warn'"
                            aria-label="พิมพ์ PDF"
                            v-tooltip.top="data.internalCurrentRevisionPrinted ? 'พิมพ์ PDF revision ล่าสุดอีกครั้ง' : 'พิมพ์ PDF ก่อนส่ง'"
                            :loading="isTransitioning(data.id)"
                            @click="printInternalDraft(data)"
                        />
                        <Button
                            v-if="canGenerateExternalFromList(data)"
                            icon="pi pi-key"
                            rounded
                            outlined
                            severity="warn"
                            aria-label="สร้างลิงก์ผู้เซ็นภายนอก"
                            :loading="isGeneratingExternalForDocument(data)"
                            @click="openExternalSignerFromRow(data)"
                        />
                        <Button
                            v-if="queue === 'history'"
                            icon="pi pi-file-pdf"
                            rounded
                            outlined
                            severity="secondary"
                            aria-label="ดูเอกสารเซ็นครบ"
                            @click="previewDocumentPDF(data, 'current')"
                        />
                        <DocumentAttachmentActionButton :count="attachmentCount(data)" @click="openAttachmentsDialog(data)" />
                        <Button v-if="!isInternalDocument(data)" icon="pi pi-sitemap" rounded outlined severity="secondary" aria-label="ดู Flow เอกสาร" @click="openDocumentFlowFromRow(data)" />
                        <Button v-if="queue !== 'draft' && !isInternalDocument(data)" icon="pi pi-list" rounded outlined severity="secondary" aria-label="ตรวจสอบเอกสารอ้างอิง" @click="openReferenceCheck(data)" />
                        <Button icon="pi pi-eye" rounded outlined severity="secondary" aria-label="ดูเอกสาร" @click="openDetail(data)" />
                        <Button
                            v-if="data.status === 'draft'"
                            icon="pi pi-trash"
                            rounded
                            outlined
                            severity="danger"
                            aria-label="ยกเลิกเอกสาร"
                            :loading="isTransitioning(data.id)"
                            @click="confirmCancel(data)"
                        />
                    </div>
                </template>
            </Column>
        </DataTable>

        <DocumentFlowDialog :visible="flowDialog" :document="flowDocument" @update:visible="setFlowDialogVisible" @open-document="(documentId) => openDetail({ id: documentId })" />
        <BatchDocumentImportDialog v-model:visible="batchImportDialog" @completed="onBatchImportCompleted" />
        <DocumentAttachmentsDialog
            v-model:visible="attachmentsDialog"
            :title="attachmentsDialogTitle"
            :subtitle="attachmentsDialogSubtitle"
            :loader-key="attachmentsDialogKey"
            :loader="loadAttachmentsForDialog"
            :file-url-resolver="attachmentFileUrlForDialog"
            :headers="api.authHeaders()"
        />

        <Dialog
            v-model:visible="referenceDialog"
            modal
            :draggable="false"
            class="reference-check-dialog reference-audit-dialog"
            :style="{ width: 'min(1280px, 96vw)', height: 'min(820px, 90vh)' }"
            :breakpoints="{ '640px': '100vw' }"
            :header="referenceDialogTitle || 'ตรวจสอบเอกสารอ้างอิง'"
        >
            <template #header>
                <div class="reference-dialog-title">
                    <span class="reference-dialog-title-icon">
                        <i class="pi pi-list-check"></i>
                    </span>
                    <span class="reference-dialog-title-copy">
                        <strong>ตรวจสอบเอกสารอ้างอิง</strong>
                        <small>{{ referenceDialogTitle || 'เช็คลิสต์เอกสารก่อนเซ็น' }}</small>
                    </span>
                </div>
            </template>

            <div class="reference-dialog-layout">
                <DocumentReferenceCheck
                    :document="referenceDocument"
                    :loader="loadReferenceCheckForDialog"
                    compact
                    dialog-mode
                    display-mode="flow"
                    open-in-new-tab
                    :document-route-resolver="referenceDocumentUrl"
                    @open-document="(documentId) => openDetail({ id: documentId })"
                />
            </div>
        </Dialog>

        <ReadOnlyPdfDialog v-model:visible="readonlyPdfDialog" :url="readonlyPdfUrl" :title="readonlyPdfTitle" />

        <Dialog v-model:visible="externalSignerDialog" modal header="ผู้เซ็นภายนอก" :style="{ width: 'min(42rem, 94vw)' }">
            <div class="external-dialog">
                <Message severity="info" class="m-0">เลือกผู้เซ็นภายนอกที่ต้องการสร้าง Link/OTP สำหรับ {{ externalSignerDocument?.docNo || 'เอกสารนี้' }}</Message>
                <div class="external-list">
                    <div v-for="signer in pendingExternalSigners(externalSignerDocument)" :key="signer.id" class="external-row">
                        <span class="external-main">
                            <strong>{{ signerDisplayName(signer) }}</strong>
                            <small>{{ signerTypeLabel(signer) }}{{ signer.externalTokenId ? ' · มีลิงก์เดิมแล้ว' : ' · ยังไม่มีลิงก์' }}</small>
                        </span>
                        <Button
                            label="สร้างลิงก์/OTP"
                            icon="pi pi-key"
                            severity="secondary"
                            outlined
                            :loading="isGeneratingExternal(signer.id)"
                            @click="requestExternalToken(signer, externalSignerDocument)"
                        />
                    </div>
                </div>
            </div>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="externalSignerDialog = false" />
            </template>
        </Dialog>

        <Dialog v-model:visible="tokenDialog" modal header="ลิงก์ภายนอก / OTP" :style="{ width: 'min(42rem, 92vw)' }">
            <div v-if="generatedToken" class="token-box">
                <Message severity="success" class="m-0">
                    สร้างลิงก์สำหรับ {{ generatedToken.signerLabel }}{{ generatedToken.docNo ? ` · ${generatedToken.docNo}` : '' }} แล้ว
                </Message>
                <Message v-if="copyFallbackVisible" severity="warn" class="m-0">
                    คัดลอกอัตโนมัติไม่ได้ กรุณาเลือกข้อความด้านล่างแล้วคัดลอกเอง
                </Message>
                <label>Link</label>
                <div class="copy-row">
                    <InputText :modelValue="generatedToken.url" readonly class="w-full" @focus="selectInput" @click="selectInput" />
                    <Button icon="pi pi-copy" rounded outlined aria-label="copy link" @click="copyText(generatedToken.url)" />
                </div>
                <label>OTP</label>
                <div class="copy-row">
                    <InputText :modelValue="generatedToken.otp" readonly class="w-full otp-text" @focus="selectInput" @click="selectInput" />
                    <Button icon="pi pi-copy" rounded outlined aria-label="copy otp" @click="copyText(generatedToken.otp)" />
                </div>
                <Textarea v-if="copyFallbackVisible" :modelValue="copyFallbackValue" readonly rows="3" autoResize @focus="selectInput" @click="selectInput" />
                <small class="text-muted-color">OTP หมดอายุ {{ formatThaiDateTime(generatedToken.expiresAt) }}</small>
            </div>
        </Dialog>
    </div>
</template>

<style scoped>
.status-cell,
.token-box,
.external-dialog,
.external-main {
    min-width: 0;
    display: grid;
}

.status-cell {
    gap: 0.3rem;
    align-items: start;
}

.status-hint,
.external-main small {
    color: var(--text-color-secondary);
}

.status-hint {
    max-width: 18rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.external-dialog,
.external-list,
.token-box {
    gap: 0.75rem;
}

.external-list {
    display: grid;
}

.external-row {
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.75rem;
}

.external-main {
    gap: 0.15rem;
}

.external-main strong,
.external-main small {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.copy-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

:global(.reference-check-dialog .p-dialog-content) {
    height: calc(100% - 4.25rem);
    display: flex;
    flex-direction: column;
    padding-top: 0.75rem;
    background: transparent;
    overflow: hidden;
}

.reference-dialog-layout {
    min-height: 0;
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: var(--surface-card);
}

.reference-dialog-layout :deep(.reference-check) {
    flex: 1 1 auto;
    min-height: 0;
}

.otp-text {
    font-size: 1.3rem;
    font-weight: 700;
    letter-spacing: 0;
}

@media (max-width: 640px) {
    .external-row {
        align-items: stretch;
        flex-direction: column;
    }

    .external-row :deep(.p-button) {
        width: 100%;
    }
}
</style>
