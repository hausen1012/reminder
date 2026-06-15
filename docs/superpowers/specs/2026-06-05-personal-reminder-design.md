# 个人提醒助手 / 通知秘书 — 设计文档

- 日期：2026-06-05
- 基础项目：bedrock（Go + Gin + GORM + SQLite + React + TS + Tailwind + shadcn）
- 部署形态：单二进制 + 单 SQLite + 单进程，沿用 bedrock 的全栈一体化方式

## 1. 目标与范围

构建一个个人使用为主、同时对外开放 API 的提醒/通知系统。核心功能：

- 在指定时间触发提醒，通过一个或多个通道发送
- 支持单次、周期（分/时/天/月/年）、cron、农历四种调度形态
- 多类型通道：SMTP、钉钉机器人、企业微信机器人、Webhook（GET/POST）
- 可选"需要确认"机制：未点击确认链接前按配置重发
- 失败通道自动重试
- 完整发送日志，可按 7 天 / 30 天 / 全部清理
- 外部 API：通过 API Key 创建提醒，未指定通道则回退到 Key 默认通道
- 区分手动 / API 来源，列表默认只看手动

明确不做：多用户隔离、国际化、Prometheus 监控、数据备份/导出、集群部署。

---

## 2. 整体架构

### 2.1 后端分层

```
Gin Router
  ├── /api/auth/*            登录、鉴权（沿用 bedrock）
  ├── /api/reminders         提醒 CRUD、启用/禁用、试发、预览下次触发
  ├── /api/channels          通道 CRUD、试发
  ├── /api/logs              日志查询、清理
  ├── /api/apikeys           API Key 管理
  ├── /api/ingest/*          外部 API（X-API-Key 鉴权）
  └── /c/:token              确认链接（无需登录，一次性 token）

Service 层
  ├── ReminderService        增删改查、与 Scheduler 同步
  ├── ChannelService         通道配置、敏感字段加解密
  ├── DispatchService        模板渲染 → 多通道并发发送 → 写日志 → 重试
  ├── LogService             查询、过滤、清理
  ├── ApiKeyService          生成、哈希存储、默认通道绑定
  └── ConfirmService         token 生成、校验、确认链路

Scheduler 层
  ├── Engine                 trigger 接口 + cron/once/lunar 三种实现
  ├── Registry               reminder_id → 已注册 entry 的内存映射
  ├── Sweeper                每分钟补漏扫表 goroutine
  ├── ConfirmRetryManager    未确认重发链（AfterFunc 链式）
  └── 不引入持久化队列

Notifier 层
  ├── Notifier interface     Send(ctx, channel, rendered) error
  ├── notifier_smtp.go
  ├── notifier_dingtalk.go
  ├── notifier_wecom.go
  └── notifier_webhook.go    GET / POST
```

### 2.2 后端目录结构（新增）

```
backend/internal/
  models/
    user.go                          已存在
    reminder.go                      Reminder + ScheduleSpec JSON
    channel.go                       Channel + 加密字段
    delivery.go                      DeliveryLog
    delivery_attempt.go              DeliveryAttempt
    apikey.go                        APIKey + APIKeyDefaultChannel
    confirm_token.go                 ConfirmToken
  scheduler/
    engine.go                        Engine + Job + Registry
    trigger_cron.go                  cron / once / interval 触发器
    trigger_lunar.go                 农历自调度循环
    sweeper.go                       补漏 goroutine
    confirm_retry.go                 ConfirmRetryManager
    nextfire.go                      纯函数：从 ScheduleSpec 算下一次公历时间
  notifier/
    notifier.go                      接口 + 工厂
    smtp.go / dingtalk.go / wecom.go / webhook.go
    template.go                      变量替换
  services/
    reminder_service.go
    channel_service.go
    dispatch_service.go
    log_service.go
    apikey_service.go
    confirm_service.go
  handlers/
    reminder.go / channel.go / log.go / apikey.go / ingest.go / confirm.go
  crypto/
    secretbox.go                     AES-GCM，含硬编码 fallback key
  middleware/
    apikey.go                        X-API-Key 校验 + 限流
```

### 2.3 前端目录结构（新增）

```
frontend/src/pages/
  reminders/
    index.tsx                        列表 + 筛选
    edit.tsx                         创建 / 编辑
  channels/
    index.tsx
    edit.tsx
  logs/
    index.tsx                        列表 + 详情抽屉 + 清理
  apikeys/
    index.tsx                        列表 + 创建对话框（一次性显示明文）

frontend/src/components/
  reminders/
    ScheduleForm.tsx                 一次性 / 周期 / cron / 农历 4 个 Tab
    LunarPicker.tsx                  公历⇄农历日期选择器
    NextFirePreview.tsx              输入变化时调后端"试算下 5 次"
  channels/
    ChannelTypeForm.tsx              按 type 切换不同表单
```

