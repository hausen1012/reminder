# 提醒与通知测试功能 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 统一新建/编辑模态框的"测试"按钮走 dry-run 接口，直接发送真实通知但不记录日志。

**Architecture:** 后端新增 `DispatchService.DryRun` / `ChannelService.DryRun` 无写入路径，前端统一调用新接口。旧 test 端点及对应 service/handler 代码一并删除。

**Tech Stack:** Go (Gin/GORM), TypeScript (React/axios)

---

### Task 1: DispatchService.DryRun — 核心发送路径（无 DB 写入）

**Files:**
- Modify: `backend/internal/services/dispatch_service.go`

dispatch_service.go 添加 DryRun 方法，以及对应的内部 helper `sendDryRun`。两个方法语义同 Run/sendWithRetry，但**不写 delivery_log / delivery_attempt**。错误仅通过 log.Printf 输出到控制台。

```go
// DryRun 发送测试消息，不落任何 delivery_log / delivery_attempt。
func (d *DispatchService) DryRun(ctx context.Context, channels []*models.Channel, msg notifier.Message) error {
    var wg sync.WaitGroup
    errCh := make(chan error, len(channels))
    for i := range channels {
        i := i
        ch := channels[i]
        wg.Add(1)
        go func() {
            defer wg.Done()
            if err := d.sendDryRun(ctx, ch, msg); err != nil {
                errCh <- fmt.Errorf("channel=%d name=%s type=%s: %w", ch.ID, ch.Name, ch.Type, err)
            }
        }()
    }
    wg.Wait()
    close(errCh)
    // 收集所有错误
    var errs []string
    for err := range errCh {
        errs = append(errs, err.Error())
    }
    if len(errs) > 0 {
        return fmt.Errorf("试发失败: %s", strings.Join(errs, "; "))
    }
    return nil
}

// sendDryRun 单通道试发（有重试，不写 attempt）。
func (d *DispatchService) sendDryRun(ctx context.Context, ch *models.Channel, msg notifier.Message) error {
    n, err := notifier.Get(ch.Type)
    if err != nil {
        return err
    }
    plainConfig, err := d.ChannelSvc.DecryptedConfig(ch)
    if err != nil {
        return fmt.Errorf("解密配置失败: %w", err)
    }
    for i, delay := range d.RetryDelays {
        if delay > 0 {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
        }
        if sendErr := n.Send(ctx, plainConfig, msg); sendErr == nil {
            return nil
        } else {
            log.Printf("[dispatch-dryrun] 通道发送失败 ch=%d name=%s attempt=%d: %v", ch.ID, ch.Name, i+1, sendErr)
            if notifier.IsPermanent(sendErr) {
                return sendErr
            }
        }
    }
    return fmt.Errorf("所有重试均失败")
}
```

**注意事项：**
- `sync` 已在 import 中；`fmt` 已在 import 中；`strings` 已在 import 中——只需确认即可。
- `log` 已在 import 中。

- [ ] **Step 1: 在 dispatch_service.go 末尾添加 `DryRun` 和 `sendDryRun` 方法**

- [ ] **Step 2: 编译验证**

Run: `cd backend && go build ./...`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add backend/internal/services/dispatch_service.go
git commit -m "feat(dispatch): add DryRun method without DB writes"
```

---

### Task 2: ReminderService + ChannelService DryRun 方法

**Files:**
- Modify: `backend/internal/services/reminder_service.go`
- Modify: `backend/internal/services/channel_service.go`

#### reminder_service.go

替换 `TestOnce` 为 `TestDryRun`。新方法从入参构建临时 Reminder，加载 channels，调 DispatchService.DryRun。

```go
// TestDryRun 用表单数据试发提醒，不写 delivery_log / delivery_attempt。
func (s *ReminderService) TestDryRun(ctx context.Context, title, content, contentFormat string, channelIDs []uint) error {
    if s.Dispatch == nil {
        return errors.New("dispatch 未初始化")
    }
    if len(channelIDs) == 0 {
        return middleware.NewAppError(middleware.CodeValidationFailed, "至少选择一个通知渠道")
    }
    if strings.TrimSpace(title) == "" {
        return middleware.NewAppError(middleware.CodeValidationFailed, "标题必填")
    }
    // 加载绑定的 enabled 通道
    channels, err := s.loadChannelsByIDs(channelIDs)
    if err != nil {
        return err
    }
    if len(channels) == 0 {
        return middleware.NewAppError(middleware.CodeValidationFailed, "没有可用的通知渠道")
    }

    now := time.Now()
    vars := buildVars(&models.Reminder{
        Title:       title,
        Content:     content,
        NextFireAt:  &now,
    }, now, now, s.Loc)

    body := notifier.Render(content, vars)
    rendered := notifier.Message{
        Subject: notifier.Render(title, vars),
        Body:    body,
        Format:  contentFormat,
        Vars:    vars,
    }
    return s.Dispatch.DryRun(ctx, channels, rendered)
}

