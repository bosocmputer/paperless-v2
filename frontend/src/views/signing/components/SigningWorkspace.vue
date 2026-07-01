<script setup>
import * as pdfjsLib from 'pdfjs-dist';
import pdfWorker from 'pdfjs-dist/build/pdf.worker.mjs?url';
import DocumentWorkflowTimeline from '@/views/signing/components/DocumentWorkflowTimeline.vue';
import RelatedDocumentsPanel from '@/views/signing/components/RelatedDocumentsPanel.vue';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;

const props = defineProps({
    document: { type: Object, default: null },
    task: { type: Object, default: null },
    legal: { type: Object, default: null },
    pdfUrl: { type: String, default: '' },
    pdfHeaders: { type: Object, default: () => ({}) },
    loading: { type: Boolean, default: false },
    saving: { type: Boolean, default: false },
    identityLabel: { type: String, default: '' },
    publicMode: { type: Boolean, default: false },
    onBack: { type: Function, default: null },
    onReload: { type: Function, default: null },
    onSign: { type: Function, default: null },
    onReject: { type: Function, default: null },
    onAttach: { type: Function, default: null },
    onRecordEvent: { type: Function, default: null },
    relatedLoader: { type: Function, default: null },
    readOnly: { type: Boolean, default: false },
    readOnlyReason: { type: String, default: '' },
    openEventName: { type: String, default: '' },
    pdfOpenEventName: { type: String, default: '' }
});

const confirm = useConfirm();
const toast = useToast();
const viewerRef = ref(null);
const pdfCanvas = ref(null);
const signCanvas = ref(null);
const pdfDoc = shallowRef(null);
const currentPage = ref(1);
const pageCount = ref(0);
const zoom = ref(1);
const fitWidthActive = ref(true);
const pdfLoading = ref(false);
const pdfReady = ref(false);
const pdfError = ref('');
const hasSignature = ref(false);
const legalAccepted = ref(false);
const rejectVisible = ref(false);
const rejectReason = ref('');
const attachmentNote = ref('');
const uploadingAttachment = ref(false);
const attachmentCount = ref(0);
const localSaving = ref(false);
const relatedVisible = ref(false);
const attachmentVisible = ref(false);
const legalDialogVisible = ref(false);
const pdfDialogVisible = ref(false);
const pdfDialogUrl = ref('');
const pdfDialogLoading = ref(false);
const signIdempotencyKey = ref(newRequestKey());
const rejectIdempotencyKey = ref(newRequestKey());

const sessionId = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
const openedAt = Date.now();
let renderSequence = 0;
let renderTask = null;
let resizeObserver = null;
let signCtx = null;
let drawing = false;
let submitted = false;
let taskOpenRecorded = false;

const isBusy = computed(() => props.saving || localSaving.value);
const legalText = computed(() => props.legal?.text || 'ข้าพเจ้ายืนยันการลงลายเซ็นอิเล็กทรอนิกส์นี้ตาม พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์ และยอมรับให้ใช้เป็นหลักฐานประกอบเอกสารนี้');
const taskStatus = computed(() => props.task?.status || '');
const canInteract = computed(() => !props.readOnly && taskStatus.value === 'pending');
const canConfirm = computed(() => canInteract.value && pdfReady.value && hasSignature.value && legalAccepted.value && !isBusy.value);
const pageOptions = computed(() => Array.from({ length: pageCount.value }, (_, index) => ({ label: `${index + 1}/${pageCount.value}`, value: index + 1 })));
const statusView = computed(() => statusMeta(taskStatus.value));
const taskOpenEvent = computed(() => {
    if (props.openEventName) return props.openEventName;
    if (taskStatus.value === 'pending') return 'ready_task_open';
    if (taskStatus.value === 'waiting') return 'waiting_task_open';
    return 'task_open';
});
const primaryDisabledReason = computed(() => {
    if (!canInteract.value) return statusView.value.message;
    if (!pdfReady.value) return 'รอให้ PDF โหลดเสร็จก่อน';
    if (!hasSignature.value) return 'กรุณาวาดลายเซ็นก่อน';
    if (!legalAccepted.value) return 'กรุณายืนยันข้อความ พ.ร.บ. ก่อน';
    return '';
});

