import { Solar, Lunar } from 'lunar-typescript'
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { Reminder, ReminderCalendar } from '@/types'

const CHINESE_DIGITS = ['零', '一', '二', '三', '四', '五', '六', '七', '八', '九']

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatChineseLunarYear(year: number): string {
  return String(year).split('').map((digit) => CHINESE_DIGITS[+digit]).join('')
}

export function formatChineseLunarMonth(month: number): string {
  if (month <= 10) return CHINESE_DIGITS[month] + '月'
  if (month === 11) return '十一月'
  return '十二月'
}

export function formatChineseLunarDay(day: number): string {
  if (day <= 0 || day > 30) return ''
  if (day <= 10) return day === 10 ? '初十' : `初${CHINESE_DIGITS[day]}`
  if (day < 20) return '十' + CHINESE_DIGITS[day - 10]
  if (day === 20) return '二十'
  if (day < 30) return '廿' + CHINESE_DIGITS[day - 20]
  return '三十'
}

export function getLunarYmd(dateStr: string): { year: number; month: number; day: number } | null {
  if (!dateStr) return null
  const values = dateStr.slice(0, 10).split('-')
  if (values.length !== 3) return null
  const solar = Solar.fromYmd(Number(values[0]), Number(values[1]), Number(values[2]))
  const lunar = solar.getLunar()
  return { year: lunar.getYear(), month: lunar.getMonth(), day: lunar.getDay() }
}

export function getReminderSpecHour(spec: Record<string, unknown>): number | undefined {
  if (typeof spec.hour === 'number') return spec.hour as number
  const at = (spec.at ?? spec.start_at) as string | undefined
  if (!at || at.length < 16) return undefined
  const hour = Number(at.slice(11, 13))
  return Number.isFinite(hour) ? hour : undefined
}

export function getReminderSpecMinute(spec: Record<string, unknown>): number | undefined {
  if (typeof spec.minute === 'number') return spec.minute as number
  const at = (spec.at ?? spec.start_at) as string | undefined
  if (!at || at.length < 16) return undefined
  const minute = Number(at.slice(14, 16))
  return Number.isFinite(minute) ? minute : undefined
}

export function formatReminderSolarLine(spec: Record<string, unknown>, calendar: ReminderCalendar): string {
  const at = (spec.at ?? spec.start_at) as string | undefined
  if (at) return `${at.slice(0, 10)} ${at.slice(11, 16)}:00`
  if (calendar === 'lunar') {
    const lunar = (spec.lunar ?? spec.start_lunar) as { year: number; month: number; day: number } | undefined
    if (lunar) {
      const solar = Lunar.fromYmd(lunar.year, lunar.month, lunar.day).getSolar()
      return `${solar.getYear()}-${String(solar.getMonth()).padStart(2, '0')}-${String(solar.getDay()).padStart(2, '0')} ${String(spec.hour ?? 9).padStart(2, '0')}:${String(spec.minute ?? 0).padStart(2, '0')}:00`
    }
  }
  return '选择日期'
}

export function formatReminderLunarLine(spec: Record<string, unknown>, calendar: ReminderCalendar): string {
  let ymd: { year: number; month: number; day: number } | null = null
  if (calendar === 'lunar') {
    ymd = (spec.lunar ?? spec.start_lunar) as { year: number; month: number; day: number } | undefined ?? null
  } else {
    const at = (spec.at ?? spec.start_at) as string | undefined
    if (at) ymd = getLunarYmd(at)
  }
  if (!ymd) return ''
  return `${formatChineseLunarYear(ymd.year)}年${formatChineseLunarMonth(ymd.month)}${formatChineseLunarDay(ymd.day)}`
}

export function formatReminderDetail(reminder: Reminder): string {
  const spec = reminder.schedule_spec ?? {}
  if (reminder.schedule_type === 'cron') {
    return (spec.expr as string) ?? ''
  }

  const solarLine = formatReminderSolarLine(spec, reminder.calendar)
  const lunarLine = formatReminderLunarLine(spec, reminder.calendar)

  if (reminder.calendar === 'lunar') {
    return lunarLine
      ? `${lunarLine} ${String(spec.hour ?? 9).padStart(2, '0')}:${String(spec.minute ?? 0).padStart(2, '0')}:00`
      : '—'
  }

  return solarLine === '选择日期' ? '公历' : solarLine.replace(/-/g, '/')
}

/** YYYY/MM/DD HH:mm:ss 格式，且只显示未来时间 */
export function formatNextFire(dateStr?: string): string {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  if (isNaN(d.getTime()) || d.getTime() <= Date.now()) return '—'
  const y = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  return `${y}/${month}/${day} ${h}:${min}:${s}`
}

/** YYYY/MM/DD HH:mm 格式 */
export function formatTime(dateStr?: string): string {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return '—'
  const y = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${y}/${month}/${day} ${h}:${min}`
}
