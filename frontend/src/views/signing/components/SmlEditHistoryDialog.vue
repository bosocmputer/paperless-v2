<script setup>
/**
 * Shows who changed an SML document and exactly what they changed, read from
 * SML's own audit trail (erp_logs) — the same source the SML ERP ประวัติ
 * screen displays, so PaperLess and SML never tell a user different stories.
 *
 * This exists because the old "ข้อมูลเอกสารใน SML ถูกแก้ไข" warning could not
 * answer the only question a user actually asks: แก้ตรงไหน. Without that
 * answer, the warning read as a system error, and users reported it as a bug
 * rather than acting on it. Blocking someone's work without evidence is what
 * this dialog fixes.
 */
import { api } from '@/services/api';
import { ref, watch } from 'vue';

const props = defineProps({
    visible: { type: Boolean, default: false },
    documentId: { type: String, default: '' }
});
const emit = defineEmits(['update:visible']);

const loading = ref(false);
const loadError = ref('');
const entries = ref([]);
const truncated = ref(false);

function close() {
    emit('update:visible', false);
}

async function load() {
    if (!props.documentId) return;
    loading.value = true;
    loadError.value = '';
    try {
        const response = await api.getSigningDocumentSMLEditHistory(props.documentId);
        entries.value = Array.isArray(response?.entries) ? response.entries : [];
        truncated.value = !!response?.truncated;
    } catch (error) {
        // The audit database is a separate database from the SML data itself,
        // so it can be unreachable while everything else works. Say so plainly
        // rather than rendering an empty list, which would read as "nobody
        // edited it" and contradict the warning that opened this dialog.
        loadError.value = error?.message || 'ไม่สามารถอ่านประวัติการแก้ไขจาก SML ได้';
        entries.value = [];
    } finally {
        loading.value = false;
    }
}

watch(
    () => [props.visible, props.documentId],
    ([isVisible]) => {
        if (isVisible) load();
    },
    { immediate: true }
);

const actionLabels = {
    created: 'สร้างเอกสาร',
    edited: 'แก้ไขเอกสาร',
    deleted: 'ลบเอกสาร'
};

function actionLabel(entry) {
    return actionLabels[entry.action] || 'เปลี่ยนแปลง';
}

function actionSeverity(entry) {
    if (entry.action === 'deleted') return 'danger';
    if (entry.action === 'created') return 'warn';
    return 'info';
}

function editorLabel(entry) {
    const parts = [];
    if (entry.userCode) parts.push(entry.userCode);
    if (entry.computerName) parts.push(entry.computerName);
    return parts.length ? parts.join(' · ') : 'ไม่ทราบผู้ใช้';
}

/** Empty values render as a dash so "changed to blank" stays visible. */
function displayValue(value) {
    const text = (value ?? '').toString().trim();
    return text === '' ? '—' : text;
}
</script>

<template>
    <Dialog
        :visible="visible"
        modal
        header="ประวัติการแก้ไขใน SML"
        :style="{ width: '56rem' }"
        :breakpoints="{ '960px': '92vw' }"
        @update:visible="close"
    >
        <div v-if="loading" class="history-state">
            <i class="pi pi-spin pi-spinner" />
            <span>กำลังโหลดประวัติการแก้ไข</span>
        </div>

        <Message v-else-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>

        <div v-else-if="!entries.length" class="history-state">
            <i class="pi pi-info-circle" />
            <span>ไม่พบประวัติการแก้ไขหลังจากเริ่มงานเอกสารนี้</span>
        </div>

        <div v-else class="history-list">
            <Message severity="info" :closable="false" class="history-note">
                ข้อมูลนี้อ่านจากประวัติของ SML โดยตรง (เมนูประวัติใน SML ERP) จึงตรงกับที่ตรวจสอบได้ในระบบ SML
            </Message>

            <article v-for="entry in entries" :key="entry.roworder" class="history-entry">
                <header class="entry-header">
                    <Tag :value="actionLabel(entry)" :severity="actionSeverity(entry)" />
                    <span class="entry-user">{{ editorLabel(entry) }}</span>
                    <span class="entry-time">{{ entry.dateTime }}</span>
                </header>

                <p v-if="entry.menuName" class="entry-menu">เมนู: {{ entry.menuName }}</p>

                <p v-if="entry.diffSkipped" class="entry-skipped">{{ entry.diffSkipped }}</p>

                <table v-else-if="entry.changes?.length" class="change-table">
                    <thead>
                        <tr>
                            <th>รายการ</th>
                            <th>ค่าเดิม</th>
                            <th>ค่าใหม่</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr v-for="(change, index) in entry.changes" :key="`${entry.roworder}-${index}`">
                            <td>
                                <span class="change-label">{{ change.label || change.field }}</span>
                                <span v-if="change.rowLabel" class="change-row">{{ change.rowLabel }}</span>
                            </td>
                            <td class="value-old">{{ displayValue(change.old) }}</td>
                            <td class="value-new">{{ displayValue(change.new) }}</td>
                        </tr>
                    </tbody>
                </table>

                <p v-else-if="entry.action === 'created'" class="entry-note">
                    เอกสารเลขที่นี้ถูกสร้างใหม่ทับของเดิมใน SML
                </p>
                <p v-else class="entry-note">บันทึกซ้ำโดยไม่มีการเปลี่ยนแปลงข้อมูล</p>
            </article>

            <Message v-if="truncated" severity="warn" :closable="false">
                แสดงเฉพาะรายการล่าสุด หากต้องการดูทั้งหมดกรุณาตรวจสอบในเมนูประวัติของ SML ERP
            </Message>
        </div>

        <template #footer>
            <Button label="ปิด" severity="secondary" outlined @click="close" />
        </template>
    </Dialog>
</template>

<style scoped>
.history-state {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 1.5rem 0;
    color: var(--text-color-secondary);
}

.history-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.history-note {
    margin: 0;
}

.history-entry {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    padding: 0.875rem;
}

.entry-header {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    flex-wrap: wrap;
}

.entry-user {
    font-weight: 600;
}

.entry-time,
.entry-menu {
    color: var(--text-color-secondary);
    font-size: 0.875rem;
}

.entry-menu,
.entry-note,
.entry-skipped {
    margin: 0.5rem 0 0;
}

.entry-note {
    color: var(--text-color-secondary);
    font-size: 0.875rem;
}

.entry-skipped {
    color: var(--yellow-700);
    font-size: 0.875rem;
}

.change-table {
    width: 100%;
    border-collapse: collapse;
    margin-top: 0.75rem;
    font-size: 0.875rem;
}

.change-table th,
.change-table td {
    border-bottom: 1px solid var(--surface-border);
    padding: 0.5rem;
    text-align: left;
    vertical-align: top;
}

.change-table th {
    color: var(--text-color-secondary);
    font-weight: 600;
}

.change-label {
    display: block;
    font-weight: 600;
}

.change-row {
    display: block;
    color: var(--text-color-secondary);
    font-size: 0.8125rem;
}

.value-old {
    color: var(--red-600);
    text-decoration: line-through;
}

.value-new {
    color: var(--green-700);
    font-weight: 600;
}

/* The change table is the one element allowed to scroll sideways on a phone;
   the dialog itself must never scroll horizontally. */
@media (max-width: 640px) {
    .change-table {
        display: block;
        overflow-x: auto;
        white-space: nowrap;
    }
}
</style>
