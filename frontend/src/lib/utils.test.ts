import { describe, expect, it, vi, afterEach } from 'vitest'
import {
  copyToClipboard,
  formatChineseLunarDay,
  formatChineseLunarMonth,
  formatChineseLunarYear,
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

describe('copyToClipboard 安全复制', () => {
  const originalClipboard = navigator.clipboard
  const originalExecCommand = document.execCommand

  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', {
      value: originalClipboard,
      configurable: true,
      writable: true,
    })
    document.execCommand = originalExecCommand
  })

  it('优先使用 navigator.clipboard.writeText', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
      writable: true,
    })

    await expect(copyToClipboard('hello')).resolves.toBeUndefined()
    expect(writeText).toHaveBeenCalledWith('hello')
  })

  it('当 navigator.clipboard 不存在时自动降级到 execCommand', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: undefined,
      configurable: true,
      writable: true,
    })
    const execCommand = vi.fn().mockReturnValue(true)
    document.execCommand = execCommand

    await expect(copyToClipboard('fallback-test')).resolves.toBeUndefined()
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('当两者均不可用或失败时抛出友好异常', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: undefined,
      configurable: true,
      writable: true,
    })
    document.execCommand = vi.fn().mockReturnValue(false)

    await expect(copyToClipboard('fail-test')).rejects.toThrow('当前环境限制自动复制，请手动选中文本复制')
  })
})

