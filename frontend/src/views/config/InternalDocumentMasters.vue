<script setup>
import { api } from '@/services/api';
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';

const router = useRouter();
const confirm = useConfirm();
const toast = useToast();
const masters = ref([]);
const loading = ref(false);
const saving = ref(false);
const dialogVisible = ref(false);
const editing = ref(null);
const search = ref('');
const form = reactive(emptyForm());

const filteredMasters = computed(() => {
    const query = search.value.trim().toLocaleLowerCase('th');
    if (!query) return masters.value;
    return masters.value.filter((item) => `${item.code} ${item.name} ${item.prefix} ${item.runningPattern}`.toLocaleLowerCase('th').includes(query));
});
const runningPreview = computed(() => {
    const date = new Date();
    const yyyy = String(date.getFullYear());
    const yy = yyyy.slice(-2);
    const mm = String(date.getMonth() + 1).padStart(2, '0');
    const dd = String(date.getDate()).padStart(2, '0');
    const pattern = String(form.runningPattern || '').toUpperCase().replace(/^@/, '');
    return `${String(form.prefix || '').toUpperCase()}${pattern.replaceAll('YYYY', yyyy).replaceAll('YY', yy).replaceAll('MM', mm).replaceAll('DD', dd).replace(/#+/, (value) => '1'.padStart(value.length, '0'))}`;
});

onMounted(loadMasters);

function emptyForm() {
    return { code: '', name: '', prefix: '', runningPattern: '@YYMMDD-###', status: 'inactive', revision: 0 };
}

async function loadMasters() {
    loading.value = true;
    try {
        const result = await api.listInternalDocumentMasters();
        masters.value = result.masters || [];
    } catch (error) {
        toast.add({ severity: 'error', summary: 'โหลด Master ไม่สำเร็จ', detail: error.message, life: 4000 });
    } finally {
        loading.value = false;
    }
}

function openCreate() {
    editing.value = null;
    Object.assign(form, emptyForm());
    dialogVisible.value = true;
}

function openEdit(master) {
    editing.value = master;
    Object.assign(form, {
        code: master.code,
        name: master.name,
        prefix: master.prefix,
        runningPattern: master.runningPattern,
        status: master.status,
        revision: master.revision
    });
    dialogVisible.value = true;
}

function setActive(value) {
    if (value && editing.value && (!editing.value.workflowReady || !editing.value.templateReady)) {
        toast.add({ severity: 'warn', summary: 'ยังเปิดใช้งานไม่ได้', detail: 'กรุณาตั้งค่า Workflow และกรอบลายเซ็นให้พร้อมก่อน', life: 3500 });
        form.status = 'inactive';
        return;
    }
    form.status = value ? 'active' : 'inactive';
}

async function saveMaster() {
    if (!form.code.trim() || !form.name.trim() || !form.prefix.trim() || !form.runningPattern.trim()) {
        toast.add({ severity: 'warn', summary: 'กรอกข้อมูลไม่ครบ', detail: 'กรุณากรอกชื่อ รหัส Prefix และ Running pattern', life: 3000 });
        return;
    }
    saving.value = true;
    try {
        const payload = {
            code: form.code,
            name: form.name,
            prefix: form.prefix,
            runningPattern: form.runningPattern,
            status: editing.value ? form.status : 'inactive',
            revision: form.revision
        };
        if (editing.value) await api.updateInternalDocumentMaster(editing.value.id, payload);
        else await api.createInternalDocumentMaster(payload);
        dialogVisible.value = false;
        toast.add({ severity: 'success', summary: editing.value ? 'บันทึก Master แล้ว' : 'เพิ่ม Master แล้ว', life: 2500 });
        await loadMasters();
    } catch (error) {
        toast.add({ severity: 'error', summary: 'บันทึก Master ไม่สำเร็จ', detail: error.message, life: 4500 });
    } finally {
        saving.value = false;
    }
}

