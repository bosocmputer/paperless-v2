<script setup>
import { api } from '@/services/api';
import { authStore } from '@/stores/auth';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();
const documents = ref([]);
const loading = ref(false);
const searchQuery = ref('');

const rows = computed(() => {
    const username = authStore.user?.username || '';
    return documents.value.flatMap((doc) =>
        (doc.signers || [])
            .filter((signer) => signer.status === 'pending' && signer.signerUser?.toLowerCase() === username.toLowerCase())
            .map((signer) => ({ doc, signer }))
    );
});

const filteredRows = computed(() => {
    const query = String(searchQuery.value || '').toLowerCase().trim();
    if (!query) return rows.value;
    return rows.value.filter(({ doc, signer }) => `${doc.docNo} ${doc.docFormatCode} ${doc.partyName} ${signer.positionName}`.toLowerCase().includes(query));
});

onMounted(loadTasks);

async function loadTasks() {
    loading.value = true;
    try {
        const result = await api.listMySigningTasks();
        documents.value = result.documents || [];
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดงานเซ็นไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openTask(taskId) {
    router.push({ name: 'my-signing-task', params: { taskId } });
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium' }).format(new Date(value));
}
</script>

<template>
    <section class="tasks-page">
        <header class="tasks-header">
            <div>
                <h1>งานรอเซ็น</h1>
                <p>เอกสารจะแสดงเมื่อถึงลำดับของคุณแล้ว</p>
            </div>
            <Tag :value="`${rows.length} งาน`" severity="info" />
        </header>

        <div class="task-search">
            <InputText v-model="searchQuery" type="search" placeholder="ค้นหาเลขเอกสาร, คู่ค้า, position" />
            <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadTasks" />
        </div>

        <div v-if="loading" class="task-state">
            <i class="pi pi-spin pi-spinner"></i>
            <span>กำลังโหลดงานรอเซ็น</span>
        </div>

        <div v-else-if="filteredRows.length === 0" class="empty-state">
            <i class="pi pi-inbox"></i>
            <strong>{{ searchQuery ? 'ไม่พบงานที่ค้นหา' : 'ยังไม่มีเอกสารที่ถึงลำดับของคุณ' }}</strong>
            <p>{{ searchQuery ? 'ลองค้นหาด้วยเลขเอกสารหรือชื่อคู่ค้าอีกครั้ง' : 'เมื่อขั้นตอนก่อนหน้าเซ็นครบ เอกสารจะปรากฏในหน้านี้' }}</p>
        </div>

        <div v-else class="task-list">
            <article v-for="{ doc, signer } in filteredRows" :key="signer.id" class="task-card">
                <div class="task-main">
                    <div>
                        <strong>{{ doc.docNo }}</strong>
                        <span>{{ doc.docFormatCode }} · {{ doc.partyName || doc.partyCode || '-' }}</span>
                    </div>
                    <Tag :value="signer.positionName" severity="info" />
                </div>
                <dl>
                    <div>
                        <dt>วันที่เอกสาร</dt>
                        <dd>{{ formatDate(doc.docDate) }}</dd>
                    </div>
                    <div>
                        <dt>สถานะ</dt>
                        <dd>รอเซ็น</dd>
                    </div>
                </dl>
                <Button label="เปิดเอกสาร" icon="pi pi-pencil" class="open-button" @click="openTask(signer.id)" />
            </article>
        </div>
    </section>
</template>

<style scoped>
.tasks-page {
    min-height: calc(100dvh - 56px);
    padding: 0.85rem;
    display: grid;
    align-content: start;
    gap: 0.85rem;
}

.tasks-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.tasks-header h1 {
    margin: 0;
    font-size: 1.35rem;
    line-height: 1.2;
}

.tasks-header p {
    margin: 0.25rem 0 0;
    color: var(--text-color-secondary);
}

.task-search {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.6rem;
}

.task-search :deep(.p-inputtext) {
    min-height: 44px;
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

.empty-state {
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 1.25rem;
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

.task-main {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
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

dl {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.65rem;
}

dt {
    font-size: 0.78rem;
}

dd {
    margin: 0.15rem 0 0;
    font-weight: 600;
}

.open-button {
    min-height: 44px;
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
</style>
