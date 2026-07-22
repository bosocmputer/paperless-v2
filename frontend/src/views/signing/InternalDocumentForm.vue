<script setup>
import { api } from '@/services/api';
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';

const route = useRoute();
const router = useRouter();
const toast = useToast();
const loading = ref(false);
const saving = ref(false);
const dirty = ref(false);
const masters = ref([]);
const internalDocument = ref(null);
const idempotencyKey = ref(crypto.randomUUID?.() || `internal-${Date.now()}-${Math.random()}`);
const form = reactive({
    masterId: '',
    documentDate: new Date(),
    requiredDate: new Date(),
    requesterName: '',
    positionName: '',
    departmentName: '',
    purpose: '',
    items: [newItem()]
});

const isEdit = computed(() => Boolean(route.params.id));
const pageTitle = computed(() => (isEdit.value ? `แก้ไข ${internalDocument.value?.documentNo || 'เอกสารภายใน'}` : 'สร้างเอกสารภายใน'));
const activeMasters = computed(() => masters.value.filter((item) => item.status === 'active' && item.workflowReady && item.templateReady));
const masterOptions = computed(() => {
    if (!isEdit.value) return activeMasters.value;
    const selected = masters.value.find((item) => item.id === form.masterId);
    return selected ? [selected] : [];
});
const selectedMaster = computed(() => masters.value.find((item) => item.id === form.masterId));
const totalAmount = computed(() => form.items.reduce((total, item) => total + Number(item.amount || 0), 0));

onMounted(() => {
    window.addEventListener('beforeunload', beforeUnload);
    void loadPage();
});
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));

onBeforeRouteLeave(() => {
    if (!dirty.value || saving.value) return true;
    return window.confirm('ข้อมูลที่แก้ไขยังไม่ได้บันทึก ต้องการออกจากหน้านี้หรือไม่?');
});

function newItem() {
    return { clientKey: crypto.randomUUID?.() || `row-${Date.now()}-${Math.random()}`, description: '', amount: null };
}

function beforeUnload(event) {
    if (!dirty.value || saving.value) return;
    event.preventDefault();
    event.returnValue = '';
}

async function loadPage() {
    loading.value = true;
    try {
        const masterResult = await api.listInternalDocumentMasters();
        masters.value = masterResult.masters || [];
        if (isEdit.value) {
            const result = await api.getInternalDocument(route.params.id);
            internalDocument.value = result.internalDocument;
            const document = result.internalDocument;
            Object.assign(form, {
                masterId: document.masterId,
                documentDate: toDate(document.documentDate),
                requiredDate: toDate(document.requiredDate),
                requesterName: document.requesterName || '',
                positionName: document.positionName || '',
                departmentName: document.departmentName || '',
                purpose: document.purpose || '',
                items: (document.items || []).map((item) => ({ clientKey: item.id || crypto.randomUUID(), description: item.description, amount: Number(item.amount) }))
            });
        } else if (activeMasters.value.length) {
            form.masterId = activeMasters.value[0].id;
        }
        dirty.value = false;
    } catch (error) {
        toast.add({ severity: 'error', summary: 'โหลดแบบฟอร์มไม่สำเร็จ', detail: error.message, life: 4500 });
    } finally {
        loading.value = false;
    }
}

function toDate(value) {
    const parsed = value ? new Date(`${value}T12:00:00`) : new Date();
    return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
}

