<script setup>
import { api } from '@/services/api';
import DocumentFlowViewer from '@/views/signing/components/DocumentFlowViewer.vue';
import ReadOnlyPdfDialog from '@/views/signing/components/ReadOnlyPdfDialog.vue';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useToast } from 'primevue/usetoast';

const props = defineProps({
    visible: { type: Boolean, default: false },
    document: { type: Object, default: null },
    loader: { type: Function, default: null },
    recordEvent: { type: Function, default: null },
    admin: { type: Boolean, default: true },
    openPdfOnSelect: { type: Boolean, default: true }
});

const emit = defineEmits(['update:visible', 'open-document']);

const toast = useToast();

const flowLoading = ref(false);
const flowError = ref('');
const flowNotice = ref('');
const flowGraph = ref(null);
const flowDocument = ref(null);
const flowRequestSeq = ref(0);
const pdfDialog = ref(false);
const pdfUrl = ref('');
const pdfTitle = ref('');
const pdfActionUrl = ref('');
const flowCache = new Map();
const flowSessionId = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
const openedAt = ref(Date.now());

const dialogVisible = computed({
    get: () => props.visible,
    set: (value) => emit('update:visible', value)
});

const requestedDocumentKey = computed(() => {
    const doc = normalizeFlowDocument(props.document);
    if (!doc.docNo) return '';
    return `${doc.docFormatCode}:${doc.docNo}`;
});

const flowHeader = computed(() => {
    const doc = flowDocument.value;
    if (!doc?.docNo) return 'Flow เอกสาร';
    const party = doc.partyName || doc.partyCode || '';
    return `${doc.docNo} ~ ${doc.docFormatCode || '-'}${party ? ` · ${party}` : ''}`;
});
const flowNodeCount = computed(() => flowGraph.value?.nodes?.length || 0);
const flowStatusSummary = computed(() => {
    const summary = { missing: 0, inProgress: 0, completed: 0 };
    for (const node of flowGraph.value?.nodes || []) {
        if (node.paperlessStatus === 'completed') summary.completed += 1;
        else if (node.paperlessStatus || node.canViewCurrentPdf || node.hasCurrentPdf || node.currentPdfUrl) summary.inProgress += 1;
        else summary.missing += 1;
    }
    return summary;
});

watch(
    () => [props.visible, requestedDocumentKey.value],
    ([visible]) => {
        if (!visible) return;
        openRequestedDocument();
    },
    { immediate: true }
);

onBeforeUnmount(() => {
    clearPDFPreview();
});

function normalizeFlowDocument(doc = {}) {
    return {
        docNo: String(doc?.docNo || doc?.doc_no || '').trim().toUpperCase(),
        docFormatCode: String(doc?.docFormatCode || doc?.doc_format_code || '').trim().toUpperCase(),
        partyName: doc?.partyName || doc?.party_name || '',
        partyCode: doc?.partyCode || doc?.party_code || ''
    };
}

function openRequestedDocument() {
    const doc = normalizeFlowDocument(props.document);
    if (!doc.docNo) return;
    openedAt.value = Date.now();
    clearPDFPreview();
    flowDocument.value = doc;
    flowError.value = '';
    flowNotice.value = '';
    void loadDocumentFlow(doc);
}

