// ScheduleForm 使用 Select 下拉框选择提醒类型 + CalendarPopover 选择日期时间
import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CalendarPopover } from './CalendarPopover'
import type { ReminderCalendar, ReminderScheduleType } from '@/types'
import type { CalendarResult } from '@/types'

export interface ScheduleValue {
  calendar: ReminderCalendar
  schedule_type: ReminderScheduleType
  schedule_spec: Record<string, unknown>
}

interface Props {
  value: ScheduleValue
  onChange: (v: ScheduleValue) => void
}

const INTERVAL_UNITS = [
  { value: 'minute', label: '分钟' },
  { value: 'hour', label: '小时' },
  { value: 'day', label: '天' },
  { value: 'month', label: '月' },
  { value: 'year', label: '年' },
]

const LUNAR_INTERVAL_UNITS = [
  { value: 'day', label: '天' },
  { value: 'month', label: '月' },
  { value: 'year', label: '年' },
]

export function ScheduleForm({ value, onChange }: Props) {
  const [calendarOpen, setCalendarOpen] = useState(false)
  const spec = value.schedule_spec ?? {}

  function handleTypeChange(t: ReminderScheduleType) {
    let defaults: Record<string, unknown> = {}
    if (t === 'once') {
      if (value.calendar === 'lunar') {
        defaults = { lunar: { year: 2026, month: 1, day: 1 }, hour: 9, minute: 0 }
      } else {
        defaults = { at: '' }
      }
    } else if (t === 'interval') {
      if (value.calendar === 'lunar') {
        defaults = { start_lunar: { year: 2026, month: 1, day: 1 }, every: 1, unit: 'month', hour: 9, minute: 0 }
      } else {
        defaults = { start_at: '', every: 1, unit: 'day' }
      }
    } else {
      defaults = { expr: '0 9 * * *' }
    }
    onChange({ ...value, schedule_type: t, schedule_spec: defaults })
  }

  function patchSpec(patch: Record<string, unknown>) {
    onChange({ ...value, schedule_spec: { ...spec, ...patch } })
  }

  function handleCalendarSelect(result: CalendarResult) {
    if (result.calendar === 'lunar' && result.lunar) {
      const key = value.schedule_type === 'interval' ? 'start_lunar' : 'lunar'
      onChange({
        ...value,
        calendar: 'lunar',
        schedule_spec: { ...spec, [key]: result.lunar, hour: result.hour, minute: result.minute },
      })
    } else if (result.calendar === 'solar') {
      const key = value.schedule_type === 'interval' ? 'start_at' : 'at'
      onChange({
        ...value,
        calendar: 'solar',
        schedule_spec: { ...spec, [key]: result.date },
      })
    }
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label>提醒类型</Label>
          <Select value={value.schedule_type} onValueChange={handleTypeChange}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="once">一次性</SelectItem>
              <SelectItem value="interval">周期</SelectItem>
              <SelectItem value="cron">Cron</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {value.schedule_type === 'cron' && (
          <div className="space-y-2">
            <Label htmlFor="spec-expr">Cron 表达式</Label>
            <Input
              id="spec-expr"
              value={(spec.expr as string) ?? ''}
              onChange={(e) => patchSpec({ expr: e.target.value })}
              placeholder="例如：0 9 * * 1-5"
            />
          </div>
        )}

        {value.schedule_type === 'once' && (
          <div className="relative space-y-2">
            <Label>
              触发时间
              {value.calendar === 'lunar' && (
                <span className="text-xs text-muted-foreground ml-2">（农历）</span>
              )}
            </Label>
            <Input
              readOnly
              value={formatScheduleDate(spec, value.calendar)}
              placeholder="点击选择日期"
              onClick={() => setCalendarOpen(true)}
              className="cursor-pointer"
            />
            {calendarOpen && (
              <CalendarPopover
                date={value.calendar === 'solar' ? ((spec.at ?? spec.start_at) as string | undefined) : undefined}
                hour={(spec.hour as number) ?? 9}
                minute={(spec.minute as number) ?? 0}
                onSelect={handleCalendarSelect}
                onClose={() => setCalendarOpen(false)}
              />
            )}
          </div>
        )}
      </div>

      {value.schedule_type === 'interval' && (
        <div className="grid grid-cols-2 gap-3">
          <div className="relative space-y-2">
            <Label>
              起始时间
              {value.calendar === 'lunar' && (
                <span className="text-xs text-muted-foreground ml-2">（农历）</span>
              )}
            </Label>
            <Input
              readOnly
              value={formatScheduleDate(spec, value.calendar)}
              placeholder="点击选择日期"
              onClick={() => setCalendarOpen(true)}
              className="cursor-pointer"
            />
            {calendarOpen && (
              <CalendarPopover
                date={value.calendar === 'solar' ? ((spec.at ?? spec.start_at) as string | undefined) : undefined}
                hour={(spec.hour as number) ?? 9}
                minute={(spec.minute as number) ?? 0}
                onSelect={handleCalendarSelect}
                onClose={() => setCalendarOpen(false)}
              />
            )}
          </div>
          <div className="flex gap-2 items-end">
            <div className="space-y-2 flex-1">
              <Label htmlFor="spec-every">每</Label>
              <Input
                id="spec-every"
                type="number"
                min={1}
                value={(spec.every as number) ?? 1}
                onChange={(e) => patchSpec({ every: Number(e.target.value) || 1 })}
              />
            </div>
            <div className="space-y-2 flex-1">
              <Label htmlFor="spec-unit">单位</Label>
              <Select
                value={(spec.unit as string) ?? (value.calendar === 'lunar' ? 'month' : 'day')}
                onValueChange={(v) => patchSpec({ unit: v })}
              >
                <SelectTrigger id="spec-unit">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(value.calendar === 'lunar' ? LUNAR_INTERVAL_UNITS : INTERVAL_UNITS).map((u) => (
                    <SelectItem key={u.value} value={u.value}>
                      {u.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
      )}

      {value.schedule_type === 'cron' && (
        <p className="text-xs text-muted-foreground">
          5 字段标准 cron。例：每天 09:00 → <code>0 9 * * *</code>；工作日早 9 点 → <code>0 9 * * 1-5</code>。
        </p>
      )}
    </div>
  )
}

const LUNAR_MONTHS_DISP = [
  '正月','二月','三月','四月','五月','六月',
  '七月','八月','九月','十月','十一月','腊月',
]

const LUNAR_DAYS_DISP = [
  '初一','初二','初三','初四','初五','初六','初七','初八','初九','初十',
  '十一','十二','十三','十四','十五','十六','十七','十八','十九','二十',
  '廿一','廿二','廿三','廿四','廿五','廿六','廿七','廿八','廿九','三十',
]

function formatScheduleDate(spec: Record<string, unknown>, calendar: ReminderCalendar): string {
  if (calendar === 'lunar') {
    const lunar = (spec.lunar ?? spec.start_lunar) as { year: number; month: number; day: number } | undefined
    if (lunar) {
      return `${lunar.year}年 ${LUNAR_MONTHS_DISP[lunar.month - 1] ?? ''} ${LUNAR_DAYS_DISP[lunar.day - 1] ?? ''} ${String(spec.hour ?? 9).padStart(2, '0')}:${String(spec.minute ?? 0).padStart(2, '0')}`
    }
    return '选择农历日期'
  }
  const at = (spec.at ?? spec.start_at) as string | undefined
  if (at) return at.slice(0, 16)
  return '选择日期'
}