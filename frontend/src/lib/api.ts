import axios from 'axios'
import type {
  APIKey,
  ApiResponse,
  Channel,
  ChannelInput,
  ChannelTestResult,
  ChannelType,
  CreateAPIKeyResult,
  DeliveryLog,
  LogFilter,
  LogListResp,
  LoginResponse,
  Reminder,
  ReminderInput,
  ReminderListResp,
  ReminderPreviewInput,
  User,
} from '@/types'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401 && error.config?.url !== '/auth/login') {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  },
)

export async function login(username: string, password: string) {
  const res = await api.post<ApiResponse<LoginResponse>>('/auth/login', { username, password })
  return res.data
}

export async function getMe() {
  const res = await api.get<ApiResponse<User>>('/auth/me')
  return res.data
}

export async function updatePassword(oldPassword: string, newPassword: string) {
  const res = await api.put<ApiResponse<null>>('/auth/password', { old_password: oldPassword, new_password: newPassword })
  return res.data
}

export async function logout() {
  const res = await api.post<ApiResponse<null>>('/auth/logout')
  return res.data
}

// --- 通道 ---

export async function listChannels() {
  const res = await api.get<ApiResponse<Channel[]>>('/channels')
  return res.data.data ?? []
}

export async function getChannel(id: number) {
  const res = await api.get<ApiResponse<Channel>>(`/channels/${id}`)
  return res.data.data
}

export async function createChannel(input: ChannelInput) {
  const res = await api.post<ApiResponse<Channel>>('/channels', input)
  return res.data.data
}

export async function updateChannel(id: number, input: ChannelInput) {
  const res = await api.put<ApiResponse<Channel>>(`/channels/${id}`, input)
  return res.data.data
}

export async function deleteChannel(id: number) {
  await api.delete<ApiResponse<null>>(`/channels/${id}`)
}

export async function toggleChannel(id: number) {
  const res = await api.patch<ApiResponse<Channel>>(`/channels/${id}/toggle`)
  return res.data.data
}

export async function testChannel(id: number, body?: { subject?: string; body?: string }) {
  const res = await api.post<ApiResponse<ChannelTestResult>>(`/channels/${id}/test`, body ?? {})
  return res.data.data
}

// --- 提醒 ---

export interface ListRemindersQuery {
  source?: 'manual' | 'api' | 'all'
  enabled?: boolean
  search?: string
  limit?: number
  offset?: number
}

export async function listReminders(q: ListRemindersQuery = {}) {
  const res = await api.get<ApiResponse<ReminderListResp>>('/reminders', { params: q })
  return res.data.data
}

export async function getReminder(id: number) {
  const res = await api.get<ApiResponse<Reminder>>(`/reminders/${id}`)
  return res.data.data
}

export async function createReminder(input: ReminderInput) {
  const res = await api.post<ApiResponse<Reminder>>('/reminders', input)
  return res.data.data
}

export async function updateReminder(id: number, input: ReminderInput) {
  const res = await api.put<ApiResponse<Reminder>>(`/reminders/${id}`, input)
  return res.data.data
}

export async function deleteReminder(id: number) {
  await api.delete<ApiResponse<null>>(`/reminders/${id}`)
}

export async function toggleReminder(id: number) {
  const res = await api.patch<ApiResponse<Reminder>>(`/reminders/${id}/toggle`)
  return res.data.data
}

export async function previewReminder(input: ReminderPreviewInput) {
  const res = await api.post<ApiResponse<{ times: string[] }>>('/reminders/preview', input)
  return res.data.data?.times ?? []
}

export async function testReminder(id: number) {
  const res = await api.post<ApiResponse<{ delivery_log_id: number }>>(`/reminders/${id}/test`)
  return res.data.data
}

// --- 日志 ---

export async function listLogs(f: LogFilter = {}) {
  const res = await api.get<ApiResponse<LogListResp>>('/logs', { params: f })
  return res.data.data
}

export async function getLogDetail(id: number) {
  const res = await api.get<ApiResponse<DeliveryLog>>(`/logs/${id}`)
  return res.data.data
}

export async function purgeLogs(olderThan?: string, all?: boolean) {
  const res = await api.delete<ApiResponse<{ deleted: number }>>('/logs', { params: { older_than: olderThan, all } })
  return res.data.data
}

export async function countPurgeLogs(olderThan?: string, all?: boolean) {
  const res = await api.get<ApiResponse<{ count: number }>>('/logs/count', { params: { older_than: olderThan, all } })
  return res.data.data
}

// --- API Key ---

export async function listApiKeys() {
  const res = await api.get<ApiResponse<APIKey[]>>('/apikeys')
  return res.data.data ?? []
}

export async function createApiKey(name: string, defaultChannelIDs?: number[]) {
  const res = await api.post<ApiResponse<CreateAPIKeyResult>>('/apikeys', { name, default_channel_ids: defaultChannelIDs ?? [] })
  return res.data.data
}

export async function toggleApiKey(id: number) {
  const res = await api.patch<ApiResponse<APIKey>>(`/apikeys/${id}/toggle`)
  return res.data.data
}

export async function deleteApiKey(id: number) {
  await api.delete<ApiResponse<null>>(`/apikeys/${id}`)
}

export async function updateApiKeyChannels(id: number, channelIDs: number[]) {
  await api.put<ApiResponse<null>>(`/apikeys/${id}/channels`, { channel_ids: channelIDs })
}

// --- Dashboard ---

export async function listUpcomingReminders(within = '24h', limit = 10) {
  const res = await api.get<ApiResponse<Reminder[]>>('/reminders/upcoming', { params: { within, limit } })
  return res.data.data ?? []
}

export interface ChannelStats {
  id: number
  name: string
  type: ChannelType
  total: number
  success: number
  failed: number
  success_rate: number
}

export async function listChannelStats(window = '24h') {
  const res = await api.get<ApiResponse<ChannelStats[]>>('/channels/stats', { params: { window } })
  return res.data.data ?? []
}

export async function listApiKeyStats() {
  const res = await api.get<ApiResponse<{ id: number; name: string; usage_24h: number }[]>>('/apikeys/stats')
  return res.data.data ?? []
}