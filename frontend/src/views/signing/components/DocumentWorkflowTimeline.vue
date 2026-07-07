<script setup>
import { buildWorkflowTimeline, documentProgressIssue } from '@/utils/workflowTimeline';
import { formatThaiDateTime } from '@/utils/signingFormatters';
import { computed } from 'vue';

const props = defineProps({
    document: { type: Object, default: null },
    showExternalActions: { type: Boolean, default: false },
    compact: { type: Boolean, default: false }
});

const emit = defineEmits(['generate-external']);

const items = computed(() => buildWorkflowTimeline(props.document));
const issue = computed(() => documentProgressIssue(props.document));

function canGenerateExternal(signer) {
    return props.showExternalActions && signer.signerType === 'external' && signer.status !== 'signed' && signer.status !== 'skipped';
}
</script>

<template>
    <div class="workflow-progress" :class="{ 'workflow-progress-compact': compact }">
        <Message v-if="issue" :severity="issue.severity" class="mb-3">{{ issue.message }}</Message>

        <div v-if="items.length === 0" class="workflow-empty">
            <i class="pi pi-inbox"></i>
            <span>ยังไม่มีขั้นตอนในเอกสารนี้</span>
        </div>

        <Timeline v-else :value="items" align="left" class="workflow-timeline">
            <template #opposite="{ item }">
                <div class="workflow-opposite">
                    <strong>ขั้นตอน {{ item.sequenceLabel }}</strong>
                    <small v-if="item.timeLabel">{{ item.timeLabel }}</small>
                </div>
            </template>

            <template #marker="{ item }">
                <span class="workflow-marker" :class="`workflow-${item.severity}`">
                    <i :class="item.icon"></i>
                </span>
            </template>

            <template #content="{ item }">
                <div class="workflow-content" :class="{ muted: item.muted }">
                    <div class="workflow-title-row">
                        <div class="min-w-0">
                            <strong class="workflow-title">{{ item.title }}</strong>
                            <small>{{ item.conditionLabel }}</small>
                        </div>
                        <Tag :value="item.statusLabel" :severity="item.severity" />
                    </div>

                    <p class="workflow-summary">{{ item.summary }}</p>

                    <div v-if="item.signers.length" class="workflow-signers">
                        <div v-for="signer in item.signers" :key="signer.id || `${item.key}-${signer.label}`" class="workflow-signer">
                            <span class="min-w-0">
                                <strong>{{ signer.label }}</strong>
                                <small v-if="signer.signedAt">เซ็นเมื่อ {{ formatThaiDateTime(signer.signedAt) }}</small>
                                <small v-else-if="signer.rejectedAt">ปฏิเสธเมื่อ {{ formatThaiDateTime(signer.rejectedAt) }}</small>
                                <small v-if="signer.signNote" class="workflow-note">หมายเหตุ: {{ signer.signNote }}</small>
                            </span>
                            <span class="workflow-signer-actions">
                                <Tag :value="signer.statusLabel" :severity="signer.severity" />
                                <Button
                                    v-if="canGenerateExternal(signer)"
                                    icon="pi pi-key"
                                    rounded
                                    outlined
                                    size="small"
                                    aria-label="สร้างลิงก์ผู้เซ็นภายนอก"
                                    @click="emit('generate-external', signer)"
                                />
                            </span>
                        </div>
                    </div>
                </div>
            </template>
        </Timeline>
    </div>
</template>

<style scoped>
.workflow-progress {
    min-width: 0;
}

.workflow-progress-compact {
    display: grid;
    gap: 0.45rem;
}

