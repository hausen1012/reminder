import axios from 'axios'
import type {
  APIKey,
  ApiResponse,
  Channel,
  ChannelInput,
  ChannelListResp,
  ChannelType,
  CreateAPIKeyResult,
  DeliveryLog,
  LogFilter,
  LogListResp,
  LoginResponse,
  Reminder,
  ReminderInput,
  ReminderListResp,
  SchedulerStatus,
  User,
  ApiKeyListResp,
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
      window.location.href = '/#/login'
    }
    return Promise.reject(error)
  },
)

/** 从 axios error 中提取服务端返回的消息，路径兼容 data.message 和 data.error.message */
export function extractApiError(err: unknown, fallback = '操作失败'): string {
  const data: unknown = (err as { response?: { data?: unknown } })?.response?.data
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (typeof obj.message === 'string') return obj.message
    if (obj.error && typeof obj.error === 'object') {
      const errObj = obj.error as Record<string, unknown>
      if (typeof errObj.message === 'string') return errObj.message
    }
  }
  if (err instanceof Error) return err.message
  return fallback
}

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

// --- 通知 ---

export interface ListChannelsQuery {
  enabled?: boolean
  search?: string
  limit?: number
  offset?: number
  sort_by?: string
  sort_order?: string
}

export async function listChannels() {
  const res = await api.get<ApiResponse<Channel[]>>('/channels')
  return res.data.data ?? []
}

export async function listChannelsPaged(q: ListChannelsQuery = {}) {
  const res = await api.get<ApiResponse<ChannelListResp>>('/channels', { params: q })
  return res.data.data
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

export async function testChannelDryRun(input: {
  id?: number
  type: ChannelType
  config: Record<string, unknown>
}) {
  const res = await api.post<ApiResponse<{ success: boolean }>>('/channels/test-dry', input)
  return res.data.data
}

// --- 提醒 ---

export interface ListRemindersQuery {
  source?: 'web' | 'api' | 'all'
  enabled?: boolean
  search?: string
  limit?: number
  offset?: number
  sort_by?: string
  sort_order?: string
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

export async function testReminderDryRun(input: {
  id?: number
  title: string
  content: string
  content_format: string
  channel_ids: number[]
}) {
  const res = await api.post<ApiResponse<{ success: boolean }>>('/reminders/test-dry', input)
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

export async function listApiKeys(params?: { limit?: number; offset?: number; search?: string }) {
  const res = await api.get<ApiResponse<ApiKeyListResp>>('/apikeys', { params })
  return res.data.data ?? { items: [], total: 0 }
}

export async function getApiKey(id: number) {
  const res = await api.get<ApiResponse<APIKey>>(`/apikeys/${id}`)
  return res.data.data
}

export async function getApiKeyPlaintext(id: number) {
  const res = await api.get<ApiResponse<{ plaintext: string }>>(`/apikeys/${id}/plaintext`)
  return res.data.data?.plaintext ?? ''
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

// --- 监控 ---

export async function getSchedulerStatus(params?: { limit?: number; offset?: number }) {
  const res = await api.get<ApiResponse<SchedulerStatus>>('/scheduler/status', { params })
  return res.data.data
}

// --- Dashboard ---

export async function listTodayReminders() {
  const res = await api.get<ApiResponse<Reminder[]>>('/reminders/upcoming', { params: { scope: 'today' } })
  return res.data.data ?? []
}

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

// --- 站点配置 ---

export async function getConfig() {
  const res = await api.get<ApiResponse<Record<string, string>>>('/config')
  return res.data.data ?? {}
}

export async function updateConfig(cfg: Record<string, string>) {
  const res = await api.put<ApiResponse<Record<string, string>>>('/config', cfg)
  return res.data.data ?? {}
}