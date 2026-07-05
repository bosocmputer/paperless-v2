<script setup>
import { api } from '@/services/api';
import { authStore } from '@/stores/auth';
import SigningWorkspace from '@/views/signing/components/SigningWorkspace.vue';
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();

const document = ref(null);
const task = ref(null);
const legal = ref(null);
const loading = ref(false);

const pdfUrl = computed(() => api.mySigningHistoryPDFUrlForTask(task.value, document.value, 'current'));
const identityLabel = computed(() => authStore.user?.displayName || authStore.user?.username || task.value?.signerName || task.value?.signerUser || '');
const historyListRouteName = computed(() => (route.meta.adminSignerWorkspace === true ? 'admin-my-signing-history' : 'my-signing-history'));
const readOnlyReason = computed(() => {
    if (task.value?.status === 'rejected') return 'คุณปฏิเสธเอกสารนี้แล้ว หน้านี้เปิดดูย้อนหลังได้อย่างเดียว';
    return 'คุณเซ็นเอกสารนี้แล้ว หน้านี้เปิดดูย้อนหลังได้อย่างเดียว';
});

onMounted(loadHistory);

async function loadHistory() {
    loading.value = true;
    try {
        const result = await api.getMySigningHistory(route.params.taskId);
        document.value = result.document;
        task.value = result.task;
        legal.value = result.legal;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดประวัติไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function recordEvent(payload) {
    api.recordMySigningTaskEvent(route.params.taskId, payload).catch(() => {});
}

function goBackToHistory() {
    router.push({ name: historyListRouteName.value });
}

</script>

<template>
    <SigningWorkspace
        :document="document"
        :task="task"
        :legal="legal"
        :pdf-url="pdfUrl"
        :pdf-headers="api.authHeaders()"
        :loading="loading"
        :identity-label="identityLabel"
        :on-back="goBackToHistory"
        :on-reload="loadHistory"
        :on-record-event="recordEvent"
        read-only
        history-focus
        open-event-name="history_detail_open"
        pdf-open-event-name="history_pdf_open"
        :read-only-reason="readOnlyReason"
    />
</template>
