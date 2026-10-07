<script setup>
import { authStore } from '@/stores/auth';
import { trialBannerMessage, trialBannerState } from '@/utils/trialBanner';
import Message from 'primevue/message';
import { computed } from 'vue';

// The signed-in layout reads the date from the auth store. The login page has
// no session yet, so it fetches the date itself and passes it in - which is why
// this prop exists and why `undefined` (not passed) differs from `null` (the
// shop has no trial).
const props = defineProps({
    expiresAt: { type: String, default: undefined }
});

const effectiveExpiresAt = computed(() => (props.expiresAt !== undefined ? props.expiresAt : authStore.trialExpiresAt));

const state = computed(() => trialBannerState(effectiveExpiresAt.value));
const visible = computed(() => state.value.visible);
// Past the end date it shows as an error: on the login page it explains why
// signing in is refused, instead of leaving the user with only a rejected form.
const expired = computed(() => state.value.expired);
const message = computed(() => trialBannerMessage(state.value));
</script>

<template>
    <Message v-if="visible" :severity="expired ? 'error' : 'warn'" :closable="false" class="trial-banner">
        {{ message }}
    </Message>
</template>

<style scoped>
/* No top margin and no positioning of its own: the parent decides where it sits.
   It used to carry margin: 0.5rem 1rem 0 and be placed outside the content
   container, so it landed underneath the fixed topbar and was only visible while
   scrolled to the very top. */
.trial-banner {
    margin: 0 0 1rem;
}
</style>
