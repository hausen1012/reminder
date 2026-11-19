// ReminderEditDialog 是提醒的新建 / 编辑表单。
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { useToast } from '@/components/ui/use-toast'
import { useSiteTimezone } from '@/contexts/ConfigContext'
import { nowInZone, toWallClockString } from '@/lib/datetime'
import { ChannelMultiSelect } from '@/components/channels/ChannelMultiSelect'
import { ScheduleForm, type ScheduleValue } from './ScheduleForm'
import { createReminder, extractApiError, listChannels, testReminderDryRun, updateReminder } from '@/lib/api'
import type { Channel, Reminder, ReminderInput } from '@/types'
import { toContentFormat } from '@/types'
import { cn, insertSnippet } from '@/lib/utils'

const INSERT_PLACEHOLDERS = [
  { label: '日期', code: '{{now_date}}' },
  { label: '时间', code: '{{now_time}}' },
  { label: '农历', code: '{{lunar_date}}' },
  { label: '星期', code: '{{weekday}}' },
  { label: '标题', code: '{{title}}' },
] as const

const RETRY_INTERVAL_PRESETS = [
  { label: '30 分钟', sec: 1800 },
  { label: '1 小时', sec: 3600 },
  { label: '2 小时', sec: 7200 },
] as const

const RETRY_MAX_PRESETS = [1, 2, 3, 5] as const

interface Props {
  reminder: Reminder | null
  open: boolean
  onClose: () => void
  onSaved: () => void
}

function defaultInput(siteTimezone: string): ReminderInput {
  // 默认时间 = 站点时区下的当前时间整点 + 1 小时
  const nextHour = nowInZone(siteTimezone, 60)
  const at = toWallClockString({ ...nextHour, minute: 0 })
  return {
    title: '',
    content: '',
    content_format: 'text',
    calendar: 'solar',
    schedule_type: 'once',
    schedule_spec: { at },
    source: 'web',
    channel_ids: [],
    require_confirm: false,
    confirm_retry_interval_sec: 7200,
    confirm_max_retries: 3,
  }
}

