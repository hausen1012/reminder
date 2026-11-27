// ChannelProxyField 组件测试：验证模式决议、全局回显、直连与自定义代理非空校验。
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  ChannelProxyField,
  detectProxyMode,
} from '@/components/channels/ChannelProxyField'

describe('detectProxyMode', () => {
  it('空串与空白字符判定为 global', () => {
    expect(detectProxyMode('')).toBe('global')
    expect(detectProxyMode('   ')).toBe('global')
  })

  it('direct 不区分大小写判定为 direct', () => {
    expect(detectProxyMode('direct')).toBe('direct')
    expect(detectProxyMode('DIRECT')).toBe('direct')
    expect(detectProxyMode(' Direct ')).toBe('direct')
  })

  it('常规代理 URL 判定为 custom', () => {
    expect(detectProxyMode('http://127.0.0.1:7890')).toBe('custom')
    expect(detectProxyMode('socks5://localhost:1080')).toBe('custom')
  })
})

describe('ChannelProxyField 渲染与选项文案', () => {
  it('默认或空值时展示系统全局代理，带全局回显地址', () => {
    render(<ChannelProxyField value="" onChange={vi.fn()} globalProxy="http://10.0.0.1:7890" />)
    expect(screen.getByText('系统全局代理 (http://10.0.0.1:7890)')).toBeInTheDocument()
    expect(screen.queryByLabelText('自定义代理地址')).not.toBeInTheDocument()
  })

  it('全局未配置代理时直接展示系统全局代理', () => {
    render(<ChannelProxyField value="" onChange={vi.fn()} />)
    expect(screen.getByText('系统全局代理')).toBeInTheDocument()
    expect(screen.queryByLabelText('自定义代理地址')).not.toBeInTheDocument()
  })

  it('值为 direct 时选中直连', () => {
    render(<ChannelProxyField value="direct" onChange={vi.fn()} />)
    expect(screen.getByText('直连')).toBeInTheDocument()
    expect(screen.queryByLabelText('自定义代理地址')).not.toBeInTheDocument()
  })

  it('自定义代理地址时展示输入框并回显数值', () => {
    render(<ChannelProxyField value="http://127.0.0.1:8888" onChange={vi.fn()} />)
    expect(screen.getByText('自定义代理')).toBeInTheDocument()
    const input = screen.getByLabelText('自定义代理地址') as HTMLInputElement
    expect(input).toBeInTheDocument()
    expect(input.value).toBe('http://127.0.0.1:8888')
    expect(input).toBeRequired()
  })
})

describe('ChannelProxyField 交互与非空校验', () => {
  it('输入自定义代理地址时派发 onChange', async () => {
    const user = userEvent.setup()
    const handleChange = vi.fn()
    render(<ChannelProxyField value="http://init" onChange={handleChange} />)

    const input = screen.getByLabelText('自定义代理地址')
    await user.type(input, ':9090')
    expect(handleChange).toHaveBeenCalled()
  })

  it('切换下拉选项为直连时派发 direct', async () => {
    const user = userEvent.setup()
    const handleChange = vi.fn()
    render(<ChannelProxyField value="" onChange={handleChange} />)

    const trigger = screen.getByRole('combobox')
    await user.click(trigger)

    const directOption = await screen.findByRole('option', { name: '直连' })
    await user.click(directOption)

    expect(handleChange).toHaveBeenCalledWith('direct')
  })

  it('选择自定义代理且输入为空时触发校验失败与错误提示', async () => {
    const user = userEvent.setup()
    const handleValidity = vi.fn()
    const { rerender } = render(
      <ChannelProxyField value="" onChange={vi.fn()} onValidityChange={handleValidity} />,
    )

    // 切换到自定义代理
    const trigger = screen.getByRole('combobox')
    await user.click(trigger)
    const customOption = await screen.findByRole('option', { name: '自定义代理' })
    await user.click(customOption)

    // 输入框展开且为空，校验应通知 false
    expect(handleValidity).toHaveBeenCalledWith(false)
    const input = screen.getByLabelText('自定义代理地址')
    expect(input).toBeRequired()

    // 触发 showError 或失焦时应显示不能为空提示
    rerender(
      <ChannelProxyField
        value=""
        onChange={vi.fn()}
        onValidityChange={handleValidity}
        showError={true}
      />,
    )
    expect(screen.getByText('自定义代理地址不能为空')).toBeInTheDocument()

    // 输入有效地址后应恢复合法
    await user.type(input, 'http://127.0.0.1:7890')
    expect(handleValidity).toHaveBeenCalledWith(true)
  })
})
