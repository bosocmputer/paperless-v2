<script setup>
import ReadOnlyPdfDialog from '@/views/signing/components/ReadOnlyPdfDialog.vue';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useToast } from 'primevue/usetoast';

const props = defineProps({
    attachments: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false },
    error: { type: String, default: '' },
    readonly: { type: Boolean, default: false },
    canUpload: { type: Boolean, default: false },
    requirements: { type: Array, default: () => [] },
    signerId: { type: String, default: '' },
    allowOptionalUpload: { type: Boolean, default: true },
    uploadLabel: { type: String, default: 'เลือก PDF/รูปภาพ' },
    title: { type: String, default: 'ไฟล์แนบอ้างอิง' },
    headers: { type: Object, default: () => ({}) },
    fileUrlResolver: { type: Function, default: null },
    onUpload: { type: Function, default: null },
    onReload: { type: Function, default: null }
});

const toast = useToast();
const note = ref('');
const uploading = ref(false);
const pdfVisible = ref(false);
const pdfUrl = ref('');
const pdfTitle = ref('ดูไฟล์แนบ');
const imageVisible = ref(false);
const imageUrl = ref('');
const imageTitle = ref('ดูไฟล์แนบ');

const attachmentCount = computed(() => props.attachments.length || 0);
const showUpload = computed(() => !props.readonly && props.canUpload && !!props.onUpload);
const requirementItems = computed(() =>
    (props.requirements || [])
        .map((item) => ({
            key: String(item?.key || '').trim(),
            label: String(item?.label || '').trim()
        }))
        .filter((item) => item.key && item.label)
);
const hasRequirements = computed(() => requirementItems.value.length > 0);
const attachmentsByRequirement = computed(() => {
    const map = new Map();
    for (const attachment of props.attachments || []) {
        const key = String(attachment?.requirementKey || '').trim();
        if (!key) continue;
        if (props.signerId && String(attachment?.signerId || '').trim() !== props.signerId) continue;
        if (!map.has(key)) map.set(key, []);
        map.get(key).push(attachment);
    }
    return map;
});

watch(
    () => imageVisible.value,
    (visible) => {
        if (!visible) revokeImageUrl();
    }
);

onBeforeUnmount(() => {
    revokeImageUrl();
});

function fileName(attachment) {
    return attachment?.file?.originalName || 'ไฟล์แนบ';
}

function fileMeta(attachment) {
    const file = attachment?.file || {};
    const parts = [];
    if (file.contentType) parts.push(file.contentType);
    if (file.sizeBytes) parts.push(formatBytes(file.sizeBytes));
    if (file.pageCount) parts.push(`${file.pageCount} หน้า`);
    return parts.join(' · ') || '-';
}

function signerMeta(attachment) {
    const parts = [];
    if (attachment?.positionName) parts.push(attachment.positionName);
    if (attachment?.signerName) parts.push(attachment.signerName);
    return parts.join(' · ');
}

