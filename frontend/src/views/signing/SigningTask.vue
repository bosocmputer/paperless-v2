<script setup>
import { api } from '@/services/api';
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();

const document = ref(null);
const task = ref(null);
const loading = ref(false);
const saving = ref(false);
const pdfUrl = ref('');
const canvas = ref(null);
const hasSignature = ref(false);
const legalAccepted = ref(false);
const rejectVisible = ref(false);
const rejectReason = ref('');
let drawing = false;
let ctx;

const canConfirm = computed(() => hasSignature.value && legalAccepted.value && task.value?.status === 'pending');
const legalText = 'ข้าพเจ้ายืนยันการลงลายเซ็นอิเล็กทรอนิกส์นี้ตาม พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์ และยอมรับให้ใช้เป็นหลักฐานประกอบเอกสารนี้';

onMounted(loadTask);
onBeforeUnmount(() => {
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
});

async function loadTask() {
    loading.value = true;
    try {
        const result = await api.getMySigningTask(route.params.taskId);
        document.value = result.document;
        task.value = result.task;
        await loadPdf();
        await nextTick();
        setupCanvas();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดเอกสารไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadPdf() {
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
    const response = await fetch(api.signingDocumentPDFUrl(document.value.id), { headers: api.authHeaders() });
    if (!response.ok) throw new Error('โหลด PDF ไม่สำเร็จ');
    pdfUrl.value = URL.createObjectURL(await response.blob());
}

function setupCanvas() {
    if (!canvas.value) return;
    const rect = canvas.value.getBoundingClientRect();
    canvas.value.width = Math.floor(rect.width * window.devicePixelRatio);
    canvas.value.height = Math.floor(180 * window.devicePixelRatio);
    ctx = canvas.value.getContext('2d');
    ctx.scale(window.devicePixelRatio, window.devicePixelRatio);
    ctx.lineWidth = 2.4;
    ctx.lineCap = 'round';
    ctx.strokeStyle = '#111827';
    clearSignature();
}

function point(event) {
    const rect = canvas.value.getBoundingClientRect();
    const source = event.touches?.[0] || event;
    return { x: source.clientX - rect.left, y: source.clientY - rect.top };
}

function startDraw(event) {
    event.preventDefault();
    drawing = true;
    const p = point(event);
    ctx.beginPath();
    ctx.moveTo(p.x, p.y);
}

function moveDraw(event) {
    if (!drawing) return;
    event.preventDefault();
    const p = point(event);
    ctx.lineTo(p.x, p.y);
    ctx.stroke();
    hasSignature.value = true;
}

function endDraw() {
    drawing = false;
}

function clearSignature() {
    if (!ctx || !canvas.value) return;
    ctx.clearRect(0, 0, canvas.value.width, canvas.value.height);
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(0, 0, canvas.value.width, canvas.value.height);
    hasSignature.value = false;
}

function deviceId() {
    const key = 'paperless_device_id';
    let value = localStorage.getItem(key);
    if (!value) {
        value = crypto.randomUUID?.() || `${Date.now()}-${Math.random()}`;
        localStorage.setItem(key, value);
    }
    return value;
}

async function confirmSign() {
    if (!canConfirm.value) return;
    saving.value = true;
    try {
        const result = await api.signMyTask(task.value.id, {
            signatureDataUrl: canvas.value.toDataURL('image/png'),
            deviceId: deviceId(),
            legalText
        });
        toast.add({ severity: 'success', summary: 'เซ็นเอกสารแล้ว', life: 2500 });
        document.value = result.document;
        router.push({ name: 'my-signing-tasks' });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'เซ็นไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        saving.value = false;
    }
}

async function rejectTask() {
    if (!rejectReason.value.trim()) {
        toast.add({ severity: 'warn', summary: 'กรุณาระบุเหตุผล', life: 2500 });
        return;
    }
    saving.value = true;
    try {
        await api.rejectMyTask(task.value.id, { reason: rejectReason.value, deviceId: deviceId() });
        toast.add({ severity: 'success', summary: 'ปฏิเสธเอกสารแล้ว', life: 2500 });
        router.push({ name: 'my-signing-tasks' });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ปฏิเสธไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        saving.value = false;
    }
}
</script>

<template>
    <div class="sign-task">
        <div class="task-bar">
            <Button icon="pi pi-arrow-left" text rounded aria-label="กลับ" @click="router.push({ name: 'my-signing-tasks' })" />
            <div class="bar-title">
                <strong>{{ document?.docNo || 'เอกสาร' }}</strong>
                <span>{{ task?.positionName }} · {{ document?.partyName || document?.partyCode || '-' }}</span>
            </div>
            <Button label="Reject" icon="pi pi-times" severity="danger" outlined :disabled="saving || task?.status !== 'pending'" @click="rejectVisible = true" />
            <Button label="Confirm" icon="pi pi-check" :disabled="!canConfirm" :loading="saving" @click="confirmSign" />
        </div>

        <div class="task-grid">
            <section class="pdf-panel">
                <iframe v-if="pdfUrl" :src="pdfUrl" title="PDF preview"></iframe>
                <div v-else class="empty-pdf">กำลังโหลด PDF...</div>
            </section>

            <aside class="sign-panel">
                <div>
                    <div class="font-semibold text-lg mb-1">ลายเซ็น</div>
                    <p class="text-muted-color m-0">วาดลายเซ็นในกรอบด้านล่าง</p>
                </div>
                <canvas
                    ref="canvas"
                    class="signature-canvas"
                    @mousedown="startDraw"
                    @mousemove="moveDraw"
                    @mouseup="endDraw"
                    @mouseleave="endDraw"
                    @touchstart="startDraw"
                    @touchmove="moveDraw"
                    @touchend="endDraw"
                ></canvas>
                <Button label="ล้างลายเซ็น" icon="pi pi-eraser" severity="secondary" outlined @click="clearSignature" />
                <label class="legal-check">
                    <Checkbox v-model="legalAccepted" binary />
                    <span>{{ legalText }}</span>
                </label>
                <Message v-if="!hasSignature || !legalAccepted" severity="info">ต้องมีลายเซ็นและยอมรับข้อความยืนยันก่อน Confirm</Message>
            </aside>
        </div>
    </div>

    <Dialog v-model:visible="rejectVisible" modal header="ปฏิเสธเอกสาร" :style="{ width: 'min(36rem, 92vw)' }">
        <div class="grid gap-3">
            <label class="font-medium">เหตุผล</label>
            <Textarea v-model="rejectReason" rows="4" autoResize />
        </div>
        <template #footer>
            <Button label="ยกเลิก" severity="secondary" outlined @click="rejectVisible = false" />
            <Button label="ยืนยัน Reject" severity="danger" :loading="saving" @click="rejectTask" />
        </template>
    </Dialog>
</template>

<style scoped>
.sign-task {
    height: calc(100dvh - 8rem);
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}
.task-bar {
    min-height: 4rem;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.65rem 0.75rem;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
}
.bar-title {
    flex: 1;
    display: grid;
}
.bar-title span {
    color: var(--text-color-secondary);
}
.task-grid {
    min-height: 0;
    flex: 1;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(340px, 400px);
    gap: 0.75rem;
}
.pdf-panel,
.sign-panel {
    min-height: 0;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
}
.pdf-panel iframe {
    width: 100%;
    height: 100%;
    border: 0;
}
.empty-pdf {
    height: 100%;
    display: grid;
    place-items: center;
    color: var(--text-color-secondary);
}
.sign-panel {
    padding: 0.9rem;
    display: grid;
    gap: 0.8rem;
    align-content: start;
}
.signature-canvas {
    width: 100%;
    height: 180px;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: #fff;
    touch-action: none;
}
.legal-check {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    line-height: 1.45;
}
@media (max-width: 920px) {
    .sign-task {
        height: auto;
    }
    .task-grid {
        grid-template-columns: 1fr;
    }
    .pdf-panel {
        height: 68dvh;
    }
}
</style>
