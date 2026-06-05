// CalendarPopover 是弹出日历组件，支持公历/农历视图切换，同时展示两种日期
import { useState, useMemo } from 'react'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Solar, Lunar, LunarMonth } from 'lunar-typescript'
import { Switch } from '@/components/ui/switch'
import type { CalendarResult, ReminderCalendar } from '@/types'

interface Props {
  open: boolean
  calendar?: ReminderCalendar
  date?: string
  lunarDate?: { year: number; month: number; day: number }
  hour?: number
  minute?: number
  onSelect: (result: CalendarResult) => void
  onClose: () => void
}

const WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日']

const LUNAR_MONTHS = [
  '正月','二月','三月','四月','五月','六月',
  '七月','八月','九月','十月','十一月','腊月',
]

const LUNAR_DAYS = [
  '初一','初二','初三','初四','初五','初六','初七','初八','初九','初十',
  '十一','十二','十三','十四','十五','十六','十七','十八','十九','二十',
  '廿一','廿二','廿三','廿四','廿五','廿六','廿七','廿八','廿九','三十',
]

// 将 JS getDay() (周日=0) 转为周一=0..周日=6
function toMondayBase(jsWeekday: number): number {
  return jsWeekday === 0 ? 6 : jsWeekday - 1
}

const YEAR_OPTIONS = Array.from({ length: 31 }, (_, i) => 2020 + i)

interface DayCell {
  key: string
  solarDay: number
  solarMonth: number
  solarYear: number
  lunarDay: number
  lunarMonth: number
  lunarYear: number
  lunarDayName: string
  current: boolean
}

// 外层组件：处理 open/close，确保内层组件始终在 open=true 时完整挂载
export function CalendarPopover({ open, ...rest }: Props) {
  if (!open) return null
  return <CalendarPopoverInner {...rest} />
}

