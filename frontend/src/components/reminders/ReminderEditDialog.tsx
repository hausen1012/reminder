// ReminderEditDialog 是提醒的新建 / 编辑表单。
import { useEffect, useState, type FormEvent } from 'react'
import { Info } from 'lucide-react'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { useToast } from '@/components/ui/use-toast'
import { ChannelMultiSelect } from '@/components/channels/ChannelMultiSelect'
import { ScheduleForm, type ScheduleValue } from './ScheduleForm'
import { createReminder, extractApiError, listChannels, testReminderDryRun, updateReminder } from '@/lib/api'
import type { Channel, ContentFormat, Reminder, ReminderInput } from '@/types'

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
    content_format: 'text',
    calendar: 'solar',
    schedule_type: 'once',
    schedule_spec: { at: '' },
    timezone: 'Asia/Shanghai',
    source: 'web',
    channel_ids: [],
    require_confirm: false,
    confirm_retry_interval_sec: 300,
    confirm_max_retries: 3,
  }
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
  const [testing, setTesting] = useState(false)
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
        content_format: (reminder.content_format as ContentFormat) || 'text',
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
      toast({ title: '测试失败', description: String(err), variant: 'destructive' })
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
      if (isEdit && reminder) {
        await updateReminder(reminder.id, input)
      } else {
        await createReminder(input)
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
        className="max-w-2xl max-h-[90vh] overflow-y-auto"
        onPointerDownOutside={(e) => {
          // CalendarPopover 通过 createPortal(document.body) 渲染，不在 Dialog DOM 子树内，
          // Radix Dialog 的 DismissableLayer 会拦截其 pointerdown 并阻止 click 产生。
          // 检测到点击来自日历弹窗时解除拦截，其他外部点击仍正常关闭 Dialog。
          const target = e.detail.originalEvent.target as Element | null
          if (target?.closest('[data-calendar-popover]')) {
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
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5">
                <Label htmlFor="r-content">内容</Label>
                <div className="group relative inline-flex">
                  <button
                    type="button"
                    className="rounded-sm p-0.5 text-muted-foreground transition-colors hover:text-foreground"
                    aria-label="查看可用占位符"
                  >
                    <Info className="h-3.5 w-3.5" />
                  </button>
                  <div className="pointer-events-none absolute left-full top-1/2 z-50 ml-2 hidden w-64 -translate-y-1/2 rounded-md border bg-popover p-3 text-xs text-popover-foreground shadow-md group-hover:block">
                    <div className="space-y-1">
                      <p className="font-medium">可用占位符</p>
                      <p><code>{'{{now}}'}</code> 当前日期时间</p>
                      <p><code>{'{{now_date}}'}</code> 当前日期</p>
                      <p><code>{'{{lunar_date}}'}</code> 当前农历日期</p>
                    </div>
                  </div>
                </div>
              </div>
              <Tabs
                value={input.content_format || 'text'}
                onValueChange={(v) => patch('content_format', v as ContentFormat)}
                className="w-auto"
              >
                <TabsList className="h-8">
                  <TabsTrigger value="text" className="text-xs px-3">纯文本</TabsTrigger>
                  <TabsTrigger value="markdown" className="text-xs px-3">Markdown</TabsTrigger>
                  <TabsTrigger value="html" className="text-xs px-3">HTML</TabsTrigger>
                </TabsList>
              </Tabs>
            </div>
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

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div className="space-y-2">
              <Label>通知</Label>
              <ChannelMultiSelect
                channels={channels}
                value={input.channel_ids}
                open={channelOpen}
                onOpenChange={setChannelOpen}
                onChange={(channelIds) => patch('channel_ids', channelIds)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="r-tz">时区</Label>
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
                <Label htmlFor="r-confirm" className="cursor-pointer">确认提醒</Label>
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
              <div className="mt-3 grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="r-retry-interval">重试间隔（秒）</Label>
                  <Input
                    id="r-retry-interval"
                    type="number"
                    min={60}
                    value={input.confirm_retry_interval_sec}
                    onChange={(e) => patch('confirm_retry_interval_sec', Number(e.target.value) || 60)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="r-retry-max">最大重试次数</Label>
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
