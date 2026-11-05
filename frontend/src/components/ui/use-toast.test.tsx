import { render, screen, fireEvent, act } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { ToastProviderRoot, useToast } from './use-toast'

function TestTrigger() {
  const { toast } = useToast()
  return (
    <div>
      <button onClick={() => toast({ title: '通知 1', description: '这是第一条' })}>
        弹第 1 条
      </button>
      <button onClick={() => toast({ title: '通知 2', description: '这是第二条' })}>
        弹第 2 条
      </button>
      <button onClick={() => toast({ title: '通知 3', description: '这是第三条' })}>
        弹第 3 条
      </button>
    </div>
  )
}

describe('ToastProviderRoot - 多条通知卡片折叠与交互', () => {
  it('单条通知正常展示且无折叠徽标', () => {
    render(
      <ToastProviderRoot>
        <TestTrigger />
      </ToastProviderRoot>,
    )

    fireEvent.click(screen.getByText('弹第 1 条'))

    expect(screen.getByText('通知 1')).toBeInTheDocument()
    expect(screen.getByText('这是第一条')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /\+.*条/ })).not.toBeInTheDocument()
  })

  it('多条通知默认折叠并显示数量徽标，点击展开后可一键全部关闭', () => {
    render(
      <ToastProviderRoot>
        <TestTrigger />
      </ToastProviderRoot>,
    )

    act(() => {
      fireEvent.click(screen.getByText('弹第 1 条'))
      fireEvent.click(screen.getByText('弹第 2 条'))
      fireEvent.click(screen.getByText('弹第 3 条'))
    })

    // 最新一条应该在最顶上展示
    expect(screen.getByText('通知 3')).toBeInTheDocument()
    // 折叠徽标显示剩余 +2 条
    const expandBtn = screen.getByText('+2 条')
    expect(expandBtn).toBeInTheDocument()

    // 点击展开徽标
    fireEvent.click(expandBtn)

    // 展开状态显示共 3 条提醒，并提供全部关闭按钮
    expect(screen.getByText('共 3 条提醒')).toBeInTheDocument()
    expect(screen.getByText('全部关闭')).toBeInTheDocument()
    expect(screen.getByText('收起')).toBeInTheDocument()

    // 展开后所有通知可见
    expect(screen.getByText('通知 1')).toBeInTheDocument()
    expect(screen.getByText('通知 2')).toBeInTheDocument()
    expect(screen.getByText('通知 3')).toBeInTheDocument()

    // 点击全部关闭
    fireEvent.click(screen.getByText('全部关闭'))

    expect(screen.queryByText('通知 1')).not.toBeInTheDocument()
    expect(screen.queryByText('通知 2')).not.toBeInTheDocument()
    expect(screen.queryByText('通知 3')).not.toBeInTheDocument()
  })
})
