<script setup>
import { api } from '@/services/api';
import { LEGAL_NOTICE_DISPLAY_TEXT, LEGAL_NOTICE_TEXT, legalNoticeOverflowMessage, legalNoticePreviewFontSize } from '@/utils/legalNoticePreview';
import * as pdfjsLib from 'pdfjs-dist';
import pdfWorker from 'pdfjs-dist/build/pdf.worker.mjs?url';
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { useConfirm } from 'primevue/useconfirm';

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;
const confirm = useConfirm();

const props = defineProps({
    pdfUrl: { type: String, default: '' },
    pageCount: { type: Number, default: 0 },
    configs: { type: Array, default: () => [] },
    modelValue: { type: Array, default: () => [] },
    signNoteBoxes: { type: Array, default: () => [] },
    legalNoticeBox: { type: Object, default: null },
    legalNoticeBoxes: { type: Array, default: () => [] },
    presetTemplate: { type: Object, default: null },
    fullHeight: { type: Boolean, default: false },
    readOnly: { type: Boolean, default: false },
    allowSignNoteBoxes: { type: Boolean, default: false }
});

const emit = defineEmits(['update:modelValue', 'update:signNoteBoxes', 'update:legalNoticeBox', 'update:legalNoticeBoxes', 'apply-preset', 'event', 'validation-change']);

const canvasRef = ref(null);
const viewportRef = ref(null);
const pdfDoc = shallowRef(null);
const renderTask = shallowRef(null);
const renderSequence = ref(0);
const rendering = ref(false);
const currentPage = ref(1);
const zoom = ref(1);
const fitWidthActive = ref(true);
const renderedSize = ref({ width: 0, height: 0 });
const selectedBoxKey = ref('');
let resizeObserver;
let dragState = null;
let fitTimer;
const legalNoticeKeyPrefix = 'legal_notice_box_';
const signNoteKeyPrefix = 'sign_note_box_';
const legalNoticeText = LEGAL_NOTICE_TEXT;
const legalNoticePreviewText = LEGAL_NOTICE_DISPLAY_TEXT;
const legalNoticeOverflowKeys = ref(new Set());
const legalNoticePreviewRefs = new Map();

const boxes = computed(() => props.modelValue || []);
const currentPageBoxes = computed(() => boxes.value.filter((box) => Number(box.pageNo || 1) === currentPage.value));
const signNoteBoxes = computed(() => (props.allowSignNoteBoxes ? props.signNoteBoxes || [] : []).map((box) => withSignNoteKey(box)));
const currentPageSignNoteBoxes = computed(() => signNoteBoxes.value.filter((box) => Number(box.pageNo || 1) === currentPage.value).map(signNoteOverlayBox));
const legalNotices = computed(() => {
    const boxes = Array.isArray(props.legalNoticeBoxes) && props.legalNoticeBoxes.length ? props.legalNoticeBoxes : props.legalNoticeBox ? [props.legalNoticeBox] : [];
    return boxes.map((box) => withLegalNoticeKey(box));
});
const currentPageLegalNotices = computed(() => legalNotices.value.filter((box) => Number(box.pageNo || 1) === currentPage.value).map(legalOverlayBox));
const selectedBox = computed(() => boxes.value.find((box) => box.clientKey === selectedBoxKey.value) || null);
const selectedSignNote = computed(() => {
    const box = signNoteBoxes.value.find((item) => item.clientKey === selectedBoxKey.value);
    return box ? signNoteOverlayBox(box) : null;
});
const selectedLegalNotice = computed(() => {
    const box = legalNotices.value.find((item) => item.clientKey === selectedBoxKey.value);
    return box ? legalOverlayBox(box) : null;
});
const selectedItem = computed(() => selectedLegalNotice.value || selectedSignNote.value || selectedBox.value);
const selectedIsLegalNotice = computed(() => !!selectedLegalNotice.value);
const selectedIsSignNote = computed(() => !!selectedSignNote.value);
const selectedStep = computed(() => props.configs.find((step) => step.positionCode === (selectedSignNote.value || selectedBox.value)?.positionCode) || null);
const totalBoxes = computed(() => boxes.value.length);

