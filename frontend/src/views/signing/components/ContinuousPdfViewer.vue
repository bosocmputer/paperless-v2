<script setup>
import * as pdfjsLib from 'pdfjs-dist';
import pdfWorker from 'pdfjs-dist/build/pdf.worker.mjs?url';
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';

pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;

const props = defineProps({
    url: { type: String, default: '' },
    headers: { type: Object, default: () => ({}) },
    emptyMessage: { type: String, default: 'ยังไม่มี PDF' },
    allowOpenFull: { type: Boolean, default: false },
    toolbarLabel: { type: String, default: 'PDF' },
    initialZoomMode: {
        type: String,
        default: 'fit-width',
        validator: (value) => ['fit-width', 'actual-size'].includes(value)
    },
    noteBoxes: { type: Array, default: () => [] },
    selectedNoteBoxKey: { type: String, default: '' },
    editableNoteBoxes: { type: Boolean, default: false }
});

const emit = defineEmits(['load-success', 'load-error', 'page-change', 'open-full', 'update:noteBoxes', 'note-box-select']);

const viewerRef = ref(null);
const pdfDoc = shallowRef(null);
const loading = ref(false);
const error = ref('');
const currentPage = ref(1);
const pageCount = ref(0);
const zoom = ref(1);
const fitWidthActive = ref(true);
const pageSizes = ref([]);
const renderedPages = ref(new Set());
const renderingPages = ref(new Set());

const pageOptions = computed(() => Array.from({ length: pageCount.value }, (_, index) => ({ label: `${index + 1}/${pageCount.value}`, value: index + 1 })));
const metaLabel = computed(() => (pageCount.value ? `หน้า ${currentPage.value} / ${pageCount.value} · ${Math.round(zoom.value * 100)}%` : 'ยังไม่มี PDF'));

let loadSequence = 0;
let loadingTask = null;
let observer = null;
let resizeObserver = null;
let scrollFrame = 0;
let renderGeneration = 0;
let renderTokenSeed = 0;
const pageShells = new Map();
const pageCanvases = new Map();
const renderTasks = new Set();
const renderQueue = [];
const queuedPages = new Set();
const visiblePages = new Set();
const activePageRenders = new Map();
let noteDragState = null;

watch(
    () => props.url,
    () => {
        void loadPDF();
    },
    { immediate: true }
);

watch(
    () => props.initialZoomMode,
    () => {
        if (!pdfDoc.value) return;
        applyInitialZoom();
        scheduleVisiblePages();
    }
);

watch(zoom, () => {
    if (!pdfDoc.value) return;
    resetRenderedPages();
    scheduleVisiblePages();
});

watch(currentPage, (value) => {
    emit('page-change', value);
});

onBeforeUnmount(() => {
    removeNotePointerListeners();
    cleanupPDF();
});

async function loadPDF() {
    const url = String(props.url || '').trim();
    const sequence = ++loadSequence;
    cleanupPDF({ keepSequence: true });
    error.value = '';
    if (!url) {
        loading.value = false;
        return;
    }

    loading.value = true;
    try {
        loadingTask = pdfjsLib.getDocument({ url, httpHeaders: props.headers || {} });
        const loaded = await loadingTask.promise;
        if (sequence !== loadSequence) {
            loaded?.destroy?.().catch(() => {});
            return;
        }
        pdfDoc.value = loaded;
        pageCount.value = loaded.numPages;
        currentPage.value = 1;
        pageSizes.value = await readPageSizes(loaded, sequence);
        if (sequence !== loadSequence) return;
        loading.value = false;
        await nextTick();
        setupObservers();
        applyInitialZoom();
        schedulePagesAround(1);
        emit('load-success', { pageCount: pageCount.value });
    } catch (err) {
        if (sequence !== loadSequence || err?.name === 'RenderingCancelledException') return;
        error.value = pdfLoadErrorMessage(err);
        emit('load-error', err);
    } finally {
        if (sequence === loadSequence) {
            loading.value = false;
            loadingTask = null;
        }
    }
}

async function readPageSizes(doc, sequence) {
    const sizes = [];
    for (let pageNo = 1; pageNo <= doc.numPages; pageNo += 1) {
        if (sequence !== loadSequence) return sizes;
        const page = await doc.getPage(pageNo);
        const viewport = page.getViewport({ scale: 1 });
        sizes.push({ width: viewport.width, height: viewport.height });
    }
    return sizes;
}

