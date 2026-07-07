<script setup>
import { formatDocumentDate } from '@/utils/signingFormatters';
import { computed, ref } from 'vue';

const props = defineProps({
    graph: { type: Object, default: null },
    admin: { type: Boolean, default: false },
    compact: { type: Boolean, default: false },
    showTable: { type: Boolean, default: true },
    tableFirst: { type: Boolean, default: false },
    openPdfOnSelect: { type: Boolean, default: false },
    showDetailPanel: { type: Boolean, default: true }
});

const emit = defineEmits(['open-document', 'preview-pdf', 'node-click']);

const activeNodeKey = ref('');

const missingPaperLessPdfMessage = 'เอกสารนี้มีข้อมูลจาก SML แต่ยังไม่มี PDF ใน PaperLess';
const nodes = computed(() => props.graph?.nodes || []);
const edges = computed(() => props.graph?.edges || []);
const warnings = computed(() => props.graph?.warnings || []);
const rootDocNo = computed(() => props.graph?.root?.doc_no || props.graph?.root?.docNo || '');
const timelineItems = computed(() =>
    nodes.value.map((node, index) => {
        const docNo = docNoValue(node);
        return {
            ...node,
            _originalIndex: index,
            incoming: edges.value.filter((edge) => normalizeDocNo(edge.to_doc_no || edge.toDocNo) === normalizeDocNo(docNo)),
            outgoing: edges.value.filter((edge) => normalizeDocNo(edge.from_doc_no || edge.fromDocNo) === normalizeDocNo(docNo)),
            isRoot: normalizeDocNo(docNo) === normalizeDocNo(rootDocNo.value)
        };
    })
);
const flowLayout = computed(() => buildFlowLayout(timelineItems.value, edges.value));
const selectedFlowNode = computed(() => {
    const active = flowLayout.value.items.find((node) => flowNodeKey(node) === activeNodeKey.value);
    if (active) return active;
    return flowLayout.value.items.find((node) => node.isRoot) || flowLayout.value.items[0] || null;
});
const selectedFlowNodeKey = computed(() => (selectedFlowNode.value ? flowNodeKey(selectedFlowNode.value) : ''));
const selectedStatus = computed(() => referenceStatusMeta(selectedFlowNode.value || {}));
const flowCanvasStyle = computed(() => ({
    width: `${flowLayout.value.width}px`,
    height: `${flowLayout.value.height}px`
}));