function confirmDelete(master) {
    confirm.require({
        header: 'ลบ Master เอกสาร',
        message: `ต้องการลบ ${master.code} - ${master.name} ใช่ไหม?`,
        icon: 'pi pi-trash',
        rejectProps: { label: 'ยกเลิก', severity: 'secondary', outlined: true },
        acceptProps: { label: 'ลบ', severity: 'danger' },
        accept: async () => {
            try {
                await api.deleteInternalDocumentMaster(master.id);
                toast.add({ severity: 'success', summary: 'ลบ Master แล้ว', life: 2500 });
                await loadMasters();
            } catch (error) {
                toast.add({ severity: 'error', summary: 'ลบ Master ไม่สำเร็จ', detail: error.message, life: 4000 });
            }
        }
    });
}

function openWorkflow(master) {
    router.push({ name: 'document-config-workflow', params: { docFormatCode: master.code } });
}

function openTemplate(master) {
    router.push({ name: 'signature-template', params: { docFormatCode: master.code } });
}
</script>

<template>
    <div class="card internal-master-page">
        <div class="page-toolbar">
            <div>
                <div class="title-row">
                    <h1>Master เอกสารภายใน</h1>
                    <Tag value="PaperLess" severity="info" />
                </div>
                <p>กำหนดชื่อ รหัส และเลข Running สำหรับเอกสารที่สร้างภายในระบบ</p>
            </div>
            <div class="toolbar-actions">
                <IconField>
                    <InputIcon><i class="pi pi-search" /></InputIcon>
                    <InputText v-model="search" placeholder="ค้นหา Master" />
                </IconField>
                <Button icon="pi pi-refresh" severity="secondary" outlined rounded aria-label="โหลดใหม่" :loading="loading" @click="loadMasters" />
                <Button label="เพิ่ม Master" icon="pi pi-plus" @click="openCreate" />
            </div>
        </div>

        <DataTable :value="filteredMasters" :loading="loading" dataKey="id" stripedRows responsiveLayout="scroll">
            <template #empty><div class="empty-state">ยังไม่มี Master เอกสารภายใน</div></template>
            <Column field="code" header="รหัส" sortable style="min-width: 9rem">
                <template #body="{ data }"><strong>{{ data.code }}</strong></template>
            </Column>
            <Column field="name" header="ชื่อเอกสาร" sortable style="min-width: 16rem" />
            <Column header="เลข Running" style="min-width: 16rem">
                <template #body="{ data }">
                    <div class="running-cell"><strong>{{ data.prefix }}{{ data.runningPattern }}</strong><small>เอกสารแล้ว {{ data.documentCount }} รายการ</small></div>
                </template>
            </Column>
            <Column header="ความพร้อม" style="min-width: 15rem">
                <template #body="{ data }">
                    <div class="readiness-tags">
                        <Tag :value="data.workflowReady ? 'Workflow พร้อม' : 'ยังไม่มี Workflow'" :severity="data.workflowReady ? 'success' : 'warn'" />
                        <Tag :value="data.templateReady ? 'กรอบพร้อม' : 'ยังไม่มีกรอบ'" :severity="data.templateReady ? 'success' : 'warn'" />
                    </div>
                </template>
            </Column>
            <Column field="status" header="สถานะ" sortable style="min-width: 9rem">
                <template #body="{ data }"><Tag :value="data.status === 'active' ? 'เปิดใช้งาน' : 'ปิดใช้งาน'" :severity="data.status === 'active' ? 'success' : 'secondary'" /></template>
            </Column>
            <Column header="จัดการ" :exportable="false" style="min-width: 14rem">
                <template #body="{ data }">
                    <div class="row-actions">
                        <Button icon="pi pi-file-edit" rounded outlined severity="secondary" v-tooltip.top="'ตั้งค่า Workflow'" aria-label="ตั้งค่า Workflow" @click="openWorkflow(data)" />
                        <Button icon="pi pi-map-marker" rounded outlined severity="secondary" v-tooltip.top="'วางกรอบลายเซ็น'" aria-label="วางกรอบลายเซ็น" @click="openTemplate(data)" />
                        <Button icon="pi pi-pencil" rounded outlined severity="secondary" aria-label="แก้ไข" @click="openEdit(data)" />
                        <Button v-if="data.documentCount === 0 && !data.workflowReady && !data.templateReady" icon="pi pi-trash" rounded outlined severity="danger" aria-label="ลบ" @click="confirmDelete(data)" />
                    </div>
                </template>
            </Column>
        </DataTable>
    </div>

    <Dialog v-model:visible="dialogVisible" modal :header="editing ? 'แก้ไข Master เอกสารภายใน' : 'เพิ่ม Master เอกสารภายใน'" :style="{ width: 'min(42rem, 94vw)' }" :draggable="false">
        <div class="master-form">
            <div class="field-grid">
                <label>รหัสเอกสาร<InputText v-model="form.code" maxlength="20" :disabled="Boolean(editing?.documentCount)" placeholder="ADV" /></label>
                <label>ชื่อเอกสาร<InputText v-model="form.name" maxlength="120" placeholder="ใบขอเบิกเงินทดรอง" /></label>
                <label>Prefix<InputText v-model="form.prefix" maxlength="20" :disabled="Boolean(editing?.documentCount)" placeholder="ADV" /></label>
                <label>Running pattern<InputText v-model="form.runningPattern" maxlength="32" :disabled="Boolean(editing?.documentCount)" placeholder="@YYMMDD-###" /></label>
            </div>
            <div class="preview-line"><span>ตัวอย่างเลขเอกสาร</span><strong>{{ runningPreview || '-' }}</strong></div>
            <div v-if="editing" class="status-switch">
                <span><strong>เปิดใช้งาน</strong><small>ต้องมี Workflow และ Active Template พร้อมแล้ว</small></span>
                <ToggleSwitch :modelValue="form.status === 'active'" @update:modelValue="setActive" />
            </div>
            <Message v-else severity="info" class="m-0">Master ใหม่จะเริ่มเป็นปิดใช้งาน เพื่อให้ตั้งค่า Workflow และกรอบลายเซ็นก่อน</Message>
        </div>
        <template #footer>
            <Button label="ยกเลิก" severity="secondary" text @click="dialogVisible = false" />
            <Button label="บันทึก" icon="pi pi-check" :loading="saving" @click="saveMaster" />
        </template>
    </Dialog>
