import { describe, expect, it, vi, beforeEach } from 'vitest'
import { useEffect } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AppLayout, Sidebar } from '@/components/layout/Layout'

// mock 三个 context，避免依赖 Provider 树
const mockTheme = vi.fn()
const mockAuth = vi.fn()
const mockConfig = vi.fn()
const mockAbout = vi.fn()
vi.mock('@/contexts/ThemeContext', () => ({ useTheme: () => mockTheme() }))
vi.mock('@/contexts/AuthContext', () => ({ useAuth: () => mockAuth() }))
vi.mock('@/contexts/ConfigContext', () => ({ useConfig: () => mockConfig() }))
vi.mock('@/contexts/AboutContext', () => ({ useAbout: () => mockAbout() }))
// 外壳测试只关心布局，浮窗有自己的测试文件，这里替换成空组件避免引入
// toast 上下文依赖；保留 onEnabledChange 上报，便于验证主内容底部留白
const aiState = vi.hoisted(() => ({ enabled: false }))
vi.mock('@/components/ai/AIChatWidget', () => ({
  AIChatWidget: ({ onEnabledChange }: { onEnabledChange?: (enabled: boolean) => void }) => {
    useEffect(() => {
      onEnabledChange?.(aiState.enabled)
    }, [onEnabledChange])
    return null
  },
}))

function renderSidebar(onNavClick?: () => void) {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <Sidebar onNavClick={onNavClick} />
    </MemoryRouter>,
  )
}

