// 应用外壳：侧边栏 + 主内容区。
//
// AppLayout 管桌面固定栏与移动端抽屉的切换，Sidebar 只负责导航内容，
// 两者共用同一份导航项与主题/用户状态，因此放在一个文件里。
import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import {
  Activity,
  ArrowUpRight,
  Bell,
  BellRing,
  BookOpen,
  Github,
  Key,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  ScrollText,
  Send,
  Sun,
  User,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { useTheme } from '@/contexts/ThemeContext'
import { useAuth } from '@/contexts/AuthContext'
import { useConfig } from '@/contexts/ConfigContext'
import { AppLogo } from '@/components/common/AppLogo'
import { AIChatWidget } from '@/components/ai/AIChatWidget'
import { DOCS_URL, GITHUB_URL } from '@/lib/constants'
import { cn } from '@/lib/utils'

export function AppLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)

  return (
    <div className="flex h-screen">
      {/* 移动端：覆盖式抽屉 */}
      <div
        className={
          'fixed inset-0 z-40 md:hidden ' +
          (sidebarOpen
            ? 'visible opacity-100 transition-opacity duration-200'
            : 'invisible opacity-0 transition-opacity duration-200')
        }
      >
        {/* 遮罩层 */}
        <div
          className="absolute inset-0 bg-black/60"
          onClick={() => setSidebarOpen(false)}
        />
        {/* 抽屉面板 */}
        <div
          className={
            'absolute left-0 top-0 flex h-full w-64 flex-col bg-card shadow-lg transition-transform duration-200 ' +
            (sidebarOpen ? 'translate-x-0' : '-translate-x-full')
          }
        >
          {/* 关闭按钮 */}
          <div className="flex justify-end p-2">
            <Button variant="ghost" size="icon" onClick={() => setSidebarOpen(false)} aria-label="关闭菜单">
              <X className="h-5 w-5" />
            </Button>
          </div>
          <div className="flex-1 overflow-hidden">
            <Sidebar onNavClick={() => setSidebarOpen(false)} />
          </div>
        </div>
      </div>

      {/* 桌面端：固定侧边栏 */}
      <div className="hidden md:block">
        <Sidebar />
      </div>

      {/* 主内容区 */}
      <main className="relative flex-1 overflow-y-auto bg-background p-4 md:p-8">
        {/* 移动端汉堡菜单 */}
        <div className="mb-4 md:hidden">
          <Button variant="ghost" size="icon" onClick={() => setSidebarOpen(true)} aria-label="打开菜单">
            <Menu className="h-5 w-5" />
          </Button>
        </div>
        <Outlet />
      </main>

      {/* AI 助手：右下角悬浮窗，不占导航、不进数据库 */}
      <AIChatWidget />
    </div>
  )
}

interface SidebarProps {
  onNavClick?: () => void
}

const navItems = [
  { to: '/', label: '首页', icon: LayoutDashboard },
  { to: '/reminders', label: '提醒', icon: Bell },
  { to: '/channels', label: '通知', icon: Send },
  { to: '/logs', label: '日志', icon: ScrollText },
  { to: '/tokens', label: '令牌', icon: Key },
  { to: '/scheduler', label: '监控', icon: Activity },
  { to: '/profile', label: '设置', icon: User },
]

// 站外链接：放在导航区末尾，用分隔线与功能页区分，右侧箭头提示会打开新窗口
const externalLinks = [
  { href: DOCS_URL, label: '文档', icon: BookOpen },
  { href: GITHUB_URL, label: 'GitHub', icon: Github },
]

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
      {/* 站外链接：文档与开源仓库，样式与导航项一致但右侧带外链箭头 */}
      <div className="space-y-1 p-3">
        {externalLinks.map((item) => (
          <a
            key={item.href}
            href={item.href}
            target="_blank"
            rel="noreferrer noopener"
            onClick={onNavClick}
            className="group flex items-center gap-3 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-secondary/50 hover:text-foreground"
          >
            <item.icon className="h-4 w-4" />
            {item.label}
            <ArrowUpRight className="ml-auto h-3 w-3 opacity-40 transition-opacity group-hover:opacity-100" />
          </a>
        ))}
      </div>
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
