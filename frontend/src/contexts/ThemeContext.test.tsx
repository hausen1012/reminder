import { describe, expect, it, beforeEach, vi } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ThemeProvider, useTheme } from '@/contexts/ThemeContext'

// 探针组件：把 context 值渲染出来便于断言
function ThemeProbe() {
  const { theme, toggle } = useTheme()
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <button onClick={toggle}>切换</button>
    </div>
  )
}

function renderWithProvider() {
  return render(
    <ThemeProvider>
      <ThemeProbe />
    </ThemeProvider>,
  )
}

describe('ThemeContext', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.classList.remove('dark')
  })

  it('无保存记录时按系统偏好初始化', () => {
    // setup.ts 里 matchMedia 默认返回 matches=false → light
    renderWithProvider()
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
  })

  it('读取 localStorage 中已保存的主题', () => {
    localStorage.setItem('theme', 'dark')
    renderWithProvider()
    expect(screen.getByTestId('theme')).toHaveTextContent('dark')
  })

  it('忽略非法的 localStorage 值', () => {
    localStorage.setItem('theme', 'rainbow')
    renderWithProvider()
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
  })

  it('toggle 应在 light/dark 间切换', async () => {
    const user = userEvent.setup()
    renderWithProvider()

    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    await user.click(screen.getByText('切换'))
    expect(screen.getByTestId('theme')).toHaveTextContent('dark')
    await user.click(screen.getByText('切换'))
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
  })

  it('主题变化应同步到 documentElement 的 dark 类与 localStorage', async () => {
    const user = userEvent.setup()
    renderWithProvider()

    await act(async () => {
      await user.click(screen.getByText('切换'))
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
  })

  it('useTheme 在 Provider 外调用应抛错', () => {
    // 抑制 React 的错误日志噪音
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<ThemeProbe />)).toThrow('useTheme must be used within ThemeProvider')
    spy.mockRestore()
  })
})
