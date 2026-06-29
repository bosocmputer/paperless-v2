<script setup>
import { api } from '@/services/api';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();

const document = ref(null);
const loading = ref(false);
const pdfUrl = ref('');
const retryingLock = ref(false);
const retryingFinalPDF = ref(false);
const printing = ref(false);
const tokenDialog = ref(false);
const generatedToken = ref(null);

const signersByStep = computed(() => {
    const groups = new Map();
    (document.value?.steps || []).forEach((step) => groups.set(step.id, { step, signers: [] }));
    (document.value?.signers || []).forEach((signer) => {
        if (!groups.has(signer.stepId)) groups.set(signer.stepId, { step: signer, signers: [] });
        groups.get(signer.stepId).signers.push(signer);
    });
    return [...groups.values()].sort((a, b) => Number(a.step.sequenceNo || 0) - Number(b.step.sequenceNo || 0));
});
const importantEvents = computed(() =>
    (document.value?.events || [])
        .map((event) => ({ ...event, view: movementEventView(event) }))
        .filter((event) => event.view)
);
const printEvents = computed(() => document.value?.printEvents || []);

onMounted(loadPage);
onBeforeUnmount(() => {
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
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
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
    const response = await fetch(api.signingDocumentPDFUrl(document.value.id), { headers: api.authHeaders() });
    if (!response.ok) throw new Error('โหลด PDF ไม่สำเร็จ');
    const blob = await response.blob();
    pdfUrl.value = URL.createObjectURL(blob);
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
            summary: result.lockOk ? 'Final PDF และ Lock SML สำเร็จ' : 'Final PDF สำเร็จ แต่ Lock SML ยังไม่สำเร็จ',
            life: 3200
        });
        await loadPage();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'สร้าง Final PDF ไม่สำเร็จ', detail: err.message, life: 4000 });
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

function statusSeverity(status) {
    return {
        pending: 'info',
        waiting: 'secondary',
        signed: 'success',
        completed: 'success',
        completed_evidence_failed: 'warn',
        rejected: 'danger',
        skipped: 'secondary',
        in_progress: 'info',
        completed_lock_failed: 'danger'
    }[status] || 'secondary';
}

function conditionLabel(value) {
    if (Number(value) === 1) return 'คนใดคนหนึ่ง';
    if (Number(value) === 2) return 'ทุกคน';
    return 'บุคคลภายนอก';
}