onMounted(async () => {
    window.addEventListener('beforeunload', handleBeforeUnload);
    await nextTick();
    setupSignatureCanvas();
    setupResizeObserver();
    if (props.pdfUrl) await loadPDF();
});

onBeforeUnmount(() => {
    window.removeEventListener('beforeunload', handleBeforeUnload);
    cleanupPDF();
    cleanupPdfDialog();
    if (resizeObserver) resizeObserver.disconnect();
});

onBeforeRouteLeave((_to, _from, next) => {
    if (!shouldWarnBeforeLeave()) {
        next();
        return;
    }
    confirm.require({
        message: 'คุณวาดลายเซ็นไว้แล้ว แต่ยังไม่ได้ยืนยัน ต้องการออกจากหน้านี้หรือไม่?',
        header: 'ออกจากหน้าเซ็นเอกสาร',
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

watch(
    () => props.pdfUrl,
    async () => {
        if (props.pdfUrl) await loadPDF();
    }
);

watch(pdfDialogVisible, (visible) => {
    if (!visible) cleanupPdfDialog();
});

watch(
    () => props.task?.id,
    (taskId) => {
        if (taskId && !taskOpenRecorded) {
            taskOpenRecorded = true;
            recordEvent(taskOpenEvent.value);
        }
    },
    { immediate: true }
);

watch(
    () => [props.loading, props.task?.id],
    async ([loading, taskId], oldValue = []) => {
        const previousTaskId = oldValue[1];
        if (!loading && taskId) {
            await nextTick();
            setupSignatureCanvas(taskId !== previousTaskId);
            if (taskId !== previousTaskId) {
                signIdempotencyKey.value = newRequestKey();
                rejectIdempotencyKey.value = newRequestKey();
                attachmentVisible.value = false;
                legalDialogVisible.value = false;
                pdfDialogVisible.value = false;
            }
        }
    },
    { immediate: true }
);

watch([currentPage, zoom], async () => {
    if (pdfDoc.value) await renderCurrentPage();
});

function setupResizeObserver() {
    if (!viewerRef.value || !window.ResizeObserver) return;
    resizeObserver = new ResizeObserver(() => {
        if (fitWidthActive.value) fitWidth();
    });
    resizeObserver.observe(viewerRef.value);
}

async function loadPDF() {
    cleanupPDF();
    pdfLoading.value = true;
    pdfReady.value = false;
    pdfError.value = '';
    try {
        const loadingTask = pdfjsLib.getDocument({ url: props.pdfUrl, httpHeaders: props.pdfHeaders || {} });
        pdfDoc.value = await loadingTask.promise;
        pageCount.value = pdfDoc.value.numPages;
        currentPage.value = 1;
        await nextTick();
        await fitWidth();
        pdfReady.value = true;
        recordEvent('pdf_load_success');
    } catch (err) {
        pdfError.value = err?.message || 'โหลด PDF ไม่สำเร็จ';
        recordEvent('pdf_load_error', { errorCode: 'pdf_load_error' });
    } finally {
        pdfLoading.value = false;
    }
}

async function fitWidth() {
    if (!pdfDoc.value || !viewerRef.value) return;
    fitWidthActive.value = true;
    const page = await pdfDoc.value.getPage(currentPage.value);
    const viewport = page.getViewport({ scale: 1 });
    const available = Math.max(viewerRef.value.clientWidth - 32, 240);
    zoom.value = clamp(available / viewport.width, 0.45, 2.25);
    await renderCurrentPage();
}

async function renderCurrentPage() {
    if (!pdfDoc.value || !pdfCanvas.value) return;
    const sequence = ++renderSequence;
    cancelRenderTask();
    try {
        const page = await pdfDoc.value.getPage(currentPage.value);
        if (sequence !== renderSequence) return;
        const viewport = page.getViewport({ scale: zoom.value });
        const outputScale = Math.min(window.devicePixelRatio || 1, 2);
        const canvas = pdfCanvas.value;
        const context = canvas.getContext('2d');
        canvas.width = Math.floor(viewport.width * outputScale);
        canvas.height = Math.floor(viewport.height * outputScale);
        canvas.style.width = `${viewport.width}px`;
        canvas.style.height = `${viewport.height}px`;
        context.setTransform(outputScale, 0, 0, outputScale, 0, 0);
        renderTask = page.render({ canvasContext: context, viewport });
        await renderTask.promise;
    } catch (err) {
        if (err?.name === 'RenderingCancelledException') return;
        pdfError.value = err?.message || 'แสดง PDF ไม่สำเร็จ';
        recordEvent('pdf_load_error', { errorCode: 'pdf_render_error' });
    } finally {
        if (sequence === renderSequence) renderTask = null;
    }
}

function cancelRenderTask() {
    if (!renderTask) return;
    try {
        renderTask.cancel();
    } catch {
        // PDF.js may throw if rendering already completed.
    }
    renderTask = null;
}

function cleanupPDF() {
    cancelRenderTask();
    if (pdfDoc.value?.destroy) pdfDoc.value.destroy().catch(() => {});
    pdfDoc.value = null;
    pageCount.value = 0;
    pdfReady.value = false;
}

function zoomIn() {
    fitWidthActive.value = false;
    zoom.value = clamp(zoom.value + 0.15, 0.45, 2.5);
}

function zoomOut() {
    fitWidthActive.value = false;
    zoom.value = clamp(zoom.value - 0.15, 0.45, 2.5);
}

function setupSignatureCanvas(force = false) {
    if (!signCanvas.value) return;
    if (signCtx && !force) return;
    const rect = signCanvas.value.getBoundingClientRect();
    if (rect.width <= 0) {
        window.requestAnimationFrame(() => setupSignatureCanvas(force));
        return;
    }
    const ratio = Math.min(window.devicePixelRatio || 1, 2);
    signCanvas.value.width = Math.floor(rect.width * ratio);
    signCanvas.value.height = Math.floor(188 * ratio);
    signCanvas.value.style.height = '188px';
    signCtx = signCanvas.value.getContext('2d');
    signCtx.setTransform(ratio, 0, 0, ratio, 0, 0);
    signCtx.lineWidth = 2.4;
    signCtx.lineCap = 'round';
    signCtx.lineJoin = 'round';
    signCtx.strokeStyle = '#111827';
    clearSignature(false);
}

function point(event) {
    const rect = signCanvas.value.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
}

function startDraw(event) {
    if (!canInteract.value || !signCtx) return;
    event.preventDefault();
    signCanvas.value.setPointerCapture?.(event.pointerId);
    drawing = true;
    const p = point(event);
    signCtx.beginPath();
    signCtx.moveTo(p.x, p.y);
    if (!hasSignature.value) recordEvent('signature_started');
}

function moveDraw(event) {
    if (!drawing || !signCtx) return;
    event.preventDefault();
    const p = point(event);
    signCtx.lineTo(p.x, p.y);
    signCtx.stroke();
    hasSignature.value = true;
}

function endDraw(event) {
    drawing = false;
    if (event?.pointerId) signCanvas.value?.releasePointerCapture?.(event.pointerId);
}

function clearSignature(sendEvent = true) {
    if (!signCtx || !signCanvas.value) return;
    const rect = signCanvas.value.getBoundingClientRect();
    signCtx.clearRect(0, 0, rect.width, 188);
    signCtx.fillStyle = '#ffffff';
    signCtx.fillRect(0, 0, rect.width, 188);
    hasSignature.value = false;
    if (sendEvent) recordEvent('signature_cleared');
}

async function confirmSign() {
    if (!canConfirm.value || !props.onSign) return;
    localSaving.value = true;
    recordEvent('sign_attempt');
    try {
        const payload = {
            signatureDataUrl: signCanvas.value.toDataURL('image/png'),
            legalAccepted: true,
            legalText: legalText.value,
            deviceId: deviceId(),
            idempotencyKey: signIdempotencyKey.value
        };
        await props.onSign(payload);
        submitted = true;
        recordEvent('sign_success');
    } catch (err) {
        recordEvent('sign_error', { errorCode: err?.payload?.error || err?.message || 'sign_error' });
    } finally {
        localSaving.value = false;
    }
}

async function rejectTask() {
    const reason = rejectReason.value.trim();
    if (!reason) {
        toast.add({ severity: 'warn', summary: 'กรุณาระบุเหตุผล', life: 2500 });
        return;
    }
    confirm.require({
        message: 'ยืนยันปฏิเสธเอกสารนี้หรือไม่?',
        header: 'ปฏิเสธเอกสาร',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'ยกเลิก',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ปฏิเสธเอกสาร',
            severity: 'danger'
        },
        accept: () => submitRejectTask(reason)
    });
}

async function submitRejectTask(reason) {
    localSaving.value = true;
    try {
        await props.onReject?.({
            reason,
            deviceId: deviceId(),
            idempotencyKey: rejectIdempotencyKey.value
        });
        submitted = true;
        recordEvent('reject_success');
        rejectVisible.value = false;
    } catch {
        // Parent handlers show the actionable error toast.
    } finally {
        localSaving.value = false;
    }
}

async function uploadAttachment(event) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file || !props.onAttach) return;
    uploadingAttachment.value = true;
    try {
        await props.onAttach(file, attachmentNote.value);
        attachmentCount.value += 1;
        attachmentNote.value = '';
        recordEvent('attachment_upload');
        toast.add({ severity: 'success', summary: 'แนบไฟล์แล้ว', life: 2200 });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'แนบไฟล์ไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        uploadingAttachment.value = false;
    }
}

async function fetchPdfBlob() {
    const response = await fetch(props.pdfUrl, { headers: props.pdfHeaders || {} });
    if (!response.ok) throw new Error('โหลด PDF ไม่สำเร็จ');
    return response.blob();
}

async function openFullPDF() {
    if (pdfDialogUrl.value) {
        pdfDialogVisible.value = true;
        return;
    }
    pdfDialogVisible.value = true;
    pdfDialogLoading.value = true;
    try {
        if (props.pdfOpenEventName) recordEvent(props.pdfOpenEventName);
        const blob = await fetchPdfBlob();
        pdfDialogUrl.value = URL.createObjectURL(blob);
    } catch (err) {
        pdfDialogVisible.value = false;
        toast.add({ severity: 'error', summary: 'เปิด PDF ไม่สำเร็จ', detail: err.message, life: 3000 });
    } finally {
        pdfDialogLoading.value = false;
    }
}

function cleanupPdfDialog() {
    if (pdfDialogUrl.value) URL.revokeObjectURL(pdfDialogUrl.value);
    pdfDialogUrl.value = '';
    pdfDialogLoading.value = false;
}

async function openPDF() {
    try {
        if (props.pdfOpenEventName) recordEvent(props.pdfOpenEventName);
        const blob = await fetchPdfBlob();
        const url = URL.createObjectURL(blob);
        window.open(url, '_blank', 'noopener');
        setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch (err) {
        toast.add({ severity: 'error', summary: 'เปิด PDF ไม่สำเร็จ', detail: err.message, life: 3000 });
    }
}

async function downloadPDF() {
    try {
        if (props.pdfOpenEventName) recordEvent(props.pdfOpenEventName);
        const blob = await fetchPdfBlob();
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `${props.document?.docNo || 'document'}.pdf`;
        link.click();
        URL.revokeObjectURL(url);
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ดาวน์โหลด PDF ไม่สำเร็จ', detail: err.message, life: 3000 });
    }
}

function recordEvent(event, extra = {}) {
    props.onRecordEvent?.({
        event,
        sessionId,
        elapsedMs: Date.now() - openedAt,
        pdfPage: currentPage.value,
        pdfPageCount: pageCount.value,
        attachmentCount: attachmentCount.value,
        viewport: { width: window.innerWidth, height: window.innerHeight },
        ...extra
    });
}

function shouldWarnBeforeLeave() {
    return canInteract.value && hasSignature.value && !submitted && !isBusy.value;
}

function handleBeforeUnload(event) {
    if (!shouldWarnBeforeLeave()) return;
    event.preventDefault();
    event.returnValue = '';
}

function deviceId() {
    const key = props.publicMode ? 'paperless_external_device_id' : 'paperless_device_id';
    let value = localStorage.getItem(key);
    if (!value) {
        value = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
        localStorage.setItem(key, value);
    }
    return value;
}

function toggleRelatedDocuments() {
    relatedVisible.value = !relatedVisible.value;
    if (relatedVisible.value) recordEvent('related_documents_open');
}

function statusMeta(status) {
    switch (status) {
        case 'pending':
            return { label: 'รอเซ็น', severity: 'info', message: 'เอกสารนี้ถึงลำดับของคุณแล้ว' };
        case 'signed':
            return { label: 'เซ็นแล้ว', severity: 'success', message: 'คุณเซ็นเอกสารนี้แล้ว' };
        case 'rejected':
            return { label: 'ปฏิเสธแล้ว', severity: 'danger', message: 'เอกสารถูกปฏิเสธแล้ว' };
        case 'waiting':
            return { label: 'ยังไม่ถึงลำดับ', severity: 'warn', message: 'เอกสารยังไม่ถึงลำดับเซ็นของคุณ' };
        case 'skipped':
            return { label: 'ข้ามแล้ว', severity: 'secondary', message: 'ขั้นตอนนี้ถูกข้ามตามเงื่อนไขเอกสาร' };
        default:
            return { label: status || 'ไม่ทราบสถานะ', severity: 'secondary', message: 'ไม่สามารถเซ็นเอกสารนี้ได้' };
    }
}

function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
}

function newRequestKey() {
    return crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
}
</script>

<template>
    <section class="signing-workspace" :class="{ 'read-only-workspace': !canInteract }">
        <div class="signing-header">
            <Button v-if="onBack" icon="pi pi-arrow-left" text rounded aria-label="กลับ" @click="onBack" />
            <div class="doc-title">
                <strong>{{ document?.docNo || 'เอกสาร' }}</strong>
                <span>{{ task?.positionName || '-' }} · {{ document?.partyName || document?.partyCode || '-' }}</span>
            </div>
            <Tag :value="statusView.label" :severity="statusView.severity" />
        </div>

        <div v-if="loading" class="loading-state">
            <i class="pi pi-spin pi-spinner"></i>
            <span>กำลังโหลดเอกสาร</span>
        </div>

        <div v-else class="workspace-grid" :class="{ 'readonly-grid': !canInteract }">
            <section class="pdf-shell">
                <div class="pdf-toolbar">
                    <div class="toolbar-title">
                        <strong>PDF</strong>
                        <span v-if="pageCount">หน้า {{ currentPage }} จาก {{ pageCount }}</span>
                    </div>
                    <Select v-if="pageOptions.length > 1" v-model="currentPage" :options="pageOptions" optionLabel="label" optionValue="value" class="page-select" />
                    <div class="toolbar-actions">
                        <Button class="desktop-tool" icon="pi pi-minus" text rounded aria-label="ซูมออก" :disabled="!pdfDoc" @click="zoomOut" />
                        <Button label="พอดีกว้าง" icon="pi pi-arrows-h" severity="secondary" text :disabled="!pdfDoc" @click="fitWidth" />
                        <Button class="desktop-tool" icon="pi pi-plus" text rounded aria-label="ซูมเข้า" :disabled="!pdfDoc" @click="zoomIn" />
                        <Button label="เต็มจอ" icon="pi pi-window-maximize" text :disabled="!pdfUrl" @click="openFullPDF" />
                    </div>
                </div>
                <div ref="viewerRef" class="pdf-viewer">
                    <div v-if="pdfLoading" class="pdf-state">
                        <i class="pi pi-spin pi-spinner"></i>
                        <span>กำลังแสดง PDF</span>
                    </div>
                    <Message v-else-if="pdfError" severity="error" class="pdf-error">
                        {{ pdfError }}
                        <div class="mt-3 flex gap-2">
                            <Button size="small" label="ลองใหม่" icon="pi pi-refresh" severity="secondary" outlined @click="loadPDF" />
                            <Button size="small" label="ดูเต็มจอ" icon="pi pi-window-maximize" @click="openFullPDF" />
                        </div>
                    </Message>
                    <canvas v-show="!pdfLoading && !pdfError" ref="pdfCanvas" class="pdf-canvas"></canvas>
                </div>
            </section>

            <aside v-if="canInteract" class="sign-card">
                <div class="signer-summary position-summary">
                    <span><i class="pi pi-user-edit"></i> ตำแหน่งของคุณ</span>
                    <strong>{{ task?.positionName || '-' }}</strong>
                    <small>{{ identityLabel || task?.signerName || task?.signerUser || '-' }}</small>
                    <Message :severity="canInteract ? 'info' : statusView.severity">{{ statusView.message }}</Message>
                </div>

                <div v-if="relatedLoader" class="related-section">
                    <Button
                        :label="relatedVisible ? 'ซ่อนเอกสารประกอบ' : 'ดูเอกสารประกอบ'"
                        :icon="relatedVisible ? 'pi pi-chevron-up' : 'pi pi-sitemap'"
                        severity="secondary"
                        outlined
                        class="w-full"
                        @click="toggleRelatedDocuments"
                    />
                    <RelatedDocumentsPanel v-if="relatedVisible" compact :loader="relatedLoader" :record-event="recordEvent" />
                </div>

                <div class="signature-block">
                    <div class="section-heading">
                        <strong>ลายเซ็น</strong>
                        <Button label="ล้าง" icon="pi pi-eraser" severity="secondary" outlined size="small" :disabled="!hasSignature || !canInteract" @click="clearSignature" />
                    </div>
                    <canvas
                        ref="signCanvas"
                        class="signature-canvas"
                        :class="{ disabled: !canInteract }"
                        @pointerdown="startDraw"
                        @pointermove="moveDraw"
                        @pointerup="endDraw"
                        @pointercancel="endDraw"
                        @pointerleave="endDraw"
                    ></canvas>
                </div>

                <div class="attachment-block">
                    <Button
                        :label="attachmentVisible ? `ซ่อนไฟล์อ้างอิง (${attachmentCount} ไฟล์)` : `แนบไฟล์อ้างอิง (${attachmentCount} ไฟล์)`"
                        :icon="attachmentVisible ? 'pi pi-chevron-up' : 'pi pi-paperclip'"
                        severity="secondary"
                        outlined
                        class="w-full"
                        @click="attachmentVisible = !attachmentVisible"
                    />
                    <div v-if="attachmentVisible" class="attachment-fields">
                        <InputText v-model="attachmentNote" placeholder="หมายเหตุไฟล์แนบ (ถ้ามี)" :disabled="!canInteract || uploadingAttachment" />
                        <label class="attach-button" :class="{ disabled: !canInteract || uploadingAttachment }">
                            <input type="file" accept="application/pdf,image/png,image/jpeg" :disabled="!canInteract || uploadingAttachment" @change="uploadAttachment" />
                            <i :class="uploadingAttachment ? 'pi pi-spin pi-spinner' : 'pi pi-paperclip'"></i>
                            <span>{{ uploadingAttachment ? 'กำลังแนบไฟล์' : 'เลือก PDF/รูปภาพ' }}</span>
                        </label>
                    </div>
                </div>

                <div class="legal-check">
                    <Checkbox v-model="legalAccepted" inputId="legalAccepted" binary :disabled="!canInteract" />
                    <label for="legalAccepted">ยืนยันข้อความ พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์</label>
                    <Button label="อ่านข้อความ" icon="pi pi-book" text size="small" @click="legalDialogVisible = true" />
                </div>
            </aside>

            <aside v-else class="sign-card readonly-card">
                <div class="signer-summary position-summary">
                    <span><i class="pi pi-user-edit"></i> ตำแหน่งของคุณ</span>
                    <strong>{{ task?.positionName || '-' }}</strong>
                    <small>{{ identityLabel || task?.signerName || task?.signerUser || '-' }}</small>
                    <Message :severity="statusView.severity">
                        {{ readOnlyReason || (taskStatus === 'waiting' ? 'ยังไม่ถึงคิวของคุณ ต้องรอขั้นตอนก่อนหน้าเสร็จก่อน' : statusView.message) }}
                    </Message>
                </div>

                <div class="section-heading readonly-heading">
                    <strong>ความคืบหน้าเอกสาร</strong>
                    <Tag :value="statusView.label" :severity="statusView.severity" />
                </div>
                <DocumentWorkflowTimeline :document="document" compact />
                <div v-if="relatedLoader" class="related-section">
                    <Button
                        :label="relatedVisible ? 'ซ่อนเอกสารประกอบ' : 'ดูเอกสารประกอบ'"
                        :icon="relatedVisible ? 'pi pi-chevron-up' : 'pi pi-sitemap'"
                        severity="secondary"
                        outlined
                        class="w-full"
                        @click="toggleRelatedDocuments"
                    />
                    <RelatedDocumentsPanel v-if="relatedVisible" compact :loader="relatedLoader" :record-event="recordEvent" />
                </div>
            </aside>
        </div>

        <div v-if="!loading && canInteract" class="sticky-actions">
            <Message v-if="primaryDisabledReason" severity="warn" class="sticky-reason">{{ primaryDisabledReason }}</Message>
            <div class="sticky-buttons">
                <Button label="ปฏิเสธ" icon="pi pi-times" severity="danger" outlined :disabled="!canInteract || isBusy" @click="rejectVisible = true" />
                <Button label="ยืนยันเซ็น" icon="pi pi-check" :disabled="!canConfirm" :loading="isBusy" @click="confirmSign" />
            </div>
        </div>

        <Dialog v-model:visible="rejectVisible" modal header="ปฏิเสธเอกสาร" :style="{ width: 'min(34rem, 94vw)' }">
            <div class="grid gap-3">
                <Message severity="warn">การปฏิเสธจะหยุด workflow ของเอกสารนี้ และไม่ lock SML</Message>
                <label class="font-medium">เหตุผล</label>
                <Textarea v-model="rejectReason" rows="4" autoResize autofocus />
            </div>
            <template #footer>
                <Button label="ยกเลิก" severity="secondary" outlined @click="rejectVisible = false" />
                <Button label="ยืนยันปฏิเสธ" severity="danger" :loading="isBusy" @click="rejectTask" />
            </template>
        </Dialog>

        <Dialog v-model:visible="legalDialogVisible" modal header="ข้อความยืนยัน พ.ร.บ." :style="{ width: 'min(34rem, 94vw)' }">
            <p class="legal-dialog-text">{{ legalText }}</p>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="legalDialogVisible = false" />
                <Button
                    v-if="canInteract"
                    label="รับทราบและยืนยัน"
                    icon="pi pi-check"
                    @click="
                        legalAccepted = true;
                        legalDialogVisible = false;
                    "
                />
            </template>
        </Dialog>

        <Dialog v-model:visible="pdfDialogVisible" modal header="ดู PDF" class="pdf-dialog" :style="{ width: 'min(960px, 96vw)' }">
            <div class="pdf-dialog-body">
                <div v-if="pdfDialogLoading" class="pdf-state">
                    <i class="pi pi-spin pi-spinner"></i>
                    <span>กำลังเปิด PDF</span>
                </div>
                <iframe v-else-if="pdfDialogUrl" :src="pdfDialogUrl" title="PDF" class="pdf-frame"></iframe>
            </div>
        </Dialog>
    </section>
</template>

<style scoped>
.signing-workspace {
    min-height: calc(100dvh - 56px);
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
    padding: 0.65rem;
    padding-bottom: 6.75rem;
}

.signing-header {
    min-height: 46px;
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
}

.doc-title {
    min-width: 0;
    flex: 1;
    display: grid;
    line-height: 1.2;
}

.doc-title strong,
.doc-title span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.doc-title span {
    color: var(--text-color-secondary);
    font-size: 0.9rem;
}

.loading-state,
.pdf-state {
    display: grid;
    place-items: center;
    gap: 0.75rem;
    color: var(--text-color-secondary);
}

.loading-state {
    min-height: 50dvh;
}

.workspace-grid {
    min-height: 0;
    flex: 1;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(340px, 400px);
    gap: 0.65rem;
}

.pdf-shell,
.sign-card {
    min-width: 0;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
}

.pdf-shell {
    min-height: 0;
    display: flex;
    flex-direction: column;
}

.pdf-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.5rem 0.65rem;
    border-bottom: 1px solid var(--surface-border);
}

