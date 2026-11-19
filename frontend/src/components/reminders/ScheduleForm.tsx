// ScheduleForm 使用 Select 下拉框选择提醒类型 + CalendarPopover 选择日期时间
import { useState, useRef } from 'react'
import { cn } from '@/lib/utils'
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
  { value: 'month', label: '月' },
  { value: 'year', label: '年' },
]

const SOLAR_INTERVAL_PRESETS = [
  { label: '每天', every: 1, unit: 'day' },
  { label: '每周', every: 7, unit: 'day' },
  { label: '每月', every: 1, unit: 'month' },
  { label: '每年', every: 1, unit: 'year' },
] as const

const LUNAR_INTERVAL_PRESETS = [
  { label: '每月', every: 1, unit: 'month' },
  { label: '每年', every: 1, unit: 'year' },
] as const

const CRON_PRESETS = [
  { label: '工作日 09:00', expr: '0 9 * * 1-5' },
  { label: '每天 09:00', expr: '0 9 * * *' },
  { label: '每周一 09:00', expr: '0 9 * * 1' },
  { label: '每小时', expr: '0 * * * *' },
  { label: '每 30 分钟', expr: '*/30 * * * *' },
] as const

export function ScheduleForm({ value, onChange }: Props) {
  const [calendarOpen, setCalendarOpen] = useState(false)
  const [customInterval, setCustomInterval] = useState(false)
  const triggerRef = useRef<HTMLDivElement>(null)
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
        next = { at: String(old.at ?? old.start_at ?? '') }
      }
    } else if (t === 'interval') {
      if (value.calendar === 'lunar') {
        const lunar = (old.lunar ?? old.start_lunar) as { year: number; month: number; day: number } | undefined
        next = {
          start_lunar: lunar ?? { year: 2026, month: 1, day: 1 },
          every: Number(old.every ?? 1),
          unit: String(old.unit ?? 'month'),
          hour: getReminderSpecHour(old) || 9,
          minute: getReminderSpecMinute(old) ?? 0,
        }
      } else {
        next = {
          start_at: String(old.at ?? old.start_at ?? ''),
          every: Number(old.every ?? 1),
          unit: String(old.unit ?? 'day'),
        }
      }
    } else {
      next = { expr: String(old.expr ?? '0 9 * * *') }
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
        {value.schedule_type === 'interval' && (() => {
          const everyRaw = spec.every
          const everyNum = Number(everyRaw ?? 1)
          const rawUnit = String(spec.unit ?? (value.calendar === 'lunar' ? 'month' : 'day'))
          const unit = value.calendar === 'lunar' && rawUnit !== 'month' && rawUnit !== 'year' ? 'month' : rawUnit
          const presets = value.calendar === 'lunar' ? LUNAR_INTERVAL_PRESETS : SOLAR_INTERVAL_PRESETS
          const matched = presets.find((p) => p.every === everyNum && p.unit === unit)
          const isCustom = customInterval || !matched

          return (
            <div className="space-y-2 pt-1">
              <div className="flex flex-wrap items-center gap-1.5">
                {presets.map((p) => {
                  const active = !customInterval && matched?.every === p.every && matched?.unit === p.unit
                  return (
                    <button
                      key={`${p.every}-${p.unit}`}
                      type="button"
                      onClick={() => {
                        setCustomInterval(false)
                        patchSpec({ every: p.every, unit: p.unit })
                      }}
                      className={cn(
                        'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                        active
                          ? 'bg-primary text-primary-foreground shadow-sm'
                          : 'border border-border/80 bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                      )}
                    >
                      {p.label}
                    </button>
                  )
                })}
                <button
                  type="button"
                  onClick={() => setCustomInterval(true)}
                  className={cn(
                    'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                    isCustom
                      ? 'bg-primary text-primary-foreground shadow-sm'
                      : 'border border-border/80 bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                  )}
                >
                  自定义…
                </button>
              </div>

              {isCustom && (
                <div className="flex items-center gap-2 pt-1">
                  <span className="text-xs text-muted-foreground">每隔</span>
                  <Input
                    type="number"
                    min={1}
                    value={everyRaw === '' ? '' : (everyRaw !== undefined ? Number(everyRaw) : 1)}
                    onChange={(e) => {
                      const val = e.target.value
                      if (val === '') {
                        patchSpec({ every: '' })
                        return
                      }
                      const num = parseInt(val, 10)
                      patchSpec({ every: isNaN(num) ? '' : Math.max(1, num) })
                    }}
                    onBlur={() => {
                      if (spec.every === '' || spec.every === undefined || Number(spec.every) < 1) {
                        patchSpec({ every: 1 })
                      }
                    }}
                    className="h-8 w-20 text-center text-xs"
                    aria-label="周期数值"
                  />
                  <Select value={unit} onValueChange={(v) => patchSpec({ unit: v })}>
                    <SelectTrigger className="h-8 w-24 text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {(value.calendar === 'lunar' ? LUNAR_INTERVAL_UNITS : INTERVAL_UNITS).map((u) => (
                        <SelectItem key={u.value} value={u.value} className="text-xs">
                          {u.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <span className="text-xs text-muted-foreground">重复</span>
                </div>
              )}
            </div>
          )
        })()}
      </div>

      {value.schedule_type === 'cron' && (
        <div className="space-y-2">
          <Label htmlFor="spec-expr">Cron 表达式</Label>
          <div className="flex flex-wrap items-center gap-1.5 pb-0.5">
            {CRON_PRESETS.map((p) => (
              <button
                key={p.expr}
                type="button"
                onClick={() => patchSpec({ expr: p.expr })}
                className={cn(
                  'rounded-md px-2 py-0.5 text-xs font-medium transition-colors',
                  spec.expr === p.expr
                    ? 'bg-primary text-primary-foreground shadow-sm'
                    : 'border border-border/80 bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                )}
              >
                {p.label}
              </button>
            ))}
          </div>
          <Input
            id="spec-expr"
            value={String(spec.expr ?? '')}
            onChange={(e) => patchSpec({ expr: e.target.value })}
            placeholder="标准 5 位格式，例如：0 9 * * 1-5"
          />
        </div>
      )}

      {value.schedule_type !== 'cron' && (
        <div className="relative space-y-2">
          <Label>时间</Label>
          <div
            ref={triggerRef}
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
              triggerRef={triggerRef}
              date={(() => {
                if (value.calendar === 'solar') return String(spec.at ?? spec.start_at ?? '') || undefined
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
  )
}
