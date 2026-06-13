# 提醒内容格式支持 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 superpowers:subagent-driven-development 逐任务实施。步骤使用复选框（`- [ ]`）追踪。

**目标：** 用户创建提醒时选择内容格式（text / markdown / html），各通道根据自身能力自动适配或降级为纯文本。

**架构：** Reminder 模型新增 `content_format` 字段 → 分发时传给 `Message.Format` → 每个 Notifier 在 `Send()` 中根据格式做转换（SMTP 发 HTML 邮件、钉钉/企微按格式选 msgtype、不支持的格式 strip 后降级 text）。

**Tech Stack:** Go (backend, no markdown→HTML lib yet → 需要引入 goldmark), React + TypeScript (frontend)

---

## 文件结构

### 修改文件

| 文件 | 变更内容 |
|------|---------|
| `backend/internal/models/reminder.go` | Reminder 结构体新增 `ContentFormat` |
| `backend/internal/notifier/notifier.go` | `Message` 新增 `Format` 字段 |
| `backend/internal/notifier/format.go` **(新)** | `StripHTML()`, `StripMarkdown()` 工具函数 |
| `backend/internal/notifier/smtp.go` | 邮件按格式切换 Content-Type / 内容 |
| `backend/internal/notifier/dingtalk.go` | 去掉 `MsgType` 配置引用，改用 `msg.Format` |
| `backend/internal/notifier/wecom.go` | 去掉 `MsgType` 配置引用，改用 `msg.Format` |
| `backend/internal/notifier/log.go` | 按格式 strip 后输出 |
| `backend/internal/services/dispatch_service.go` | 构建 Message 时设置 `Format` |
| `backend/internal/services/reminder_service.go` | validate 增加 `content_format` 校验, Create/Update 写入 |
| `backend/go.mod` | 新增 `github.com/yuin/goldmark` 依赖 |
| `frontend/src/types/index.ts` | 新增 `ContentFormat` 类型 |
| `frontend/src/components/reminders/ReminderEditDialog.tsx` | 新增格式切换 UI |
| `frontend/src/pages/reminders/index.tsx` | 列表行展示格式标签（可选） |

---

### Task 1: 后端模型层 — Reminder + Message 新增 ContentFormat

**文件：**
- 修改: `backend/internal/models/reminder.go`
- 修改: `backend/internal/notifier/notifier.go`

- [ ] **1a: Reminder 模型新增 ContentFormat 字段**

在 `backend/internal/models/reminder.go` 的 `Content` 字段后插入：

```go
type Reminder struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Title   string `gorm:"size:200;not null" json:"title"`
	Content string `gorm:"type:text" json:"content"`
	ContentFormat string `gorm:"size:8;default:text" json:"content_format"`
	// ... 其余不变
```

- [ ] **1b: Message 结构体新增 Format 字段**

在 `backend/internal/notifier/notifier.go` 的 `Message` 结构体中增加：

```go
type Message struct {
	Subject string
	Body    string
	Format  string // "text" | "markdown" | "html"
	Vars    map[string]string
}
```

---

### Task 2: 通用格式转换工具

**文件：**
- 创建: `backend/internal/notifier/format.go`

- [ ] **2a: 实现 StripHTML 和 StripMarkdown**

```go
package notifier

import (
	"regexp"
	"strings"
)

var (
	htmlTagRe = regexp.MustCompile(`<[^>]*>`)
	markdownRe = regexp.MustCompile(`[#*_~` + "`" + `>\[\]()\-!|]+`)
	// 更精准的 markdown strip：只去掉标记结构，保留文字
	mdHeaderRe    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdBoldRe      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdItalicRe    = regexp.MustCompile(`\*(.+?)\*`)
	mdCodeRe      = regexp.MustCompile("`{1,3}[^`\n]+`{1,3}")
	mdLinkRe      = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	mdImageRe     = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
	mdBlockquoteRe = regexp.MustCompile(`(?m)^>\s+`)
	mdHrRe        = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
	mdListRe      = regexp.MustCompile(`(?m)^[\s]*[-*+]\s+`)
	mdNumListRe   = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
)

// StripHTML 去除所有 HTML 标签，返回纯文本。
func StripHTML(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}

