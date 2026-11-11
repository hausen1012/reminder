// 路由守卫：登录态与未登录态访问控制。
import { Navigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from '@/contexts/AuthContext'

// LoadingGate 是守卫共用的加载态占位。
function LoadingGate() {
  return (
    <div className="flex h-screen items-center justify-center">
      <p className="text-muted-foreground">加载中...</p>
    </div>
  )
}

export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth()

  if (loading) return <LoadingGate />

  if (!user) return <Navigate to="/login" replace />
  return <>{children}</>
}

export function PublicRoute({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth()

  if (loading) return <LoadingGate />

  if (user) return <Navigate to="/" replace />
  return <>{children}</>
}