.toolbar-title {
    display: grid;
    line-height: 1.2;
    min-width: 4rem;
}

.toolbar-title span {
    color: var(--text-color-secondary);
    font-size: 0.82rem;
}

.toolbar-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.15rem;
    flex-wrap: wrap;
    justify-content: flex-end;
}

.toolbar-actions :deep(.p-button) {
    min-height: 36px;
}

.pdf-viewer {
    min-height: 0;
    flex: 1;
    overflow: auto;
    padding: 1rem;
    display: grid;
    justify-items: center;
    align-items: start;
    background: color-mix(in srgb, var(--surface-ground) 70%, var(--surface-card));
}

.pdf-canvas {
    display: block;
    background: white;
    box-shadow: 0 2px 8px rgba(15, 23, 42, 0.16);
}

.pdf-error {
    width: min(34rem, 100%);
    align-self: center;
}

.page-select {
    width: 6.5rem;
    flex: 0 0 auto;
}

.sign-card {
    padding: 0.85rem;
    display: grid;
    gap: 0.85rem;
    align-content: start;
}

.readonly-card {
    max-height: 100%;
    overflow: auto;
}

.signer-summary {
    display: grid;
    gap: 0.65rem;
}

.position-summary {
    display: grid;
    gap: 0.2rem;
    border: 1px solid color-mix(in srgb, var(--primary-color) 24%, var(--surface-border));
    border-radius: 8px;
    padding: 0.7rem 0.8rem;
    background: color-mix(in srgb, var(--primary-color) 8%, var(--surface-card));
}

