import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Pagination } from '@/components/ui/pagination'

describe('Pagination', () => {
  it('无数据时显示 0-0 与 1/1', () => {
    render(<Pagination total={0} limit={10} offset={0} onPageChange={vi.fn()} />)
    expect(screen.getByText(/共 0 条/)).toBeInTheDocument()
    expect(screen.getByText('1 / 1')).toBeInTheDocument()
  })

  it('正确显示条目区间', () => {
    render(<Pagination total={95} limit={10} offset={20} onPageChange={vi.fn()} />)
    expect(screen.getByText(/第 21-30 条/)).toBeInTheDocument()
    expect(screen.getByText('3 / 10')).toBeInTheDocument()
  })

  it('最后一页的 end 应被 total 夹住', () => {
    render(<Pagination total={25} limit={10} offset={20} onPageChange={vi.fn()} />)
    expect(screen.getByText(/第 21-25 条/)).toBeInTheDocument()
  })

  it('首页时上一页按钮禁用', () => {
    render(<Pagination total={50} limit={10} offset={0} onPageChange={vi.fn()} />)
    expect(screen.getByLabelText('上一页')).toBeDisabled()
    expect(screen.getByLabelText('下一页')).not.toBeDisabled()
  })

  it('末页时下一页按钮禁用', () => {
    render(<Pagination total={50} limit={10} offset={40} onPageChange={vi.fn()} />)
    expect(screen.getByLabelText('下一页')).toBeDisabled()
    expect(screen.getByLabelText('上一页')).not.toBeDisabled()
  })

  it('点击下一页应回调新的 offset', async () => {
    const onPageChange = vi.fn()
    const user = userEvent.setup()
    render(<Pagination total={50} limit={10} offset={0} onPageChange={onPageChange} />)

    await user.click(screen.getByLabelText('下一页'))
    expect(onPageChange).toHaveBeenCalledWith(10)
  })

  it('点击上一页应回调新的 offset', async () => {
    const onPageChange = vi.fn()
    const user = userEvent.setup()
    render(<Pagination total={50} limit={10} offset={20} onPageChange={onPageChange} />)

    await user.click(screen.getByLabelText('上一页'))
    expect(onPageChange).toHaveBeenCalledWith(10)
  })

  it('未传 onLimitChange 时不渲染每页选择器', () => {
    render(<Pagination total={50} limit={10} offset={0} onPageChange={vi.fn()} />)
    expect(screen.queryByText('每页')).not.toBeInTheDocument()
  })

  it('传了 onLimitChange 时渲染每页选择器', () => {
    render(
      <Pagination total={50} limit={10} offset={0} onPageChange={vi.fn()} onLimitChange={vi.fn()} />,
    )
    expect(screen.getByText('每页')).toBeInTheDocument()
  })

  it('offset 超出总页数时 currentPage 被夹到末页', () => {
    // total=15 limit=10 → 2 页；offset=100 属于越界
    render(<Pagination total={15} limit={10} offset={100} onPageChange={vi.fn()} />)
    expect(screen.getByText('2 / 2')).toBeInTheDocument()
  })
})
