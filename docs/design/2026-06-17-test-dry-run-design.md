# 提醒与通知测试功能设计

## 概述

编辑/新建弹窗中的"测试提醒"和"测试通知"按钮，统一走无副作用的 dry-run 接口：直接发送真实通知，但不记录任何 delivery_log / delivery_attempt。

## 后端接口

### POST /api/reminders/test-dry

用表单当前数据试发提醒，不落日志。

```json
// Request
{
  "id": 0,                    // 编辑时传真实 id，新建时传 0（仅用于日志标识，不写库）
  "title": "测试标题",
  "content": "测试内容",
  "content_format": "text",
  "channel_ids": [1, 2]
}

// Response
{
  "code": 0,
  "message": "ok",
  "data": {
    "success": true
  }
}
```

### POST /api/channels/test-dry

用表单当前配置试发通知，不落日志。

```json
// Request
{
  "id": 0,                    // 编辑时传真实 id，新建时传 0
  "type": "smtp",
  "config": { ... }
}

// Response
{
  "code": 0,
  "message": "ok",
  "data": {
    "success": true
  }
}
```

## 后端实现

### dispatch_service.go 新增 DryRun

```go
func (d *DispatchService) DryRun(ctx context.Context, r *models.Reminder) error
```

- 加载绑定 channels（同 run）
- 构建 vars、渲染 title/content（同 run）
- 并发调用 `sendWithRetry`（同 run）
- **不写 delivery_log / delivery_attempt / finalize**

### channel_service.go: TestDryRun

```go
func (s *ChannelService) TestDryRun(ctx context.Context, input ChannelInput) error
```

- 直接从 `input.Config` 解密，不需要查 DB
- 调用 `notifier.Send()`

### reminder_handler.go / channel_handler.go 新增路由

- `POST /api/reminders/test-dry` → `ReminderHandler.TestDryRun`
- `POST /api/channels/test-dry` → `ChannelHandler.TestDryRun`

### 删除旧路由

- 删除 `POST /api/reminders/:id/test`
- 删除 `POST /api/channels/:id/test`
- 删除 `ReminderHandler.Test` / `ChannelHandler.Test`
- 删除 `ReminderService.TestOnce` / `ChannelService.Test`
- 删除 `DispatchService.TestOnce`

## 前端实现

### api.ts

新增两个 API 函数：

```typescript
export async function testReminderDryRun(input: {
  id?: number
  title: string
  content: string
  content_format: string
  channel_ids: number[]
}) {
  const res = await api.post('/reminders/test-dry', input)
  return res.data.data
}

export async function testChannelDryRun(input: {
  id?: number
  type: ChannelType
  config: Record<string, unknown>
}) {
  const res = await api.post('/channels/test-dry', input)
  return res.data.data
}
```

### ReminderEditDialog.tsx

- 新建和编辑模式都显示"测试提醒"按钮
- 按钮禁用条件：`channel_ids` 为空时 disabled
- 统一调 `testReminderDryRun({ id: reminder?.id ?? 0, title, content, content_format, channel_ids })`
- 成功 toast 改为 "测试提醒已触发，请检查通知渠道接收情况"

### ChannelEditDialog.tsx

- 新建和编辑模式都显示"测试通知"按钮
- 统一调 `testChannelDryRun({ id: channel?.id ?? 0, type, config })`
- 成功 toast 改为 "测试通知已发送，请检查通知渠道接收情况"

## 边界情况

- channel_ids 为空时按钮禁用，hover 提示"请先选择通知渠道"
- 网络失败 toast 展示错误信息
- 测试进行中按钮显示 loading 状态
- 后端发送失败返回对应的 error message
