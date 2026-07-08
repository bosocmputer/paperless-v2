<script setup>
import { api } from '@/services/api';
import { formatDocumentDate } from '@/utils/signingFormatters';
import ReadOnlyPdfDialog from '@/views/signing/components/ReadOnlyPdfDialog.vue';
import { computed, onMounted, ref, watch } from 'vue';

const props = defineProps({
    document: { type: Object, default: null },
    loader: { type: Function, required: true },
    compact: { type: Boolean, default: false },
    displayMode: {
        type: String,
        default: 'list',
        validator: (value) => ['list', 'flow'].includes(value)
    },
    openInNewTab: { type: Boolean, default: false },
    documentRouteResolver: { type: Function, default: null },
    allowPreview: { type: Boolean, default: true },
    dialogMode: { type: Boolean, default: false }
});

defineEmits(['open-document']);

const loading = ref(false);
const error = ref('');
const referenceCheck = ref(null);
const requestSeq = ref(0);
const pdfDialog = ref(false);
const pdfUrl = ref('');
const pdfTitle = ref('');
const pdfActionUrl = ref('');

const items = computed(() => referenceCheck.value?.items || []);
const summary = computed(() => referenceCheck.value?.summary || { total: 0, missing: 0, inProgress: 0, completed: 0 });
const warnings = computed(() => referenceCheck.value?.warnings || []);
const summaryItems = computed(() => {
    const total = summary.value.total || 0;
    const items = [{ value: props.displayMode === 'flow' ? `${total} เอกสาร` : `ทั้งหมด ${total}`, severity: 'info' }];
    if (summary.value.missing) items.push({ value: `ยังไม่เข้า ${summary.value.missing}`, severity: 'danger' });
    if (summary.value.inProgress) items.push({ value: `กำลังเซ็น ${summary.value.inProgress}`, severity: 'warn' });
    if (summary.value.completed) items.push({ value: `ครบแล้ว ${summary.value.completed}`, severity: 'success' });
    return items;
});
const compactFlowMode = computed(() => props.compact && props.displayMode === 'flow');

onMounted(load);

watch(
    () => props.document?.id,
    () => load()
);

async function load() {
    const seq = requestSeq.value + 1;
    requestSeq.value = seq;
    loading.value = true;
    error.value = '';
    try {
        const result = await props.loader();
        if (seq !== requestSeq.value) return;
        referenceCheck.value = result.referenceCheck || result;
    } catch (err) {
        if (seq !== requestSeq.value) return;
        referenceCheck.value = null;
        error.value = err?.message || 'ไม่สามารถตรวจสอบเอกสารอ้างอิงได้';
    } finally {
        if (seq === requestSeq.value) loading.value = false;
    }
}

function statusMeta(item = {}) {
    const status = item.paperlessStatus || 'missing';
    if (status === 'completed') {
        return { label: 'เซ็นครบแล้ว', severity: 'success', icon: 'pi pi-check-circle' };
    }
    if (status === 'in_progress') {
        return { label: 'กำลังเซ็น/ยังไม่เสร็จ', severity: 'warn', icon: 'pi pi-clock' };
    }
    return { label: 'ยังไม่เข้า PaperLess', severity: 'danger', icon: 'pi pi-exclamation-triangle' };
}

function docNo(item = {}) {
    return item.doc_no || item.docNo || '-';
}

function docFormat(item = {}) {
    const name = item.doc_format_name || item.docFormatName || item.trans_flag_name_th || item.transFlagNameTh || '';
    const code = item.doc_format_code || item.docFormatCode || '';
    if (name && code) return `${name} (${code})`;
    return name || code || '-';
}

function sourceLabel(item = {}) {
    const table = item.source_table || item.sourceTable || '';
    const column = item.source_column || item.sourceColumn || '';
    if (!table && !column) return '-';
    return `${table}${column ? `.${column}` : ''}`;
}

function docDate(item = {}) {
    return formatDocumentDate(item.doc_date || item.docDate);
}