function setupObservers() {
    disconnectObservers();
    if (!viewerRef.value) return;

    observer = new IntersectionObserver(handleIntersections, {
        root: viewerRef.value,
        rootMargin: '700px 0px',
        threshold: [0, 0.08, 0.45, 0.85]
    });
    pageShells.forEach((element) => observer.observe(element));

    if (window.ResizeObserver) {
        resizeObserver = new ResizeObserver(() => {
            if (fitWidthActive.value) fitWidth();
        });
        resizeObserver.observe(viewerRef.value);
    }
}

function disconnectObservers() {
    if (observer) observer.disconnect();
    observer = null;
    if (resizeObserver) resizeObserver.disconnect();
    resizeObserver = null;
}

function setPageShell(pageNo, element) {
    const previous = pageShells.get(pageNo);
    if (previous && observer) observer.unobserve(previous);
    if (element) {
        pageShells.set(pageNo, element);
        if (observer) observer.observe(element);
    } else {
        pageShells.delete(pageNo);
    }
}

function setPageCanvas(pageNo, element) {
    if (element) pageCanvases.set(pageNo, element);
    else pageCanvases.delete(pageNo);
}

function handleIntersections(entries) {
    entries.forEach((entry) => {
        const pageNo = Number(entry.target.dataset.pageNo || 0);
        if (!pageNo) return;
        if (entry.isIntersecting) {
            visiblePages.add(pageNo);
            schedulePagesAround(pageNo);
        } else {
            visiblePages.delete(pageNo);
        }
    });
    updateActivePageFromScroll();
}

function handleScroll() {
    if (scrollFrame) return;
    scrollFrame = window.requestAnimationFrame(() => {
        scrollFrame = 0;
        updateActivePageFromScroll();
        scheduleVisiblePages();
    });
}

function updateActivePageFromScroll() {
    if (!viewerRef.value || pageShells.size === 0) return;
    const rootRect = viewerRef.value.getBoundingClientRect();
    const targetY = rootRect.top + Math.min(rootRect.height * 0.38, 220);
    let nearestPage = currentPage.value;
    let nearestDistance = Number.POSITIVE_INFINITY;
    pageShells.forEach((element, pageNo) => {
        const rect = element.getBoundingClientRect();
        if (rect.bottom < rootRect.top || rect.top > rootRect.bottom) return;
        const distance = Math.abs(rect.top - targetY);
        if (distance < nearestDistance) {
            nearestDistance = distance;
            nearestPage = pageNo;
        }
    });
    if (nearestPage !== currentPage.value) currentPage.value = nearestPage;
}

function scheduleVisiblePages() {
    if (visiblePages.size === 0) {
        schedulePagesAround(currentPage.value || 1);
        return;
    }
    visiblePages.forEach((pageNo) => schedulePagesAround(pageNo));
}

function schedulePagesAround(pageNo) {
    for (let next = pageNo - 1; next <= pageNo + 1; next += 1) {
        schedulePage(next);
    }
}

function schedulePage(pageNo) {
    if (!pdfDoc.value || pageNo < 1 || pageNo > pageCount.value) return;
    if (renderedPages.value.has(pageNo) || activePageRenders.has(pageNo) || queuedPages.has(pageNo)) return;
    queuedPages.add(pageNo);
    renderQueue.push(pageNo);
    drainRenderQueue();
}

function drainRenderQueue() {
    while (activePageRenders.size < 2 && renderQueue.length > 0) {
        const pageNo = renderQueue.shift();
        queuedPages.delete(pageNo);
        if (!pageNo || renderedPages.value.has(pageNo) || activePageRenders.has(pageNo)) continue;
        void renderPage(pageNo, renderGeneration);
    }
}

