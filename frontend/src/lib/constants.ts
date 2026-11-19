// 静态常量定义。

// 项目仓库与文档地址。
export const GITHUB_URL = 'https://github.com/hausen1012/reminder'
export const DOCS_URL = 'https://hausen1012.github.io/reminder/'
// 版本列表页面：关于页的「全部版本」入口。
export const RELEASES_URL = 'https://github.com/hausen1012/reminder/releases'

// 站点时区常用候选列表（供设置页使用）。
export const COMMON_TIMEZONES = [
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Hong_Kong',
  'Asia/Singapore',
  'Asia/Seoul',
  'Asia/Taipei',
  'Asia/Kolkata',
  'Asia/Dubai',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Moscow',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Australia/Sydney',
  'Pacific/Auckland',
  'UTC',
]

// 提醒来源展示文案与徽章配色（web / ai / api）。
export const SOURCE_LABEL: Record<string, string> = {
  web: 'Web',
  ai: 'AI',
  api: 'API',
}

// 徽章配色：Web 中性、AI 强调、API 次强调，便于快速扫读。
export const SOURCE_BADGE_VARIANT: Record<string, 'muted' | 'violet' | 'info'> = {
  web: 'muted',
  ai: 'violet',
  api: 'info',
}

/** 取来源展示文案，未知来源原样返回。 */
export function sourceLabel(source: string | undefined): string {
  if (!source) return '—'
  return SOURCE_LABEL[source] ?? source
}

// 投递日志状态展示文案与配色（success / partial / failed / pending / expired）。
export const LOG_STATUS_LABEL: Record<string, string> = {
  success: '成功',
  partial: '部分成功',
  failed: '失败',
  pending: '发送中',
  expired: '已过期',
}

// 投递日志状态的徽章样式：状态语义直接对应 Badge 变体，避免各页各写一份色值。
const LOG_STATUS_VARIANT: Record<string, 'success' | 'warning' | 'danger' | 'info' | 'muted'> = {
  success: 'success',
  partial: 'warning',
  failed: 'danger',
  pending: 'info',
  expired: 'muted',
}

/** 取日志状态对应的 Badge 变体，未知状态按中性色处理。 */
export function logStatusVariant(status: string): 'success' | 'warning' | 'danger' | 'info' | 'muted' {
  return LOG_STATUS_VARIANT[status] ?? 'muted'
}

/** 投递耗时的文字色：100ms 内正常，500ms 内偏慢，再长为异常。 */
export function latencyClass(ms: number): string {
  if (ms < 100) return 'text-emerald-600 dark:text-emerald-400'
  if (ms < 500) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

// 提醒调度类型展示文案。
export const SCHEDULE_TYPE_LABEL: Record<string, string> = {
  once: '单次',
  interval: '周期',
  cron: 'Cron',
}

/** 取状态展示文案，未知状态原样返回。 */
export function logStatusLabel(status: string): string {
  return LOG_STATUS_LABEL[status] ?? status
}
