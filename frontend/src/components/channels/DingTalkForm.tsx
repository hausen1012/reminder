// 钉钉机器人通道表单
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
import { type SubFormProps, updateField } from './form-utils'

export function DingTalkForm({ config, onChange, isEdit }: SubFormProps) {
  const hasOriginalSecret = config.secret_enc === '***'
  const [secretInput, setSecretInput] = useState(hasOriginalSecret ? '' : (config.secret_enc as string ?? ''))

  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      <div className="space-y-2">
        <Label>Webhook URL</Label>
        <Input
          placeholder="https://oapi.dingtalk.com/robot/send?access_token=..."
          value={(config.webhook_url as string) ?? ''}
          onChange={(e) => updateField(onChange, 'webhook_url', e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label>加签密钥（可选）</Label>
        <Input
          type="password"
          placeholder={isEdit && hasOriginalSecret ? '留空表示不修改；清空请输入空格' : '机器人安全设置 - 加签'}
          value={secretInput}
          onChange={(e) => {
            const v = e.target.value
            setSecretInput(v)
            if (isEdit && hasOriginalSecret && v === '') {
              updateField(onChange, 'secret_enc', '***')
            } else {
              updateField(onChange, 'secret_enc', v)
            }
          }}
        />
      </div>
      <div className="space-y-2">
        <Label>消息类型</Label>
        <Select
          value={(config.msg_type as string) ?? 'text'}
          onValueChange={(v) => updateField(onChange, 'msg_type', v)}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="text">文本（text）</SelectItem>
            <SelectItem value="markdown">Markdown</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}