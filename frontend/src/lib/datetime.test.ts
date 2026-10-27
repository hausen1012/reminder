import { describe, expect, it } from 'vitest'
import { formatInZone, formatNextFireInZone } from '@/lib/datetime'

// 2026-07-01T01:00:00Z 在 Asia/Shanghai 是 09:00，在 UTC 是 01:00。
// 用它来验证"展示按站点时区而非浏览器时区"这一核心约定。
const SAMPLE = '2026-07-01T01:00:00Z'

describe('formatInZone', () => {
  it('按站点时区渲染，不依赖浏览器时区', () => {
    expect(formatInZone(SAMPLE, 'Asia/Shanghai')).toBe('2026/07/01 09:00')
    expect(formatInZone(SAMPLE, 'UTC')).toBe('2026/07/01 01:00')
    expect(formatInZone(SAMPLE, 'America/New_York')).toBe('2026/06/30 21:00')
  })

  it('支持带秒 / 仅日期 / 仅时刻三种风格', () => {
    expect(formatInZone(SAMPLE, 'Asia/Shanghai', 'datetime-second')).toBe('2026/07/01 09:00:00')
    expect(formatInZone(SAMPLE, 'Asia/Shanghai', 'date')).toBe('2026/07/01')
    expect(formatInZone(SAMPLE, 'Asia/Shanghai', 'clock')).toBe('09:00')
  })

  it('午夜按 h23 渲染成 00:xx，不出现 24:xx', () => {
    expect(formatInZone('2026-07-01T16:00:00Z', 'Asia/Shanghai', 'clock')).toBe('00:00')
  })

  it('空值与非法值返回占位符', () => {
    expect(formatInZone(undefined, 'Asia/Shanghai')).toBe('—')
    expect(formatInZone('', 'Asia/Shanghai')).toBe('—')
    expect(formatInZone('not-a-date', 'Asia/Shanghai')).toBe('—')
  })

  it('非法时区名不应抛错，退回浏览器时区', () => {
    expect(() => formatInZone(SAMPLE, 'Not/AZone')).not.toThrow()
    expect(formatInZone(SAMPLE, 'Not/AZone')).toMatch(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}$/)
  })
})

describe('formatNextFireInZone', () => {
  it('过去时间返回占位符', () => {
    expect(formatNextFireInZone('2000-01-01T00:00:00Z', 'Asia/Shanghai')).toBe('—')
  })

  it('将来时间按站点时区输出到秒', () => {
    const future = new Date(Date.now() + 86_400_000).toISOString()
    expect(formatNextFireInZone(future, 'Asia/Shanghai')).toMatch(
      /^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}$/,
    )
  })

  it('空值返回占位符', () => {
    expect(formatNextFireInZone(undefined, 'Asia/Shanghai')).toBe('—')
  })
})
