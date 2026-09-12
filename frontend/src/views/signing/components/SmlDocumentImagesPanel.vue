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

// Each state gets the message that tells the viewer what to do next, rather than
// a bare "no images" that leaves them calling support.
const emptyMessage = computed(() => {
    switch (String(props.documentStatus || '').trim()) {
        case 'completed_image_failed':
            return 'ส่งรูปเข้า SML ไม่สำเร็จ — กดปุ่ม "ส่งรูป SML อีกครั้ง" ด้านบนเพื่อลองใหม่';
        case 'completed_evidence_failed':
            return 'ยังสร้าง PDF หลักฐานไม่สำเร็จ จึงยังไม่ได้ส่งรูปเข้า SML — กดปุ่ม "สร้าง PDF อีกครั้ง" ด้านบน';
        case 'completed_lock_failed':
            // The images were pushed before the lock step, so an empty gallery here
            // means something else went wrong and a reload is the useful action.
            return 'ยังไม่พบรูปในระบบ SML สำหรับเอกสารนี้ — ลองโหลดใหม่อีกครั้ง';
        case 'completed':
            return 'ยังไม่มีรูปในระบบ SML สำหรับเอกสารนี้';
        case '':
            return 'ยังไม่มีรูปในระบบ SML สำหรับเอกสารนี้';
        default:
            return 'เอกสารต้องลงนามให้เสร็จก่อน จึงจะมีรูปในระบบ SML';
    }
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
            // The first page is awaited so the viewer sees something immediately;
            // the rest follow in the background so every thumbnail fills in and
            // paging never waits on a fetch.
            await loadPage(0);
            loadRemainingPages();
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

// Every page is fetched, a few at a time, so the thumbnail strip is complete and
// paging is instant. Concurrency is capped because a document can hold dozens of
// images and the browser would otherwise open a connection for each at once.
async function loadRemainingPages() {
    const token = requestToken;
    const queue = images.value.map((_, index) => index).filter((index) => index !== 0);
    const workers = Array.from({ length: Math.min(3, queue.length) }, async () => {
        while (queue.length) {
            if (token !== requestToken) return;
            await loadPage(queue.shift());
        }
    });
    await Promise.all(workers);
}

function onActiveIndexChange(index) {
    activeIndex.value = index;
    // Still requested directly: a page the background pass has not reached yet
    // should jump the queue when the viewer navigates straight to it.
    loadPage(index);
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

// Thumbnail counts follow the sakai-vue Galleria reference so the strip degrades
// the same way the rest of the UI kit does on narrow screens.
const galleriaResponsiveOptions = [
    { breakpoint: '1280px', numVisible: 6 },
    { breakpoint: '1024px', numVisible: 5 },
    { breakpoint: '960px', numVisible: 4 },
    { breakpoint: '768px', numVisible: 3 },
    { breakpoint: '560px', numVisible: 1 }
];

// Watching a string key rather than an array literal: an array getter allocates
// a fresh array on every parent re-render, which Vue reads as a change and would
// re-run the whole listing each time.
watch(
    () => `${props.enabled ? '1' : '0'}:${props.documentId}`,
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
            <Galleria
                :value="images"
                :activeIndex="activeIndex"
                :numVisible="8"
                :responsiveOptions="galleriaResponsiveOptions"
                :circular="false"
                :showItemNavigators="imageCount > 1"
                :showThumbnails="imageCount > 1"
                :showItemNavigatorsOnHover="false"
                containerClass="sml-galleria"
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

        </template>
    </div>
</template>

<style scoped>
.sml-images-panel {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    /* Fills the height the dialog hands down, so the frame below can take the
       leftover space instead of the panel overflowing into a dialog scrollbar. */
    height: 100%;
    min-height: 0;
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

.sml-image-frame {
    display: flex;
    align-items: center;
    justify-content: center;
    /* min-height:0 lets a flex child shrink; without it the image floors the
       frame at its natural size and the dialog scrolls. */
    min-height: 0;
    height: 100%;
    width: 100%;
    background: var(--surface-100);
    border-radius: 6px;
}

.sml-image {
    display: block;
    max-width: 100%;
    max-height: 100%;
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

/* Page scans are almost entirely white, so a thumbnail needs a dark ground and
   a border to read as a separate item rather than blending into its neighbours
   and into the strip behind them. */
.sml-thumb {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 62px;
    height: 84px;
    padding: 3px;
    margin: 0 3px;
    overflow: hidden;
    background: rgba(0, 0, 0, 0.35);
    border: 1px solid rgba(0, 0, 0, 0.45);
    border-radius: 4px;
    transition: border-color 0.15s ease;
}

.sml-thumb:hover {
    border-color: var(--primary-color, #10b981);
}

/* contain, not cover: a page scan is portrait, so cover crops it to its middle
   and every thumbnail ends up looking like the same grey block. */
.sml-thumb img {
    width: 100%;
    height: 100%;
    object-fit: contain;
    /* Sits inside the padding so the dark ground shows as a frame. */
    border-radius: 2px;
    background: #fff;
}

.sml-thumb-failed {
    border-color: var(--p-amber-500, #f59e0b);
}

/* PrimeVue owns the thumbnail wrapper and the class marking the current one, so
   the highlight is drawn on that wrapper - a fully global selector, since none
   of these elements carry this component's scope attribute. */
:global(.sml-galleria .p-galleria-thumbnail-item-current .sml-thumb) {
    border-color: var(--primary-color, #10b981) !important;
    background: rgba(0, 0, 0, 0.55) !important;
}

.sml-thumb-placeholder {
    font-size: 0.75rem;
    color: var(--text-color-secondary);
}

/* PrimeVue's Galleria is display:block with statically sized children, so each
   level from the root down to the item wrapper has to opt into flex for the
   image area to absorb the dialog's leftover height instead of overflowing. */
/* PrimeVue already makes .p-galleria-items a flex row with the nav buttons
   absolutely positioned inside it, and .p-galleria-items-container a flex
   column. All this needs to add is permission for those to shrink, so the
   image area takes the leftover dialog height instead of the content growing
   past it. Global, not scoped: the panel renders inside a teleported dialog,
   so a scoped attribute never reaches a child component's elements. */
:global(.sml-galleria) {
    flex: 1;
    min-height: 0;
    max-width: 100%;
    display: flex;
    flex-direction: column;
}

:global(.sml-galleria .p-galleria-content) {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
}

:global(.sml-galleria .p-galleria-items-container) {
    flex: 1;
    min-height: 0;
}

/* min-width:0 alongside min-height:0 so a wide scan cannot push the flex row
   wider than the dialog either. */
:global(.sml-galleria .p-galleria-items) {
    min-height: 0;
    min-width: 0;
}

/* The item wraps the image; it must fill the row without becoming a flex
   parent of its own, which is what previously squeezed the nav buttons and
   thumbnail strip out of view. */
:global(.sml-galleria .p-galleria-item) {
    min-height: 0;
    min-width: 0;
    width: 100%;
}

/* The thumbnail strip keeps its natural height so only the image area flexes. */
:global(.sml-galleria .p-galleria-thumbnails) {
    flex: 0 0 auto;
}

</style>