### 2.4 单一事实源原则

所有调度状态的真相在 DB（`reminders.next_fire_at`）。Scheduler 内的 Registry 是缓存。进程重启 = 重建 Registry。补漏扫描兜底进程切换或时间跳变。

---

## 3. 数据模型

### 3.1 `reminders`

```go
type Reminder struct {
  ID            uint           `gorm:"primaryKey"`
  Title         string         `gorm:"size:200;not null"`
  Content       string         `gorm:"type:text"`

  Calendar      string         `gorm:"size:8;index"`       // solar | lunar
  ScheduleType  string         `gorm:"size:16;index"`      // once | interval | cron
  ScheduleSpec  datatypes.JSON `gorm:"type:text"`          // 见 3.2
  Timezone      string         `gorm:"size:48;default:Asia/Shanghai"`

  Enabled       bool           `gorm:"index;default:true"`
  Source        string         `gorm:"size:16;index"`      // manual | api
  APIKeyID      *uint          `gorm:"index"`

  RequireConfirm          bool   `gorm:"default:false"`
  ConfirmRetryIntervalSec int    `gorm:"default:0"`
  ConfirmMaxRetries       int    `gorm:"default:0"`

  NextFireAt    *time.Time     `gorm:"index"`              // 公历 UTC
  LastFiredAt   *time.Time
  FireCount     int

  CreatedAt, UpdatedAt time.Time
  DeletedAt     sql.NullTime   `gorm:"index"`              // 软删
}
```

### 3.2 ScheduleSpec 各形态

合法组合：

| Calendar | ScheduleType | 含义 |
|---|---|---|
| `solar` | `once` | 公历某时刻单次 |
| `solar` | `interval` | 公历起点 + 每 N 分/时/天/月/年 |
| `solar` | `cron` | 标准 5 字段 cron |
| `lunar` | `once` | 农历某年月日 + 时分单次 |
| `lunar` | `interval` | 农历起点 + 每 N 天/月/年，时分独立 |

禁用 `lunar` + `cron`。

JSON 形态：

| 组合 | JSON |
|---|---|
| `solar` + `once` | `{ "at": "2026-06-10T09:00:00" }` |
| `solar` + `interval` | `{ "start_at": "2026-06-05T09:00:00", "every": 10, "unit": "minute\|hour\|day\|month\|year" }` |
| `solar` + `cron` | `{ "expr": "0 9 * * MON-FRI" }` |
| `lunar` + `once` | `{ "lunar": { "year": 2026, "month": 7, "day": 7 }, "hour": 9, "minute": 0 }` |
| `lunar` + `interval` | `{ "start_lunar": { "year": 2026, "month": 7, "day": 7 }, "hour": 9, "minute": 0, "every": 1, "unit": "day\|month\|year", "leap_policy": "skip", "size_policy": "shift" }` |

农历相关字段说明：
- `hour` / `minute` 总是从 spec 取，不从 `start_lunar` 派生（即"日期靠计算、时分用设置"）
- `leap_policy = skip`：闰月跳过，仅在正常月触发（初版固定 skip，UI 不可改）
- `size_policy = shift`：候选日在小月不存在时顺延到下月初一（初版固定 shift，UI 不可改）

### 3.3 `channels`

```go
type Channel struct {
  ID        uint           `gorm:"primaryKey"`
  Name      string         `gorm:"size:64;not null;uniqueIndex"`
  Type      string         `gorm:"size:16;index"`     // smtp | dingtalk | wecom | webhook
  Enabled   bool           `gorm:"default:true"`
  Config    datatypes.JSON `gorm:"type:text"`         // 见下，敏感字段加密
  CreatedAt, UpdatedAt time.Time
}
```

按 Type 解析 Config：

- `smtp`：`{ host, port, username, password_enc, from, to[], use_tls, use_html }`
- `dingtalk`：`{ webhook_url, secret_enc, at_mobiles[], msg_format: "text|markdown" }`
- `wecom`：`{ webhook_url, msg_format }`
- `webhook`：`{ url, method: "GET|POST", headers{}, content_type, body_template }`

`_enc` 后缀字段在 ChannelService 保存/加载时透明加解密。Type 创建后不可修改（避免 Config schema 漂移），需要换类型则重建。

### 3.4 `reminder_channels`（多对多）

```go
type ReminderChannel struct {
  ReminderID uint `gorm:"primaryKey"`
  ChannelID  uint `gorm:"primaryKey"`
}
```

