# 提醒管理 UI 优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 优化提醒管理 UI，重构新建提醒对话框（日历弹出组件、下拉框代替 Tabs），精简侧边栏文案

**Architecture:** 创建 CalendarPopover 弹出日历组件（内置农历切换），重写 ScheduleForm（Select 下拉框 + 日历组件），更新页面文案

**Tech Stack:** React 18, TypeScript, Tailwind CSS, Radix UI, lunar-typescript

---

### Task 1: 安装 lunar-typescript 依赖

**Files:**
- Modify: `frontend/package.json`

- [ ] **Step 1: 确认依赖已安装**

```bash
cd /d/code/reminder2/frontend
```

预期：lunar-typescript@1.8.6 已在 node_modules 中（先前已安装）

---

### Task 2: 创建 CalendarPopover 日历弹出组件

**Files:**
- Create: `frontend/src/components/reminders/CalendarPopover.tsx`
- Modify: `frontend/src/types/index.ts` - 导出 CalendarResult 类型

- [ ] **Step 1: 在 types/index.ts 末尾添加 CalendarResult 类型**

```typescript
// --- 日历组件 ---

export interface CalendarResult {
  date: string   // ISO 格式 "YYYY-MM-DDTHH:mm"
  calendar: ReminderCalendar
}
```

在第 161 行 Token 接口定义之后添加。

- [ ] **Step 2: 创建 CalendarPopover 组件**

文件：`frontend/src/components/reminders/CalendarPopover.tsx`

组件结构：
- Popover（使用 Radix UI Popover 或 Dialog）
- 点击输入框触发弹出
- 内部功能：
  - 月份导航（◀ / ▶）
  - 公历/农历模式切换按钮
  - 星期头（一 二 三 四 五 六 日）
  - 日期网格
  - 时间选择（时：分 Select）
  - 确定/取消按钮

核心逻辑：
- `mode: 'solar' | 'lunar'` 状态
- 公历模式：用 Solar 对象遍历当月天数，每格显示公历日期 + 农历日期
- 农历模式：用 LunarMonth 获取当月信息，网格从初一对应星期开始，每格显示农历日 + 公历日期
- 选中日期时根据 mode 确定 calendar 字段

```typescript
import { useState, useMemo } from 'react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Solar, Lunar, LunarMonth } from 'lunar-typescript'
import type { CalendarResult, ReminderCalendar } from '@/types'

interface Props {
  open: boolean
  initialCalendar?: ReminderCalendar
  onSelect: (result: CalendarResult) => void
  onClose: () => void
}

const WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日']

export function CalendarPopover({ open, initialCalendar, onSelect, onClose }: Props) {
  const now = new Date()
  const [mode, setMode] = useState<'solar' | 'lunar'>(initialCalendar ?? 'solar')
  const [solarYear, setSolarYear] = useState(now.getFullYear())
  const [solarMonth, setSolarMonth] = useState(now.getMonth() + 1)
  const [lunarYear, setLunarYear] = useState(now.getFullYear())
  const [lunarM, setLunarM] = useState(1)
  const [selectedDay, setSelectedDay] = useState<number | null>(null)
  const [selectedLunarDay, setSelectedLunarDay] = useState<number | null>(null)
  const [hour, setHour] = useState(9)
  const [minute, setMinute] = useState(0)

  if (!open) return null

  function toggleMode() {
    if (mode === 'solar') {
      // 切换到农历模式：从当前公历日期转换到农历
      const solar = Solar.fromYmd(solarYear, solarMonth, selectedDay || 1)
      const lunar = solar.getLunar()
      setLunarYear(lunar.getYear())
      setLunarM(lunar.getMonth())
      setSelectedLunarDay(lunar.getDay())
      setMode('lunar')
    } else {
      // 切换到公历模式
      const lunar = Lunar.fromYmd(lunarYear, lunarM, selectedLunarDay || 1)
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
      onSelect({ date: dateStr, calendar: 'solar' })
    } else if (mode === 'lunar' && selectedLunarDay !== null) {
      // 农历模式下以 lunar 字段返回
      onSelect({
        date: `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`,
        calendar: 'lunar',
      })
      // 实际 lunar 日期通过存 lunarYear/lunarM/selectedLunarDay 在 ScheduleForm 中拼接
      // 这里通过 onSelect 回传，但需要额外字段 => 修改 CalendarResult 接口
    }
    onClose()
  }

  // ... 渲染日历网格逻辑
  // 公历模式：生成该月的日期数据
  // 农历模式：生成该农历月的日期数据（按星期对齐）
}
```

