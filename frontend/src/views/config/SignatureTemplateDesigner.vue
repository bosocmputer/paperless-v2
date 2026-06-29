<script setup>
import { api } from '@/services/api';
import * as pdfjsLib from 'pdfjs-dist';
import pdfWorker from 'pdfjs-dist/build/pdf.worker.mjs?url';
import { computed, nextTick, onMounted, ref, shallowRef, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;

const route = useRoute();
const router = useRouter();
const toast = useToast();

const docFormatCode = computed(() => String(route.params.docFormatCode || '').trim());
const loading = ref(false);
const uploading = ref(false);
const saving = ref(false);
const publishing = ref(false);
const rendering = ref(false);
const error = ref('');
const docFormat = ref(null);
const configs = ref([]);
const draft = ref(null);
const active = ref(null);
const boxes = ref([]);
const dirty = ref(false);
const selectedPositionCode = ref('');
const selectedBoxKey = ref('');
const fileInput = ref(null);
const canvasRef = ref(null);
const overlayRef = ref(null);
const pdfDoc = shallowRef(null);
const currentPage = ref(1);
const pageCount = ref(0);
const zoom = ref(1.2);
const pageSize = ref({ width: 0, height: 0 });
const maxTemplatePages = ref(20);

const template = computed(() => draft.value || active.value);
const isDraft = computed(() => template.value?.status === 'draft');
const statusSeverity = computed(() => (template.value?.status === 'active' ? 'success' : template.value?.status === 'draft' ? 'warn' : 'secondary'));
const pageOptions = computed(() => Array.from({ length: pageCount.value }, (_, index) => ({ label: `หน้า ${index + 1}`, value: index + 1 })));
const selectedStep = computed(() => configs.value.find((item) => item.positionCode === selectedPositionCode.value));
const currentPageBoxes = computed(() => boxes.value.filter((box) => Number(box.pageNo) === Number(currentPage.value)));
const validationIssues = computed(() => validateBoxes());
const canSave = computed(() => isDraft.value && !saving.value && !!template.value);
const canPublish = computed(() => isDraft.value && validationIssues.value.length === 0 && !publishing.value);
const storedPageCount = computed(() => Number(template.value?.sampleFile?.pageCount || pageCount.value || 0));

onMounted(loadState);

watch([currentPage, zoom], async () => {
    if (pdfDoc.value) await renderPage();
});

async function loadState() {
    loading.value = true;
    error.value = '';
    try {
        const result = await api.getSignatureTemplateState(docFormatCode.value);
        docFormat.value = result.docFormat;
        configs.value = result.configs || [];
        draft.value = result.draft;
        active.value = result.active;
        maxTemplatePages.value = result.maxTemplatePages || 20;
        boxes.value = withClientKeys((draft.value || active.value)?.boxes || []);
        selectedPositionCode.value = configs.value[0]?.positionCode || '';
        dirty.value = false;
        await nextTick();
        if (template.value?.id) await loadPDF();
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลด Template ไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadPDF() {
    if (!template.value?.sampleFileId) return;
    rendering.value = true;
    try {
        const loadingTask = pdfjsLib.getDocument({
            url: api.signatureTemplateSamplePDFUrl(template.value.id),
            httpHeaders: api.authHeaders()
        });
        pdfDoc.value = await loadingTask.promise;
        pageCount.value = pdfDoc.value.numPages;
        currentPage.value = Math.min(currentPage.value || 1, pageCount.value || 1);
        if (pageCount.value > maxTemplatePages.value) {
            toast.add({ severity: 'warn', summary: 'PDF มีหลายหน้าเกินกำหนด', detail: `รองรับสูงสุด ${maxTemplatePages.value} หน้า`, life: 5000 });
        }
        await renderPage();
    } catch (err) {
        error.value = err.message || 'Cannot render PDF preview.';
        toast.add({ severity: 'error', summary: 'แสดง PDF ไม่สำเร็จ', detail: err.message || error.value, life: 4000 });
    } finally {
        rendering.value = false;
    }
}

async function renderPage() {
    if (!pdfDoc.value || !canvasRef.value) return;
    rendering.value = true;
    try {
        const page = await pdfDoc.value.getPage(currentPage.value);
        const viewport = page.getViewport({ scale: zoom.value });
        const canvas = canvasRef.value;
        const context = canvas.getContext('2d');
        canvas.width = viewport.width;
        canvas.height = viewport.height;
        canvas.style.width = `${viewport.width}px`;
        canvas.style.height = `${viewport.height}px`;
        pageSize.value = { width: viewport.width, height: viewport.height };
        await page.render({ canvasContext: context, viewport }).promise;
    } finally {
        rendering.value = false;
    }
}

function triggerUpload() {
    fileInput.value?.click();
}

async function handleFileChange(event) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    if (boxes.value.length > 0 && !window.confirm('อัปโหลด PDF ใหม่จะล้างกรอบเดิมทั้งหมด ต้องการทำต่อไหม?')) return;

    uploading.value = true;
    error.value = '';
    try {
        const result = await api.uploadSignatureTemplateSamplePDF(docFormatCode.value, file);
        draft.value = result.template;
        active.value = null;
        boxes.value = withClientKeys(result.template?.boxes || []);
        dirty.value = false;
        currentPage.value = 1;
        toast.add({ severity: 'success', summary: 'อัปโหลด PDF แล้ว', life: 2500 });
        await loadState();
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'อัปโหลดไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        uploading.value = false;
    }
}

function addBox(step) {
    if (!isDraft.value) {
        toast.add({ severity: 'warn', summary: 'ต้องอัปโหลด PDF เพื่อสร้าง Draft ก่อน', life: 3500 });
        return;
    }
    const existing = boxes.value.filter((box) => box.positionCode === step.positionCode);
    const box = {
        clientKey: makeClientKey(),
        positionCode: step.positionCode,
        signerSlot: existing.length + 1,
        signerType: 'any',
        signerUser: '',
        pageNo: currentPage.value,
        xRatio: 0.12,
        yRatio: 0.72,
        widthRatio: 0.2,
        heightRatio: 0.08,
        label: step.positionName
    };

    if (step.conditionType === 2) {
        const used = new Set(existing.map((item) => item.signerUser).filter(Boolean));
        const user = stepUsers(step).find((item) => !used.has(item));
        if (!user) {
            toast.add({ severity: 'warn', summary: 'เพิ่มกรอบครบแล้ว', detail: `${step.positionName} ต้องมีกรอบเท่าจำนวน user ที่กำหนด`, life: 3500 });
            return;
        }
        box.signerType = 'internal';
        box.signerUser = user;
        box.signerSlot = Math.max(1, stepUsers(step).indexOf(user) + 1);
        box.label = user || step.positionName;
    }
    if (step.conditionType === 3) {
        box.signerType = 'external';
        box.label = 'บุคคลภายนอก';
    }

    boxes.value.push(box);
    selectedBoxKey.value = box.clientKey;
    dirty.value = true;
}

function deleteBox(box) {
    if (box.signerUser && !window.confirm(`ลบกรอบของ ${box.signerUser} ใช่ไหม?`)) return;
    boxes.value = boxes.value.filter((item) => item.clientKey !== box.clientKey);
    if (selectedBoxKey.value === box.clientKey) selectedBoxKey.value = '';
    dirty.value = true;
}

function boxStyle(box) {
    return {
        left: `${box.xRatio * 100}%`,
        top: `${box.yRatio * 100}%`,
        width: `${box.widthRatio * 100}%`,
        height: `${box.heightRatio * 100}%`
    };
}

function startBoxPointer(event, box, mode) {
    if (!isDraft.value || !overlayRef.value) return;
    event.preventDefault();
    event.stopPropagation();
    selectedBoxKey.value = box.clientKey;
    const rect = overlayRef.value.getBoundingClientRect();
    const start = {
        x: event.clientX,
        y: event.clientY,
        box: { ...box }
    };
    let frame = null;
    let latestEvent = event;

    const applyMove = () => {
        frame = null;
        const dx = (latestEvent.clientX - start.x) / rect.width;
        const dy = (latestEvent.clientY - start.y) / rect.height;
        const target = boxes.value.find((item) => item.clientKey === box.clientKey);
        if (!target) return;
        if (mode === 'move') {
            target.xRatio = clamp(start.box.xRatio + dx, 0, 1 - target.widthRatio);
            target.yRatio = clamp(start.box.yRatio + dy, 0, 1 - target.heightRatio);
        } else {
            target.widthRatio = clamp(start.box.widthRatio + dx, 0.03, 1 - target.xRatio);
            target.heightRatio = clamp(start.box.heightRatio + dy, 0.03, 1 - target.yRatio);
        }
        dirty.value = true;
    };

    const onMove = (moveEvent) => {
        latestEvent = moveEvent;
        if (!frame) frame = requestAnimationFrame(applyMove);
    };
    const onUp = () => {
        window.removeEventListener('pointermove', onMove);
        window.removeEventListener('pointerup', onUp);
        if (frame) cancelAnimationFrame(frame);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
}

async function saveDraft(showToast = true) {
    if (!template.value?.id) return null;
    if (!isDraft.value) {
        toast.add({ severity: 'warn', summary: 'Active template แก้ไม่ได้', detail: 'อัปโหลด PDF เพื่อสร้าง draft version ใหม่ก่อน', life: 4000 });
        return null;
    }

    saving.value = true;
    error.value = '';
    try {
        const payload = {
            revision: template.value.revision,
            boxes: boxes.value.map((box) => ({
                positionCode: box.positionCode,
                signerSlot: Number(box.signerSlot),
                signerType: box.signerType,
                signerUser: box.signerUser || '',
                pageNo: Number(box.pageNo),
                xRatio: Number(box.xRatio),
                yRatio: Number(box.yRatio),
                widthRatio: Number(box.widthRatio),
                heightRatio: Number(box.heightRatio),
                label: box.label || ''
            }))
        };
        const result = await api.saveSignatureTemplateBoxes(template.value.id, payload);
        draft.value = result.template;
        boxes.value = withClientKeys(result.template?.boxes || []);
        dirty.value = false;
        if (showToast) toast.add({ severity: 'success', summary: 'บันทึก Draft แล้ว', life: 2500 });
        return result.template;
    } catch (err) {
        const detail = err.status === 409 ? 'template ถูกแก้จากที่อื่นแล้ว กรุณา refresh' : err.message;
        error.value = detail;
        toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail, life: 4500 });
        return null;
    } finally {
        saving.value = false;
    }
}

async function publishTemplate() {
    if (validationIssues.value.length > 0) return;
    if (dirty.value) {
        const saved = await saveDraft(false);
        if (!saved) return;
    }

    publishing.value = true;
    error.value = '';
    try {
        const result = await api.publishSignatureTemplate(template.value.id);
        active.value = result.template;
        draft.value = null;
        boxes.value = withClientKeys(result.template?.boxes || []);
        dirty.value = false;
        toast.add({ severity: 'success', summary: 'Publish Template แล้ว', life: 3000 });
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'Publish ไม่สำเร็จ', detail: err.payload?.issues?.[0]?.message || err.message, life: 5000 });
    } finally {
        publishing.value = false;
    }
}

