// 前端使用的静态常量：项目对外地址、站点时区候选列表。
//
// 这些都是不随运行时变化的字面量；需要从后端读取的配置走 ConfigContext，
// 不要混进这里。

// 项目对外地址：开源仓库与文档站点。
// 界面上需要指回项目本身时统一引用这里，避免各处硬编码。
export const GITHUB_URL = 'https://github.com/hausen1012/reminder'
export const DOCS_URL = 'https://hausen1012.github.io/reminder/'

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
