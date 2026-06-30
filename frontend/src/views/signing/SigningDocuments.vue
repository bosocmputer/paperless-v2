<script setup>
import { api } from '@/services/api';
import { formatDocumentDate, formatThaiDateTime, signingStatusLabel, signingStatusSeverity } from '@/utils/signingFormatters';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const toast = useToast();

const documents = ref([]);
const loading = ref(false);
const searchQuery = ref('');

const filteredDocuments = computed(() => {
    const query = normalize(searchQuery.value);
    if (!query) return documents.value;
    return documents.value.filter((doc) => normalize(`${doc.docFormatCode} ${doc.docNo} ${doc.partyName} ${doc.partyCode} ${signingStatusLabel(doc.status)}`).includes(query));
});

onMounted(loadPage);

async function loadPage() {
    loading.value = true;
    try {
        const result = await api.listSigningDocuments();
        documents.value = result.documents || [];
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openCreate() {
    router.push({ name: 'signing-document-new' });
}

function openDetail(doc) {
    router.push({ name: 'signing-document-detail', params: { id: doc.id } });
}

function documentLine(doc) {
    return `${doc.docNo || '-'} ~ ${doc.docFormatCode || '-'} · ${doc.partyName || doc.partyCode || '-'}`;
}

function formatMoney(value) {
    return Number(value || 0).toLocaleString('th-TH', { minimumFractionDigits: 2 });
}

function normalize(value) {
    return String(value || '').toLowerCase().trim();
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div class="min-w-0 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                <div class="font-semibold text-xl whitespace-nowrap truncate">เอกสารเซ็น</div>
                <p class="text-muted-color m-0 min-w-0 truncate">ส่งเอกสารใหม่และติดตามสถานะการเซ็น</p>
            </div>
            <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                <IconField class="w-full sm:w-80">
                    <InputIcon><i class="pi pi-search" /></InputIcon>
                    <InputText v-model="searchQuery" type="search" placeholder="ค้นหาเลขเอกสาร คู่ค้า สถานะ" class="w-full" />
                </IconField>
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadPage" />
                <Button label="ส่งเอกสารใหม่" icon="pi pi-send" @click="openCreate" />
            </div>
        </div>

        <DataTable :value="filteredDocuments" :loading="loading" dataKey="id" paginator :rows="10" responsiveLayout="scroll" stripedRows>
            <template #empty>
                <div class="py-8 text-center text-muted-color">
                    {{ searchQuery ? 'ไม่พบเอกสารที่ค้นหา' : 'ยังไม่มีเอกสารเซ็น เริ่มจากปุ่มส่งเอกสารใหม่' }}
                </div>
            </template>

            <Column field="docNo" header="เลขที่เอกสาร" sortable style="min-width: 16rem">
                <template #body="{ data }">
                    <Button link class="p-0 font-bold text-left" @click="openDetail(data)">
                        <span class="whitespace-nowrap">{{ documentLine(data) }}</span>
                    </Button>
                </template>
            </Column>
            <Column field="docDate" header="วันที่เอกสาร" sortable style="min-width: 10rem">
                <template #body="{ data }">{{ formatDocumentDate(data.docDate) }}</template>
            </Column>
            <Column field="totalAmount" header="ยอดเงิน" sortable style="min-width: 10rem">
                <template #body="{ data }">{{ formatMoney(data.totalAmount) }}</template>
            </Column>
            <Column field="status" header="สถานะ" sortable style="min-width: 12rem">
                <template #body="{ data }">
                    <Tag :value="signingStatusLabel(data.status)" :severity="signingStatusSeverity(data.status)" />
                </template>
            </Column>
            <Column field="updatedAt" header="อัปเดตล่าสุด" sortable style="min-width: 14rem">
                <template #body="{ data }">{{ formatThaiDateTime(data.updatedAt) }}</template>
            </Column>
            <Column header="จัดการ" :exportable="false" style="width: 8rem">
                <template #body="{ data }">
                    <Button icon="pi pi-eye" rounded outlined severity="secondary" aria-label="ดูเอกสาร" @click="openDetail(data)" />
                </template>
            </Column>
        </DataTable>
    </div>
</template>
