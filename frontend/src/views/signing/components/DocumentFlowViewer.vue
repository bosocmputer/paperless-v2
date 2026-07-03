<script setup>
import { formatDocumentDate, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import { computed, ref } from 'vue';

const props = defineProps({
    graph: { type: Object, default: null },
    admin: { type: Boolean, default: false },
    compact: { type: Boolean, default: false },
    showTable: { type: Boolean, default: true }
});

const emit = defineEmits(['open-document', 'preview-pdf', 'node-click']);

const selectedNode = ref(null);
const detailVisible = ref(false);

const nodes = computed(() => props.graph?.nodes || []);
const edges = computed(() => props.graph?.edges || []);
const warnings = computed(() => props.graph?.warnings || []);
const rootDocNo = computed(() => props.graph?.root?.doc_no || props.graph?.root?.docNo || '');
const timelineItems = computed(() =>
    nodes.value.map((node) => ({
        ...node,
        incoming: edges.value.filter((edge) => edge.to_doc_no === node.doc_no),
        outgoing: edges.value.filter((edge) => edge.from_doc_no === node.doc_no),
        isRoot: node.doc_no === rootDocNo.value
    }))
);

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
    if (item.incoming.length === 0) return 'เอกสารต้นทาง';
    const from = [...new Set(item.incoming.map((edge) => edge.from_doc_no).filter(Boolean))];
    return from.length ? `ต่อจาก ${from.join(', ')}` : 'เอกสารที่เกี่ยวข้อง';
}

function technicalRelations(item) {
    const all = [...(item.incoming || []), ...(item.outgoing || [])];
    return all.map((edge) => `${edge.from_doc_no} → ${edge.to_doc_no} (${edge.source_table}.${edge.source_column})`).join('\n');
}

