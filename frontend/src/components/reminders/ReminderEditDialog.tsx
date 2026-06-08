// ReminderEditDialog 是提醒的新建 / 编辑表单。
import { useEffect, useState, type FormEvent } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { ChevronDown } from 'lucide-react'
import { useToast } from '@/components/ui/use-toast'
import { ScheduleForm, type ScheduleValue } from './ScheduleForm'
import { createReminder, listChannels, updateReminder } from '@/lib/api'
import type { Channel, Reminder, ReminderInput } from '@/types'

interface Props {
  reminder: Reminder | null
  open: boolean
  onClose: () => void
  onSaved: () => void
}

function defaultInput(): ReminderInput {
  return {
    title: '',
    content: '',
    calendar: 'solar',
    schedule_type: 'once',
    schedule_spec: { at: '' },
    timezone: 'Asia/Shanghai',
    source: 'manual',
    channel_ids: [],
    require_confirm: false,
    confirm_retry_interval_sec: 300,
    confirm_max_retries: 3,
  }
}

function formatChannelNames(channels: Channel[], ids: number[]): string {
  if (ids.length === 0) return '选择通道'
  const names = ids.map((id) => channels.find((c) => c.id === id)?.name ?? `#${id}`)
  return names.join(', ')
}

const COMMON_TIMEZONES = [
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Hong_Kong',
  'Asia/Singapore',
  'Asia/Seoul',
  'Asia/Taipei',
  'Asia/Kolkata',
  'Asia/Dubai',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Moscow',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Australia/Sydney',
  'Pacific/Auckland',
  'UTC',
]

export function ReminderEditDialog({ reminder, open, onClose, onSaved }: Props) {
  const isEdit = Boolean(reminder)
  const [input, setInput] = useState<ReminderInput>(defaultInput())
  const [saving, setSaving] = useState(false)
  const [channelOpen, setChannelOpen] = useState(false)
  const [channels, setChannels] = useState<Channel[]>([])
  const { toast } = useToast()

  useEffect(() => {
    listChannels()
      .then(setChannels)
      .catch(() => setChannels([]))
  }, [])

  useEffect(() => {
    if (reminder) {
      setInput({
        title: reminder.title,
        content: reminder.content,
        calendar: reminder.calendar,
        schedule_type: reminder.schedule_type,
        schedule_spec: reminder.schedule_spec,
        timezone: reminder.timezone || 'Asia/Shanghai',
        channel_ids: reminder.channel_ids,
        require_confirm: reminder.require_confirm,
        confirm_retry_interval_sec: reminder.confirm_retry_interval_sec || 300,
        confirm_max_retries: reminder.confirm_max_retries || 3,
      })
    } else {
      setInput(defaultInput())
    }
  }, [reminder])

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

  function toggleChannel(id: number) {
    setInput((prev) => {
      const has = prev.channel_ids.includes(id)
      return { ...prev, channel_ids: has ? prev.channel_ids.filter((x) => x !== id) : [...prev.channel_ids, id] }
    })
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!input.title.trim()) {
      toast({ title: '标题必填', variant: 'destructive' })
      return
    }
    if (input.channel_ids.length === 0) {
      toast({ title: '至少选择一个通道', variant: 'destructive' })
      return
    }
    setSaving(true)
    try {
      if (isEdit && reminder) {
        await updateReminder(reminder.id, input)
      } else {
        await createReminder(input)
      }
      toast({ title: isEdit ? '提醒已更新' : '提醒已创建', variant: 'success' })
      onSaved()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: { message?: string } } } })?.response?.data?.error?.message
      toast({ title: '保存失败', description: msg ?? String(err), variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl max-h-[95vh] overflow-visible">
        <DialogHeader>
          <DialogTitle>{isEdit ? '编辑提醒' : '新建提醒'}</DialogTitle>
          <DialogDescription>
            到达触发时间后会通过所选通道发送；内容支持 <code>{'{{var}}'}</code> 占位（如{' '}
            <code>{'{{now}}'}</code>、<code>{'{{lunar_date}}'}</code>）。
          </DialogDescription>
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
            <Label htmlFor="r-content">内容</Label>
            <Textarea
              id="r-content"
              value={input.content}
              onChange={(e) => patch('content', e.target.value)}
              rows={3}
              placeholder={'例如：今天是 {{now_date}}，{{lunar_date}}'}
            />
          </div>

          <ScheduleForm
            value={{
              calendar: input.calendar,
              schedule_type: input.schedule_type,
              schedule_spec: input.schedule_spec,
            }}
            onChange={patchSchedule}
          />

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-2">
              <Label>通道</Label>
              {channels.length === 0 ? (
                <p className="text-xs text-muted-foreground">还没有通道，先到「通知」页面创建一个。</p>
              ) : (
                <div className="relative">
                  <button
                    type="button"
                    onClick={() => setChannelOpen(!channelOpen)}
                    className="flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  >
                    <span className={`truncate ${input.channel_ids.length > 0 ? '' : 'text-muted-foreground'}`}>
                      {formatChannelNames(channels, input.channel_ids)}
                    </span>
                    <ChevronDown className="h-4 w-4 opacity-50 shrink-0 ml-1" />
                  </button>
                  {channelOpen && (
                    <>
                      <div className="fixed inset-0 z-40" onClick={() => setChannelOpen(false)} />
                      <div className="absolute bottom-full z-50 mb-1 max-h-64 w-full overflow-y-auto rounded-md border bg-card shadow-lg">
                        {channels.map((ch) => {
                          const checked = input.channel_ids.includes(ch.id)
                          return (
                            <label
                              key={ch.id}
                              className="flex cursor-pointer items-center gap-2 px-3 py-2 text-sm hover:bg-muted first:rounded-t-md last:rounded-b-md"
                            >
                              <input
                                type="checkbox"
                                checked={checked}
                                onChange={() => toggleChannel(ch.id)}
                                className="h-4 w-4"
                              />
                              <span>{ch.name}</span>
                              <span className="ml-auto text-xs text-muted-foreground">{ch.type}</span>
                            </label>
                          )
                        })}
                      </div>
                    </>
                  )}
                </div>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="r-tz">提醒时区</Label>
              <Select value={input.timezone || 'Asia/Shanghai'} onValueChange={(v) => patch('timezone', v)}>
                <SelectTrigger id="r-tz">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {COMMON_TIMEZONES.map((tz) => (
                    <SelectItem key={tz} value={tz}>
                      {tz}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="rounded-md border p-3">
            <div className="flex items-center justify-between">
              <div>
                <Label htmlFor="r-confirm" className="cursor-pointer">是否需要确认提醒</Label>
                <p className="text-xs text-muted-foreground mt-1">
                  消息会附带确认链接；未点击则按下方配置重发。
                </p>
              </div>
              <Switch
                id="r-confirm"
                checked={input.require_confirm}
                onCheckedChange={(v) => patch('require_confirm', v)}
              />
            </div>
            {input.require_confirm && (
              <div className="mt-3 grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="r-retry-interval">重发间隔（秒，≥60）</Label>
                  <Input
                    id="r-retry-interval"
                    type="number"
                    min={60}
                    value={input.confirm_retry_interval_sec}
                    onChange={(e) => patch('confirm_retry_interval_sec', Number(e.target.value) || 60)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="r-retry-max">最大重发次数（≥1）</Label>
                  <Input
                    id="r-retry-max"
                    type="number"
                    min={1}
                    value={input.confirm_max_retries}
                    onChange={(e) => patch('confirm_max_retries', Number(e.target.value) || 1)}
                  />
                </div>
              </div>
            )}
          </div>

          <DialogFooter>
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