### 3.5 `delivery_logs`

```go
type DeliveryLog struct {
  ID             uint       `gorm:"primaryKey"`
  ReminderID     uint       `gorm:"index"`
  FiredAt        time.Time  `gorm:"index"`
  Title          string                            // 触发时的快照
  Content        string     `gorm:"type:text"`
  Status         string     `gorm:"size:16;index"` // pending | success | partial | failed | expired
  Confirmed      bool       `gorm:"index;default:false"`
  ConfirmedAt    *time.Time
  ConfirmChainID *string    `gorm:"size:32;index"` // 同一确认重发链共享
  RetryRound     int                               // 0 = 第一轮，>0 = 第 N 次重发
  Source         string     `gorm:"size:16;index"`
  CreatedAt      time.Time
}
```

### 3.6 `delivery_attempts`

```go
type DeliveryAttempt struct {
  ID            uint      `gorm:"primaryKey"`
  DeliveryLogID uint      `gorm:"index"`
  ChannelID     uint      `gorm:"index"`
  ChannelType   string    `gorm:"size:16"`        // 冗余，通道删除后仍可展示
  ChannelName   string    `gorm:"size:64"`        // 冗余
  Attempt       int                                // 第几次尝试，从 1 起
  Status        string    `gorm:"size:16"`        // success | failed
  Error         string    `gorm:"type:text"`
  LatencyMs     int
  CreatedAt     time.Time
}
```

### 3.7 `api_keys` / `api_key_default_channels`

```go
type APIKey struct {
  ID         uint       `gorm:"primaryKey"`
  Name       string     `gorm:"size:64;not null"`
  KeyHash    string     `gorm:"size:64;uniqueIndex"`  // sha256(明文)
  Prefix     string     `gorm:"size:8"`               // bdrk_xxx 前 8 位
  Enabled    bool       `gorm:"default:true"`
  LastUsedAt *time.Time
  CreatedAt  time.Time
}

type APIKeyDefaultChannel struct {
  APIKeyID  uint `gorm:"primaryKey"`
  ChannelID uint `gorm:"primaryKey"`
}
```

Key 明文格式：`bdrk_` + 24 字节 base62，明文仅创建时一次性返回。

### 3.8 `confirm_tokens`

```go
type ConfirmToken struct {
  Token         string    `gorm:"size:64;primaryKey"`  // 32 字节随机 hex
  DeliveryLogID uint      `gorm:"index"`
  ExpiresAt     time.Time `gorm:"index"`
  UsedAt        *time.Time
}
```

### 3.9 SQLite 并发与连接参数

DSN：`?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000`

`db.SetMaxOpenConns(1)`，写串行化、读复用 WAL。事务在 service 层内聚，不嵌套。

`Scheduler.fire` 内的"更新 NextFireAt + 创建 delivery_log"必须在同一短事务里，配合乐观锁防补漏重复触发（见 4.2）。

---

## 4. 调度器

### 4.1 Engine 抽象

```go
type Engine struct {
  cron     *cron.Cron
  reg      map[uint]registered
  mu       sync.Mutex
  dispatch DispatchService
  store    ReminderStore
  loc      *time.Location
  stop     chan struct{}
}

type registered struct {
  kind    string         // "cron" | "afterfunc"
  entryID cron.EntryID
  timer   *time.Timer
}

func (e *Engine) Add(r *Reminder)
func (e *Engine) Update(r *Reminder)   // = Remove + Add
func (e *Engine) Remove(id uint)
```

### 4.2 分发表

```
solar/cron       → cron.AddFunc(expr, fire)        kind=cron
solar/once       → AfterFunc(at - now, fire)       kind=afterfunc
solar/interval   → AfterFunc(next - now, fire)     kind=afterfunc
lunar/once       → AfterFunc(next - now, fire)     kind=afterfunc
lunar/interval   → AfterFunc(next - now, fire)     kind=afterfunc
```

`fire` 闭包：

```
fire(reminderID):
  1. 重读最新 reminder，若不存在或 !enabled → return
  2. 事务：
       UPDATE reminders SET last_fired_at=now,
              next_fire_at = nextfire.Compute(spec, now),
              fire_count = fire_count + 1
        WHERE id=? AND next_fire_at=?           // 乐观锁
       INSERT delivery_logs(reminder_id, fired_at=now, status='pending', source, retry_round=0, ...)
     若 UPDATE 影响 0 行 → 视为别的路径（补漏）已处理，return
  3. 异步 go dispatch.Run(reminder, logID)（不阻塞调度线程）
  4. 若 schedule_type != cron → reschedule(reminder)
```

