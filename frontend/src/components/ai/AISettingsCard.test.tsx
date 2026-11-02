import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import { AISettingsCard } from '@/components/ai/AISettingsCard'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    getAISettings: vi.fn(),
    updateAISettings: vi.fn(),
    testAIConnection: vi.fn(),
  }
})

import { getAISettings, testAIConnection, updateAISettings } from '@/lib/api'

function renderCard() {
  return render(
    <ToastProviderRoot>
      <AISettingsCard />
    </ToastProviderRoot>,
  )
}

describe('AI 设置卡片', () => {
  beforeEach(() => {
    vi.mocked(getAISettings).mockReset()
    vi.mocked(updateAISettings).mockReset()
    vi.mocked(testAIConnection).mockReset()
  })

  it('回显已有设置，密钥框留空并提示不修改', async () => {
    vi.mocked(getAISettings).mockResolvedValue({
      enabled: true,
      provider: 'openai',
      base_url: 'https://api.deepseek.com',
      model: 'deepseek-chat',
      api_key: '***',
    })
    renderCard()

    await waitFor(() => {
      expect(screen.getByDisplayValue('deepseek-chat')).toBeInTheDocument()
    })
    expect(screen.getByDisplayValue('https://api.deepseek.com')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('已配置，留空表示不修改')).toBeInTheDocument()
  })

  it('保存时未改密钥回传占位符，改了就传明文', async () => {
    vi.mocked(getAISettings).mockResolvedValue({
      enabled: true,
      provider: 'anthropic',
      base_url: '',
      model: 'claude-opus-5-5',
      api_key: '***',
    })
    vi.mocked(updateAISettings).mockImplementation(async (input) => ({
      ...input,
      api_key: input.api_key === '***' ? '***' : '***',
    }))
    renderCard()

    await waitFor(() => expect(screen.getByPlaceholderText('已配置，留空表示不修改')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateAISettings)).toHaveBeenCalledWith(
        expect.objectContaining({ api_key: '***', provider: 'anthropic' }),
      )
    })

    // 填入新密钥后再保存，应传明文
    vi.mocked(updateAISettings).mockClear()
    await userEvent.type(screen.getByPlaceholderText('已配置，留空表示不修改'), 'sk-new')
    await userEvent.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => {
      expect(vi.mocked(updateAISettings)).toHaveBeenCalledWith(
        expect.objectContaining({ api_key: 'sk-new' }),
      )
    })
  })

  it('测试连接成功时弹出回复内容', async () => {
    vi.mocked(getAISettings).mockResolvedValue({
      enabled: true,
      provider: 'anthropic',
      base_url: '',
      model: 'claude-opus-5-5',
      api_key: '***',
    })
    vi.mocked(testAIConnection).mockResolvedValue({ reply: 'OK' })
    renderCard()

    await waitFor(() => expect(screen.getByRole('button', { name: /测试连接/ })).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /测试连接/ }))

    await waitFor(() => {
      expect(vi.mocked(testAIConnection)).toHaveBeenCalled()
      expect(screen.getByText(/连接成功/)).toBeInTheDocument()
    })
  })
})
