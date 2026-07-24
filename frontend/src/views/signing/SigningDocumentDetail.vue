<script setup>
import { useLayout } from '@/layout/composables/layout';
import { api } from '@/services/api';
import { authStore } from '@/stores/auth';
import { formatThaiDateTime, signingStatusLabel, signingStatusSeverity, smlImageFailureDetail } from '@/utils/signingFormatters';
import { isSigningDocumentMenuKey, normalizeSigningDocumentQueue, signingDocumentMenuKeyForQueue, signingDocumentQueueForStatus } from '@/utils/signingQueue';
import ContinuousPdfViewer from '@/views/signing/components/ContinuousPdfViewer.vue';
import DocumentAttachmentsPanel from '@/views/signing/components/DocumentAttachmentsPanel.vue';
import DocumentFlowDialog from '@/views/signing/components/DocumentFlowDialog.vue';
import DocumentLayoutDesigner from '@/views/signing/components/DocumentLayoutDesigner.vue';
import DocumentReferenceCheck from '@/views/signing/components/DocumentReferenceCheck.vue';
import DocumentWorkflowTimeline from '@/views/signing/components/DocumentWorkflowTimeline.vue';
import ReadOnlyPdfDialog from '@/views/signing/components/ReadOnlyPdfDialog.vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const confirm = useConfirm();
const toast = useToast();
const { layoutState } = useLayout();

const document = ref(null);
const loading = ref(false);
const retryingLock = ref(false);
const retryingFinalPDF = ref(false);
const retryingImages = ref(false);
const printing = ref(false);
const printingInternal = ref(false);
const sending = ref(false);
const confirmingDocument = ref(false);
const cancellingDocument = ref(false);
const tokenDialog = ref(false);
const generatedToken = ref(null);
const activeTab = ref('progress');
const documentAttachments = ref([]);
const documentAttachmentsLoading = ref(false);
const documentAttachmentsError = ref('');
const flowDialog = ref(false);
const flowDocument = ref(null);
const evidenceDialog = ref(false);
const evidencePdfUrl = ref('');
const evidencePdfTitle = ref('');
const copyFallbackVisible = ref(false);
const copyFallbackValue = ref('');
const layoutDialog = ref(false);
const savingLayout = ref(false);
const layoutValidationIssues = ref([]);
const layoutDraft = ref({ boxes: [], legalNoticeBox: null, legalNoticeBoxes: [] });
const cancelDialog = ref(false);
const cancelReason = ref('');

const importantEvents = computed(() =>
    (document.value?.events || [])
        .map((event) => ({ ...event, view: movementEventView(event) }))
        .filter((event) => event.view)
);
const printEvents = computed(() => document.value?.printEvents || []);
const documentAttachmentCount = computed(() => Math.max(Number(document.value?.attachmentCount || 0), documentAttachments.value.length));
const pdfPreviewUrl = computed(() => (document.value?.id ? api.signingDocumentPDFUrlForDocument(document.value) : ''));
const externalSigners = computed(() => (document.value?.signers || []).filter((signer) => signer.signerType === 'external'));
const documentHeaderLine = computed(() => {
    const doc = document.value;
    if (!doc) return 'เอกสาร';
    return `${doc.docNo || 'เอกสาร'} ~ ${doc.docFormatCode || '-'} · ${doc.partyName || doc.partyCode || '-'}`;
});
const canViewEvidencePDF = computed(() => document.value?.status === 'completed' && Boolean(document.value?.finalFileId || document.value?.finalFile));
const isInternalDocument = computed(() => document.value?.documentSource === 'internal');
const internalLayoutReady = computed(() => !isInternalDocument.value || document.value?.layoutReady === true);
const internalLayoutPDFUrl = computed(() => (document.value?.id ? api.signingDocumentPDFUrlForDocument(document.value, 'original') : ''));
const canCancelInternalDocument = computed(() => {
    if (!isInternalDocument.value || !['draft', 'in_progress'].includes(document.value?.status)) return false;
    return authStore.user?.role === 'superadmin' || document.value?.createdBy === authStore.user?.id;
});
const documentStatusLabel = computed(() => {
    if (isInternalDocument.value && document.value?.status === 'pending_confirm') return 'รอสร้างเอกสารสมบูรณ์';
    if (isInternalDocument.value && document.value?.status === 'auto_confirming') return 'กำลังสร้าง PDF';
    return signingStatusLabel(document.value?.status);
});
const backRouteName = computed(() => {
    if (document.value?.status === 'draft') return 'signing-document-drafts';
    if (document.value?.status === 'completed') return 'signing-document-history';
    return 'signing-documents';
});

function currentDetailQueue() {
    if (document.value?.status) return signingDocumentQueueForStatus(document.value.status);
    return normalizeSigningDocumentQueue(route.query.from_queue) || 'active';
}

function syncActiveMenuFromRoute() {
    layoutState.activeMenuKey = signingDocumentMenuKeyForQueue(normalizeSigningDocumentQueue(route.query.from_queue) || 'active');
}

function syncActiveMenuFromDocument() {
    layoutState.activeMenuKey = signingDocumentMenuKeyForQueue(currentDetailQueue());
}

function loadReferenceCheck() {
    if (isInternalDocument.value) return Promise.resolve({ referenceCheck: { items: [], summary: { total: 0, missing: 0, inProgress: 0, completed: 0 } } });
    if (!document.value?.id) return Promise.resolve({ referenceCheck: { items: [], summary: { total: 0, missing: 0, inProgress: 0, completed: 0 } } });
    return api.getSigningDocumentReferenceCheck(document.value.id);
}

function clearActiveSigningMenu() {
    if (isSigningDocumentMenuKey(layoutState.activeMenuKey)) layoutState.activeMenuKey = null;
}

onMounted(async () => {
    syncActiveMenuFromRoute();
    await loadPage();
});
onBeforeUnmount(() => {
    clearActiveSigningMenu();
});

