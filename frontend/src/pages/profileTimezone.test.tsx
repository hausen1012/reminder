import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

import { getConfig, updateConfig } from '@/lib/api'

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

// 设置页的时区卡片：加载现值、保存后调用 updateConfig。
describe('Profile 时区设置', () => {
  beforeEach(() => {
    localStorage.setItem('token', 'test-token')
    vi.mocked(getConfig).mockResolvedValue({ timezone: 'Europe/Berlin', app_name: '提醒' })
    vi.mocked(updateConfig).mockImplementation(async (cfg) => cfg)
  })

  it('回显当前站点时区', async () => {
    renderProfile()
    await waitFor(() => {
      expect(screen.getByText('时区')).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(screen.getByText('Europe/Berlin')).toBeInTheDocument()
    })
  })

  it('保存时区会提交 timezone 配置', async () => {
    renderProfile()
    await waitFor(() => expect(screen.getByText('Europe/Berlin')).toBeInTheDocument())

    const saveButtons = screen.getAllByRole('button', { name: '保存' })
    await userEvent.click(saveButtons[saveButtons.length - 1])

    await waitFor(() => {
      expect(vi.mocked(updateConfig)).toHaveBeenCalledWith({ timezone: 'Europe/Berlin' })
    })
  })
})