const validationIssues = computed(() => {
    const issues = [];
    if (!props.pdfUrl) issues.push('อัปโหลด PDF ก่อน');
    if (boxes.value.length === 0) issues.push('ต้องวางกรอบอย่างน้อย 1 กรอบ');
    if (legalNotices.value.length === 0) issues.push('ต้องวางกรอบข้อความกฎหมาย');
    for (const box of boxes.value) {
        if (box.xRatio < 0 || box.yRatio < 0 || box.widthRatio <= 0 || box.heightRatio <= 0 || box.xRatio + box.widthRatio > 1 || box.yRatio + box.heightRatio > 1) {
            issues.push(`กรอบ ${box.label || box.positionCode} อยู่นอกหน้า PDF`);
        }
        if (box.pageNo < 1 || box.pageNo > props.pageCount) issues.push(`กรอบ ${box.label || box.positionCode} อยู่หน้าที่ไม่ถูกต้อง`);
    }
    for (const box of signNoteBoxes.value) {
        if (box.xRatio < 0 || box.yRatio < 0 || box.widthRatio <= 0 || box.heightRatio <= 0 || box.xRatio + box.widthRatio > 1 || box.yRatio + box.heightRatio > 1) {
            issues.push(`กรอบหมายเหตุ ${box.label || box.positionCode} อยู่นอกหน้า PDF`);
        }
        if (box.pageNo < 1 || box.pageNo > props.pageCount) issues.push(`กรอบหมายเหตุ ${box.label || box.positionCode} อยู่หน้าที่ไม่ถูกต้อง`);
    }
    for (const box of legalNotices.value) {
        if (box.pageNo < 1 || box.pageNo > props.pageCount) issues.push('กรอบข้อความกฎหมายอยู่หน้าที่ไม่ถูกต้อง');
        if (box.xRatio < 0 || box.yRatio < 0 || box.widthRatio <= 0 || box.heightRatio <= 0 || box.xRatio + box.widthRatio > 1 || box.yRatio + box.heightRatio > 1) {
            issues.push('กรอบข้อความกฎหมายอยู่นอกหน้า PDF');
        }
        if (box.widthRatio < 0.2 || box.heightRatio < 0.035) issues.push('กรอบข้อความกฎหมายเล็กเกินไป');
        if (legalNoticeOverflowKeys.value.has(box.clientKey)) issues.push(legalNoticeOverflowMessage());
    }
    for (const step of props.configs) {
        const stepBoxes = boxes.value.filter((box) => box.positionCode === step.positionCode);
        if (stepBoxes.length === 0) continue;
        if (step.conditionType === 1 && stepBoxes.length < 1) issues.push(`${step.positionName} ต้องมีอย่างน้อย 1 กรอบ`);
        if (step.conditionType === 3 && stepBoxes.length < 1) issues.push(`${step.positionName} ต้องมีอย่างน้อย 1 กรอบบุคคลภายนอก`);
        if (step.conditionType === 2) {
            const required = new Set(stepUsers(step).map((user) => signerUsername(user)).filter(Boolean));
            const seen = new Set();
            for (const box of stepBoxes) {
                const user = signerUsername(box.signerUser);
                if (!user) issues.push(`${step.positionName} ต้องเลือก user ทุกกรอบ`);
                if (user && !required.has(user)) issues.push(`${step.positionName} มี user ที่ไม่อยู่ใน Workflow`);
                if (user) seen.add(user);
            }
            required.forEach((user) => {
                if (!seen.has(user)) issues.push(`${step.positionName} ต้องมีกรอบของ ${user}`);
            });
            for (const box of signNoteBoxes.value.filter((item) => item.positionCode === step.positionCode)) {
                const user = signerUsername(box.signerUser);
                if (!user) issues.push(`กรอบหมายเหตุ ${step.positionName} ต้องเลือก user`);
                if (user && !required.has(user)) issues.push(`กรอบหมายเหตุ ${step.positionName} มี user ที่ไม่อยู่ใน Workflow`);
            }
        }
    }
    return [...new Set(issues)];
});

const canApplyPreset = computed(() => !props.readOnly && (!!props.presetTemplate?.boxes?.length || !!props.presetTemplate?.legalNoticeBox) && !!props.pdfUrl);

const stepRows = computed(() =>
    [...props.configs]
        .sort((a, b) => Number(a.sequenceNo || 0) - Number(b.sequenceNo || 0) || String(a.positionCode).localeCompare(String(b.positionCode)))
        .map((step) => {
            const stepBoxes = boxes.value.filter((box) => box.positionCode === step.positionCode);
            const users = stepUsers(step);
            let canAdd = !!props.pdfUrl && !props.readOnly;
            let addReason = '';
            if (!props.pdfUrl) addReason = 'ต้องอัปโหลด PDF ก่อน';
            if (props.readOnly) addReason = 'ใช้กรอบจาก template เท่านั้น';
            if (step.conditionType === 2 && users.length === 0) {
                canAdd = false;
                addReason = 'ยังไม่มี user ใน Workflow';
            }
            return {
                ...step,
                boxes: stepBoxes,
                noteBoxes: props.allowSignNoteBoxes ? signNoteBoxes.value.filter((box) => box.positionCode === step.positionCode) : [],
                users,
                canAdd,
                canAddNote: props.allowSignNoteBoxes && !!props.pdfUrl && !props.readOnly && !(step.conditionType === 2 && users.length === 0),
                addReason,
                statusLabel: stepBoxes.length > 0 ? `ใช้ ${stepBoxes.length} กรอบ / ${countPagesWithBoxes(stepBoxes)} หน้า` : 'ไม่อยู่ในงานเซ็น'
            };
        })
);

watch(
    () => props.pdfUrl,
    async () => {
        await loadPDF();
    },
    { immediate: true }
);

watch(currentPage, renderPage);
watch(zoom, renderPage);
watch(validationIssues, (issues) => emit('validation-change', issues), { immediate: true });
watch([currentPageLegalNotices, renderedSize, zoom, selectedBoxKey], () => checkLegalNoticeOverflow(), { deep: true, immediate: true });

onBeforeUnmount(async () => {
    clearTimeout(fitTimer);
    cancelRenderTask();
    if (resizeObserver) resizeObserver.disconnect();
    removePointerListeners();
    if (pdfDoc.value?.destroy) await pdfDoc.value.destroy().catch(() => {});
});

async function loadPDF() {
    cancelRenderTask();
    if (pdfDoc.value?.destroy) await pdfDoc.value.destroy().catch(() => {});
    pdfDoc.value = null;
    renderedSize.value = { width: 0, height: 0 };
    currentPage.value = 1;
    if (!props.pdfUrl) return;
    rendering.value = true;
    try {
        const loadingTask = pdfjsLib.getDocument({ url: props.pdfUrl, httpHeaders: api.authHeaders() });
        pdfDoc.value = await loadingTask.promise;
        await nextTick();
        observeResize();
        if (fitWidthActive.value) await fitWidth(false);
        await renderPage();
        emit('event', 'pdf_upload_success');
    } catch (err) {
        emit('event', 'pdf_upload_error');
    } finally {
        rendering.value = false;
    }
}

