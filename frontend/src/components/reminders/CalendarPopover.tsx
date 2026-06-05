// CalendarPopover 是弹出日历组件，定位在触发元素下方
// 始终展示公历网格，农历切换仅在单元格中额外显示农历日期
import { useState, useMemo, useRef, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Solar } from 'lunar-typescript'
import type { CalendarResult } from '@/types'

interface Props {
  date?: string
  hour?: number
  minute?: number
  onSelect: (result: CalendarResult) => void
  onClose: () => void
}

const WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日']

// 将 JS getDay() (周日=0) 转为周一=0..周日=6
function toMondayBase(jsWeekday: number): number {
  return jsWeekday === 0 ? 6 : jsWeekday - 1
}

const YEAR_OPTIONS = Array.from({ length: 31 }, (_, i) => 2020 + i)

interface DayCell {
  key: string
  solarDay: number
  lunarDayName: string
  current: boolean
}

export function CalendarPopover({ date, hour: initHour, minute: initMin, onSelect, onClose }: Props) {
  const now = new Date()
  const popoverRef = useRef<HTMLDivElement>(null)
  const [showLunar, setShowLunar] = useState(false)
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
  const [hour, setHour] = useState(initHour ?? 9)
  const [minute, setMinute] = useState(initMin ?? 0)

  // 点击外部关闭
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        onClose()
      }
    }
    // 延迟添加避免触发打开弹窗的点击事件
    const timer = setTimeout(() => {
      document.addEventListener('mousedown', handleClick)
    }, 0)
    return () => {
      clearTimeout(timer)
      document.removeEventListener('mousedown', handleClick)
    }
  }, [onClose])

  // 公历模式网格
  const solarGrid = useMemo(() => {
    const daysInMonth = new Date(solarYear, solarMonth, 0).getDate()
    const firstWeekday = toMondayBase(new Date(solarYear, solarMonth - 1, 1).getDay())
    const cells: (DayCell | null)[] = []

    for (let i = 0; i < firstWeekday; i++) {
      cells.push(null)
    }

    for (let d = 1; d <= daysInMonth; d++) {
      const solar = Solar.fromYmd(solarYear, solarMonth, d)
      cells.push({
        key: `s-${d}`,
        solarDay: d,
        lunarDayName: solar.getLunar().getDayInChinese(),
        current: true,
      })
    }

    return cells
  }, [solarYear, solarMonth])

  function handleConfirm() {
    if (selectedDay === null) return
    const dateStr = `${solarYear}-${String(solarMonth).padStart(2, '0')}-${String(selectedDay).padStart(2, '0')}T${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
    onSelect({ date: dateStr, calendar: 'solar', hour, minute })
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

  return (
    <div
      ref={popoverRef}
      className="absolute top-full left-0 z-50 mt-1 bg-card rounded-lg border shadow-lg w-[272px] p-2.5"
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
            <SelectContent>
              {YEAR_OPTIONS.map((y) => (
                <SelectItem key={y} value={String(y)} className="text-xs">{y}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={String(solarMonth)} onValueChange={(v) => setSolarMonth(Number(v))}>
            <SelectTrigger className="h-6 text-[11px] border-0 bg-transparent hover:bg-muted px-1 w-10 [&>svg]:h-3 [&>svg]:w-3">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Array.from({ length: 12 }, (_, i) => i + 1).map((m) => (
                <SelectItem key={m} value={String(m)} className="text-xs">{m}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button type="button" variant="ghost" size="icon" onClick={nextMonth} className="h-6 w-6 shrink-0">
            <ChevronRight className="h-3 w-3" />
          </Button>
        </div>
        <label className="flex items-center gap-1 cursor-pointer shrink-0">
          <span className="text-[11px] text-muted-foreground select-none">农历</span>
          <button
            type="button"
            role="switch"
            aria-checked={showLunar}
            onClick={() => setShowLunar(!showLunar)}
            className={`relative inline-flex h-5 w-[34px] shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ${showLunar ? 'bg-primary' : 'bg-input'}`}
          >
            <span className={`block h-4 w-4 rounded-full bg-background shadow-sm ring-0 transition-transform ${showLunar ? 'translate-x-[15px]' : 'translate-x-0'}`} />
          </button>
        </label>
      </div>

      {/* 星期头 */}
      <div className="grid grid-cols-7 mb-0.5">
        {WEEKDAYS.map((w) => (
          <div key={w} className="text-center text-[10px] text-muted-foreground py-0.5">{w}</div>
        ))}
      </div>

      {/* 日期网格 */}
      <div className="grid grid-cols-7">
        {solarGrid.map((cell, i) => {
          if (!cell) return <div key={`e-${i}`} className="h-8" />
          const isSelected = cell.solarDay === selectedDay
          return (
            <button
              key={cell.key}
              type="button"
              className={`h-8 flex flex-col items-center justify-center rounded text-[10px] leading-tight transition-colors
                ${isSelected ? 'bg-primary text-primary-foreground' : 'hover:bg-muted'}`}
              onClick={() => setSelectedDay(cell.solarDay)}
            >
              <span className="text-[11px] font-medium leading-none">{cell.solarDay}</span>
              {showLunar && (
                <span className={`text-[8px] leading-none mt-[1px] ${isSelected ? 'text-primary-foreground/70' : 'text-muted-foreground'}`}>
                  {cell.lunarDayName}
                </span>
              )}
            </button>
          )
        })}
      </div>

      {/* 底部：时间 + 操作按钮 */}
      <div className="flex items-center gap-1 mt-2 pt-2 border-t">
        <Select value={String(hour)} onValueChange={(v) => setHour(Number(v))}>
          <SelectTrigger className="h-6 w-14 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {Array.from({ length: 24 }, (_, i) => (
              <SelectItem key={i} value={String(i)} className="text-xs">
                {String(i).padStart(2, '0')}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-xs text-muted-foreground shrink-0">:</span>
        <Select value={String(minute)} onValueChange={(v) => setMinute(Number(v))}>
          <SelectTrigger className="h-6 w-14 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {Array.from({ length: 60 }, (_, i) => (
              <SelectItem key={i} value={String(i)} className="text-xs">
                {String(i).padStart(2, '0')}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="flex gap-1 ml-auto">
          <Button type="button" onClick={onClose} className="h-6 text-xs px-2" variant="outline">取消</Button>
          <Button type="button" onClick={handleConfirm} className="h-6 text-xs px-2" disabled={selectedDay === null}>确定</Button>
        </div>
      </div>
    </div>
  )
}