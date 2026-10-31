# 批量删除功能实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为提醒、通知、日志三个页面添加复选框批量删除功能，同时将提醒单条删除从软删除改为硬删除

**Architecture:** 后端新增三个批量删除 API，前端统一添加 Checkbox 组件、选中状态管理和批量删除交互

**Tech Stack:** Go(Gin+GORM) + React(TypeScript)+Tailwind CSS

---

### Task 1: 后端 — 修改提醒模型和单条删除为硬删除

**Files:**
- Modify: `backend/internal/models/reminder.go:41`
- Modify: `backend/internal/services/reminder_service.go:330-346`
- Modify: `backend/internal/services/log_service.go:174-181`（移除对 DeletedAt 的引用）

- [ ] **Step 1: 从 Reminder 模型中移除 DeletedAt 字段**

`backend/internal/models/reminder.go` 第 41 行，删除 `DeletedAt sql.NullTime` 行：

```go
// 删除这一整行：
DeletedAt sql.NullTime `gorm:"index" json:"-"` // 软删
```

- [ ] **Step 2: 修改 ReminderService.Delete 为硬删除**

`backend/internal/services/reminder_service.go:331-346`，将 `s.DB.Delete(r)` 改为 `s.DB.Unscoped().Delete(r)`：

```go
// Delete 硬删除一条提醒并从调度器移除。
func (s *ReminderService) Delete(id uint) error {
	r, err := s.getOrNotFound(id)
	if err != nil {
		return err
	}
	if err := s.DB.Unscoped().Delete(r).Error; err != nil {
		return err
	}
	if s.Engine != nil {
		s.Engine.Remove(id)
	}
	if s.ConfirmMgr != nil {
		s.ConfirmMgr.CancelByReminderID(id)
	}
	return nil
}
```

- [ ] **Step 3: 移除 LogService 中对 r.DeletedAt.Valid 的引用**

`backend/internal/services/log_service.go:178-181`，由于 Reminder 模型不再有 DeletedAt 字段，需要移除相关引用：

```go
// 将
var r models.Reminder
reminderTitle := dl.Title
reminderDeleted := false
if err := s.DB.Unscoped().First(&r, dl.ReminderID).Error; err == nil {
	reminderTitle = r.Title
	reminderDeleted = r.DeletedAt.Valid
}

// 改为
var r models.Reminder
reminderTitle := dl.Title
reminderDeleted := false
if err := s.DB.Unscoped().First(&r, dl.ReminderID).Error; err == nil {
	reminderTitle = r.Title
}
```

- [ ] **Step 4: 更新 LogView SQL JOIN 查询中 reminder_deleted 的引用**

`backend/internal/services/log_service.go:59`，移除 SQL 中 `r.deleted_at IS NOT NULL AS reminder_deleted`：

```go
// 将
q := s.DB.Model(&models.DeliveryLog{}).
	Select("delivery_logs.*, COALESCE(r.title, delivery_logs.title) AS reminder_title, r.deleted_at IS NOT NULL AS reminder_deleted").
	Joins("LEFT JOIN reminders r ON r.id = delivery_logs.reminder_id")

// 改为
q := s.DB.Model(&models.DeliveryLog{}).
	Select("delivery_logs.*, COALESCE(r.title, delivery_logs.title) AS reminder_title").
	Joins("LEFT JOIN reminders r ON r.id = delivery_logs.reminder_id")
```

- [ ] **Step 5: 移除 row 结构体中的 ReminderDeleted 字段**

`backend/internal/services/log_service.go:88-92`，移除 `ReminderDeleted` 字段：

```go
// 将
type row struct {
	models.DeliveryLog
	ReminderTitle   string `gorm:"column:reminder_title"`
	ReminderDeleted bool   `gorm:"column:reminder_deleted"`
}

// 改为
type row struct {
	models.DeliveryLog
	ReminderTitle string `gorm:"column:reminder_title"`
}
```

- [ ] **Step 6: 更新 LogView 构造处移除 ReminderDeleted**

`backend/internal/services/log_service.go:142-160`，移除两处 `ReminderDeleted` 赋值：

```go
views = append(views, &LogView{
	DeliveryLog:     main.DeliveryLog,
	ReminderTitle:   main.ReminderTitle,
	// ReminderDeleted: main.ReminderDeleted,  // 移除
})
// ...
views = append(views, &LogView{
	DeliveryLog:     sub.DeliveryLog,
	ReminderTitle:   sub.ReminderTitle,
	// ReminderDeleted: sub.ReminderDeleted,  // 移除
})
```

