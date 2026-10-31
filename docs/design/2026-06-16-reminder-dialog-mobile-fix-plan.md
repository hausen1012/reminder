# 提醒弹窗移动端适配 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复提醒编辑弹窗在移动端时底部按钮溢出和日历组件被裁剪的问题

**Architecture:** DialogContent 改为可滚动（`overflow-y-auto`）；CalendarPopover 通过 `createPortal` 渲染到 `body` + `fixed` 定位 + viewport 边界自适应，脱离 Dialog CSS 容器的裁剪。

**Tech Stack:** React 18, shadcn/ui (Radix Dialog), Tailwind CSS, react-dom/createPortal

---

### Task 1: DialogContent 改为可滚动

**Files:**
- Modify: `frontend/src/components/reminders/ReminderEditDialog.tsx:168`

- [ ] **Step 1: 修改 overflow 属性**

将 `<DialogContent>` 的 `overflow-visible` 改为 `overflow-y-auto`

**改前：**
```tsx
<DialogContent className="max-w-2xl max-h-[95vh] overflow-visible">
```

**改后：**
```tsx
<DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
```

> `max-h` 从 95vh 降到 90vh 是为了在滚动时上下留出 5vh 的边距，视觉上不贴边。

- [ ] **Step 2: 验证编译**

Run: `cd frontend && npx tsc --noEmit`
Expected: 无类型错误

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/reminders/ReminderEditDialog.tsx
git commit -m "fix: DialogContent 改为 overflow-y-auto 支持滚动"
```

---

### Task 2: ScheduleForm 添加 triggerRef

**Files:**
- Modify: `frontend/src/components/reminders/ScheduleForm.tsx`

- [ ] **Step 1: 添加 useRef import 和 triggerRef**

**在文件顶部导入 useRef：**（当前已有 `useState`，加 `useRef`）
```tsx
import { useState, useRef } from 'react'
```

**在函数组件内添加 ref：**
```tsx
const triggerRef = useRef<HTMLDivElement>(null)
```

**在"时间"字段的触发元素上绑定 ref：**
```tsx
<div
  ref={triggerRef}
  onClick={() => setCalendarOpen(true)}
  className="flex min-h-[2.5rem] cursor-pointer items-center rounded-md border border-input bg-background px-3 py-1.5 text-sm ring-offset-background hover:bg-accent"
>
```

**在 CalendarPopover 传递中增加 triggerRef prop：**
```tsx
{calendarOpen && (
  <CalendarPopover
    triggerRef={triggerRef}
    date={...
```

- [ ] **Step 2: 验证编译**

Run: `cd frontend && npx tsc --noEmit`
Expected: CalendarPopover 报类型错误（triggerRef prop 未声明）——这正是预期的，下一步会修复

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/reminders/ScheduleForm.tsx
git commit -m "chore: ScheduleForm 添加 triggerRef 传递给 CalendarPopover"
```

---

### Task 3: CalendarPopover → Portal + Fixed 定位

**Files:**
- Modify: `frontend/src/components/reminders/CalendarPopover.tsx`

- [ ] **Step 1: 添加新 import**

在文件顶部添加 `createPortal`，以及 `type RefObject`：
```tsx
import { useState, useMemo, useRef, useEffect } from 'react'
import { createPortal } from 'react-dom'
```
> 注意：原有的 `import ... from 'react'` 不需要改，直接在 `useState` 所在行的花括号里加 `useMemo, useRef, useEffect` 就行（当前已导入）。

- [ ] **Step 2: 更新 Props 接口**

添加 `triggerRef` 字段：
```tsx
interface Props {
  date?: string
  hour?: number
  minute?: number
  initialCalendar?: 'solar' | 'lunar'
  triggerRef: React.RefObject<HTMLDivElement | null>
  onSelect: (result: CalendarResult) => void
  onClose: () => void
}
```

- [ ] **Step 3: 添加位置状态和计算逻辑**

在 `const popoverRef = useRef<HTMLDivElement>(null)` 之后添加位置状态：
```tsx
const [position, setPosition] = useState<{ top: number; left: number } | null>(null)
```

在 `const [timePanelOpen, setTimePanelOpen] = useState(false)` 之后（或在 useEffect 中）添加计算位置的逻辑：
```tsx
// 日历打开时计算 fixed 位置
useEffect(() => {
  if (!triggerRef.current) return
  const rect = triggerRef.current.getBoundingClientRect()
  const popupHeight = 360 // 预估日历高度
  const top = rect.bottom + 4
  const left = Math.max(8, Math.min(rect.left, window.innerWidth - 280))
  // viewport 底部溢出时翻转到触发元素上方
  if (top + popupHeight > window.innerHeight) {
    setPosition({ top: rect.top - popupHeight - 4, left })
  } else {
    setPosition({ top, left })
  }
}, [triggerRef])
```

- [ ] **Step 4: 修改 CalendarPopover 渲染为 Portal + fixed**

将 CalendarPopover 的 `<div ref={popoverRef} ...>` 及其所有子元素用 `createPortal` 包裹，并替换定位 class：

**改前：**
```tsx
return (
  <div
    ref={popoverRef}
    className="absolute top-full left-0 z-50 mt-1 bg-card rounded-lg border shadow-lg w-[272px] max-w-[calc(100vw-2rem)] p-2.5"
  >
    {/* ... 全部内容 ... */}
  </div>
)
```

**改后：**
```tsx
const popoverContent = (
  <div
    ref={popoverRef}
    className="fixed z-[100] bg-card rounded-lg border shadow-lg w-[272px] max-w-[calc(100vw-2rem)] p-2.5"
    style={position ? { top: position.top, left: position.left } : { top: -9999, left: -9999 }}
  >
    {/* ... 全部内容，无变化 ... */}
  </div>
)

return createPortal(popoverContent, document.body)
```

- [ ] **Step 5: 调整外部点击关闭逻辑**

当前外部点击关闭逻辑通过 `document.addEventListener('mousedown', handleClick)` 实现，portal 化后依然有效——点击 Dialog 内容区会触发 `onClose`，这是正确的。

但需要注意：Portal 将日历渲染到 `document.body` 下，点击日历外面（包括 Dialog 蒙层）应该关闭日历。当前逻辑是 `popoverRef.current.contains(target)` 则保留，否则 `onClose()`——这已正确处理。

- [ ] **Step 6: 验证编译**

Run: `cd frontend && npx tsc --noEmit`
Expected: 无类型错误

- [ ] **Step 7: 提交**

```bash
git add frontend/src/components/reminders/CalendarPopover.tsx
git commit -m "fix: CalendarPopover 改为 Portal + fixed 定位，避免被 Dialog 裁剪"
```

---

## Spec Coverage 检查

| Spec 要求 | Task | 状态 |
|---|---|---|
| DialogContent 内容区可滚动 | Task 1 | ✓ |
| CalendarPopover Portal 到 body | Task 3 | ✓ |
| Fixed 定位脱离 Dialog 容器 | Task 3 | ✓ |
| Viewport 边界检测（底部溢出翻转） | Task 3 Step 3 | ✓ |
| ScheduleForm 传递 ref | Task 2 | ✓ |
| 桌面端不受影响 | Task 1 (max-w-2xl 不变) | ✓ |

## 执行说明

改动只涉及 3 个文件，每个文件的改动都是原子化的。建议按 Task 1 → Task 2 → Task 3 顺序执行。
