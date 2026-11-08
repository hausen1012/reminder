// 通用通道表单：断言完全由字段描述驱动，覆盖各控件类型与敏感字段占位符语义。
import { describe, expect, it } from 'vitest'
import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChannelGenericForm } from '@/components/channels/ChannelGenericForm'
import type { ChannelTypeSpec } from '@/types'

const SPEC: ChannelTypeSpec = {
  type: 'demo',
  label: '演示通道',
  description: '演示用',
  max_body_len: 0,
  fields: [
    { key: 'name', label: '名称', kind: 'text', required: true, placeholder: '请输入名称' },
    { key: 'token_enc', label: '令牌', kind: 'password', required: true, placeholder: '请输入令牌' },
    { key: 'port', label: '端口', kind: 'number', required: false, default: 80 },
    { key: 'enabled', label: '启用', kind: 'switch', required: false },
    { key: 'mode', label: '模式', kind: 'select', required: false, options: [
      { value: 'a', label: '模式 A' },
      { value: 'b', label: '模式 B' },
    ] },
    { key: 'tags', label: '标签', kind: 'list', required: false },
    { key: 'note', label: '备注', kind: 'textarea', required: false },
  ],
}

function Controlled({ initial, isEdit }: { initial: Record<string, unknown>; isEdit: boolean }) {
  const [config, setConfig] = useState<Record<string, unknown>>(initial)
  return (
    <div>
      <ChannelGenericForm spec={SPEC} config={config} onChange={setConfig} isEdit={isEdit} />
      <pre data-testid="state">{JSON.stringify(config)}</pre>
    </div>
  )
}

function readState(): Record<string, unknown> {
  return JSON.parse(screen.getByTestId('state').textContent ?? '{}')
}

describe('ChannelGenericForm', () => {
  it('按字段描述渲染标签与控件', () => {
    render(<Controlled initial={{}} isEdit={false} />)
    expect(screen.getByLabelText('名称')).toBeInTheDocument()
    expect(screen.getByLabelText('端口')).toHaveAttribute('type', 'number')
    expect(screen.getByLabelText('启用')).toBeInTheDocument()
    expect(screen.getByLabelText('备注').tagName).toBe('TEXTAREA')
  })

  it('文本字段写入 config', async () => {
    const user = userEvent.setup()
    render(<Controlled initial={{}} isEdit={false} />)
    await user.type(screen.getByLabelText('名称'), '值班')
    expect(readState().name).toBe('值班')
  })

  it('列表字段按逗号切分为数组', async () => {
    const user = userEvent.setup()
    render(<Controlled initial={{}} isEdit={false} />)
    await user.type(screen.getByLabelText('标签'), 'a, b')
    expect(readState().tags).toEqual(['a', 'b'])
  })

  it('数组字段回显为逗号拼接文本', () => {
    render(<Controlled initial={{ tags: ['x', 'y'] }} isEdit={false} />)
    expect(screen.getByLabelText('标签')).toHaveValue('x, y')
  })

  it('开关字段写入布尔值', async () => {
    const user = userEvent.setup()
    render(<Controlled initial={{}} isEdit={false} />)
    await user.click(screen.getByLabelText('启用'))
    expect(readState().enabled).toBe(true)
  })

  it('编辑态敏感字段回显为空并提示留空不修改', () => {
    render(<Controlled initial={{ token_enc: '***' }} isEdit />)
    const input = screen.getByLabelText('令牌')
    expect(input).toHaveValue('')
    expect(input).toHaveAttribute('placeholder', '留空表示不修改')
  })

  it('编辑态未改动敏感字段时提交占位符', async () => {
    const user = userEvent.setup()
    render(<Controlled initial={{ token_enc: '***' }} isEdit />)
    await user.type(screen.getByLabelText('名称'), 'x')
    expect(readState().token_enc).toBe('***')
  })

  it('编辑态输入新密钥后写入明文', async () => {
    const user = userEvent.setup()
    render(<Controlled initial={{ token_enc: '***' }} isEdit />)
    await user.type(screen.getByLabelText('令牌'), 'new-secret')
    expect(readState().token_enc).toBe('new-secret')
  })

  it('编辑态清空已输入的新密钥后回到占位符', async () => {
    const user = userEvent.setup()
    render(<Controlled initial={{ token_enc: '***' }} isEdit />)
    const input = screen.getByLabelText('令牌')
    await user.type(input, 'abc')
    await user.clear(input)
    expect(readState().token_enc).toBe('***')
  })
})