### 4.3 Registry 同步

```
ReminderService.Create  → DB Insert → Engine.Add
ReminderService.Update  → DB Update → Engine.Update
ReminderService.Delete  → DB Delete → Engine.Remove
ReminderService.Toggle  → DB Update → Engine.Update（禁用时只 Remove）
```

### 4.4 Sweeper（补漏）

每 60s（环境变量 `SWEEP_INTERVAL_SEC`）一次：

```sql
SELECT id FROM reminders
WHERE enabled = 1
  AND deleted_at IS NULL
  AND next_fire_at IS NOT NULL
  AND next_fire_at <= datetime('now', '-30 seconds')
  AND next_fire_at >= datetime('now', ?)        -- MISS_TOLERANCE_MINUTES
```

`-30s` 缓冲让正常调度器先处理；早于 `MISS_TOLERANCE_MINUTES`（默认 60 分钟）的判定为 expired，标记 `delivery_logs.status='expired'` 但不发送。

逐条调 `Engine.fire(id)`，乐观锁防重复。

### 4.5 启动序列

```
1. db.Init()
2. 加载所有 enabled 提醒
3. engine := NewEngine(...)
4. for r := range reminders: engine.Add(r)
5. engine.cron.Start()
6. go sweeper.Run()
```

### 4.6 进程重启契约

- 不丢提醒：DB 是事实源
- 漏触发自动补发，但超 `MISS_TOLERANCE_MINUTES` 则标记 expired 不发
- 确认重发链不持久化，重启即丢失（接受）

### 4.7 时区

全局 `cfg.Timezone`（默认 `Asia/Shanghai`），用于：
- 解析用户输入的时间字符串
- 解析 cron 表达式
- 农历转公历
- API 响应里的 `*_local` 字段格式化

DB 存 UTC。

---

## 5. 下一次触发时间计算

`nextfire.Compute(spec, after time.Time) (*time.Time, error)`：

```
switch (calendar, schedule_type):
  solar/once     → at < after ? nil : at
  solar/interval → 从 start_at 累加 every*unit，直到 > after
  solar/cron     → cron.ParseStandard(expr).Next(after)
  lunar/once     → lunar_to_solar(year, month, day) + (hour:minute)
                   若 < after → nil
  lunar/interval → 循环：
                   候选农历日 = lunar_add(start_lunar, k * every, unit)
                   若是闰月 (leap_policy=skip) → k++ continue
                   若该日不存在 (size_policy=shift) → 顺延到下月初一
                   候选公历日 = lunar_to_solar(候选农历日)
                   候选时间 = 候选公历日 + spec.hour:minute
                   若 > after → return
                   k++
```

农历转换库：`github.com/6tail/lunar-go`。Cron 解析库：`github.com/robfig/cron/v3`。

---

## 6. 通知模板与变量

### 6.1 模板归属

模板写在提醒级（Title + Content），通道侧不存模板，由 Notifier 内部决定如何映射。

### 6.2 渲染引擎

`Render(tmpl string, vars map[string]string) string`：
- 简单 `{{name}}` 替换，不支持表达式/条件/循环
- 不引入 `text/template`（避免 Go 模板的 `.X` 语义误伤）
- 未定义变量保留原样

### 6.3 内置变量

| 变量 | 内容 |
|---|---|
| `{{title}}` | 渲染后的标题 |
| `{{content}}` | 渲染后的正文 |
| `{{trigger_at}}` | 触发时刻，配置时区 `YYYY-MM-DD HH:mm` |
| `{{trigger_date}}` | `YYYY-MM-DD` |
| `{{trigger_time}}` | `HH:mm` |
| `{{lunar_date}}` | 当日农历，如 "丙午年五月十一" |
| `{{weekday}}` | 星期几 |
| `{{reminder_id}}` | 提醒 ID |
| `{{source}}` | manual / api |
| `{{confirm_url}}` | 确认链接（仅 RequireConfirm 时注入，否则空串） |

渲染顺序：先准备原子变量，再渲染 Title/Content；Title/Content 内的 `{{title}}` / `{{content}}` 自引用不递归。

### 6.4 通道适配规则

| 通道 | Title | Content |
|---|---|---|
| SMTP | Subject | Body（HTML 或文本，看 use_html） |
| 钉钉 text | `{title}\n\n{content}` 单段 | — |
| 钉钉 markdown | `markdown.title` | `markdown.text = "# {title}\n\n{content}"`，at_mobiles 来自 config |
| 企微 text | 同钉钉 text | — |
| 企微 markdown | `"## {title}\n\n{content}"` 进 `markdown.content` | — |
| Webhook GET | URL query `?title=...&content=...&trigger_at=...`（全变量 URL-encode） | — |
| Webhook POST | 默认 body `{"title":..., "content":..., "vars":{所有变量}}`；若 config.body_template 非空则渲染该模板 | — |

