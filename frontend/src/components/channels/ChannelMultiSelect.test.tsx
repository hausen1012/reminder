import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChannelMultiSelect } from '@/components/channels/ChannelMultiSelect'
import type { Channel } from '@/types'

const channels: Channel[] = [
  { id: 1, name: '邮件通知', type: 'smtp', enabled: true, config: {}, created_at: '', updated_at: '' },
  { id: 2, name: '钉钉群', type: 'dingtalk', enabled: true, config: {}, created_at: '', updated_at: '' },
]

describe('ChannelMultiSelect', () => {
  it('无渠道时显示空状态文案', () => {
    render(
      <ChannelMultiSelect channels={[]} value={[]} open={false} onOpenChange={vi.fn()} onChange={vi.fn()} />,
    )
    expect(screen.getByText(/还没有通知渠道/)).toBeInTheDocument()
  })

  it('未选中时显示占位文案', () => {
    render(
      <ChannelMultiSelect channels={channels} value={[]} open={false} onOpenChange={vi.fn()} onChange={vi.fn()} />,
    )
    expect(screen.getByText('选择通知渠道')).toBeInTheDocument()
  })

  it('已选中时显示渠道名（逗号分隔）', () => {
    render(
      <ChannelMultiSelect channels={channels} value={[1, 2]} open={false} onOpenChange={vi.fn()} onChange={vi.fn()} />,
    )
    expect(screen.getByText('邮件通知, 钉钉群')).toBeInTheDocument()
  })

  it('找不到对应渠道时回退显示 #id', () => {
    render(
      <ChannelMultiSelect channels={channels} value={[999]} open={false} onOpenChange={vi.fn()} onChange={vi.fn()} />,
    )
    expect(screen.getByText('#999')).toBeInTheDocument()
  })

  it('点击按钮切换展开状态', async () => {
    const onOpenChange = vi.fn()
    const user = userEvent.setup()
    render(
      <ChannelMultiSelect channels={channels} value={[]} open={false} onOpenChange={onOpenChange} onChange={vi.fn()} />,
    )

    await user.click(screen.getByRole('button'))
    expect(onOpenChange).toHaveBeenCalledWith(true)
  })

  it('展开时列出全部渠道复选框', () => {
    render(
      <ChannelMultiSelect channels={channels} value={[]} open onOpenChange={vi.fn()} onChange={vi.fn()} />,
    )
    expect(screen.getAllByRole('checkbox')).toHaveLength(2)
    expect(screen.getByText('邮件通知')).toBeInTheDocument()
    expect(screen.getByText('钉钉群')).toBeInTheDocument()
  })

  it('勾选未选项应追加 id', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(
      <ChannelMultiSelect channels={channels} value={[1]} open onOpenChange={vi.fn()} onChange={onChange} />,
    )

    const boxes = screen.getAllByRole('checkbox')
    // 第一个已勾选，第二个未勾选
    expect(boxes[0]).toBeChecked()
    expect(boxes[1]).not.toBeChecked()

    await user.click(boxes[1])
    expect(onChange).toHaveBeenCalledWith([1, 2])
  })

  it('取消勾选已选项应从数组移除', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(
      <ChannelMultiSelect channels={channels} value={[1, 2]} open onOpenChange={vi.fn()} onChange={onChange} />,
    )

    await user.click(screen.getAllByRole('checkbox')[0])
    expect(onChange).toHaveBeenCalledWith([2])
  })

  it('点击遮罩层应关闭下拉', async () => {
    const onOpenChange = vi.fn()
    const user = userEvent.setup()
    const { container } = render(
      <ChannelMultiSelect channels={channels} value={[]} open onOpenChange={onOpenChange} onChange={vi.fn()} />,
    )

    // 遮罩层是带 fixed inset-0 的 div
    const overlay = container.querySelector('div.fixed.inset-0')
    expect(overlay).not.toBeNull()
    await user.click(overlay as HTMLElement)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
