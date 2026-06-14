// 通知编辑对话框：根据 Type 切换四套表单。
// 新建时 type 可选；编辑时 type 锁死。
import { useEffect, useMemo, useState, type FormEvent } from 'react'
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
import { useToast } from '@/components/ui/use-toast'
import { SMTPForm } from './SMTPForm'
import { DingTalkForm } from './DingTalkForm'
import { WeComForm } from './WeComForm'
import { WebhookForm } from './WebhookForm'
import { LogForm } from './LogForm'
import type { Channel, ChannelType } from '@/types'
import { createChannel, testChannel, updateChannel } from '@/lib/api'

interface Props {
  channel: Channel | null
  open: boolean
  onClose: () => void
  onSaved: () => void
}

const TYPE_OPTIONS: { value: ChannelType; label: string }[] = [
  { value: 'smtp', label: '邮件 SMTP' },
  { value: 'dingtalk', label: '钉钉机器人' },
  { value: 'wecom', label: '企业微信机器人' },
  { value: 'webhook', label: '通用 Webhook' },
  { value: 'log', label: '日志输出' },
]

// 各类型默认 Config 模板，新建时初始化使用。
function defaultConfig(type: ChannelType): Record<string, unknown> {
  switch (type) {
    case 'smtp':
      return {
        host: '',
        port: 587,
        username: '',
        password_enc: '',
        from_addr: '',
        from_name: '',
        to: [],
        use_starttls: true,
        security_mode: 'starttls',
      }
    case 'dingtalk':
      return { webhook_url: '', secret_enc: '', msg_type: 'text' }
    case 'wecom':
      return { webhook_url: '', msg_type: 'text' }
    case 'webhook':
      return {
        method: 'POST',
        url: '',
        headers: {},
        authorization_enc: '',
        body_template: '',
        query_template: {},
        content_type: 'application/json',
      }
    case 'log':
      return {}
  }
}

export function ChannelEditDialog({ channel, open, onClose, onSaved }: Props) {
  const isEdit = Boolean(channel)
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState<ChannelType>(channel?.type ?? 'smtp')
  const [config, setConfig] = useState<Record<string, unknown>>(channel?.config ?? defaultConfig(channel?.type ?? 'smtp'))
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const { toast } = useToast()

  useEffect(() => {
    if (channel) {
      setName(channel.name)
      setType(channel.type)
      setConfig(channel.config ?? defaultConfig(channel.type))
    } else {
      setName('')
      setType('smtp')
      setConfig(defaultConfig('smtp'))
    }
  }, [channel])

  function handleTypeChange(next: ChannelType) {
    setType(next)
    setConfig(defaultConfig(next))
  }

  const SubForm = useMemo(() => {
    switch (type) {
      case 'smtp':
        return SMTPForm
      case 'dingtalk':
        return DingTalkForm
      case 'wecom':
        return WeComForm
      case 'webhook':
        return WebhookForm
      case 'log':
        return LogForm
    }
  }, [type])

  async function handleTest() {
    if (!channel) return
    setTesting(true)
    try {
      const result = await testChannel(channel.id)
      if (result.success) {
        toast({ title: '试发成功', description: `通知 ${channel.name} 已成功发送`, variant: 'success' })
      } else {
        toast({ title: '试发失败', description: result.error ?? '未知错误', variant: 'destructive' })
      }
    } catch (err) {
      toast({ title: '试发请求异常', description: String(err), variant: 'destructive' })
    } finally {
      setTesting(false)
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!name.trim()) {
      toast({ title: '通知名称必填', variant: 'destructive' })
      return
    }
    setSaving(true)
    try {
      if (isEdit && channel) {
        await updateChannel(channel.id, { name, type, config })
      } else {
        await createChannel({ name, type, config })
      }
      toast({ title: isEdit ? '通知已更新' : '通知已创建', variant: 'success' })
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
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEdit ? '编辑通知' : '新建通知'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="channel-name">名称</Label>
            <Input
              id="channel-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={64}
              placeholder="例如：值班邮箱、运维群机器人"
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="channel-type">类型</Label>
            <Select value={type} onValueChange={(v) => handleTypeChange(v as ChannelType)} disabled={isEdit}>
              <SelectTrigger id="channel-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TYPE_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {isEdit && (
              <p className="text-xs text-muted-foreground">通知类型创建后不可修改。</p>
            )}
          </div>

          <SubForm config={config} onChange={setConfig} isEdit={isEdit} />

          <DialogFooter>
            {isEdit && (
              <Button type="button" variant="secondary" onClick={handleTest} disabled={testing} title="向该通知发送一条测试消息">
                {testing ? '试发中…' : '测试通知'}
              </Button>
            )}
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
