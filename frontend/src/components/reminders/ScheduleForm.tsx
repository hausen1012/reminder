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
import {
  formatReminderSolarLine,
  formatReminderLunarLine,
  getReminderSpecHour,
  getReminderSpecMinute,
} from '@/lib/utils'
import { Lunar } from 'lunar-typescript'
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
    const old = spec
    let next: Record<string, unknown> = {}

    if (t === 'once') {
      if (value.calendar === 'lunar') {
        const lunar = (old.lunar ?? old.start_lunar) as { year: number; month: number; day: number } | undefined
        next = {
          lunar: lunar ?? { year: 2026, month: 1, day: 1 },
          hour: getReminderSpecHour(old) || 9,
          minute: getReminderSpecMinute(old) ?? 0,
        }
      } else {
        next = { at: (old.at ?? old.start_at ?? '') as string }
      }
    } else if (t === 'interval') {
      if (value.calendar === 'lunar') {
        const lunar = (old.lunar ?? old.start_lunar) as { year: number; month: number; day: number } | undefined
        next = {
          start_lunar: lunar ?? { year: 2026, month: 1, day: 1 },
          every: (old.every as number) ?? 1,
          unit: (old.unit as string) ?? 'month',
          hour: getReminderSpecHour(old) || 9,
          minute: getReminderSpecMinute(old) ?? 0,
        }
      } else {
        next = {
          start_at: (old.at ?? old.start_at ?? '') as string,
          every: (old.every as number) ?? 1,
          unit: (old.unit as string) ?? 'day',
        }
      }
    } else {
      next = { expr: (old.expr as string) || '0 9 * * *' }
    }
    onChange({ ...value, schedule_type: t, schedule_spec: next })
  }

  function patchSpec(patch: Record<string, unknown>) {
    onChange({ ...value, schedule_spec: { ...spec, ...patch } })
  }

  function handleCalendarSelect(result: CalendarResult) {
    const { at, start_at, lunar, start_lunar, hour, minute, ...rest } = spec

    if (result.calendar === 'lunar' && result.lunar) {
      const key = value.schedule_type === 'interval' ? 'start_lunar' : 'lunar'
      onChange({
        ...value,
        calendar: 'lunar',
        schedule_spec: { ...rest, [key]: result.lunar, hour: result.hour, minute: result.minute },
      })
    } else if (result.calendar === 'solar') {
      const key = value.schedule_type === 'interval' ? 'start_at' : 'at'
      onChange({
        ...value,
        calendar: 'solar',
        schedule_spec: { ...rest, [key]: result.date },
      })
    }
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label>类型</Label>
          <Select value={value.schedule_type} onValueChange={handleTypeChange}>
            <SelectTrigger className="flex-1">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="once">单次</SelectItem>
              <SelectItem value="interval">周期</SelectItem>
              <SelectItem value="cron">Cron</SelectItem>
            </SelectContent>
          </Select>
          {value.schedule_type === 'interval' && (
            <div className="space-y-2 pt-2">
              <Label>间隔</Label>
              <div className="flex gap-2">
                <Input
                  type="number"
                  min={1}
                  value={(spec.every as number) ?? 1}
                  onChange={(e) => patchSpec({ every: Number(e.target.value) || 1 })}
                  className="flex-1"
                />
                <Select
                  value={(spec.unit as string) ?? (value.calendar === 'lunar' ? 'month' : 'day')}
                  onValueChange={(v) => patchSpec({ unit: v })}
                >
                  <SelectTrigger className="flex-1">
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
          )}
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

        {value.schedule_type !== 'cron' && (
          <div className="relative space-y-2">
            <Label>时间</Label>
            <div
              onClick={() => setCalendarOpen(true)}
              className="flex min-h-[2.5rem] cursor-pointer items-center rounded-md border border-input bg-background px-3 py-1.5 text-sm ring-offset-background hover:bg-accent"
            >
              <span className="truncate text-xs leading-tight">
                {formatReminderSolarLine(spec, value.calendar)}
                {formatReminderLunarLine(spec, value.calendar) && (
                  <span className="ml-1.5 text-[11px] text-muted-foreground">
                    · {formatReminderLunarLine(spec, value.calendar)}
                  </span>
                )}
              </span>
            </div>
            {calendarOpen && (
              <CalendarPopover
                date={(() => {
                  if (value.calendar === 'solar') return (spec.at ?? spec.start_at) as string | undefined
                  const lunar = (spec.lunar ?? spec.start_lunar) as { year: number; month: number; day: number } | undefined
                  if (lunar) {
                    const l = Lunar.fromYmd(lunar.year, lunar.month, lunar.day)
                    const s = l.getSolar()
                    return `${s.getYear()}-${String(s.getMonth()).padStart(2, '0')}-${String(s.getDay()).padStart(2, '0')}T${String(spec.hour ?? 9).padStart(2, '0')}:${String(spec.minute ?? 0).padStart(2, '0')}`
                  }
                  return undefined
                })()}
                hour={getReminderSpecHour(spec)}
                minute={getReminderSpecMinute(spec)}
                initialCalendar={value.calendar}
                onSelect={handleCalendarSelect}
                onClose={() => setCalendarOpen(false)}
              />
            )}
          </div>
        )}
      </div>

      {value.schedule_type === 'cron' && (
        <p className="text-xs text-muted-foreground">
          5 字段标准 cron。例：每天 09:00 → <code>0 9 * * *</code>；工作日早 9 点 → <code>0 9 * * 1-5</code>。
        </p>
      )}
    </div>
  )
}
