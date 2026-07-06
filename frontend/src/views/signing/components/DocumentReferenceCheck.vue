<script setup>
import { formatDocumentDate } from '@/utils/signingFormatters';
import { computed, onMounted, ref, watch } from 'vue';

const props = defineProps({
    document: { type: Object, default: null },
    loader: { type: Function, required: true },
    compact: { type: Boolean, default: false }
});

const emit = defineEmits(['open-document']);

const loading = ref(false);
const error = ref('');
const referenceCheck = ref(null);
const requestSeq = ref(0);

const items = computed(() => referenceCheck.value?.items || []);
const summary = computed(() => referenceCheck.value?.summary || { total: 0, missing: 0, inProgress: 0, completed: 0 });
const warnings = computed(() => referenceCheck.value?.warnings || []);
const summaryItems = computed(() => [
    { label: 'ทั้งหมด', value: summary.value.total || 0, severity: 'info' },
    { label: 'ยังไม่เข้า', value: summary.value.missing || 0, severity: 'danger' },
    { label: 'กำลังเซ็น', value: summary.value.inProgress || 0, severity: 'warn' },
    { label: 'ครบแล้ว', value: summary.value.completed || 0, severity: 'success' }
]);

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

function openPaperless(item = {}) {
    const id = item.paperlessDocumentId || item.paperless_document_id;
    if (!id || !item.canOpenPaperless) return;
    emit('open-document', id);
}
</script>

<template>
    <div class="reference-check" :class="{ compact }">
        <div class="reference-head">
            <div>
                <div class="font-semibold">ตรวจสอบเอกสารอ้างอิง</div>
                <small class="text-muted-color">ดูว่าเอกสารก่อนหน้าจาก SML ถูกนำเข้าและเซ็นครบใน PaperLess แล้วหรือยัง</small>
            </div>
            <div class="reference-actions">
                <div class="reference-summary">
                    <Tag v-for="item in summaryItems" :key="item.label" :value="`${item.label} ${item.value}`" :severity="item.severity" />
                </div>
                <Button icon="pi pi-refresh" rounded outlined severity="secondary" aria-label="โหลดใหม่" :loading="loading" @click="load" />
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
            <div v-else class="reference-list">
                <div v-for="item in items" :key="`${docNo(item)}-${sourceLabel(item)}`" class="reference-item" :class="`status-${item.paperlessStatus || 'missing'}`">
                    <div class="reference-item-main">
                        <Tag :value="statusMeta(item).label" :severity="statusMeta(item).severity" :icon="statusMeta(item).icon" />
                        <div class="reference-doc-line">
                            <Button v-if="item.canOpenPaperless" link class="p-0 font-bold text-left" @click="openPaperless(item)">{{ docNo(item) }}</Button>
                            <strong v-else>{{ docNo(item) }}</strong>
                        </div>
                        <small class="text-muted-color">{{ docFormat(item) }}</small>
                        <dl class="reference-meta">
                            <dt>วันที่</dt>
                            <dd>{{ docDate(item) }}</dd>
                            <dt>อ้างอิงจาก</dt>
                            <dd>{{ sourceLabel(item) }}</dd>
                        </dl>
                    </div>
                    <Button
                        :label="item.canOpenPaperless ? 'รายละเอียด' : 'ยังไม่มี PDF'"
                        :icon="item.canOpenPaperless ? 'pi pi-external-link' : 'pi pi-ban'"
                        size="small"
                        outlined
                        :severity="item.canOpenPaperless ? 'secondary' : 'danger'"
                        :disabled="!item.canOpenPaperless"
                        @click="openPaperless(item)"
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
                    <Button v-if="data.canOpenPaperless" link class="p-0 font-bold text-left" @click="openPaperless(data)">{{ docNo(data) }}</Button>
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
                        :label="data.canOpenPaperless ? 'รายละเอียด' : 'ยังไม่มี PDF'"
                        :icon="data.canOpenPaperless ? 'pi pi-external-link' : 'pi pi-ban'"
                        size="small"
                        outlined
                        :severity="data.canOpenPaperless ? 'secondary' : 'danger'"
                        :disabled="!data.canOpenPaperless"
                        @click="openPaperless(data)"
                    />
                </template>
            </Column>
        </DataTable>
    </div>
</template>

<style scoped>
.reference-check {
    display: grid;
    gap: 0.85rem;
}

.reference-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.reference-actions {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
}

.reference-summary {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.35rem;
}

.reference-empty {
    min-height: 8rem;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.5rem;
    color: var(--text-color-secondary);
    text-align: center;
}

.reference-compact {
    min-width: 0;
}

.reference-list {
    display: grid;
    gap: 0.65rem;
}

.reference-item {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
    border: 1px solid var(--surface-border);
    border-left-width: 4px;
    border-radius: 8px;
    padding: 0.75rem;
    background: var(--surface-card);
}

.reference-item.status-completed {
    border-left-color: var(--green-500);
}

.reference-item.status-in_progress {
    border-left-color: var(--orange-500);
}

.reference-item.status-missing,
.reference-item.status-rejected,
.reference-item.status-draft,
.reference-item.status-pending_confirm,
.reference-item.status-auto_confirming,
.reference-item.status-completed_evidence_failed,
.reference-item.status-completed_image_failed,
.reference-item.status-completed_lock_failed {
    border-left-color: var(--red-500);
}

.reference-item-main {
    min-width: 0;
    display: grid;
    gap: 0.35rem;
}

.reference-doc-line {
    min-width: 0;
    overflow-wrap: anywhere;
}

.reference-meta {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 0.25rem 0.5rem;
    margin: 0.15rem 0 0;
    font-size: 0.85rem;
}

.reference-meta dt {
    color: var(--text-color-secondary);
}

.reference-meta dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
}

.compact .reference-head {
    flex-direction: column;
}

.compact .reference-actions {
    width: 100%;
    justify-content: space-between;
}

.compact .reference-summary {
    justify-content: flex-start;
}

@media (max-width: 720px) {
    .reference-head {
        flex-direction: column;
    }

    .reference-actions,
    .reference-item {
        width: 100%;
    }

    .reference-item {
        flex-direction: column;
    }
}
</style>
