<script setup>
import { formatDocumentDate, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import { computed, ref } from 'vue';

const props = defineProps({
    graph: { type: Object, default: null },
    admin: { type: Boolean, default: false },
    compact: { type: Boolean, default: false },
    showTable: { type: Boolean, default: true },
    tableFirst: { type: Boolean, default: false }
});

const emit = defineEmits(['open-document', 'preview-pdf', 'node-click']);

const selectedNode = ref(null);
const detailVisible = ref(false);
const activeNodeKey = ref('');

const nodes = computed(() => props.graph?.nodes || []);
const edges = computed(() => props.graph?.edges || []);
const warnings = computed(() => props.graph?.warnings || []);
const rootDocNo = computed(() => props.graph?.root?.doc_no || props.graph?.root?.docNo || '');
const useTableFirst = computed(() => props.tableFirst || props.compact || nodes.value.length > 5);
const timelineItems = computed(() =>
    nodes.value.map((node) => ({
        ...node,
        incoming: edges.value.filter((edge) => edge.to_doc_no === node.doc_no),
        outgoing: edges.value.filter((edge) => edge.from_doc_no === node.doc_no),
        isRoot: node.doc_no === rootDocNo.value
    }))
);
const selectedFlowNode = computed(() => {
    const active = timelineItems.value.find((node) => flowNodeKey(node) === activeNodeKey.value);
    if (active) return active;
    return timelineItems.value.find((node) => node.isRoot) || timelineItems.value[0] || null;
});
const selectedFlowNodeKey = computed(() => (selectedFlowNode.value ? flowNodeKey(selectedFlowNode.value) : ''));
const missingPaperLessPdfMessage = 'เอกสารนี้มีข้อมูลจาก SML แต่ยังไม่มี PDF ใน PaperLess';

function flowNodeKey(node = {}) {
    return `${node.doc_format_code || node.docFormatCode || 'doc'}-${node.doc_no || node.docNo || ''}`;
}

function selectFlowNode(node) {
    activeNodeKey.value = flowNodeKey(node);
    emit('node-click', node);
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

function referenceStatusMeta(node = {}) {
    if (node.paperlessStatus === 'completed') {
        return { label: 'เซ็นครบแล้ว', severity: 'success', icon: 'pi pi-check-circle' };
    }
    if (node.paperlessStatus || canPreviewCurrentPDF(node)) {
        return { label: 'กำลังเซ็น/ยังไม่เสร็จ', severity: 'warn', icon: 'pi pi-clock' };
    }
    return { label: 'ยังไม่เข้า PaperLess', severity: 'danger', icon: 'pi pi-exclamation-triangle' };
}

function referenceStatusClass(node = {}) {
    if (node.paperlessStatus === 'completed') return 'completed';
    if (node.paperlessStatus || canPreviewCurrentPDF(node)) return 'in-progress';
    return 'missing';
}

function lockSeverity(node) {
    return node.is_lock_record === 1 ? 'success' : 'secondary';
}

function relationText(item) {
    if (item.incoming.length === 0) return 'เอกสารต้นทาง';
    const from = [...new Set(item.incoming.map((edge) => edge.from_doc_no).filter(Boolean))];
    return from.length ? `ต่อจาก ${from.join(', ')}` : 'เอกสารที่เกี่ยวข้อง';
}

function documentTypeLabel(node) {
    const transFlagName = node.trans_flag_name_th || node.transFlagNameTh || node.trans_flag_name_en || node.transFlagNameEn || '';
    if (transFlagName) return transFlagName;
    const name = node.doc_format_name || node.docFormatName || '';
    if (name) return name;
    const code = String(node.doc_format_code || node.docFormatCode || '').toUpperCase();
    const labels = {
        PO: 'ใบสั่งซื้อ',
        PA: 'ซื้อ',
        PUP: 'ซื้อ',
        PB: 'ใบรับวางบิล',
        PBV: 'ใบรับวางบิล',
        PV: 'จ่ายชำระหนี้',
        PVV: 'จ่ายชำระหนี้',
        INV: 'ใบขาย'
    };
    return labels[code] || code || 'เอกสาร';
}

function sourceDocNo(node) {
    const explicit = node.source_doc_no || node.sourceDocNo || '';
    if (explicit) return explicit;
    const incoming = node.incoming || [];
    if (!incoming.length) return node.doc_no || '-';
    return incoming
        .map((edge) => edge.from_doc_no)
        .filter(Boolean)
        .join(', ');
}

function technicalRelations(item) {
    const all = [...(item.incoming || []), ...(item.outgoing || [])];
    return all.map((edge) => `${edge.from_doc_no} → ${edge.to_doc_no} (${edge.source_table}.${edge.source_column})`).join('\n');
}

function normalizeTime(value) {
    const text = String(value || '').trim();
    if (!text) return '';
    const match = text.match(/^(\d{1,2}):(\d{2})/);
    if (!match) return text;
    return `${match[1].padStart(2, '0')}:${match[2]}`;
}

function formatDocumentDateTime(node) {
    const dateText = formatDocumentDate(node.doc_date);
    const timeText = normalizeTime(node.doc_time || node.docTime);
    if (dateText === '-') return timeText || '-';
    return timeText ? `${dateText} ${timeText}` : dateText;
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

function canPreviewCurrentPDF(node = {}) {
    return !!(node.canViewCurrentPdf || node.hasCurrentPdf || node.currentPdfUrl);
}

function isMissingPaperLessPdf(node = {}) {
    return !canPreviewCurrentPDF(node);
}

function previewPDF(node, version) {
    emit('node-click', node);
    emit('preview-pdf', { node, version, url: version === 'final' ? node.signedPdfUrl : node.currentPdfUrl });
}

function previewCurrentPDF(node) {
    previewPDF(node, 'current');
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

        <Timeline v-else-if="!useTableFirst" :value="timelineItems" align="alternate" :data-key="'doc_no'" class="document-flow-timeline" :class="{ compact }">
            <template #opposite="{ item }">
                <small class="text-muted-color">{{ formatDocumentDateTime(item) }}</small>
            </template>
            <template #marker="{ item, index }">
                <button
                    type="button"
                    class="flex w-8 h-8 items-center justify-center rounded-full z-10 shadow-sm border transition-colors"
                    :class="selectedFlowNodeKey === flowNodeKey(item) || item.isRoot ? 'bg-primary text-primary-contrast border-primary' : 'bg-surface-0 text-muted-color border-surface-300 dark:bg-surface-900 dark:border-surface-600'"
                    :aria-label="`เลือกเอกสาร ${item.doc_no || ''}`"
                    @click="selectFlowNode(item)"
                >
                    {{ index + 1 }}
                </button>
            </template>
            <template #content="{ item }">
                <div
                    class="flow-card-hitarea"
                    role="button"
                    tabindex="0"
                    :aria-label="`เลือกเอกสาร ${item.doc_no || ''}`"
                    @click="selectFlowNode(item)"
                    @keydown.enter.prevent="selectFlowNode(item)"
                    @keydown.space.prevent="selectFlowNode(item)"
                >
                    <Card class="mt-4 flow-document-card" :class="{ selected: selectedFlowNodeKey === flowNodeKey(item), root: item.isRoot }">
                        <template #title>
                            <div class="flex items-center justify-between gap-2">
                                <span class="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">{{ documentTypeLabel(item) }}</span>
                                <Tag v-if="item.isRoot" value="เอกสารที่ค้นหา" severity="info" />
                            </div>
                        </template>
                        <template #subtitle>
                            <span>{{ item.doc_no || '-' }} · {{ relationText(item) }}</span>
                        </template>
                        <template #content>
                            <dl class="flow-metadata-grid">
                                <dt>เลขที่เอกสาร</dt>
                                <dd class="flow-doc-no">{{ item.doc_no || '-' }}</dd>
                                <dt>วันที่-เวลา</dt>
                                <dd>{{ formatDocumentDateTime(item) }}</dd>
                                <dt>มูลค่าเอกสาร</dt>
                                <dd>{{ formatAmount(item.total_amount) }}</dd>
                                <dt>เอกสารต้นทาง</dt>
                                <dd>{{ sourceDocNo(item) }}</dd>
                            </dl>
                            <Message v-if="isMissingPaperLessPdf(item)" severity="error" class="mt-3" :closable="false">
                                {{ missingPaperLessPdfMessage }}
                            </Message>
                            <div class="flex flex-wrap gap-2 mt-3">
                                <Tag :value="sourceLabel(item)" :severity="sourceSeverity(item)" />
                                <Tag :value="statusLabel(item)" :severity="item.paperlessStatus ? signingStatusSeverity(item.paperlessStatus) : lockSeverity(item)" />
                                <Tag v-if="item.matchCount > 1" :value="`${item.matchCount} รายการใน PaperLess`" severity="warn" />
                            </div>
                            <div v-if="admin" class="flow-card-actions" @click.stop>
                                <Button icon="pi pi-info-circle" label="ข้อมูล" size="small" outlined severity="secondary" @click="openInfo(item)" />
                                <Button v-if="canPreviewCurrentPDF(item)" icon="pi pi-file-pdf" label="ดูเอกสาร" size="small" outlined severity="secondary" @click="previewCurrentPDF(item)" />
                                <Button v-if="item.canOpenPaperless" icon="pi pi-external-link" label="รายละเอียด" size="small" outlined severity="secondary" @click="openPaperless(item)" />
                            </div>
                        </template>
                    </Card>
                </div>
            </template>
        </Timeline>

        <div v-else class="flow-compact-list">
            <div
                v-for="(item, index) in timelineItems"
                :key="flowNodeKey(item)"
                class="flow-compact-item"
                :class="[`flow-${referenceStatusClass(item)}`, { selected: selectedFlowNodeKey === flowNodeKey(item), root: item.isRoot }]"
            >
                <div class="flow-compact-rail">
                    <button
                        type="button"
                        class="flow-compact-marker"
                        :aria-label="`เลือกเอกสาร ${item.doc_no || ''}`"
                        @click="selectFlowNode(item)"
                    >
                        {{ index + 1 }}
                    </button>
                </div>
                <div class="flow-compact-card">
                    <div class="flow-compact-head">
                        <div class="min-w-0">
                            <div class="flow-compact-title">{{ documentTypeLabel(item) }}</div>
                            <div class="flow-compact-doc">{{ item.doc_no || '-' }}</div>
                        </div>
                        <div class="flow-compact-tags">
                            <Tag v-if="item.isRoot" value="เอกสารที่ค้นหา" severity="info" />
                            <Tag :value="referenceStatusMeta(item).label" :severity="referenceStatusMeta(item).severity" :icon="referenceStatusMeta(item).icon" />
                        </div>
                    </div>
                    <div class="flow-compact-subtitle">
                        <span>{{ relationText(item) }}</span>
                        <span v-if="item.party_name || item.party_code">· {{ item.party_name || item.party_code }}</span>
                    </div>
                    <dl class="flow-compact-meta">
                        <dt>วันที่</dt>
                        <dd>{{ formatDocumentDateTime(item) }}</dd>
                        <dt>มูลค่า</dt>
                        <dd>{{ formatAmount(item.total_amount) }}</dd>
                        <dt>ต้นทาง</dt>
                        <dd>{{ sourceDocNo(item) }}</dd>
                    </dl>
                    <Message v-if="isMissingPaperLessPdf(item)" severity="error" class="mt-2" :closable="false">
                        {{ missingPaperLessPdfMessage }}
                    </Message>
                    <div v-if="admin" class="flow-compact-actions">
                        <Button icon="pi pi-info-circle" label="ข้อมูล" size="small" outlined severity="secondary" @click="openInfo(item)" />
                        <Button v-if="canPreviewCurrentPDF(item)" icon="pi pi-file-pdf" label="ดูเอกสาร" size="small" outlined severity="secondary" @click="previewCurrentPDF(item)" />
                        <Button v-if="item.canOpenPaperless" icon="pi pi-external-link" label="รายละเอียด" size="small" outlined severity="secondary" @click="openPaperless(item)" />
                    </div>
                </div>
            </div>
        </div>

        <DataTable v-if="showTable && !useTableFirst && !compact && nodes.length" :value="nodes" responsiveLayout="scroll" stripedRows class="mt-4">
            <Column field="doc_no" header="เลขที่เอกสาร" style="min-width: 11rem">
                <template #body="{ data }">
                    <Button :label="data.doc_no" link class="p-0" @click="openInfo(data)" />
                </template>
            </Column>
            <Column field="doc_format_code" header="ชนิด" style="min-width: 7rem" />
            <Column header="คู่ค้า" style="min-width: 14rem">
                <template #body="{ data }">{{ data.party_name || data.party_code || '-' }}</template>
            </Column>
            <Column header="วันที่" style="min-width: 8rem">
                <template #body="{ data }">{{ formatDocumentDateTime(data) }}</template>
            </Column>
            <Column header="ยอดเงิน" style="min-width: 9rem">
                <template #body="{ data }">{{ formatAmount(data.total_amount) }}</template>
            </Column>
            <Column header="เอกสารต้นทาง" style="min-width: 11rem">
                <template #body="{ data }">{{ sourceDocNo(data) }}</template>
            </Column>
            <Column header="PaperLess" style="min-width: 12rem">
                <template #body="{ data }">
                    <Tag :value="referenceStatusMeta(data).label" :severity="referenceStatusMeta(data).severity" :icon="referenceStatusMeta(data).icon" />
                </template>
            </Column>
            <Column header="PDF" style="min-width: 10rem">
                <template #body="{ data }">
                    <div class="flex gap-2 flex-wrap">
                        <Tag :value="data.hasCurrentPdf ? 'มีเอกสารใน PaperLess' : 'ยังไม่มีเอกสาร'" :severity="data.hasCurrentPdf ? 'info' : 'secondary'" />
                    </div>
                </template>
            </Column>
            <Column header="จัดการ" style="min-width: 13rem">
                <template #body="{ data }">
                    <div class="flex gap-2 flex-wrap">
                        <Button icon="pi pi-info-circle" label="ข้อมูล" size="small" outlined severity="secondary" @click="openInfo(data)" />
                        <Button v-if="admin && canPreviewCurrentPDF(data)" icon="pi pi-file-pdf" label="ดูเอกสาร" size="small" outlined severity="secondary" @click="previewCurrentPDF(data)" />
                        <Button v-if="admin && data.canOpenPaperless" icon="pi pi-external-link" label="รายละเอียด" size="small" outlined severity="secondary" @click="openPaperless(data)" />
                    </div>
                </template>
            </Column>
        </DataTable>

        <Dialog v-model:visible="detailVisible" modal header="ข้อมูลเอกสารจาก SML" :style="{ width: 'min(42rem, 94vw)' }">
            <div v-if="selectedNode" class="grid gap-3">
                <Message v-if="isMissingPaperLessPdf(selectedNode)" severity="error" :closable="false">{{ missingPaperLessPdfMessage }}</Message>
                <Message v-if="selectedNode.matchCount > 1" severity="warn">พบเอกสารนี้ใน PaperLess มากกว่า 1 รายการ ระบบเลือกเอกสารที่อัปเดตล่าสุดเป็นค่าเริ่มต้น</Message>
                <dl class="metadata-grid">
                    <dt>เลขที่เอกสาร</dt>
                    <dd>{{ selectedNode.doc_no }}</dd>
                    <dt>ชนิดเอกสาร</dt>
                    <dd>{{ documentTypeLabel(selectedNode) }} ({{ selectedNode.doc_format_code || '-' }})</dd>
                    <dt>วันที่-เวลา</dt>
                    <dd>{{ formatDocumentDateTime(selectedNode) }}</dd>
                    <dt>เอกสารต้นทาง</dt>
                    <dd>{{ sourceDocNo(selectedNode) }}</dd>
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

.flow-card-hitarea {
    cursor: pointer;
}

.flow-card-hitarea:focus-visible {
    outline: 2px solid var(--primary-color);
    outline-offset: 3px;
    border-radius: 8px;
}

.flow-document-card {
    border: 1px solid var(--surface-border);
}

.flow-document-card.root,
.flow-document-card.selected {
    border-color: var(--primary-color);
}

.flow-document-card.selected {
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--primary-color) 65%, transparent);
}

.flow-metadata-grid,
.metadata-grid {
    display: grid;
    grid-template-columns: 8rem minmax(0, 1fr);
    gap: 0.45rem 0.75rem;
    margin: 0;
}

.flow-metadata-grid dt,
.metadata-grid dt {
    color: var(--text-color);
    font-weight: 600;
}

.flow-metadata-grid dd,
.metadata-grid dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
}

.flow-doc-no {
    color: var(--primary-color);
    font-weight: 700;
}

.flow-card-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-top: 0.85rem;
}