async function renderPage() {
    if (!pdfDoc.value || !canvasRef.value) return;
    const sequence = ++renderSequence.value;
    cancelRenderTask();
    rendering.value = true;
    try {
        const page = await pdfDoc.value.getPage(currentPage.value);
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        const displayViewport = page.getViewport({ scale: zoom.value });
        const renderViewport = page.getViewport({ scale: zoom.value * dpr });
        const canvas = canvasRef.value;
        canvas.width = Math.floor(renderViewport.width);
        canvas.height = Math.floor(renderViewport.height);
        canvas.style.width = `${displayViewport.width}px`;
        canvas.style.height = `${displayViewport.height}px`;
        renderedSize.value = { width: displayViewport.width, height: displayViewport.height };
        renderTask.value = page.render({ canvasContext: canvas.getContext('2d'), viewport: renderViewport });
        await renderTask.value.promise;
        if (sequence !== renderSequence.value) return;
    } catch (err) {
        if (err?.name !== 'RenderingCancelledException') emit('event', 'pdf_render_error');
    } finally {
        if (sequence === renderSequence.value) rendering.value = false;
    }
}

function cancelRenderTask() {
    if (renderTask.value?.cancel) renderTask.value.cancel();
    renderTask.value = null;
}

function observeResize() {
    if (resizeObserver || !viewportRef.value) return;
    resizeObserver = new ResizeObserver(() => {
        if (!fitWidthActive.value) return;
        clearTimeout(fitTimer);
        fitTimer = setTimeout(() => fitWidth(false), 120);
    });
    resizeObserver.observe(viewportRef.value);
}

async function fitWidth(render = true) {
    if (!pdfDoc.value || !viewportRef.value) return;
    const page = await pdfDoc.value.getPage(currentPage.value);
    const base = page.getViewport({ scale: 1 });
    const available = Math.max(280, viewportRef.value.clientWidth - 32);
    fitWidthActive.value = true;
    zoom.value = clamp(available / base.width, 0.35, 2.5);
    if (render) await renderPage();
}

function setZoom(value) {
    fitWidthActive.value = false;
    zoom.value = clamp(value, 0.35, 2.5);
}

function applyPreset() {
    if (props.readOnly || !canApplyPreset.value) return;
    const hasExisting = boxes.value.length > 0 || legalNotices.value.length > 0 || (props.allowSignNoteBoxes && signNoteBoxes.value.length > 0);
    if (hasExisting) {
        confirm.require({
            message: 'การใช้กรอบเริ่มต้นจะล้างกรอบที่วางอยู่ทั้งหมดและสร้างใหม่ตาม PDF ทุกหน้า ต้องการดำเนินการต่อหรือไม่?',
            header: 'ใช้กรอบเริ่มต้น',
            icon: 'pi pi-exclamation-triangle',
            rejectProps: { label: 'ยกเลิก', severity: 'secondary', outlined: true },
            acceptProps: { label: 'ใช้กรอบเริ่มต้น', severity: 'warn' },
            accept: () => applyPresetNow()
        });
        return;
    }
    applyPresetNow();
}

function applyPresetNow() {
    const next = expandPresetBoxes(props.presetTemplate?.boxes || [], props.presetTemplate?.sampleFile?.pageCount, props.pageCount).map((box) => ({
        ...box,
        clientKey: makeKey(),
        pageNo: Number(box.pageNo || 1),
        xRatio: Number(box.xRatio || 0.1),
        yRatio: Number(box.yRatio || 0.1),
        widthRatio: Number(box.widthRatio || 0.2),
        heightRatio: Number(box.heightRatio || 0.08)
    }));
    emitBoxes(next);
    const signNoteNext = props.allowSignNoteBoxes
        ? expandPresetBoxes(props.presetTemplate?.signNoteBoxes || [], props.presetTemplate?.sampleFile?.pageCount, props.pageCount).map((box) => ({
              ...box,
              clientKey: makeSignNoteKey(),
              pageNo: Number(box.pageNo || 1),
              xRatio: Number(box.xRatio || 0.55),
              yRatio: Number(box.yRatio || 0.72),
              widthRatio: Number(box.widthRatio || 0.25),
              heightRatio: Number(box.heightRatio || 0.06),
              label: box.label || 'หมายเหตุผู้เซ็น'
          }))
        : [];
    emitSignNoteBoxes(signNoteNext);
    const noticePattern = props.presetTemplate?.legalNoticeBox ? [props.presetTemplate.legalNoticeBox] : [];
    const legalNext = expandPresetBoxes(noticePattern, props.presetTemplate?.sampleFile?.pageCount, props.pageCount).map((box) => ({
        ...box,
        clientKey: makeLegalNoticeKey(),
        pageNo: Number(box.pageNo || 1),
        xRatio: Number(box.xRatio || 0.2),
        yRatio: Number(box.yRatio || 0.62),
        widthRatio: Number(box.widthRatio || 0.6),
        heightRatio: Number(box.heightRatio || 0.06),
        label: box.label || 'ข้อความกฎหมาย',
        source: 'preset'
    }));
    emitLegalNoticeBoxes(legalNext);
    selectedBoxKey.value = legalNext[0]?.clientKey || signNoteNext[0]?.clientKey || next[0]?.clientKey || '';
    emit('apply-preset', props.presetTemplate);
    emit('event', 'preset_applied');
}

function addLegalNoticeBox() {
    if (props.readOnly || !props.pdfUrl) return;
    const box = {
        clientKey: makeLegalNoticeKey(),
        pageNo: currentPage.value,
        xRatio: 0.2,
        yRatio: 0.62,
        widthRatio: 0.6,
        heightRatio: 0.065,
        label: 'ข้อความกฎหมาย',
        source: 'per_document'
    };
    emitLegalNoticeBoxes([...legalNotices.value, box]);
    selectedBoxKey.value = box.clientKey;
    emit('event', 'legal_notice_box_add');
}

function deleteLegalNoticeBox(box = selectedLegalNotice.value) {
    if (props.readOnly || !box) return;
    emitLegalNoticeBoxes(legalNotices.value.filter((item) => item.clientKey !== box.clientKey));
    if (selectedBoxKey.value === box.clientKey) selectedBoxKey.value = '';
    emit('event', 'legal_notice_box_delete');
}

