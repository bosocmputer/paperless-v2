<script setup>
import { authStore } from '@/stores/auth';
import Message from 'primevue/message';
import { computed } from 'vue';

const daysLeft = computed(() => {
    if (!authStore.trialExpiresAt) return null;
    const expiresAt = new Date(authStore.trialExpiresAt);
    if (Number.isNaN(expiresAt.getTime())) return null;
    const diffMs = expiresAt.getTime() - Date.now();
    return Math.ceil(diffMs / (24 * 60 * 60 * 1000));
});

const visible = computed(() => daysLeft.value !== null && daysLeft.value <= 3 && daysLeft.value >= 0);

const message = computed(() => {
    if (daysLeft.value === 0) return 'ระยะเวลาทดลองใช้งานจะสิ้นสุดวันนี้ กรุณาติดต่อทีมงานเพื่อต่ออายุการใช้งาน';
    return `ระยะเวลาทดลองใช้งานจะสิ้นสุดในอีก ${daysLeft.value} วัน กรุณาติดต่อทีมงานเพื่อต่ออายุการใช้งาน`;
});
</script>

<template>
    <Message v-if="visible" severity="warn" :closable="false" class="trial-banner">
        {{ message }}
    </Message>
</template>

<style scoped>
.trial-banner {
    margin: 0.5rem 1rem 0;
}
</style>