async function renderPage(pageNo, generation) {
    const canvas = pageCanvases.get(pageNo);
    const size = pageSizes.value[pageNo - 1];
    if (!pdfDoc.value || !canvas || !size) return;

    setRendering(pageNo, true);
    const renderToken = ++renderTokenSeed;
    activePageRenders.set(pageNo, renderToken);
    let task = null;
    try {
        const page = await pdfDoc.value.getPage(pageNo);
        if (generation !== renderGeneration) return;
        const viewport = page.getViewport({ scale: zoom.value });
        const outputScale = Math.min(window.devicePixelRatio || 1, 2);
        const renderCanvas = window.document.createElement('canvas');
        const renderContext = renderCanvas.getContext('2d');
        renderCanvas.width = Math.floor(viewport.width * outputScale);
        renderCanvas.height = Math.floor(viewport.height * outputScale);
        renderContext.setTransform(outputScale, 0, 0, outputScale, 0, 0);
        renderContext.clearRect(0, 0, viewport.width, viewport.height);
        task = page.render({ canvasContext: renderContext, viewport });
        renderTasks.add(task);
        await task.promise;
        if (generation !== renderGeneration || pageCanvases.get(pageNo) !== canvas) return;
        const context = canvas.getContext('2d');
        canvas.width = renderCanvas.width;
        canvas.height = renderCanvas.height;
        canvas.style.width = `${viewport.width}px`;
        canvas.style.height = `${viewport.height}px`;
        context.setTransform(1, 0, 0, 1, 0, 0);
        context.clearRect(0, 0, canvas.width, canvas.height);
        context.drawImage(renderCanvas, 0, 0);
        setRendered(pageNo, true);
    } catch (err) {
        if (err?.name !== 'RenderingCancelledException') {
            error.value = pdfRenderErrorMessage(err, pageNo);
            emit('load-error', err);
        }
    } finally {
        if (task) renderTasks.delete(task);
        if (activePageRenders.get(pageNo) === renderToken) {
            activePageRenders.delete(pageNo);
            setRendering(pageNo, false);
        }
        drainRenderQueue();
    }
}

function setRendered(pageNo, value) {
    const next = new Set(renderedPages.value);
    if (value) next.add(pageNo);
    else next.delete(pageNo);
    renderedPages.value = next;
}

function setRendering(pageNo, value) {
    const next = new Set(renderingPages.value);
    if (value) next.add(pageNo);
    else next.delete(pageNo);
    renderingPages.value = next;
}

function resetRenderedPages() {
    renderGeneration += 1;
    cancelRenderTasks();
    renderQueue.splice(0);
    queuedPages.clear();
    activePageRenders.clear();
    pageCanvases.forEach((canvas) => {
        canvas.width = 0;
        canvas.height = 0;
        canvas.style.width = '';
        canvas.style.height = '';
    });
    renderedPages.value = new Set();
    renderingPages.value = new Set();
}

function fitWidth() {
    if (!viewerRef.value || pageSizes.value.length === 0) return;
    fitWidthActive.value = true;
    const firstPage = pageSizes.value[0];
    const available = Math.max(viewerRef.value.clientWidth - 32, 240);
    zoom.value = clamp(available / firstPage.width, 0.45, 2.25);
}

function applyInitialZoom() {
    if (props.initialZoomMode === 'actual-size') {
        fitWidthActive.value = false;
        zoom.value = 1;
        return;
    }
    fitWidth();
}

function setZoom(value) {
    fitWidthActive.value = false;
    zoom.value = clamp(value, 0.45, 2.5);
}

function goToPage(pageNo) {
    const next = clamp(Number(pageNo || 1), 1, pageCount.value || 1);
    currentPage.value = next;
    const shell = pageShells.get(next);
    if (shell) shell.scrollIntoView({ behavior: 'smooth', block: 'start' });
    schedulePagesAround(next);
}

function pageShellStyle(size) {
    const width = Math.max(1, size.width * zoom.value);
    const height = Math.max(1, size.height * zoom.value);
    return {
        width: `${width}px`,
        minHeight: `${height}px`
    };
}

function noteBoxesForPage(pageNo) {
    return (props.noteBoxes || []).filter((box) => Number(box.pageNo || 1) === Number(pageNo));
}

function noteBoxStyle(box) {
    return {
        left: `${clampRatio(box.xRatio) * 100}%`,
        top: `${clampRatio(box.yRatio) * 100}%`,
        width: `${clampRatio(box.widthRatio) * 100}%`,
        height: `${clampRatio(box.heightRatio) * 100}%`
    };
}

