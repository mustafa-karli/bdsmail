<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import AdminNav from '../components/AdminNav.vue'
import AlertBanner from '../components/AlertBanner.vue'
import type { DomainResult } from '../types'

const authed = ref(false)
const secret = ref('')
const domains = ref<string[]>([])
const newDomain = ref('')
const result = ref<DomainResult | null>(null)
const error = ref('')

async function login() {
  error.value = ''
  try {
    await api.post('/admin/login', { secret: secret.value })
    authed.value = true
    await load()
  } catch { error.value = 'Invalid admin secret' }
}

async function load() {
  const { data } = await api.get<string[]>('/admin/domains')
  domains.value = data
}

async function addDomain() {
  error.value = ''; result.value = null
  try {
    const { data } = await api.post<DomainResult>('/admin/domains', { domain: newDomain.value })
    result.value = data
    newDomain.value = ''
    await load()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to add domain'
  }
}

onMounted(async () => {
  try { await load(); authed.value = true } catch { /* not authed */ }
})
</script>

<template>
  <div v-if="!authed" class="login-container">
    <div class="card">
      <h2>Admin Access</h2>
      <p>Enter admin secret to continue</p>
      <AlertBanner :error="error" />
      <form @submit.prevent="login">
        <div class="form-group">
          <label for="secret">Admin Secret</label>
          <input id="secret" v-model="secret" type="password" required autofocus />
        </div>
        <button class="btn" style="width:100%" type="submit">Sign In</button>
      </form>
    </div>
  </div>
  <div v-else class="card">
    <AdminNav />
    <AlertBanner :error="error" />
    <div v-if="result" class="card" style="margin-top:12px;background:#f5faff">
      <h3 style="color:#1976D2;margin-bottom:12px">DNS Records for {{ result.domain }}</h3>
      <p style="margin-bottom:12px;color:#666;font-size:0.9em">Add these records to your DNS provider:</p>
      <table class="message-list">
        <thead><tr><th>Type</th><th>Name</th><th>Value</th><th>Priority</th></tr></thead>
        <tbody>
          <tr v-for="(r, i) in result.dnsRecords" :key="i">
            <td><strong>{{ r.type }}</strong></td><td>{{ r.name }}</td>
            <td style="word-break:break-all;max-width:400px;font-size:0.85em">{{ r.value }}</td>
            <td>{{ r.priority }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <form @submit.prevent="addDomain" style="margin-top:16px">
      <div class="form-group" style="display:flex;gap:8px;align-items:end">
        <div style="flex:1"><label for="domain">New Domain</label><input id="domain" v-model="newDomain" type="text" placeholder="newdomain.com" required /></div>
        <button class="btn" style="height:42px" type="submit">Add Domain</button>
      </div>
    </form>
    <h3 style="color:#1976D2;margin-top:24px;margin-bottom:12px">Active Domains</h3>
    <table class="message-list">
      <thead><tr><th>Domain</th><th>Mail Subdomain</th></tr></thead>
      <tbody><tr v-for="d in domains" :key="d"><td>{{ d }}</td><td>mail.{{ d }}</td></tr></tbody>
    </table>
  </div>
</template>
