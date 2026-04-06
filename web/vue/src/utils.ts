import type { Message } from './types'

export function formatRelativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const mins = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)
  const days = Math.floor(diff / 86400000)

  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins} min ago`
  if (hours < 24) return `${hours} hour${hours > 1 ? 's' : ''} ago`
  if (hours < 48) return 'yesterday'
  if (days < 7) return `${days} days ago`

  const d = new Date(dateStr)
  const now = new Date()
  return d.getFullYear() === now.getFullYear()
    ? d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' })
    : d.toLocaleDateString('en-US', { month: 'short', day: '2-digit', year: 'numeric' })
}

export function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`
  return `${(bytes / 1073741824).toFixed(1)} GB`
}

export function hasAttachments(msg: Message): boolean {
  return msg.attachments?.length > 0
}

export function extractEmail(s: string): string {
  const match = s.match(/<(.+?)>/)
  return match ? match[1] : s.trim()
}

export function quoteBody(from: string, date: string, body: string): string {
  const d = new Date(date).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short' })
  const quoted = body.split('\n').map((l) => `> ${l}`).join('\n')
  return `\n\nOn ${d}, ${from} wrote:\n${quoted}\n`
}

export function forwardBody(from: string, date: string, to: string, subject: string, body: string): string {
  const d = new Date(date).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short' })
  return `\n\n---------- Forwarded message ----------\nFrom: ${from}\nDate: ${d}\nSubject: ${subject}\nTo: ${to}\n\n${body}`
}
