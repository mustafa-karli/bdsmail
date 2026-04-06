export interface User {
  username: string
  displayName: string
  email: string
  domain: string
}

export interface Attachment {
  id: string
  filename: string
  contentType: string
  size: number
  bucketKey: string
}

export interface Message {
  id: string
  messageId: string
  from: string
  to: string[]
  cc: string[]
  bcc: string[]
  subject: string
  contentType: string
  body: string
  attachments: Attachment[]
  ownerUser: string
  folder: string
  seen: boolean
  receivedAt: string
}

export interface PaginatedMessages {
  messages: Message[]
  page: number
  totalPages: number
  unreadCount: number
}

export interface FilterCondition {
  field: string
  operator: string
  value: string
}

export interface FilterAction {
  type: string
  value: string
}

export interface Filter {
  id: string
  name: string
  priority: number
  conditions: FilterCondition[]
  actions: FilterAction[]
  enabled: boolean
}

export interface AutoReply {
  enabled: boolean
  subject: string
  body: string
  startDate: string
  endDate: string
}

export interface Contact {
  id: string
  name: string
  email: string
  phone: string
}

export interface DomainResult {
  domain: string
  message: string
  dnsRecords: DnsRecord[]
}

export interface DnsRecord {
  type: string
  name: string
  value: string
  priority: string
}

export interface Alias {
  aliasEmail: string
  targetEmails: string[]
  isCatchAll: boolean
}

export interface MailingList {
  listAddress: string
  name: string
  description: string
  ownerEmail: string
  members: string[]
}