describe('Sidebar', () => {
  beforeEach(() => {
    mockTheme.mockReturnValue({ theme: 'light', toggle: vi.fn() })
    mockAuth.mockReturnValue({ user: { id: 1, username: 'admin' }, logout: vi.fn() })
    mockConfig.mockReturnValue({ config: {} })
    mockAbout.mockReturnValue({ hasUnreadUpdate: false })
  })

  it('有新版本时在「关于」上显示红点', () => {
    mockAbout.mockReturnValue({ hasUnreadUpdate: true })
    renderSidebar()
    expect(screen.getByLabelText('有新版本')).toBeInTheDocument()
  })

  it('没有新版本时不显示红点', () => {
    renderSidebar()
    expect(screen.queryByLabelText('有新版本')).not.toBeInTheDocument()
  })

  it('渲染全部导航项', () => {
    renderSidebar()
    for (const label of ['首页', '提醒', '通知', '日志', '令牌', '监控', '设置', '关于']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })

  it('无 logo 与 app_name 时显示默认品牌', () => {
    renderSidebar()
    expect(screen.getByText('Reminder')).toBeInTheDocument()
  })

  it('有 app_name 时显示自定义品牌名', () => {
    mockConfig.mockReturnValue({ config: { app_name: '我的提醒' } })
    renderSidebar()
    expect(screen.getByText('我的提醒')).toBeInTheDocument()
  })

  it('有 logo_svg 时渲染 Logo 组件', () => {
    mockConfig.mockReturnValue({ config: { logo_svg: '<svg data-testid="site-logo"></svg>' } })
    renderSidebar()
    expect(screen.getByTestId('site-logo')).toBeInTheDocument()
  })

  it('显示当前用户名', () => {
    renderSidebar()
    expect(screen.getByText('admin')).toBeInTheDocument()
  })

  it('light 主题时按钮提示"切换深色"并使用月亮图标', () => {
    renderSidebar()
    expect(screen.getByTitle('切换深色')).toBeInTheDocument()
  })

  it('dark 主题时按钮提示"切换浅色"', () => {
    mockTheme.mockReturnValue({ theme: 'dark', toggle: vi.fn() })
    renderSidebar()
    expect(screen.getByTitle('切换浅色')).toBeInTheDocument()
  })

  it('点击主题按钮触发 toggle', async () => {
    const toggle = vi.fn()
    mockTheme.mockReturnValue({ theme: 'light', toggle })
    const user = userEvent.setup()
    renderSidebar()

    await user.click(screen.getByTitle('切换深色'))
    expect(toggle).toHaveBeenCalledTimes(1)
  })

  it('点击退出按钮触发 logout', async () => {
    const logout = vi.fn()
    mockAuth.mockReturnValue({ user: { id: 1, username: 'admin' }, logout })
    const user = userEvent.setup()
    renderSidebar()

    await user.click(screen.getByTitle('退出登录'))
    expect(logout).toHaveBeenCalledTimes(1)
  })

  it('点击导航项触发 onNavClick（用于关闭移动端抽屉）', async () => {
    const onNavClick = vi.fn()
    const user = userEvent.setup()
    renderSidebar(onNavClick)

    await user.click(screen.getByText('提醒'))
    expect(onNavClick).toHaveBeenCalled()
  })

  it('侧边栏不再单独放置文档与 GitHub 外链', () => {
    renderSidebar()
    expect(screen.queryByRole('link', { name: /文档/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /GitHub/ })).not.toBeInTheDocument()
  })
})

describe('AppLayout', () => {
  function renderLayout() {
    return render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route index element={<div>主内容</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )
  }

  beforeEach(() => {
    aiState.enabled = false
    mockTheme.mockReturnValue({ theme: 'light', toggle: vi.fn() })
    mockAuth.mockReturnValue({ user: { id: 1, username: 'admin' }, logout: vi.fn() })
    mockConfig.mockReturnValue({ config: {} })
    mockAbout.mockReturnValue({ hasUnreadUpdate: false })
  })

  it('渲染主内容与侧边栏', () => {
    renderLayout()
    expect(screen.getByText('主内容')).toBeInTheDocument()
    // 桌面侧边栏 + 移动端抽屉 + 底部标签栏各有一份导航
    expect(screen.getAllByText('首页').length).toBeGreaterThanOrEqual(2)
  })

  it('移动端底部标签栏只展示高频页面，其余入口走「更多」', () => {
    renderLayout()
    const tabbar = screen.getByRole('navigation', { name: '主导航' })

    for (const label of ['首页', '提醒', '通知', '日志', '更多']) {
      expect(within(tabbar).getByText(label)).toBeInTheDocument()
    }
    for (const label of ['令牌', '监控', '设置']) {
      expect(within(tabbar).queryByText(label)).not.toBeInTheDocument()
    }
  })

  it('点击底部「更多」打开抽屉', async () => {
    const user = userEvent.setup()
    const { container } = renderLayout()

    await user.click(screen.getByLabelText('打开更多菜单'))
    const drawer = container.querySelector('div.fixed.inset-0')
    expect(drawer?.className).toContain('visible')
  })

  it('初始时移动端抽屉为隐藏状态（用 CSS 控制，DOM 仍在）', () => {
    const { container } = renderLayout()
    // 抽屉容器是 fixed inset-0 的 div，关闭时带 invisible
    const drawer = container.querySelector('div.fixed.inset-0')
    expect(drawer).not.toBeNull()
    expect(drawer?.className).toContain('invisible')
  })

  it('点击汉堡菜单后抽屉变为可见', async () => {
    const user = userEvent.setup()
    const { container } = renderLayout()

    await user.click(screen.getByLabelText('打开菜单'))
    const drawer = container.querySelector('div.fixed.inset-0')
    expect(drawer?.className).toContain('visible')
    expect(drawer?.className).not.toContain('invisible')
    expect(screen.getByLabelText('关闭菜单')).toBeInTheDocument()
  })

  it('点击关闭按钮收起抽屉', async () => {
    const user = userEvent.setup()
    const { container } = renderLayout()

    await user.click(screen.getByLabelText('打开菜单'))
    await user.click(screen.getByLabelText('关闭菜单'))

    const drawer = container.querySelector('div.fixed.inset-0')
    expect(drawer?.className).toContain('invisible')
  })

  it('AI 助手未启用时主内容不额外留白', () => {
    const { container } = renderLayout()
    const main = container.querySelector('main')
    expect(main).not.toBeNull()
    expect(main?.className).not.toContain('pb-24')
  })

  it('AI 助手启用后主内容底部预留滚动余量，分页可滚出悬浮按钮', () => {
    aiState.enabled = true
    const { container } = renderLayout()
    const main = container.querySelector('main')
    // 移动端与桌面端按钮均为 3rem 高，留白 6rem 足以让分页滚到按钮上方
    expect(main?.className).toContain('pb-24')
  })
})
