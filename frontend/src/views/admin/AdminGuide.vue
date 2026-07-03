<script setup>
import loginCredentialsImage from '@/assets/user-guide/01-login-credentials.png';
import loginDatabaseImage from '@/assets/user-guide/02b-login-database-selected.png';
import dashboardImage from '@/assets/user-guide/03-dashboard.png';
import draftsImage from '@/assets/user-guide/04-documents-drafts.png';
import createDocumentImage from '@/assets/user-guide/05-create-document.png';
import activeDocumentsImage from '@/assets/user-guide/06-documents-active.png';
import historyImage from '@/assets/user-guide/07-documents-history.png';
import detailCurrentImage from '@/assets/user-guide/08-document-detail-current.png';
import evidenceDialogImage from '@/assets/user-guide/09-evidence-dialog.png';
import printActionImage from '@/assets/user-guide/10-print-action-area.png';
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';

const router = useRouter();
const search = ref('');
const selectedShot = ref(null);

const quickLinks = [
    { label: 'ภาพรวม', icon: 'pi pi-home', to: { name: 'dashboard' } },
    { label: 'สร้างเอกสาร', icon: 'pi pi-file-plus', to: { name: 'signing-document-new' } },
    { label: 'เอกสารรอเซ็น', icon: 'pi pi-send', to: { name: 'signing-documents' } },
    { label: 'ประวัติเอกสาร', icon: 'pi pi-history', to: { name: 'signing-document-history' } },
    { label: 'คู่มือผู้เซ็น', icon: 'pi pi-info-circle', to: { name: 'admin-user-guide' } }
];

const guideSections = [
    {
        id: 'login',
        title: 'เข้าสู่ระบบและเลือกฐานข้อมูล',
        icon: 'pi pi-sign-in',
        severity: 'info',
        summary: 'ใช้บัญชี SML เพื่อยืนยันตัวตน แล้วเลือกฐานข้อมูลที่ต้องการทำงานทุกครั้ง',
        steps: [
            'กรอกชื่อผู้ใช้และรหัสผ่าน SML',
            'กดตรวจสอบบัญชีเพื่อดึงรายการ database ที่มีสิทธิ์',
            'เลือก SML1_2026 หรือฐานข้อมูลงานจริงที่ต้องการใช้งาน'
        ],
        notes: ['PaperLess จะแยกเอกสารและ workflow ตาม database ที่เลือกใน session นั้น'],
        shots: [
            { title: 'กรอกบัญชี SML', src: loginCredentialsImage },
            { title: 'เลือก database ก่อนเข้าใช้งาน', src: loginDatabaseImage }
        ]
    },
    {
        id: 'dashboard',
        title: 'ตรวจภาพรวมงาน',
        icon: 'pi pi-chart-bar',
        severity: 'success',
        route: { name: 'dashboard' },
        summary: 'หน้าแรกใช้ดูจำนวนเอกสารตามสถานะ งานที่ต้องรีบจัดการ และเอกสารล่าสุด',
        steps: [
            'ดูการ์ดสรุป เตรียมส่ง, รอลายเซ็น, รอยืนยัน, เสร็จสมบูรณ์',
            'เปิดเอกสารจากรายการที่ต้องดำเนินการ',
            'ใช้ปุ่ม refresh เมื่อต้องการเช็คสถานะล่าสุดหลังผู้ใช้เซ็น'
        ],
        shots: [{ title: 'Dashboard สำหรับ admin', src: dashboardImage }]
    },
    {
        id: 'create',
        title: 'สร้างเอกสารเตรียมส่ง',
        icon: 'pi pi-file-plus',
        severity: 'warn',
        route: { name: 'signing-document-new' },
        summary: 'ค้นหาเอกสารจาก SML, อัปโหลด PDF จริง, วางกรอบลายเซ็นและข้อความกฎหมาย แล้วบันทึกเป็น draft',
        steps: [
            'เลือกประเภทเอกสารและค้นหาเลขเอกสารจาก SML',
            'อัปโหลด PDF จริงของเอกสารนั้น',
            'ใช้กรอบเริ่มต้นหรือปรับกรอบเองในแต่ละหน้า',
            'ตรวจจำนวนกรอบและหน้าที่มีกรอบก่อนบันทึก'
        ],
        notes: ['ถ้า PDF หลายหน้า ระบบจะ clone กรอบเริ่มต้นไปทุกหน้า แต่ admin ยังลบหรือแก้แต่ละหน้าได้'],
        shots: [{ title: 'หน้าสร้างเอกสาร', src: createDocumentImage }]
    },
    {
        id: 'drafts',
        title: 'ส่งเอกสารจาก draft',
        icon: 'pi pi-send',
        severity: 'secondary',
        route: { name: 'signing-document-drafts' },
        summary: 'เอกสารที่บันทึกแล้วจะอยู่ในเมนูเอกสารเตรียมส่ง เพื่อให้ admin ตรวจครั้งสุดท้ายก่อนส่งให้ผู้เซ็น',
        steps: [
            'เปิด draft เพื่อตรวจ PDF และ flow',
            'กดส่งเอกสารเมื่อข้อมูลพร้อม',
            'ถ้าเลขเอกสารซ้ำ ให้เปิดเอกสารเดิมแทนการสร้างใหม่'
        ],
        shots: [{ title: 'คิวเอกสารเตรียมส่ง', src: draftsImage }]
    },
    {
        id: 'active',
        title: 'ติดตามเอกสารรอเซ็น',
        icon: 'pi pi-clock',
        severity: 'info',
        route: { name: 'signing-documents' },
        summary: 'ใช้เมนูเอกสารรอเซ็นเพื่อติดตามว่ารอใคร เซ็นถึงขั้นตอนไหน และมีผู้เซ็นภายนอกหรือไม่',
        steps: [
            'ดู column สถานะเพื่อรู้ว่าเอกสารรอลายเซ็นอยู่',
            'เปิด detail เพื่อดู timeline และ PDF ปัจจุบัน',
            'กรณีบุคคลภายนอก ให้ใช้ section ผู้เซ็นภายนอกในหน้า detail เพื่อสร้างลิงก์และ OTP'
        ],
        shots: [{ title: 'คิวเอกสารรอเซ็น', src: activeDocumentsImage }]
    },
    {
        id: 'history',
        title: 'ตรวจประวัติเอกสารเซ็นครบ',
        icon: 'pi pi-history',
        severity: 'success',
        route: { name: 'signing-document-history' },
        summary: 'หลังเอกสารเสร็จสมบูรณ์ admin เปิดดูเอกสารเซ็นครบได้จาก history โดยหน้า preview ปกติจะแสดงเอกสารจริง ไม่รวม evidence appendix',
        steps: [
            'เปิดประวัติเอกสารเซ็นเพื่อค้นหาเอกสาร completed',
            'กดดูเอกสารเซ็นครบเพื่อตรวจ current PDF',
            'เปิด detail เมื่อต้องการดู workflow, evidence หรือพิมพ์ official copy'
        ],
        shots: [
            { title: 'ประวัติเอกสารเซ็นครบ', src: historyImage },
            { title: 'หน้า detail แสดง current PDF', src: detailCurrentImage }
        ]
    },
    {
        id: 'evidence-print',
        title: 'หลักฐานการลงนามและการพิมพ์',
        icon: 'pi pi-shield',
        severity: 'warn',
        summary: 'หลักฐานการลงนามเก็บไว้เพื่อ audit ส่วนการพิมพ์ต้องใช้ปุ่มพิมพ์เอกสารในระบบเพื่อบันทึกประวัติ',
        steps: [
            'กดดูหลักฐานการลงนามเมื่อต้องตรวจ audit/e-sign proof',
            'กดพิมพ์เอกสารจากระบบเท่านั้น เพื่อสร้าง print event และ official print copy',
            'ถ้า SML image หรือ lock fail ให้ใช้ปุ่ม retry ในหน้า detail แทนการแก้ database ตรง ๆ'
        ],
        notes: ['Final PDF ที่สร้างก่อนอัปเดต font จะไม่ถูก rewrite อัตโนมัติ เอกสารใหม่หลัง deploy จะใช้ font evidence รุ่นใหม่'],
        shots: [
            { title: 'Dialog ดูหลักฐานการลงนาม', src: evidenceDialogImage },
            { title: 'พื้นที่ action สำหรับพิมพ์และ retry', src: printActionImage }
        ]
    }
];

