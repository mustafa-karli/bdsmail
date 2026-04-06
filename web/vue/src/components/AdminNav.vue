<script setup lang="ts">
</script>

<template>
  <div class="toolbar">
    <div class="folder-tabs">
      <router-link to="/admin/domains">Domains</router-link>
      <router-link to="/admin/users">Users</router-link>
      <router-link to="/admin/aliases">Aliases</router-link>
      <router-link to="/admin/lists">Lists</router-link>
    </div>
    <form action="/api/admin/logout" method="POST" style="display:inline">
      <button class="btn btn-sm btn-secondary" type="submit">Logout</button>
    </form>
  </div>
</template>