function selectNoteBox(box) {
    emit('note-box-select', box?.clientKey || '');
}

function deleteNoteBox(box) {
    if (!props.editableNoteBoxes) return;
    const key = box?.clientKey || '';
    emit(
        'update:noteBoxes',
        (props.noteBoxes || []).filter((item) => item.clientKey !== key)
    );
    if (props.selectedNoteBoxKey === key) emit('note-box-select', '');
}

function startNotePointer(event, box, mode = 'move') {
    if (!props.editableNoteBoxes || !box?.clientKey) return;
    event.preventDefault();
    event.stopPropagation();
    selectNoteBox(box);
    const shell = pageShells.get(Number(box.pageNo || 1));
    if (!shell) return;
    noteDragState = {
        key: box.clientKey,
        mode,
        pageNo: Number(box.pageNo || 1),
        startX: event.clientX,
        startY: event.clientY,
        shell,
        startBox: {
            xRatio: Number(box.xRatio || 0),
            yRatio: Number(box.yRatio || 0),
            widthRatio: Number(box.widthRatio || 0),
            heightRatio: Number(box.heightRatio || 0)
        }
    };
    window.addEventListener('pointermove', moveNotePointer, { passive: false });
    window.addEventListener('pointerup', endNotePointer, { once: true });
    window.addEventListener('pointercancel', endNotePointer, { once: true });
}

function moveNotePointer(event) {
    if (!noteDragState) return;
    event.preventDefault();
    const rect = noteDragState.shell.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return;
    const dx = (event.clientX - noteDragState.startX) / rect.width;
    const dy = (event.clientY - noteDragState.startY) / rect.height;
    const minW = 0.04;
    const minH = 0.015;
    const start = noteDragState.startBox;
    const patch = {};
    if (noteDragState.mode === 'resize') {
        patch.widthRatio = clamp(start.widthRatio + dx, minW, 1 - start.xRatio);
        patch.heightRatio = clamp(start.heightRatio + dy, minH, 1 - start.yRatio);
    } else {
        patch.xRatio = clamp(start.xRatio + dx, 0, 1 - start.widthRatio);
        patch.yRatio = clamp(start.yRatio + dy, 0, 1 - start.heightRatio);
    }
    updateNoteBox(noteDragState.key, patch);
}

function endNotePointer() {
    removeNotePointerListeners();
    noteDragState = null;
}

function removeNotePointerListeners() {
    window.removeEventListener('pointermove', moveNotePointer);
    window.removeEventListener('pointerup', endNotePointer);
    window.removeEventListener('pointercancel', endNotePointer);
}

function updateNoteBox(key, patch) {
    emit(
        'update:noteBoxes',
        (props.noteBoxes || []).map((box) => (box.clientKey === key ? { ...box, ...patch } : box))
    );
}

function retryLoad() {
    void loadPDF();
}

function cleanupPDF(options = {}) {
    if (!options.keepSequence) loadSequence += 1;
    if (scrollFrame) {
        window.cancelAnimationFrame(scrollFrame);
        scrollFrame = 0;
    }
    disconnectObservers();
    resetRenderedPages();
    if (loadingTask?.destroy) loadingTask.destroy().catch(() => {});
    loadingTask = null;
    if (pdfDoc.value?.destroy) pdfDoc.value.destroy().catch(() => {});
    pdfDoc.value = null;
    pageCount.value = 0;
    currentPage.value = 1;
    pageSizes.value = [];
    visiblePages.clear();
    pageShells.clear();
    pageCanvases.clear();
}

function cancelRenderTasks() {
    renderTasks.forEach((task) => {
        try {
            task.cancel();
        } catch {
            // PDF.js can throw if rendering finished at the same time.
        }
    });
    renderTasks.clear();
}

function pdfRenderErrorMessage(err, pageNo) {
    const message = String(err?.message || '');
    if (message.includes('same canvas') || message.includes('multiple render')) return `กำลังแสดง PDF หน้า ${pageNo} ซ้ำ กรุณาลองใหม่อีกครั้ง`;
    return err?.message || `แสดง PDF หน้า ${pageNo} ไม่สำเร็จ`;
}

