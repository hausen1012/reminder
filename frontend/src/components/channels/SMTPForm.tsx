// SMTP 通道表单
import { useMemo, useState } from 'react'
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

type SMTPSecurityMode = 'plain' | 'starttls' | 'implicit_tls'

const CONNECTION_LABEL: Record<SMTPSecurityMode, string> = {
  starttls: '推荐：加密连接（常见邮箱）',
  implicit_tls: '465 专用加密连接',
  plain: '不加密连接（仅特殊服务器）',
}

function getSecurityMode(config: Record<string, unknown>): SMTPSecurityMode {
  const mode = config.security_mode as SMTPSecurityMode | undefined
  if (mode === 'plain' || mode === 'starttls' || mode === 'implicit_tls') return mode
  if (config.use_starttls) return 'starttls'
  if (config.port === 465) return 'implicit_tls'
  return 'plain'
}

function getConnectionHint(mode: SMTPSecurityMode): string {
  switch (mode) {
    case 'starttls':
      return '适合大多数邮箱服务，通常使用 587 端口。技术上等同于 STARTTLS。'
    case 'implicit_tls':
      return '如果服务商文档明确要求 465 端口，请选这个。技术上等同于 SMTPS / 直连 TLS。'
    case 'plain':
      return '仅在内网或旧服务器明确要求时使用。技术上等同于明文 SMTP。'
  }
}

function getPortHint(port: number): string {
  if (port === 587) return '当前端口通常对应推荐加密连接。'
  if (port === 465) return '当前端口通常对应 465 专用加密连接。'
  if (port === 25) return '当前端口通常对应不加密连接。'
  return '常见端口：587（推荐）、465（直连加密）、25（不加密）。'
}

export function SMTPForm({ config, onChange, isEdit }: SubFormProps) {
  const [toText, setToText] = useState(((config.to as string[] | undefined) ?? []).join(', '))
  const hasOriginalSecret = config.password_enc === '***'
  const [pwdInput, setPwdInput] = useState(hasOriginalSecret ? '' : (config.password_enc as string ?? ''))
  const securityMode = getSecurityMode(config)
  const port = Number(config.port ?? 0)
  const connectionHint = useMemo(() => getConnectionHint(securityMode), [securityMode])
  const portHint = useMemo(() => getPortHint(port), [port])

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
            placeholder="587"
            value={(config.port as number) ?? ''}
            onChange={(e) => updateField(onChange, 'port', Number(e.target.value))}
          />
          <p className="text-xs text-muted-foreground">{portHint}</p>
        </div>
      </div>
      <div className="space-y-2">
        <Label>连接方式</Label>
        <Select
          value={securityMode}
          onValueChange={(value) => {
            const mode = value as SMTPSecurityMode
            updateField(onChange, 'security_mode', mode)
            updateField(onChange, 'use_starttls', mode === 'starttls')

            const currentPort = Number(config.port ?? 0)
            if (mode === 'starttls' && [0, 25, 465].includes(currentPort)) {
              updateField(onChange, 'port', 587)
            }
            if (mode === 'implicit_tls' && [0, 25, 587].includes(currentPort)) {
              updateField(onChange, 'port', 465)
            }
          }}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="starttls">{CONNECTION_LABEL.starttls}</SelectItem>
            <SelectItem value="implicit_tls">{CONNECTION_LABEL.implicit_tls}</SelectItem>
            <SelectItem value="plain">{CONNECTION_LABEL.plain}</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">{connectionHint}</p>
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
    </div>
  )
}
