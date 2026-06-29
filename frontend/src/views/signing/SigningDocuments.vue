<script setup>
import { api } from '@/services/api';
import { computed, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const documents = ref([]);
const docFormats = ref([]);
const loading = ref(false);
const creating = ref(false);
const createVisible = ref(false);
const searchQuery = ref('');
const form = ref(emptyForm());
const candidates = ref([]);
const candidatePage = ref(1);
const candidateTotal = ref(0);
const candidateHasMore = ref(false);
const searchingCandidates = ref(false);
let searchTimer;

const filteredDocuments = computed(() => {
    const query = normalize(searchQuery.value);
    if (!query) return documents.value;
    return documents.value.filter((doc) => normalize(`${doc.docFormatCode} ${doc.docNo} ${doc.partyName} ${doc.status}`).includes(query));
});

const docFormatOptions = computed(() =>
    docFormats.value.map((format) => ({
        label: `${format.code} - ${format.name_1 || format.name_2 || format.format || 'ไม่มีชื่อเอกสาร'}`,
        value: format.code
    }))
);

watch(
    () => [form.value.docFormatCode, form.value.search],
    () => {
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

function emptyForm() {
    return {
        docFormatCode: '',
        search: '',
        docNo: '',
        file: null,
        selectedCandidate: null,
        confirmLocked: false
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
    createVisible.value = true;
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
    form.value.search = candidate.doc_no;
    form.value.confirmLocked = false;
}

function onFileChange(event) {
    form.value.file = event.target.files?.[0] || null;
}

async function createDocument() {
    if (!form.value.selectedCandidate || !form.value.file) {
        toast.add({ severity: 'warn', summary: 'ข้อมูลไม่ครบ', detail: 'เลือกเอกสารจาก SML และอัปโหลด PDF ก่อน', life: 3000 });
        return;
    }
    if (Number(form.value.selectedCandidate.is_lock_record || 0) === 1 && !form.value.confirmLocked) {
        toast.add({ severity: 'warn', summary: 'เอกสารถูก Lock แล้ว', detail: 'ยืนยัน checkbox ก่อนสร้าง PaperLess document', life: 3500 });
        return;
    }
    creating.value = true;
    try {
        const result = await api.createSigningDocument({
            docFormatCode: form.value.docFormatCode,
            docNo: form.value.selectedCandidate.doc_no,
            file: form.value.file,
            confirmLocked: form.value.confirmLocked
        });
        createVisible.value = false;
        toast.add({ severity: 'success', summary: 'ส่งเอกสารเพื่อเซ็นแล้ว', life: 2500 });
        await loadPage();
        router.push({ name: 'signing-document-detail', params: { id: result.document.id } });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'สร้างเอกสารไม่สำเร็จ', detail: err.message, life: 5000 });
    } finally {
        creating.value = false;
    }
}

function openDetail(doc) {
    router.push({ name: 'signing-document-detail', params: { id: doc.id } });
}

function statusSeverity(status) {
    return {
        in_progress: 'info',
        completed: 'success',
        completed_lock_failed: 'danger',
        rejected: 'danger',
        cancelled: 'secondary',
        draft: 'secondary'
    }[status] || 'secondary';
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
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
            <Column field="docDate" header="วันที่เอกสาร" sortable />
            <Column field="totalAmount" header="ยอดเงิน" sortable>
                <template #body="{ data }">{{ Number(data.totalAmount || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 }) }}</template>
            </Column>
            <Column field="status" header="สถานะ" sortable>
                <template #body="{ data }"><Tag :value="data.status" :severity="statusSeverity(data.status)" /></template>
            </Column>
            <Column field="updatedAt" header="อัปเดตล่าสุด" sortable>
                <template #body="{ data }">{{ formatDate(data.updatedAt) }}</template>
            </Column>
            <Column header="จัดการ" style="width: 8rem">
                <template #body="{ data }">
                    <Button icon="pi pi-eye" rounded outlined severity="secondary" aria-label="ดูเอกสาร" @click="openDetail(data)" />
                </template>
            </Column>
        </DataTable>
    </div>

    <Dialog v-model:visible="createVisible" modal header="ส่งเอกสารเพื่อเซ็น" :style="{ width: 'min(58rem, 94vw)' }">
        <div class="create-grid">
            <div class="form-stack">
                <label class="font-medium">Doc Format</label>
                <Select v-model="form.docFormatCode" :options="docFormatOptions" optionLabel="label" optionValue="value" filter />
            </div>
            <div class="form-stack">
                <label class="font-medium">ค้นหาเลขเอกสารจาก SML</label>
                <InputText v-model="form.search" placeholder="เช่น PO2606" />
                <small class="text-muted-color">ต้องเลือกจากผลลัพธ์เท่านั้น ระบบจะ validate กับ SML ซ้ำตอนบันทึก</small>
            </div>

            <div class="candidate-list" @scroll.passive="($event.target.scrollTop + $event.target.clientHeight >= $event.target.scrollHeight - 12) && loadMoreCandidates()">
                <div v-if="searchingCandidates && candidates.length === 0" class="candidate-empty">กำลังค้นหา...</div>
                <div v-else-if="candidates.length === 0" class="candidate-empty">พิมพ์อย่างน้อย 2 ตัวอักษรเพื่อค้นหา</div>
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
                        <small>{{ candidate.party_name || candidate.party_code || '-' }} · {{ candidate.doc_date }}</small>
                    </span>
                    <Tag v-if="candidate.is_lock_record === 1" value="SML locked" severity="danger" />
                    <span>{{ Number(candidate.total_amount || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 }) }}</span>
                </button>
                <Button v-if="candidateHasMore" label="โหลดเพิ่ม" severity="secondary" text :loading="searchingCandidates" @click="loadMoreCandidates" />
            </div>

            <Message v-if="form.selectedCandidate?.is_lock_record === 1" severity="warn">
                เอกสารนี้ถูก lock ใน SML แล้ว ถ้าต้องการบันทึกซ้ำให้ยืนยันก่อน
            </Message>
            <label v-if="form.selectedCandidate?.is_lock_record === 1" class="flex items-center gap-2">
                <Checkbox v-model="form.confirmLocked" binary />
                <span>ยืนยันสร้าง PaperLess document จากเอกสาร SML ที่ lock แล้ว</span>
            </label>

            <div class="form-stack">
                <label class="font-medium">PDF จริงจาก SML</label>
                <input type="file" accept="application/pdf" @change="onFileChange" />
                <small class="text-muted-color">{{ form.file?.name || 'ยังไม่ได้เลือกไฟล์' }}</small>
            </div>
        </div>

        <template #footer>
            <Button label="ยกเลิก" severity="secondary" outlined @click="createVisible = false" />
            <Button label="ส่งเซ็น" icon="pi pi-send" :loading="creating" @click="createDocument" />
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
.create-grid {
    display: grid;
    gap: 1rem;
}
.form-stack {
    display: grid;
    gap: 0.45rem;
}
.candidate-list {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    max-height: 18rem;
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
}
</style>
