<script setup>
import loginImage from '@/assets/user-guide/user-01-login.png';
import databaseImage from '@/assets/user-guide/user-02-database.png';
import tasksImage from '@/assets/user-guide/user-03-task-list.png';
import signingPdfImage from '@/assets/user-guide/user-04-signing-pdf.png';
import signatureImage from '@/assets/user-guide/user-05-signature.png';
import flowImage from '@/assets/user-guide/user-06-flow.png';
import historyImage from '@/assets/user-guide/user-07-history-list.png';
import historyDetailImage from '@/assets/user-guide/user-08-history-detail.png';
import { authStore } from '@/stores/auth';
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

const router = useRouter();
const route = useRoute();
const search = ref('');
const selectedShot = ref(null);

const isAdminView = computed(() => authStore.user?.role === 'admin' || route.meta.guideAudience === 'admin');
const quickLinks = computed(() => {
    if (isAdminView.value) return [];
    return [
        { label: 'งานรอเซ็น', icon: 'pi pi-inbox', to: { name: 'my-signing-tasks' } },
        { label: 'ประวัติ', icon: 'pi pi-history', to: { name: 'my-signing-history' } }
    ];
});

const guideSections = [
    {
        id: 'login',
        title: 'เข้าสู่ระบบ',
        icon: 'pi pi-sign-in',
        severity: 'info',
        summary: 'เข้าสู่ระบบด้วยบัญชีที่ได้รับ แล้วเลือก database ที่ต้องการทำงานทุกครั้ง',
        steps: ['กรอกชื่อผู้ใช้และรหัสผ่าน', 'กดตรวจสอบบัญชี', 'เลือก database ที่ได้รับสิทธิ์ เช่น SML1_2026', 'กดเข้าสู่ PaperLess'],
        notes: ['ถ้า login ไม่ได้ ให้แจ้งผู้ดูแลเพื่อตรวจบัญชีและสิทธิ์ database'],
        shots: [
            { title: 'กรอกบัญชี', src: loginImage },
            { title: 'เลือก database', src: databaseImage }
        ]
    },
    {
        id: 'tasks',
        title: 'ดูงานรอเซ็น',
        icon: 'pi pi-inbox',
        severity: 'success',
        route: { name: 'my-signing-tasks' },
        summary: 'หน้าแรกของผู้ใช้แสดงงานที่ถึงคิวเซ็นแล้ว และงานที่ยังรอขั้นตอนก่อนหน้า',
        steps: ['ดูจำนวนงานในแท็ก เซ็นได้ และ รอคิว', 'ใช้ช่องค้นหาเมื่อต้องหาเลขเอกสารหรือคู่ค้า', 'กดเปิดเอกสารเฉพาะรายการที่ต้องการดำเนินการ'],
        notes: ['ถ้าเอกสารอยู่ในรอคิว ระบบจะแสดงเหตุผลว่าต้องรอขั้นตอนไหนก่อน'],
        shots: [{ title: 'รายการงานรอเซ็น', src: tasksImage }]
    },
    {
        id: 'sign',
        title: 'เปิดเอกสารและเซ็น',
        icon: 'pi pi-pencil',
        severity: 'warn',
        summary: 'อ่าน PDF แบบเลื่อนต่อเนื่อง วาดลายเซ็น ยอมรับข้อความกฎหมาย แล้วกดยืนยันเซ็น',
        steps: ['ตรวจเลขเอกสาร คู่ค้า และตำแหน่งของคุณ', 'เลื่อนอ่าน PDF ให้ครบก่อนเซ็น', 'วาดลายเซ็นในช่องลายเซ็น ถ้าผิดให้กดล้าง', 'ติ๊กยืนยันข้อความ พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์', 'กดยืนยันเซ็น เมื่อปุ่มพร้อมใช้งาน'],
        notes: ['หลังเซ็นสำเร็จ เอกสารจะหายจากคิวเซ็นได้ตอนนี้ และไปอยู่ในประวัติเมื่อ flow เสร็จตามระบบ'],
        shots: [
            { title: 'อ่าน PDF ในหน้าเซ็น', src: signingPdfImage },
            { title: 'ลายเซ็นและปุ่มยืนยัน', src: signatureImage }
        ]
    },
    {
        id: 'flow',
        title: 'ดู Flow เอกสาร',
        icon: 'pi pi-sitemap',
        severity: 'secondary',
        summary: 'Flow เอกสารช่วยให้ดูความสัมพันธ์และลำดับของเอกสารโดยไม่ต้องออกจากหน้าเซ็น',
        steps: ['กดปุ่ม Flow เอกสาร', 'อ่านลำดับเอกสารและสถานะใน dialog', 'ปิด dialog เพื่อกลับมาเซ็นต่อที่หน้าเดิม'],
        notes: ['Flow เป็นข้อมูลประกอบการตัดสินใจ ไม่ใช่ปุ่มสำหรับพิมพ์หรือดาวน์โหลดเอกสาร'],
        shots: [{ title: 'Dialog Flow เอกสาร', src: flowImage }]
    },
    {
        id: 'reject-attach',
        title: 'ปฏิเสธหรือแนบไฟล์',
        icon: 'pi pi-paperclip',
        severity: 'warn',
        summary: 'ใช้เฉพาะกรณีต้องส่งเหตุผลกลับ หรือมีไฟล์อ้างอิงที่จำเป็นต่อการพิจารณา',
        steps: ['กดปฏิเสธเมื่อต้องหยุดเอกสารและแจ้งเหตุผล', 'กรอกเหตุผลให้ชัดเจนก่อนยืนยันปฏิเสธ', 'ใช้แนบไฟล์อ้างอิงเมื่อมี PDF หรือรูปภาพที่เกี่ยวข้อง', 'ตรวจไฟล์และหมายเหตุก่อนส่ง'],
        notes: ['การปฏิเสธจะหยุด workflow ของเอกสารนั้น ให้ใช้เมื่อมีเหตุผลจริงเท่านั้น'],
        shots: [{ title: 'ส่วนลายเซ็นและไฟล์อ้างอิง', src: signatureImage }]
    },
    {
        id: 'history',
        title: 'ดูประวัติการเซ็น',
        icon: 'pi pi-history',
        severity: 'success',
        route: { name: 'my-signing-history' },
        summary: 'ดูเอกสารที่คุณเคยเซ็นหรือปฏิเสธ โดยเน้น PDF และสถานะของคุณเอง',
        steps: ['เปิดเมนูประวัติการเซ็น', 'ค้นหาด้วยเลขเอกสาร คู่ค้า หรือตำแหน่ง', 'กดดูเอกสารเพื่อเปิด PDF ล่าสุดของเอกสารจริง', 'ย้อนกลับเมื่อดูเสร็จ'],
        notes: ['หน้าประวัติของผู้ใช้ไม่แสดงหลักฐาน audit ของ admin และไม่รวมหน้า evidence appendix เป็นค่า default'],
        shots: [
            { title: 'รายการประวัติการเซ็น', src: historyImage },
            { title: 'ดูเอกสารย้อนหลัง', src: historyDetailImage }
        ]
    },
    {
        id: 'troubleshooting',
        title: 'ปัญหาที่พบบ่อย',
        icon: 'pi pi-question-circle',
        severity: 'info',
        summary: 'เช็คสถานะพื้นฐานก่อนแจ้งผู้ดูแล เพื่อช่วยให้แก้ปัญหาได้เร็วขึ้น',
        steps: ['ไม่มีงานรอเซ็น: กดโหลดใหม่ และตรวจว่าผู้ดูแลส่งเอกสารแล้วหรือยัง', 'ยังไม่ถึงคิว: รอผู้เซ็นก่อนหน้าเซ็นครบก่อน', 'PDF โหลดไม่ขึ้น: กดโหลดใหม่หรือออกเข้าใหม่ แล้วแจ้งเลขเอกสารให้ผู้ดูแล', 'เซ็นแล้วไม่เห็นในคิว: ตรวจประวัติการเซ็น หรือรอผู้ดูแลยืนยันเอกสาร'],
        notes: ['อย่าส่งต่อหน้าจอที่มีข้อมูลลูกค้าให้คนที่ไม่เกี่ยวข้อง'],
        shots: [{ title: 'งานรอเซ็นและปุ่มโหลดใหม่', src: tasksImage }]
    }
];