const filteredSections = computed(() => {
    const query = search.value.trim().toLowerCase();
    if (!query) return guideSections;
    return guideSections.filter((section) => {
        const haystack = [
            section.title,
            section.summary,
            ...(section.steps || []),
            ...(section.notes || [])
        ]
            .join(' ')
            .toLowerCase();
        return haystack.includes(query);
    });
});

function openRoute(route) {
    if (!route) return;
    router.push(route);
}

function openShot(section, shot) {
    selectedShot.value = {
        title: `${section.title} · ${shot.title}`,
        src: shot.src
    };
}

function closeShot() {
    selectedShot.value = null;
}
</script>

<template>
    <div class="admin-guide">
        <div class="card">
            <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4 mb-6">
                <div>
                    <div class="flex items-center gap-2 mb-2">
                        <i class="pi pi-book text-primary text-xl"></i>
                        <h1 class="text-2xl font-semibold m-0">คู่มือการใช้งาน PaperLess</h1>
                    </div>
                    <p class="text-muted-color m-0">คู่มือสำหรับ admin จาก QA บนฐานข้อมูล SML1_2026 พร้อมภาพหน้าจอของระบบจริง</p>
                </div>
                <div class="flex flex-col sm:flex-row gap-2 sm:items-center">
                    <IconField>
                        <InputIcon><i class="pi pi-search" /></InputIcon>
                        <InputText v-model="search" placeholder="ค้นหาหัวข้อ..." class="w-full sm:w-72" />
                    </IconField>
                    <Button icon="pi pi-refresh" label="ล้างค้นหา" severity="secondary" outlined :disabled="!search" @click="search = ''" />
                </div>
            </div>

            <Message severity="info" class="mb-6">
                คู่มือนี้แสดงเฉพาะผู้ดูแลระบบ และใช้สำหรับสอนงาน flow หลัก: login, สร้างเอกสาร, ส่งเซ็น, ติดตาม, ตรวจหลักฐาน และพิมพ์ผ่านระบบ
            </Message>

            <div class="grid grid-cols-12 gap-3 mb-6">
                <div v-for="link in quickLinks" :key="link.label" class="col-span-12 sm:col-span-6 lg:col-span-3 xl:col-span-2">
                    <Button :label="link.label" :icon="link.icon" severity="secondary" outlined class="w-full justify-center" @click="openRoute(link.to)" />
                </div>
            </div>

            <div class="grid grid-cols-12 gap-6">
                <aside class="col-span-12 lg:col-span-3">
                    <div class="guide-nav border border-surface rounded-md p-3">
                        <div class="font-semibold mb-3">สารบัญ</div>
                        <a v-for="section in guideSections" :key="section.id" :href="`#${section.id}`" class="guide-nav-link">
                            <i :class="section.icon"></i>
                            <span>{{ section.title }}</span>
                        </a>
                    </div>
                </aside>

                <main class="col-span-12 lg:col-span-9 flex flex-col gap-5">
                    <Message v-if="filteredSections.length === 0" severity="warn">ไม่พบหัวข้อที่ค้นหา</Message>

                    <section v-for="section in filteredSections" :id="section.id" :key="section.id" class="guide-section border border-surface rounded-md p-4">
                        <div class="flex flex-col md:flex-row md:items-start justify-between gap-3 mb-4">
                            <div>
                                <div class="flex items-center gap-2 mb-2">
                                    <i :class="[section.icon, 'text-primary']"></i>
                                    <h2 class="text-xl font-semibold m-0">{{ section.title }}</h2>
                                    <Tag :value="section.steps.length + ' ขั้นตอน'" :severity="section.severity" />
                                </div>
                                <p class="text-muted-color m-0">{{ section.summary }}</p>
                            </div>
                            <Button v-if="section.route" label="เปิดหน้านี้" icon="pi pi-external-link" severity="secondary" outlined @click="openRoute(section.route)" />
                        </div>

                        <div class="grid grid-cols-12 gap-4">
                            <div class="col-span-12 xl:col-span-5">
                                <ol class="guide-steps m-0 pl-5">
                                    <li v-for="step in section.steps" :key="step">{{ step }}</li>
                                </ol>
                                <Message v-for="note in section.notes || []" :key="note" severity="secondary" class="mt-4">{{ note }}</Message>
                            </div>

                            <div class="col-span-12 xl:col-span-7">
                                <div class="grid grid-cols-12 gap-3">
                                    <button
                                        v-for="shot in section.shots"
                                        :key="shot.title"
                                        type="button"
                                        class="guide-shot col-span-12 md:col-span-6"
                                        @click="openShot(section, shot)"
                                    >
                                        <img :src="shot.src" :alt="shot.title" />
                                        <span>{{ shot.title }}</span>
                                    </button>
                                </div>
                            </div>
                        </div>
                    </section>
                </main>
            </div>
        </div>

        <Dialog :visible="Boolean(selectedShot)" modal :header="selectedShot?.title || 'ภาพตัวอย่าง'" :style="{ width: 'min(92rem, 96vw)' }" @update:visible="(value) => !value && closeShot()">
            <img v-if="selectedShot" :src="selectedShot.src" :alt="selectedShot.title" class="guide-preview-image" />
            <template #footer>
                <Button label="ปิด" icon="pi pi-times" severity="secondary" outlined @click="closeShot" />
            </template>
        </Dialog>
    </div>
