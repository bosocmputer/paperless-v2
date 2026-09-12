<script setup>
import { api } from '@/services/api';
import { computed, onBeforeUnmount, ref, watch } from 'vue';

const props = defineProps({
    documentId: { type: String, default: '' },
    // Images only exist once a document has been pushed to SML, so the caller
    // tells us whether it is even worth asking.
    enabled: { type: Boolean, default: false },
    // Drives the "why is this empty" message without this panel having to know
    // the whole signing status vocabulary.
    documentStatus: { type: String, default: '' }
});

const emit = defineEmits(['update:count']);

const loading = ref(false);
const errorCode = ref('');
const images = ref([]);
const activeIndex = ref(0);
const loadedCount = ref(0);

// Blob URLs are revoked on teardown; keeping them keyed by page number means a
// page already fetched is never fetched twice while the panel stays open.
const objectUrls = ref(new Map());
const failedPages = ref(new Set());
const pendingPages = new Set();

let listController = null;
const pageControllers = new Map();
// Guards against a stale response from a previous document overwriting the
// current one when the user pages quickly through documents.
let requestToken = 0;

const imageCount = computed(() => images.value.length);
const hasImages = computed(() => imageCount.value > 0);
const activeImage = computed(() => images.value[activeIndex.value] || null);

const emptyMessage = computed(() => {
    const status = String(props.documentStatus || '').trim();
    if (status === 'completed_image_failed') {
        return 'ส่งรูปเข้า SML ไม่สำเร็จ — กดปุ่ม "ส่งรูป SML อีกครั้ง" ด้านบนเพื่อลองใหม่';
    }
    if (status && status !== 'completed' && !status.startsWith('completed')) {
        return 'เอกสารต้องลงนามให้เสร็จก่อน จึงจะมีรูปในระบบ SML';
    }
    return 'ยังไม่มีรูปในระบบ SML สำหรับเอกสารนี้';
});

const errorMessage = computed(() => {
    switch (errorCode.value) {
        case 'sml_not_configured':
            return 'ยังไม่ได้ตั้งค่าการเชื่อมต่อ SML';
        case 'sml_images_unsupported':
            return 'เอกสารนี้ไม่มีรูปในระบบ SML';
        case 'sml_images_unavailable':
            return 'เชื่อมต่อระบบ SML ไม่ได้ในขณะนี้';
        default:
            return 'โหลดรูปจาก SML ไม่สำเร็จ';
    }
});

function releaseObjectUrls() {
    for (const url of objectUrls.value.values()) {
        URL.revokeObjectURL(url);
    }
    objectUrls.value = new Map();
}

function abortPageRequests() {
    for (const controller of pageControllers.values()) {
        controller.abort();
    }
    pageControllers.clear();
    pendingPages.clear();
}

function resetState() {
    abortPageRequests();
    releaseObjectUrls();
    failedPages.value = new Set();
    images.value = [];
    activeIndex.value = 0;
    loadedCount.value = 0;
    errorCode.value = '';
    emit('update:count', 0);
}

function isAbortError(error) {
    return error?.name === 'AbortError';
}

async function loadList() {
    if (!props.enabled || !props.documentId) {
        resetState();
        return;
    }

    listController?.abort();
    listController = new AbortController();
    const token = ++requestToken;

    resetState();
    loading.value = true;
    try {
        const response = await api.listSigningDocumentSMLImages(props.documentId, { signal: listController.signal });
        if (token !== requestToken) return;
        images.value = Array.isArray(response?.images) ? response.images : [];
        emit('update:count', images.value.length);
        if (hasImages.value) {
            await loadPage(0);
            prefetchNeighbour(0);
        }
    } catch (error) {
        if (isAbortError(error) || token !== requestToken) return;
        errorCode.value = String(error?.payload?.error || '');
    } finally {
        if (token === requestToken) loading.value = false;
    }
}

async function loadPage(index) {
    const image = images.value[index];
    if (!image) return;
    const pageNo = image.page_no;
    if (objectUrls.value.has(pageNo) || pendingPages.has(pageNo)) return;

    const token = requestToken;
    const controller = new AbortController();
    pageControllers.set(pageNo, controller);
    pendingPages.add(pageNo);
    try {
        const blob = await api.signingDocumentSMLImageBlob(props.documentId, pageNo, { signal: controller.signal });
        if (token !== requestToken) {
            return;
        }
        const next = new Map(objectUrls.value);
        next.set(pageNo, URL.createObjectURL(blob));
        objectUrls.value = next;
        loadedCount.value = next.size;
        const stillFailed = new Set(failedPages.value);
        stillFailed.delete(pageNo);
        failedPages.value = stillFailed;
    } catch (error) {
        if (isAbortError(error) || token !== requestToken) return;
        const nextFailed = new Set(failedPages.value);
        nextFailed.add(pageNo);
        failedPages.value = nextFailed;
    } finally {
        pendingPages.delete(pageNo);
        pageControllers.delete(pageNo);
    }
}

// Fetching the next page while the viewer looks at the current one makes the
// arrow keys feel instant without loading the whole document up front.
function prefetchNeighbour(index) {
    if (index + 1 < images.value.length) loadPage(index + 1);
}

function onActiveIndexChange(index) {
    activeIndex.value = index;
    loadPage(index);
    prefetchNeighbour(index);
}

