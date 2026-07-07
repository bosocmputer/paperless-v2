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
const attachments = ref([]);
const attachmentsLoading = ref(false);
const attachmentsError = ref('');
const externalState = ref(sessionToken.value ? 'loading' : 'otp');
const terminalMessage = ref('');
const terminalDocNo = ref('');

const verified = computed(() => externalState.value === 'signing' && !!sessionToken.value && !!document.value && !!task.value);
const pdfHeaders = computed(() => (sessionToken.value ? { Authorization: `Bearer ${sessionToken.value}` } : {}));
const pdfUrl = computed(() => (verified.value ? api.publicSigningPDFUrl(token, api.signingTaskPDFCacheKey(task.value, document.value)) : ''));
const identityLabel = computed(() => task.value?.signerName || task.value?.signerUser || 'ผู้ลงนามภายนอก');
const canSubmitOTP = computed(() => otp.value.length === 6 && !loading.value);
const isTerminalState = computed(() => ['signed_success', 'already_signed', 'expired', 'unavailable'].includes(externalState.value));
const terminalView = computed(() => {
    switch (externalState.value) {
        case 'signed_success':
            return {
                icon: 'pi pi-check-circle',
                severity: 'success',
                title: 'เซ็นเอกสารเรียบร้อยแล้ว',
                detail: terminalMessage.value || 'ระบบบันทึกลายเซ็นแล้ว สามารถปิดหน้านี้ได้'
            };
        case 'already_signed':
            return {
                icon: 'pi pi-check-circle',
                severity: 'success',
                title: 'เอกสารนี้เซ็นเรียบร้อยแล้ว',
                detail: terminalMessage.value || 'ลิงก์นี้ใช้งานครบแล้ว สามารถปิดหน้านี้ได้'
            };
        case 'expired':
            return {
                icon: 'pi pi-clock',
                severity: 'warn',
                title: 'ลิงก์หรือ session หมดอายุ',
                detail: terminalMessage.value || 'กรุณากรอก OTP อีกครั้ง หรือขอ OTP ใหม่จากผู้ดูแล'
            };
        default:
            return {
                icon: 'pi pi-exclamation-triangle',
                severity: 'danger',
                title: 'ไม่สามารถเซ็นเอกสารนี้ได้',
                detail: terminalMessage.value || 'เอกสารนี้ไม่อยู่ในสถานะที่เปิดให้เซ็นจากลิงก์ภายนอก'
            };
    }
});

onMounted(() => {
    if (sessionToken.value) {
        loadDocument().catch(() => {
            if (externalState.value === 'loading') {
                clearExternalSession();
                externalState.value = 'otp';
            }
        });
    }
});

async function verifyOTP() {
    if (!canSubmitOTP.value) return;
    loading.value = true;
    try {
        const result = await api.verifyExternalOTP(token, otp.value);
        sessionToken.value = result.session.sessionToken;
        sessionStorage.setItem(`external_session_${token}`, sessionToken.value);
        await loadDocument();
        toast.add({ severity: 'success', summary: 'ยืนยัน OTP แล้ว', life: 2200 });
    } catch (err) {
        if (!handlePublicSigningError(err)) {
            toast.add({ severity: 'error', summary: 'OTP ไม่ถูกต้อง', detail: err.message, life: 4000 });
        }
    } finally {
        loading.value = false;
    }
}

async function loadDocument() {
    loading.value = true;
    externalState.value = 'loading';
    try {
        const result = await api.getPublicSigningDocument(token, sessionToken.value);
        document.value = result.document;
        task.value = result.task;
        legal.value = result.legal;
        if (task.value?.status === 'pending') {
            externalState.value = 'signing';
            await loadAttachments();
        } else {
            setTerminalFromTask(task.value?.status);
        }
    } catch (err) {
        handlePublicSigningError(err);
        throw err;
    } finally {
        loading.value = false;
    }
}

async function signTask(payload) {
    saving.value = true;
    try {
        const result = await api.signPublicTask(token, sessionToken.value, payload);
        setSignedSuccess(result);
        toast.add({ severity: 'success', summary: 'เซ็นเอกสารแล้ว', life: 3000 });
    } catch (err) {
        if (handlePublicSigningError(err)) return;
        toast.add({ severity: 'error', summary: 'เซ็นไม่สำเร็จ', detail: err.message, life: 4200 });
        throw err;
    } finally {
        saving.value = false;
    }
}

