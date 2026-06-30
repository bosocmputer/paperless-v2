<script setup>
import { formatDocumentDate, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import { computed, onMounted, ref } from 'vue';
import { useToast } from 'primevue/usetoast';

const props = defineProps({
    loader: { type: Function, default: null },
    admin: { type: Boolean, default: false },
    compact: { type: Boolean, default: false },
    recordEvent: { type: Function, default: null }
});

const emit = defineEmits(['open-document']);

const toast = useToast();
const loading = ref(false);
const loaded = ref(false);
const error = ref('');
const graph = ref(null);
const selectedNode = ref(null);
const detailVisible = ref(false);

const nodes = computed(() => graph.value?.nodes || []);
const edges = computed(() => graph.value?.edges || []);
const warnings = computed(() => graph.value?.warnings || []);
const rootDocNo = computed(() => graph.value?.root?.doc_no || graph.value?.root?.docNo || '');
const timelineItems = computed(() =>
    nodes.value.map((node) => ({
        ...node,
        incoming: edges.value.filter((edge) => edge.to_doc_no === node.doc_no),
        outgoing: edges.value.filter((edge) => edge.from_doc_no === node.doc_no),
        isRoot: node.doc_no === rootDocNo.value
    }))
);

onMounted(() => {
    void load();
});

async function load() {
    if (!props.loader || loading.value) return;
    loading.value = true;
    error.value = '';
    try {
        const result = await props.loader();
        graph.value = result.relatedDocuments || result.related_documents || result;
        loaded.value = true;
        props.recordEvent?.({ event: 'related_documents_load_success' });
    } catch (err) {
        error.value = err?.message || 'โหลดเอกสารประกอบไม่สำเร็จ';
        props.recordEvent?.({ event: 'related_documents_load_error', errorCode: 'related_documents_load_error' });
        toast.add({ severity: 'error', summary: 'โหลดเอกสารประกอบไม่สำเร็จ', detail: error.value, life: 3500 });
    } finally {
        loading.value = false;
    }
}

function openNode(node) {
    props.recordEvent?.({ event: 'related_document_click' });
    if (props.admin && node.canOpenPaperless && node.paperlessDocumentId) {
        emit('open-document', node.paperlessDocumentId);
        return;
    }
    selectedNode.value = node;
    detailVisible.value = true;
}

function sourceLabel(node) {
    return node.paperlessStatus ? 'เอกสารใน PaperLess' : 'ข้อมูลจาก SML';
}

function sourceSeverity(node) {
    if (node.paperlessStatus) return signingStatusSeverity(node.paperlessStatus);
    return 'secondary';
}

function statusLabel(node) {
    if (node.paperlessStatus) return signingStatusLabel(node.paperlessStatus);
    return node.is_lock_record === 1 ? 'Lock ใน SML' : 'ยังไม่ Lock';
}

function lockSeverity(node) {
    return node.is_lock_record === 1 ? 'success' : 'secondary';
}

function relationText(item) {
    const parts = [];
    item.incoming.forEach((edge) => parts.push(`มาจาก ${edge.from_doc_no}: ${edge.relation}`));
    item.outgoing.forEach((edge) => parts.push(`ต่อไป ${edge.to_doc_no}: ${edge.relation}`));
    return parts.join(' · ') || 'เอกสารต้นทาง';
}

function formatAmount(value) {
    const amount = Number(value || 0);
    return new Intl.NumberFormat('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount);
}
</script>

<template>
    <div class="related-documents" :class="{ compact }">
        <div class="flex items-center justify-between gap-3 mb-3">
            <div class="min-w-0">
                <div class="font-semibold">เอกสารประกอบ</div>
                <small class="text-muted-color">ข้อมูลความสัมพันธ์จาก SML</small>
            </div>
            <Button icon="pi pi-refresh" rounded outlined severity="secondary" aria-label="โหลดเอกสารประกอบใหม่" :loading="loading" @click="load" />
        </div>

        <Message v-if="error" severity="error" class="mb-3">
            {{ error }}
            <div class="mt-3">
                <Button size="small" label="ลองใหม่" icon="pi pi-refresh" severity="secondary" outlined @click="load" />
            </div>
        </Message>

        <Message v-for="warning in warnings" :key="`${warning.code}-${warning.doc_no}`" severity="warn" class="mb-2">
            พบความสัมพันธ์บางส่วนจาก SML: {{ warning.doc_no || warning.message }}
        </Message>

        <div v-if="loading && !loaded" class="related-empty">
            <i class="pi pi-spin pi-spinner"></i>
            <span>กำลังโหลดเอกสารประกอบ</span>
        </div>
        <div v-else-if="!loading && nodes.length === 0 && !error" class="related-empty">
            <i class="pi pi-inbox"></i>
            <span>ยังไม่พบเอกสารประกอบจาก SML</span>
        </div>

        <Timeline v-else :value="timelineItems" align="left" class="related-timeline">
            <template #opposite="{ item }">
                <div class="related-opposite">
                    <strong>{{ formatDocumentDate(item.doc_date) }}</strong>
                    <small>{{ item.doc_format_code || '-' }}</small>
                </div>
            </template>

            <template #marker="{ item }">
                <span class="related-marker" :class="{ root: item.isRoot }">
                    <i :class="item.isRoot ? 'pi pi-star-fill' : 'pi pi-file'"></i>
                </span>
            </template>

            <template #content="{ item }">
                <div class="related-content" :class="{ root: item.isRoot }">
                    <div class="flex items-start justify-between gap-3">
                        <div class="min-w-0">
                            <strong class="doc-no">{{ item.doc_no }}</strong>
                            <small>{{ item.doc_format_code || '-' }} · {{ item.party_name || item.party_code || '-' }}</small>
                        </div>
                        <div class="flex gap-2 flex-wrap justify-end">
                            <Tag v-if="item.isRoot" value="เอกสารนี้" severity="info" />
                            <Tag :value="sourceLabel(item)" :severity="sourceSeverity(item)" />
                            <Tag :value="statusLabel(item)" :severity="lockSeverity(item)" />
                        </div>
                    </div>
                    <p>{{ relationText(item) }}</p>
                    <Button
                        :label="props.admin && item.canOpenPaperless ? 'เปิดเอกสาร' : 'ดูข้อมูลจาก SML'"
                        :icon="props.admin && item.canOpenPaperless ? 'pi pi-external-link' : 'pi pi-info-circle'"
                        size="small"
                        severity="secondary"
                        outlined
                        @click="openNode(item)"
                    />
                </div>
            </template>
        </Timeline>

        <DataTable v-if="!compact && nodes.length" :value="nodes" responsiveLayout="scroll" stripedRows class="mt-4">
            <Column field="doc_no" header="เลขที่เอกสาร" style="min-width: 11rem">
                <template #body="{ data }">
                    <Button :label="data.doc_no" link class="p-0" @click="openNode(data)" />
                </template>
            </Column>
            <Column field="doc_format_code" header="ชนิด" style="min-width: 7rem" />
            <Column header="คู่ค้า" style="min-width: 14rem">
                <template #body="{ data }">{{ data.party_name || data.party_code || '-' }}</template>
            </Column>
            <Column header="วันที่" style="min-width: 8rem">
                <template #body="{ data }">{{ formatDocumentDate(data.doc_date) }}</template>
            </Column>
            <Column header="ยอดเงิน" style="min-width: 9rem">
                <template #body="{ data }">{{ formatAmount(data.total_amount) }}</template>
            </Column>
            <Column header="สถานะ" style="min-width: 10rem">
                <template #body="{ data }">
                    <Tag :value="statusLabel(data)" :severity="data.paperlessStatus ? signingStatusSeverity(data.paperlessStatus) : lockSeverity(data)" />
                </template>
            </Column>
        </DataTable>

        <Dialog v-model:visible="detailVisible" modal header="ข้อมูลเอกสารจาก SML" :style="{ width: 'min(34rem, 94vw)' }">
            <div v-if="selectedNode" class="grid gap-3">
                <Message v-if="!selectedNode.canOpenPaperless" severity="info">เอกสารนี้ยังไม่มี PDF หรือหน้ารายละเอียดใน PaperLess</Message>
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
                    <dt>แหล่งข้อมูล</dt>
                    <dd>{{ selectedNode.table }}</dd>
                </dl>
            </div>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="detailVisible = false" />
            </template>
        </Dialog>
    </div>
</template>

<style scoped>
.related-empty {
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

.related-opposite {
    display: grid;
    gap: 0.1rem;
    min-width: 6.5rem;
    text-align: right;
    color: var(--text-color-secondary);
}

.related-marker {
    width: 1.65rem;
    height: 1.65rem;
    border-radius: 999px;
    display: inline-grid;
    place-items: center;
    color: var(--text-color-secondary);
    background: var(--surface-hover);
    border: 2px solid var(--surface-card);
    font-size: 0.78rem;
}

.related-marker.root {
    color: var(--primary-contrast-color);
    background: var(--primary-color);
}

.related-content {
    min-width: 0;
    display: grid;
    gap: 0.55rem;
    padding: 0 0 1.1rem 0.35rem;
}

.related-content.root {
    border-left: 3px solid var(--primary-color);
    padding-left: 0.75rem;
}

.related-content p {
    margin: 0;
    color: var(--text-color-secondary);
}

.doc-no {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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

.related-timeline :deep(.p-timeline-event-opposite) {
    flex: 0 0 7.5rem;
    padding: 0 0.75rem 0 0;
}

.related-timeline :deep(.p-timeline-event-content) {
    padding-left: 0.75rem;
}

.related-timeline :deep(.p-timeline-event-marker) {
    border: 0;
}

@media (max-width: 640px) {
    .related-timeline :deep(.p-timeline-event-opposite) {
        flex: 0 0 5.75rem;
        padding-right: 0.5rem;
    }

    .related-opposite {
        min-width: 0;
        text-align: left;
    }

    .metadata-grid {
        grid-template-columns: 1fr;
    }
}
</style>
