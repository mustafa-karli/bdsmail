<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api/client'
import { useAuthStore } from '../stores/auth'
import { extractEmail, quoteBody, forwardBody } from '../utils'
import AlertBanner from '../components/AlertBanner.vue'
import type { Message } from '../types'

const route = useRoute()
const auth = useAuthStore()

const to = ref('')
const cc = ref('')
const bcc = ref('')
const subject = ref('')
const body = ref('')
const contentType = ref('text/plain')
const files = ref<FileList | null>(null)
const error = ref('')
const success = ref('')
const dirty = ref(false)

function beforeUnload(e: BeforeUnloadEvent) {
  if (dirty.value) { e.preventDefault(); e.returnValue = '' }
}
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload)
  prefill()
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))

async function prefill() {
  if (route.query.to) to.value = route.query.to as string

  const replyId = (route.query.reply || route.query.replyall) as string
  if (replyId) {
    const { data: msg } = await api.get<Message>(`/messages/${replyId}`)
    to.value = extractEmail(msg.from)
    if (route.query.replyall) {
      const others = [...msg.to, ...msg.cc]
        .map(extractEmail)
        .filter((a) => a !== auth.user?.email && a !== extractEmail(msg.from))
      cc.value = others.join(', ')
    }
    subject.value = msg.subject.toLowerCase().startsWith('re:') ? msg.subject : `Re: ${msg.subject}`
    body.value = quoteBody(msg.from, msg.receivedAt, msg.body)
  }

  const fwdId = route.query.forward as string
  if (fwdId) {
    const { data: msg } = await api.get<Message>(`/messages/${fwdId}`)
    subject.value = msg.subject.toLowerCase().startsWith('fwd:') ? msg.subject : `Fwd: ${msg.subject}`
    body.value = forwardBody(msg.from, msg.receivedAt, msg.to.join(', '), msg.subject, msg.body)
  }
}

async function submit() {
  error.value = ''
  success.value = ''
  const form = new FormData()
  form.append('to', to.value)
  form.append('cc', cc.value)
  form.append('bcc', bcc.value)
  form.append('subject', subject.value)
  form.append('body', body.value)
  form.append('content_type', contentType.value)
  if (files.value) {
    for (const f of files.value) form.append('attachments', f)
  }
  try {
    await api.post('/compose', form)
    dirty.value = false
    success.value = 'Message sent successfully'
    to.value = cc.value = bcc.value = subject.value = body.value = ''
    files.value = null
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to send message'
  }
}
</script>

<template>
  <div class="card">
    <h2>Compose Message</h2>
    <p class="message-meta" style="margin-bottom:16px">
      Sending as <strong>{{ auth.user?.displayName ? `${auth.user.displayName} <${auth.user.email}>` : auth.user?.email }}</strong>
    </p>
    <AlertBanner :error="error" :success="success" />
    <form @submit.prevent="submit" @input="dirty = true">
      <div class="form-group">
        <label for="to">To</label>
        <input id="to" v-model="to" type="text" placeholder="recipient@example.com" required />
      </div>
      <div class="form-group">
        <label for="cc">CC</label>
        <input id="cc" v-model="cc" type="text" placeholder="cc@example.com (comma-separated)" />
      </div>
      <div class="form-group">
        <label for="bcc">BCC</label>
        <input id="bcc" v-model="bcc" type="text" placeholder="bcc@example.com (comma-separated)" />
      </div>
      <div class="form-group">
        <label for="subject">Subject</label>
        <input id="subject" v-model="subject" type="text" />
      </div>
      <div class="form-group">
        <label for="content_type">Format</label>
        <select id="content_type" v-model="contentType">
          <option value="text/plain">Plain Text</option>
          <option value="text/html">HTML</option>
        </select>
      </div>
      <div class="form-group">
        <label for="body">Body</label>
        <textarea id="body" v-model="body" @input="($event.target as HTMLTextAreaElement).style.height = 'auto'; ($event.target as HTMLTextAreaElement).style.height = ($event.target as HTMLTextAreaElement).scrollHeight + 'px'"></textarea>
      </div>
      <div class="form-group">
        <label for="attachments">Attachments</label>
        <input id="attachments" type="file" multiple @change="files = ($event.target as HTMLInputElement).files" />
      </div>
      <button class="btn" type="submit">Send</button>
      <router-link to="/inbox" class="btn btn-secondary" style="margin-left:8px">Cancel</router-link>
    </form>
  </div>
</template>
