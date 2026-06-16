# 提醒弹窗移动端适配设计

## 问题描述

提醒编辑/新建弹窗在移动端有两个问题：

1. **底部按钮溢出**：Dialog 固定 `max-h-[95vh]` 且 `overflow-visible`，移动端表单内容过长时，底部的「测试」「取消」「保存」按钮被截断不可点击
2. **日历组件被裁剪**：`CalendarPopover` 使用 `absolute top-full` 定位，在移动端小屏下超出 Dialog / viewport 边界被裁剪

要求：在不影响桌面布局的前提下修复。

## 方案

### 核心思路：Dialog 滚动 + Portal 化日历

1. DialogContent 内容区改为可滚动（`overflow-y-auto`）
2. CalendarPopover 脱离 Dialog CSS 容器，通过 Portal + fixed 定位到 body
3. 添加 viewport 边界自适应逻辑

### 改动清单

#### 1. DialogContent → 可滚动

- **文件**：`ReminderEditDialog.tsx`
- **改动**：`overflow-visible` → `overflow-y-auto`
- **效果**：所有表单内容可滚动到底，按钮始终可见

#### 2. CalendarPopover → Portal + Fixed 定位

- **文件**：`ScheduleForm.tsx`、`CalendarPopover.tsx`
- **ScheduleForm 改动**：
  - 给触发元素添加 `ref`
  - 将 ref 传递给 CalendarPopover
- **CalendarPopover 改动**：
  - 新增 props：`triggerRef`
  - 打开时通过 `getBoundingClientRect()` 计算位置
  - 用 `createPortal` 渲染到 `document.body`
  - 定位改为 `fixed` + 动态 `top/left`
  - viewport 边界检测：底部溢出时弹到触发元素上方
  - `z-index` 提升到 `[100]` 确保覆盖 Dialog 蒙层

#### 3. 移动端位置微调

- 移动端弹出扩展时间面板的位置保护（目前 `bottom-full` 向上弹出，portal 化后依然适用）

#### 4. 桌面端影响

- `max-w-2xl` 不变 ⇒ 宽屏布局无变化
- 内容不超时时 `overflow-y-auto` 不显示滚动条 ⇒ 视觉上完全一致
- DatePicker 的 Portal 仅在打开日历组件时生效 ⇒ 不影响静态 DOM

## 技术要点

- Radix Dialog 的 `translateX(-50%) translateY(-50%)` 是 CSS `transform`，它创建新的定位容器，使得 `absolute` 子元素的 `top/left` 参考的是 Dialog 而非 viewport——这是日历被裁剪的根本原因。`fixed` 定位脱离所有父级影响，只参考 viewport。
- `createPortal` 将 Calendar DOM 节点移到 `body` 下，完全隔离 Dialog 的 `overflow` 约束。
- 位置更新在 `calendarOpen` 变化时计算一次即可，无需 `scroll/resize` 监听。

## 非功能性要求

- 不支持桌面端布局变化
- 移动端 Dialog 内所有字段可完整查看和操作
- 日历组件不被任何边界裁剪，始终可见
