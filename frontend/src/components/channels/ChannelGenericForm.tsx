// 由后端类型元数据驱动的通用通道表单。
//
// smtp / dingtalk / wecom / webhook 仍用 ChannelForms.tsx 里的定制表单（有联动逻辑），
// 其余类型（含后续新增的）统一走这里，新增通道类型不需要再写前端表单。
//
// 敏感字段（kind=password，键名以 _enc 结尾）沿用与定制表单一致的占位符约定：
// 编辑态回显 "***"，输入框留空提交 "***" 表示保持原密文。
import { useRef, useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { ChannelFieldSpec, ChannelTypeSpec } from '@/types'
import type { SubFormProps } from './ChannelForms'
import { updateField } from './ChannelForms'

export function ChannelGenericForm({ spec, config, onChange, isEdit }: SubFormProps & { spec: ChannelTypeSpec }) {
  return (
    <div className="space-y-3 rounded-md border bg-card/30 p-4">
      {spec.description && (
        <p className="text-xs text-muted-foreground">{spec.description}</p>
      )}
      {spec.fields.map((field) => (
        <FieldRow
          key={field.key}
          field={field}
          config={config}
          onChange={onChange}
          isEdit={isEdit}
        />
      ))}
    </div>
  )
}

function FieldRow({
  field,
  config,
  onChange,
  isEdit,
}: {
  field: ChannelFieldSpec
  config: Record<string, unknown>
  onChange: SubFormProps['onChange']
  isEdit: boolean
}) {
  const value = config[field.key]

  if (field.kind === 'switch') {
    return (
      <div className="flex items-center justify-between gap-3">
        <div className="space-y-0.5">
          <Label htmlFor={`field-${field.key}`}>{field.label}</Label>
          {field.help && <p className="text-xs text-muted-foreground">{field.help}</p>}
        </div>
        <Switch
          id={`field-${field.key}`}
          checked={value === true}
          onCheckedChange={(checked) => updateField(onChange, field.key, checked)}
        />
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <Label htmlFor={`field-${field.key}`}>{field.label}</Label>
      {renderControl(field, value, config, onChange, isEdit)}
      {field.help && <p className="text-xs text-muted-foreground">{field.help}</p>}
    </div>
  )
}

function renderControl(
  field: ChannelFieldSpec,
  value: unknown,
  config: Record<string, unknown>,
  onChange: SubFormProps['onChange'],
  isEdit: boolean,
) {
  const id = `field-${field.key}`

  switch (field.kind) {
    case 'password':
      return (
        <SecretInput
          id={id}
          field={field}
          config={config}
          onChange={onChange}
          isEdit={isEdit}
        />
      )
    case 'number':
      return (
        <Input
          id={id}
          type="number"
          placeholder={field.placeholder}
          value={typeof value === 'number' ? value : Number(value ?? 0) || ''}
          onChange={(e) => updateField(onChange, field.key, Number(e.target.value))}
        />
      )
    case 'textarea':
      return (
        <Textarea
          id={id}
          placeholder={field.placeholder}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => updateField(onChange, field.key, e.target.value)}
        />
      )
    case 'select':
      return (
        <Select
          value={typeof value === 'string' ? value : String(field.default ?? '')}
          onValueChange={(v) => updateField(onChange, field.key, v)}
        >
          <SelectTrigger id={id}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(field.options ?? []).map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )
    case 'list':
      return (
        <ListInput id={id} field={field} value={value} onChange={onChange} />
      )
    default:
      return (
        <Input
          id={id}
          placeholder={field.placeholder}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => updateField(onChange, field.key, e.target.value)}
        />
      )
  }
}

// SecretInput 处理敏感字段的占位符语义。
//
// 初始值是否为 "***" 用 ref 锁定：用户一旦输入，config 里的值就变成新密钥，
// 若实时判断会把「清空输入框」误判成「保持原值」。
function SecretInput({
  id,
  field,
  config,
  onChange,
  isEdit,
}: {
  id: string
  field: ChannelFieldSpec
  config: Record<string, unknown>
  onChange: SubFormProps['onChange']
  isEdit: boolean
}) {
  const raw = config[field.key]
  const hasOriginalRef = useRef(raw === '***')
  const hasOriginal = hasOriginalRef.current
  const [input, setInput] = useState(hasOriginal ? '' : typeof raw === 'string' ? raw : '')

  return (
    <Input
      id={id}
      type="password"
      placeholder={isEdit && hasOriginal ? '留空表示不修改' : field.placeholder}
      value={input}
      onChange={(e) => {
        const v = e.target.value
        setInput(v)
        if (isEdit && hasOriginal && v === '') {
          updateField(onChange, field.key, '***')
        } else {
          updateField(onChange, field.key, v)
        }
      }}
    />
  )
}

// ListInput 维护逗号分隔文本的本地状态。
//
// 不能直接由数组值反推输入框内容：输入 "a, " 时数组会变成 ["a"]，
// 回显又丢掉逗号，导致无法连续输入多个值。
function ListInput({
  id,
  field,
  value,
  onChange,
}: {
  id: string
  field: ChannelFieldSpec
  value: unknown
  onChange: SubFormProps['onChange']
}) {
  const [text, setText] = useState(() => listToText(value))
  return (
    <Input
      id={id}
      placeholder={field.placeholder}
      value={text}
      onChange={(e) => {
        const v = e.target.value
        setText(v)
        updateField(onChange, field.key, textToList(v))
      }}
    />
  )
}

// 列表中若含逗号则按逗号分隔拼接，避免回显后再保存时被拆成多段。
function listToText(value: unknown): string {
  if (Array.isArray(value)) return value.map((v) => String(v)).join(', ')
  if (typeof value === 'string') return value
  return ''
}

function textToList(text: string): string[] {
  return text
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}
