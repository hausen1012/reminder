import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ConfigProvider } from '@/contexts/ConfigContext'
import type { SchedulerStatus } from '@/types'
import SchedulerPage from './index'

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    getSchedulerStatus: vi.fn(),
    getConfig: vi.fn(),
  }
})

import { getSchedulerStatus, getConfig } from '@/lib/api'

describe('SchedulerPage 监控页', () => {
  beforeEach(() => {
    vi.mocked(getConfig).mockResolvedValue({ timezone: 'Asia/Shanghai' })
  })

  it('展示通知渠道列，不展示 ID 列与 ID 编号', async () => {
    vi.mocked(getSchedulerStatus).mockResolvedValue({
      engine: {
        running: true,
        registered_count: 2,
        entries_total: 2,
        entries: [
          {
            id: 101,
            kind: 'cron',
            title: '早报提醒',
            schedule_type: 'cron',
            next_fire_at: '2026-11-04T09:00:00Z',
            last_fired_at: '2026-11-03T09:00:00Z',
            channels: [
              { id: 1, name: '企业微信', type: 'wecom' },
              { id: 2, name: '邮件', type: 'smtp' },
            ],
          },
          {
            id: 102,
            kind: 'afterfunc',
            title: '无渠道提醒',
            schedule_type: 'once',
            next_fire_at: '2026-11-04T10:00:00Z',
            channels: [],
          },
        ],
      },
      sweeper: {
        running: true,
        interval_seconds: 60,
      },
    })

    render(
      <MemoryRouter>
        <ConfigProvider>
          <SchedulerPage />
        </ConfigProvider>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getAllByText('早报提醒').length).toBeGreaterThan(0))

    // 验证表头有「通知渠道」，没有「ID」
    expect(screen.getByRole('columnheader', { name: '通知渠道' })).toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: 'ID' })).not.toBeInTheDocument()

    // 验证表格内容中渠道信息渲染正确
    expect(screen.getAllByText('企业微信、邮件').length).toBeGreaterThan(0)
    expect(screen.getAllByText('—').length).toBeGreaterThan(0)

    // 验证不出现 #101 或单独作为单元格文本的 101
    expect(screen.queryByText('#101')).not.toBeInTheDocument()
    expect(screen.queryByText('#102')).not.toBeInTheDocument()
  })

  it('定时刷新时保持表格展示，不闪烁出「加载中…」', async () => {
    const mockData: SchedulerStatus = {
      engine: {
        running: true,
        registered_count: 1,
        entries_total: 1,
        entries: [
          {
            id: 201,
            kind: 'cron' as const,
            title: '测试任务',
            schedule_type: 'cron',
            next_fire_at: '2026-11-04T09:00:00Z',
            last_fired_at: '2026-11-03T09:00:00Z',
            channels: [],
          },
        ],
      },
      sweeper: {
        running: true,
        interval_seconds: 60,
      },
    }
    vi.mocked(getSchedulerStatus).mockResolvedValue(mockData)

    render(
      <MemoryRouter>
        <ConfigProvider>
          <SchedulerPage />
        </ConfigProvider>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getAllByText('测试任务').length).toBeGreaterThan(0))

    // 再次调用 getSchedulerStatus 模拟定时器触发返回
    vi.mocked(getSchedulerStatus).mockResolvedValue({
      ...mockData,
      engine: {
        ...mockData.engine,
        entries: [
          {
            ...mockData.engine.entries[0],
            title: '更新后的任务',
          },
        ],
      },
    })

    // 手动触发一次刷新验证不闪烁加载中
    const refreshBtn = screen.getByRole('button', { name: /刷新/ })
    fireEvent.click(refreshBtn)

    // 应该直接更新出新任务名，且始终不会看到全块的「加载中…」
    await waitFor(() => expect(screen.getAllByText('更新后的任务').length).toBeGreaterThan(0))
    expect(screen.queryByText('加载中…')).not.toBeInTheDocument()
  })
})