async function loadDocumentFlow(doc = flowDocument.value, options = {}) {
    const normalizedDoc = normalizeFlowDocument(doc);
    if (!normalizedDoc.docNo) return;
    const cacheKey = `${normalizedDoc.docFormatCode}:${normalizedDoc.docNo}`;
    const requestSeq = flowRequestSeq.value + 1;
    flowRequestSeq.value = requestSeq;
    flowError.value = '';
    flowNotice.value = '';

    if (!options.force && flowCache.has(cacheKey)) {
        flowGraph.value = flowCache.get(cacheKey);
        flowLoading.value = false;
        touchFlowCache(cacheKey, flowGraph.value);
        recordFlowEvent('document_flow_load_success', { docFormatCode: normalizedDoc.docFormatCode, nodeCount: flowNodeCount.value });
        return;
    }

    flowLoading.value = true;
    flowGraph.value = null;
    recordFlowEvent('document_flow_search', { docFormatCode: normalizedDoc.docFormatCode });

    try {
        const result = props.loader
            ? await props.loader(normalizedDoc, { depth: 3 })
            : await api.getAdminDocumentFlow({ docNo: normalizedDoc.docNo, docFormatCode: normalizedDoc.docFormatCode, depth: 3 });
        if (requestSeq !== flowRequestSeq.value) return;
        const graph = result.relatedDocuments || result.related_documents || result.documentFlow || result.document_flow || result;
        flowGraph.value = graph;
        touchFlowCache(cacheKey, graph);
        recordFlowEvent('document_flow_load_success', { docFormatCode: normalizedDoc.docFormatCode, nodeCount: graph?.nodes?.length || 0 });
    } catch (err) {
        if (requestSeq !== flowRequestSeq.value) return;
        flowError.value = userFacingFlowError(err);
        recordFlowEvent('document_flow_load_error', { docFormatCode: normalizedDoc.docFormatCode, errorCode: err?.payload?.code || 'document_flow_load_error' });
        toast.add({ severity: 'error', summary: 'โหลด Flow เอกสารไม่สำเร็จ', detail: flowError.value, life: 4000 });
    } finally {
        if (requestSeq === flowRequestSeq.value) flowLoading.value = false;
    }
}

function handleFlowNodeClick(node = {}) {
    recordFlowEvent('document_flow_node_click', { docFormatCode: node.doc_format_code || node.docFormatCode || flowDocument.value?.docFormatCode || '', nodeCount: flowNodeCount.value });
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
    flowError.value = '';
    flowNotice.value = '';
    pdfDialog.value = false;
    clearPDFPreview();
    dialogVisible.value = false;
}

async function previewFlowPDF(payload = {}) {
    const node = payload.node || {};
    const version = payload.version === 'final' ? 'final' : 'current';
    const rawUrl = payload.url || (version === 'final' ? node.signedPdfUrl : node.currentPdfUrl);
    const url = api.withPDFCacheKey(rawUrl, api.signingDocumentPDFCacheKey(node, version));
    const docNo = node.doc_no || node.docNo || 'เอกสารนี้';
    if (!url) {
        pdfActionUrl.value = '';
        flowNotice.value = `${docNo} ยังไม่มีเอกสาร PDF ใน PaperLess`;
        toast.add({ severity: 'info', summary: 'ยังไม่มีเอกสาร PDF', detail: flowNotice.value, life: 3000 });
        return;
    }

    flowNotice.value = '';
    pdfUrl.value = url;
    pdfTitle.value = `${docNo} · ${version === 'final' ? 'หลักฐานการลงนาม' : 'เอกสารใน PaperLess'}`;
    pdfActionUrl.value = paperlessDocumentUrl(node);
    pdfDialog.value = true;
    recordFlowEvent('document_flow_pdf_open', { docFormatCode: node.doc_format_code || flowDocument.value?.docFormatCode || '', nodeCount: flowNodeCount.value });
}

function clearPDFPreview() {
    pdfUrl.value = '';
    pdfTitle.value = '';
    pdfActionUrl.value = '';
}

function paperlessDocumentUrl(node = {}) {
    const id = node.paperlessDocumentId || node.paperless_document_id || '';
    if (!id || !node.canOpenPaperless) return '';
    const status = node.paperlessStatus || node.status || '';
    const queue = status === 'completed' ? 'history' : status === 'draft' ? 'drafts' : 'active';
    return `/signing/documents/${encodeURIComponent(id)}?from_queue=${encodeURIComponent(queue)}`;
}

function userFacingFlowError(err) {
    const code = err?.payload?.code || '';
    if (code === 'sml_document_not_found') return 'ไม่พบเลขเอกสารนี้ใน SML';
    if (code === 'sml_unavailable' || code === 'sml_not_configured') return 'เชื่อมต่อ SML ไม่สำเร็จ กรุณาลองใหม่';
    return err?.message || 'ไม่สามารถโหลด Flow เอกสารได้ กรุณาลองใหม่';
}

