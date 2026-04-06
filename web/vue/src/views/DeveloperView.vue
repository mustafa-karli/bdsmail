<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import { useAuthStore } from '../stores/auth'
import AlertBanner from '../components/AlertBanner.vue'

interface OAuthClient { id: string; name: string; clientId: string; redirectUri: string; createdAt: string }
interface NewClient extends OAuthClient { clientSecret: string }

const auth = useAuthStore()
const clients = ref<OAuthClient[]>([])
const name = ref('')
const redirectUri = ref('')
const error = ref('')
const success = ref('')
const newClient = ref<NewClient | null>(null)

async function load() {
  const { data } = await api.get<OAuthClient[]>('/oauth/clients')
  clients.value = data
}

async function create() {
  error.value = ''; success.value = ''; newClient.value = null
  try {
    const { data } = await api.post<NewClient>('/oauth/clients', { name: name.value, redirectUri: redirectUri.value })
    newClient.value = data
    name.value = redirectUri.value = ''
    success.value = 'Application registered'
    await load()
  } catch (e: any) { error.value = e.response?.data?.error || 'Failed to register' }
}

async function remove(id: string) {
  await api.delete(`/oauth/clients/${id}`)
  success.value = 'Application deleted'
  await load()
}

onMounted(load)
</script>

<template>
  <div class="card">
    <div class="toolbar">
      <h2>Developer Portal</h2>
      <router-link to="/inbox" class="btn btn-sm btn-secondary">Back to Inbox</router-link>
    </div>
    <p style="color:#666;margin-bottom:16px">Register applications to enable "Sign in with {{ auth.user?.domain }}" for your users.</p>
    <AlertBanner :error="error" :success="success" />
    <div v-if="newClient" class="alert alert-success" style="background:#fff3cd;color:#856404;border-color:#ffeeba">
      <strong>Save these credentials now — the secret won't be shown again.</strong><br><br>
      <strong>Client ID:</strong> <code>{{ newClient.clientId }}</code><br>
      <strong>Client Secret:</strong> <code>{{ newClient.clientSecret }}</code>
    </div>
    <form @submit.prevent="create" style="margin-top:16px">
      <h3 style="color:#1976D2;margin-bottom:12px">Register Application</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end">
        <div style="flex:1"><label>App Name</label><input v-model="name" type="text" placeholder="My App" required /></div>
        <div style="flex:2"><label>Redirect URI</label><input v-model="redirectUri" type="text" placeholder="https://myapp.com/callback" required /></div>
        <button class="btn" style="height:42px" type="submit">Register</button>
      </div>
    </form>
    <h3 style="color:#1976D2;margin-top:24px;margin-bottom:12px">Your Applications</h3>
    <table class="message-list">
      <thead><tr><th>Name</th><th>Client ID</th><th>Redirect URI</th><th>Actions</th></tr></thead>
      <tbody>
        <tr v-for="c in clients" :key="c.id">
          <td>{{ c.name }}</td>
          <td><code style="font-size:0.8em">{{ c.clientId }}</code></td>
          <td style="font-size:0.85em">{{ c.redirectUri }}</td>
          <td><button class="btn btn-sm btn-secondary" @click="remove(c.id)">Delete</button></td>
        </tr>
      </tbody>
    </table>
    <h3 style="color:#1976D2;margin-top:24px;margin-bottom:12px">Integration Guide</h3>
    <div style="background:#f5f5f5;padding:16px;border-radius:4px;font-size:0.9em">
      <p><strong>Authorization URL:</strong> <code>https://{{ auth.user?.domain }}/oauth/authorize</code></p>
      <p><strong>Token URL:</strong> <code>https://{{ auth.user?.domain }}/oauth/token</code></p>
      <p><strong>UserInfo URL:</strong> <code>https://{{ auth.user?.domain }}/oauth/userinfo</code></p>
      <p><strong>OIDC Discovery:</strong> <code>https://{{ auth.user?.domain }}/.well-known/openid-configuration</code></p>
      <p style="margin-top:8px"><strong>Scopes:</strong> <code>openid email profile</code></p>
    </div>
  </div>
</template>
