<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import AlertBanner from '../components/AlertBanner.vue'

const enabled = ref(false)
const secret = ref('')
const qrUri = ref('')
const backupCodes = ref<string[]>([])
const disableCode = ref('')
const error = ref('')
const success = ref('')

async function load() {
  try {
    await api.get('/auth/me')
  } catch { /* not logged in */ }
}

async function setup() {
  error.value = ''; success.value = ''
  try {
    const { data } = await api.post('/auth/2fa/setup')
    secret.value = data.secret
    qrUri.value = data.qrUri
    backupCodes.value = data.backupCodes
    enabled.value = true
    success.value = '2FA enabled. Scan the QR code and save your backup codes.'
  } catch (e: any) { error.value = e.response?.data?.detail || 'Setup failed' }
}

async function disable() {
  error.value = ''; success.value = ''
  try {
    await api.post('/auth/2fa/disable', { code: disableCode.value })
    enabled.value = false
    secret.value = qrUri.value = ''
    backupCodes.value = []
    success.value = '2FA has been disabled'
  } catch (e: any) { error.value = e.response?.data?.detail || 'Failed to disable' }
}

onMounted(load)
</script>

<template>
  <div class="card">
    <div class="toolbar">
      <h2>Two-Factor Authentication</h2>
      <router-link to="/inbox" class="btn btn-sm btn-secondary">Back to Inbox</router-link>
    </div>
    <AlertBanner :error="error" :success="success" />

    <div v-if="qrUri" style="background:#f5faff;padding:16px;border-radius:4px;margin-bottom:16px">
      <h3 style="color:#1976D2;margin-bottom:12px">Scan QR Code</h3>
      <p style="margin-bottom:8px;font-size:0.9em;color:#555">Scan with your authenticator app:</p>
      <p><img :src="'https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=' + encodeURIComponent(qrUri)" width="200" height="200" /></p>
      <p style="font-size:0.85em;color:#888">Or enter manually: <code>{{ secret }}</code></p>
    </div>

    <div v-if="backupCodes.length" class="alert alert-success" style="background:#fff3cd;color:#856404;border-color:#ffeeba">
      <strong>Save these backup codes now — they won't be shown again.</strong><br><br>
      <code v-for="c in backupCodes" :key="c" style="display:inline-block;margin:2px 4px;padding:2px 6px;background:#fff;border:1px solid #ddd;border-radius:3px">{{ c }}</code>
    </div>

    <div v-if="enabled" style="margin-top:16px">
      <p style="color:#2e7d32;margin-bottom:16px">2FA is <strong>enabled</strong> for your account.</p>
      <form @submit.prevent="disable" style="display:flex;gap:8px;align-items:end">
        <div style="flex:1"><label>Enter current 2FA code to disable</label><input v-model="disableCode" type="text" placeholder="123456" required /></div>
        <button class="btn btn-danger" style="height:42px" type="submit">Disable 2FA</button>
      </form>
    </div>
    <div v-else style="margin-top:16px">
      <p style="color:#666;margin-bottom:16px">2FA is <strong>not enabled</strong> for your account.</p>
      <button class="btn" @click="setup">Enable 2FA</button>
    </div>
  </div>
</template>