function formatAPIDate(value) {
    if (!(value instanceof Date) || Number.isNaN(value.getTime())) return '';
    return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}`;
}

function markDirty() {
    dirty.value = true;
}

function addItem() {
    if (form.items.length >= 100) return;
    form.items.push(newItem());
    markDirty();
}

function removeItem(index) {
    if (form.items.length === 1) {
        form.items[0].description = '';
        form.items[0].amount = null;
    } else {
        form.items.splice(index, 1);
    }
    markDirty();
}

function validateForm() {
    if (!form.masterId) return 'กรุณาเลือกประเภทเอกสาร';
    if (!form.requesterName.trim()) return 'กรุณากรอกชื่อผู้ขอเบิก';
    if (!form.purpose.trim()) return 'กรุณากรอกวัตถุประสงค์';
    const invalidIndex = form.items.findIndex((item) => !String(item.description || '').trim() || Number(item.amount || 0) <= 0);
    if (invalidIndex >= 0) return `กรุณาตรวจสอบรายการที่ ${invalidIndex + 1}`;
    return '';
}

async function saveDocument() {
    const issue = validateForm();
    if (issue) {
        toast.add({ severity: 'warn', summary: 'ข้อมูลยังไม่ครบ', detail: issue, life: 3200 });
        return;
    }
    saving.value = true;
    try {
        const payload = {
            masterId: form.masterId,
            documentDate: formatAPIDate(form.documentDate),
            requiredDate: formatAPIDate(form.requiredDate),
            requesterName: form.requesterName,
            positionName: form.positionName,
            departmentName: form.departmentName,
            purpose: form.purpose,
            items: form.items.map((item, index) => ({ sequenceNo: index + 1, description: item.description, amount: Number(item.amount).toFixed(2) }))
        };
        let result;
        if (isEdit.value) {
            result = await api.updateInternalDocument(internalDocument.value.id, { ...payload, revision: internalDocument.value.revision });
            internalDocument.value = result.internalDocument;
        } else {
            result = await api.createInternalDocument(payload, idempotencyKey.value);
        }
        dirty.value = false;
        const signingDocumentId = result.signingDocument?.id || result.signingDocumentId || result.internalDocument?.signingDocumentId;
        toast.add({ severity: 'success', summary: isEdit.value ? 'สร้าง PDF revision ใหม่แล้ว' : 'สร้าง Draft และ PDF แล้ว', life: 2800 });
        if (signingDocumentId) router.push({ name: 'signing-document-detail', params: { id: signingDocumentId }, query: { from_queue: 'draft' } });
        else router.push({ name: 'signing-document-drafts' });
    } catch (error) {
        const issues = Array.isArray(error.payload?.issues) ? error.payload.issues.join(' · ') : error.message;
        toast.add({ severity: 'error', summary: 'บันทึกเอกสารไม่สำเร็จ', detail: issues, life: 5000 });
    } finally {
        saving.value = false;
    }
}

function formatMoney(value) {
    return Number(value || 0).toLocaleString('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}
</script>

<template>
    <div class="internal-document-form" :class="{ loading }">
        <div class="form-toolbar">
            <Button icon="pi pi-arrow-left" text rounded aria-label="กลับ" @click="router.push({ name: 'signing-document-drafts' })" />
            <div class="toolbar-title">
                <strong>{{ pageTitle }}</strong>
                <small v-if="isEdit">Revision {{ internalDocument?.revision || '-' }} · วันที่และเลขที่เอกสารถูกล็อกแล้ว</small>
                <small v-else>กรอกข้อมูลครั้งเดียว ระบบจะสร้าง PDF และ Draft ให้อัตโนมัติ</small>
            </div>
            <Tag v-if="selectedMaster" :value="selectedMaster.code" severity="info" />
            <Button :label="isEdit ? 'บันทึก revision ใหม่' : 'บันทึกและสร้าง Draft'" icon="pi pi-check" :loading="saving" :disabled="loading || (!isEdit && activeMasters.length === 0)" @click="saveDocument" />
        </div>

        <Message v-if="!loading && activeMasters.length === 0 && !isEdit" severity="warn" :closable="false">
            ยังไม่มี Master ที่พร้อมใช้งาน กรุณาให้ Superadmin เปิดใช้งาน Master หลังตั้งค่า Workflow และกรอบลายเซ็นแล้ว
        </Message>

        <section class="form-section">
            <div class="section-head"><h2>ข้อมูลเอกสาร</h2><span>ข้อมูลบริษัทจะดึงจาก SML และ snapshot ตอนบันทึก</span></div>
            <div class="form-grid">
                <label class="span-2">ประเภทเอกสาร
                    <Select v-model="form.masterId" :options="masterOptions" optionLabel="name" optionValue="id" :disabled="isEdit" placeholder="เลือกประเภทเอกสาร" class="w-full" @change="markDirty">
                        <template #option="{ option }"><div class="master-option"><strong>{{ option.code }}</strong><span>{{ option.name }}</span></div></template>
                    </Select>
                </label>
                <label>วันที่เอกสาร<DatePicker v-model="form.documentDate" dateFormat="dd/mm/yy" showIcon :disabled="isEdit" fluid @date-select="markDirty" /></label>
                <label>วันที่ต้องการใช้เงิน<DatePicker v-model="form.requiredDate" dateFormat="dd/mm/yy" showIcon fluid @date-select="markDirty" /></label>
                <label>ผู้ขอเบิก<InputText v-model="form.requesterName" maxlength="160" placeholder="กรอกชื่อผู้ขอเบิก" @input="markDirty" /></label>
                <label>ตำแหน่ง<InputText v-model="form.positionName" maxlength="120" placeholder="ตำแหน่ง" @input="markDirty" /></label>
                <label class="span-2">ส่วนงาน / ฝ่าย / แผนก<InputText v-model="form.departmentName" maxlength="160" placeholder="ส่วนงาน / ฝ่าย / แผนก" @input="markDirty" /></label>
                <label class="span-2">วัตถุประสงค์<Textarea v-model="form.purpose" rows="3" maxlength="1000" autoResize placeholder="ระบุวัตถุประสงค์" @input="markDirty" /></label>
            </div>
        </section>

        <section class="form-section items-section">
            <div class="section-head">
                <div><h2>รายการค่าใช้จ่าย</h2><span>สูงสุด 100 รายการ</span></div>
                <Button label="เพิ่มรายการ" icon="pi pi-plus" severity="secondary" outlined :disabled="form.items.length >= 100" @click="addItem" />
            </div>
            <DataTable :value="form.items" dataKey="clientKey" responsiveLayout="scroll" class="items-table">
                <Column header="ลำดับ" style="width: 5rem"><template #body="{ index }"><span class="sequence">{{ index + 1 }}</span></template></Column>
                <Column header="รายการ" style="min-width: 24rem"><template #body="{ data }"><InputText v-model="data.description" maxlength="500" placeholder="รายละเอียดรายการ" class="w-full" @input="markDirty" /></template></Column>
                <Column header="จำนวนเงิน (บาท)" style="min-width: 13rem"><template #body="{ data }"><InputNumber v-model="data.amount" mode="decimal" :min="0" :maxFractionDigits="2" :minFractionDigits="2" fluid @input="markDirty" /></template></Column>
                <Column header="" style="width: 5rem"><template #body="{ index }"><Button icon="pi pi-trash" rounded text severity="danger" aria-label="ลบรายการ" @click="removeItem(index)" /></template></Column>
                <template #footer><div class="total-row"><span>รวมทั้งสิ้น</span><strong>{{ formatMoney(totalAmount) }} บาท</strong></div></template>
            </DataTable>
        </section>
    </div>
</template>

<style scoped>
.internal-document-form { display: grid; gap: .8rem; }
.form-toolbar { min-height: 4rem; display: flex; align-items: center; gap: .75rem; padding: .7rem .8rem; background: var(--surface-card); border: 1px solid var(--surface-border); border-radius: 8px; position: sticky; top: 0; z-index: 3; }
.toolbar-title { min-width: 0; flex: 1; display: grid; gap: .1rem; }
.toolbar-title strong { font-size: 1.1rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.toolbar-title small, .section-head span { color: var(--text-color-secondary); }
.form-section { background: var(--surface-card); border: 1px solid var(--surface-border); border-radius: 8px; padding: 1rem; }
.section-head { display: flex; align-items: center; justify-content: space-between; gap: 1rem; margin-bottom: 1rem; }
.section-head h2 { font-size: 1rem; margin: 0; }
.section-head > div { display: grid; gap: .15rem; }
.form-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 1rem; }
.form-grid label { min-width: 0; display: grid; gap: .4rem; font-weight: 600; }
.form-grid .span-2 { grid-column: span 2; }
.master-option { display: flex; gap: .6rem; }
.master-option strong { min-width: 4.5rem; }
.sequence { display: inline-grid; width: 2rem; height: 2rem; place-items: center; border-radius: 50%; background: var(--surface-ground); font-weight: 700; }
.total-row { display: flex; justify-content: flex-end; align-items: baseline; gap: 1rem; padding-right: 5rem; }
.total-row strong { font-size: 1.15rem; color: var(--primary-color); }
@media (max-width: 1100px) { .form-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 680px) { .form-toolbar { align-items: flex-start; flex-wrap: wrap; } .form-toolbar > :last-child { width: 100%; } .form-grid { grid-template-columns: 1fr; } .form-grid .span-2 { grid-column: span 1; } .section-head { align-items: flex-start; } .total-row { padding-right: 0; justify-content: space-between; } }
</style>