- [ ] **Step 7: 移除前端 LogView 类型中的 reminder_deleted**

`frontend/src/types/index.ts:139`，从 `DeliveryLog` 接口中移除 `reminder_deleted` 字段：

```typescript
export interface DeliveryLog {
  id: number
  reminder_id: number
  fired_at: string
  title: string
  content: string
  status: LogStatus
  confirmed: boolean
  confirmed_at?: string
  confirm_chain_id?: string
  confirm_url?: string
  retry_round: number
  source: string
  created_at: string
  reminder_title: string
  // reminder_deleted: boolean  // 删除
  attempts?: DeliveryAttempt[]
}
```

- [ ] **Step 8: 编译确认**

```bash
cd backend && go build ./...
```

Expected: `go build ./...` 成功无错误。


### Task 2: 后端 — 提醒批量删除 API

**Files:**
- Modify: `backend/internal/services/reminder_service.go`（新增 BatchDelete 方法）
- Modify: `backend/internal/handlers/reminder.go`（新增 BatchDelete 处理器）
- Modify: `backend/internal/router/router.go`（注册路由）

- [ ] **Step 1: 在 ReminderService 中新增 BatchDelete 方法**

在 `backend/internal/services/reminder_service.go` 中 Delete 方法之后新增：

```go
// BatchDelete 批量硬删除提醒并从调度器移除。
func (s *ReminderService) BatchDelete(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			if err := tx.Unscoped().Delete(&models.Reminder{}, id).Error; err != nil {
				return err
			}
			if s.Engine != nil {
				s.Engine.Remove(id)
			}
			if s.ConfirmMgr != nil {
				s.ConfirmMgr.CancelByReminderID(id)
			}
		}
		return nil
	})
}
```

- [ ] **Step 2: 在 ReminderHandler 中新增 BatchDelete 处理器**

在 `backend/internal/handlers/reminder.go` 末尾新增：

```go
// BatchDelete DELETE /api/reminders/batch
func (h *ReminderHandler) BatchDelete(c *gin.Context) {
	var in struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	if len(in.IDs) == 0 {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "ids 不能为空"))
		return
	}
	if err := h.Svc.BatchDelete(in.IDs); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}
```

注意：需要确认 import 中已包含 `middleware` 和 `services`，这两个应该已经在文件顶部的 import 中。

- [ ] **Step 3: 注册提醒批量删除路由**

`backend/internal/router/router.go:148-158`，在 reminders 路由组中新增：

```go
reminders := protected.Group("/reminders")
{
	reminders.GET("", reminderHandler.List)
	reminders.POST("", reminderHandler.Create)
	reminders.POST("/preview", reminderHandler.Preview)
	reminders.GET("/upcoming", reminderHandler.Upcoming)
	reminders.GET("/:id", reminderHandler.Get)
	reminders.PUT("/:id", reminderHandler.Update)
	reminders.DELETE("/:id", reminderHandler.Delete)
	reminders.DELETE("/batch", reminderHandler.BatchDelete)  // 新增（放在 /:id 之前以优先匹配精确路径）
	reminders.PATCH("/:id/toggle", reminderHandler.Toggle)
	reminders.POST("/test-dry", reminderHandler.TestDryRun)
}
```

- [ ] **Step 4: 编译确认**

```bash
cd backend && go build ./...
```

Expected: `go build ./...` 成功无错误。


### Task 3: 后端 — 通知批量删除 API

**Files:**
- Modify: `backend/internal/services/channel_service.go`（新增 BatchDelete 方法）
- Modify: `backend/internal/handlers/channel.go`（新增 BatchDelete 处理器）
- Modify: `backend/internal/router/router.go`（注册路由）

- [ ] **Step 1: 在 ChannelService 中新增 BatchDelete 方法**

在 `backend/internal/services/channel_service.go` Delete 方法之后新增：

```go
// BatchDelete 批量删除通知通道。
func (s *ChannelService) BatchDelete(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		// 先清理关联表
		if err := tx.Where("channel_id IN ?", ids).Delete(&models.ReminderChannel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Channel{}, ids).Error
	})
}
```

- [ ] **Step 2: 在 ChannelHandler 中新增 BatchDelete 处理器**

在 `backend/internal/handlers/channel.go` Delete 方法之后新增：

