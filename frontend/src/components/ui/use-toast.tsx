// 极简 toast 上下文：通过 useToast() 获取 toast() 调用即可弹出通知。
// 比 shadcn 自带的 useToast 简化，避免额外的 hook 复杂度。
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
}

const ToastContext = React.createContext<ToastContextValue | undefined>(undefined)

export function ToastProviderRoot({ children }: { children: React.ReactNode }) {
  const [items, setItems] = React.useState<QueuedToast[]>([])

  const toast = React.useCallback((opts: ToastOptions) => {
    setItems((prev) => [...prev, { ...opts, id: Date.now() + Math.random() }])
  }, [])

  const remove = React.useCallback((id: number) => {
    setItems((prev) => prev.filter((it) => it.id !== id))
  }, [])

  return (
    <ToastContext.Provider value={{ toast }}>
      <ToastProvider>
        {children}
        {items.map((it) => (
          <Toast
            key={it.id}
            variant={it.variant}
            duration={it.duration ?? 4000}
            onOpenChange={(open) => {
              if (!open) remove(it.id)
            }}
          >
            <div className="grid gap-1">
              {it.title && <ToastTitle>{it.title}</ToastTitle>}
              {it.description && <ToastDescription>{it.description}</ToastDescription>}
            </div>
            <ToastClose />
          </Toast>
        ))}
        <ToastViewport />
      </ToastProvider>
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = React.useContext(ToastContext)
  if (!ctx) throw new Error('useToast 必须在 <ToastProviderRoot> 内部使用')
  return ctx
}
