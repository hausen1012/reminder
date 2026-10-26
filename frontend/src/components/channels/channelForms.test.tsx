import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { WeComForm } from '@/components/channels/WeComForm'
import { LogForm } from '@/components/channels/LogForm'

// 受控包装：模拟父组件维护 config 状态
function ControlledWeCom({ initial = {} }: { initial?: Record<string, unknown> }) {
  const [config, setConfig] = useState<Record<string, unknown>>(initial)
  return (
    <div>
      <WeComForm config={config} onChange={setConfig} isEdit={false} />
      <pre data-testid="state">{JSON.stringify(config)}</pre>
    </div>
  )
}

describe('WeComForm', () => {
  it('渲染 Webhook URL 输入框与消息类型选择器', () => {
    render(<ControlledWeCom />)
    expect(
      screen.getByPlaceholderText('https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...'),
    ).toBeInTheDocument()
    expect(screen.getByText('Webhook URL')).toBeInTheDocument()
    expect(screen.getByText('消息类型')).toBeInTheDocument()
  })

  it('输入 webhook_url 应写入 config', async () => {
    const user = userEvent.setup()
    render(<ControlledWeCom />)

    await user.type(screen.getByRole('textbox'), 'https://example.test/hook')
    expect(screen.getByTestId('state')).toHaveTextContent('https://example.test/hook')
  })

  it('已有配置应回显到输入框', () => {
    render(<ControlledWeCom initial={{ webhook_url: 'https://existing.test' }} />)
    expect(screen.getByRole('textbox')).toHaveValue('https://existing.test')
  })

  it('msg_type 缺省时选择器显示默认 text', () => {
    render(<ControlledWeCom />)
    // Radix Select 会把当前值渲染到 trigger 中
    expect(screen.getByText('文本')).toBeInTheDocument()
  })
})

describe('LogForm', () => {
  it('渲染说明文案，不包含任何输入控件', () => {
    render(<LogForm config={{}} onChange={vi.fn()} isEdit={false} />)
    expect(screen.getByText(/日志通知无需额外配置/)).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })
})