function buildFlowLayout(items, rawEdges) {
    const constants = items.length > 20 ? { nodeWidth: 212, nodeHeight: 104, columnGap: 48, rowGap: 14 } : { nodeWidth: 240, nodeHeight: 116, columnGap: 58, rowGap: 16 };
    if (!items.length) return emptyFlowLayout(constants);

    const itemByKey = new Map();
    const keyByDocNo = new Map();
    for (const item of items) {
        const key = flowNodeKey(item);
        itemByKey.set(key, item);
        keyByDocNo.set(normalizeDocNo(docNoValue(item)), key);
    }

    const validEdges = [];
    const seenEdges = new Set();
    for (const edge of rawEdges || []) {
        const fromKey = keyByDocNo.get(normalizeDocNo(edge.from_doc_no || edge.fromDocNo));
        const toKey = keyByDocNo.get(normalizeDocNo(edge.to_doc_no || edge.toDocNo));
        if (!fromKey || !toKey || fromKey === toKey) continue;
        const edgeKey = `${fromKey}->${toKey}`;
        if (seenEdges.has(edgeKey)) continue;
        seenEdges.add(edgeKey);
        validEdges.push({ ...edge, fromKey, toKey });
    }

    if (!validEdges.length) {
        return fallbackFlowLayout(items, constants, 'แสดงตามลำดับรายการจาก SML');
    }

    const indegree = new Map();
    const outgoing = new Map();
    const levels = new Map();
    for (const item of items) {
        const key = flowNodeKey(item);
        indegree.set(key, 0);
        outgoing.set(key, []);
        levels.set(key, 0);
    }
    for (const edge of validEdges) {
        outgoing.get(edge.fromKey).push(edge);
        indegree.set(edge.toKey, (indegree.get(edge.toKey) || 0) + 1);
    }

    const byOriginalOrder = (a, b) => (itemByKey.get(a)?._originalIndex || 0) - (itemByKey.get(b)?._originalIndex || 0);
    const queue = [...indegree.entries()]
        .filter(([, count]) => count === 0)
        .map(([key]) => key)
        .sort(byOriginalOrder);
    const visited = [];

    while (queue.length) {
        const key = queue.shift();
        visited.push(key);
        for (const edge of outgoing.get(key) || []) {
            levels.set(edge.toKey, Math.max(levels.get(edge.toKey) || 0, (levels.get(key) || 0) + 1));
            indegree.set(edge.toKey, (indegree.get(edge.toKey) || 0) - 1);
            if (indegree.get(edge.toKey) === 0) {
                queue.push(edge.toKey);
                queue.sort((a, b) => {
                    const levelDiff = (levels.get(a) || 0) - (levels.get(b) || 0);
                    return levelDiff || byOriginalOrder(a, b);
                });
            }
        }
    }

    if (visited.length !== items.length) {
        return fallbackFlowLayout(items, constants, 'ข้อมูลความสัมพันธ์จาก SML มีวงรอบ ระบบแสดงตามลำดับรายการแทน');
    }

    const columns = new Map();
    for (const item of items) {
        const key = flowNodeKey(item);
        const level = levels.get(key) || 0;
        if (!columns.has(level)) columns.set(level, []);
        columns.get(level).push(item);
    }

    const layoutItems = [];
    const sortedLevels = [...columns.keys()].sort((a, b) => a - b);
    let maxRows = 1;
    for (const level of sortedLevels) {
        const column = columns.get(level).sort((a, b) => a._originalIndex - b._originalIndex);
        maxRows = Math.max(maxRows, column.length);
        column.forEach((item, row) => {
            layoutItems.push({
                ...item,
                _flowLevel: level,
                _flowRow: row,
                _flowX: level * (constants.nodeWidth + constants.columnGap),
                _flowY: row * (constants.nodeHeight + constants.rowGap)
            });
        });
    }

    return completeFlowLayout(layoutItems, validEdges, constants, '', false);
}

function completeFlowLayout(layoutItems, flowEdges, constants, fallbackMessage, hasFallback) {
    const byKey = new Map(layoutItems.map((item) => [flowNodeKey(item), item]));
    const maxLevel = Math.max(0, ...layoutItems.map((item) => item._flowLevel || 0));
    const maxRow = Math.max(0, ...layoutItems.map((item) => item._flowRow || 0));
    const width = (maxLevel + 1) * constants.nodeWidth + maxLevel * constants.columnGap;
    const height = (maxRow + 1) * constants.nodeHeight + maxRow * constants.rowGap;
    const edgePaths = flowEdges
        .map((edge) => {
            const from = byKey.get(edge.fromKey);
            const to = byKey.get(edge.toKey);
            if (!from || !to) return null;
            const startX = from._flowX + constants.nodeWidth;
            const startY = from._flowY + constants.nodeHeight / 2;
            const endX = to._flowX;
            const endY = to._flowY + constants.nodeHeight / 2;
            const midX = startX + Math.max(24, (endX - startX) / 2);
            return {
                key: `${edge.fromKey}->${edge.toKey}`,
                status: referenceStatusClass(to),
                path: `M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}`
            };
        })
        .filter(Boolean);

    return {
        items: layoutItems.sort((a, b) => (a._flowLevel - b._flowLevel) || (a._flowRow - b._flowRow) || (a._originalIndex - b._originalIndex)),
        edges: edgePaths,
        width: Math.max(width, constants.nodeWidth),
        height: Math.max(height, constants.nodeHeight),
        nodeWidth: constants.nodeWidth,
        nodeHeight: constants.nodeHeight,
        hasFallback,
        fallbackMessage,
        isLarge: layoutItems.length > 20
    };
}

function fallbackFlowLayout(items, constants, fallbackMessage) {
    const layoutItems = items.map((item, index) => ({
        ...item,
        _flowLevel: index,
        _flowRow: 0,
        _flowX: index * (constants.nodeWidth + constants.columnGap),
        _flowY: 0
    }));
    const fallbackEdges = layoutItems.slice(1).map((item, index) => ({
        fromKey: flowNodeKey(layoutItems[index]),
        toKey: flowNodeKey(item)
    }));
    return completeFlowLayout(layoutItems, fallbackEdges, constants, fallbackMessage, true);
}

