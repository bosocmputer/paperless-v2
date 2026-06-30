<script setup>
import { api } from '@/services/api';
import { formatDocumentDate, formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();

const docNo = ref(String(route.query.doc_no || '').toUpperCase());
const docFormatCode = ref(String(route.query.doc_format_code || '').toUpperCase());
const loading = ref(false);
const loadingDocuments = ref(false);
const searched = ref(false);
const error = ref('');
const graph = ref(null);
const documents = ref([]);
const selectedNode = ref(null);
const detailVisible = ref(false);
const pdfDialog = ref(false);
const pdfLoading = ref(false);
const pdfUrl = ref('');
const pdfTitle = ref('');

const sessionId = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
const openedAt = Date.now();

const nodes = computed(() => graph.value?.nodes || []);
const edges = computed(() => graph.value?.edges || []);
const root = computed(() => graph.value?.root || null);
const flowRows = computed(() =>
    orderedNodes().map((node, index) => ({
        ...node,
        flowIndex: index + 1,
        relationLabel: relationText(node)
    }))
);
const flowStrip = computed(() => flowRows.value.map((node) => `${node.doc_format_code || '-'} ${node.doc_no}`).join(' → '));
const summary = computed(() => {
    const total = flowRows.value.length;
    const paperless = flowRows.value.filter((node) => !!node.paperlessStatus).length;
    const completed = flowRows.value.filter((node) => !!node.hasFinalPdf || node.paperlessStatus === 'completed').length;
    return {
        total,
        paperless,
        completed,
        missing: Math.max(total - paperless, 0)
    };
});
const warnings = computed(() => graph.value?.warnings || []);

onMounted(() => {
    recordEvent('document_flow_open');
    void loadDocuments();
    if (docNo.value) void search();
});

onBeforeUnmount(() => {
    clearPDFUrl();
});

function normalizeInputs() {
    docNo.value = docNo.value.trim().toUpperCase();
    docFormatCode.value = docFormatCode.value.trim().toUpperCase();
}

async function search() {
    normalizeInputs();
    if (!docNo.value || loading.value) return;
    loading.value = true;
    searched.value = true;
    error.value = '';
    graph.value = null;
    selectedNode.value = null;
    recordEvent('document_flow_search');
    router.replace({
        name: 'document-flow',
        query: {
            doc_no: docNo.value,
            ...(docFormatCode.value ? { doc_format_code: docFormatCode.value } : {})
        }
    });

    try {
        const result = await api.getAdminDocumentFlow({ docNo: docNo.value, docFormatCode: docFormatCode.value, depth: 3 });
        graph.value = result.documentFlow;
        recordEvent('document_flow_load_success', { nodeCount: nodes.value.length, docFormatCode: root.value?.doc_format_code || docFormatCode.value });
    } catch (err) {
        error.value = userFacingError(err);
        recordEvent('document_flow_load_error', { errorCode: err?.payload?.code || 'document_flow_load_error' });
        toast.add({ severity: 'error', summary: 'โหลด Flow เอกสารไม่สำเร็จ', detail: error.value, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadDocuments() {
    loadingDocuments.value = true;
    try {
        const result = await api.listSigningDocuments();
        documents.value = result.documents || [];
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารใน PaperLess ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loadingDocuments.value = false;
    }
}

function userFacingError(err) {
    const code = err?.payload?.code || '';
    if (code === 'sml_document_not_found') return 'ไม่พบเลขเอกสารนี้ใน SML';
    if (code === 'sml_unavailable' || code === 'sml_not_configured') return 'เชื่อมต่อ SML ไม่สำเร็จ กรุณาลองใหม่';
    return err?.message || 'ไม่สามารถโหลด Flow เอกสารได้ กรุณาลองใหม่';
}

function relationText(node) {
    const incoming = edges.value.filter((edge) => edge.to_doc_no === node.doc_no);
    if (incoming.length === 0) return 'เอกสารต้นทาง';
    const from = [...new Set(incoming.map((edge) => edge.from_doc_no).filter(Boolean))];
    return from.length ? `ต่อจาก ${from.join(', ')}` : 'เอกสารที่เกี่ยวข้อง';
}

function orderedNodes() {
    if (edges.value.length > 0) return nodes.value;
    const businessRank = new Map([
        ['PO', 1],
        ['PA', 2],
        ['PB', 3],
        ['PV', 4]
    ]);
    if (!nodes.value.every((node) => businessRank.has(String(node.doc_format_code || '').toUpperCase()))) return nodes.value;
    return [...nodes.value].sort((a, b) => {
        const rank = businessRank.get(String(a.doc_format_code || '').toUpperCase()) - businessRank.get(String(b.doc_format_code || '').toUpperCase());
        if (rank !== 0) return rank;
        return String(a.doc_no || '').localeCompare(String(b.doc_no || ''));
    });
}

function technicalRelations(node) {
    const all = edges.value.filter((edge) => edge.to_doc_no === node.doc_no || edge.from_doc_no === node.doc_no);
    return all.map((edge) => `${edge.from_doc_no} → ${edge.to_doc_no} (${edge.source_table}.${edge.source_column})`).join('\n');
}

function paperlessLabel(node) {
    return node.paperlessStatus ? signingStatusLabel(node.paperlessStatus) : 'ยังไม่ได้อัปโหลด';
}

function paperlessSeverity(node) {
    return node.paperlessStatus ? signingStatusSeverity(node.paperlessStatus) : 'secondary';
}

function smlLabel(node) {
    return Number(node.is_lock_record || 0) === 1 ? 'Lock แล้ว' : 'ยังไม่ Lock';
}

function smlSeverity(node) {
    return Number(node.is_lock_record || 0) === 1 ? 'success' : 'secondary';
}

function currentPdfLabel(node) {
    if (node.hasFinalPdf) return 'มี PDF ที่เซ็นครบ';
    if (node.hasCurrentPdf) return 'มี PDF ล่าสุด';
    return 'ยังไม่มี PDF';
}

function currentPdfSeverity(node) {
    if (node.hasFinalPdf) return 'success';
    if (node.hasCurrentPdf) return 'info';
    return 'secondary';
}

function formatAmount(value) {
    return Number(value || 0).toLocaleString('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

function documentLine(doc) {
    return `${doc.docNo || '-'} ~ ${doc.docFormatCode || '-'} · ${doc.partyName || doc.partyCode || '-'}`;
}

function documentPdfLabel(doc) {
    if (doc.finalFileId) return 'มี PDF ที่เซ็นครบ';
    if (doc.currentFileId) return 'มี PDF ล่าสุด';
    return 'ยังไม่มี PDF';
}

function documentPdfSeverity(doc) {
    if (doc.finalFileId) return 'success';
    if (doc.currentFileId) return 'info';
    return 'secondary';
}

function openInfo(node) {
    recordEvent('document_flow_node_click', { nodeCount: nodes.value.length });
    selectedNode.value = node;
    detailVisible.value = true;
}

function openDocument(documentId) {
    if (!documentId) return;
    recordEvent('document_flow_node_click', { nodeCount: nodes.value.length });
    router.push({ name: 'signing-document-detail', params: { id: documentId } });
}

function openSigningDocument(doc) {
    if (!doc?.id) return;
    router.push({ name: 'signing-document-detail', params: { id: doc.id } });
}

function openFlowFromDocument(doc) {
    docNo.value = String(doc?.docNo || '').toUpperCase();
    docFormatCode.value = String(doc?.docFormatCode || '').toUpperCase();
    void search();
}

function openPaperless(node) {
    if (node.canOpenPaperless && node.paperlessDocumentId) openDocument(node.paperlessDocumentId);
}

function startUpload(node) {
    if (!node?.doc_no || !node?.doc_format_code) return;
    router.push({
        name: 'signing-document-new',
        query: {
            doc_format_code: node.doc_format_code,
            doc_no: node.doc_no
        }
    });
}

async function previewPDF(node, version) {
    const url = version === 'final' ? node.signedPdfUrl : node.currentPdfUrl;
    if (!url) {
        toast.add({ severity: 'warn', summary: 'ยังไม่มี PDF', detail: version === 'final' ? 'เอกสารยังไม่มี PDF ที่เซ็นครบ' : 'เอกสารยังไม่มี PDF ล่าสุด', life: 3000 });
        return;
    }
    recordEvent('document_flow_pdf_open', { nodeCount: nodes.value.length, docFormatCode: node?.doc_format_code || docFormatCode.value });
    clearPDFUrl();
    pdfLoading.value = true;
    pdfDialog.value = true;
    pdfTitle.value = `${node?.doc_no || 'เอกสาร'} · ${version === 'final' ? 'PDF ที่เซ็นครบแล้ว' : 'PDF ล่าสุด'}`;
    try {
        const response = await fetch(url, { headers: api.authHeaders() });
        if (!response.ok) throw new Error('โหลด PDF ไม่สำเร็จ');
        const blob = await response.blob();
        pdfUrl.value = URL.createObjectURL(blob);
    } catch (err) {
        toast.add({ severity: 'error', summary: 'เปิด PDF ไม่สำเร็จ', detail: err?.message || 'กรุณาลองใหม่', life: 3500 });
        pdfDialog.value = false;
    } finally {
        pdfLoading.value = false;
    }
}

function clearPDFUrl() {
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
    pdfUrl.value = '';
}

function recordEvent(event, extra = {}) {
    api.recordDocumentFlowEvent({
        event,
        sessionId,
        docFormatCode: extra.docFormatCode || docFormatCode.value,
        elapsedMs: Date.now() - openedAt,
        nodeCount: extra.nodeCount || nodes.value.length,
        errorCode: extra.errorCode || ''
    }).catch(() => {});
}
</script>

<template>
    <div class="flex flex-col gap-4">
        <div class="card">
            <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
                <div class="min-w-0 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <div class="font-semibold text-xl whitespace-nowrap truncate">ตรวจสอบ Flow เอกสาร</div>
                    <div class="text-muted-color whitespace-nowrap truncate">ค้นเลขเอกสารจาก SML เพื่อดูว่าเอกสารไหนอัปโหลดและเซ็นครบแล้ว</div>
                </div>
            </div>

            <Toolbar>
                <template #start>
                    <div class="flex flex-col md:flex-row gap-3 w-full">
                        <IconField class="w-full md:w-80">
                            <InputIcon><i class="pi pi-search" /></InputIcon>
                            <InputText v-model="docNo" class="w-full" placeholder="เลขที่เอกสาร เช่น PO26060001" :disabled="loading" @keyup.enter="search" />
                        </IconField>
                        <InputText v-model="docFormatCode" class="w-full md:w-40" placeholder="ชนิด เช่น PO" :disabled="loading" @keyup.enter="search" />
                    </div>
                </template>
                <template #end>
                    <Button label="ค้นหา" icon="pi pi-search" :loading="loading" :disabled="!docNo.trim()" @click="search" />
                </template>
            </Toolbar>
        </div>

        <Message v-if="error" severity="error">
            {{ error }}
            <div class="mt-3">
                <Button label="ลองใหม่" icon="pi pi-refresh" severity="secondary" outlined :disabled="!docNo.trim()" @click="search" />
            </div>
        </Message>

        <div v-if="!searched" class="card">
            <div class="flex flex-col md:flex-row md:items-center justify-between gap-3 mb-4">
                <div class="min-w-0">
                    <div class="font-semibold text-lg">เอกสารที่มีใน PaperLess</div>
                    <div class="text-muted-color">เปิดหน้านี้มาเห็นรายการที่อัปโหลดแล้วทันที กดดู Flow เพื่อเช็คเอกสารเชื่อมโยง</div>
                </div>
                <Tag :value="`${documents.length} เอกสาร`" severity="secondary" />
            </div>

            <DataTable :value="documents" :loading="loadingDocuments" dataKey="id" paginator :rows="10" responsiveLayout="scroll" stripedRows>
                <template #empty>
                    <div class="py-8 text-center text-muted-color">ยังไม่มีเอกสารใน PaperLess</div>
                </template>

                <Column field="docNo" header="เลขที่เอกสาร" sortable style="min-width: 18rem">
                    <template #body="{ data }">
                        <Button link class="p-0 font-bold text-left" @click="openSigningDocument(data)">
                            <span class="whitespace-nowrap">{{ documentLine(data) }}</span>
                        </Button>
                    </template>
                </Column>
                <Column field="docDate" header="วันที่" sortable style="min-width: 9rem">
                    <template #body="{ data }">{{ formatDocumentDate(data.docDate) }}</template>
                </Column>
                <Column field="totalAmount" header="ยอดเงิน" sortable style="min-width: 10rem">
                    <template #body="{ data }">{{ formatAmount(data.totalAmount) }}</template>
                </Column>
                <Column field="status" header="สถานะ" sortable style="min-width: 11rem">
                    <template #body="{ data }">
                        <Tag :value="signingStatusLabel(data.status)" :severity="signingStatusSeverity(data.status)" />
                    </template>
                </Column>
                <Column header="PDF" style="min-width: 12rem">
                    <template #body="{ data }">
                        <Tag :value="documentPdfLabel(data)" :severity="documentPdfSeverity(data)" />
                    </template>
                </Column>
                <Column field="updatedAt" header="อัปเดตล่าสุด" sortable style="min-width: 13rem">
                    <template #body="{ data }">{{ formatThaiDateTime(data.updatedAt) }}</template>
                </Column>
                <Column header="จัดการ" :exportable="false" style="min-width: 13rem">
                    <template #body="{ data }">
                        <div class="flex gap-2 flex-wrap">
                            <Button icon="pi pi-sitemap" label="ดู Flow" size="small" @click="openFlowFromDocument(data)" />
                            <Button icon="pi pi-eye" rounded outlined severity="secondary" aria-label="ดูเอกสาร" @click="openSigningDocument(data)" />
                        </div>
                    </template>
                </Column>
            </DataTable>
        </div>

        <div v-else class="card">
            <div class="flex flex-col md:flex-row md:items-start justify-between gap-3 mb-4">
                <div class="min-w-0">
                    <div class="font-semibold text-lg">เอกสารใน Flow</div>
                    <div class="text-muted-color">
                        {{ root?.doc_no || docNo }}<span v-if="root?.doc_format_code"> · {{ root.doc_format_code }}</span>
                        <span v-if="flowRows.length"> · {{ flowRows.length }} เอกสาร</span>
                    </div>
                </div>
                <Tag v-if="graph?.truncated" value="ข้อมูลถูกจำกัดจำนวน" severity="warn" />
            </div>

            <div v-if="loading" class="empty-state">
                <i class="pi pi-spin pi-spinner"></i>
                <strong>กำลังโหลด Flow เอกสาร</strong>
            </div>

            <template v-else-if="flowRows.length">
                <Message v-for="warning in warnings" :key="`${warning.code}-${warning.doc_no || warning.message}`" severity="warn" class="mb-3">
                    {{ warning.message || 'พบความสัมพันธ์บางส่วนจาก SML' }}<span v-if="warning.doc_no">: {{ warning.doc_no }}</span>
                </Message>

                <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-3 mb-4">
                    <div class="rounded-lg border border-surface p-3">
                        <div class="text-muted-color">ทั้งหมด</div>
                        <div class="font-semibold text-2xl">{{ summary.total }}</div>
                    </div>
                    <div class="rounded-lg border border-surface p-3">
                        <div class="text-muted-color">มีใน PaperLess</div>
                        <div class="font-semibold text-2xl">{{ summary.paperless }}</div>
                    </div>
                    <div class="rounded-lg border border-surface p-3">
                        <div class="text-muted-color">เซ็นครบแล้ว</div>
                        <div class="font-semibold text-2xl">{{ summary.completed }}</div>
                    </div>
                    <div class="rounded-lg border border-surface p-3">
                        <div class="text-muted-color">ยังไม่ได้อัปโหลด</div>
                        <div class="font-semibold text-2xl">{{ summary.missing }}</div>
                    </div>
                </div>

                <Message severity="secondary" class="mb-4">
                    <span class="font-semibold">Flow:</span>
                    <span class="ml-2">{{ flowStrip }}</span>
                </Message>

                <DataTable :value="flowRows" dataKey="doc_no" paginator :rows="10" responsiveLayout="scroll" stripedRows>
                    <Column field="flowIndex" header="ลำดับ" style="width: 6rem" />
                    <Column field="doc_format_code" header="ชนิด" sortable style="min-width: 7rem" />
                    <Column field="doc_no" header="เลขที่เอกสาร" sortable style="min-width: 13rem">
                        <template #body="{ data }">
                            <Button :label="data.doc_no" link class="p-0 font-bold" @click="openInfo(data)" />
                            <div class="text-muted-color text-sm">{{ data.relationLabel }}</div>
                        </template>
                    </Column>
                    <Column header="คู่ค้า" style="min-width: 16rem">
                        <template #body="{ data }">{{ data.party_name || data.party_code || '-' }}</template>
                    </Column>
                    <Column field="doc_date" header="วันที่" sortable style="min-width: 9rem">
                        <template #body="{ data }">{{ formatDocumentDate(data.doc_date) }}</template>
                    </Column>
                    <Column field="total_amount" header="ยอดเงิน" sortable style="min-width: 10rem">
                        <template #body="{ data }">{{ formatAmount(data.total_amount) }}</template>
                    </Column>
                    <Column header="SML" style="min-width: 9rem">
                        <template #body="{ data }">
                            <Tag :value="smlLabel(data)" :severity="smlSeverity(data)" />
                        </template>
                    </Column>
                    <Column header="PaperLess" style="min-width: 12rem">
                        <template #body="{ data }">
                            <div class="flex flex-wrap gap-2">
                                <Tag :value="paperlessLabel(data)" :severity="paperlessSeverity(data)" />
                                <Tag v-if="data.matchCount > 1" value="พบหลายรายการ" severity="warn" />
                            </div>
                        </template>
                    </Column>
                    <Column header="PDF" style="min-width: 12rem">
                        <template #body="{ data }">
                            <Tag :value="currentPdfLabel(data)" :severity="currentPdfSeverity(data)" />
                        </template>
                    </Column>
                    <Column header="จัดการ" style="min-width: 16rem">
                        <template #body="{ data }">
                            <div class="flex flex-wrap gap-2">
                                <Button icon="pi pi-info-circle" rounded outlined severity="secondary" aria-label="ข้อมูล SML" @click="openInfo(data)" />
                                <Button v-if="data.canOpenPaperless" icon="pi pi-external-link" rounded outlined severity="secondary" aria-label="เปิดเอกสาร" @click="openPaperless(data)" />
                                <Button v-if="data.canViewCurrentPdf" icon="pi pi-file-pdf" rounded outlined severity="secondary" aria-label="ดู PDF ล่าสุด" @click="previewPDF(data, 'current')" />
                                <Button v-if="data.canViewSignedPdf" icon="pi pi-check-circle" rounded outlined severity="success" aria-label="ดู PDF ที่เซ็นครบแล้ว" @click="previewPDF(data, 'final')" />
                                <Button v-if="!data.paperlessStatus" label="ส่งเข้า PaperLess" icon="pi pi-send" size="small" @click="startUpload(data)" />
                            </div>
                        </template>
                    </Column>
                </DataTable>
            </template>

            <div v-else class="empty-state">
                <i class="pi pi-inbox"></i>
                <strong>ยังไม่พบเอกสารประกอบจาก SML</strong>
                <span>ลองตรวจเลขเอกสาร หรือค้นด้วยเลขเอกสารอื่นใน Flow เดียวกัน</span>
            </div>
        </div>

        <Dialog v-model:visible="detailVisible" modal header="ข้อมูลเอกสารจาก SML" :style="{ width: 'min(48rem, 94vw)' }">
            <div v-if="selectedNode" class="grid gap-3">
                <Message v-if="!selectedNode.paperlessStatus" severity="info">เอกสารนี้มีข้อมูลจาก SML แต่ยังไม่ได้อัปโหลดเข้า PaperLess</Message>
                <Message v-if="selectedNode.matchCount > 1" severity="warn">พบเอกสารนี้ใน PaperLess มากกว่า 1 รายการ ระบบเลือกเอกสารที่อัปเดตล่าสุดเป็นค่าเริ่มต้น</Message>
                <dl class="metadata-grid">
                    <dt>เลขที่เอกสาร</dt>
                    <dd>{{ selectedNode.doc_no }}</dd>
                    <dt>ชนิดเอกสาร</dt>
                    <dd>{{ selectedNode.doc_format_code || '-' }}</dd>
                    <dt>วันที่เอกสาร</dt>
                    <dd>{{ formatDocumentDate(selectedNode.doc_date) }}</dd>
                    <dt>คู่ค้า</dt>
                    <dd>{{ selectedNode.party_name || selectedNode.party_code || '-' }}</dd>
                    <dt>ยอดเงิน</dt>
                    <dd>{{ formatAmount(selectedNode.total_amount) }}</dd>
                    <dt>สถานะ SML</dt>
                    <dd>{{ smlLabel(selectedNode) }}</dd>
                    <dt>PaperLess</dt>
                    <dd>{{ paperlessLabel(selectedNode) }}</dd>
                    <dt>ความสัมพันธ์</dt>
                    <dd class="whitespace-pre-line">{{ technicalRelations(selectedNode) || '-' }}</dd>
                </dl>

                <DataTable v-if="selectedNode.paperlessMatches?.length" :value="selectedNode.paperlessMatches" responsiveLayout="scroll" stripedRows>
                    <Column field="docNo" header="เลขที่เอกสาร" style="min-width: 11rem" />
                    <Column header="สถานะ" style="min-width: 10rem">
                        <template #body="{ data }">
                            <Tag :value="signingStatusLabel(data.status)" :severity="signingStatusSeverity(data.status)" />
                        </template>
                    </Column>
                    <Column header="PDF" style="min-width: 11rem">
                        <template #body="{ data }">
                            <Tag :value="data.hasFinalPdf ? 'มี PDF เซ็นครบ' : data.hasCurrentPdf ? 'มี PDF ล่าสุด' : 'ยังไม่มี PDF'" :severity="data.hasFinalPdf ? 'success' : data.hasCurrentPdf ? 'info' : 'secondary'" />
                        </template>
                    </Column>
                    <Column header="อัปเดตล่าสุด" style="min-width: 12rem">
                        <template #body="{ data }">{{ formatThaiDateTime(data.updatedAt) }}</template>
                    </Column>
                    <Column header="จัดการ" style="width: 7rem">
                        <template #body="{ data }">
                            <Button icon="pi pi-external-link" rounded outlined severity="secondary" aria-label="เปิดเอกสาร PaperLess" @click="openDocument(data.id)" />
                        </template>
                    </Column>
                </DataTable>
            </div>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="detailVisible = false" />
            </template>
        </Dialog>

        <Dialog v-model:visible="pdfDialog" modal :header="pdfTitle" :style="{ width: 'min(72rem, 96vw)' }" @hide="clearPDFUrl">
            <div v-if="pdfLoading" class="empty-state">
                <i class="pi pi-spin pi-spinner"></i>
                <strong>กำลังโหลด PDF</strong>
            </div>
            <iframe v-else-if="pdfUrl" :src="pdfUrl" class="pdf-frame" title="PDF preview"></iframe>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="pdfDialog = false" />
            </template>
        </Dialog>
    </div>
</template>

<style scoped>
.empty-state {
    min-height: 10rem;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.6rem;
    text-align: center;
    color: var(--text-color-secondary);
}

.empty-state i {
    font-size: 1.75rem;
}

.metadata-grid {
    display: grid;
    grid-template-columns: 9rem minmax(0, 1fr);
    gap: 0.65rem 1rem;
    margin: 0;
}

.metadata-grid dt {
    color: var(--text-color-secondary);
}

.metadata-grid dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
}

.pdf-frame {
    width: 100%;
    height: min(72vh, 52rem);
    border: 1px solid var(--surface-border);
    border-radius: 8px;
}

@media (max-width: 640px) {
    .metadata-grid {
        grid-template-columns: 1fr;
    }
}
</style>