```go
// BatchDelete DELETE /api/channels/batch
func (h *ChannelHandler) BatchDelete(c *gin.Context) {
	var in struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	if len(in.IDs) == 0 {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "ids 不能为空"))
		return
	}
	if err := h.Svc.BatchDelete(in.IDs); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}
```

需要确认 import 中有 `middleware`。

- [ ] **Step 3: 注册通知批量删除路由**

`backend/internal/router/router.go:135-145`，在 channels 路由组中新增：

```go
channels := protected.Group("/channels")
{
	channels.GET("", channelHandler.List)
	channels.POST("", channelHandler.Create)
	channels.GET("/:id", channelHandler.Get)
	channels.PUT("/:id", channelHandler.Update)
	channels.DELETE("/:id", channelHandler.Delete)
	channels.DELETE("/batch", channelHandler.BatchDelete)  // 新增
	channels.PATCH("/:id/toggle", channelHandler.Toggle)
	channels.POST("/test-dry", channelHandler.TestDryRun)
	channels.GET("/stats", channelHandler.Stats)
}
```

- [ ] **Step 4: 编译确认**

```bash
cd backend && go build ./...
```

Expected: `go build ./...` 成功无错误。


### Task 4: 后端 — 日志批量删除 API

**Files:**
- Modify: `backend/internal/services/log_service.go`（新增 BatchDelete 方法）
- Modify: `backend/internal/handlers/log.go`（新增 BatchDelete 处理器）
- Modify: `backend/internal/router/router.go`（注册路由）

- [ ] **Step 1: 在 LogService 中新增 BatchDelete 方法**

在 `backend/internal/services/log_service.go` Purge 方法之前新增：

```go
// BatchDelete 按 ID 批量删除日志及关联的投递尝试。
func (s *LogService) BatchDelete(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("delivery_log_id IN ?", ids).Delete(&models.DeliveryAttempt{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", ids).Delete(&models.DeliveryLog{}).Error; err != nil {
			return err
		}
		// 清理孤立的 confirm_tokens
		return tx.Where("delivery_log_id NOT IN (SELECT id FROM delivery_logs)").
			Delete(&models.ConfirmToken{}).Error
	})
}
```

- [ ] **Step 2: 在 LogHandler 中新增 BatchDelete 处理器**

在 `backend/internal/handlers/log.go` Purge 方法之后新增：

```go
// BatchDelete DELETE /api/logs/batch
func (h *LogHandler) BatchDelete(c *gin.Context) {
	var in struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}
	if len(in.IDs) == 0 {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "ids 不能为空"))
		return
	}
	if err := h.Svc.BatchDelete(in.IDs); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}
```

- [ ] **Step 3: 注册日志批量删除路由**

`backend/internal/router/router.go:160-166`，在 logs 路由组中新增：

```go
logs := protected.Group("/logs")
{
	logs.GET("", logHandler.List)
	logs.GET("/count", logHandler.PurgeCount)
	logs.GET("/:id", logHandler.GetDetail)
	logs.DELETE("", logHandler.Purge)
	logs.DELETE("/batch", logHandler.BatchDelete)  // 新增
}
```

- [ ] **Step 4: 编译确认**

```bash
cd backend && go build ./...
```

Expected: `go build ./...` 成功无错误。


### Task 5: 前端 — 创建 Checkbox UI 组件

**Files:**
- Create: `frontend/src/components/ui/checkbox.tsx`

- [ ] **Step 1: 创建 Checkbox 组件**

`frontend/src/components/ui/checkbox.tsx`：

```tsx
import * as React from 'react'
import * as CheckboxPrimitive from '@radix-ui/react-checkbox'
import { Check } from 'lucide-react'
import { cn } from '@/lib/utils'

const Checkbox = React.forwardRef<
  React.ElementRef<typeof CheckboxPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>
>(({ className, ...props }, ref) => (
  <CheckboxPrimitive.Root
    ref={ref}
    className={cn(
      'peer h-4 w-4 shrink-0 rounded-sm border border-primary ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground',
      className,
    )}
    {...props}
  >
    <CheckboxPrimitive.Indicator className={cn('flex items-center justify-center text-current')}>
      <Check className="h-4 w-4" />
    </CheckboxPrimitive.Indicator>
  </CheckboxPrimitive.Root>
))
Checkbox.displayName = CheckboxPrimitive.Root.displayName

export { Checkbox }
```

