<script setup>
import ContinuousPdfViewer from '@/views/signing/components/ContinuousPdfViewer.vue';
import DocumentFlowDialog from '@/views/signing/components/DocumentFlowDialog.vue';
import DocumentReferenceCheck from '@/views/signing/components/DocumentReferenceCheck.vue';
import DocumentWorkflowTimeline from '@/views/signing/components/DocumentWorkflowTimeline.vue';
import ReadOnlyPdfDialog from '@/views/signing/components/ReadOnlyPdfDialog.vue';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

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
    externalSignOnly: { type: Boolean, default: false },
    adminWorkspace: { type: Boolean, default: false },
    onBack: { type: Function, default: null },
    onReload: { type: Function, default: null },
    onSign: { type: Function, default: null },
    onReject: { type: Function, default: null },
    onAttach: { type: Function, default: null },
    onRecordEvent: { type: Function, default: null },
    relatedLoader: { type: Function, default: null },
    referenceCheckLoader: { type: Function, default: null },
    readOnly: { type: Boolean, default: false },
    historyFocus: { type: Boolean, default: false },
    readOnlyReason: { type: String, default: '' },
    openEventName: { type: String, default: '' },
    pdfOpenEventName: { type: String, default: '' }
});

const confirm = useConfirm();
const toast = useToast();
const signCanvas = ref(null);
const currentPage = ref(1);
const pageCount = ref(0);
const pdfReady = ref(false);
const hasSignature = ref(false);
const legalAccepted = ref(false);
const rejectVisible = ref(false);
const rejectReason = ref('');
const attachmentNote = ref('');
const uploadingAttachment = ref(false);
const attachmentCount = ref(0);
const localSaving = ref(false);
const flowDialogVisible = ref(false);
const referenceDialogVisible = ref(false);
const attachmentVisible = ref(false);
const legalDialogVisible = ref(false);
const pdfDialogVisible = ref(false);
const signIdempotencyKey = ref(newRequestKey());
const rejectIdempotencyKey = ref(newRequestKey());

const sessionId = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
const openedAt = Date.now();
let signCtx = null;
let drawing = false;
let submitted = false;
let taskOpenRecorded = false;

const isBusy = computed(() => props.saving || localSaving.value);
const legalText = computed(() => props.legal?.text || 'ข้าพเจ้ายืนยันการลงลายเซ็นอิเล็กทรอนิกส์นี้ตาม พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์ และยอมรับให้ใช้เป็นหลักฐานประกอบเอกสารนี้');
const taskStatus = computed(() => props.task?.status || '');
const canInteract = computed(() => !props.readOnly && taskStatus.value === 'pending');
const canConfirm = computed(() => canInteract.value && pdfReady.value && hasSignature.value && legalAccepted.value && !isBusy.value);
const allowFullPDF = computed(() => !props.externalSignOnly);
const allowReject = computed(() => !props.externalSignOnly && !!props.onReject);
const allowAttachments = computed(() => !props.externalSignOnly && !!props.onAttach);
const allowRelatedDocuments = computed(() => !props.externalSignOnly && !props.historyFocus && !!props.relatedLoader);
const allowReferenceCheck = computed(() => !props.externalSignOnly && !props.historyFocus && !!props.referenceCheckLoader);
const showReadOnlyPanel = computed(() => !props.externalSignOnly && !props.historyFocus);
const statusView = computed(() => statusMeta(taskStatus.value));
const signatureTitle = computed(() => ['ลายเซ็น', props.task?.positionName].filter(Boolean).join(' · '));
const signerLine = computed(() => props.identityLabel || props.task?.signerName || props.task?.signerUser || '-');
const historySummary = computed(() => {
    const label = props.task?.positionName ? `ตำแหน่ง ${props.task.positionName}` : 'รายการเซ็นของคุณ';
    if (taskStatus.value === 'rejected') return `${label} · คุณปฏิเสธเอกสารนี้แล้ว`;
    if (taskStatus.value === 'signed') return `${label} · คุณเซ็นเอกสารนี้แล้ว`;
    return `${label} · ${statusView.value.message}`;
});
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
});