async function attachFile(file, note, requirementKey = '') {
    await api.uploadPublicTaskAttachment(token, sessionToken.value, file, note, requirementKey);
    await loadAttachments();
}

async function loadAttachments() {
    if (!sessionToken.value || !task.value?.id) return;
    attachmentsLoading.value = true;
    attachmentsError.value = '';
    try {
        const result = await api.getPublicTaskAttachments(token, sessionToken.value);
        attachments.value = Array.isArray(result.attachments) ? result.attachments : [];
    } catch (err) {
        attachmentsError.value = err.message || 'โหลดไฟล์แนบไม่สำเร็จ';
    } finally {
        attachmentsLoading.value = false;
    }
}

function attachmentFileUrl(attachment) {
    return api.publicTaskAttachmentFileUrl(token, attachment?.id || '');
}

function recordEvent(payload) {
    if (!sessionToken.value || externalState.value !== 'signing') return;
    api.recordPublicSigningTaskEvent(token, sessionToken.value, payload).catch(() => {});
}

function handleOTPInput(value) {
    otp.value = String(value || '')
        .replace(/\D/g, '')
        .slice(0, 6);
}

function clearExternalSession() {
    sessionStorage.removeItem(`external_session_${token}`);
    sessionToken.value = '';
}

function clearSigningData() {
    document.value = null;
    task.value = null;
    legal.value = null;
    attachments.value = [];
    attachmentsError.value = '';
}

function setSignedSuccess(result = {}) {
    terminalDocNo.value = result.document?.docNo || document.value?.docNo || terminalDocNo.value;
    terminalMessage.value = 'ระบบบันทึกลายเซ็นแล้ว สามารถปิดหน้านี้ได้';
    externalState.value = 'signed_success';
    clearExternalSession();
    clearSigningData();
}

function setTerminalFromTask(status) {
    if (status === 'signed') {
        externalState.value = 'already_signed';
        terminalMessage.value = 'เอกสารนี้เซ็นเรียบร้อยแล้ว สามารถปิดหน้านี้ได้';
        clearExternalSession();
        clearSigningData();
        return true;
    }
    if (status === 'rejected') {
        externalState.value = 'unavailable';
        terminalMessage.value = 'เอกสารนี้ถูกปฏิเสธแล้ว ไม่สามารถเซ็นจากลิงก์นี้ได้';
        clearExternalSession();
        clearSigningData();
        return true;
    }
    if (status === 'waiting') {
        externalState.value = 'unavailable';
        terminalMessage.value = 'เอกสารนี้ยังไม่ถึงลำดับเซ็นของลิงก์นี้';
        clearExternalSession();
        clearSigningData();
        return true;
    }
    externalState.value = 'unavailable';
    clearExternalSession();
    clearSigningData();
    return true;
}

function handlePublicSigningError(err) {
    const code = err?.payload?.error || '';
    if (code === 'already_signed') {
        terminalDocNo.value = document.value?.docNo || terminalDocNo.value;
        externalState.value = 'already_signed';
        terminalMessage.value = 'เอกสารนี้เซ็นเรียบร้อยแล้ว สามารถปิดหน้านี้ได้';
        clearExternalSession();
        clearSigningData();
        return true;
    }
    if (code === 'already_rejected') {
        externalState.value = 'unavailable';
        terminalMessage.value = 'เอกสารนี้ถูกปฏิเสธแล้ว ไม่สามารถเซ็นจากลิงก์นี้ได้';
        clearExternalSession();
        clearSigningData();
        return true;
    }
    if (code === 'external_session_required' || code === 'external_session_invalid') {
        externalState.value = 'expired';
        terminalMessage.value = 'session สำหรับเซ็นเอกสารหมดอายุ กรุณากรอก OTP อีกครั้ง';
        clearExternalSession();
        clearSigningData();
        return true;
    }
    if (code === 'signing_task_not_turn' || code === 'signing_task_unavailable' || code === 'external_sign_only') {
        externalState.value = 'unavailable';
        terminalMessage.value = err?.message || 'เอกสารนี้ไม่อยู่ในสถานะที่เปิดให้เซ็นจากลิงก์ภายนอก';
        clearExternalSession();
        clearSigningData();
        return true;
    }
    return false;
}

function resetToOTP() {
    clearExternalSession();
    clearSigningData();
    terminalMessage.value = '';
    terminalDocNo.value = '';
    externalState.value = 'otp';
}
</script>