- [ ] **Step 2: 安装 Radix Checkbox 依赖**

```bash
cd frontend && npm install @radix-ui/react-checkbox
```

Expected: npm install 成功。

- [ ] **Step 3: 验证构建**

```bash
cd frontend && npx tsc --noEmit 2>&1 | head -30
```

Expected: 无类型错误。


### Task 6: 前端 — 新增批量删除 API 函数

**Files:**
- Modify: `frontend/src/lib/api.ts`

- [ ] **Step 1: 新增三个批量删除函数**

在 `frontend/src/lib/api.ts` 末尾新增：

```typescript
// --- 批量删除 ---

export async function batchDeleteReminders(ids: number[]) {
  await api.delete<ApiResponse<null>>('/reminders/batch', { data: { ids } })
}

export async function batchDeleteChannels(ids: number[]) {
  await api.delete<ApiResponse<null>>('/channels/batch', { data: { ids } })
}

export async function batchDeleteLogs(ids: number[]) {
  await api.delete<ApiResponse<null>>('/logs/batch', { data: { ids } })
}
```

注意：axios DELETE 请求发送 body 需要使用 `data` 字段（与 POST/PUT 不同）。`api.delete` 的签名是 `delete(url, config?)`，所以 `{ data: { ids } }` 作为第二个参数传入。


### Task 7: 前端 — 提醒页批量删除功能

**Files:**
- Modify: `frontend/src/pages/reminders/index.tsx`

- [ ] **Step 1: 新增状态和导入**

在 reminders/index.tsx `import` 区域增加导入：

```tsx
import { Checkbox } from '@/components/ui/checkbox'
import { batchDeleteReminders } from '@/lib/api'
```

在 state 区域新增：

```tsx
const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
const [batchDeleting, setBatchDeleting] = useState(false)
```

- [ ] **Step 2: 添加选中操作方法**

在 `handleDuplicate` 之后新增：

```tsx
function toggleSelect(id: number) {
  setSelectedIds((prev) => {
    const next = new Set(prev)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    return next
  })
}

function toggleSelectAll() {
  if (selectedIds.size === items.length) {
    setSelectedIds(new Set())
  } else {
    setSelectedIds(new Set(items.map((r) => r.id)))
  }
}

async function handleBatchDelete() {
  setBatchDeleting(true)
  try {
    await batchDeleteReminders(Array.from(selectedIds))
    toast({ title: `已删除 ${selectedIds.size} 条提醒`, variant: 'success' })
    setSelectedIds(new Set())
    setTotal((n) => Math.max(0, n - selectedIds.size))
    const remainAfterDelete = items.length - selectedIds.size
    if (remainAfterDelete === 0 && offset > 0) {
      setOffset(Math.max(0, offset - limit))
    } else {
      await refresh()
    }
  } catch (err) {
    toast({ title: '批量删除失败', description: String(err), variant: 'destructive' })
  } finally {
    setBatchDeleting(false)
  }
}
```

- [ ] **Step 3: 在工具栏添加批量删除按钮**

`PageHeader` 中「新建提醒」按钮之前新增：

```tsx
<PageHeader title="提醒">
  <div className="flex items-center gap-2">
    {selectedIds.size > 0 && (
      <>
        <span className="text-sm text-muted-foreground">已选 {selectedIds.size} 项</span>
        <ConfirmDialog
          open={false}
          title="批量删除提醒"
          description={`确认删除选中的 ${selectedIds.size} 条提醒？该操作不可撤销。`}
          confirmText="删除"
          destructive
          loading={batchDeleting}
          onConfirm={handleBatchDelete}
          onCancel={() => {}}
        >
          <Button size="sm" variant="destructive" onClick={() => {}}>
            <Trash2 className="h-4 w-4 mr-1" />
            删除选中 ({selectedIds.size})
          </Button>
        </ConfirmDialog>
      </>
    )}
    <Button size="sm" onClick={() => setCreating(true)}>
      <Plus className="h-4 w-4 mr-1" />
      新建提醒
    </Button>
  </div>
</PageHeader>
```

等等，这里的 ConfirmDialog 需要改成受控模式。让我重新思考 - 我需要添加一个 `batchConfirmOpen` 状态来控制确认对话框。

重新组织 Step 3:

新增状态：

```tsx
const [batchConfirmOpen, setBatchConfirmOpen] = useState(false)
```

PageHeader 中的按钮：

