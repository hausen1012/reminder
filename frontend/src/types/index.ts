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

export type ChannelType = 'smtp' | 'dingtalk' | 'wecom' | 'webhook'

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

// --- 提醒 ---

export type ReminderCalendar = 'solar' | 'lunar'
export type ReminderScheduleType = 'once' | 'interval' | 'cron'
export type ReminderSource = 'manual' | 'api'

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