import { ChevronDown } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { Channel } from '@/types'
import { useChannelTypes, labelOf } from '@/lib/channelSpec'

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
  return names.join('、')
}

export function ChannelMultiSelect({
  channels,
  value,
  open,
  placeholder = '选择通知渠道',
  emptyText = '还没有通知渠道，先到「通知」页创建。',
  onOpenChange,
  onChange,
}: Props) {
  const { specs } = useChannelTypes()

  function toggleChannel(id: number) {
    const has = value.includes(id)
    onChange(has ? value.filter((x) => x !== id) : [...value, id])
  }

  const unknownIds = value.filter((id) => !channels.some((c) => c.id === id))

  if (channels.length === 0 && unknownIds.length === 0) {
    return <p className="text-xs text-muted-foreground">{emptyText}</p>
  }

  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-base outline-none transition-colors hover:bg-accent focus:ring-2 focus:ring-ring focus:ring-offset-2 md:text-sm"
        >
          <span className={`truncate ${value.length > 0 ? '' : 'text-muted-foreground'}`}>
            {formatChannelNames(channels, value, placeholder)}
          </span>
          <ChevronDown className="ml-1 h-4 w-4 shrink-0 opacity-50" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="w-[var(--radix-dropdown-menu-trigger-width)] max-h-64 overflow-y-auto scroll-area"
      >
        {unknownIds.map((id) => (
          <DropdownMenuCheckboxItem
            key={`unknown-${id}`}
            checked
            onSelect={(e) => e.preventDefault()}
            onCheckedChange={() => toggleChannel(id)}
          >
            <span className="truncate">#{id}</span>
            <span className="shrink-0 text-xs text-muted-foreground">未知</span>
          </DropdownMenuCheckboxItem>
        ))}
        {channels.map((ch) => (
          <DropdownMenuCheckboxItem
            key={ch.id}
            checked={value.includes(ch.id)}
            onSelect={(e) => e.preventDefault()}
            onCheckedChange={() => toggleChannel(ch.id)}
          >
            <span className="truncate">{ch.name}</span>
            <span className="shrink-0 text-xs text-muted-foreground">{labelOf(specs, ch.type)}</span>
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}