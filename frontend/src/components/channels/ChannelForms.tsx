// 定制通知渠道配置表单（SMTP、钉钉、企微、Webhook）。
// 敏感字段编辑态回显 "***"，留空提交保持原值。
import { useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

// --- 公共类型与小工具 ---

export interface SubFormProps {
  config: Record<string, unknown>
  onChange: Dispatch<SetStateAction<Record<string, unknown>>>
  isEdit: boolean
}

export function updateField<T>(
  onChange: SubFormProps['onChange'],
  key: string,
  value: T,
) {
  onChange((prev) => ({ ...prev, [key]: value }))
}

/** 从 config 中安全读取字符串字段 */
export function cfgStr(config: Record<string, unknown>, key: string, fallback = ''): string {
  const v = config[key]
  return typeof v === 'string' ? v : fallback
}

/** 从 config 中安全读取数字字段 */
export function cfgNum(config: Record<string, unknown>, key: string, fallback: number): number {
  const v = config[key]
  return typeof v === 'number' ? v : fallback
}

/** 从 config 中安全读取数组字段 */
export function cfgArr<T>(config: Record<string, unknown>, key: string, fallback: T[] = []): T[] {
  const v = config[key]
  return Array.isArray(v) ? v as T[] : fallback
}

/** 从 config 中安全读取对象字段 */
export function cfgObj(config: Record<string, unknown>, key: string, fallback: Record<string, string> = {}): Record<string, string> {
  const v = config[key]
  return v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, string> : fallback
}

// --- SMTP ---

type SMTPSecurityMode = 'plain' | 'starttls' | 'implicit_tls'

function toSMTPSecurityMode(v: string): SMTPSecurityMode {
  if (v === 'plain' || v === 'starttls' || v === 'implicit_tls') return v
  return 'starttls'
}

const CONNECTION_LABEL: Record<SMTPSecurityMode, string> = {
  starttls: '推荐：加密连接（常见邮箱）',
  implicit_tls: '465 专用加密连接',
  plain: '不加密连接（仅特殊服务器）',
}

function getSecurityMode(config: Record<string, unknown>): SMTPSecurityMode {
  return toSMTPSecurityMode(cfgStr(config, 'security_mode', 'starttls'))
}

export function SMTPForm({ config, onChange, isEdit }: SubFormProps) {
  const [toText, setToText] = useState(cfgArr<string>(config, 'to').join(', '))
  // 初始锁定原密码状态，避免输入后判定翻转
  const hasOriginalSecretRef = useRef(config.password_enc === '***')
  const hasOriginalSecret = hasOriginalSecretRef.current
  const [pwdInput, setPwdInput] = useState(hasOriginalSecret ? '' : cfgStr(config, 'password_enc'))
  const securityMode = getSecurityMode(config)

  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label>SMTP Host</Label>
          <Input
            placeholder="smtp.example.com"
            value={cfgStr(config, 'host')}
            onChange={(e) => updateField(onChange, 'host', e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>端口</Label>
          <Input
            type="number"
            placeholder="587"
            value={cfgNum(config, 'port', 587) || ''}
            onChange={(e) => updateField(onChange, 'port', Number(e.target.value))}
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label>连接方式</Label>
        <Select
          value={securityMode}
          onValueChange={(value) => {
            const mode = toSMTPSecurityMode(value)
            updateField(onChange, 'security_mode', mode)

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
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label>用户名</Label>
          <Input
            placeholder="登录用户名"
            value={cfgStr(config, 'username')}
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
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label>发件人地址</Label>
          <Input
            placeholder="sender@example.com"
            value={cfgStr(config, 'from_addr')}
            onChange={(e) => updateField(onChange, 'from_addr', e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>发件人显示名</Label>
          <Input
            placeholder="提醒助手"
            value={cfgStr(config, 'from_name')}
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

// --- 钉钉 ---

export function DingTalkForm({ config, onChange, isEdit }: SubFormProps) {
  // 同 SMTPForm：用 ref 锁定初始状态，避免用户输入后判定翻转。
  const hasOriginalSecretRef = useRef(config.secret_enc === '***')
  const hasOriginalSecret = hasOriginalSecretRef.current
  const [secretInput, setSecretInput] = useState(hasOriginalSecret ? '' : cfgStr(config, 'secret_enc'))

  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      <div className="space-y-2">
        <Label>Webhook URL</Label>
        <Input
          placeholder="https://oapi.dingtalk.com/robot/send?access_token=..."
          value={cfgStr(config, 'webhook_url')}
          onChange={(e) => updateField(onChange, 'webhook_url', e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label>加签密钥（可选）</Label>
        <Input
          type="password"
          placeholder={isEdit && hasOriginalSecret ? '留空表示不修改' : '机器人安全设置 - 加签'}
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
    </div>
  )
}

// --- 企业微信 ---

export function WeComForm({ config, onChange }: SubFormProps) {
  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      <div className="space-y-2">
        <Label>Webhook URL</Label>
        <Input
          placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."
          value={cfgStr(config, 'webhook_url')}
          onChange={(e) => updateField(onChange, 'webhook_url', e.target.value)}
        />
      </div>
    </div>
  )
}

// --- Webhook ---

// 把 Record<string, string> 转成 "key: value" 多行文本，便于编辑
function kvToText(obj: Record<string, string> | undefined): string {
  if (!obj) return ''
  return Object.entries(obj)
    .map(([k, v]) => `${k}: ${v}`)
    .join('\n')
}

function parseKV(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  text.split(/\r?\n/).forEach((line) => {
    const idx = line.indexOf(':')
    if (idx <= 0) return
    const k = line.slice(0, idx).trim()
    const v = line.slice(idx + 1).trim()
    if (k) out[k] = v
  })
  return out
}

export function WebhookForm({ config, onChange, isEdit }: SubFormProps) {
  const method = cfgStr(config, 'method', 'POST').toUpperCase()
  const [headersText, setHeadersText] = useState(kvToText(cfgObj(config, 'headers')))
  const [queryText, setQueryText] = useState(kvToText(cfgObj(config, 'query_template')))
  // 同 SMTPForm/DingTalkForm：用 ref 锁定初始状态，避免用户输入后判定翻转。
  const hasOriginalAuthRef = useRef(config.authorization_enc === '***')
  const hasOriginalAuth = hasOriginalAuthRef.current
  const [authInput, setAuthInput] = useState(hasOriginalAuth ? '' : cfgStr(config, 'authorization_enc'))

  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div className="space-y-2 sm:col-span-1">
          <Label>请求方法</Label>
          <Select value={method} onValueChange={(v) => updateField(onChange, 'method', v)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="GET">GET</SelectItem>
              <SelectItem value="POST">POST</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2 sm:col-span-2">
          <Label>URL</Label>
          <Input
            placeholder="https://example.com/notify"
            value={cfgStr(config, 'url')}
            onChange={(e) => updateField(onChange, 'url', e.target.value)}
          />
        </div>
      </div>

      <div className="space-y-2">
        <Label>Authorization（可选，加密存储）</Label>
        <Input
          type="password"
          placeholder={isEdit && hasOriginalAuth ? '留空表示不修改' : '例如：Bearer xxx 或 Basic xxx'}
          value={authInput}
          onChange={(e) => {
            const v = e.target.value
            setAuthInput(v)
            if (isEdit && hasOriginalAuth && v === '') {
              updateField(onChange, 'authorization_enc', '***')
            } else {
              updateField(onChange, 'authorization_enc', v)
            }
          }}
        />
      </div>

      <div className="space-y-2">
        <Label>自定义 Headers（每行一个，格式 key: value）</Label>
        <Textarea
          rows={3}
          placeholder={'X-Source: reminder\nX-Tag: prod'}
          value={headersText}
          onChange={(e) => {
            setHeadersText(e.target.value)
            updateField(onChange, 'headers', parseKV(e.target.value))
          }}
        />
      </div>

      {method === 'POST' && (
        <>
          <div className="space-y-2">
            <Label>Content-Type</Label>
            <Input
              placeholder="application/json; charset=utf-8"
              value={cfgStr(config, 'content_type', 'application/json')}
              onChange={(e) => updateField(onChange, 'content_type', e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>Body 模板（支持 {'{{title}}'} {'{{content}}'} 等占位）</Label>
            <Textarea
              rows={5}
              placeholder={'{\n  "title": "{{title}}",\n  "content": "{{content}}"\n}'}
              value={cfgStr(config, 'body_template')}
              onChange={(e) => updateField(onChange, 'body_template', e.target.value)}
            />
          </div>
        </>
      )}

      {method === 'GET' && (
        <div className="space-y-2">
          <Label>Query 模板（每行一个，格式 key: 值模板）</Label>
          <Textarea
            rows={3}
            placeholder={'text: {{title}} {{content}}\ntag: reminder'}
            value={queryText}
            onChange={(e) => {
              setQueryText(e.target.value)
              updateField(onChange, 'query_template', parseKV(e.target.value))
            }}
          />
        </div>
      )}
    </div>
  )
}
