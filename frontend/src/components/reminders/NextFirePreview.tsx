// NextFirePreview 调用 /api/reminders/preview 显示下 5 次触发时间。
import { useEffect, useState } from 'react'
import { previewReminder } from '@/lib/api'
import type { ReminderCalendar, ReminderScheduleType } from '@/types'

interface Props {
  calendar: ReminderCalendar
  scheduleType: ReminderScheduleType
  spec: Record<string, unknown>
  timezone?: string
  count?: number
}

export function NextFirePreview({ calendar, scheduleType, spec, timezone, count = 5 }: Props) {
  const [times, setTimes] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    // 防抖：等输入稳定一会儿再请求
    const t = setTimeout(async () => {
      try {
        const arr = await previewReminder({
          calendar,
          schedule_type: scheduleType,
          schedule_spec: spec,
          timezone,
          count,
        })
        if (!cancelled) setTimes(arr)
      } catch (err: unknown) {
        const msg = (err as { response?: { data?: { error?: { message?: string } } } })?.response?.data?.error?.message
        if (!cancelled) {
          setTimes([])
          setError(msg ?? '预览失败')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }, 400)
    return () => {
      cancelled = true
      clearTimeout(t)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [calendar, scheduleType, JSON.stringify(spec), timezone, count])

  return (
    <div className="rounded-md border bg-muted/30 p-3">
      <div className="mb-2 text-sm font-medium">下 {count} 次触发预览</div>
      {loading ? (
        <p className="text-xs text-muted-foreground">计算中…</p>
      ) : error ? (
        <p className="text-xs text-destructive">{error}</p>
      ) : times.length === 0 ? (
        <p className="text-xs text-muted-foreground">无可预览的触发时间（spec 已过期或未填全）。</p>
      ) : (
        <ul className="space-y-0.5 text-xs font-mono">
          {times.map((t, i) => (
            <li key={i}>{new Date(t).toLocaleString()}</li>
          ))}
        </ul>
      )}
    </div>
  )
}
