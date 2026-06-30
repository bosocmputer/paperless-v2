<script setup>
import { api } from '@/services/api';
import { formatDocumentDate, formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import DocumentLayoutDesigner from './components/DocumentLayoutDesigner.vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const documents = ref([]);
const docFormats = ref([]);
const loading = ref(false);
const creating = ref(false);
const uploading = ref(false);
const loadingLayoutContext = ref(false);
const createVisible = ref(false);
const searchQuery = ref('');
const form = ref(emptyForm());
const candidates = ref([]);
const candidatePage = ref(1);
const candidateTotal = ref(0);
const candidateHasMore = ref(false);
const searchingCandidates = ref(false);
const layoutDesignerRef = ref(null);
let searchTimer;
let suppressSearchWatch = false;
let createOpenedAt = Date.now();
const createSessionId = ref(makeClientId());

const filteredDocuments = computed(() => {
    const query = normalize(searchQuery.value);
    if (!query) return documents.value;
    return documents.value.filter((doc) => normalize(`${doc.docFormatCode} ${doc.docNo} ${doc.partyName} ${doc.status} ${signingStatusLabel(doc.status)}`).includes(query));
});

const docFormatOptions = computed(() =>
    docFormats.value.map((format) => ({
        label: `${format.code} - ${format.name_1 || format.name_2 || format.format || 'ไม่มีชื่อเอกสาร'}`,
        value: format.code
    }))
);

const createDisabledReason = computed(() => {
    if (!form.value.selectedCandidate) return 'เลือกเลขเอกสารจาก SML ก่อน';
    if (!form.value.fileId) return 'อัปโหลด PDF จริงจาก SML ก่อน';
    if (Number(form.value.selectedCandidate.is_lock_record || 0) === 1 && !form.value.confirmLocked) return 'ยืนยันเอกสาร SML ที่ lock แล้วก่อน';
    if (form.value.layoutBoxes.length === 0) return 'วางกรอบลายเซ็นอย่างน้อย 1 กรอบ';
    if (layoutValidationIssues.value.length > 0) return layoutValidationIssues.value[0];
    return '';
});

const createIsDirty = computed(() => createVisible.value && (form.value.selectedCandidate || form.value.fileId || form.value.layoutBoxes.length > 0));

const layoutValidationIssues = computed(() => {
    const issues = [];
    const pageCount = Number(form.value.uploadedFile?.pageCount || 0);
    const boxes = form.value.layoutBoxes || [];
    const configsByPosition = new Map((form.value.configs || []).map((step) => [String(step.positionCode), step]));
    boxes.forEach((box) => {
        if (!configsByPosition.has(String(box.positionCode))) issues.push(`Position ${box.positionCode} ไม่อยู่ใน config`);
        if (box.pageNo < 1 || (pageCount && box.pageNo > pageCount)) issues.push(`กรอบ ${box.label || box.positionCode} อยู่หน้าที่ไม่ถูกต้อง`);
        if (box.xRatio < 0 || box.yRatio < 0 || box.widthRatio <= 0 || box.heightRatio <= 0 || box.xRatio + box.widthRatio > 1 || box.yRatio + box.heightRatio > 1) {
            issues.push(`กรอบ ${box.label || box.positionCode} อยู่นอกหน้า PDF`);
        }
    });
    (form.value.configs || []).forEach((step) => {
        const stepBoxes = boxes.filter((box) => String(box.positionCode) === String(step.positionCode));
        if (stepBoxes.length === 0) return;
        if (step.conditionType === 1 && stepBoxes.length !== 1) issues.push(`${step.positionName} ต้องมี 1 กรอบ`);
        if (step.conditionType === 3 && stepBoxes.length !== 1) issues.push(`${step.positionName} ต้องมี 1 กรอบบุคคลภายนอก`);
        if (step.conditionType === 2) {
            const seen = new Set();
            stepBoxes.forEach((box) => {
                const user = signerUsername(box.signerUser);
                if (!user) issues.push(`${step.positionName} ต้องเลือก user ทุกกรอบ`);
                if (user && seen.has(user)) issues.push(`${step.positionName} มี user ซ้ำ`);
                if (user) seen.add(user);
            });
        }
    });
    return [...new Set(issues)];
});

watch(
    () => form.value.docFormatCode,
    async () => {
        clearTimeout(searchTimer);
        resetCandidateSearch();
        resetUploadedLayout();
        if (createVisible.value) await loadLayoutContext();
    }
);

watch(
    () => form.value.search,
    () => {
        if (suppressSearchWatch) {
            suppressSearchWatch = false;
            return;
        }
        clearTimeout(searchTimer);
        form.value.docNo = '';
        form.value.selectedCandidate = null;
        candidates.value = [];
        candidatePage.value = 1;
        candidateHasMore.value = false;
        if (!form.value.docFormatCode || String(form.value.search || '').trim().length < 2) return;
        searchTimer = setTimeout(() => searchCandidates(1), 300);
    }
);

onMounted(loadPage);
window.addEventListener('beforeunload', beforeUnload);

onBeforeUnmount(() => {
    clearTimeout(searchTimer);
    window.removeEventListener('beforeunload', beforeUnload);
});

onBeforeRouteLeave((_to, _from, next) => {
    if (!createIsDirty.value || window.confirm('ยังไม่ได้ส่งเอกสารและวางกรอบลายเซ็น ต้องการออกจากหน้านี้หรือไม่?')) {
        next();
        return;
    }
    next(false);
});

function emptyForm() {
    return {
        docFormatCode: '',
        search: '',
        docNo: '',
        file: null,
        fileId: '',
        fileUrl: '',
        uploadedFile: null,
        selectedCandidate: null,
        confirmLocked: false,
        configs: [],
        presetTemplate: null,
        selectedPresetId: '',
        layoutBoxes: []
    };
}

async function loadPage() {
    loading.value = true;
    try {
        const [docsResult, formatsResult] = await Promise.all([api.listSigningDocuments(), api.listSMLDocFormats()]);
        documents.value = docsResult.documents || [];
        docFormats.value = formatsResult.docFormats || [];
        if (!form.value.docFormatCode && docFormats.value[0]) form.value.docFormatCode = docFormats.value[0].code;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openCreate() {
    form.value = emptyForm();
    if (docFormats.value[0]) form.value.docFormatCode = docFormats.value[0].code;
    candidates.value = [];
    createSessionId.value = makeClientId();
    createOpenedAt = Date.now();
    createVisible.value = true;
    loadLayoutContext();
    recordCreateEvent('create_layout_open');
}

async function searchCandidates(page = 1) {
    searchingCandidates.value = true;
    try {
        const result = await api.listSMLDocumentCandidates({
            docFormatCode: form.value.docFormatCode,
            search: form.value.search,
            page,
            size: 20
        });
        const rows = result.documents || [];
        candidates.value = page === 1 ? rows : [...candidates.value, ...rows];
        candidatePage.value = result.page || page;
        candidateTotal.value = result.total || 0;
        candidateHasMore.value = !!result.hasMore;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ค้นหา SML ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        searchingCandidates.value = false;
    }
}

function loadMoreCandidates() {
    if (!candidateHasMore.value || searchingCandidates.value) return;
    searchCandidates(candidatePage.value + 1);
}

function selectCandidate(candidate) {
    form.value.selectedCandidate = candidate;
    form.value.docNo = candidate.doc_no;
    suppressSearchWatch = true;
    form.value.search = candidate.doc_no;
    form.value.confirmLocked = false;
}

async function onFileChange(event) {
    const file = event.target.files?.[0] || null;
    if (!file) return;
    form.value.file = file;
    form.value.fileId = '';
    form.value.fileUrl = '';
    form.value.uploadedFile = null;
    form.value.layoutBoxes = [];
    form.value.selectedPresetId = '';
    uploading.value = true;
    try {
        const result = await api.uploadSigningDocumentPDF(file);
        form.value.uploadedFile = result.file;
        form.value.fileId = result.file?.id || '';
        form.value.fileUrl = result.fileUrl || api.signingDocumentUploadPDFUrl(form.value.fileId);
        recordCreateEvent('pdf_upload_success');
        toast.add({ severity: 'success', summary: 'อัปโหลด PDF แล้ว', detail: `${result.file?.pageCount || 0} หน้า`, life: 2500 });
    } catch (err) {
        form.value.file = null;
        recordCreateEvent('pdf_upload_error');
        toast.add({ severity: 'error', summary: 'อัปโหลด PDF ไม่สำเร็จ', detail: err.message, life: 5000 });
    } finally {
        uploading.value = false;
        event.target.value = '';
    }
}

async function createDocument() {
    const disabledReason = createDisabledReason.value;
    if (disabledReason) {
        recordCreateEvent('layout_validation_error');
        toast.add({ severity: 'warn', summary: 'ยังส่งเซ็นไม่ได้', detail: disabledReason, life: 3000 });
        return;
    }
    creating.value = true;
    try {
        const result = await api.createSigningDocument({
            docFormatCode: form.value.docFormatCode,
            docNo: form.value.selectedCandidate.doc_no,
            fileId: form.value.fileId,
            signatureTemplateId: form.value.selectedPresetId,
            confirmLocked: form.value.confirmLocked,
            layoutBoxes: form.value.layoutBoxes.map(toLayoutPayload),
            idempotencyKey: crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`
        });
        createVisible.value = false;
        recordCreateEvent('create_submit_success');
        toast.add({ severity: 'success', summary: 'ส่งเอกสารเพื่อเซ็นแล้ว', life: 2500 });
        await loadPage();
        router.push({ name: 'signing-document-detail', params: { id: result.document.id } });
    } catch (err) {
        recordCreateEvent('create_submit_error');
        toast.add({ severity: 'error', summary: 'สร้างเอกสารไม่สำเร็จ', detail: err.message, life: 5000 });
    } finally {
        creating.value = false;
    }
}

async function loadLayoutContext() {
    if (!form.value.docFormatCode) return;
    loadingLayoutContext.value = true;
    try {
        const [configsResult, templateResult] = await Promise.all([api.listDocumentConfigs({ docFormatCode: form.value.docFormatCode }), api.getSignatureTemplateState(form.value.docFormatCode).catch(() => ({}))]);
        form.value.configs = configsResult.configs || [];
        form.value.presetTemplate = templateResult.active || templateResult.draft || null;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลด config ไม่สำเร็จ', detail: err.message, life: 4500 });
    } finally {
        loadingLayoutContext.value = false;
    }
}

function onApplyPreset(template) {
    form.value.selectedPresetId = template?.id || '';
    toast.add({ severity: 'success', summary: 'ใช้ preset แล้ว', detail: 'ตรวจตำแหน่งกรอบกับ PDF จริงก่อนส่งเซ็น', life: 3000 });
}

function onDesignerEvent(eventName) {
    if (eventName === 'preset_page_mismatch') {
        recordCreateEvent('layout_validation_error');
        toast.add({ severity: 'warn', summary: 'ใช้ preset ไม่ได้', detail: 'จำนวนหน้า PDF ของ preset ไม่ตรงกับ PDF จริง', life: 4000 });
        return;
    }
    recordCreateEvent(eventName);
}

function closeCreate() {
    if (createIsDirty.value && !window.confirm('ยังไม่ได้ส่งเอกสารและวางกรอบลายเซ็น ต้องการปิดหน้าต่างนี้หรือไม่?')) return;
    createVisible.value = false;
}

function beforeUnload(event) {
    if (!createIsDirty.value) return;
    event.preventDefault();
    event.returnValue = '';
}

function resetCandidateSearch() {
    form.value.search = '';
    form.value.docNo = '';
    form.value.selectedCandidate = null;
    candidates.value = [];
    candidatePage.value = 1;
    candidateTotal.value = 0;
    candidateHasMore.value = false;
}

function resetUploadedLayout() {
    form.value.file = null;
    form.value.fileId = '';
    form.value.fileUrl = '';
    form.value.uploadedFile = null;
    form.value.layoutBoxes = [];
    form.value.selectedPresetId = '';
}

function toLayoutPayload(box) {
    return {
        positionCode: box.positionCode,
        signerSlot: box.signerSlot,
        signerType: box.signerType,
        signerUser: box.signerUser || '',
        pageNo: box.pageNo,
        xRatio: box.xRatio,
        yRatio: box.yRatio,
        widthRatio: box.widthRatio,
        heightRatio: box.heightRatio,
        label: box.label || ''
    };
}

function signerUsername(value) {
    return String(value || '').split(':')[0].trim().toLowerCase();
}

function recordCreateEvent(event) {
    const allowed = new Set(['create_layout_open', 'pdf_upload_success', 'pdf_upload_error', 'preset_applied', 'box_add', 'box_delete', 'layout_validation_error', 'create_submit_success', 'create_submit_error', 'pdf_render_error']);
    if (!allowed.has(event)) return;
    void api
        .recordSigningDocumentCreateEvent({
            event,
            sessionId: createSessionId.value,
            docFormatCode: form.value.docFormatCode,
            elapsedMs: Date.now() - createOpenedAt,
            boxCount: form.value.layoutBoxes.length,
            validationIssueCount: layoutValidationIssues.value.length,
            viewport: {
                width: window.innerWidth || 0,
                height: window.innerHeight || 0
            }
        })
        .catch(() => {});
}

function makeClientId() {
    return crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
}

function openDetail(doc) {
    router.push({ name: 'signing-document-detail', params: { id: doc.id } });
}

function normalize(value) {
    return String(value || '').toLowerCase().trim();
}
</script>

<template>
    <div class="card signing-documents-page">
        <div class="page-header">
            <div>
                <div class="font-semibold text-xl mb-1">เอกสารเพื่อเซ็น</div>
                <p class="text-muted-color m-0">เลือกเอกสารจาก SML, upload PDF จริง, และติดตามสถานะการเซ็น</p>
            </div>
            <div class="header-actions">
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหา doc no, คู่ค้า, สถานะ" class="w-full sm:w-80" />
                <Button icon="pi pi-refresh" severity="secondary" outlined :loading="loading" aria-label="โหลดใหม่" @click="loadPage" />
                <Button label="ส่งเอกสารเพื่อเซ็น" icon="pi pi-send" @click="openCreate" />
            </div>
        </div>

        <DataTable :value="filteredDocuments" :loading="loading" dataKey="id" paginator :rows="10" responsiveLayout="scroll" stripedRows>
            <template #empty>
                <div class="py-6 text-center text-muted-color">{{ searchQuery ? 'ไม่พบเอกสารที่ค้นหา' : 'ยังไม่มีเอกสารเพื่อเซ็น' }}</div>
            </template>
            <Column field="docNo" header="เลขที่เอกสาร" sortable>
                <template #body="{ data }">
                    <button class="link-button" type="button" @click="openDetail(data)">{{ data.docNo }}</button>
                    <div class="text-sm text-muted-color">{{ data.docFormatCode }} · {{ data.partyName || data.partyCode || '-' }}</div>
                </template>
            </Column>
            <Column field="docDate" header="วันที่เอกสาร" sortable>
                <template #body="{ data }">{{ formatDocumentDate(data.docDate) }}</template>
            </Column>
            <Column field="totalAmount" header="ยอดเงิน" sortable>
                <template #body="{ data }">{{ Number(data.totalAmount || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 }) }}</template>
            </Column>
            <Column field="status" header="สถานะ" sortable>
                <template #body="{ data }"><Tag :value="signingStatusLabel(data.status)" :severity="signingStatusSeverity(data.status)" /></template>
            </Column>
            <Column field="updatedAt" header="อัปเดตล่าสุด" sortable>
                <template #body="{ data }">{{ formatThaiDateTime(data.updatedAt) }}</template>
            </Column>
            <Column header="จัดการ" style="width: 8rem">
                <template #body="{ data }">
                    <Button icon="pi pi-eye" rounded outlined severity="secondary" aria-label="ดูเอกสาร" @click="openDetail(data)" />
                </template>
            </Column>
        </DataTable>
    </div>

    <Dialog v-model:visible="createVisible" modal header="ส่งเอกสารเพื่อเซ็น" :closable="false" :closeOnEscape="false" :style="{ width: 'min(98vw, 118rem)' }" :contentStyle="{ paddingTop: '0.75rem' }">
        <div class="create-wizard">
            <section class="wizard-summary">
                <div class="form-stack">
                    <label class="font-medium">Doc Format</label>
                    <Select v-model="form.docFormatCode" :options="docFormatOptions" optionLabel="label" optionValue="value" filter />
                </div>
                <div class="form-stack">
                    <label class="font-medium">ค้นหาเลขเอกสารจาก SML</label>
                    <InputText v-model="form.search" placeholder="เช่น PO2606" />
                    <small class="text-muted-color">ต้องเลือกจากผลลัพธ์เท่านั้น ระบบจะ validate กับ SML ซ้ำตอนส่งเซ็น</small>
                </div>
                <div class="form-stack">
                    <label class="font-medium">PDF จริงจาก SML</label>
                    <input type="file" accept="application/pdf" :disabled="uploading" @change="onFileChange" />
                    <small class="text-muted-color">
                        <span v-if="uploading">กำลังอัปโหลด...</span>
                        <span v-else-if="form.uploadedFile">{{ form.uploadedFile.originalName }} · {{ form.uploadedFile.pageCount }} หน้า</span>
                        <span v-else>ยังไม่ได้อัปโหลดไฟล์</span>
                    </small>
                </div>
                <div class="summary-card">
                    <strong>{{ form.selectedCandidate?.doc_no || 'ยังไม่ได้เลือกเอกสาร' }}</strong>
                    <span>{{ form.selectedCandidate?.party_name || form.selectedCandidate?.party_code || 'เลือกจากรายการ SML ด้านล่าง' }}</span>
                    <Tag v-if="form.selectedCandidate?.is_lock_record === 1" value="SML lock แล้ว" severity="danger" />
                </div>
            </section>

            <section class="candidate-list" @scroll.passive="($event.target.scrollTop + $event.target.clientHeight >= $event.target.scrollHeight - 12) && loadMoreCandidates()">
                <div v-if="searchingCandidates && candidates.length === 0" class="candidate-empty">กำลังค้นหา...</div>
                <div v-else-if="candidates.length === 0" class="candidate-empty">พิมพ์อย่างน้อย 2 ตัวอักษรเพื่อค้นหาเอกสารจาก SML</div>
                <button
                    v-for="candidate in candidates"
                    :key="candidate.doc_no"
                    type="button"
                    class="candidate-row"
                    :class="{ selected: form.selectedCandidate?.doc_no === candidate.doc_no }"
                    @click="selectCandidate(candidate)"
                >
                    <span>
                        <strong>{{ candidate.doc_no }}</strong>
                        <small>{{ candidate.party_name || candidate.party_code || '-' }} · {{ formatDocumentDate(candidate.doc_date) }}</small>
                    </span>
                    <Tag v-if="candidate.is_lock_record === 1" value="SML lock แล้ว" severity="danger" />
                    <span>{{ Number(candidate.total_amount || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 }) }}</span>
                </button>
                <Button v-if="candidateHasMore" label="โหลดเพิ่ม" severity="secondary" text :loading="searchingCandidates" @click="loadMoreCandidates" />
            </section>

            <Message v-if="form.selectedCandidate?.is_lock_record === 1" severity="warn">
                เอกสารนี้ถูก lock ใน SML แล้ว ถ้าต้องการบันทึกซ้ำให้ยืนยันก่อน
            </Message>
            <label v-if="form.selectedCandidate?.is_lock_record === 1" class="flex items-center gap-2">
                <Checkbox v-model="form.confirmLocked" binary />
                <span>ยืนยันสร้าง PaperLess document จากเอกสาร SML ที่ lock แล้ว</span>
            </label>

            <Message v-if="loadingLayoutContext" severity="info">กำลังโหลด config และ preset กรอบลายเซ็น...</Message>
            <DocumentLayoutDesigner
                ref="layoutDesignerRef"
                v-model="form.layoutBoxes"
                :pdfUrl="form.fileUrl"
                :pageCount="form.uploadedFile?.pageCount || 0"
                :configs="form.configs"
                :presetTemplate="form.presetTemplate"
                @apply-preset="onApplyPreset"
                @event="onDesignerEvent"
            />
        </div>

        <template #footer>
            <div class="dialog-footer">
                <span class="send-hint">{{ createDisabledReason || `${form.layoutBoxes.length} กรอบพร้อมส่งเซ็น` }}</span>
                <div class="footer-actions">
                    <Button label="ยกเลิก" severity="secondary" outlined @click="closeCreate" />
                    <Button label="ส่งเซ็น" icon="pi pi-send" :loading="creating" :disabled="!!createDisabledReason || uploading" @click="createDocument" />
                </div>
            </div>
        </template>
    </Dialog>
</template>

<style scoped>
.page-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.25rem;
}
.header-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    justify-content: flex-end;
}
.link-button {
    border: 0;
    background: transparent;
    color: var(--primary-color);
    font-weight: 700;
    padding: 0;
    cursor: pointer;
}
.create-wizard {
    display: grid;
    gap: 1rem;
}
.wizard-summary {
    display: grid;
    grid-template-columns: minmax(14rem, 18rem) minmax(18rem, 1fr) minmax(12rem, 18rem) minmax(14rem, 18rem);
    align-items: start;
    gap: 0.85rem;
}
.form-stack {
    display: grid;
    gap: 0.45rem;
}
.summary-card {
    min-height: 4.25rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.7rem;
    display: grid;
    gap: 0.25rem;
    align-content: start;
}
.summary-card span {
    color: var(--text-color-secondary);
    font-size: 0.9rem;
}
.candidate-list {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    max-height: 13rem;
    overflow: auto;
}
.candidate-row {
    width: 100%;
    border: 0;
    border-bottom: 1px solid var(--surface-border);
    background: transparent;
    display: grid;
    grid-template-columns: 1fr auto auto;
    align-items: center;
    gap: 0.75rem;
    padding: 0.8rem;
    text-align: left;
    cursor: pointer;
}
.candidate-row small {
    display: block;
    color: var(--text-color-secondary);
    margin-top: 0.2rem;
}
.candidate-row.selected {
    background: color-mix(in srgb, var(--primary-color) 10%, transparent);
}
.candidate-empty {
    padding: 1rem;
    color: var(--text-color-secondary);
    text-align: center;
}
.dialog-footer {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
}
.send-hint {
    color: var(--text-color-secondary);
    text-align: left;
}
.footer-actions {
    display: flex;
    gap: 0.5rem;
}
@media (max-width: 768px) {
    .page-header {
        flex-direction: column;
    }
    .header-actions {
        width: 100%;
        justify-content: stretch;
    }
    .candidate-row {
        grid-template-columns: 1fr;
    }
    .wizard-summary {
        grid-template-columns: 1fr;
    }
    .dialog-footer {
        align-items: stretch;
        flex-direction: column;
    }
    .footer-actions {
        justify-content: flex-end;
    }
}
</style>
