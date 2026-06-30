<script setup>
import { api } from '@/services/api';
import * as pdfjsLib from 'pdfjs-dist';
import pdfWorker from 'pdfjs-dist/build/pdf.worker.mjs?url';
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;

const props = defineProps({
    pdfUrl: { type: String, default: '' },
    pageCount: { type: Number, default: 0 },
    configs: { type: Array, default: () => [] },
    modelValue: { type: Array, default: () => [] },
    presetTemplate: { type: Object, default: null }
});

const emit = defineEmits(['update:modelValue', 'apply-preset', 'event']);

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

const boxes = computed(() => props.modelValue || []);
const currentPageBoxes = computed(() => boxes.value.filter((box) => Number(box.pageNo || 1) === currentPage.value));
const selectedBox = computed(() => boxes.value.find((box) => box.clientKey === selectedBoxKey.value) || null);
const selectedStep = computed(() => props.configs.find((step) => step.positionCode === selectedBox.value?.positionCode) || null);
const totalBoxes = computed(() => boxes.value.length);

const validationIssues = computed(() => {
    const issues = [];
    if (!props.pdfUrl) issues.push('อัปโหลด PDF ก่อน');
    if (boxes.value.length === 0) issues.push('ต้องวางกรอบอย่างน้อย 1 กรอบ');
    for (const box of boxes.value) {
        if (box.xRatio < 0 || box.yRatio < 0 || box.widthRatio <= 0 || box.heightRatio <= 0 || box.xRatio + box.widthRatio > 1 || box.yRatio + box.heightRatio > 1) {
            issues.push(`กรอบ ${box.label || box.positionCode} อยู่นอกหน้า PDF`);
        }
        if (box.pageNo < 1 || box.pageNo > props.pageCount) issues.push(`กรอบ ${box.label || box.positionCode} อยู่หน้าที่ไม่ถูกต้อง`);
    }
    for (const step of props.configs) {
        const stepBoxes = boxes.value.filter((box) => box.positionCode === step.positionCode);
        if (stepBoxes.length === 0) continue;
        if (step.conditionType === 1 && stepBoxes.length !== 1) issues.push(`${step.positionName} ต้องมี 1 กรอบ`);
        if (step.conditionType === 3 && stepBoxes.length !== 1) issues.push(`${step.positionName} ต้องมี 1 กรอบบุคคลภายนอก`);
        if (step.conditionType === 2) {
            const seen = new Set();
            for (const box of stepBoxes) {
                const user = signerUsername(box.signerUser);
                if (!user) issues.push(`${step.positionName} ต้องเลือก user ทุกกรอบ`);
                if (user && seen.has(user)) issues.push(`${step.positionName} มี user ซ้ำ`);
                if (user) seen.add(user);
            }
        }
    }
    return [...new Set(issues)];
});

const canApplyPreset = computed(() => !!props.presetTemplate?.boxes?.length && !!props.pdfUrl);

