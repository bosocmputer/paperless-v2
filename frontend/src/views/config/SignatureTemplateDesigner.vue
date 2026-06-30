<script setup>
import { api } from '@/services/api';
import * as pdfjsLib from 'pdfjs-dist';
import pdfWorker from 'pdfjs-dist/build/pdf.worker.mjs?url';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;

const route = useRoute();
const router = useRouter();
const confirm = useConfirm();
const toast = useToast();

const docFormatCode = computed(() => String(route.params.docFormatCode || '').trim());
const loading = ref(false);
const uploading = ref(false);
const saving = ref(false);
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
const viewerRef = ref(null);
const pdfDoc = shallowRef(null);
const currentPage = ref(1);
const pageCount = ref(0);
const zoom = ref(1.2);
const fitWidthActive = ref(false);
const pageSize = ref({ width: 0, height: 0 });
const maxTemplatePages = ref(20);

const designerSessionId = makeDesignerSessionId();
const openedAt = Date.now();
let designerOpenRecorded = false;
let renderSequence = 0;
let renderTask = null;
let resizeObserver = null;
let fitWidthTimer = null;
let pointerCleanup = null;
let discardNavigationConfirmed = false;

const template = computed(() => active.value || draft.value);
const canEdit = computed(() => !!template.value && template.value.status !== 'archived');
const pageOptions = computed(() => Array.from({ length: pageCount.value }, (_, index) => ({ label: `หน้า ${index + 1}`, value: index + 1 })));
const selectedStep = computed(() => configs.value.find((item) => item.positionCode === selectedPositionCode.value));
const selectedBox = computed(() => boxes.value.find((box) => box.clientKey === selectedBoxKey.value) || null);
const selectedBoxStep = computed(() => (selectedBox.value ? configs.value.find((item) => item.positionCode === selectedBox.value.positionCode) : null));
const selectedBoxSignerOptions = computed(() => stepUsers(selectedBoxStep.value || {}).map((user, index) => ({ label: user, value: user, slot: index + 1 })));
const selectedBoxSignerTypeLabel = computed(() => {
    if (!selectedBox.value) return '-';
    if (selectedBox.value.signerType === 'any') return 'คนใดคนหนึ่ง';
    if (selectedBox.value.signerType === 'external') return 'บุคคลภายนอก';
    return 'User ภายใน';
});
const boxesByPosition = computed(() => groupBoxesBy((box) => box.positionCode));
const boxesByPage = computed(() => groupBoxesBy((box) => Number(box.pageNo)));
const currentPageBoxes = computed(() => boxesByPage.value.get(Number(currentPage.value)) || []);
const validationIssues = computed(() => validateBoxes());
const validationByPosition = computed(() => {
    const grouped = new Map();
    validationIssues.value.forEach((issue) => {
        const key = issue.positionCode || '_global';
        if (!grouped.has(key)) grouped.set(key, []);
        grouped.get(key).push(issue);
    });
    return grouped;
});
const stepViews = computed(() =>
    configs.value.map((step) => {
        const stepBoxes = boxesByPosition.value.get(step.positionCode) || [];
        const required = requiredBoxesForStep(step);
        return {
            ...step,
            users: stepUsers(step),
            boxes: stepBoxes,
            required,
            issues: validationByPosition.value.get(step.positionCode) || [],
            isActive: selectedPositionCode.value === step.positionCode,
            isComplete: required > 0 && stepBoxes.length >= required && !(validationByPosition.value.get(step.positionCode) || []).length,
            canAdd: canAddBoxForStep(step, stepBoxes),
            addDisabledReason: addDisabledReasonForStep(step, stepBoxes)
        };
    })
);
const canSave = computed(() => canEdit.value && !saving.value && !!template.value);
const storedPageCount = computed(() => Number(template.value?.sampleFile?.pageCount || pageCount.value || 0));
const requiredBoxCount = computed(() => configs.value.reduce((total, step) => total + requiredBoxesForStep(step), 0));
const boxProgressLabel = computed(() => `${boxes.value.length}/${requiredBoxCount.value || 0}`);
const validationStatusLabel = computed(() => (validationIssues.value.length === 0 ? 'พร้อมใช้เป็นค่าเริ่มต้น' : `${validationIssues.value.length} จุดต้องแก้`));
const validationStatusSeverity = computed(() => (validationIssues.value.length === 0 ? 'success' : 'warn'));
const canAddBoxes = computed(() => canEdit.value && !!template.value?.sampleFileId);
const docTitle = computed(() => docFormat.value?.name_1 || docFormat.value?.name_2 || 'กรอบเริ่มต้น');
const pdfMetaLabel = computed(() => {
    if (rendering.value) return 'กำลัง render PDF';
    if (!pageSize.value.width) return '';
    return `${Math.round(pageSize.value.width)} x ${Math.round(pageSize.value.height)} px · ${storedPageCount.value || pageCount.value} หน้า`;
});

onMounted(async () => {
    window.addEventListener('beforeunload', handleBeforeUnload);
    await loadState();
    await nextTick();
    setupResizeObserver();
});

onBeforeUnmount(() => {
    window.removeEventListener('beforeunload', handleBeforeUnload);
    cleanupPointerListeners();
    cancelRenderTask();
    destroyPDF();
    if (resizeObserver) resizeObserver.disconnect();
    if (fitWidthTimer) clearTimeout(fitWidthTimer);
});

