# 提醒内容格式支持

## 背景

当前所有提醒内容的 `Title` 和 `Content` 均为纯文本，各通道统一发送相同内容。用户需要按需求选择内容格式（text / markdown / html），各通道根据自身能力进行适配或降级。

## 核心规则

1. 用户在创建/编辑提醒时选择内容格式，默认 `text`
2. 每个 Notifier（通道发送器）自己负责格式转换和适配
3. 通道不支持所选格式时，自动降级到 text（strip 标记/标签后纯文本输出）
4. Webhook 通道不做自动转换，由用户的 body_template 自行处理

## 存储变更

### reminders 表

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| content_format | varchar(8) | text | text / markdown / html |

## API 变更

POST/PUT `/api/reminders` 和 GET 响应中增加 `content_format` 字段，External API 同样支持。

## Notifier 格式适配规则

| 通道 | text | markdown | html |
|------|------|----------|------|
| SMTP 邮件 | text/plain | 转 HTML 发 multipart | 直接发 text/html |
| 钉钉 | msgtype: text | msgtype: markdown | strip→text |
| 企业微信 | text 消息 | markdown 消息 | strip→text |
| Webhook | 用户模板自控 | 用户模板自控 | 用户模板自控 |
| Log | 直接输出 | strip 标记输出 | strip 标签输出 |

钉钉/企微通道配置中的 `msg_type` 字段不再使用（保留兼容），格式完全由提醒的 content_format 决定。

## 影响文件清单

### 后端
- `backend/internal/models/reminder.go` — Reminder 新增 ContentFormat
- `backend/internal/models/delivery.go` — DeliveryLog 新增 ContentFormat
- `backend/internal/notifier/notifier.go` — Message 新增 Format 字段
- `backend/internal/notifier/format.go`（新增）— StripHTML / StripMarkdown 工具
- `backend/internal/notifier/smtp.go` — 按格式切换邮件类型
- `backend/internal/notifier/dingtalk.go` — 按格式决定 msgtype
- `backend/internal/notifier/wecom.go` — 按格式决定 msgtype
- `backend/internal/notifier/log.go` — 按格式 strip 输出
- `backend/internal/services/dispatch_service.go` — 传递 Format
- `backend/internal/services/reminder_service.go` — 校验 content_format

### 前端
- `frontend/src/types/index.ts` — 新增 ContentFormat 类型
- `frontend/src/components/reminders/reminder-form.tsx` — 格式切换 UI
- `frontend/src/pages/reminders/index.tsx` — 列表展示格式标签