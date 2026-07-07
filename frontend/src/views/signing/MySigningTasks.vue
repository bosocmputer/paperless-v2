<script setup>
import { api } from '@/services/api';
import { authStore } from '@/stores/auth';
import DocumentAttachmentActionButton from '@/views/signing/components/DocumentAttachmentActionButton.vue';
import DocumentAttachmentsDialog from '@/views/signing/components/DocumentAttachmentsDialog.vue';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const readyDocuments = ref([]);
const waitingDocuments = ref([]);
const counts = ref({ ready: 0, waiting: 0 });
const pagination = ref({
    ready: { page: 1, size: 20, hasMore: false },
    waiting: { page: 1, size: 20, hasMore: false }
});
const loading = ref(false);
const loadingReadyMore = ref(false);
const loadingWaitingMore = ref(false);
const searchQuery = ref('');
const attachmentsDialog = ref(false);
const attachmentsRow = ref(null);
const waitingSeenRecorded = ref(false);
const sessionId = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
const openedAt = Date.now();

const readyRows = computed(() => readyDocuments.value.map(normalizeQueueRow).filter(Boolean));
const waitingRows = computed(() => waitingDocuments.value.map(normalizeQueueRow).filter(Boolean));

const filteredReadyRows = computed(() => filterRows(readyRows.value));
const filteredWaitingRows = computed(() => filterRows(waitingRows.value));
const hasAnyRows = computed(() => readyRows.value.length > 0 || waitingRows.value.length > 0);
const emptyTitle = computed(() => {
    if (searchQuery.value) return 'ไม่พบงานที่ค้นหา';
    return 'ยังไม่มีเอกสารที่เกี่ยวข้องกับคุณ';
});
const emptyDescription = computed(() => {
    if (searchQuery.value) return 'ลองค้นหาด้วยเลขเอกสาร ชื่อคู่ค้า หรือชื่อผู้เซ็นอีกครั้ง';
    return 'เมื่อมีเอกสารส่งถึงคุณ ระบบจะแสดงทั้งงานที่เซ็นได้และงานที่ยังรอคิว';
});
const attachmentsDialogTitle = computed(() => {
    const docNo = attachmentsRow.value?.doc?.docNo || '';
    return docNo ? `ไฟล์แนบอ้างอิง · ${docNo}` : 'ไฟล์แนบอ้างอิง';
});
const attachmentsDialogSubtitle = computed(() => {
    const doc = attachmentsRow.value?.doc || {};
    const parts = [doc.docFormatCode, doc.partyName || doc.partyCode, formatDate(doc.docDate)].filter((part) => part && part !== '-');
    return parts.join(' · ');
});
const attachmentsDialogKey = computed(() => attachmentsRow.value?.task?.id || '');

onMounted(() => loadTasks());

