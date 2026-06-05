import axios from 'axios'
import type {
  ApiResponse,
  Channel,
  ChannelInput,
  ChannelTestResult,
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