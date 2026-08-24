<script setup>
import { api } from '@/services/api';
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const confirm = useConfirm();
const toast = useToast();

// Mirrors backend/internal/store/user_permissions.go's grantableMenuKeys
// exactly - keep both lists in sync if a menu is ever added/removed.
const GRANTABLE_MENUS = [
    { key: 'signing-document-drafts', label: 'เอกสารเตรียมส่ง' },
    { key: 'internal-document-create', label: 'สร้างเอกสารภายใน' },
    { key: 'signing-documents', label: 'เอกสารรอเซ็น' },
    { key: 'signing-document-history', label: 'ประวัติเอกสาร' },
    { key: 'admin-my-signing-tasks', label: 'งานรอเซ็นของฉัน' },
    { key: 'admin-my-signing-history', label: 'ประวัติการเซ็นของฉัน' },
    { key: 'admin-guide', label: 'คู่มือการใช้งาน' },
    { key: 'admin-user-guide', label: 'คู่มือผู้เซ็น' }
];

const users = ref([]);
const loading = ref(false);
const savingAll = ref(false);
const error = ref('');
const searchQuery = ref('');
// rowId -> { menuKeys: Set<string>, documentScope: 'all'|'own', updatedAt: string|null, configured: boolean, _savedMenuKeys: string[], _savedDocumentScope: string }
const edits = reactive({});

const filteredUsers = computed(() => {
    const query = normalizeSearch(searchQuery.value);
    if (!query) return users.value;
    return users.value.filter((user) => normalizeSearch(`${user.displayName} ${user.username} ${user.role}`).includes(query));
});

function isRowDirty(userId) {
    const row = edits[userId];
    if (!row) return false;
    const savedKeys = new Set(row._savedMenuKeys || []);
    const currentKeys = row.menuKeys;
    if (savedKeys.size !== currentKeys.size) return true;
    if (row.documentScope !== (row._savedDocumentScope || 'all')) return true;
    for (const key of currentKeys) {
        if (!savedKeys.has(key)) return true;
    }
    return false;
}

const dirtyUsers = computed(() => users.value.filter((user) => isRowDirty(user.id)));
const hasUnsavedChanges = computed(() => dirtyUsers.value.length > 0);

onMounted(() => {
    window.addEventListener('beforeunload', beforeUnload);
    void loadAll();
});
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));

onBeforeRouteLeave(() => {
    if (!hasUnsavedChanges.value || savingAll.value) return true;
    return window.confirm('มีการแก้ไขสิทธิ์ที่ยังไม่ได้บันทึก ต้องการออกจากหน้านี้หรือไม่?');
});

function beforeUnload(event) {
    if (!hasUnsavedChanges.value || savingAll.value) return;
    event.preventDefault();
    event.returnValue = '';
}

async function loadAll() {
    loading.value = true;
    error.value = '';
    try {
        const [usersResult, permsResult] = await Promise.all([api.listUsers(), api.listAllUserMenuPermissions()]);
        const eligible = (usersResult.users || []).filter((user) => user.role !== 'superadmin' && user.status === 'active');
        users.value = eligible;
        const permsByUserId = permsResult.permissions || {};
        for (const user of eligible) {
            const perm = permsByUserId[user.id];
            edits[user.id] = {
                menuKeys: new Set(perm?.menuKeys || []),
                documentScope: perm?.documentScope || 'all',
                updatedAt: perm?.updatedAt || null,
                configured: Boolean(perm?.configured),
                _savedMenuKeys: perm?.menuKeys || [],
                _savedDocumentScope: perm?.documentScope || 'all'
            };
        }
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลดข้อมูลสิทธิ์ไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loading.value = false;
    }
}

function isChecked(userId, menuKey) {
    return edits[userId]?.menuKeys?.has(menuKey) === true;
}

function toggleMenu(userId, menuKey) {
    const row = edits[userId];
    if (!row || savingAll.value) return;
    if (row.menuKeys.has(menuKey)) row.menuKeys.delete(menuKey);
    else row.menuKeys.add(menuKey);
}

// Standard "select all" header checkbox for one menu column, tri-state
// (checked / unchecked / indeterminate) reflecting the currently-visible
// rows - the recognizable spreadsheet/admin-table pattern, rather than a
// per-row control mixed into the name cell.
function columnState(menuKey) {
    const rows = filteredUsers.value.map((user) => edits[user.id]).filter(Boolean);
    if (!rows.length) return { checked: false, indeterminate: false };
    const checkedCount = rows.filter((row) => row.menuKeys.has(menuKey)).length;
    return {
        checked: checkedCount === rows.length,
        indeterminate: checkedCount > 0 && checkedCount < rows.length
    };
}

