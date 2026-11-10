import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider } from '@/contexts/AuthContext'
import { ConfigProvider } from '@/contexts/ConfigContext'
import { AboutProvider } from '@/contexts/AboutContext'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import About from '@/pages/About'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    getConfig: vi.fn(),
    getMe: vi.fn(),
    getAboutInfo: vi.fn(),
    checkUpdate: vi.fn(),
    setUpdateCheckEnabled: vi.fn(),
  }
})

import { checkUpdate, getAboutInfo, getConfig, getMe, setUpdateCheckEnabled } from '@/lib/api'
import type { AboutInfo } from '@/types'

// 默认状态：已检查过、没有新版本
function aboutInfo(overrides: Partial<AboutInfo['update']> = {}): AboutInfo {
  return {
    version: 'v1.4.1',
    build_time: '2026-11-01T00:00:00Z',
    update: {
      enabled: true,
      comparable: true,
      latest_version: 'v1.4.1',
      release_url: 'https://github.com/hausen1012/reminder/releases/tag/v1.4.1',
      checked_at: '2026-11-02T00:00:00Z',
      error: '',
      update_available: false,
      ...overrides,
    },
  }
}

function renderAbout() {
  return render(
    <ToastProviderRoot>
      <AuthProvider>
        <ConfigProvider>
          <AboutProvider>
            <About />
          </AboutProvider>
        </ConfigProvider>
      </AuthProvider>
    </ToastProviderRoot>,
  )
}

// 关于页：版本信息、更新检查与项目地址。
describe('关于页', () => {
  beforeEach(() => {
    localStorage.setItem('token', 'test-token')
    // 已读标记跨用例共享同一个 localStorage，先清掉避免相互影响
    localStorage.removeItem('update_seen_version')
    vi.mocked(getMe).mockResolvedValue({
      code: 200,
      message: 'ok',
      data: { id: 1, username: 'admin', created_at: '' },
    })
    vi.mocked(getConfig).mockResolvedValue({ timezone: 'Asia/Shanghai' })
    vi.mocked(getAboutInfo).mockResolvedValue(aboutInfo())
    vi.mocked(checkUpdate).mockResolvedValue(aboutInfo())
    vi.mocked(setUpdateCheckEnabled).mockResolvedValue(aboutInfo({ enabled: false }))
  })

  it('展示当前版本与项目地址', async () => {
    renderAbout()

    // 当前版本与最新版本都是 v1.4.1，两处都应显示
    await waitFor(() => expect(screen.getAllByText('v1.4.1')).toHaveLength(2))
    expect(screen.getByRole('heading', { name: '关于项目' })).toBeInTheDocument()

    const github = screen.getByRole('link', { name: /github\.com\/hausen1012\/reminder/ })
    expect(github).toHaveAttribute('href', 'https://github.com/hausen1012/reminder')
    expect(github).toHaveAttribute('target', '_blank')
    expect(github).toHaveAttribute('rel', 'noreferrer noopener')

    const docs = screen.getByRole('link', { name: /hausen1012\.github\.io\/reminder/ })
    expect(docs).toHaveAttribute('href', 'https://hausen1012.github.io/reminder/')
    expect(docs).toHaveAttribute('target', '_blank')
    expect(docs).toHaveAttribute('rel', 'noreferrer noopener')
  })

  it('有新版本时展示版本号与发布链接', async () => {
    vi.mocked(getAboutInfo).mockResolvedValue(
      aboutInfo({
        latest_version: 'v1.5.0',
        release_url: 'https://github.com/hausen1012/reminder/releases/tag/v1.5.0',
        update_available: true,
      }),
    )
    renderAbout()

    await waitFor(() => expect(screen.getByText('有新版本')).toBeInTheDocument())
    expect(screen.getByText('v1.5.0')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /查看版本/ })).toHaveAttribute(
      'href',
      'https://github.com/hausen1012/reminder/releases/tag/v1.5.0',
    )
  })

  it('打开页面后把最新版本标记为已读', async () => {
    vi.mocked(getAboutInfo).mockResolvedValue(
      aboutInfo({ latest_version: 'v1.5.0', update_available: true }),
    )
    renderAbout()

    await waitFor(() => expect(localStorage.getItem('update_seen_version')).toBe('v1.5.0'))
  })

  it('点击立即检查会请求检查接口并刷新状态', async () => {
    vi.mocked(checkUpdate).mockResolvedValue(
      aboutInfo({ latest_version: 'v1.5.0', update_available: true }),
    )
    renderAbout()

    await userEvent.click(await screen.findByRole('button', { name: /立即检查/ }))

    await waitFor(() => expect(vi.mocked(checkUpdate)).toHaveBeenCalledTimes(1))
    expect(await screen.findByText('v1.5.0')).toBeInTheDocument()
  })

  it('关闭自动检查会提交开关状态', async () => {
    renderAbout()

    const toggle = await screen.findByRole('switch', { name: '每天自动检查更新' })
    await waitFor(() => expect(toggle).toBeChecked())

    await userEvent.click(toggle)
    await waitFor(() => expect(vi.mocked(setUpdateCheckEnabled)).toHaveBeenCalledWith(false))
  })

  it('检查失败时展示错误文案', async () => {
    vi.mocked(getAboutInfo).mockResolvedValue(aboutInfo({ error: '版本列表接口返回 HTTP 502' }))
    renderAbout()

    await waitFor(() =>
      expect(screen.getByText(/检查失败：版本列表接口返回 HTTP 502/)).toBeInTheDocument(),
    )
  })

  it('开发构建不参与版本比较', async () => {
    vi.mocked(getAboutInfo).mockResolvedValue({
      version: 'dev',
      build_time: '',
      update: {
        enabled: true,
        comparable: false,
        latest_version: 'v1.5.0',
        release_url: '',
        checked_at: '',
        error: '',
        update_available: false,
      },
    })
    renderAbout()

    await waitFor(() =>
      expect(screen.getByText('当前为开发构建，不参与版本比较')).toBeInTheDocument(),
    )
    expect(screen.queryByText('有新版本')).not.toBeInTheDocument()
  })
})
