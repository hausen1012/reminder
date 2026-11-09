import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { Bell, Plus, SearchX } from 'lucide-react'
import { EmptyState } from './EmptyState'

describe('EmptyState 组件', () => {
  it('正确渲染标题、描述与图标', () => {
    render(
      <EmptyState
        icon={Bell}
        title="还没有提醒"
        description="创建你的第一条提醒，到达时间后准时推送。"
      />,
    )

    expect(screen.getByText('还没有提醒')).toBeInTheDocument()
    expect(screen.getByText('创建你的第一条提醒，到达时间后准时推送。')).toBeInTheDocument()
  })

  it('点击主操作按钮触发 onClick', () => {
    const handleClick = vi.fn()
    render(
      <EmptyState
        icon={Bell}
        title="还没有提醒"
        action={{
          label: '新建提醒',
          onClick: handleClick,
          icon: Plus,
        }}
      />,
    )

    const btn = screen.getByRole('button', { name: /新建提醒/ })
    expect(btn).toBeInTheDocument()
    fireEvent.click(btn)
    expect(handleClick).toHaveBeenCalledTimes(1)
  })

  it('支持次要操作按钮', () => {
    const handlePrimary = vi.fn()
    const handleSecondary = vi.fn()
    render(
      <EmptyState
        icon={SearchX}
        title="未找到匹配项"
        action={{
          label: '重置筛选',
          onClick: handlePrimary,
        }}
        secondaryAction={{
          label: '新建提醒',
          onClick: handleSecondary,
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /重置筛选/ }))
    fireEvent.click(screen.getByRole('button', { name: /新建提醒/ }))
    expect(handlePrimary).toHaveBeenCalledTimes(1)
    expect(handleSecondary).toHaveBeenCalledTimes(1)
  })

  it('支持 dashed 变体和 compact 尺寸', () => {
    const { container } = render(
      <EmptyState
        icon={Bell}
        title="紧凑状态"
        variant="dashed"
        size="compact"
      />,
    )

    expect(container.querySelector('.border-dashed')).toBeInTheDocument()
    expect(screen.getByText('紧凑状态')).toBeInTheDocument()
  })
})
