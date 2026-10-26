import { describe, expect, it, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { ConfigProvider, useConfig } from '@/contexts/ConfigContext'

// 只 mock API 层，context 自身的逻辑不 mock
const getConfig = vi.fn()
const updateConfigApi = vi.fn()
vi.mock('@/lib/api', () => ({
  getConfig: () => getConfig(),
  updateConfig: (cfg: Record<string, string>) => updateConfigApi(cfg),
}))

function ConfigProbe() {
  const { config, loading } = useConfig()
  return (
    <div>
      <span data-testid="loading">{String(loading)}</span>
      <span data-testid="app-name">{config.app_name ?? ''}</span>
    </div>
  )
}

function renderWithProvider() {
  return render(
    <ConfigProvider>
      <ConfigProbe />
    </ConfigProvider>,
  )
}

describe('ConfigContext', () => {
  beforeEach(() => {
    localStorage.clear()
    getConfig.mockReset()
    updateConfigApi.mockReset()
    document.title = ''
    document.querySelectorAll('link[rel="icon"]').forEach((el) => el.remove())
  })

  it('无 token 时不请求配置，直接结束 loading', async () => {
    renderWithProvider()
    expect(await screen.findByTestId('loading')).toHaveTextContent('false')
    expect(getConfig).not.toHaveBeenCalled()
  })

  it('有 token 时拉取配置并应用 app_name 到标题', async () => {
    localStorage.setItem('token', 'tk')
    getConfig.mockResolvedValue({ app_name: '我的提醒助手' })

    renderWithProvider()

    await waitFor(() => {
      expect(screen.getByTestId('app-name')).toHaveTextContent('我的提醒助手')
    })
    expect(document.title).toBe('我的提醒助手')
  })

  it('拉取失败时不抛错，loading 正常结束', async () => {
    localStorage.setItem('token', 'tk')
    getConfig.mockRejectedValue(new Error('网络错误'))

    renderWithProvider()

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })
    // 配置保持空
    expect(screen.getByTestId('app-name')).toHaveTextContent('')
  })

  it('logo_svg 应写入 favicon link', async () => {
    localStorage.setItem('token', 'tk')
    getConfig.mockResolvedValue({ logo_svg: '<svg>logo</svg>' })

    renderWithProvider()

    await waitFor(() => {
      const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
      expect(link).not.toBeNull()
      expect(link!.href).toContain('data:image/svg+xml')
    })
  })

  it('无 logo_svg 时移除已存在的 favicon link', async () => {
    // 预先插入一个 link
    const existing = document.createElement('link')
    existing.rel = 'icon'
    document.head.appendChild(existing)

    localStorage.setItem('token', 'tk')
    getConfig.mockResolvedValue({ app_name: 'x' })

    renderWithProvider()

    await waitFor(() => {
      expect(document.querySelector('link[rel="icon"]')).toBeNull()
    })
  })

  it('useConfig 在 Provider 外调用应抛错', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<ConfigProbe />)).toThrow('useConfig must be used within ConfigProvider')
    spy.mockRestore()
  })
})