async function loadTasks() {
    loading.value = true;
    try {
        const result = await api.listMySigningTasks({ readyPage: 1, waitingPage: 1, size: 20 });
        readyDocuments.value = result.documents || [];
        waitingDocuments.value = result.waitingDocuments || [];
        applyQueueMeta(result);
        recordWaitingQueueSeen();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดงานเซ็นไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadMoreReady() {
    if (!pagination.value.ready.hasMore || loadingReadyMore.value) return;
    loadingReadyMore.value = true;
    try {
        const nextPage = Number(pagination.value.ready.page || 1) + 1;
        const result = await api.listMySigningTasks({ readyPage: nextPage, waitingPage: pagination.value.waiting.page, size: pagination.value.ready.size || 20 });
        readyDocuments.value = [...readyDocuments.value, ...(result.documents || [])];
        pagination.value = { ...pagination.value, ready: result.pagination?.ready || { page: nextPage, size: 20, hasMore: false } };
        counts.value = result.counts || counts.value;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดงานเพิ่มไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loadingReadyMore.value = false;
    }
}

async function loadMoreWaiting() {
    if (!pagination.value.waiting.hasMore || loadingWaitingMore.value) return;
    loadingWaitingMore.value = true;
    try {
        const nextPage = Number(pagination.value.waiting.page || 1) + 1;
        const result = await api.listMySigningTasks({ readyPage: pagination.value.ready.page, waitingPage: nextPage, size: pagination.value.waiting.size || 20 });
        waitingDocuments.value = [...waitingDocuments.value, ...(result.waitingDocuments || [])];
        pagination.value = { ...pagination.value, waiting: result.pagination?.waiting || { page: nextPage, size: 20, hasMore: false } };
        counts.value = result.counts || counts.value;
        recordWaitingQueueSeen();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดงานรอคิวเพิ่มไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loadingWaitingMore.value = false;
    }
}

function applyQueueMeta(result) {
    counts.value = result.counts || { ready: readyDocuments.value.length, waiting: waitingDocuments.value.length };
    pagination.value = {
        ready: result.pagination?.ready || { page: 1, size: 20, hasMore: false },
        waiting: result.pagination?.waiting || { page: 1, size: 20, hasMore: false }
    };
}

function normalizeQueueRow(doc) {
    const username = authStore.user?.username || '';
    const task =
        doc.task ||
        (doc.signers || []).find((signer) => signer.signerUser?.toLowerCase() === username.toLowerCase()) ||
        (doc.signers || [])[0];
    if (!task?.id) return null;
    return { doc, task };
}

function filterRows(rows) {
    const query = String(searchQuery.value || '').toLowerCase().trim();
    if (!query) return rows;
    return rows.filter(({ doc, task }) =>
        [
            doc.docNo,
            doc.docFormatCode,
            doc.partyName,
            doc.partyCode,
            task.positionName,
            task.signerName,
            doc.blockSummary,
            ...(doc.blockedBy || []).flatMap((blocker) => [blocker.positionName, blocker.summary, ...(blocker.signers || []).map((signer) => signer.signerName || signer.signerUser)])
        ]
            .filter(Boolean)
            .join(' ')
            .toLowerCase()
            .includes(query)
    );
}

function openTask(row) {
    router.push({ name: 'my-signing-task', params: { taskId: row.task.id } });
}

function openAttachmentsDialog(row) {
    if (!row?.task?.id || attachmentCount(row.doc) <= 0) return;
    attachmentsRow.value = row;
    attachmentsDialog.value = true;
}

function loadAttachmentsForDialog() {
    if (!attachmentsRow.value?.task?.id) return Promise.resolve({ attachments: [] });
    return api.getMyTaskAttachments(attachmentsRow.value.task.id);
}

function attachmentFileUrlForDialog(attachment) {
    if (!attachmentsRow.value?.task?.id || !attachment?.id) return '';
    return api.myTaskAttachmentFileUrl(attachmentsRow.value.task.id, attachment.id);
}

function recordWaitingQueueSeen() {
    if (waitingSeenRecorded.value || waitingRows.value.length === 0) return;
    waitingSeenRecorded.value = true;
    recordTaskEvent(waitingRows.value[0].task.id, 'waiting_queue_seen');
}

function recordTaskEvent(taskId, event) {
    api.recordMySigningTaskEvent(taskId, {
        event,
        sessionId,
        elapsedMs: Date.now() - openedAt,
        viewport: { width: window.innerWidth, height: window.innerHeight }
    }).catch(() => {});
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium' }).format(new Date(value));
}

function attachmentCount(doc) {
    return Number(doc?.attachmentCount || 0);
}
</script>

<template>
    <section class="tasks-page">
        <header class="tasks-header">
            <div>
                <h1>งานรอเซ็น</h1>
                <p>ดูงานที่เซ็นได้ทันที และเอกสารที่กำลังรอขั้นตอนก่อนหน้า</p>
            </div>
            <div class="header-actions">
                <div class="queue-tags">
                    <Tag :value="`เซ็นได้ ${counts.ready || 0}`" severity="info" />
                    <Tag :value="`รอคิว ${counts.waiting || 0}`" severity="secondary" />
                </div>
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadTasks" />
            </div>
        </header>

        <div class="task-search">
            <IconField class="search-field">
                <InputIcon><i class="pi pi-search" /></InputIcon>
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหาเลขเอกสาร, คู่ค้า, ขั้นตอน หรือผู้เซ็น" />
            </IconField>
        </div>

        <div v-if="loading" class="task-state">
            <i class="pi pi-spin pi-spinner"></i>
            <span>กำลังโหลดงานรอเซ็น</span>
        </div>

        <template v-else>
            <div v-if="!hasAnyRows" class="empty-state">
                <i class="pi pi-inbox"></i>
                <strong>{{ emptyTitle }}</strong>
                <p>{{ emptyDescription }}</p>
            </div>

            <template v-else>
                <section class="queue-section">
                    <div class="queue-header">
                        <div>
                            <h2>เซ็นได้ตอนนี้</h2>
                            <p>เอกสารที่ถึงลำดับของคุณแล้ว</p>
                        </div>
                        <Tag :value="`${filteredReadyRows.length} งาน`" severity="info" />
                    </div>

                    <Message v-if="filteredReadyRows.length === 0" severity="secondary">
                        {{ searchQuery ? 'ไม่พบงานที่เซ็นได้จากคำค้นนี้' : waitingRows.length > 0 ? 'ยังไม่มีงานที่ต้องเซ็นตอนนี้ ดูเอกสารรอคิวด้านล่าง' : 'ตอนนี้ยังไม่มีงานที่ต้องเซ็น' }}
                    </Message>

                    <div v-else class="task-list">
                        <article v-for="row in filteredReadyRows" :key="row.task.id" class="task-card">
                            <div class="task-main">
                                <div>
                                    <strong>{{ row.doc.docNo }}</strong>
                                    <span>{{ row.doc.docFormatCode }} · {{ row.doc.partyName || row.doc.partyCode || '-' }}</span>
                                </div>
                                <span class="task-tags">
                                    <Tag value="รอเซ็น" severity="info" />
                                </span>
                            </div>
                            <div class="position-banner">
                                <span><i class="pi pi-user-edit"></i> ตำแหน่งของคุณ</span>
                                <strong>{{ row.task.positionName || '-' }}</strong>
                            </div>
                            <dl>
                                <div>
                                    <dt>วันที่เอกสาร</dt>
                                    <dd>{{ formatDate(row.doc.docDate) }}</dd>
                                </div>
                            </dl>
                            <div class="card-actions">
                                <DocumentAttachmentActionButton :count="attachmentCount(row.doc)" @click="openAttachmentsDialog(row)" />
                                <Button label="เปิดเอกสาร" icon="pi pi-pencil" class="open-button" @click="openTask(row, 'ready')" />
                            </div>
                        </article>
                    </div>

                    <Button
                        v-if="pagination.ready.hasMore"
                        label="โหลดงานที่เซ็นได้เพิ่ม"
                        icon="pi pi-angle-down"
                        severity="secondary"
                        outlined
                        :loading="loadingReadyMore"
                        @click="loadMoreReady"
                    />
                </section>

                <section class="queue-section">
                    <div class="queue-header">
                        <div>
                            <h2>รอคิวเซ็น</h2>
                            <p>เอกสารที่มีชื่อคุณอยู่ใน workflow แต่ต้องรอขั้นตอนก่อนหน้า</p>
                        </div>
                        <Tag :value="`${filteredWaitingRows.length} รายการ`" severity="secondary" />
                    </div>

                    <Message v-if="filteredWaitingRows.length === 0" severity="secondary">
                        {{ searchQuery ? 'ไม่พบเอกสารรอคิวจากคำค้นนี้' : 'ไม่มีเอกสารที่รอคิวของคุณ' }}
                    </Message>

                    <div v-else class="task-list">
                        <article v-for="row in filteredWaitingRows" :key="row.task.id" class="task-card waiting">
                            <div class="task-main">
                                <div>
                                    <strong>{{ row.doc.docNo }}</strong>
                                    <span>{{ row.doc.docFormatCode }} · {{ row.doc.partyName || row.doc.partyCode || '-' }}</span>
                                </div>
                                <span class="task-tags">
                                    <Tag value="ยังไม่ถึงคิว" severity="secondary" />
                                </span>
                            </div>
                            <div class="position-banner waiting-position">
                                <span><i class="pi pi-user-edit"></i> ตำแหน่งของคุณ</span>
                                <strong>{{ row.task.positionName || '-' }}</strong>
                            </div>
                            <dl>
                                <div>
                                    <dt>วันที่เอกสาร</dt>
                                    <dd>{{ formatDate(row.doc.docDate) }}</dd>
                                </div>
                            </dl>
                            <Message severity="warn" class="block-message">{{ row.doc.blockSummary || 'รอขั้นตอนก่อนหน้าเสร็จก่อน' }}</Message>
                            <div class="card-actions">
                                <DocumentAttachmentActionButton :count="attachmentCount(row.doc)" @click="openAttachmentsDialog(row)" />
                                <Button label="ดูความคืบหน้า" icon="pi pi-eye" severity="secondary" outlined class="open-button" @click="openTask(row, 'waiting')" />
                            </div>
                        </article>
                    </div>

                    <Button
                        v-if="pagination.waiting.hasMore"
                        label="โหลดเอกสารรอคิวเพิ่ม"
                        icon="pi pi-angle-down"
                        severity="secondary"
                        outlined
                        :loading="loadingWaitingMore"
                        @click="loadMoreWaiting"
                    />
                </section>
            </template>
        </template>

        <DocumentAttachmentsDialog
            v-model:visible="attachmentsDialog"
            :title="attachmentsDialogTitle"
            :subtitle="attachmentsDialogSubtitle"
            :loader-key="attachmentsDialogKey"
            :loader="loadAttachmentsForDialog"
            :file-url-resolver="attachmentFileUrlForDialog"
            :headers="api.authHeaders()"
        />
    </section>
</template>

<style scoped>
.tasks-page {
    min-height: calc(100dvh - 56px);
    padding: 0.75rem;
    display: grid;
    align-content: start;
    gap: 0.75rem;
}

.tasks-header,
.queue-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.tasks-header h1,
.queue-header h2 {
    margin: 0;
    line-height: 1.2;
}

.tasks-header h1 {
    font-size: 1.35rem;
}

.queue-header h2 {
    font-size: 1.08rem;
}

.tasks-header p,
.queue-header p {
    margin: 0.2rem 0 0;
    color: var(--text-color-secondary);
}

.header-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
}

.queue-tags {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    justify-content: flex-end;
}

.task-search {
    display: block;
}

.search-field {
    width: 100%;
}

.search-field :deep(.p-inputtext) {
    width: 100%;
    min-height: 44px;
    font-size: 16px;
}

.task-state,
.empty-state {
    min-height: 42dvh;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.65rem;
    text-align: center;
    color: var(--text-color-secondary);
}

.empty-state,
.queue-section {
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 1rem;
}

.empty-state i {
    font-size: 2rem;
    color: var(--primary-color);
}

.empty-state strong {
    color: var(--text-color);
}

.empty-state p {
    margin: 0;
    max-width: 34rem;
}

.queue-section {
    display: grid;
    gap: 0.75rem;
}

.task-list {
    display: grid;
    gap: 0.75rem;
}

.task-card {
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 0.85rem;
    display: grid;
    gap: 0.75rem;
}

.task-card.waiting {
    background: color-mix(in srgb, var(--surface-card) 88%, var(--surface-ground));
}

.task-main {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}

.task-tags {
    display: inline-flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.35rem;
}

.card-actions {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    align-items: center;
    gap: 0.6rem;
}

.task-main > div {
    min-width: 0;
    display: grid;
    gap: 0.2rem;
}

.task-main strong,
.task-main span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.task-main span,
dt {
    color: var(--text-color-secondary);
}

.position-banner {
    border: 1px solid color-mix(in srgb, var(--primary-color) 28%, var(--surface-border));
    background: color-mix(in srgb, var(--primary-color) 10%, var(--surface-card));
    border-radius: 8px;
    padding: 0.7rem 0.8rem;
    display: grid;
    gap: 0.2rem;
}

.position-banner span {
    color: var(--text-color-secondary);
    font-size: 0.82rem;
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
}

.position-banner strong {
    color: var(--primary-color);
    font-size: 1.15rem;
    line-height: 1.2;
}

.waiting-position {
    border-color: color-mix(in srgb, var(--surface-border) 70%, var(--primary-color));
    background: color-mix(in srgb, var(--surface-ground) 70%, var(--surface-card));
}

.waiting-position strong {
    color: var(--text-color);
}

dl {
    margin: 0;
}

dt {
    font-size: 0.78rem;
}

dd {
    margin: 0.15rem 0 0;
    font-weight: 600;
}

.block-message {
    margin: 0;
}

.open-button {
    min-height: 44px;
    width: 100%;
}

@media (min-width: 760px) {
    .tasks-page {
        max-width: 920px;
        margin: 0 auto;
        padding-top: 1.25rem;
    }

    .task-list {
        grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    }
}

@media (max-width: 520px) {
    .tasks-header,
    .queue-header {
        display: grid;
        gap: 0.55rem;
    }

    .header-actions {
        justify-content: space-between;
    }

    .queue-tags {
        justify-content: flex-start;
    }

    dl {
        grid-template-columns: 1fr;
    }
}
</style>
