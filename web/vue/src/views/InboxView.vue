<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api/client'
import { useMailStore } from '../stores/mail'
import MessageList from '../components/MessageList.vue'
import type { Message } from '../types'

const props = defineProps<{ folder: string }>()
const route = useRoute()
const mail = useMailStore()

const messages = ref<Message[]>([])
const page = ref(1)
const totalPages = ref(1)
const folders = ref<string[]>([])

async function load() {
  page.value = Number(route.query.page) || 1
  const { data } = await api.get('/messages', { params: { folder: props.folder, page: page.value } })
  messages.value = data.messages || []
  totalPages.value = data.totalPages
  mail.unreadCount = data.unreadCount
  await mail.fetchFolders()
  folders.value = mail.folders
}

watch(() => [props.folder, route.query.page], load, { immediate: true })
</script>

<template>
  <div class="card">
    <div class="toolbar">
      <div class="folder-tabs">
        <router-link to="/inbox" :class="{ active: folder === 'INBOX' }">
          Inbox<span v-if="mail.unreadCount && folder === 'INBOX'" class="badge">{{ mail.unreadCount }}</span>
        </router-link>
        <router-link to="/sent" :class="{ active: folder === 'Sent' }">Sent</router-link>
        <template v-for="f in folders" :key="f">
          <router-link v-if="f !== 'INBOX' && f !== 'Sent'" :to="`/folder/${f}`" :class="{ active: folder === f }">{{ f }}</router-link>
        </template>
      </div>
      <div style="display:flex;gap:8px">
        <form @submit.prevent="$router.push({ path: '/search', query: { q: ($refs.q as HTMLInputElement).value } })" style="display:flex;gap:4px">
          <input ref="q" type="text" placeholder="Search..." style="width:150px;padding:4px 8px;font-size:0.9em" />
          <button class="btn btn-sm btn-secondary" type="submit">Search</button>
        </form>
        <router-link to="/compose" class="btn btn-sm">Compose</router-link>
      </div>
    </div>
    <MessageList :messages="messages" :folder="folder" :page="page" :total-pages="totalPages" />
  </div>
</template>
