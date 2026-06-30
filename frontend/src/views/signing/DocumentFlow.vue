<script setup>
import { api } from '@/services/api';
import DocumentFlowViewer from '@/views/signing/components/DocumentFlowViewer.vue';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();

const docNo = ref(String(route.query.doc_no || '').toUpperCase());
const docFormatCode = ref(String(route.query.doc_format_code || '').toUpperCase());
const loading = ref(false);
const searched = ref(false);
const error = ref('');
const graph = ref(null);
const pdfDialog = ref(false);
const pdfLoading = ref(false);
const pdfUrl = ref('');
const pdfTitle = ref('');

const sessionId = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
const openedAt = Date.now();

const nodes = computed(() => graph.value?.nodes || []);
const root = computed(() => graph.value?.root || null);

onMounted(() => {
    recordEvent('document_flow_open');
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
    recordEvent('document_flow_search');
    try {
        const result = await api.getAdminDocumentFlow({ docNo: docNo.value, docFormatCode: docFormatCode.value, depth: 3 });
        graph.value = result.documentFlow;
        recordEvent('document_flow_load_success', { nodeCount: nodes.value.length, docFormatCode: root.value?.doc_format_code || docFormatCode.value });
        router.replace({
            name: 'document-flow',
            query: {
                doc_no: docNo.value,
                ...(docFormatCode.value ? { doc_format_code: docFormatCode.value } : {})
            }
        });
    } catch (err) {
        error.value = err?.message || 'ไม่สามารถโหลด Flow เอกสารได้';
        recordEvent('document_flow_load_error', { errorCode: err?.payload?.code || 'document_flow_load_error' });
        toast.add({ severity: 'error', summary: 'โหลด Flow เอกสารไม่สำเร็จ', detail: error.value, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openDocument(documentId) {
    if (!documentId) return;
    recordEvent('document_flow_node_click', { nodeCount: nodes.value.length });
    router.push({ name: 'signing-document-detail', params: { id: documentId } });
}

async function previewPDF({ node, version, url }) {
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
                    <div class="text-muted-color whitespace-nowrap truncate">ค้นเลขเอกสารจาก SML และเปิด PDF ที่มีใน PaperLess</div>
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
                <Button label="ลองใหม่" icon="pi pi-refresh" severity="secondary" outlined @click="search" />
            </div>
        </Message>

        <div v-if="!searched" class="card">
            <div class="empty-state">
                <i class="pi pi-sitemap"></i>
                <strong>กรอกเลขเอกสารเพื่อดู Flow จาก SML</strong>
                <span>ระบบจะแสดงเอกสารประกอบทั้งหมด และบอกว่าเอกสารไหนมี PDF ใน PaperLess</span>
            </div>
        </div>

        <div v-else class="card">
            <div class="flex flex-col md:flex-row md:items-start justify-between gap-3 mb-4">
                <div class="min-w-0">
                    <div class="font-semibold text-lg">Flow เอกสาร</div>
                    <div class="text-muted-color">
                        {{ root?.doc_no || docNo }}<span v-if="root?.doc_format_code"> · {{ root.doc_format_code }}</span>
                        <span v-if="nodes.length"> · {{ nodes.length }} เอกสารใน Flow</span>
                    </div>
                </div>
                <Tag v-if="graph?.truncated" value="ข้อมูลถูกจำกัดจำนวน" severity="warn" />
            </div>

            <div v-if="loading" class="empty-state">
                <i class="pi pi-spin pi-spinner"></i>
                <strong>กำลังโหลด Flow เอกสาร</strong>
            </div>
            <DocumentFlowViewer v-else :graph="graph" admin @open-document="openDocument" @preview-pdf="previewPDF" @node-click="recordEvent('document_flow_node_click')" />
        </div>

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

.pdf-frame {
    width: 100%;
    height: min(72vh, 52rem);
    border: 1px solid var(--surface-border);
    border-radius: 8px;
}
</style>
