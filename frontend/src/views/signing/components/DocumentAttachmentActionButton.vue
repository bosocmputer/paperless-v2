<script setup>
import { computed } from 'vue';

const props = defineProps({
    count: { type: Number, default: 0 },
    loading: { type: Boolean, default: false },
    ariaLabel: { type: String, default: '' }
});

defineEmits(['click']);

const safeCount = computed(() => Math.max(0, Number(props.count || 0)));
const label = computed(() => props.ariaLabel || `ไฟล์แนบอ้างอิง ${safeCount.value} ไฟล์`);
</script>

<template>
    <OverlayBadge v-if="safeCount > 0" :value="String(safeCount)" severity="info" class="attachment-action-button">
        <Button icon="pi pi-paperclip" rounded outlined severity="secondary" :loading="loading" :aria-label="label" :title="label" @click="$emit('click')" />
    </OverlayBadge>
</template>

<style scoped>
.attachment-action-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    vertical-align: middle;
}

.attachment-action-button :deep(.p-badge) {
    min-width: 1.05rem;
    height: 1.05rem;
    padding: 0 0.28rem;
    font-size: 0.65rem;
    line-height: 1.05rem;
}
</style>