function formatBytes(value) {
    const bytes = Number(value || 0);
    if (!bytes) return '-';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(value) {
    if (!value) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat('th-TH', {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    }).format(date);
}

function requirementAttachments(requirement) {
    return attachmentsByRequirement.value.get(requirement.key) || [];
}

async function uploadAttachment(event, requirement = null) {
    const input = event.target;
    const file = input.files?.[0];
    input.value = '';
    if (!file || !showUpload.value || uploading.value) return;
    uploading.value = true;
    try {
        await props.onUpload(file, note.value, requirement?.key || '');
        note.value = '';
        toast.add({ severity: 'success', summary: 'แนบไฟล์แล้ว', life: 2200 });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'แนบไฟล์ไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        uploading.value = false;
    }
}

async function openAttachment(attachment) {
    const url = props.fileUrlResolver?.(attachment);
    if (!url) {
        toast.add({ severity: 'warn', summary: 'ยังเปิดไฟล์ไม่ได้', detail: 'ไม่พบ URL ของไฟล์แนบ', life: 3000 });
        return;
    }
    const title = fileName(attachment);
    const contentType = String(attachment?.file?.contentType || '').toLowerCase();
    if (contentType.includes('pdf') || title.toLowerCase().endsWith('.pdf')) {
        pdfTitle.value = title;
        pdfUrl.value = url;
        pdfVisible.value = true;
        return;
    }
    try {
        const objectUrl = await fetchAttachmentObjectUrl(url);
        if (contentType.startsWith('image/')) {
            imageTitle.value = title;
            imageUrl.value = objectUrl;
            imageVisible.value = true;
            return;
        }
        window.open(objectUrl, '_blank', 'noopener,noreferrer');
        window.setTimeout(() => URL.revokeObjectURL(objectUrl), 30_000);
    } catch (err) {
        toast.add({ severity: 'error', summary: 'เปิดไฟล์แนบไม่สำเร็จ', detail: err.message, life: 3500 });
    }
}

async function fetchAttachmentObjectUrl(url) {
    const response = await fetch(url, {
        headers: props.headers || {},
        cache: 'no-store'
    });
    if (!response.ok) throw new Error('ไม่สามารถโหลดไฟล์แนบได้');
    const blob = await response.blob();
    return URL.createObjectURL(blob);
}

function revokeImageUrl() {
    if (!imageUrl.value) return;
    URL.revokeObjectURL(imageUrl.value);
    imageUrl.value = '';
}
</script>

<template>
    <section class="attachments-panel">
        <div class="attachments-head">
            <div>
                <div class="attachments-title">
                    <i class="pi pi-paperclip"></i>
                    <span>{{ title }}</span>
                </div>
                <small v-if="attachmentCount" class="text-muted-color">ดูได้เฉพาะผู้เกี่ยวข้องภายในเอกสาร</small>
            </div>
            <div class="attachments-actions">
                <Tag :value="`${attachmentCount} ไฟล์`" :severity="attachmentCount ? 'info' : 'secondary'" />
                <Button v-if="onReload" icon="pi pi-refresh" severity="secondary" text rounded aria-label="โหลดไฟล์แนบใหม่" :loading="loading" @click="onReload" />
            </div>
        </div>

        <Message v-if="error" severity="warn" class="m-0">{{ error }}</Message>
        <div v-if="hasRequirements" class="requirements-list">
            <article v-for="requirement in requirementItems" :key="requirement.key" class="requirement-row" :class="{ complete: requirementAttachments(requirement).length > 0 }">
                <div class="requirement-copy">
                    <i :class="requirementAttachments(requirement).length ? 'pi pi-check-circle' : 'pi pi-exclamation-circle'"></i>
                    <div>
                        <strong>{{ requirement.label }}</strong>
                        <small>{{ requirementAttachments(requirement).length ? `${requirementAttachments(requirement).length} ไฟล์` : 'ยังไม่ได้แนบ' }}</small>
                    </div>
                </div>
                <label v-if="showUpload" class="slot-upload-button" :class="{ disabled: uploading }">
                    <input type="file" accept="application/pdf,image/png,image/jpeg" :disabled="uploading" @change="uploadAttachment($event, requirement)" />
                    <i :class="uploading ? 'pi pi-spin pi-spinner' : 'pi pi-upload'"></i>
                    <span>{{ uploading ? 'กำลังแนบ' : 'แนบไฟล์' }}</span>
                </label>
            </article>
        </div>
        <div v-if="loading" class="attachments-loading">
            <i class="pi pi-spin pi-spinner"></i>
            <span>กำลังโหลดไฟล์แนบ</span>
        </div>
        <div v-else-if="attachmentCount" class="attachments-list">
            <article v-for="attachment in attachments" :key="attachment.id" class="attachment-row">
                <div class="attachment-main">
                    <i class="pi pi-file attachment-icon"></i>
                    <div class="attachment-copy">
                        <strong>{{ fileName(attachment) }}</strong>
                        <span>{{ fileMeta(attachment) }}</span>
                        <small v-if="attachment.requirementLabel">เอกสารบังคับ: {{ attachment.requirementLabel }}</small>
                        <small v-if="signerMeta(attachment)">แนบโดย {{ signerMeta(attachment) }}</small>
                        <small v-if="attachment.note">หมายเหตุ: {{ attachment.note }}</small>
                        <small v-if="formatDate(attachment.createdAt)">แนบเมื่อ {{ formatDate(attachment.createdAt) }}</small>
                    </div>
                </div>
                <Button label="ดูไฟล์" icon="pi pi-eye" severity="secondary" outlined size="small" @click="openAttachment(attachment)" />
            </article>
        </div>
        <Message v-else severity="info" class="m-0">ยังไม่มีไฟล์แนบอ้างอิง</Message>

        <div v-if="showUpload && allowOptionalUpload" class="attachment-upload">
            <InputText v-model="note" placeholder="หมายเหตุไฟล์แนบ (ถ้ามี)" :disabled="uploading" />
            <label class="upload-button" :class="{ disabled: uploading }">
                <input type="file" accept="application/pdf,image/png,image/jpeg" :disabled="uploading" @change="uploadAttachment($event)" />
                <i :class="uploading ? 'pi pi-spin pi-spinner' : 'pi pi-paperclip'"></i>
                <span>{{ uploading ? 'กำลังแนบไฟล์' : uploadLabel }}</span>
            </label>
        </div>

        <ReadOnlyPdfDialog v-model:visible="pdfVisible" :url="pdfUrl" :headers="headers" :title="pdfTitle" full-height />
        <Dialog v-model:visible="imageVisible" modal :header="imageTitle" :style="{ width: 'min(72rem, 96vw)' }">
            <div class="attachment-image-preview">
                <img v-if="imageUrl" :src="imageUrl" :alt="imageTitle" />
            </div>
            <template #footer>
                <Button label="ปิด" severity="secondary" outlined @click="imageVisible = false" />
            </template>
        </Dialog>
    </section>
</template>

<style scoped>
.attachments-panel {
    display: grid;
    gap: 0.85rem;
    padding: 1rem;
    border: 1px solid var(--surface-border);
    border-radius: 12px;
    background: var(--surface-card);
}

.attachments-head,
.attachments-actions,
.attachment-main,
.upload-button {
    display: flex;
    align-items: center;
    gap: 0.75rem;
}

.attachments-head {
    justify-content: space-between;
}

.attachments-title {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-weight: 700;
}

.attachments-list {
    display: grid;
    gap: 0.75rem;
    max-height: min(24rem, 42vh);
    overflow-y: auto;
    padding-right: 0.15rem;
}

.requirements-list {
    display: grid;
    gap: 0.6rem;
}

.requirement-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.75rem;
    border: 1px solid color-mix(in srgb, var(--orange-400) 55%, var(--surface-border));
    border-radius: 10px;
    background: color-mix(in srgb, var(--orange-50) 65%, var(--surface-card));
}