function pdfLoadErrorMessage(err) {
    const status = err?.status;
    const message = String(err?.message || '');
    if (status === 409 || message.includes('409')) return 'เอกสารนี้ไม่อยู่ในสถานะที่เปิด PDF ได้';
    if (status === 401 || status === 403 || message.includes('401') || message.includes('403')) return 'ไม่มีสิทธิ์เปิด PDF หรือลิงก์หมดอายุ';
    return err?.message || 'โหลด PDF ไม่สำเร็จ กรุณาลองใหม่';
}

function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
}

function clampRatio(value) {
    return clamp(Number(value || 0), 0, 1);
}
</script>

<template>
    <div class="continuous-pdf">
        <div class="viewer-toolbar">
            <div class="toolbar-title">
                <strong>{{ toolbarLabel }}</strong>
                <span>{{ metaLabel }}</span>
            </div>
            <div class="toolbar-actions">
                <Select :modelValue="currentPage" :options="pageOptions" optionLabel="label" optionValue="value" :disabled="pageOptions.length === 0" class="page-select" @update:modelValue="goToPage" />
                <Button class="zoom-step" icon="pi pi-search-minus" severity="secondary" text rounded :disabled="!pdfDoc || zoom <= 0.45" aria-label="ซูมออก" @click="setZoom(zoom - 0.1)" />
                <span class="zoom-value">{{ Math.round(zoom * 100) }}%</span>
                <Button class="zoom-step" icon="pi pi-search-plus" severity="secondary" text rounded :disabled="!pdfDoc || zoom >= 2.5" aria-label="ซูมเข้า" @click="setZoom(zoom + 0.1)" />
                <Button label="พอดีกว้าง" icon="pi pi-arrows-h" severity="secondary" text :disabled="!pdfDoc" @click="fitWidth" />
                <Button v-if="allowOpenFull" label="เต็มจอ" icon="pi pi-window-maximize" severity="secondary" text :disabled="!url" @click="emit('open-full')" />
            </div>
        </div>

        <div ref="viewerRef" class="pdf-scroll" @scroll.passive="handleScroll">
            <div v-if="loading" class="empty-pdf">
                <i class="pi pi-spin pi-spinner"></i>
                <span>กำลังโหลด PDF...</span>
            </div>

            <Message v-else-if="error" severity="error" class="pdf-error">
                {{ error }}
                <div class="mt-3">
                    <Button v-if="url" size="small" label="ลองใหม่" icon="pi pi-refresh" severity="secondary" outlined @click="retryLoad" />
                </div>
            </Message>

            <div v-else-if="pdfDoc" class="pdf-pages">
                <div
                    v-for="(size, index) in pageSizes"
                    :key="index + 1"
                    :ref="(element) => setPageShell(index + 1, element)"
                    class="pdf-page-shell"
                    :data-page-no="index + 1"
                    :style="pageShellStyle(size)"
                >
                    <div class="page-number">หน้า {{ index + 1 }}</div>
                    <canvas :ref="(element) => setPageCanvas(index + 1, element)" class="pdf-canvas" aria-label="PDF preview"></canvas>
                    <button
                        v-for="box in noteBoxesForPage(index + 1)"
                        :key="box.clientKey"
                        type="button"
                        class="runtime-note-box"
                        :class="{ selected: box.clientKey === selectedNoteBoxKey, editable: editableNoteBoxes, empty: !String(box.text || '').trim() }"
                        :style="noteBoxStyle(box)"
                        @click.stop="selectNoteBox(box)"
                        @pointerdown.stop="startNotePointer($event, box, 'move')"
                    >
                        <span>{{ box.text || 'พิมพ์หมายเหตุ' }}</span>
                        <i v-if="editableNoteBoxes" class="pi pi-trash" aria-label="ลบกล่องหมายเหตุ" @pointerdown.stop @click.stop="deleteNoteBox(box)"></i>
                        <b v-if="editableNoteBoxes" @pointerdown.stop="startNotePointer($event, box, 'resize')"></b>
                    </button>
                    <div v-if="!renderedPages.has(index + 1)" class="page-placeholder">
                        <i v-if="renderingPages.has(index + 1)" class="pi pi-spin pi-spinner"></i>
                    </div>
                </div>
            </div>

            <div v-else class="empty-pdf">{{ emptyMessage }}</div>
        </div>
    </div>
