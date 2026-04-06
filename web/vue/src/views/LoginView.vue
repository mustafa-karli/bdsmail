<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import AlertBanner from '../components/AlertBanner.vue'

const auth = useAuthStore()
const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  try {
    await auth.login(username.value, password.value)
    router.push('/inbox')
  } catch {
    error.value = 'Invalid username or password'
  }
}
</script>

<template>
  <div class="login-container">
    <div class="card">
      <h2>BDS Mail</h2>
      <p>Sign in to your account</p>
      <AlertBanner :error="error" />
      <form @submit.prevent="submit">
        <div class="form-group">
          <label for="username">Username</label>
          <input id="username" v-model="username" type="text" required autofocus />
        </div>
        <div class="form-group">
          <label for="password">Password</label>
          <input id="password" v-model="password" type="password" required />
        </div>
        <button class="btn" style="width:100%" type="submit">Sign In</button>
      </form>
    </div>
  </div>
</template>
