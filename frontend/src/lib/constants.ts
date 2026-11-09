// 前端使用的静态常量：项目对外地址、站点时区候选列表。
//
// 这些都是不随运行时变化的字面量；需要从后端读取的配置走 ConfigContext，
// 不要混进这里。

// 项目对外地址：开源仓库与文档站点。
// 界面上需要指回项目本身时统一引用这里，避免各处硬编码。
export const GITHUB_URL = 'https://github.com/hausen1012/reminder'
export const DOCS_URL = 'https://hausen1012.github.io/reminder/'
// 版本列表页面：关于页的「全部版本」入口。
export const RELEASES_URL = 'https://github.com/hausen1012/reminder/releases'

// 站点时区的常用候选列表。
//
// 时区是全局设置（设置页），提醒本身不再单独保存时区，
// 因此这份列表只给设置页的下拉使用。
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

// 提醒来源的展示文案与徽章配色。
//
// 来源取值由后端写入：web（面板表单）、ai（AI 助手确认后创建）、api（外部令牌）。
// 各处列表、详情、筛选统一引用这里，避免出现「api 显示成 Web」这类漏判。
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
