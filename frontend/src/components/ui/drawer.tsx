// 简易 Drawer 组件（基于 sheet/dialog 模式，从底部滑入）
import { createContext, useContext, useCallback, useEffect, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { X } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface DrawerContextValue {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const DrawerContext = createContext<DrawerContextValue>({ open: false, onOpenChange: () => {} })

interface DrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: ReactNode
}

export function Drawer({ open, onOpenChange, children }: DrawerProps) {
  return (
    <DrawerContext.Provider value={{ open, onOpenChange }}>
      {children}
    </DrawerContext.Provider>
  )
}

export function DrawerContent({ children, className }: { children: ReactNode; className?: string }) {
  const { open, onOpenChange } = useContext(DrawerContext)

  // ESC 关闭
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') onOpenChange(false)
    },
    [onOpenChange],
  )

  useEffect(() => {
    if (open) document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [open, handleKeyDown])

  if (!open) return null

  return (
    <>
      {/* 遮罩 */}
      <div
        className="fixed inset-0 z-40 bg-black/50"
        onClick={() => onOpenChange(false)}
      />
      {/* 面板 */}
      <div
        className={cn(
          'fixed bottom-0 left-0 right-0 z-50 flex flex-col bg-background border-t shadow-xl rounded-t-xl',
          'animate-in slide-in-from-bottom',
          className,
        )}
      >
        <div className="flex items-center justify-between px-6 py-3 border-b">
          <div className="w-12 h-1.5 rounded-full bg-muted mx-auto" />
        </div>
        {children}
      </div>
    </>
  )
}

export function DrawerHeader({ children, className }: { children: ReactNode; className?: string }) {
  const { onOpenChange } = useContext(DrawerContext)
  return (
    <div className={cn('flex items-center justify-between px-6 py-4 border-b', className)}>
      <div>{children}</div>
      <Button variant="ghost" size="icon" onClick={() => onOpenChange(false)}>
        <X className="h-4 w-4" />
      </Button>
    </div>
  )
}

export function DrawerTitle({ children }: { children: ReactNode }) {
  return <h2 className="text-lg font-semibold">{children}</h2>
}