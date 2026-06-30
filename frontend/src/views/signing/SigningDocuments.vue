<script setup>
import { api } from '@/services/api';
import { formatDocumentDate, formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import DocumentFlowViewer from '@/views/signing/components/DocumentFlowViewer.vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();

const documents = ref([]);
const loading = ref(false);
const searchQuery = ref('');
const flowDialog = ref(false);
const flowLoading = ref(false);
const flowError = ref('');
const flowNotice = ref('');
const flowGraph = ref(null);
const flowDocument = ref(null);
const flowRequestSeq = ref(0);
const pdfDialog = ref(false);
const pdfLoading = ref(false);
const pdfUrl = ref('');
const pdfTitle = ref('');

const flowCache = new Map();
const flowSessionId = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
const openedAt = Date.now();

const filteredDocuments = computed(() => {
    const query = normalize(searchQuery.value);
    if (!query) return documents.value;
    return documents.value.filter((doc) => normalize(`${doc.docFormatCode} ${doc.docNo} ${doc.partyName} ${doc.partyCode} ${signingStatusLabel(doc.status)}`).includes(query));
});

const flowHeader = computed(() => {
    const doc = flowDocument.value;
    if (!doc?.docNo) return 'Flow เอกสาร';
    const party = doc.partyName || doc.partyCode || '';
    return `${doc.docNo} ~ ${doc.docFormatCode || '-'}${party ? ` · ${party}` : ''}`;
});

onMounted(loadPage);

watch(
    () => [route.query.flow_doc_no, route.query.flow_doc_format_code],
    ([docNo, docFormatCode]) => {
        if (!docNo) return;
        void openDocumentFlow(
            {
                docNo: String(docNo),
                docFormatCode: String(docFormatCode || '')
            },
            { syncQuery: false }
        );
    },
    { immediate: true }
);

onBeforeUnmount(() => {
    clearPDFUrl();
});

async function loadPage() {
    loading.value = true;
    try {
        const result = await api.listSigningDocuments();
        documents.value = result.documents || [];
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openCreate() {
    router.push({ name: 'signing-document-new' });
}

function openDetail(doc) {
    router.push({ name: 'signing-document-detail', params: { id: doc.id } });
}

function openDocumentFlowFromRow(doc) {
    if (!doc?.docNo) return;
    const { flow_doc_no: _flowDocNo, flow_doc_format_code: _flowDocFormatCode, ...rest } = route.query;
    router.replace({
        name: 'signing-documents',
        query: {
            ...rest,
            flow_doc_no: doc.docNo,
            ...(doc.docFormatCode ? { flow_doc_format_code: doc.docFormatCode } : {})
        }
    });
}

async function openDocumentFlow(doc, options = {}) {
    const docNo = String(doc?.docNo || doc?.doc_no || '').trim().toUpperCase();
    if (!docNo) return;
    const docFormatCode = String(doc?.docFormatCode || doc?.doc_format_code || '').trim().toUpperCase();
    flowDocument.value = {
        docNo,
        docFormatCode,
        partyName: doc?.partyName || doc?.party_name || '',
        partyCode: doc?.partyCode || doc?.party_code || ''
    };
    flowDialog.value = true;
    flowError.value = '';
    flowNotice.value = '';

    if (options.syncQuery !== false) {
        const { flow_doc_no: _flowDocNo, flow_doc_format_code: _flowDocFormatCode, ...rest } = route.query;
        router.replace({
            name: 'signing-documents',
            query: {
                ...rest,
                flow_doc_no: docNo,
                ...(docFormatCode ? { flow_doc_format_code: docFormatCode } : {})
            }
        });
    }

    await loadDocumentFlow({ docNo, docFormatCode }, { force: options.force === true });
}

async function loadDocumentFlow(doc = flowDocument.value, options = {}) {
    const docNo = String(doc?.docNo || '').trim().toUpperCase();
    if (!docNo) return;
    const docFormatCode = String(doc?.docFormatCode || '').trim().toUpperCase();
    const cacheKey = `${docFormatCode}:${docNo}`;
    const requestSeq = flowRequestSeq.value + 1;
    flowRequestSeq.value = requestSeq;
    flowError.value = '';
    flowNotice.value = '';

    if (!options.force && flowCache.has(cacheKey)) {
        flowGraph.value = flowCache.get(cacheKey);
        flowLoading.value = false;
        touchFlowCache(cacheKey, flowGraph.value);
        recordFlowEvent('document_flow_cache_hit', { docFormatCode, nodeCount: flowGraph.value?.nodes?.length || 0 });
        return;
    }

    flowLoading.value = true;
    flowGraph.value = null;
    recordFlowEvent('document_flow_search', { docFormatCode });

    try {
        const result = await api.getAdminDocumentFlow({ docNo, docFormatCode, depth: 3 });
        if (requestSeq !== flowRequestSeq.value) return;
        const graph = result.documentFlow;
        flowGraph.value = graph;
        touchFlowCache(cacheKey, graph);
        recordFlowEvent('document_flow_load_success', { docFormatCode, nodeCount: graph?.nodes?.length || 0 });
    } catch (err) {
        if (requestSeq !== flowRequestSeq.value) return;
        flowError.value = userFacingFlowError(err);
        recordFlowEvent('document_flow_load_error', { docFormatCode, errorCode: err?.payload?.code || 'document_flow_load_error' });
        toast.add({ severity: 'error', summary: 'โหลด Flow เอกสารไม่สำเร็จ', detail: flowError.value, life: 4000 });
    } finally {
        if (requestSeq === flowRequestSeq.value) flowLoading.value = false;
    }
}

function touchFlowCache(key, value) {
    if (flowCache.has(key)) flowCache.delete(key);
    flowCache.set(key, value);
    while (flowCache.size > 20) {
        const oldestKey = flowCache.keys().next().value;
        flowCache.delete(oldestKey);
    }
}

function closeFlowDialog() {
    flowDialog.value = false;
    flowError.value = '';
    flowNotice.value = '';
    const { flow_doc_no: _flowDocNo, flow_doc_format_code: _flowDocFormatCode, ...rest } = route.query;
    if (_flowDocNo || _flowDocFormatCode) router.replace({ name: 'signing-documents', query: rest });
}

async function previewFlowPDF(payload = {}) {
    const node = payload.node || {};
    const version = payload.version === 'final' ? 'final' : 'current';
    const url = payload.url || (version === 'final' ? node.signedPdfUrl : node.currentPdfUrl);
    const docNo = node.doc_no || 'เอกสารนี้';
    if (!url) {
        flowNotice.value = `${docNo} ยังไม่มี PDF ใน PaperLess`;
        toast.add({ severity: 'info', summary: 'ยังไม่มี PDF ใน PaperLess', detail: flowNotice.value, life: 3000 });
        return;
    }

    clearPDFUrl();
    flowNotice.value = '';
    pdfLoading.value = true;
    pdfDialog.value = true;
    pdfTitle.value = `${docNo} · ${version === 'final' ? 'PDF ที่เซ็นครบแล้ว' : 'PDF ล่าสุด'}`;
    recordFlowEvent('document_flow_pdf_open', { docFormatCode: node.doc_format_code || flowDocument.value?.docFormatCode || '', nodeCount: flowGraph.value?.nodes?.length || 0 });

    try {
        const response = await fetch(url, { headers: api.authHeaders() });
        if (!response.ok) throw new Error('โหลด PDF ไม่สำเร็จ');
        const blob = await response.blob();
        pdfUrl.value = URL.createObjectURL(blob);
    } catch (err) {
        pdfDialog.value = false;
        toast.add({ severity: 'error', summary: 'เปิด PDF ไม่สำเร็จ', detail: err?.message || 'กรุณาลองใหม่', life: 3500 });
    } finally {
        pdfLoading.value = false;
    }
}

function clearPDFUrl() {
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
    pdfUrl.value = '';
}

function userFacingFlowError(err) {
    const code = err?.payload?.code || '';
    if (code === 'sml_document_not_found') return 'ไม่พบเลขเอกสารนี้ใน SML';
    if (code === 'sml_unavailable' || code === 'sml_not_configured') return 'เชื่อมต่อ SML ไม่สำเร็จ กรุณาลองใหม่';
    return err?.message || 'ไม่สามารถโหลด Flow เอกสารได้ กรุณาลองใหม่';
}

function recordFlowEvent(event, extra = {}) {
    api.recordDocumentFlowEvent({
        event,
        sessionId: flowSessionId,
        docFormatCode: extra.docFormatCode || flowDocument.value?.docFormatCode || '',
        elapsedMs: Date.now() - openedAt,
        nodeCount: extra.nodeCount || flowGraph.value?.nodes?.length || 0,
        errorCode: extra.errorCode || ''
    }).catch(() => {});
}

function documentLine(doc) {
    return `${doc.docNo || '-'} ~ ${doc.docFormatCode || '-'} · ${doc.partyName || doc.partyCode || '-'}`;
}

function formatMoney(value) {
    return Number(value || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 });
}

function normalize(value) {
    return String(value || '').toLowerCase().trim();
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div class="min-w-0 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                <div class="font-semibold text-xl whitespace-nowrap truncate">เอกสารเซ็น</div>
                <p class="text-muted-color m-0 min-w-0 truncate">ส่งเอกสารใหม่และติดตามสถานะการเซ็น</p>
            </div>
            <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                <IconField class="w-full sm:w-80">
                    <InputIcon><i class="pi pi-search" /></InputIcon>
                    <InputText v-model="searchQuery" type="search" placeholder="ค้นหาเลขเอกสาร คู่ค้า สถานะ" class="w-full" />
                </IconField>
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadPage" />
                <Button label="ส่งเอกสารใหม่" icon="pi pi-send" @click="openCreate" />
            </div>
        </div>

        <DataTable :value="filteredDocuments" :loading="loading" dataKey="id" paginator :rows="10" responsiveLayout="scroll" stripedRows>
            <template #empty>
                <div class="py-8 text-center text-muted-color">
                    {{ searchQuery ? 'ไม่พบเอกสารที่ค้นหา' : 'ยังไม่มีเอกสารเซ็น เริ่มจากปุ่มส่งเอกสารใหม่' }}
                </div>
            </template>

            <Column field="docNo" header="เลขที่เอกสาร" sortable style="min-width: 16rem">
                <template #body="{ data }">
                    <Button link class="p-0 font-bold text-left" @click="openDetail(data)">
                        <span class="whitespace-nowrap">{{ documentLine(data) }}</span>
                    </Button>
                </template>
            </Column>
            <Column field="docDate" header="วันที่เอกสาร" sortable style="min-width: 10rem">
                <template #body="{ data }">{{ formatDocumentDate(data.docDate) }}</template>
            </Column>
            <Column field="totalAmount" header="ยอดเงิน" sortable style="min-width: 10rem">
                <template #body="{ data }">{{ formatMoney(data.totalAmount) }}</template>
            </Column>
            <Column field="status" header="สถานะ" sortable style="min-width: 12rem">
                <template #body="{ data }">
                    <Tag :value="signingStatusLabel(data.status)" :severity="signingStatusSeverity(data.status)" />
                </template>
            </Column>
            <Column field="updatedAt" header="อัปเดตล่าสุด" sortable style="min-width: 14rem">
                <template #body="{ data }">{{ formatThaiDateTime(data.updatedAt) }}</template>
            </Column>
            <Column header="จัดการ" :exportable="false" style="width: 9rem">
                <template #body="{ data }">
                    <div class="flex gap-2">
                        <Button icon="pi pi-sitemap" rounded outlined severity="secondary" aria-label="ดู Flow เอกสาร" @click="openDocumentFlowFromRow(data)" />
                        <Button icon="pi pi-eye" rounded outlined severity="secondary" aria-label="ดูเอกสาร" @click="openDetail(data)" />
                    </div>
                </template>
            </Column>
        </DataTable>

        <Dialog v-model:visible="flowDialog" modal :header="flowHeader" :style="{ width: 'min(70rem, 96vw)' }" @hide="closeFlowDialog">
            <div class="flex flex-col gap-3">
                <div class="flex flex-col md:flex-row md:items-center justify-between gap-3">
                    <div class="min-w-0">
                        <div class="font-semibold">Flow เอกสาร</div>
                        <small class="text-muted-color">ดูความสัมพันธ์จาก SML และเปิด PDF ของเอกสารที่มีใน PaperLess</small>
                    </div>
                    <Button icon="pi pi-refresh" label="โหลดใหม่" severity="secondary" outlined :loading="flowLoading" @click="loadDocumentFlow(flowDocument, { force: true })" />
                </div>

                <Message v-if="flowNotice" severity="info">{{ flowNotice }}</Message>
                <Message v-if="flowError" severity="error">
                    {{ flowError }}
                    <div class="mt-3">
                        <Button size="small" label="ลองใหม่" icon="pi pi-refresh" severity="secondary" outlined @click="loadDocumentFlow(flowDocument, { force: true })" />
                    </div>
                </Message>

                <div v-if="flowLoading && !flowGraph" class="flow-loading">
                    <i class="pi pi-spin pi-spinner"></i>
                    <span>กำลังโหลด Flow เอกสาร</span>
                </div>
                <DocumentFlowViewer
                    v-else
                    :graph="flowGraph"
                    admin
                    compact
                    :show-table="false"
                    @open-document="(documentId) => openDetail({ id: documentId })"
                    @preview-pdf="previewFlowPDF"
                />
            </div>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="flowDialog = false" />
            </template>
        </Dialog>

        <Dialog v-model:visible="pdfDialog" modal :header="pdfTitle" :style="{ width: 'min(72rem, 96vw)' }" @hide="clearPDFUrl">
            <div v-if="pdfLoading" class="flow-loading">
                <i class="pi pi-spin pi-spinner"></i>
                <span>กำลังโหลด PDF</span>
            </div>
            <iframe v-else-if="pdfUrl" :src="pdfUrl" class="pdf-frame" title="PDF preview"></iframe>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="pdfDialog = false" />
            </template>
        </Dialog>
    </div>
</template>

<style scoped>
.flow-loading {
    min-height: 10rem;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.6rem;
    color: var(--text-color-secondary);
    text-align: center;
}

.pdf-frame {
    width: 100%;
    height: min(72vh, 52rem);
    border: 1px solid var(--surface-border);
    border-radius: 8px;
}
</style>
