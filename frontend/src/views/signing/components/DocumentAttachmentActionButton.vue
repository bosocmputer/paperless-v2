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
    <span v-if="safeCount > 0" class="attachment-action-button">
        <Button icon="pi pi-paperclip" rounded outlined severity="secondary" :loading="loading" :aria-label="label" :title="label" @click="$emit('click')" />
        <span class="attachment-action-count" aria-hidden="true">{{ safeCount }}</span>
    </span>
</template>

<style scoped>
.attachment-action-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.2rem;
}

.attachment-action-count {
    min-width: 0.9rem;
    color: var(--text-color-secondary);
    font-size: 0.78rem;
    font-weight: 700;
    line-height: 1;
    pointer-events: none;
}
</style>