// StripMarkdown 去除常见 Markdown 标记，返回纯文本。
func StripMarkdown(s string) string {
	result := s
	result = mdHeaderRe.ReplaceAllString(result, "")
	result = mdImageRe.ReplaceAllString(result, "$1")
	result = mdLinkRe.ReplaceAllString(result, "$1")
	result = mdBoldRe.ReplaceAllString(result, "$1")
	result = mdItalicRe.ReplaceAllString(result, "$1")
	result = mdCodeRe.ReplaceAllString(result, "")
	result = mdBlockquoteRe.ReplaceAllString(result, "")
	result = mdHrRe.ReplaceAllString(result, "")
	result = mdListRe.ReplaceAllString(result, "")
	result = mdNumListRe.ReplaceAllString(result, "")
	result = htmlTagRe.ReplaceAllString(result, "") // 安全兜底
	// 合并多余空白
	result = strings.Join(strings.Fields(result), " ")
	return strings.TrimSpace(result)
}
```

---

### Task 3: 各 Notifier 适配格式转换

**文件：**
- 修改: `backend/internal/notifier/smtp.go`
- 修改: `backend/internal/notifier/dingtalk.go`
- 修改: `backend/internal/notifier/wecom.go`
- 修改: `backend/internal/notifier/log.go`

- [ ] **3a: SMTP 按格式切换邮件类型**

在 `buildMIME` 中根据 `msg.Format` 选择 Content-Type：

```go
func buildMIME(cfg SMTPConfig, msg Message) string {
	from := cfg.FromAddr
	if cfg.FromName != "" {
		from = fmt.Sprintf("=?UTF-8?B?%s?= <%s>", base64Encode(cfg.FromName), cfg.FromAddr)
	}
	subjectEncoded := fmt.Sprintf("=?UTF-8?B?%s?=", base64Encode(msg.Subject))

	var contentType string
	var body string
	switch msg.Format {
	case "html":
		contentType = "text/html; charset=UTF-8"
		body = msg.Body
	case "markdown":
		contentType = "text/html; charset=UTF-8"
		body = renderMarkdownToHTML(msg.Body)
	default: // "text" 或未知
		contentType = "text/plain; charset=UTF-8"
		body = msg.Body
	}

	return strings.Join([]string{
		"From: " + from,
		"To: " + strings.Join(cfg.To, ", "),
		"Subject: " + subjectEncoded,
		"MIME-Version: 1.0",
		"Content-Type: " + contentType,
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n")
}
```

新增 `renderMarkdownToHTML` 函数（使用 goldmark）：

```go
import "github.com/yuin/goldmark"

var mdRenderer = goldmark.New()

func renderMarkdownToHTML(src string) string {
	var buf bytes.Buffer
	if err := mdRenderer.Convert([]byte(src), &buf); err != nil {
		return src // 降级返回原文
	}
	return buf.String()
}
```

- [ ] **3b: 安装 goldmark 依赖**

```bash
cd backend
go get github.com/yuin/goldmark
```

- [ ] **3c: 钉钉 — 改用 msg.Format 决定 msgtype**

修改 `buildDingTalkPayload`：

```go
func buildDingTalkPayload(cfg DingTalkConfig, msg Message) map[string]any {
	switch msg.Format {
	case "markdown":
		title := msg.Subject
		if title == "" {
			title = "提醒"
		}
		p := map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": title,
				"text":  msg.Body,
			},
		}
		if cfg.AtAll || len(cfg.AtMobiles) > 0 {
			p["at"] = map[string]any{"atMobiles": cfg.AtMobiles, "isAtAll": cfg.AtAll}
		}
		return p
	default:
		// format = "text" 或 降级逻辑
		body := msg.Body
		if msg.Format == "html" {
			body = StripHTML(msg.Body)
		}
		content := body
		if msg.Subject != "" {
			content = msg.Subject + "\n" + body
		}
		p := map[string]any{
			"msgtype": "text",
			"text":    map[string]any{"content": content},
		}
		if cfg.AtAll || len(cfg.AtMobiles) > 0 {
			p["at"] = map[string]any{"atMobiles": cfg.AtMobiles, "isAtAll": cfg.AtAll}
		}
		return p
	}
}
```

同时移除 `Send()` 中对 `cfg.MsgType` 的引用（约第 38-41 行的 `if cfg.MsgType == "" { cfg.MsgType = "text" }` 不再需要，用 `msg.Format` 代替）。

`DingTalkConfig` 中的 `MsgType` 字段保留不做修改（兼容已有配置，只是不再读取）。

- [ ] **3d: 企微 — 改用 msg.Format 决定 msgtype**

修改 `buildWeComPayload`：

```go
func buildWeComPayload(cfg WeComConfig, msg Message) map[string]any {
	switch msg.Format {
	case "markdown":
		return map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]any{"content": msg.Body},
		}
	default:
		body := msg.Body
		if msg.Format == "html" {
			body = StripHTML(msg.Body)
		}
		content := body
		if msg.Subject != "" {
			content = msg.Subject + "\n" + body
		}
		text := map[string]any{"content": content}
		if len(cfg.MentionedList) > 0 {
			text["mentioned_list"] = cfg.MentionedList
		}
		if len(cfg.MentionedMobileList) > 0 {
			text["mentioned_mobile_list"] = cfg.MentionedMobileList
		}
		return map[string]any{
			"msgtype": "text",
			"text":    text,
		}
	}
}
```

同样去掉 `Send()` 中对 `cfg.MsgType` 的引用。`WeComConfig` 中的 `MsgType` 字段保留兼容。

- [ ] **3e: Log — 按格式 strip 后输出**

修改 `logNotifier.Send()`：

```go
func (n *logNotifier) Send(_ context.Context, _ []byte, msg Message) error {
	body := msg.Body
	switch msg.Format {
	case "html":
		body = StripHTML(msg.Body)
	case "markdown":
		body = StripMarkdown(msg.Body)
	}
	log.Printf(
		"[log-notifier] %s | subject=%s | body=%s",
		time.Now().Format(time.RFC3339),
		msg.Subject,
		body,
	)
	fmt.Printf(
		"=== LOG NOTIFICATION ===\nTime: %s\nSubject: %s\nBody:\n%s\n========================\n",
		time.Now().Format(time.RFC3339),
		msg.Subject,
		body,
	)
	return nil
}
```

---

### Task 4: 分发服务传递 Format

**文件：**
- 修改: `backend/internal/services/dispatch_service.go`

- [ ] **4a: 在构建 Message 时设置 Format**

在第 108-112 行附近，渲染 Message 时传入 `Format`：

```go
rendered := notifier.Message{
	Subject: notifier.Render(r.Title, vars),
	Body:    notifier.Render(r.Content, vars),
	Format:  r.ContentFormat,
	Vars:    vars,
}
```

如果 `r.ContentFormat` 为空（历史数据），默认用 `"text"`：
- 这个由 GORM default 和 model 的 `default:text` 保证，如果 r 是从 DB 读出的，`ContentFormat` 不会为空。
- 安全起见也可以加：`if r.ContentFormat == "" { r.ContentFormat = "text" }`（可选，不影响功能）

---

### Task 5: Reminder Service 校验

**文件：**
- 修改: `backend/internal/services/reminder_service.go`

- [ ] **5a: validate 增加 content_format 校验**

在 `validate` 函数底部增加：

```go
if in.ContentFormat != "" && in.ContentFormat != "text" && in.ContentFormat != "markdown" && in.ContentFormat != "html" {
	return middleware.NewAppError(middleware.CodeValidationFailed, "content_format 仅支持 text / markdown / html").WithField("content_format")
}
```

- [ ] **5b: Create 写入 ContentFormat**

在 `Create` 函数构造 Reminder 时，在第 92-106 行附近增加 `ContentFormat`：

```go
r := &models.Reminder{
	Title:                   strings.TrimSpace(in.Title),
	Content:                 in.Content,
	ContentFormat:           in.ContentFormat, // 新增
	// ... 其余不变
```

如果 `in.ContentFormat` 为空，默认用 `"text"`（在 validate 之后，ContentFormat 已经保证合法或为空，这里可以直接用）：

```go
contentFormat := in.ContentFormat
if contentFormat == "" {
	contentFormat = "text"
}
// 然后在 r 的初始化中用 contentFormat
```

- [ ] **5c: Update 写入 ContentFormat**

在 `Update` 的 `updates` map 中增加：

```go
updates := map[string]any{
	"title":                      strings.TrimSpace(in.Title),
	"content":                    in.Content,
	"content_format":             in.ContentFormat, // 新增
	// ... 其余不变
```

同样为空时默认处理同上。

- [ ] **5d: ReminderInput 新增 ContentFormat**

在 `ReminderInput` 结构体中增加：

```go
type ReminderInput struct {
	Title                   string         `json:"title"`
	Content                 string         `json:"content"`
	ContentFormat           string         `json:"content_format"` // 新增
	// ... 其余不变
```

---

### Task 6: 前端类型定义

**文件：**
- 修改: `frontend/src/types/index.ts`

- [ ] **6a: 新增 ContentFormat 类型 + Reminder/ReminderInput 接口字段**

```typescript
export type ContentFormat = 'text' | 'markdown' | 'html'

export interface Reminder {
  // ... 现有字段
  content_format: ContentFormat
}

export interface ReminderInput {
  // ... 现有字段
  content_format?: ContentFormat
  // ^ 可选，因为编辑已有数据时可能后端不返回
}
```

---

### Task 7: 前端表单 — 新增格式切换器

**文件：**
- 修改: `frontend/src/components/reminders/ReminderEditDialog.tsx`

- [ ] **7a: 导入 Tabs 组件**

```typescript
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
```

确认该组件是否存在（shadcn/ui 默认包含 `tabs`）。

- [ ] **7b: defaultInput 增加默认值**

```typescript
function defaultInput(): ReminderInput {
  return {
    // ... 现有
    content_format: 'text',
  }
}
```

- [ ] **7c: 编辑时回填**

在 `useEffect` 的 `setInput` 中增加：

```typescript
setInput({
  // ... 现有
  content_format: (reminder.content_format as ContentFormat) || 'text',
})
```

- [ ] **7d: 内容区域上方增加格式切换**

将内容输入框区域（约第 166-194 行）修改为：

```tsx
<div className="space-y-2">
  <div className="flex items-center justify-between">
    <div className="flex items-center gap-1.5">
      <Label htmlFor="r-content">内容</Label>
      <div className="group relative inline-flex">
        <button
          type="button"
          className="rounded-sm p-0.5 text-muted-foreground transition-colors hover:text-foreground"
          aria-label="查看可用占位符"
        >
          <Info className="h-3.5 w-3.5" />
        </button>
        <div className="pointer-events-none absolute left-full top-1/2 z-50 ml-2 hidden w-64 -translate-y-1/2 rounded-md border bg-popover p-3 text-xs text-popover-foreground shadow-md group-hover:block">
          <div className="space-y-1">
            <p className="font-medium">可用占位符</p>
            <p><code>{'{{now}}'}</code> 当前日期时间</p>
            <p><code>{'{{now_date}}'}</code> 当前日期</p>
            <p><code>{'{{lunar_date}}'}</code> 当前农历日期</p>
          </div>
        </div>
      </div>
    </div>
    <Tabs
      value={input.content_format || 'text'}
      onValueChange={(v) => patch('content_format', v as ContentFormat)}
      className="w-auto"
    >
      <TabsList className="h-8">
        <TabsTrigger value="text" className="text-xs px-3">纯文本</TabsTrigger>
        <TabsTrigger value="markdown" className="text-xs px-3">Markdown</TabsTrigger>
        <TabsTrigger value="html" className="text-xs px-3">HTML</TabsTrigger>
      </TabsList>
    </Tabs>
  </div>
  <Textarea
    id="r-content"
    value={input.content}
    onChange={(e) => patch('content', e.target.value)}
    rows={3}
    placeholder={'例如：今天是 {{now_date}}，{{lunar_date}}'}
  />
</div>
```

需要导入 `ContentFormat` 类型。

---

### Task 8: 编译验证

- [ ] **8a: 后端编译**

```bash
cd backend && go build ./...
```

预期：无错误通过。

- [ ] **8b: 前端编译**

```bash
cd frontend && npm run build
```

预期：无错误通过。

---

### Task 9 (可选): 提醒列表展示格式标签

**文件：**
- 修改: `frontend/src/pages/reminders/index.tsx`

- [ ] **9a: 表格增加"格式"列**

在表格行中，在"类型"列后增加一列（或整合到详情列中），展示 `content_format`：

```tsx
// 在格式化函数附近增加
const FORMAT_LABEL: Record<string, string> = {
  text: '文本',
  markdown: 'MD',
  html: 'HTML',
}
```

在表格的表头和数据行中增加一列：

```tsx
// 表头：在"类型"后面
<th className="px-4 py-2.5">格式</th>

// 数据行
<td className="px-4 py-2.5">
  <span className="text-xs px-1.5 py-0.5 rounded bg-muted">
    {FORMAT_LABEL[r.content_format] ?? r.content_format ?? '文本'}
  </span>
</td>
```

---

## 验证方法

1. **后端构建通过** — `cd backend && go build ./...`
2. **前端构建通过** — `cd frontend && npm run build`
3. **新建提醒测试** — 分别用三种格式创建，观察 DB 中 `content_format` 写入正确
4. **已有数据兼容** — 历史提醒的 `content_format` 自动为 "text"，行为不变（default 约束保证）
5. **触发送达测试** — 用 test 触发，分别检查各通道收到内容格式：
   - text 格式 → 邮件看到纯文本
   - markdown 格式 → 邮件渲染为 HTML
   - html 格式 → 钉钉/企微降级为纯文本