.flow-compact-list {
    display: grid;
    gap: 0;
    padding: 0.25rem 0.15rem 0.5rem;
}

.flow-compact-item {
    display: grid;
    grid-template-columns: 2.5rem minmax(0, 1fr);
    gap: 0.75rem;
    position: relative;
}

.flow-compact-item:not(:last-child) {
    padding-bottom: 0.8rem;
}

.flow-compact-rail {
    position: relative;
    display: flex;
    justify-content: center;
}

.flow-compact-rail::after {
    content: '';
    position: absolute;
    top: 2rem;
    bottom: -0.8rem;
    width: 2px;
    background: var(--surface-border);
}

.flow-compact-item:last-child .flow-compact-rail::after {
    display: none;
}

.flow-compact-marker {
    position: relative;
    z-index: 1;
    width: 2rem;
    height: 2rem;
    border: 1px solid var(--surface-border);
    border-radius: 999px;
    background: var(--surface-card);
    color: var(--text-color);
    font-weight: 700;
}

.flow-completed .flow-compact-marker {
    border-color: var(--green-500);
    color: var(--green-700);
}

.flow-in-progress .flow-compact-marker {
    border-color: var(--orange-500);
    color: var(--orange-700);
}

.flow-missing .flow-compact-marker {
    border-color: var(--red-500);
    color: var(--red-700);
}

