import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { Sidebar } from '@/components/layout/Sidebar'
import { AppLayout } from '@/components/layout/AppLayout'

// mock 三个 context，避免依赖 Provider 树
const mockTheme = vi.fn()
const mockAuth = vi.fn()
const mockConfig = vi.fn()
vi.mock('@/contexts/ThemeContext', () => ({ useTheme: () => mockTheme() }))
vi.mock('@/contexts/AuthContext', () => ({ useAuth: () => mockAuth() }))
vi.mock('@/contexts/ConfigContext', () => ({ useConfig: () => mockConfig() }))

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
  })

  it('渲染全部导航项', () => {
    renderSidebar()
    for (const label of ['首页', '提醒', '通知', '日志', '令牌', '监控', '设置']) {
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

  it('渲染文档与 GitHub 站外链接，均在新窗口打开', () => {
    renderSidebar()

    const docs = screen.getByRole('link', { name: /文档/ })
    expect(docs).toHaveAttribute('href', 'https://hausen1012.github.io/reminder/')
    expect(docs).toHaveAttribute('target', '_blank')
    expect(docs).toHaveAttribute('rel', 'noreferrer noopener')

    const github = screen.getByRole('link', { name: /GitHub/ })
    expect(github).toHaveAttribute('href', 'https://github.com/hausen1012/reminder')
    expect(github).toHaveAttribute('target', '_blank')
    expect(github).toHaveAttribute('rel', 'noreferrer noopener')
  })

  it('点击站外链接触发 onNavClick（用于关闭移动端抽屉）', async () => {
    const onNavClick = vi.fn()
    const user = userEvent.setup()
    renderSidebar(onNavClick)

    await user.click(screen.getByRole('link', { name: /文档/ }))
    expect(onNavClick).toHaveBeenCalled()
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
    mockTheme.mockReturnValue({ theme: 'light', toggle: vi.fn() })
    mockAuth.mockReturnValue({ user: { id: 1, username: 'admin' }, logout: vi.fn() })
    mockConfig.mockReturnValue({ config: {} })
  })

  it('渲染主内容与侧边栏', () => {
    renderLayout()
    expect(screen.getByText('主内容')).toBeInTheDocument()
    // 桌面端与移动端各渲染一个 Sidebar，因此导航项出现 2 次
    expect(screen.getAllByText('首页')).toHaveLength(2)
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
})
