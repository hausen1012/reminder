// 站点时区下的时间格式化。
//
// 后端所有时间字段都以 UTC 落库、以 RFC3339（带 Z）返回，而站点时区是全局设置
// （config 表 key=timezone，面板「设置」页可改）。因此前端展示时间必须显式
// 按站点时区渲染，不能依赖浏览器本地时区——否则用户在另一台设备上看到的
// 触发时间会整体偏移。
//
// 这里统一用 Intl.DateTimeFormat 的 timeZone 选项，不引入 dayjs / date-fns，
// 避免为几处格式化增加依赖。

/** 默认站点时区，与后端 timezone.Fallback 保持一致。 */
export const DEFAULT_SITE_TIMEZONE = 'Asia/Shanghai'

/** 格式化风格。 */
export type TimeStyle = 'datetime' | 'datetime-second' | 'date' | 'clock'

interface ZonedParts {
  date: Date
  year: string
  month: string
  day: string
  hour: string
  minute: string
  second: string
}

// 拆出各字段，避免依赖各浏览器对 toLocaleString 输出格式的差异。
function toZonedParts(dateStr: string, timeZone: string): ZonedParts | null {
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return null

  let formatted: Intl.DateTimeFormat
  try {
    formatted = new Intl.DateTimeFormat('en-CA', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      // h23：避免午夜被格式化成 24 点
      hourCycle: 'h23',
    })
  } catch {
    // 时区名非法时退回浏览器本地时区，至少不要让页面崩掉
    formatted = new Intl.DateTimeFormat('en-CA', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    })
  }

  const map: Record<string, string> = {}
  for (const part of formatted.formatToParts(date)) {
    map[part.type] = part.value
  }
  return {
    date,
    year: map.year,
    month: map.month,
    day: map.day,
    hour: map.hour,
    minute: map.minute,
    second: map.second,
  }
}

const EMPTY = '—'

/**
 * 按站点时区格式化时间，非法或空值返回占位符。
 *
 * datetime        → YYYY/MM/DD HH:mm
 * datetime-second → YYYY/MM/DD HH:mm:ss
 * date            → YYYY/MM/DD
 * clock           → HH:mm
 */
export function formatInZone(
  dateStr: string | undefined,
  timeZone: string = DEFAULT_SITE_TIMEZONE,
  style: TimeStyle = 'datetime',
): string {
  if (!dateStr) return EMPTY
  const p = toZonedParts(dateStr, timeZone)
  if (!p) return EMPTY

  const datePart = `${p.year}/${p.month}/${p.day}`
  switch (style) {
    case 'date':
      return datePart
    case 'clock':
      return `${p.hour}:${p.minute}`
    case 'datetime-second':
      return `${datePart} ${p.hour}:${p.minute}:${p.second}`
    default:
      return `${datePart} ${p.hour}:${p.minute}`
  }
}

/**
 * 下次触发的展示：只显示未来时间，已过期返回占位符。
 *
 * 输出 YYYY/MM/DD HH:mm:ss，与列表里的"下次触发"列对齐。
 */
export function formatNextFireInZone(
  dateStr: string | undefined,
  timeZone: string = DEFAULT_SITE_TIMEZONE,
): string {
  if (!dateStr) return EMPTY
  const p = toZonedParts(dateStr, timeZone)
  if (!p) return EMPTY
  // 用绝对时刻比较：展示时区只影响排版，不影响"是否已过"的判断
  if (p.date.getTime() <= Date.now()) return EMPTY
  return `${p.year}/${p.month}/${p.day} ${p.hour}:${p.minute}:${p.second}`
}

/** 站点时区下的一组"墙上时间"数字。 */
export interface ZonedWallClock {
  year: number
  month: number // 1-12
  day: number
  hour: number
  minute: number
}

/**
 * 取站点时区下的当前墙上时间，可用于给表单填默认值。
 *
 * offsetMinutes 为正表示往后偏移（如 +5 分钟）。注意必须"先把偏移加到时刻上、
 * 再按时区格式化"，而不是直接给墙上时间加分钟——跨夏令时切换时后者会算错。
 */
export function nowInZone(
  timeZone: string = DEFAULT_SITE_TIMEZONE,
  offsetMinutes = 0,
): ZonedWallClock {
  const instant = new Date(Date.now() + offsetMinutes * 60_000)
  const p = toZonedParts(instant.toISOString(), timeZone)
  if (!p) {
    // toZonedParts 只在日期非法时返回 null，instant 一定合法，兜底取本地时间
    const d = new Date()
    return {
      year: d.getFullYear(),
      month: d.getMonth() + 1,
      day: d.getDate(),
      hour: d.getHours(),
      minute: d.getMinutes(),
    }
  }
  return {
    year: Number(p.year),
    month: Number(p.month),
    day: Number(p.day),
    hour: Number(p.hour),
    minute: Number(p.minute),
  }
}

/**
 * 把站点时区下的墙上时间拼成后端要求的本地时间字符串。
 *
 * withSeconds 为 true 时输出 YYYY-MM-DDTHH:mm:ss，否则 YYYY-MM-DDTHH:mm。
 */
export function toWallClockString(w: ZonedWallClock, withSeconds = false): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const base = `${w.year}-${pad(w.month)}-${pad(w.day)}T${pad(w.hour)}:${pad(w.minute)}`
  return withSeconds ? `${base}:00` : base
}
