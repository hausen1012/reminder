import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from '@/contexts/ConfigContext'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import Profile from '@/pages/Profile'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    getAISettings: vi.fn(),
    getConfig: vi.fn(),
    updateConfig: vi.fn(),
    updatePassword: vi.fn(),
  }
})

import { getAISettings, getConfig, updateConfig } from '@/lib/api'

// Profile 依赖站点配置与 toast 上下文，这里用真实 Provider 包一层
function renderProfile() {
  return render(
    <ToastProviderRoot>
      <ConfigProvider>
        <Profile />
      </ConfigProvider>
    </ToastProviderRoot>,
  )
}

// 导航统一在 nav 属性为设置分区的容器内查找
function navButton(name: string) {
  const nav = screen.getByRole('navigation', { name: '设置分区' })
  return within(nav).getByRole('button', { name })
}

// 设置页三大分区：账户信息、基础设置、AI 助手
describe('Profile 分区导航', () => {
  beforeEach(() => {
    localStorage.setItem('token', 'test-token')
    vi.mocked(getConfig).mockResolvedValue({ timezone: 'Europe/Berlin', app_name: '提醒' })
    vi.mocked(updateConfig).mockImplementation(async (cfg) => cfg)
    vi.mocked(getAISettings).mockResolvedValue({
      enabled: false,
      provider: 'anthropic',
      base_url: '',
      model: 'claude-opus-5-5',
      api_key: '',
      custom_prompt: '',
      max_retries: 10,
    })
  })

  it('默认展示账户信息，其余分区默认隐藏', async () => {
    renderProfile()
    expect(navButton('账户信息')).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('heading', { name: '账户信息' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '基本信息' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '修改密码' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '基础设置' })).not.toBeInTheDocument()
    await waitFor(() => expect(vi.mocked(getConfig)).toHaveBeenCalled())
  })

  it('点击导航项切换到对应分区', async () => {
    renderProfile()

    await userEvent.click(navButton('基础设置'))
    expect(screen.getByRole('heading', { name: '基础设置' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '站点信息' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '时区设置' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '关于项目' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '账户信息' })).not.toBeInTheDocument()

    await userEvent.click(navButton('AI 助手'))
    expect(screen.getByRole('heading', { name: 'AI 助手' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '基础设置' })).not.toBeInTheDocument()
  })

  it('回显当前站点时区', async () => {
    renderProfile()
    await userEvent.click(navButton('基础设置'))
    // 下拉里同时存在隐藏的 option 与触发器文本，因此按触发器 id 断言
    await waitFor(() => {
      expect(document.getElementById('site-tz')).toHaveTextContent('Europe/Berlin')
    })
  })

  it('保存时区会提交 timezone 配置', async () => {
    renderProfile()
    await userEvent.click(navButton('基础设置'))
    await waitFor(() => {
      expect(document.getElementById('site-tz')).toHaveTextContent('Europe/Berlin')
    })

    const buttons = screen.getAllByRole('button', { name: '保存' })
    // 时区卡片中的保存按钮为第二个（第一个是站点信息的保存）
    await userEvent.click(buttons[1])

    await waitFor(() => {
      expect(vi.mocked(updateConfig)).toHaveBeenCalledWith({ timezone: 'Europe/Berlin' })
    })
  })
})