// loadChannelsByIDs 根据 ID 列表加载 enabled 通道。
func (s *ReminderService) loadChannelsByIDs(ids []uint) ([]*models.Channel, error) {
    if len(ids) == 0 {
        return nil, nil
    }
    var rows []models.Channel
    if err := s.DB.Where("id IN ? AND enabled = ?", ids, true).Find(&rows).Error; err != nil {
        return nil, err
    }
    out := make([]*models.Channel, 0, len(rows))
    for i := range rows {
        out = append(out, &rows[i])
    }
    return out, nil
}
```

**注意：** `buildVars` 在当前文件中未使用（它在 `dispatch_service.go` 中定义），但因为两个文件在同一个 package `services` 下，可以直接调用。无需额外操作。

#### channel_service.go

添加 `DryRun` 方法。直接使用表单传入的明文 config 发送，不查 DB，不写日志。

```go
// DryRun 用表单配置试发通知，不写 delivery_log。
func (s *ChannelService) DryRun(ctx context.Context, chType string, config map[string]any) error {
    n, err := notifier.Get(chType)
    if err != nil {
        return middleware.NewAppError(middleware.CodeValidationFailed, err.Error())
    }
    // 配置可能含 _enc 后缀字段，dry-run 时前端传的是明文，直接序列化
    plainConfig, _ := json.Marshal(config)

    subject := "通道试发 - " + chType
    body := "这是来自 reminder2 的通道试发消息。\n类型：{{channel_type}}\n时间：{{now}}"
    vars := map[string]string{
        "channel_type": chType,
        "now":          time.Now().Format("2006-01-02 15:04:05"),
        "title":        subject,
        "content":      body,
    }
    rendered := notifier.Message{
        Subject: notifier.Render(subject, vars),
        Body:    notifier.Render(body, vars),
        Vars:    vars,
    }
    if err := n.Send(ctx, plainConfig, rendered); err != nil {
        log.Printf("[channel-dryrun] 试发失败 type=%s: %v", chType, err)
        return err
    }
    return nil
}
```

- [ ] **Step 1: 用 `TestDryRun` + `loadChannelsByIDs` 替换 reminder_service.go 中的 `TestOnce`**

- [ ] **Step 2: 在 channel_service.go 添加 `DryRun` 方法**

- [ ] **Step 3: 编译验证**

Run: `cd backend && go build ./...`
Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add backend/internal/services/reminder_service.go backend/internal/services/channel_service.go
git commit -m "feat(services): add TestDryRun and DryRun methods"
```

---

### Task 3: HTTP Handlers — TestDryRun

**Files:**
- Create: —（修改）
- Modify: `backend/internal/handlers/reminder.go`
- Modify: `backend/internal/handlers/channel.go`
- Modify: `backend/internal/router/router.go`

#### reminder.go

添加 `TestDryRun` handler（替换旧的 `Test` handler）。

```go
// TestDryRun POST /api/reminders/test-dry
func (h *ReminderHandler) TestDryRun(c *gin.Context) {
    var in struct {
        ID            uint   `json:"id"`
        Title         string `json:"title"`
        Content       string `json:"content"`
        ContentFormat string `json:"content_format"`
        ChannelIDs    []uint `json:"channel_ids"`
    }
    if err := c.ShouldBindJSON(&in); err != nil {
        abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
        return
    }
    if err := h.Svc.TestDryRun(c.Request.Context(), in.Title, in.Content, in.ContentFormat, in.ChannelIDs); err != nil {
        abortErr(c, err)
        return
    }
    successJSON(c, gin.H{"success": true})
}
```

**同样删除** 旧的 `Test` handler 方法（约 L151-164）。

#### channel.go

添加 `TestDryRun` handler（替换旧的 `Test` handler）。

