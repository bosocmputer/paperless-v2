<script setup>
import { api } from '@/services/api';
import { authStore } from '@/stores/auth';
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
const dashboard = ref({
    totals: { ...emptyTotals },
    recentDocuments: [],
    needsAttention: []
});

const statusMap = {
    in_progress: { label: 'รอเซ็น', severity: 'info' },
    completed: { label: 'เสร็จสมบูรณ์', severity: 'success' },
    completed_evidence_failed: { label: 'สร้าง PDF หลักฐานไม่สำเร็จ', severity: 'warn' },
    completed_lock_failed: { label: 'Lock SML ไม่สำเร็จ', severity: 'danger' },
    rejected: { label: 'ถูกปฏิเสธ', severity: 'danger' },
    cancelled: { label: 'ยกเลิก', severity: 'secondary' },
    draft: { label: 'แบบร่าง', severity: 'secondary' }
};

const stats = computed(() => {
    const totals = dashboard.value.totals || emptyTotals;
    return [
        { label: 'เอกสารทั้งหมด', value: totals.total, icon: 'pi pi-file', severity: 'info' },
        { label: 'กำลังรอเซ็น', value: totals.inProgress, icon: 'pi pi-clock', severity: 'info' },
        { label: 'ต้องตรวจสอบ', value: totals.completedEvidenceFailed + totals.completedLockFailed, icon: 'pi pi-exclamation-triangle', severity: 'warn' },
        { label: 'เสร็จสมบูรณ์', value: totals.completed, icon: 'pi pi-check-circle', severity: 'success' }
    ];
});

const needsAttention = computed(() => dashboard.value.needsAttention || []);
const recentDocuments = computed(() => dashboard.value.recentDocuments || []);

onMounted(loadDashboard);

async function loadDashboard() {
    loading.value = true;
    try {
        const result = await api.getAdminDashboard();
        dashboard.value = {
            totals: { ...emptyTotals, ...(result.totals || {}) },
            recentDocuments: result.recentDocuments || [],
            needsAttention: result.needsAttention || []
        };
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดภาพรวมไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function statusLabel(status) {
    return statusMap[status]?.label || status || '-';
}

function statusSeverity(status) {
    return statusMap[status]?.severity || 'secondary';
}

function openDocument(doc) {
    router.push({ name: 'signing-document-detail', params: { id: doc.id } });
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
</script>

<template>
    <section class="admin-dashboard">
        <div class="dashboard-header">
            <div>
                <h1>ภาพรวม</h1>
                <p>ภาพรวมเอกสารเซ็นและงานที่ admin ต้องจัดการ</p>
            </div>
            <div class="dashboard-actions">
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadDashboard" />
                <Button label="ส่งเอกสารใหม่" icon="pi pi-send" @click="router.push({ name: 'signing-document-new' })" />
            </div>
        </div>

        <div class="stat-grid">
            <article v-for="item in stats" :key="item.label" class="stat-card">
                <span class="stat-icon" :class="`stat-${item.severity}`"><i :class="item.icon"></i></span>
                <div>
                    <strong>{{ item.value }}</strong>
                    <span>{{ item.label }}</span>
                </div>
            </article>
        </div>

        <div class="dashboard-grid">
            <section class="card dashboard-panel">
                <div class="panel-head">
                    <div>
                        <h2>ต้องตรวจสอบ</h2>
                        <p>PDF หลักฐานหรือ SML lock ที่ต้องดำเนินการอีกครั้ง</p>
                    </div>
                    <Tag :value="`${needsAttention.length} รายการ`" :severity="needsAttention.length ? 'warn' : 'success'" />
                </div>
                <div v-if="needsAttention.length === 0" class="empty-panel">
                    <i class="pi pi-check-circle"></i>
                    <span>ไม่มีงานค้างที่ต้องแก้ตอนนี้</span>
                </div>
                <button v-for="doc in needsAttention" v-else :key="doc.id" type="button" class="doc-row attention-row" @click="openDocument(doc)">
                    <span>
                        <strong>{{ doc.docNo }}</strong>
                        <small>{{ doc.docFormatCode }} · {{ doc.partyName || doc.partyCode || '-' }}</small>
                    </span>
                    <Tag :value="statusLabel(doc.status)" :severity="statusSeverity(doc.status)" />
                </button>
            </section>

            <section class="card dashboard-panel">
                <div class="panel-head">
                    <div>
                        <h2>เอกสารล่าสุด</h2>
                        <p>รายการที่มีการอัปเดตล่าสุด</p>
                    </div>
                    <Button label="ดูทั้งหมด" text icon="pi pi-arrow-right" iconPos="right" @click="router.push({ name: 'signing-documents' })" />
                </div>
                <div v-if="loading" class="empty-panel">
                    <i class="pi pi-spin pi-spinner"></i>
                    <span>กำลังโหลด</span>
                </div>
                <div v-else-if="recentDocuments.length === 0" class="empty-panel">
                    <i class="pi pi-inbox"></i>
                    <span>ยังไม่มีเอกสารเซ็น</span>
                </div>
                <button v-for="doc in recentDocuments" v-else :key="doc.id" type="button" class="doc-row" @click="openDocument(doc)">
                    <span>
                        <strong>{{ doc.docNo }}</strong>
                        <small>{{ formatDate(doc.updatedAt) }}</small>
                    </span>
                    <Tag :value="statusLabel(doc.status)" :severity="statusSeverity(doc.status)" />
                </button>
            </section>

            <aside class="card dashboard-panel quick-panel">
                <div class="panel-head">
                    <div>
                        <h2>ทางลัด admin</h2>
                        <p>{{ authStore.user?.displayName || authStore.user?.username }}</p>
                    </div>
                </div>
                <Button label="ส่งเอกสารใหม่" icon="pi pi-send" outlined @click="router.push({ name: 'signing-document-new' })" />
                <Button label="เอกสารเซ็น" icon="pi pi-list" outlined @click="router.push({ name: 'signing-documents' })" />
                <Button label="ตั้งค่า Workflow" icon="pi pi-file-edit" outlined @click="router.push({ name: 'document-config' })" />
                <Button label="กรอบเริ่มต้น" icon="pi pi-pencil" outlined @click="router.push({ name: 'signature-templates' })" />
                <Button label="ผู้ใช้งาน" icon="pi pi-users" outlined @click="router.push({ name: 'users' })" />
            </aside>
        </div>
    </section>
</template>

<style scoped>
.admin-dashboard {
    display: grid;
    gap: 1rem;
}

.dashboard-header,
.panel-head,
.doc-row,
.stat-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
}

.dashboard-header h1,
.panel-head h2 {
    margin: 0;
    color: var(--text-color);
}

.dashboard-header h1 {
    font-size: 1.5rem;
}

.dashboard-header p,
.panel-head p,
.doc-row small,
.stat-card span {
    margin: 0.15rem 0 0;
    color: var(--text-color-secondary);
}

.dashboard-actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
    justify-content: flex-end;
}

.stat-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
    gap: 0.75rem;
}

