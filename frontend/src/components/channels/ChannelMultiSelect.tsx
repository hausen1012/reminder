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

function formatChannelNames(channels: Channel[], ids: number[], placeholder: string): string {
  if (ids.length === 0) return placeholder
  const names = ids.map((id) => channels.find((c) => c.id === id)?.name ?? `#${id}`)
  return names.join(', ')
}

export function ChannelMultiSelect({
  channels,
  value,
  open,
  placeholder = '选择通道',
  emptyText = '还没有通道，先到「通知」页面创建一个。',
  onOpenChange,
  onChange,
}: Props) {
  function toggleChannel(id: number) {
    const has = value.includes(id)
    onChange(has ? value.filter((x) => x !== id) : [...value, id])
  }

  if (channels.length === 0) {
    return <p className="text-xs text-muted-foreground">{emptyText}</p>
  }

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => onOpenChange(!open)}
        className="flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      >
        <span className={`truncate ${value.length > 0 ? '' : 'text-muted-foreground'}`}>
          {formatChannelNames(channels, value, placeholder)}
        </span>
        <ChevronDown className="ml-1 h-4 w-4 shrink-0 opacity-50" />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => onOpenChange(false)} />
          <div className="absolute bottom-full z-50 mb-1 max-h-64 w-full overflow-y-auto rounded-md border bg-card shadow-lg">
            {channels.map((ch) => {
              const checked = value.includes(ch.id)
              return (
                <label
                  key={ch.id}
                  className="flex cursor-pointer items-center gap-2 px-3 py-2 text-sm hover:bg-muted first:rounded-t-md last:rounded-b-md"
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggleChannel(ch.id)}
                    className="h-4 w-4"
                  />
                  <span>{ch.name}</span>
                  <span className="ml-auto text-xs text-muted-foreground">{ch.type}</span>
                </label>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}
