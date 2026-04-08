<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api/client'
import AlertBanner from '../components/AlertBanner.vue'

const route = useRoute()
const router = useRouter()
const code = ref('')
const trustDevice = ref(false)
const loginToken = ref((route.query.token as string) || '')
const error = ref('')

function getFingerprint(): string {
  try { return btoa(navigator.userAgent).substring(0, 64) } catch { return '' }
}

async function submit() {
  error.value = ''
  try {
    const { data } = await api.post('/auth/verify-2fa', {
      loginToken: loginToken.value,
      code: code.value,
      trustDevice: trustDevice.value,
      deviceFingerprint: getFingerprint(),
      deviceName: navigator.platform || 'Unknown',
    })
    if (data.error) {
      error.value = data.error
      loginToken.value = data.loginToken || loginToken.value
    } else {
      router.push('/inbox')
    }
  } catch (e: any) {
    error.value = e.response?.data?.detail || 'Verification failed'
  }
}
</script>

<template>
  <div class="login-container">
    <div class="card">
      <img src="/static/bdsmail_logo1.png" alt="BDS Mail" style="width:80px;margin-bottom:12px">
      <h2>Two-Factor Authentication</h2>
      <p style="color:#666;margin-bottom:16px">Enter your authenticator code or a backup code.</p>
      <AlertBanner :error="error" />
      <form @submit.prevent="submit">
        <div class="form-group">
          <label for="code">Code</label>
          <input id="code" v-model="code" type="text" placeholder="123456" required autofocus autocomplete="one-time-code" inputmode="numeric" />
        </div>
        <div class="form-group">
          <label><input type="checkbox" v-model="trustDevice" /> Trust this device for 30 days</label>
        </div>
        <button class="btn" style="width:100%" type="submit">Verify</button>
      </form>
      <p style="margin-top:12px;font-size:0.85em;color:#888;text-align:center">
        <router-link to="/login">Back to login</router-link>
      </p>
    </div>
  </div>
</template>
