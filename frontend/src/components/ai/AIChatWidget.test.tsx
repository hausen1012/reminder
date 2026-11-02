import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import { AIChatWidget } from '@/components/ai/AIChatWidget'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    getAISettings: vi.fn(),
    listAIActions: vi.fn(),
    sendAIChat: vi.fn(),
    applyAIAction: vi.fn(),
    rejectAIAction: vi.fn(),
  }
})

import {
  applyAIAction,
  getAISettings,
  listAIActions,
  rejectAIAction,
  sendAIChat,
} from '@/lib/api'
import type { AIAction, AISettings } from '@/types'

const enabledSettings: AISettings = {
  enabled: true,
  provider: 'anthropic',
  base_url: '',
  model: 'claude-opus-5-5',
  api_key: '***',
}

const pendingAction: AIAction = {
  id: 'act-1',
  kind: 'create_reminder',
  summary: '创建提醒：喝水',
  status: 'pending',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

function renderWidget() {
  return render(
    <ToastProviderRoot>
      <AIChatWidget />
    </ToastProviderRoot>,
  )
}

describe('AI 助手浮窗', () => {
  beforeEach(() => {
    sessionStorage.clear()
    vi.mocked(getAISettings).mockReset()
    vi.mocked(listAIActions).mockReset().mockResolvedValue([])
    vi.mocked(sendAIChat).mockReset()
    vi.mocked(applyAIAction).mockReset()
    vi.mocked(rejectAIAction).mockReset()
  })

  it('未启用时不渲染任何入口', async () => {
    vi.mocked(getAISettings).mockResolvedValue({ ...enabledSettings, enabled: false })
    renderWidget()
    await waitFor(() => expect(vi.mocked(getAISettings)).toHaveBeenCalled())
    expect(screen.queryByLabelText('打开 AI 助手')).toBeNull()
  })

  it('启用后右下角出现悬浮按钮，点击打开面板', async () => {
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    renderWidget()

    const trigger = await screen.findByLabelText('打开 AI 助手')
    await userEvent.click(trigger)

    expect(screen.getByText('AI 助手')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/描述你要的提醒或通道/)).toBeInTheDocument()
    // 打开后按钮隐藏，避免挡住面板
    expect(screen.queryByLabelText('打开 AI 助手')).toBeNull()
  })

  it('挂载时就拉待确认数量并显示角标', async () => {
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    vi.mocked(listAIActions).mockResolvedValue([pendingAction])
    renderWidget()

    const trigger = await screen.findByLabelText('打开 AI 助手')
    await waitFor(() => expect(trigger).toHaveTextContent('1'))
  })

  it('发送消息后展示回复，并把对话写进 sessionStorage', async () => {
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    vi.mocked(sendAIChat).mockResolvedValue({ reply: '已生成待确认卡片', actions: [] })
    renderWidget()

    await userEvent.click(await screen.findByLabelText('打开 AI 助手'))
    await userEvent.type(screen.getByPlaceholderText(/描述你要的提醒或通道/), '明天九点提醒我喝水')
    await userEvent.click(screen.getByRole('button', { name: /发送/ }))

    await waitFor(() => expect(screen.getByText('已生成待确认卡片')).toBeInTheDocument())
    expect(vi.mocked(sendAIChat)).toHaveBeenCalledWith('明天九点提醒我喝水', [])

    const stored = JSON.parse(sessionStorage.getItem('ai-chat-history') ?? '[]')
    expect(stored).toEqual([
      { role: 'user', content: '明天九点提醒我喝水' },
      { role: 'assistant', content: '已生成待确认卡片' },
    ])
  })

  it('后续请求带上已有历史（多轮上下文）', async () => {
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    vi.mocked(sendAIChat).mockResolvedValue({ reply: '好的', actions: [] })
    renderWidget()

    await userEvent.click(await screen.findByLabelText('打开 AI 助手'))
    const box = screen.getByPlaceholderText(/描述你要的提醒或通道/)
    await userEvent.type(box, '第一条')
    await userEvent.click(screen.getByRole('button', { name: /发送/ }))
    await waitFor(() => expect(screen.getByText('好的')).toBeInTheDocument())

    await userEvent.type(box, '第二条')
    await userEvent.click(screen.getByRole('button', { name: /发送/ }))
    await waitFor(() => expect(vi.mocked(sendAIChat)).toHaveBeenCalledTimes(2))

    expect(vi.mocked(sendAIChat).mock.calls[1]).toEqual([
      '第二条',
      [
        { role: 'user', content: '第一条' },
        { role: 'assistant', content: '好的' },
      ],
    ])
  })

  it('刷新后从 sessionStorage 恢复对话', async () => {
    sessionStorage.setItem(
      'ai-chat-history',
      JSON.stringify([{ role: 'user', content: '上次说的话' }]),
    )
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    renderWidget()

    await userEvent.click(await screen.findByLabelText('打开 AI 助手'))
    expect(screen.getByText('上次说的话')).toBeInTheDocument()
  })

  it('损坏的 sessionStorage 不导致崩溃', async () => {
    sessionStorage.setItem('ai-chat-history', '{not json')
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    renderWidget()

    await userEvent.click(await screen.findByLabelText('打开 AI 助手'))
    expect(screen.getByText(/用自然语言告诉我要做什么/)).toBeInTheDocument()
  })

  it('待确认草稿展示卡片，确认后从列表移除', async () => {
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    vi.mocked(listAIActions).mockResolvedValue([pendingAction])
    vi.mocked(applyAIAction).mockResolvedValue({ ...pendingAction, status: 'applied' })
    renderWidget()

    await userEvent.click(await screen.findByLabelText('打开 AI 助手'))
    await waitFor(() => expect(screen.getByText('创建提醒：喝水')).toBeInTheDocument())

    await userEvent.click(screen.getByRole('button', { name: /确认/ }))
    await waitFor(() => expect(vi.mocked(applyAIAction)).toHaveBeenCalledWith('act-1'))
    // 卡片连同「确认」按钮一起消失（toast 里也会出现同名摘要，故按按钮判断）
    await waitFor(() => expect(screen.queryByRole('button', { name: /确认/ })).toBeNull())
  })

  it('忽略草稿后从列表移除', async () => {
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    vi.mocked(listAIActions).mockResolvedValue([pendingAction])
    vi.mocked(rejectAIAction).mockResolvedValue({ ...pendingAction, status: 'rejected' })
    renderWidget()

    await userEvent.click(await screen.findByLabelText('打开 AI 助手'))
    await waitFor(() => expect(screen.getByText('创建提醒：喝水')).toBeInTheDocument())

    await userEvent.click(screen.getByRole('button', { name: /忽略/ }))
    await waitFor(() => expect(vi.mocked(rejectAIAction)).toHaveBeenCalledWith('act-1'))
    await waitFor(() => expect(screen.queryByRole('button', { name: /忽略/ })).toBeNull())
  })

  it('请求失败时回填输入框并提示', async () => {
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    vi.mocked(sendAIChat).mockRejectedValue({
      response: { data: { error: { code: 'validation_failed', message: 'AI 调用失败' } } },
    })
    renderWidget()

    await userEvent.click(await screen.findByLabelText('打开 AI 助手'))
    const box = screen.getByPlaceholderText(/描述你要的提醒或通道/) as HTMLTextAreaElement
    await userEvent.type(box, '明天提醒我')
    await userEvent.click(screen.getByRole('button', { name: /发送/ }))

    await waitFor(() => expect(screen.getByText('AI 调用失败')).toBeInTheDocument())
    expect(box.value).toBe('明天提醒我')
    // 失败的消息不应留在历史里（只回填到输入框）
    expect(JSON.parse(sessionStorage.getItem('ai-chat-history') ?? '[]')).toEqual([])
  })

  it('清空对话会同时清掉本地存储', async () => {
    sessionStorage.setItem('ai-chat-history', JSON.stringify([{ role: 'user', content: '旧消息' }]))
    vi.mocked(getAISettings).mockResolvedValue(enabledSettings)
    renderWidget()

    await userEvent.click(await screen.findByLabelText('打开 AI 助手'))
    await userEvent.click(screen.getByTitle('清空对话'))

    expect(screen.queryByText('旧消息')).toBeNull()
    expect(JSON.parse(sessionStorage.getItem('ai-chat-history') ?? '[]')).toEqual([])
  })
})