> 注：完整实现在执行时填充

- [ ] **Step 3: 更新 CalendarResult 类型以支持农历选择**

```typescript
export interface CalendarResult {
  date: string
  calendar: ReminderCalendar
  lunar?: { year: number; month: number; day: number }
  hour: number
  minute: number
}
```

- [ ] **Step 4: 实现公历模式下日历网格渲染**

逻辑：
1. 获取当月天数：`new Date(year, month, 0).getDate()`
2. 获取当月第一天星期：`new Date(year, month - 1, 1).getDay()` 
3. 调整星期值（周日=0 转为周一=0...周日=6）
4. 生成 6 行 × 7 列的网格数组
5. 每个日期格渲染：公历日期数字（大号）+ `Solar.fromYmd(y,m,d).getLunar().getDayInChinese()`（小号）

- [ ] **Step 5: 实现农历模式下日历网格渲染**

逻辑：
1. 用 `LunarMonth.fromYm(lunarYear, lunarM)` 获取农历月信息
2. 用 `Lunar.fromYmd(lunarYear, lunarM, 1).getSolar()` 获取初一对应的公历日期
3. 获取该公历日期的星期，确定初一在网格中的列位置
4. 遍历农历月所有天（初一→廿九/三十）
5. 每格渲染：农历日中文名（大号）+ 对应的公历日期数字（小号）
6. 用 `getDayInChinese()` 获取农历日中文名

- [ ] **Step 6: 完善 CalendarPopover 渲染**

```
┌───────────────────────────────┐
│  ◀  2026年 6月  ▶  [农历]     │
│  ┌──┬──┬──┬──┬──┬──┬──┐     │
│  │一│二│三│四│五│六│日│     │
│  ├──┼──┼──┼──┼──┼──┼──┤     │
│  │  │ 1│ 2│ 3│ 4│ 5│ 6│     │
│  │  │十六│十七│...│  │  │     │
│  ├──┼──┼──┼──┼──┼──┼──┤     │
│  │..│  │  │  │  │  │  │     │
│  └──┴──┴──┴──┴──┴──┴──┘     │
│  时间: [09] : [00]            │
│        [取消]  [确定]          │
└───────────────────────────────┘
```

---

### Task 3: 重写 ScheduleForm

**Files:**
- Modify: `frontend/src/components/reminders/ScheduleForm.tsx`

移除 Tabs，改为 Select 下拉框 + CalendarPopover。移除外部农历切换。

