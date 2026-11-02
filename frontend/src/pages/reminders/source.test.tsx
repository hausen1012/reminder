import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import { SOURCE_BADGE_CLASS, SOURCE_LABEL, sourceLabel } from '@/lib/constants'

vi.mock('@/contexts/ConfigContext', () => ({
  useSiteTimezone: () => 'Asia/Shanghai',
  useConfig: () => ({ config: {}, loading: false, updateConfig: vi.fn() }),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    listReminders: vi.fn(),
    listChannels: vi.fn(),
    toggleReminder: vi.fn(),
    deleteReminder: vi.fn(),
    createReminder: vi.fn(),
    batchDeleteReminders: vi.fn(),
  }
})

import { listChannels, listReminders } from '@/lib/api'
import RemindersPage from '@/pages/reminders'
import type { Reminder } from '@/types'

function makeReminder(id: number, title: string, source: Reminder['source']): Reminder {
  return {
    id,
    title,
    content: '',
    content_format: 'text',
    calendar: 'solar',
    schedule_type: 'once',
    schedule_spec: { at: '2030-01-01T09:00:00' },
    enabled: true,
    source,
    require_confirm: false,
    confirm_retry_interval_sec: 0,
    confirm_max_retries: 0,
    fire_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    channel_ids: [],
  }
}

function renderPage() {
  return render(
    <ToastProviderRoot>
      <RemindersPage />
    </ToastProviderRoot>,
  )
}

describe('来源展示常量', () => {
  it('三种来源都有文案与配色', () => {
    for (const key of ['web', 'ai', 'api']) {
      expect(SOURCE_LABEL[key]).toBeTruthy()
      expect(SOURCE_BADGE_CLASS[key]).toBeTruthy()
    }
  })

  it('sourceLabel 对未知来源原样返回，空值给占位符', () => {
    expect(sourceLabel('web')).toBe('Web')
    expect(sourceLabel('ai')).toBe('AI')
    expect(sourceLabel('api')).toBe('API')
    expect(sourceLabel('custom')).toBe('custom')
    expect(sourceLabel(undefined)).toBe('—')
  })
})

describe('提醒列表来源展示', () => {
  beforeEach(() => {
    vi.mocked(listChannels).mockResolvedValue([])
    vi.mocked(listReminders).mockResolvedValue({
      items: [
        makeReminder(1, '表单创建的提醒', 'web'),
        makeReminder(2, 'AI 创建的提醒', 'ai'),
        makeReminder(3, '接口创建的提醒', 'api'),
      ],
      total: 3,
    })
  })

  it('列表同时展示 Web / AI / API 三种来源', async () => {
    renderPage()
    // 桌面端表格与移动端卡片都会渲染，同一标题出现两次
    await waitFor(() => expect(screen.getAllByText('AI 创建的提醒').length).toBeGreaterThan(0))

    expect(screen.getAllByText('Web').length).toBeGreaterThan(0)
    expect(screen.getAllByText('AI').length).toBeGreaterThan(0)
    expect(screen.getAllByText('API').length).toBeGreaterThan(0)
  })

  it('默认请求不带 source，AI 创建的提醒不会被默认筛掉', async () => {
    renderPage()
    await waitFor(() => expect(vi.mocked(listReminders)).toHaveBeenCalled())

    const query = vi.mocked(listReminders).mock.calls[0][0]
    expect(query?.source).toBeUndefined()
  })
})