function formatAmount(value) {
    const amount = Number(value || 0);
    return new Intl.NumberFormat('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount);
}

function openInfo(node) {
    emit('node-click', node);
    selectedNode.value = node;
    detailVisible.value = true;
}

function openPaperless(node) {
    emit('node-click', node);
    if (node.canOpenPaperless && node.paperlessDocumentId) emit('open-document', node.paperlessDocumentId);
}

function previewPDF(node, version) {
    emit('node-click', node);
    emit('preview-pdf', { node, version, url: version === 'final' ? node.signedPdfUrl : node.currentPdfUrl });
}

function previewBestPDF(node) {
    emit('node-click', node);
    if (node.canViewCurrentPdf || node.hasCurrentPdf || node.currentPdfUrl) {
        emit('preview-pdf', { node, version: 'current', url: node.currentPdfUrl });
        return;
    }
    emit('preview-pdf', { node, version: 'final', url: node.signedPdfUrl });
}
</script>

<template>
    <div class="document-flow-viewer" :class="{ compact }">
        <Message v-for="warning in warnings" :key="`${warning.code}-${warning.doc_no || warning.message}`" severity="warn" class="mb-3">
            {{ warning.message || 'พบความสัมพันธ์บางส่วนจาก SML' }}<span v-if="warning.doc_no">: {{ warning.doc_no }}</span>
        </Message>

        <div v-if="nodes.length === 0" class="flow-empty">
            <i class="pi pi-inbox"></i>
            <span>ยังไม่พบเอกสารประกอบจาก SML</span>
        </div>

        <Timeline v-else :value="timelineItems" align="left" class="flow-timeline">
            <template #opposite="{ item }">
                <div class="flow-opposite">
                    <strong>{{ formatDocumentDate(item.doc_date) }}</strong>
                    <small>{{ item.doc_format_code || '-' }}</small>
                </div>
            </template>

            <template #marker="{ item }">
                <span class="flow-marker" :class="{ root: item.isRoot, paperless: item.paperlessStatus }">
                    <i :class="item.isRoot ? 'pi pi-star-fill' : item.paperlessStatus ? 'pi pi-file-check' : 'pi pi-file'"></i>
                </span>
            </template>

            <template #content="{ item }">
                <div class="flow-content" :class="{ root: item.isRoot }">
                    <div class="flex flex-col lg:flex-row lg:items-start justify-between gap-3">
                        <div class="min-w-0">
                            <Button :label="item.doc_no" link class="p-0 font-semibold text-lg text-left max-w-full" @click="previewBestPDF(item)" />
                            <div class="text-muted-color">{{ item.doc_format_code || '-' }} · {{ item.party_name || item.party_code || '-' }}</div>
                            <div class="text-muted-color text-sm mt-1">{{ relationText(item) }}</div>
                        </div>
                        <div class="flex gap-2 flex-wrap lg:justify-end">
                            <Tag v-if="item.isRoot" value="เอกสารที่ค้นหา" severity="info" />
                            <Tag :value="sourceLabel(item)" :severity="sourceSeverity(item)" />
                            <Tag :value="statusLabel(item)" :severity="item.paperlessStatus ? signingStatusSeverity(item.paperlessStatus) : lockSeverity(item)" />
                            <Tag v-if="item.matchCount > 1" :value="`${item.matchCount} รายการใน PaperLess`" severity="warn" />
                        </div>
                    </div>

                    <div class="flex flex-wrap gap-2 mt-3">
                        <Button label="ข้อมูล SML" icon="pi pi-info-circle" size="small" severity="secondary" outlined @click="openInfo(item)" />
                        <Button
                            v-if="admin && item.canOpenPaperless"
                            label="เปิด PaperLess"
                            icon="pi pi-external-link"
                            size="small"
                            severity="secondary"
                            outlined
                            @click="openPaperless(item)"
                        />
                        <Button
                            v-if="admin && item.canViewCurrentPdf"
                            label="ดู PDF ล่าสุด"
                            icon="pi pi-file-pdf"
                            size="small"
                            severity="secondary"
                            outlined
                            @click="previewPDF(item, 'current')"
                        />
                        <Button
                            v-if="admin"
                            label="ดูหลักฐานการลงนาม"
                            icon="pi pi-shield"
                            size="small"
                            :disabled="!item.canViewSignedPdf"
                            :severity="item.canViewSignedPdf ? 'success' : 'secondary'"
                            outlined
                            @click="previewPDF(item, 'final')"
                        />
                    </div>
                    <small v-if="admin && !item.canViewSignedPdf" class="text-muted-color block mt-2">ยังไม่มีหลักฐานการลงนาม</small>
                </div>
            </template>
        </Timeline>

        <DataTable v-if="showTable && !compact && nodes.length" :value="nodes" responsiveLayout="scroll" stripedRows class="mt-4">
            <Column field="doc_no" header="เลขที่เอกสาร" style="min-width: 11rem">
                <template #body="{ data }">
                    <Button :label="data.doc_no" link class="p-0" @click="previewBestPDF(data)" />
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
            <Column header="PaperLess" style="min-width: 12rem">
                <template #body="{ data }">
                    <Tag :value="data.paperlessStatus ? signingStatusLabel(data.paperlessStatus) : 'ยังไม่มีใน PaperLess'" :severity="data.paperlessStatus ? signingStatusSeverity(data.paperlessStatus) : 'secondary'" />
                </template>
            </Column>
            <Column header="PDF" style="min-width: 10rem">
                <template #body="{ data }">
                    <div class="flex gap-2 flex-wrap">
                        <Tag :value="data.hasCurrentPdf ? 'มี PDF ล่าสุด' : 'ไม่มี PDF'" :severity="data.hasCurrentPdf ? 'info' : 'secondary'" />
                        <Tag :value="data.hasFinalPdf ? 'มีหลักฐานการลงนาม' : 'ยังไม่มีหลักฐาน'" :severity="data.hasFinalPdf ? 'success' : 'secondary'" />
                    </div>
                </template>
            </Column>
            <Column header="จัดการ" style="min-width: 13rem">
                <template #body="{ data }">
                    <div class="flex gap-2 flex-wrap">
                        <Button icon="pi pi-info-circle" rounded outlined severity="secondary" aria-label="ดูข้อมูล SML" @click="openInfo(data)" />
                        <Button v-if="admin && data.canOpenPaperless" icon="pi pi-external-link" rounded outlined severity="secondary" aria-label="เปิด PaperLess" @click="openPaperless(data)" />
                        <Button v-if="admin && data.canViewSignedPdf" icon="pi pi-shield" rounded outlined severity="success" aria-label="ดูหลักฐานการลงนาม" @click="previewPDF(data, 'final')" />
                    </div>
                </template>
            </Column>
        </DataTable>

        <Dialog v-model:visible="detailVisible" modal header="ข้อมูลเอกสารจาก SML" :style="{ width: 'min(42rem, 94vw)' }">
            <div v-if="selectedNode" class="grid gap-3">
                <Message v-if="!selectedNode.paperlessStatus" severity="info">เอกสารนี้มีข้อมูลจาก SML แต่ยังไม่มี PDF ใน PaperLess</Message>
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
                    <dd>{{ selectedNode.is_lock_record === 1 ? 'Lock แล้ว' : 'ยังไม่ Lock' }}</dd>
                    <dt>แหล่งข้อมูล</dt>
                    <dd>{{ selectedNode.table }}</dd>
                    <dt>ความสัมพันธ์</dt>
                    <dd class="whitespace-pre-line">{{ technicalRelations(selectedNode) || '-' }}</dd>
                </dl>
            </div>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="detailVisible = false" />
            </template>
        </Dialog>
    </div>
</template>

<style scoped>
.flow-empty {
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

.flow-opposite {
    display: grid;
    gap: 0.1rem;
    min-width: 6.5rem;
    text-align: right;
    color: var(--text-color-secondary);
}

.flow-marker {
    width: 1.8rem;
    height: 1.8rem;
    border-radius: 999px;
    display: inline-grid;
    place-items: center;
    color: var(--text-color-secondary);
    background: var(--surface-hover);
    border: 2px solid var(--surface-card);
    font-size: 0.8rem;
}

.flow-marker.root,
.flow-marker.paperless {
    color: var(--primary-contrast-color);
    background: var(--primary-color);
}

.flow-content {
    min-width: 0;
    display: grid;
    gap: 0.35rem;
    padding: 0 0 1.25rem 0.35rem;
}

.flow-content.root {
    border-left: 3px solid var(--primary-color);
    padding-left: 0.75rem;
}

.flow-timeline :deep(.p-timeline-event-opposite) {
    flex: 0 0 7.5rem;
    padding: 0 0.75rem 0 0;
}

.flow-timeline :deep(.p-timeline-event-content) {
    padding-left: 0.75rem;
}

.flow-timeline :deep(.p-timeline-event-marker) {
    border: 0;
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

@media (max-width: 640px) {
    .flow-timeline :deep(.p-timeline-event-opposite) {
        flex: 0 0 5.5rem;
        padding-right: 0.5rem;
    }

    .flow-opposite {
        min-width: 0;
        text-align: left;
    }

    .metadata-grid {
        grid-template-columns: 1fr;
    }
}
</style>