function retryPage(pageNo) {
    const nextFailed = new Set(failedPages.value);
    nextFailed.delete(pageNo);
    failedPages.value = nextFailed;
    const index = images.value.findIndex((item) => item.page_no === pageNo);
    if (index >= 0) loadPage(index);
}

function urlFor(pageNo) {
    return objectUrls.value.get(pageNo) || '';
}

function formatBytes(bytes) {
    const value = Number(bytes) || 0;
    if (value >= 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`;
    if (value >= 1024) return `${Math.round(value / 1024)} KB`;
    return `${value} B`;
}

watch(
    () => [props.documentId, props.enabled],
    () => {
        loadList();
    },
    { immediate: true }
);

onBeforeUnmount(() => {
    listController?.abort();
    abortPageRequests();
    releaseObjectUrls();
});

defineExpose({ reload: loadList });
</script>

<template>
    <div class="sml-images-panel">
        <div v-if="loading" class="sml-images-state">
            <ProgressSpinner style="width: 36px; height: 36px" strokeWidth="4" />
            <span>กำลังโหลดรูปจาก SML...</span>
        </div>

        <Message v-else-if="errorCode" severity="warn" :closable="false">
            <div class="sml-images-error">
                <span>{{ errorMessage }}</span>
                <Button label="ลองใหม่" icon="pi pi-refresh" size="small" severity="secondary" outlined @click="loadList" />
            </div>
        </Message>

        <div v-else-if="!hasImages" class="sml-images-state sml-images-empty">
            <i class="pi pi-images" />
            <span>{{ emptyMessage }}</span>
        </div>

        <template v-else>
            <div class="sml-images-summary">
                <span>ทั้งหมด {{ imageCount }} รูป</span>
                <span v-if="activeImage" class="sml-images-meta"> หน้า {{ activeImage.page_no }} · {{ formatBytes(activeImage.bytes) }} </span>
            </div>

            <Galleria
                :value="images"
                :activeIndex="activeIndex"
                :numVisible="5"
                :circular="false"
                :showItemNavigators="imageCount > 1"
                :showThumbnails="imageCount > 1"
                :showItemNavigatorsOnHover="false"
                containerStyle="max-width: 100%"
                @update:activeIndex="onActiveIndexChange"
            >
                <template #item="slotProps">
                    <div class="sml-image-frame">
                        <img v-if="urlFor(slotProps.item.page_no)" :src="urlFor(slotProps.item.page_no)" :alt="`หน้า ${slotProps.item.page_no}`" class="sml-image" />
                        <div v-else-if="failedPages.has(slotProps.item.page_no)" class="sml-image-failed">
                            <i class="pi pi-exclamation-triangle" />
                            <span>โหลดรูปหน้า {{ slotProps.item.page_no }} ไม่สำเร็จ</span>
                            <Button label="ลองใหม่" icon="pi pi-refresh" size="small" severity="secondary" outlined @click="retryPage(slotProps.item.page_no)" />
                        </div>
                        <div v-else class="sml-image-loading">
                            <ProgressSpinner style="width: 32px; height: 32px" strokeWidth="4" />
                        </div>
                    </div>
                </template>

                <template #thumbnail="slotProps">
                    <div class="sml-thumb" :class="{ 'sml-thumb-failed': failedPages.has(slotProps.item.page_no) }">
                        <img v-if="urlFor(slotProps.item.page_no)" :src="urlFor(slotProps.item.page_no)" :alt="`ย่อหน้า ${slotProps.item.page_no}`" />
                        <span v-else class="sml-thumb-placeholder">{{ slotProps.item.page_no }}</span>
                    </div>
                </template>
            </Galleria>

            <p class="sml-images-hint">แสดงรูปทั้งหมดที่จัดเก็บในระบบ SML (หน้าจอ SML ERP แสดงได้เพียง 8 รูปแรก)</p>
        </template>
    </div>
</template>

<style scoped>
.sml-images-panel {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}

.sml-images-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
    padding: 2rem 1rem;
    color: var(--text-color-secondary);
    text-align: center;
}

.sml-images-empty i {
    font-size: 1.75rem;
    opacity: 0.6;
}

.sml-images-error {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
}

.sml-images-summary {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
    font-size: 0.875rem;
    font-weight: 600;
}

.sml-images-meta {
    font-weight: 400;
    color: var(--text-color-secondary);
}

.sml-image-frame {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 260px;
    background: var(--surface-100);
    border-radius: 6px;
}

.sml-image {
    display: block;
    max-width: 100%;
    max-height: 60vh;
    object-fit: contain;
}

.sml-image-failed,
.sml-image-loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
    padding: 1.5rem;
    color: var(--text-color-secondary);
    text-align: center;
}

.sml-image-failed i {
    font-size: 1.5rem;
    color: var(--p-amber-500, #f59e0b);
}

.sml-thumb {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 54px;
    height: 54px;
    overflow: hidden;
    background: var(--surface-200);
    border-radius: 4px;
}

.sml-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.sml-thumb-failed {
    outline: 1px solid var(--p-amber-500, #f59e0b);
}

.sml-thumb-placeholder {
    font-size: 0.75rem;
    color: var(--text-color-secondary);
}

.sml-images-hint {
    margin: 0;
    font-size: 0.75rem;
    color: var(--text-color-secondary);
}
</style>
