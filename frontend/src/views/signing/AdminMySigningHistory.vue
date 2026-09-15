<script setup>
import { api } from '@/services/api';
import { formatDocumentDate, formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import DocumentAttachmentActionButton from '@/views/signing/components/DocumentAttachmentActionButton.vue';
import DocumentAttachmentsDialog from '@/views/signing/components/DocumentAttachmentsDialog.vue';
import { useTableRowsPerPage } from '@/composables/useTableRowsPerPage';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const { rowsPerPage: size, rowsPerPageOptions } = useTableRowsPerPage('admin-my-signing-history');
const documents = ref([]);
const page = ref(1);
const total = ref(0);
const loading = ref(false);
const searchQuery = ref('');
const docFormatCodeFilter = ref('');
const departmentCodeFilter = ref('');
const partyCodeFilter = ref('');
const dateRange = ref(null);
const filterOptions = ref({ docFormatCodes: [], departments: [], parties: [] });
const openingTaskId = ref('');
const attachmentsDialog = ref(false);
const attachmentsRow = ref(null);
let searchTimer = null;
let requestSequence = 0;

const docFormatCodeOptions = computed(() => (filterOptions.value.docFormatCodes || []).map((code) => ({ label: code, value: code })));
const departmentOptions = computed(() =>
    (filterOptions.value.departments || []).map((item) => ({ label: item.name ? `${item.name} (${item.code})` : item.code, value: item.code }))
);
const partyOptions = computed(() => (filterOptions.value.parties || []).map((item) => ({ label: item.name || item.code, value: item.code })));
const hasActiveFilters = computed(
    () => Boolean(docFormatCodeFilter.value || departmentCodeFilter.value || partyCodeFilter.value || dateRange.value?.[0] || searchQuery.value)
);

const firstRow = computed(() => Math.max(0, (Number(page.value || 1) - 1) * Number(size.value || 10)));
const attachmentsDialogTitle = computed(() => {
    const docNo = attachmentsRow.value?.docNo || '';
    return docNo ? `ไฟล์แนบอ้างอิง · ${docNo}` : 'ไฟล์แนบอ้างอิง';
});
const attachmentsDialogSubtitle = computed(() => {
    const row = attachmentsRow.value || {};
    const parts = [row.docFormatCode, partyLine(row), formatDocumentDate(row.docDate)].filter((part) => part && part !== '-');
    return parts.join(' · ');
});
const attachmentsDialogKey = computed(() => attachmentsRow.value?.taskId || '');

function filterParams() {
    return {
        docFormatCode: docFormatCodeFilter.value,
        departmentCode: departmentCodeFilter.value,
        partyCode: partyCodeFilter.value,
        dateFrom: formatDateForApi(dateRange.value?.[0]),
        dateTo: formatDateForApi(dateRange.value?.[1])
    };
}

function formatDateForApi(date) {
    if (!(date instanceof Date) || Number.isNaN(date.getTime())) return '';
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${date.getFullYear()}-${month}-${day}`;
}

function clearFilters() {
    searchQuery.value = '';
    docFormatCodeFilter.value = '';
    departmentCodeFilter.value = '';
    partyCodeFilter.value = '';
    dateRange.value = null;
    loadHistory(1);
}

async function loadFilterOptions() {
    try {
        filterOptions.value = await api.listMySigningHistoryFilterOptions();
    } catch {
        filterOptions.value = { docFormatCodes: [], departments: [], parties: [] };
    }
}

onMounted(() => {
    loadHistory(1);
    loadFilterOptions();
});

// Any change to which rows match returns to page 1, or the viewer can sit on a
// page the new result set does not reach.
watch([docFormatCodeFilter, departmentCodeFilter, partyCodeFilter], () => loadHistory(1));

watch(
    () => (dateRange.value ? [dateRange.value[0], dateRange.value[1]] : [null, null]),
    ([start, end], [prevStart, prevEnd]) => {
        const cleared = !start && !end && (prevStart || prevEnd);
        if (cleared || (start && end)) loadHistory(1);
    }
);
onBeforeUnmount(() => {
    if (searchTimer) window.clearTimeout(searchTimer);
});

watch(searchQuery, () => {
    if (searchTimer) window.clearTimeout(searchTimer);
    searchTimer = window.setTimeout(() => loadHistory(1), 300);
});

async function loadHistory(nextPage = page.value, nextSize = size.value) {
    const sequence = ++requestSequence;
    loading.value = true;
    try {
        const result = await api.listMySigningHistory({ page: nextPage, size: nextSize, search: searchQuery.value, ...filterParams() });
        if (sequence !== requestSequence) return;
        documents.value = result.documents || [];
        page.value = result.page || nextPage;
        size.value = result.size || nextSize;
        total.value = result.total || 0;
    } catch (err) {
        if (sequence === requestSequence) toast.add({ severity: 'error', summary: 'โหลดประวัติของฉันไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        if (sequence === requestSequence) loading.value = false;
    }
}

function onPage(event) {
    const nextPage = Number(event.page || 0) + 1;
    const nextSize = Number(event.rows || size.value || 10);
    loadHistory(nextPage, nextSize);
}

function openHistory(row) {
    if (!row?.taskId || openingTaskId.value) return;
    openingTaskId.value = row.taskId;
    router.push({ name: 'admin-my-signing-history-detail', params: { taskId: row.taskId } }).finally(() => {
        openingTaskId.value = '';
    });
}

function openAttachmentsDialog(row) {
    if (!row?.taskId || attachmentCount(row) <= 0) return;
    attachmentsRow.value = row;
    attachmentsDialog.value = true;
}

function loadAttachmentsForDialog() {
    if (!attachmentsRow.value?.taskId) return Promise.resolve({ attachments: [] });
    return api.getMyTaskAttachments(attachmentsRow.value.taskId);
}

function attachmentFileUrlForDialog(attachment) {
    if (!attachmentsRow.value?.taskId || !attachment?.id) return '';
    return api.myTaskAttachmentFileUrl(attachmentsRow.value.taskId, attachment.id);
}

function statusView(row) {
    if (row.taskStatus === 'rejected') return { label: 'ปฏิเสธแล้ว', severity: 'danger', icon: 'pi pi-times-circle' };
    return { label: 'เซ็นแล้ว', severity: 'success', icon: 'pi pi-check-circle' };
}

function actionDate(row) {
    return row.taskStatus === 'rejected' ? row.rejectedAt : row.signedAt;
}

function documentLine(row) {
    return [row.docNo, row.docFormatCode].filter(Boolean).join(' · ') || '-';
}

// sml_source_changed/sml_source_missing block the whole document after a
// signer already signed their step — the row's own taskStatus stays
// whatever it was ('signed' etc.), so without this the reason the
// document stalled is invisible in the list, buried under a muted caption.
function documentNeedsAttention(row) {
    return row.documentStatus === 'sml_source_changed' || row.documentStatus === 'sml_source_missing';
}

function attachmentCount(row) {
    return Number(row?.attachmentCount || 0);
}

function partyLine(row) {
    return row.partyName || row.partyCode || '-';
}

function rejectReason(row) {
    return row.taskStatus === 'rejected' ? row.rejectReason || '-' : '-';
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
            <div class="min-w-0">
                <div class="font-semibold text-xl mb-1">ประวัติการเซ็นของฉัน</div>
                <p class="text-muted-color m-0">เอกสารที่คุณเคยเซ็นหรือปฏิเสธในฐานข้อมูลนี้</p>
            </div>
            <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                <Tag :value="`${total || 0} รายการ`" severity="secondary" />
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadHistory(1)" />
            </div>
        </div>

        <div class="border border-surface rounded-lg bg-surface-50 dark:bg-surface-900 p-3 mb-6 flex flex-col gap-3">
            <IconField class="w-full">
                <InputIcon><i class="pi pi-search" /></InputIcon>
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหาเลขเอกสาร คู่ค้า หรือตำแหน่ง" class="w-full" />
            </IconField>

            <div class="flex flex-col sm:flex-row sm:items-center gap-2">
                <div class="grid grid-cols-1 sm:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)_minmax(0,1fr)_minmax(0,1fr)] gap-2 flex-1 min-w-0">
                    <DatePicker v-model="dateRange" selectionMode="range" :manualInput="false" showIcon iconDisplay="input" dateFormat="dd/mm/yy" placeholder="เลือกช่วงวันที่เอกสาร" showButtonBar class="w-full" />
                    <Select v-model="docFormatCodeFilter" :options="docFormatCodeOptions" optionLabel="label" optionValue="value" placeholder="ทุกประเภท" showClear class="w-full" />
                    <Select v-model="departmentCodeFilter" :options="departmentOptions" optionLabel="label" optionValue="value" placeholder="ทุกแผนก" showClear filter class="w-full" />
                    <Select v-model="partyCodeFilter" :options="partyOptions" optionLabel="label" optionValue="value" placeholder="ทุกคู่ค้า" showClear filter class="w-full" />
                </div>
                <Button v-if="hasActiveFilters" label="ล้างตัวกรอง" icon="pi pi-filter-slash" severity="secondary" text class="shrink-0" @click="clearFilters" />
            </div>
        </div>


        <DataTable
            :value="documents"
            :loading="loading"
            dataKey="taskId"
            lazy
            paginator
            :rows="size"
            :first="firstRow"
            :totalRecords="total"
            :rowsPerPageOptions="rowsPerPageOptions"
            responsiveLayout="scroll"
            stripedRows
            @page="onPage"
        >
            <template #empty>
                <div class="py-8 text-center text-muted-color">
                    {{ searchQuery ? 'ไม่พบประวัติจากคำค้นนี้' : 'ยังไม่มีประวัติการเซ็นในฐานข้อมูลนี้' }}
                </div>
            </template>

            <Column header="เลขที่เอกสาร" style="min-width: 15rem">
                <template #body="{ data }">
                    <Button link class="p-0 font-bold text-left" @click="openHistory(data)">{{ documentLine(data) }}</Button>
                </template>
            </Column>
            <Column header="คู่ค้า" style="min-width: 14rem">
                <template #body="{ data }">{{ partyLine(data) }}</template>
            </Column>
            <Column header="แผนก" style="min-width: 10rem">
                <template #body="{ data }">{{ data.departmentName || '-' }}</template>
            </Column>
            <Column header="วันที่เอกสาร" style="min-width: 10rem">
                <template #body="{ data }">{{ formatDocumentDate(data.docDate) }}</template>
            </Column>
            <Column header="ตำแหน่ง" style="min-width: 13rem">
                <template #body="{ data }">{{ data.positionName || '-' }}</template>
            </Column>
            <Column header="วันที่ดำเนินการ" style="min-width: 14rem">
                <template #body="{ data }">
                    <span class="inline-flex items-center gap-2">
                        <i :class="statusView(data).icon"></i>
                        {{ formatThaiDateTime(actionDate(data)) }}
                    </span>
                </template>
            </Column>
            <Column header="ผลการเซ็น" style="min-width: 12rem">
                <template #body="{ data }">
                    <div class="grid gap-1">
                        <Tag :value="statusView(data).label" :severity="statusView(data).severity" class="w-fit" />
                        <Tag
                            v-if="documentNeedsAttention(data)"
                            :value="`เอกสาร: ${signingStatusLabel(data.documentStatus)}`"
                            :severity="signingStatusSeverity(data.documentStatus)"
                            class="w-fit"
                            icon="pi pi-exclamation-triangle"
                        />
                        <small v-else class="text-muted-color">เอกสาร: {{ signingStatusLabel(data.documentStatus) }}</small>
                    </div>
                </template>
            </Column>
            <Column header="เหตุผลปฏิเสธ" style="min-width: 14rem">
                <template #body="{ data }">{{ rejectReason(data) }}</template>
            </Column>
            <Column header="จัดการ" :exportable="false" style="min-width: 13rem">
                <template #body="{ data }">
                    <div class="flex items-center gap-2">
                        <DocumentAttachmentActionButton :count="attachmentCount(data)" @click="openAttachmentsDialog(data)" />
                        <Button label="ดูเอกสาร" icon="pi pi-eye" size="small" severity="secondary" outlined :loading="openingTaskId === data.taskId" @click="openHistory(data)" />
                    </div>
                </template>
            </Column>
        </DataTable>

        <DocumentAttachmentsDialog
            v-model:visible="attachmentsDialog"
            :title="attachmentsDialogTitle"
            :subtitle="attachmentsDialogSubtitle"
            :loader-key="attachmentsDialogKey"
            :loader="loadAttachmentsForDialog"
            :file-url-resolver="attachmentFileUrlForDialog"
            :headers="api.authHeaders()"
        />
    </div>
</template>