### 6.5 转义规则

- 钉钉/企微 markdown：替换反引号、行首 `#`
- Webhook POST + body_template：用户自负 JSON 合法性，不额外转义
- Webhook GET：所有变量值 URL-encode

### 6.6 试发

通道列表"测试发送"：用占位变量（title="测试标题"、content="测试内容"、trigger_at=now）跑完整渲染 + 发送，结果返回前端，**不写日志**。

---

## 7. 通道发送与重试

### 7.1 Notifier 接口

```go
type Notifier interface {
  Type() string
  Send(ctx context.Context, ch *Channel, r *Rendered) error
}

var registry = map[string]Notifier{
  "smtp":     &SMTPNotifier{},
  "dingtalk": &DingTalkNotifier{},
  "wecom":    &WeComNotifier{},
  "webhook":  &WebhookNotifier{},
}
```

### 7.2 DispatchService

```
Run(reminder, deliveryLogID):
  vars     = buildVars(reminder, now, deliveryLogID)
  rendered = { Title: Render(r.Title), Content: Render(r.Content), Vars: vars }
  channels = loadChannels(reminder)             // 见 7.6
  并发执行：每条 channel 调 attemptWithRetry(ctx, ch, rendered, deliveryLogID)
  汇总：
    全成功 → status=success
    部分  → partial
    全失败 → failed
```

通道间并发，通道内串行重试。整个 dispatch 总超时 60s。

### 7.3 重试策略

```
MaxAttempts = 3
Backoff     = [0, 10s, 30s]
```

每次尝试无论成败写一条 `delivery_attempts`。重试在 dispatch goroutine 内同步等待，不开独立队列。

单次 Send 超时：SMTP/Webhook 15s，钉钉/企微 10s。

不重试错误：Notifier 返回 `ErrPermanent`（4xx 客户端错、配置错），直接 failed，跳过剩余尝试。

### 7.4 各 Notifier 要点

**SMTP：** `net/smtp` + STARTTLS（按 use_tls），收件人 = config.to[]

**钉钉：** POST JSON 到 webhook_url；若 secret_enc 非空则 URL 追加 `&timestamp=&sign=`（HMAC-SHA256，钉钉官方算法）；响应 `errcode != 0` → ErrPermanent

**企微：** POST JSON；响应 `errcode != 0` → ErrPermanent

**Webhook：**
- GET：拼 query，无 body
- POST：按 content_type（默认 application/json）发；body 见 6.4
- 自定义 headers 全部追加
- HTTP 2xx 成功；4xx ErrPermanent；5xx/网络错可重试

### 7.5 敏感字段加密

`crypto/secretbox.go`：AES-GCM，提供 `Encrypt(plain)` / `Decrypt(cipher)`。

密钥来源优先级：
1. 环境变量 `ENCRYPTION_KEY`（base64，32 字节）
2. 否则使用 `crypto/secretbox.go` 内置硬编码常量

注释说明"生产部署建议设置 ENCRYPTION_KEY 环境变量"。**启动不报错。**

### 7.6 通道回退规则

```
loadChannels(reminder):
  if reminder.Source == "api" && reminder 没绑通道：
     return APIKey.DefaultChannels
  else:
     return reminder.Channels
```

手动提醒必须至少绑一个通道（前后端双校验）。API 提醒允许空（走 Key 默认）。

---

## 8. 确认机制

### 8.1 触发时

`RequireConfirm = true` 时，`DispatchService` 在渲染变量前：

```go
token := randHex(32)
db.Create(&ConfirmToken{
  Token:         token,
  DeliveryLogID: logID,
  ExpiresAt:     time.Now().Add(durationOfChain(reminder)),
})
vars["confirm_url"] = cfg.PublicBaseURL + "/c/" + token
```

`ExpiresAt = now + ConfirmRetryIntervalSec * (ConfirmMaxRetries + 1) + 1h`。

### 8.2 `/c/:token` 端点

无需登录，独立于 auth 中间件外：

```
GET /c/:token
  → 查 confirm_tokens：不存在/已用/已过期 → 错误页（极简内嵌 HTML）
  → 事务：标记 token.used_at；更新 delivery_log.confirmed=true, confirmed_at
  → 内嵌成功页：✓ 已确认 · 提醒标题 · 触发时间
```

### 8.3 确认重发链

`ConfirmRetryManager`：