</template>

<style scoped>
.guide-nav {
    position: sticky;
    top: 5.5rem;
}

.guide-nav-link {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    padding: 0.65rem 0.75rem;
    border-radius: 0.45rem;
    color: var(--text-color);
    text-decoration: none;
}

.guide-nav-link:hover {
    background: var(--surface-hover);
}

.guide-section {
    scroll-margin-top: 6rem;
}

.guide-steps {
    line-height: 1.8;
}

.guide-shot {
    border: 1px solid var(--surface-border);
    border-radius: 0.5rem;
    background: var(--surface-card);
    padding: 0;
    text-align: left;
    overflow: hidden;
    cursor: pointer;
}

.guide-shot:hover {
    border-color: var(--primary-color);
}

.guide-shot img {
    display: block;
    width: 100%;
    aspect-ratio: 16 / 10;
    object-fit: cover;
    object-position: top left;
    background: var(--surface-ground);
}

.guide-shot span {
    display: block;
    padding: 0.75rem;
    font-weight: 600;
}

.guide-preview-image {
    width: 100%;
    max-height: 78vh;
    object-fit: contain;
    border: 1px solid var(--surface-border);
    border-radius: 0.5rem;
    background: var(--surface-ground);
}

@media (max-width: 991px) {
    .guide-nav {
        position: static;
    }
}
</style>
