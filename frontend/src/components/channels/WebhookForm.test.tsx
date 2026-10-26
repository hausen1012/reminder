import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { WebhookForm } from '@/components/channels/WebhookForm'

function ControlledWebhook({
  initial = {},
  isEdit = false,
}: {
  initial?: Record<string, unknown>
  isEdit?: boolean
}) {
  const [config, setConfig] = useState<Record<string, unknown>>(initial)
  return (
    <div>
      <WebhookForm config={config} onChange={setConfig} isEdit={isEdit} />
      <pre data-testid="state">{JSON.stringify(config)}</pre>
    </div>
  )
}

function readState(): Record<string, unknown> {
  return JSON.parse(screen.getByTestId('state').textContent ?? '{}')
}

describe('WebhookForm', () => {
  it('默认 POST 时显示 Content-Type 与 Body 模板', () => {
    render(<ControlledWebhook />)
    expect(screen.getByText('请求方法')).toBeInTheDocument()
    expect(screen.getByText('Content-Type')).toBeInTheDocument()
    expect(screen.getByText(/Body 模板/)).toBeInTheDocument()
    // GET 专属字段不显示
    expect(screen.queryByText(/Query 模板/)).not.toBeInTheDocument()
  })

  it('GET 方法时显示 Query 模板且隐藏 Body 模板', () => {
    render(<ControlledWebhook initial={{ method: 'GET' }} />)
    expect(screen.getByText(/Query 模板/)).toBeInTheDocument()
    expect(screen.queryByText(/Body 模板/)).not.toBeInTheDocument()
    expect(screen.queryByText('Content-Type')).not.toBeInTheDocument()
  })

  it('输入 URL 应写入 config', async () => {
    const user = userEvent.setup()
    render(<ControlledWebhook />)

    await user.type(screen.getByPlaceholderText('https://example.com/notify'), 'https://hook.test')
    expect(readState().url).toBe('https://hook.test')
  })

  it('headers 多行文本应解析为对象', async () => {
    const user = userEvent.setup()
    render(<ControlledWebhook />)

    const ta = screen.getByPlaceholderText(/X-Source: reminder/)
    await user.click(ta)
    await user.paste('X-A: 1\nX-B: 2')

    expect(readState().headers).toEqual({ 'X-A': '1', 'X-B': '2' })
  })

  it('无冒号的行应被忽略', async () => {
    const user = userEvent.setup()
    render(<ControlledWebhook />)

    const ta = screen.getByPlaceholderText(/X-Source: reminder/)
    await user.click(ta)
    await user.paste('no-colon-line\nX-A: 1')

    expect(readState().headers).toEqual({ 'X-A': '1' })
  })

  it('GET 的 query_template 也应解析为对象', async () => {
    const user = userEvent.setup()
    render(<ControlledWebhook initial={{ method: 'GET' }} />)

    const ta = screen.getByPlaceholderText(/text: \{\{title\}\}/)
    await user.click(ta)
    await user.paste('tag: reminder')

    expect(readState().query_template).toEqual({ tag: 'reminder' })
  })

  it('编辑态已有 Authorization 时占位提示"留空表示不修改"', () => {
    render(<ControlledWebhook initial={{ authorization_enc: '***' }} isEdit />)
    expect(screen.getByPlaceholderText('留空表示不修改')).toBeInTheDocument()
  })

  it('编辑态清空 Authorization 应回写 *** 占位符', async () => {
    const user = userEvent.setup()
    render(<ControlledWebhook initial={{ authorization_enc: '***' }} isEdit />)

    const input = screen.getByPlaceholderText('留空表示不修改')
    await user.type(input, 'Bearer x')
    await user.clear(input)

    expect(readState().authorization_enc).toBe('***')
  })

  it('新建态 Authorization 占位为示例值', () => {
    render(<ControlledWebhook />)
    expect(screen.getByPlaceholderText('例如：Bearer xxx 或 Basic xxx')).toBeInTheDocument()
  })

  it('body_template 输入应写入 config', async () => {
    const user = userEvent.setup()
    render(<ControlledWebhook />)

    const ta = screen.getByPlaceholderText(/"title": "\{\{title\}\}"/)
    await user.click(ta)
    await user.paste('{"t":"{{title}}"}')

    expect(readState().body_template).toBe('{"t":"{{title}}"}')
  })
})
