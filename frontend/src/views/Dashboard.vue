<script setup>
import { api } from '@/services/api';
import { formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();
const loading = ref(false);

const emptyTotals = {
    total: 0,
    draft: 0,
    inProgress: 0,
    rejected: 0,
    completed: 0,
    completedEvidenceFailed: 0,
    completedLockFailed: 0,
    cancelled: 0
};
const emptyWorkflowSummary = {
    pendingDocuments: 0,
    pendingSigners: 0,
    attentionDocuments: 0,
    completedDocuments: 0,
    evidenceFailed: 0,
    lockFailed: 0
};
const dashboard = ref({
    totals: { ...emptyTotals },
    workflowSummary: { ...emptyWorkflowSummary },
    pendingByPosition: [],
    pendingDocuments: [],
    recentDocuments: [],
    needsAttention: []
});

const totals = computed(() => ({ ...emptyTotals, ...(dashboard.value.totals || {}) }));
const workflowSummary = computed(() => ({ ...emptyWorkflowSummary, ...(dashboard.value.workflowSummary || {}) }));
const needsAttention = computed(() => dashboard.value.needsAttention || []);
const pendingByPosition = computed(() => dashboard.value.pendingByPosition || []);
const pendingDocuments = computed(() => dashboard.value.pendingDocuments || []);
const recentDocuments = computed(() => dashboard.value.recentDocuments || []);
const metricCards = computed(() => [
    {
        label: 'รอเซ็น',
        value: workflowSummary.value.pendingDocuments || totals.value.inProgress,
        helper: `${workflowSummary.value.pendingSigners || 0} ผู้เซ็นที่ต้องดำเนินการ`,
        icon: 'pi pi-clock',
        severity: 'info'
    },
    {
        label: 'ผู้เซ็นที่ต้องดำเนินการ',
        value: workflowSummary.value.pendingSigners,
        helper: 'นับเฉพาะ task ที่ถึงลำดับเซ็น',
        icon: 'pi pi-users',
        severity: 'info'
    },
    {
        label: 'ต้องตรวจสอบ',
        value: workflowSummary.value.attentionDocuments || workflowSummary.value.evidenceFailed + workflowSummary.value.lockFailed,
        helper: `${workflowSummary.value.evidenceFailed} PDF, ${workflowSummary.value.lockFailed} SML`,
        icon: 'pi pi-exclamation-triangle',
        severity: workflowSummary.value.attentionDocuments ? 'warn' : 'success'
    },
    {
        label: 'เสร็จสมบูรณ์',
        value: workflowSummary.value.completedDocuments || totals.value.completed,
        helper: 'สร้างหลักฐานและ lock SML สำเร็จ',
        icon: 'pi pi-check-circle',
        severity: 'success'
    }
]);

onMounted(loadDashboard);

async function loadDashboard() {
    loading.value = true;
    try {
        const result = await api.getAdminDashboard();
        dashboard.value = {
            totals: { ...emptyTotals, ...(result.totals || {}) },
            workflowSummary: { ...emptyWorkflowSummary, ...(result.workflowSummary || {}) },
            pendingByPosition: result.pendingByPosition || [],
            pendingDocuments: result.pendingDocuments || [],
            recentDocuments: result.recentDocuments || [],
            needsAttention: result.needsAttention || []
        };
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดภาพรวมไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openDocument(doc) {
    router.push({ name: 'signing-document-detail', params: { id: doc.id } });
}

function documentLine(doc) {
    return `${doc.docNo || '-'} ~ ${doc.docFormatCode || '-'} · ${doc.partyName || doc.partyCode || '-'}`;
}

function conditionLabel(value) {
    if (Number(value) === 1) return 'คนใดคนหนึ่ง';
    if (Number(value) === 2) return 'ทุกคน';
    if (Number(value) === 3) return 'บุคคลภายนอก';
    return `เงื่อนไข ${value}`;
}

function conditionSeverity(value) {
    if (Number(value) === 1) return 'info';
    if (Number(value) === 2) return 'warn';
    return 'secondary';
}
</script>

<template>
    <section class="admin-dashboard">
        <div class="card">
            <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div class="min-w-0 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <div class="font-semibold text-xl whitespace-nowrap truncate">ภาพรวม</div>
                    <p class="text-muted-color m-0 min-w-0 truncate">ติดตามเอกสารรอเซ็น งานติดปัญหา และเอกสารล่าสุด</p>
                </div>
                <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                    <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadDashboard" />
                    <Button label="ส่งเอกสารใหม่" icon="pi pi-send" @click="router.push({ name: 'signing-document-new' })" />
                </div>
            </div>
        </div>

        <div class="metric-grid">
            <div v-for="item in metricCards" :key="item.label" class="card metric-card">
                <span class="metric-icon" :class="`metric-${item.severity}`"><i :class="item.icon"></i></span>
                <div class="min-w-0">
                    <div class="metric-value">{{ item.value }}</div>
                    <div class="font-medium truncate">{{ item.label }}</div>
                    <small class="text-muted-color">{{ item.helper }}</small>
                </div>
            </div>
        </div>

        <div class="dashboard-grid">
            <section class="card dashboard-panel attention-panel">
                <div class="panel-head">
                    <div>
                        <div class="font-semibold text-lg">งานที่ต้องตรวจสอบ</div>
                        <p class="text-muted-color m-0">PDF หลักฐานหรือ SML lock ที่ต้องแก้</p>
                    </div>
                    <Tag :value="`${needsAttention.length} รายการ`" :severity="needsAttention.length ? 'warn' : 'success'" />
                </div>
                <div v-if="loading" class="empty-panel">
                    <i class="pi pi-spin pi-spinner"></i>
                    <span>กำลังโหลด</span>
                </div>
                <div v-else-if="needsAttention.length === 0" class="empty-panel">
                    <i class="pi pi-check-circle"></i>
                    <span>ไม่มีงานค้างที่ต้องแก้ตอนนี้</span>
                </div>
                <button v-for="doc in needsAttention" v-else :key="doc.id" type="button" class="doc-row" @click="openDocument(doc)">
                    <span>
                        <strong>{{ documentLine(doc) }}</strong>
                        <small>{{ formatThaiDateTime(doc.updatedAt) }}</small>
                    </span>
                    <Tag :value="signingStatusLabel(doc.status)" :severity="signingStatusSeverity(doc.status)" />
                </button>
            </section>

            <section class="card dashboard-panel">
                <div class="panel-head">
                    <div>
                        <div class="font-semibold text-lg">รอเซ็นตามขั้นตอน</div>
                        <p class="text-muted-color m-0">ตำแหน่งที่มี task ถึงลำดับเซ็น</p>
                    </div>
                    <Tag :value="`${pendingByPosition.length} ขั้นตอน`" severity="secondary" />
                </div>
                <div v-if="pendingByPosition.length === 0" class="empty-panel">
                    <i class="pi pi-inbox"></i>
                    <span>ไม่มีขั้นตอนที่รอเซ็น</span>
                </div>
                <div v-else class="position-list">
                    <div v-for="item in pendingByPosition" :key="`${item.positionCode}-${item.conditionType}`" class="position-row">
                        <span>
                            <strong>{{ item.positionCode }} · {{ item.positionName }}</strong>
                            <small>{{ item.documentCount }} เอกสาร · {{ item.signerCount }} ผู้เซ็น</small>
                        </span>
                        <Tag :value="conditionLabel(item.conditionType)" :severity="conditionSeverity(item.conditionType)" />
                    </div>
                </div>
            </section>

            <section class="card dashboard-panel">
                <div class="panel-head">
                    <div>
                        <div class="font-semibold text-lg">เอกสารรอเซ็นล่าสุด</div>
                        <p class="text-muted-color m-0">เอกสารที่มีผู้เซ็น pending อยู่ตอนนี้</p>
                    </div>
                    <Button label="ดูทั้งหมด" text icon="pi pi-arrow-right" iconPos="right" @click="router.push({ name: 'signing-documents' })" />
                </div>
                <div v-if="pendingDocuments.length === 0" class="empty-panel">
                    <i class="pi pi-check"></i>
                    <span>ไม่มีเอกสารรอเซ็น</span>
                </div>
                <button v-for="doc in pendingDocuments" v-else :key="doc.id" type="button" class="doc-row" @click="openDocument(doc)">
                    <span>
                        <strong>{{ documentLine(doc) }}</strong>
                        <small>{{ doc.currentPositionName || '-' }} · {{ doc.pendingSignerCount }} ผู้เซ็น</small>
                    </span>
                    <small class="text-muted-color">{{ formatThaiDateTime(doc.updatedAt) }}</small>
                </button>
            </section>

            <section class="card dashboard-panel">
                <div class="panel-head">
                    <div>
                        <div class="font-semibold text-lg">เอกสารอัปเดตล่าสุด</div>
                        <p class="text-muted-color m-0">รายการที่มีความเคลื่อนไหวล่าสุด</p>
                    </div>
                    <Tag :value="`${recentDocuments.length} รายการ`" severity="secondary" />
                </div>
                <div v-if="recentDocuments.length === 0" class="empty-panel">
                    <i class="pi pi-inbox"></i>
                    <span>ยังไม่มีเอกสารเซ็น</span>
                </div>
                <button v-for="doc in recentDocuments" v-else :key="doc.id" type="button" class="doc-row" @click="openDocument(doc)">
                    <span>
                        <strong>{{ documentLine(doc) }}</strong>
                        <small>{{ formatThaiDateTime(doc.updatedAt) }}</small>
                    </span>
                    <Tag :value="signingStatusLabel(doc.status)" :severity="signingStatusSeverity(doc.status)" />
                </button>
            </section>
        </div>
    </section>
</template>

<style scoped>
.admin-dashboard {
    display: grid;
    gap: 1rem;
}

.metric-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
    gap: 0.75rem;
}

.metric-card {
    display: flex;
    align-items: center;
    gap: 0.85rem;
    min-height: 6.25rem;
}

.metric-icon {
    width: 2.75rem;
    height: 2.75rem;
    border-radius: 8px;
    display: inline-grid;
    place-items: center;
    flex: 0 0 auto;
}

.metric-info {
    color: var(--blue-700, #1d4ed8);
    background: var(--blue-100, #dbeafe);
}

.metric-success {
    color: var(--green-700, #15803d);
    background: var(--green-100, #dcfce7);
}

.metric-warn {
    color: var(--yellow-800, #854d0e);
    background: var(--yellow-100, #fef9c3);
}

.metric-value {
    font-size: 1.8rem;
    font-weight: 700;
    line-height: 1;
}

.dashboard-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
    align-items: start;
}

.dashboard-panel {
    display: grid;
    gap: 0.75rem;
}

.panel-head,
.doc-row,
.position-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
}

.doc-row,
.position-row {
    width: 100%;
    border: 1px solid var(--surface-border);
    background: transparent;
    border-radius: 8px;
    padding: 0.75rem;
    text-align: left;
}

.doc-row {
    cursor: pointer;
}

.doc-row:hover {
    background: var(--surface-hover);
}

.doc-row span,
.position-row span {
    display: grid;
    gap: 0.15rem;
    min-width: 0;
}

.doc-row strong,
.doc-row small,
.position-row strong,
.position-row small {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.position-list {
    display: grid;
    gap: 0.5rem;
}

.attention-panel {
    border-color: color-mix(in srgb, var(--yellow-500, #eab308) 45%, var(--surface-border));
}

.empty-panel {
    min-height: 8rem;
    border: 1px dashed var(--surface-border);
    border-radius: 8px;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.5rem;
    color: var(--text-color-secondary);
    text-align: center;
    padding: 1rem;
}

.empty-panel i {
    font-size: 1.45rem;
    color: var(--primary-color);
}

@media (max-width: 960px) {
    .dashboard-grid {
        grid-template-columns: 1fr;
    }
}

@media (max-width: 640px) {
    .panel-head,
    .doc-row,
    .position-row {
        align-items: flex-start;
        flex-direction: column;
    }
}
</style>