```go
// TestDryRun POST /api/channels/test-dry
func (h *ChannelHandler) TestDryRun(c *gin.Context) {
    var in struct {
        ID     uint           `json:"id"`
        Type   string         `json:"type"`
        Config map[string]any `json:"config"`
    }
    if err := c.ShouldBindJSON(&in); err != nil {
        abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
        return
    }
    if err := h.Svc.DryRun(c.Request.Context(), in.Type, in.Config); err != nil {
        abortErr(c, err)
        return
    }
    successJSON(c, gin.H{"success": true})
}
```

**同样删除** 旧的 `Test` handler 方法。

#### router.go

替换路由注册：

```go
// 删除这两行：
// channels.POST("/:id/test", channelHandler.Test)
// reminders.POST("/:id/test", reminderHandler.Test)

// 添加这两行：
channels.POST("/test-dry", channelHandler.TestDryRun)
reminders.POST("/test-dry", reminderHandler.TestDryRun)
```

- [ ] **Step 1: 在 reminder.go 替换 `Test` → `TestDryRun`**

- [ ] **Step 2: 在 channel.go 替换 `Test` → `TestDryRun`**

- [ ] **Step 3: 在 router.go 替换路由注册**

- [ ] **Step 4: 编译验证**

Run: `cd backend && go build ./...`
Expected: 编译通过

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/reminder.go backend/internal/handlers/channel.go backend/internal/router/router.go
git commit -m "feat(handlers): replace test handlers with test-dry handlers"
```

---

### Task 4: 清理旧代码

**Files:**
- Modify: `backend/internal/services/dispatch_service.go`
- Modify: `backend/internal/services/reminder_service.go`（已修改，确认 TestOnce 已删除）
- Modify: `backend/internal/services/channel_service.go`（已修改，确认 Test 已删除）

**dispatch_service.go** 删除 `TestOnce` 方法（约 L288-307）。

**reminder_service.go** 确认 `TestOnce` 已被替换（Task 2 已做）。

**channel_service.go** 确认 `Test` 方法已删除（Task 3 handler 不再引用它）。

- [ ] **Step 1: 删除 dispatch_service.go 的 `TestOnce`**

- [ ] **Step 2: 检查 reminder_service.go 和 channel_service.go 确认旧方法已清理**

- [ ] **Step 3: 编译验证**

Run: `cd backend && go build ./...`
Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add backend/internal/services/dispatch_service.go
git commit -m "refactor(dispatch): remove TestOnce"
```

---

### Task 5: 前端 — API 函数

**Files:**
- Modify: `frontend/src/lib/api.ts`

删除旧函数并添加新函数：

**删除：**
```typescript
// 删除 testReminder
export async function testReminder(id: number) { ... }

// 删除 testChannel
export async function testChannel(id: number, body?: { subject?: string; body?: string }) { ... }
```

**添加：**
```typescript
export async function testReminderDryRun(input: {
  id?: number
  title: string
  content: string
  content_format: string
  channel_ids: number[]
}) {
  const res = await api.post<ApiResponse<{ success: boolean }>>('/reminders/test-dry', input)
  return res.data.data
}

export async function testChannelDryRun(input: {
  id?: number
  type: ChannelType
  config: Record<string, unknown>
}) {
  const res = await api.post<ApiResponse<{ success: boolean }>>('/channels/test-dry', input)
  return res.data.data
}
```

**注意：** `ChannelTestResult` 接口在 types/index.ts 中定义。如果不再被引用可以后续清理，但这里暂不删以减少范围。

- [ ] **Step 1: 删除 `testReminder` 和 `testChannel`，添加 `testReminderDryRun` 和 `testChannelDryRun`**

- [ ] **Step 2: 编译验证（TypeScript 类型检查）**