onBeforeRouteLeave((_to, _from, next) => {
    if (discardNavigationConfirmed) {
        discardNavigationConfirmed = false;
        next();
        return;
    }
    if (!dirty.value) {
        next();
        return;
    }
    confirm.require({
        message: 'ยังไม่ได้บันทึกการแก้ไขกรอบลายเซ็น ต้องการออกจากหน้านี้หรือไม่?',
        header: 'ออกจากหน้ากรอบเริ่มต้น',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'อยู่หน้านี้ต่อ',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ออกจากหน้านี้',
            severity: 'danger'
        },
        accept: () => next(),
        reject: () => next(false)
    });
});

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
        boxes.value = withClientKeys((active.value || draft.value)?.boxes || []);
        selectedPositionCode.value = configs.value[0]?.positionCode || '';
        selectedBoxKey.value = '';
        dirty.value = false;
        await nextTick();
        if (template.value?.id) {
            await loadPDF();
            recordDesignerOpen();
        } else {
            await destroyPDF();
        }
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลดกรอบเริ่มต้นไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadPDF() {
    if (!template.value?.sampleFileId) {
        await destroyPDF();
        return;
    }
    rendering.value = true;
    try {
        cancelRenderTask();
        if (pdfDoc.value?.destroy) await pdfDoc.value.destroy().catch(() => {});
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
        if (fitWidthActive.value) scheduleFitWidth();
    } catch (err) {
        error.value = err.message || 'Cannot render PDF preview.';
        recordDesignerEvent('pdf_render_error');
        toast.add({ severity: 'error', summary: 'แสดง PDF ไม่สำเร็จ', detail: err.message || error.value, life: 4000 });
    } finally {
        rendering.value = false;
    }
}

async function renderPage() {
    if (!pdfDoc.value || !canvasRef.value) return;
    const sequence = ++renderSequence;
    cancelRenderTask();
    rendering.value = true;
    try {
        const page = await pdfDoc.value.getPage(currentPage.value);
        if (sequence !== renderSequence) return;
        const viewport = page.getViewport({ scale: zoom.value });
        const canvas = canvasRef.value;
        const context = canvas.getContext('2d');
        canvas.width = viewport.width;
        canvas.height = viewport.height;
        canvas.style.width = `${viewport.width}px`;
        canvas.style.height = `${viewport.height}px`;
        pageSize.value = { width: viewport.width, height: viewport.height };
        renderTask = page.render({ canvasContext: context, viewport });
        await renderTask.promise;
    } catch (err) {
        if (err?.name === 'RenderingCancelledException') return;
        error.value = err.message || 'Cannot render PDF preview.';
        recordDesignerEvent('pdf_render_error');
        toast.add({ severity: 'error', summary: 'แสดง PDF ไม่สำเร็จ', detail: error.value, life: 4000 });
    } finally {
        if (sequence === renderSequence) {
            renderTask = null;
            rendering.value = false;
        }
    }
}

function cancelRenderTask() {
    if (!renderTask) return;
    try {
        renderTask.cancel();
    } catch {
        // PDF.js can throw if the render already completed.
    }
    renderTask = null;
}

async function destroyPDF() {
    cancelRenderTask();
    const doc = pdfDoc.value;
    pdfDoc.value = null;
    pageCount.value = 0;
    pageSize.value = { width: 0, height: 0 };
    if (doc?.destroy) await doc.destroy().catch(() => {});
}

function triggerUpload() {
    fileInput.value?.click();
}

async function handleFileChange(event) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    if (boxes.value.length > 0) {
        confirm.require({
            message: 'อัปโหลด PDF ใหม่จะล้างกรอบเดิมทั้งหมด และใช้ไฟล์ใหม่นี้แทนของเก่า',
            header: 'แทนที่ PDF ตัวอย่าง',
            icon: 'pi pi-exclamation-triangle',
            rejectProps: {
                label: 'ยกเลิก',
                severity: 'secondary',
                outlined: true
            },
            acceptProps: {
                label: 'แทนที่ PDF และล้างกรอบ',
                severity: 'danger'
            },
            accept: () => {
                recordDesignerEvent('upload_confirm');
                uploadSamplePDF(file);
            }
        });
        return;
    }

    recordDesignerEvent('upload_confirm');
    await uploadSamplePDF(file);
}

async function uploadSamplePDF(file) {
    uploading.value = true;
    error.value = '';
    try {
        const result = await api.uploadSignatureTemplateSamplePDF(docFormatCode.value, file);
        active.value = result.template;
        draft.value = null;
        boxes.value = withClientKeys(result.template?.boxes || []);
        dirty.value = false;
        currentPage.value = 1;
        selectedBoxKey.value = '';
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
    selectedPositionCode.value = step.positionCode;
    if (!canEdit.value || !template.value?.sampleFileId) {
        toast.add({ severity: 'warn', summary: 'ต้องอัปโหลด PDF ตัวอย่างก่อน', life: 3500 });
        return;
    }
    const existing = boxesByPosition.value.get(step.positionCode) || [];
    const required = requiredBoxesForStep(step);
    if (existing.length >= required) {
        toast.add({ severity: 'info', summary: 'เพิ่มกรอบครบแล้ว', detail: `${step.positionName} มีกรอบครบตามเงื่อนไขแล้ว`, life: 3000 });
        return;
    }
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
    selectBox(box, { scrollIntoView: true });
    dirty.value = true;
    recordDesignerEvent('box_add', { positionCode: step.positionCode, conditionType: Number(step.conditionType) });
}

function deleteBox(box) {
    const label = box.signerUser || box.label || `Position ${box.positionCode}`;
    confirm.require({
        message: `ลบกรอบ "${label}" ออกจาก template นี้ใช่ไหม?`,
        header: 'ลบกรอบลายเซ็น',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'ยกเลิก',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ลบกรอบ',
            severity: 'danger'
        },
        accept: () => removeBox(box)
    });
}

