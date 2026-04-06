<script setup lang="ts">
defineProps<{ error?: string; success?: string }>()
</script>

<template>
  <div v-if="error" class="alert alert-error">{{ error }}</div>
  <div v-if="success" class="alert alert-success">{{ success }}</div>
</template>