function CalendarPopoverInner({ calendar, date, lunarDate, hour: initHour, minute: initMin, onSelect, onClose }: Omit<Props, 'open'>) {
  const now = new Date()
  const [mode, setMode] = useState<ReminderCalendar>(calendar ?? 'solar')
  const [solarYear, setSolarYear] = useState(() => {
    if (date) return new Date(date).getFullYear()
    if (calendar === 'lunar' && lunarDate) return Solar.fromYmd(now.getFullYear(), now.getMonth() + 1, 1).getLunar().getSolar().getYear()
    return now.getFullYear()
  })
  const [solarMonth, setSolarMonth] = useState(() => {
    if (date) return new Date(date).getMonth() + 1
    if (calendar === 'lunar' && lunarDate) {
      const s = Solar.fromYmd(now.getFullYear(), now.getMonth() + 1, 1)
      return s.getLunar().getSolar().getMonth()
    }
    return now.getMonth() + 1
  })
  const [lunarYear, setLunarYear] = useState(lunarDate?.year ?? now.getFullYear())
  const [lunarM, setLunarM] = useState(lunarDate?.month ?? 1)
  const [selectedDay, setSelectedDay] = useState<number | null>(() => {
    if (date) return new Date(date).getDate()
    return mode === 'solar' ? now.getDate() : null
  })
  const [selectedLunarDay, setSelectedLunarDay] = useState<number | null>(lunarDate?.day ?? null)
  const [hour, setHour] = useState(initHour ?? 9)
  const [minute, setMinute] = useState(initMin ?? 0)

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
      const lunar = solar.getLunar()
      cells.push({
        key: `s-${d}`,
        solarDay: d,
        solarMonth,
        solarYear,
        lunarDay: lunar.getDay(),
        lunarMonth: lunar.getMonth(),
        lunarYear: lunar.getYear(),
        lunarDayName: lunar.getDayInChinese(),
        current: true,
      })
    }

    return cells
  }, [solarYear, solarMonth])

  // 农历模式网格
  const lunarGrid = useMemo(() => {
    const firstSolar = Lunar.fromYmd(lunarYear, lunarM, 1).getSolar()
    const firstWeekday = toMondayBase(new Date(firstSolar.getYear(), firstSolar.getMonth() - 1, firstSolar.getDay()).getDay())
    const dayCount = LunarMonth.fromYm(lunarYear, lunarM)?.getDayCount() ?? 30
    const cells: (DayCell | null)[] = []

    for (let i = 0; i < firstWeekday; i++) {
      cells.push(null)
    }

    for (let d = 1; d <= dayCount; d++) {
      const l = Lunar.fromYmd(lunarYear, lunarM, d)
      const s = l.getSolar()
      cells.push({
        key: `l-${d}`,
        solarDay: s.getDay(),
        solarMonth: s.getMonth(),
        solarYear: s.getYear(),
        lunarDay: d,
        lunarMonth: lunarM,
        lunarYear,
        lunarDayName: LUNAR_DAYS[d - 1],
        current: true,
      })
    }

    return cells
  }, [lunarYear, lunarM])

  function toggleMode() {
    if (mode === 'solar') {
      const day = selectedDay || 1
      const solar = Solar.fromYmd(solarYear, solarMonth, day)
      const lunar = solar.getLunar()
      setLunarYear(lunar.getYear())
      setLunarM(lunar.getMonth())
      setSelectedLunarDay(lunar.getDay() <= (LunarMonth.fromYm(lunar.getYear(), lunar.getMonth())?.getDayCount() ?? 30) ? lunar.getDay() : 1)
      setMode('lunar')
    } else {
      const day = selectedLunarDay || 1
      const lunar = Lunar.fromYmd(lunarYear, lunarM, day)
      const solar = lunar.getSolar()
      setSolarYear(solar.getYear())
      setSolarMonth(solar.getMonth())
      setSelectedDay(solar.getDay())
      setMode('solar')
    }
  }

  function handleConfirm() {
    if (mode === 'solar' && selectedDay !== null) {
      const dateStr = `${solarYear}-${String(solarMonth).padStart(2, '0')}-${String(selectedDay).padStart(2, '0')}T${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
      onSelect({ date: dateStr, calendar: 'solar', hour, minute })
    } else if (mode === 'lunar' && selectedLunarDay !== null) {
      onSelect({
        date: '',
        calendar: 'lunar',
        lunar: { year: lunarYear, month: lunarM, day: selectedLunarDay },
        hour,
        minute,
      })
    }
    onClose()
  }

  function prevMonth() {
    if (mode === 'solar') {
      if (solarMonth === 1) {
        setSolarYear(y => y - 1)
        setSolarMonth(12)
      } else {
        setSolarMonth(m => m - 1)
      }
    } else {
      if (lunarM === 1) {
        setLunarYear(y => y - 1)
        setLunarM(12)
      } else {
        setLunarM(m => m - 1)
      }
    }
  }

  function nextMonth() {
    if (mode === 'solar') {
      if (solarMonth === 12) {
        setSolarYear(y => y + 1)
        setSolarMonth(1)
      } else {
        setSolarMonth(m => m + 1)
      }
    } else {
      if (lunarM === 12) {
        setLunarYear(y => y + 1)
        setLunarM(1)
      } else {
        setLunarM(m => m + 1)
      }
    }
  }

  const cells = mode === 'solar' ? solarGrid : lunarGrid

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30" onClick={onClose}>
      <div
        className="bg-card rounded-lg border shadow-lg w-[280px] p-3"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 月份导航 + 模式切换 */}
        <div className="flex items-center mb-2">
          {/* 左侧：前后月导航 + 年/月下拉 */}
          <div className="flex items-center gap-0.5">
            <Button type="button" variant="ghost" size="icon" onClick={prevMonth} className="h-6 w-6">
              <ChevronLeft className="h-3 w-3" />
            </Button>
            {mode === 'solar' ? (
              <>
                <Select value={String(solarYear)} onValueChange={(v) => setSolarYear(Number(v))}>
                  <SelectTrigger className="h-6 text-xs border-0 bg-transparent hover:bg-muted px-1 w-[64px] [&>svg]:h-3 [&>svg]:w-3">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {YEAR_OPTIONS.map((y) => (
                      <SelectItem key={y} value={String(y)} className="text-xs">{y}年</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select value={String(solarMonth)} onValueChange={(v) => setSolarMonth(Number(v))}>
                  <SelectTrigger className="h-6 text-xs border-0 bg-transparent hover:bg-muted px-1 w-[48px] [&>svg]:h-3 [&>svg]:w-3">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Array.from({ length: 12 }, (_, i) => i + 1).map((m) => (
                      <SelectItem key={m} value={String(m)} className="text-xs">{m}月</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </>
            ) : (
              <>
                <Select value={String(lunarYear)} onValueChange={(v) => setLunarYear(Number(v))}>
                  <SelectTrigger className="h-6 text-xs border-0 bg-transparent hover:bg-muted px-1 w-[64px] [&>svg]:h-3 [&>svg]:w-3">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {YEAR_OPTIONS.map((y) => (
                      <SelectItem key={y} value={String(y)} className="text-xs">{y}年</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select value={String(lunarM)} onValueChange={(v) => setLunarM(Number(v))}>
                  <SelectTrigger className="h-6 text-xs border-0 bg-transparent hover:bg-muted px-1 w-[52px] [&>svg]:h-3 [&>svg]:w-3">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {LUNAR_MONTHS.map((m, i) => (
                      <SelectItem key={i + 1} value={String(i + 1)} className="text-xs">{m}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </>
            )}
            <Button type="button" variant="ghost" size="icon" onClick={nextMonth} className="h-6 w-6">
              <ChevronRight className="h-3 w-3" />
            </Button>
          </div>
          {/* 右侧：农历切换 */}
          <div className="ml-auto">
            <label className="flex items-center gap-1 cursor-pointer">
              <span className="text-[11px] text-muted-foreground select-none">农历</span>
              <Switch
                checked={mode === 'lunar'}
                onCheckedChange={(v) => {
                  if (v && mode === 'solar') toggleMode()
                  else if (!v && mode === 'lunar') toggleMode()
                }}
                className="h-4 w-7"
              />
            </label>
          </div>
        </div>

        {/* 星期头 */}
        <div className="grid grid-cols-7 mb-0.5">
          {WEEKDAYS.map((w) => (
            <div key={w} className="text-center text-[10px] text-muted-foreground py-0.5">
              {w}
            </div>
          ))}
        </div>

        {/* 日期网格 */}
        <div className="grid grid-cols-7">
          {cells.map((cell, i) => {
            if (!cell) {
              return <div key={`empty-${i}`} className="h-8" />
            }
            const isSelected = mode === 'solar'
              ? cell.solarDay === selectedDay
              : cell.lunarDay === selectedLunarDay
            return (
              <button
                key={cell.key}
                type="button"
                className={`h-8 flex flex-col items-center justify-center rounded text-[10px] leading-tight transition-colors
                  ${isSelected
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-muted'
                  }`}
                onClick={() => {
                  if (mode === 'solar') {
                    setSelectedDay(cell.solarDay)
                  } else {
                    setSelectedLunarDay(cell.lunarDay)
                  }
                }}
              >
                <span className="text-[11px] font-medium leading-none">
                  {mode === 'solar' ? cell.solarDay : cell.lunarDayName}
                </span>
                {mode === 'lunar' && (
                  <span className={`text-[9px] leading-none mt-[1px] ${isSelected ? 'text-primary-foreground/70' : 'text-muted-foreground'}`}>
                    {cell.solarDay}
                  </span>
                )}
              </button>
            )
          })}
        </div>

        {/* 时间选择 */}
        <div className="flex items-center gap-1.5 mt-2 pt-2 border-t">
          <Select value={String(hour)} onValueChange={(v) => setHour(Number(v))}>
            <SelectTrigger className="h-7 w-14 text-xs">
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
          <span className="text-xs text-muted-foreground">:</span>
          <Select value={String(minute)} onValueChange={(v) => setMinute(Number(v))}>
            <SelectTrigger className="h-7 w-14 text-xs">
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
          <div className="ml-auto flex gap-1">
            <Button type="button" variant="outline" onClick={onClose} className="h-7 text-xs px-2">
              取消
            </Button>
            <Button type="button" onClick={handleConfirm} className="h-7 text-xs px-2" disabled={
              (mode === 'solar' && selectedDay === null) ||
              (mode === 'lunar' && selectedLunarDay === null)
            }>
              确定
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}