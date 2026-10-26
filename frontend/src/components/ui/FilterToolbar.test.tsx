import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FilterToolbar } from '@/components/ui/FilterToolbar'

function setup(overrides: Partial<React.ComponentProps<typeof FilterToolbar>> = {}) {
  const props = {
    searchValue: '',
    onSearchChange: vi.fn(),
    onSearch: vi.fn(),
    onRefresh: vi.fn(),
    ...overrides,
  }
  render(<FilterToolbar {...props} />)
  return props
}

describe('FilterToolbar', () => {
  it('渲染搜索框与占位文案', () => {
    setup()
    expect(screen.getByPlaceholderText('搜索…')).toBeInTheDocument()
  })

  it('支持自定义占位文案', () => {
    setup({ placeholder: '搜索提醒标题' })
    expect(screen.getByPlaceholderText('搜索提醒标题')).toBeInTheDocument()
  })

  it('输入文字应回调 onSearchChange', async () => {
    // FilterToolbar 是受控组件：必须由外部维护 value，
    // 否则每次输入都会被重置为空串（这也是本用例要覆盖的契约）。
    function Controlled() {
      const [v, setV] = React.useState('')
      return (
        <FilterToolbar
          searchValue={v}
          onSearchChange={setV}
          onSearch={vi.fn()}
          onRefresh={vi.fn()}
        />
      )
    }
    render(<Controlled />)
    const user = userEvent.setup()

    await user.type(screen.getByPlaceholderText('搜索…'), 'abc')
    expect(screen.getByPlaceholderText('搜索…')).toHaveValue('abc')
  })

  it('提交表单应回调 onSearch（并阻止默认跳转）', async () => {
    const props = setup({ searchValue: '关键字' })
    const user = userEvent.setup()

    await user.click(screen.getByTitle('搜索'))
    expect(props.onSearch).toHaveBeenCalledTimes(1)
  })

  it('在输入框按回车也应触发搜索', async () => {
    const props = setup({ searchValue: 'k' })
    const user = userEvent.setup()

    await user.type(screen.getByPlaceholderText('搜索…'), '{Enter}')
    expect(props.onSearch).toHaveBeenCalled()
  })

  it('点击刷新按钮应回调 onRefresh', async () => {
    const props = setup()
    const user = userEvent.setup()

    await user.click(screen.getByTitle('刷新'))
    expect(props.onRefresh).toHaveBeenCalledTimes(1)
  })

  it('children 应渲染在搜索框之前', () => {
    render(
      <FilterToolbar
        searchValue=""
        onSearchChange={vi.fn()}
        onSearch={vi.fn()}
        onRefresh={vi.fn()}
      >
        <select data-testid="custom-filter" />
      </FilterToolbar>,
    )
    expect(screen.getByTestId('custom-filter')).toBeInTheDocument()
  })
})
