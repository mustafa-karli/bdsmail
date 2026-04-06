<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api/client'
import { formatRelativeTime, humanSize } from '../utils'
import type { Message } from '../types'

const props = defineProps<{ id: string }>()
const router = useRouter()
const msg = ref<Message | null>(null)

onMounted(async () => {
  const { data } = await api.get<Message>(`/messages/${props.id}`)
  msg.value = data
})

async function deleteMsg() {
  if (!confirm('Delete this message?')) return
  await api.post(`/messages/${props.id}/delete`)
  router.push('/inbox')
}
</script>

<template>
  <div v-if="msg" class="card">
    <div class="message-header">
      <h2>{{ msg.subject || '(no subject)' }}</h2>
      <div class="message-meta">
        <span><strong>From:</strong> {{ msg.from }}</span>
        <span><strong>To:</strong> {{ msg.to.join(', ') }}</span>
        <span v-if="msg.cc?.length"><strong>CC:</strong> {{ msg.cc.join(', ') }}</span>
        <span><strong>Date:</strong> {{ new Date(msg.receivedAt).toLocaleString() }} ({{ formatRelativeTime(msg.receivedAt) }})</span>
      </div>
    </div>
    <div v-if="msg.contentType === 'text/html'" class="message-body" v-html="msg.body"></div>
    <div v-else class="message-body text-body">{{ msg.body }}</div>
    <div v-if="msg.attachments?.length" class="attachments" style="margin-top:16px;padding:12px;background:#f5f5f5;border-radius:4px">
      <strong>Attachments:</strong>
      <ul style="margin:8px 0 0;padding-left:20px">
        <li v-for="att in msg.attachments" :key="att.id">
          <a :href="`/attachment/${msg.id}/${att.id}`">{{ att.filename }}</a> ({{ humanSize(att.size) }})
        </li>
      </ul>
    </div>
    <div class="message-actions">
      <router-link :to="{ path: '/compose', query: { reply: msg.id } }" class="btn btn-sm">Reply</router-link>
      <router-link v-if="msg.cc?.length" :to="{ path: '/compose', query: { replyall: msg.id } }" class="btn btn-sm">Reply All</router-link>
      <router-link :to="{ path: '/compose', query: { forward: msg.id } }" class="btn btn-sm btn-secondary">Forward</router-link>
      <router-link to="/inbox" class="btn btn-secondary btn-sm">Back</router-link>
      <button class="btn btn-danger btn-sm" @click="deleteMsg">Delete</button>
    </div>
  </div>
  <div v-else class="card"><p>Loading...</p></div>
</template>