```typescript
// ScheduleForm 重写 - 使用 Select 选择提醒类型 + CalendarPopover 选择日期
import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CalendarPopover } from './CalendarPopover'
import type { ReminderCalendar, ReminderScheduleType } from '@/types'

export interface ScheduleValue {
  calendar: ReminderCalendar
  schedule_type: ReminderScheduleType
  schedule_spec: Record<string, unknown>
}

interface Props {
  value: ScheduleValue
  onChange: (v: ScheduleValue) => void
}

const INTERVAL_UNITS = [
  { value: 'minute', label: '分钟' },
  { value: 'hour', label: '小时' },
  { value: 'day', label: '天' },
  { value: 'month', label: '月' },
  { value: 'year', label: '年' },
]

const LUNAR_INTERVAL_UNITS = [
  { value: 'day', label: '天' },
  { value: 'month', label: '月' },
  { value: 'year', label: '年' },
]

export function ScheduleForm({ value, onChange }: Props) {
  const [calendarOpen, setCalendarOpen] = useState(false)
  const spec = value.schedule_spec ?? {}

  function handleTypeChange(t: ReminderScheduleType) {
    let defaults: Record<string, unknown> = {}
    if (t === 'once') {
      if (value.calendar === 'lunar') {
        defaults = { lunar: { year: 2026, month: 1, day: 1 }, hour: 9, minute: 0 }
      } else {
        defaults = { at: '' }
      }
    } else if (t === 'interval') {
      if (value.calendar === 'lunar') {
        defaults = { start_lunar: { year: 2026, month: 1, day: 1 }, every: 1, unit: 'month', hour: 9, minute: 0 }
      } else {
        defaults = { start_at: '', every: 1, unit: 'day' }
      }
    } else {
      defaults = { expr: '0 9 * * *' }
    }
    onChange({ ...value, schedule_type: t, schedule_spec: defaults })
  }

  function patchSpec(patch: Record<string, unknown>) {
    onChange({ ...value, schedule_spec: { ...spec, ...patch } })
  }

  function handleCalendarSelect(result: { date: string; calendar: ReminderCalendar; hour: number; minute: number; lunar?: { year: number; month: number; day: number } }) {
    if (result.calendar === 'lunar' && result.lunar) {
      onChange({
        ...value,
        calendar: 'lunar',
        schedule_spec: {
          ...spec,
          lunar: result.lunar,
          hour: result.hour,
          minute: result.minute,
        },
      })
    } else {
      onChange({
        ...value,
        calendar: 'solar',
        schedule_spec: {
          ...spec,
          at: result.date,
        },
      })
    }
  }

  return (
    <div className="space-y-3">
      <div className="space-y-2">
        <Label>提醒类型</Label>
        <Select value={value.schedule_type} onValueChange={handleTypeChange}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="once">一次性</SelectItem>
            <SelectItem value="interval">周期</SelectItem>
            <SelectItem value="cron">Cron</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Cron 表达式 */}
      {value.schedule_type === 'cron' && (
        <div className="space-y-2">
          <Label htmlFor="spec-expr">Cron 表达式</Label>
          <Input
            id="spec-expr"
            value={(spec.expr as string) ?? ''}
            onChange={(e) => patchSpec({ expr: e.target.value })}
            placeholder="例如：0 9 * * 1-5"
          />
          <p className="text-xs text-muted-foreground">
            5 字段标准 cron。例：每天 09:00 → <code>0 9 * * *</code>
          </p>
        </div>
      )}

      {/* 一次性 / 周期：显示日期输入 */}
      {(value.schedule_type === 'once' || value.schedule_type === 'interval') && (
        <>
          <div className="space-y-2">
            <Label>
              {value.schedule_type === 'once' ? '触发时间' : '起始时间'}
              {value.calendar === 'lunar' && (
                <span className="text-xs text-muted-foreground ml-2">（农历）</span>
              )}
            </Label>
            <div className="flex gap-2">
              <Input
                readOnly
                value={formatScheduleDate(spec, value.calendar)}
                placeholder="点击选择日期"
                onClick={() => setCalendarOpen(true)}
                className="cursor-pointer"
              />
              <Button type="button" variant="outline" onClick={() => setCalendarOpen(true)}>
                选择
              </Button>
            </div>
          </div>

          <CalendarPopover
            open={calendarOpen}
            initialCalendar={value.calendar}
            onSelect={handleCalendarSelect}
            onClose={() => setCalendarOpen(false)}
          />

          {/* 周期参数 */}
          {value.schedule_type === 'interval' && (
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="spec-every">每</Label>
                <Input
                  id="spec-every"
                  type="number"
                  min={1}
                  value={(spec.every as number) ?? 1}
                  onChange={(e) => patchSpec({ every: Number(e.target.value) || 1 })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="spec-unit">单位</Label>
                <Select
                  value={(spec.unit as string) ?? 'day'}
                  onValueChange={(v) => patchSpec({ unit: v })}
                >
                  <SelectTrigger id="spec-unit">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(value.calendar === 'lunar' ? LUNAR_INTERVAL_UNITS : INTERVAL_UNITS).map((u) => (
                      <SelectItem key={u.value} value={u.value}>
                        {u.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function formatScheduleDate(spec: Record<string, unknown>, calendar: ReminderCalendar): string {
  if (calendar === 'lunar') {
    const lunar = spec.lunar as { year: number; month: number; day: number } | undefined
    if (lunar) {
      const LUNAR_MONTHS = ['正月','二月','三月','四月','五月','六月','七月','八月','九月','十月','十一月','腊月']
      const LUNAR_DAYS = ['初一','初二','初三','初四','初五','初六','初七','初八','初九','初十','十一','十二','十三','十四','十五','十六','十七','十八','十九','二十','廿一','廿二','廿三','廿四','廿五','廿六','廿七','廿八','廿九','三十']
      return `${lunar.year}年 ${LUNAR_MONTHS[lunar.month - 1] ?? ''} ${LUNAR_DAYS[lunar.day - 1] ?? ''} ${String(spec.hour ?? 9).padStart(2, '0')}:${String(spec.minute ?? 0).padStart(2, '0')}`
    }
    return '选择农历日期'
  }
  if (spec.at) {
    return (spec.at as string).slice(0, 16)
  }
  return '选择日期'
}
```