</template>

<style scoped>
.continuous-pdf {
    min-height: 0;
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
}

.viewer-toolbar {
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.65rem;
}

.toolbar-title {
    min-width: 0;
    display: grid;
    gap: 0.1rem;
    line-height: 1.15;
}

.toolbar-title strong,
.toolbar-title span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.toolbar-title span,
.zoom-value {
    color: var(--text-color-secondary);
    font-size: 0.85rem;
}

.toolbar-actions {
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    flex-wrap: wrap;
    gap: 0.25rem;
}

.page-select {
    width: 6.75rem;
    flex: 0 0 auto;
}

.zoom-value {
    width: 3.2rem;
    text-align: center;
}

.pdf-scroll {
    min-height: 0;
    flex: 1;
    overflow: auto;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: color-mix(in srgb, var(--surface-ground) 78%, var(--surface-card));
    padding: 0.85rem;
}

.pdf-pages {
    min-width: max-content;
    display: grid;
    justify-items: center;
    gap: 0.85rem;
}

.pdf-page-shell {
    position: relative;
    display: block;
    background: white;
    line-height: 0;
    box-shadow: 0 2px 8px rgba(15, 23, 42, 0.14);
}

.pdf-canvas {
    display: block;
    user-select: none;
}

.page-number {
    position: absolute;
    top: 0.45rem;
    left: 0.45rem;
    z-index: 2;
    line-height: 1;
    padding: 0.22rem 0.42rem;
    border-radius: 999px;
    background: color-mix(in srgb, var(--surface-card) 88%, transparent);
    color: var(--text-color-secondary);
    font-size: 0.75rem;
    box-shadow: 0 1px 4px rgba(15, 23, 42, 0.12);
}

.page-placeholder {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    color: var(--text-color-secondary);
    pointer-events: none;
}

.runtime-note-box {
    position: absolute;
    z-index: 3;
    display: flex;
    align-items: flex-start;
    justify-content: flex-start;
    min-width: 0;
    min-height: 0;
    padding: 0.18rem 0.28rem;
    border: 1.5px solid color-mix(in srgb, #f59e0b 72%, var(--surface-border));
    border-radius: 4px;
    background: color-mix(in srgb, #f59e0b 12%, transparent);
    color: #78350f;
    line-height: 1.2;
    text-align: left;
    overflow: hidden;
    cursor: pointer;
    user-select: none;
}

.runtime-note-box span {
    min-width: 0;
    flex: 1;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    font-size: clamp(0.58rem, 1.45vw, 0.82rem);
    font-weight: 700;
    overflow-wrap: anywhere;
}

.runtime-note-box.empty span {
    color: color-mix(in srgb, #92400e 64%, white);
}

.runtime-note-box.selected {
    border-color: #0284c7;
    background: color-mix(in srgb, #38bdf8 18%, white);
    box-shadow: 0 0 0 2px color-mix(in srgb, #38bdf8 38%, transparent);
}

.runtime-note-box.editable {
    cursor: move;
}

.runtime-note-box i {
    flex: 0 0 auto;
    margin-left: 0.25rem;
    line-height: 1;
    color: #b45309;
}

.runtime-note-box b {
    position: absolute;
    right: -5px;
    bottom: -5px;
    width: 12px;
    height: 12px;
    border-radius: 999px;
    border: 2px solid white;
    background: #0284c7;
    cursor: nwse-resize;
}

.empty-pdf {
    min-height: 18rem;
    display: grid;
    place-items: center;
    align-content: center;
    gap: 0.6rem;
    color: var(--text-color-secondary);
    text-align: center;
}

.pdf-error {
    width: min(34rem, 100%);
    margin: 1rem auto;
}

@media (max-width: 640px) {
    .viewer-toolbar {
        align-items: stretch;
        flex-direction: column;
        gap: 0.45rem;
    }

    .toolbar-actions {
        width: 100%;
        justify-content: flex-start;
        gap: 0.15rem;
    }

    .page-select {
        flex: 1 1 8rem;
        min-width: 8rem;
    }

    .zoom-step,
    .zoom-value {
        display: none;
    }

    .pdf-scroll {
        padding: 0.55rem;
    }
}
</style>
