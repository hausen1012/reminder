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
export const SOURCE_BADGE_CLASS: Record<string, string> = {
  web: 'bg-muted text-muted-foreground',
  ai: 'bg-violet-500/15 text-violet-700 dark:text-violet-300',
  api: 'bg-sky-500/15 text-sky-700 dark:text-sky-300',
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

// 状态胶囊底色，用于首页等紧凑列表。
export const LOG_STATUS_PILL_CLASS: Record<string, string> = {
  success: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
  partial: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
  failed: 'bg-red-500/15 text-red-700 dark:text-red-300',
  pending: 'bg-sky-500/15 text-sky-700 dark:text-sky-300',
  expired: 'bg-muted text-muted-foreground',
}

// 状态文字与图标色，用于日志页表格。
export const LOG_STATUS_TEXT_CLASS: Record<string, string> = {
  success: 'text-emerald-600 dark:text-emerald-400',
  partial: 'text-amber-600 dark:text-amber-400',
  failed: 'text-red-600 dark:text-red-400',
  pending: 'text-sky-600 dark:text-sky-400',
  expired: 'text-muted-foreground',
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
