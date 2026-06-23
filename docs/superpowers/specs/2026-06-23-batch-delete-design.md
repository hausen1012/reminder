# 批量删除功能设计

## 概述

为提醒（Reminders）、通知（Channels）、日志（Logs）三个页面添加批量删除功能，提升用户管理效率。

## 交互设计

### 统一模式

每个页面的表格行左侧新增复选框列，表头添加「全选」复选框。选中任意行后，工具栏区域出现「批量删除」按钮，显示选中数量（如「删除选中(3)」）。

### 流程

1. 用户勾选需要删除的记录
2. 点击「批量删除」按钮
3. 弹出确认对话框，提示「确认删除选中的 N 条记录？」
4. 确认后调用后端批量删除 API
5. 成功后刷新列表、清空选中状态
6. 失败时 toast 提示错误信息

### 状态管理

每个页面新增 `selectedIds: Set<number>` 状态，提供全选/取消全选/单选/取消单选操作。

## 后端 API

> 注意：本次改动将提醒（Reminder）现有的单条删除从软删除（`DeletedAt`）统一改为硬删除（物理删除），与通知、日志保持一致。所有删除操作均不可撤销。

### 提醒批量删除

### 提醒批量删除

```
DELETE /api/reminders/batch
Body: { "ids": [1, 2, 3] }
Response: { "code": 0, "message": "ok", "data": null }
```

后端在事务内遍历 ID 列表，为每个 ID 执行：
- 数据库硬删除（`gorm.DB.Unscoped().Delete`，彻底物理删除）
- 从调度器移除（`Engine.Remove`）
- 取消确认重试（`ConfirmMgr.CancelByReminderID`）

### 通知批量删除

```
DELETE /api/channels/batch
Body: { "ids": [1, 2, 3] }
Response: { "code": 0, "message": "ok", "data": null }
```

后端在事务内按 ID 批量物理删除，同时清理关联的 ReminderChannel 关联表。

### 日志批量删除

```
DELETE /api/logs/batch
Body: { "ids": [1, 2, 3] }
Response: { "code": 0, "message": "ok", "data": null }
```

后端直接按 ID 批量硬删 delivery_log 及关联的 delivery_attempt。

## 前端变更

### 通用新增

- 每个页面新增 `selectedIds: Set<number>` 状态
- 表格首列新增复选框
- 表头新增全选复选框
- 工具栏区域新增「批量删除」按钮（选中 > 0 时显示）
- 复用现有 `ConfirmDialog` 组件做二次确认
- 在 `api.ts` 中新增 `batchDeleteReminders` / `batchDeleteChannels` / `batchDeleteLogs` 三个函数

### 各页面具体改动

#### 提醒页（pages/reminders/index.tsx）

- 复选框列宽 `w-10`
- 批量删除按钮放在 PageHeader 右侧，与「新建提醒」并列
- 确认对话框使用现有 `ConfirmDialog`

#### 通知页（pages/channels/index.tsx）

- 复选框列宽 `w-10`
- 批量删除按钮放在 PageHeader 右侧，与「新建通知」并列
- 确认对话框使用现有 `ConfirmDialog`

#### 日志页（pages/logs/index.tsx）

- 折叠展开列改为复选框列
- 批量删除按钮放在 PageHeader 右侧，与现有清理按钮并列
- 注意：日志的折叠链由 `displayRows` 管理，批量删除基于原始 `items` 而非 `displayRows`，选中时按实际 log ID 选中

## 未涉及

- 不涉及排序/搜索/分页逻辑改动
- 不涉及移动端适配特殊处理（复选框在移动端卡片同样展示）
- 不涉及权限验证（复用现有 JWT 鉴权中间件）