</template>

<style scoped>
.internal-master-page { display: grid; gap: 1rem; min-width: 0; }
.page-toolbar, .toolbar-actions, .title-row, .row-actions, .readiness-tags { display: flex; align-items: center; gap: .65rem; }
.page-toolbar { justify-content: space-between; flex-wrap: wrap; }
.page-toolbar, .page-toolbar > div, :deep(.p-datatable) { min-width: 0; }
.internal-master-page :deep(.p-datatable-table-container) { max-width: 100%; overflow-x: auto; }
.page-toolbar h1 { margin: 0; font-size: 1.35rem; }
.page-toolbar p { margin: .25rem 0 0; color: var(--text-color-secondary); }
.toolbar-actions { flex-wrap: wrap; }
.running-cell { display: grid; gap: .2rem; }
.running-cell small { color: var(--text-color-secondary); }
.readiness-tags { flex-wrap: wrap; }
.empty-state { padding: 2.5rem; text-align: center; color: var(--text-color-secondary); }
.master-form { display: grid; gap: 1rem; }
.field-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
.field-grid label { display: grid; gap: .4rem; font-weight: 600; }
.preview-line, .status-switch { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: .8rem 1rem; border: 1px solid var(--surface-border); border-radius: 8px; background: var(--surface-ground); }
.preview-line strong { color: var(--primary-color); }
.status-switch span { display: grid; gap: .15rem; }
.status-switch small { color: var(--text-color-secondary); }
@media (max-width: 720px) { .field-grid { grid-template-columns: 1fr; } .toolbar-actions, .toolbar-actions :deep(.p-iconfield) { width: 100%; } }
</style>