---

### Task 4: 更新 ReminderEditDialog 适配新 ScheduleForm

**Files:**
- Modify: `frontend/src/components/reminders/ReminderEditDialog.tsx`
- Modify: `frontend/src/lib/api.ts`

适配内容：
- ScheduleForm 的 `onChange` 回调签名不变
- 移除 `calendar` 相关特殊处理（ScheduleForm 内部处理了）
- 更新通道选择区域的文案

- [ ] **Step 1: 更新通道选择区域的空状态文案**

将 `「通知通道」页面` 改为 `「通知」页面`：

```typescript
// 第 184 行
<p className="text-xs text-muted-foreground">还没有通道，先到「通知」页面创建一个。</p>
```

---

### Task 5: 更新侧边栏和页面标题

**Files:**
- Modify: `frontend/src/components/layout/Sidebar.tsx`
- Modify: `frontend/src/pages/channels/index.tsx`
- Modify: `frontend/src/pages/tokens/index.tsx`

- [ ] **Step 1: 更新 Sidebar.tsx**

```typescript
// 第 13 行：'通知通道' → '通知'
// 第 14 行：'令牌' → 'API'
const navItems = [
  { to: '/', label: '首页', icon: LayoutDashboard },
  { to: '/reminders', label: '提醒', icon: Bell },
  { to: '/logs', label: '日志', icon: ScrollText },
  { to: '/channels', label: '通知', icon: Send },
  { to: '/tokens', label: 'API', icon: Key },
  { to: '/profile', label: '设置', icon: User },
]
```

- [ ] **Step 2: 更新 channels/index.tsx 页面标题**

```typescript
// 第 100 行
<h1 className="text-3xl font-bold tracking-tight">通知</h1>
// 第 101 行
<p className="text-sm text-muted-foreground mt-1">
  管理用于发送提醒的通道：邮件、钉钉、企微、Webhook。
</p>
```

- [ ] **Step 3: 更新 tokens/index.tsx 页面标题**

```typescript
// 第 96 行
<h1 className="text-3xl font-bold tracking-tight">API</h1>
// 第 97 行
<p className="text-sm text-muted-foreground mt-1">
  管理外部 API 调用的密钥。创建后密钥仅展示一次。
</p>
```

---

### Task 6: 提醒列表新增通道列

**Files:**
- Modify: `frontend/src/pages/reminders/index.tsx`

- [ ] **Step 1: 在 reminders 页面加载通道列表**

```typescript
// 在组件内新增状态
const [channels, setChannels] = useState<Channel[]>([])

// 在 refresh 或 useEffect 中加载
useEffect(() => {
  listChannels().then(setChannels).catch(() => setChannels([]))
}, [])
```

- [ ] **Step 2: 生成 id→name 映射并新增表格列**

```typescript
// 添加映射
const channelMap = useMemo(() => {
  const map = new Map<number, string>()
  channels.forEach((ch) => map.set(ch.id, ch.name))
  return map
}, [channels])

// 在表格 thead 新增列（在「调度」和「下次触发」之间）
<th className="px-4 py-3">通道</th>

// 在 tbody 每行新增列（在调度列后面）
<td className="px-4 py-3">
  <div className="flex flex-wrap gap-1">
    {r.channel_ids.map((cid) => (
      <Badge key={cid} variant="outline" className="text-xs">
        {channelMap.get(cid) || `#${cid}`}
      </Badge>
    ))}
  </div>
</td>
```

- [ ] **Step 3: 添加 useMemo 导入（如果尚未导入）**

---

### Task 7: 删除旧的 LunarPicker 组件

**Files:**
- Delete: `frontend/src/components/reminders/LunarPicker.tsx`

- [ ] **Step 1: 确认 ScheduleForm 不再引用 LunarPicker 后删除文件**

```bash
rm /d/code/reminder2/frontend/src/components/reminders/LunarPicker.tsx
```

---

### Task 8: 编译验证

- [ ] **Step 1: 运行 TypeScript 编译检查**

```bash
cd /d/code/reminder2/frontend && npx tsc --noEmit 2>&1
```

预期：无编译错误

- [ ] **Step 2: 运行 vite build 验证**

```bash
cd /d/code/reminder2/frontend && npx vite build 2>&1
```

预期：构建成功