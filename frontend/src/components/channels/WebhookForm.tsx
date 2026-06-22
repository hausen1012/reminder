// 通用 Webhook 通知表单
import { useState } from 'react'
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
import { type SubFormProps, updateField, cfgStr, cfgObj } from './form-utils'

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
  const hasOriginalAuth = config.authorization_enc === '***'
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