function paperlessDocumentUrl(item = {}) {
    const id = item.paperlessDocumentId || item.paperless_document_id;
    if (!id || !item.canOpenPaperless) return '';
    return props.documentRouteResolver ? props.documentRouteResolver(id) : `/signing/documents/${encodeURIComponent(id)}`;
}

function currentPdfUrl(item = {}) {
    return item.currentPdfUrl || item.current_pdf_url || item.pdfUrl || item.pdf_url || '';
}

function canPreviewPDF(item = {}) {
    return props.allowPreview && !!currentPdfUrl(item);
}

function openReferencePDF(item = {}) {
    if (!canPreviewPDF(item)) return;
    const rawUrl = currentPdfUrl(item);
    pdfUrl.value = api.withPDFCacheKey(rawUrl, api.signingDocumentPDFCacheKey(item, 'current'));
    pdfTitle.value = `${docNo(item)} · เอกสารใน PaperLess`;
    pdfActionUrl.value = paperlessDocumentUrl(item);
    pdfDialog.value = true;
}
</script>

<template>
    <div class="reference-check" :class="{ compact, 'dialog-mode': dialogMode }">
        <div class="reference-head">
            <div class="reference-actions">
                <div class="reference-summary">
                    <Tag v-for="item in summaryItems" :key="item.value" :value="item.value" :severity="item.severity" />
                </div>
                <Button icon="pi pi-refresh" :label="dialogMode ? 'โหลดใหม่' : undefined" :rounded="!dialogMode" outlined severity="secondary" aria-label="โหลดใหม่" :loading="loading" @click="load" />
            </div>
        </div>

        <Message v-if="error" severity="error" :closable="false">
            {{ error }}
            <div class="mt-3">
                <Button size="small" label="ลองใหม่" icon="pi pi-refresh" outlined severity="secondary" @click="load" />
            </div>
        </Message>
        <Message v-for="warning in warnings" :key="`${warning.code}-${warning.doc_no || warning.message}`" severity="warn" :closable="false">
            {{ warning.message || 'พบข้อมูลอ้างอิงบางส่วนจาก SML' }}<span v-if="warning.doc_no">: {{ warning.doc_no }}</span>
        </Message>

        <div v-if="compact" class="reference-compact">
            <div v-if="loading" class="reference-empty">
                <i class="pi pi-spin pi-spinner"></i>
                <span>กำลังโหลดเอกสารอ้างอิง</span>
            </div>
            <div v-else-if="items.length === 0" class="reference-empty">
                <i class="pi pi-inbox"></i>
                <span>ยังไม่พบเอกสารอ้างอิงก่อนหน้าใน SML</span>
            </div>
            <div v-else-if="compactFlowMode" class="reference-flow-scroll">
                <div class="reference-flow-row">
                    <template v-for="(item, index) in items" :key="`${docNo(item)}-${sourceLabel(item)}`">
                        <div
                            class="reference-card"
                            :class="[`status-${item.paperlessStatus || 'missing'}`, { 'can-preview': canPreviewPDF(item) }]"
                            :role="canPreviewPDF(item) ? 'button' : undefined"
                            :tabindex="canPreviewPDF(item) ? 0 : undefined"
                            :aria-label="canPreviewPDF(item) ? `ดู PDF เอกสาร ${docNo(item)}` : `เอกสาร ${docNo(item)} ยังไม่มี PDF ใน PaperLess`"
                            @click="openReferencePDF(item)"
                            @keydown.enter.prevent="openReferencePDF(item)"
                            @keydown.space.prevent="openReferencePDF(item)"
                        >
                            <span class="reference-card-topline">
                                <span class="reference-status-dot" aria-hidden="true"></span>
                                <span class="reference-card-type">{{ docFormat(item) }}</span>
                            </span>
                            <strong class="reference-card-doc">{{ docNo(item) }}</strong>
                            <span class="reference-card-meta">{{ docDate(item) }}</span>
                            <span class="reference-card-meta">จาก {{ sourceLabel(item) }}</span>
                            <span class="reference-card-tags">
                                <Tag :value="statusMeta(item).label" :severity="statusMeta(item).severity" :icon="statusMeta(item).icon" />
                                <Tag v-if="canPreviewPDF(item)" value="กดดู PDF" severity="info" />
                            </span>
                        </div>
                        <span v-if="index < items.length - 1" class="reference-connector" :class="`status-${item.paperlessStatus || 'missing'}`" aria-hidden="true"></span>
                    </template>
                </div>
            </div>
            <div v-else class="reference-list">
                <div
                    v-for="item in items"
                    :key="`${docNo(item)}-${sourceLabel(item)}`"
                    class="reference-item"
                    :class="[`status-${item.paperlessStatus || 'missing'}`, { 'can-preview': canPreviewPDF(item) }]"
                    :role="canPreviewPDF(item) ? 'button' : undefined"
                    :tabindex="canPreviewPDF(item) ? 0 : undefined"
                    :aria-label="canPreviewPDF(item) ? `ดู PDF เอกสาร ${docNo(item)}` : `เอกสาร ${docNo(item)} ยังไม่มี PDF ใน PaperLess`"
                    @click="openReferencePDF(item)"
                    @keydown.enter.prevent="openReferencePDF(item)"
                    @keydown.space.prevent="openReferencePDF(item)"
                >
                    <span class="reference-status-dot" aria-hidden="true"></span>
                    <div class="reference-item-main">
                        <div class="reference-item-top">
                            <Tag :value="statusMeta(item).label" :severity="statusMeta(item).severity" :icon="statusMeta(item).icon" />
                            <div class="reference-doc-line">
                                <strong>{{ docNo(item) }}</strong>
                            </div>
                        </div>
                        <div class="reference-meta-line">
                            <span>{{ docDate(item) }}</span>
                            <span>{{ docFormat(item) }}</span>
                            <span>จาก {{ sourceLabel(item) }}</span>
                        </div>
                    </div>
                    <Button
                        v-if="allowPreview"
                        :icon="canPreviewPDF(item) ? 'pi pi-file-pdf' : 'pi pi-ban'"
                        size="small"
                        rounded
                        outlined
                        :severity="canPreviewPDF(item) ? 'secondary' : 'danger'"
                        :disabled="!canPreviewPDF(item)"
                        :aria-label="canPreviewPDF(item) ? 'ดู PDF' : 'ยังไม่มี PDF'"
                        @click.stop="openReferencePDF(item)"
                    />
                </div>
            </div>
        </div>

        <DataTable v-else :value="items" :loading="loading" dataKey="doc_no" responsiveLayout="scroll" stripedRows :paginator="items.length > 10" :rows="10">
            <template #empty>
                <div class="reference-empty">
                    <i class="pi pi-inbox"></i>
                    <span>{{ loading ? 'กำลังโหลดเอกสารอ้างอิง' : 'ยังไม่พบเอกสารอ้างอิงก่อนหน้าใน SML' }}</span>
                </div>
            </template>
            <Column header="สถานะ" style="min-width: 13rem">
                <template #body="{ data }">
                    <Tag :value="statusMeta(data).label" :severity="statusMeta(data).severity" :icon="statusMeta(data).icon" />
                </template>
            </Column>
            <Column header="วันที่เอกสาร" style="min-width: 10rem">
                <template #body="{ data }">{{ formatDocumentDate(data.doc_date || data.docDate) }}</template>
            </Column>
            <Column header="เลขที่เอกสาร" style="min-width: 13rem">
                <template #body="{ data }">
                    <Button v-if="canPreviewPDF(data)" link class="p-0 font-bold text-left" @click.stop="openReferencePDF(data)">{{ docNo(data) }}</Button>
                    <strong v-else>{{ docNo(data) }}</strong>
                </template>
            </Column>
            <Column header="ชนิดเอกสาร" style="min-width: 14rem">
                <template #body="{ data }">{{ docFormat(data) }}</template>
            </Column>
            <Column header="แหล่งอ้างอิง" style="min-width: 13rem">
                <template #body="{ data }">{{ sourceLabel(data) }}</template>
            </Column>
            <Column header="จัดการ" style="min-width: 10rem">
                <template #body="{ data }">
                    <Button
                        :label="canPreviewPDF(data) ? 'ดูเอกสาร' : 'ยังไม่มี PDF'"
                        :icon="canPreviewPDF(data) ? 'pi pi-file-pdf' : 'pi pi-ban'"
                        size="small"
                        outlined
                        :severity="canPreviewPDF(data) ? 'secondary' : 'danger'"
                        :disabled="!canPreviewPDF(data)"
                        @click.stop="openReferencePDF(data)"
                    />
                </template>
            </Column>
        </DataTable>

        <ReadOnlyPdfDialog v-model:visible="pdfDialog" :url="pdfUrl" :title="pdfTitle" :action-url="pdfActionUrl" action-label="เปิด PaperLess" full-height />
    </div>