function recordFlowEvent(event, extra = {}) {
    if (props.recordEvent) {
        const signerEvent = {
            document_flow_load_success: 'related_documents_load_success',
            document_flow_load_error: 'related_documents_load_error',
            document_flow_node_click: 'related_document_click',
            document_flow_pdf_open: 'related_document_click'
        }[event];
        if (signerEvent) {
            props.recordEvent({ event: signerEvent, errorCode: extra.errorCode || '' });
        }
        return;
    }
    api.recordDocumentFlowEvent({
        event,
        sessionId: flowSessionId,
        docFormatCode: extra.docFormatCode || flowDocument.value?.docFormatCode || '',
        elapsedMs: Date.now() - openedAt.value,
        nodeCount: extra.nodeCount || flowNodeCount.value,
        errorCode: extra.errorCode || ''
    }).catch(() => {});
}
</script>

<template>
    <Dialog
        v-model:visible="dialogVisible"
        modal
        :draggable="false"
        class="document-flow-dialog"
        :style="{ width: 'min(1280px, 96vw)', height: 'min(820px, 90vh)' }"
        :breakpoints="{ '640px': '100vw' }"
        :header="flowHeader"
        @hide="closeFlowDialog"
    >
        <div class="flow-dialog-layout">
            <div class="flow-dialog-toolbar flex flex-col md:flex-row md:items-center justify-between gap-2">
                <div class="flex flex-wrap items-center gap-2 md:justify-end">
                    <Tag v-if="flowNodeCount" :value="`${flowNodeCount} เอกสาร`" severity="info" />
                    <Tag v-if="flowStatusSummary.missing" :value="`ยังไม่เข้า ${flowStatusSummary.missing}`" severity="danger" />
                    <Tag v-if="flowStatusSummary.inProgress" :value="`กำลังเซ็น ${flowStatusSummary.inProgress}`" severity="warn" />
                    <Tag v-if="flowStatusSummary.completed" :value="`ครบแล้ว ${flowStatusSummary.completed}`" severity="success" />
                </div>
                <div class="flex flex-wrap items-center gap-2 md:justify-end">
                    <Button icon="pi pi-refresh" label="โหลดใหม่" severity="secondary" outlined :loading="flowLoading" @click="loadDocumentFlow(flowDocument, { force: true })" />
                </div>
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
            <div v-else class="flow-dialog-viewer">
                <DocumentFlowViewer
                    :graph="flowGraph"
                    :admin="admin"
                    show-table
                    :open-pdf-on-select="openPdfOnSelect"
                    :show-detail-panel="false"
                    @open-document="(documentId) => emit('open-document', documentId)"
                    @node-click="handleFlowNodeClick"
                    @preview-pdf="previewFlowPDF"
                />
            </div>
        </div>
    </Dialog>

    <ReadOnlyPdfDialog v-model:visible="pdfDialog" :url="pdfUrl" :title="pdfTitle" :action-url="pdfActionUrl" action-label="เปิด PaperLess" full-height />
</template>

<style scoped>
.flow-dialog-layout {
    display: flex;
    height: 100%;
    min-height: 0;
    flex-direction: column;
    gap: 0.55rem;
}

.flow-dialog-toolbar {
    flex: 0 0 auto;
    padding-bottom: 0.55rem;
    border-bottom: 1px solid var(--surface-border);
}

.flow-dialog-viewer {
    min-height: 0;
    flex: 1 1 auto;
    overflow: hidden;
    padding: 0.05rem 0 0.1rem;
}

.flow-loading {
    min-height: 18rem;
    flex: 1 1 auto;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.6rem;
    color: var(--text-color-secondary);
}

:global(.document-flow-dialog .p-dialog-content) {
    height: calc(100% - 4.25rem);
    display: flex;
    flex-direction: column;
    padding-top: 0.75rem;
}

@media (max-width: 640px) {
    :global(.document-flow-dialog.p-dialog) {
        width: 100vw !important;
        max-width: 100vw !important;
        height: 100dvh !important;
        max-height: 100dvh;
        margin: 0;
        border-radius: 0;
    }

    :global(.document-flow-dialog .p-dialog-content) {
        height: calc(100dvh - 4.25rem);
        padding-inline: 0.75rem;
    }

    .flow-dialog-layout {
        height: 100%;
        max-height: none;
    }
}
</style>
