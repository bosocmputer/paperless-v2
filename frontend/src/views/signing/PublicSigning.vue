<script setup>
import { api } from '@/services/api';
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const toast = useToast();
const token = String(route.params.token || '');

const otp = ref('');
const sessionToken = ref(sessionStorage.getItem(`external_session_${token}`) || '');
const document = ref(null);
const task = ref(null);
const pdfUrl = ref('');
const loading = ref(false);
const saving = ref(false);
const verified = computed(() => !!sessionToken.value && !!document.value);
const canvas = ref(null);
const hasSignature = ref(false);
const legalAccepted = ref(false);
const rejectVisible = ref(false);
const rejectReason = ref('');
let drawing = false;
let ctx;

const legalText = 'ข้าพเจ้ายืนยันการลงลายเซ็นอิเล็กทรอนิกส์นี้ตาม พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์ และยอมรับให้ใช้เป็นหลักฐานประกอบเอกสารนี้';
const canConfirm = computed(() => verified.value && hasSignature.value && legalAccepted.value && task.value?.status === 'pending');

onMounted(() => {
    if (sessionToken.value) {
        loadDocument().catch(() => {
            sessionStorage.removeItem(`external_session_${token}`);
            sessionToken.value = '';
        });
    }
});

onBeforeUnmount(() => {
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
});

async function verifyOTP() {
    loading.value = true;
    try {
        const result = await api.verifyExternalOTP(token, otp.value);
        sessionToken.value = result.session.sessionToken;
        sessionStorage.setItem(`external_session_${token}`, sessionToken.value);
        await loadDocument();
        toast.add({ severity: 'success', summary: 'ยืนยัน OTP แล้ว', life: 2200 });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'OTP ไม่ถูกต้อง', detail: err.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

async function loadDocument() {
    const result = await api.getPublicSigningDocument(token, sessionToken.value);
    document.value = result.document;
    task.value = result.task;
    await loadPdf();
    await nextTick();
    setupCanvas();
}

async function loadPdf() {
    if (pdfUrl.value) URL.revokeObjectURL(pdfUrl.value);
    const response = await fetch(api.publicSigningPDFUrl(token), {
        headers: { Authorization: `Bearer ${sessionToken.value}` }
    });
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
    const key = 'paperless_external_device_id';
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
        await api.signPublicTask(token, sessionToken.value, {
            signatureDataUrl: canvas.value.toDataURL('image/png'),
            deviceId: deviceId(),
            legalText
        });
        toast.add({ severity: 'success', summary: 'เซ็นเอกสารแล้ว', life: 3000 });
        await loadDocument();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'เซ็นไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        saving.value = false;
    }
}

async function rejectTask() {
    if (!rejectReason.value.trim()) {
        toast.add({ severity: 'warn', summary: 'กรุณาระบุเหตุผล', life: 2400 });
        return;
    }
    saving.value = true;
    try {
        await api.rejectPublicTask(token, sessionToken.value, { reason: rejectReason.value, deviceId: deviceId() });
        toast.add({ severity: 'success', summary: 'ปฏิเสธเอกสารแล้ว', life: 3000 });
        await loadDocument();
        rejectVisible.value = false;
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ปฏิเสธไม่สำเร็จ', detail: err.message, life: 4000 });
    } finally {
        saving.value = false;
    }
}
</script>

<template>
    <main class="public-sign">
        <section v-if="!verified" class="otp-panel">
            <h1>PaperLess Signing</h1>
            <p>กรอก OTP ที่ได้รับจากผู้ส่งเอกสารเพื่อเปิดเอกสารสำหรับเซ็น</p>
            <InputText v-model="otp" inputmode="numeric" maxlength="8" placeholder="OTP" class="otp-input" />
            <Button label="ยืนยัน OTP" icon="pi pi-lock-open" :loading="loading" :disabled="otp.length < 4" @click="verifyOTP" />
        </section>

        <section v-else class="public-workspace">
            <div class="task-bar">
                <div class="bar-title">
                    <strong>{{ document?.docNo }}</strong>
                    <span>{{ task?.positionName }} · {{ document?.partyName || document?.partyCode || '-' }}</span>
                </div>
                <Tag :value="task?.status" :severity="task?.status === 'signed' ? 'success' : task?.status === 'rejected' ? 'danger' : 'info'" />
                <Button label="Reject" severity="danger" outlined :disabled="saving || task?.status !== 'pending'" @click="rejectVisible = true" />
                <Button label="Confirm" icon="pi pi-check" :disabled="!canConfirm" :loading="saving" @click="confirmSign" />
            </div>
            <div class="public-grid">
                <iframe v-if="pdfUrl" :src="pdfUrl" title="PDF preview"></iframe>
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
                </aside>
            </div>
        </section>
    </main>

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
.public-sign {
    min-height: 100dvh;
    background: var(--surface-ground);
    padding: 1rem;
}
.otp-panel {
    width: min(28rem, 100%);
    margin: 12dvh auto 0;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 1.25rem;
    display: grid;
    gap: 1rem;
}
.otp-panel h1 {
    margin: 0;
    font-size: 1.5rem;
}
.otp-panel p {
    margin: 0;
    color: var(--text-color-secondary);
}
.otp-input {
    font-size: 1.35rem;
    text-align: center;
    letter-spacing: 0;
}
.public-workspace {
    height: calc(100dvh - 2rem);
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}
.task-bar {
    min-height: 4rem;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 0.7rem;
    display: flex;
    align-items: center;
    gap: 0.75rem;
}
.bar-title {
    flex: 1;
    display: grid;
}
.bar-title span {
    color: var(--text-color-secondary);
}
.public-grid {
    min-height: 0;
    flex: 1;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(340px, 400px);
    gap: 0.75rem;
}
.public-grid iframe,
.sign-panel {
    min-height: 0;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
}
.public-grid iframe {
    width: 100%;
    height: 100%;
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
    .public-workspace {
        height: auto;
    }
    .public-grid {
        grid-template-columns: 1fr;
    }
    .public-grid iframe {
        height: 68dvh;
    }
}
</style>
