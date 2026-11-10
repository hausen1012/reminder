import { useLayoutEffect, useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import type { Channel } from '@/types'

interface Props {
  channels: Channel[]
  value: number[]
  open: boolean
  placeholder?: string
  emptyText?: string
  onOpenChange: (open: boolean) => void
  onChange: (value: number[]) => void
}

// 列表最大高度，同时也是判断向上弹出是否有足够空间的门槛
const LIST_MAX_HEIGHT = 256

function formatChannelNames(channels: Channel[], ids: number[], placeholder: string): string {
  if (ids.length === 0) return placeholder
  const names = ids.map((id) => channels.find((c) => c.id === id)?.name ?? `#${id}`)
  return names.join(', ')
}

export function ChannelMultiSelect({
  channels,
  value,
  open,
  placeholder = '选择通知渠道',
  emptyText = '还没有通知渠道，先到「通知」页面创建一个。',
  onOpenChange,
  onChange,
}: Props) {
  const triggerRef = useRef<HTMLButtonElement>(null)
  // 触发按钮上方空间不足时改为向下展开，避免列表被对话框裁切
  const [dropUp, setDropUp] = useState(true)

  useLayoutEffect(() => {
    if (!open || !triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    setDropUp(rect.top > LIST_MAX_HEIGHT)
  }, [open])

  function toggleChannel(id: number) {
    const has = value.includes(id)
    onChange(has ? value.filter((x) => x !== id) : [...value, id])
  }

  const unknownIds = value.filter((id) => !channels.some((c) => c.id === id))

  if (channels.length === 0 && unknownIds.length === 0) {
    return <p className="text-xs text-muted-foreground">{emptyText}</p>
  }

  return (
    <div className="relative">
      <button
        ref={triggerRef}
        type="button"
        onClick={() => onOpenChange(!open)}
        className="flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-base outline-none md:text-sm"
      >
        <span className={`truncate ${value.length > 0 ? '' : 'text-muted-foreground'}`}>
          {formatChannelNames(channels, value, placeholder)}
        </span>
        <ChevronDown className="ml-1 h-4 w-4 shrink-0 opacity-50" />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => onOpenChange(false)} />
          <div
            className={
              'absolute z-50 max-h-64 w-full overflow-y-auto scroll-area rounded-md border bg-card shadow-lg ' +
              (dropUp ? 'bottom-full mb-1' : 'top-full mt-1')
            }
          >
            {unknownIds.map((id) => (
              <label
                key={`unknown-${id}`}
                className="flex cursor-pointer items-center gap-2.5 px-3 py-2.5 text-sm hover:bg-muted active:bg-muted md:py-2"
              >
                <input
                  type="checkbox"
                  checked={value.includes(id)}
                  onChange={() => toggleChannel(id)}
                  className="h-4 w-4"
                />
                <span className="truncate">#{id}</span>
                <span className="ml-auto shrink-0 text-xs text-muted-foreground">未知</span>
              </label>
            ))}
            {channels.map((ch) => {
              const checked = value.includes(ch.id)
              return (
                <label
                  key={ch.id}
                  className="flex cursor-pointer items-center gap-2.5 px-3 py-2.5 text-sm hover:bg-muted active:bg-muted md:py-2"
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggleChannel(ch.id)}
                    className="h-4 w-4"
                  />
                  <span className="truncate">{ch.name}</span>
                  <span className="ml-auto shrink-0 text-xs text-muted-foreground">{ch.type}</span>
                </label>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}
