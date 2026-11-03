import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ConfigProvider } from '@/contexts/ConfigContext'
import { AuthProvider } from '@/contexts/AuthContext'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import Dashboard from '@/pages/Dashboard'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    listTodayReminders: vi.fn(),
    listLogs: vi.fn(),
    listChannelStats: vi.fn(),
    getSchedulerStatus: vi.fn(),
    getConfig: vi.fn(),
    getMe: vi.fn(),
  }
})

import {
  listTodayReminders,
  listLogs,
  listChannelStats,
  getSchedulerStatus,
  getConfig,
  getMe,
} from '@/lib/api'

describe('Dashboard 首页', () => {
  beforeEach(() => {
    localStorage.setItem('token', 'test-token')
    vi.mocked(getConfig).mockResolvedValue({ timezone: 'Asia/Shanghai' })
    vi.mocked(getMe).mockResolvedValue({
      code: 0,
      message: 'ok',
      data: { id: 1, username: 'admin', is_admin: true, created_at: '' },
    })
    vi.mocked(listTodayReminders).mockResolvedValue([
      {
        id: 1,
        title: '每日晨会',
        content: '准备日报',
        content_format: 'text',
        calendar: 'solar',
        schedule_type: 'cron',
        schedule_spec: { expr: '0 9 * * *' },
        enabled: true,
        source: 'web',
        channel_ids: [1],
        require_confirm: false,
        confirm_retry_interval_sec: 300,
        confirm_max_retries: 3,
        next_fire_at: '2026-11-03T09:00:00Z',
        created_at: '2026-11-01T00:00:00Z',
        updated_at: '2026-11-01T00:00:00Z',
      },
    ])
    vi.mocked(listLogs).mockResolvedValue({
      items: [
        {
          id: 101,
          reminder_id: 1,
          fired_at: '2026-11-03T09:00:00Z',
          title: '每日晨会',
          content: '日报内容',
          status: 'success',
          confirmed: false,
          retry_round: 0,
          source: 'web',
          created_at: '2026-11-03T09:00:00Z',
          reminder_title: '每日晨会',
        },
      ],
      total: 1,
    })
    vi.mocked(listChannelStats).mockResolvedValue([
      {
        id: 1,
        name: '企业微信群',
        type: 'wecom',
        total: 5,
        success: 5,
        failed: 0,
        success_rate: 1,
      },
    ])
    vi.mocked(getSchedulerStatus).mockResolvedValue({
      engine: {
        running: true,
        registered_count: 8,
        entries: [],
        entries_total: 8,
      },
      sweeper: {
        running: true,
        interval_seconds: 5,
      },
    })
  })

  it('展示调度引擎状态卡片（替换原令牌调用）及快捷操作按钮', async () => {
    render(
      <MemoryRouter>
        <ToastProviderRoot>
          <AuthProvider>
            <ConfigProvider>
              <Dashboard />
            </ConfigProvider>
          </AuthProvider>
        </ToastProviderRoot>
      </MemoryRouter>,
    )

    // 检查快捷按钮
    expect(screen.getByRole('button', { name: /新建提醒/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /刷新/ })).toBeInTheDocument()

    // 检查调度引擎状态卡片已渲染，且显示“运行中”
    await waitFor(() => {
      expect(screen.getByText('调度引擎')).toBeInTheDocument()
      expect(screen.getByText('运行中')).toBeInTheDocument()
      expect(screen.getByText(/已装载 8 个调度任务/)).toBeInTheDocument()
    })

    // 验证今日待发与日志中包含该提醒
    expect(screen.getAllByText('每日晨会').length).toBe(2)
    expect(screen.getByText(/100.0%/)).toBeInTheDocument()
  })
})
