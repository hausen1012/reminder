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
export type ReminderSource = 'web' | 'api'

export interface Reminder {
  id: number
  title: string
  content: string
  calendar: ReminderCalendar
  schedule_type: ReminderScheduleType
  schedule_spec: Record<string, unknown>
  timezone: string
  enabled: boolean
  source: ReminderSource
  api_key_id?: number
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
  calendar: ReminderCalendar
  schedule_type: ReminderScheduleType
  schedule_spec: Record<string, unknown>
  timezone?: string
  enabled?: boolean
  source?: 'web' | 'api'
  channel_ids: number[]
  require_confirm: boolean
  confirm_retry_interval_sec: number
  confirm_max_retries: number
}

export interface ReminderListResp {
  items: Reminder[]
  total: number
}

export interface ReminderPreviewInput {
  calendar: ReminderCalendar
  schedule_type: ReminderScheduleType
  schedule_spec: Record<string, unknown>
  timezone?: string
  count?: number
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
  created_at: string
  reminder_title: string
  reminder_deleted: boolean
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

// --- API Key ---

export interface APIKey {
  id: number
  name: string
  prefix: string
  enabled: boolean
  last_used_at?: string
  created_at: string
  default_channel_ids: number[]
  usage_24h: number
}

export interface APIKeyPlaintextResp {
  plaintext: string
}

export interface CreateAPIKeyResult {
  plaintext: string
  key: APIKey
}

// --- 监控 ---

export interface SchedulerEntry {
  id: number
  kind: 'cron' | 'afterfunc'
  next_fire_at?: string
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