.workflow-empty {
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

.workflow-empty i {
    font-size: 1.45rem;
    color: var(--primary-color);
}

.workflow-opposite {
    display: grid;
    gap: 0.15rem;
    min-width: 7.5rem;
    text-align: right;
    color: var(--text-color-secondary);
}

.workflow-opposite strong {
    color: var(--text-color);
    font-size: 0.85rem;
}

.workflow-opposite small {
    font-size: 0.78rem;
}

.workflow-marker {
    width: 1.65rem;
    height: 1.65rem;
    border-radius: 999px;
    display: inline-grid;
    place-items: center;
    border: 2px solid var(--surface-card);
    font-size: 0.78rem;
}

.workflow-content {
    min-width: 0;
    display: grid;
    gap: 0.55rem;
    padding: 0 0 1.2rem 0.35rem;
}

.workflow-content.muted {
    opacity: 0.72;
}

.workflow-title-row,
.workflow-signer {
    min-width: 0;
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}

.workflow-title-row small,
.workflow-signer small {
    display: block;
    color: var(--text-color-secondary);
}

.workflow-title {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.workflow-summary {
    margin: 0;
    color: var(--text-color-secondary);
}

.workflow-signers {
    display: grid;
    gap: 0.45rem;
}

.workflow-signer {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.55rem 0.65rem;
}

.workflow-signer strong {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.workflow-signer-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    flex: 0 0 auto;
}

.workflow-success {
    color: var(--green-700, #15803d);
    background: var(--green-100, #dcfce7);
}

.workflow-info {
    color: var(--blue-700, #1d4ed8);
    background: var(--blue-100, #dbeafe);
}

.workflow-danger {
    color: var(--red-700, #b91c1c);
    background: var(--red-100, #fee2e2);
}

.workflow-warn {
    color: var(--yellow-800, #854d0e);
    background: var(--yellow-100, #fef9c3);
}

.workflow-secondary {
    color: var(--text-color-secondary);
    background: var(--surface-hover);
}

.workflow-timeline :deep(.p-timeline-event-opposite) {
    flex: 0 0 8.5rem;
    padding: 0 0.75rem 0 0;
}

.workflow-timeline :deep(.p-timeline-event-content) {
    padding-left: 0.75rem;
}

.workflow-timeline :deep(.p-timeline-event-marker) {
    border: 0;
}

.workflow-progress-compact .workflow-timeline :deep(.p-timeline-event-opposite) {
    display: none;
}

.workflow-progress-compact .workflow-timeline :deep(.p-timeline-event-content) {
    padding-left: 0.45rem;
}

.workflow-progress-compact .workflow-content {
    gap: 0.35rem;
    padding: 0 0 0.6rem 0.2rem;
}

.workflow-progress-compact .workflow-title-row {
    align-items: center;
    gap: 0.45rem;
}

.workflow-progress-compact .workflow-title-row small {
    display: none;
}

.workflow-progress-compact .workflow-summary {
    overflow: hidden;
    color: var(--text-color-secondary);
    font-size: 0.82rem;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.workflow-progress-compact .workflow-signers {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
}

.workflow-progress-compact .workflow-signer {
    min-width: 0;
    max-width: 100%;
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    border-radius: 999px;
    padding: 0.35rem 0.45rem 0.35rem 0.6rem;
    background: color-mix(in srgb, var(--surface-ground) 58%, var(--surface-card));
}

.workflow-progress-compact .workflow-signer span:first-child {
    min-width: 0;
}

.workflow-progress-compact .workflow-signer strong {
    max-width: 8.5rem;
    font-size: 0.84rem;
}

.workflow-progress-compact .workflow-signer small {
    display: none;
}

.workflow-progress-compact .workflow-signer-actions {
    gap: 0.2rem;
}

.workflow-progress-compact .workflow-signer-actions :deep(.p-tag) {
    font-size: 0.72rem;
}

@media (max-width: 640px) {
    .workflow-timeline :deep(.p-timeline-event) {
        align-items: flex-start;
    }

    .workflow-timeline :deep(.p-timeline-event-opposite) {
        flex: 0 0 5.75rem;
        padding-right: 0.5rem;
    }

    .workflow-opposite {
        min-width: 0;
        overflow-wrap: anywhere;
        text-align: left;
    }

    .workflow-title-row,
    .workflow-signer {
        align-items: stretch;
        flex-direction: column;
    }

    .workflow-signer-actions {
        justify-content: flex-start;
    }
}
</style>