function addBox(step) {
    if (props.readOnly || !props.pdfUrl) return;
    const users = stepUsers(step);
    const existing = boxes.value.filter((box) => box.positionCode === step.positionCode);
    let signerType = 'any';
    let signerUser = '';
    let signerSlot = nextSignerSlot(existing);
    if (step.conditionType === 2) {
        signerType = 'internal';
        const usedOnCurrentPage = new Set(existing.filter((box) => Number(box.pageNo || 1) === currentPage.value).map((box) => signerUsername(box.signerUser)));
        signerUser = users.find((user) => !usedOnCurrentPage.has(signerUsername(user))) || users[0] || '';
        if (!signerUser) return;
        signerSlot = signerSlotForUser(users, signerUser);
    } else if (step.conditionType === 3) {
        signerType = 'external';
        signerSlot = 1;
    } else {
        signerSlot = 1;
    }
    const box = {
        clientKey: makeKey(),
        positionCode: step.positionCode,
        signerSlot,
        signerType,
        signerUser,
        pageNo: currentPage.value,
        xRatio: 0.34,
        yRatio: 0.68,
        widthRatio: 0.22,
        heightRatio: 0.08,
        label: step.positionName
    };
    emitBoxes([...boxes.value, box]);
    selectedBoxKey.value = box.clientKey;
    emit('event', 'box_add');
}

function addSignNoteBox(step) {
    if (props.readOnly || !props.pdfUrl) return;
    const users = stepUsers(step);
    const existing = signNoteBoxes.value.filter((box) => box.positionCode === step.positionCode);
    let signerType = 'any';
    let signerUser = '';
    let signerSlot = nextSignerSlot(existing);
    if (step.conditionType === 2) {
        signerType = 'internal';
        const usedOnCurrentPage = new Set(existing.filter((box) => Number(box.pageNo || 1) === currentPage.value).map((box) => signerUsername(box.signerUser)));
        signerUser = users.find((user) => !usedOnCurrentPage.has(signerUsername(user))) || users[0] || '';
        if (!signerUser) return;
        signerSlot = signerSlotForUser(users, signerUser);
    } else if (step.conditionType === 3) {
        signerType = 'external';
        signerSlot = 1;
    } else {
        signerSlot = 1;
    }
    const box = {
        clientKey: makeSignNoteKey(),
        positionCode: step.positionCode,
        signerSlot,
        signerType,
        signerUser,
        pageNo: currentPage.value,
        xRatio: 0.55,
        yRatio: 0.72,
        widthRatio: 0.25,
        heightRatio: 0.06,
        label: 'หมายเหตุผู้เซ็น'
    };
    emitSignNoteBoxes([...signNoteBoxes.value, box]);
    selectedBoxKey.value = box.clientKey;
    emit('event', 'sign_note_box_add');
}

function deleteSignNoteBox(box) {
    if (props.readOnly) return;
    emitSignNoteBoxes(signNoteBoxes.value.filter((item) => item.clientKey !== box.clientKey));
    if (selectedBoxKey.value === box.clientKey) selectedBoxKey.value = '';
    emit('event', 'sign_note_box_delete');
}

function deleteBox(box) {
    if (props.readOnly) return;
    emitBoxes(boxes.value.filter((item) => item.clientKey !== box.clientKey));
    if (selectedBoxKey.value === box.clientKey) selectedBoxKey.value = '';
    emit('event', 'box_delete');
}

function updateSelected(field, value) {
    if (props.readOnly || !selectedItem.value) return;
    updateBox(selectedItem.value.clientKey, { [field]: value });
}

function updateBox(key, patch) {
    if (props.readOnly) return;
    if (isLegalNoticeKey(key)) {
        updateLegalNoticeBox(key, patch);
        return;
    }
    if (isSignNoteKey(key)) {
        updateSignNoteBox(key, patch);
        return;
    }
    emitBoxes(boxes.value.map((box) => (box.clientKey === key ? { ...box, ...patch } : box)));
}

function emitBoxes(next) {
    emit('update:modelValue', next);
}

function emitSignNoteBoxes(next) {
    emit('update:signNoteBoxes', (next || []).map((box) => withSignNoteKey(box)));
}

function emitLegalNoticeBox(next) {
    emit('update:legalNoticeBox', next);
}

function emitLegalNoticeBoxes(next) {
    const normalized = (next || []).map((box) => withLegalNoticeKey(box));
    emit('update:legalNoticeBoxes', normalized);
    emitLegalNoticeBox(normalized[0] || null);
}

function updateLegalNoticeBox(key, patch) {
    if (!isLegalNoticeKey(key)) return;
    emitLegalNoticeBoxes(legalNotices.value.map((box) => (box.clientKey === key ? { ...box, ...patch } : box)));
}

function updateSignNoteBox(key, patch) {
    if (!isSignNoteKey(key)) return;
    emitSignNoteBoxes(signNoteBoxes.value.map((box) => (box.clientKey === key ? { ...box, ...patch } : box)));
}

function selectBox(box) {
    selectedBoxKey.value = box.clientKey;
    if (box.pageNo !== currentPage.value) currentPage.value = Number(box.pageNo || 1);
}

function selectSignNoteBox(box = selectedSignNote.value || signNoteBoxes.value[0]) {
    if (!box) return;
    selectedBoxKey.value = box.clientKey;
    if (box.pageNo !== currentPage.value) currentPage.value = Number(box.pageNo || 1);
}

function selectLegalNoticeBox(box = selectedLegalNotice.value || legalNotices.value[0]) {
    if (!box) return;
    selectedBoxKey.value = box.clientKey;
    if (box.pageNo !== currentPage.value) currentPage.value = Number(box.pageNo || 1);
}