```go
type ConfirmRetryManager struct {
  active map[string]*time.Timer   // chain_id → timer
  mu     sync.Mutex
}
```

流程：

```
触发完第一轮 → 创建 delivery_log(retry_round=0, chain_id=uuid)
              → ConfirmRetryManager.Schedule(reminder, chainID, round=0)

N 秒后回调：
  重读 reminder：!enabled || deleted → 终止
  查 chain_id 下最新 delivery_log：confirmed → 终止
  retry_round + 1 > ConfirmMaxRetries → 终止
  否则：
    再 dispatch 一轮：复用 vars / 复用 confirm_url（同 token）
    新建 delivery_log(retry_round=N+1, chain_id 同前)
    Schedule(round=N+1)
```

链上 confirm_url 全程不变，一次点击即终止整条链。

### 8.4 取消路径

- 用户禁用/删除提醒 → 下次 tick 自检终止
- 用户修改 RequireConfirm = false → 下次 tick 自检终止
- 进程重启 → 重发链丢失（接受的取舍）

### 8.5 关闭确认

`RequireConfirm = false`：不生成 token；`{{confirm_url}}` 渲染为空串；无重发链。

---

## 9. 外部 API

### 9.1 鉴权

Header `X-API-Key: bdrk_<24 字符>`。

中间件流程：
1. 取 header，sha256 → 查 `api_keys`
2. 校验 `enabled = true`
3. 注入 `c.Set("apikey_id", id)`
4. 异步更新 `last_used_at`（节流：每 Key 每分钟最多 1 次写）
5. 缺/错/禁用 → 401

### 9.2 端点

```
POST   /api/ingest/reminders      创建提醒
GET    /api/ingest/reminders/:id  查询单条
GET    /api/ingest/reminders      列出本 Key 创建的提醒（分页）
DELETE /api/ingest/reminders/:id  删除（仅本 Key 创建的）
```

初版不开放 PUT、不开放通道/Key 管理、不开放日志查询。

### 9.3 创建 Payload

```json
{
  "title": "今日提醒",
  "content": "记得吃药 {{trigger_time}}",
  "calendar": "solar",
  "schedule_type": "once",
  "schedule_spec": { "at": "2026-06-10T09:00:00" },
  "timezone": "Asia/Shanghai",
  "channel_ids": [3, 5],
  "require_confirm": false,
  "confirm_retry_interval_sec": 0,
  "confirm_max_retries": 0
}
```

返回：

```json
{
  "id": 142,
  "next_fire_at": "2026-06-10T01:00:00Z",
  "next_fire_at_local": "2026-06-10 09:00",
  "source": "api"
}
```

### 9.4 校验

- title 必填，<= 200
- content 可空，<= 5000
- calendar / schedule_type / schedule_spec 用与面板共用的同一份校验函数与 nextfire.Compute
- channel_ids：每个存在且 enabled；为空且 Key 无默认通道 → 400
- require_confirm=true 时 confirm_retry_interval_sec >= 60 且 confirm_max_retries >= 1
- 禁用 lunar + cron

### 9.5 限流

per-Key per-minute 内存计数器，默认 60/min。超过 → 429 + Retry-After。进程重启计数重置。

### 9.6 错误响应

```json
{ "error": { "code": "validation_failed", "message": "...", "field": "schedule_spec.at" } }
```

`code` 枚举：`unauthorized` / `forbidden` / `not_found` / `validation_failed` / `rate_limited` / `internal_error`

### 9.7 来源区分

- `source` 由后端固定写：内部 API → manual，ingest API → api
- 列表查询 `?source=manual|api|all`（前端默认 manual）
- API Key 详情可链到"本 Key 创建的提醒"过滤视图

### 9.8 文档

启动时挂 `/api/ingest/docs`（无鉴权）静态 HTML，列 4 个端点 + 示例 curl + 字段表。手写，不引 Swagger。

---

## 10. 前端

### 10.1 侧边栏

在 bedrock 现有 Sidebar 上加菜单：

```
首页              Dashboard（改造）
提醒              Reminders
通道              Channels
日志              Logs
API               API Keys
─────
设置              Profile（原样）
```

图标：`Bell` / `Send` / `ScrollText` / `Key`（Lucide）

### 10.2 Dashboard 改造

四张卡：
- 今日待发（未来 24h 下次触发列表，最多 10）
- 最近发送（最近 10 条 delivery_logs，状态彩点）
- 通道健康（每个 enabled 通道近 24h 成功率）
- API Key 调用（每个 Key 近 24h 调用次数）

每卡右上"查看全部"链到对应页。

### 10.3 提醒页

