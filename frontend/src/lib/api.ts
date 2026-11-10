import axios from 'axios'
import type {
  Token,
  AboutInfo,
  ApiResponse,
  AIAction,
  AIChatMessage,
  AIChatProgress,
  AIChatResult,
  AIChatRetry,
  AIChatStatus,
  AISettings,
  AISettingsInput,
  Channel,
  ChannelInput,
  ChannelListResp,
  ChannelType,
  ChannelTypeSpec,
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
 * 后端错误统一为 { "error": { "code", "message", "field"? } }，取 `data.error.message`。
 */
export function extractApiError(err: unknown, fallback = '操作失败'): string {
  const data: unknown = (err as { response?: { data?: unknown } })?.response?.data
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>

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

export async function listChannelTypes() {
  const res = await api.get<ApiResponse<ChannelTypeSpec[]>>('/channels/types')
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

// --- 关于 ---

export async function getAboutInfo() {
  const res = await api.get<ApiResponse<AboutInfo>>('/about')
  return res.data.data
}

/** 立即检查一次更新，等待服务端请求 GitHub 后返回最新状态。 */
export async function checkUpdate() {
  const res = await api.post<ApiResponse<AboutInfo>>('/about/check')
  return res.data.data
}

export async function setUpdateCheckEnabled(enabled: boolean) {
  const res = await api.put<ApiResponse<AboutInfo>>('/about/update-check', { enabled })
  return res.data.data
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

export async function fetchAIModels(input: AISettingsInput) {
  const res = await api.post<ApiResponse<{ items: string[] }>>('/ai/models', input)
  return res.data.data?.items ?? []
}

/**
 * 发一轮对话。
 *
 * 接口是无状态的：历史由前端持有并随请求上传，服务端不保存聊天记录。
 * 超时放宽到 120 秒——工具循环可能持续数十秒。
 */
export async function sendAIChat(
  message: string,
  history: AIChatMessage[],
  model?: string,
) {
  const payload: { message: string; history: AIChatMessage[]; model?: string } = {
    message,
    history,
  }
  if (model) {
    payload.model = model
  }
  const res = await api.post<ApiResponse<AIChatResult>>(
    '/ai/chat',
    payload,
    { timeout: 120000 },
  )
  return res.data.data
}

/**
 * 发一轮对话，并接收服务端推送的过程状态。
 *
 * 用 SSE 而不是完整 token 流：本系统的助手要跑多轮工具调用，逐字打字机的
 * 改造成本高价值低；真正让用户干等的是「重试」与「工具执行」这两个阶段。
 * 所以服务端只推 status / retry / done / error 四类事件，正文仍一次性返回。
 *
 * 这里用 fetch 而不是 axios：axios 在浏览器里拿不到增量响应体。
 * 事件格式与后端 handlers.AIHandler.ChatStream 的注释一一对应。
 */
export async function sendAIChatStream(
  message: string,
  history: AIChatMessage[],
  model: string | undefined,
  onProgress?: (progress: AIChatProgress) => void,
  signal?: AbortSignal,
): Promise<AIChatResult> {
  const payload: { message: string; history: AIChatMessage[]; model?: string } = {
    message,
    history,
  }
  if (model) {
    payload.model = model
  }

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  }
  const token = localStorage.getItem('token')
  if (token) headers.Authorization = `Bearer ${token}`

  const res = await fetch('/api/ai/chat/stream', {
    method: 'POST',
    headers,
    body: JSON.stringify(payload),
    signal,
  })

  if (res.status === 401) {
    localStorage.removeItem('token')
    window.location.href = '/#/login'
  }

  // 请求体解析失败等「还没开始推流就出错」的情况仍是普通 JSON 错误体
  if (!res.ok || !res.headers.get('content-type')?.includes('text/event-stream')) {
    const text = await res.text()
    let body: unknown = text
    try {
      body = JSON.parse(text)
    } catch {
      // 非 JSON 就原样保留，extractApiError 会退化成 text
    }
    throw { response: { data: body, status: res.status } }
  }

  if (!res.body) {
    throw new Error('浏览器不支持流式响应')
  }

  let result: AIChatResult | null = null
  let streamErr: unknown = null
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''

  const handleEvent = (event: string, data: string) => {
    let parsed: unknown
    try {
      parsed = JSON.parse(data)
    } catch {
      return
    }
    switch (event) {
      case 'status':
        onProgress?.({ kind: 'status', status: parsed as AIChatStatus })
        break
      case 'retry':
        onProgress?.({ kind: 'retry', retry: parsed as AIChatRetry })
        break
      case 'done':
        result = parsed as AIChatResult
        break
      case 'error':
        streamErr = { response: { data: { error: parsed } } }
        break
    }
  }

  // SSE 以空行分隔事件，data 可能跨多行（这里服务端只发单行 JSON）。
  // 统一换行符：避免反向代理或网关把换行规范化为 CRLF（\r\n\r\n）导致分块失败。
  const drain = (flush: boolean) => {
    buf = buf.replace(/\r\n/g, '\n')
    const blocks = buf.split('\n\n')
    buf = flush ? '' : (blocks.pop() ?? '')
    for (const block of blocks) {
      let event = 'message'
      const dataLines: string[] = []
      for (const line of block.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim()
        else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
      }
      if (dataLines.length > 0) handleEvent(event, dataLines.join('\n'))
    }
  }

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    drain(false)
  }
  buf += decoder.decode()
  drain(true)

  if (streamErr) throw streamErr
  if (!result) throw new Error('对话未正常结束，请重试')
  return result
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
