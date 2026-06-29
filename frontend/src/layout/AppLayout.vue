<script setup>
import AppSidebar from '@/layout/AppSidebar.vue';
import AppTopbar from '@/layout/AppTopbar.vue';
</script>

<template>
  <div class="layout-shell">
    <AppSidebar />
    <div class="layout-main">
      <AppTopbar />
      <main class="layout-content">
        <router-view />
      </main>
    </div>
  </div>
</template>

