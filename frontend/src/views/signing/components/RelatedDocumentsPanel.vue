<script setup>
import DocumentFlowViewer from '@/views/signing/components/DocumentFlowViewer.vue';
import { onMounted, ref } from 'vue';
import { useToast } from 'primevue/usetoast';

const props = defineProps({
    loader: { type: Function, default: null },
    admin: { type: Boolean, default: false },
    compact: { type: Boolean, default: false },
    title: { type: String, default: 'เอกสารประกอบ' },
    subtitle: { type: String, default: 'ข้อมูลความสัมพันธ์จาก SML' },
    recordEvent: { type: Function, default: null }
});

const emit = defineEmits(['open-document', 'preview-pdf']);

const toast = useToast();
const loading = ref(false);
const loaded = ref(false);
const error = ref('');
const graph = ref(null);

onMounted(() => {
    void load();
});

async function load() {
    if (!props.loader || loading.value) return;
    loading.value = true;
    error.value = '';
    try {
        const result = await props.loader();
        graph.value = result.relatedDocuments || result.related_documents || result.documentFlow || result;
        loaded.value = true;
        props.recordEvent?.({ event: 'related_documents_load_success' });
    } catch (err) {
        error.value = err?.message || 'โหลดเอกสารประกอบไม่สำเร็จ';
        props.recordEvent?.({ event: 'related_documents_load_error', errorCode: 'related_documents_load_error' });
        toast.add({ severity: 'error', summary: 'โหลดเอกสารประกอบไม่สำเร็จ', detail: error.value, life: 3500 });
    } finally {
        loading.value = false;
    }
}

function recordClick() {
    props.recordEvent?.({ event: 'related_document_click' });
}
</script>

<template>
    <div class="related-documents" :class="{ compact }">
        <div class="flex items-center justify-between gap-3 mb-3">
            <div class="min-w-0">
                <div class="font-semibold">{{ title }}</div>
                <small class="text-muted-color">{{ subtitle }}</small>
            </div>
            <Button icon="pi pi-refresh" rounded outlined severity="secondary" aria-label="โหลดเอกสารประกอบใหม่" :loading="loading" @click="load" />
        </div>

        <Message v-if="error" severity="error" class="mb-3">
            {{ error }}
            <div class="mt-3">
                <Button size="small" label="ลองใหม่" icon="pi pi-refresh" severity="secondary" outlined @click="load" />
            </div>
        </Message>

        <div v-if="loading && !loaded" class="related-empty">
            <i class="pi pi-spin pi-spinner"></i>
            <span>กำลังโหลดเอกสารประกอบ</span>
        </div>

        <DocumentFlowViewer
            v-else
            :graph="graph"
            :admin="admin"
            :compact="compact"
            @node-click="recordClick"
            @open-document="(documentId) => emit('open-document', documentId)"
            @preview-pdf="(payload) => emit('preview-pdf', payload)"
        />
    </div>
</template>

<style scoped>
.related-empty {
    min-height: 7rem;
    border: 1px dashed var(--surface-border);
    border-radius: 8px;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.5rem;
    color: var(--text-color-secondary);
    text-align: center;
    padding: 1rem;
}
</style>