```tsx
<PageHeader title="提醒">
  <div className="flex items-center gap-2">
    {selectedIds.size > 0 && (
      <Button size="sm" variant="destructive" onClick={() => setBatchConfirmOpen(true)}>
        <Trash2 className="h-4 w-4 mr-1" />
        删除选中 ({selectedIds.size})
      </Button>
    )}
    <Button size="sm" onClick={() => setCreating(true)}>
      <Plus className="h-4 w-4 mr-1" />
      新建提醒
    </Button>
  </div>
</PageHeader>
```

表格表头新增全选列：

```tsx
<thead className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
  <tr>
    <th className="px-4 py-2.5 w-10">
      <Checkbox
        checked={items.length > 0 && selectedIds.size === items.length}
        onCheckedChange={toggleSelectAll}
      />
    </th>
    <th className="px-4 py-2.5 w-[12rem]">标题</th>
    {/* ... 其余列 ... */}
  </tr>
</thead>
```

表格行新增复选框：

```tsx
<tr key={r.id} className="border-b last:border-b-0 hover:bg-muted/30">
  <td className="px-4 py-2.5">
    <Checkbox
      checked={selectedIds.has(r.id)}
      onCheckedChange={() => toggleSelect(r.id)}
    />
  </td>
  {/* ... 其余列 ... */}
</tr>
```

移动端卡片也加复选框。

最后，在页面底部附近新增批量删除确认对话框：

```tsx
<ConfirmDialog
  open={batchConfirmOpen}
  title="批量删除提醒"
  description={`确认删除选中的 ${selectedIds.size} 条提醒？该操作不可撤销。`}
  confirmText="删除"
  destructive
  loading={batchDeleting}
  onConfirm={() => {
    handleBatchDelete()
    setBatchConfirmOpen(false)
  }}
  onCancel={() => setBatchConfirmOpen(false)}
/>
```


### Task 8: 前端 — 通知页批量删除功能

**Files:**
- Modify: `frontend/src/pages/channels/index.tsx`

与 Task 7 类似，在通知页添加相同的批量删除交互模式。

- [ ] **Step 1: 新增导入和状态**

```tsx
import { Checkbox } from '@/components/ui/checkbox'
import { batchDeleteChannels } from '@/lib/api'
```

新增 state：
```tsx
const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
const [batchDeleting, setBatchDeleting] = useState(false)
const [batchConfirmOpen, setBatchConfirmOpen] = useState(false)
```

- [ ] **Step 2: 添加选中操作方法**

```tsx
function toggleSelect(id: number) {
  setSelectedIds((prev) => {
    const next = new Set(prev)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    return next
  })
}

function toggleSelectAll() {
  if (selectedIds.size === items.length) {
    setSelectedIds(new Set())
  } else {
    setSelectedIds(new Set(items.map((ch) => ch.id)))
  }
}

async function handleBatchDelete() {
  setBatchDeleting(true)
  try {
    await batchDeleteChannels(Array.from(selectedIds))
    toast({ title: `已删除 ${selectedIds.size} 条通知`, variant: 'success' })
    setSelectedIds(new Set())
    const remainAfterDelete = items.length - selectedIds.size
    if (remainAfterDelete === 0 && offset > 0) {
      setOffset(Math.max(0, offset - limit))
    } else {
      await refresh()
    }
  } catch (err) {
    toast({ title: '批量删除失败', description: String(err), variant: 'destructive' })
  } finally {
    setBatchDeleting(false)
  }
}
```

- [ ] **Step 3: 在 PageHeader 添加批量删除按钮**

```tsx
<PageHeader title="通知">
  <div className="flex items-center gap-2">
    {selectedIds.size > 0 && (
      <Button size="sm" variant="destructive" onClick={() => setBatchConfirmOpen(true)}>
        <Trash2 className="h-4 w-4 mr-1" />
        删除选中 ({selectedIds.size})
      </Button>
    )}
    <Button size="sm" onClick={() => setCreating(true)}>
      <Plus className="h-4 w-4 mr-1" />
      新建通知
    </Button>
  </div>
</PageHeader>
```

- [ ] **Step 4: 在表格添加复选框列**

表头（在「名称」列前新增全选列）：
```tsx
<th className="px-4 py-2.5 w-10">
  <Checkbox
    checked={items.length > 0 && selectedIds.size === items.length}
    onCheckedChange={toggleSelectAll}
  />
</th>
```

