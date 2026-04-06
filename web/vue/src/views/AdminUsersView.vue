<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import AdminNav from '../components/AdminNav.vue'
import AlertBanner from '../components/AlertBanner.vue'

interface AdminUser { email: string; displayName: string; domain: string }

const authed = ref(false)
const secret = ref('')
const users = ref<AdminUser[]>([])
const email = ref('')
const displayName = ref('')
const password = ref('')
const error = ref('')
const success = ref('')

async function login() {
  error.value = ''
  try { await api.post('/admin/login', { secret: secret.value }); authed.value = true; await load() }
  catch { error.value = 'Invalid admin secret' }
}

async function load() { const { data } = await api.get<AdminUser[]>('/admin/users'); users.value = data }

async function create() {
  error.value = ''; success.value = ''
  try {
    await api.post('/admin/users', { email: email.value, displayName: displayName.value, password: password.value })
    email.value = displayName.value = password.value = ''
    success.value = 'User created'
    await load()
  } catch (e: any) { error.value = e.response?.data?.error || 'Failed to create user' }
}

async function remove(e: string) {
  if (!confirm(`Delete user ${e}?`)) return
  await api.delete('/admin/users', { data: { email: e } })
  success.value = 'User deleted'
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
      <h3 style="color:#1976D2;margin-bottom:12px">Create User</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end">
        <div style="flex:1"><label>Email</label><input v-model="email" type="text" placeholder="user@domain.com" required /></div>
        <div style="flex:1"><label>Display Name</label><input v-model="displayName" type="text" placeholder="Full Name" /></div>
        <div style="flex:1"><label>Password</label><input v-model="password" type="password" required /></div>
        <button class="btn" style="height:42px" type="submit">Create</button>
      </div>
    </form>
    <h3 style="color:#1976D2;margin-top:24px;margin-bottom:12px">Users</h3>
    <table class="message-list">
      <thead><tr><th>Email</th><th>Display Name</th><th>Actions</th></tr></thead>
      <tbody>
        <tr v-for="u in users" :key="u.email">
          <td>{{ u.email }}</td><td>{{ u.displayName }}</td>
          <td><button class="btn btn-sm btn-secondary" @click="remove(u.email)">Delete</button></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