Run: `cd frontend && npx tsc --noEmit`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/api.ts
git commit -m "feat(api): add test-dry-run API functions, remove old test API"
```

---

### Task 6: 前端 — ReminderEditDialog 测试按钮

**Files:**
- Modify: `frontend/src/components/reminders/ReminderEditDialog.tsx`

改动点：
1. 删除 `testReminder` import
2. 添加 `testReminderDryRun` import
3. "测试提醒"按钮不再隐藏在 `isEdit &&` 条件后，始终显示
4. 统一调用 `testReminderDryRun`
5. 更新成功消息
6. 按钮禁用条件：channel_ids 为空时 disabled + 展示 tooltip 提示

**handleTest 方法**（替换原有）：

```tsx
async function handleTest() {
  if (input.channel_ids.length === 0) {
    toast({ title: '请先选择通知渠道', variant: 'destructive' })
    return
  }
  setTesting(true)
  try {
    await testReminderDryRun({
      id: reminder?.id ?? 0,
      title: input.title,
      content: input.content,
      content_format: input.content_format,
      channel_ids: input.channel_ids,
    })
    toast({ title: '测试提醒已触发', description: '请检查通知渠道是否收到消息。', variant: 'success' })
  } catch (err) {
    toast({ title: '测试失败', description: String(err), variant: 'destructive' })
  } finally {
    setTesting(false)
  }
}
```

**按钮 JSX**（删除 `isEdit &&` 条件）：

```tsx
<Button
  type="button"
  variant="secondary"
  onClick={handleTest}
  disabled={testing || input.channel_ids.length === 0}
  title={input.channel_ids.length === 0 ? '请先选择通知渠道' : '立即触发一次测试发送'}
>
  {testing ? '测试中…' : '测试提醒'}
</Button>
```

- [ ] **Step 1: 更新 ReminderEditDialog.tsx：替换 import、handleTest、按钮 JSX**

- [ ] **Step 2: 编译验证**

Run: `cd frontend && npx tsc --noEmit`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/reminders/ReminderEditDialog.tsx
git commit -m "feat(ReminderEditDialog): test button always visible, use test-dry-run"
```

---

### Task 7: 前端 — ChannelEditDialog 测试按钮

**Files:**
- Modify: `frontend/src/components/channels/ChannelEditDialog.tsx`

改动点同 Task 6，对称处理。

**删除/新增 import：**
- 删除 `testChannel` import
- 添加 `testChannelDryRun` import

**handleTest 方法**（替换原有）：

```tsx
async function handleTest() {
  setTesting(true)
  try {
    await testChannelDryRun({
      id: channel?.id ?? 0,
      type,
      config,
    })
    toast({ title: '测试通知已发送', description: '请检查通知渠道是否收到消息。', variant: 'success' })
  } catch (err) {
    const msg = (err as { response?: { data?: { error?: { message?: string } } } })?.response?.data?.error?.message
    toast({ title: '测试失败', description: msg ?? String(err), variant: 'destructive' })
  } finally {
    setTesting(false)
  }
}
```

**按钮 JSX**（删除 `isEdit &&` 条件）：

```tsx
<Button type="button" variant="secondary" onClick={handleTest} disabled={testing} title="向该通知发送一条测试消息">
  {testing ? '试发中…' : '测试通知'}
</Button>
```

- [ ] **Step 1: 更新 ChannelEditDialog.tsx：替换 import、handleTest、按钮 JSX**

- [ ] **Step 2: 编译验证**

Run: `cd frontend && npx tsc --noEmit`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/channels/ChannelEditDialog.tsx
git commit -m "feat(ChannelEditDialog): test button always visible, use test-dry-run"
```

---

### Task 8: 最终验证

- [ ] **Step 1: 后端完整编译**

Run: `cd backend && go build ./...`
Expected: 编译通过，无任何 warning

- [ ] **Step 2: 前端完整编译**

Run: `cd frontend && npx tsc --noEmit`
Expected: 编译通过，无任何 error

- [ ] **Step 3: 检查无引用的旧符号残留**

Run: `cd backend && grep -rn "TestOnce\|\.Test(" --include="*.go" | grep -v "_test.go"`

Expected: 只有新定义的 TestDryRun 被引用，无残留的 TestOnce 或 old-style Test

- [ ] **Step 3 补充：Go 层面只查找 `TestOnce` 和 `func.*Test(`**

Run: `cd backend && grep -rn "TestOnce" --include="*.go" | grep -v "_test.go"`
Expected: 无输出（已全部删除）

- [ ] **Step 4: 前端检查无残留旧引用**

Run: `cd frontend && grep -rn "testReminder\|testChannel" --include="*.tsx" --include="*.ts" src/`
Expected: 只有 `testReminderDryRun` 和 `testChannelDryRun`，无 `testReminder(` 或 `testChannel(`

- [ ] **Step 5: 最终提交**

```bash
git add -A
git status
git commit -m "chore: final cleanup after test-dry-run refactor"
```