.position-summary span,
.attachment-block span {
    color: var(--text-color-secondary);
    font-size: 0.86rem;
}

.position-summary span {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
}

.position-summary strong {
    color: var(--primary-color);
    font-size: 1.12rem;
    line-height: 1.2;
}

.position-summary small {
    color: var(--text-color-secondary);
}

.section-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
}

.readonly-heading {
    margin-bottom: 0;
}

.related-section {
    min-width: 0;
    display: grid;
    gap: 0.75rem;
}

.signature-canvas {
    width: 100%;
    height: 188px;
    display: block;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: #fff;
    touch-action: none;
}

.signature-canvas.disabled {
    opacity: 0.65;
}

.attachment-block {
    display: grid;
    gap: 0.55rem;
}

.attachment-fields {
    display: grid;
    gap: 0.55rem;
}

.attach-button {
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    border: 1px dashed var(--surface-border);
    border-radius: 8px;
    color: var(--text-color);
    cursor: pointer;
}

.attach-button input {
    display: none;
}

.attach-button.disabled {
    cursor: not-allowed;
    opacity: 0.6;
}

.legal-check {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.65rem;
    line-height: 1.45;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.7rem 0.8rem;
    background: var(--surface-card);
}

.legal-check label {
    min-width: 0;
    font-weight: 600;
}

