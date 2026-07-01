<script setup>
import { api } from '@/services/api';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const documents = ref([]);
const page = ref(1);
const size = ref(20);
const total = ref(0);
const hasMore = ref(false);
const loading = ref(false);
const loadingMore = ref(false);
const searchQuery = ref('');
const historyOpenRecorded = ref(false);
const sessionId = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
const openedAt = Date.now();
let searchTimer = null;

const hasRows = computed(() => documents.value.length > 0);

onMounted(() => loadHistory());
onBeforeUnmount(() => {
    if (searchTimer) window.clearTimeout(searchTimer);
});

watch(searchQuery, () => {
    if (searchTimer) window.clearTimeout(searchTimer);
    searchTimer = window.setTimeout(() => loadHistory(), 300);
});

async function loadHistory() {
    loading.value = true;
    try {
        const result = await api.listMySigningHistory({ page: 1, size: size.value, search: searchQuery.value });
        documents.value = result.documents || [];
        page.value = result.page || 1;
        total.value = result.total || 0;
        hasMore.value = Boolean(result.hasMore);
        recordHistoryOpen();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดประวัติไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadMore() {
    if (!hasMore.value || loadingMore.value) return;
    loadingMore.value = true;
    try {
        const nextPage = page.value + 1;
        const result = await api.listMySigningHistory({ page: nextPage, size: size.value, search: searchQuery.value });
        documents.value = [...documents.value, ...(result.documents || [])];
        page.value = result.page || nextPage;
        total.value = result.total || total.value;
        hasMore.value = Boolean(result.hasMore);
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดประวัติเพิ่มไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loadingMore.value = false;
    }
}

function openHistory(row) {
    router.push({ name: 'my-signing-history-detail', params: { taskId: row.taskId } });
}

function recordHistoryOpen() {
    if (historyOpenRecorded.value || documents.value.length === 0) return;
    historyOpenRecorded.value = true;
    api.recordMySigningTaskEvent(documents.value[0].taskId, {
        event: 'history_open',
        sessionId,
        elapsedMs: Date.now() - openedAt,
        viewport: { width: window.innerWidth, height: window.innerHeight }
    }).catch(() => {});
}

function statusView(row) {
    if (row.taskStatus === 'rejected') return { label: 'ปฏิเสธแล้ว', severity: 'danger', icon: 'pi pi-times-circle' };
    return { label: 'เซ็นแล้ว', severity: 'success', icon: 'pi pi-check-circle' };
}

function actionDate(row) {
    return row.taskStatus === 'rejected' ? row.rejectedAt : row.signedAt;
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium' }).format(new Date(value));
}

function formatDateTime(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
</script>

<template>
    <section class="history-page">
        <header class="history-header">
            <div>
                <h1>ประวัติการเซ็น</h1>
                <p>ดูเอกสารที่คุณเคยเซ็นหรือปฏิเสธไว้</p>
            </div>
            <Tag :value="`${total || 0} รายการ`" severity="secondary" />
        </header>

        <div class="history-search">
            <IconField class="search-field">
                <InputIcon><i class="pi pi-search" /></InputIcon>
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหาเลขเอกสาร, คู่ค้า หรือตำแหน่ง" />
            </IconField>
            <Button label="โหลดใหม่" icon="pi pi-refresh" severity="secondary" outlined aria-label="โหลดใหม่" class="refresh-button" :loading="loading" @click="loadHistory" />
        </div>

        <div v-if="loading" class="history-state">
            <i class="pi pi-spin pi-spinner"></i>
            <span>กำลังโหลดประวัติการเซ็น</span>
        </div>

        <template v-else>
            <div v-if="!hasRows" class="empty-state">
                <i class="pi pi-history"></i>
                <strong>{{ searchQuery ? 'ไม่พบประวัติจากคำค้นนี้' : 'ยังไม่มีประวัติการเซ็น' }}</strong>
                <p>{{ searchQuery ? 'ลองค้นหาด้วยเลขเอกสาร ชื่อคู่ค้า หรือตำแหน่งอีกครั้ง' : 'เมื่อคุณเซ็นหรือปฏิเสธเอกสาร รายการจะแสดงที่นี่' }}</p>
            </div>

            <div v-else class="history-list">
                <article v-for="row in documents" :key="row.taskId" class="history-card">
                    <div class="history-main">
                        <div>
                            <strong>{{ row.docNo }}</strong>
                            <span>{{ row.docFormatCode }} · {{ row.partyName || row.partyCode || '-' }}</span>
                        </div>
                        <Tag :value="statusView(row).label" :severity="statusView(row).severity" />
                    </div>

                    <div class="position-banner" :class="{ rejected: row.taskStatus === 'rejected' }">
                        <span><i class="pi pi-user-edit"></i> ตำแหน่งของคุณ</span>
                        <strong>{{ row.positionName || '-' }}</strong>
                    </div>

                    <dl>
                        <div>
                            <dt>วันที่ดำเนินการ</dt>
                            <dd><i :class="statusView(row).icon"></i> {{ formatDateTime(actionDate(row)) }}</dd>
                        </div>
                        <div>
                            <dt>วันที่เอกสาร</dt>
                            <dd>{{ formatDate(row.docDate) }}</dd>
                        </div>
                    </dl>

                    <Message v-if="row.taskStatus === 'rejected' && row.rejectReason" severity="warn" class="history-message">เหตุผล: {{ row.rejectReason }}</Message>
                    <Message v-else-if="!row.hasFinalPdf && row.hasCurrentPdf" severity="secondary" class="history-message">เอกสารยังรอขั้นตอนผู้ดูแล จะแสดง PDF ล่าสุดที่ระบบมี</Message>

                    <Button label="ดูเอกสาร" icon="pi pi-eye" class="open-button" @click="openHistory(row)" />
                </article>
            </div>

            <Button v-if="hasMore" label="โหลดประวัติเพิ่ม" icon="pi pi-angle-down" severity="secondary" outlined :loading="loadingMore" @click="loadMore" />
        </template>
    </section>
</template>

<style scoped>
.history-page {
    min-height: calc(100dvh - 96px);
    padding: 0.85rem;
    display: grid;
    align-content: start;
    gap: 0.85rem;
}

.history-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.history-header h1 {
    margin: 0;
    font-size: 1.35rem;
    line-height: 1.2;
}

.history-header p {
    margin: 0.25rem 0 0;
    color: var(--text-color-secondary);
}

.history-search {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: stretch;
    gap: 0.6rem;
}

.search-field {
    width: 100%;
}

.search-field :deep(.p-inputtext) {
    width: 100%;
    min-height: 44px;
}

.refresh-button {
    min-height: 44px;
    white-space: nowrap;
}

.history-state,
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
.history-card {
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

.history-list {
    display: grid;
    gap: 0.75rem;
}

.history-card {
    display: grid;
    gap: 0.75rem;
}

.history-main {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}

.history-main > div {
    min-width: 0;
    display: grid;
    gap: 0.2rem;
}

.history-main strong,
.history-main span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.history-main span,
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

.position-banner.rejected {
    border-color: color-mix(in srgb, var(--red-500, #ef4444) 30%, var(--surface-border));
    background: color-mix(in srgb, var(--red-500, #ef4444) 7%, var(--surface-card));
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

.position-banner.rejected strong {
    color: var(--red-600, #dc2626);
}

dl {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.75rem;
}

dt {
    font-size: 0.78rem;
}

dd {
    margin: 0.15rem 0 0;
    font-weight: 600;
}

dd i {
    margin-right: 0.25rem;
}

.history-message {
    margin: 0;
}

.open-button {
    min-height: 44px;
}

@media (min-width: 760px) {
    .history-page {
        max-width: 920px;
        margin: 0 auto;
        padding-top: 1.25rem;
    }

    .history-list {
        grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    }
}

@media (max-width: 520px) {
    .history-header {
        display: grid;
    }

    dl {
        grid-template-columns: 1fr;
    }

    .refresh-button {
        width: 44px;
        padding-inline: 0;
    }

    .refresh-button :deep(.p-button-label) {
        display: none;
    }
}
</style>
