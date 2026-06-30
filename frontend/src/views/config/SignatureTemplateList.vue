<script setup>
import { api } from '@/services/api';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const docFormats = ref([]);
const configs = ref([]);
const templateStates = ref({});
const loading = ref(false);
const error = ref('');
const searchQuery = ref('');

const rows = computed(() => {
    const groups = new Map();
    configs.value.forEach((config) => {
        const code = String(config.docFormatCode || '').trim();
        if (!code) return;
        if (!groups.has(code)) {
            groups.set(code, {
                docFormatCode: code,
                positions: []
            });
        }
        groups.get(code).positions.push(config);
    });

    return [...groups.values()]
        .map((row) => {
            const positions = [...row.positions].sort((left, right) => Number(left.sequenceNo || 0) - Number(right.sequenceNo || 0));
            const state = templateStates.value[row.docFormatCode] || null;
            const template = state?.active || state?.draft || null;
            const issues = state?.active ? state.activeIssues || [] : state?.draft ? state.draftIssues || [] : [];
            const requiredBoxCount = positions.reduce((total, step) => total + requiredBoxesForStep(step), 0);
            const boxCount = template?.boxes?.length || 0;
            const progressPercent = requiredBoxCount ? Math.min(100, Math.round((Math.min(boxCount, requiredBoxCount) / requiredBoxCount) * 100)) : 0;
            const enriched = {
                ...row,
                positions,
                state,
                template,
                issues,
                format: formatDetail(row.docFormatCode),
                requiredBoxCount,
                boxCount,
                progressPercent
            };
            return {
                ...enriched,
                status: resolveTemplateStatus(enriched),
                firstIssue: issues[0] || null
            };
        })
        .sort((left, right) => left.docFormatCode.localeCompare(right.docFormatCode, 'th'));
});

const readyCount = computed(() => rows.value.filter((row) => row.status.severity === 'success').length);
const needsWorkCount = computed(() => rows.value.filter((row) => ['warn', 'danger'].includes(row.status.severity)).length);
const noPdfCount = computed(() => rows.value.filter((row) => !row.template?.sampleFileId).length);
const filteredRows = computed(() => {
    const query = normalizeSearch(searchQuery.value);
    if (!query) return rows.value;
    return rows.value.filter((row) =>
        normalizeSearch(
            `${row.docFormatCode} ${formatName(row)} ${formatPattern(row)} ${row.status.label} ${statusHelper(row)} ${positionPreview(row)} ${pdfLabel(row)} ${pdfFileName(row)}`
        ).includes(query)
    );
});

onMounted(loadPage);