</template>

<style scoped>
.reference-check {
    --reference-success: var(--p-green-500, #22c55e);
    --reference-warning: var(--p-orange-500, #f97316);
    --reference-danger: var(--p-red-500, #ef4444);
    display: grid;
    gap: 0.65rem;
}

.reference-check.compact {
    min-height: 0;
    display: flex;
    flex-direction: column;
}

.reference-check.dialog-mode {
    height: 100%;
    flex: 1 1 auto;
    gap: 0.55rem;
    background: color-mix(in srgb, var(--orange-50) 16%, var(--surface-card));
}

.reference-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.reference-check.dialog-mode .reference-head {
    flex: 0 0 auto;
    align-items: center;
    padding-bottom: 0.55rem;
    border-bottom: 1px solid var(--surface-border);
}

.reference-actions {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
}

.reference-summary {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-start;
    gap: 0.35rem;
}

.reference-empty {
    min-height: 5.25rem;
    border: 1px dashed var(--surface-border);
    border-radius: 8px;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.5rem;
    color: var(--text-color-secondary);
    text-align: center;
    padding: 0.85rem;
}

.reference-compact {
    min-width: 0;
}

.reference-check.compact .reference-compact {
    min-height: 0;
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
}

.reference-check.dialog-mode .reference-compact {
    overflow: hidden;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: color-mix(in srgb, var(--surface-ground) 72%, var(--surface-card));
}

.reference-list {
    display: grid;
    gap: 0.45rem;
}

.reference-check.compact .reference-list {
    min-height: 0;
    flex: 1 1 auto;
    align-content: start;
}

.reference-item {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.6rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.58rem 0.68rem;
    background: var(--surface-card);
}

.reference-item.can-preview {
    cursor: pointer;
    transition:
        border-color 0.15s ease,
        background-color 0.15s ease,
        transform 0.15s ease;
}

.reference-item.can-preview:hover,
.reference-item.can-preview:focus-visible {
    border-color: var(--primary-color);
    outline: none;
    transform: translateY(-1px);
}

.reference-item.status-completed {
    border-color: color-mix(in srgb, var(--reference-success) 42%, var(--surface-border));
    background: color-mix(in srgb, var(--reference-success) 6%, var(--surface-card));
}

.reference-item.status-in_progress {
    border-color: color-mix(in srgb, var(--reference-warning) 42%, var(--surface-border));
    background: color-mix(in srgb, var(--reference-warning) 6%, var(--surface-card));
}

.reference-item.status-missing,
.reference-item.status-rejected,
.reference-item.status-draft,
.reference-item.status-pending_confirm,
.reference-item.status-auto_confirming,
.reference-item.status-completed_evidence_failed,
.reference-item.status-completed_image_failed,
.reference-item.status-completed_lock_failed {
    border-color: color-mix(in srgb, var(--reference-danger) 38%, var(--surface-border));
    background: color-mix(in srgb, var(--reference-danger) 5%, var(--surface-card));
}

.reference-flow-scroll {
    min-height: 18rem;
    overflow: auto;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.85rem;
    background: color-mix(in srgb, var(--surface-ground) 68%, var(--surface-card));
}

.reference-check.compact .reference-flow-scroll {
    min-height: 0;
    flex: 1 1 auto;
}

.reference-check.dialog-mode .reference-flow-scroll {
    height: 100%;
    border: 1px solid color-mix(in srgb, var(--orange-200) 48%, var(--surface-border));
    border-radius: 8px;
    background: color-mix(in srgb, var(--orange-50) 38%, var(--surface-ground));
}

.reference-flow-row {
    min-width: max-content;
    display: flex;
    align-items: center;
    gap: 0;
}

.reference-card {
    width: 15.5rem;
    min-height: 7.35rem;
    display: grid;
    align-content: start;
    gap: 0.16rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.55rem 0.6rem;
    background: var(--surface-card);
    color: var(--text-color);
}

.reference-card.can-preview {
    cursor: pointer;
    transition:
        border-color 0.15s ease,
        background-color 0.15s ease,
        transform 0.15s ease;
}

.reference-card.can-preview:hover,
.reference-card.can-preview:focus-visible {
    border-color: var(--orange-500);
    outline: none;
    transform: translateY(-1px);
}

:global(.reference-check-dialog.reference-audit-dialog .p-dialog-header) {
    border-bottom: 1px solid color-mix(in srgb, var(--orange-200) 58%, var(--surface-border));
    background: color-mix(in srgb, var(--orange-50) 42%, var(--surface-card));
}

:global(.reference-check-dialog.reference-audit-dialog .p-dialog-content) {
    background: color-mix(in srgb, var(--orange-50) 18%, var(--surface-card));
}

:global(.reference-check-dialog.reference-audit-dialog .reference-dialog-layout) {
    background: color-mix(in srgb, var(--orange-50) 16%, var(--surface-card));
}

:global(.reference-audit-dialog .reference-dialog-title) {
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 0.75rem;
}

:global(.reference-audit-dialog .reference-dialog-title-icon) {
    width: 2.35rem;
    height: 2.35rem;
    display: inline-grid;
    place-items: center;
    flex: 0 0 auto;
    border: 1px solid color-mix(in srgb, var(--orange-500) 35%, var(--surface-border));
    border-radius: 10px;
    background: color-mix(in srgb, var(--orange-50) 78%, var(--surface-card));
    color: var(--orange-600);
}

:global(.reference-audit-dialog .reference-dialog-title-copy) {
    min-width: 0;
    display: grid;
    gap: 0.1rem;
}

:global(.reference-audit-dialog .reference-dialog-title-copy strong),
:global(.reference-audit-dialog .reference-dialog-title-copy small) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

:global(.reference-audit-dialog .reference-dialog-title-copy strong) {
    font-size: 1rem;
    color: var(--text-color);
}

:global(.reference-audit-dialog .reference-dialog-title-copy small) {
    font-size: 0.86rem;
    color: var(--text-color-secondary);
}

.reference-card.status-completed {
    border-color: color-mix(in srgb, var(--reference-success) 46%, var(--surface-border));
    background: color-mix(in srgb, var(--reference-success) 6%, var(--surface-card));
}

.reference-card.status-in_progress {
    border-color: color-mix(in srgb, var(--reference-warning) 46%, var(--surface-border));
    background: color-mix(in srgb, var(--reference-warning) 7%, var(--surface-card));
}

.reference-card.status-missing,
.reference-card.status-rejected,
.reference-card.status-draft,
.reference-card.status-pending_confirm,
.reference-card.status-auto_confirming,
.reference-card.status-completed_evidence_failed,
.reference-card.status-completed_image_failed,
.reference-card.status-completed_lock_failed {
    border-color: color-mix(in srgb, var(--reference-danger) 42%, var(--surface-border));
    background: color-mix(in srgb, var(--reference-danger) 5%, var(--surface-card));
}

.reference-card-topline {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 0.4rem;
}

.reference-status-dot {
    width: 0.62rem;
    height: 0.62rem;
    border-radius: 999px;
    background: var(--reference-danger);
    flex: 0 0 auto;
}

.reference-card.status-completed .reference-status-dot {
    background: var(--reference-success);
}

.reference-card.status-in_progress .reference-status-dot {
    background: var(--reference-warning);
}

.reference-item-main {
    min-width: 0;
    display: grid;
    gap: 0.22rem;
}

.reference-item-top {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 0.5rem;
}

.reference-item-top:deep(.p-tag) {
    padding: 0.18rem 0.38rem;
    font-size: 0.72rem;
    white-space: nowrap;
}

.reference-doc-line {
    min-width: 0;
    overflow-wrap: anywhere;
    font-size: 0.95rem;
}

.reference-meta-line {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.7rem;
    color: var(--text-color-secondary);
    font-size: 0.82rem;
    overflow-wrap: anywhere;
}

.reference-item > :deep(.p-button.p-button-sm) {
    width: 1.9rem;
    height: 1.9rem;
    padding: 0;
    flex: 0 0 auto;
    justify-self: end;
}

.reference-card-type,
.reference-card-meta {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.reference-card-type {
    color: var(--text-color);
    font-size: 0.86rem;
    font-weight: 700;
}

.reference-card-doc {
    min-width: 0;
    color: var(--primary-color);
    font-size: 1rem;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.reference-card-meta {
    color: var(--text-color-secondary);
    font-size: 0.78rem;
}

.reference-card-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.28rem;
    margin-top: 0.2rem;
}

.reference-card-tags:deep(.p-tag) {
    max-width: 100%;
    padding: 0.15rem 0.34rem;
    font-size: 0.68rem;
}

.reference-connector {
    width: 2.15rem;
    height: 2px;
    background: color-mix(in srgb, var(--text-color-secondary) 34%, transparent);
}

.reference-connector.status-completed {
    background: color-mix(in srgb, var(--reference-success) 58%, var(--surface-border));
}

.reference-connector.status-in_progress {
    background: color-mix(in srgb, var(--reference-warning) 58%, var(--surface-border));
}

.reference-connector.status-missing,
.reference-connector.status-rejected,
.reference-connector.status-draft,
.reference-connector.status-pending_confirm,
.reference-connector.status-auto_confirming,
.reference-connector.status-completed_evidence_failed,
.reference-connector.status-completed_image_failed,
.reference-connector.status-completed_lock_failed {
    background: color-mix(in srgb, var(--reference-danger) 56%, var(--surface-border));
}

.compact .reference-head {
    align-items: center;
    flex-direction: row;
    gap: 0.75rem;
}

.compact .reference-actions {
    width: 100%;
    justify-content: space-between;
    align-items: center;
}

.compact .reference-summary {
    justify-content: flex-start;
}

.compact .reference-summary:deep(.p-tag) {
    padding: 0.18rem 0.4rem;
    font-size: 0.72rem;
}

@media (max-width: 720px) {
    .reference-head {
        flex-direction: column;
    }

    .reference-actions {
        width: 100%;
    }

    .reference-item {
        width: 100%;
        grid-template-columns: auto minmax(0, 1fr);
        align-items: flex-start;
    }

    .reference-item > :deep(.p-button.p-button-sm) {
        grid-column: 2;
        justify-self: start;
    }

    .reference-item-top {
        align-items: flex-start;
        flex-direction: column;
        gap: 0.35rem;
    }

    .reference-flow-scroll {
        min-height: 16rem;
        padding: 0.65rem;
    }

    .reference-flow-row {
        min-width: 0;
        width: 100%;
        display: grid;
        gap: 0.55rem;
    }

    .reference-card {
        width: 100%;
    }

    .reference-connector {
        display: none;
    }

    .compact .reference-head {
        align-items: flex-start;
        flex-direction: column;
    }

    .compact .reference-actions {
        width: 100%;
    }

    .compact .reference-summary {
        justify-content: flex-start;
    }

}
</style>
