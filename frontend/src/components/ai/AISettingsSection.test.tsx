import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import { AISettingsSection } from '@/components/ai/AISettingsSection'

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

function renderSection() {
  return render(
    <ToastProviderRoot>
      <AISettingsSection />
    </ToastProviderRoot>,
  )
}

describe('AI 设置分区', () => {
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
      custom_prompt: '',
    })
    renderSection()

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
      custom_prompt: '',
    })
    vi.mocked(updateAISettings).mockImplementation(async (input) => ({
      ...input,
      api_key: '***',
      custom_prompt: '',
    }))
    renderSection()

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

  it('回显已有自定义提示词，保存时随设置一起提交', async () => {
    vi.mocked(getAISettings).mockResolvedValue({
      enabled: true,
      provider: 'anthropic',
      base_url: '',
      model: 'claude-opus-5-5',
      api_key: '***',
      custom_prompt: '称呼我为老板',
    })
    vi.mocked(updateAISettings).mockImplementation(async (input) => ({
      ...input,
      api_key: '***',
    }))
    renderSection()

    await waitFor(() => expect(screen.getByDisplayValue('称呼我为老板')).toBeInTheDocument())

    const box = screen.getByLabelText('自定义提示词（可选）')
    await userEvent.clear(box)
    await userEvent.type(box, '回复一句话说完')
    await userEvent.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateAISettings)).toHaveBeenCalledWith(
        expect.objectContaining({ custom_prompt: '回复一句话说完' }),
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
      custom_prompt: '',
    })
    vi.mocked(testAIConnection).mockResolvedValue({ reply: 'OK' })
    renderSection()

    await waitFor(() => expect(screen.getByRole('button', { name: /测试连接/ })).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /测试连接/ }))

    await waitFor(() => {
      expect(vi.mocked(testAIConnection)).toHaveBeenCalled()
      expect(screen.getByText(/连接成功/)).toBeInTheDocument()
    })
  })
})
