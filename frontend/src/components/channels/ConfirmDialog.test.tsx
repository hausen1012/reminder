import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmDialog } from '@/components/channels/ConfirmDialog'

describe('ConfirmDialog', () => {
  it('open=false 时不渲染内容', () => {
    render(
      <ConfirmDialog open={false} title="删除确认" onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    expect(screen.queryByText('删除确认')).not.toBeInTheDocument()
  })

  it('open=true 时渲染标题与默认按钮文案', () => {
    render(
      <ConfirmDialog open title="删除确认" onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    expect(screen.getByText('删除确认')).toBeInTheDocument()
    expect(screen.getByText('取消')).toBeInTheDocument()
    expect(screen.getByText('确认')).toBeInTheDocument()
  })

  it('有 description 时渲染描述', () => {
    render(
      <ConfirmDialog
        open
        title="删除"
        description="此操作不可撤销"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    expect(screen.getByText('此操作不可撤销')).toBeInTheDocument()
  })

  it('自定义按钮文案生效', () => {
    render(
      <ConfirmDialog
        open
        title="标题"
        confirmText="删除"
        cancelText="算了"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    expect(screen.getByText('删除')).toBeInTheDocument()
    expect(screen.getByText('算了')).toBeInTheDocument()
  })

  it('点击确认按钮触发 onConfirm', async () => {
    const onConfirm = vi.fn()
    const user = userEvent.setup()
    render(<ConfirmDialog open title="标题" onConfirm={onConfirm} onCancel={vi.fn()} />)

    await user.click(screen.getByText('确认'))
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('点击取消按钮触发 onCancel', async () => {
    const onCancel = vi.fn()
    const user = userEvent.setup()
    render(<ConfirmDialog open title="标题" onConfirm={vi.fn()} onCancel={onCancel} />)

    await user.click(screen.getByText('取消'))
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('loading 时两个按钮都禁用且确认文案带"中…"', () => {
    render(<ConfirmDialog open title="标题" loading onConfirm={vi.fn()} onCancel={vi.fn()} />)

    expect(screen.getByText('确认中…')).toBeInTheDocument()
    expect(screen.getByText('确认中…').closest('button')).toBeDisabled()
    expect(screen.getByText('取消').closest('button')).toBeDisabled()
  })

  it('destructive 时确认按钮使用 destructive 变体', () => {
    render(
      <ConfirmDialog open title="标题" destructive onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    const btn = screen.getByText('确认').closest('button')
    // shadcn 的 destructive 变体带 bg-destructive class
    expect(btn?.className).toContain('destructive')
  })

  it('loading 时按 Esc 不应触发 onCancel', async () => {
    const onCancel = vi.fn()
    const user = userEvent.setup()
    render(<ConfirmDialog open title="标题" loading onConfirm={vi.fn()} onCancel={onCancel} />)

    await user.keyboard('{Escape}')
    expect(onCancel).not.toHaveBeenCalled()
  })
})
