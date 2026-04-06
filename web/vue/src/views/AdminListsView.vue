<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import AdminNav from '../components/AdminNav.vue'
import AlertBanner from '../components/AlertBanner.vue'
import type { MailingList } from '../types'

const authed = ref(false)
const secret = ref('')
const lists = ref<MailingList[]>([])
const listAddress = ref('')
const name = ref('')
const ownerEmail = ref('')
const memberListAddr = ref('')
const memberEmail = ref('')
const error = ref('')
const success = ref('')

async function login() {
  error.value = ''
  try { await api.post('/admin/login', { secret: secret.value }); authed.value = true; await load() }
  catch { error.value = 'Invalid admin secret' }
}

async function load() { const { data } = await api.get<MailingList[]>('/admin/lists'); lists.value = data }

async function create() {
  error.value = ''; success.value = ''
  try {
    await api.post('/admin/lists', { listAddress: listAddress.value, name: name.value, ownerEmail: ownerEmail.value })
    listAddress.value = name.value = ownerEmail.value = ''
    success.value = 'Mailing list created'
    await load()
  } catch (e: any) { error.value = e.response?.data?.error || 'Failed to create list' }
}

async function addMember() {
  error.value = ''; success.value = ''
  try {
    await api.post('/admin/lists/members', { listAddress: memberListAddr.value, memberEmail: memberEmail.value })
    memberEmail.value = ''
    success.value = 'Member added'
    await load()
  } catch (e: any) { error.value = e.response?.data?.error || 'Failed to add member' }
}

async function remove(addr: string) {
  if (!confirm(`Delete list ${addr}?`)) return
  await api.delete('/admin/lists', { data: { listAddress: addr } })
  success.value = 'List deleted'
  await load()
}

onMounted(async () => { try { await load(); authed.value = true } catch { /* */ } })
</script>

<template>
  <div v-if="!authed" class="login-container">
    <div class="card">
      <h2>Admin Access</h2><p>Enter admin secret to continue</p>
      <AlertBanner :error="error" />
      <form @submit.prevent="login">
        <div class="form-group"><label>Admin Secret</label><input v-model="secret" type="password" required autofocus /></div>
        <button class="btn" style="width:100%" type="submit">Sign In</button>
      </form>
    </div>
  </div>
  <div v-else class="card">
    <AdminNav />
    <AlertBanner :error="error" :success="success" />
    <form @submit.prevent="create" style="margin-top:16px">
      <h3 style="color:#1976D2;margin-bottom:12px">Create Mailing List</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end">
        <div style="flex:1"><label>List Address</label><input v-model="listAddress" type="text" placeholder="team@domain.com" required /></div>
        <div style="flex:1"><label>Name</label><input v-model="name" type="text" placeholder="Team Updates" required /></div>
        <div style="flex:1"><label>Owner Email</label><input v-model="ownerEmail" type="text" placeholder="admin@domain.com" required /></div>
        <button class="btn" style="height:42px" type="submit">Create</button>
      </div>
    </form>
    <form @submit.prevent="addMember" style="margin-top:16px">
      <h3 style="color:#1976D2;margin-bottom:12px">Add Member</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end">
        <div style="flex:1"><label>List</label>
          <select v-model="memberListAddr">
            <option v-for="l in lists" :key="l.listAddress" :value="l.listAddress">{{ l.listAddress }}</option>
          </select>
        </div>
        <div style="flex:1"><label>Member Email</label><input v-model="memberEmail" type="text" placeholder="user@domain.com" required /></div>
        <button class="btn" style="height:42px" type="submit">Add</button>
      </div>
    </form>
    <h3 style="color:#1976D2;margin-top:24px;margin-bottom:12px">Mailing Lists</h3>
    <table class="message-list">
      <thead><tr><th>Address</th><th>Name</th><th>Owner</th><th>Actions</th></tr></thead>
      <tbody>
        <tr v-for="l in lists" :key="l.listAddress">
          <td>{{ l.listAddress }}</td><td>{{ l.name }}</td><td>{{ l.ownerEmail }}</td>
          <td><button class="btn btn-sm btn-secondary" @click="remove(l.listAddress)">Delete</button></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
