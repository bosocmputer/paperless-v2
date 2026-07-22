<script setup>
import { api } from '@/services/api';
import { formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import DocumentWorkflowTimeline from '@/views/signing/components/DocumentWorkflowTimeline.vue';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();
const loading = ref(false);
const timelineDialog = ref(false);
const timelineLoading = ref(false);
const timelineDocument = ref(null);
const timelineEvents = ref([]);
const timelineError = ref('');

const emptyTotals = {
    total: 0,
    draft: 0,
    inProgress: 0,
    pendingConfirm: 0,
    rejected: 0,
    completed: 0,
    completedEvidenceFailed: 0,
    completedImageFailed: 0,
    completedLockFailed: 0,
    cancelled: 0
};
const emptyWorkflowSummary = {
    pendingDocuments: 0,
    pendingSigners: 0,
    pendingConfirm: 0,
    attentionDocuments: 0,
    completedDocuments: 0,
    evidenceFailed: 0,
    imageFailed: 0,
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
const timelineRecentEvents = computed(() => timelineEvents.value.slice(0, 5));
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
            currentPositionName: problemReason(doc.status, doc),
            statusLabel: signingStatusLabel(doc.status),
            statusSeverity: signingStatusSeverity(doc.status),
            helper: attentionHelper(doc),
            priority: 1
        });
    });
    pendingDocuments.value.forEach((doc) => {
        const step = pendingByPosition.value.find((item) => item.positionName === doc.currentPositionName);
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
            helper: pendingDocumentHelper(doc, step),
            priority: 2
        });
    });
    return rows.sort((a, b) => a.priority - b.priority || new Date(b.updatedAt || 0) - new Date(a.updatedAt || 0));
});
const metricCards = computed(() => [
    {
        label: 'เตรียมส่ง',
        value: totals.value.draft,
        helperStrong: 'ยังไม่ส่ง',
        helper: '· ให้ผู้เซ็น',
        icon: 'pi pi-file-plus',
        accentClass: 'bg-linear-to-b from-slate-400 dark:from-slate-300 to-slate-600 dark:to-slate-500'
    },
    {
        label: 'รอลายเซ็น',
        value: workflowSummary.value.pendingDocuments || totals.value.inProgress,
        helperStrong: `${workflowSummary.value.pendingSigners || 0} คน`,
        helper: '· ต้องเซ็น',
        icon: 'pi pi-clock',
        accentClass: 'bg-linear-to-b from-cyan-400 dark:from-cyan-300 to-cyan-600 dark:to-cyan-500'
    },
    {
        label: 'รอยืนยัน',
        value: workflowSummary.value.pendingConfirm || totals.value.pendingConfirm,
        helperStrong: `${workflowSummary.value.evidenceFailed} PDF`,
        helper: `· ${workflowSummary.value.imageFailed + workflowSummary.value.lockFailed} SML ต้องแก้`,
        icon: 'pi pi-check-circle',
        accentClass: 'bg-linear-to-b from-orange-400 dark:from-orange-300 to-orange-600 dark:to-orange-500'
    },
    {
        label: 'เสร็จสมบูรณ์',
        value: workflowSummary.value.completedDocuments || totals.value.completed,
        helperStrong: 'พร้อมพิมพ์',
        helper: '· และตรวจย้อนหลัง',
        icon: 'pi pi-check-circle',
        accentClass: 'bg-linear-to-b from-emerald-400 dark:from-emerald-300 to-emerald-600 dark:to-emerald-500'
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

async function openTimeline(doc) {
    timelineDialog.value = true;
    timelineLoading.value = true;
    timelineDocument.value = doc;
    timelineEvents.value = [];
    timelineError.value = '';
    try {
        const result = await api.getSigningDocument(doc.id);
        timelineDocument.value = result.document || doc;
        timelineEvents.value = (result.document?.events || [])
            .map((event) => ({ ...event, view: movementEventView(event) }))
            .filter((event) => event.view);
    } catch (err) {
        timelineError.value = err?.message || 'โหลดความคืบหน้าไม่สำเร็จ';
        toast.add({ severity: 'error', summary: 'โหลดความคืบหน้าไม่สำเร็จ', detail: timelineError.value, life: 3500 });
    } finally {
        timelineLoading.value = false;
    }
}

function documentLine(doc) {
    return `${doc.docNo || '-'} ~ ${doc.docFormatCode || '-'} · ${doc.partyName || doc.partyCode || '-'}`;
}

function isInternalDocument(doc) {
    return String(doc?.documentSource || doc?.document_source || '').toLowerCase() === 'internal';
}

function attentionHelper(doc) {
    if (isInternalDocument(doc)) {
        return doc.status === 'pending_confirm' || doc.status === 'auto_confirming' ? 'เซ็นครบแล้ว ระบบกำลังสร้างเอกสารฉบับสมบูรณ์' : 'เปิดเอกสารเพื่อลองสร้าง PDF หรือหลักฐานอีกครั้ง';
    }
    return doc.status === 'pending_confirm' || doc.status === 'auto_confirming' ? 'เซ็นครบแล้ว ระบบกำลังส่งเข้า SML อัตโนมัติ' : 'เปิดเอกสารเพื่อลองสร้างหลักฐาน ส่งรูป หรือส่งสถานะกลับ SML อีกครั้ง';
}

function problemReason(status, doc) {
    if (status === 'pending_confirm') return isInternalDocument(doc) ? 'รอสร้างเอกสารฉบับสมบูรณ์' : 'รอระบบส่งเข้า SML';
    if (status === 'auto_confirming') return isInternalDocument(doc) ? 'กำลังสร้างเอกสารฉบับสมบูรณ์' : 'กำลังส่งเข้า SML';
    if (status === 'completed_evidence_failed') return 'สร้างไฟล์หลักฐานไม่สำเร็จ';
    if (status === 'completed_image_failed') return isInternalDocument(doc) ? 'สร้างเอกสารฉบับสมบูรณ์ไม่สำเร็จ' : 'ส่งรูปเอกสารเข้า SML ไม่สำเร็จ';
    if (status === 'completed_lock_failed') return isInternalDocument(doc) ? 'ปิดงานเอกสารไม่สำเร็จ' : 'ส่งสถานะกลับ SML ไม่สำเร็จ';
    return 'ต้องตรวจสอบ';
}

function conditionLabel(value) {
    if (Number(value) === 1) return 'ใครเซ็นก่อนก็ผ่าน';
    if (Number(value) === 2) return 'ต้องเซ็นครบทุกคน';
    if (Number(value) === 3) return 'ผู้เซ็นภายนอก';
    return `เงื่อนไข ${value}`;
}

function conditionSeverity(value) {
    if (Number(value) === 1) return 'info';
    if (Number(value) === 2) return 'warn';
    return 'secondary';
}

function pendingDocumentHelper(doc, step) {
    const signerCount = Number(doc.pendingSignerCount || step?.signerCount || 0);
    if (Number(step?.conditionType) === 1) return `มีผู้มีสิทธิ์เซ็น ${signerCount} คน, ใครเซ็นก่อนก็ผ่าน`;
    if (Number(step?.conditionType) === 2) return `ต้องรอลายเซ็นครบ ${signerCount} คน`;
    if (Number(step?.conditionType) === 3) return 'รอลายเซ็นจากบุคคลภายนอก';
    return `${signerCount} คนต้องเซ็นในขั้นตอนนี้`;
}

function pendingPositionTitle(item) {
    return `${item.positionName}`;
}

function pendingPositionSummary(item) {
    return `${item.documentCount} เอกสารอยู่ในขั้นตอนนี้`;
}

function pendingPositionRule(item) {
    if (Number(item.conditionType) === 1) return `วิธีผ่าน: มีผู้มีสิทธิ์เซ็น ${item.signerCount} คน, ใครเซ็นก่อนก็ผ่าน`;
    if (Number(item.conditionType) === 2) return `วิธีผ่าน: ต้องรอลายเซ็นครบ ${item.signerCount} คน`;
    if (Number(item.conditionType) === 3) return 'วิธีผ่าน: รอลายเซ็นจากบุคคลภายนอก';
    return `วิธีผ่าน: ${item.signerCount} คนต้องเซ็น`;
}

function pendingPositionExamples(item) {
    return pendingDocuments.value
        .filter((doc) => doc.currentPositionName === item.positionName)
        .slice(0, 3)
        .map((doc) => doc.docNo)
        .filter(Boolean);
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
            detail: 'สร้าง PDF พร้อมลายเซ็นและหน้าหลักฐานแล้ว'
        },
        final_pdf_failed: {
            title: 'PDF หลักฐานไม่สำเร็จ',
            icon: 'pi pi-file-excel',
            severity: 'danger',
            detail: 'ต้องสร้าง PDF อีกครั้งก่อนส่งสถานะกลับ SML หรือพิมพ์เอกสาร'
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
            title: 'ส่งสถานะกลับ SML สำเร็จ',
            icon: 'pi pi-lock',
            severity: 'success',
            detail: 'อัปเดตเอกสารกลับไปที่ SML แล้ว'
        },
        sml_lock_failed: {
            title: 'ส่งสถานะกลับ SML ไม่สำเร็จ',
            icon: 'pi pi-exclamation-triangle',
            severity: 'danger',
            detail: 'เอกสารเซ็นครบแล้ว แต่ยังต้อง retry ส่งสถานะกลับ SML'
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
            detail: `สร้างสำเนาสำหรับพิมพ์${metadata.printerName ? ` (${metadata.printerName})` : ''}`
        }
    };
    return labels[action] || null;
}
</script>

<template>
    <section class="flex flex-col gap-4">
        <div class="card dashboard-header-card">
            <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div class="min-w-0 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <div class="font-semibold text-xl whitespace-nowrap truncate">ภาพรวมงานเซ็นเอกสาร</div>
                    <p class="text-muted-color m-0 min-w-0 truncate">ติดตามเอกสารที่กำลังรอลายเซ็นและงานที่ต้องแก้</p>
                </div>
            </div>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <div v-for="item in metricCards" :key="item.label" class="bg-surface-0 dark:bg-surface-900 shadow-sm p-5 rounded-2xl">
                <div class="flex justify-between gap-4">
                    <div class="flex flex-col gap-2 min-w-0">
                        <span class="text-surface-700 dark:text-surface-300 font-normal leading-tight truncate">{{ item.label }}</span>
                        <div class="text-surface-900 dark:text-surface-0 font-semibold text-2xl leading-tight">{{ item.value }}</div>
                    </div>
                    <div class="flex items-center justify-center rounded-lg w-10 h-10 flex-none" :class="item.accentClass">
                        <i :class="item.icon" class="text-surface-0 dark:text-surface-900 text-xl leading-none" />
                    </div>
                </div>
                <div class="mt-4">
                    <span class="text-surface-600 dark:text-surface-300 font-medium leading-tight">{{ item.helperStrong }}</span>
                    <span class="ml-1 text-surface-500 dark:text-surface-300 leading-tight">{{ item.helper }}</span>
                </div>
            </div>
        </div>

        <div class="grid grid-cols-12 gap-4 items-start">
            <div class="col-span-12">
                <div class="card dashboard-stack-card">
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
                        <Column header="จัดการ" style="width: 13rem">
                            <template #body="{ data }">
                                <div class="flex gap-2">
                                    <Button label="ความคืบหน้า" icon="pi pi-sitemap" size="small" severity="secondary" outlined @click="openTimeline(data)" />
                                    <Button label="เปิด" icon="pi pi-arrow-right" iconPos="right" size="small" outlined @click="openDocument(data)" />
                                </div>
                            </template>
                        </Column>
                    </DataTable>
                </div>
            </div>

            <div class="col-span-12 grid grid-cols-12 gap-4 items-start">
                <div class="card dashboard-stack-card col-span-12 xl:col-span-6">
                    <div class="flex items-start justify-between gap-3 mb-4">
                        <div>
                            <div class="font-semibold text-lg">สรุปคิวรอลายเซ็น</div>
                            <p class="text-muted-color m-0">ภาพรวมตามขั้นตอน ส่วนรายเอกสารดูจากตารางหลัก</p>
                        </div>
                        <Tag :value="`${pendingByPosition.length} ขั้นตอนมีงานค้าง`" severity="secondary" />
                    </div>
                    <div v-if="pendingByPosition.length === 0" class="empty-panel">
                        <i class="pi pi-inbox"></i>
                        <span>ไม่มีเอกสารค้างรอลายเซ็น</span>
                    </div>
                    <div v-else class="flex flex-col gap-2">
                        <div v-for="item in pendingByPosition" :key="`${item.positionCode}-${item.conditionType}`" class="surface-row">
                            <div class="min-w-0">
                                <div class="font-medium truncate">ขั้นตอน {{ item.positionCode }}: {{ pendingPositionTitle(item) }}</div>
                                <small class="text-muted-color">{{ pendingPositionSummary(item) }}</small>
                                <small class="text-muted-color block">{{ pendingPositionRule(item) }}</small>
                                <small v-if="pendingPositionExamples(item).length" class="text-muted-color block">ตัวอย่างเอกสาร: {{ pendingPositionExamples(item).join(', ') }}</small>
                            </div>
                            <Tag :value="conditionLabel(item.conditionType)" :severity="conditionSeverity(item.conditionType)" />
                        </div>
                    </div>
                </div>

                <div class="card dashboard-stack-card col-span-12 xl:col-span-6">
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
                        <button v-for="doc in recentDocuments" :key="doc.id" type="button" class="surface-row surface-button" @click="openTimeline(doc)">
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

        <Dialog v-model:visible="timelineDialog" modal :header="timelineDocument ? `ความคืบหน้า: ${timelineDocument.docNo || '-'}` : 'ความคืบหน้า'" :style="{ width: 'min(52rem, 94vw)' }">
            <div class="flex flex-col gap-4">
                <div v-if="timelineDocument" class="flex flex-col gap-1">
                    <div class="font-semibold">{{ documentLine(timelineDocument) }}</div>
                    <small class="text-muted-color">ดูว่าเอกสารผ่านขั้นตอนไหนแล้ว และตอนนี้ค้างที่ใคร</small>
                </div>

                <div v-if="timelineLoading" class="empty-panel">
                    <i class="pi pi-spin pi-spinner"></i>
                    <span>กำลังโหลดความคืบหน้า</span>
                </div>
                <Message v-else-if="timelineError" severity="error">
                    {{ timelineError }}
                    <div class="mt-3">
                        <Button v-if="timelineDocument?.id" label="เปิดเอกสาร" icon="pi pi-arrow-right" iconPos="right" severity="secondary" outlined @click="openDocument(timelineDocument)" />
                    </div>
                </Message>
                <template v-else>
                    <DocumentWorkflowTimeline :document="timelineDocument" />

                    <Divider />

                    <div class="flex items-start justify-between gap-3">
                        <div>
                            <div class="font-semibold">เหตุการณ์ล่าสุด</div>
                            <small class="text-muted-color">แสดงเฉพาะ audit log สำคัญล่าสุด</small>
                        </div>
                        <Tag :value="`${timelineEvents.length} รายการ`" severity="secondary" />
                    </div>
                    <div v-if="timelineRecentEvents.length === 0" class="empty-panel">
                        <i class="pi pi-inbox"></i>
                        <span>ยังไม่มีเหตุการณ์สำคัญ</span>
                    </div>
                    <Timeline v-else :value="timelineRecentEvents" align="left" class="dashboard-timeline">
                        <template #opposite="{ item }">
                            <div class="timeline-time">{{ formatThaiDateTime(item.createdAt) }}</div>
                        </template>
                        <template #marker="{ item }">
                            <span class="timeline-marker" :class="`timeline-${item.view.severity}`">
                                <i :class="item.view.icon"></i>
                            </span>
                        </template>
                        <template #content="{ item }">
                            <div class="timeline-content">
                                <strong>{{ item.view.title }}</strong>
                                <span>{{ item.view.detail }}</span>
                                <small v-if="item.actorLabel" class="text-muted-color">โดย {{ item.actorLabel }}</small>
                            </div>
                        </template>
                    </Timeline>
                </template>

                <div class="flex justify-end gap-2">
                    <Button label="ปิด" severity="secondary" outlined @click="timelineDialog = false" />
                    <Button v-if="timelineDocument?.id" label="เปิดเอกสาร" icon="pi pi-arrow-right" iconPos="right" @click="openDocument(timelineDocument)" />
                </div>
            </div>
        </Dialog>
    </section>
</template>

<style scoped>
.dashboard-header-card {
    margin-bottom: 0;
}

.dashboard-stack-card {
    margin-bottom: 0;
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

.timeline-time {
    min-width: 7.5rem;
    padding-top: 0.15rem;
    text-align: right;
    font-size: 0.85rem;
    color: var(--text-color-secondary);
}

.timeline-marker {
    width: 1.65rem;
    height: 1.65rem;
    border-radius: 999px;
    display: inline-grid;
    place-items: center;
    border: 2px solid var(--surface-card);
    font-size: 0.78rem;
}

.timeline-content {
    display: grid;
    gap: 0.2rem;
    min-width: 0;
    padding: 0 0 1.25rem 0.35rem;
}

.dashboard-timeline :deep(.p-timeline-event-opposite) {
    flex: 0 0 8.25rem;
    padding: 0 0.75rem 0 0;
}

.dashboard-timeline :deep(.p-timeline-event-content) {
    padding-left: 0.75rem;
}

.dashboard-timeline :deep(.p-timeline-event-marker) {
    border: 0;
}

.timeline-info {
    color: var(--blue-700, #1d4ed8);
    background: var(--blue-100, #dbeafe);
}

.timeline-success {
    color: var(--green-700, #15803d);
    background: var(--green-100, #dcfce7);
}

.timeline-danger {
    color: var(--red-700, #b91c1c);
    background: var(--red-100, #fee2e2);
}

.timeline-warn {
    color: var(--yellow-800, #854d0e);
    background: var(--yellow-100, #fef9c3);
}

@media (max-width: 640px) {
    .surface-row {
        align-items: flex-start;
        flex-direction: column;
    }

    .dashboard-timeline :deep(.p-timeline-event-opposite) {
        display: block;
        flex: 0 0 5.5rem;
        padding-right: 0.5rem;
    }

    .timeline-time {
        min-width: 0;
        overflow-wrap: anywhere;
        font-size: 0.78rem;
    }
}
</style>