<template>
    <main class="public-sign" :class="{ 'signing-active': verified }">
        <section v-if="externalState === 'otp'" class="otp-panel">
            <div class="otp-brand">
                <span><i class="pi pi-file-check"></i></span>
                <div>
                    <h1>PaperLess</h1>
                    <p>ลิงก์นี้ใช้สำหรับเซ็นเอกสารเท่านั้น</p>
                </div>
            </div>
            <Message severity="info" class="otp-copy">กรอก OTP 6 หลักเพื่อเปิดหน้าเซ็น หน้านี้ใช้สำหรับเซ็นและแนบเอกสารที่ระบบกำหนดเท่านั้น</Message>
            <InputText
                :modelValue="otp"
                inputmode="numeric"
                pattern="[0-9]*"
                maxlength="6"
                placeholder="OTP 6 หลัก"
                class="otp-input"
                autofocus
                @update:modelValue="handleOTPInput"
                @keyup.enter="verifyOTP"
            />
            <Button label="ยืนยัน OTP" icon="pi pi-lock-open" :loading="loading" :disabled="!canSubmitOTP" @click="verifyOTP" />
        </section>

        <section v-else-if="externalState === 'loading'" class="status-panel">
            <i class="pi pi-spin pi-spinner"></i>
            <div>
                <h1>กำลังเปิดหน้าเซ็น</h1>
                <p>กรุณารอสักครู่</p>
            </div>
        </section>

        <section v-else-if="isTerminalState" class="terminal-panel" :class="`terminal-${terminalView.severity}`">
            <i :class="terminalView.icon"></i>
            <h1>{{ terminalView.title }}</h1>
            <p>{{ terminalView.detail }}</p>
            <small v-if="terminalDocNo">เอกสาร {{ terminalDocNo }}</small>
            <Button v-if="externalState === 'expired'" label="กรอก OTP อีกครั้ง" icon="pi pi-refresh" severity="secondary" outlined @click="resetToOTP" />
        </section>

        <SigningWorkspace
            v-else-if="verified"
            public-mode
            external-sign-only
            :document="document"
            :task="task"
            :legal="legal"
            :pdf-url="pdfUrl"
            :pdf-headers="pdfHeaders"
            :loading="loading"
            :saving="saving"
            :identity-label="identityLabel"
            :attachments="attachments"
            :attachments-loading="attachmentsLoading"
            :attachments-error="attachmentsError"
            :allow-external-attachments="true"
            :on-reload="loadDocument"
            :on-sign="signTask"
            :on-attach="attachFile"
            :on-reload-attachments="loadAttachments"
            :attachment-file-url="attachmentFileUrl"
            :on-record-event="recordEvent"
        />
    </main>
</template>

<style scoped>
.public-sign {
    min-height: 100dvh;
    background: var(--surface-ground);
    display: grid;
    align-items: start;
}

.public-sign.signing-active {
    display: block;
}

.otp-panel,
.status-panel,
.terminal-panel {
    width: min(28rem, calc(100vw - 2rem));
    margin: 12dvh auto 0;
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 1.15rem;
    display: grid;
    gap: 1rem;
}

.status-panel,
.terminal-panel {
    justify-items: center;
    text-align: center;
    padding-block: 1.45rem;
}

.status-panel > i,
.terminal-panel > i {
    font-size: 2.4rem;
}

.status-panel h1,
.terminal-panel h1 {
    margin: 0;
    font-size: 1.25rem;
}

.status-panel p,
.terminal-panel p {
    margin: 0;
    color: var(--text-color-secondary);
    line-height: 1.55;
}

.terminal-panel small {
    color: var(--text-color-secondary);
}

.terminal-success > i {
    color: var(--green-500);
}

.terminal-warn > i {
    color: var(--yellow-600);
}

.terminal-danger > i {
    color: var(--red-500);
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

.otp-copy {
    margin: 0;
}

.otp-input {
    min-height: 44px;
    font-size: 1.3rem;
    text-align: center;
    letter-spacing: 0;
}

@media (max-width: 520px) {
    .otp-panel,
    .status-panel,
    .terminal-panel {
        margin-top: 8dvh;
        padding: 1rem;
    }

    .otp-brand > span {
        width: 2.75rem;
        height: 2.75rem;
    }
}
</style>