function signNoteOverlayBox(box) {
    return {
        ...box,
        clientKey: box.clientKey || makeSignNoteKey(),
        label: box.label || 'หมายเหตุผู้เซ็น',
        boxType: 'sign_note'
    };
}

function legalOverlayBox(box) {
    return {
        ...box,
        clientKey: box.clientKey || makeLegalNoticeKey(),
        label: box.label || 'ข้อความกฎหมาย',
        boxType: 'legal_notice'
    };
}

function boxStyle(box) {
    return {
        left: `${box.xRatio * renderedSize.value.width}px`,
        top: `${box.yRatio * renderedSize.value.height}px`,
        width: `${box.widthRatio * renderedSize.value.width}px`,
        height: `${box.heightRatio * renderedSize.value.height}px`
    };
}

function legalNoticeStyle(box) {
    return {
        ...boxStyle(box),
        '--legal-preview-font-size': `${legalNoticePreviewFontSize(zoom.value)}px`
    };
}

async function checkLegalNoticeOverflow() {
    await nextTick();
    const next = new Set();
    for (const box of currentPageLegalNotices.value) {
        const element = legalNoticePreviewRefs.get(box.clientKey);
        if (!element) continue;
        if (element.scrollWidth > element.clientWidth + 1 || element.scrollHeight > element.clientHeight + 1) {
            next.add(box.clientKey);
        }
    }
    legalNoticeOverflowKeys.value = next;
}

function setLegalNoticePreviewRef(key, element) {
    if (!key) return;
    if (element) legalNoticePreviewRefs.set(key, element);
    else legalNoticePreviewRefs.delete(key);
}

function isLegalNoticeOverflow(key) {
    return legalNoticeOverflowKeys.value.has(key);
}

function startPointer(event, box, mode) {
    if (props.readOnly) return;
    if (!renderedSize.value.width || !renderedSize.value.height) return;
    if (box.boxType === 'legal_notice') selectLegalNoticeBox(box);
    else if (box.boxType === 'sign_note') selectSignNoteBox(box);
    else selectBox(box);
    dragState = {
        mode,
        key: box.clientKey,
        startX: event.clientX,
        startY: event.clientY,
        box: { ...box }
    };
    window.addEventListener('pointermove', movePointer);
    window.addEventListener('pointerup', stopPointer, { once: true });
    event.preventDefault();
}

function movePointer(event) {
    if (!dragState) return;
    const dx = (event.clientX - dragState.startX) / renderedSize.value.width;
    const dy = (event.clientY - dragState.startY) / renderedSize.value.height;
    const start = dragState.box;
    if (dragState.mode === 'move') {
        updateBox(dragState.key, {
            xRatio: clamp(start.xRatio + dx, 0, 1 - start.widthRatio),
            yRatio: clamp(start.yRatio + dy, 0, 1 - start.heightRatio)
        });
    } else {
        const minWidth = 0.05;
        const minHeight = 0.035;
        updateBox(dragState.key, {
            widthRatio: clamp(start.widthRatio + dx, minWidth, 1 - start.xRatio),
            heightRatio: clamp(start.heightRatio + dy, minHeight, 1 - start.yRatio)
        });
    }
}

function stopPointer() {
    removePointerListeners();
    dragState = null;
}

function removePointerListeners() {
    window.removeEventListener('pointermove', movePointer);
}

function stepUsers(step) {
    return [step.user01, step.user02, step.user03].map((value) => String(value || '').trim()).filter(Boolean);
}

function signerSlotForUser(users, value) {
    const username = signerUsername(value);
    const index = users.findIndex((user) => signerUsername(user) === username);
    return index >= 0 ? index + 1 : 1;
}

function signerUsername(value) {
    return String(value || '').split(':')[0].trim().toLowerCase();
}

function nextSignerSlot(existing) {
    const used = new Set(existing.map((box) => Number(box.signerSlot || 0)));
    let slot = 1;
    while (used.has(slot)) slot += 1;
    return slot;
}

function signerLabel(value) {
    const [code, name] = String(value || '').split(':');
    return name ? `${code.trim()}: ${name.trim()}` : String(value || '').trim();
}

function conditionLabel(value) {
    if (value === 1) return 'คนใดคนหนึ่ง';
    if (value === 2) return 'ทุกคน';
    if (value === 3) return 'บุคคลภายนอก';
    return `เงื่อนไข ${value}`;
}

function expandPresetBoxes(sourceBoxes, samplePageCount, targetPageCount) {
    const pages = Math.max(1, Number(targetPageCount || 1));
    const samplePages = Number(samplePageCount || 0);
    const mismatch = samplePages > 0 && pages > 0 && samplePages !== pages;
    if (!mismatch) {
        return (sourceBoxes || []).map((box) => ({ ...box, pageNo: clampPage(box.pageNo, pages) }));
    }
    const pattern = (sourceBoxes || []).filter((box) => Number(box.pageNo || 1) === 1);
    const base = pattern.length ? pattern : sourceBoxes || [];
    const out = [];
    for (let pageNo = 1; pageNo <= pages; pageNo += 1) {
        base.forEach((box) => out.push({ ...box, pageNo }));
    }
    return out;
}

function countPagesWithBoxes(items) {
    return new Set((items || []).map((box) => Number(box.pageNo || 1))).size;
}

function clampPage(value, pageCount) {
    return Math.min(Math.max(Number(value || 1), 1), Math.max(1, Number(pageCount || 1)));
}

function withLegalNoticeKey(box) {
    if (!box) return { clientKey: makeLegalNoticeKey(), pageNo: 1 };
    return {
        ...box,
        clientKey: box?.clientKey || makeLegalNoticeKey()
    };
}

