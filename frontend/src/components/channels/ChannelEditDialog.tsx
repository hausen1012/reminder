// 通知编辑对话框：新建时 type 可选；编辑时 type 锁死。
//
// 类型清单与字段定义来自后端 /api/channels/types。smtp / dingtalk / wecom / webhook
// 有联动逻辑，仍用 ChannelForms 里的定制表单；其余类型统一走通用表单。
import { useEffect, useRef, useState, type FormEvent } from 'react'
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
import { SMTPForm, DingTalkForm, WeComForm, WebhookForm } from './ChannelForms'
import { ChannelGenericForm } from './ChannelGenericForm'
import type { Channel, ChannelType, ChannelTypeSpec } from '@/types'
import { createChannel, extractApiError, testChannelDryRun, updateChannel } from '@/lib/api'
import { defaultConfigOf, findSpec, useChannelTypes } from '@/lib/channelSpec'

interface Props {
  channel: Channel | null
  open: boolean
  onClose: () => void
  onSaved: () => void
}

// 有定制表单的类型；其余一律走通用表单。
const CUSTOM_FORMS = {
  smtp: SMTPForm,
  dingtalk: DingTalkForm,
  wecom: WeComForm,
  webhook: WebhookForm,
} as const

// 定制表单的初始配置；含 object 类型字段（headers / query_template）的不便由字段描述推导。
const CUSTOM_DEFAULT_CONFIG: Record<string, Record<string, unknown>> = {
  smtp: {
    host: '',
    port: 587,
    username: '',
    password_enc: '',
    from_addr: '',
    from_name: '',
    to: [],
    security_mode: 'starttls',
  },
  dingtalk: { webhook_url: '', secret_enc: '' },
  wecom: { webhook_url: '' },
  webhook: {
    method: 'POST',
    url: '',
    headers: {},
    authorization_enc: '',
    body_template: '',
    query_template: {},
    content_type: 'application/json',
  },
}

// buildDefaultConfig 返回新建通道时的初始配置：定制表单用固定模板，其余按字段定义推导。
function buildDefaultConfig(
  type: ChannelType,
  specs: ChannelTypeSpec[],
): Record<string, unknown> {
  if (type in CUSTOM_DEFAULT_CONFIG) {
    return { ...CUSTOM_DEFAULT_CONFIG[type] }
  }
  return defaultConfigOf(findSpec(specs, type))
}

export function ChannelEditDialog({ channel, open, onClose, onSaved }: Props) {
  const { specs } = useChannelTypes()
  const isEdit = Boolean(channel)
  const initialType = channel?.type ?? 'smtp'
  // 类型元数据晚到时只影响下拉框渲染；重置表单要用最新元数据，但不能因此覆盖用户输入，
  // 所以放在 ref 里供重置逻辑读取。
  const specsRef = useRef<ChannelTypeSpec[]>(specs)
  specsRef.current = specs
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState<ChannelType>(initialType)
  const [config, setConfig] = useState<Record<string, unknown>>(
    channel?.config ?? buildDefaultConfig(initialType, specs),
  )
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const { toast } = useToast()

  // 只在切换编辑对象时重置表单；类型元数据晚到不应覆盖用户已输入的内容。
  useEffect(() => {
    if (channel) {
      setName(channel.name)
      setType(channel.type)
      setConfig(channel.config ?? buildDefaultConfig(channel.type, specsRef.current))
    } else {
      setName('')
      setType('smtp')
      setConfig(buildDefaultConfig('smtp', specsRef.current))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channel])

  function handleTypeChange(next: ChannelType) {
    setType(next)
    setConfig(buildDefaultConfig(next, specs))
  }

  const spec = findSpec(specs, type)
  const CustomForm = CUSTOM_FORMS[type as keyof typeof CUSTOM_FORMS]

  async function handleTest() {
    setTesting(true)
    try {
      await testChannelDryRun({
        id: channel?.id ?? 0,
        type,
        config,
      })
      toast({ title: '测试通知已发送', description: '请检查通知渠道是否收到消息。', variant: 'success' })
    } catch (err) {
      toast({ title: '测试失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setTesting(false)
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!name.trim()) {
      toast({ title: '渠道名称必填', variant: 'destructive' })
      return
    }
    setSaving(true)
    try {
      if (isEdit && channel) {
        await updateChannel(channel.id, { name, type, config })
      } else {
        await createChannel({ name, type, config })
      }
      toast({ title: isEdit ? '通知渠道已更新' : '通知渠道已创建', variant: 'success' })
      onSaved()
    } catch (err: unknown) {
      toast({ title: '保存失败', description: extractApiError(err), variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl md:max-h-[90dvh] md:overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEdit ? '编辑通知渠道' : '新建通知渠道'}</DialogTitle>
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
            <Select value={type} onValueChange={handleTypeChange} disabled={isEdit}>
              <SelectTrigger id="channel-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {specs.map((opt) => (
                  <SelectItem key={opt.type} value={opt.type}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {isEdit && (
              <p className="text-xs text-muted-foreground">通知类型创建后不可修改。</p>
            )}
          </div>

          {CustomForm ? (
            <CustomForm config={config} onChange={setConfig} isEdit={isEdit} />
          ) : spec ? (
            <ChannelGenericForm spec={spec} config={config} onChange={setConfig} isEdit={isEdit} />
          ) : null}

          <DialogFooter>
            <Button type="button" variant="secondary" onClick={handleTest} disabled={testing} title="向该通知发送一条测试消息">
              {testing ? '试发中…' : '测试通知'}
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
