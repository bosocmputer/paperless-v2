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
    <div class="card">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div>
                <div class="font-semibold text-xl mb-1">เอกสารรอเซ็น</div>
                <p class="text-muted-color m-0">เอกสารที่ถึงลำดับของคุณแล้ว</p>
            </div>
            <div class="flex gap-2">
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหาเอกสาร" class="w-full sm:w-72" />
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadTasks" />
            </div>
        </div>

        <DataTable :value="filteredRows" :loading="loading" dataKey="signer.id" paginator :rows="10" responsiveLayout="scroll" stripedRows>
            <template #empty>
                <div class="py-6 text-center text-muted-color">{{ searchQuery ? 'ไม่พบงานที่ค้นหา' : 'ไม่มีเอกสารรอเซ็น' }}</div>
            </template>
            <Column header="เอกสาร">
                <template #body="{ data }">
                    <div class="font-semibold">{{ data.doc.docNo }}</div>
                    <div class="text-sm text-muted-color">{{ data.doc.docFormatCode }} · {{ data.doc.partyName || data.doc.partyCode || '-' }}</div>
                </template>
            </Column>
            <Column header="Position">
                <template #body="{ data }">
                    <Tag :value="data.signer.positionName" severity="info" />
                </template>
            </Column>
            <Column header="วันที่เอกสาร">
                <template #body="{ data }">{{ formatDate(data.doc.docDate) }}</template>
            </Column>
            <Column header="จัดการ" style="width: 9rem">
                <template #body="{ data }">
                    <Button label="เปิดเซ็น" icon="pi pi-pencil" @click="openTask(data.signer.id)" />
                </template>
            </Column>
        </DataTable>
    </div>
</template>