onBeforeUnmount(() => {
    window.removeEventListener('beforeunload', handleBeforeUnload);
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
    (url) => {
        pdfReady.value = false;
        currentPage.value = 1;
        pageCount.value = 0;
    }
);

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
                flowDialogVisible.value = false;
                referenceDialogVisible.value = false;
                legalDialogVisible.value = false;
                pdfDialogVisible.value = false;
            }
        }
    },
    { immediate: true }
);

function onPdfLoadSuccess(payload = {}) {
    pdfReady.value = true;
    pageCount.value = Number(payload.pageCount || 0);
    currentPage.value = 1;
    recordEvent('pdf_load_success');
}

function onPdfLoadError(err) {
    pdfReady.value = false;
    recordEvent('pdf_load_error', { errorCode: err?.status || err?.name || 'pdf_load_error' });
}

function onPdfPageChange(pageNo) {
    currentPage.value = Number(pageNo || 1);
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

function openFullPDF() {
    if (!allowFullPDF.value) return;
    if (!props.pdfUrl) {
        toast.add({ severity: 'warn', summary: 'ยังไม่มี PDF', detail: 'รอให้เอกสารโหลดเสร็จก่อน', life: 2500 });
        return;
    }
    pdfDialogVisible.value = true;
    if (props.pdfOpenEventName) recordEvent(props.pdfOpenEventName);
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

function openFlowDialog() {
    flowDialogVisible.value = true;
    recordEvent('related_documents_open');
}

function openReferenceDialog() {
    referenceDialogVisible.value = true;
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

function newRequestKey() {
    return crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
}
</script>

<template>
    <section class="signing-workspace" :class="{ 'read-only-workspace': !canInteract, 'history-focus-workspace': historyFocus, 'admin-workspace': adminWorkspace }">
        <div class="signing-header">
            <Button v-if="onBack" :label="adminWorkspace ? 'กลับ' : undefined" icon="pi pi-arrow-left" text rounded aria-label="กลับ" @click="onBack" />
            <div class="doc-title">
                <strong>{{ document?.docNo || 'เอกสาร' }}</strong>
                <span>{{ [adminWorkspace ? document?.docFormatCode : '', task?.positionName || '-', document?.partyName || document?.partyCode || '-'].filter(Boolean).join(' · ') }}</span>
            </div>
            <div class="header-status">
                <Tag :value="statusView.label" :severity="statusView.severity" />
                <Button v-if="adminWorkspace && onReload" icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="onReload" />
            </div>
        </div>

        <div v-if="loading" class="loading-state">
            <i class="pi pi-spin pi-spinner"></i>
            <span>กำลังโหลดเอกสาร</span>
        </div>

        <Message v-if="historyFocus && !loading" :severity="statusView.severity" class="history-summary">
            {{ historySummary }}
        </Message>

        <div v-if="!loading" class="workspace-grid" :class="{ 'readonly-grid': !canInteract, 'history-focus-grid': historyFocus }">
            <section class="pdf-shell">
                <ContinuousPdfViewer
                    :url="pdfUrl"
                    :headers="pdfHeaders"
                    :allow-open-full="allowFullPDF"
                    toolbar-label="PDF"
                    @open-full="openFullPDF"
                    @load-success="onPdfLoadSuccess"
                    @load-error="onPdfLoadError"
                    @page-change="onPdfPageChange"
                />
            </section>

            <aside v-if="canInteract" class="sign-card">
                <div v-if="allowRelatedDocuments || allowReferenceCheck" class="related-section">
                    <Button
                        v-if="allowRelatedDocuments"
                        label="Flow เอกสาร"
                        icon="pi pi-sitemap"
                        severity="secondary"
                        outlined
                        class="w-full"
                        @click="openFlowDialog"
                    />
                    <Button
                        v-if="allowReferenceCheck"
                        label="ตรวจสอบเอกสาร"
                        icon="pi pi-list"
                        severity="secondary"
                        outlined
                        class="w-full"
                        @click="openReferenceDialog"
                    />
                </div>

                <div class="signature-block">
                    <div class="section-heading">
                        <div class="signature-heading-text">
                            <strong>{{ signatureTitle }}</strong>
                            <small>{{ signerLine }}</small>
                        </div>
                        <Button label="ล้าง" icon="pi pi-eraser" severity="secondary" outlined size="small" :disabled="!hasSignature || !canInteract" @click="clearSignature" />
                    </div>
                    <Message severity="info" class="compact-status">{{ statusView.message }}</Message>
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

                <div v-if="allowAttachments" class="attachment-block">
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

                <div v-if="adminWorkspace" class="admin-actions">
                    <Message v-if="primaryDisabledReason" severity="warn" class="sticky-reason">{{ primaryDisabledReason }}</Message>
                    <div class="admin-action-buttons" :class="{ 'single-action': !allowReject }">
                        <Button v-if="allowReject" label="ปฏิเสธ" icon="pi pi-times" severity="danger" outlined :disabled="!canInteract || isBusy" @click="rejectVisible = true" />
                        <Button label="ยืนยันเซ็น" icon="pi pi-check" :disabled="!canConfirm" :loading="isBusy" @click="confirmSign" />
                    </div>
                </div>
            </aside>

            <aside v-else-if="showReadOnlyPanel" class="sign-card readonly-card">
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
                <div v-if="allowRelatedDocuments || allowReferenceCheck" class="related-section">
                    <Button
                        v-if="allowRelatedDocuments"
                        label="Flow เอกสาร"
                        icon="pi pi-sitemap"
                        severity="secondary"
                        outlined
                        class="w-full"
                        @click="openFlowDialog"
                    />
                    <Button
                        v-if="allowReferenceCheck"
                        label="ตรวจสอบเอกสาร"
                        icon="pi pi-list"
                        severity="secondary"
                        outlined
                        class="w-full"
                        @click="openReferenceDialog"
                    />
                </div>
            </aside>
        </div>

        <div v-if="!adminWorkspace && !loading && canInteract" class="sticky-actions">
            <Message v-if="primaryDisabledReason" severity="warn" class="sticky-reason">{{ primaryDisabledReason }}</Message>
            <div class="sticky-buttons" :class="{ 'single-action': !allowReject }">
                <Button v-if="allowReject" label="ปฏิเสธ" icon="pi pi-times" severity="danger" outlined :disabled="!canInteract || isBusy" @click="rejectVisible = true" />
                <Button label="ยืนยันเซ็น" icon="pi pi-check" :disabled="!canConfirm" :loading="isBusy" @click="confirmSign" />
            </div>
        </div>

        <Dialog v-if="allowReject" v-model:visible="rejectVisible" modal header="ปฏิเสธเอกสาร" :style="{ width: 'min(34rem, 94vw)' }">
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

        <ReadOnlyPdfDialog v-if="allowFullPDF" v-model:visible="pdfDialogVisible" :url="pdfUrl" :headers="pdfHeaders" title="ดู PDF" full-height />

        <DocumentFlowDialog
            v-if="allowRelatedDocuments"
            v-model:visible="flowDialogVisible"
            :document="document"
            :loader="relatedLoader"
            :record-event="recordEvent"
            :admin="false"
            :open-pdf-on-select="false"
        />

        <Dialog
            v-if="allowReferenceCheck"
            v-model:visible="referenceDialogVisible"
            modal
            header="ตรวจสอบเอกสาร"
            class="reference-check-dialog"
            :style="{ width: 'min(1120px, 96vw)', height: 'min(760px, 90vh)' }"
            :breakpoints="{ '640px': '100vw' }"
        >
            <DocumentReferenceCheck
                v-if="referenceDialogVisible"
                compact
                :document="document"
                :loader="referenceCheckLoader"
                :allow-preview="false"
            />
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="referenceDialogVisible = false" />
            </template>
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

.history-focus-workspace {
    padding-bottom: 0.65rem;
}

.admin-workspace {
    min-height: calc(100dvh - 8rem);
    padding: 0;
    padding-bottom: 0;
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

.admin-workspace .signing-header {
    min-height: 56px;
    padding: 0.75rem 1rem;
}

.header-status {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
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

.loading-state {
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
    padding: 0.65rem;
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

.signature-heading-text {
    min-width: 0;
    display: grid;
    gap: 0.15rem;
    line-height: 1.2;
}

.signature-heading-text strong,
.signature-heading-text small {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.signature-heading-text small {
    color: var(--text-color-secondary);
}

.compact-status {
    margin: 0 0 0.55rem;
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

.sticky-buttons.single-action {
    grid-template-columns: 1fr;
}

.sticky-buttons :deep(.p-button) {
    min-height: 44px;
}

.admin-actions {
    display: grid;
    gap: 0.65rem;
    padding-top: 0.25rem;
}

.admin-action-buttons {
    display: grid;
    grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
    gap: 0.65rem;
}

.admin-action-buttons.single-action {
    grid-template-columns: 1fr;
}

.admin-action-buttons :deep(.p-button) {
    min-height: 42px;
}

.history-summary {
    margin: 0;
}

.history-focus-grid {
    grid-template-columns: minmax(0, 1fr);
}

.legal-dialog-text {
    margin: 0;
    line-height: 1.7;
    color: var(--text-color);
}

:global(.reference-check-dialog .p-dialog-content) {
    height: calc(100% - 4.25rem);
    padding-top: 0.75rem;
    overflow: auto;
}

@media (max-width: 920px) {
    .signing-workspace {
        padding: 0;
        padding-bottom: 7.25rem;
        gap: 0.55rem;
    }

    .admin-workspace {
        padding-bottom: 0;
    }

    .signing-header,
    .history-summary {
        margin-inline: 0.55rem;
    }

    .signing-header {
        margin-top: 0.55rem;
    }

    .workspace-grid {
        grid-template-columns: 1fr;
        gap: 0.55rem;
    }

    .pdf-shell {
        height: min(64dvh, 620px);
        min-height: 0;
        border-right: 0;
        border-left: 0;
        border-radius: 0;
        padding: 0.55rem;
    }

    .pdf-viewer {
        padding: 0.75rem;
    }

    .sign-card {
        margin-inline: 0.55rem;
        padding: 0.75rem;
    }

    .readonly-grid .sign-card {
        order: -1;
    }

    .history-focus-grid .pdf-shell {
        height: min(72dvh, 640px);
    }

    .admin-workspace .pdf-shell {
        height: min(68dvh, 620px);
        border-inline: 1px solid var(--surface-border);
        border-radius: 8px;
        margin-inline: 0.55rem;
    }
}

@media (min-width: 921px) {
    .signing-workspace {
        height: calc(100dvh - 56px);
        overflow: hidden;
    }

    .admin-workspace.signing-workspace {
        height: calc(100dvh - 8rem);
    }

    .admin-workspace .workspace-grid {
        grid-template-columns: minmax(0, 1fr) minmax(360px, 420px);
        gap: 1rem;
    }

    .admin-workspace .pdf-shell,
    .admin-workspace .sign-card {
        min-height: 0;
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

@media (max-width: 640px) {
    :global(.reference-check-dialog.p-dialog) {
        width: 100vw !important;
        max-width: 100vw !important;
        height: 100dvh;
        max-height: 100dvh;
        margin: 0;
        border-radius: 0;
    }

    :global(.reference-check-dialog .p-dialog-content) {
        max-height: none;
        height: calc(100dvh - 8.5rem);
    }
}
</style>
