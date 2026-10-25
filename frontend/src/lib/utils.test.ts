import { describe, expect, it } from 'vitest'
import {
  formatChineseLunarDay,
  formatChineseLunarMonth,
  formatChineseLunarYear,
  formatNextFire,
  formatTime,
  getLunarYmd,
  getReminderSpecHour,
  getReminderSpecMinute,
} from '@/lib/utils'

describe('农历中文格式化', () => {
  it('年份逐位转中文', () => {
    expect(formatChineseLunarYear(2026)).toBe('二零二六')
  })

  it('月份 1-12 与边界', () => {
    expect(formatChineseLunarMonth(1)).toBe('一月')
    expect(formatChineseLunarMonth(9)).toBe('九月')
    expect(formatChineseLunarMonth(10)).toBe('十月')
    expect(formatChineseLunarMonth(11)).toBe('十一月')
    expect(formatChineseLunarMonth(12)).toBe('十二月')
  })

  it('日期 1-30 全覆盖且不抛错', () => {
    expect(formatChineseLunarDay(1)).toBe('初一')
    expect(formatChineseLunarDay(10)).toBe('初十')
    expect(formatChineseLunarDay(11)).toBe('十一')
    expect(formatChineseLunarDay(20)).toBe('二十')
    expect(formatChineseLunarDay(21)).toBe('廿一')
    expect(formatChineseLunarDay(30)).toBe('三十')
  })

  it('非法日期返回空串', () => {
    expect(formatChineseLunarDay(0)).toBe('')
    expect(formatChineseLunarDay(31)).toBe('')
    expect(formatChineseLunarDay(Number.NaN)).toBe('')
  })
})

describe('公历转农历', () => {
  it('合法日期返回结构', () => {
    const r = getLunarYmd('2026-02-17')
    expect(r).not.toBeNull()
    expect(r!.year).toBeGreaterThan(2000)
    expect(r!.month).toBeGreaterThanOrEqual(1)
    expect(r!.day).toBeGreaterThanOrEqual(1)
  })

  it('空串返回 null', () => {
    expect(getLunarYmd('')).toBeNull()
  })
})

describe('从 schedule_spec 提取时分', () => {
  it('优先使用显式 hour/minute', () => {
    expect(getReminderSpecHour({ hour: 8 })).toBe(8)
    expect(getReminderSpecMinute({ minute: 30 })).toBe(30)
  })

  it('回退到 at 字符串解析', () => {
    expect(getReminderSpecHour({ at: '2026-06-06T09:15:00' })).toBe(9)
    expect(getReminderSpecMinute({ at: '2026-06-06T09:15:00' })).toBe(15)
  })

  it('无有效信息返回 undefined', () => {
    expect(getReminderSpecHour({})).toBeUndefined()
    expect(getReminderSpecMinute({ at: 'bad' })).toBeUndefined()
  })
})

describe('时间格式化', () => {
  it('空值显示占位符', () => {
    expect(formatTime()).toBe('—')
    expect(formatNextFire()).toBe('—')
  })

  it('非法时间显示占位符', () => {
    expect(formatTime('not-a-date')).toBe('—')
  })

  it('formatNextFire 对过去时间返回占位符', () => {
    expect(formatNextFire('2000-01-01T00:00:00Z')).toBe('—')
  })

  it('formatNextFire 对将来时间输出 YYYY/MM/DD HH:mm:ss', () => {
    const future = new Date(Date.now() + 86_400_000).toISOString()
    expect(formatNextFire(future)).toMatch(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}$/)
  })
})
