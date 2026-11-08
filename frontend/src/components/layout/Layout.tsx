// 应用外壳：桌面侧边栏 + 移动端顶栏/底部标签栏，两者共用同一份导航数据。
//
// 桌面（md 及以上）使用固定侧边栏；移动端使用底部标签栏承载高频页面，
// 其余入口（令牌、监控、设置、外链、账户）收进左滑抽屉。
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
  MoreHorizontal,
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
  // AI 入口启用时，主内容区底部预留滚动余量，避免右下角悬浮按钮压住分页等底部控件
  const [aiEnabled, setAiEnabled] = useState(false)
  const { theme, toggle } = useTheme()
  const { config } = useConfig()

  return (
    <div className="flex h-dvh flex-col md:flex-row">
      {/* 移动端：覆盖式抽屉 */}
      <div
        className={
          'fixed inset-0 z-50 md:hidden ' +
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
            'absolute left-0 top-0 flex h-full w-[17rem] max-w-[85vw] flex-col bg-card shadow-xl transition-transform duration-200 ' +
            (sidebarOpen ? 'translate-x-0' : '-translate-x-full')
          }
        >
          <div className="flex justify-end p-2 pt-[calc(0.5rem+env(safe-area-inset-top))]">
            <Button variant="ghost" size="icon" onClick={() => setSidebarOpen(false)} aria-label="关闭菜单">
              <X className="h-5 w-5" />
            </Button>
          </div>
          <div className="flex-1 overflow-y-auto scroll-area">
            <Sidebar onNavClick={() => setSidebarOpen(false)} />
          </div>
        </div>
      </div>

      {/* 桌面端：固定侧边栏 */}
      <div className="hidden shrink-0 md:block md:w-48">
        <Sidebar />
      </div>

      {/* 移动端：顶栏（品牌 + 主题切换） */}
      <header className="flex h-[calc(3.5rem+env(safe-area-inset-top))] shrink-0 items-center gap-2 border-b bg-background px-2 pt-[env(safe-area-inset-top)] md:hidden">
        <Button variant="ghost" size="icon" onClick={() => setSidebarOpen(true)} aria-label="打开菜单">
          <Menu className="h-5 w-5" />
        </Button>
        <div className="flex min-w-0 flex-1 items-center gap-2 px-1 font-medium text-base leading-none">
          {config.logo_svg ? (
            <AppLogo svg={config.logo_svg} className="h-6 w-auto shrink-0" />
          ) : (
            <BellRing className="h-5 w-5 shrink-0" strokeWidth={1.5} />
          )}
          <span className="truncate">{config.app_name || 'Reminder'}</span>
        </div>
        <Button
          variant="ghost"
          size="icon"
          onClick={toggle}
          aria-label={theme === 'light' ? '切换深色' : '切换浅色'}
          title={theme === 'light' ? '切换深色' : '切换浅色'}
        >
          {theme === 'light' ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
        </Button>
      </header>

      {/* 主内容区 */}
      <main
        className={cn(
          'relative min-h-0 min-w-0 flex-1 overflow-y-auto scroll-area bg-background p-4 md:p-8',
          // AI 悬浮按钮固定在右下角，会盖住列表底部的分页；启用时在主内容底部留出滚动余量，
          // 滚到底部后分页会停在按钮上方。
          aiEnabled && 'pb-24',
        )}
      >
        <Outlet />
      </main>

      {/* 移动端：底部标签栏 */}
      <nav
        aria-label="主导航"
        className="flex shrink-0 items-stretch border-t bg-card pb-[env(safe-area-inset-bottom)] md:hidden"
      >
        {mobileTabs.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) =>
              cn(
                'flex flex-1 flex-col items-center justify-center gap-0.5 py-1.5 text-[11px] leading-none transition-colors',
                isActive ? 'text-primary font-medium' : 'text-muted-foreground',
              )
            }
          >
            <item.icon className="h-5 w-5" strokeWidth={1.75} />
            {item.label}
          </NavLink>
        ))}
        <button
          type="button"
          onClick={() => setSidebarOpen(true)}
          aria-label="打开更多菜单"
          className={cn(
            'flex flex-1 flex-col items-center justify-center gap-0.5 py-1.5 text-[11px] leading-none transition-colors',
            sidebarOpen ? 'text-primary font-medium' : 'text-muted-foreground',
          )}
        >
          <MoreHorizontal className="h-5 w-5" strokeWidth={1.75} />
          更多
        </button>
      </nav>

      {/* AI 助手：右下角悬浮窗，不占导航、不进数据库 */}
      <AIChatWidget onEnabledChange={setAiEnabled} />
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

// 移动端底部标签栏：高频页面直达，其余入口走「更多」抽屉
const mobileTabs = navItems.filter((item) =>
  ['/', '/reminders', '/channels', '/logs'].includes(item.to),
)

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
    <aside className="flex h-full w-full flex-col border-r bg-card md:w-48">
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
                'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors md:py-2',
                isActive
                  ? 'bg-secondary text-secondary-foreground font-medium'
                  : 'text-muted-foreground hover:bg-secondary/50 hover:text-foreground',
              )
            }
          >
            <item.icon className="h-[18px] w-[18px] md:h-4 md:w-4" />
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
            className="group flex items-center gap-3 rounded-md px-3 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-secondary/50 hover:text-foreground md:py-2"
          >
            <item.icon className="h-[18px] w-[18px] md:h-4 md:w-4" />
            {item.label}
            <ArrowUpRight className="ml-auto h-3 w-3 opacity-40 transition-opacity group-hover:opacity-100" />
          </a>
        ))}
      </div>
      <Separator />
      <div className="flex items-center justify-between p-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] md:pb-3">
        <span className="truncate text-sm text-muted-foreground">{user?.username}</span>
        <div className="flex shrink-0 gap-1">
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