**列表 toolbar：** 搜索 / 来源（手动/API/全部，默认手动）/ 状态 / 新建按钮

**列表列：** 标题 · 日期源徽章 · 类型 · 下次触发 · 通道数 · 状态开关 · 操作

**编辑：** 四个 Tab — `一次性` / `周期` / `Cron` / `农历`

- **一次性：** 公历日期 + 时分；校验晚于 now
- **周期：** 起点 + 每 N 单位
- **Cron：** 单 input + 下次 5 次预览 + 常用例子可点击
- **农历：** 子 Tab `单次`/`周期`，LunarPicker 显示"正月/.../腊月、初一/.../三十"，旁注公历日，时分独立两个 select；周期固定显示 skip+shift 文案不可改

共有字段：title / content（提示可用变量）/ 通道多选 / 需要确认折叠项 / 下次触发预览

保存前端校验 → POST → 列表刷新

### 10.4 通道页

卡片网格：名称 + 类型徽章 + 启用开关 + 试发按钮 + 编辑/删除

编辑表单按 type 动态：
- SMTP: host/port/username/password/from/to[]/use_tls/use_html
- 钉钉: webhook_url/secret/at_mobiles[]/msg_format
- 企微: webhook_url/msg_format
- Webhook: url/method/headers{}/content_type/body_template(仅 POST)

Type 创建后**不可改**。

试发：弹窗输入测试 title/content（默认填好）→ 弹结果。

### 10.5 日志页

**toolbar：** 日期范围 / 状态 / 来源 / 搜索 / 清理菜单

**清理菜单：** 7 天前 / 30 天前 / 全部（红色二次确认对话框，先查统计 N 条）

**列表列：** 触发时间 · 提醒标题 · 来源 · 通道彩点条（绿成功/红失败/灰进行中）· 状态 · 操作

确认重发链按 chain_id 折叠为一行，三角展开看每轮。

**详情抽屉：** 提醒元数据 + 模板渲染后 title/content + delivery_attempts 按 channel 分组列表 + confirm_url 复制按钮（若有）

### 10.6 API Key 页

列表：名称 · 前缀 · 默认通道 · 最近使用 · 24h 调用次数 · 状态开关 · 操作

创建对话框：名称 + 默认通道多选；创建后一次性弹明文 Key + 复制按钮（关闭后不可见）

详情抽屉：链到"本 Key 创建的提醒"过滤视图

### 10.7 时区策略

- DB UTC
- 后端 cfg.Timezone 用于解析输入、cron、农历、API `*_local` 字段格式化
- 前端**只显示**后端返回的本地化字符串，不在前端做时区转换
- API 时间字段双形态：`next_fire_at` (UTC ISO) + `next_fire_at_local` (展示字符串)
- 用户提交也用本地时间字符串（无 Z），后端按 cfg.Timezone 解析

### 10.8 UI 一致性

沿用 shadcn 组件；缺什么补什么（dialog/drawer/select/switch/tabs/table/badge/popover/calendar/tooltip/toast）；中性灰、状态彩点仅用 emerald-500/rose-500/amber-500。

保留暗黑模式，沿用现有 ThemeContext。

---

## 11. 日志清理

### 11.1 手动清理

```
DELETE /api/logs?older_than=7d
DELETE /api/logs?older_than=30d
DELETE /api/logs?all=true
```

事务内顺序：

```sql
DELETE FROM delivery_attempts WHERE delivery_log_id IN (
  SELECT id FROM delivery_logs WHERE fired_at < ?
);
DELETE FROM delivery_logs WHERE fired_at < ?;
DELETE FROM confirm_tokens
WHERE expires_at < datetime('now')
   OR delivery_log_id NOT IN (SELECT id FROM delivery_logs);
```

`all=true` 后跑 `VACUUM`。永不删 reminders / channels / api_keys。

### 11.2 自动清理（可选）

环境变量 `LOG_AUTO_PURGE_DAYS`（默认 0 = 关）。>0 时挂 cron `0 3 * * *`，清理超过 N 天的日志。

### 11.3 软删提醒的日志归属

`reminders.deleted_at` 非空仍保留日志；详情抽屉显示 `[已删除]` 灰色前缀。

---

## 12. 错误处理与可观测性

### 12.1 错误层次

| 层 | 失败时 |
|---|---|
| Notifier | 写 delivery_attempts.error，不向上抛 panic |
| DispatchService | 写 delivery_logs.status，log.Error |
| Scheduler.fire | log.Error，外层 defer recover，依赖 sweeper 兜底 |
| Handler | 统一中间件 → HTTP code + JSON 错误体，不暴露 stack |

### 12.2 后端日志

