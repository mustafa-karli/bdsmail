<script setup lang="ts">
import { onMounted } from 'vue'
import { useAuthStore } from './stores/auth'
import NavBar from './components/NavBar.vue'

const auth = useAuthStore()
onMounted(() => auth.fetchUser())

function onKeydown(e: KeyboardEvent) {
  const tag = (e.target as HTMLElement).tagName
  if (e.key === 'c' && !['INPUT', 'TEXTAREA', 'SELECT'].includes(tag)) {
    window.location.href = '/compose'
  }
}
</script>

<template>
  <div @keydown="onKeydown">
    <NavBar v-if="auth.user" />
    <div class="container">
      <router-view />
    </div>
  </div>
</template>
