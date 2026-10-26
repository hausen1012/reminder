import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { ProtectedRoute } from '@/components/guards/ProtectedRoute'
import { PublicRoute } from '@/components/guards/PublicRoute'

// mock useAuth，让守卫组件按需返回不同状态
const mockAuth = vi.fn()
vi.mock('@/contexts/AuthContext', () => ({
  useAuth: () => mockAuth(),
}))

function renderWithRouter(ui: React.ReactNode, initialPath = '/') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route path="/" element={ui} />
        <Route path="/login" element={<div>登录页</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('ProtectedRoute', () => {
  it('loading 时显示加载文案', () => {
    mockAuth.mockReturnValue({ user: null, loading: true })
    renderWithRouter(
      <ProtectedRoute>
        <div>受保护内容</div>
      </ProtectedRoute>,
    )
    expect(screen.getByText('加载中...')).toBeInTheDocument()
    expect(screen.queryByText('受保护内容')).not.toBeInTheDocument()
  })

  it('未登录时重定向到 /login', () => {
    mockAuth.mockReturnValue({ user: null, loading: false })
    renderWithRouter(
      <ProtectedRoute>
        <div>受保护内容</div>
      </ProtectedRoute>,
    )
    expect(screen.getByText('登录页')).toBeInTheDocument()
    expect(screen.queryByText('受保护内容')).not.toBeInTheDocument()
  })

  it('已登录时渲染子内容', () => {
    mockAuth.mockReturnValue({ user: { id: 1, username: 'admin' }, loading: false })
    renderWithRouter(
      <ProtectedRoute>
        <div>受保护内容</div>
      </ProtectedRoute>,
    )
    expect(screen.getByText('受保护内容')).toBeInTheDocument()
  })
})

describe('PublicRoute', () => {
  it('loading 时显示加载文案', () => {
    mockAuth.mockReturnValue({ user: null, loading: true })
    renderWithRouter(
      <PublicRoute>
        <div>公开内容</div>
      </PublicRoute>,
    )
    expect(screen.getByText('加载中...')).toBeInTheDocument()
  })

  it('已登录时重定向到首页', () => {
    mockAuth.mockReturnValue({ user: { id: 1, username: 'admin' }, loading: false })
    render(
      <MemoryRouter initialEntries={['/login']}>
        <Routes>
          <Route
            path="/login"
            element={
              <PublicRoute>
                <div>公开内容</div>
              </PublicRoute>
            }
          />
          <Route path="/" element={<div>首页</div>} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('首页')).toBeInTheDocument()
    expect(screen.queryByText('公开内容')).not.toBeInTheDocument()
  })

  it('未登录时渲染子内容', () => {
    mockAuth.mockReturnValue({ user: null, loading: false })
    renderWithRouter(
      <PublicRoute>
        <div>公开内容</div>
      </PublicRoute>,
    )
    expect(screen.getByText('公开内容')).toBeInTheDocument()
  })
})
