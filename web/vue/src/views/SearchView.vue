<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api/client'
import MessageList from '../components/MessageList.vue'
import type { Message } from '../types'

const route = useRoute()
const messages = ref<Message[]>([])
const query = ref('')

async function load() {
  query.value = (route.query.q as string) || ''
  if (!query.value) { messages.value = []; return }
  const { data } = await api.get('/search', { params: { q: query.value } })
  messages.value = data.messages || []
}

watch(() => route.query.q, load, { immediate: true })
</script>

<template>
  <div class="card">
    <div class="toolbar">
      <h2>Search: "{{ query }}"</h2>
      <router-link to="/inbox" class="btn btn-sm btn-secondary">Back to Inbox</router-link>
    </div>
    <MessageList :messages="messages" folder="Search" :page="1" :total-pages="1" />
  </div>
</template>
