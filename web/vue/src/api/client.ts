import axios from 'axios'
import router from '../router'

// Detect backend URL:
// - If served from Go binary (/app/ or localhost:5173), use relative /api
// - If served from Amplify (webmail.domain.com), call mail.domain.com/api
function getBaseURL(): string {
  const host = window.location.hostname
  if (host.startsWith('webmail.')) {
    const domain = host.replace('webmail.', '')
    return `https://mail.${domain}/api`
  }
  return '/api'
}

const api = axios.create({
  baseURL: getBaseURL(),
  withCredentials: true,
})

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      router.push('/login')
    }
    return Promise.reject(err)
  },
)

export default api
