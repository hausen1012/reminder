import { NavLink } from 'react-router-dom'
import { LayoutDashboard, User, Moon, Sun, LogOut, BellRing, Send, Bell, ScrollText, Key, Activity } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useTheme } from '@/contexts/ThemeContext'
import { useAuth } from '@/contexts/AuthContext'
import { useConfig } from '@/contexts/ConfigContext'
import { AppLogo } from '@/components/ui/AppLogo'
import { cn } from '@/lib/utils'

const navItems = [
  { to: '/', label: '首页', icon: LayoutDashboard },
  { to: '/reminders', label: '提醒', icon: Bell },
  { to: '/channels', label: '通知', icon: Send },
  { to: '/logs', label: '日志', icon: ScrollText },
  { to: '/tokens', label: '令牌', icon: Key },
  { to: '/scheduler', label: '监控', icon: Activity },
  { to: '/profile', label: '设置', icon: User },
]

interface SidebarProps {
  onNavClick?: () => void
}

export function Sidebar({ onNavClick }: SidebarProps) {
  const { theme, toggle } = useTheme()
  const { user, logout } = useAuth()
  const { config } = useConfig()

  return (
    <aside className="flex h-full w-48 flex-col border-r bg-card md:h-screen">
      <div className="flex h-14 items-center gap-2 px-6 font-medium text-base leading-none">
        {config.logo_svg ? (
          <AppLogo svg={config.logo_svg} className="h-6 w-auto shrink-0" />
        ) : (
          <BellRing className="h-5 w-5 shrink-0" strokeWidth={1.5} />
        )}
        <span className="truncate">{config.app_name || 'Reminder'}</span>
      </div>
      <Separator />
      <nav className="flex-1 space-y-1 p-3">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            onClick={onNavClick}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors md:py-2',
                isActive
                  ? 'bg-secondary text-secondary-foreground font-medium'
                  : 'text-muted-foreground hover:bg-secondary/50 hover:text-foreground',
              )
            }
          >
            <item.icon className="h-4 w-4" />
            {item.label}
          </NavLink>
        ))}
      </nav>
      <Separator />
      <div className="flex items-center justify-between p-3">
        <span className="truncate text-sm text-muted-foreground">{user?.username}</span>
        <div className="flex gap-1 shrink-0">
          <Button variant="ghost" size="icon" onClick={toggle} title={theme === 'light' ? '切换深色' : '切换浅色'}>
            {theme === 'light' ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
          </Button>
          <Button variant="ghost" size="icon" onClick={logout} title="退出登录">
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </aside>
  )
}
