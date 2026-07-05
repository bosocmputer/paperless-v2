<script setup>
import { api } from '@/services/api';
import { computed, onMounted, ref } from 'vue';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const confirm = useConfirm();
const toast = useToast();
const users = ref([]);
const loading = ref(false);
const saving = ref(false);
const dialogVisible = ref(false);
const editingUser = ref(null);
const error = ref('');
const searchQuery = ref('');
const form = ref(emptyForm());

const roleOptions = [
    { label: 'superadmin', value: 'superadmin' },
    { label: 'admin', value: 'admin' },
    { label: 'user', value: 'user' }
];

const statusOptions = [
    { label: 'active', value: 'active' },
    { label: 'inactive', value: 'inactive' }
];

const dialogTitle = computed(() => (editingUser.value ? 'แก้ไขผู้ใช้' : 'เพิ่มผู้ใช้'));
const passwordHint = computed(() => (editingUser.value ? 'เว้นว่างไว้ถ้าไม่ต้องการเปลี่ยนรหัสผ่าน' : 'รหัสผ่านอย่างน้อย 6 ตัวอักษร'));
const filteredUsers = computed(() => {
    const query = normalizeSearch(searchQuery.value);
    if (!query) return users.value;
    return users.value.filter((user) =>
        normalizeSearch(`${user.displayName} ${user.username} ${user.role} ${user.status}`).includes(query)
    );
});

onMounted(loadUsers);

function emptyForm() {
    return {
        displayName: '',
        username: '',
        password: '',
        role: 'user',
        status: 'active'
    };
}

async function loadUsers() {
    loading.value = true;
    error.value = '';
    try {
        const result = await api.listUsers();
        users.value = result.users || [];
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'โหลดผู้ใช้ไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        loading.value = false;
    }
}

function openCreate() {
    editingUser.value = null;
    form.value = emptyForm();
    dialogVisible.value = true;
}

function openEdit(user) {
    editingUser.value = user;
    form.value = {
        displayName: user.displayName,
        username: user.username,
        password: '',
        role: user.role,
        status: user.status
    };
    dialogVisible.value = true;
}

function closeDialog() {
    if (saving.value) return;
    dialogVisible.value = false;
}

async function saveUser() {
    saving.value = true;
    error.value = '';
    try {
        const payload = { ...form.value };
        if (editingUser.value && !payload.password) delete payload.password;

        if (editingUser.value) {
            await api.updateUser(editingUser.value.id, payload);
            toast.add({ severity: 'success', summary: 'บันทึกผู้ใช้แล้ว', life: 2500 });
        } else {
            await api.createUser(payload);
            toast.add({ severity: 'success', summary: 'เพิ่มผู้ใช้แล้ว', life: 2500 });
        }

        dialogVisible.value = false;
        await loadUsers();
    } catch (err) {
        error.value = err.message;
        toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: err.message, life: 3500 });
    } finally {
        saving.value = false;
    }
}

function confirmDeactivate(user) {
    confirm.require({
        message: `ปิดใช้งาน ${user.displayName} ใช่ไหม?`,
        header: 'ปิดใช้งานผู้ใช้',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: {
            label: 'ยกเลิก',
            severity: 'secondary',
            outlined: true
        },
        acceptProps: {
            label: 'ปิดใช้งาน',
            severity: 'danger'
        },
        accept: () => deactivateUser(user)
    });
}

async function deactivateUser(user) {
    try {
        await api.deactivateUser(user.id);
        toast.add({ severity: 'success', summary: 'ปิดใช้งานผู้ใช้แล้ว', life: 2500 });
        await loadUsers();
    } catch (err) {
        toast.add({ severity: 'error', summary: 'ปิดใช้งานไม่สำเร็จ', detail: err.message, life: 3500 });
    }
}

function formatDate(value) {
    if (!value) return '-';
    return new Intl.DateTimeFormat('th-TH', {
        dateStyle: 'medium',
        timeStyle: 'short'
    }).format(new Date(value));
}

function roleSeverity(role) {
    if (role === 'superadmin') return 'danger';
    return role === 'admin' ? 'success' : 'info';
}

function statusSeverity(status) {
    return status === 'active' ? 'success' : 'secondary';
}

function normalizeSearch(value) {
    return String(value || '').toLowerCase().trim();
}
</script>

