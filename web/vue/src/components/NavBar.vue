<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useMailStore } from '../stores/mail'

const auth = useAuthStore()
const mail = useMailStore()
const router = useRouter()

onMounted(() => mail.fetchUnread())

async function handleLogout() {
  await auth.logout()
  router.push('/login')
}
</script>

<template>
  <nav>
    <div class="container">
      <router-link to="/inbox" class="brand">BDS Mail</router-link>
      <div class="nav-links">
        <router-link to="/inbox">Inbox<span v-if="mail.unreadCount" class="badge">{{ mail.unreadCount }}</span></router-link>
        <router-link to="/compose">Compose</router-link>
        <router-link to="/contacts">Contacts</router-link>
        <router-link to="/filters">Filters</router-link>
        <router-link to="/settings/autoreply">Auto-Reply</router-link>
        <router-link to="/developer">Developer</router-link>
        <span class="user-info">{{ auth.user?.displayName || auth.user?.email }}</span>
        <button class="btn btn-sm btn-secondary" @click="handleLogout">Logout</button>
      </div>
    </div>
  </nav>
</template>