function removeBox(box) {
    const step = configs.value.find((item) => item.positionCode === box.positionCode);
    boxes.value = boxes.value.filter((item) => item.clientKey !== box.clientKey);
    if (selectedBoxKey.value === box.clientKey) selectedBoxKey.value = '';
    dirty.value = true;
    recordDesignerEvent('box_delete', { positionCode: box.positionCode, conditionType: Number(step?.conditionType || 0) });
}

function selectBox(box, options = {}) {
    if (!box) return;
    selectedBoxKey.value = box.clientKey;
    selectedPositionCode.value = box.positionCode;
    if (Number(box.pageNo) !== Number(currentPage.value)) currentPage.value = Number(box.pageNo);
    if (options.scrollIntoView) nextTick(() => scrollBoxIntoView(box));
}

function updateBoxLabel(box, value) {
    if (!canEdit.value || !box) return;
    box.label = String(value || '').slice(0, 80);
    dirty.value = true;
}

function updateBoxPage(box, value) {
    if (!canEdit.value || !box) return;
    const pageNo = Number(value);
    if (!Number.isFinite(pageNo) || pageNo < 1 || pageNo > Math.max(pageCount.value, 1)) return;
    box.pageNo = pageNo;
    currentPage.value = pageNo;
    dirty.value = true;
}

function updateBoxSignerUser(box, value) {
    if (!canEdit.value || !box || !selectedBoxStep.value || Number(selectedBoxStep.value.conditionType) !== 2) return;
    const user = String(value || '').trim();
    const option = selectedBoxSignerOptions.value.find((item) => item.value === user);
    box.signerType = 'internal';
    box.signerUser = user;
    box.signerSlot = option?.slot || Math.max(1, Number(box.signerSlot || 1));
    box.label = user || selectedBoxStep.value.positionName;
    dirty.value = true;
}

function ratioPercent(box, field) {
    if (!box) return 0;
    return Number((Number(box[field] || 0) * 100).toFixed(2));
}

