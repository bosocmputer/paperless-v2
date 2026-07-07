<script setup>
import DocumentAttachmentsPanel from '@/views/signing/components/DocumentAttachmentsPanel.vue';
import { computed, ref, watch } from 'vue';

const props = defineProps({
    visible: { type: Boolean, default: false },
    title: { type: String, default: 'ไฟล์แนบอ้างอิง' },
    subtitle: { type: String, default: '' },
    loader: { type: Function, required: true },
    loaderKey: { type: String, default: '' },
    fileUrlResolver: { type: Function, required: true },
    headers: { type: Object, default: () => ({}) }
});

const emit = defineEmits(['update:visible']);

const loading = ref(false);
const error = ref('');
const attachments = ref([]);
let requestSeq = 0;

const dialogVisible = computed({
    get: () => props.visible,
    set: (value) => emit('update:visible', value)
});

watch(
    () => [props.visible, props.loaderKey],
    ([visible]) => {
        if (visible) void loadAttachments();
    },
    { immediate: true }
);

watch(
    () => props.visible,
    (visible) => {
        if (!visible) resetDialog();
    }
);

async function loadAttachments() {
    const seq = ++requestSeq;
    loading.value = true;
    error.value = '';
    try {
        const result = await props.loader();
        if (seq !== requestSeq) return;
        attachments.value = Array.isArray(result.attachments) ? result.attachments : [];
    } catch (err) {
        if (seq !== requestSeq) return;
        attachments.value = [];
        error.value = err?.message || 'โหลดไฟล์แนบไม่สำเร็จ';
    } finally {
        if (seq === requestSeq) loading.value = false;
    }
}

function resetDialog() {
    requestSeq += 1;
    loading.value = false;
    error.value = '';
    attachments.value = [];
}
</script>

<template>
    <Dialog v-model:visible="dialogVisible" modal :draggable="false" class="attachments-dialog" :style="{ width: 'min(42rem, 94vw)' }">
        <template #header>
            <div class="attachments-dialog-header">
                <strong>{{ title }}</strong>
                <small v-if="subtitle">{{ subtitle }}</small>
            </div>
        </template>

        <DocumentAttachmentsPanel
            v-if="dialogVisible"
            readonly
            :title="'ไฟล์แนบอ้างอิง'"
            :attachments="attachments"
            :loading="loading"
            :error="error"
            :headers="headers"
            :on-reload="loadAttachments"
            :file-url-resolver="fileUrlResolver"
        />

        <template #footer>
            <Button label="ปิด" severity="secondary" outlined @click="dialogVisible = false" />
        </template>
    </Dialog>
</template>

<style scoped>
.attachments-dialog-header {
    min-width: 0;
    display: grid;
    gap: 0.15rem;
}

.attachments-dialog-header strong,
.attachments-dialog-header small {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.attachments-dialog-header small {
    color: var(--text-color-secondary);
    font-weight: 500;
}

:global(.attachments-dialog .p-dialog-content) {
    background: var(--surface-ground);
}

:deep(.attachments-panel) {
    border-radius: 8px;
}
</style>
