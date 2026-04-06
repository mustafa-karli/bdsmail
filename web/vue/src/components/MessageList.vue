<script setup lang="ts">
import { useRouter } from 'vue-router'
import { formatRelativeTime, hasAttachments } from '../utils'
import Pagination from './Pagination.vue'
import type { Message } from '../types'

defineProps<{
  messages: Message[]
  folder: string
  page: number
  totalPages: number
}>()

const router = useRouter()
</script>

<template>
  <table v-if="messages.length" class="message-list">
    <thead>
      <tr>
        <th>{{ folder === 'Sent' ? 'To' : 'From' }}</th>
        <th>Subject</th>
        <th>Date</th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="msg in messages"
        :key="msg.id"
        :class="{ unread: !msg.seen }"
        @click="router.push(`/message/${msg.id}`)"
      >
        <td>{{ folder === 'Sent' ? msg.to.join(', ') : msg.from }}</td>
        <td>
          <span v-if="hasAttachments(msg)" class="att-icon"></span>
          {{ msg.subject || '(no subject)' }}
        </td>
        <td :title="new Date(msg.receivedAt).toLocaleString()">
          {{ formatRelativeTime(msg.receivedAt) }}
        </td>
      </tr>
    </tbody>
  </table>
  <div v-else class="empty-state"><p>No messages in {{ folder }}.</p></div>
  <Pagination v-if="totalPages > 1" :page="page" :total-pages="totalPages" />
</template>
