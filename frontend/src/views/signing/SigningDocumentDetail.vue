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
            <Button v-if="document?.status === 'completed_lock_failed'" label="Retry SML Lock" icon="pi pi-refresh" severity="danger" outlined :loading="retryingLock" @click="retryLock" />
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
                    <div class="block-title">Movement Log</div>
                    <Timeline :value="document?.events || []" align="left" class="compact-timeline">
                        <template #content="{ item }">
                            <div class="event-line">
                                <strong>{{ item.action }}</strong>
                                <span>{{ item.message }}</span>
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
.signer-actions {
    display: flex;
    align-items: center;
    gap: 0.35rem;
}
.event-line {
    display: grid;
    gap: 0.15rem;
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