function validateBoxes() {
    const issues = [];
    if (!template.value?.sampleFileId) {
        issues.push({ code: 'sample_pdf_required', message: 'ต้องอัปโหลด PDF ตัวอย่างก่อน' });
    }
    if (storedPageCount.value > maxTemplatePages.value) {
        issues.push({ code: 'too_many_pages', message: `PDF ต้องไม่เกิน ${maxTemplatePages.value} หน้า` });
    }
    const byPosition = new Map();
    const usedSlots = new Set();
    boxes.value.forEach((box) => {
        if (!byPosition.has(box.positionCode)) byPosition.set(box.positionCode, []);
        byPosition.get(box.positionCode).push(box);
        const slotKey = `${box.positionCode}:${box.signerSlot}`;
        if (usedSlots.has(slotKey)) {
            issues.push({ code: 'box_signer_slot_duplicate', positionCode: box.positionCode, message: `กรอบของ Position ${box.positionCode} มีลำดับ signer ซ้ำ` });
        }
        usedSlots.add(slotKey);
        if (box.xRatio < 0 || box.yRatio < 0 || box.widthRatio <= 0 || box.heightRatio <= 0 || box.xRatio + box.widthRatio > 1 || box.yRatio + box.heightRatio > 1) {
            issues.push({ code: 'box_bounds_invalid', positionCode: box.positionCode, message: `กรอบของ Position ${box.positionCode} อยู่นอกหน้า PDF` });
        }
    });

    configs.value.forEach((step) => {
        const stepBoxes = byPosition.get(step.positionCode) || [];
        if (step.conditionType === 1 && !stepBoxes.some((box) => box.signerType === 'any')) {
            issues.push({ code: 'condition_any_box_required', positionCode: step.positionCode, message: `${step.positionName} ต้องมีกรอบอย่างน้อย 1 กรอบ` });
        }
        if (step.conditionType === 1 && stepBoxes.some((box) => box.signerType !== 'any' || box.signerUser)) {
            issues.push({ code: 'condition_any_type_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องเป็นกรอบแบบคนใดคนหนึ่งเท่านั้น` });
        }
        if (step.conditionType === 2) {
            const required = stepUsers(step);
            if (stepBoxes.length !== required.length) {
                issues.push({ code: 'condition_all_box_count_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องมี ${required.length} กรอบตามจำนวน user` });
            }
            stepBoxes.forEach((box) => {
                if (box.signerType !== 'internal' || !box.signerUser) {
                    issues.push({ code: 'condition_all_type_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องผูก user ภายในทุกกรอบ` });
                }
            });
            required.forEach((user) => {
                const count = stepBoxes.filter((box) => box.signerType === 'internal' && box.signerUser === user).length;
                if (count === 0) issues.push({ code: 'condition_all_missing_user_box', positionCode: step.positionCode, message: `${step.positionName} ต้องมีกรอบสำหรับ ${user}` });
                if (count > 1) issues.push({ code: 'condition_all_duplicate_user_box', positionCode: step.positionCode, message: `${step.positionName} มีกรอบของ ${user} ซ้ำ` });
            });
            stepBoxes
                .filter((box) => box.signerUser && !required.includes(box.signerUser))
                .forEach((box) => issues.push({ code: 'condition_all_unknown_user_box', positionCode: step.positionCode, message: `${step.positionName} มี user นอก config: ${box.signerUser}` }));
        }
        if (step.conditionType === 3 && !stepBoxes.some((box) => box.signerType === 'external')) {
            issues.push({ code: 'condition_external_box_required', positionCode: step.positionCode, message: `${step.positionName} ต้องมีกรอบบุคคลภายนอก` });
        }
        if (step.conditionType === 3 && stepBoxes.some((box) => box.signerType !== 'external' || box.signerUser)) {
            issues.push({ code: 'condition_external_type_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องเป็นกรอบบุคคลภายนอกเท่านั้น` });
        }
    });
    return issues;
}

function stepUsers(step) {
    return [step.user01, step.user02, step.user03].map((item) => String(item || '').trim()).filter(Boolean);
}

function conditionLabel(value) {
    if (Number(value) === 1) return '1 - คนใดคนหนึ่ง';
    if (Number(value) === 2) return '2 - ทุกคน';
    return '3 - บุคคลภายนอก';
}

function conditionSeverity(value) {
    if (Number(value) === 1) return 'info';
    if (Number(value) === 2) return 'warn';
    return 'secondary';
}

function withClientKeys(items) {
    return items.map((item) => ({ ...item, clientKey: item.id || makeClientKey() }));
}

function makeClientKey() {
    return `box_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
</script>

<template>
    <div class="flex flex-col gap-4">
        <div class="card">
            <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4">
                <div>
                    <div class="flex items-center gap-2 mb-2">
                        <Button icon="pi pi-arrow-left" severity="secondary" text rounded aria-label="กลับ" @click="router.push({ name: 'document-config' })" />
                        <div>
                            <div class="font-semibold text-xl">ตั้งค่ากรอบลายเซ็น {{ docFormatCode }}</div>
                            <p class="text-muted-color m-0">{{ docFormat?.name_1 || docFormat?.name_2 || 'Signature Template Designer' }}</p>
                        </div>
                    </div>
                    <div class="flex flex-wrap gap-2">
                        <Tag v-if="template" :value="`${template.status} v${template.version}`" :severity="statusSeverity" />
                        <Tag v-if="dirty" value="ยังไม่ได้บันทึก" severity="warn" />
                        <span class="text-sm text-muted-color">แก้ไขล่าสุด {{ formatDate(template?.updatedAt) }}</span>
                    </div>
                </div>

                <div class="flex flex-wrap gap-2">
                    <input ref="fileInput" type="file" accept="application/pdf,.pdf" class="hidden" @change="handleFileChange" />
                    <Button label="อัปโหลด PDF" icon="pi pi-upload" severity="secondary" outlined :loading="uploading" @click="triggerUpload" />
                    <Button label="บันทึก Draft" icon="pi pi-save" :disabled="!canSave" :loading="saving" @click="saveDraft()" />
                    <Button label="Publish" icon="pi pi-check" severity="success" :disabled="!canPublish" :loading="publishing" @click="publishTemplate" />
                </div>
            </div>
        </div>

        <Message v-if="error" severity="error">{{ error }}</Message>

        <div class="grid grid-cols-1 xl:grid-cols-12 gap-4">
            <div class="xl:col-span-8 card">
                <div class="flex flex-col md:flex-row md:items-center justify-between gap-3 mb-4">
                    <div class="flex items-center gap-2">
                        <Select v-model="currentPage" :options="pageOptions" optionLabel="label" optionValue="value" :disabled="pageOptions.length === 0" style="min-width: 8rem" />
                        <Button icon="pi pi-search-minus" severity="secondary" outlined :disabled="zoom <= 0.6" aria-label="Zoom out" @click="zoom = Number((zoom - 0.1).toFixed(2))" />
                        <span class="text-sm text-muted-color w-16 text-center">{{ Math.round(zoom * 100) }}%</span>
                        <Button icon="pi pi-search-plus" severity="secondary" outlined :disabled="zoom >= 2" aria-label="Zoom in" @click="zoom = Number((zoom + 0.1).toFixed(2))" />
                    </div>
                    <span class="text-sm text-muted-color">{{ rendering ? 'กำลัง render PDF' : pageSize.width ? `${Math.round(pageSize.width)} x ${Math.round(pageSize.height)} px · ${storedPageCount || pageCount} หน้า` : '' }}</span>
                </div>

                <div v-if="!template?.sampleFileId" class="signature-empty">
                    <i class="pi pi-file-pdf text-4xl text-muted-color"></i>
                    <div class="font-semibold mt-3">อัปโหลด PDF ตัวอย่างก่อน</div>
                    <p class="text-muted-color m-0">ใช้ไฟล์ PDF ของเอกสารจริงเพื่อกำหนดตำแหน่งลายเซ็น</p>
                </div>

                <div v-else class="pdf-scroll">
                    <div class="pdf-page-shell">
                        <canvas ref="canvasRef" class="pdf-canvas"></canvas>
                        <div ref="overlayRef" class="pdf-overlay">
                            <div
                                v-for="box in currentPageBoxes"
                                :key="box.clientKey"
                                class="signature-box"
                                :class="{ selected: selectedBoxKey === box.clientKey, readonly: !isDraft }"
                                :style="boxStyle(box)"
                                @pointerdown="startBoxPointer($event, box, 'move')"
                            >
                                <div class="signature-box-label">{{ box.label || box.signerUser || box.positionCode }}</div>
                                <button v-if="isDraft" class="signature-box-delete" type="button" @click.stop="deleteBox(box)">
                                    <i class="pi pi-times"></i>
                                </button>
                                <span v-if="isDraft" class="signature-box-handle" @pointerdown="startBoxPointer($event, box, 'resize')"></span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="xl:col-span-4 flex flex-col gap-4">
                <div class="card">
                    <div class="font-semibold text-lg mb-3">Position</div>
                    <div class="flex flex-col gap-3">
                        <div v-for="step in configs" :key="step.id" class="position-row" :class="{ active: selectedPositionCode === step.positionCode }" @click="selectedPositionCode = step.positionCode">
                            <div class="flex items-start justify-between gap-2">
                                <div>
                                    <div class="font-semibold">{{ step.positionCode }} - {{ step.positionName }}</div>
                                    <div class="text-sm text-muted-color">{{ stepUsers(step).join(', ') || 'ไม่มี user' }}</div>
                                </div>
                                <Tag :value="conditionLabel(step.conditionType)" :severity="conditionSeverity(step.conditionType)" />
                            </div>
                            <Button label="เพิ่มกรอบ" icon="pi pi-plus" size="small" class="mt-3" :disabled="!isDraft || !template?.sampleFileId" @click.stop="addBox(step)" />
                        </div>
                    </div>
                </div>

                <div class="card">
                    <div class="font-semibold text-lg mb-3">Validation</div>
                    <Message v-if="!isDraft && active" severity="info">Template นี้ Active แล้ว ถ้าต้องการแก้ไขให้อัปโหลด PDF เพื่อสร้าง Draft version ใหม่</Message>
                    <Message v-if="validationIssues.length === 0" severity="success">กรอบลายเซ็นครบตามเงื่อนไข สามารถ Publish ได้</Message>
                    <div v-else class="flex flex-col gap-2">
                        <Message v-for="issue in validationIssues" :key="`${issue.code}-${issue.positionCode}-${issue.message}`" severity="warn">
                            {{ issue.message }}
                        </Message>
                    </div>
                </div>

                <div class="card">
                    <div class="font-semibold text-lg mb-3">กล่องบนหน้าปัจจุบัน</div>
                    <div v-if="currentPageBoxes.length === 0" class="text-muted-color">ยังไม่มีกรอบในหน้านี้</div>
                    <div v-else class="flex flex-col gap-2">
                        <button
                            v-for="box in currentPageBoxes"
                            :key="box.clientKey"
                            type="button"
                            class="box-list-item"
                            :class="{ active: selectedBoxKey === box.clientKey }"
                            @click="selectedBoxKey = box.clientKey"
                        >
                            <span>{{ box.label || box.signerUser || box.positionCode }}</span>
                            <small>{{ box.positionCode }} / {{ box.signerType }}</small>
                        </button>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

<style scoped>
.pdf-scroll {
    overflow: auto;
    max-height: calc(100vh - 16rem);
    background: var(--surface-ground);
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 1rem;
}

.pdf-page-shell {
    position: relative;
    display: inline-block;
    line-height: 0;
    background: white;
    box-shadow: 0 4px 10px rgba(0, 0, 0, 0.12);
}

.pdf-canvas {
    display: block;
}

.pdf-overlay {
    position: absolute;
    inset: 0;
}

.signature-box {
    position: absolute;
    border: 2px solid var(--primary-color);
    background: color-mix(in srgb, var(--primary-color) 14%, transparent);
    cursor: move;
    line-height: 1.2;
    min-width: 32px;
    min-height: 24px;
}

.signature-box.selected {
    border-color: var(--p-orange-500, #f59e0b);
    background: rgba(245, 158, 11, 0.18);
}

.signature-box.readonly {
    cursor: default;
}

.signature-box-label {
    color: var(--primary-700, #1d4ed8);
    font-size: 0.75rem;
    font-weight: 700;
    padding: 0.25rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

.signature-box-delete {
    position: absolute;
    top: -0.7rem;
    right: -0.7rem;
    width: 1.4rem;
    height: 1.4rem;
    border: 0;
    border-radius: 999px;
    background: var(--red-500, #ef4444);
    color: white;
    cursor: pointer;
}

.signature-box-handle {
    position: absolute;
    right: -0.35rem;
    bottom: -0.35rem;
    width: 0.85rem;
    height: 0.85rem;
    border-radius: 999px;
    background: var(--primary-color);
    cursor: nwse-resize;
}

.signature-empty {
    min-height: 28rem;
    border: 1px dashed var(--surface-border);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    gap: 0.25rem;
}

.position-row,
.box-list-item {
    width: 100%;
    text-align: left;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 0.75rem;
}

.position-row.active,
.box-list-item.active {
    border-color: var(--primary-color);
    background: color-mix(in srgb, var(--primary-color) 8%, var(--surface-card));
}

.box-list-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    cursor: pointer;
}

@media (max-width: 960px) {
    .pdf-scroll {
        max-height: 70vh;
    }
}
</style>
