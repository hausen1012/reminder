import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { ThemeProvider } from '@/contexts/ThemeContext'
import { AuthProvider } from '@/contexts/AuthContext'
import { ProtectedRoute } from '@/components/guards/ProtectedRoute'
import { PublicRoute } from '@/components/guards/PublicRoute'
import { AppLayout } from '@/components/layout/AppLayout'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import Login from '@/pages/Login'
import Dashboard from '@/pages/Dashboard'
import Profile from '@/pages/Profile'
import ChannelsPage from '@/pages/channels'
import RemindersPage from '@/pages/reminders'
import LogsPage from '@/pages/logs'
import ApiKeysPage from '@/pages/apikeys'

export default function App() {
  return (
    <BrowserRouter>
      <ThemeProvider>
        <ToastProviderRoot>
          <AuthProvider>
            <Routes>
              <Route
                path="/login"
                element={
                  <PublicRoute>
                    <Login />
                  </PublicRoute>
                }
              />
              <Route
                element={
                  <ProtectedRoute>
                    <AppLayout />
                  </ProtectedRoute>
                }
              >
                <Route index element={<Dashboard />} />
                <Route path="reminders" element={<RemindersPage />} />
                <Route path="logs" element={<LogsPage />} />
                <Route path="channels" element={<ChannelsPage />} />
                <Route path="apikeys" element={<ApiKeysPage />} />
                <Route path="profile" element={<Profile />} />
              </Route>
            </Routes>
          </AuthProvider>
        </ToastProviderRoot>
      </ThemeProvider>
    </BrowserRouter>
  )
}
