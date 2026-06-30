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
const actionRows = computed(() => {
    const rows = [];
    needsAttention.value.forEach((doc) => {
        rows.push({
            key: `attention-${doc.id}`,
            id: doc.id,
            docNo: doc.docNo,
            docFormatCode: doc.docFormatCode,
            partyName: doc.partyName,
            partyCode: doc.partyCode,
            updatedAt: doc.updatedAt,
            currentPositionName: problemReason(doc.status),
            statusLabel: signingStatusLabel(doc.status),
            statusSeverity: signingStatusSeverity(doc.status),
            helper: 'เปิดเอกสารเพื่อลองสร้างหลักฐานหรือส่งสถานะกลับ SML อีกครั้ง',
            priority: 1
        });
    });
    pendingDocuments.value.forEach((doc) => {
        rows.push({
            key: `pending-${doc.id}`,
            id: doc.id,
            docNo: doc.docNo,
            docFormatCode: doc.docFormatCode,
            partyName: doc.partyName,
            partyCode: doc.partyCode,
            updatedAt: doc.updatedAt,
            currentPositionName: doc.currentPositionName || 'รอลายเซ็น',
            pendingSignerCount: doc.pendingSignerCount,
            statusLabel: 'รอลายเซ็น',
            statusSeverity: 'info',
            helper: `${doc.pendingSignerCount || 0} คนต้องเซ็นในขั้นตอนนี้`,
            priority: 2
        });
    });
    return rows.sort((a, b) => a.priority - b.priority || new Date(b.updatedAt || 0) - new Date(a.updatedAt || 0));
});
const metricCards = computed(() => [
    {
        label: 'รอลายเซ็น',
        value: workflowSummary.value.pendingDocuments || totals.value.inProgress,
        helper: `${workflowSummary.value.pendingSigners || 0} คนต้องเซ็น`,
        icon: 'pi pi-clock',
        severity: 'info'
    },
    {
        label: 'มีปัญหาต้องแก้',
        value: workflowSummary.value.attentionDocuments || workflowSummary.value.evidenceFailed + workflowSummary.value.lockFailed,
        helper: `${workflowSummary.value.evidenceFailed} ไฟล์หลักฐาน, ${workflowSummary.value.lockFailed} ส่งกลับ SML`,
        icon: 'pi pi-exclamation-triangle',
        severity: workflowSummary.value.attentionDocuments ? 'warn' : 'success'
    },
    {
        label: 'เสร็จสมบูรณ์',
        value: workflowSummary.value.completedDocuments || totals.value.completed,
        helper: 'พร้อมตรวจย้อนหลังหรือพิมพ์เอกสาร',
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

function problemReason(status) {
    if (status === 'completed_evidence_failed') return 'สร้างไฟล์หลักฐานไม่สำเร็จ';
    if (status === 'completed_lock_failed') return 'ส่งสถานะกลับ SML ไม่สำเร็จ';
    return 'ต้องตรวจสอบ';
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
    <section class="flex flex-col gap-4">
        <div class="card">
            <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div class="min-w-0 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <div class="font-semibold text-xl whitespace-nowrap truncate">ภาพรวมงานเซ็นเอกสาร</div>
                    <p class="text-muted-color m-0 min-w-0 truncate">เริ่มงานใหม่ และติดตามเอกสารที่ต้องดำเนินการ</p>
                </div>
                <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                    <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadDashboard" />
                    <Button label="เอกสารทั้งหมด" icon="pi pi-list" severity="secondary" outlined @click="router.push({ name: 'signing-documents' })" />
                    <Button label="ส่งเอกสารใหม่" icon="pi pi-send" @click="router.push({ name: 'signing-document-new' })" />
                </div>
            </div>
        </div>

        <div class="grid grid-cols-12 gap-4">
            <div v-for="item in metricCards" :key="item.label" class="col-span-12 md:col-span-4">
                <div class="card metric-card">
                    <span class="metric-icon" :class="`metric-${item.severity}`"><i :class="item.icon"></i></span>
                    <div class="min-w-0">
                        <div class="metric-value">{{ item.value }}</div>
                        <div class="font-semibold truncate">{{ item.label }}</div>
                        <small class="text-muted-color">{{ item.helper }}</small>
                    </div>
                </div>
            </div>
        </div>

        <div class="grid grid-cols-12 gap-4 items-start">
            <div class="col-span-12 xl:col-span-8">
                <div class="card">
                    <Toolbar class="mb-4">
                        <template #start>
                            <div>
                                <div class="font-semibold text-lg">เอกสารที่ต้องติดตาม</div>
                                <p class="text-muted-color m-0">รายการที่กำลังรอลายเซ็น หรือมีปัญหาที่ต้องแก้</p>
                            </div>
                        </template>
                        <template #end>
                            <Tag :value="`${actionRows.length} รายการ`" :severity="actionRows.length ? 'info' : 'success'" />
                        </template>
                    </Toolbar>

                    <DataTable :value="actionRows" :loading="loading" dataKey="key" responsiveLayout="scroll" stripedRows>
                        <template #empty>
                            <div class="py-8 text-center text-muted-color">
                                <i class="pi pi-check-circle block mb-3 text-2xl text-primary"></i>
                                ไม่มีเอกสารที่ต้องติดตามตอนนี้
                            </div>
                        </template>
                        <Column header="เอกสาร" sortable sortField="docNo">
                            <template #body="{ data }">
                                <div class="font-medium text-surface-900 dark:text-surface-0 truncate">{{ documentLine(data) }}</div>
                            </template>
                        </Column>
                        <Column header="สถานะ" style="width: 10rem">
                            <template #body="{ data }">
                                <Tag :value="data.statusLabel" :severity="data.statusSeverity" />
                            </template>
                        </Column>
                        <Column header="ตอนนี้อยู่ที่" style="min-width: 14rem">
                            <template #body="{ data }">
                                <div class="font-medium">{{ data.currentPositionName }}</div>
                                <small class="text-muted-color">{{ data.helper }}</small>
                            </template>
                        </Column>
                        <Column header="อัปเดตล่าสุด" style="width: 10rem">
                            <template #body="{ data }">{{ formatThaiDateTime(data.updatedAt) }}</template>
                        </Column>
                        <Column header="จัดการ" style="width: 7rem">
                            <template #body="{ data }">
                                <Button label="เปิด" icon="pi pi-arrow-right" iconPos="right" size="small" outlined @click="openDocument(data)" />
                            </template>
                        </Column>
                    </DataTable>
                </div>
            </div>

            <div class="col-span-12 xl:col-span-4 flex flex-col gap-4">
                <div class="card">
                    <div class="flex items-start justify-between gap-3 mb-4">
                        <div>
                            <div class="font-semibold text-lg">รอตามขั้นตอน</div>
                            <p class="text-muted-color m-0">ดูว่าขั้นตอนไหนมีงานรอลายเซ็น</p>
                        </div>
                        <Tag :value="`${pendingByPosition.length} ขั้นตอน`" severity="secondary" />
                    </div>
                    <div v-if="pendingByPosition.length === 0" class="empty-panel">
                        <i class="pi pi-inbox"></i>
                        <span>ไม่มีขั้นตอนที่รอลายเซ็น</span>
                    </div>
                    <div v-else class="flex flex-col gap-2">
                        <div v-for="item in pendingByPosition" :key="`${item.positionCode}-${item.conditionType}`" class="surface-row">
                            <div class="min-w-0">
                                <div class="font-medium truncate">{{ item.positionCode }} · {{ item.positionName }}</div>
                                <small class="text-muted-color">{{ item.documentCount }} เอกสาร · {{ item.signerCount }} คนต้องเซ็น</small>
                            </div>
                            <Tag :value="conditionLabel(item.conditionType)" :severity="conditionSeverity(item.conditionType)" />
                        </div>
                    </div>
                </div>

                <div class="card">
                    <div class="flex items-start justify-between gap-3 mb-4">
                        <div>
                            <div class="font-semibold text-lg">ความเคลื่อนไหวล่าสุด</div>
                            <p class="text-muted-color m-0">รายการที่มีการเปลี่ยนแปลงล่าสุด</p>
                        </div>
                        <Tag :value="`${recentDocuments.length} รายการ`" severity="secondary" />
                    </div>
                    <div v-if="recentDocuments.length === 0" class="empty-panel">
                        <i class="pi pi-inbox"></i>
                        <span>ยังไม่มีเอกสารเซ็น</span>
                    </div>
                    <div v-else class="flex flex-col gap-2">
                        <button v-for="doc in recentDocuments" :key="doc.id" type="button" class="surface-row surface-button" @click="openDocument(doc)">
                            <div class="min-w-0 text-left">
                                <div class="font-medium truncate">{{ documentLine(doc) }}</div>
                                <small class="text-muted-color">{{ formatThaiDateTime(doc.updatedAt) }}</small>
                            </div>
                            <Tag :value="signingStatusLabel(doc.status)" :severity="signingStatusSeverity(doc.status)" />
                        </button>
                    </div>
                </div>
            </div>
        </div>
    </section>
</template>

<style scoped>
.metric-card {
    min-height: 5.25rem;
    display: flex;
    align-items: center;
    gap: 0.85rem;
}

.metric-icon {
    width: 2.5rem;
    height: 2.5rem;
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
    font-size: 1.65rem;
    font-weight: 700;
    line-height: 1;
}

.surface-row {
    width: 100%;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.75rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    background: transparent;
}

.surface-button {
    cursor: pointer;
    text-align: inherit;
}

.surface-button:hover {
    background: var(--surface-hover);
}

.empty-panel {
    min-height: 7rem;
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

@media (max-width: 640px) {
    .surface-row {
        align-items: flex-start;
        flex-direction: column;
    }
}
</style>
