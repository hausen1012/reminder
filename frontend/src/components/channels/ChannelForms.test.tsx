// 通道表单测试：定制表单共用一个受控包装，断言全部基于用户可见的文案/占位符。
import { describe, expect, it } from 'vitest'
import type { ComponentType } from 'react'
import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  cfgArr,
  cfgNum,
  cfgObj,
  cfgStr,
  SMTPForm,
  DingTalkForm,
  WeComForm,
  WebhookForm,
  type SubFormProps,
} from '@/components/channels/ChannelForms'

// 受控包装：父组件维护 config 状态，并把当前状态挂到 DOM 上便于断言字段写入
function Controlled({
  form: Form,
  initial = {},
  isEdit = false,
}: {
  form: ComponentType<SubFormProps>
  initial?: Record<string, unknown>
  isEdit?: boolean
}) {
  const [config, setConfig] = useState<Record<string, unknown>>(initial)
  return (
    <div>
      <Form config={config} onChange={setConfig} isEdit={isEdit} />
      <pre data-testid="state">{JSON.stringify(config)}</pre>
    </div>
  )
}

// 读取包装层挂出的当前 config
function readState(): Record<string, unknown> {
  return JSON.parse(screen.getByTestId('state').textContent ?? '{}')
}

// --- 配置读取工具 ---

describe('cfgStr', () => {
  it('返回字符串值', () => {
    expect(cfgStr({ a: 'x' }, 'a')).toBe('x')
  })

  it('非字符串时返回 fallback', () => {
    expect(cfgStr({ a: 1 }, 'a')).toBe('')
    expect(cfgStr({ a: null }, 'a', 'def')).toBe('def')
    expect(cfgStr({}, 'missing', 'def')).toBe('def')
  })
})

describe('cfgNum', () => {
  it('返回数字值', () => {
    expect(cfgNum({ p: 587 }, 'p', 25)).toBe(587)
  })

  it('非数字时返回 fallback', () => {
    expect(cfgNum({ p: '587' }, 'p', 25)).toBe(25)
    expect(cfgNum({ p: null }, 'p', 25)).toBe(25)
    expect(cfgNum({}, 'p', 25)).toBe(25)
  })

  it('数字 0 应被保留（不是 fallback）', () => {
    expect(cfgNum({ p: 0 }, 'p', 25)).toBe(0)
  })
})

describe('cfgArr', () => {
  it('返回数组', () => {
    expect(cfgArr<string>({ to: ['a', 'b'] }, 'to')).toEqual(['a', 'b'])
  })

  it('非数组时返回空数组', () => {
    expect(cfgArr({ to: 'a' }, 'to')).toEqual([])
    expect(cfgArr({}, 'to')).toEqual([])
  })

  it('支持自定义 fallback', () => {
    expect(cfgArr({}, 'to', ['x'])).toEqual(['x'])
  })

  it('空数组应原样返回（不被 fallback 覆盖）', () => {
    expect(cfgArr({ to: [] }, 'to', ['x'])).toEqual([])
  })
})

describe('cfgObj', () => {
  it('返回对象', () => {
    expect(cfgObj({ h: { a: '1' } }, 'h')).toEqual({ a: '1' })
  })

  it('数组不算对象', () => {
    expect(cfgObj({ h: ['a'] }, 'h')).toEqual({})
  })

  it('null / 非对象返回 fallback', () => {
    expect(cfgObj({ h: null }, 'h')).toEqual({})
    expect(cfgObj({ h: 'str' }, 'h')).toEqual({})
    expect(cfgObj({}, 'h', { d: 'v' })).toEqual({ d: 'v' })
  })
})

// --- SMTP ---