function withSignNoteKey(box) {
    if (!box) return { clientKey: makeSignNoteKey(), pageNo: 1 };
    const currentKey = String(box?.clientKey || '');
    return {
        ...box,
        clientKey: isSignNoteKey(currentKey) ? currentKey : makeSignNoteKey(),
        label: box?.label || 'หมายเหตุผู้เซ็น'
    };
}

function isLegalNoticeKey(key) {
    const value = String(key || '');
    return value.startsWith(legalNoticeKeyPrefix) || value.startsWith('legal_');
}

function isSignNoteKey(key) {
    const value = String(key || '');
    return value.startsWith(signNoteKeyPrefix) || value.startsWith('sign_note_');
}

function makeKey() {
    return crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
}

function makeLegalNoticeKey() {
    return `${legalNoticeKeyPrefix}${makeKey()}`;
}

function makeSignNoteKey() {
    return `${signNoteKeyPrefix}${makeKey()}`;
}

function clamp(value, min, max) {
    return Math.min(max, Math.max(min, Number(value) || 0));
}

defineExpose({ validationIssues, totalBoxes });
</script>

<template>
    <div class="layout-designer" :class="{ 'layout-designer-full': fullHeight, 'layout-designer-readonly': props.readOnly }">
        <div class="pdf-pane">
            <div class="layout-toolbar">
                <div class="toolbar-group">
                    <Button icon="pi pi-angle-left" severity="secondary" text :disabled="currentPage <= 1" @click="currentPage--" />
                    <span class="page-label">หน้า {{ currentPage }} / {{ props.pageCount || pdfDoc?.numPages || 0 }}</span>
                    <Button icon="pi pi-angle-right" severity="secondary" text :disabled="currentPage >= (props.pageCount || pdfDoc?.numPages || 1)" @click="currentPage++" />
                </div>
                <div class="toolbar-group">
                    <Button label="พอดีกว้าง" severity="secondary" outlined size="small" @click="fitWidth()" />
                    <Button label="100%" severity="secondary" outlined size="small" @click="setZoom(1)" />
                    <Button icon="pi pi-minus" severity="secondary" text @click="setZoom(zoom - 0.1)" />
                    <span class="zoom-label">{{ Math.round(zoom * 100) }}%</span>
                    <Button icon="pi pi-plus" severity="secondary" text @click="setZoom(zoom + 0.1)" />
                </div>
            </div>

            <div ref="viewportRef" class="pdf-viewport">
                <div v-if="!pdfUrl" class="pdf-empty">อัปโหลด PDF เพื่อเริ่มวางกรอบลายเซ็น</div>
                <div v-else class="pdf-page-shell" :class="{ rendering }">
                    <canvas ref="canvasRef"></canvas>
                    <button
                        v-for="legalBox in currentPageLegalNotices"
                        :key="legalBox.clientKey"
                        type="button"
                        class="signature-layout-box legal-notice-layout-box"
                        :class="{ selected: legalBox.clientKey === selectedBoxKey, overflow: isLegalNoticeOverflow(legalBox.clientKey) }"
                        :style="legalNoticeStyle(legalBox)"
                        @click.stop="selectLegalNoticeBox(legalBox)"
                        @pointerdown.stop="startPointer($event, legalBox, 'move')"
                    >
                        <span :ref="(el) => setLegalNoticePreviewRef(legalBox.clientKey, el)" class="legal-notice-preview-text">{{ legalNoticePreviewText }}</span>
                        <i v-if="!props.readOnly" class="pi pi-trash" @pointerdown.stop @click.stop="deleteLegalNoticeBox(legalBox)"></i>
                        <b v-if="!props.readOnly" @pointerdown.stop="startPointer($event, legalBox, 'resize')"></b>
                    </button>
                    <template v-if="props.allowSignNoteBoxes">
                        <button
                            v-for="noteBox in currentPageSignNoteBoxes"
                            :key="noteBox.clientKey"
                            type="button"
                            class="signature-layout-box sign-note-layout-box"
                            :class="{ selected: noteBox.clientKey === selectedBoxKey }"
                            :style="boxStyle(noteBox)"
                            @click.stop="selectSignNoteBox(noteBox)"
                            @pointerdown.stop="startPointer($event, noteBox, 'move')"
                        >
                            <span class="signature-layout-label">{{ noteBox.label || 'หมายเหตุผู้เซ็น' }}</span>
                            <i v-if="!props.readOnly" class="pi pi-trash" @pointerdown.stop @click.stop="deleteSignNoteBox(noteBox)"></i>
                            <b v-if="!props.readOnly" @pointerdown.stop="startPointer($event, noteBox, 'resize')"></b>
                        </button>
                    </template>
                    <button
                        v-for="box in currentPageBoxes"
                        :key="box.clientKey"
                        type="button"
                        class="signature-layout-box"
                        :class="{ selected: box.clientKey === selectedBoxKey }"
                        :style="boxStyle(box)"
                        @click.stop="selectBox(box)"
                        @pointerdown.stop="startPointer($event, box, 'move')"
                    >
                        <span class="signature-layout-label">{{ box.label || box.signerUser || box.positionCode }}</span>
                        <i v-if="!props.readOnly" class="pi pi-trash" @pointerdown.stop @click.stop="deleteBox(box)"></i>
                        <b v-if="!props.readOnly" @pointerdown.stop="startPointer($event, box, 'resize')"></b>
                    </button>
                </div>
            </div>
        </div>

        <aside class="layout-inspector">
            <div class="inspector-section">
                <div class="section-title">กรอบที่เลือก</div>
                <div v-if="!selectedItem" class="empty-hint">เลือกกรอบจาก PDF หรือเพิ่มกรอบจากขั้นตอนด้านล่าง</div>
                <div v-else class="selected-form">
                    <label>ข้อความบนกรอบ</label>
                    <InputText :modelValue="selectedItem.label" :disabled="selectedIsLegalNotice || selectedIsSignNote || props.readOnly" @update:modelValue="updateSelected('label', $event)" />
                    <small v-if="selectedIsLegalNotice" class="text-muted-color">{{ legalNoticeText }}</small>
                    <small v-else-if="props.allowSignNoteBoxes && selectedIsSignNote" class="text-muted-color">ข้อความหมายเหตุจริงจะมาจากผู้เซ็นตอนยืนยันเอกสาร</small>
                    <label>หน้า</label>
                    <InputNumber :modelValue="selectedItem.pageNo" :min="1" :max="props.pageCount || 1" showButtons :disabled="props.readOnly" @update:modelValue="updateSelected('pageNo', $event || 1)" />
                    <label v-if="!selectedIsLegalNotice && selectedStep?.conditionType === 2">User ผู้เซ็น</label>
                    <Select
                        v-if="!selectedIsLegalNotice && selectedStep?.conditionType === 2"
                        :modelValue="selectedItem.signerUser"
                        :options="stepUsers(selectedStep).map((user) => ({ label: signerLabel(user), value: user }))"
                        optionLabel="label"
                        optionValue="value"
                        :disabled="props.readOnly"
                        @update:modelValue="updateSelected('signerUser', $event)"
                    />
                </div>
            </div>

            <div class="inspector-section">
                <div class="section-heading">
                    <div>
                        <div class="section-title">ข้อความกฎหมาย</div>
                        <small>{{ legalNotices.length ? `${legalNotices.length} กรอบ / ${countPagesWithBoxes(legalNotices)} หน้า` : 'ต้องวางก่อนส่งเซ็น' }}</small>
                    </div>
                    <Button label="เพิ่มกรอบ" icon="pi pi-plus" size="small" :disabled="!props.pdfUrl || props.readOnly" @click="addLegalNoticeBox" />
                </div>
                <Message v-if="!legalNotices.length" severity="warn" class="mb-3">ต้องวางกรอบข้อความกฎหมายบน PDF ก่อนส่งเซ็น</Message>
                <Message v-else-if="legalNoticeOverflowKeys.size" severity="warn" class="mb-3">{{ legalNoticeOverflowMessage() }}</Message>
                <div v-if="legalNotices.length" class="step-boxes">
                    <button v-for="box in legalNotices" :key="box.clientKey" type="button" :class="{ selected: box.clientKey === selectedBoxKey }" @click="selectLegalNoticeBox(box)">
                        หน้า {{ box.pageNo }} · {{ box.label || 'ข้อความกฎหมาย' }}
                    </button>
                </div>
                <Button v-if="selectedIsLegalNotice && !props.readOnly" class="mt-3" label="ลบกรอบข้อความกฎหมาย" icon="pi pi-trash" severity="danger" outlined size="small" @click="deleteLegalNoticeBox()" />
            </div>

            <div class="inspector-section">
                <div class="section-heading">
                    <div>
                        <div class="section-title">ขั้นตอนและกรอบ</div>
                    <small>{{ totalBoxes }} กรอบลายเซ็น / {{ legalNotices.length }} กรอบข้อความกฎหมาย</small>
                    </div>
                    <Button label="ใช้กรอบเริ่มต้น" icon="pi pi-clone" severity="secondary" outlined size="small" :disabled="!canApplyPreset" @click="applyPreset" />
                </div>

                <Message v-if="validationIssues.length" severity="warn" class="mb-3">
                    <div v-for="issue in validationIssues.slice(0, 4)" :key="issue">{{ issue }}</div>
                </Message>

                <div class="step-list">
                    <div v-for="step in stepRows" :key="step.id || step.positionCode" class="step-row" :class="{ active: step.boxes.length > 0 }">
                        <div class="step-main">
                            <strong>{{ step.positionCode }} · {{ step.positionName }}</strong>
                            <small>{{ conditionLabel(step.conditionType) }} · {{ step.statusLabel }}</small>
                        </div>
                        <div class="step-actions">
                            <Button label="ลายเซ็น" icon="pi pi-plus" size="small" :disabled="!step.canAdd" :title="step.addReason" @click="addBox(step)" />
                            <Button v-if="props.allowSignNoteBoxes" label="หมายเหตุ" icon="pi pi-comment" severity="secondary" outlined size="small" :disabled="!step.canAddNote" :title="step.addReason" @click="addSignNoteBox(step)" />
                        </div>
                        <div v-if="step.boxes.length" class="step-boxes">
                            <button v-for="box in step.boxes" :key="box.clientKey" type="button" :class="{ selected: box.clientKey === selectedBoxKey }" @click="selectBox(box)">
                                หน้า {{ box.pageNo }} · {{ box.label || signerLabel(box.signerUser) || step.positionName }}
                            </button>
                        </div>
                        <div v-if="props.allowSignNoteBoxes && step.noteBoxes.length" class="step-boxes sign-note-list">
                            <button v-for="box in step.noteBoxes" :key="box.clientKey" type="button" :class="{ selected: box.clientKey === selectedBoxKey }" @click="selectSignNoteBox(box)">
                                หน้า {{ box.pageNo }} · หมายเหตุ {{ signerLabel(box.signerUser) || step.positionName }}
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        </aside>
    </div>
