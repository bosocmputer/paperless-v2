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
            <div class="flex flex-wrap gap-2 justify-end">
                <Tag :value="`${summary.total || 0} รายการ`" severity="info" />
                <Tag :value="`ยังไม่เข้า ${summary.missing || 0}`" severity="danger" />
                <Tag :value="`กำลังเซ็น ${summary.inProgress || 0}`" severity="warn" />
                <Tag :value="`ครบแล้ว ${summary.completed || 0}`" severity="success" />
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

        <DataTable :value="items" :loading="loading" dataKey="doc_no" responsiveLayout="scroll" stripedRows :paginator="items.length > 10" :rows="10">
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

.reference-empty {
    min-height: 8rem;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.5rem;
    color: var(--text-color-secondary);
    text-align: center;
}

@media (max-width: 720px) {
    .reference-head {
        flex-direction: column;
    }
}
</style>
