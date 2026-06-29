<script setup>
import { api } from '@/services/api';
import SigningWorkspace from '@/views/signing/components/SigningWorkspace.vue';
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const toast = useToast();
const token = String(route.params.token || '');

const otp = ref('');
const sessionToken = ref(sessionStorage.getItem(`external_session_${token}`) || '');
const document = ref(null);
const task = ref(null);
const legal = ref(null);
const loading = ref(false);
const saving = ref(false);

const verified = computed(() => !!sessionToken.value && !!document.value);
const pdfHeaders = computed(() => (sessionToken.value ? { Authorization: `Bearer ${sessionToken.value}` } : {}));
const identityLabel = computed(() => task.value?.signerName || task.value?.signerUser || 'ผู้ลงนามภายนอก');

onMounted(() => {
    if (sessionToken.value) {
        loadDocument().catch(() => {
            sessionStorage.removeItem(`external_session_${token}`);
            sessionToken.value = '';
        });
    }
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
    loading.value = true;
    try {
        const result = await api.getPublicSigningDocument(token, sessionToken.value);
        document.value = result.document;
        task.value = result.task;
        legal.value = result.legal;
    } finally {
        loading.value = false;
    }
}

async function signTask(payload) {
    saving.value = true;
    try {
        const result = await api.signPublicTask(token, sessionToken.value, payload);
        document.value = result.document;
        task.value = (result.document?.signers || []).find((item) => item.id === task.value?.id) || task.value;
        toast.add({ severity: 'success', summary: 'เซ็นเอกสารแล้ว', life: 3000 });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'เซ็นไม่สำเร็จ', detail: err.message, life: 4200 });
        throw err;
    } finally {
        saving.value = false;
    }
}

async function rejectTask(payload) {
    saving.value = true;
    try {
        const result = await api.rejectPublicTask(token, sessionToken.value, payload);
        document.value = result.document;
        task.value = (result.document?.signers || []).find((item) => item.id === task.value?.id) || task.value;
        toast.add({ severity: 'success', summary: 'ปฏิเสธเอกสารแล้ว', life: 3000 });
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ปฏิเสธไม่สำเร็จ', detail: err.message, life: 4200 });
        throw err;
    } finally {
        saving.value = false;
    }
}

async function attachFile(file, note) {
    await api.uploadPublicTaskAttachment(token, sessionToken.value, file, note);
}

function recordEvent(payload) {
    if (!sessionToken.value) return;
    api.recordPublicSigningTaskEvent(token, sessionToken.value, payload).catch(() => {});
}
</script>

<template>
    <main class="public-sign">
        <section v-if="!verified" class="otp-panel">
            <div class="otp-brand">
                <span><i class="pi pi-file-check"></i></span>
                <div>
                    <h1>PaperLess</h1>
                    <p>กรอก OTP เพื่อเปิดเอกสารสำหรับเซ็น</p>
                </div>
            </div>
            <InputText v-model="otp" inputmode="numeric" maxlength="8" placeholder="OTP" class="otp-input" autofocus />
            <Button label="ยืนยัน OTP" icon="pi pi-lock-open" :loading="loading" :disabled="otp.length < 4" @click="verifyOTP" />
        </section>

        <SigningWorkspace
            v-else
            public-mode
            :document="document"
            :task="task"
            :legal="legal"
            :pdf-url="api.publicSigningPDFUrl(token)"
            :pdf-headers="pdfHeaders"
            :loading="loading"
            :saving="saving"
            :identity-label="identityLabel"
            :on-reload="loadDocument"
            :on-sign="signTask"
            :on-reject="rejectTask"
            :on-attach="attachFile"
            :on-record-event="recordEvent"
        />
    </main>
</template>

<style scoped>
.public-sign {
    min-height: 100dvh;
    background: var(--surface-ground);
}

.otp-panel {
    width: min(28rem, calc(100vw - 2rem));
    margin: 12dvh auto 0;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 1.15rem;
    display: grid;
    gap: 1rem;
}

.otp-brand {
    display: flex;
    align-items: center;
    gap: 0.8rem;
}

.otp-brand > span {
    width: 3rem;
    height: 3rem;
    border-radius: 8px;
    display: grid;
    place-items: center;
    background: var(--primary-color);
    color: var(--primary-contrast-color);
    font-size: 1.35rem;
}

.otp-brand h1 {
    margin: 0;
    font-size: 1.35rem;
}

.otp-brand p {
    margin: 0.2rem 0 0;
    color: var(--text-color-secondary);
}

.otp-input {
    min-height: 44px;
    font-size: 1.3rem;
    text-align: center;
    letter-spacing: 0;
}
</style>