// Toggles one menu column across every currently-visible (filtered) user's
// IN-MEMORY selection only - "บันทึกทั้งหมด" still needs to be clicked to
// persist any change made here or via the per-cell checkboxes.
function toggleColumn(menuKey) {
    if (savingAll.value) return;
    const rows = filteredUsers.value.map((user) => edits[user.id]).filter(Boolean);
    if (!rows.length) return;
    const { checked } = columnState(menuKey);
    for (const row of rows) {
        if (checked) row.menuKeys.delete(menuKey);
        else row.menuKeys.add(menuKey);
    }
}

function saveAll() {
    if (savingAll.value || !dirtyUsers.value.length) return;

    const isPureRevocationOverall = dirtyUsers.value.some((user) => {
        const row = edits[user.id];
        if (!row?.configured) return false;
        const previousKeys = new Set(row._savedMenuKeys || []);
        const newKeys = row.menuKeys;
        for (const key of newKeys) {
            if (!previousKeys.has(key)) return false;
        }
        return newKeys.size < previousKeys.size;
    });

    if (isPureRevocationOverall) {
        confirm.require({
            message: `บันทึกจะลบสิทธิ์เมนูบางส่วนของผู้ใช้บางคน (${dirtyUsers.value.length} คนที่แก้ไข) ยืนยันหรือไม่?`,
            header: 'ยืนยันลดสิทธิ์',
            icon: 'pi pi-exclamation-triangle',
            rejectProps: { label: 'ยกเลิก', severity: 'secondary', outlined: true },
            acceptProps: { label: 'ยืนยัน', severity: 'danger' },
            accept: submitAll
        });
        return;
    }
    submitAll();
}

async function submitAll() {
    savingAll.value = true;
    const targets = dirtyUsers.value;
    let succeeded = 0;
    let conflicted = 0;
    let failed = 0;

    for (const user of targets) {
        const row = edits[user.id];
        if (!row) continue;
        try {
            const payload = {
                menuKeys: Array.from(row.menuKeys),
                documentScope: row.documentScope,
                ...(row.configured ? { expectedUpdatedAt: row.updatedAt } : {})
            };
            const result = await api.setUserMenuPermissions(user.id, payload);
            const perm = result.permissions || {};
            row.updatedAt = perm.updatedAt || null;
            row.configured = true;
            row._savedMenuKeys = Array.from(row.menuKeys);
            row._savedDocumentScope = row.documentScope;
            succeeded += 1;
        } catch (err) {
            if (err.status === 409) {
                conflicted += 1;
                await reloadOneUser(user);
            } else {
                failed += 1;
            }
        }
    }

    savingAll.value = false;

    if (!conflicted && !failed) {
        toast.add({ severity: 'success', summary: `บันทึกสิทธิ์แล้ว ${succeeded} คน`, life: 2500 });
        return;
    }
    const parts = [];
    if (succeeded) parts.push(`บันทึกสำเร็จ ${succeeded} คน`);
    if (conflicted) parts.push(`มีคนแก้ไขไปแล้ว ${conflicted} คน (โหลดข้อมูลล่าสุดให้แล้ว กรุณาตรวจสอบและบันทึกอีกครั้ง)`);
    if (failed) parts.push(`บันทึกไม่สำเร็จ ${failed} คน`);
    toast.add({ severity: conflicted || failed ? 'warn' : 'success', summary: 'ผลการบันทึก', detail: parts.join(' · '), life: 5500 });
}

async function reloadOneUser(user) {
    try {
        const result = await api.getUserMenuPermissions(user.id);
        const perm = result.permissions || {};
        edits[user.id] = {
            menuKeys: new Set(perm.menuKeys || []),
            documentScope: perm.documentScope || 'all',
            updatedAt: perm.updatedAt || null,
            configured: Boolean(perm.configured),
            _savedMenuKeys: perm.menuKeys || [],
            _savedDocumentScope: perm.documentScope || 'all'
        };
    } catch (err) {
        toast.add({ severity: 'error', summary: 'โหลดข้อมูลล่าสุดไม่สำเร็จ', detail: err.message, life: 3500 });
    }
}

function normalizeSearch(value) {
    return String(value || '').toLowerCase().trim();
}

