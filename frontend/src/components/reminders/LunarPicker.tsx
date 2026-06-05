// LunarPicker 是农历日期选择器：年/月/日三个 select。
// 月份显示中文 "正月/二月/.../腊月"，日显示 "初一/初二/.../*日/三十"。
import { useMemo } from 'react'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const LUNAR_MONTHS = [
  '正月', '二月', '三月', '四月', '五月', '六月',
  '七月', '八月', '九月', '十月', '十一月', '腊月',
]

// 农历日 1-30，用中文数字显示
const LUNAR_DAYS = [
  '初一', '初二', '初三', '初四', '初五', '初六', '初七', '初八', '初九', '初十',
  '十一', '十二', '十三', '十四', '十五', '十六', '十七', '十八', '十九', '二十',
  '廿一', '廿二', '廿三', '廿四', '廿五', '廿六', '廿七', '廿八', '廿九', '三十',
]

// 合理农历年份范围（对应公历 2020-2040）
const YEAR_OPTIONS = (() => {
  const years: number[] = []
  for (let y = 2020; y <= 2040; y++) {
    years.push(y)
  }
  return years
})()

interface LunarValue {
  year: number
  month: number // 1-12
  day: number   // 1-30
}

interface Props {
  value: LunarValue
  onChange: (v: LunarValue) => void
  hour?: number
  minute?: number
  onHourChange?: (h: number) => void
  onMinuteChange?: (m: number) => void
}

export function LunarPicker({ value, onChange, hour = 9, minute = 0, onHourChange, onMinuteChange }: Props) {
  const days = useMemo(() => {
    // 农历通常 29 或 30 天，显示全 30 天让用户选；size_policy 在后端自动 shift
    return LUNAR_DAYS
  }, [value.year, value.month])

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        以下日期均为农历。若选中日期在对应月份不存在（例：三十在小月），后端自动顺延至月末。
      </p>
      <div className="grid grid-cols-3 gap-3">
        <div className="space-y-2">
          <Label>年</Label>
          <Select
            value={String(value.year)}
            onValueChange={(v) => onChange({ ...value, year: Number(v) })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="max-h-60">
              {YEAR_OPTIONS.map((y) => (
                <SelectItem key={y} value={String(y)}>
                  {y}年
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>月</Label>
          <Select
            value={String(value.month)}
            onValueChange={(v) => onChange({ ...value, month: Number(v) })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LUNAR_MONTHS.map((m, i) => (
                <SelectItem key={i + 1} value={String(i + 1)}>
                  {m}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>日</Label>
          <Select
            value={String(value.day)}
            onValueChange={(v) => onChange({ ...value, day: Number(v) })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="max-h-60">
              {days.map((d, i) => (
                <SelectItem key={i + 1} value={String(i + 1)}>
                  {d}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      {onHourChange != null && onMinuteChange != null && (
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-2">
            <Label htmlFor="lunar-hour">时</Label>
            <Select value={String(hour)} onValueChange={(v) => onHourChange(Number(v))}>
              <SelectTrigger id="lunar-hour">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Array.from({ length: 24 }, (_, i) => (
                  <SelectItem key={i} value={String(i)}>
                    {String(i).padStart(2, '0')}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="lunar-minute">分</Label>
            <Select value={String(minute)} onValueChange={(v) => onMinuteChange(Number(v))}>
              <SelectTrigger id="lunar-minute">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Array.from({ length: 60 }, (_, i) => (
                  <SelectItem key={i} value={String(i)}>
                    {String(i).padStart(2, '0')}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      )}
    </div>
  )
}
