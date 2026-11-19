import { useState, useMemo, useRef, useEffect } from 'react'
import { Check, ChevronDown, Search, X } from 'lucide-react'
import { COMMON_TIMEZONES } from '@/lib/constants'
import { getAllTimezones } from '@/lib/datetime'
import { cn } from '@/lib/utils'

interface TimezoneSelectProps {
  id?: string
  value: string
  onValueChange: (value: string) => void
  disabled?: boolean
  className?: string
  side?: 'top' | 'bottom'
}

/** 站点时区下拉选择器，支持全量时区检索与常用时区置顶。 */
export function TimezoneSelect({
  id,
  value,
  onValueChange,
  disabled = false,
  className,
  side = 'top',
}: TimezoneSelectProps) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const allTimezones = useMemo(() => getAllTimezones(value), [value])
  const commonSet = useMemo(() => new Set(COMMON_TIMEZONES), [])
  const otherTimezones = useMemo(
    () => allTimezones.filter((tz) => !commonSet.has(tz)),
    [allTimezones, commonSet],
  )

  // 展开时自动聚焦搜索输入框
  useEffect(() => {
    if (open) {
      inputRef.current?.focus()
    } else {
      setSearch('')
    }
  }, [open])

  // 点击外部或按 Escape 时收起下拉
  useEffect(() => {
    if (!open) return

    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false)
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return null
    return allTimezones.filter((tz) => tz.toLowerCase().includes(q))
  }, [allTimezones, search])

  const handleSelect = (tz: string) => {
    onValueChange(tz)
    setOpen(false)
  }

  return (
    <div ref={containerRef} className={cn('relative w-full', className)}>
      <button
        id={id}
        type="button"
        disabled={disabled}
        onClick={() => setOpen((prev) => !prev)}
        className={cn(
          'flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-base md:text-sm outline-none transition-colors placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50 text-left',
          open && 'ring-2 ring-ring ring-offset-2 ring-offset-background',
        )}
      >
        <span className="truncate">{value || '请选择时区'}</span>
        <ChevronDown
          className={cn(
            'h-4 w-4 shrink-0 opacity-50 transition-transform duration-200',
            open && 'rotate-180',
          )}
        />
      </button>

      {open && (
        <div
          className={cn(
            'absolute left-0 z-50 max-h-80 w-full min-w-[14rem] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95',
            side === 'top' ? 'bottom-full mb-1' : 'top-full mt-1',
          )}
        >
          <div className="flex items-center border-b px-2.5 py-2">
            <Search className="mr-2 h-4 w-4 shrink-0 opacity-50" />
            <input
              ref={inputRef}
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索时区（如 Shanghai、London）…"
              className="flex h-7 w-full rounded-md bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            {search && (
              <button
                type="button"
                onClick={() => setSearch('')}
                className="rounded p-0.5 hover:bg-muted text-muted-foreground"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>

          <div className="max-h-60 overflow-y-auto p-1 text-sm [scrollbar-width:thin]">
            {filtered !== null ? (
              filtered.length === 0 ? (
                <div className="py-6 text-center text-xs text-muted-foreground">
                  未找到匹配的时区
                </div>
              ) : (
                filtered.map((tz) => (
                  <button
                    key={tz}
                    type="button"
                    onClick={() => handleSelect(tz)}
                    className={cn(
                      'relative flex w-full cursor-pointer select-none items-center justify-between rounded-sm px-2.5 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground text-left',
                      tz === value && 'bg-accent/60 font-medium',
                    )}
                  >
                    <span>{tz}</span>
                    {tz === value && <Check className="h-4 w-4 shrink-0 text-primary" />}
                  </button>
                ))
              )
            ) : (
              <>
                <div className="px-2.5 py-1 text-xs font-semibold text-muted-foreground">
                  常用时区
                </div>
                {COMMON_TIMEZONES.map((tz) => (
                  <button
                    key={tz}
                    type="button"
                    onClick={() => handleSelect(tz)}
                    className={cn(
                      'relative flex w-full cursor-pointer select-none items-center justify-between rounded-sm px-2.5 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground text-left',
                      tz === value && 'bg-accent/60 font-medium',
                    )}
                  >
                    <span>{tz}</span>
                    {tz === value && <Check className="h-4 w-4 shrink-0 text-primary" />}
                  </button>
                ))}

                {otherTimezones.length > 0 && (
                  <>
                    <div className="my-1 h-px bg-muted" />
                    <div className="px-2.5 py-1 text-xs font-semibold text-muted-foreground">
                      全部时区
                    </div>
                    {otherTimezones.map((tz) => (
                      <button
                        key={tz}
                        type="button"
                        onClick={() => handleSelect(tz)}
                        className={cn(
                          'relative flex w-full cursor-pointer select-none items-center justify-between rounded-sm px-2.5 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground text-left',
                          tz === value && 'bg-accent/60 font-medium',
                        )}
                      >
                        <span>{tz}</span>
                        {tz === value && <Check className="h-4 w-4 shrink-0 text-primary" />}
                      </button>
                    ))}
                  </>
                )}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