function roleSeverity(role) {
    return role === 'admin' ? 'success' : 'info';
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div>
                <div class="font-semibold text-xl mb-1">สิทธิ์การเข้าถึงเมนู</div>
                <p class="text-muted-color m-0">กำหนดว่าแต่ละผู้ใช้ (admin/user) เข้าเมนูไหนได้บ้าง และเห็นเอกสารทั้งหมดหรือเฉพาะของตัวเอง</p>
            </div>
            <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหา user หรือชื่อ" class="w-full sm:w-80" />
                <Button
                    :label="hasUnsavedChanges ? `บันทึกทั้งหมด (${dirtyUsers.length})` : 'บันทึกทั้งหมด'"
                    icon="pi pi-save"
                    :loading="savingAll"
                    :disabled="!hasUnsavedChanges"
                    @click="saveAll"
                />
            </div>
        </div>

        <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

        <div v-if="loading" class="py-10 text-center text-muted-color">
            <ProgressSpinner aria-label="กำลังโหลด" style="width: 2.5rem; height: 2.5rem" />
        </div>

        <div v-else class="permission-matrix-scroll">
            <table class="permission-matrix">
                <thead>
                    <tr>
                        <th class="sticky-col">ผู้ใช้</th>
                        <th v-for="menu in GRANTABLE_MENUS" :key="menu.key" class="text-center">
                            <div class="column-header">
                                <Checkbox
                                    :modelValue="columnState(menu.key).checked"
                                    :indeterminate="columnState(menu.key).indeterminate"
                                    binary
                                    :aria-label="`เลือกทั้งหมด: ${menu.label}`"
                                    @update:modelValue="toggleColumn(menu.key)"
                                />
                                <span>{{ menu.label }}</span>
                            </div>
                        </th>
                        <th>ขอบเขตเอกสาร</th>
                    </tr>
                </thead>
                <tbody>
                    <tr v-for="user in filteredUsers" :key="user.id" :class="{ unconfigured: !edits[user.id]?.configured, dirty: isRowDirty(user.id) }">
                        <td class="sticky-col">
                            <div class="font-medium">{{ user.displayName }}</div>
                            <div class="flex items-center gap-2">
                                <span class="text-sm text-muted-color">@{{ user.username }}</span>
                                <Tag :value="user.role" :severity="roleSeverity(user.role)" />
                                <Tag v-if="isRowDirty(user.id)" value="ยังไม่บันทึก" severity="warn" />
                            </div>
                            <small v-if="!edits[user.id]?.configured" class="text-muted-color">ยังไม่เคยกำหนด (สิทธิ์เต็มตามค่าเริ่มต้น)</small>
                        </td>
                        <td v-for="menu in GRANTABLE_MENUS" :key="menu.key" class="text-center">
                            <Checkbox
                                :modelValue="isChecked(user.id, menu.key)"
                                binary
                                :disabled="savingAll"
                                @update:modelValue="toggleMenu(user.id, menu.key)"
                            />
                        </td>
                        <td>
                            <Select
                                v-if="edits[user.id]"
                                v-model="edits[user.id].documentScope"
                                :disabled="user.role !== 'admin' || savingAll"
                                :options="[
                                    { label: 'ทั้งหมด', value: 'all' },
                                    { label: 'เฉพาะที่เซ็นเอง', value: 'own' }
                                ]"
                                optionLabel="label"
                                optionValue="value"
                                class="w-full"
                            />
                        </td>
                    </tr>
                </tbody>
            </table>
            <Message v-if="!filteredUsers.length" severity="info" class="mt-4">{{ searchQuery ? 'ไม่พบผู้ใช้ที่ค้นหา' : 'ยังไม่มีผู้ใช้ (admin/user) ให้กำหนดสิทธิ์' }}</Message>
        </div>
    </div>
</template>

<style scoped>
.permission-matrix-scroll {
    overflow: auto;
    max-height: min(64vh, 42rem);
    border: 1px solid var(--surface-border);
    border-radius: 8px;
}

.permission-matrix {
    width: 100%;
    border-collapse: collapse;
    min-width: 60rem;
}

.permission-matrix th,
.permission-matrix td {
    padding: 0.65rem 0.75rem;
    border-bottom: 1px solid var(--surface-border);
    text-align: left;
    vertical-align: middle;
    white-space: nowrap;
}

.column-header {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
}

.permission-matrix th {
    font-size: 0.85rem;
    color: var(--text-color-secondary);
    font-weight: 600;
    background: var(--surface-card);
}

.permission-matrix thead th {
    position: sticky;
    top: 0;
    z-index: 2;
    box-shadow: inset 0 -1px 0 var(--surface-border);
}

.sticky-col {
    position: sticky;
    left: 0;
    background: var(--surface-card);
    z-index: 1;
}

.permission-matrix thead th.sticky-col {
    z-index: 3;
}

tr.unconfigured .sticky-col {
    background: color-mix(in srgb, var(--surface-ground) 60%, var(--surface-card));
}

tr.dirty {
    background: color-mix(in srgb, var(--yellow-50) 55%, transparent);
}

tr.dirty .sticky-col {
    background: color-mix(in srgb, var(--yellow-50) 70%, var(--surface-card));
}
</style>
