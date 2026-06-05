// SMTP 通道表单
import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { type SubFormProps, updateField } from './form-utils'

export function SMTPForm({ config, onChange, isEdit }: SubFormProps) {
  const [toText, setToText] = useState(((config.to as string[] | undefined) ?? []).join(', '))
  const hasOriginalSecret = config.password_enc === '***'
  const [pwdInput, setPwdInput] = useState(hasOriginalSecret ? '' : (config.password_enc as string ?? ''))

  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label>SMTP Host</Label>
          <Input
            placeholder="smtp.example.com"
            value={(config.host as string) ?? ''}
            onChange={(e) => updateField(onChange, 'host', e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>端口</Label>
          <Input
            type="number"
            placeholder="465"
            value={(config.port as number) ?? ''}
            onChange={(e) => updateField(onChange, 'port', Number(e.target.value))}
          />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label>用户名</Label>
          <Input
            placeholder="登录用户名"
            value={(config.username as string) ?? ''}
            onChange={(e) => updateField(onChange, 'username', e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>密码 / 授权码</Label>
          <Input
            type="password"
            placeholder={isEdit && hasOriginalSecret ? '留空表示不修改' : '密码或授权码'}
            value={pwdInput}
            onChange={(e) => {
              const v = e.target.value
              setPwdInput(v)
              // 留空 + 原本有密文 → 占位符
              if (isEdit && hasOriginalSecret && v === '') {
                updateField(onChange, 'password_enc', '***')
              } else {
                updateField(onChange, 'password_enc', v)
              }
            }}
          />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label>发件人地址</Label>
          <Input
            placeholder="sender@example.com"
            value={(config.from_addr as string) ?? ''}
            onChange={(e) => updateField(onChange, 'from_addr', e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>发件人显示名</Label>
          <Input
            placeholder="提醒助手"
            value={(config.from_name as string) ?? ''}
            onChange={(e) => updateField(onChange, 'from_name', e.target.value)}
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label>收件人</Label>
        <Input
          placeholder="多个地址用英文逗号分隔：a@x.com, b@x.com"
          value={toText}
          onChange={(e) => {
            const v = e.target.value
            setToText(v)
            const arr = v.split(',').map((s) => s.trim()).filter(Boolean)
            updateField(onChange, 'to', arr)
          }}
        />
      </div>
      <div className="flex items-center gap-3">
        <Switch
          checked={Boolean(config.use_starttls)}
          onCheckedChange={(v) => updateField(onChange, 'use_starttls', v)}
          id="use-starttls"
        />
        <Label htmlFor="use-starttls" className="text-sm cursor-pointer">
          启用 STARTTLS（587 端口需要打开）
        </Label>
      </div>
    </div>
  )
}
