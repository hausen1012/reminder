import { HashRouter, Routes, Route } from 'react-router-dom'
import { ThemeProvider } from '@/contexts/ThemeContext'
import { AuthProvider } from '@/contexts/AuthContext'
import { ConfigProvider } from '@/contexts/ConfigContext'
import { AboutProvider } from '@/contexts/AboutContext'
import { ProtectedRoute, PublicRoute } from '@/components/common/Routing'
import { AppLayout } from '@/components/layout/Layout'
import { ToastProviderRoot } from '@/components/ui/use-toast'
import Login from '@/pages/Login'
import Dashboard from '@/pages/Dashboard'
import Profile from '@/pages/Profile'
import About from '@/pages/About'
import ChannelsPage from '@/pages/channels'
import RemindersPage from '@/pages/reminders'
import LogsPage from '@/pages/logs'
import TokensPage from '@/pages/tokens'
import SchedulerPage from '@/pages/scheduler'

export default function App() {
  return (
    <HashRouter>
      <ThemeProvider>
        <ToastProviderRoot>
          <AuthProvider>
            <ConfigProvider>
              <AboutProvider>
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
                    <Route path="tokens" element={<TokensPage />} />
                    <Route path="scheduler" element={<SchedulerPage />} />
                    <Route path="profile" element={<Profile />} />
                    <Route path="about" element={<About />} />
                  </Route>
                </Routes>
              </AboutProvider>
            </ConfigProvider>
          </AuthProvider>
        </ToastProviderRoot>
      </ThemeProvider>
    </HashRouter>
  )
}