沿用 gin logger，加结构化前缀：

```
[2026-06-05 09:00:00] [INFO]  scheduler  reminder=42 next_fire=...
[2026-06-05 09:00:01] [INFO]  dispatch   reminder=42 chain=abc123 channels=2
[2026-06-05 09:00:02] [WARN]  notifier   channel=5 type=dingtalk attempt=2 err="..."
[2026-06-05 09:00:05] [ERROR] dispatch   reminder=42 status=partial
```

`LOG_FILE` 默认 stdout。不引入 zap/logrus。

### 12.3 健康检查

`GET /api/health` 沿用 + 补字段：

```json
{
  "status": "ok",
  "scheduler": { "registered": 17, "last_fire_at": "...", "sweeper_running": true },
  "db": { "alive": true }
}
```

### 12.4 性能预算

- 提醒总数：1k 以内
- 触发并发：每秒 < 1
- DispatchService 全局并发上限 32 个 goroutine
- SQLite 写 QPS 约 100-200

不做集群、不做水平扩展。

---

## 13. 测试策略

### 13.1 单元测试

- `nextfire.Compute` 全组合，含农历闰月、大小月边界、农历三月初一、闰四月、腊月三十
- `notifier/template.Render`：变量替换、未定义保留、自引用不递归
- `crypto/secretbox`：加解密往返、`ENCRYPTION_KEY` 缺失时 fallback 行为
- `scheduler.Engine`：mock store + mock dispatch，验证 Add/Update/Remove 后的 Registry 状态、fire 内乐观锁
- API Key sha256 + 限流计数器

### 13.2 集成测试

- 创建提醒 → cron 时间推进 → 触发 → mock notifier 调用 → 日志
- RequireConfirm 链：触发 → 未确认 → 重发 → 点 confirm → 链终止
- API ingest 创建提醒、Key 默认通道回退
- 通道试发不写日志

### 13.3 测试技术栈

标准 `testing` + `httptest`，不引 testify。SQLite 走 `:memory:?cache=shared`。

---

## 14. 部署与环境变量

单二进制（embed 前端 dist），docker-compose 挂 `/data/db` 卷。

| 变量 | 默认值 | 说明 |
|---|---|---|
| PORT | 8765 | 已有 |
| DB_PATH | /data/db/reminder.db | 已有 |
| JWT_SECRET | changeme | 已有 |
| USERNAME | admin | 已有 |
| PASSWORD | admin123 | 已有 |
| ENCRYPTION_KEY | 内置硬编码 fallback | 新增；建议生产覆盖 |
| BASE_URL | http://localhost:8765 | 新增；confirm_url 用 |
| TIMEZONE | Asia/Shanghai | 新增 |
| SWEEP_INTERVAL_SEC | 60 | 新增 |
| MISS_TOLERANCE_MINUTES | 60 | 新增 |
| LOG_AUTO_PURGE_DAYS | 0 | 新增；0 = 关 |
| LOG_FILE | /var/log/reminder | 新增 |

启动时**不**因 `ENCRYPTION_KEY` 缺失退出。

---

## 15. 显式不做

- 多用户隔离（DB 表不带 user_id）
- 国际化（纯中文）
- Prometheus/监控指标
- 数据备份/导出
- 集群、主从、水平扩展
- 提醒附件、富文本编辑器
- SDK（Python/JS 等）

---

## 16. 决策记录

| 决策 | 选择 | 备选 |
|---|---|---|
| 用户模型 | 单用户 + API 调用方 | 多用户 SaaS |
| 通道初版 | SMTP、钉钉、企微、Webhook(GET/POST) | + Bark/Server酱/Telegram |
| 模板归属 | 提醒级统一，通道适配 | 通道级模板 |
| 确认机制 | URL 一次性 token | 面板内确认 / 任一通道成功视为送达 |
| 调度引擎 | robfig/cron + 农历自循环 + sweeper 兜底 | 纯 DB 轮询 |
| 农历闰月 | skip | 也触发 |
| 农历大小月 | shift 顺延 | 跳过 |
| 农历周期时分 | 取 spec.hour/minute | 取 start_lunar 的时分 |
| 通道失败重试 | 3 次 [0,10s,30s] | 不重试 |
| API 范围 | 与面板一致 + Key 默认通道回退 | 仅一次性 |
| 重启重发链 | 丢失（不持久化） | 持久化到 DB |
| 通道类型 | 创建后不可改 | 可改但需 migrate Config |
| ENCRYPTION_KEY | 缺失用硬编码 fallback | 缺失用硬编码 fallback |
| 时区 | 后端格式化、前端只展示 | 前端做时区转换 |
