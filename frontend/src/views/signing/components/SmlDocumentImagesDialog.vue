<script setup>
import SmlDocumentImagesPanel from '@/views/signing/components/SmlDocumentImagesPanel.vue';
import { computed, ref, watch } from 'vue';

const props = defineProps({
    visible: { type: Boolean, default: false },
    documentId: { type: String, default: '' },
    docNo: { type: String, default: '' },
    documentStatus: { type: String, default: '' }
});

const emit = defineEmits(['update:visible']);

const imageCount = ref(0);

const title = computed(() => {
    const base = props.docNo ? `รูปใน SML · ${props.docNo}` : 'รูปใน SML';
    return imageCount.value > 0 ? `${base} (${imageCount.value} รูป)` : base;
});

// The panel only fetches while the dialog is open, so closing it also drops the
// blob URLs it was holding rather than keeping a document's images in memory.
const open = computed({
    get: () => props.visible,
    set: (value) => emit('update:visible', value)
});

watch(
    () => props.visible,
    (value) => {
        if (!value) imageCount.value = 0;
    }
);
</script>

<template>
    <Dialog
        v-model:visible="open"
        :header="title"
        modal
        dismissableMask
        class="sml-images-dialog"
        :style="{ width: '76rem', height: '88dvh' }"
        :breakpoints="{ '1280px': '92vw', '768px': '96vw' }"
        contentClass="sml-images-dialog-content"
    >
        <SmlDocumentImagesPanel :document-id="documentId" :enabled="visible" :document-status="documentStatus" @update:count="imageCount = $event" />
    </Dialog>
</template>

<style scoped>
/* Dialog is teleported out of this component's tree, so its internals are only
   reachable with :global - the same approach ReadOnlyPdfDialog already uses.
   Pinning the content height is what lets the gallery size to the space left
   over; without it the content grows past the dialog and the whole dialog
   scrolls, which is wrong for a viewer you page through. */
:global(.sml-images-dialog .p-dialog-content) {
    height: calc(88dvh - 5rem);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    padding-top: 0.5rem;
}
</style>