.flow-compact-card {
    min-width: 0;
    border: 1px solid var(--surface-border);
    border-left-width: 4px;
    border-radius: 8px;
    background: var(--surface-card);
    padding: 0.8rem;
}

.flow-compact-item.selected .flow-compact-card,
.flow-compact-item.root .flow-compact-card {
    border-color: var(--primary-color);
}

.flow-completed .flow-compact-card {
    border-left-color: var(--green-500);
}

.flow-in-progress .flow-compact-card {
    border-left-color: var(--orange-500);
}

.flow-missing .flow-compact-card {
    border-left-color: var(--red-500);
}

.flow-compact-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}

.flow-compact-title {
    font-weight: 700;
    line-height: 1.25;
}

.flow-compact-doc {
    color: var(--primary-color);
    font-weight: 700;
    overflow-wrap: anywhere;
}

.flow-compact-tags,
.flow-compact-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    justify-content: flex-end;
}

.flow-compact-subtitle {
    margin-top: 0.25rem;
    color: var(--text-color-secondary);
    font-size: 0.9rem;
    overflow-wrap: anywhere;
}

.flow-compact-meta {
    display: grid;
    grid-template-columns: repeat(3, auto minmax(0, 1fr));
    gap: 0.35rem 0.55rem;
    margin: 0.7rem 0 0;
    font-size: 0.9rem;
}

.flow-compact-meta dt {
    color: var(--text-color-secondary);
}

.flow-compact-meta dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
}

.flow-compact-actions {
    margin-top: 0.7rem;
}

@media screen and (max-width: 960px) {
    .document-flow-timeline:deep(.p-timeline-event:nth-child(even)) {
        flex-direction: row !important;
    }

    .document-flow-timeline:deep(.p-timeline-event:nth-child(even) .p-timeline-event-content) {
        text-align: left !important;
    }

    .document-flow-timeline:deep(.p-timeline-event-opposite) {
        flex: 0;
    }

    .document-flow-timeline:deep(.p-card) {
        margin-top: 1rem;
    }
}

@media (max-width: 640px) {
    .flow-metadata-grid,
    .metadata-grid,
    .flow-compact-meta {
        grid-template-columns: 1fr;
    }

    .flow-compact-head {
        flex-direction: column;
    }

    .flow-compact-tags,
    .flow-compact-actions {
        justify-content: flex-start;
    }
}
</style>
