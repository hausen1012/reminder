import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'
import { login as apiLogin, getMe, logout as apiLogout } from '@/lib/api'
import type { User } from '@/types'

interface AuthContextValue {
  user: User | null
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  updateUser: (user: User) => void
}

export const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      setLoading(false)
      return
    }
    getMe()
      .then((res) => setUser(res.data))
      .catch(() => localStorage.removeItem('token'))
      .finally(() => setLoading(false))
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const res = await apiLogin(username, password)
    localStorage.setItem('token', res.data.token)
    setUser(res.data.user)
  }, [])

  const logout = useCallback(async () => {
    try {
      await apiLogout()
    } catch {
      // ignore server errors during logout
    }
    localStorage.removeItem('token')
    setUser(null)
  }, [])

  const updateUser = useCallback((u: User) => setUser(u), [])

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, updateUser }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
