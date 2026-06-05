# 提醒管理 UI 优化设计

## 概述

对提醒管理相关页面进行 UI 优化，重点是新建提醒对话框的交互重构，以及侧边栏/页面标题的文案精简。

## 1. 日历组件

### 交互流程

```
┌─ 新建提醒 ──────────────────────────────────┐
│                                              │
│  提醒类型: [ 一次性  ▼ ]                    │
│            ├ 一次性                          │
│            ├ 周期                            │
│            └ Cron                            │
│                                              │
│  ┌─ 触发时间 ─────────────────────────┐      │
│  │ 2026-06-15 09:00    [📅]          │      │  ← 点击输入框弹出日历
│  └────────────────────────────────────┘      │
│                                              │
│  (Cron 模式时: 显示 cron 表达式输入框)       │
└──────────────────────────────────────────────┘
```

### 日历弹出组件

| 区域 | 说明 |
|---|---|
| 月份导航 | ◀ 2026年 6月 ▶ |
| 视图切换 | `[农历]` 按钮，点击切换公历月/农历月视图 |
| 星期头 | 一 二 三 四 五 六 日 |
| 日期格 | 公历视图：15(大) + 廿七(小)；农历视图：初一(大) + 15(小) |
| 时间选择 | 时:分下拉框，位于日历底部 |
| 操作 | [确定] / [取消] |

### 核心行为

- 日历弹出组件独立为 `CalendarPopover` 组件
- 公历模式下点击选择公历日期，农历模式下点击选择农历日期
- 两种视图都在格子内同时展示公历+农历日期
- 选择时间和日期后点确定填入输入框
- 使用 `lunar-typescript` 库进行农历/公历转换

## 2. ScheduleForm 重构

### 当前 → 目标

| 当前 | 目标 |
|---|---|
| 外层 Tabs: 公历/农历 | 移除，由日历组件内置切换 |
| 内层 Tabs: 一次性/周期/Cron | 替换为 Select 下拉框 |
| 公历: datetime-local 输入 | 日历弹出组件 |
| 农历: LunarPicker (三个下拉框) | 日历弹出组件（内置农历切换） |

### 状态管理

`calendar` 字段仍由 ScheduleForm 维护，但不再通过 Tabs 切换：
- 日历弹出组件内部有公历/农历视图切换
- 用户选择日期时，根据当前视图自动确定 calendar: 公历视图 → 'solar'，农历视图 → 'lunar'
- ScheduleForm 的 `onChange` 仍返回完整的 `{ calendar, schedule_type, schedule_spec }`

## 3. 文案精简

| 位置 | 当前 | 目标 |
|---|---|---|
| 侧边栏 | 通知通道 | 通知 |
| 侧边栏 | API Key | API |
| 通道列表页标题 | 通知通道 | 通知 |
| API Key 页标题 | API Key | API |

## 4. 列表展示通道

提醒列表表格新增「通道」列，显示该提醒绑定的通道名称（Badge 形式）。
- Reminder 类型仅有 `channel_ids: number[]`，无通道名称
- 前端在 reminders 页面加载时同时加载所有通道列表，建立 id→name 映射
- 表格「通道」列根据 `channel_ids` 映射显示通道名称 Badge

## 涉及文件

| 文件 | 改动 |
|---|---|
| `frontend/src/components/reminders/ReminderEditDialog.tsx` | 适配新 ScheduleForm |
| `frontend/src/components/reminders/ScheduleForm.tsx` | 完全重写，移除 Tabs，使用 Select + CalendarPopover |
| `frontend/src/components/reminders/CalendarPopover.tsx` | **新建**，日历弹出组件 |
| `frontend/src/components/reminders/LunarPicker.tsx` | **删除**，功能合并到 CalendarPopover |
| `frontend/src/components/layout/Sidebar.tsx` | 更新标签 |
| `frontend/src/pages/channels/index.tsx` | 更新标题 |
| `frontend/src/pages/apikeys/index.tsx` | 更新标题 |
| `frontend/src/pages/reminders/index.tsx` | 新增通道列 |
| `frontend/package.json` | 添加 `lunar-typescript` 依赖 |