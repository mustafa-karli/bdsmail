<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api/client'
import AlertBanner from '../components/AlertBanner.vue'
import type { Filter } from '../types'

const filters = ref<Filter[]>([])
const name = ref('')
const field = ref('from')
const operator = ref('contains')
const value = ref('')
const actionType = ref('move')
const actionValue = ref('')
const error = ref('')
const success = ref('')

async function load() {
  const { data } = await api.get<Filter[]>('/filters')
  filters.value = data
}

async function create() {
  error.value = ''; success.value = ''
  try {
    await api.post('/filters', {
      name: name.value, field: field.value, operator: operator.value,
      value: value.value, actionType: actionType.value, actionValue: actionValue.value,
    })
    name.value = value.value = actionValue.value = ''
    success.value = 'Filter created'
    await load()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to create filter'
  }
}

async function remove(id: string) {
  await api.delete(`/filters/${id}`)
  success.value = 'Filter deleted'
  await load()
}

onMounted(load)
</script>

<template>
  <div class="card">
    <div class="toolbar">
      <h2>Mail Filters</h2>
      <router-link to="/inbox" class="btn btn-sm btn-secondary">Back to Inbox</router-link>
    </div>
    <AlertBanner :error="error" :success="success" />
    <form @submit.prevent="create" style="margin-top:16px">
      <h3 style="color:#1976D2;margin-bottom:12px">New Filter</h3>
      <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end">
        <div><label>Name</label><input v-model="name" type="text" placeholder="Filter name" required /></div>
        <div><label>Field</label>
          <select v-model="field"><option value="from">From</option><option value="to">To</option><option value="subject">Subject</option></select>
        </div>
        <div><label>Operator</label>
          <select v-model="operator"><option value="contains">Contains</option><option value="equals">Equals</option><option value="not_contains">Not Contains</option></select>
        </div>
        <div style="flex:1"><label>Value</label><input v-model="value" type="text" placeholder="match text" required /></div>
        <div><label>Action</label>
          <select v-model="actionType"><option value="move">Move to folder</option><option value="mark_read">Mark as read</option><option value="delete">Delete</option><option value="flag">Flag</option></select>
        </div>
        <div><label>Action Value</label><input v-model="actionValue" type="text" placeholder="folder name" /></div>
        <button class="btn" style="height:42px" type="submit">Create</button>
      </div>
    </form>
    <h3 style="color:#1976D2;margin-top:24px;margin-bottom:12px">Active Filters</h3>
    <table class="message-list">
      <thead><tr><th>Name</th><th>Priority</th><th>Enabled</th><th>Actions</th></tr></thead>
      <tbody>
        <tr v-for="f in filters" :key="f.id">
          <td>{{ f.name }}</td>
          <td>{{ f.priority }}</td>
          <td>{{ f.enabled ? 'Yes' : 'No' }}</td>
          <td><button class="btn btn-sm btn-secondary" @click="remove(f.id)">Delete</button></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