const filteredSections = computed(() => {
    const query = search.value.trim().toLowerCase();
    if (!query) return guideSections;
    return guideSections.filter((section) => {
        const haystack = [section.title, section.summary, ...(section.steps || []), ...(section.notes || [])].join(' ').toLowerCase();
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
    <section class="user-guide">
        <div class="guide-card">
            <div class="guide-header">
                <div>
                    <div class="guide-title">
                        <i class="pi pi-book text-primary"></i>
                        <h1>คู่มือการใช้งาน PaperLess</h1>
                    </div>
                    <p v-if="isAdminView">คู่มือฉบับผู้เซ็นสำหรับ admin ใช้สอนงานและอ้างอิง flow ฝั่ง user</p>
                    <p v-else>คู่มือสำหรับผู้เซ็นเอกสาร ใช้ดูงานรอเซ็น เซ็นเอกสาร และตรวจประวัติของตัวเอง</p>
                </div>
                <div class="guide-search">
                    <IconField>
                        <InputIcon><i class="pi pi-search" /></InputIcon>
                        <InputText v-model="search" placeholder="ค้นหาหัวข้อ..." />
                    </IconField>
                    <Button icon="pi pi-refresh" label="ล้าง" severity="secondary" outlined :disabled="!search" @click="search = ''" />
                </div>
            </div>

            <Message v-if="isAdminView" severity="info" class="m-0">
                หน้านี้แสดงคู่มือฝั่ง user ภายใน admin console เพื่อให้ผู้ดูแลใช้ประกอบการสอนงาน โดยไม่เปิด shortcut ไปหน้า user โดยตรง
            </Message>
            <Message v-else severity="info" class="m-0">
                คู่มือนี้แสดงเฉพาะผู้ใช้งาน role user และเน้นการทำงานบนมือถือเป็นหลัก
            </Message>

            <div v-if="quickLinks.length" class="quick-links">
                <Button v-for="link in quickLinks" :key="link.label" :label="link.label" :icon="link.icon" severity="secondary" outlined @click="openRoute(link.to)" />
            </div>

            <div class="guide-layout">
                <aside class="guide-nav">
                    <div class="font-semibold mb-3">สารบัญ</div>
                    <a v-for="section in guideSections" :key="section.id" :href="`#${section.id}`" class="guide-nav-link">
                        <i :class="section.icon"></i>
                        <span>{{ section.title }}</span>
                    </a>
                </aside>

                <main class="guide-content">
                    <Message v-if="filteredSections.length === 0" severity="warn">ไม่พบหัวข้อที่ค้นหา</Message>

                    <section v-for="section in filteredSections" :id="section.id" :key="section.id" class="guide-section">
                        <div class="section-header">
                            <div>
                                <div class="section-title">
                                    <i :class="[section.icon, 'text-primary']"></i>
                                    <h2>{{ section.title }}</h2>
                                    <Tag :value="`${section.steps.length} ขั้นตอน`" :severity="section.severity" />
                                </div>
                                <p>{{ section.summary }}</p>
                            </div>
                            <Button v-if="section.route && !isAdminView" label="เปิดหน้านี้" icon="pi pi-external-link" severity="secondary" outlined @click="openRoute(section.route)" />
                        </div>

                        <div class="section-grid">
                            <div>
                                <ol class="guide-steps">
                                    <li v-for="step in section.steps" :key="step">{{ step }}</li>
                                </ol>
                                <Message v-for="note in section.notes || []" :key="note" severity="secondary" class="mt-4">{{ note }}</Message>
                            </div>

                            <div class="shot-grid">
                                <button v-for="shot in section.shots" :key="shot.title" type="button" class="guide-shot" @click="openShot(section, shot)">
                                    <img :src="shot.src" :alt="shot.title" loading="lazy" decoding="async" />
                                    <span>{{ shot.title }}</span>
                                </button>
                            </div>
                        </div>
                    </section>
                </main>
            </div>
        </div>

        <Dialog :visible="Boolean(selectedShot)" modal :header="selectedShot?.title || 'ภาพตัวอย่าง'" :style="{ width: 'min(34rem, 96vw)' }" @update:visible="(value) => !value && closeShot()">
            <img v-if="selectedShot" :src="selectedShot.src" :alt="selectedShot.title" class="guide-preview-image" />
            <template #footer>
                <Button label="ปิด" icon="pi pi-times" severity="secondary" outlined @click="closeShot" />
            </template>
        </Dialog>
    </section>
</template>

<style scoped>
.user-guide {
    min-height: calc(100dvh - 96px);
    padding: 0.75rem;
}

.guide-card {
    max-width: 1080px;
    margin: 0 auto;
    display: grid;
    gap: 1rem;
}

.guide-header {
    display: grid;
    gap: 0.9rem;
}

.guide-title,
.section-title {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
}

.guide-title h1,
.section-title h2 {
    margin: 0;
    line-height: 1.2;
}

.guide-title h1 {
    font-size: 1.35rem;
}

.guide-header p,
.section-header p {
    margin: 0.25rem 0 0;
    color: var(--text-color-secondary);
}

.guide-search {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.5rem;
}

.guide-search :deep(.p-inputtext) {
    width: 100%;
    min-height: 44px;
    font-size: 16px;
}

.quick-links {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.5rem;
}

.guide-layout {
    display: grid;
    gap: 1rem;
}

.guide-nav,
.guide-section {
    border: 1px solid var(--surface-border);
    background: var(--surface-card);
    border-radius: 8px;
    padding: 1rem;
}

.guide-nav {
    display: grid;
    gap: 0.15rem;
}

.guide-nav-link {
    min-height: 42px;
    display: flex;
    align-items: center;
    gap: 0.65rem;
    padding: 0.55rem 0.65rem;
    border-radius: 8px;
    color: var(--text-color);
    text-decoration: none;
}

.guide-nav-link:hover {
    background: var(--surface-hover);
}

.guide-content {
    display: grid;
    gap: 1rem;
}

.guide-section {
    scroll-margin-top: 7rem;
    display: grid;
    gap: 1rem;
}

.section-header {
    display: grid;
    gap: 0.75rem;
}

.section-title {
    flex-wrap: wrap;
}

.section-title h2 {
    font-size: 1.12rem;
}

.section-grid {
    display: grid;
    gap: 1rem;
}

.guide-steps {
    margin: 0;
    padding-left: 1.25rem;
    line-height: 1.75;
}

.shot-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: 0.75rem;
}

.guide-shot {
    border: 1px solid var(--surface-border);
    border-radius: 8px;
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
    aspect-ratio: 9 / 16;
    object-fit: cover;
    object-position: top left;
    background: var(--surface-ground);
}

.guide-shot span {
    display: block;
    padding: 0.7rem;
    font-weight: 600;
}

.guide-preview-image {
    width: 100%;
    max-height: 78dvh;
    object-fit: contain;
    border: 1px solid var(--surface-border);
    border-radius: 8px;
    background: var(--surface-ground);
}

@media (min-width: 760px) {
    .user-guide {
        padding: 1.25rem;
    }

    .guide-header {
        grid-template-columns: minmax(0, 1fr) minmax(20rem, 28rem);
        align-items: start;
    }

    .guide-layout {
        grid-template-columns: 16rem minmax(0, 1fr);
        align-items: start;
    }

    .guide-nav {
        position: sticky;
        top: 7.5rem;
    }

    .section-grid {
        grid-template-columns: minmax(0, 1fr) minmax(16rem, 22rem);
    }
}

@media (max-width: 420px) {
    .guide-search {
        grid-template-columns: 1fr;
    }

    .quick-links {
        grid-template-columns: 1fr;
    }
}
</style>
