import { describe, expect, it, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider, useAuth } from '@/contexts/AuthContext'

// 只 mock API 层
const apiLogin = vi.fn()
const apiGetMe = vi.fn()
const apiLogout = vi.fn()
vi.mock('@/lib/api', () => ({
  login: (u: string, p: string) => apiLogin(u, p),
  getMe: () => apiGetMe(),
  logout: () => apiLogout(),
}))

function AuthProbe() {
  const { user, loading, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="loading">{String(loading)}</span>
      <span data-testid="user">{user?.username ?? 'none'}</span>
      <button onClick={() => login('admin', 'admin123')}>登录</button>
      <button onClick={() => logout()}>登出</button>
    </div>
  )
}

function renderWithProvider() {
  return render(
    <AuthProvider>
      <AuthProbe />
    </AuthProvider>,
  )
}

describe('AuthContext', () => {
  beforeEach(() => {
    localStorage.clear()
    apiLogin.mockReset()
    apiGetMe.mockReset()
    apiLogout.mockReset()
  })

  it('无 token 时不请求 /me，直接结束 loading', async () => {
    renderWithProvider()
    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })
    expect(apiGetMe).not.toHaveBeenCalled()
    expect(screen.getByTestId('user')).toHaveTextContent('none')
  })

  it('有 token 时拉取当前用户', async () => {
    localStorage.setItem('token', 'tk')
    apiGetMe.mockResolvedValue({ data: { id: 1, username: 'admin' } })

    renderWithProvider()

    await waitFor(() => {
      expect(screen.getByTestId('user')).toHaveTextContent('admin')
    })
    expect(screen.getByTestId('loading')).toHaveTextContent('false')
  })

  it('token 失效时清除 localStorage 并保持未登录', async () => {
    localStorage.setItem('token', 'bad-token')
    apiGetMe.mockRejectedValue(new Error('401'))

    renderWithProvider()

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })
    expect(localStorage.getItem('token')).toBeNull()
    expect(screen.getByTestId('user')).toHaveTextContent('none')
  })

  it('登录成功后写入 token 并设置用户', async () => {
    apiLogin.mockResolvedValue({
      data: { token: 'new-token', user: { id: 2, username: 'tester' } },
    })
    const user = userEvent.setup()
    renderWithProvider()

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })
    await user.click(screen.getByText('登录'))

    await waitFor(() => {
      expect(screen.getByTestId('user')).toHaveTextContent('tester')
    })
    expect(localStorage.getItem('token')).toBe('new-token')
  })

  it('登录失败时向上抛错且不写 token', async () => {
    apiLogin.mockRejectedValue(new Error('密码错误'))
    const user = userEvent.setup()

    // 捕获未处理的 rejection
    const onRejection = vi.fn()
    render(
      <AuthProvider>
        <div>
          <AuthRejectProbe onError={onRejection} />
        </div>
      </AuthProvider>,
    )

    await user.click(screen.getByText('触发登录'))
    await waitFor(() => expect(onRejection).toHaveBeenCalled())
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('登出应清除 token 与用户', async () => {
    localStorage.setItem('token', 'tk')
    apiGetMe.mockResolvedValue({ data: { id: 1, username: 'admin' } })
    apiLogout.mockResolvedValue({})

    const user = userEvent.setup()
    renderWithProvider()

    await waitFor(() => {
      expect(screen.getByTestId('user')).toHaveTextContent('admin')
    })

    await user.click(screen.getByText('登出'))

    await waitFor(() => {
      expect(screen.getByTestId('user')).toHaveTextContent('none')
    })
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('登出接口失败也应清除本地状态', async () => {
    localStorage.setItem('token', 'tk')
    apiGetMe.mockResolvedValue({ data: { id: 1, username: 'admin' } })
    apiLogout.mockRejectedValue(new Error('服务端挂了'))

    const user = userEvent.setup()
    renderWithProvider()

    await waitFor(() => {
      expect(screen.getByTestId('user')).toHaveTextContent('admin')
    })

    await user.click(screen.getByText('登出'))

    await waitFor(() => {
      expect(screen.getByTestId('user')).toHaveTextContent('none')
    })
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('useAuth 在 Provider 外调用应抛错', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<AuthProbe />)).toThrow('useAuth must be used within AuthProvider')
    spy.mockRestore()
  })
})

// 用于验证登录失败向上抛错的探针
function AuthRejectProbe({ onError }: { onError: () => void }) {
  const { login } = useAuth()
  return (
    <button
      onClick={() => {
        login('admin', 'wrong').catch(() => onError())
      }}
    >
      触发登录
    </button>
  )
}
