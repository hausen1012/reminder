// ScheduleForm 把 (calendar, schedule_type, schedule_spec) 三元组的编辑界面
// 集中放在一处。Phase 2 只实现 solar 三型；lunar Tab 在 Phase 3 接入。
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
import type { ReminderCalendar, ReminderScheduleType } from '@/types'

type SolarType = 'once' | 'interval' | 'cron'

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

// 把 ISO 字符串转成 datetime-local 控件期望的 "YYYY-MM-DDTHH:mm"。
function toDatetimeLocal(v: string | undefined): string {
  if (!v) return ''
  // 后端约定字符串就是配置时区下的 "2006-01-02T15:04:05"，可直接截前 16 位
  return v.slice(0, 16)
}

export function ScheduleForm({ value, onChange }: Props) {
  // calendar 切换
  function setCalendar(next: ReminderCalendar) {
    if (next === 'lunar') return // Phase 2 禁用
    onChange({ calendar: 'solar', schedule_type: value.schedule_type, schedule_spec: value.schedule_spec })
  }

  function setSolarType(t: SolarType) {
    let defaults: Record<string, unknown> = {}
    if (t === 'once') defaults = { at: '' }
    if (t === 'interval') defaults = { start_at: '', every: 1, unit: 'day' }
    if (t === 'cron') defaults = { expr: '0 9 * * *' }
    onChange({ calendar: 'solar', schedule_type: t, schedule_spec: defaults })
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
          <TabsTrigger value="lunar" disabled title="Phase 3 启用">
            农历
          </TabsTrigger>
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
      </Tabs>
    </div>
  )
}