function updateBoxRatio(box, field, value) {
    if (!canEdit.value || !box) return;
    const percent = Number(value);
    if (!Number.isFinite(percent)) return;
    const ratio = percent / 100;

    if (field === 'xRatio') {
        box.xRatio = clamp(ratio, 0, 1 - box.widthRatio);
    } else if (field === 'yRatio') {
        box.yRatio = clamp(ratio, 0, 1 - box.heightRatio);
    } else if (field === 'widthRatio') {
        box.widthRatio = clamp(ratio, 0.03, 1 - box.xRatio);
    } else if (field === 'heightRatio') {
        box.heightRatio = clamp(ratio, 0.03, 1 - box.yRatio);
    }

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
    selectBox(box);
    if (!canEdit.value || !overlayRef.value) return;
    cleanupPointerListeners();
    event.preventDefault();
    event.stopPropagation();
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
    const onUp = () => cleanupPointerListeners();

    pointerCleanup = () => {
        window.removeEventListener('pointermove', onMove);
        window.removeEventListener('pointerup', onUp);
        if (frame) cancelAnimationFrame(frame);
        frame = null;
        pointerCleanup = null;
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
}

function cleanupPointerListeners() {
    if (pointerCleanup) pointerCleanup();
}

async function saveTemplate(showToast = true) {
    if (!template.value?.id) return null;
    if (!canEdit.value) {
        toast.add({ severity: 'warn', summary: 'กรอบเริ่มต้นนี้แก้ไขไม่ได้', life: 4000 });
        return null;
    }

    const selectedSnapshot = selectedBox.value ? boxSnapshot(selectedBox.value) : null;
    recordDesignerEvent('save_attempt');
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
        active.value = result.template;
        draft.value = null;
        boxes.value = withClientKeys(result.template?.boxes || []);
        restoreSelectedBox(selectedSnapshot);
        dirty.value = false;
        recordDesignerEvent('save_success');
        if (showToast) toast.add({ severity: 'success', summary: 'บันทึกแล้ว', life: 2500 });
        return result.template;
    } catch (err) {
        const detail = err.status === 409 ? 'template ถูกแก้จากที่อื่นแล้ว กรุณา refresh' : err.message;
        error.value = detail;
        recordDesignerEvent(err.status === 409 ? 'revision_conflict' : 'save_error');
        toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail, life: 4500 });
        return null;
    } finally {
        saving.value = false;
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
        if (stepBoxes.length === 0) return;
        if (step.conditionType === 1 && !stepBoxes.some((box) => box.signerType === 'any')) {
            issues.push({ code: 'condition_any_box_required', positionCode: step.positionCode, message: `${step.positionName} ต้องมีกรอบอย่างน้อย 1 กรอบ` });
        }
        if (step.conditionType === 1 && stepBoxes.length > 1) {
            issues.push({ code: 'condition_any_box_count_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องมีได้ 1 กรอบเท่านั้น` });
        }
        if (step.conditionType === 1 && stepBoxes.some((box) => box.signerType !== 'any' || box.signerUser)) {
            issues.push({ code: 'condition_any_type_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องเป็นกรอบแบบคนใดคนหนึ่งเท่านั้น` });
        }
        if (step.conditionType === 2) {
            const required = stepUsers(step);
            const seen = new Map();
            stepBoxes.forEach((box) => {
                if (box.signerType !== 'internal' || !box.signerUser) {
                    issues.push({ code: 'condition_all_type_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องผูก user ภายในทุกกรอบ` });
                }
                if (box.signerUser) seen.set(box.signerUser, (seen.get(box.signerUser) || 0) + 1);
            });
            Array.from(seen.entries()).forEach(([user, count]) => {
                if (count > 1) issues.push({ code: 'condition_all_duplicate_user_box', positionCode: step.positionCode, message: `${step.positionName} มีกรอบของ ${user} ซ้ำ` });
            });
            stepBoxes
                .filter((box) => box.signerUser && !required.includes(box.signerUser))
                .forEach((box) => issues.push({ code: 'condition_all_unknown_user_box', positionCode: step.positionCode, message: `${step.positionName} มี user นอก config: ${box.signerUser}` }));
        }
        if (step.conditionType === 3 && !stepBoxes.some((box) => box.signerType === 'external')) {
            issues.push({ code: 'condition_external_box_required', positionCode: step.positionCode, message: `${step.positionName} ต้องมีกรอบบุคคลภายนอก` });
        }
        if (step.conditionType === 3 && stepBoxes.length > 1) {
            issues.push({ code: 'condition_external_box_count_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องมีได้ 1 กรอบเท่านั้น` });
        }
        if (step.conditionType === 3 && stepBoxes.some((box) => box.signerType !== 'external' || box.signerUser)) {
            issues.push({ code: 'condition_external_type_invalid', positionCode: step.positionCode, message: `${step.positionName} ต้องเป็นกรอบบุคคลภายนอกเท่านั้น` });
        }
    });
    return issues;
}

function setupResizeObserver() {
    if (!viewerRef.value || typeof ResizeObserver === 'undefined') return;
    resizeObserver = new ResizeObserver(() => {
        if (fitWidthActive.value) scheduleFitWidth();
    });
    resizeObserver.observe(viewerRef.value);
}

function scheduleFitWidth() {
    if (fitWidthTimer) clearTimeout(fitWidthTimer);
    fitWidthTimer = setTimeout(() => fitPDFToWidth(), 80);
}

function fitPDFToWidth() {
    if (!viewerRef.value || !pageSize.value.width || !zoom.value) return;
    const baseWidth = pageSize.value.width / zoom.value;
    const availableWidth = Math.max(320, viewerRef.value.clientWidth - 40);
    zoom.value = Number(clamp(availableWidth / baseWidth, 0.6, 2).toFixed(2));
}

function activateFitWidth() {
    fitWidthActive.value = true;
    fitPDFToWidth();
}

function setZoom(value) {
    fitWidthActive.value = false;
    zoom.value = Number(clamp(value, 0.6, 2).toFixed(2));
}

function stepUsers(step) {
    return [step.user01, step.user02, step.user03].map((item) => String(item || '').trim()).filter(Boolean);
}

function requiredBoxesForStep(step) {
    if (Number(step.conditionType) === 2) return stepUsers(step).length;
    return 1;
}

function canAddBoxForStep(step, stepBoxes = []) {
    return canAddBoxes.value && stepBoxes.length < requiredBoxesForStep(step);
}

function addDisabledReasonForStep(step, stepBoxes = []) {
    if (!template.value?.sampleFileId) return 'ต้องอัปโหลด PDF ก่อน';
    if (!canEdit.value) return 'กรอบเริ่มต้นนี้แก้ไขไม่ได้';
    if (stepBoxes.length >= requiredBoxesForStep(step)) return 'เพิ่มครบแล้ว';
    return '';
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

function signerTypeShortLabel(value) {
    if (value === 'external') return 'ภายนอก';
    if (value === 'internal') return 'ภายใน';
    if (value === 'any') return 'คนใดคนหนึ่ง';
    return value || '-';
}

function groupBoxesBy(getKey) {
    const grouped = new Map();
    boxes.value.forEach((box) => {
        const key = getKey(box);
        if (!grouped.has(key)) grouped.set(key, []);
        grouped.get(key).push(box);
    });
    return grouped;
}

function withClientKeys(items) {
    return items.map((item) => ({ ...item, clientKey: item.id || makeClientKey() }));
}

function makeClientKey() {
    return `box_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

function makeDesignerSessionId() {
    if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
    return `designer_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

function boxSnapshot(box) {
    return {
        positionCode: box.positionCode,
        signerSlot: Number(box.signerSlot),
        signerUser: box.signerUser || '',
        signerType: box.signerType
    };
}

function restoreSelectedBox(snapshot) {
    if (!snapshot) return;
    const match =
        boxes.value.find(
            (box) =>
                box.positionCode === snapshot.positionCode &&
                Number(box.signerSlot) === snapshot.signerSlot &&
                (box.signerUser || '') === snapshot.signerUser &&
                box.signerType === snapshot.signerType
        ) || boxes.value.find((box) => box.positionCode === snapshot.positionCode);
    if (match) selectBox(match);
}

function scrollBoxIntoView(box) {
    const scroll = viewerRef.value;
    if (!scroll || !pageSize.value.width || !pageSize.value.height) return;
    const maxTop = Math.max(0, scroll.scrollHeight - scroll.clientHeight);
    const maxLeft = Math.max(0, scroll.scrollWidth - scroll.clientWidth);
    const top = clamp(box.yRatio * pageSize.value.height - scroll.clientHeight * 0.35, 0, maxTop);
    const left = clamp(box.xRatio * pageSize.value.width - scroll.clientWidth * 0.25, 0, maxLeft);
    scroll.scrollTo({ top, left, behavior: 'smooth' });
}

function requestBackNavigation() {
    if (!dirty.value) {
        goBackToTemplateList();
        return;
    }
    confirm.require({
        message: 'มีการแก้ไขกรอบลายเซ็นที่ยังไม่ได้บันทึก ต้องการกลับไปหน้ารายการกรอบเริ่มต้นและทิ้งการแก้ไขหรือไม่?',
        header: 'ยังไม่ได้บันทึก',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'อยู่หน้านี้ต่อ',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ออกโดยไม่บันทึก',
            severity: 'danger'
        },
        accept: () => goBackToTemplateList({ discard: true })
    });
}

function goBackToTemplateList(options = {}) {
    if (options.discard) discardNavigationConfirmed = true;
    router.push({ name: 'signature-templates' });
}

function handleBeforeUnload(event) {
    if (!dirty.value) return;
    event.preventDefault();
    event.returnValue = '';
}

function recordDesignerOpen() {
    if (designerOpenRecorded) return;
    designerOpenRecorded = true;
    recordDesignerEvent('designer_open');
}

function recordDesignerEvent(event, extra = {}) {
    const id = template.value?.id;
    if (!id) return;
    const payload = {
        event,
        sessionId: designerSessionId,
        docFormatCode: docFormatCode.value,
        elapsedMs: Date.now() - openedAt,
        boxCount: boxes.value.length,
        validationIssueCount: validationIssues.value.length,
        viewport: {
            width: window.innerWidth || 0,
            height: window.innerHeight || 0
        },
        ...extra
    };
    void api.recordSignatureDesignerEvent(id, payload).catch(() => {});
}
</script>

<template>
    <div class="signature-designer">
        <div class="editor-bar">
            <div class="editor-title">
                <Button icon="pi pi-arrow-left" severity="secondary" text rounded aria-label="กลับ" @click="requestBackNavigation" />
                <div class="min-w-0">
                    <div class="doc-heading">
                        <span>กรอบเริ่มต้น {{ docFormatCode }}</span>
                        <Tag :value="boxProgressLabel" :severity="validationStatusSeverity" />
                        <Tag :value="validationStatusLabel" :severity="validationStatusSeverity" />
                        <Tag v-if="dirty" value="ยังไม่ได้บันทึก" severity="warn" />
                    </div>
                    <div class="doc-subtitle">
                        <span class="truncate">{{ docTitle }}</span>
                        <span>แก้ไขล่าสุด {{ formatDate(template?.updatedAt) }}</span>
                    </div>
                </div>
            </div>

            <div class="editor-actions">
                <input ref="fileInput" type="file" accept="application/pdf,.pdf" class="hidden" @change="handleFileChange" />
                <Button label="อัปโหลด PDF" icon="pi pi-upload" severity="secondary" outlined :loading="uploading" @click="triggerUpload" />
                <Button label="บันทึก" icon="pi pi-save" :disabled="!canSave" :loading="saving" @click="saveTemplate()" />
            </div>
        </div>

        <Message v-if="error" severity="error">{{ error }}</Message>

        <div class="editor-main">
            <section class="pdf-workspace">
                <div class="pdf-toolbar">
                    <div class="toolbar-group">
                        <Select v-model="currentPage" :options="pageOptions" optionLabel="label" optionValue="value" :disabled="pageOptions.length === 0" class="page-select" />
                        <Button icon="pi pi-search-minus" severity="secondary" outlined :disabled="zoom <= 0.6" aria-label="Zoom out" @click="setZoom(zoom - 0.1)" />
                        <span class="zoom-value">{{ Math.round(zoom * 100) }}%</span>
                        <Button icon="pi pi-search-plus" severity="secondary" outlined :disabled="zoom >= 2" aria-label="Zoom in" @click="setZoom(zoom + 0.1)" />
                        <Button label="พอดีกว้าง" icon="pi pi-arrows-h" severity="secondary" outlined :disabled="!pageSize.width" @click="activateFitWidth" />
                        <Button label="100%" severity="secondary" outlined :disabled="!pageSize.width" @click="setZoom(1)" />
                    </div>
                    <span class="pdf-meta">{{ pdfMetaLabel }}</span>
                </div>

                <div v-if="loading && !template?.sampleFileId" class="signature-empty compact">
                    <i class="pi pi-spin pi-spinner text-3xl text-muted-color"></i>
                    <div class="font-semibold mt-3">กำลังโหลดกรอบเริ่มต้น</div>
                </div>

                <div v-else-if="!template?.sampleFileId" class="signature-empty">
                    <i class="pi pi-file-pdf text-4xl text-muted-color"></i>
                    <div class="font-semibold mt-3">อัปโหลด PDF ตัวอย่างก่อน</div>
                    <p class="text-muted-color m-0">ใช้ไฟล์ PDF ของเอกสารจริงเพื่อกำหนดตำแหน่งลายเซ็น</p>
                    <Button label="อัปโหลด PDF" icon="pi pi-upload" class="mt-3" :loading="uploading" @click="triggerUpload" />
                </div>

                <div v-else ref="viewerRef" class="pdf-scroll">
                    <div class="pdf-page-shell">
                        <canvas ref="canvasRef" class="pdf-canvas"></canvas>
                        <div ref="overlayRef" class="pdf-overlay">
                            <div
                                v-for="box in currentPageBoxes"
                                :key="box.clientKey"
                                class="signature-box"
                                :class="{ selected: selectedBoxKey === box.clientKey, readonly: !canEdit }"
                                :style="boxStyle(box)"
                                @pointerdown="startBoxPointer($event, box, 'move')"
                            >
                                <div class="signature-box-label">{{ box.label || box.signerUser || box.positionCode }}</div>
                                <button v-if="canEdit" class="signature-box-delete" type="button" aria-label="ลบกรอบ" @pointerdown.stop @click.stop="deleteBox(box)">
                                    <i class="pi pi-times"></i>
                                </button>
                                <span v-if="canEdit" class="signature-box-handle" @pointerdown="startBoxPointer($event, box, 'resize')"></span>
                            </div>
                        </div>
                    </div>
                </div>
            </section>

            <aside class="inspector">
                <section class="inspector-panel selected-panel">
                    <div class="panel-title">
                        <div>
                            <div class="font-semibold">รายละเอียดกรอบที่เลือก</div>
                            <div v-if="selectedBoxStep" class="text-sm text-muted-color">{{ selectedBoxStep.positionCode }} - {{ selectedBoxStep.positionName }}</div>
                        </div>
                    </div>

                    <div v-if="!selectedBox" class="selected-empty">
                        <i class="pi pi-mouse-pointer text-muted-color"></i>
                        <span>เลือกกรอบจาก PDF หรือจากรายการด้านล่างเพื่อแก้รายละเอียด</span>
                    </div>

                    <div v-else class="selected-form">
                        <div class="field-row full">
                            <label :for="`box-label-${selectedBox.clientKey}`">ข้อความบนกรอบ</label>
                            <InputText
                                :id="`box-label-${selectedBox.clientKey}`"
                                :modelValue="selectedBox.label"
                                maxlength="80"
                                :disabled="!canEdit"
                                @update:modelValue="updateBoxLabel(selectedBox, $event)"
                            />
                        </div>

                        <div class="field-grid">
                            <div class="field-row">
                                <label :for="`box-page-${selectedBox.clientKey}`">หน้า PDF</label>
                                <Select
                                    :id="`box-page-${selectedBox.clientKey}`"
                                    :modelValue="Number(selectedBox.pageNo)"
                                    :options="pageOptions"
                                    optionLabel="label"
                                    optionValue="value"
                                    :disabled="!canEdit || pageOptions.length <= 1"
                                    @update:modelValue="updateBoxPage(selectedBox, $event)"
                                />
                            </div>
                            <div class="field-row">
                                <span>ประเภท</span>
                                <Tag :value="selectedBoxSignerTypeLabel" :severity="conditionSeverity(selectedBoxStep?.conditionType)" class="w-fit" />
                            </div>
                        </div>

                        <div v-if="Number(selectedBoxStep?.conditionType) === 2" class="field-row full">
                            <label :for="`box-user-${selectedBox.clientKey}`">ผู้ลงนาม</label>
                            <Select
                                :id="`box-user-${selectedBox.clientKey}`"
                                :modelValue="selectedBox.signerUser"
                                :options="selectedBoxSignerOptions"
                                optionLabel="label"
                                optionValue="value"
                                :disabled="!canEdit"
                                @update:modelValue="updateBoxSignerUser(selectedBox, $event)"
                            />
                            <small class="text-muted-color">เงื่อนไข “ทุกคน” ต้องมีผู้ใช้งานไม่ซ้ำกันในตำแหน่งเดียวกัน</small>
                        </div>

                        <div class="ratio-grid">
                            <div class="field-row">
                                <label :for="`box-x-${selectedBox.clientKey}`">X (%)</label>
                                <input
                                    :id="`box-x-${selectedBox.clientKey}`"
                                    type="number"
                                    class="p-inputtext p-component w-full"
                                    :value="ratioPercent(selectedBox, 'xRatio')"
                                    min="0"
                                    max="100"
                                    step="0.01"
                                    :disabled="!canEdit"
                                    @input="updateBoxRatio(selectedBox, 'xRatio', $event.target.value)"
                                />
                            </div>
                            <div class="field-row">
                                <label :for="`box-y-${selectedBox.clientKey}`">Y (%)</label>
                                <input
                                    :id="`box-y-${selectedBox.clientKey}`"
                                    type="number"
                                    class="p-inputtext p-component w-full"
                                    :value="ratioPercent(selectedBox, 'yRatio')"
                                    min="0"
                                    max="100"
                                    step="0.01"
                                    :disabled="!canEdit"
                                    @input="updateBoxRatio(selectedBox, 'yRatio', $event.target.value)"
                                />
                            </div>
                            <div class="field-row">
                                <label :for="`box-width-${selectedBox.clientKey}`">กว้าง (%)</label>
                                <input
                                    :id="`box-width-${selectedBox.clientKey}`"
                                    type="number"
                                    class="p-inputtext p-component w-full"
                                    :value="ratioPercent(selectedBox, 'widthRatio')"
                                    min="3"
                                    max="100"
                                    step="0.01"
                                    :disabled="!canEdit"
                                    @input="updateBoxRatio(selectedBox, 'widthRatio', $event.target.value)"
                                />
                            </div>
                            <div class="field-row">
                                <label :for="`box-height-${selectedBox.clientKey}`">สูง (%)</label>
                                <input
                                    :id="`box-height-${selectedBox.clientKey}`"
                                    type="number"
                                    class="p-inputtext p-component w-full"
                                    :value="ratioPercent(selectedBox, 'heightRatio')"
                                    min="3"
                                    max="100"
                                    step="0.01"
                                    :disabled="!canEdit"
                                    @input="updateBoxRatio(selectedBox, 'heightRatio', $event.target.value)"
                                />
                            </div>
                        </div>
                    </div>
                </section>

                <section class="inspector-panel">
                    <div class="panel-title">
                        <div>
                            <div class="font-semibold">ขั้นตอนและกรอบ</div>
                            <div class="text-sm text-muted-color">{{ boxes.length }} กรอบที่ใช้เป็นค่าเริ่มต้น</div>
                        </div>
                        <Tag :value="validationStatusLabel" :severity="validationStatusSeverity" />
                    </div>

                    <Message v-if="!template?.sampleFileId" severity="warn" class="mb-3">ต้องอัปโหลด PDF ก่อนจึงจะเพิ่มกรอบได้</Message>

                    <div class="step-list">
                        <div
                            v-for="step in stepViews"
                            :key="step.id || step.positionCode"
                            class="step-row"
                            :class="{ active: step.isActive, invalid: step.issues.length > 0, complete: step.isComplete }"
                            @click="selectedPositionCode = step.positionCode"
                        >
                            <div class="step-head">
                                <div class="min-w-0">
                                    <div class="step-name">{{ step.positionCode }} - {{ step.positionName }}</div>
                                    <div class="step-users">{{ step.users.join(', ') || 'ไม่มี user' }}</div>
                                </div>
                                <Tag :value="conditionLabel(step.conditionType)" :severity="conditionSeverity(step.conditionType)" />
                            </div>

                            <div class="step-status-row">
                                <span :class="['box-count', { ok: step.isComplete, warn: step.issues.length > 0 }]">{{ step.boxes.length }}/{{ step.required }}</span>
                                <Button
                                    :label="step.canAdd ? 'เพิ่มกรอบ' : step.addDisabledReason || 'เพิ่มกรอบ'"
                                    :icon="step.canAdd ? 'pi pi-plus' : 'pi pi-check'"
                                    size="small"
                                    :disabled="!step.canAdd"
                                    @click.stop="addBox(step)"
                                />
                            </div>

                            <small v-if="step.addDisabledReason" class="text-muted-color">{{ step.addDisabledReason }}</small>

                            <div v-if="step.boxes.length" class="step-box-list">
                                <button
                                    v-for="box in step.boxes"
                                    :key="box.clientKey"
                                    type="button"
                                    class="box-list-item"
                                    :class="{ active: selectedBoxKey === box.clientKey }"
                                    @click.stop="selectBox(box, { scrollIntoView: true })"
                                >
                                    <span>{{ box.label || box.signerUser || box.positionCode }}</span>
                                    <small>หน้า {{ box.pageNo }} / {{ signerTypeShortLabel(box.signerType) }}</small>
                                </button>
                            </div>

                            <div v-if="step.issues.length" class="step-issues">
                                <div v-for="issue in step.issues" :key="`${issue.code}-${issue.message}`" class="issue-line">
                                    <i class="pi pi-exclamation-triangle"></i>
                                    <span>{{ issue.message }}</span>
                                </div>
                            </div>
                        </div>
                    </div>
                </section>

                <section v-if="validationByPosition.get('_global')?.length" class="inspector-panel">
                    <div class="font-semibold mb-3">แจ้งเตือนกรอบเริ่มต้น</div>
                    <div class="step-issues">
                        <div v-for="issue in validationByPosition.get('_global')" :key="`${issue.code}-${issue.message}`" class="issue-line">
                            <i class="pi pi-exclamation-triangle"></i>
                            <span>{{ issue.message }}</span>
                        </div>
                    </div>
                </section>
            </aside>
        </div>
    </div>
</template>

<style scoped>
.signature-designer {
    display: flex;
    min-height: calc(100dvh - 8rem);
    flex-direction: column;
    gap: 0.75rem;
}

.editor-bar {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    min-height: 64px;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 0.65rem 0.85rem;
}

.editor-title {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 0.65rem;
}

.doc-heading {
    display: flex;
    min-width: 0;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.45rem;
    color: var(--text-color);
    font-size: 1rem;
    font-weight: 700;
}

.doc-subtitle {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    color: var(--text-color-secondary);
    font-size: 0.82rem;
}

.editor-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
}

.editor-main {
    display: grid;
    min-height: calc(100dvh - 13rem);
    align-items: start;
    gap: 0.75rem;
    grid-template-columns: minmax(0, 1fr) minmax(380px, 420px);
}

.pdf-workspace,
.inspector-panel {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
}

.pdf-workspace {
    min-width: 0;
    padding: 0.75rem;
}

.pdf-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding-bottom: 0.75rem;
}

.toolbar-group {
    display: flex;
    min-width: 0;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.45rem;
}

.page-select {
    min-width: 8rem;
}

.zoom-value {
    width: 3.4rem;
    text-align: center;
    color: var(--text-color-secondary);
    font-size: 0.875rem;
}

.pdf-meta {
    white-space: nowrap;
    color: var(--text-color-secondary);
    font-size: 0.875rem;
}

.pdf-scroll {
    height: calc(100dvh - 18rem);
    min-height: 34rem;
    overflow: auto;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-ground);
    padding: 0.85rem;
}

.pdf-page-shell {
    position: relative;
    display: inline-block;
    background: white;
    line-height: 0;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.14);
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
    min-width: 32px;
    min-height: 24px;
    border: 2px solid var(--primary-color);
    background: color-mix(in srgb, var(--primary-color) 14%, transparent);
    cursor: move;
    line-height: 1.2;
}

.signature-box.selected {
    border-color: var(--p-orange-500, #f59e0b);
    background: rgba(245, 158, 11, 0.2);
    outline: 2px solid rgba(245, 158, 11, 0.22);
    outline-offset: 2px;
}

.signature-box.readonly {
    cursor: default;
}

.signature-box-label {
    overflow: hidden;
    padding: 0.25rem;
    color: var(--primary-700, #1d4ed8);
    font-size: 0.75rem;
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    display: flex;
    min-height: 32rem;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.25rem;
    border: 1px dashed var(--surface-border);
    border-radius: 8px;
    text-align: center;
}

.signature-empty.compact {
    min-height: 22rem;
}

.inspector {
    display: flex;
    max-height: calc(100dvh - 13rem);
    flex-direction: column;
    gap: 0.75rem;
    overflow: auto;
}

.inspector-panel {
    padding: 0.9rem;
}

.selected-panel {
    position: sticky;
    top: 0;
    z-index: 2;
}

.panel-title {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.75rem;
}

.selected-empty {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: var(--text-color-secondary);
    font-size: 0.9rem;
}

.selected-form,
.step-list,
.step-box-list,
.step-issues {
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
}

.field-grid,
.ratio-grid {
    display: grid;
    gap: 0.65rem;
    grid-template-columns: repeat(2, minmax(0, 1fr));
}

.ratio-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
}

.field-row {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 0.35rem;
    font-size: 0.86rem;
}

.field-row.full {
    grid-column: 1 / -1;
}

.field-row label,
.field-row > span {
    color: var(--text-color);
    font-weight: 600;
}

.step-row {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 0.75rem;
}

.step-row.active {
    border-color: var(--primary-color);
    background: color-mix(in srgb, var(--primary-color) 7%, var(--surface-card));
}

.step-row.invalid {
    border-color: var(--p-orange-300, #fbbf24);
}

.step-row.complete:not(.active) {
    border-color: color-mix(in srgb, var(--green-500, #22c55e) 55%, var(--surface-border));
}

.step-head,
.step-status-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
}

.step-status-row {
    align-items: center;
}

.step-name {
    color: var(--text-color);
    font-weight: 700;
}

.step-users {
    color: var(--text-color-secondary);
    font-size: 0.82rem;
}

.box-count {
    display: inline-flex;
    min-width: 3.25rem;
    align-items: center;
    justify-content: center;
    border-radius: 999px;
    background: var(--surface-ground);
    padding: 0.25rem 0.55rem;
    color: var(--text-color-secondary);
    font-size: 0.82rem;
    font-weight: 700;
}

.box-count.ok {
    background: color-mix(in srgb, var(--green-500, #22c55e) 12%, var(--surface-card));
    color: var(--green-700, #15803d);
}

.box-count.warn {
    background: color-mix(in srgb, var(--orange-500, #f97316) 12%, var(--surface-card));
    color: var(--orange-700, #c2410c);
}

.box-list-item {
    display: flex;
    width: 100%;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-card);
    padding: 0.55rem 0.65rem;
    color: var(--text-color);
    cursor: pointer;
    text-align: left;
}

.box-list-item.active {
    border-color: var(--p-orange-500, #f59e0b);
    background: rgba(245, 158, 11, 0.12);
}

.box-list-item small {
    color: var(--text-color-secondary);
    white-space: nowrap;
}

.issue-line {
    display: flex;
    align-items: flex-start;
    gap: 0.45rem;
    border-radius: 8px;
    background: color-mix(in srgb, var(--orange-500, #f97316) 10%, var(--surface-card));
    padding: 0.5rem 0.6rem;
    color: var(--orange-800, #9a3412);
    font-size: 0.84rem;
}

@media (max-width: 1200px) {
    .editor-main {
        grid-template-columns: 1fr;
    }

    .inspector {
        max-height: none;
        overflow: visible;
    }

    .selected-panel {
        position: static;
    }
}

@media (max-width: 768px) {
    .editor-bar,
    .pdf-toolbar {
        align-items: stretch;
        flex-direction: column;
    }

    .editor-actions,
    .toolbar-group {
        justify-content: flex-start;
    }

    .pdf-scroll {
        height: 70vh;
        min-height: 28rem;
    }

    .field-grid,
    .ratio-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}

@media (max-width: 520px) {
    .ratio-grid {
        grid-template-columns: 1fr;
    }
}
</style>
