<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import AlertBanner from '../components/AlertBanner.vue'
import type { Contact } from '../types'

const contacts = ref<Contact[]>([])
const name = ref('')
const email = ref('')
const phone = ref('')
const error = ref('')
const success = ref('')

async function load() {
  const { data } = await api.get<Contact[]>('/contacts')
  contacts.value = data
}

async function create() {
  error.value = ''; success.value = ''
  try {
    await api.post('/contacts', { name: name.value, email: email.value, phone: phone.value })
    name.value = email.value = phone.value = ''
    success.value = 'Contact added'
    await load()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to add contact'
  }
}

async function remove(id: string) {
  await api.delete(`/contacts/${id}`)
  success.value = 'Contact deleted'
  await load()
}

onMounted(load)
</script>

<template>
  <div class="card">
    <div class="toolbar">
      <h2>Contacts</h2>
      <router-link to="/inbox" class="btn btn-sm btn-secondary">Back to Inbox</router-link>
    </div>
    <AlertBanner :error="error" :success="success" />
    <form @submit.prevent="create" style="margin-top:16px">
      <h3 style="color:#1976D2;margin-bottom:12px">Add Contact</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end">
        <div style="flex:1"><label>Name</label><input v-model="name" type="text" placeholder="Full Name" required /></div>
        <div style="flex:1"><label>Email</label><input v-model="email" type="email" placeholder="email@example.com" required /></div>
        <div style="flex:1"><label>Phone</label><input v-model="phone" type="text" placeholder="+1234567890" /></div>
        <button class="btn" style="height:42px" type="submit">Add</button>
      </div>
    </form>
    <h3 style="color:#1976D2;margin-top:24px;margin-bottom:12px">Your Contacts ({{ contacts.length }})</h3>
    <table class="message-list">
      <thead><tr><th>Name</th><th>Email</th><th>Phone</th><th>Actions</th></tr></thead>
      <tbody>
        <tr v-for="c in contacts" :key="c.id">
          <td>{{ c.name }}</td>
          <td>{{ c.email }}</td>
          <td>{{ c.phone }}</td>
          <td style="display:flex;gap:4px">
            <router-link :to="{ path: '/compose', query: { to: c.email } }" class="btn btn-sm">Email</router-link>
            <button class="btn btn-sm btn-secondary" @click="remove(c.id)">Delete</button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
