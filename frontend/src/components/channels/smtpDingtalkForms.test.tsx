import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { SMTPForm } from '@/components/channels/SMTPForm'
import { DingTalkForm } from '@/components/channels/DingTalkForm'

// 受控包装：父组件维护 config 状态，便于断言字段写入
function ControlledSMTP({
  initial = {},
  isEdit = false,
}: {
  initial?: Record<string, unknown>
  isEdit?: boolean
}) {
  const [config, setConfig] = useState<Record<string, unknown>>(initial)
  return (
    <div>
      <SMTPForm config={config} onChange={setConfig} isEdit={isEdit} />
      <pre data-testid="state">{JSON.stringify(config)}</pre>
    </div>
  )
}

function ControlledDingTalk({
  initial = {},
  isEdit = false,
}: {
  initial?: Record<string, unknown>
  isEdit?: boolean
}) {
  const [config, setConfig] = useState<Record<string, unknown>>(initial)
  return (
    <div>
      <DingTalkForm config={config} onChange={setConfig} isEdit={isEdit} />
      <pre data-testid="state">{JSON.stringify(config)}</pre>
    </div>
  )
}

describe('SMTPForm', () => {
  it('渲染全部字段标签', () => {
    render(<ControlledSMTP />)
    for (const label of ['SMTP Host', '端口', '连接方式', '用户名', '密码 / 授权码', '发件人地址', '发件人显示名', '收件人']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })

  it('输入 host 应写入 config', async () => {
    const user = userEvent.setup()
    render(<ControlledSMTP />)

    await user.type(screen.getByPlaceholderText('smtp.example.com'), 'smtp.test')
    expect(screen.getByTestId('state')).toHaveTextContent('smtp.test')
  })

  it('已有配置应回显', () => {
    render(
      <ControlledSMTP
        initial={{ host: 'smtp.old.test', port: 465, username: 'me@test', from_addr: 'f@test' }}
      />,
    )
    expect(screen.getByPlaceholderText('smtp.example.com')).toHaveValue('smtp.old.test')
    expect(screen.getByPlaceholderText('登录用户名')).toHaveValue('me@test')
  })

  it('收件人输入按逗号拆分并去空', async () => {
    const user = userEvent.setup()
    render(<ControlledSMTP />)

    await user.type(screen.getByPlaceholderText(/多个地址用英文逗号分隔/), 'a@x.com, b@x.com')
    const state = JSON.parse(screen.getByTestId('state').textContent ?? '{}')
    expect(state.to).toEqual(['a@x.com', 'b@x.com'])
  })

  it('编辑态且已有密码时，密码框占位提示"留空表示不修改"', () => {
    render(<ControlledSMTP initial={{ password_enc: '***' }} isEdit />)
    expect(screen.getByPlaceholderText('留空表示不修改')).toBeInTheDocument()
  })

  it('新建态密码框使用普通占位', () => {
    render(<ControlledSMTP />)
    expect(screen.getByPlaceholderText('密码或授权码')).toBeInTheDocument()
  })

  it('编辑态清空密码应回写 *** 占位符', async () => {
    const user = userEvent.setup()
    render(<ControlledSMTP initial={{ password_enc: '***' }} isEdit />)

    const pwd = screen.getByPlaceholderText('留空表示不修改')
    await user.type(pwd, 'x')
    await user.clear(pwd)

    const state = JSON.parse(screen.getByTestId('state').textContent ?? '{}')
    expect(state.password_enc).toBe('***')
  })

  it('未指定 security_mode 时默认为 starttls', () => {
    render(<ControlledSMTP />)
    expect(screen.getByText('推荐：加密连接（常见邮箱）')).toBeInTheDocument()
  })

  it('显式 implicit_tls 时显示对应连接方式', () => {
    render(<ControlledSMTP initial={{ security_mode: 'implicit_tls' }} />)
    expect(screen.getByText('465 专用加密连接')).toBeInTheDocument()
  })

  it('显式 plain 时显示对应连接方式', () => {
    render(<ControlledSMTP initial={{ security_mode: 'plain' }} />)
    expect(screen.getByText('不加密连接（仅特殊服务器）')).toBeInTheDocument()
  })
})

describe('DingTalkForm', () => {
  it('渲染 Webhook URL 与加签密钥', () => {
    render(<ControlledDingTalk />)
    expect(screen.getByText('Webhook URL')).toBeInTheDocument()
    expect(screen.getByText('加签密钥（可选）')).toBeInTheDocument()
  })

  it('输入 webhook_url 应写入 config', async () => {
    const user = userEvent.setup()
    render(<ControlledDingTalk />)

    await user.type(
      screen.getByPlaceholderText('https://oapi.dingtalk.com/robot/send?access_token=...'),
      'https://hook.test',
    )
    expect(screen.getByTestId('state')).toHaveTextContent('https://hook.test')
  })

  it('编辑态已有加签密钥时显示"留空表示不修改"', () => {
    render(<ControlledDingTalk initial={{ secret_enc: '***' }} isEdit />)
    expect(screen.getByPlaceholderText(/留空表示不修改/)).toBeInTheDocument()
  })

  it('新建态加签密钥占位为"机器人安全设置 - 加签"', () => {
    render(<ControlledDingTalk />)
    expect(screen.getByPlaceholderText('机器人安全设置 - 加签')).toBeInTheDocument()
  })

  it('编辑态清空加签密钥应回写 *** 占位符', async () => {
    const user = userEvent.setup()
    render(<ControlledDingTalk initial={{ secret_enc: '***' }} isEdit />)

    const input = screen.getByPlaceholderText(/留空表示不修改/)
    await user.type(input, 's')
    await user.clear(input)

    const state = JSON.parse(screen.getByTestId('state').textContent ?? '{}')
    expect(state.secret_enc).toBe('***')
  })
})