<template>
    <div class="card">
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div>
                <div class="font-semibold text-xl mb-1">ผู้ใช้งาน</div>
                <p class="text-muted-color m-0">จัดการชื่อผู้ใช้ รหัสผ่าน และระดับสิทธิ์ superadmin/admin/user</p>
            </div>
            <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                <InputText v-model="searchQuery" type="search" placeholder="ค้นหา user, ชื่อ, สิทธิ์" class="w-full sm:w-72" />
                <Button label="เพิ่มผู้ใช้" icon="pi pi-plus" @click="openCreate" />
            </div>
        </div>

        <Message v-if="error && !dialogVisible" severity="error" class="mb-4">{{ error }}</Message>

        <DataTable :value="filteredUsers" :loading="loading" dataKey="id" paginator :rows="10" responsiveLayout="scroll" stripedRows>
            <template #empty>
                <div class="py-6 text-center text-muted-color">{{ searchQuery ? 'ไม่พบผู้ใช้ที่ค้นหา' : 'ยังไม่มีผู้ใช้' }}</div>
            </template>
            <Column field="displayName" header="ชื่อ" sortable>
                <template #body="{ data }">
                    <div class="font-medium text-surface-900 dark:text-surface-0">{{ data.displayName }}</div>
                    <div class="text-sm text-muted-color">@{{ data.username }}</div>
                </template>
            </Column>
            <Column field="username" header="Username" sortable />
            <Column field="role" header="ระดับ" sortable>
                <template #body="{ data }">
                    <Tag :value="data.role" :severity="roleSeverity(data.role)" />
                </template>
            </Column>
            <Column field="status" header="สถานะ" sortable>
                <template #body="{ data }">
                    <Tag :value="data.status" :severity="statusSeverity(data.status)" />
                </template>
            </Column>
            <Column field="createdAt" header="วันที่สร้าง" sortable>
                <template #body="{ data }">{{ formatDate(data.createdAt) }}</template>
            </Column>
            <Column header="จัดการ" style="width: 10rem">
                <template #body="{ data }">
                    <div class="flex gap-2">
                        <Button icon="pi pi-pencil" severity="secondary" rounded outlined aria-label="แก้ไขผู้ใช้" @click="openEdit(data)" />
                        <Button icon="pi pi-ban" severity="danger" rounded outlined aria-label="ปิดใช้งานผู้ใช้" :disabled="data.status !== 'active'" @click="confirmDeactivate(data)" />
                    </div>
                </template>
            </Column>
        </DataTable>
    </div>

    <Dialog v-model:visible="dialogVisible" modal :header="dialogTitle" :style="{ width: 'min(42rem, 92vw)' }" @hide="closeDialog">
        <form class="flex flex-col gap-4" @submit.prevent="saveUser">
            <Message v-if="error && dialogVisible" severity="error">{{ error }}</Message>

            <div class="flex flex-col gap-2">
                <label for="displayName" class="font-medium">ชื่อ</label>
                <InputText id="displayName" v-model="form.displayName" autocomplete="name" />
            </div>

            <div class="flex flex-col gap-2">
                <label for="username" class="font-medium">Username</label>
                <InputText id="username" v-model="form.username" autocomplete="username" />
            </div>

            <div class="flex flex-col gap-2">
                <label for="password" class="font-medium">Password</label>
                <Password id="password" v-model="form.password" :feedback="false" toggleMask fluid autocomplete="new-password" />
                <small class="text-muted-color">{{ passwordHint }}</small>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div class="flex flex-col gap-2">
                    <label for="role" class="font-medium">ระดับ</label>
                    <Select id="role" v-model="form.role" :options="roleOptions" optionLabel="label" optionValue="value" />
                </div>
                <div class="flex flex-col gap-2">
                    <label for="status" class="font-medium">สถานะ</label>
                    <Select id="status" v-model="form.status" :options="statusOptions" optionLabel="label" optionValue="value" />
                </div>
            </div>

            <div class="flex justify-end gap-2 pt-2">
                <Button type="button" label="ยกเลิก" severity="secondary" outlined @click="closeDialog" />
                <Button type="submit" label="บันทึกผู้ใช้" icon="pi pi-save" :loading="saving" />
            </div>
        </form>
    </Dialog>
</template>
