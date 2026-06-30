<script setup>
import * as pdfjsLib from 'pdfjs-dist';
import pdfWorker from 'pdfjs-dist/build/pdf.worker.mjs?url';
import { api } from '@/services/api';
import { formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import DocumentWorkflowTimeline from '@/views/signing/components/DocumentWorkflowTimeline.vue';
import RelatedDocumentsPanel from '@/views/signing/components/RelatedDocumentsPanel.vue';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;

const route = useRoute();
const router = useRouter();
const toast = useToast();

const document = ref(null);
const loading = ref(false);
const pdfUrl = ref('');
const viewerRef = ref(null);
const canvasRef = ref(null);
const pdfDoc = shallowRef(null);
const pdfLoading = ref(false);
const pdfError = ref('');
const currentPage = ref(1);
const pageCount = ref(0);
const zoom = ref(1);
const fitWidthActive = ref(true);
const retryingLock = ref(false);
const retryingFinalPDF = ref(false);
const printing = ref(false);
const tokenDialog = ref(false);
const generatedToken = ref(null);
const activeTab = ref('progress');
let renderSequence = 0;
let renderTask = null;
let resizeObserver = null;

const importantEvents = computed(() =>
    (document.value?.events || [])
        .map((event) => ({ ...event, view: movementEventView(event) }))
        .filter((event) => event.view)
);
const printEvents = computed(() => document.value?.printEvents || []);
const pageOptions = computed(() => Array.from({ length: pageCount.value }, (_, index) => ({ label: `${index + 1}/${pageCount.value}`, value: index + 1 })));
const pdfMetaLabel = computed(() => (pageCount.value ? `หน้า ${currentPage.value} / ${pageCount.value} · ${Math.round(zoom.value * 100)}%` : 'ยังไม่มี PDF'));
const documentHeaderLine = computed(() => {
    const doc = document.value;
    if (!doc) return 'เอกสาร';
    return `${doc.docNo || 'เอกสาร'} ~ ${doc.docFormatCode || '-'} · ${doc.partyName || doc.partyCode || '-'}`;
});

onMounted(async () => {
    await nextTick();
    setupResizeObserver();
    await loadPage();
});
onBeforeUnmount(() => {
    cleanupPDF();
    if (resizeObserver) resizeObserver.disconnect();
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
});

watch([currentPage, zoom], async () => {
    if (pdfDoc.value) await renderCurrentPage();
});

async function loadPage() {
    loading.value = true;
    try {
        const result = await api.getSigningDocument(route.params.id);
        document.value = result.document;
        await loadPdf();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadPdf() {
    if (!document.value?.id) return;
    cleanupPDF();
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
    pdfLoading.value = true;
    pdfError.value = '';
    try {
        const response = await fetch(api.signingDocumentPDFUrl(document.value.id), { headers: api.authHeaders() });
        if (!response.ok) throw new Error('โหลด PDF ไม่สำเร็จ');
        const blob = await response.blob();
        pdfUrl.value = URL.createObjectURL(blob);
        const task = pdfjsLib.getDocument({ url: pdfUrl.value });
        pdfDoc.value = await task.promise;
        pageCount.value = pdfDoc.value.numPages;
        currentPage.value = 1;
        pdfLoading.value = false;
        await nextTick();
        await fitWidth();
    } catch (err) {
        pdfError.value = err?.message || 'โหลด PDF ไม่สำเร็จ';
        throw err;
    } finally {
        pdfLoading.value = false;
    }
}

function setupResizeObserver() {
    if (!viewerRef.value || !window.ResizeObserver) return;
    resizeObserver = new ResizeObserver(() => {
        if (fitWidthActive.value) void fitWidth();
    });
    resizeObserver.observe(viewerRef.value);
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

function setZoom(value) {
    fitWidthActive.value = false;
    zoom.value = clamp(value, 0.45, 2.5);
}

async function renderCurrentPage() {
    if (!pdfDoc.value || !canvasRef.value) return;
    const sequence = ++renderSequence;
    cancelRenderTask();
    try {
        const page = await pdfDoc.value.getPage(currentPage.value);
        if (sequence !== renderSequence) return;
        const viewport = page.getViewport({ scale: zoom.value });
        const outputScale = Math.min(window.devicePixelRatio || 1, 2);
        const canvas = canvasRef.value;
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
    } finally {
        if (sequence === renderSequence) renderTask = null;
    }
}

function cancelRenderTask() {
    if (!renderTask) return;
    try {
        renderTask.cancel();
    } catch {
        // PDF.js can throw if rendering finished at the same time.
    }
    renderTask = null;
}

function cleanupPDF() {
    cancelRenderTask();
    if (pdfDoc.value?.destroy) pdfDoc.value.destroy().catch(() => {});
    pdfDoc.value = null;
    pageCount.value = 0;
}

async function retryLock() {
    retryingLock.value = true;
    try {
        await api.retrySigningDocumentLock(document.value.id);
        toast.add({ severity: 'success', summary: 'Lock SML สำเร็จ', life: 2500 });
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'Lock SML ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        retryingLock.value = false;
    }
}

async function retryFinalPDF() {
    retryingFinalPDF.value = true;
    try {
        const result = await api.retrySigningDocumentFinalPDF(document.value.id);
        toast.add({
            severity: result.lockOk ? 'success' : 'warn',
            summary: result.lockOk ? 'PDF หลักฐานและ Lock SML สำเร็จ' : 'PDF หลักฐานสำเร็จ แต่ Lock SML ยังไม่สำเร็จ',
            life: 3200
        });
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'สร้าง PDF หลักฐานไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        retryingFinalPDF.value = false;
    }
}

async function printOfficialCopy() {
    if (!document.value?.id) return;
    const popup = window.open('', '_blank');
    printing.value = true;
    try {
        const deviceId = getAdminDeviceId();
        const result = await api.createSigningDocumentPrintCopy(document.value.id, {
            channel: 'web',
            deviceId,
            clientTimezone: Intl.DateTimeFormat().resolvedOptions().timeZone || ''
        });
        const fileUrl = result.fileUrl || api.signingDocumentPrintCopyPDFUrl(document.value.id, result.printCopyId);
        const response = await fetch(fileUrl, { headers: api.authHeaders() });
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

async function generateExternal(signer) {
    try {
        const result = await api.regenerateExternalToken(signer.id);
        generatedToken.value = result.external;
        tokenDialog.value = true;
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'สร้าง public link ไม่สำเร็จ', detail: err.message, life: 4000 });
    }
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
    await navigator.clipboard.writeText(value);
    toast.add({ severity: 'success', summary: 'คัดลอกแล้ว', life: 1800 });
}

function printerLabel(value) {
    if (!value) return '-';
    if (value === 'not_available_web_browser') return 'ไม่สามารถอ่านชื่อเครื่องพิมพ์จาก Web';
    return value;
}

function formatTimelineDate(value) {
    return formatThaiDateTime(value);
}

function loadRelatedDocuments() {
    return api.getSigningDocumentRelatedDocuments(document.value.id);
}

function openRelatedDocument(documentId) {
    if (!documentId || documentId === document.value?.id) return;
    router.push({ name: 'signing-document-detail', params: { id: documentId } });
}

function openDocumentFlow(doc = document.value) {
    if (!doc?.docNo) return;
    router.push({
        name: 'signing-documents',
        query: {
            flow_doc_no: doc.docNo,
            ...(doc.docFormatCode ? { flow_doc_format_code: doc.docFormatCode } : {})
        }
    });
}

function openRelatedFlow(payload) {
    const node = payload?.node;
    if (!node?.doc_no) {
        openDocumentFlow();
        return;
    }
    router.push({
        name: 'signing-documents',
        query: {
            flow_doc_no: node.doc_no,
            ...(node.doc_format_code ? { flow_doc_format_code: node.doc_format_code } : {})
        }
    });
}

function movementEventView(event) {
    const action = String(event?.action || '');
    const metadata = event?.metadata || {};
    const labels = {
        document_created: {
            title: 'สร้างเอกสารเซ็น',
            icon: 'pi pi-send',
            severity: 'info',
            detail: event.message || 'เริ่ม workflow เอกสารนี้'
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

function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
}
</script>

<template>
    <div class="signing-detail">
        <div class="editor-bar">
            <Button icon="pi pi-arrow-left" text rounded aria-label="กลับ" @click="router.push({ name: 'signing-documents' })" />
            <div class="bar-title">
                <strong>{{ documentHeaderLine }}</strong>
            </div>
            <Tag v-if="document" :value="signingStatusLabel(document.status)" :severity="signingStatusSeverity(document.status)" />
            <Button v-if="document" label="ตรวจสอบ Flow" icon="pi pi-sitemap" severity="secondary" outlined @click="openDocumentFlow()" />
            <Button v-if="document?.status === 'completed_evidence_failed'" label="สร้าง PDF อีกครั้ง" icon="pi pi-file-check" severity="warn" outlined :loading="retryingFinalPDF" @click="retryFinalPDF" />
            <Button v-if="document?.status === 'completed_lock_failed'" label="Lock SML อีกครั้ง" icon="pi pi-refresh" severity="danger" outlined :loading="retryingLock" @click="retryLock" />
            <Button v-if="document?.status === 'completed'" label="พิมพ์เอกสาร" icon="pi pi-print" severity="primary" :loading="printing" @click="printOfficialCopy" />
            <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadPage" />
        </div>

        <div class="detail-grid">
            <section class="pdf-panel">
                <div class="pdf-toolbar">
                    <div class="toolbar-group">
                        <Select v-model="currentPage" :options="pageOptions" optionLabel="label" optionValue="value" :disabled="pageOptions.length === 0" class="page-select" />
                        <Button icon="pi pi-search-minus" severity="secondary" outlined :disabled="!pdfDoc || zoom <= 0.45" aria-label="ซูมออก" @click="setZoom(zoom - 0.1)" />
                        <span class="zoom-value">{{ Math.round(zoom * 100) }}%</span>
                        <Button icon="pi pi-search-plus" severity="secondary" outlined :disabled="!pdfDoc || zoom >= 2.5" aria-label="ซูมเข้า" @click="setZoom(zoom + 0.1)" />
                        <Button label="พอดีกว้าง" icon="pi pi-arrows-h" severity="secondary" outlined :disabled="!pdfDoc" @click="fitWidth" />
                        <Button label="100%" severity="secondary" outlined :disabled="!pdfDoc" @click="setZoom(1)" />
                    </div>
                    <div class="toolbar-group right">
                        <span class="pdf-meta">{{ pdfMetaLabel }}</span>
                    </div>
                </div>
                <div ref="viewerRef" class="pdf-scroll">
                    <div v-if="pdfLoading" class="empty-pdf">
                        <i class="pi pi-spin pi-spinner"></i>
                        <span>กำลังโหลด PDF...</span>
                    </div>
                    <Message v-else-if="pdfError" severity="error" class="pdf-error">
                        {{ pdfError }}
                        <div class="mt-3">
                            <Button size="small" label="ลองใหม่" icon="pi pi-refresh" severity="secondary" outlined @click="loadPdf" />
                        </div>
                    </Message>
                    <div v-else-if="pdfDoc" class="pdf-page-shell">
                        <canvas ref="canvasRef" class="pdf-canvas"></canvas>
                    </div>
                    <div v-else class="empty-pdf">ยังไม่มี PDF</div>
                </div>
            </section>

            <aside class="side-panel">
                <Tabs v-model:value="activeTab">
                    <TabList>
                        <Tab value="progress">ความคืบหน้า</Tab>
                        <Tab value="related">เอกสารประกอบ</Tab>
                        <Tab value="print">พิมพ์</Tab>
                        <Tab value="events">เหตุการณ์</Tab>
                    </TabList>
                    <TabPanels>
                        <TabPanel value="progress">
                            <div class="info-block">
                                <div class="block-head">
                                    <div>
                                        <div class="block-title">ความคืบหน้าเอกสาร</div>
                                        <small>แสดงทุกขั้นตอน รวมขั้นตอนที่ยังไม่ถึงคิว</small>
                                    </div>
                                    <Tag v-if="document" :value="signingStatusLabel(document.status)" :severity="signingStatusSeverity(document.status)" />
                                </div>
                                <DocumentWorkflowTimeline :document="document" show-external-actions @generate-external="generateExternal" />
                            </div>
                        </TabPanel>
                        <TabPanel value="related">
                            <div class="info-block">
                                <div class="flex justify-end mb-3">
                                    <Button label="เปิด Flow ในรายการเอกสาร" icon="pi pi-sitemap" severity="secondary" outlined @click="openDocumentFlow()" />
                                </div>
                                <RelatedDocumentsPanel
                                    v-if="activeTab === 'related' && document?.id"
                                    admin
                                    :loader="loadRelatedDocuments"
                                    @preview-pdf="openRelatedFlow"
                                    @open-document="openRelatedDocument"
                                />
                            </div>
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

    <Dialog v-model:visible="tokenDialog" modal header="ลิงก์ภายนอก / OTP" :style="{ width: 'min(42rem, 92vw)' }">
        <div v-if="generatedToken" class="token-box">
            <label>Link</label>
            <div class="copy-row">
                <InputText :modelValue="generatedToken.url" readonly class="w-full" />
                <Button icon="pi pi-copy" rounded outlined aria-label="copy link" @click="copy(generatedToken.url)" />
            </div>
            <label>OTP</label>
            <div class="copy-row">
                <InputText :modelValue="generatedToken.otp" readonly class="w-full otp-text" />
                <Button icon="pi pi-copy" rounded outlined aria-label="copy otp" @click="copy(generatedToken.otp)" />
            </div>
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
    overflow: auto;
    padding: 0.75rem;
    display: grid;
    gap: 0.75rem;
    align-content: start;
}
.info-block {
    display: grid;
    gap: 0.6rem;
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
}
</style>