async function loadPage() {
    loading.value = true;
    error.value = '';
    try {
        const [formatsResult, configsResult] = await Promise.all([api.listSMLDocFormats(), api.listDocumentConfigs()]);
        docFormats.value = formatsResult.docFormats || [];
        configs.value = configsResult.configs || [];

        const codes = [...new Set(configs.value.map((item) => item.docFormatCode).filter(Boolean))];
        const states = {};
        await Promise.all(
            codes.map(async (code) => {
                try {
                    states[code] = await api.getSignatureTemplateState(code);
                } catch (err) {
                    states[code] = { error: err.message };
                }
            })
        );
        templateStates.value = states;
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลดรายการ Template ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openDesigner(code) {
    router.push({ name: 'signature-template', params: { docFormatCode: code } });
}

function openDocumentConfig() {
    router.push({ name: 'document-config' });
}

function formatDetail(code) {
    return docFormats.value.find((item) => sameCode(item.code, code));
}

function formatName(row) {
    return row.format?.name_1 || row.format?.name_2 || row.format?.format || '-';
}

function formatPattern(row) {
    return row.format?.format || '';
}

function resolveTemplateStatus(row) {
    if (row.state?.error) return { label: 'โหลดสถานะไม่ได้', severity: 'danger' };
    if (!row.template) return { label: 'ยังไม่ได้เริ่ม', severity: 'secondary' };
    if (!row.template.sampleFileId) return { label: 'รออัปโหลด PDF', severity: 'warn' };
    if (row.issues.length > 0) return { label: 'ต้องแก้ไข', severity: 'warn' };
    if (row.boxCount > 0) return { label: 'มี preset', severity: 'success' };
    return { label: 'มี PDF ยังไม่วางกรอบ', severity: 'info' };
}

function statusHelper(row) {
    if (row.state?.error) return row.state.error;
    if (!row.template) return 'เปิดเข้าไปอัปโหลด PDF เพื่อสร้าง preset ใช้เป็นค่าเริ่มต้นตอนส่งเซ็น';
    if (!row.template.sampleFileId) return 'ยังไม่มี PDF ตัวอย่าง';
    if (row.firstIssue) return issueLabel(row.firstIssue);
    if (row.boxCount === 0) return 'ยังไม่มีกรอบ preset แต่ไม่กระทบการส่งเซ็นจริง';
    return 'ใช้เป็น preset ได้ และแก้กรอบอีกครั้งตอน upload เอกสารจริง';
}

function issueLabel(issue) {
    const labels = {
        sample_pdf_required: 'ต้องอัปโหลด PDF ตัวอย่าง',
        pdf_too_many_pages: 'PDF มีจำนวนหน้าเกินกำหนด',
        document_config_required: 'ต้องมี Config เอกสารก่อน',
        condition_any_box_required: 'ยังขาดกรอบสำหรับเงื่อนไขคนใดคนหนึ่ง',
        condition_any_box_count_invalid: 'จำนวนกรอบเกินเงื่อนไขคนใดคนหนึ่ง',
        condition_any_type_invalid: 'ประเภทกรอบไม่ตรงกับเงื่อนไข',
        condition_all_users_required: 'ต้องกำหนด user ใน position นี้',
        condition_all_box_count_invalid: 'จำนวนกรอบไม่ตรงกับจำนวน user',
        condition_all_missing_user_box: 'ยังขาดกรอบของ user บางคน',
        condition_all_duplicate_user_box: 'มีกรอบ user ซ้ำ',
        condition_all_unknown_user_box: 'มี user นอก config',
        condition_external_box_required: 'ยังขาดกรอบบุคคลภายนอก',
        condition_external_box_count_invalid: 'จำนวนกรอบบุคคลภายนอกเกินเงื่อนไข',
        condition_external_type_invalid: 'ประเภทกรอบไม่ตรงกับบุคคลภายนอก',
        box_position_unknown: 'มีกรอบที่ไม่ตรงกับ position config',
        box_bounds_invalid: 'มีกรอบอยู่นอกหน้า PDF'
    };
    const prefix = issue.positionCode ? `Position ${issue.positionCode}: ` : '';
    return `${prefix}${labels[issue.code] || issue.message || 'ต้องตรวจสอบ Template'}`;
}

function pdfLabel(row) {
    if (!row.template) return 'ยังไม่มี Template';
    if (!row.template.sampleFileId) return 'ยังไม่มี PDF';
    const pageCount = Number(row.template.sampleFile?.pageCount || 0);
    return pageCount > 0 ? `${pageCount} หน้า` : 'มี PDF แล้ว';
}

function pdfFileName(row) {
    return row.template?.sampleFile?.originalName || '';
}

function lastUpdated(row) {
    const value = row.template?.updatedAt;
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

function conditionSummary(row) {
    const values = new Set(row.positions.map((item) => Number(item.conditionType)));
    return [...values].sort((a, b) => a - b);
}

function conditionLabel(value) {
    if (value === 1) return 'คนใดคนหนึ่ง';
    if (value === 2) return 'ทุกคน';
    return 'บุคคลภายนอก';
}

function conditionSeverity(value) {
    if (value === 1) return 'info';
    if (value === 2) return 'warn';
    return 'secondary';
}

function requiredBoxesForStep(step) {
    if (Number(step.conditionType) === 2) return stepUsers(step).length;
    return 1;
}

function stepUsers(step) {
    return [step.user01, step.user02, step.user03].map((item) => String(item || '').trim()).filter(Boolean);
}

function positionPreview(row) {
    return row.positions.map((item) => `${item.positionCode}:${item.positionName}`).join(', ');
}

function sameCode(left, right) {
    return String(left || '').toLowerCase() === String(right || '').toLowerCase();
}

function normalizeSearch(value) {
    return String(value || '').toLowerCase().trim();
}
</script>

<template>
    <div class="card signature-template-page">
        <div class="page-header">
            <div>
                <div class="font-semibold text-xl mb-1">Preset กรอบลายเซ็น</div>
                <p class="text-muted-color m-0">กำหนดกรอบเริ่มต้นไว้ช่วยตอน upload เอกสารจริง ไม่ใช่เงื่อนไขบังคับของ workflow</p>
            </div>
            <div class="header-actions">
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหา doc, PDF, สถานะ" class="w-full sm:w-80" />
                <Button icon="pi pi-refresh" severity="secondary" outlined :loading="loading" aria-label="โหลดใหม่" @click="loadPage" />
                <Button label="แก้ Config เอกสาร" icon="pi pi-file-edit" severity="secondary" outlined @click="openDocumentConfig" />
            </div>
        </div>

        <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

        <div v-if="rows.length > 0" class="summary-strip" aria-label="สรุปรายการ Template">
            <div class="summary-item">
                <span class="summary-label">เอกสารทั้งหมด</span>
                <strong>{{ rows.length }}</strong>
            </div>
            <div class="summary-item">
                <span class="summary-label">มี preset</span>
                <strong>{{ readyCount }}</strong>
            </div>
            <div class="summary-item">
                <span class="summary-label">ต้องแก้ไข</span>
                <strong>{{ needsWorkCount }}</strong>
            </div>
            <div class="summary-item">
                <span class="summary-label">ยังไม่มี PDF</span>
                <strong>{{ noPdfCount }}</strong>
            </div>
        </div>

        <DataTable :value="filteredRows" :loading="loading" dataKey="docFormatCode" responsiveLayout="scroll" stripedRows>
            <template #empty>
                <div class="empty-state">
                    <i class="pi pi-file-edit"></i>
                    <div class="font-semibold">{{ searchQuery ? 'ไม่พบ preset ที่ค้นหา' : 'ยังไม่มีเอกสารสำหรับทำ preset กรอบลายเซ็น' }}</div>
                    <p class="text-muted-color m-0">{{ searchQuery ? 'ลองค้นหาด้วย doc code, ชื่อ PDF หรือสถานะอื่น' : 'เพิ่ม Position ใน Config เอกสารก่อน แล้วค่อยสร้าง preset จากหน้านี้' }}</p>
                    <Button v-if="!searchQuery" label="ไปที่ Config เอกสาร" icon="pi pi-file-edit" class="mt-3" @click="openDocumentConfig" />
                </div>
            </template>

            <Column header="เอกสาร" style="min-width: 18rem">
                <template #body="{ data }">
                    <div class="doc-cell">
                        <div class="doc-code">{{ data.docFormatCode }}</div>
                        <div class="font-medium">{{ formatName(data) }}</div>
                        <div v-if="formatPattern(data)" class="text-sm text-muted-color">{{ formatPattern(data) }}</div>
                    </div>
                </template>
            </Column>

            <Column header="ความพร้อม" style="min-width: 18rem">
                <template #body="{ data }">
                    <div class="status-cell">
                        <div class="status-line">
                            <Tag :value="data.status.label" :severity="data.status.severity" />
                            <span class="box-ratio">{{ data.boxCount }} กรอบ preset</span>
                        </div>
                        <div class="progress-track" :aria-label="`กรอบ preset ${data.boxCount} จาก ${data.requiredBoxCount}`">
                            <span class="progress-fill" :class="{ complete: data.status.severity === 'success', invalid: data.issues.length > 0 }" :style="{ width: `${data.progressPercent}%` }"></span>
                        </div>
                        <div class="helper-text">{{ statusHelper(data) }}</div>
                    </div>
                </template>
            </Column>

            <Column header="ขั้นตอน" style="min-width: 22rem">
                <template #body="{ data }">
                    <div class="font-medium">{{ data.positions.length }} positions</div>
                    <div class="position-preview">{{ positionPreview(data) }}</div>
                    <div class="condition-tags">
                        <Tag v-for="condition in conditionSummary(data)" :key="condition" :value="`${condition} - ${conditionLabel(condition)}`" :severity="conditionSeverity(condition)" />
                    </div>
                </template>
            </Column>

            <Column header="PDF" style="min-width: 14rem">
                <template #body="{ data }">
                    <div class="font-medium">{{ pdfLabel(data) }}</div>
                    <div class="pdf-name">{{ pdfFileName(data) || 'รออัปโหลดจาก Designer' }}</div>
                </template>
            </Column>

            <Column header="แก้ไขล่าสุด" style="min-width: 12rem">
                <template #body="{ data }">
                    <span class="text-muted-color">{{ lastUpdated(data) }}</span>
                </template>
            </Column>

            <Column header="จัดการ" style="min-width: 12rem">
                <template #body="{ data }">
                    <div class="row-actions">
                        <Button label="แก้ preset" icon="pi pi-pencil" severity="info" @click="openDesigner(data.docFormatCode)" />
                    </div>
                </template>
            </Column>
        </DataTable>
    </div>
</template>

<style scoped>
.signature-template-page {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.page-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.header-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
}

.summary-strip {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: 0.75rem;
}

.summary-item {
    display: flex;
    min-height: 4rem;
    flex-direction: column;
    justify-content: center;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.75rem 1rem;
    background: var(--surface-ground);
}

.summary-label {
    color: var(--text-color-secondary);
    font-size: 0.85rem;
}

.summary-item strong {
    color: var(--text-color);
    font-size: 1.35rem;
    line-height: 1.2;
}

.empty-state {
    display: flex;
    min-height: 14rem;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    text-align: center;
}

.empty-state i {
    color: var(--text-color-secondary);
    font-size: 2rem;
}

.doc-cell {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 0.2rem;
}

.doc-code {
    color: var(--text-color);
    font-size: 1.2rem;
    font-weight: 700;
}

.status-cell {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 0.45rem;
}

.status-line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
}

.box-ratio {
    color: var(--text-color-secondary);
    font-size: 0.9rem;
}

.progress-track {
    position: relative;
    height: 0.5rem;
    overflow: hidden;
    border-radius: 999px;
    background: var(--p-surface-200, var(--surface-border));
}

.progress-fill {
    position: absolute;
    inset-block: 0;
    left: 0;
    border-radius: inherit;
    background: var(--primary-color);
    transition: width 180ms ease-out;
}

.progress-fill.complete {
    background: var(--p-green-500, #22c55e);
}

.progress-fill.invalid {
    background: var(--p-yellow-500, #eab308);
}

.helper-text,
.position-preview,
.pdf-name {
    color: var(--text-color-secondary);
    font-size: 0.88rem;
}

.position-preview {
    display: -webkit-box;
    max-width: 36rem;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
}

.condition-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin-top: 0.5rem;
}

.pdf-name {
    max-width: 16rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.row-actions {
    display: flex;
    justify-content: flex-end;
}

@media (max-width: 960px) {
    .page-header {
        flex-direction: column;
    }

    .header-actions,
    .row-actions {
        justify-content: flex-start;
    }
}

@media (prefers-reduced-motion: reduce) {
    .progress-fill {
        transition: none;
    }
}
</style>
