<template>
  <aside class="app-sidebar">
    <router-link to="/" class="brand-lockup" aria-label="PaperLess dashboard">
      <span class="brand-mark"><i class="pi pi-file-edit"></i></span>
      <span>
        <strong>PaperLess</strong>
        <small>V2</small>
      </span>
    </router-link>

    <nav class="side-nav" aria-label="Main menu">
      <router-link to="/" class="side-link">
        <i class="pi pi-home"></i>
        <span>Dashboard</span>
      </router-link>
      <span class="side-link is-disabled">
        <i class="pi pi-inbox"></i>
        <span>Inbox</span>
      </span>
      <span class="side-link is-disabled">
        <i class="pi pi-sitemap"></i>
        <span>Workflow</span>
      </span>
      <span class="side-link is-disabled">
        <i class="pi pi-history"></i>
        <span>Audit</span>
      </span>
    </nav>
  </aside>
</template>

