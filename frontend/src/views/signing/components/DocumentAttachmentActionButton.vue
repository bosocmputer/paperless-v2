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
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
}

.attachment-action-count {
    position: absolute;
    top: -0.35rem;
    right: -0.35rem;
    min-width: 1.2rem;
    height: 1.2rem;
    display: inline-grid;
    place-items: center;
    border: 2px solid var(--surface-card);
    border-radius: 999px;
    background: color-mix(in srgb, var(--p-blue-500, #3b82f6) 14%, var(--surface-card));
    color: var(--p-blue-700, #1d4ed8);
    font-size: 0.68rem;
    font-weight: 700;
    line-height: 1;
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--p-blue-500, #3b82f6) 24%, transparent);
    pointer-events: none;
}
</style>
