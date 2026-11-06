export interface User {
  id: number
  username: string
  created_at: string
}

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface LoginResponse {
  token: string
  user: User
}

export type ChannelType = 'smtp' | 'dingtalk' | 'wecom' | 'webhook' | 'log'

export interface Channel {
  id: number
  name: string
  type: ChannelType
  enabled: boolean
  config: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface ChannelInput {
  name: string
  type: ChannelType
  enabled?: boolean
  config: Record<string, unknown>
}

export interface ChannelTestResult {
  success: boolean
  error?: string
}

export interface ChannelListResp {
  items: Channel[]
  total: number
}

// --- 提醒 ---

export type ReminderCalendar = 'solar' | 'lunar'
export type ReminderScheduleType = 'once' | 'interval' | 'cron'
export type ReminderSource = 'web' | 'api' | 'ai'
export type ContentFormat = 'text' | 'markdown' | 'html'

export function toContentFormat(v: string): ContentFormat {
  if (v === 'text' || v === 'markdown' || v === 'html') return v
  return 'text'
}

export function toChannelType(v: string): ChannelType {
  if (v === 'smtp' || v === 'dingtalk' || v === 'wecom' || v === 'webhook' || v === 'log') return v
  return 'webhook'
}

export interface Reminder {
  id: number
  title: string
  content: string
  content_format: ContentFormat
  calendar: ReminderCalendar
  schedule_type: ReminderScheduleType
  schedule_spec: Record<string, unknown>
  enabled: boolean
  source: ReminderSource
  token_id?: number
  require_confirm: boolean
  confirm_retry_interval_sec: number
  confirm_max_retries: number
  next_fire_at?: string
  last_fired_at?: string
  fire_count: number
  created_at: string
  updated_at: string
  channel_ids: number[]
}

export interface ReminderInput {
  title: string
  content: string
  content_format?: ContentFormat
  calendar: ReminderCalendar
  schedule_type: ReminderScheduleType
  schedule_spec: Record<string, unknown>
  enabled?: boolean
  source?: 'web' | 'api' | 'ai'
  channel_ids: number[]
  require_confirm: boolean
  confirm_retry_interval_sec: number
  confirm_max_retries: number
}

export interface ReminderListResp {
  items: Reminder[]
  total: number
}

// --- 日志 ---

export type LogStatus = 'pending' | 'success' | 'partial' | 'failed' | 'expired'

export interface DeliveryAttempt {
  id: number
  delivery_log_id: number
  channel_id: number
  channel_type: ChannelType
  channel_name: string
  attempt: number
  status: 'success' | 'failed'
  error?: string
  latency_ms: number
  created_at: string
}

export interface DeliveryLog {
  id: number
  reminder_id: number
  fired_at: string
  title: string
  content: string
  status: LogStatus
  confirmed: boolean
  confirmed_at?: string
  confirm_chain_id?: string
  confirm_url?: string
  retry_round: number
  source: string
  // 日志本身未走到终态的原因（如进程在投递中途退出后被兜底扫描收敛）。
  // 通道级失败原因在 attempts[].error 里，与这个字段不是一回事。
  error?: string
  created_at: string
  reminder_title: string
  attempts?: DeliveryAttempt[]
}

export interface LogListResp {
  items: DeliveryLog[]
  total: number
}

export interface LogFilter {
  status?: string
  source?: string
  search?: string
  reminder_id?: number
  since?: string
  limit?: number
  offset?: number
}

// --- 令牌 ---

export interface Token {
  id: number
  name: string
  prefix: string
  enabled: boolean
  last_used_at?: string
  created_at: string
  default_channel_ids: number[]
  usage_24h: number
}

export interface CreateTokenResult {
  plaintext: string
  key: Token
}

export interface TokenListResp {
  items: Token[]
  total: number
}

// --- 监控 ---
export interface SchedulerChannel {
  id: number
  name: string
  type: string
}

export interface SchedulerEntry {
  id: number
  kind: 'cron' | 'afterfunc'
  next_fire_at?: string
  title: string
  last_fired_at?: string
  schedule_type: string
  channels?: SchedulerChannel[]
}

export interface SchedulerStatus {
  engine: {
    running: boolean
    registered_count: number
    entries: SchedulerEntry[]
    entries_total: number
  }
  sweeper: {
    running: boolean
    interval_seconds: number
  }
}

// --- 日历组件 ---

export interface CalendarResult {
  date: string
  calendar: ReminderCalendar
  lunar?: { year: number; month: number; day: number }
  hour: number
  minute: number
}
// --- AI 助手 ---

export type AIProvider = 'anthropic' | 'openai'

export interface AISettings {
  enabled: boolean
  provider: AIProvider
  base_url: string
  model: string
  models?: string[]
  /** 已配置时后端回 "***"，否则为空串 */
  api_key: string
  /** 用户自定义的追加提示词，与内置规则拼接后发给模型 */
  custom_prompt: string
  /** 接口调用失败后的最大重试次数，0 表示不重试 */
  max_retries: number
}

export interface AISettingsInput {
  enabled: boolean
  provider: AIProvider
  base_url: string
  model: string
  models?: string[]
  api_key: string
  custom_prompt: string
  max_retries: number
}

export type AIActionKind =
  | 'create_channel'
  | 'create_reminder'
  | 'update_reminder'
  | 'delete_reminder'

export type AIActionStatus = 'pending' | 'applied' | 'rejected' | 'failed'

export interface AIAction {
  id: string
  kind: AIActionKind
  summary: string
  status: AIActionStatus
  error?: string
  result_ref?: string
  created_at: string
  updated_at: string
}

/** 日志行里保存的一条对话消息（只存在浏览器里，不入库）。 */
export interface AIChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface AIChatResult {
  reply: string
  actions: AIAction[]
  /** 本次对话因临时性失败重试的总次数 */
  retries?: number
}

/** AI 对话过程中的状态事件（SSE）。 */
export interface AIChatStatus {
  phase: 'thinking' | 'tools'
  text: string
}

/** 一次即将开始的重试。 */
export interface AIChatRetry {
  attempt: number
  max: number
  delay_ms: number
  reason: string
}

/** 对话进度：由 SSE 回调驱动，null 表示空闲。 */
export type AIChatProgress =
  | { kind: 'status'; status: AIChatStatus }
  | { kind: 'retry'; retry: AIChatRetry }

export const AI_ACTION_LABEL: Record<AIActionKind, string> = {
  create_channel: '新建通知渠道',
  create_reminder: '新建提醒',
  update_reminder: '修改提醒',
  delete_reminder: '删除提醒',
}
