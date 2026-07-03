<script setup>
import { authStore } from '@/stores/auth';
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';

const router = useRouter();
const route = useRoute();

const pageLabel = computed(() => {
    const routeName = String(route.name || '');
    if (routeName.includes('guide')) return 'คู่มือการใช้งาน';
    if (routeName.includes('history')) return 'ประวัติการเซ็น';
    return 'งานรอเซ็น';
});
const compactMode = computed(() => Boolean(route.meta.compactSignerLayout));
const isGuideRoute = computed(() => String(route.name || '').includes('guide'));

function openGuide() {
    router.push({ name: 'my-signing-guide' });
}

async function logout() {
    await authStore.logout();
    router.push({ name: 'login' });
}
</script>

<template>
    <div class="signer-layout" :class="{ 'compact-signer-layout': compactMode }">
        <header class="signer-topbar">
            <button class="brand" type="button" @click="router.push({ name: 'my-signing-tasks' })">
                <span class="brand-icon"><i class="pi pi-file-check"></i></span>
                <span>
                    <strong>PaperLess</strong>
                    <small v-if="!compactMode">{{ pageLabel }}</small>
                </span>
            </button>
            <div class="signer-user">
                <span>{{ authStore.user?.displayName || authStore.user?.username }}</span>
                <Button
                    icon="pi pi-info-circle"
                    text
                    rounded
                    :class="{ 'guide-action-active': isGuideRoute }"
                    aria-label="เปิดคู่มือการใช้งาน"
                    title="คู่มือการใช้งาน"
                    @click="openGuide"
                />
                <Button icon="pi pi-sign-out" text rounded aria-label="ออกจากระบบ" @click="logout" />
            </div>
        </header>
        <nav v-if="!compactMode" class="signer-nav" aria-label="เมนูงานเซ็น">
            <RouterLink :to="{ name: 'my-signing-tasks' }" class="signer-nav-link">
                <i class="pi pi-inbox"></i>
                <span>งานรอเซ็น</span>
            </RouterLink>
            <RouterLink :to="{ name: 'my-signing-history' }" class="signer-nav-link">
                <i class="pi pi-history"></i>
                <span>ประวัติการเซ็น</span>
            </RouterLink>
        </nav>
        <main class="signer-main">
            <router-view />
        </main>
    </div>
</template>

<style scoped>
.signer-layout {
    min-height: 100dvh;
    background: var(--surface-ground);
}

.signer-topbar {
    position: sticky;
    top: 0;
    z-index: 21;
    height: 56px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.45rem 0.85rem;
    border-bottom: 1px solid var(--surface-border);
    background: var(--surface-card);
}

.signer-nav {
    position: sticky;
    top: 56px;
    z-index: 20;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.35rem;
    padding: 0.45rem 0.75rem;
    border-bottom: 1px solid var(--surface-border);
    background: var(--surface-card);
}

.signer-nav-link {
    min-height: 40px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.45rem;
    border-radius: 8px;
    color: var(--text-color-secondary);
    text-decoration: none;
    font-weight: 600;
}

.signer-nav-link span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.signer-nav-link:focus-visible,
.brand:focus-visible {
    outline: 2px solid color-mix(in srgb, var(--primary-color) 45%, transparent);
    outline-offset: 2px;
    box-shadow: none;
}

.signer-nav-link.router-link-active {
    background: color-mix(in srgb, var(--primary-color) 12%, var(--surface-card));
    color: var(--primary-color);
}

.brand {
    border: 0;
    background: transparent;
    display: inline-flex;
    align-items: center;
    gap: 0.65rem;
    min-width: 0;
    padding: 0;
    color: var(--text-color);
    text-align: left;
    cursor: pointer;
}

.brand-icon {
    width: 2.25rem;
    height: 2.25rem;
    border-radius: 8px;
    display: grid;
    place-items: center;
    background: var(--primary-color);
    color: var(--primary-contrast-color);
}

.brand span:last-child {
    display: grid;
    line-height: 1.15;
}

.brand small {
    color: var(--text-color-secondary);
    font-size: 0.78rem;
}

.signer-user {
    min-width: 0;
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    color: var(--text-color-secondary);
    font-size: 0.9rem;
}

.signer-user span {
    max-width: 36vw;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.guide-action-active {
    color: var(--primary-color);
    background: color-mix(in srgb, var(--primary-color) 12%, transparent);
}

.signer-main {
    min-width: 0;
}

.compact-signer-layout .signer-topbar {
    height: 52px;
}

@media (max-width: 420px) {
    .signer-topbar {
        padding-inline: 0.7rem;
    }

    .brand small {
        display: none;
    }

    .signer-nav {
        padding-inline: 0.55rem;
    }

    .signer-nav-link {
        font-size: 0.92rem;
        gap: 0.25rem;
    }

    .compact-signer-layout .brand-icon {
        width: 2rem;
        height: 2rem;
    }

    .compact-signer-layout .brand strong {
        font-size: 0.98rem;
    }
}
</style>