.sticky-actions {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 30;
    display: grid;
    gap: 0.45rem;
    padding: 0.75rem;
    padding-bottom: max(0.75rem, env(safe-area-inset-bottom));
    border-top: 1px solid var(--surface-border);
    background: var(--surface-card);
}

.sticky-reason {
    margin: 0;
}

.sticky-buttons {
    display: grid;
    grid-template-columns: minmax(0, 0.85fr) minmax(0, 1.15fr);
    gap: 0.65rem;
}

.sticky-buttons :deep(.p-button) {
    min-height: 44px;
}

.legal-dialog-text {
    margin: 0;
    line-height: 1.7;
    color: var(--text-color);
}

.pdf-dialog-body {
    min-height: 70dvh;
    display: grid;
}

.pdf-frame {
    width: 100%;
    height: 76dvh;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: white;
}

:global(.pdf-dialog .p-dialog-content) {
    padding-top: 0;
}

@media (max-width: 920px) {
    .signing-workspace {
        padding: 0.55rem;
        padding-bottom: 7.25rem;
    }

    .workspace-grid {
        grid-template-columns: 1fr;
    }

    .pdf-shell {
        height: clamp(340px, 46dvh, 430px);
        min-height: 0;
    }

    .pdf-viewer {
        padding: 0.75rem;
    }

    .sign-card {
        padding: 0.75rem;
    }

    .readonly-grid .sign-card {
        order: -1;
    }
}

@media (min-width: 921px) {
    .signing-workspace {
        height: calc(100dvh - 56px);
        overflow: hidden;
    }

    .sticky-actions {
        left: auto;
        width: min(400px, 32vw);
        right: 0.75rem;
        bottom: 0.75rem;
        border: 1px solid var(--surface-border);
        border-radius: 8px;
    }
}

@media (max-width: 520px) {
    .desktop-tool {
        display: none;
    }
}

@media (max-width: 430px) {
    .signing-header {
        padding-inline: 0.45rem;
    }

    .pdf-toolbar {
        padding-inline: 0.55rem;
    }

    .toolbar-actions {
        gap: 0;
    }

    .toolbar-actions :deep(.p-button) {
        padding-inline: 0.45rem;
    }

    .legal-check {
        grid-template-columns: auto minmax(0, 1fr);
    }

    .legal-check :deep(.p-button) {
        grid-column: 2;
        justify-self: start;
        padding-left: 0;
    }

    .signature-canvas {
        height: 168px;
    }
}
</style>
