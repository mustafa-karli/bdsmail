<script setup lang="ts">
import { ref } from 'vue'
import api from '../api/client'
import AlertBanner from '../components/AlertBanner.vue'

interface DnsRecord { type: string; name: string; value: string; priority: string }

const step = ref(1) // 1=form, 2=verify DNS, 3=complete

// Step 1: Form
const domain = ref('')
const username = ref('')
const displayName = ref('')
const password = ref('')

// Step 2: Verify
const signupId = ref('')
const dnsRecords = ref<DnsRecord[]>([])

// Step 3: Complete
const completionRecords = ref<DnsRecord[]>([])

const error = ref('')
const success = ref('')
const loading = ref(false)

async function submitSignup() {
  error.value = ''; loading.value = true
  try {
    const { data } = await api.post('/signup', {
      domain: domain.value,
      username: username.value,
      displayName: displayName.value,
      password: password.value,
    })
    signupId.value = data.signupId
    dnsRecords.value = data.dnsRecords
    step.value = 2
  } catch (e: any) {
    error.value = e.response?.data?.detail || 'Signup failed'
  } finally { loading.value = false }
}

async function verifyDns() {
  error.value = ''; loading.value = true
  try {
    const { data } = await api.post('/signup/verify', { signupId: signupId.value })
    if (data.status === 'verified') {
      completionRecords.value = data.dnsRecords || []
      success.value = `Domain ${domain.value} is ready! You are logged in as ${data.email}.`
      step.value = 3
    }
  } catch (e: any) {
    error.value = e.response?.data?.detail || 'DNS verification failed. Add the MX record and wait for propagation.'
  } finally { loading.value = false }
}
</script>

<template>
  <div style="max-width:600px;margin:40px auto">
    <!-- Step 1: Signup Form -->
    <div v-if="step === 1" class="card">
      <img src="/static/bdsmail_logo1.png" alt="BDS Mail" style="width:80px;margin-bottom:12px">
      <h2>Register Your Domain</h2>
      <p style="color:#666;margin-bottom:16px">Get your own email server in minutes.</p>
      <AlertBanner :error="error" />
      <form @submit.prevent="submitSignup">
        <div class="form-group">
          <label>Domain Name</label>
          <input v-model="domain" type="text" placeholder="yourdomain.com" required />
        </div>
        <div class="form-group">
          <label>Username</label>
          <input v-model="username" type="text" placeholder="admin" required />
          <p style="font-size:0.8em;color:#888">Your email will be {{ username || 'username' }}@{{ domain || 'yourdomain.com' }}</p>
        </div>
        <div class="form-group">
          <label>Display Name</label>
          <input v-model="displayName" type="text" placeholder="John Smith" />
        </div>
        <div class="form-group">
          <label>Password</label>
          <input v-model="password" type="password" placeholder="min 8 characters" required minlength="8" />
        </div>
        <button class="btn" style="width:100%" type="submit" :disabled="loading">
          {{ loading ? 'Creating...' : 'Continue' }}
        </button>
      </form>
      <p style="margin-top:12px;font-size:0.85em;color:#888;text-align:center">
        Already have an account? <router-link to="/login">Login</router-link>
      </p>
    </div>

    <!-- Step 2: DNS Verification -->
    <div v-if="step === 2" class="card">
      <h2>Verify Domain: {{ domain }}</h2>
      <AlertBanner :error="error" />
      <p style="color:#666;margin-bottom:16px">Add these DNS records to your domain, then click <strong>Verify</strong>.</p>
      <table class="message-list">
        <thead><tr><th>Type</th><th>Name</th><th>Value</th><th>Priority</th></tr></thead>
        <tbody>
          <tr v-for="(r, i) in dnsRecords" :key="i">
            <td><strong>{{ r.type }}</strong></td>
            <td>{{ r.name }}</td>
            <td style="word-break:break-all;max-width:350px;font-size:0.85em">{{ r.value }}</td>
            <td>{{ r.priority }}</td>
          </tr>
        </tbody>
      </table>
      <div style="margin-top:16px;padding:12px;background:#f5faff;border-radius:4px">
        <p style="font-size:0.9em;color:#555;margin-bottom:8px"><strong>How to add DNS records:</strong></p>
        <ol style="font-size:0.85em;color:#666;padding-left:20px">
          <li>Log in to your DNS provider (GoDaddy, Cloudflare, Route 53, etc.)</li>
          <li>Add each record above to your domain</li>
          <li>Wait 5-15 minutes for DNS propagation</li>
          <li>Click Verify below</li>
        </ol>
      </div>
      <button class="btn" style="width:100%;margin-top:16px" @click="verifyDns" :disabled="loading">
        {{ loading ? 'Verifying...' : 'Verify DNS Records' }}
      </button>
    </div>

    <!-- Step 3: Complete -->
    <div v-if="step === 3" class="card">
      <h2>Domain Ready!</h2>
      <div class="alert alert-success">{{ success }}</div>
      <div v-if="completionRecords.length">
        <h3 style="color:#1976D2;margin-bottom:12px">Additional DNS Records</h3>
        <p style="color:#666;margin-bottom:12px">Add these for DKIM signing and email delivery:</p>
        <table class="message-list">
          <thead><tr><th>Type</th><th>Name</th><th>Value</th><th>Priority</th></tr></thead>
          <tbody>
            <tr v-for="(r, i) in completionRecords" :key="i">
              <td><strong>{{ r.type }}</strong></td>
              <td>{{ r.name }}</td>
              <td style="word-break:break-all;max-width:350px;font-size:0.85em">{{ r.value }}</td>
              <td>{{ r.priority }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <router-link to="/inbox" class="btn" style="width:100%;display:block;text-align:center;margin-top:16px">Go to Inbox</router-link>
    </div>
  </div>
</template>
