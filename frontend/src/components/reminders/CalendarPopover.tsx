// CalendarPopover 是弹出日历组件，定位在触发元素下方
// 始终展示公历网格，农历切换仅在单元格中额外显示农历日期
import { useState, useMemo, useRef, useEffect, useLayoutEffect } from 'react'
import { createPortal } from 'react-dom'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ChevronLeft, ChevronRight, Clock3 } from 'lucide-react'
import { Solar } from 'lunar-typescript'
import type { CalendarResult } from '@/types'

interface Props {
  date?: string
  hour?: number
  minute?: number
  initialCalendar?: 'solar' | 'lunar'
  triggerRef: React.RefObject<HTMLDivElement | null>
  onSelect: (result: CalendarResult) => void
  onClose: () => void
}

const WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日']

// 将 JS getDay() (周日=0) 转为周一=0..周日=6
function toMondayBase(jsWeekday: number): number {
  return jsWeekday === 0 ? 6 : jsWeekday - 1
}

// 年份从今年开始，往后 70 年
const YEAR_OPTIONS = Array.from({ length: 70 }, (_, i) => new Date().getFullYear() + i)

interface DayCell {
  key: string
  day: number
  month: number
  year: number
  lunarDayName: string
  current: boolean
}

export function CalendarPopover({ date, hour: initHour, minute: initMin, initialCalendar = 'solar', triggerRef, onSelect, onClose }: Props) {
  const now = new Date()
  const popoverRef = useRef<HTMLDivElement>(null)
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null)
  const hourListRef = useRef<HTMLDivElement>(null)
  const minuteListRef = useRef<HTMLDivElement>(null)
  const onSelectRef = useRef(onSelect)
  const hasSyncedRef = useRef(false)
  const [showLunar, setShowLunar] = useState(initialCalendar === 'lunar')
  const [solarYear, setSolarYear] = useState(() => {
    if (date) return new Date(date).getFullYear()
    return now.getFullYear()
  })
  const [solarMonth, setSolarMonth] = useState(() => {
    if (date) return new Date(date).getMonth() + 1
    return now.getMonth() + 1
  })
  const [selectedDay, setSelectedDay] = useState<number | null>(() => {
    if (date) return new Date(date).getDate()
    return now.getDate()
  })
  const [hour, setHour] = useState(initHour ?? now.getHours())
  const [minute, setMinute] = useState(initMin ?? now.getMinutes())
  const [timePanelOpen, setTimePanelOpen] = useState(false)

  // 计算 fixed 位置（使用 useLayoutEffect 避免首次渲染时位置闪烁）
  useLayoutEffect(() => {
    if (!triggerRef.current) return

    function calcPosition() {
      const rect = triggerRef.current!.getBoundingClientRect()
      const popupHeight = 360
      const popupWidth = 272
      const top = rect.bottom + 4
      // 水平居中于触发元素
      const centerX = rect.left + rect.width / 2
      const left = Math.max(8, Math.min(centerX - popupWidth / 2, window.innerWidth - popupWidth - 8))
      if (top + popupHeight > window.innerHeight) {
        setPosition({ top: rect.top - popupHeight - 4, left })
      } else {
        setPosition({ top, left })
      }
    }

    calcPosition()

    window.addEventListener('scroll', calcPosition, true)
    window.addEventListener('resize', calcPosition)

    return () => {
      window.removeEventListener('scroll', calcPosition, true)
      window.removeEventListener('resize', calcPosition)
    }
  }, [triggerRef, setPosition])

  // 点击外部关闭（排除 Radix Select portal）
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      const target = e.target as Node
      if (!popoverRef.current) return
      if (popoverRef.current.contains(target)) return
      if (target instanceof Element && (target.closest('[role="listbox"]') || target.closest('[data-radix-popper-content-wrapper]'))) return
      onClose()
    }
    const timer = setTimeout(() => {
      document.addEventListener('mousedown', handleClick)
    }, 0)
    return () => {
      clearTimeout(timer)
      document.removeEventListener('mousedown', handleClick)
    }
  }, [onClose])

  useEffect(() => {
    onSelectRef.current = onSelect
  }, [onSelect])

  useEffect(() => {
    if (!timePanelOpen) return

    function scrollSelectedIntoView(container: HTMLDivElement | null, index: number) {
      if (!container) return
      const item = container.children[index] as HTMLElement | undefined
      if (!item) return
      const offset = item.offsetTop - (container.clientHeight - item.offsetHeight) / 2
      container.scrollTop = Math.max(0, offset)
    }

    scrollSelectedIntoView(hourListRef.current, hour)
    scrollSelectedIntoView(minuteListRef.current, minute)
  }, [timePanelOpen, hour, minute])

  function buildResult(): CalendarResult | null {
    if (selectedDay === null) return null
    const dateStr = `${solarYear}-${String(solarMonth).padStart(2, '0')}-${String(selectedDay).padStart(2, '0')}T${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
    if (showLunar) {
      const solar = Solar.fromYmd(solarYear, solarMonth, selectedDay)
      const lunar = solar.getLunar()
      return { date: dateStr, calendar: 'lunar', lunar: { year: lunar.getYear(), month: lunar.getMonth(), day: lunar.getDay() }, hour, minute }
    }
    return { date: dateStr, calendar: 'solar', hour, minute }
  }

  useEffect(() => {
    if (!hasSyncedRef.current) {
      hasSyncedRef.current = true
      return
    }
    const result = buildResult()
    if (result) onSelectRef.current(result)
  }, [solarYear, solarMonth, selectedDay, hour, minute, showLunar])

  // 公历模式网格
  const solarGrid = useMemo(() => {
    const daysInMonth = new Date(solarYear, solarMonth, 0).getDate()
    const firstWeekday = toMondayBase(new Date(solarYear, solarMonth - 1, 1).getDay())

    const prevYear = solarMonth === 1 ? solarYear - 1 : solarYear
    const prevMonth = solarMonth === 1 ? 12 : solarMonth - 1
    const prevDaysInMonth = new Date(prevYear, prevMonth, 0).getDate()

    const cells: DayCell[] = []

    for (let i = firstWeekday - 1; i >= 0; i--) {
      const d = prevDaysInMonth - i
      const solar = Solar.fromYmd(prevYear, prevMonth, d)
      cells.push({
        key: `p-${d}`,
        day: d,
        month: prevMonth,
        year: prevYear,
        lunarDayName: solar.getLunar().getDayInChinese(),
        current: false,
      })
    }

    for (let d = 1; d <= daysInMonth; d++) {
      const solar = Solar.fromYmd(solarYear, solarMonth, d)
      cells.push({
        key: `s-${d}`,
        day: d,
        month: solarMonth,
        year: solarYear,
        lunarDayName: solar.getLunar().getDayInChinese(),
        current: true,
      })
    }

    const nextYear = solarMonth === 12 ? solarYear + 1 : solarYear
    const nextMonth = solarMonth === 12 ? 1 : solarMonth + 1
    const remainder = cells.length % 7
    if (remainder > 0) {
      for (let d = 1; d <= 7 - remainder; d++) {
        const solar = Solar.fromYmd(nextYear, nextMonth, d)
        cells.push({
          key: `n-${d}`,
          day: d,
          month: nextMonth,
          year: nextYear,
          lunarDayName: solar.getLunar().getDayInChinese(),
          current: false,
        })
      }
    }

    return cells
  }, [solarYear, solarMonth])

  function handleConfirm() {
    const result = buildResult()
    if (result) onSelectRef.current(result)
    onClose()
  }

  function prevMonth() {
    if (solarMonth === 1) {
      setSolarYear(y => y - 1)
      setSolarMonth(12)
    } else {
      setSolarMonth(m => m - 1)
    }
  }

  function nextMonth() {
    if (solarMonth === 12) {
      setSolarYear(y => y + 1)
      setSolarMonth(1)
    } else {
      setSolarMonth(m => m + 1)
    }
  }

  function goToday() {
    const n = new Date()
    setSolarYear(n.getFullYear())
    setSolarMonth(n.getMonth() + 1)
    setSelectedDay(n.getDate())
    setHour(n.getHours())
    setMinute(n.getMinutes())
  }

  function toggleTimePanel() {
    setTimePanelOpen((open) => !open)
  }

  function handleHourSelect(value: number) {
    setHour(value)
  }

  function handleMinuteSelect(value: number) {
    setMinute(value)
    setTimePanelOpen(false)
  }

  const popoverContent = (
    <div
      ref={popoverRef}
      className="fixed z-[100] bg-card rounded-lg border shadow-lg w-[272px] max-w-[calc(100vw-2rem)] p-2.5"
      data-calendar-popover
      style={position ? { top: position.top, left: position.left, pointerEvents: 'auto' as React.CSSProperties['pointerEvents'] } : { top: -9999, left: -9999 }}
    >
      {/* 顶部导航 */}
      <div className="flex items-center justify-between mb-1.5">
        <div className="flex items-center gap-0.5">
          <Button type="button" variant="ghost" size="icon" onClick={prevMonth} className="h-6 w-6 shrink-0">
            <ChevronLeft className="h-3 w-3" />
          </Button>
          <Select value={String(solarYear)} onValueChange={(v) => setSolarYear(Number(v))}>
            <SelectTrigger className="h-6 text-[11px] border-0 bg-transparent hover:bg-muted px-1 w-14 [&>svg]:h-3 [&>svg]:w-3">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="z-[110] max-h-[220px] overflow-y-auto [scrollbar-width:thin]" side="bottom" align="start" sideOffset={2} position="popper">
              {YEAR_OPTIONS.map((y) => (
                <SelectItem key={y} value={String(y)} className="text-xs">{y}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={String(solarMonth)} onValueChange={(v) => setSolarMonth(Number(v))}>
            <SelectTrigger className="h-6 text-[11px] border-0 bg-transparent hover:bg-muted px-1 w-10 [&>svg]:h-3 [&>svg]:w-3">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="z-[110] max-h-[220px] overflow-y-auto [scrollbar-width:thin]" side="bottom" align="start" sideOffset={2} position="popper">
              {Array.from({ length: 12 }, (_, i) => i + 1).map((m) => (
                <SelectItem key={m} value={String(m)} className="text-xs">{m}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button type="button" variant="ghost" size="icon" onClick={nextMonth} className="h-6 w-6 shrink-0">
            <ChevronRight className="h-3 w-3" />
          </Button>
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={goToday}
            className="h-5 px-1.5 text-[11px] text-muted-foreground hover:text-foreground hover:bg-muted rounded transition-colors"
          >
            现在
          </button>
          <label className="flex items-center gap-1 cursor-pointer shrink-0">
            <span className="text-[11px] text-muted-foreground select-none">农历</span>
            <button
              type="button"
              role="switch"
              aria-checked={showLunar}
              onClick={() => setShowLunar(!showLunar)}
              className={`relative inline-flex h-4 w-[26px] shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ${showLunar ? 'bg-primary' : 'bg-input'}`}
            >
              <span className={`block h-3 w-3 rounded-full bg-background shadow-sm ring-0 transition-transform ${showLunar ? 'translate-x-[11px]' : 'translate-x-0'}`} />
            </button>
          </label>
        </div>
      </div>

      {/* 星期头 + 日期网格（带切换动画） */}
      <div key={`body-${solarYear}-${solarMonth}`} style={{ animation: 'cfade 0.15s ease-out' }}>
        <style>{`@keyframes cfade{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:translateY(0)}}`}</style>
        <div className="grid grid-cols-7 mb-0.5">
          {WEEKDAYS.map((w) => (
            <div key={w} className="text-center text-[10px] text-muted-foreground py-0.5">{w}</div>
          ))}
        </div>

        <div className="grid grid-cols-7">
        {solarGrid.map((cell) => {
          const isSelected = cell.current && cell.day === selectedDay
          return (
            <button
              key={cell.key}
              type="button"
              className={`h-8 flex flex-col items-center justify-center rounded text-[10px] leading-tight transition-colors
                ${isSelected ? 'bg-primary text-primary-foreground' : cell.current ? 'hover:bg-muted' : 'text-muted-foreground/40 hover:text-muted-foreground/60'}`}
              onClick={() => {
                if (cell.current) {
                  setSelectedDay(cell.day)
                } else {
                  setSolarYear(cell.year)
                  setSolarMonth(cell.month)
                  setSelectedDay(cell.day)
                }
              }}
            >
              <span className="text-[11px] font-medium leading-none">{cell.day}</span>
              {showLunar && (
                <span className={`text-[8px] leading-none mt-[1px] ${isSelected ? 'text-primary-foreground/70' : 'text-muted-foreground/50'}`}>
                  {cell.lunarDayName}
                </span>
              )}
            </button>
          )
        })}
        </div>
      </div>

      {/* 底部：时间 + 操作按钮 */}
      <div className="mt-2 border-t pt-2">
        <div className="flex items-center justify-between gap-2">
          <div className="relative">
            <button
              type="button"
              onClick={toggleTimePanel}
              className="flex items-center gap-1 rounded-md border bg-background px-2 py-1 text-sm leading-none transition-colors hover:bg-muted"
            >
              <Clock3 className="h-3.5 w-3.5 text-muted-foreground" />
              <span className="font-medium tracking-tight text-foreground">{String(hour).padStart(2, '0')}:{String(minute).padStart(2, '0')}</span>
            </button>
            {timePanelOpen && (
              <div className="absolute bottom-full left-0 z-20 mb-1 w-[108px] overflow-hidden rounded-md border bg-card shadow-lg">
                <div className="grid grid-cols-2">
                  <div className="border-r bg-background/40">
                    <div ref={hourListRef} className="max-h-40 overflow-y-auto py-0.5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                      {Array.from({ length: 24 }, (_, i) => (
                        <button
                          key={i}
                          type="button"
                          onClick={() => handleHourSelect(i)}
                          className={`block w-full px-1 py-1 text-center text-[13px] leading-none transition-colors ${hour === i ? 'bg-muted font-medium text-foreground' : 'hover:bg-muted/70'}`}
                        >
                          {String(i).padStart(2, '0')}
                        </button>
                      ))}
                    </div>
                  </div>
                  <div className="bg-background/40">
                    <div ref={minuteListRef} className="max-h-40 overflow-y-auto py-0.5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                      {Array.from({ length: 60 }, (_, i) => (
                        <button
                          key={i}
                          type="button"
                          onClick={() => handleMinuteSelect(i)}
                          className={`block w-full px-1 py-1 text-center text-[13px] leading-none transition-colors ${minute === i ? 'bg-muted font-medium text-foreground' : 'hover:bg-muted/70'}`}
                        >
                          {String(i).padStart(2, '0')}
                        </button>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>
          <div className="flex gap-1">
            <Button type="button" onClick={onClose} className="h-6 px-2 text-xs" variant="outline">取消</Button>
            <Button type="button" onClick={handleConfirm} className="h-6 px-2 text-xs" disabled={selectedDay === null}>确定</Button>
          </div>
        </div>
      </div>
    </div>
  )

  return createPortal(popoverContent, document.body)
}