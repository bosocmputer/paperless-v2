<script setup>
import { api } from '@/services/api';
import { formatDocumentDate, formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const readyDocuments = ref([]);
const waitingDocuments = ref([]);
const counts = ref({ ready: 0, waiting: 0 });
const pagination = ref({
    ready: { page: 1, size: 20, hasMore: false },
    waiting: { page: 1, size: 20, hasMore: false }
});
const activeTab = ref('ready');
const searchQuery = ref('');
const loading = ref(false);
const loadingReadyMore = ref(false);
const loadingWaitingMore = ref(false);
const openingTaskId = ref('');
let requestSequence = 0;

const readyRows = computed(() => readyDocuments.value.map(normalizeQueueRow).filter(Boolean));
const waitingRows = computed(() => waitingDocuments.value.map(normalizeQueueRow).filter(Boolean));
const filteredReadyRows = computed(() => filterRows(readyRows.value));
const filteredWaitingRows = computed(() => filterRows(waitingRows.value));
const totalTasks = computed(() => Number(counts.value.ready || 0) + Number(counts.value.waiting || 0));

onMounted(() => loadTasks());

async function loadTasks() {
    const sequence = ++requestSequence;
    loading.value = true;
    try {
        const result = await api.listMySigningTasks({ readyPage: 1, waitingPage: 1, size: 20 });
        if (sequence !== requestSequence) return;
        readyDocuments.value = result.documents || [];
        waitingDocuments.value = result.waitingDocuments || [];
        applyQueueMeta(result);
    } catch (err) {
        if (sequence === requestSequence) toast.add({ severity: 'error', summary: 'โหลดงานของฉันไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        if (sequence === requestSequence) loading.value = false;
    }
}

async function loadMoreReady() {
    if (!pagination.value.ready.hasMore || loadingReadyMore.value) return;
    const sequence = requestSequence;
    loadingReadyMore.value = true;
    try {
        const nextPage = Number(pagination.value.ready.page || 1) + 1;
        const result = await api.listMySigningTasks({ readyPage: nextPage, waitingPage: pagination.value.waiting.page, size: pagination.value.ready.size || 20 });
        if (sequence !== requestSequence) return;
        readyDocuments.value = [...readyDocuments.value, ...(result.documents || [])];
        pagination.value = { ...pagination.value, ready: result.pagination?.ready || { page: nextPage, size: 20, hasMore: false } };
        counts.value = result.counts || counts.value;
    } catch (err) {
        if (sequence === requestSequence) toast.add({ severity: 'error', summary: 'โหลดงานเพิ่มไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loadingReadyMore.value = false;
    }
}

async function loadMoreWaiting() {
    if (!pagination.value.waiting.hasMore || loadingWaitingMore.value) return;
    const sequence = requestSequence;
    loadingWaitingMore.value = true;
    try {
        const nextPage = Number(pagination.value.waiting.page || 1) + 1;
        const result = await api.listMySigningTasks({ readyPage: pagination.value.ready.page, waitingPage: nextPage, size: pagination.value.waiting.size || 20 });
        if (sequence !== requestSequence) return;
        waitingDocuments.value = [...waitingDocuments.value, ...(result.waitingDocuments || [])];
        pagination.value = { ...pagination.value, waiting: result.pagination?.waiting || { page: nextPage, size: 20, hasMore: false } };
        counts.value = result.counts || counts.value;
    } catch (err) {
        if (sequence === requestSequence) toast.add({ severity: 'error', summary: 'โหลดงานรอคิวเพิ่มไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loadingWaitingMore.value = false;
    }
}

function applyQueueMeta(result) {
    counts.value = result.counts || { ready: readyDocuments.value.length, waiting: waitingDocuments.value.length };
    pagination.value = {
        ready: result.pagination?.ready || { page: 1, size: 20, hasMore: false },
        waiting: result.pagination?.waiting || { page: 1, size: 20, hasMore: false }
    };
}

function normalizeQueueRow(doc) {
    const task = doc.task || (doc.signers || [])[0];
    if (!doc?.id || !task?.id) return null;
    return { rowKey: task.id, doc, task };
}

function filterRows(rows) {
    const query = normalizeSearch(searchQuery.value);
    if (!query) return rows;
    return rows.filter(({ doc, task }) =>
        normalizeSearch(
            [
                doc.docNo,
                doc.docFormatCode,
                doc.partyName,
                doc.partyCode,
                task.positionName,
                task.signerName,
                task.signerUser,
                doc.blockSummary,
                ...(doc.blockedBy || []).flatMap((blocker) => [blocker.positionName, blocker.summary, ...(blocker.signers || []).map((signer) => signer.signerName || signer.signerUser)])
            ]
                .filter(Boolean)
                .join(' ')
        ).includes(query)
    );
}

function openTask(row) {
    if (!row?.task?.id || openingTaskId.value) return;
    openingTaskId.value = row.task.id;
    router.push({ name: 'admin-my-signing-task', params: { taskId: row.task.id } }).finally(() => {
        openingTaskId.value = '';
    });
}

function documentLine(doc) {
    return [doc.docNo, doc.docFormatCode].filter(Boolean).join(' · ') || '-';
}

function attachmentCount(doc) {
    return Number(doc?.attachmentCount || 0);
}

function partyLine(doc) {
    return doc.partyName || doc.partyCode || '-';
}

function waitingDetail(row) {
    return row.doc.blockSummary || 'รอขั้นตอนก่อนหน้าเสร็จก่อน';
}

function formatMoney(value) {
    const amount = Number(value || 0);
    if (!amount) return '-';
    return new Intl.NumberFormat('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount);
}

function normalizeSearch(value) {
    return String(value || '').toLowerCase().trim();
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
            <div class="min-w-0">
                <div class="font-semibold text-xl mb-1">งานรอเซ็นของฉัน</div>
                <p class="text-muted-color m-0">งานที่ระบุคุณเป็นผู้เซ็นในฐานข้อมูลนี้</p>
            </div>
            <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                <div class="flex flex-wrap gap-2">
                    <Tag :value="`เซ็นได้ ${counts.ready || 0}`" severity="info" />
                    <Tag :value="`รอคิว ${counts.waiting || 0}`" severity="secondary" />
                    <Tag :value="`ทั้งหมด ${totalTasks}`" severity="contrast" />
                </div>
                <IconField class="w-full sm:w-80">
                    <InputIcon><i class="pi pi-search" /></InputIcon>
                    <InputText v-model="searchQuery" type="search" placeholder="ค้นหาเลขเอกสาร คู่ค้า หรือตำแหน่ง" class="w-full" />
                </IconField>
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadTasks" />
            </div>
        </div>

        <Tabs v-model:value="activeTab">
            <TabList>
                <Tab value="ready">เซ็นได้ตอนนี้ ({{ counts.ready || 0 }})</Tab>
                <Tab value="waiting">รอคิวเซ็น ({{ counts.waiting || 0 }})</Tab>
            </TabList>
            <TabPanels>
                <TabPanel value="ready">
                    <DataTable :value="filteredReadyRows" :loading="loading" dataKey="rowKey" paginator :rows="10" responsiveLayout="scroll" stripedRows>
                        <template #empty>
                            <div class="py-8 text-center text-muted-color">
                                {{ searchQuery ? 'ไม่พบงานที่เซ็นได้จากคำค้นนี้' : 'ยังไม่มีงานที่ต้องเซ็นในฐานข้อมูลนี้' }}
                            </div>
                        </template>
                        <Column header="เลขที่เอกสาร" style="min-width: 15rem">
                            <template #body="{ data }">
                                <div class="grid gap-1">
                                    <Button link class="p-0 font-bold text-left" @click="openTask(data)">{{ documentLine(data.doc) }}</Button>
                                    <Tag v-if="attachmentCount(data.doc)" :value="`แนบ ${attachmentCount(data.doc)}`" severity="info" class="w-fit" />
                                </div>
                            </template>
                        </Column>
                        <Column header="คู่ค้า" style="min-width: 14rem">
                            <template #body="{ data }">{{ partyLine(data.doc) }}</template>
                        </Column>
                        <Column header="วันที่เอกสาร" style="min-width: 10rem">
                            <template #body="{ data }">{{ formatDocumentDate(data.doc.docDate) }}</template>
                        </Column>
                        <Column header="ยอดเงิน" style="min-width: 10rem">
                            <template #body="{ data }">{{ formatMoney(data.doc.totalAmount) }}</template>
                        </Column>
                        <Column header="ตำแหน่งของฉัน" style="min-width: 13rem">
                            <template #body="{ data }">{{ data.task.positionName || '-' }}</template>
                        </Column>
                        <Column header="สถานะ" style="min-width: 12rem">
                            <template #body="{ data }">
                                <Tag value="รอเซ็น" severity="info" />
                                <small class="block text-muted-color mt-1">{{ formatThaiDateTime(data.doc.updatedAt) }}</small>
                            </template>
                        </Column>
                        <Column header="จัดการ" :exportable="false" style="min-width: 10rem">
                            <template #body="{ data }">
                                <Button label="เซ็นเอกสาร" icon="pi pi-pencil" size="small" :loading="openingTaskId === data.task.id" @click="openTask(data)" />
                            </template>
                        </Column>
                    </DataTable>
                    <div v-if="pagination.ready.hasMore" class="mt-4">
                        <Button label="โหลดงานที่เซ็นได้เพิ่ม" icon="pi pi-angle-down" severity="secondary" outlined :loading="loadingReadyMore" @click="loadMoreReady" />
                    </div>
                </TabPanel>

                <TabPanel value="waiting">
                    <DataTable :value="filteredWaitingRows" :loading="loading" dataKey="rowKey" paginator :rows="10" responsiveLayout="scroll" stripedRows>
                        <template #empty>
                            <div class="py-8 text-center text-muted-color">
                                {{ searchQuery ? 'ไม่พบเอกสารรอคิวจากคำค้นนี้' : 'ไม่มีเอกสารที่รอคิวของคุณ' }}
                            </div>
                        </template>
                        <Column header="เลขที่เอกสาร" style="min-width: 15rem">
                            <template #body="{ data }">
                                <div class="grid gap-1">
                                    <Button link class="p-0 font-bold text-left" @click="openTask(data)">{{ documentLine(data.doc) }}</Button>
                                    <Tag v-if="attachmentCount(data.doc)" :value="`แนบ ${attachmentCount(data.doc)}`" severity="info" class="w-fit" />
                                </div>
                            </template>
                        </Column>
                        <Column header="คู่ค้า" style="min-width: 14rem">
                            <template #body="{ data }">{{ partyLine(data.doc) }}</template>
                        </Column>
                        <Column header="วันที่เอกสาร" style="min-width: 10rem">
                            <template #body="{ data }">{{ formatDocumentDate(data.doc.docDate) }}</template>
                        </Column>
                        <Column header="ตำแหน่งของฉัน" style="min-width: 13rem">
                            <template #body="{ data }">{{ data.task.positionName || '-' }}</template>
                        </Column>
                        <Column header="รอใคร" style="min-width: 18rem">
                            <template #body="{ data }">
                                <div class="grid gap-1">
                                    <Tag :value="signingStatusLabel(data.task.status)" :severity="signingStatusSeverity(data.task.status)" class="w-fit" />
                                    <small class="text-muted-color">{{ waitingDetail(data) }}</small>
                                </div>
                            </template>
                        </Column>
                        <Column header="จัดการ" :exportable="false" style="min-width: 11rem">
                            <template #body="{ data }">
                                <Button label="ดูความคืบหน้า" icon="pi pi-eye" size="small" severity="secondary" outlined :loading="openingTaskId === data.task.id" @click="openTask(data)" />
                            </template>
                        </Column>
                    </DataTable>
                    <div v-if="pagination.waiting.hasMore" class="mt-4">
                        <Button label="โหลดเอกสารรอคิวเพิ่ม" icon="pi pi-angle-down" severity="secondary" outlined :loading="loadingWaitingMore" @click="loadMoreWaiting" />
                    </div>
                </TabPanel>
            </TabPanels>
        </Tabs>
    </div>
</template>
