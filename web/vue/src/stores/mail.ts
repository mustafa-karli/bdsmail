import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../api/client'

export const useMailStore = defineStore('mail', () => {
  const unreadCount = ref(0)
  const folders = ref<string[]>([])

  async function fetchUnread() {
    try {
      const { data } = await api.get<{ count: number }>('/unread')
      unreadCount.value = data.count
    } catch {
      unreadCount.value = 0
    }
  }

  async function fetchFolders() {
    try {
      const { data } = await api.get<string[]>('/folders')
      folders.value = data
    } catch {
      folders.value = []
    }
  }

  return { unreadCount, folders, fetchUnread, fetchFolders }
})