describe('SMTPForm', () => {
  it('渲染全部字段标签', () => {
    render(<Controlled form={SMTPForm} />)
    for (const label of ['SMTP Host', '端口', '连接方式', '用户名', '密码 / 授权码', '发件人地址', '发件人显示名', '收件人']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })

  it('输入 host 应写入 config', async () => {
    const user = userEvent.setup()
    render(<Controlled form={SMTPForm} />)

    await user.type(screen.getByPlaceholderText('smtp.example.com'), 'smtp.test')
    expect(screen.getByTestId('state')).toHaveTextContent('smtp.test')
  })

  it('已有配置应回显', () => {
    render(
      <Controlled
        form={SMTPForm}
        initial={{ host: 'smtp.old.test', port: 465, username: 'me@test', from_addr: 'f@test' }}
      />,
    )
    expect(screen.getByPlaceholderText('smtp.example.com')).toHaveValue('smtp.old.test')
    expect(screen.getByPlaceholderText('登录用户名')).toHaveValue('me@test')
  })

  it('收件人输入按逗号拆分并去空', async () => {
    const user = userEvent.setup()
    render(<Controlled form={SMTPForm} />)

    await user.type(screen.getByPlaceholderText(/多个地址用英文逗号分隔/), 'a@x.com, b@x.com')
    expect(readState().to).toEqual(['a@x.com', 'b@x.com'])
  })

  it('编辑态且已有密码时，密码框占位提示"留空表示不修改"', () => {
    render(<Controlled form={SMTPForm} initial={{ password_enc: '***' }} isEdit />)
    expect(screen.getByPlaceholderText('留空表示不修改')).toBeInTheDocument()
  })

  it('新建态密码框使用普通占位', () => {
    render(<Controlled form={SMTPForm} />)
    expect(screen.getByPlaceholderText('密码或授权码')).toBeInTheDocument()
  })

  it('编辑态清空密码应回写 *** 占位符', async () => {
    const user = userEvent.setup()
    render(<Controlled form={SMTPForm} initial={{ password_enc: '***' }} isEdit />)

    const pwd = screen.getByPlaceholderText('留空表示不修改')
    await user.type(pwd, 'x')
    await user.clear(pwd)

    expect(readState().password_enc).toBe('***')
  })

  it('未指定 security_mode 时默认为 starttls', () => {
    render(<Controlled form={SMTPForm} />)
    expect(screen.getByText('推荐：加密连接（常见邮箱）')).toBeInTheDocument()
  })

  it('显式 implicit_tls 时显示对应连接方式', () => {
    render(<Controlled form={SMTPForm} initial={{ security_mode: 'implicit_tls' }} />)
    expect(screen.getByText('465 专用加密连接')).toBeInTheDocument()
  })

  it('显式 plain 时显示对应连接方式', () => {
    render(<Controlled form={SMTPForm} initial={{ security_mode: 'plain' }} />)
    expect(screen.getByText('不加密连接（仅特殊服务器）')).toBeInTheDocument()
  })
})

// --- 钉钉 ---

describe('DingTalkForm', () => {
  it('渲染 Webhook URL 与加签密钥', () => {
    render(<Controlled form={DingTalkForm} />)
    expect(screen.getByText('Webhook URL')).toBeInTheDocument()
    expect(screen.getByText('加签密钥（可选）')).toBeInTheDocument()
  })

  it('输入 webhook_url 应写入 config', async () => {
    const user = userEvent.setup()
    render(<Controlled form={DingTalkForm} />)

    await user.type(
      screen.getByPlaceholderText('https://oapi.dingtalk.com/robot/send?access_token=...'),
      'https://hook.test',
    )
    expect(screen.getByTestId('state')).toHaveTextContent('https://hook.test')
  })

  it('编辑态已有加签密钥时显示"留空表示不修改"', () => {
    render(<Controlled form={DingTalkForm} initial={{ secret_enc: '***' }} isEdit />)
    expect(screen.getByPlaceholderText(/留空表示不修改/)).toBeInTheDocument()
  })

  it('新建态加签密钥占位为"机器人安全设置 - 加签"', () => {
    render(<Controlled form={DingTalkForm} />)
    expect(screen.getByPlaceholderText('机器人安全设置 - 加签')).toBeInTheDocument()
  })

  it('编辑态清空加签密钥应回写 *** 占位符', async () => {
    const user = userEvent.setup()
    render(<Controlled form={DingTalkForm} initial={{ secret_enc: '***' }} isEdit />)

    const input = screen.getByPlaceholderText(/留空表示不修改/)
    await user.type(input, 's')
    await user.clear(input)

    expect(readState().secret_enc).toBe('***')
  })
})

// --- 企业微信 ---

describe('WeComForm', () => {
  it('渲染 Webhook URL 输入框', () => {
    render(<Controlled form={WeComForm} />)
    expect(
      screen.getByPlaceholderText('https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...'),
    ).toBeInTheDocument()
    expect(screen.getByText('Webhook URL')).toBeInTheDocument()
  })

  it('输入 webhook_url 应写入 config', async () => {
    const user = userEvent.setup()
    render(<Controlled form={WeComForm} />)

    await user.type(screen.getByRole('textbox'), 'https://example.test/hook')
    expect(screen.getByTestId('state')).toHaveTextContent('https://example.test/hook')
  })

  it('已有配置应回显到输入框', () => {
    render(<Controlled form={WeComForm} initial={{ webhook_url: 'https://existing.test' }} />)
    expect(screen.getByRole('textbox')).toHaveValue('https://existing.test')
  })
})

// --- Webhook ---

describe('WebhookForm', () => {
  it('默认 POST 时显示 Content-Type 与 Body 模板', () => {
    render(<Controlled form={WebhookForm} />)
    expect(screen.getByText('请求方法')).toBeInTheDocument()
    expect(screen.getByText('Content-Type')).toBeInTheDocument()
    expect(screen.getByText(/Body 模板/)).toBeInTheDocument()
    // GET 专属字段不显示
    expect(screen.queryByText(/Query 模板/)).not.toBeInTheDocument()
  })

  it('GET 方法时显示 Query 模板且隐藏 Body 模板', () => {
    render(<Controlled form={WebhookForm} initial={{ method: 'GET' }} />)
    expect(screen.getByText(/Query 模板/)).toBeInTheDocument()
    expect(screen.queryByText(/Body 模板/)).not.toBeInTheDocument()
    expect(screen.queryByText('Content-Type')).not.toBeInTheDocument()
  })

  it('输入 URL 应写入 config', async () => {
    const user = userEvent.setup()
    render(<Controlled form={WebhookForm} />)

    await user.type(screen.getByPlaceholderText('https://example.com/notify'), 'https://hook.test')
    expect(readState().url).toBe('https://hook.test')
  })

  it('headers 多行文本应解析为对象', async () => {
    const user = userEvent.setup()
    render(<Controlled form={WebhookForm} />)

    const ta = screen.getByPlaceholderText(/X-Source: reminder/)
    await user.click(ta)
    await user.paste('X-A: 1\nX-B: 2')

    expect(readState().headers).toEqual({ 'X-A': '1', 'X-B': '2' })
  })

  it('无冒号的行应被忽略', async () => {
    const user = userEvent.setup()
    render(<Controlled form={WebhookForm} />)

    const ta = screen.getByPlaceholderText(/X-Source: reminder/)
    await user.click(ta)
    await user.paste('no-colon-line\nX-A: 1')

    expect(readState().headers).toEqual({ 'X-A': '1' })
  })

  it('GET 的 query_template 也应解析为对象', async () => {
    const user = userEvent.setup()
    render(<Controlled form={WebhookForm} initial={{ method: 'GET' }} />)

    const ta = screen.getByPlaceholderText(/text: \{\{title\}\}/)
    await user.click(ta)
    await user.paste('tag: reminder')

    expect(readState().query_template).toEqual({ tag: 'reminder' })
  })

  it('编辑态已有 Authorization 时占位提示"留空表示不修改"', () => {
    render(<Controlled form={WebhookForm} initial={{ authorization_enc: '***' }} isEdit />)
    expect(screen.getByPlaceholderText('留空表示不修改')).toBeInTheDocument()
  })

  it('编辑态清空 Authorization 应回写 *** 占位符', async () => {
    const user = userEvent.setup()
    render(<Controlled form={WebhookForm} initial={{ authorization_enc: '***' }} isEdit />)

    const input = screen.getByPlaceholderText('留空表示不修改')
    await user.type(input, 'Bearer x')
    await user.clear(input)

    expect(readState().authorization_enc).toBe('***')
  })

  it('新建态 Authorization 占位为示例值', () => {
    render(<Controlled form={WebhookForm} />)
    expect(screen.getByPlaceholderText('例如：Bearer xxx 或 Basic xxx')).toBeInTheDocument()
  })

  it('body_template 输入应写入 config', async () => {
    const user = userEvent.setup()
    render(<Controlled form={WebhookForm} />)

    const ta = screen.getByPlaceholderText(/"title": "\{\{title\}\}"/)
    await user.click(ta)
    await user.paste('{"t":"{{title}}"}')

    expect(readState().body_template).toBe('{"t":"{{title}}"}')
  })
})