export function ReminderEditDialog({ reminder, open, onClose, onSaved }: Props) {
  const isEdit = Boolean(reminder)
  // 站点时区是全局设置：既用于提示用户时间按哪个时区解释，
  // 也用于生成当前时间整点这类默认值（不能按浏览器时区算）。
  const tz = useSiteTimezone()
  const [input, setInput] = useState<ReminderInput>(() => defaultInput(tz))
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [channelOpen, setChannelOpen] = useState(false)
  const [channels, setChannels] = useState<Channel[]>([])
  const { toast } = useToast()
  const siteTimezone = tz

  const initialSec = input.confirm_retry_interval_sec || 7200
  const initialIsHour = initialSec >= 3600 && initialSec % 3600 === 0
  const [intervalUnit, setIntervalUnit] = useState<'minute' | 'hour'>(initialIsHour ? 'hour' : 'minute')
  const [intervalValue, setIntervalValue] = useState<number | ''>(
    initialIsHour ? initialSec / 3600 : Math.max(1, Math.round(initialSec / 60)),
  )
  const [customInterval, setCustomInterval] = useState(false)
  const [customMax, setCustomMax] = useState(false)
  const contentRef = useRef<HTMLTextAreaElement>(null)

  // 在光标处插入变量；未定位过光标时追加到末尾，并把光标放到插入内容之后
  function insertPlaceholder(code: string) {
    const el = contentRef.current
    const text = input.content
    const start = el?.selectionStart ?? text.length
    const end = el?.selectionEnd ?? text.length
    const next = insertSnippet(text, code, start, end)
    patch('content', next.text)
    requestAnimationFrame(() => {
      if (!el) return
      el.focus()
      el.setSelectionRange(next.cursor, next.cursor)
    })
  }

  useEffect(() => {
    listChannels()
      .then(setChannels)
      .catch(() => setChannels([]))
  }, [])

  useEffect(() => {
    if (reminder) {
      const sec = reminder.confirm_retry_interval_sec || 7200
      const isHour = sec >= 3600 && sec % 3600 === 0
      setIntervalUnit(isHour ? 'hour' : 'minute')
      setIntervalValue(isHour ? sec / 3600 : Math.max(1, Math.round(sec / 60)))
      setCustomInterval(!RETRY_INTERVAL_PRESETS.some((p) => p.sec === sec))
      setCustomMax(!RETRY_MAX_PRESETS.includes((reminder.confirm_max_retries || 3) as 1 | 2 | 3 | 5))
      setInput({
        title: reminder.title,
        content: reminder.content,
        content_format: reminder.content_format === 'html' ? 'markdown' : (reminder.content_format || 'text'),
        calendar: reminder.calendar,
        schedule_type: reminder.schedule_type,
        schedule_spec: reminder.schedule_spec,
        channel_ids: reminder.channel_ids,
        require_confirm: reminder.require_confirm,
        confirm_retry_interval_sec: sec,
        confirm_max_retries: reminder.confirm_max_retries || 3,
      })
    } else {
      const def = defaultInput(tz)
      setIntervalUnit('hour')
      setIntervalValue(2)
      setCustomInterval(false)
      setCustomMax(false)
      setInput(def)
    }
  }, [reminder, tz])

  function handleIntervalChange(val: number | '', unit: 'minute' | 'hour') {
    setIntervalValue(val)
    setIntervalUnit(unit)
    if (typeof val === 'number' && val > 0) {
      const totalSec = Math.max(60, val * (unit === 'hour' ? 3600 : 60))
      patch('confirm_retry_interval_sec', totalSec)
    }
  }

  function patch<K extends keyof ReminderInput>(key: K, val: ReminderInput[K]) {
    setInput((prev) => ({ ...prev, [key]: val }))
  }

  function patchSchedule(v: ScheduleValue) {
    setInput((prev) => ({
      ...prev,
      calendar: v.calendar,
      schedule_type: v.schedule_type,
      schedule_spec: v.schedule_spec,
    }))
  }

  async function handleTest() {
    if (input.channel_ids.length === 0) {
      toast({ title: '请先选择通知渠道', variant: 'destructive' })
      return
    }
    setTesting(true)
    try {
      await testReminderDryRun({
        id: reminder?.id ?? 0,
        title: input.title,
        content: input.content,
        content_format: input.content_format || 'text',
        channel_ids: input.channel_ids,
      })
      toast({ title: '测试提醒已触发', description: '请检查通知渠道是否收到消息。', variant: 'success' })
    } catch (err) {
      toast({ title: '测试失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setTesting(false)
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!input.title.trim()) {
      toast({ title: '标题必填', variant: 'destructive' })
      return
    }
    if (input.channel_ids.length === 0) {
      toast({ title: '至少选择一个通知渠道', variant: 'destructive' })
      return
    }
    setSaving(true)
    try {
      const sanitizedSpec = { ...input.schedule_spec }
      if (input.schedule_type === 'interval') {
        sanitizedSpec.every = Math.max(1, Number(sanitizedSpec.every) || 1)
      }
      const payload: ReminderInput = {
        ...input,
        schedule_spec: sanitizedSpec,
        confirm_max_retries: Math.max(1, Number(input.confirm_max_retries) || 3),
        confirm_retry_interval_sec: Math.max(60, Number(input.confirm_retry_interval_sec) || 60),
      }
      if (isEdit && reminder) {
        await updateReminder(reminder.id, payload)
      } else {
        await createReminder(payload)
      }
      toast({ title: isEdit ? '提醒已更新' : '提醒已创建', variant: 'success' })
      onSaved()
    } catch (err: unknown) {
      toast({ title: '保存失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        className="max-w-2xl md:max-h-[90dvh] md:overflow-y-auto"
        onPointerDownOutside={(e) => {
          // CalendarPopover 通过 createPortal(document.body) 渲染，不在 Dialog DOM 子树内，
          // Radix Dialog 的 DismissableLayer 会拦截其 pointerdown 并阻止 click 产生。
          // 检测到点击来自日历弹窗时解除拦截，其他外部点击仍正常关闭 Dialog。
          const target = e.detail.originalEvent.target as Element | null
          if (!target) return
          if (target.closest('[data-calendar-popover]')) {
            e.preventDefault()
          }
          // Radix Select 也通过 portal 渲染，点击月份/年份选择器时不关闭 Dialog
          if (target.closest('[role="listbox"]') || target.closest('[data-radix-popper-content-wrapper]')) {
            e.preventDefault()
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>{isEdit ? '编辑提醒' : '新建提醒'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="r-title">标题</Label>
            <Input
              id="r-title"
              value={input.title}
              onChange={(e) => patch('title', e.target.value)}
              maxLength={200}
              required
            />
          </div>
          <div className="space-y-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Label htmlFor="r-content">内容</Label>
              <Tabs
                value={input.content_format || 'text'}
                onValueChange={(v) => patch('content_format', toContentFormat(v))}
                className="w-auto"
              >
                <TabsList className="h-8">
                  <TabsTrigger value="text" className="text-xs px-3">纯文本</TabsTrigger>
                  <TabsTrigger value="markdown" className="text-xs px-3">Markdown</TabsTrigger>
                </TabsList>
              </Tabs>
            </div>
            <Textarea
              id="r-content"
              ref={contentRef}
              value={input.content}
              onChange={(e) => patch('content', e.target.value)}
              rows={3}
              placeholder="输入提醒详细内容…"
            />
            <div className="flex flex-wrap items-center gap-1.5 pt-0.5">
              {INSERT_PLACEHOLDERS.map((p) => (
                <button
                  key={p.code}
                  type="button"
                  onClick={() => insertPlaceholder(p.code)}
                  className="inline-flex items-center rounded border border-border/60 bg-muted/40 px-1.5 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  title={`插入 ${p.code}`}
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-1">
            <ScheduleForm
              value={{
                calendar: input.calendar,
                schedule_type: input.schedule_type,
                schedule_spec: input.schedule_spec,
              }}
              onChange={patchSchedule}
            />
            <p className="text-xs text-muted-foreground">
              时间按站点时区（{siteTimezone}）解释
            </p>
          </div>

          <div className="space-y-2">
            <Label>通知渠道</Label>
            <ChannelMultiSelect
              channels={channels}
              value={input.channel_ids}
              open={channelOpen}
              onOpenChange={setChannelOpen}
              onChange={(channelIds) => patch('channel_ids', channelIds)}
            />
          </div>

          <div className="rounded-md border p-3">
            <div className="flex items-center justify-between">
              <Label htmlFor="r-confirm" className="cursor-pointer font-medium">确认提醒</Label>
              <Switch
                id="r-confirm"
                checked={input.require_confirm}
                onCheckedChange={(v) => patch('require_confirm', v)}
              />
            </div>
            {input.require_confirm && (
              <div className="mt-3 space-y-3 border-t pt-3">
                <div className="space-y-1.5">
                  <span className="text-xs font-medium text-muted-foreground">重试间隔</span>
                  <div className="flex flex-wrap items-center gap-1.5">
                    {RETRY_INTERVAL_PRESETS.map((p) => {
                      const active = !customInterval && input.confirm_retry_interval_sec === p.sec
                      return (
                        <button
                          key={p.sec}
                          type="button"
                          onClick={() => {
                            setCustomInterval(false)
                            patch('confirm_retry_interval_sec', p.sec)
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
                        customInterval || !RETRY_INTERVAL_PRESETS.some((p) => p.sec === input.confirm_retry_interval_sec)
                          ? 'bg-primary text-primary-foreground shadow-sm'
                          : 'border border-border/80 bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                      )}
                    >
                      自定义…
                    </button>
                  </div>
                  {(customInterval || !RETRY_INTERVAL_PRESETS.some((p) => p.sec === input.confirm_retry_interval_sec)) && (
                    <div className="flex items-center gap-2 pt-1">
                      <Input
                        type="number"
                        min={1}
                        value={intervalValue}
                        onChange={(e) => {
                          const val = e.target.value
                          if (val === '') {
                            handleIntervalChange('', intervalUnit)
                            return
                          }
                          const num = parseInt(val, 10)
                          handleIntervalChange(isNaN(num) ? '' : num, intervalUnit)
                        }}
                        onBlur={() => {
                          if (intervalValue === '' || intervalValue < 1) {
                            handleIntervalChange(1, intervalUnit)
                          }
                        }}
                        className="h-8 w-20 text-center text-xs"
                        aria-label="重试间隔数值"
                      />
                      <Select
                        value={intervalUnit}
                        onValueChange={(u) => handleIntervalChange(intervalValue, u as 'minute' | 'hour')}
                      >
                        <SelectTrigger className="h-8 w-24 text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="minute" className="text-xs">分钟</SelectItem>
                          <SelectItem value="hour" className="text-xs">小时</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                </div>

                <div className="space-y-1.5">
                  <span className="text-xs font-medium text-muted-foreground">最多重试</span>
                  <div className="flex flex-wrap items-center gap-1.5">
                    {RETRY_MAX_PRESETS.map((count) => {
                      const active = !customMax && input.confirm_max_retries === count
                      return (
                        <button
                          key={count}
                          type="button"
                          onClick={() => {
                            setCustomMax(false)
                            patch('confirm_max_retries', count)
                          }}
                          className={cn(
                            'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                            active
                              ? 'bg-primary text-primary-foreground shadow-sm'
                              : 'border border-border/80 bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                          )}
                        >
                          {count} 次
                        </button>
                      )
                    })}
                    <button
                      type="button"
                      onClick={() => setCustomMax(true)}
                      className={cn(
                        'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                        customMax || !RETRY_MAX_PRESETS.includes(input.confirm_max_retries as 1 | 2 | 3 | 5)
                          ? 'bg-primary text-primary-foreground shadow-sm'
                          : 'border border-border/80 bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                      )}
                    >
                      自定义…
                    </button>
                  </div>
                  {(customMax || !RETRY_MAX_PRESETS.includes(input.confirm_max_retries as 1 | 2 | 3 | 5)) && (
                    <div className="flex items-center gap-2 pt-1">
                      <Input
                        type="number"
                        min={1}
                        max={100}
                        value={input.confirm_max_retries}
                        onChange={(e) => {
                          const val = e.target.value
                          if (val === '') {
                            patch('confirm_max_retries', '' as unknown as number)
                            return
                          }
                          const num = parseInt(val, 10)
                          patch('confirm_max_retries', (isNaN(num) ? '' : num) as unknown as number)
                        }}
                        onBlur={() => {
                          if (input.confirm_max_retries === ('' as unknown as number) || Number(input.confirm_max_retries) < 1) {
                            patch('confirm_max_retries', 3)
                          }
                        }}
                        className="h-8 w-20 text-center text-xs"
                        aria-label="最大重试次数"
                      />
                      <span className="text-xs text-muted-foreground">次</span>
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="secondary"
              onClick={handleTest}
              disabled={testing || input.channel_ids.length === 0}
              title={input.channel_ids.length === 0 ? '请先选择通知渠道' : '立即触发一次测试发送'}
            >
              {testing ? '测试中…' : '测试提醒'}
            </Button>
            <Button type="button" variant="outline" onClick={onClose}>
              取消
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? '保存中…' : '保存'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
