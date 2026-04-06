<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import AlertBanner from '../components/AlertBanner.vue'
import type { AutoReply } from '../types'

const enabled = ref(false)
const subject = ref('')
const body = ref('')
const startDate = ref('')
const endDate = ref('')
const error = ref('')
const success = ref('')

async function load() {
  try {
    const { data } = await api.get<AutoReply>('/autoreply')
    enabled.value = data.enabled
    subject.value = data.subject
    body.value = data.body
    startDate.value = data.startDate || ''
    endDate.value = data.endDate || ''
  } catch { /* no auto-reply configured */ }
}

async function save() {
  error.value = ''; success.value = ''
  try {
    await api.post('/autoreply', {
      enabled: enabled.value, subject: subject.value, body: body.value,
      startDate: startDate.value, endDate: endDate.value,
    })
    success.value = 'Auto-reply settings saved'
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to save'
  }
}

onMounted(load)
</script>

<template>
  <div class="card">
    <div class="toolbar">
      <h2>Auto-Reply / Vacation</h2>
      <router-link to="/inbox" class="btn btn-sm btn-secondary">Back to Inbox</router-link>
    </div>
    <AlertBanner :error="error" :success="success" />
    <form @submit.prevent="save" style="margin-top:16px">
      <div class="form-group">
        <label><input type="checkbox" v-model="enabled" /> Enable Auto-Reply</label>
      </div>
      <div class="form-group">
        <label for="subject">Subject</label>
        <input id="subject" v-model="subject" type="text" placeholder="Out of Office" />
      </div>
      <div class="form-group">
        <label for="body">Message Body</label>
        <textarea id="body" v-model="body" rows="6" placeholder="I am currently out of the office..."></textarea>
      </div>
      <div style="display:flex;gap:16px">
        <div class="form-group" style="flex:1">
          <label for="start_date">Start Date (optional)</label>
          <input id="start_date" v-model="startDate" type="date" />
        </div>
        <div class="form-group" style="flex:1">
          <label for="end_date">End Date (optional)</label>
          <input id="end_date" v-model="endDate" type="date" />
        </div>
      </div>
      <button class="btn" type="submit">Save Settings</button>
    </form>
  </div>
</template>