表格行：
```tsx
<td className="px-4 py-2.5">
  <Checkbox
    checked={selectedIds.has(ch.id)}
    onCheckedChange={() => toggleSelect(ch.id)}
  />
</td>
```

移动端卡片同样在顶部添加复选框。

- [ ] **Step 5: 添加确认对话框（在页面底部）**

```tsx
<ConfirmDialog
  open={batchConfirmOpen}
  title="批量删除通知"
  description={`确认删除选中的 ${selectedIds.size} 条通知？该操作不可撤销。`}
  confirmText="删除"
  destructive
  loading={batchDeleting}
  onConfirm={() => {
    handleBatchDelete()
    setBatchConfirmOpen(false)
  }}
  onCancel={() => setBatchConfirmOpen(false)}
/>
```


### Task 9: 前端 — 日志页批量删除功能

**Files:**
- Modify: `frontend/src/pages/logs/index.tsx`

- [ ] **Step 1: 新增导入和状态**

```tsx
import { Checkbox } from '@/components/ui/checkbox'
import { batchDeleteLogs } from '@/lib/api'
```

新增 state：
```tsx
const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
const [batchDeleting, setBatchDeleting] = useState(false)
const [batchConfirmOpen, setBatchConfirmOpen] = useState(false)
```

- [ ] **Step 2: 添加选中操作方法**

```tsx
function toggleSelect(id: number) {
  setSelectedIds((prev) => {
    const next = new Set(prev)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    return next
  })
}

function toggleSelectAll() {
  const allIds = new Set(items.map((log) => log.id))
  if (selectedIds.size === items.length && [...selectedIds].every((id) => allIds.has(id))) {
    setSelectedIds(new Set())
  } else {
    setSelectedIds(allIds)
  }
}

async function handleBatchDelete() {
  setBatchDeleting(true)
  try {
    await batchDeleteLogs(Array.from(selectedIds))
    toast({ title: `已删除 ${selectedIds.size} 条日志`, variant: 'success' })
    setSelectedIds(new Set())
    const remainAfterDelete = items.length - selectedIds.size
    if (remainAfterDelete === 0 && offset > 0) {
      setOffset(Math.max(0, offset - limit))
    } else {
      await refresh()
    }
  } catch (err) {
    toast({ title: '批量删除失败', description: String(err), variant: 'destructive' })
  } finally {
    setBatchDeleting(false)
  }
}
```

- [ ] **Step 3: 在 PageHeader 添加批量删除按钮**

```tsx
<PageHeader title="日志">
  <div className="flex flex-wrap items-center gap-2">
    {selectedIds.size > 0 && (
      <Button size="sm" variant="destructive" onClick={() => setBatchConfirmOpen(true)}>
        <Trash2 className="h-4 w-4 mr-1" />
        删除选中 ({selectedIds.size})
      </Button>
    )}
    {/* 原有的清理按钮... */}
  </div>
</PageHeader>
```

- [ ] **Step 4: 在桌面端表格添加复选框列**

表头（原有折叠列改为复选框列）：
```tsx
<th className="px-4 py-3 w-10">
  <Checkbox
    checked={items.length > 0 && selectedIds.size === items.length}
    onCheckedChange={toggleSelectAll}
  />
</th>
```

表格行（每行添加复选框，保留折叠按钮）：
```tsx
<td className="px-4 py-3">
  <div className="flex items-center gap-1">
    {subs.length > 0 && (
      <button onClick={() => toggleExpand(main.id)} className="p-1">
        {expandedRows.has(main.id) ? (
          <ChevronDown className="h-4 w-4" />
        ) : (
          <ChevronRight className="h-4 w-4" />
        )}
      </button>
    )}
    <Checkbox
      checked={selectedIds.has(main.id)}
      onCheckedChange={() => toggleSelect(main.id)}
    />
  </div>
</td>
```

移动端卡片在标题旁加复选框。

- [ ] **Step 5: 添加确认对话框**

```tsx
<ConfirmDialog
  open={batchConfirmOpen}
  title="批量删除日志"
  description={`确认删除选中的 ${selectedIds.size} 条日志？该操作不可撤销。`}
  confirmText="删除"
  destructive
  loading={batchDeleting}
  onConfirm={() => {
    handleBatchDelete()
    setBatchConfirmOpen(false)
  }}
  onCancel={() => setBatchConfirmOpen(false)}
/>
```
