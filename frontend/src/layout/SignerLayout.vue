<script setup>
import { authStore } from '@/stores/auth';
import { useRouter } from 'vue-router';

const router = useRouter();

async function logout() {
    await authStore.logout();
    router.push({ name: 'login' });
}
</script>

<template>
    <div class="signer-layout">
        <header class="signer-topbar">
            <button class="brand" type="button" @click="router.push({ name: 'my-signing-tasks' })">
                <span class="brand-icon"><i class="pi pi-file-check"></i></span>
                <span>
                    <strong>PaperLess</strong>
                    <small>เอกสารรอเซ็น</small>
                </span>
            </button>
            <div class="signer-user">
                <span>{{ authStore.user?.displayName || authStore.user?.username }}</span>
                <Button icon="pi pi-sign-out" text rounded aria-label="ออกจากระบบ" @click="logout" />
            </div>
        </header>
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
    z-index: 20;
    height: 56px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.45rem 0.85rem;
    border-bottom: 1px solid var(--surface-border);
    background: var(--surface-card);
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
    max-width: 42vw;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.signer-main {
    min-width: 0;
}

@media (max-width: 420px) {
    .signer-topbar {
        padding-inline: 0.7rem;
    }

    .brand small {
        display: none;
    }
}
</style>