</template>

<style scoped>
.layout-designer {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(22rem, 24rem);
    gap: 1rem;
    min-height: min(70dvh, 46rem);
}
.layout-designer-full {
    height: clamp(26rem, calc(100dvh - 20rem), 82rem);
    min-height: clamp(26rem, calc(100dvh - 20rem), 82rem);
}
.pdf-pane {
    min-width: 0;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    overflow: hidden;
    background: var(--surface-ground);
}
.layout-designer-full .pdf-pane {
    display: flex;
    min-height: 0;
    flex-direction: column;
}
.layout-toolbar {
    min-height: 3rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.45rem 0.65rem;
    border-bottom: 1px solid var(--surface-border);
    background: var(--surface-card);
}
.toolbar-group {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    white-space: nowrap;
}
.page-label,
.zoom-label {
    min-width: 5rem;
    text-align: center;
    color: var(--text-color-secondary);
    font-size: 0.9rem;
}
.pdf-viewport {
    height: min(64dvh, 42rem);
    overflow: auto;
    padding: 1rem;
}
.layout-designer-full .pdf-viewport {
    height: auto;
    min-height: 0;
    flex: 1 1 auto;
}
.pdf-empty {
    height: 100%;
    display: grid;
    place-items: center;
    color: var(--text-color-secondary);
}
.pdf-page-shell {
    position: relative;
    width: max-content;
    margin: 0 auto;
    min-height: 12rem;
    box-shadow: 0 8px 24px rgba(15, 23, 42, 0.16);
    background: white;
}
.pdf-page-shell.rendering {
    opacity: 0.82;
}
.pdf-page-shell canvas {
    display: block;
}
.signature-layout-box {
    position: absolute;
    border: 2px solid #06b6d4;
    background: rgba(6, 182, 212, 0.14);
    color: #0f172a;
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.25rem;
    padding: 0.2rem;
    cursor: move;
    text-align: left;
}
.signature-layout-box.selected {
    border-color: #f59e0b;
    background: rgba(245, 158, 11, 0.2);
    box-shadow: 0 0 0 2px rgba(245, 158, 11, 0.22);
}
.legal-notice-layout-box {
    align-items: center;
    justify-content: center;
    border: 1px solid var(--surface-400, #9ca3af);
    background: rgba(255, 255, 255, 0.96);
    padding: 0.25rem;
    text-align: center;
}
.legal-notice-layout-box.selected {
    border-color: var(--primary-color);
    background: rgba(255, 255, 255, 0.98);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--primary-color) 22%, transparent);
}
.legal-notice-layout-box.overflow {
    border-color: var(--p-orange-500, #f59e0b);
    box-shadow: 0 0 0 2px rgba(245, 158, 11, 0.24);
}
.sign-note-layout-box {
    border-color: var(--p-orange-500, #f97316);
    background: rgba(251, 146, 60, 0.16);
}
.sign-note-layout-box.selected {
    border-color: var(--p-orange-600, #ea580c);
    background: rgba(251, 146, 60, 0.24);
    box-shadow: 0 0 0 2px rgba(251, 146, 60, 0.22);
}
.signature-layout-label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.78rem;
    font-weight: 700;
}
.legal-notice-preview-text {
    display: block;
    width: 100%;
    max-height: 100%;
    overflow: hidden;
    color: #111827;
    font-size: var(--legal-preview-font-size, 9px);
    font-weight: 600;
    line-height: 1.45;
    text-align: center;
    white-space: normal;
    overflow-wrap: break-word;
}
.signature-layout-box i {
    cursor: pointer;
    font-size: 0.75rem;
    background: rgba(255, 255, 255, 0.85);
    border-radius: 999px;
    padding: 0.2rem;
}
.legal-notice-layout-box i {
    position: absolute;
    top: -0.65rem;
    right: -0.65rem;
}
.signature-layout-box b {
    position: absolute;
    right: -0.35rem;
    bottom: -0.35rem;
    width: 0.75rem;
    height: 0.75rem;
    border-radius: 999px;
    background: #f59e0b;
    cursor: nwse-resize;
}
.layout-inspector {
    min-width: 0;
    display: grid;
    gap: 1rem;
    align-content: start;
}
.layout-designer-full .layout-inspector {
    max-height: 100%;
    overflow: auto;
    padding-right: 0.1rem;
}
.inspector-section {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.85rem;
    background: var(--surface-card);
}
.section-title {
    font-weight: 700;
}
.section-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.75rem;
}
.empty-hint,
.section-heading small,
.step-main small {
    color: var(--text-color-secondary);
}
.selected-form {
    display: grid;
    gap: 0.45rem;
    margin-top: 0.75rem;
}
.step-list {
    display: grid;
    gap: 0.65rem;
}
.step-row {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.7rem;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.6rem;
}
.step-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.35rem;
}
.step-row.active {
    border-color: color-mix(in srgb, var(--primary-color) 50%, var(--surface-border));
}
.step-main {
    display: grid;
    gap: 0.2rem;
    min-width: 0;
}
.step-boxes {
    grid-column: 1 / -1;
    display: grid;
    gap: 0.35rem;
}
.step-boxes button {
    border: 1px solid var(--surface-border);
    border-radius: 6px;
    background: transparent;
    padding: 0.4rem 0.5rem;
    text-align: left;
    cursor: pointer;
}
.step-boxes button.selected {
    border-color: #f59e0b;
    background: rgba(245, 158, 11, 0.12);
}
.sign-note-list button {
    border-color: color-mix(in srgb, var(--p-orange-500, #f97316) 35%, var(--surface-border));
    background: color-mix(in srgb, var(--p-orange-500, #f97316) 7%, transparent);
}
.layout-designer-readonly .signature-layout-box {
    cursor: pointer;
}
.layout-designer-readonly .signature-layout-box:hover {
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--primary-color) 35%, transparent);
}
@media (max-width: 980px) {
    .layout-designer {
        grid-template-columns: 1fr;
    }
    .layout-designer-full {
        height: auto;
        min-height: 0;
    }
    .pdf-viewport {
        height: 58dvh;
    }
    .layout-designer-full .layout-inspector {
        max-height: none;
        overflow: visible;
    }
}
@media (max-width: 640px) {
    .layout-toolbar {
        align-items: stretch;
        flex-direction: column;
    }
    .toolbar-group {
        justify-content: space-between;
    }
}
</style>
