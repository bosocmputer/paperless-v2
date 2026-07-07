<script setup>
import { api } from '@/services/api';
import ContinuousPdfViewer from '@/views/signing/components/ContinuousPdfViewer.vue';
import { computed, onBeforeUnmount, watch } from 'vue';
import { useToast } from 'primevue/usetoast';

const props = defineProps({
    visible: { type: Boolean, default: false },
    url: { type: String, default: '' },
    title: { type: String, default: 'ดูเอกสาร' },
    emptyMessage: { type: String, default: 'ยังไม่มี PDF' },
    fullHeight: { type: Boolean, default: false },
    actionUrl: { type: String, default: '' },
    actionLabel: { type: String, default: 'เปิด PaperLess' },
    actionIcon: { type: String, default: 'pi pi-external-link' }
});

const emit = defineEmits(['update:visible']);

const toast = useToast();

const dialogVisible = computed({
    get: () => props.visible,
    set: (value) => emit('update:visible', value)
});

watch(
    () => props.visible,
    (visible) => {
        if (visible) addPrintGuard();
        else removePrintGuard();
    },
    { immediate: true }
);

onBeforeUnmount(() => {
    removePrintGuard();
});

function addPrintGuard() {
    document.addEventListener('keydown', blockBrowserPrint, true);
}

function removePrintGuard() {
    document.removeEventListener('keydown', blockBrowserPrint, true);
}

function blockBrowserPrint(event) {
    if (!props.visible) return;
    if (!(event.ctrlKey || event.metaKey) || String(event.key).toLowerCase() !== 'p') return;
    event.preventDefault();
    event.stopPropagation();
    toast.add({
        severity: 'warn',
        summary: 'ใช้ปุ่มพิมพ์เอกสาร',
        detail: 'ระบบจะบันทึกประวัติเมื่อพิมพ์ผ่านปุ่มพิมพ์เอกสารเท่านั้น',
        life: 3500
    });
}

function openActionUrl() {
    if (!props.actionUrl) return;
    const anchor = document.createElement('a');
    anchor.href = props.actionUrl;
    anchor.target = '_blank';
    anchor.rel = 'noopener noreferrer';
    anchor.style.display = 'none';
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
}
</script>

<template>
    <Dialog
        v-model:visible="dialogVisible"
        modal
        :header="title"
        class="readonly-pdf-dialog"
        :class="{ 'readonly-pdf-dialog-full': fullHeight }"
        :style="{ width: fullHeight ? 'min(96rem, 98vw)' : 'min(72rem, 96vw)', height: fullHeight ? '96dvh' : undefined }"
        @hide="dialogVisible = false"
    >
        <div class="readonly-pdf" :class="{ 'full-height': fullHeight }" @keydown.capture="blockBrowserPrint" @contextmenu.prevent>
            <Message severity="info" class="m-0">หน้าดูอย่างเดียว หากต้องพิมพ์ให้ใช้ปุ่มพิมพ์เอกสารเพื่อบันทึกประวัติ</Message>
            <ContinuousPdfViewer :url="url" :headers="api.authHeaders()" :empty-message="emptyMessage" toolbar-label="เอกสาร" />
        </div>
        <template #footer>
            <Button v-if="actionUrl" :label="actionLabel" :icon="actionIcon" severity="secondary" outlined @click="openActionUrl" />
            <Button label="ปิด" severity="secondary" outlined @click="dialogVisible = false" />
        </template>
    </Dialog>
</template>

<style scoped>
.readonly-pdf {
    height: min(78dvh, 52rem);
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    gap: 0.75rem;
}

.readonly-pdf.full-height {
    height: 100%;
    min-height: 0;
}

:global(.readonly-pdf-dialog-full .p-dialog-content) {
    height: calc(96dvh - 8rem);
    display: flex;
    flex-direction: column;
}

:global(.readonly-pdf-dialog-full .p-dialog-footer) {
    flex: 0 0 auto;
}
</style>
