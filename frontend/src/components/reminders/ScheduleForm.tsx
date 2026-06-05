// ScheduleForm 把 (calendar, schedule_type, schedule_spec) 三元组的编辑界面
// 集中放在一处。公历三型 + 农历两型。
import { useEffect } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { LunarPicker } from './LunarPicker'
import type { ReminderCalendar, ReminderScheduleType } from '@/types'

type SolarType = 'once' | 'interval' | 'cron'
type LunarType = 'once' | 'interval'

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

function toDatetimeLocal(v: string | undefined): string {
  if (!v) return ''
  return v.slice(0, 16)
}

export function ScheduleForm({ value, onChange }: Props) {
  function setCalendar(next: ReminderCalendar) {
    if (next === 'lunar') {
      onChange({
        calendar: 'lunar',
        schedule_type: 'once',
        schedule_spec: {
          lunar: { year: 2026, month: 1, day: 1 },
          hour: 9,
          minute: 0,
        },
      })
    } else {
      onChange({ calendar: 'solar', schedule_type: 'once', schedule_spec: { at: '' } })
    }
  }

  function setSolarType(t: SolarType) {
    let defaults: Record<string, unknown> = {}
    if (t === 'once') defaults = { at: '' }
    if (t === 'interval') defaults = { start_at: '', every: 1, unit: 'day' }
    if (t === 'cron') defaults = { expr: '0 9 * * *' }
    onChange({ calendar: 'solar', schedule_type: t, schedule_spec: defaults })
  }

  function setLunarType(t: LunarType) {
    if (t === 'once') {
      onChange({
        calendar: 'lunar',
        schedule_type: 'once',
        schedule_spec: { lunar: { year: 2026, month: 1, day: 1 }, hour: 9, minute: 0 },
      })
    } else {
      onChange({
        calendar: 'lunar',
        schedule_type: 'interval',
        schedule_spec: { start_lunar: { year: 2026, month: 1, day: 1 }, every: 1, unit: 'month', hour: 9, minute: 0 },
      })
    }
  }

  function patchSpec(patch: Record<string, unknown>) {
    onChange({ ...value, schedule_spec: { ...value.schedule_spec, ...patch } })
  }

  // 初次挂载时若 spec 为空，按 schedule_type 填默认
  useEffect(() => {
    if (Object.keys(value.schedule_spec ?? {}).length === 0) {
      setSolarType((value.schedule_type as SolarType) || 'once')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const spec = value.schedule_spec ?? {}

  return (
    <div className="space-y-3">
      <Tabs value={value.calendar} onValueChange={(v) => setCalendar(v as ReminderCalendar)}>
        <TabsList>
          <TabsTrigger value="solar">公历</TabsTrigger>
          <TabsTrigger value="lunar">农历</TabsTrigger>
        </TabsList>

        <TabsContent value="solar" className="space-y-3 pt-3">
          <Tabs value={value.schedule_type} onValueChange={(v) => setSolarType(v as SolarType)}>
            <TabsList>
              <TabsTrigger value="once">一次性</TabsTrigger>
              <TabsTrigger value="interval">周期</TabsTrigger>
              <TabsTrigger value="cron">Cron</TabsTrigger>
            </TabsList>

            <TabsContent value="once" className="pt-3 space-y-2">
              <Label htmlFor="spec-at">触发时间</Label>
              <Input
                id="spec-at"
                type="datetime-local"
                value={toDatetimeLocal(spec.at as string)}
                onChange={(e) => patchSpec({ at: e.target.value })}
              />
              <p className="text-xs text-muted-foreground">
                按下方"提醒时区"解读；时间到达后会立即通过通道发送。
              </p>
            </TabsContent>

            <TabsContent value="interval" className="pt-3 space-y-3">
              <div className="space-y-2">
                <Label htmlFor="spec-start">起始时间</Label>
                <Input
                  id="spec-start"
                  type="datetime-local"
                  value={toDatetimeLocal(spec.start_at as string)}
                  onChange={(e) => patchSpec({ start_at: e.target.value })}
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="spec-every">每</Label>
                  <Input
                    id="spec-every"
                    type="number"
                    min={1}
                    value={(spec.every as number) ?? 1}
                    onChange={(e) => patchSpec({ every: Number(e.target.value) || 1 })}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="spec-unit">单位</Label>
                  <Select value={(spec.unit as string) ?? 'day'} onValueChange={(v) => patchSpec({ unit: v })}>
                    <SelectTrigger id="spec-unit">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {INTERVAL_UNITS.map((u) => (
                        <SelectItem key={u.value} value={u.value}>
                          {u.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                月 / 年单位使用日历语义：1 月 31 日 + 1 月 = 2 月 28 日（自动归一）。
              </p>
            </TabsContent>

            <TabsContent value="cron" className="pt-3 space-y-2">
              <Label htmlFor="spec-expr">Cron 表达式</Label>
              <Input
                id="spec-expr"
                value={(spec.expr as string) ?? ''}
                onChange={(e) => patchSpec({ expr: e.target.value })}
                placeholder="例如：0 9 * * 1-5"
              />
              <p className="text-xs text-muted-foreground">
                5 字段标准 cron。例：每天 09:00 → <code>0 9 * * *</code>；工作日早 9 点 → <code>0 9 * * 1-5</code>。
              </p>
            </TabsContent>
          </Tabs>
        </TabsContent>

        <TabsContent value="lunar" className="space-y-3 pt-3">
          <Tabs value={value.schedule_type} onValueChange={(v) => setLunarType(v as LunarType)}>
            <TabsList>
              <TabsTrigger value="once">一次性</TabsTrigger>
              <TabsTrigger value="interval">周期</TabsTrigger>
            </TabsList>

            <TabsContent value="once" className="pt-3 space-y-3">
              <LunarPicker
                value={(spec.lunar as { year: number; month: number; day: number }) ?? { year: 2026, month: 1, day: 1 }}
                onChange={(v) =>
                  onChange({
                    ...value,
                    schedule_spec: { ...value.schedule_spec, lunar: v, hour: spec.hour ?? 9, minute: spec.minute ?? 0 },
                  })
                }
                hour={(spec.hour as number) ?? 9}
                minute={(spec.minute as number) ?? 0}
                onHourChange={(h) => patchSpec({ hour: h })}
                onMinuteChange={(m) => patchSpec({ minute: m })}
              />
            </TabsContent>

            <TabsContent value="interval" className="pt-3 space-y-3">
              <LunarPicker
                value={
                  (spec.start_lunar as { year: number; month: number; day: number }) ?? {
                    year: 2026,
                    month: 1,
                    day: 1,
                  }
                }
                onChange={(v) =>
                  onChange({
                    ...value,
                    schedule_spec: {
                      ...value.schedule_spec,
                      start_lunar: v,
                      hour: spec.hour ?? 9,
                      minute: spec.minute ?? 0,
                    },
                  })
                }
                hour={(spec.hour as number) ?? 9}
                minute={(spec.minute as number) ?? 0}
                onHourChange={(h) => patchSpec({ hour: h })}
                onMinuteChange={(m) => patchSpec({ minute: m })}
              />
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="lunar-every">每</Label>
                  <Input
                    id="lunar-every"
                    type="number"
                    min={1}
                    value={(spec.every as number) ?? 1}
                    onChange={(e) => patchSpec({ every: Number(e.target.value) || 1 })}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="lunar-unit">单位</Label>
                  <Select value={(spec.unit as string) ?? 'month'} onValueChange={(v) => patchSpec({ unit: v })}>
                    <SelectTrigger id="lunar-unit">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {LUNAR_INTERVAL_UNITS.map((u) => (
                        <SelectItem key={u.value} value={u.value}>
                          {u.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                闰月跳过（按公历年份安排）。日数超出该月天数时自动顺延至月末。
              </p>
            </TabsContent>
          </Tabs>
        </TabsContent>
      </Tabs>
    </div>
  )
}
