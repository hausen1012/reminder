import axios from 'axios'
import type {
  Token,
  ApiResponse,
  AIAction,
  AIChatMessage,
  AIChatResult,
  AISettings,
  AISettingsInput,
  Channel,
  ChannelInput,
  ChannelListResp,
  ChannelType,
  CreateTokenResult,
  DeliveryLog,
  LogFilter,
  LogListResp,
  LoginResponse,
  Reminder,
  ReminderInput,
  ReminderListResp,
  SchedulerStatus,
  User,
  TokenListResp,
} from '@/types'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
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

/**
 * 从 axios error 中提取服务端返回的提示文案。
 *
 * 后端所有错误出口（业务 handler / 鉴权中间件 / panic 恢复）都经由
 * `middleware.ErrorBody` 渲染，统一格式为：
 *
 *   { "error": { "code": "not_found", "message": "提醒不存在", "field": "title" } }
 *
 * 正常路径读取 `data.error.message`。
 */
export function extractApiError(err: unknown, fallback = '操作失败'): string {
  const data: unknown = (err as { response?: { data?: unknown } })?.response?.data
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>

    // 统一格式：{ error: { code, message, field? } }
    if (obj.error && typeof obj.error === 'object') {
      const errObj = obj.error as Record<string, unknown>
      if (typeof errObj.message === 'string' && errObj.message) return errObj.message
    }
  }
  if (err instanceof Error) return err.message
  return fallback
}

/** 从 axios error 中取出后端错误码（如 not_found / validation_failed）。 */
export function extractApiErrorCode(err: unknown): string {
  const data: unknown = (err as { response?: { data?: unknown } })?.response?.data
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (obj.error && typeof obj.error === 'object') {
      const code = (obj.error as Record<string, unknown>).code
      if (typeof code === 'string') return code
    }
  }
  return ''
}

/** 从 axios error 中取出出错字段名（用于表单定位），无则为空串。 */
export function extractApiErrorField(err: unknown): string {
  const data: unknown = (err as { response?: { data?: unknown } })?.response?.data
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (obj.error && typeof obj.error === 'object') {
      const field = (obj.error as Record<string, unknown>).field
      if (typeof field === 'string') return field
    }
  }
  return ''
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
  source?: 'web' | 'api' | 'ai' | 'all'
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

// --- 令牌 ---

export async function listTokens(params?: { limit?: number; offset?: number; search?: string }) {
  const res = await api.get<ApiResponse<TokenListResp>>('/tokens', { params })
  return res.data.data ?? { items: [], total: 0 }
}

export async function getToken(id: number) {
  const res = await api.get<ApiResponse<Token>>(`/tokens/${id}`)
  return res.data.data
}

export async function getTokenPlaintext(id: number) {
  const res = await api.get<ApiResponse<{ plaintext: string }>>(`/tokens/${id}/plaintext`)
  return res.data.data?.plaintext ?? ''
}

export async function createToken(name: string, defaultChannelIDs?: number[]) {
  const res = await api.post<ApiResponse<CreateTokenResult>>('/tokens', { name, default_channel_ids: defaultChannelIDs ?? [] })
  return res.data.data
}

export async function toggleToken(id: number) {
  const res = await api.patch<ApiResponse<Token>>(`/tokens/${id}/toggle`)
  return res.data.data
}

export async function deleteToken(id: number) {
  await api.delete<ApiResponse<null>>(`/tokens/${id}`)
}

export async function updateTokenChannels(id: number, channelIDs: number[]) {
  await api.put<ApiResponse<null>>(`/tokens/${id}/channels`, { channel_ids: channelIDs })
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

export async function listTokenStats() {
  const res = await api.get<ApiResponse<{ id: number; name: string; usage_24h: number }[]>>('/tokens/stats')
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

// --- 批量删除 ---

export async function batchDeleteReminders(ids: number[]) {
  await api.delete<ApiResponse<null>>('/reminders/batch', { data: { ids } })
}

export async function batchDeleteChannels(ids: number[]) {
  await api.delete<ApiResponse<null>>('/channels/batch', { data: { ids } })
}

export async function batchDeleteLogs(ids: number[]) {
  await api.delete<ApiResponse<null>>('/logs/batch', { data: { ids } })
}
// --- AI 助手 ---

export async function getAISettings() {
  const res = await api.get<ApiResponse<AISettings>>('/ai/settings')
  return res.data.data
}

export async function updateAISettings(input: AISettingsInput) {
  const res = await api.put<ApiResponse<AISettings>>('/ai/settings', input)
  return res.data.data
}

export async function testAIConnection(input: AISettingsInput) {
  const res = await api.post<ApiResponse<{ reply: string }>>('/ai/test', input)
  return res.data.data
}

/**
 * 发一轮对话。
 *
 * 接口是无状态的：历史由前端持有并随请求上传，服务端不保存聊天记录。
 * 超时放宽到 120 秒——工具循环可能持续数十秒。
 */
export async function sendAIChat(message: string, history: AIChatMessage[]) {
  const res = await api.post<ApiResponse<AIChatResult>>(
    '/ai/chat',
    { message, history },
    { timeout: 120000 },
  )
  return res.data.data
}

/** 拉取待确认草稿（刷新页面后恢复用）。 */
export async function listAIActions(status: 'pending' | 'all' = 'pending') {
  const res = await api.get<ApiResponse<{ items: AIAction[] }>>('/ai/actions', { params: { status } })
  return res.data.data?.items ?? []
}

export async function applyAIAction(id: string) {
  const res = await api.post<ApiResponse<AIAction>>(`/ai/actions/${id}/apply`)
  return res.data.data
}

export async function rejectAIAction(id: string) {
  const res = await api.post<ApiResponse<AIAction>>(`/ai/actions/${id}/reject`)
  return res.data.data
}
