import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { ConfigProvider } from '@/contexts/ConfigContext'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import Profile from '@/pages/Profile'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    getConfig: vi.fn(),
    updateConfig: vi.fn(),
    updatePassword: vi.fn(),
  }
})

import { getConfig } from '@/lib/api'

// 设置页的「关于项目」卡片：展示开源仓库与文档站点地址。
describe('Profile 关于项目', () => {
  beforeEach(() => {
    localStorage.setItem('token', 'test-token')
    vi.mocked(getConfig).mockResolvedValue({ timezone: 'Asia/Shanghai' })
  })

  it('展示 GitHub 与文档站地址，均在新窗口打开', async () => {
    render(
      <ToastProviderRoot>
        <ConfigProvider>
          <Profile />
        </ConfigProvider>
      </ToastProviderRoot>,
    )

    await waitFor(() => expect(screen.getByText('关于项目')).toBeInTheDocument())

    const github = screen.getByRole('link', { name: /github\.com\/hausen1012\/reminder/ })
    expect(github).toHaveAttribute('href', 'https://github.com/hausen1012/reminder')
    expect(github).toHaveAttribute('target', '_blank')
    expect(github).toHaveAttribute('rel', 'noreferrer noopener')

    const docs = screen.getByRole('link', { name: /hausen1012\.github\.io\/reminder/ })
    expect(docs).toHaveAttribute('href', 'https://hausen1012.github.io/reminder/')
    expect(docs).toHaveAttribute('target', '_blank')
    expect(docs).toHaveAttribute('rel', 'noreferrer noopener')
  })
})