const stepRows = computed(() =>
    [...props.configs]
        .sort((a, b) => Number(a.sequenceNo || 0) - Number(b.sequenceNo || 0) || String(a.positionCode).localeCompare(String(b.positionCode)))
        .map((step) => {
            const stepBoxes = boxes.value.filter((box) => box.positionCode === step.positionCode);
            const users = stepUsers(step);
            const boxedUsers = new Set(stepBoxes.map((box) => signerUsername(box.signerUser)).filter(Boolean));
            let canAdd = !!props.pdfUrl;
            let addReason = '';
            if (!props.pdfUrl) addReason = 'ต้องอัปโหลด PDF ก่อน';
            if (step.conditionType !== 2 && stepBoxes.length >= 1) {
                canAdd = false;
                addReason = 'มีกรอบครบแล้ว';
            }
            if (step.conditionType === 2 && users.every((user) => boxedUsers.has(signerUsername(user)))) {
                canAdd = false;
                addReason = 'เลือก user ครบแล้ว';
            }
            return {
                ...step,
                boxes: stepBoxes,
                users,
                canAdd,
                addReason,
                statusLabel: stepBoxes.length > 0 ? `ใช้ ${stepBoxes.length} กรอบ` : 'ไม่อยู่ในงานเซ็น'
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
    if (!canApplyPreset.value) return;
    if (props.presetTemplate.sampleFile?.pageCount && props.pageCount && props.presetTemplate.sampleFile.pageCount !== props.pageCount) {
        emit('event', 'preset_page_mismatch');
        return;
    }
    const next = (props.presetTemplate.boxes || []).map((box) => ({
        ...box,
        clientKey: makeKey(),
        pageNo: Number(box.pageNo || 1),
        xRatio: Number(box.xRatio || 0.1),
        yRatio: Number(box.yRatio || 0.1),
        widthRatio: Number(box.widthRatio || 0.2),
        heightRatio: Number(box.heightRatio || 0.08)
    }));
    emitBoxes(next);
    selectedBoxKey.value = next[0]?.clientKey || '';
    emit('apply-preset', props.presetTemplate);
    emit('event', 'preset_applied');
}

function addBox(step) {
    if (!props.pdfUrl) return;
    const users = stepUsers(step);
    const existing = boxes.value.filter((box) => box.positionCode === step.positionCode);
    if (step.conditionType !== 2 && existing.length >= 1) return;
    let signerType = 'any';
    let signerUser = '';
    let signerSlot = nextSignerSlot(existing);
    if (step.conditionType === 2) {
        signerType = 'internal';
        const used = new Set(existing.map((box) => signerUsername(box.signerUser)));
        signerUser = users.find((user) => !used.has(signerUsername(user))) || users[0] || '';
        if (!signerUser) return;
    } else if (step.conditionType === 3) {
        signerType = 'external';
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

function deleteBox(box) {
    emitBoxes(boxes.value.filter((item) => item.clientKey !== box.clientKey));
    if (selectedBoxKey.value === box.clientKey) selectedBoxKey.value = '';
    emit('event', 'box_delete');
}

function updateSelected(field, value) {
    if (!selectedBox.value) return;
    updateBox(selectedBox.value.clientKey, { [field]: value });
}

function updateBox(key, patch) {
    emitBoxes(boxes.value.map((box) => (box.clientKey === key ? { ...box, ...patch } : box)));
}

function emitBoxes(next) {
    emit('update:modelValue', next);
}

function selectBox(box) {
    selectedBoxKey.value = box.clientKey;
    if (box.pageNo !== currentPage.value) currentPage.value = Number(box.pageNo || 1);
}

function boxStyle(box) {
    return {
        left: `${box.xRatio * renderedSize.value.width}px`,
        top: `${box.yRatio * renderedSize.value.height}px`,
        width: `${box.widthRatio * renderedSize.value.width}px`,
        height: `${box.heightRatio * renderedSize.value.height}px`
    };
}

function startPointer(event, box, mode) {
    if (!renderedSize.value.width || !renderedSize.value.height) return;
    selectBox(box);
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

function makeKey() {
    return crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
}

function clamp(value, min, max) {
    return Math.min(max, Math.max(min, Number(value) || 0));
}

defineExpose({ validationIssues, totalBoxes });
</script>

<template>
    <div class="layout-designer">
        <div class="pdf-pane">
            <div class="layout-toolbar">
                <div class="toolbar-group">
                    <Button icon="pi pi-angle-left" severity="secondary" text :disabled="currentPage <= 1" @click="currentPage--" />
                    <span class="page-label">หน้า {{ currentPage }} / {{ props.pageCount || pdfDoc?.numPages || 0 }}</span>
                    <Button icon="pi pi-angle-right" severity="secondary" text :disabled="currentPage >= (props.pageCount || pdfDoc?.numPages || 1)" @click="currentPage++" />
                </div>
                <div class="toolbar-group">
                    <Button label="Fit width" severity="secondary" outlined size="small" @click="fitWidth()" />
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
                        v-for="box in currentPageBoxes"
                        :key="box.clientKey"
                        type="button"
                        class="signature-layout-box"
                        :class="{ selected: box.clientKey === selectedBoxKey }"
                        :style="boxStyle(box)"
                        @click.stop="selectBox(box)"
                        @pointerdown.stop="startPointer($event, box, 'move')"
                    >
                        <span>{{ box.label || box.signerUser || box.positionCode }}</span>
                        <i class="pi pi-trash" @pointerdown.stop @click.stop="deleteBox(box)"></i>
                        <b @pointerdown.stop="startPointer($event, box, 'resize')"></b>
                    </button>
                </div>
            </div>
        </div>

        <aside class="layout-inspector">
            <div class="inspector-section">
                <div class="section-title">กรอบที่เลือก</div>
                <div v-if="!selectedBox" class="empty-hint">เลือกกรอบจาก PDF หรือเพิ่มกรอบจากขั้นตอนด้านล่าง</div>
                <div v-else class="selected-form">
                    <label>ข้อความบนกรอบ</label>
                    <InputText :modelValue="selectedBox.label" @update:modelValue="updateSelected('label', $event)" />
                    <label>หน้า</label>
                    <InputNumber :modelValue="selectedBox.pageNo" :min="1" :max="props.pageCount || 1" showButtons @update:modelValue="updateSelected('pageNo', $event || 1)" />
                    <label v-if="selectedStep?.conditionType === 2">User ผู้เซ็น</label>
                    <Select
                        v-if="selectedStep?.conditionType === 2"
                        :modelValue="selectedBox.signerUser"
                        :options="stepUsers(selectedStep).map((user) => ({ label: signerLabel(user), value: user }))"
                        optionLabel="label"
                        optionValue="value"
                        @update:modelValue="updateSelected('signerUser', $event)"
                    />
                </div>
            </div>

            <div class="inspector-section">
                <div class="section-heading">
                    <div>
                        <div class="section-title">ขั้นตอนและกรอบ</div>
                        <small>{{ totalBoxes }} กรอบที่จะสร้างงานเซ็น</small>
                    </div>
                    <Button label="ใช้ preset" icon="pi pi-clone" severity="secondary" outlined size="small" :disabled="!canApplyPreset" @click="applyPreset" />
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
                        <Button label="เพิ่ม" icon="pi pi-plus" size="small" :disabled="!step.canAdd" :title="step.addReason" @click="addBox(step)" />
                        <div v-if="step.boxes.length" class="step-boxes">
                            <button v-for="box in step.boxes" :key="box.clientKey" type="button" :class="{ selected: box.clientKey === selectedBoxKey }" @click="selectBox(box)">
                                หน้า {{ box.pageNo }} · {{ box.label || signerLabel(box.signerUser) || step.positionName }}
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
.pdf-pane {
    min-width: 0;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    overflow: hidden;
    background: var(--surface-ground);
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
.signature-layout-box span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.78rem;
    font-weight: 700;
}
.signature-layout-box i {
    cursor: pointer;
    font-size: 0.75rem;
    background: rgba(255, 255, 255, 0.85);
    border-radius: 999px;
    padding: 0.2rem;
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
@media (max-width: 980px) {
    .layout-designer {
        grid-template-columns: 1fr;
    }
    .pdf-viewport {
        height: 58dvh;
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
