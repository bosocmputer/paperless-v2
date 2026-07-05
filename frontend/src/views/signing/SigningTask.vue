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
const saving = ref(false);

const pdfUrl = computed(() => api.signingDocumentPDFUrlForDocument(document.value));
const identityLabel = computed(() => authStore.user?.displayName || authStore.user?.username || task.value?.signerName || task.value?.signerUser || '');
const isAdminSignerWorkspace = computed(() => route.meta.adminSignerWorkspace === true);
const taskListRouteName = computed(() => (isAdminSignerWorkspace.value ? 'admin-my-signing-tasks' : 'my-signing-tasks'));

onMounted(loadTask);

async function loadTask() {
    loading.value = true;
    try {
        const result = await api.getMySigningTask(route.params.taskId);
        document.value = result.document;
        task.value = result.task;
        legal.value = result.legal;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function signTask(payload) {
    saving.value = true;
    try {
        const result = await api.signMyTask(route.params.taskId, payload);
        document.value = result.document;
        task.value = (result.document?.signers || []).find((item) => item.id === route.params.taskId) || task.value;
        toast.add({ severity: 'success', summary: 'เซ็นเอกสารแล้ว', life: 2600 });
        goBackToTasks();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'เซ็นไม่สำเร็จ', detail: err.message, life: 4200 });
        throw err;
    } finally {
        saving.value = false;
    }
}

async function rejectTask(payload) {
    saving.value = true;
    try {
        const result = await api.rejectMyTask(route.params.taskId, payload);
        document.value = result.document;
        task.value = (result.document?.signers || []).find((item) => item.id === route.params.taskId) || task.value;
        toast.add({ severity: 'success', summary: 'ปฏิเสธเอกสารแล้ว', life: 2600 });
        goBackToTasks();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ปฏิเสธไม่สำเร็จ', detail: err.message, life: 4200 });
        throw err;
    } finally {
        saving.value = false;
    }
}

async function attachFile(file, note) {
    await api.uploadMyTaskAttachment(route.params.taskId, file, note);
}

function recordEvent(payload) {
    api.recordMySigningTaskEvent(route.params.taskId, payload).catch(() => {});
}

function loadRelatedDocuments() {
    return api.getMySigningTaskRelatedDocuments(route.params.taskId);
}

function goBackToTasks() {
    router.push({ name: taskListRouteName.value });
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
        :saving="saving"
        :identity-label="identityLabel"
        :admin-workspace="isAdminSignerWorkspace"
        :on-back="goBackToTasks"
        :on-reload="loadTask"
        :on-sign="signTask"
        :on-reject="rejectTask"
        :on-attach="attachFile"
        :on-record-event="recordEvent"
        :related-loader="loadRelatedDocuments"
    />
</template>
