// 企业微信机器人通知表单
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

export function WeComForm({ config, onChange }: SubFormProps) {
  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      <div className="space-y-2">
        <Label>Webhook URL</Label>
        <Input
          placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."
          value={(config.webhook_url as string) ?? ''}
          onChange={(e) => updateField(onChange, 'webhook_url', e.target.value)}
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
            <SelectItem value="text">文本</SelectItem>
            <SelectItem value="markdown">Markdown</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}