function emptyFlowLayout(constants) {
    return { items: [], edges: [], width: constants.nodeWidth, height: constants.nodeHeight, nodeWidth: constants.nodeWidth, nodeHeight: constants.nodeHeight, hasFallback: false, fallbackMessage: '', isLarge: false };
}

function flowNodeKey(node = {}) {
    return `${docFormatValue(node) || 'doc'}-${docNoValue(node)}`;
}

function normalizeDocNo(value) {
    return String(value || '').trim().toUpperCase();
}

function docNoValue(node = {}) {
    return node.doc_no || node.docNo || '';
}

function docFormatValue(node = {}) {
    return node.doc_format_code || node.docFormatCode || '';
}

function selectFlowNode(node) {
    activeNodeKey.value = flowNodeKey(node);
    if (props.openPdfOnSelect && canPreviewCurrentPDF(node)) {
        previewCurrentPDF(node);
        return;
    }
    emit('node-click', node);
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

function relationText(item = {}) {
    if (!item.incoming?.length) return 'เอกสารต้นทาง';
    const from = [...new Set(item.incoming.map((edge) => edge.from_doc_no || edge.fromDocNo).filter(Boolean))];
    return from.length ? `ต่อจาก ${from.join(', ')}` : 'เอกสารที่เกี่ยวข้อง';
}

function documentTypeLabel(node = {}) {
    const transFlagName = node.trans_flag_name_th || node.transFlagNameTh || node.trans_flag_name_en || node.transFlagNameEn || '';
    if (transFlagName) return transFlagName;
    const name = node.doc_format_name || node.docFormatName || '';
    if (name) return name;
    const code = String(docFormatValue(node)).toUpperCase();
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

function sourceDocNo(node = {}) {
    const explicit = node.source_doc_no || node.sourceDocNo || '';
    if (explicit) return explicit;
    const incoming = node.incoming || [];
    if (!incoming.length) return docNoValue(node) || '-';
    return incoming
        .map((edge) => edge.from_doc_no || edge.fromDocNo)
        .filter(Boolean)
        .join(', ');
}

function technicalRelations(item = {}) {
    const all = [...(item.incoming || []), ...(item.outgoing || [])];
    return all.map((edge) => `${edge.from_doc_no || edge.fromDocNo} -> ${edge.to_doc_no || edge.toDocNo} (${edge.source_table || edge.sourceTable}.${edge.source_column || edge.sourceColumn})`).join('\n');
}

function normalizeTime(value) {
    const text = String(value || '').trim();
    if (!text) return '';
    const match = text.match(/^(\d{1,2}):(\d{2})/);
    if (!match) return text;
    return `${match[1].padStart(2, '0')}:${match[2]}`;
}

function formatDocumentDateTime(node = {}) {
    const dateText = formatDocumentDate(node.doc_date || node.docDate);
    const timeText = normalizeTime(node.doc_time || node.docTime);
    if (dateText === '-') return timeText || '-';
    return timeText ? `${dateText} ${timeText}` : dateText;
}

function formatAmount(value) {
    const amount = Number(value || 0);
    return new Intl.NumberFormat('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount);
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
    activeNodeKey.value = flowNodeKey(node);
    emit('node-click', node);
    emit('preview-pdf', { node, version, url: version === 'final' ? node.signedPdfUrl : node.currentPdfUrl });
}

function previewCurrentPDF(node) {
    previewPDF(node, 'current');
}
</script>

<template>
    <div class="document-flow-viewer" :class="{ compact, 'large-flow': flowLayout.isLarge, 'no-detail-panel': !showDetailPanel }">
        <Message v-for="warning in warnings" :key="`${warning.code}-${warning.doc_no || warning.message}`" severity="warn" class="mb-3">
            {{ warning.message || 'พบความสัมพันธ์บางส่วนจาก SML' }}<span v-if="warning.doc_no">: {{ warning.doc_no }}</span>
        </Message>

        <div v-if="nodes.length === 0" class="flow-empty">
            <i class="pi pi-inbox"></i>
            <span>ยังไม่พบเอกสารประกอบจาก SML</span>
        </div>

        <div v-else class="flow-workspace">
            <section class="flow-map-panel" aria-label="แผนผัง Flow เอกสาร">
                <small v-if="flowLayout.hasFallback" class="flow-fallback-note flow-map-note">
                    <i class="pi pi-info-circle" aria-hidden="true"></i>
                    {{ flowLayout.fallbackMessage }}
                </small>

                <div class="flow-map-scroll">
                    <div class="flow-map-canvas" :style="flowCanvasStyle">
                        <svg class="flow-edge-layer" :width="flowLayout.width" :height="flowLayout.height" aria-hidden="true">
                            <path v-for="edge in flowLayout.edges" :key="edge.key" :d="edge.path" class="flow-edge" :class="`edge-${edge.status}`" />
                        </svg>
                        <button
                            v-for="item in flowLayout.items"
                            :key="flowNodeKey(item)"
                            type="button"
                            class="flow-node"
                            :class="[`flow-${referenceStatusClass(item)}`, { selected: selectedFlowNodeKey === flowNodeKey(item), root: item.isRoot }]"
                            :style="{ left: `${item._flowX}px`, top: `${item._flowY}px`, width: `${flowLayout.nodeWidth}px`, minHeight: `${flowLayout.nodeHeight}px` }"
                            :aria-label="openPdfOnSelect ? `ดู PDF เอกสาร ${docNoValue(item) || ''}` : `เลือกเอกสาร ${docNoValue(item) || ''}`"
                            @click="selectFlowNode(item)"
                        >
                            <span class="flow-node-topline">
                                <span class="flow-status-dot" aria-hidden="true"></span>
                                <span class="flow-node-type">{{ documentTypeLabel(item) }}</span>
                            </span>
                            <strong class="flow-node-doc">{{ docNoValue(item) || '-' }}</strong>
                            <span class="flow-node-meta">{{ formatDocumentDateTime(item) }}</span>
                            <span class="flow-node-meta">มูลค่า {{ formatAmount(item.total_amount) }}</span>
                            <span class="flow-node-tags">
                                <Tag v-if="selectedFlowNodeKey === flowNodeKey(item)" value="กำลังเลือก" severity="info" class="flow-selected-tag" />
                                <Tag v-if="item.isRoot" value="เอกสารที่ค้นหา" severity="info" />
                                <Tag :value="referenceStatusMeta(item).label" :severity="referenceStatusMeta(item).severity" :icon="referenceStatusMeta(item).icon" />
                            </span>
                        </button>
                    </div>
                </div>

                <div class="flow-mobile-list" aria-label="รายการ Flow เอกสาร">
                    <button
                        v-for="(item, index) in flowLayout.items"
                        :key="`mobile-${flowNodeKey(item)}`"
                        type="button"
                        class="flow-mobile-node"
                        :class="[`flow-${referenceStatusClass(item)}`, { selected: selectedFlowNodeKey === flowNodeKey(item), root: item.isRoot }]"
                        :aria-label="openPdfOnSelect ? `ดู PDF เอกสาร ${docNoValue(item) || ''}` : `เลือกเอกสาร ${docNoValue(item) || ''}`"
                        @click="selectFlowNode(item)"
                    >
                        <span class="flow-mobile-index">{{ index + 1 }}</span>
                        <span class="flow-mobile-main">
                            <span class="flow-mobile-title">{{ documentTypeLabel(item) }} <strong>{{ docNoValue(item) || '-' }}</strong></span>
                            <span class="flow-mobile-meta">{{ relationText(item) }} · {{ formatDocumentDateTime(item) }}</span>
                        </span>
                        <Tag :value="referenceStatusMeta(item).label" :severity="referenceStatusMeta(item).severity" />
                    </button>
                </div>
            </section>

            <aside v-if="showDetailPanel && selectedFlowNode" class="flow-detail-panel" aria-label="รายละเอียดเอกสารที่เลือก">
                <div class="flow-detail-head">
                    <div class="min-w-0">
                        <div class="flow-detail-selection-label">
                            <i class="pi pi-map-marker" aria-hidden="true"></i>
                            เอกสารที่เลือก
                        </div>
                        <div class="flow-detail-type">{{ documentTypeLabel(selectedFlowNode) }}</div>
                        <strong>{{ docNoValue(selectedFlowNode) || '-' }}</strong>
                    </div>
                    <Tag :value="selectedStatus.label" :severity="selectedStatus.severity" :icon="selectedStatus.icon" />
                </div>

                <div v-if="isMissingPaperLessPdf(selectedFlowNode) || selectedFlowNode.matchCount > 1" class="flow-detail-messages">
                    <Message v-if="isMissingPaperLessPdf(selectedFlowNode)" severity="error" :closable="false" class="flow-detail-message">
                        {{ missingPaperLessPdfMessage }}
                    </Message>
                    <Message v-if="selectedFlowNode.matchCount > 1" severity="warn" :closable="false" class="flow-detail-message">
                        พบเอกสารนี้ใน PaperLess มากกว่า 1 รายการ ระบบเลือกเอกสารที่อัปเดตล่าสุดเป็นค่าเริ่มต้น
                    </Message>
                </div>

                <dl class="metadata-grid">
                    <dt>วันที่-เวลา</dt>
                    <dd>{{ formatDocumentDateTime(selectedFlowNode) }}</dd>
                    <dt>มูลค่าเอกสาร</dt>
                    <dd>{{ formatAmount(selectedFlowNode.total_amount) }}</dd>
                    <dt>เอกสารต้นทาง</dt>
                    <dd>{{ sourceDocNo(selectedFlowNode) }}</dd>
                    <dt>คู่ค้า</dt>
                    <dd>{{ selectedFlowNode.party_name || selectedFlowNode.party_code || '-' }}</dd>
                    <dt>สถานะ SML</dt>
                    <dd>{{ selectedFlowNode.is_lock_record === 1 ? 'Lock แล้ว' : 'ยังไม่ Lock' }}</dd>
                    <dt>แหล่งข้อมูล</dt>
                    <dd>{{ selectedFlowNode.table || '-' }}</dd>
                    <dt>ความสัมพันธ์</dt>
                    <dd class="whitespace-pre-line">{{ technicalRelations(selectedFlowNode) || '-' }}</dd>
                </dl>

                <div v-if="admin" class="flow-detail-actions">
                    <Button v-if="canPreviewCurrentPDF(selectedFlowNode)" icon="pi pi-file-pdf" label="ดูเอกสาร" size="small" outlined severity="secondary" @click="previewCurrentPDF(selectedFlowNode)" />
                    <Button v-if="selectedFlowNode.canOpenPaperless" icon="pi pi-external-link" label="รายละเอียด" size="small" outlined severity="secondary" @click="openPaperless(selectedFlowNode)" />
                </div>
            </aside>
        </div>

        <details v-if="showTable && flowLayout.hasFallback" class="flow-fallback-details">
            <summary>ดูรายการแบบตาราง</summary>
            <DataTable :value="flowLayout.items" responsiveLayout="scroll" stripedRows size="small">
                <Column header="สถานะ" style="min-width: 11rem">
                    <template #body="{ data }">
                        <Tag :value="referenceStatusMeta(data).label" :severity="referenceStatusMeta(data).severity" :icon="referenceStatusMeta(data).icon" />
                    </template>
                </Column>
                <Column header="เลขที่เอกสาร" style="min-width: 12rem">
                    <template #body="{ data }">{{ docNoValue(data) || '-' }}</template>
                </Column>
                <Column header="ชนิดเอกสาร" style="min-width: 14rem">
                    <template #body="{ data }">{{ documentTypeLabel(data) }}</template>
                </Column>
                <Column header="วันที่" style="min-width: 10rem">
                    <template #body="{ data }">{{ formatDocumentDateTime(data) }}</template>
                </Column>
                <Column header="เอกสารต้นทาง" style="min-width: 12rem">
                    <template #body="{ data }">{{ sourceDocNo(data) }}</template>
                </Column>
            </DataTable>
        </details>
    </div>
</template>

<style scoped>
.document-flow-viewer {
    --flow-success: var(--p-green-500, #22c55e);
    --flow-success-soft: color-mix(in srgb, var(--flow-success) 8%, var(--surface-card));
    --flow-warning: var(--p-orange-500, #f97316);
    --flow-warning-soft: color-mix(in srgb, var(--flow-warning) 8%, var(--surface-card));
    --flow-danger: var(--p-red-500, #ef4444);
    --flow-danger-soft: color-mix(in srgb, var(--flow-danger) 7%, var(--surface-card));
    --flow-root: var(--p-sky-500, #0ea5e9);
    height: 100%;
    min-width: 0;
    min-height: 0;
}

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

.flow-workspace {
    height: 100%;
    min-height: 0;
    display: grid;
    grid-template-rows: minmax(320px, 1fr) auto;
    gap: 0.75rem;
}

.no-detail-panel .flow-workspace {
    grid-template-rows: minmax(0, 1fr);
}

.flow-map-panel,
.flow-detail-panel {
    min-width: 0;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
}

.flow-map-panel {
    min-height: 0;
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    overflow: hidden;
}

.flow-fallback-note {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--text-color-secondary);
}

.flow-map-note {
    padding: 0.45rem 0.65rem;
    border-bottom: 1px solid var(--surface-border);
}

.flow-map-scroll {
    min-height: 280px;
    height: 100%;
    overflow: auto;
    padding: 0.85rem;
    background: color-mix(in srgb, var(--surface-ground) 68%, var(--surface-card));
}

.flow-map-canvas {
    position: relative;
    min-width: 100%;
}

.flow-edge-layer {
    position: absolute;
    inset: 0;
    overflow: visible;
    pointer-events: none;
}

.flow-edge {
    fill: none;
    stroke: color-mix(in srgb, var(--text-color-secondary) 34%, transparent);
    stroke-width: 2;
    stroke-linecap: round;
}

.flow-edge.edge-completed {
    stroke: color-mix(in srgb, var(--flow-success) 58%, var(--surface-border));
}

.flow-edge.edge-in-progress {
    stroke: color-mix(in srgb, var(--flow-warning) 58%, var(--surface-border));
}

.flow-edge.edge-missing {
    stroke: color-mix(in srgb, var(--flow-danger) 56%, var(--surface-border));
}

.flow-node {
    position: absolute;
    z-index: 1;
    display: grid;
    align-content: start;
    gap: 0.16rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.55rem 0.6rem;
    background: var(--surface-card);
    color: var(--text-color);
    text-align: left;
    cursor: pointer;
    transition:
        border-color 0.15s ease,
        background-color 0.15s ease,
        transform 0.15s ease;
}

.flow-node:hover,
.flow-node:focus-visible {
    border-color: var(--primary-color);
    outline: none;
}

.flow-node.selected {
    border-color: var(--primary-color);
    background: color-mix(in srgb, var(--primary-color) 7%, var(--surface-card));
    box-shadow:
        inset 0 0 0 1px var(--primary-color),
        0 0 0 3px color-mix(in srgb, var(--primary-color) 18%, transparent);
    transform: translateY(-1px);
}

.flow-node.root {
    border-color: color-mix(in srgb, var(--flow-root) 62%, var(--surface-border));
}

.flow-node.flow-completed {
    border-color: color-mix(in srgb, var(--flow-success) 46%, var(--surface-border));
    background: var(--flow-success-soft);
}

.flow-node.flow-in-progress {
    border-color: color-mix(in srgb, var(--flow-warning) 46%, var(--surface-border));
    background: var(--flow-warning-soft);
}

.flow-node.flow-missing {
    border-color: color-mix(in srgb, var(--flow-danger) 44%, var(--surface-border));
    background: var(--flow-danger-soft);
}

.flow-node.root {
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--flow-root) 70%, transparent);
}

.flow-node-topline {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 0.4rem;
}

.flow-status-dot {
    width: 0.62rem;
    height: 0.62rem;
    border-radius: 999px;
    background: var(--text-color-secondary);
    flex: 0 0 auto;
}

.flow-completed .flow-status-dot {
    background: var(--flow-success);
}

.flow-in-progress .flow-status-dot {
    background: var(--flow-warning);
}

.flow-missing .flow-status-dot {
    background: var(--flow-danger);
}

.flow-node-type,
.flow-node-meta {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.flow-node-type {
    color: var(--text-color);
    font-size: 0.86rem;
    font-weight: 700;
}

.flow-node-doc {
    min-width: 0;
    color: var(--primary-color);
    font-size: 1rem;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.flow-node-meta {
    color: var(--text-color-secondary);
    font-size: 0.78rem;
}

.flow-node-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.28rem;
    margin-top: 0.2rem;
}

.flow-node-tags:deep(.p-tag) {
    max-width: 100%;
    padding: 0.15rem 0.34rem;
    font-size: 0.68rem;
}

.flow-mobile-list {
    display: none;
}

.flow-mobile-node {
    width: 100%;
    min-width: 0;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.55rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    color: var(--text-color);
    padding: 0.55rem;
    text-align: left;
}

.flow-mobile-node.selected {
    border-color: var(--primary-color);
    background: color-mix(in srgb, var(--primary-color) 7%, var(--surface-card));
}

.flow-mobile-node.flow-completed {
    border-color: color-mix(in srgb, var(--flow-success) 46%, var(--surface-border));
    background: var(--flow-success-soft);
}

.flow-mobile-node.flow-in-progress {
    border-color: color-mix(in srgb, var(--flow-warning) 46%, var(--surface-border));
    background: var(--flow-warning-soft);
}

.flow-mobile-node.flow-missing {
    border-color: color-mix(in srgb, var(--flow-danger) 44%, var(--surface-border));
    background: var(--flow-danger-soft);
}

.flow-mobile-node.root {
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--flow-root) 70%, transparent);
}

.flow-mobile-index {
    width: 1.65rem;
    height: 1.65rem;
    display: inline-grid;
    place-items: center;
    border-radius: 999px;
    background: var(--surface-100);
    color: var(--text-color);
    font-weight: 700;
}

.flow-mobile-main {
    min-width: 0;
    display: grid;
    gap: 0.1rem;
}

.flow-mobile-title,
.flow-mobile-meta {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.flow-mobile-title {
    font-weight: 700;
}

.flow-mobile-title strong {
    color: var(--primary-color);
}

.flow-mobile-meta {
    color: var(--text-color-secondary);
    font-size: 0.82rem;
}

.flow-detail-panel {
    min-height: 0;
    max-height: 16rem;
    overflow: auto;
    display: grid;
    grid-template-columns: minmax(220px, 0.75fr) minmax(0, 1.7fr) auto;
    grid-template-areas:
        'head meta actions'
        'messages messages messages';
    align-items: start;
    gap: 0.75rem;
    padding: 0.75rem 0.9rem;
}

.flow-detail-head {
    grid-area: head;
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}

.flow-detail-head strong {
    display: block;
    color: var(--primary-color);
    font-size: 1.05rem;
    line-height: 1.25;
    overflow-wrap: anywhere;
}

.flow-detail-type {
    color: var(--text-color);
    font-weight: 700;
}

.flow-detail-selection-label {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--primary-color);
    font-size: 0.78rem;
    font-weight: 700;
}

.flow-detail-messages {
    grid-area: messages;
    display: grid;
    gap: 0.5rem;
}

.flow-detail-message {
    margin: 0;
}

.metadata-grid {
    grid-area: meta;
    display: grid;
    grid-template-columns: 7rem minmax(0, 1fr);
    gap: 0.45rem 0.65rem;
    margin: 0;
}

.metadata-grid dt {
    color: var(--text-color-secondary);
    font-weight: 600;
}

.metadata-grid dd {
    margin: 0;
    min-width: 0;
    color: var(--text-color);
    overflow-wrap: anywhere;
}

.flow-detail-actions {
    grid-area: actions;
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
    min-width: 12rem;
}

.flow-fallback-details {
    margin-top: 0.75rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 0.55rem 0.65rem;
}

.flow-fallback-details summary {
    cursor: pointer;
    color: var(--text-color);
    font-weight: 700;
}

.flow-fallback-details:deep(.p-datatable) {
    margin-top: 0.55rem;
}

@media (max-width: 760px) {
    .flow-workspace {
        height: auto;
        grid-template-rows: auto auto;
    }

    .flow-map-panel {
        display: block;
        overflow: visible;
    }

    .flow-map-scroll {
        display: none;
    }

    .flow-mobile-list {
        display: grid;
        gap: 0.45rem;
        padding: 0.55rem;
    }

    .flow-mobile-node {
        grid-template-columns: auto minmax(0, 1fr);
    }

    .flow-mobile-node:deep(.p-tag) {
        grid-column: 2;
        justify-self: start;
    }

    .flow-detail-panel {
        max-height: none;
        grid-template-columns: 1fr;
        grid-template-areas:
            'head'
            'messages'
            'meta'
            'actions';
    }

    .flow-detail-actions {
        min-width: 0;
        justify-content: flex-start;
    }

    .metadata-grid {
        grid-template-columns: 1fr;
    }
}
</style>