function movementEventView(event) {
    const action = String(event?.action || '');
    const metadata = event?.metadata || {};
    const labels = {
        document_created: {
            title: 'สร้างเอกสารเพื่อเซ็น',
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
            detail: event.message || 'เอกสารพร้อมสร้าง Final PDF'
        },
        final_pdf_ready: {
            title: 'Final PDF พร้อมหลักฐานแล้ว',
            icon: 'pi pi-file-check',
            severity: 'success',
            detail: 'สร้าง PDF พร้อมลายเซ็นและ Evidence Appendix แล้ว'
        },
        final_pdf_failed: {
            title: 'Final PDF ไม่สำเร็จ',
            icon: 'pi pi-file-excel',
            severity: 'danger',
            detail: 'ต้องกด Retry Final PDF ก่อน lock SML หรือพิมพ์เอกสาร'
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

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
</script>

<template>
    <div class="signing-detail">
        <div class="editor-bar">
            <Button icon="pi pi-arrow-left" text rounded aria-label="กลับ" @click="router.push({ name: 'signing-documents' })" />
            <div class="bar-title">
                <strong>{{ document?.docNo || 'เอกสาร' }}</strong>
                <span>{{ document?.docFormatCode }} · {{ document?.partyName || document?.partyCode || '-' }}</span>
            </div>
            <Tag v-if="document" :value="document.status" :severity="statusSeverity(document.status)" />
            <Button v-if="document?.status === 'completed_evidence_failed'" label="Retry Final PDF" icon="pi pi-file-check" severity="warn" outlined :loading="retryingFinalPDF" @click="retryFinalPDF" />
            <Button v-if="document?.status === 'completed_lock_failed'" label="Retry SML Lock" icon="pi pi-refresh" severity="danger" outlined :loading="retryingLock" @click="retryLock" />
            <Button v-if="document?.status === 'completed'" label="พิมพ์เอกสาร" icon="pi pi-print" severity="primary" :loading="printing" @click="printOfficialCopy" />
            <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadPage" />
        </div>

        <div class="detail-grid">
            <section class="pdf-panel">
                <iframe v-if="pdfUrl" :src="pdfUrl" title="PDF preview"></iframe>
                <div v-else class="empty-pdf">กำลังโหลด PDF...</div>
            </section>

            <aside class="side-panel">
                <div class="info-block">
                    <div class="block-title">ข้อมูลเอกสาร</div>
                    <dl>
                        <dt>วันที่</dt>
                        <dd>{{ document?.docDate || '-' }}</dd>
                        <dt>ยอดเงิน</dt>
                        <dd>{{ Number(document?.totalAmount || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 }) }}</dd>
                        <dt>SML lock</dt>
                        <dd>{{ document?.smlIsLockRecord === 1 ? 'locked ก่อนส่ง' : document?.lockedAt ? 'locked แล้ว' : '-' }}</dd>
                    </dl>
                </div>

                <div class="info-block">
                    <div class="block-title">ขั้นตอนและผู้เซ็น</div>
                    <div v-for="group in signersByStep" :key="group.step.id" class="step-card">
                        <div class="step-head">
                            <strong>{{ group.step.positionCode }} · {{ group.step.positionName }}</strong>
                            <Tag :value="group.step.status" :severity="statusSeverity(group.step.status)" />
                        </div>
                        <small>{{ conditionLabel(group.step.conditionType) }}</small>
                        <div class="signer-list">
                            <div v-for="signer in group.signers" :key="signer.id" class="signer-row">
                                <span>
                                    <strong>{{ signer.signerName || signer.signerUser || 'บุคคลภายนอก' }}</strong>
                                    <small>{{ signer.signerType }} · page {{ signer.pageNo }}</small>
                                </span>
                                <div class="signer-actions">
                                    <Tag :value="signer.status" :severity="statusSeverity(signer.status)" />
                                    <Button v-if="signer.signerType === 'external' && signer.status !== 'signed'" icon="pi pi-key" rounded outlined aria-label="สร้าง OTP" @click="generateExternal(signer)" />
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="info-block">
                    <div class="block-head">
                        <div>
                            <div class="block-title">ประวัติพิมพ์</div>
                            <small>Official print copy</small>
                        </div>
                        <Tag :value="`${printEvents.length} ครั้ง`" severity="secondary" />
                    </div>
                    <div v-if="printEvents.length === 0" class="empty-log">ยังไม่มีการพิมพ์ official copy</div>
                    <div v-else class="print-list">
                        <div v-for="item in printEvents" :key="item.id" class="print-row">
                            <span>
                                <strong>{{ formatDate(item.printedAt) }}</strong>
                                <small>{{ item.channel }} · {{ item.printerName }}</small>
                            </span>
                            <Tag :value="item.file?.sha256 ? item.file.sha256.slice(0, 10) : '-'" severity="secondary" />
                        </div>
                    </div>
                </div>

                <div class="info-block">
                    <div class="block-head">
                        <div>
                            <div class="block-title">Movement Log</div>
                            <small>แสดงเฉพาะเหตุการณ์สำคัญ</small>
                        </div>
                        <Tag :value="`${importantEvents.length} รายการ`" severity="secondary" />
                    </div>
                    <div v-if="importantEvents.length === 0" class="empty-log">ยังไม่มีเหตุการณ์สำคัญ</div>
                    <Timeline v-else :value="importantEvents" align="left" class="compact-timeline">
                        <template #content="{ item }">
                            <div class="event-line">
                                <div class="event-title">
                                    <span class="event-icon" :class="`event-${item.view.severity}`">
                                        <i :class="item.view.icon"></i>
                                    </span>
                                    <strong>{{ item.view.title }}</strong>
                                </div>
                                <span>{{ item.view.detail }}</span>
                                <small>{{ formatDate(item.createdAt) }}</small>
                            </div>
                        </template>
                    </Timeline>
                </div>
            </aside>
        </div>
    </div>

    <Dialog v-model:visible="tokenDialog" modal header="Public link / OTP" :style="{ width: 'min(42rem, 92vw)' }">
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
            <small class="text-muted-color">OTP หมดอายุ {{ formatDate(generatedToken.expiresAt) }}</small>
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
    flex: 1;
    display: grid;
    gap: 0.1rem;
}
.bar-title span,
.signer-row small,
.print-row small,
.event-line small {
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
.pdf-panel iframe {
    width: 100%;
    height: 100%;
    border: 0;
    border-radius: 8px;
}
.empty-pdf {
    height: 100%;
    display: grid;
    place-items: center;
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
dl {
    display: grid;
    grid-template-columns: 7rem 1fr;
    gap: 0.4rem 0.75rem;
    margin: 0;
}
dt {
    color: var(--text-color-secondary);
}
dd {
    margin: 0;
}
.step-card {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.75rem;
    display: grid;
    gap: 0.5rem;
}
.step-head,
.signer-row,
.copy-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
}
.signer-list {
    display: grid;
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
.signer-actions {
    display: flex;
    align-items: center;
    gap: 0.35rem;
}
.event-line {
    display: grid;
    gap: 0.2rem;
    padding-bottom: 0.2rem;
}
.event-title {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
}
.event-icon {
    width: 1.75rem;
    height: 1.75rem;
    border-radius: 999px;
    display: inline-grid;
    place-items: center;
    font-size: 0.82rem;
    flex: 0 0 auto;
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
</style>