watch(
    () => route.params.id,
    async (id, previousId) => {
        if (!previousId || id === previousId) return;
        activeTab.value = 'progress';
        flowDialog.value = false;
        flowDocument.value = null;
        syncActiveMenuFromRoute();
        await loadPage();
    }
);

async function loadPage() {
    loading.value = true;
    try {
        const result = await api.getSigningDocument(route.params.id);
        document.value = result.document;
        syncActiveMenuFromDocument();
        loadDocumentAttachments();
        if (route.query.open_layout === '1' && document.value?.documentSource === 'internal' && document.value?.status === 'draft') {
            openInternalLayout();
            const { open_layout, ...query } = route.query;
            router.replace({ query });
        }
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadDocumentAttachments() {
    if (!document.value?.id) return;
    documentAttachmentsLoading.value = true;
    documentAttachmentsError.value = '';
    try {
        const result = await api.getSigningDocumentAttachments(document.value.id);
        documentAttachments.value = Array.isArray(result.attachments) ? result.attachments : [];
    } catch (err) {
        documentAttachmentsError.value = err.message || 'โหลดไฟล์แนบไม่สำเร็จ';
    } finally {
        documentAttachmentsLoading.value = false;
    }
}

function documentAttachmentFileUrl(attachment) {
    return api.signingDocumentAttachmentFileUrl(document.value?.id || route.params.id, attachment?.id || '');
}

async function retryLock() {
    retryingLock.value = true;
    try {
        await api.retrySigningDocumentLock(document.value.id, { idempotencyKey: makeTransitionKey('retry-lock') });
        toast.add({ severity: 'success', summary: 'Lock SML สำเร็จ', life: 2500 });
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'Lock SML ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        retryingLock.value = false;
    }
}

async function retryImages() {
    retryingImages.value = true;
    try {
        const result = await api.retrySigningDocumentImages(document.value.id, { idempotencyKey: makeTransitionKey('retry-images') });
        toast.add({
            severity: result.lockOk ? 'success' : 'warn',
            summary: result.lockOk ? 'ส่งรูป SML และ Lock SML สำเร็จ' : 'ส่งรูป SML สำเร็จ แต่ Lock SML ยังไม่สำเร็จ',
            detail: result.lockOk ? imageTruncatedDetail(result) : 'กรุณา retry Lock SML อีกครั้ง',
            life: 4000
        });
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ส่งรูป SML ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        retryingImages.value = false;
    }
}

async function retryFinalPDF() {
    retryingFinalPDF.value = true;
    try {
        const result = await api.retrySigningDocumentFinalPDF(document.value.id, { idempotencyKey: makeTransitionKey('retry-final-pdf') });
        toast.add({
            severity: confirmResultSeverity(result),
            summary: confirmResultSummary(result),
            detail: confirmResultDetail(result),
            life: 4000
        });
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'สร้าง PDF หลักฐานไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        retryingFinalPDF.value = false;
    }
}

function confirmResultSeverity(result = {}) {
    return result.finalOk && result.imageOk && result.lockOk ? 'success' : 'warn';
}

function confirmResultSummary(result = {}) {
    return result.finalOk && result.imageOk && result.lockOk ? 'ยืนยันเอกสารสำเร็จ' : 'ยืนยันแล้วแต่ยังมีงานต้องตรวจสอบ';
}

function confirmResultDetail(result = {}) {
    if (!result.finalOk) return 'สร้าง final PDF/evidence ไม่สำเร็จ กรุณา retry';
    if (!result.imageOk) return smlImageFailureDetail(result);
    if (!result.lockOk) return ['Lock SML ไม่สำเร็จ กรุณา retry', imageTruncatedDetail(result)].filter(Boolean).join(' · ');
    return imageTruncatedDetail(result);
}

function imageTruncatedDetail(result = {}) {
    const image = result.image || {};
    if (!image.truncated) return '';
    return `ส่งรูปเข้า SML เฉพาะ ${image.imageCount || 8} จาก ${image.totalPages || '-'} หน้าแรก`;
}

function confirmSendDocument() {
    if (isInternalDocument.value && !internalLayoutReady.value) {
        toast.add({ severity: 'warn', summary: 'กรุณาจัดวางกรอบก่อนส่ง', detail: 'วางกรอบลายเซ็นและข้อความกฎหมายบน PDF ฉบับจริงให้ครบก่อนส่งเข้า Workflow', life: 3800 });
        return;
    }
    confirm.require({
        header: 'ส่งเอกสารไปเซ็น',
        message: `ต้องการส่ง ${document.value?.docNo || 'เอกสารนี้'} ให้ผู้เซ็นใช่ไหม?`,
        icon: 'pi pi-send',
        acceptLabel: 'ส่งไปเซ็น',
        rejectLabel: 'ยกเลิก',
        accept: () => sendDocument()
    });
}

function openInternalEdit() {
    if (!document.value?.internalDocumentId) return;
    router.push({ name: 'internal-document-edit', params: { id: document.value.internalDocumentId } });
}

function copyInternalDocument() {
    if (!document.value?.internalDocumentId) return;
    router.push({ name: 'internal-document-new', query: { copy_from: document.value.internalDocumentId } });
}

function withLayoutClientKey(box, index) {
    return { ...box, clientKey: box.clientKey || `internal-layout-${index}-${crypto.randomUUID?.() || Date.now()}` };
}

function toInternalLayoutPayload(box) {
    const { clientKey: _clientKey, ...payload } = box || {};
    return payload;
}

function openInternalLayout() {
    if (!document.value?.id || !document.value?.originalFile) return;
    const boxes = (document.value.signaturePlacements || []).map((box, index) => withLayoutClientKey(box, index));
    const legalNoticeBoxes = (document.value.legalNoticeBoxes || []).map((box, index) => withLayoutClientKey(box, `legal-${index}`));
    layoutDraft.value = {
        boxes,
        legalNoticeBox: legalNoticeBoxes[0] || null,
        legalNoticeBoxes
    };
    layoutValidationIssues.value = [];
    layoutDialog.value = true;
}

async function saveInternalLayout() {
    if (!document.value?.id || savingLayout.value) return;
    if (layoutValidationIssues.value.length) {
        toast.add({ severity: 'warn', summary: 'กรอบยังไม่พร้อม', detail: layoutValidationIssues.value[0], life: 4000 });
        return;
    }
    savingLayout.value = true;
    try {
        const result = await api.saveInternalDraftLayout(document.value.id, {
            expectedVersion: document.value.currentVersion,
            layoutBoxes: layoutDraft.value.boxes.map(toInternalLayoutPayload),
            legalNoticeBoxes: layoutDraft.value.legalNoticeBoxes.map(toInternalLayoutPayload)
        });
        document.value = result.document;
        layoutDialog.value = false;
        toast.add({ severity: 'success', summary: 'บันทึกกรอบลายเซ็นแล้ว', detail: 'Draft พร้อมสำหรับพิมพ์ PDF และส่งเข้า Workflow', life: 3200 });
    } catch (error) {
        const issues = Array.isArray(error.payload?.issues) ? error.payload.issues.join(' · ') : error.message;
        toast.add({ severity: 'error', summary: 'บันทึกกรอบไม่สำเร็จ', detail: issues, life: 5200 });
    } finally {
        savingLayout.value = false;
    }
}

async function printInternalDraft() {
    if (!document.value?.internalDocumentId) return;
    const popup = window.open('', '_blank');
    printingInternal.value = true;
    try {
        const result = await api.printInternalDocument(document.value.internalDocumentId);
        const response = await fetch(result.pdfUrl || api.internalDocumentPDFUrl(document.value.internalDocumentId, document.value.internalRevision), { headers: api.authHeaders(), cache: 'no-store' });
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
        printingInternal.value = false;
    }
}

async function sendDocument() {
    if (!document.value?.id) return;
    sending.value = true;
    try {
        await api.sendSigningDocument(document.value.id, { idempotencyKey: makeTransitionKey('send') });
        toast.add({ severity: 'success', summary: 'ส่งเอกสารไปเซ็นแล้ว', life: 2500 });
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ส่งเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        sending.value = false;
    }
}

function confirmAdminConfirmDocument() {
    const message = isInternalDocument.value
        ? `ต้องการยืนยัน ${document.value?.docNo || 'เอกสารนี้'} ใช่ไหม? ระบบจะสร้าง PDF ฉบับสมบูรณ์และหลักฐานการลงนามใน PaperLess`
        : `ต้องการยืนยัน ${document.value?.docNo || 'เอกสารนี้'} ใช่ไหม? ระบบจะสร้าง final PDF/evidence ส่งรูปเข้า SML และ Lock SML`;
    confirm.require({
        header: 'ยืนยันเอกสาร',
        message,
        icon: 'pi pi-check-circle',
        acceptLabel: 'ยืนยันเอกสาร',
        rejectLabel: 'ยกเลิก',
        accept: () => adminConfirmDocument()
    });
}

async function adminConfirmDocument() {
    if (!document.value?.id) return;
    confirmingDocument.value = true;
    try {
        const result = await api.confirmSigningDocument(document.value.id, { idempotencyKey: makeTransitionKey('confirm') });
        toast.add({
            severity: confirmResultSeverity(result),
            summary: confirmResultSummary(result),
            detail: confirmResultDetail(result),
            life: 4000
        });
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ยืนยันเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        confirmingDocument.value = false;
    }
}

function confirmCancelDocument() {
    if (isInternalDocument.value) {
        cancelReason.value = '';
        cancelDialog.value = true;
        return;
    }
    confirm.require({
        header: 'ยกเลิกเอกสาร',
        message: `ต้องการยกเลิก ${document.value?.docNo || 'เอกสารนี้'} ใช่ไหม?`,
        icon: 'pi pi-exclamation-triangle',
        acceptLabel: 'ยกเลิกเอกสาร',
        rejectLabel: 'กลับ',
        acceptClass: 'p-button-danger',
        accept: () => cancelDocument()
    });
}

async function cancelDocument() {
    if (!document.value?.id) return;
    cancellingDocument.value = true;
    try {
        await api.cancelSigningDocument(document.value.id, { idempotencyKey: makeTransitionKey('cancel'), reason: cancelReason.value });
        toast.add({ severity: 'success', summary: 'ยกเลิกเอกสารแล้ว', life: 2500 });
        cancelDialog.value = false;
        if (isInternalDocument.value) await loadPage();
        else router.push({ name: 'signing-document-drafts' });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ยกเลิกเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        cancellingDocument.value = false;
    }
}

function makeTransitionKey(action) {
    return `${action}-${document.value?.id || 'document'}-${crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`}`;
}

async function printOfficialCopy() {
    if (!document.value?.id) return;
    const popup = window.open('', '_blank');
    printing.value = true;
    try {
        const deviceId = getAdminDeviceId();
        const result = await api.createSigningDocumentPrintCopy(document.value.id, {
            idempotencyKey: makeTransitionKey('print-copy'),
            channel: 'web',
            deviceId,
            clientTimezone: Intl.DateTimeFormat().resolvedOptions().timeZone || ''
        });
        const fileUrl = result.fileUrl || api.signingDocumentPrintCopyPDFUrl(document.value.id, result.printCopyId);
        const response = await fetch(fileUrl, { headers: api.authHeaders(), cache: 'no-store' });
        if (!response.ok) throw new Error('โหลดไฟล์พิมพ์ไม่สำเร็จ');
        const blob = await response.blob();
        const objectUrl = URL.createObjectURL(blob);
        if (popup) {
            popup.location.href = objectUrl;
        } else {
            window.open(objectUrl, '_blank');
        }
        setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000);
        toast.add({ severity: 'success', summary: 'สร้างไฟล์พิมพ์แล้ว', life: 2500 });
        await loadPage();
    } catch (err) {
        if (popup) popup.close();
        toast.add({ severity: 'error', summary: 'พิมพ์เอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        printing.value = false;
    }
}

function previewEvidencePDF() {
    if (!canViewEvidencePDF.value) {
        toast.add({ severity: 'info', summary: 'ยังไม่มีหลักฐานการลงนาม', detail: 'เอกสารนี้ยังไม่มี final PDF สำหรับ audit', life: 3000 });
        return;
    }
    evidencePdfUrl.value = api.signingDocumentPDFUrlForDocument(document.value, 'final');
    evidencePdfTitle.value = `${document.value?.docNo || 'เอกสาร'} · หลักฐานการลงนาม`;
    evidenceDialog.value = true;
}

async function generateExternal(signer) {
    if (!canGenerateExternalToken(signer)) {
        toast.add({ severity: 'info', summary: 'ยังสร้างลิงก์ไม่ได้', detail: signer?.status === 'waiting' ? 'ยังไม่ถึงคิวผู้เซ็นภายนอกคนนี้' : 'ผู้เซ็นภายนอกคนนี้ไม่พร้อมใช้งานแล้ว', life: 3000 });
        return;
    }
    try {
        copyFallbackVisible.value = false;
        copyFallbackValue.value = '';
        const result = await api.regenerateExternalToken(signer.id);
        generatedToken.value = result.external;
        tokenDialog.value = true;
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'สร้าง public link ไม่สำเร็จ', detail: err.message, life: 4000 });
    }
}

function requestExternalToken(signer) {
    if (!signer?.id) return;
    if (!canGenerateExternalToken(signer)) {
        toast.add({ severity: 'info', summary: 'ยังสร้างลิงก์ไม่ได้', detail: signer?.status === 'waiting' ? 'ยังไม่ถึงคิวผู้เซ็นภายนอกคนนี้' : 'ผู้เซ็นภายนอกคนนี้ไม่พร้อมใช้งานแล้ว', life: 3000 });
        return;
    }
    if (!signer.externalTokenId) {
        void generateExternal(signer);
        return;
    }
    confirm.require({
        header: 'สร้างลิงก์ใหม่?',
        message: 'ลิงก์และ OTP เดิมของผู้เซ็นภายนอกคนนี้จะใช้ไม่ได้ ต้องส่งลิงก์ใหม่ให้ลูกค้าอีกครั้ง',
        icon: 'pi pi-exclamation-triangle',
        rejectLabel: 'ยกเลิก',
        acceptLabel: 'สร้างใหม่',
        accept: () => generateExternal(signer)
    });
}

function getAdminDeviceId() {
    const key = 'paperless_admin_device_id';
    let value = localStorage.getItem(key);
    if (!value) {
        value = window.crypto?.randomUUID ? window.crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
        localStorage.setItem(key, value);
    }
    return value;
}

async function copy(value) {
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

function signerLabel(signer) {
    return signer?.signerName || signer?.signerUser || 'บุคคลภายนอก';
}

function externalTokenHint(signer) {
    if (signer?.status === 'waiting') return 'ยังไม่ถึงคิว';
    if (signer?.status === 'signed') return 'เซ็นแล้ว ไม่ต้องสร้างลิงก์ใหม่';
    if (signer?.status && signer.status !== 'pending') return 'ผู้เซ็นนี้ไม่พร้อมใช้งานแล้ว';
    if (signer?.externalTokenId) return 'มีลิงก์เดิมอยู่แล้ว หากสร้างใหม่ลิงก์เดิมจะถูกยกเลิก';
    return 'ยังไม่มีลิงก์สำหรับส่งให้ลูกค้า';
}

function canGenerateExternalToken(signer) {
    return document.value?.status === 'in_progress' && signer?.status === 'pending';
}

function printerLabel(value) {
    if (!value) return '-';
    if (value === 'not_available_web_browser') return 'ไม่สามารถอ่านชื่อเครื่องพิมพ์จาก Web';
    return value;
}

function formatTimelineDate(value) {
    return formatThaiDateTime(value);
}

function openDocumentFlow(doc = document.value) {
    if (!doc?.docNo) return;
    flowDocument.value = {
        docNo: doc.docNo,
        docFormatCode: doc.docFormatCode,
        partyName: doc.partyName,
        partyCode: doc.partyCode
    };
    flowDialog.value = true;
}

function setFlowDialogVisible(value) {
    flowDialog.value = value;
}

function openFlowDocument(documentId) {
    if (!documentId || documentId === document.value?.id) return;
    router.push({
        name: 'signing-document-detail',
        params: { id: documentId },
        query: { from_queue: currentDetailQueue() }
    });
}

function referenceDocumentUrl(documentId) {
    return router.resolve({
        name: 'signing-document-detail',
        params: { id: documentId },
        query: { from_queue: currentDetailQueue() }
    }).href;
}

function movementEventView(event) {
    const action = String(event?.action || '');
    const metadata = event?.metadata || {};
    const labels = {
        document_draft_created: {
            title: 'สร้างเอกสารเตรียมส่ง',
            icon: 'pi pi-file-plus',
            severity: 'info',
            detail: event.message || 'สร้างเอกสารไว้ก่อนส่งให้ผู้เซ็น'
        },
        document_created: {
            title: 'สร้างเอกสารเซ็น',
            icon: 'pi pi-send',
            severity: 'info',
            detail: event.message || 'เริ่ม workflow เอกสารนี้'
        },
        document_sent: {
            title: 'ส่งเอกสารไปเซ็น',
            icon: 'pi pi-send',
            severity: 'info',
            detail: event.message || 'เปิดคิวให้ผู้เซ็นดำเนินการ'
        },
        signed: {
            title: `${event.actorLabel || 'ผู้เซ็น'} เซ็นแล้ว`,
            icon: 'pi pi-check',
            severity: 'success',
            detail: event.message || 'เซ็นเอกสารแล้ว'
        },
        rejected: {
            title: `${event.actorLabel || 'ผู้เซ็น'} ปฏิเสธเอกสาร`,
            icon: 'pi pi-times',
            severity: 'danger',
            detail: metadata.reason ? `เหตุผล: ${metadata.reason}` : event.message || 'เอกสารถูกปฏิเสธ'
        },
        document_ready_to_confirm: {
            title: 'เซ็นครบ รอยืนยัน',
            icon: 'pi pi-verified',
            severity: 'success',
            detail: event.message || 'เอกสารพร้อมให้ผู้ดูแลยืนยัน'
        },
        document_confirm_attempt: {
            title: 'เริ่มยืนยันเอกสาร',
            icon: 'pi pi-check-circle',
            severity: 'info',
            detail: event.message || 'กำลังสร้างหลักฐานและส่งสถานะกลับ SML'
        },
        document_confirmed: {
            title: 'ยืนยันเอกสารแล้ว',
            icon: 'pi pi-check-circle',
            severity: 'success',
            detail: event.message || 'เอกสารเสร็จสมบูรณ์'
        },
        document_completed: {
            title: 'เซ็นครบทุกขั้นตอน',
            icon: 'pi pi-verified',
            severity: 'success',
            detail: event.message || 'เอกสารพร้อมสร้าง PDF หลักฐาน'
        },
        final_pdf_ready: {
            title: 'PDF หลักฐานพร้อมแล้ว',
            icon: 'pi pi-file-check',
            severity: 'success',
            detail: 'สร้าง PDF พร้อมลายเซ็นและ Evidence Appendix แล้ว'
        },
        final_pdf_failed: {
            title: 'PDF หลักฐานไม่สำเร็จ',
            icon: 'pi pi-file-excel',
            severity: 'danger',
            detail: 'ต้องสร้าง PDF อีกครั้งก่อน lock SML หรือพิมพ์เอกสาร'
        },
        sml_images_success: {
            title: 'ส่งรูป SML สำเร็จ',
            icon: 'pi pi-images',
            severity: 'success',
            detail: metadata.truncated ? `ส่ง ${metadata.imageCount || 8} จาก ${metadata.totalPages || '-'} หน้าเข้า SML` : event.message || 'บันทึกรูปเอกสารเข้า SML แล้ว'
        },
        sml_images_failed: {
            title: 'ส่งรูป SML ไม่สำเร็จ',
            icon: 'pi pi-images',
            severity: 'danger',
            detail: 'ต้องส่งรูป SML อีกครั้งก่อน Lock SML หรือพิมพ์เอกสาร'
        },
        sml_lock_success: {
            title: 'Lock SML สำเร็จ',
            icon: 'pi pi-lock',
            severity: 'success',
            detail: 'อัปเดตเอกสารกลับไปที่ SML แล้ว'
        },
        sml_lock_failed: {
            title: 'Lock SML ไม่สำเร็จ',
            icon: 'pi pi-exclamation-triangle',
            severity: 'danger',
            detail: 'เอกสารเซ็นครบแล้ว แต่ยังต้อง retry lock SML'
        },
        pdf_stamp_failed: {
            title: 'สร้าง PDF ลายเซ็นไม่สำเร็จ',
            icon: 'pi pi-file-excel',
            severity: 'danger',
            detail: 'ต้องตรวจสอบก่อนให้ผู้ใช้เปิดเอกสารต่อ'
        },
        document_printed: {
            title: 'พิมพ์เอกสารแล้ว',
            icon: 'pi pi-print',
            severity: 'info',
            detail: `สร้าง official print copy${metadata.printerName ? ` (${metadata.printerName})` : ''}`
        }
    };
    return labels[action] || null;
}

</script>

<template>
    <div class="signing-detail">
        <div class="editor-bar">
            <Button icon="pi pi-arrow-left" text rounded aria-label="กลับ" @click="router.push({ name: backRouteName })" />
            <div class="bar-title">
                <strong>{{ documentHeaderLine }}</strong>
            </div>
            <Tag v-if="document" :value="documentStatusLabel" :severity="signingStatusSeverity(document.status)" />
            <Tag v-if="isInternalDocument" value="เอกสารภายใน" severity="info" />
            <Tag v-if="isInternalDocument && document?.status === 'draft'" :value="internalLayoutReady ? 'จัดวางกรอบแล้ว' : 'ต้องจัดวางกรอบ'" :severity="internalLayoutReady ? 'success' : 'warn'" />
            <Button v-if="document && !isInternalDocument" label="ตรวจสอบ Flow" icon="pi pi-sitemap" severity="secondary" outlined @click="openDocumentFlow()" />
            <Button v-if="document?.status === 'draft' && isInternalDocument" label="แก้ไขแบบฟอร์ม" icon="pi pi-pencil" severity="secondary" outlined @click="openInternalEdit" />
            <Button v-if="document?.status === 'draft' && isInternalDocument" label="จัดวางกรอบ" icon="pi pi-objects-column" severity="secondary" outlined @click="openInternalLayout" />
            <Button v-if="document?.status === 'draft' && isInternalDocument" label="พิมพ์ PDF" icon="pi pi-print" severity="secondary" outlined :disabled="!internalLayoutReady" v-tooltip.bottom="internalLayoutReady ? 'พิมพ์ PDF revision ล่าสุด (ไม่บังคับก่อนส่ง)' : 'กรุณาจัดวางกรอบก่อนพิมพ์'" :loading="printingInternal" @click="printInternalDraft" />
            <Button v-if="document?.status === 'cancelled' && isInternalDocument" label="สร้างฉบับใหม่" icon="pi pi-copy" severity="secondary" outlined @click="copyInternalDocument" />
            <Button v-if="document?.status === 'draft'" label="ส่งไปเซ็น" icon="pi pi-send" severity="success" :loading="sending" :disabled="isInternalDocument && !internalLayoutReady" v-tooltip.bottom="isInternalDocument && !internalLayoutReady ? 'กรุณาจัดวางกรอบก่อนส่ง' : 'ส่งไปเซ็น'" @click="confirmSendDocument" />
            <Button v-if="(document?.status === 'draft' && !isInternalDocument) || canCancelInternalDocument" label="ยกเลิก" icon="pi pi-trash" severity="danger" outlined :loading="cancellingDocument" @click="confirmCancelDocument" />
            <Button v-if="document?.status === 'completed_evidence_failed'" label="สร้าง PDF อีกครั้ง" icon="pi pi-file-check" severity="warn" outlined :loading="retryingFinalPDF" @click="retryFinalPDF" />
            <Button v-if="document?.status === 'completed_image_failed' && !isInternalDocument" label="ส่งรูป SML อีกครั้ง" icon="pi pi-images" severity="danger" outlined :loading="retryingImages" @click="retryImages" />
            <Button v-if="document?.status === 'completed_lock_failed' && !isInternalDocument" label="Lock SML อีกครั้ง" icon="pi pi-refresh" severity="danger" outlined :loading="retryingLock" @click="retryLock" />
            <Button v-if="canViewEvidencePDF" label="ดูหลักฐานการลงนาม" icon="pi pi-shield" severity="secondary" outlined @click="previewEvidencePDF" />
            <Button v-if="document?.status === 'completed'" label="พิมพ์เอกสาร" icon="pi pi-print" severity="primary" :loading="printing" @click="printOfficialCopy" />
            <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadPage" />
        </div>

        <div class="detail-grid">
            <section class="pdf-panel">
                <ContinuousPdfViewer :url="pdfPreviewUrl" :headers="api.authHeaders()" toolbar-label="เอกสาร" />
            </section>

            <aside class="side-panel">
                <Tabs v-model:value="activeTab">
                    <TabList>
                        <Tab value="progress">ความคืบหน้า</Tab>
                        <Tab v-if="!isInternalDocument" value="references">ตรวจสอบเอกสาร</Tab>
                        <Tab value="attachments">ไฟล์แนบอ้างอิง ({{ documentAttachmentCount }})</Tab>
                        <Tab value="print">พิมพ์</Tab>
                        <Tab value="events">เหตุการณ์</Tab>
                    </TabList>
                    <TabPanels>
                        <TabPanel value="progress">
                            <div v-if="externalSigners.length" class="info-block external-signer-block">
                                <div class="block-head">
                                    <div>
                                        <div class="block-title">ผู้เซ็นภายนอก</div>
                                        <small>สร้างลิงก์/OTP สำหรับส่งให้ลูกค้าโดยตรง</small>
                                    </div>
                                    <Tag :value="`${externalSigners.length} คน`" severity="info" />
                                </div>
                                <div class="external-signer-list">
                                    <div v-for="signer in externalSigners" :key="signer.id" class="external-signer-row">
                                        <span class="external-signer-main">
                                            <strong>{{ signerLabel(signer) }}</strong>
                                            <small>{{ signer.positionName || signer.positionCode || 'ผู้เซ็นภายนอก' }}</small>
                                            <small>{{ externalTokenHint(signer) }}</small>
                                        </span>
                                        <span class="external-signer-actions">
                                            <Tag :value="signingStatusLabel(signer.status)" :severity="signingStatusSeverity(signer.status)" />
                                            <Button
                                                label="สร้างลิงก์/OTP"
                                                icon="pi pi-key"
                                                severity="secondary"
                                                outlined
                                                :disabled="!canGenerateExternalToken(signer)"
                                                @click="requestExternalToken(signer)"
                                            />
                                        </span>
                                    </div>
                                </div>
                            </div>
                            <div class="info-block">
                                <div class="block-head">
                                    <div>
                                        <div class="block-title">ความคืบหน้าเอกสาร</div>
                                    </div>
                                    <Tag v-if="document" :value="signingStatusLabel(document.status)" :severity="signingStatusSeverity(document.status)" />
                                </div>
                                <DocumentWorkflowTimeline :document="document" :show-external-actions="false" compact @generate-external="requestExternalToken" />
                            </div>
                        </TabPanel>
                        <TabPanel v-if="!isInternalDocument" value="references">
                            <div class="info-block">
                                <DocumentReferenceCheck :document="document" :loader="loadReferenceCheck" compact open-in-new-tab :document-route-resolver="referenceDocumentUrl" @open-document="openFlowDocument" />
                            </div>
                        </TabPanel>
                        <TabPanel value="attachments">
                            <DocumentAttachmentsPanel
                                readonly
                                :attachments="documentAttachments"
                                :loading="documentAttachmentsLoading"
                                :error="documentAttachmentsError"
                                :headers="api.authHeaders()"
                                :on-reload="loadDocumentAttachments"
                                :file-url-resolver="documentAttachmentFileUrl"
                            />
                        </TabPanel>
                        <TabPanel value="print">
                            <div class="info-block">
                                <div class="block-head">
                                    <div>
                                        <div class="block-title">ประวัติพิมพ์</div>
                                        <small>สำเนาสำหรับพิมพ์อย่างเป็นทางการ</small>
                                    </div>
                                    <Tag :value="`${printEvents.length} ครั้ง`" severity="secondary" />
                                </div>
                                <div v-if="printEvents.length === 0" class="empty-log">ยังไม่มีการพิมพ์สำเนาอย่างเป็นทางการ</div>
                                <div v-else class="print-list">
                                    <div v-for="item in printEvents" :key="item.id" class="print-row">
                                        <span>
                                            <strong>{{ formatThaiDateTime(item.printedAt) }}</strong>
                                            <small>{{ item.channel === 'web' ? 'Web' : 'App' }} · {{ printerLabel(item.printerName) }}</small>
                                        </span>
                                        <Tag :value="item.file?.sha256 ? item.file.sha256.slice(0, 10) : '-'" severity="secondary" />
                                    </div>
                                </div>
                            </div>
                        </TabPanel>
                        <TabPanel value="events">
                            <div class="info-block">
                                <div class="block-head">
                                    <div>
                                        <div class="block-title">เหตุการณ์สำคัญ</div>
                                        <small>แสดงเฉพาะเหตุการณ์สำคัญ</small>
                                    </div>
                                    <Tag :value="`${importantEvents.length} รายการ`" severity="secondary" />
                                </div>
                                <div v-if="importantEvents.length === 0" class="empty-log">ยังไม่มีเหตุการณ์สำคัญ</div>
                                <Timeline v-else :value="importantEvents" align="left" class="opposite-timeline">
                                    <template #opposite="{ item }">
                                        <div class="event-time">{{ formatTimelineDate(item.createdAt) }}</div>
                                    </template>
                                    <template #marker="{ item }">
                                        <span class="event-marker" :class="`event-${item.view.severity}`">
                                            <i :class="item.view.icon"></i>
                                        </span>
                                    </template>
                                    <template #content="{ item }">
                                        <div class="event-line">
                                            <strong>{{ item.view.title }}</strong>
                                            <span>{{ item.view.detail }}</span>
                                            <small v-if="item.actorLabel">โดย {{ item.actorLabel }}</small>
                                        </div>
                                    </template>
                                </Timeline>
                            </div>
                        </TabPanel>
                    </TabPanels>
                </Tabs>
            </aside>
        </div>
    </div>

    <DocumentFlowDialog :visible="flowDialog" :document="flowDocument" @update:visible="setFlowDialogVisible" @open-document="openFlowDocument" />
    <ReadOnlyPdfDialog v-model:visible="evidenceDialog" :url="evidencePdfUrl" :title="evidencePdfTitle" />

    <Dialog v-model:visible="layoutDialog" modal maximizable :style="{ width: 'min(92rem, 98vw)' }" :contentStyle="{ padding: '1rem', overflow: 'hidden' }" header="จัดวางกรอบบน PDF ฉบับจริง" :draggable="false">
        <div class="internal-layout-dialog">
            <Message severity="info" :closable="false" class="m-0">กรอบนี้ใช้กับ Draft ฉบับนี้เท่านั้น วางลายเซ็นของทุกตำแหน่งและข้อความกฎหมายให้ครบก่อนส่ง</Message>
            <DocumentLayoutDesigner
                v-model="layoutDraft.boxes"
                v-model:legalNoticeBox="layoutDraft.legalNoticeBox"
                v-model:legalNoticeBoxes="layoutDraft.legalNoticeBoxes"
                :pdfUrl="internalLayoutPDFUrl"
                :pageCount="document?.originalFile?.pageCount || 0"
                :configs="document?.steps || []"
                fullHeight
                @validation-change="layoutValidationIssues = $event"
            />
        </div>
        <template #footer>
            <Button label="ยกเลิก" severity="secondary" text :disabled="savingLayout" @click="layoutDialog = false" />
            <Button label="บันทึกกรอบ" icon="pi pi-check" :loading="savingLayout" @click="saveInternalLayout" />
        </template>
    </Dialog>

    <Dialog v-model:visible="cancelDialog" modal header="ยกเลิกเอกสารภายใน" :style="{ width: 'min(34rem, 94vw)' }" :draggable="false">
        <div class="cancel-document-form">
            <Message severity="warn" :closable="false" class="m-0">หลังยกเลิก เอกสารจะไม่ส่งต่อให้ผู้เซ็น และลิงก์ภายนอกที่มีอยู่จะใช้ไม่ได้</Message>
            <label>เหตุผลการยกเลิก <span class="required-mark">*</span></label>
            <Textarea v-model="cancelReason" rows="4" maxlength="1000" autoResize placeholder="ระบุเหตุผลเพื่อเก็บในประวัติเอกสาร" />
        </div>
        <template #footer>
            <Button label="กลับ" severity="secondary" text :disabled="cancellingDocument" @click="cancelDialog = false" />
            <Button label="ยกเลิกเอกสาร" icon="pi pi-trash" severity="danger" :disabled="!cancelReason.trim()" :loading="cancellingDocument" @click="cancelDocument" />
        </template>
    </Dialog>

    <Dialog v-model:visible="tokenDialog" modal header="ลิงก์ภายนอก / OTP" :style="{ width: 'min(42rem, 92vw)' }">
        <div v-if="generatedToken" class="token-box">
            <Message v-if="copyFallbackVisible" severity="warn" class="m-0">
                คัดลอกอัตโนมัติไม่ได้ กรุณาเลือกข้อความด้านล่างแล้วคัดลอกเอง
            </Message>
            <label>Link</label>
            <div class="copy-row">
                <InputText :modelValue="generatedToken.url" readonly class="w-full" @focus="selectInput" @click="selectInput" />
                <Button icon="pi pi-copy" rounded outlined aria-label="copy link" @click="copy(generatedToken.url)" />
            </div>
            <label>OTP</label>
            <div class="copy-row">
                <InputText :modelValue="generatedToken.otp" readonly class="w-full otp-text" @focus="selectInput" @click="selectInput" />
                <Button icon="pi pi-copy" rounded outlined aria-label="copy otp" @click="copy(generatedToken.otp)" />
            </div>
            <Textarea v-if="copyFallbackVisible" :modelValue="copyFallbackValue" readonly rows="3" autoResize @focus="selectInput" @click="selectInput" />
            <small class="text-muted-color">OTP หมดอายุ {{ formatThaiDateTime(generatedToken.expiresAt) }}</small>
        </div>
    </Dialog>
</template>

<style scoped>
.signing-detail {
    height: calc(100dvh - 8rem);
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}
.editor-bar {
    min-height: 4rem;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.75rem;
    padding: 0.65rem 0.75rem;
    background: var(--surface-card);
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    position: sticky;
    top: 0;
    z-index: 2;
}
.bar-title {
    min-width: 0;
    flex: 1;
}
.bar-title strong {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.print-row small,
.event-line small,
.event-time {
    color: var(--text-color-secondary);
}
.detail-grid {
    min-height: 0;
    flex: 1;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(360px, 420px);
    gap: 0.75rem;
}
.pdf-panel,
.side-panel {
    min-height: 0;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
}
.pdf-panel {
    display: flex;
    flex-direction: column;
    padding: 0.75rem;
}
.pdf-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding-bottom: 0.75rem;
}
.toolbar-group {
    display: flex;
    min-width: 0;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.45rem;
}
.toolbar-group.right {
    justify-content: flex-end;
}
.page-select {
    min-width: 8rem;
}
.zoom-value {
    width: 3.4rem;
    text-align: center;
    color: var(--text-color-secondary);
    font-size: 0.875rem;
}
.pdf-meta {
    white-space: nowrap;
    color: var(--text-color-secondary);
    font-size: 0.875rem;
}
.pdf-scroll {
    min-height: 0;
    flex: 1;
    overflow: auto;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-ground);
    padding: 0.85rem;
}
.pdf-page-shell {
    display: inline-block;
    background: white;
    line-height: 0;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.14);
}
.pdf-canvas {
    display: block;
}
.pdf-error {
    width: min(34rem, 100%);
    margin: 1rem auto;
}
.empty-pdf {
    min-height: 18rem;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.6rem;
    color: var(--text-color-secondary);
}
.side-panel {
    overflow: hidden;
    padding: 0.75rem;
    display: flex;
    flex-direction: column;
}
.side-panel :deep(.p-tabs) {
    min-height: 0;
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
}
.side-panel :deep(.p-tabpanels) {
    min-height: 0;
    flex: 1 1 auto;
    overflow: auto;
    padding: 0.85rem 0 0;
}
.side-panel :deep(.p-tabpanel) {
    min-height: 0;
}
.info-block {
    display: grid;
    gap: 0.5rem;
}
.block-title {
    font-weight: 700;
}
.block-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}
.block-head small,
.empty-log {
    color: var(--text-color-secondary);
}
.copy-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
}
.external-signer-block {
    margin-bottom: 0.75rem;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid var(--surface-border);
}
.external-signer-list {
    display: grid;
    gap: 0.55rem;
}
.external-signer-row {
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.7rem 0.75rem;
    background: color-mix(in srgb, var(--surface-ground) 42%, var(--surface-card));
}
.external-signer-main {
    min-width: 0;
    display: grid;
    gap: 0.12rem;
}
.external-signer-main strong,
.external-signer-main small {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.external-signer-main small {
    color: var(--text-color-secondary);
}
.external-signer-actions {
    flex: 0 0 auto;
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
}
.print-list {
    display: grid;
    gap: 0.5rem;
}
.print-row {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.65rem 0.75rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
}
.print-row span {
    display: grid;
    gap: 0.15rem;
    min-width: 0;
}
.event-line {
    display: grid;
    gap: 0.2rem;
    min-width: 0;
    padding: 0 0 1.25rem 0.35rem;
}
.event-time {
    min-width: 7.5rem;
    padding-top: 0.15rem;
    text-align: right;
    font-size: 0.85rem;
}
.event-marker {
    width: 1.65rem;
    height: 1.65rem;
    border-radius: 999px;
    display: inline-grid;
    place-items: center;
    border: 2px solid var(--surface-card);
    font-size: 0.78rem;
    flex: 0 0 auto;
}
.opposite-timeline :deep(.p-timeline-event-opposite) {
    flex: 0 0 8.25rem;
    padding: 0 0.75rem 0 0;
}
.opposite-timeline :deep(.p-timeline-event-content) {
    padding-left: 0.75rem;
}
.opposite-timeline :deep(.p-timeline-event-marker) {
    border: 0;
}
.event-success {
    color: var(--green-700, #15803d);
    background: var(--green-100, #dcfce7);
}
.event-info {
    color: var(--blue-700, #1d4ed8);
    background: var(--blue-100, #dbeafe);
}
.event-danger {
    color: var(--red-700, #b91c1c);
    background: var(--red-100, #fee2e2);
}
.event-warn {
    color: var(--yellow-800, #854d0e);
    background: var(--yellow-100, #fef9c3);
}
.empty-log {
    min-height: 3.5rem;
    display: grid;
    place-items: center;
    border: 1px dashed var(--surface-border);
    border-radius: 8px;
    padding: 0.75rem;
}
.token-box {
    display: grid;
    gap: 0.75rem;
}
.internal-layout-dialog { display: grid; gap: .75rem; min-height: min(76dvh, 56rem); }
.cancel-document-form { display: grid; gap: .75rem; }
.cancel-document-form label { font-weight: 600; }
.required-mark { color: var(--red-500, #ef4444); }
.otp-text {
    font-size: 1.35rem;
    font-weight: 700;
    letter-spacing: 0;
}
@media (max-width: 980px) {
    .signing-detail {
        height: auto;
    }
    .detail-grid {
        grid-template-columns: 1fr;
    }
    .pdf-panel {
        height: 72dvh;
    }
}
@media (max-width: 640px) {
    .pdf-toolbar,
    .toolbar-group.right {
        align-items: stretch;
        flex-direction: column;
    }
    .toolbar-group {
        width: 100%;
    }
    .opposite-timeline :deep(.p-timeline-event) {
        align-items: flex-start;
    }
    .opposite-timeline :deep(.p-timeline-event-opposite) {
        display: block;
        flex: 0 0 5.5rem;
        padding-right: 0.5rem;
    }
    .event-time {
        min-width: 0;
        overflow-wrap: anywhere;
        font-size: 0.78rem;
    }
    .external-signer-row,
    .external-signer-actions {
        align-items: stretch;
        flex-direction: column;
    }
    .external-signer-actions {
        width: 100%;
    }
    .external-signer-actions :deep(.p-button) {
        width: 100%;
    }
}
</style>
