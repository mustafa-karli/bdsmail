import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../api/client'
import type { User } from '../types'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const loading = ref(true)

  async function fetchUser() {
    try {
      const { data } = await api.get<User>('/auth/me')
      user.value = data
    } catch {
      user.value = null
    } finally {
      loading.value = false
    }
  }

  async function login(username: string, password: string) {
    const { data } = await api.post<User>('/auth/login', { username, password })
    user.value = data
  }

  async function logout() {
    await api.post('/auth/logout')
    user.value = null
  }

  return { user, loading, fetchUser, login, logout }
})
