// Toast 上下文，通过 useToast() 触发通知。
import * as React from 'react'
import {
  ToastProvider,
  ToastViewport,
  Toast,
  ToastTitle,
  ToastDescription,
  ToastClose,
  type ToastProps,
} from '@/components/ui/toast'

import { ChevronUp, Layers } from 'lucide-react'
import { cn } from '@/lib/utils'

type ToastVariant = ToastProps['variant']

interface ToastOptions {
  title?: string
  description?: string
  variant?: ToastVariant
  duration?: number
}

interface QueuedToast extends ToastOptions {
  id: number
}

interface ToastContextValue {
  toast: (opts: ToastOptions) => void
  dismiss?: (id?: number) => void
}

const ToastContext = React.createContext<ToastContextValue | undefined>(undefined)

export function ToastProviderRoot({ children }: { children: React.ReactNode }) {
  const [items, setItems] = React.useState<QueuedToast[]>([])
  const [isHovered, setIsHovered] = React.useState(false)
  const [isExpanded, setIsExpanded] = React.useState(false)
  const hoverTimerRef = React.useRef<number>()

  const expanded = isHovered || isExpanded
  const total = items.length

  const toast = React.useCallback((opts: ToastOptions) => {
    setItems((prev) => [...prev, { ...opts, id: Date.now() + Math.random() }])
  }, [])

  const remove = React.useCallback((id?: number) => {
    if (id === undefined) {
      setItems([])
      return
    }
    setItems((prev) => prev.filter((it) => it.id !== id))
  }, [])

  const clearAll = React.useCallback(() => {
    setItems([])
    setIsExpanded(false)
    setIsHovered(false)
  }, [])

  const handleMouseEnter = React.useCallback(() => {
    if (hoverTimerRef.current) {
      clearTimeout(hoverTimerRef.current)
      hoverTimerRef.current = undefined
    }
    setIsHovered(true)
  }, [])

  const handleMouseLeave = React.useCallback(() => {
    if (hoverTimerRef.current) clearTimeout(hoverTimerRef.current)
    hoverTimerRef.current = window.setTimeout(() => {
      setIsHovered(false)
    }, 200)
  }, [])

  React.useEffect(() => {
    return () => {
      if (hoverTimerRef.current) clearTimeout(hoverTimerRef.current)
    }
  }, [])

  // 数量变少时自动复位展开状态
  React.useEffect(() => {
    if (items.length <= 1) {
      setIsExpanded(false)
    }
  }, [items.length])

  return (
    <ToastContext.Provider value={{ toast, dismiss: remove }}>
      <ToastProvider>
        {children}
        {items.map((it, idx) => {
          // idx: 0 为最早发布，total - 1 为最新发布
          const offset = total - 1 - idx
          const isLatest = offset === 0

          let cardStyle: React.CSSProperties = {}
          let cardClasses = ''

          if (total > 1 && !expanded) {
            // 最新的在最表面（zIndex 最高），垂直位置在上一条下方（向下偏移更大）
            if (offset === 0) {
              cardStyle = {
                zIndex: 30,
                transform: total === 2 ? 'translateY(10px) scale(1)' : 'translateY(16px) scale(1)',
                opacity: 1,
              }
              cardClasses = 'col-start-1 row-start-1 shadow-xl'
            } else if (offset === 1) {
              cardStyle = {
                zIndex: 20,
                transform: total === 2 ? 'translateY(0) scale(0.96)' : 'translateY(8px) scale(0.96)',
                opacity: 0.85,
              }
              cardClasses = 'col-start-1 row-start-1 shadow-md pointer-events-none select-none'
            } else if (offset === 2) {
              cardStyle = {
                zIndex: 10,
                transform: 'translateY(0) scale(0.92)',
                opacity: 0.6,
              }
              cardClasses = 'col-start-1 row-start-1 shadow-sm pointer-events-none select-none'
            } else {
              cardStyle = {
                zIndex: 0,
                transform: 'translateY(0) scale(0.90)',
                opacity: 0,
                visibility: 'hidden',
              }
              cardClasses = 'col-start-1 row-start-1 pointer-events-none select-none'
            }
          } else if (expanded) {
            cardClasses = 'col-start-1 row-auto shadow-md pointer-events-auto'
          }

          return (
            <Toast
              key={it.id}
              variant={it.variant}
              duration={it.duration ?? 4000}
              style={cardStyle}
              className={cn(
                'transition-all duration-300 ease-out',
                cardClasses,
              )}
              onMouseEnter={handleMouseEnter}
              onMouseLeave={handleMouseLeave}
              onOpenChange={(open) => {
                if (!open) remove(it.id)
              }}
            >
              <div className="grid gap-1 flex-1 min-w-0 pr-2">
                {/* 展开态下的首张卡片头部操作栏 */}
                {expanded && total > 1 && idx === 0 && (
                  <div className="flex items-center justify-between pb-1.5 mb-1 border-b border-border/60">
                    <span className="flex items-center gap-1.5 text-xs font-medium text-foreground">
                      <Layers className="h-3.5 w-3.5 text-primary" />
                      共 {total} 条提醒
                    </span>
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          clearAll()
                        }}
                        className="text-[11px] text-muted-foreground hover:text-destructive transition-colors"
                      >
                        全部关闭
                      </button>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          setIsExpanded(false)
                          setIsHovered(false)
                        }}
                        className="inline-flex items-center gap-0.5 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
                      >
                        <ChevronUp className="h-3 w-3" />
                        收起
                      </button>
                    </div>
                  </div>
                )}

                <div className="flex items-center gap-2">
                  {it.title && <ToastTitle>{it.title}</ToastTitle>}
                  {/* 折叠态下：最新一条展示展开徽标 */}
                  {!expanded && total > 1 && isLatest && (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        setIsExpanded(true)
                      }}
                      className="inline-flex items-center gap-1 rounded-full border border-primary/25 bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary hover:bg-primary/20 transition-colors shrink-0"
                      title="点击展开全部提醒"
                    >
                      <Layers className="h-3 w-3" />
                      +{total - 1} 条
                    </button>
                  )}
                </div>
                {it.description && <ToastDescription>{it.description}</ToastDescription>}
              </div>
              <ToastClose />
            </Toast>
          )
        })}
        <ToastViewport
          className={cn(
            expanded ? 'flex flex-col gap-2.5 items-center' : 'grid justify-items-center',
          )}
          onMouseEnter={handleMouseEnter}
          onMouseLeave={handleMouseLeave}
        />
      </ToastProvider>
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = React.useContext(ToastContext)
  if (!ctx) throw new Error('useToast 必须在 <ToastProviderRoot> 内部使用')
  return ctx
}
