import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
    { path: '/', redirect: '/inbox' },
    { path: '/inbox', component: () => import('../views/InboxView.vue'), props: { folder: 'INBOX' } },
    { path: '/sent', component: () => import('../views/InboxView.vue'), props: { folder: 'Sent' } },
    { path: '/folder/:name', component: () => import('../views/InboxView.vue'), props: (route) => ({ folder: route.params.name }) },
    { path: '/message/:id', component: () => import('../views/MessageView.vue'), props: true },
    { path: '/compose', component: () => import('../views/ComposeView.vue') },
    { path: '/search', component: () => import('../views/SearchView.vue') },
    { path: '/contacts', component: () => import('../views/ContactsView.vue') },
    { path: '/filters', component: () => import('../views/FiltersView.vue') },
    { path: '/settings/autoreply', component: () => import('../views/AutoReplyView.vue') },
    { path: '/developer', component: () => import('../views/DeveloperView.vue') },
    { path: '/admin/domains', component: () => import('../views/AdminDomainsView.vue'), meta: { admin: true } },
    { path: '/admin/users', component: () => import('../views/AdminUsersView.vue'), meta: { admin: true } },
    { path: '/admin/aliases', component: () => import('../views/AdminAliasesView.vue'), meta: { admin: true } },
    { path: '/admin/lists', component: () => import('../views/AdminListsView.vue'), meta: { admin: true } },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (auth.loading) await auth.fetchUser()
  if (!to.meta.public && !to.meta.admin && !auth.user) return '/login'
})

export default router
