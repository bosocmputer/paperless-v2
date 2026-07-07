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
const referenceStatus = ref(null);
const attachments = ref([]);
const attachmentsLoading = ref(false);
const attachmentsError = ref('');
let referenceStatusRequestSeq = 0;
let attachmentRequestSeq = 0;

const pdfUrl = computed(() => api.signingDocumentPDFUrlForDocument(document.value));
const identityLabel = computed(() => authStore.user?.displayName || authStore.user?.username || task.value?.signerName || task.value?.signerUser || '');
const isAdminSignerWorkspace = computed(() => route.meta.adminSignerWorkspace === true);
const taskListRouteName = computed(() => (isAdminSignerWorkspace.value ? 'admin-my-signing-tasks' : 'my-signing-tasks'));

onMounted(loadTask);

async function loadTask() {
    const requestSeq = ++referenceStatusRequestSeq;
    loading.value = true;
    referenceStatus.value = null;
    attachments.value = [];
    attachmentsError.value = '';
    try {
        const result = await api.getMySigningTask(route.params.taskId);
        document.value = result.document;
        task.value = result.task;
        legal.value = result.legal;
        loadReferenceStatus(route.params.taskId, requestSeq);
        loadAttachments(route.params.taskId);
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadReferenceStatus(taskId, requestSeq = ++referenceStatusRequestSeq) {
    if (!taskId) return;
    try {
        const result = await api.getMySigningTaskReferenceStatus(taskId);
        if (requestSeq === referenceStatusRequestSeq) {
            referenceStatus.value = result;
        }
    } catch {
        if (requestSeq === referenceStatusRequestSeq) {
            referenceStatus.value = null;
        }
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
    await loadAttachments(route.params.taskId);
}

async function loadAttachments(taskId = route.params.taskId) {
    if (!taskId) return;
    const requestSeq = ++attachmentRequestSeq;
    attachmentsLoading.value = true;
    attachmentsError.value = '';
    try {
        const result = await api.getMyTaskAttachments(taskId);
        if (requestSeq === attachmentRequestSeq) {
            attachments.value = Array.isArray(result.attachments) ? result.attachments : [];
        }
    } catch (err) {
        if (requestSeq === attachmentRequestSeq) {
            attachmentsError.value = err.message || 'โหลดไฟล์แนบไม่สำเร็จ';
        }
    } finally {
        if (requestSeq === attachmentRequestSeq) {
            attachmentsLoading.value = false;
        }
    }
}

function attachmentFileUrl(attachment) {
    return api.myTaskAttachmentFileUrl(route.params.taskId, attachment?.id || '');
}

function recordEvent(payload) {
    api.recordMySigningTaskEvent(route.params.taskId, payload).catch(() => {});
}

function loadRelatedDocuments() {
    return api.getMySigningTaskRelatedDocuments(route.params.taskId);
}

function loadReferenceCheck() {
    return api.getMySigningTaskReferenceCheck(route.params.taskId);
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
        :reference-status="referenceStatus"
        :attachments="attachments"
        :attachments-loading="attachmentsLoading"
        :attachments-error="attachmentsError"
        :on-back="goBackToTasks"
        :on-reload="loadTask"
        :on-sign="signTask"
        :on-reject="rejectTask"
        :on-attach="attachFile"
        :on-reload-attachments="loadAttachments"
        :attachment-file-url="attachmentFileUrl"
        :on-record-event="recordEvent"
        :related-loader="loadRelatedDocuments"
        :reference-check-loader="loadReferenceCheck"
    />
</template>