.requirement-row.complete {
    border-color: color-mix(in srgb, var(--green-400) 60%, var(--surface-border));
    background: color-mix(in srgb, var(--green-50) 70%, var(--surface-card));
}

.requirement-copy {
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 0.65rem;
}

.requirement-copy i {
    color: var(--orange-500);
}

.requirement-row.complete .requirement-copy i {
    color: var(--green-600);
}

.requirement-copy div {
    min-width: 0;
    display: grid;
    gap: 0.1rem;
}

.requirement-copy strong,
.requirement-copy small {
    overflow-wrap: anywhere;
}

.requirement-copy small {
    color: var(--text-color-secondary);
}

.slot-upload-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.45rem;
    min-height: 2.35rem;
    padding: 0 0.75rem;
    border: 1px solid var(--primary-color);
    border-radius: 9px;
    color: var(--primary-color);
    font-weight: 700;
    white-space: nowrap;
    cursor: pointer;
}

.slot-upload-button input {
    display: none;
}

.slot-upload-button.disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

.attachment-row {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.8rem;
    border: 1px solid var(--surface-border);
    border-radius: 10px;
    background: var(--surface-ground);
}

.attachment-icon {
    color: var(--primary-color);
}

.attachment-copy {
    min-width: 0;
    display: grid;
    gap: 0.2rem;
}

.attachment-copy strong,
.attachment-copy span,
.attachment-copy small {
    min-width: 0;
    overflow-wrap: anywhere;
}

.attachment-copy span,
.attachment-copy small {
    color: var(--text-color-secondary);
}

.attachments-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    min-height: 4rem;
    color: var(--text-color-secondary);
}

.attachment-upload {
    display: grid;
    gap: 0.65rem;
}

.upload-button {
    justify-content: center;
    min-height: 2.75rem;
    border: 1px dashed var(--primary-color);
    border-radius: 10px;
    color: var(--primary-color);
    font-weight: 700;
    cursor: pointer;
}

.upload-button input {
    display: none;
}

.upload-button.disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

.attachment-image-preview {
    display: grid;
    place-items: center;
    min-height: 55vh;
    background: var(--surface-ground);
    border-radius: 10px;
    overflow: auto;
}

.attachment-image-preview img {
    max-width: 100%;
    max-height: 78vh;
    object-fit: contain;
}

@media (max-width: 640px) {
    .attachments-head,
    .attachment-row,
    .requirement-row {
        align-items: stretch;
        flex-direction: column;
    }

    .attachment-row :deep(.p-button),
    .slot-upload-button {
        width: 100%;
    }
}
</style>
