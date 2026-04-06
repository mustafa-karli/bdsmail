<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'

const props = defineProps<{ page: number; totalPages: number }>()
const route = useRoute()
const router = useRouter()

function goTo(p: number) {
  router.push({ path: route.path, query: { ...route.query, page: String(p) } })
}
</script>

<template>
  <div class="pagination">
    <a v-if="props.page > 1" @click.prevent="goTo(props.page - 1)" href="#">&laquo; Newer</a>
    <span v-else class="disabled">&laquo; Newer</span>
    <span class="current">Page {{ props.page }} of {{ props.totalPages }}</span>
    <a v-if="props.page < props.totalPages" @click.prevent="goTo(props.page + 1)" href="#">Older &raquo;</a>
    <span v-else class="disabled">Older &raquo;</span>
  </div>
</template>