.stat-card {
    min-height: 6rem;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 1rem;
    justify-content: flex-start;
}

.stat-icon {
    width: 2.75rem;
    height: 2.75rem;
    border-radius: 8px;
    display: inline-grid;
    place-items: center;
    flex: 0 0 auto;
}

.stat-info {
    color: var(--blue-700, #1d4ed8);
    background: var(--blue-100, #dbeafe);
}

.stat-success {
    color: var(--green-700, #15803d);
    background: var(--green-100, #dcfce7);
}

.stat-warn {
    color: var(--yellow-800, #854d0e);
    background: var(--yellow-100, #fef9c3);
}

.stat-card strong {
    display: block;
    font-size: 1.7rem;
    line-height: 1;
}

.dashboard-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) minmax(260px, 320px);
    gap: 1rem;
    align-items: start;
}

.dashboard-panel {
    display: grid;
    gap: 0.75rem;
}

.doc-row {
    width: 100%;
    border: 1px solid var(--surface-border);
    background: transparent;
    border-radius: 8px;
    padding: 0.75rem;
    text-align: left;
    cursor: pointer;
}

.doc-row:hover {
    background: var(--surface-hover);
}

.doc-row span {
    min-width: 0;
    display: grid;
    gap: 0.15rem;
}

.doc-row strong,
.doc-row small {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.attention-row {
    background: color-mix(in srgb, var(--yellow-100, #fef9c3) 40%, transparent);
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
    font-size: 1.5rem;
    color: var(--primary-color);
}

.quick-panel :deep(.p-button) {
    justify-content: flex-start;
}

@media (max-width: 1200px) {
    .dashboard-grid {
        grid-template-columns: 1fr 1fr;
    }

    .quick-panel {
        grid-column: 1 / -1;
    }
}

@media (max-width: 760px) {
    .dashboard-header,
    .panel-head {
        align-items: flex-start;
        flex-direction: column;
    }

    .dashboard-grid {
        grid-template-columns: 1fr;
    }

    .dashboard-actions,
    .dashboard-actions :deep(.p-button) {
        width: 100%;
    }
}
</style>
