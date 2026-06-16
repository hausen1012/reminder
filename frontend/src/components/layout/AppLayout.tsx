import { useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Menu, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Sidebar } from './Sidebar'

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
      <main className="relative flex-1 overflow-auto bg-background p-4 md:p-8">
        {/* 移动端汉堡菜单 */}
        <div className="mb-4 md:hidden">
          <Button variant="ghost" size="icon" onClick={() => setSidebarOpen(true)} aria-label="打开菜单">
            <Menu className="h-5 w-5" />
          </Button>
        </div>
        <Outlet />
      </main>
    </div>
  )
}
