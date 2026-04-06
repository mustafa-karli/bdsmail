<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import AdminNav from '../components/AdminNav.vue'
import AlertBanner from '../components/AlertBanner.vue'
import type { Alias } from '../types'

const authed = ref(false)
const secret = ref('')
const aliases = ref<Alias[]>([])
const aliasEmail = ref('')
const targetEmails = ref('')
const error = ref('')
const success = ref('')

async function login() {
  error.value = ''
  try { await api.post('/admin/login', { secret: secret.value }); authed.value = true; await load() }
  catch { error.value = 'Invalid admin secret' }
}

async function load() { const { data } = await api.get<Alias[]>('/admin/aliases'); aliases.value = data }

async function create() {
  error.value = ''; success.value = ''
  try {
    await api.post('/admin/aliases', { aliasEmail: aliasEmail.value, targetEmails: targetEmails.value })
    aliasEmail.value = targetEmails.value = ''
    success.value = 'Alias created'
    await load()
  } catch (e: any) { error.value = e.response?.data?.error || 'Failed to create alias' }
}

async function remove(e: string) {
  await api.delete('/admin/aliases', { data: { aliasEmail: e } })
  success.value = 'Alias deleted'
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
      <h3 style="color:#1976D2;margin-bottom:12px">Create Alias</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end">
        <div style="flex:1"><label>Alias Email</label><input v-model="aliasEmail" type="text" placeholder="alias@domain.com or @domain.com" required /></div>
        <div style="flex:1"><label>Target Emails</label><input v-model="targetEmails" type="text" placeholder="user1@domain.com, user2@domain.com" required /></div>
        <button class="btn" style="height:42px" type="submit">Create</button>
      </div>
    </form>
    <h3 style="color:#1976D2;margin-top:24px;margin-bottom:12px">Aliases</h3>
    <table class="message-list">
      <thead><tr><th>Alias</th><th>Targets</th><th>Actions</th></tr></thead>
      <tbody>
        <tr v-for="a in aliases" :key="a.aliasEmail">
          <td>{{ a.aliasEmail }}{{ a.isCatchAll ? ' (catch-all)' : '' }}</td>
          <td>{{ a.targetEmails.join(', ') }}</td>
          <td><button class="btn btn-sm btn-secondary" @click="remove(a.aliasEmail)">Delete</button></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
